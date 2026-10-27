package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
)

func TestPrepareGoldenVSCode(t *testing.T) {
	req := vscodeRequest(t, vscodeBuild(fakeHome+"/.local/share/jailcat/homes"))
	p, err := Prepare(context.Background(), fakeDirs(t), req)
	if err != nil {
		t.Fatal(err)
	}
	pv := p.Preview()
	golden(t, "vscode.profile", pv.Profile)
	golden(t, "vscode.sh", pv.Launcher)
	golden(t, "vscode.desktop", pv.Desktop)

	if p.IconFallback != "" {
		t.Errorf("IconFallback = %q, want none", p.IconFallback)
	}
	wantFiles := []struct {
		kind Kind
		path string
		mode os.FileMode
	}{
		{KindProfile, fakeHome + "/.local/share/jailcat/profiles/firejail-code.profile", 0o644},
		{KindLauncher, fakeHome + "/.local/bin/firejail-code", 0o755},
		{KindIcon, fakeHome + "/.local/share/icons/hicolor/scalable/apps/firejail-code.svg", 0o644},
		{KindDesktop, fakeHome + "/.local/share/applications/firejail-code.desktop", 0o644},
		{KindHome, fakeHome + "/.local/share/jailcat/homes/firejail-code", 0o700},
	}
	if len(p.Files) != len(wantFiles) {
		t.Fatalf("got %d files, want %d: %+v", len(p.Files), len(wantFiles), p.Files)
	}
	for i, w := range wantFiles {
		f := p.Files[i]
		if f.Kind != w.kind || f.Path != w.path || f.Mode != w.mode {
			t.Errorf("file %d = {%s %s %v}, want {%s %s %v}", i, f.Kind, f.Path, f.Mode, w.kind, w.path, w.mode)
		}
		sum := sha256.Sum256(f.Content)
		wantSum := hex.EncodeToString(sum[:])
		if f.Kind == KindHome {
			if f.SHA256 != "" || f.Content != nil {
				t.Errorf("the home has content or a SHA-256: %+v", f)
			}
			continue
		}
		if f.SHA256 != wantSum {
			t.Errorf("%s SHA256 = %s, want %s", f.Kind, f.SHA256, wantSum)
		}
	}
}

// entryOf parses the text of a desktop entry.
func entryOf(t *testing.T, text string) *desktopentry.Entry {
	t.Helper()
	e, err := desktopentry.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, text)
	}
	return e
}

// prepareApp prepares a build of app on the fake home and returns the desktop entry.
func prepareApp(t *testing.T, app apps.App, edit func(s *firejail.Settings)) (*Prepared, string) {
	t.Helper()
	s := firejail.DefaultSettings()
	s.SandboxName = "firejail-demo"
	s.MenuName = "Demo (sandboxed)"
	s.Comment = "A demo"
	if edit != nil {
		edit(&s)
	}
	p, err := Prepare(context.Background(), fakeDirs(t), Request{
		BuildID: 3, App: app, Settings: s, Profile: stubProfile(3), Firejail: "/usr/bin/firejail", IconTheme: "hicolor",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return p, p.Preview().Desktop
}

// entryApp is an app with a desktop entry of the given text, in a temporary folder.
func entryApp(t *testing.T, entry string) apps.App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.desktop")
	writeText(t, path, entry)
	return apps.App{
		ID: "demo.desktop", Kind: apps.KindDesktop, Name: "Demo", Binary: "/usr/bin/demo", BinaryName: "demo",
		DesktopPath: path,
	}
}

func TestDesktopEntryExecFieldCodes(t *testing.T) {
	const launcher = fakeHome + "/.local/bin/firejail-demo"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no code", nil, launcher},
		{"files", []string{"%F"}, launcher + " %F"},
		{"one file", []string{"%f"}, launcher + " %f"},
		{"urls", []string{"%U"}, launcher + " %U"},
		{"one url", []string{"%u"}, launcher + " %u"},
		{"the first of two", []string{"%U", "%F"}, launcher + " %U"},
		{"after other arguments", []string{"--new-window", "%f", "%u"}, launcher + " %f"},
		{"other codes are not kept", []string{"%i", "%c", "%k"}, launcher},
		{"the arguments are not kept", []string{"--flag", "value"}, launcher},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := entryApp(t, "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n")
			app.Args = tt.args
			_, text := prepareApp(t, app, nil)
			got, err := desktopentry.SplitExec(entryOf(t, text).Main().String("Exec"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := desktopentry.SplitExec(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("Exec words = %q, want %q\n%s", got, want, text)
			}
		})
	}
}

