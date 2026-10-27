package apps

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// scanFake scans a fresh fake system with the given locale.
func scanFake(t *testing.T, locale string) (fakeSystem, Result) {
	t.Helper()
	s := newFakeSystem(t)
	res, err := Scan(context.Background(), s.dirs, locale)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return s, res
}

func TestScanAppList(t *testing.T) {
	_, res := scanFake(t, "")
	got := make([]string, 0, len(res.Apps))
	for _, a := range res.Apps {
		got = append(got, a.ID)
	}
	wantSet := []string{
		"Alpha.desktop", "alpha-two.desktop", "actions.desktop", "dup", "dupalias", "env-options.desktop",
		"env.desktop", "firecfg-abs.desktop", "firecfg-bare.desktop", "flatpak", "flatpak.desktop",
		"hidden-value.desktop", "kde-foo.desktop", "localized.desktop", "no-name.desktop",
		"nodisplay.desktop", "plainbin", "runbin.desktop", "sandboxme", "shadowed.desktop",
		"snap.desktop", "sysapp.desktop", "tryexec-ok.desktop", "wrapped-dashdash.desktop",
		"wrapped-env.desktop", "wrapped.desktop",
	}
	sortedGot := append([]string(nil), got...)
	sort.Strings(sortedGot)
	sortedWant := append([]string(nil), wantSet...)
	sort.Strings(sortedWant)
	if !reflect.DeepEqual(sortedGot, sortedWant) {
		t.Errorf("apps = %v\nwant   %v", sortedGot, sortedWant)
	}

	// The system copy of a shadowed ID is never read, even when the user's copy is Hidden.
	for _, a := range res.Apps {
		if strings.HasSuffix(a.DesktopPath, "sys1/applications/shadowed.desktop") ||
			strings.HasSuffix(a.DesktopPath, "sys1/applications/hideme.desktop") ||
			strings.HasSuffix(a.DesktopPath, "sys1/applications/broken.desktop") ||
			strings.HasSuffix(a.DesktopPath, "sys2/applications/sysapp.desktop") {
			t.Errorf("a shadowed file was listed: %s", a.DesktopPath)
		}
	}

	// Sorted by Name (case-insensitive), then ID.
	for i := 1; i < len(res.Apps); i++ {
		a, b := res.Apps[i-1], res.Apps[i]
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if an > bn || (an == bn && a.ID > b.ID) {
			t.Errorf("apps %d and %d are out of order: %q (%s) before %q (%s)", i-1, i, a.Name, a.ID, b.Name, b.ID)
		}
	}
	// "alpha" and "Alpha" tie ignoring case, so the ID decides: "Alpha.desktop" < "alpha-two.desktop".
	if got[0] != "Alpha.desktop" || got[1] != "alpha-two.desktop" {
		t.Errorf("first apps = %v, want the two Alpha entries ordered by ID", got[:2])
	}
}

