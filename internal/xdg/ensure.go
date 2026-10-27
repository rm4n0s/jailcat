package xdg

import (
	"context"
	"io/fs"
	"os"

	"github.com/rm4n0s/errors"
)

// EnsureDirs creates Jailcat's own folders when they are missing: Data and Homes with
// mode 0700; Profiles, Imported, Launchers and Cache with mode 0755 (SPEC §3.1). The
// mode is applied to the folder it creates, whatever the umask; a folder that already
// exists keeps its mode. It does not create Bin, Applications or Icons: the installer
// creates them when it writes (step 3.3).
func EnsureDirs(ctx context.Context, d Dirs) error {
	folders := []struct {
		name string
		path string
		mode fs.FileMode
	}{
		{"Data", d.Data, 0o700},
		{"Homes", d.Homes, 0o700},
		{"Profiles", d.Profiles, 0o755},
		{"Imported", d.Imported, 0o755},
		{"Launchers", d.Launchers, 0o755},
		{"Cache", d.Cache, 0o755},
	}
	for _, f := range folders {
		if err := ctx.Err(); err != nil {
			return errors.NewErr(tagEnsureDirsCanceled, err).SetMetadata("folder", f.name)
		}
		info, err := os.Stat(f.path)
		if err == nil {
			if !info.IsDir() {
				return errors.New(tagDirNotADirectory, "a file is where a folder should be",
					"folder", f.name, "path", f.path)
			}
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return errors.NewErr(tagDirStatFailed, err).SetMetadata("folder", f.name, "path", f.path)
		}
		if err := os.MkdirAll(f.path, f.mode); err != nil {
			return errors.NewErr(tagDirCreateFailed, err).SetMetadata("folder", f.name, "path", f.path, "step", "mkdir")
		}
		// MkdirAll applies the umask, so set the mode on the folder just made.
		if err := os.Chmod(f.path, f.mode); err != nil {
			return errors.NewErr(tagDirCreateFailed, err).SetMetadata("folder", f.name, "path", f.path, "step", "chmod")
		}
	}
	return nil
}

const (
	tagEnsureDirsCanceled = "EnsureDirsCanceled"
	tagDirNotADirectory   = "DirNotADirectory"
	tagDirStatFailed      = "DirStatFailed"
	tagDirCreateFailed    = "DirCreateFailed"
)
