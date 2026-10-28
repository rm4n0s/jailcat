package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// Request is everything [Prepare] needs to compute the files of a build.
type Request struct {
	// BuildID is the build's id, written in the header of every file.
	BuildID int64
	// App is the app the build sandboxes.
	App apps.App
	// Settings are the final settings of the build (the overrides applied to the base
	// values).
	Settings firejail.Settings
	// Profile is the generated profile of the build.
	Profile *firejail.Generated
	// Firejail is the absolute, clean path of the firejail executable.
	Firejail string
	// IconTheme is the name of the current icon theme, to look the app's icon up in.
	IconTheme string
}

// Prepared is the plan of a build: every file with its target and content. It is made by
// [Prepare] and read by [Check], [Install] and [Prepared.Preview]; nothing is written
// until [Install].
type Prepared struct {
	// BuildID is the id of the build the files belong to.
	BuildID int64
	// Files are the files to write, in the order [Install] writes them: profile,
	// launcher, icon, the desktop entry when the build has one, the private home and the
	// VS Code data folder when it has them.
	Files []File
	// IconFallback is "" when the icon is the one the settings ask for; otherwise it
	// says in words why a random icon was made instead of the app's icon with the
	// badge (SPEC §10.2). The Launcher tab shows it.
	IconFallback string

	// AppData is what the build does with its VS Code data folder; nil for a build that
	// is not a VS Code app, or whose home is not Full or Only the folders I choose. The
	// folder itself is one of Files when AppData.Create is set.
	AppData *AppData

	// UserDataDir is the data folder the launcher passes to a VS Code app with
	// "--user-data-dir" (SPEC §7.1, §9.2); "" when it passes none. Try passes it too.
	UserDataDir string

	// dirs are the folders the files were computed for.
	dirs xdg.Dirs
}

// Prepare computes the files of a build (SPEC §9): the profile, the launcher, the icon,
// the desktop entry (only with Settings.InstallMenu) and the private home folder (only
// for Settings.Home == HomePrivate), each with its target under dirs, its mode and its
// SHA-256. It reads the app's own desktop entry and icon and writes nothing.
//
// A sandbox name that is not made of [a-z0-9-] is refused with [TagSandboxNameInvalid]
// (the name is validated upstream; this is a second check, because it becomes a file
// name): it is what keeps every target inside its folder.
func Prepare(ctx context.Context, dirs xdg.Dirs, req Request) (*Prepared, error) {
	s := req.Settings
	if !validSandboxName(s.SandboxName) {
		return nil, errors.New(TagSandboxNameInvalid, "the sandbox name must have at least one character, and only a-z, 0-9 and -",
			"name", s.SandboxName)
	}
	if req.Profile == nil {
		return nil, errors.New(tagProfileMissing, "the build has no generated profile", "name", s.SandboxName)
	}

	p := &Prepared{BuildID: req.BuildID, dirs: dirs}

	// The VS Code data folder: checked first, because the launcher names it.
	plan, appData, err := appDataFile(req, dirs.AppData, dirs.ConfigHome)
	if err != nil {
		return nil, err
	}
	p.AppData = plan

	profile := File{
		Kind:    KindProfile,
		Path:    filepath.Join(dirs.Profiles, s.SandboxName+".profile"),
		Content: []byte(req.Profile.String()),
		Mode:    0o644,
	}
	p.Files = append(p.Files, withSum(profile))

	// A VS Code app with its own data folder starts with it in every home mode (SPEC
	// §7.1, §9.2).
	if vs := firejail.NewVSCodeSpec(req.App.Binary, dirs.AppData, dirs.ConfigHome, s.SandboxName); vs != nil && s.VSCodeOwnData {
		p.UserDataDir = vs.DataDir
	}

	launcher, err := launcherFile(dirs, req, profile.Path, p.UserDataDir)
	if err != nil {
		return nil, err
	}
	p.Files = append(p.Files, launcher)

	icon, fallback, err := iconFile(ctx, dirs, req)
	if err != nil {
		return nil, err
	}
	p.Files = append(p.Files, icon)
	p.IconFallback = fallback

	if s.InstallMenu {
		desktop, err := desktopFile(dirs, req, launcher.Path)
		if err != nil {
			return nil, err
		}
		p.Files = append(p.Files, desktop)
	}

	if s.Home == firejail.HomePrivate {
		home, err := homeFile(dirs, s)
		if err != nil {
			return nil, err
		}
		p.Files = append(p.Files, home)
	}

	if appData != nil {
		p.Files = append(p.Files, *appData)
	}
	return p, nil
}

// withSum sets the SHA-256 of f from its content.
func withSum(f File) File {
	sum := sha256.Sum256(f.Content)
	f.SHA256 = hex.EncodeToString(sum[:])
	return f
}

// launcherFile makes the launcher script (SPEC §9.2, §9.3): in the bin folder with
// InstallBin, else in Jailcat's launchers folder.
func launcherFile(dirs xdg.Dirs, req Request, profilePath, userDataDir string) (File, error) {
	s := req.Settings
	dir := dirs.Launchers
	if s.InstallBin {
		dir = dirs.Bin
	}
	var buf bytes.Buffer
	err := firejail.WriteLauncher(&buf, firejail.LauncherSpec{
		BuildID:     req.BuildID,
		Firejail:    req.Firejail,
		Profile:     profilePath,
		Binary:      req.App.Binary,
		UserDataDir: userDataDir,
		ExtraArgs:   s.ExtraArgs,
	})
	if err != nil {
		return File{}, err
	}
	return withSum(File{
		Kind:    KindLauncher,
		Path:    filepath.Join(dir, s.SandboxName),
		Content: buf.Bytes(),
		Mode:    0o755,
	}), nil
}