func TestScanDesktopApps(t *testing.T) {
	s, res := scanFake(t, "")
	flatpakArgs := []string{"run", "--branch=stable", "--arch=x86_64", "org.example.Flat", "@@u", "%U", "@@"}
	tests := []struct {
		id   string
		want App
	}{
		{"shadowed.desktop", App{
			Name: "Shadowed (user copy)", Binary: s.path("bin1/prog"), Args: []string{"%U"},
			DesktopPath: s.path("user/applications/shadowed.desktop"),
		}},
		{"sysapp.desktop", App{
			Name: "zeta System App", Binary: s.path("bin1/prog"),
			DesktopPath: s.path("sys1/applications/sysapp.desktop"),
		}},
		{"Alpha.desktop", App{
			Name: "alpha", Binary: s.path("bin1/prog"), DesktopPath: s.path("sys2/applications/Alpha.desktop"),
		}},
		{"nodisplay.desktop", App{
			Name: "Background Helper", Binary: s.path("bin1/envprog"), NoDisplay: true, Terminal: true,
			DesktopPath: s.path("user/applications/nodisplay.desktop"),
		}},
		{"kde-foo.desktop", App{
			Name: "KDE Foo", Binary: s.path("bin1/prog"), DesktopPath: s.path("user/applications/kde/foo.desktop"),
		}},
		{"tryexec-ok.desktop", App{
			Name: "TryExec Ok", Binary: s.path("bin1/prog"), Args: []string{"--ok"},
			DesktopPath: s.path("user/applications/tryexec-ok.desktop"),
		}},
		{"env.desktop", App{
			Name: "Env App", Binary: s.path("bin1/envprog"), Args: []string{"%U"},
			DesktopPath: s.path("user/applications/env.desktop"),
		}},
		{"env-options.desktop", App{
			Name: "Env Options", Binary: s.path("bin1/prog"), Args: []string{"--flag", "%f"},
			DesktopPath: s.path("user/applications/env-options.desktop"),
		}},
		{"wrapped.desktop", App{
			Name: "Wrapped", Binary: s.path("bin1/prog"), Args: []string{"%U"}, UsesFirejail: true,
			DesktopPath: s.path("user/applications/wrapped.desktop"),
		}},
		{"wrapped-dashdash.desktop", App{
			Name: "Wrapped Dashdash", Binary: s.path("bin1/prog"), UsesFirejail: true,
			DesktopPath: s.path("user/applications/wrapped-dashdash.desktop"),
		}},
		{"wrapped-env.desktop", App{
			Name: "Wrapped Env", Binary: s.path("bin1/prog"), UsesFirejail: true,
			DesktopPath: s.path("user/applications/wrapped-env.desktop"),
		}},
		{"flatpak.desktop", App{
			Name: "Flat App", Binary: s.path("bin2/flatpak"), Args: flatpakArgs,
			AlreadySandboxed: true, SandboxKind: "flatpak",
			DesktopPath: s.path("user/applications/flatpak.desktop"),
		}},
		{"snap.desktop", App{
			Name: "Snap Browser", Binary: "/snap/bin/snapbrowser", Args: []string{"%u"},
			AlreadySandboxed: true, SandboxKind: "snap",
			DesktopPath: s.path("user/applications/snap.desktop"),
		}},
		{"firecfg-abs.desktop", App{
			Name: "Firecfg Absolute", Binary: s.path("bin2/firecfgapp"), Args: []string{"--x"},
			DesktopPath: s.path("user/applications/firecfg-abs.desktop"),
		}},
		{"firecfg-bare.desktop", App{
			Name: "Firecfg Bare", Binary: s.path("bin3/firecfgbare"), Args: []string{"%U"},
			DesktopPath: s.path("user/applications/firecfg-bare.desktop"),
		}},
		{"runbin.desktop", App{
			Name: "Run Bin", Binary: s.path("bin3/runbin"), DesktopPath: s.path("user/applications/runbin.desktop"),
		}},
		// Hidden=maybe, NoDisplay=yes and Terminal=1 are not booleans: they are false.
		{"hidden-value.desktop", App{
			Name: "Hidden Value Is Bad", Binary: s.path("bin3/onlybin3"),
			DesktopPath: s.path("user/applications/hidden-value.desktop"),
		}},
		// No Name key: the binary name stands in.
		{"no-name.desktop", App{
			Name: "onlybin3", Binary: s.path("bin3/onlybin3"), Args: []string{"--no-name"},
			DesktopPath: s.path("user/applications/no-name.desktop"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := byID(t, res.Apps, tt.id)
			want := tt.want
			want.ID, want.Kind = tt.id, KindDesktop
			want.BinaryName = filepath.Base(want.Binary)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("app = %+v\nwant  %+v", got, want)
			}
		})
	}
}