func TestDesktopEntryLauncherPathQuoted(t *testing.T) {
	dirs := fakeDirs(t)
	dirs.Bin = "/home/my user/it's bin"
	s := firejail.DefaultSettings()
	s.SandboxName = "firejail-demo"
	s.MenuName = "Demo"
	app := entryApp(t, "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n")
	app.Args = []string{"%U"}
	p, err := Prepare(context.Background(), dirs, Request{
		BuildID: 3, App: app, Settings: s, Profile: stubProfile(3), Firejail: "/usr/bin/firejail",
	})
	if err != nil {
		t.Fatal(err)
	}
	e := entryOf(t, p.Preview().Desktop)
	words, err := desktopentry.SplitExec(e.Main().String("Exec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 || words[0] != "/home/my user/it's bin/firejail-demo" || words[1] != "%U" {
		t.Errorf("Exec words = %q", words)
	}
}

func TestDesktopEntryKeys(t *testing.T) {
	full := "[Desktop Entry]\nType=Application\nName=Demo\nName[de]=Demo DE\nExec=demo %F\nIcon=demo\n" +
		"GenericName=Tool\nKeywords=a;b;\nCategories=Development;IDE;\nStartupNotify=false\nMimeType=text/plain;image/png;\n"
	tests := []struct {
		name  string
		entry string
		edit  func(s *firejail.Settings)
		want  map[string]string // key -> value ("" means the key must be missing)
	}{
		{"copied keys", full, nil, map[string]string{
			"Type": "Application", "Name": "Demo (sandboxed)", "Comment": "A demo", "Icon": "firejail-demo",
			"GenericName": "Tool", "Keywords": "a;b;", "Categories": "Development;IDE;", "StartupNotify": "false",
			"Terminal": "false", "MimeType": "", "X-Jailcat-Build": "3", "Name[de]": "",
		}},
		{"open file types", full, func(s *firejail.Settings) { s.OpenFileTypes = true }, map[string]string{
			"MimeType": "text/plain;image/png;",
		}},
		{"open file types without a MimeType", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n",
			func(s *firejail.Settings) { s.OpenFileTypes = true }, map[string]string{"MimeType": ""}},
		{"terminal on", full, func(s *firejail.Settings) { s.Terminal = true }, map[string]string{"Terminal": "true"}},
		{"no comment", full, func(s *firejail.Settings) { s.Comment = "" }, map[string]string{"Comment": ""}},
		{"no categories in the entry", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n", nil, map[string]string{
			"Categories": "", "GenericName": "", "Keywords": "", "StartupNotify": "",
		}},
		{"StartupNotify that is not a boolean is not copied", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\nStartupNotify=maybe\n",
			nil, map[string]string{"StartupNotify": ""}},
		{"special characters", full, func(s *firejail.Settings) {
			s.MenuName = "My; \"App\" \\ 日本"
			s.Comment = "line one\nline two; 100%"
		}, map[string]string{"Name": "My; \"App\" \\ 日本", "Comment": "line one\nline two; 100%"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, text := prepareApp(t, entryApp(t, tt.entry), tt.edit)
			g := entryOf(t, text).Main()
			for key, want := range tt.want {
				if want == "" {
					if g.Has(key) {
						t.Errorf("%s is set to %q, want it missing\n%s", key, g.String(key), text)
					}
					continue
				}
				if got := g.String(key); got != want {
					t.Errorf("%s = %q, want %q\n%s", key, got, want, text)
				}
			}
		})
	}
}

