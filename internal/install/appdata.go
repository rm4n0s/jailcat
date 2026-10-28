package install

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// AppData is what a build does with its VS Code data folder (SPEC §7.1), made by
// [Prepare] for a VS Code app with Full home or Only the folders I choose. The data
// folder is a [KindAppData] file of the plan when Create is set.
type AppData struct {
	// Dir is the data folder, <data>/appdata/<sandbox name>: strictly inside
	// Dirs.AppData.
	Dir string
	// ConfigDir is the user's VS Code config folder. Nothing in it is ever written.
	ConfigDir string
	// Create is set when the data folder is made (Own VS Code data folder is on).
	Create bool
	// Link is set when the shared settings are links to the config folder's (Share
	// settings with my VS Code is on); when it is not, the links Jailcat made are
	// removed.
	Link bool
}

// SyncAppData makes the data folder of the plan and its links: the folder with mode
// 0700 when it is missing, then [AppData] links or unlinks the settings, keybindings and
// snippets. Try uses it; [Install] makes the folder with the other files and the links
// after them. A nil plan does nothing.
func SyncAppData(ctx context.Context, a *AppData) error {
	if a == nil {
		return nil
	}
	if a.Create {
		if err := ensureDir(a.Dir, 0o700); err != nil {
			return err
		}
	}
	return syncLinks(ctx, a)
}

// syncLinks links the shared settings into the data folder, or removes the links Jailcat
// made there.
//
// With Link, for each of settings.json, keybindings.json and snippets: the link
// <Dir>/User/<name> to <ConfigDir>/User/<name> is made when the target exists and
// nothing is at the link's path (not even a broken link); anything already there is
// left alone. Without Link, a link is removed only when it points exactly at that
// target: a file or a link to somewhere else is the user's. A data folder that does not
// exist has no links to remove. The config folder is only ever looked at.
func syncLinks(ctx context.Context, a *AppData) error {
	if a == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.NewErr(tagAppDataCanceled, err).SetMetadata("dir", a.Dir)
	}
	if !a.Link {
		if _, err := os.Lstat(a.Dir); errors.Is(err, fs.ErrNotExist) {
			return nil
		}
	}
	for _, name := range firejail.VSCodeSharedNames() {
		link := filepath.Join(a.Dir, "User", name)
		target := filepath.Join(a.ConfigDir, "User", name)
		if a.Link {
			if err := makeLink(link, target); err != nil {
				return err
			}
			continue
		}
		if err := removeLink(link, target); err != nil {
			return err
		}
	}
	return nil
}

// makeLink makes the symbolic link at link to target when target exists and nothing is
// at link.
func makeLink(link, target string) error {
	if _, err := os.Lstat(target); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errors.NewErr(tagAppDataStatFailed, err).SetMetadata("path", target)
	}
	if _, err := os.Lstat(link); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return errors.NewErr(tagAppDataStatFailed, err).SetMetadata("path", link)
	}
	if err := ensureDir(filepath.Dir(link), 0o700); err != nil {
		return err
	}
	if err := os.Symlink(target, link); err != nil {
		return errors.NewErr(tagAppDataLinkFailed, err).SetMetadata("link", link, "target", target)
	}
	return nil
}

// removeLink removes the link at link when it is a symbolic link to exactly target.
func removeLink(link, target string) error {
	info, err := os.Lstat(link)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errors.NewErr(tagAppDataStatFailed, err).SetMetadata("path", link)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return nil
	}
	dest, err := os.Readlink(link)
	if err != nil {
		return errors.NewErr(tagAppDataUnlinkFailed, err).SetMetadata("link", link, "step", "readlink")
	}
	if dest != target {
		return nil
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.NewErr(tagAppDataUnlinkFailed, err).SetMetadata("link", link, "step", "remove")
	}
	return nil
}

// appDataFile makes the plan of the VS Code data folder of a build (SPEC §7.1) and, when
// the folder is made, its file for the plan. The folder must be strictly inside
// Dirs.AppData, so uninstalling can never delete anything else. It returns nil for a
// build that is not a VS Code app, or whose home is not Full or Only the folders I
// choose.
func appDataFile(req Request, appDataDir, configHome string) (*AppData, *File, error) {
	s := req.Settings
	spec := firejail.NewVSCodeSpec(req.App.Binary, appDataDir, configHome, s.SandboxName)
	if spec == nil || (s.Home != firejail.HomeFull && s.Home != firejail.HomeChosen) {
		return nil, nil, nil
	}
	if !strictlyInside(appDataDir, spec.DataDir) {
		return nil, nil, errors.New(TagAppDataOutside, "the VS Code data folder is not inside Jailcat's appdata folder",
			"dir", spec.DataDir, "appdata", appDataDir)
	}
	plan := &AppData{
		Dir: spec.DataDir, ConfigDir: spec.ConfigDir,
		Create: s.VSCodeDataOnDisk(), Link: s.VSCodeLinksSettings(),
	}
	if !plan.Create {
		return plan, nil, nil
	}
	return plan, &File{Kind: KindAppData, Path: spec.DataDir, Mode: 0o700}, nil
}

const (
	// TagAppDataOutside is the tag of the error [Prepare] returns when a build's VS Code
	// data folder would not be inside the appdata folder.
	TagAppDataOutside = "AppDataOutside"

	tagAppDataCanceled     = "AppDataCanceled"
	tagAppDataStatFailed   = "AppDataStatFailed"
	tagAppDataLinkFailed   = "AppDataLinkFailed"
	tagAppDataUnlinkFailed = "AppDataUnlinkFailed"
)