func TestScanDroppedEntries(t *testing.T) {
	_, res := scanFake(t, "")
	tests := []struct {
		name string
		id   string
		// reason is a part of the Skipped reason, or "" when the entry is dropped without one.
		reason string
		file   string
	}{
		{"user Hidden copy hides the system entry", "hideme.desktop", "", ""},
		{"broken user file hides the valid system copy", "broken.desktop", "cannot parse", "user/applications/broken.desktop"},
		{"TryExec that is not on PATH", "tryexec-miss.desktop", "TryExec", "user/applications/tryexec-miss.desktop"},
		{"TryExec that is a file without execute bits", "tryexec-abs-miss.desktop", "TryExec", "user/applications/tryexec-abs-miss.desktop"},
		{"Type=Link", "link.desktop", "Type", "user/applications/link.desktop"},
		{"Jailcat build", "firejail-code.desktop", "Jailcat", "user/applications/firejail-code.desktop"},
		{"env with an unsupported option", "env-bad-option.desktop", "env option", "user/applications/env-bad-option.desktop"},
		{"env with no program", "env-nothing.desktop", "no program", "user/applications/env-nothing.desktop"},
		{"no Exec", "noexec.desktop", "no Exec", "user/applications/noexec.desktop"},
		{"Exec that cannot be split", "bad-exec.desktop", "Exec", "user/applications/bad-exec.desktop"},
		{"program not on PATH", "missing-binary.desktop", "not an executable on $PATH", "user/applications/missing-binary.desktop"},
		{"relative path", "relative-binary.desktop", "relative", "user/applications/relative-binary.desktop"},
		{"absolute path without execute bits", "abs-notexec.desktop", "not an executable file", "user/applications/abs-notexec.desktop"},
		{"absolute path of a folder", "abs-dir.desktop", "not an executable file", "user/applications/abs-dir.desktop"},
		{"firecfg link with nothing behind it", "firecfg-only.desktop", "not an executable on $PATH", "user/applications/firecfg-only.desktop"},
		{"no desktop group", "no-group.desktop", "[Desktop Entry]", "user/applications/no-group.desktop"},
		{"not a .desktop file", "not-an-entry.txt", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, a := range res.Apps {
				if a.ID == tt.id {
					t.Errorf("app %q is listed: %+v", tt.id, a)
				}
			}
			var found []Skipped
			for _, sk := range res.Skipped {
				if strings.HasSuffix(sk.Path, "/"+tt.file) && tt.file != "" {
					found = append(found, sk)
				}
			}
			if tt.reason == "" {
				return // dropped on purpose: not listed, not reported (the count below checks that)
			}
			if len(found) != 1 {
				t.Fatalf("Skipped entries for %s = %v, want one", tt.file, found)
			}
			if !strings.Contains(found[0].Reason, tt.reason) {
				t.Errorf("reason = %q, want it to mention %q", found[0].Reason, tt.reason)
			}
		})
	}

	// Nothing else is skipped: a missing folder is ignored, a dropped Hidden entry or
	// shadowed ID is not reported.
	if got, want := len(res.Skipped), 15; got != want {
		t.Errorf("%d skipped entries, want %d: %v", got, want, res.Skipped)
	}
	for _, sk := range res.Skipped {
		if sk.Path == "" || sk.Reason == "" {
			t.Errorf("incomplete skipped entry %+v", sk)
		}
		if strings.Contains(sk.Path, "sys3") || strings.Contains(sk.Path, "bin4") {
			t.Errorf("a folder that does not exist was reported: %+v", sk)
		}
	}
}

func TestScanBinaries(t *testing.T) {
	s, res := scanFake(t, "")
	var bins []App
	for _, a := range res.Apps {
		if a.Kind == KindBinary {
			bins = append(bins, a)
		}
	}
	want := []App{
		// bin1 has the first "dup"; bin2's is hidden behind it.
		{ID: "dup", Name: "dup", Binary: s.path("bin1/dup"), BinaryName: "dup"},
		// A link to an ordinary executable is a binary of its own.
		{ID: "dupalias", Name: "dupalias", Binary: s.path("bin3/dupalias"), BinaryName: "dupalias"},
		// A Flatpak entry does not hide the flatpak program.
		{ID: "flatpak", Name: "flatpak", Binary: s.path("bin2/flatpak"), BinaryName: "flatpak"},
		{ID: "plainbin", Name: "plainbin", Binary: s.path("bin3/plainbin"), BinaryName: "plainbin"},
		// bin1's "sandboxme" is a firecfg link, so the name resolves to bin3.
		{ID: "sandboxme", Name: "sandboxme", Binary: s.path("bin3/sandboxme"), BinaryName: "sandboxme"},
	}
	for i := range want {
		want[i].Kind = KindBinary
		want[i].Terminal = true
	}
	if !reflect.DeepEqual(bins, want) {
		t.Errorf("binaries = %+v\nwant       %+v", bins, want)
	}
}