func TestDesktopEntryBinaryHasUtilityCategory(t *testing.T) {
	app := apps.App{ID: "tool", Kind: apps.KindBinary, Name: "tool", Binary: "/usr/bin/tool", BinaryName: "tool", Terminal: true}
	_, text := prepareApp(t, app, func(s *firejail.Settings) { s.Terminal = true })
	g := entryOf(t, text).Main()
	if got := g.Strings("Categories"); len(got) != 1 || got[0] != "Utility" {
		t.Errorf("Categories = %q\n%s", got, text)
	}
	if got, _ := g.Bool("Terminal"); !got {
		t.Errorf("Terminal is not true\n%s", text)
	}
	if got := g.String("StartupWMClass"); got != "tool" {
		t.Errorf("StartupWMClass = %q, want the binary's name", got)
	}
}

func TestDesktopEntryStartupWMClass(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		id    string
		want  string
	}{
		{"the entry's own", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\nStartupWMClass=Original\n", "demo.desktop", "Original"},
		{"the desktop ID without .desktop", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n", "org.example.Demo.desktop", "org.example.Demo"},
		{"a file ID with a folder", "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo\n", "kde-demo.desktop", "kde-demo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := entryApp(t, tt.entry)
			app.ID = tt.id
			_, text := prepareApp(t, app, nil)
			if got := entryOf(t, text).Main().String("StartupWMClass"); got != tt.want {
				t.Errorf("StartupWMClass = %q, want %q\n%s", got, tt.want, text)
			}
		})
	}
}

func TestDesktopEntryActions(t *testing.T) {
	const launcher = fakeHome + "/.local/bin/firejail-demo"
	entry := "[Desktop Entry]\nType=Application\nName=Demo\nExec=demo %U\nActions=same;other;plain;\n\n" +
		"[Desktop Action same]\nName=Same\nName[de]=Gleich\nName[pt_BR]=Mesmo\nExec=demo --new %U\n\n" +
		"[Desktop Action other]\nName=Other\nExec=other --x\n\n" +
		"[Desktop Action plain]\nName=Plain\nExec=demo --it's --\"quoted\" 100%% %F %f\n"
	app := entryApp(t, entry)
	app.Args = []string{"%U"}
	app.Actions = []apps.Action{
		{ID: "same", Name: "Same", Args: []string{"--new", "%U"}, SameBinary: true},
		{ID: "other", Name: "Other", Args: []string{"--x"}},
		{ID: "plain", Name: "Plain", Args: []string{"--it's", "--\"quoted\"", "100%", "%F", "%f"}, SameBinary: true},
	}
	_, text := prepareApp(t, app, nil)
	e := entryOf(t, text)

	if got := e.Main().Strings("Actions"); strings.Join(got, ",") != "same,plain" {
		t.Errorf("Actions = %q\n%s", got, text)
	}
	if e.Lookup("Desktop Action other") != nil {
		t.Errorf("an action of another binary was copied\n%s", text)
	}
	same := e.Lookup("Desktop Action same")
	if same == nil {
		t.Fatalf("no action group same\n%s", text)
	}
	if got := same.String("Name"); got != "Same" {
		t.Errorf("Name = %q", got)
	}
	if got := same.String("Name[de]"); got != "Gleich" {
		t.Errorf("Name[de] = %q", got)
	}
	if got := same.String("Name[pt_BR]"); got != "Mesmo" {
		t.Errorf("Name[pt_BR] = %q", got)
	}
	words, err := desktopentry.SplitExec(same.String("Exec"))
	if err != nil || strings.Join(words, "|") != launcher+"|--new|%U" {
		t.Errorf("same Exec words = %q, %v", words, err)
	}
	plain := e.Lookup("Desktop Action plain")
	words, err = desktopentry.SplitExec(plain.String("Exec"))
	if err != nil || strings.Join(words, "|") != launcher+"|--it's|--\"quoted\"|100%|%F" {
		t.Errorf("plain Exec words = %q, %v\n%s", words, err, text)
	}
}

