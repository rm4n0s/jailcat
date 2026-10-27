package install

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// fileCodes are the Exec field codes the desktop entry of a build keeps (SPEC §9.3).
var fileCodes = []string{"%f", "%F", "%u", "%U"}

// otherCodes are the field codes that are dropped from an Exec value: the menu would
// expand them to the original app's icon, name or entry file.
var otherCodes = []string{"%i", "%c", "%k"}

// entryBuilder sets keys of a group and keeps the first error, so a long run of keys
// reads as a list. The errors are the ones of package desktopentry and are returned as
// they are.
type entryBuilder struct {
	group *desktopentry.Group
	err   error
}

func (b *entryBuilder) set(key, value string) {
	if b.err == nil {
		b.err = b.group.Set(key, value)
	}
}

func (b *entryBuilder) setLocale(key, locale, value string) {
	if b.err == nil {
		b.err = b.group.SetLocale(key, locale, value)
	}
}

func (b *entryBuilder) setBool(key string, value bool) {
	if b.err == nil {
		b.err = b.group.SetBool(key, value)
	}
}

func (b *entryBuilder) setStrings(key string, items []string) {
	if b.err == nil {
		b.err = b.group.SetStrings(key, items)
	}
}

func (b *entryBuilder) comment(text string) {
	if b.err == nil {
		b.err = b.group.Comment(text)
	}
}

// desktopFile makes the desktop entry of SPEC §9.3 for the launcher at launcherPath.
func desktopFile(dirs xdg.Dirs, req Request, launcherPath string) (File, error) {
	s, app := req.Settings, req.App
	if strings.TrimSpace(s.MenuName) == "" {
		return File{}, errors.New(tagMenuNameEmpty, "the menu name is empty, and a desktop entry needs a Name",
			"name", s.SandboxName)
	}
	orig, err := readAppEntry(app)
	if err != nil {
		return File{}, err
	}
	// A nil group reads as empty, so an app without an entry copies nothing.
	var origMain *desktopentry.Group
	if orig != nil {
		origMain = orig.Main()
	}

	entry := desktopentry.NewEntry()
	b := &entryBuilder{group: entry.Group(desktopentry.MainGroup)}
	// The header comes first, at the top of the group (SPEC §9.3).
	for _, line := range firejail.Header(req.BuildID) {
		b.comment(strings.TrimPrefix(line, "# "))
	}
	b.set("Type", "Application")
	b.set("Name", s.MenuName)
	if g := origMain.String("GenericName"); g != "" {
		b.set("GenericName", g)
	}
	if s.Comment != "" {
		b.set("Comment", s.Comment)
	}
	b.set("Exec", execValue(launcherPath, firstFileCode(app.Args)))
	b.set("Icon", s.SandboxName)
	switch cats := origMain.Strings("Categories"); {
	case len(cats) > 0:
		b.setStrings("Categories", cats)
	case orig == nil:
		b.setStrings("Categories", []string{"Utility"})
	}
	if kw := origMain.Strings("Keywords"); len(kw) > 0 {
		b.setStrings("Keywords", kw)
	}
	b.setBool("Terminal", s.Terminal)
	// A StartupNotify that is not a boolean is not copied.
	if v, boolErr := origMain.Bool("StartupNotify"); origMain.Has("StartupNotify") && boolErr == nil {
		b.setBool("StartupNotify", v)
	}
	if mt := origMain.Strings("MimeType"); s.OpenFileTypes && len(mt) > 0 {
		b.setStrings("MimeType", mt)
	}
	if wm := wmClass(app, origMain); wm != "" {
		b.set("StartupWMClass", wm)
	}

	actions := sameBinaryActions(app)
	if len(actions) > 0 {
		ids := make([]string, len(actions))
		for i, a := range actions {
			ids[i] = a.ID
		}
		b.setStrings("Actions", ids)
	}
	b.set("X-Jailcat-Build", strconv.FormatInt(req.BuildID, 10))
	if b.err != nil {
		return File{}, b.err
	}

	for _, a := range actions {
		ab := &entryBuilder{group: entry.Group(desktopentry.ActionGroupPrefix + a.ID)}
		copyActionNames(ab, a, orig)
		ab.set("Exec", execValue(launcherPath, a.Args))
		if ab.err != nil {
			return File{}, ab.err
		}
	}

	var buf bytes.Buffer
	if err := desktopentry.Write(&buf, entry); err != nil {
		return File{}, err
	}
	return withSum(File{
		Kind:    KindDesktop,
		Path:    filepath.Join(dirs.Applications, s.SandboxName+".desktop"),
		Content: buf.Bytes(),
		Mode:    0o644,
	}), nil
}