func TestScanLocale(t *testing.T) {
	tests := []struct {
		name     string
		locale   string
		wantName string
		wantGen  string
		wantCom  string
		wantKeys []string
	}{
		{"no locale", "", "Localized", "Editor", "A comment", []string{"text", "edit"}},
		{"de_DE.UTF-8", "de_DE.UTF-8", "Lokalisiert", "Texteditor", "Ein Kommentar", []string{"Text", "Bearbeiten"}},
		{"fr falls back for the keys it lacks", "fr_FR.UTF-8", "Localisé", "Editor", "A comment", []string{"text", "edit"}},
		{"unknown language", "xx_YY", "Localized", "Editor", "A comment", []string{"text", "edit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, res := scanFake(t, tt.locale)
			a := byID(t, res.Apps, "localized.desktop")
			if a.Name != tt.wantName || a.GenericName != tt.wantGen || a.Comment != tt.wantCom ||
				!reflect.DeepEqual(a.Keywords, tt.wantKeys) {
				t.Errorf("got Name=%q GenericName=%q Comment=%q Keywords=%q", a.Name, a.GenericName, a.Comment, a.Keywords)
			}
			if a.Icon != "accessories-text-editor" || !a.Terminal {
				t.Errorf("Icon=%q Terminal=%v", a.Icon, a.Terminal)
			}
		})
	}
}

func TestScanActions(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		want   func(s fakeSystem) []Action
	}{
		{"no locale", "", func(s fakeSystem) []Action {
			return []Action{
				{ID: "same", Name: "Same Binary", Binary: s.path("bin1/envprog"), Args: []string{"--new-window", "%U"}, SameBinary: true},
				{ID: "other", Name: "Other Binary", Binary: s.path("bin1/prog"), Args: []string{"--other"}},
				{ID: "wrapped", Name: "Wrapped Same", Binary: s.path("bin1/envprog"), Args: []string{"--wrapped"}, SameBinary: true},
				{ID: "missing", Name: "Missing"},
				{ID: "noexec", Name: "No Exec"},
			}
		}},
		{"localized name", "de", func(s fakeSystem) []Action {
			return []Action{
				{ID: "same", Name: "Gleiches Programm", Binary: s.path("bin1/envprog"), Args: []string{"--new-window", "%U"}, SameBinary: true},
				{ID: "other", Name: "Other Binary", Binary: s.path("bin1/prog"), Args: []string{"--other"}},
				{ID: "wrapped", Name: "Wrapped Same", Binary: s.path("bin1/envprog"), Args: []string{"--wrapped"}, SameBinary: true},
				{ID: "missing", Name: "Missing"},
				{ID: "noexec", Name: "No Exec"},
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, res := scanFake(t, tt.locale)
			a := byID(t, res.Apps, "actions.desktop")
			if want := tt.want(s); !reflect.DeepEqual(a.Actions, want) {
				t.Errorf("actions = %+v\nwant     %+v", a.Actions, want)
			}
			if a.Icon != s.path("icons/actions.svg") {
				t.Errorf("Icon = %q, want the raw Icon= value", a.Icon)
			}
		})
	}
	// An app without an Actions key has no actions.
	_, res := scanFake(t, "")
	if got := byID(t, res.Apps, "shadowed.desktop").Actions; got != nil {
		t.Errorf("Actions = %v, want none", got)
	}
}

func TestScanPathOrderAndRelative(t *testing.T) {
	s := newFakeSystem(t)
	// With bin2 first, its "dup" wins and the firecfg link in bin1 still gives way.
	s.dirs.Path = []string{s.path("bin2"), s.path("bin1"), s.path("bin3")}
	res, err := Scan(context.Background(), s.dirs, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := byID(t, res.Apps, "dup").Binary; got != s.path("bin2/dup") {
		t.Errorf("dup = %q, want the one in the first folder", got)
	}
	if got := byID(t, res.Apps, "firecfg-abs.desktop").Binary; got != s.path("bin2/firecfgapp") {
		t.Errorf("firecfgapp = %q", got)
	}
}

func TestScanUnreadableFolders(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("folder modes do not stop root")
	}
	s := newFakeSystem(t)
	locked := s.path("user/applications/locked")
	mkdir(t, locked)
	writeFile(t, filepath.Join(locked, "hidden-inside.desktop"), "[Desktop Entry]\nType=Application\nName=X\nExec=prog\n", 0o644)
	// A folder on $PATH that cannot be read, in addition to the locked applications subfolder.
	unreadable := s.path("bin5")
	mkdir(t, unreadable)
	s.dirs.Path = append(s.dirs.Path, unreadable)
	for _, p := range []string{locked, unreadable} {
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(p, 0o755); err != nil {
				t.Error(err)
			}
		})
	}
	res, err := Scan(context.Background(), s.dirs, "")
	if err != nil {
		t.Fatal(err)
	}
	var gotLocked, gotBin bool
	for _, sk := range res.Skipped {
		switch sk.Path {
		case locked:
			gotLocked = strings.Contains(sk.Reason, "cannot read")
		case unreadable:
			gotBin = strings.Contains(sk.Reason, "cannot read")
		}
	}
	if !gotLocked || !gotBin {
		t.Errorf("unreadable folders not reported (applications subfolder %v, $PATH folder %v): %v", gotLocked, gotBin, res.Skipped)
	}
	// The rest of the scan is not affected.
	byID(t, res.Apps, "shadowed.desktop")
	byID(t, res.Apps, "plainbin")
}

