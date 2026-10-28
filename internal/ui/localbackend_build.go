package ui

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// PrepareBuild generates the files of the build and checks their targets. The profile
// carries the ID of the stored build in its header; a build that is not stored yet has 0
// there until Build stores it.
func (b *LocalBackend) PrepareBuild(ctx context.Context, req BuildRequest) (*PreparedBuild, error) {
	fjPath, err := firejail.FindFirejail(b.dirs)
	if err != nil {
		return nil, err
	}
	gen, prepared, err := b.generate(ctx, req, fjPath)
	if err != nil {
		return nil, err
	}
	p := &PreparedBuild{Request: req, Generated: gen, Prepared: prepared, Firejail: fjPath, Owners: map[int64]string{}}

	p.Conflicts, err = install.Check(ctx, prepared, b.ownerFunc(ctx))
	if err != nil {
		return nil, err
	}
	for _, c := range p.Conflicts {
		if c.Kind != install.ConflictOtherBuild {
			continue
		}
		if _, known := p.Owners[c.Owner]; known {
			continue
		}
		// A build that is gone since is simply not named.
		if other, err := b.store.GetBuild(ctx, c.Owner); err == nil {
			p.Owners[c.Owner] = other.SandboxName
		} else if ctx.Err() != nil {
			return nil, err
		}
	}

	if req.Build.ID != 0 {
		recorded, err := b.store.InstalledFiles(ctx, req.Build.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range recorded {
			if !slices.ContainsFunc(prepared.Files, func(n install.File) bool { return n.Path == f.Path }) {
				p.Stale = append(p.Stale, install.File{Kind: install.Kind(f.Kind), Path: f.Path, SHA256: f.SHA256})
			}
		}
	}
	return p, nil
}

// generate writes the profile of the request in memory and prepares the files around it.
func (b *LocalBackend) generate(ctx context.Context, req BuildRequest, fjPath string) (*firejail.Generated, *install.Prepared, error) {
	gen, err := firejail.Generate(firejail.ProfileSpec{
		BuildID: req.Build.ID, BaseProfile: req.BaseProfile, Profile: req.Profile, Base: req.Base,
		Overrides: req.Overrides, ExtraRules: req.ExtraRules, Home: b.dirs.Home,
		VSCode: firejail.NewVSCodeSpec(req.App.Binary, b.dirs.AppData, b.dirs.ConfigHome, req.Settings.SandboxName),
	})
	if err != nil {
		return nil, nil, err
	}
	prepared, err := install.Prepare(ctx, b.dirs, install.Request{
		BuildID: req.Build.ID, App: req.App, Settings: req.Settings, Profile: gen,
		Firejail: fjPath, IconTheme: b.iconTheme(ctx),
	})
	if err != nil {
		return nil, nil, err
	}
	return gen, prepared, nil
}

// ownerFunc tells install.Check which build records a path.
func (b *LocalBackend) ownerFunc(ctx context.Context) install.OwnerFunc {
	return func(path string) (int64, string, bool, error) {
		f, ok, err := b.store.FileOwner(ctx, path)
		if err != nil || !ok {
			return 0, "", false, err
		}
		return f.BuildID, f.SHA256, true, nil
	}
}

// buildRowOf is the build row the request describes: the columns Build writes.
func buildRowOf(req BuildRequest) store.Build {
	built := req.Build
	built.SandboxName = req.Settings.SandboxName
	built.AppKind, built.DesktopFileID = store.AppKindDesktop, req.App.ID
	if req.App.Kind == apps.KindBinary {
		built.AppKind, built.DesktopFileID = store.AppKindBinary, ""
	}
	built.BinaryPath = req.App.Binary
	built.BaseProfile = req.BaseProfile
	built.Overrides = req.Overrides
	built.ExtraRules = slices.Clone(req.ExtraRules)
	built.IconMode = store.IconModeBadge
	if req.Settings.Icon == firejail.IconRandom {
		built.IconMode = store.IconModeRandom
	}
	built.IconSeed = req.Settings.IconSeed
	built.Draft = nil
	return built
}

// Build installs the prepared build. A build that is not stored yet is stored first so
// that its ID is in the header of the files; if the install then fails, that row is
// removed again. Writing the files, recording them and the check follow SPEC §11.3.
func (b *LocalBackend) Build(ctx context.Context, p *PreparedBuild, allowOverwrite map[string]bool) (BuildResult, error) {
	version, err := firejail.Version(ctx, p.Firejail)
	if err != nil {
		return BuildResult{}, err
	}

	req := p.Request
	prepared, gen := p.Prepared, p.Generated
	created := false
	if req.Build.ID == 0 {
		stored, err := b.store.CreateBuild(ctx, buildRowOf(req))
		if err != nil {
			return BuildResult{}, err
		}
		created = true
		req.Build = stored
		// The files are made again with the real ID in their headers.
		gen, prepared, err = b.generate(ctx, req, p.Firejail)
		if err != nil {
			b.forget(ctx, stored)
			return BuildResult{}, err
		}
	}

	written, err := install.Install(ctx, prepared, b.ownerFunc(ctx), allowOverwrite)
	if err != nil {
		if created {
			b.forget(ctx, req.Build)
		}
		return BuildResult{}, err
	}

	var removed []install.File
	if len(p.Stale) > 0 {
		res, err := install.Uninstall(ctx, p.Stale, install.UninstallOpts{Dirs: b.dirs})
		if err != nil {
			return BuildResult{}, err
		}
		removed = res.Removed
	}

	if !created {
		if _, err := b.store.UpdateBuild(ctx, buildRowOf(req)); err != nil {
			return BuildResult{}, err
		}
		if err := b.store.ClearDraft(ctx, req.Build.ID); err != nil {
			return BuildResult{}, err
		}
	}
	recorded := make([]store.InstalledFile, len(written))
	for i, f := range written {
		recorded[i] = store.InstalledFile{Kind: string(f.Kind), Path: f.Path, SHA256: f.SHA256}
	}
	if err := b.store.RecordInstall(ctx, req.Build.ID, recorded, version, time.Now()); err != nil {
		if created {
			b.forget(ctx, req.Build)
		}
		return BuildResult{}, err
	}
	stored, err := b.store.GetBuild(ctx, req.Build.ID)
	if err != nil {
		return BuildResult{}, err
	}

	res := BuildResult{Build: stored, Files: written, Removed: removed, IconFallback: prepared.IconFallback}
	res.Check, res.CheckErr = b.check(ctx, p.Firejail, written, req.Settings, gen)
	return res, nil
}

// forget removes a build row that was stored for a Build that then failed, so the next
// try stores the build again. A failure to remove it is logged: the original error is
// the one to report.
func (b *LocalBackend) forget(ctx context.Context, built store.Build) {
	if err := b.store.DeleteBuild(context.WithoutCancel(ctx), built.ID); err != nil {
		b.logger.Error("removing the build that failed to install failed", "build", built.SandboxName, "err", err)
	}
}

// check runs the sandbox of the installed profile with "true" in place of the app (SPEC
// §11.3 step 5). Whatever stops it from telling is returned as the error and logged,
// except the nested sandbox, which the UI explains.
func (b *LocalBackend) check(ctx context.Context, fjPath string, files []install.File, s firejail.Settings, gen *firejail.Generated) (firejail.CheckResult, error) {
	profile := ""
	for _, f := range files {
		if f.Kind == install.KindProfile {
			profile = f.Path
		}
	}
	res, err := firejail.Runner{Firejail: fjPath}.Check(ctx, profile, firejail.CheckOpts{
		IgnorePrivateBin: len(s.PrivateBin) > 0,
		IgnoreCount:      gen.IgnoreCount.Total,
	})
	if err != nil {
		if appErr, ok := errors.FromError(err); !ok || appErr.Tag != firejail.TagNestedSandbox {
			b.logger.Error("checking the new sandbox failed", "profile", profile, "err", err)
		}
		return res, err
	}
	if !res.OK && !res.Skipped {
		b.logger.Warn("the check of the new sandbox did not pass", "profile", profile, "status", res.Status.String(), "output", res.Output)
	}
	return res, nil
}

// Try writes the profile of the request to the cache folder and runs the app in it.
func (b *LocalBackend) Try(ctx context.Context, req BuildRequest) (TrySession, error) {
	fjPath, err := firejail.FindFirejail(b.dirs)
	if err != nil {
		return nil, err
	}
	gen, prepared, err := b.generate(ctx, req, fjPath)
	if err != nil {
		return nil, err
	}
	home := ""
	for _, f := range prepared.Files {
		if f.Kind == install.KindHome {
			home = f.Path
		}
	}
	// The VS Code data folder is the build's own: a Try makes it and its links like Build
	// does, and starts the app with it (SPEC §11.2).
	if err := install.SyncAppData(ctx, prepared.AppData); err != nil {
		return nil, err
	}
	args := req.Settings.ExtraArgs
	if dir := prepared.UserDataDir; dir != "" {
		args = append([]string{firejail.UserDataDirFlag, dir}, args...)
	}
	session, err := firejail.Runner{Firejail: fjPath}.Try(ctx, firejail.TryRequest{
		Name: req.Settings.SandboxName, ProfileText: []byte(gen.String()), Binary: req.App.Binary,
		Args: args, CacheDir: b.dirs.Cache, PrivateHome: home,
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// Uninstall removes the recorded files of the build. The build is removed from the
// database, or kept as "not installed", only when no edited file was left in place or
// the choice says to leave them.
func (b *LocalBackend) Uninstall(ctx context.Context, build store.Build, choice UninstallChoice) (install.UninstallResult, error) {
	recorded, err := b.store.InstalledFiles(ctx, build.ID)
	if err != nil {
		return install.UninstallResult{}, err
	}
	files := make([]install.File, len(recorded))
	for i, f := range recorded {
		files[i] = install.File{Kind: install.Kind(f.Kind), Path: f.Path, SHA256: f.SHA256}
	}
	res, err := install.Uninstall(ctx, files, install.UninstallOpts{Dirs: b.dirs, DeleteHome: choice.DeleteHome, DeleteAppData: choice.DeleteAppData, Force: choice.Force})
	if err != nil {
		return res, err
	}
	if !UninstallDone(res, choice) {
		return res, nil
	}
	if choice.KeepSettings {
		return res, b.store.MarkUninstalled(ctx, build.ID)
	}
	return res, b.store.DeleteBuild(ctx, build.ID)
}

// InstalledFiles returns the files recorded for the build.
func (b *LocalBackend) InstalledFiles(ctx context.Context, buildID int64) ([]store.InstalledFile, error) {
	return b.store.InstalledFiles(ctx, buildID)
}

// BaseProfileMissing reports whether the file at path does not exist. Any other failure to
// look at it (a permission) does not say the profile is gone.
func (b *LocalBackend) BaseProfileMissing(ctx context.Context, path string) bool {
	if path == "" || ctx.Err() != nil {
		return false
	}
	_, err := os.Stat(filepath.Clean(path))
	return errors.Is(err, fs.ErrNotExist)
}
