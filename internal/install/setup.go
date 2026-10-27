package install

import (
	"bytes"
	"context"
	"path/filepath"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/assets"
	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// AppID is Jailcat's application ID (SPEC §3). It is the app ID of the window, so the
// Wayland app_id and the X11 WM_CLASS match, and the name of Jailcat's own desktop entry
// and icon (SPEC §3.2).
const AppID = "io.github.rm4n0s.jailcat"

// SetupResult tells where [SetupDesktop] wrote Jailcat's own files.
type SetupResult struct {
	// DesktopFile is the desktop entry in the application menu folder.
	DesktopFile string
	// IconFile is the SVG icon in the user's scalable icon folder.
	IconFile string
}

// SetupDesktop adds Jailcat itself to the application menu (SPEC §3.2): it writes Jailcat's
// icon to <dirs.Icons>/io.github.rm4n0s.jailcat.svg and a desktop entry to
// <dirs.Applications>/io.github.rm4n0s.jailcat.desktop whose Exec is exe, the absolute path
// of the running program (quoted as the Desktop Entry Specification requires). Missing
// folders are made with mode 0755 and both files get mode 0644. Each file goes through a
// temporary file renamed over the target, the icon first, so a second call replaces the
// files of an earlier one. The files are not a build: the entry has no X-Jailcat-Build key
// and nothing is recorded in the database.
//
// If the second write fails, the icon stays in place.
func SetupDesktop(ctx context.Context, dirs xdg.Dirs, exe string) (SetupResult, error) {
	if !filepath.IsAbs(exe) {
		return SetupResult{}, errors.New(tagSetupExeNotAbsolute, "the program's path is not absolute", "exe", exe)
	}
	entry := desktopentry.NewEntry()
	g := entry.Group(desktopentry.MainGroup)
	for _, kv := range [][2]string{
		{"Type", "Application"},
		{"Name", "Jailcat"},
		{"Comment", "Build firejail sandboxes for your applications"},
		{"Exec", desktopentry.QuoteExecArg(exe)},
		{"Icon", AppID},
	} {
		if err := g.Set(kv[0], kv[1]); err != nil {
			return SetupResult{}, err
		}
	}
	if err := g.SetBool("Terminal", false); err != nil {
		return SetupResult{}, err
	}
	if err := g.SetStrings("Categories", []string{"System", "Security", "Utility"}); err != nil {
		return SetupResult{}, err
	}
	if err := g.Set("StartupWMClass", AppID); err != nil {
		return SetupResult{}, err
	}
	var buf bytes.Buffer
	if err := desktopentry.Write(&buf, entry); err != nil {
		return SetupResult{}, err
	}

	files := []File{
		{Kind: KindIcon, Path: filepath.Join(dirs.Icons, AppID+".svg"), Content: assets.Icon(), Mode: 0o644},
		{Kind: KindDesktop, Path: filepath.Join(dirs.Applications, AppID+".desktop"), Content: buf.Bytes(), Mode: 0o644},
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return SetupResult{}, errors.NewErr(tagSetupCanceled, err).SetMetadata("path", f.Path)
		}
		if err := ensureDir(filepath.Dir(f.Path), 0o755); err != nil {
			return SetupResult{}, err
		}
		if err := writeAtomic(f); err != nil {
			return SetupResult{}, err
		}
	}
	return SetupResult{IconFile: files[0].Path, DesktopFile: files[1].Path}, nil
}

const (
	tagSetupExeNotAbsolute = "SetupExeNotAbsolute"
	tagSetupCanceled       = "SetupCanceled"
)
