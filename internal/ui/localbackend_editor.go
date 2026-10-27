package ui

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// profileSuffix is the end of the name of a firejail profile file.
const profileSuffix = ".profile"

// maxImportSize is the largest profile file ImportProfile copies. Real profiles are a
// few kilobytes; a bigger file is something else.
const maxImportSize = 1 << 20

// maxImportCopies is how many copies of one name ImportProfile tries (name.profile,
// name-2.profile, ...) before it gives up.
const maxImportCopies = 1000

// HomeDir returns the user's home folder.
func (b *LocalBackend) HomeDir() string { return b.dirs.Home }

// HomesDir returns the folder of Jailcat's private homes.
func (b *LocalBackend) HomesDir() string { return b.dirs.Homes }

// ProfileChoices lists the profiles of SPEC §6.1 for the app. A folder that does not
// exist is an empty group.
func (b *LocalBackend) ProfileChoices(ctx context.Context, app apps.App) (ProfileChoices, error) {
	var out ProfileChoices
	none := ProfileChoice{Group: GroupNone, Name: "None", Label: "None"}

	// For this app: the profile firejail itself would pick (the user folder first).
	var forApp ProfileChoice
	if app.BinaryName != "" && filepath.Base(app.BinaryName) == app.BinaryName {
		file := app.BinaryName + profileSuffix
		for _, dir := range []struct{ path, from string }{{b.dirs.FirejailUser, "yours"}, {b.dirs.FirejailSystem, "firejail"}} {
			path := filepath.Join(dir.path, file)
			if isFile(path) {
				forApp = ProfileChoice{Group: GroupForApp, Name: app.BinaryName, Path: path,
					Label: app.BinaryName + " (" + dir.from + ")"}
				break
			}
		}
	}

	// Default: default.profile, the user folder first.
	var def ProfileChoice
	for _, dir := range []string{b.dirs.FirejailUser, b.dirs.FirejailSystem} {
		path := filepath.Join(dir, "default"+profileSuffix)
		if isFile(path) {
			def = ProfileChoice{Group: GroupDefault, Name: "Default", Path: path, Label: "Default"}
			break
		}
	}

	if forApp.Path != "" {
		out.Entries = append(out.Entries, forApp)
	}
	out.Entries = append(out.Entries, none)
	if def.Path != "" {
		out.Entries = append(out.Entries, def)
	}
	for _, g := range []struct {
		group ProfileGroup
		dir   string
	}{{GroupYours, b.dirs.FirejailUser}, {GroupImported, b.dirs.Imported}, {GroupFirejail, b.dirs.FirejailSystem}} {
		paths, err := profilesIn(ctx, g.dir)
		if err != nil {
			return ProfileChoices{}, err
		}
		for _, p := range paths {
			name := profileName(p)
			out.Entries = append(out.Entries, ProfileChoice{Group: g.group, Name: name, Path: p, Label: name})
		}
	}

	switch {
	case forApp.Path != "":
		out.Default = forApp
	case def.Path != "":
		out.Default = def
	default:
		out.Default = none
	}
	return out, nil
}

// profilesIn returns the *.profile files of dir, sorted by name; a folder that does not
// exist has none.
func profilesIn(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr(tagProfileListCancelled, err).SetMetadata("dir", dir)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.NewErr(tagProfileListFailed, err).SetMetadata("dir", dir)
	}
	var paths []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), profileSuffix) || e.Name() == profileSuffix {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if isFile(path) {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		// By profile name, so "tool" comes before "tool-2".
		a, c := strings.ToLower(profileName(paths[i])), strings.ToLower(profileName(paths[j]))
		if a != c {
			return a < c
		}
		return paths[i] < paths[j]
	})
	return paths, nil
}

// isFile reports whether path is a regular file (a link to one counts).
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// ReadBase reads the profile at path with the include rules and the conditions of the
// installed firejail, and computes its base values. The path "" is base None.
func (b *LocalBackend) ReadBase(ctx context.Context, path string) (firejail.Base, *firejail.Profile, error) {
	if path == "" {
		return firejail.BaseNone(), nil, nil
	}
	cfg, err := firejail.ReadConfig(ctx, filepath.Join(b.dirs.FirejailSystem, "firejail.config"))
	if err != nil {
		return firejail.Base{}, nil, err
	}
	profile, err := firejail.Read(ctx, path, firejail.ReadOpts{Dirs: b.dirs, Config: cfg})
	if err != nil {
		return firejail.Base{}, nil, err
	}
	return firejail.BaseValues(profile, b.dirs.Home), profile, nil
}