func TestScanSymlinkedFiles(t *testing.T) {
	s := newFakeSystem(t)
	// A .desktop file that is a link to a file elsewhere is read; one that is a link to
	// nothing goes to Skipped.
	target := s.path("elsewhere.desktop")
	writeFile(t, target, "[Desktop Entry]\nType=Application\nName=Linked\nExec=prog\n", 0o644)
	if err := os.Symlink(target, s.path("user/applications/linked.desktop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(s.path("nothing.desktop"), s.path("user/applications/dangling.desktop")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(context.Background(), s.dirs, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := byID(t, res.Apps, "linked.desktop").Name; got != "Linked" {
		t.Errorf("Name = %q", got)
	}
	found := false
	for _, sk := range res.Skipped {
		if filepath.Base(sk.Path) == "dangling.desktop" && strings.Contains(sk.Reason, "cannot read") {
			found = true
		}
	}
	if !found {
		t.Errorf("dangling link not reported: %v", res.Skipped)
	}
}

func TestScanEmptySystem(t *testing.T) {
	root := t.TempDir()
	res, err := Scan(context.Background(), xdg.Dirs{DataHome: filepath.Join(root, "none")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Apps) != 0 || len(res.Skipped) != 0 {
		t.Errorf("result = %+v, want it empty", res)
	}
}

func TestScanCancelled(t *testing.T) {
	s := newFakeSystem(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := Scan(ctx, s.dirs, "")
	if err == nil {
		t.Fatal("Scan on a cancelled context returned no error")
	}
	appErr := requireRoute(t, err, "Scan.ScanCancelled")
	if appErr.Tag != TagScanCancelled {
		t.Errorf("tag = %q, want %q", appErr.Tag, TagScanCancelled)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("the error does not wrap context.Canceled: %v", err)
	}
	if appErr.Metadata["folder"] != s.path("user/applications") {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	if len(res.Apps) != 0 || len(res.Skipped) != 0 {
		t.Errorf("a cancelled scan returned %+v", res)
	}
}

func TestLocaleFromEnv(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"nothing set", nil, ""},
		{"LANG only", map[string]string{"LANG": "de_DE.UTF-8"}, "de_DE.UTF-8"},
		{"LC_MESSAGES beats LANG", map[string]string{"LANG": "de_DE.UTF-8", "LC_MESSAGES": "fr_FR.UTF-8"}, "fr_FR.UTF-8"},
		{"LC_ALL beats both", map[string]string{"LANG": "de", "LC_MESSAGES": "fr", "LC_ALL": "el_GR"}, "el_GR"},
		{"empty values are skipped", map[string]string{"LC_ALL": "", "LC_MESSAGES": "", "LANG": "pt_BR"}, "pt_BR"},
		{"C means no locale", map[string]string{"LANG": "C"}, ""},
		{"POSIX means no locale", map[string]string{"LANG": "POSIX"}, ""},
		{"C.UTF-8 means no locale", map[string]string{"LANG": "C.UTF-8"}, ""},
		{"C in LC_ALL wins over LANG", map[string]string{"LC_ALL": "C", "LANG": "de_DE.UTF-8"}, ""},
		{"POSIX in LC_MESSAGES wins over LANG", map[string]string{"LC_MESSAGES": "POSIX", "LANG": "de_DE"}, ""},
		{"a language that starts with C", map[string]string{"LANG": "ca_ES.UTF-8"}, "ca_ES.UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocaleFromEnv(xdg.NewEnv("/home/u", tt.vars)); got != tt.want {
				t.Errorf("LocaleFromEnv = %q, want %q", got, tt.want)
			}
		})
	}
}