func TestDesktopEntryActionLocalizedNamesGolden(t *testing.T) {
	// The VS Code fixture has three localized names; the golden shows them.
	_, text := prepareApp(t, vscodeApp(t), nil)
	for _, want := range []string{"Name[de]=Neues leeres Fenster", "Name[es]=Nueva ventana vacía", "Actions=new-empty-window;"} {
		if !strings.Contains(text, want) {
			t.Errorf("the entry lacks %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "open-docs") {
		t.Errorf("the action of another binary was copied\n%s", text)
	}
}

func TestPrepareLauncherPlace(t *testing.T) {
	tests := []struct {
		name string
		edit func(s *firejail.Settings)
		dir  string
	}{
		{"bin folder", nil, fakeHome + "/.local/bin"},
		{"launchers folder without InstallBin", func(s *firejail.Settings) { s.InstallBin = false }, fakeHome + "/.local/share/jailcat/launchers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, text := prepareApp(t, vscodeApp(t), tt.edit)
			l := file(t, p, KindLauncher)
			if l.Path != tt.dir+"/firejail-demo" {
				t.Errorf("launcher at %s, want it in %s", l.Path, tt.dir)
			}
			if l.Mode != 0o755 {
				t.Errorf("mode = %v", l.Mode)
			}
			if !strings.Contains(text, "Exec="+l.Path+" %F") {
				t.Errorf("Exec does not name the launcher %s:\n%s", l.Path, text)
			}
		})
	}
}

func TestPrepareExtraArgsInLauncher(t *testing.T) {
	p, _ := prepareApp(t, vscodeApp(t), func(s *firejail.Settings) { s.ExtraArgs = []string{"--ozone-platform=wayland", "it's"} })
	want := " -- '/usr/bin/code' '--ozone-platform=wayland' 'it'\\''s' \"$@\"\n"
	if got := file(t, p, KindLauncher).Content; !strings.HasSuffix(string(got), want) {
		t.Errorf("launcher = %q, want it to end with %q", got, want)
	}
}

func TestPrepareWithoutMenu(t *testing.T) {
	p, text := prepareApp(t, vscodeApp(t), func(s *firejail.Settings) { s.InstallMenu = false })
	if text != "" {
		t.Errorf("a desktop entry was made:\n%s", text)
	}
	for _, f := range p.Files {
		if f.Kind == KindDesktop {
			t.Errorf("the plan has a desktop entry: %+v", f)
		}
	}
	if len(p.Files) != 3 {
		t.Errorf("got %d files, want profile, launcher and icon", len(p.Files))
	}
}

func TestPrepareHome(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"absolute", fakeHome + "/.local/share/jailcat/homes/x", fakeHome + "/.local/share/jailcat/homes/x"},
		{"${HOME}", "${HOME}/.local/share/jailcat/homes/x", fakeHome + "/.local/share/jailcat/homes/x"},
		{"tilde", "~/.local/share/jailcat/homes/x", fakeHome + "/.local/share/jailcat/homes/x"},
		{"unclean", fakeHome + "/.local/share/jailcat/homes/a/../x/", fakeHome + "/.local/share/jailcat/homes/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := prepareApp(t, vscodeApp(t), func(s *firejail.Settings) {
				s.Home = firejail.HomePrivate
				s.PrivateDir = tt.dir
			})
			h := file(t, p, KindHome)
			if h.Path != tt.want || h.Mode != 0o700 {
				t.Errorf("home = %s %v, want %s 0700", h.Path, h.Mode, tt.want)
			}
		})
	}

	// No other home choice makes a home folder.
	for _, mode := range []firejail.HomeMode{firejail.HomeFull, firejail.HomeChosen, firejail.HomeEmpty} {
		p, _ := prepareApp(t, vscodeApp(t), func(s *firejail.Settings) { s.Home = mode; s.PrivateDir = "/whatever" })
		for _, f := range p.Files {
			if f.Kind == KindHome {
				t.Errorf("home choice %q made a home folder", mode)
			}
		}
	}
}