// iconFile makes the SVG icon file (SPEC §10). The second result is the reason a random
// icon was made when the settings ask for the app's icon with the badge and it cannot be
// used, see [RenderIcon].
func iconFile(ctx context.Context, dirs xdg.Dirs, req Request) (File, string, error) {
	s := req.Settings
	content, fallback, err := RenderIcon(ctx, dirs, req.IconTheme, req.App, s)
	if err != nil {
		return File{}, "", err
	}
	return withSum(File{
		Kind:    KindIcon,
		Path:    filepath.Join(dirs.Icons, s.SandboxName+".svg"),
		Content: content,
		Mode:    0o644,
	}), fallback, nil
}

// RenderIcon makes the SVG icon a build with the settings s installs (SPEC §10): the
// app's icon with the badge for [firejail.IconBadge], a random icon of s.IconSeed and
// s.MenuName for [firejail.IconRandom]. [Prepare] writes it, and the Launcher tab draws
// it as a preview, so both show the same picture. The second result is the reason a
// random icon was made when s asks for the app's icon with the badge and it cannot be
// used: no icon, an icon that is not found, or a source Jailcat cannot draw. theme is
// the name of the current icon theme. It writes nothing.
func RenderIcon(ctx context.Context, dirs xdg.Dirs, theme string, app apps.App, s firejail.Settings) ([]byte, string, error) {
	switch s.Icon {
	case firejail.IconRandom:
		return icons.RandomIcon(s.IconSeed, s.MenuName), "", nil
	case firejail.IconBadge:
		return badgeIcon(ctx, dirs, theme, app, s)
	}
	return nil, "", errors.New(TagIconModeInvalid, "the icon choice is not one of the two", "mode", string(s.Icon))
}

// badgeIcon makes the app's icon with the badge, or a random icon with the reason when
// the app's icon cannot be used. Any other failure (a cancelled ctx, an unreadable
// file) is an error.
func badgeIcon(ctx context.Context, dirs xdg.Dirs, theme string, app apps.App, s firejail.Settings) ([]byte, string, error) {
	random := func(reason string) ([]byte, string, error) {
		return icons.RandomIcon(s.IconSeed, s.MenuName), reason, nil
	}
	if app.Icon == "" {
		return random("the app has no icon")
	}
	src, err := icons.Lookup(ctx, dirs, theme, app.Icon)
	if err != nil {
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == icons.TagIconNotFound {
			return random(fmt.Sprintf("the app's icon %q was not found", app.Icon))
		}
		return nil, "", err
	}
	data, err := icons.BadgeIcon(ctx, src)
	if err != nil {
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == icons.TagIconSourceUnsupported {
			if src.Format != icons.FormatSVG && src.Format != icons.FormatPNG {
				return random(fmt.Sprintf("the app's icon is in the %s format, which Jailcat cannot draw", strings.ToUpper(string(src.Format))))
			}
			reason, _ := appErr.Metadata["reason"].(string)
			return random(fmt.Sprintf("the app's icon %s cannot be used (%s)", src.Path, reason))
		}
		return nil, "", err
	}
	return data, "", nil
}

// homeFile makes the private home folder of a build with an own private home (SPEC
// §7.1): Settings.PrivateDir, with ${HOME} or ~ resolved against the home. It must be a
// folder below Dirs.Homes, so uninstalling can never delete anything else.
func homeFile(dirs xdg.Dirs, s firejail.Settings) (File, error) {
	var path string
	switch dir := s.PrivateDir; {
	case dir == "${HOME}" || dir == "~":
		path = dirs.Home
	case strings.HasPrefix(dir, "${HOME}/"):
		path = filepath.Join(dirs.Home, strings.TrimPrefix(dir, "${HOME}/"))
	case strings.HasPrefix(dir, "~/"):
		path = filepath.Join(dirs.Home, strings.TrimPrefix(dir, "~/"))
	case filepath.IsAbs(dir):
		path = filepath.Clean(dir)
	default:
		return File{}, errors.New(tagPrivateDirInvalid, "the private home folder is empty or not an absolute path",
			"dir", dir)
	}
	if !strictlyInside(dirs.Homes, path) {
		return File{}, errors.New(tagPrivateHomeOutside, "the private home folder is not inside Jailcat's homes folder",
			"dir", path, "homes", dirs.Homes)
	}
	return File{Kind: KindHome, Path: path, Mode: 0o700}, nil
}

// PreviewTexts are the texts of the Preview dialog (SPEC §11.1).
type PreviewTexts struct {
	// Profile is the profile text.
	Profile string
	// Launcher is the launcher script.
	Launcher string
	// Desktop is the desktop entry; "" when the build installs none.
	Desktop string
}

// Preview returns the profile, the launcher and the desktop entry as text, for the
// Preview dialog (SPEC §11.1). The icon is not included.
func (p *Prepared) Preview() PreviewTexts {
	var t PreviewTexts
	for _, f := range p.Files {
		switch f.Kind {
		case KindProfile:
			t.Profile = string(f.Content)
		case KindLauncher:
			t.Launcher = string(f.Content)
		case KindDesktop:
			t.Desktop = string(f.Content)
		}
	}
	return t
}

const (
	// TagSandboxNameInvalid is the tag of the error [Prepare] returns for a sandbox name
	// that is empty or has a character outside [a-z0-9-].
	TagSandboxNameInvalid = "SandboxNameInvalid"

	// TagIconModeInvalid is the tag of the error [RenderIcon] (and so [Prepare]) returns
	// for an icon choice that is neither the badge nor a random icon.
	TagIconModeInvalid = "IconModeInvalid"

	tagProfileMissing     = "ProfileMissing"
	tagPrivateDirInvalid  = "PrivateDirInvalid"
	tagPrivateHomeOutside = "PrivateHomeOutside"
)