// readAppEntry reads the app's own desktop entry; it is nil for an app without one.
func readAppEntry(app apps.App) (*desktopentry.Entry, error) {
	if app.DesktopPath == "" {
		return nil, nil
	}
	f, err := os.Open(app.DesktopPath)
	if err != nil {
		return nil, errors.NewErr(tagAppEntryReadFailed, err).SetMetadata("path", app.DesktopPath)
	}
	defer func() { _ = f.Close() }()
	return desktopentry.Parse(f)
}

// wmClass is the class the app's window reports (SPEC §9.3): the original entry's
// StartupWMClass, else the desktop file ID without ".desktop", else the binary's name.
func wmClass(app apps.App, origMain *desktopentry.Group) string {
	if wm := origMain.String("StartupWMClass"); wm != "" {
		return wm
	}
	if app.Kind == apps.KindDesktop {
		if id := strings.TrimSuffix(app.ID, ".desktop"); id != "" {
			return id
		}
	}
	return app.BinaryName
}

// sameBinaryActions returns the app's desktop actions that run the app's own binary.
func sameBinaryActions(app apps.App) []apps.Action {
	var out []apps.Action
	for _, a := range app.Actions {
		if a.SameBinary {
			out = append(out, a)
		}
	}
	return out
}

// copyActionNames copies the Name of an action from the original entry, with its
// localized variants; without the entry's group it uses the name the scan read.
func copyActionNames(b *entryBuilder, a apps.Action, orig *desktopentry.Entry) {
	plain := false
	if orig != nil {
		for _, g := range orig.Actions() {
			if id, _ := g.ActionID(); id != a.ID {
				continue
			}
			for _, k := range g.Keys() {
				if k.Name != "Name" {
					continue
				}
				value := g.String(k.FullName())
				if k.Locale == "" {
					plain = true
					b.set("Name", value)
				} else {
					b.setLocale("Name", k.Locale, value)
				}
			}
			break
		}
	}
	if !plain {
		b.set("Name", a.Name)
	}
}

// firstFileCode returns the first of %f %F %u %U that is an argument of its own in
// args, as a list of zero or one element.
func firstFileCode(args []string) []string {
	for _, a := range args {
		if slices.Contains(fileCodes, a) {
			return []string{a}
		}
	}
	return nil
}

// execValue builds an Exec value: the launcher, then the arguments. An argument that is
// a field code of its own stays one when it is the first of %f %F %u %U (a command has at
// most one); the other field codes are dropped; every other argument is quoted. The
// result is the value after the general string escapes are decoded, as Group.Set wants.
func execValue(launcher string, args []string) string {
	parts := []string{desktopentry.QuoteExecArg(launcher)}
	haveCode := false
	for _, a := range args {
		switch {
		case slices.Contains(fileCodes, a):
			if !haveCode {
				parts = append(parts, a)
				haveCode = true
			}
		case slices.Contains(otherCodes, a):
		default:
			parts = append(parts, desktopentry.QuoteExecArg(a))
		}
	}
	return strings.Join(parts, " ")
}

const (
	tagMenuNameEmpty      = "MenuNameEmpty"
	tagAppEntryReadFailed = "AppEntryReadFailed"
)