func TestPrepareIconModes(t *testing.T) {
	p, _ := prepareApp(t, vscodeApp(t), nil)
	if p.IconFallback != "" {
		t.Errorf("IconFallback = %q", p.IconFallback)
	}
	icon := file(t, p, KindIcon)
	if !bytes.Contains(icon.Content, []byte("<!-- Generated by Jailcat.")) {
		t.Errorf("the icon has no header comment")
	}
	if icon.Path != fakeHome+"/.local/share/icons/hicolor/scalable/apps/firejail-demo.svg" {
		t.Errorf("icon at %s", icon.Path)
	}

	// Random: no lookup at all, even when the app has no icon.
	app := vscodeApp(t)
	app.Icon = ""
	p, _ = prepareApp(t, app, func(s *firejail.Settings) { s.Icon = firejail.IconRandom; s.IconSeed = 5 })
	if p.IconFallback != "" {
		t.Errorf("a random icon by choice has a fallback reason: %q", p.IconFallback)
	}
	again, _ := prepareApp(t, app, func(s *firejail.Settings) { s.Icon = firejail.IconRandom; s.IconSeed = 5 })
	other, _ := prepareApp(t, app, func(s *firejail.Settings) { s.Icon = firejail.IconRandom; s.IconSeed = 6 })
	if !bytes.Equal(file(t, p, KindIcon).Content, file(t, again, KindIcon).Content) {
		t.Error("the same seed gave two icons")
	}
	if bytes.Equal(file(t, p, KindIcon).Content, file(t, other, KindIcon).Content) {
		t.Error("two seeds gave the same icon")
	}
}

func TestPrepareIconFallback(t *testing.T) {
	tests := []struct {
		name string
		icon string
		want string // a part of the reason
	}{
		{"no icon", "", "has no icon"},
		{"missing icon", "no-such-icon-anywhere-xyz", `"no-such-icon-anywhere-xyz" was not found`},
		{"XPM icon", absPath(t, "icons", "code.xpm"), "XPM format"},
		{"broken SVG", absPath(t, "icons", "broken.svg"), "cannot be used"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := vscodeApp(t)
			app.Icon = tt.icon
			p, _ := prepareApp(t, app, func(s *firejail.Settings) { s.IconSeed = 9 })
			if !strings.Contains(p.IconFallback, tt.want) {
				t.Errorf("IconFallback = %q, want it to hold %q", p.IconFallback, tt.want)
			}
			// It is a random icon.
			icon := file(t, p, KindIcon).Content
			if err := xml.NewDecoder(bytes.NewReader(icon)).Decode(new(struct{ XMLName xml.Name })); err != nil {
				t.Errorf("the fallback icon is not XML: %v", err)
			}
			if !bytes.Contains(icon, []byte("jcr-background")) {
				t.Errorf("the fallback is not a random icon")
			}
		})
	}
}

