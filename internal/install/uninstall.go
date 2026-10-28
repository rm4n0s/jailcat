package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// UninstallOpts are the choices of [Uninstall] (SPEC §13.2).
type UninstallOpts struct {
	// Dirs gives the homes and appdata folders: a private home is removed only when it is
	// inside Dirs.Homes, a VS Code data folder only when it is inside Dirs.AppData.
	Dirs xdg.Dirs
	// DeleteHome removes the private home folder with everything in it. It is off by
	// default: the home is kept.
	DeleteHome bool
	// DeleteAppData removes the VS Code data folder with everything in it. It is off by
	// default: the folder is kept.
	DeleteAppData bool
	// Force lists the paths of edited files the user confirmed removing.
	Force map[string]bool
}

// UninstallResult says what [Uninstall] did, file by file.
type UninstallResult struct {
	// Removed are the files that were removed (a private home among them when it was
	// deleted).
	Removed []File
	// Kept are the files left in place because they changed since Jailcat wrote them
	// (their SHA-256 is not the recorded one) or are not regular files, and were not
	// forced.
	Kept []File
	// Missing are the recorded files that were already gone.
	Missing []File
	// HomeKept is true when a private home exists and was left in place, because
	// DeleteHome was off.
	HomeKept bool
	// AppDataKept is true when a VS Code data folder exists and was left in place,
	// because DeleteAppData was off.
	AppDataKept bool
}

// Uninstall removes the recorded files of a build (SPEC §13.2): the desktop entry first,
// then the icon, the launcher and the profile. A file whose SHA-256 is not the recorded
// one is edited: it is kept and listed in [UninstallResult.Kept], unless its path is in
// opts.Force. A missing file is fine. A private home is removed, with everything in it,
// only with opts.DeleteHome and only when it is inside opts.Dirs.Homes; a VS Code data
// folder likewise with opts.DeleteAppData and opts.Dirs.AppData.
//
// Nothing is removed when a recorded kind is not one of the six ([TagUninstallKindUnknown])
// or a private home or data folder to delete is outside its folder
// ([TagUninstallHomeOutside], [TagUninstallAppDataOutside]). A
// failure to remove stops the run; the error's metadata "removed" lists what was removed.
func Uninstall(ctx context.Context, files []File, opts UninstallOpts) (UninstallResult, error) {
	list := slices.Clone(files)
	for _, f := range list {
		switch f.Kind {
		case KindProfile, KindLauncher, KindDesktop, KindIcon:
		case KindHome:
			if opts.DeleteHome && !folderInside(opts.Dirs.Homes, f.Path) {
				return UninstallResult{}, errors.New(TagUninstallHomeOutside, "the private home folder is not inside Jailcat's homes folder",
					"path", f.Path, "homes", opts.Dirs.Homes)
			}
		case KindAppData:
			if opts.DeleteAppData && !folderInside(opts.Dirs.AppData, f.Path) {
				return UninstallResult{}, errors.New(TagUninstallAppDataOutside, "the VS Code data folder is not inside Jailcat's appdata folder",
					"path", f.Path, "appdata", opts.Dirs.AppData)
			}
		default:
			return UninstallResult{}, errors.New(TagUninstallKindUnknown, "a recorded file has a kind Jailcat does not know",
				"kind", string(f.Kind), "path", f.Path)
		}
	}
	slices.SortStableFunc(list, func(a, b File) int { return uninstallRank(a.Kind) - uninstallRank(b.Kind) })

	var res UninstallResult
	for _, f := range list {
		f.Content = nil
		if err := ctx.Err(); err != nil {
			return res, errors.NewErr(tagUninstallCanceled, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
		}
		info, err := os.Lstat(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			res.Missing = append(res.Missing, f)
			continue
		}
		if err != nil {
			return res, errors.NewErr(tagUninstallStatFailed, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
		}

		if f.Kind == KindHome || f.Kind == KindAppData {
			if (f.Kind == KindHome && !opts.DeleteHome) || (f.Kind == KindAppData && !opts.DeleteAppData) {
				res.HomeKept = res.HomeKept || f.Kind == KindHome
				res.AppDataKept = res.AppDataKept || f.Kind == KindAppData
				continue
			}
			if err := os.RemoveAll(f.Path); err != nil {
				return res, errors.NewErr(tagUninstallRemoveFailed, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
			}
			res.Removed = append(res.Removed, f)
			continue
		}

		if !info.Mode().IsRegular() {
			res.Kept = append(res.Kept, f)
			continue
		}
		file, err := os.Open(f.Path)
		if err != nil {
			return res, errors.NewErr(tagUninstallReadFailed, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
		}
		h := sha256.New()
		_, err = io.Copy(h, file)
		_ = file.Close()
		if err != nil {
			return res, errors.NewErr(tagUninstallReadFailed, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
		}
		if hex.EncodeToString(h.Sum(nil)) != f.SHA256 && !opts.Force[f.Path] {
			res.Kept = append(res.Kept, f)
			continue
		}
		if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return res, errors.NewErr(tagUninstallRemoveFailed, err).SetMetadata("path", f.Path, "removed", paths(res.Removed))
		}
		res.Removed = append(res.Removed, f)
	}
	return res, nil
}

// folderInside reports whether the private home or VS Code data folder at path may be
// deleted: it is a clean path below dir (the homes or the appdata folder), and the folder
// it sits in is, with its symbolic links resolved, dir or a folder below it, so a link
// inside dir cannot lead the removal out of it. The folder itself may be a link: removing
// it removes the link only.
func folderInside(homes, path string) bool {
	if !strictlyInside(homes, path) {
		return false
	}
	realHomes, err := filepath.EvalSymlinks(homes)
	if err != nil {
		// Nothing there to delete, or nothing to compare with: a missing homes folder has
		// no home in it, and the removal below finds the home missing.
		return errors.Is(err, fs.ErrNotExist)
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return realParent == realHomes || strictlyInside(realHomes, realParent)
}

const (
	// TagUninstallKindUnknown is the tag of the error [Uninstall] returns for a recorded
	// file whose kind is not one of the six.
	TagUninstallKindUnknown = "UninstallKindUnknown"
	// TagUninstallHomeOutside is the tag of the error [Uninstall] returns when a private
	// home to delete is not inside the homes folder.
	TagUninstallHomeOutside = "UninstallHomeOutside"
	// TagUninstallAppDataOutside is the tag of the error [Uninstall] returns when a VS Code
	// data folder to delete is not inside the appdata folder.
	TagUninstallAppDataOutside = "UninstallAppDataOutside"

	tagUninstallCanceled     = "UninstallCanceled"
	tagUninstallStatFailed   = "UninstallStatFailed"
	tagUninstallReadFailed   = "UninstallReadFailed"
	tagUninstallRemoveFailed = "UninstallRemoveFailed"
)
