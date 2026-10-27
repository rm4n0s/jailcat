package apps

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// jailcatBuildKey marks a desktop entry as one of Jailcat's builds (SPEC §9.3).
const jailcatBuildKey = "X-Jailcat-Build"

// scanApplications reads the *.desktop files below one applications folder. seen holds
// the desktop file IDs an earlier folder already has, broken or hidden ones included;
// the IDs found here are added. A folder that does not exist is ignored.
func scanApplications(root string, dirs xdg.Dirs, locale string, seen map[string]bool) ([]App, []Skipped) {
	files, skipped := desktopFiles(root)
	var found []App
	for _, path := range files {
		id, err := desktopentry.FileID(root, path)
		if err != nil {
			skipped = append(skipped, Skipped{Path: path, Reason: err.Error()})
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		app, keep, reason := loadDesktop(path, id, dirs, locale)
		switch {
		case keep:
			found = append(found, app)
		case reason != "":
			skipped = append(skipped, Skipped{Path: path, Reason: reason})
		}
	}
	return found, skipped
}

// desktopFiles lists the *.desktop files below root, subfolders included, in the
// order of a walk (names sorted in every folder). Symbolic links to files count;
// symbolic links to folders are not entered. A folder that cannot be read is reported.
func desktopFiles(root string) (files []string, skipped []Skipped) {
	info, err := os.Stat(root)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			skipped = append(skipped, Skipped{Path: root, Reason: "cannot read the folder: " + err.Error()})
		}
		return nil, skipped
	}
	if !info.IsDir() {
		return nil, nil
	}
	// The trailing separator makes the walk enter the folder when it is a symbolic link.
	walkErr := filepath.WalkDir(root+string(filepath.Separator), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			skipped = append(skipped, Skipped{Path: path, Reason: "cannot read: " + err.Error()})
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".desktop") {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		skipped = append(skipped, Skipped{Path: root, Reason: "cannot read the folder: " + walkErr.Error()})
	}
	return files, skipped
}

// boolKey reads a boolean key; a missing key or a value other than true or false is
// false.
func boolKey(g *desktopentry.Group, key string) bool {
	v, err := g.Bool(key)
	return err == nil && v
}

// loadDesktop turns one desktop file into an App. keep is false when the file is
// dropped; reason then says why, and is "" when the file is dropped on purpose
// (Hidden=true).
func loadDesktop(path, id string, dirs xdg.Dirs, locale string) (app App, keep bool, reason string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return App{}, false, "cannot read the file: " + err.Error()
	}
	entry, err := desktopentry.Parse(bytes.NewReader(data))
	if err != nil {
		return App{}, false, "cannot parse the file: " + err.Error()
	}
	main := entry.Main()
	if main == nil {
		return App{}, false, "the file has no [Desktop Entry] group"
	}
	if boolKey(main, "Hidden") {
		return App{}, false, ""
	}
	if t := main.String("Type"); t != "Application" {
		return App{}, false, fmt.Sprintf("Type is %q, not Application", t)
	}
	if main.Has(jailcatBuildKey) {
		return App{}, false, "it is one of Jailcat's builds"
	}
	if try := main.String("TryExec"); try != "" && !tryExecFound(try, dirs) {
		return App{}, false, fmt.Sprintf("TryExec %q is not an executable", try)
	}
	if strings.TrimSpace(main.String("Exec")) == "" {
		return App{}, false, "the file has no Exec key"
	}
	words, err := desktopentry.SplitExec(main.String("Exec"))
	if err != nil {
		return App{}, false, "cannot read Exec: " + err.Error()
	}
	cmd, reason := resolveCommand(words, dirs)
	if reason != "" {
		return App{}, false, reason
	}

	app = App{
		ID:               id,
		Kind:             KindDesktop,
		Name:             main.LocaleString("Name", locale),
		GenericName:      main.LocaleString("GenericName", locale),
		Comment:          main.LocaleString("Comment", locale),
		Keywords:         nilIfEmpty(main.LocaleStrings("Keywords", locale)),
		Icon:             main.String("Icon"),
		Binary:           cmd.binary,
		BinaryName:       filepath.Base(cmd.binary),
		Args:             nilIfEmpty(cmd.args),
		DesktopPath:      path,
		UsesFirejail:     cmd.usesFirejail,
		AlreadySandboxed: cmd.sandbox != "",
		SandboxKind:      cmd.sandbox,
		NoDisplay:        boolKey(main, "NoDisplay"),
		Terminal:         boolKey(main, "Terminal"),
	}
	if app.Name == "" {
		app.Name = app.BinaryName
	}
	for _, g := range entry.Actions() {
		app.Actions = append(app.Actions, loadAction(g, app.Binary, dirs, locale))
	}
	return app, true, ""
}

// loadAction reads a [Desktop Action] group. An action whose Exec is missing or has no
// usable program is kept with an empty Binary and SameBinary false.
func loadAction(g *desktopentry.Group, appBinary string, dirs xdg.Dirs, locale string) Action {
	id, _ := g.ActionID()
	act := Action{ID: id, Name: g.LocaleString("Name", locale)}
	words, err := desktopentry.SplitExec(g.String("Exec"))
	if err != nil {
		return act
	}
	cmd, reason := resolveCommand(words, dirs)
	if reason != "" {
		return act
	}
	act.Binary, act.Args = cmd.binary, nilIfEmpty(cmd.args)
	act.SameBinary = evalOrPlain(cmd.binary) == evalOrPlain(appBinary)
	return act
}

// nilIfEmpty returns nil for an empty list, so an app without keywords or arguments
// has none rather than an empty slice.
func nilIfEmpty(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return list
}