// ImportProfile copies the profile src into the folder of imported profiles. A name
// that is taken gets "-2", "-3", ... before ".profile". The includes of the profile are
// not copied (SPEC §6.1).
func (b *LocalBackend) ImportProfile(ctx context.Context, src string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.NewErr(tagImportCancelled, err).SetMetadata("src", src)
	}
	name := filepath.Base(src)
	if !strings.HasSuffix(name, profileSuffix) || name == profileSuffix {
		return "", errors.New(tagImportNotProfile, "the file is not a firejail profile: its name must end in .profile",
			"src", src)
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", errors.NewErr(tagImportReadFailed, err).SetMetadata("src", src)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New(tagImportNotProfile, "the profile is not a regular file", "src", src)
	}
	if info.Size() > maxImportSize {
		return "", errors.New(tagImportTooLarge, "the profile file is too large to be a profile",
			"src", src, "size", info.Size(), "max", maxImportSize)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", errors.NewErr(tagImportReadFailed, err).SetMetadata("src", src)
	}
	if err := os.MkdirAll(b.dirs.Imported, 0o755); err != nil {
		return "", errors.NewErr(tagImportWriteFailed, err).SetMetadata("dir", b.dirs.Imported)
	}

	stem := strings.TrimSuffix(name, profileSuffix)
	for n := 1; n <= maxImportCopies; n++ {
		candidate := stem + profileSuffix
		if n > 1 {
			candidate = stem + "-" + strconv.Itoa(n) + profileSuffix
		}
		dst := filepath.Join(b.dirs.Imported, candidate)
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", errors.NewErr(tagImportWriteFailed, err).SetMetadata("dst", dst)
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr == nil {
			werr = cerr
		}
		if werr != nil {
			_ = os.Remove(dst)
			return "", errors.NewErr(tagImportWriteFailed, werr).SetMetadata("dst", dst)
		}
		return dst, nil
	}
	return "", errors.New(tagImportNameExhausted, "too many imported profiles have this name",
		"name", name, "tried", maxImportCopies)
}

// BuildsForApp returns the builds of the app.
func (b *LocalBackend) BuildsForApp(ctx context.Context, app apps.App) ([]store.Build, error) {
	if app.Kind == apps.KindBinary {
		return b.store.ListBuildsForApp(ctx, store.AppKindBinary, app.Binary)
	}
	return b.store.ListBuildsForApp(ctx, store.AppKindDesktop, app.ID)
}

// CreateBuild stores a new build.
func (b *LocalBackend) CreateBuild(ctx context.Context, build store.Build) (store.Build, error) {
	return b.store.CreateBuild(ctx, build)
}

// UpdateBuild stores the editable columns of a build.
func (b *LocalBackend) UpdateBuild(ctx context.Context, build store.Build) (store.Build, error) {
	return b.store.UpdateBuild(ctx, build)
}

// SaveDraft keeps the draft of a build.
func (b *LocalBackend) SaveDraft(ctx context.Context, id int64, d store.Draft) error {
	return b.store.SaveDraft(ctx, id, d)
}

// ClearDraft removes the draft of a build.
func (b *LocalBackend) ClearDraft(ctx context.Context, id int64) error {
	return b.store.ClearDraft(ctx, id)
}

// NewBuildDefaults returns the launcher fields of a new build of the app. The sandbox
// name is the first free one of "firejail-<binary>", "firejail-<binary>-2", ... among
// the saved builds.
func (b *LocalBackend) NewBuildDefaults(ctx context.Context, app apps.App) (firejail.Settings, error) {
	builds, err := b.store.ListBuilds(ctx)
	if err != nil {
		return firejail.Settings{}, err
	}
	taken := map[string]bool{}
	for _, build := range builds {
		taken[build.SandboxName] = true
	}
	return newBuildDefaults(app, taken), nil
}

const (
	tagProfileListCancelled = "ProfileListCancelled"
	tagProfileListFailed    = "ProfileListFailed"
	tagImportCancelled      = "ProfileImportCancelled"
	tagImportNotProfile     = "ProfileImportNotProfile"
	tagImportTooLarge       = "ProfileImportTooLarge"
	tagImportReadFailed     = "ProfileImportReadFailed"
	tagImportWriteFailed    = "ProfileImportWriteFailed"
	tagImportNameExhausted  = "ProfileImportNameExhausted"
)