func TestPrepareTagsRoutes(t *testing.T) {
	ctx := context.Background()
	dirs := fakeDirs(t)
	good := func() Request {
		s := firejail.DefaultSettings()
		s.SandboxName = "firejail-demo"
		s.MenuName = "Demo"
		return Request{BuildID: 3, App: vscodeApp(t), Settings: s, Profile: stubProfile(3), Firejail: "/usr/bin/firejail"}
	}
	tests := []struct {
		name  string
		edit  func(r *Request)
		tag   string
		route string
	}{
		{"empty sandbox name", func(r *Request) { r.Settings.SandboxName = "" }, TagSandboxNameInvalid, "Prepare." + TagSandboxNameInvalid},
		{"sandbox name that escapes", func(r *Request) { r.Settings.SandboxName = "../../evil" }, TagSandboxNameInvalid, "Prepare." + TagSandboxNameInvalid},
		{"sandbox name with a capital", func(r *Request) { r.Settings.SandboxName = "Firejail" }, TagSandboxNameInvalid, "Prepare." + TagSandboxNameInvalid},
		{"no profile", func(r *Request) { r.Profile = nil }, tagProfileMissing, "Prepare." + tagProfileMissing},
		{"bad icon mode", func(r *Request) { r.Settings.Icon = "neon" }, tagIconModeInvalid, "Prepare->iconFile." + tagIconModeInvalid},
		{"empty menu name", func(r *Request) { r.Settings.MenuName = " " }, tagMenuNameEmpty, "Prepare->desktopFile." + tagMenuNameEmpty},
		{"app entry missing", func(r *Request) { r.App.DesktopPath = filepath.Join(t.TempDir(), "gone.desktop") }, tagAppEntryReadFailed,
			"Prepare->desktopFile->readAppEntry." + tagAppEntryReadFailed},
		{"app entry that does not parse", func(r *Request) { r.App.DesktopPath = absPath(t, "icons", "code.xpm") }, "",
			"Prepare->desktopFile->readAppEntry"},
		{"firejail not absolute", func(r *Request) { r.Firejail = "firejail" }, "LauncherFirejailInvalid", "Prepare->launcherFile"},
		{"binary not absolute", func(r *Request) { r.App.Binary = "code" }, "LauncherBinaryInvalid", "Prepare->launcherFile"},
		{"private dir empty", func(r *Request) { r.Settings.Home = firejail.HomePrivate }, tagPrivateDirInvalid, "Prepare->homeFile." + tagPrivateDirInvalid},
		{"private dir relative", func(r *Request) { r.Settings.Home = firejail.HomePrivate; r.Settings.PrivateDir = "homes/x" },
			tagPrivateDirInvalid, "Prepare->homeFile." + tagPrivateDirInvalid},
		{"private dir outside the homes folder", func(r *Request) { r.Settings.Home = firejail.HomePrivate; r.Settings.PrivateDir = "${HOME}/Documents" },
			tagPrivateHomeOutside, "Prepare->homeFile." + tagPrivateHomeOutside},
		{"private dir is the homes folder", func(r *Request) {
			r.Settings.Home = firejail.HomePrivate
			r.Settings.PrivateDir = fakeHome + "/.local/share/jailcat/homes"
		}, tagPrivateHomeOutside, "Prepare->homeFile." + tagPrivateHomeOutside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := good()
			tt.edit(&r)
			_, err := Prepare(ctx, dirs, r)
			if err == nil {
				t.Fatal("Prepare succeeded")
			}
			if tt.tag == "" {
				requireRouteOnly(t, err, tt.route)
				return
			}
			requireRoute(t, err, tt.tag, tt.route)
		})
	}
}

func TestPrepareBadgeIconCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := firejail.DefaultSettings()
	s.SandboxName = "firejail-demo"
	s.MenuName = "Demo"
	_, err := Prepare(ctx, fakeDirs(t), Request{BuildID: 3, App: vscodeApp(t), Settings: s, Profile: stubProfile(3), Firejail: "/usr/bin/firejail"})
	requireRoute(t, err, icons.TagBadgeCanceled, "Prepare->iconFile->badgeIcon->BadgeIcon")
}

func TestPreviewTexts(t *testing.T) {
	p, desktop := prepareApp(t, vscodeApp(t), nil)
	pv := p.Preview()
	if pv.Desktop != desktop || pv.Profile != string(file(t, p, KindProfile).Content) || pv.Launcher != string(file(t, p, KindLauncher).Content) {
		t.Errorf("Preview does not match the files: %+v", pv)
	}
	for _, text := range []string{pv.Profile, pv.Launcher, pv.Desktop} {
		if !strings.Contains(text, firejail.HeaderLine) || !strings.Contains(text, "jailcat-build: 3") {
			t.Errorf("a text lacks the header:\n%s", text)
		}
	}
}
