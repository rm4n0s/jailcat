package install

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// Install writes the planned files (SPEC §9.5, §11.3 step 3) and returns what it wrote,
// with the SHA-256 of each, for the caller to record.
//
// It runs [Check] again first, with owner, so a plan that went stale since the user
// looked at it cannot overwrite anything: a target whose conflict is not overwritable,
// or whose path is not set in allowOverwrite (the paths the user confirmed), stops the
// install with [TagConflictsRemain] before anything is written. Then it creates the
// missing folders (0755; Jailcat's data and homes folders 0700), and writes the files in
// this order: profile, launcher, icon, the private home, and the desktop entry last, so
// the menu never shows an entry whose launcher, icon or home is missing. Each file goes to
// a temporary file in the same folder, is synced, gets its mode and is renamed over its
// target. An existing private home is kept as it is.
//
// If a write fails, Install removes its temporary file and stops. The error's metadata
// "written" lists the paths already replaced, which stay in place.
func Install(ctx context.Context, p *Prepared, owner OwnerFunc, allowOverwrite map[string]bool) ([]File, error) {
	conflicts, err := Check(ctx, p, owner)
	if err != nil {
		return nil, err
	}
	var blocked []string
	for _, c := range conflicts {
		if !c.Overwritable || !allowOverwrite[c.File.Path] {
			blocked = append(blocked, c.File.Path)
		}
	}
	if len(blocked) > 0 {
		return nil, errors.New(TagConflictsRemain, "some targets are taken and were not confirmed for overwriting",
			"paths", blocked)
	}

	if err := ensureFolders(ctx, p); err != nil {
		return nil, err
	}

	files := slices.Clone(p.Files)
	slices.SortStableFunc(files, func(a, b File) int { return installRank(a.Kind) - installRank(b.Kind) })
	var written []File
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, errors.NewErr(tagInstallCanceled, err).SetMetadata("path", f.Path, "written", paths(written))
		}
		if f.Kind == KindHome {
			if err := ensureDir(f.Path, f.Mode); err != nil {
				return nil, withWritten(err, written)
			}
		} else if err := writeAtomic(f); err != nil {
			return nil, withWritten(err, written)
		}
		f.Content = nil
		written = append(written, f)
	}
	return written, nil
}

// paths returns the paths of files.
func paths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// withWritten adds the paths already written to the metadata of err, which is a tagged
// error of this package.
func withWritten(err error, written []File) error {
	if appErr, ok := errors.FromError(err); ok {
		return appErr.SetMetadata("written", paths(written))
	}
	return err
}

// ensureFolders creates the folders the planned files go into: Jailcat's own through
// xdg.EnsureDirs (data and homes with 0700), the user's bin, application and icon
// folders with 0755.
func ensureFolders(ctx context.Context, p *Prepared) error {
	if err := xdg.EnsureDirs(ctx, p.dirs); err != nil {
		return err
	}
	for _, f := range p.Files {
		if f.Kind == KindHome {
			continue
		}
		if err := ensureDir(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ensureDir creates the folder when it is missing, with exactly the mode whatever the
// umask; an existing folder keeps its mode.
func ensureDir(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.NewErr(tagDirCreateFailed, err).SetMetadata("path", path, "step", "stat")
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return errors.NewErr(tagDirCreateFailed, err).SetMetadata("path", path, "step", "mkdir")
	}
	if err := os.Chmod(path, mode); err != nil {
		return errors.NewErr(tagDirCreateFailed, err).SetMetadata("path", path, "step", "chmod")
	}
	return nil
}

// writeAtomic writes f through a temporary file in its folder: write, sync, chmod, then
// rename over the target. A failure removes the temporary file; if that fails too, its
// error is in the metadata "cleanup".
func writeAtomic(f File) (err error) {
	dir := filepath.Dir(f.Path)
	tmp, err := os.CreateTemp(dir, ".jailcat-*")
	if err != nil {
		return errors.NewErr(tagFileWriteFailed, err).SetMetadata("path", f.Path, "step", "create")
	}
	name := tmp.Name()
	defer func() {
		if err == nil {
			return
		}
		if rerr := os.Remove(name); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			if appErr, ok := errors.FromError(err); ok {
				_ = appErr.SetMetadata("cleanup", rerr.Error())
			}
		}
	}()
	if _, werr := tmp.Write(f.Content); werr != nil {
		_ = tmp.Close()
		return errors.NewErr(tagFileWriteFailed, werr).SetMetadata("path", f.Path, "step", "write")
	}
	if serr := tmp.Sync(); serr != nil {
		_ = tmp.Close()
		return errors.NewErr(tagFileWriteFailed, serr).SetMetadata("path", f.Path, "step", "sync")
	}
	if cerr := tmp.Chmod(f.Mode); cerr != nil {
		_ = tmp.Close()
		return errors.NewErr(tagFileWriteFailed, cerr).SetMetadata("path", f.Path, "step", "chmod")
	}
	if cerr := tmp.Close(); cerr != nil {
		return errors.NewErr(tagFileWriteFailed, cerr).SetMetadata("path", f.Path, "step", "close")
	}
	if rerr := os.Rename(name, f.Path); rerr != nil {
		return errors.NewErr(tagFileWriteFailed, rerr).SetMetadata("path", f.Path, "step", "rename")
	}
	return nil
}

const (
	// TagConflictsRemain is the tag of the error [Install] returns when a target is
	// taken and the user did not confirm overwriting it, or may not (SPEC §9.5). The
	// metadata "paths" lists the targets.
	TagConflictsRemain = "ConflictsRemain"

	tagInstallCanceled = "InstallCanceled"
	tagDirCreateFailed = "DirCreateFailed"
	tagFileWriteFailed = "FileWriteFailed"
)
