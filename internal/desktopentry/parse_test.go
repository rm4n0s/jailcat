package desktopentry

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func groupNames(e *Entry) []string {
	var names []string
	for _, g := range e.Groups() {
		names = append(names, g.Name())
	}
	return names
}

func TestParseFixtures(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		groups     []string
		strings    map[string]string   // key of the main group -> String
		lists      map[string][]string // key of the main group -> Strings
		actions    []string
		actionExec []string
	}{
		{
			name:   "VS Code",
			file:   "code.desktop",
			groups: []string{"Desktop Entry", "Desktop Action new-empty-window"},
			strings: map[string]string{
				"Name": "Visual Studio Code", "Exec": "/usr/share/code/code %F", "StartupWMClass": "Code",
				"Type": "Application", "Icon": "vscode", "Comment": "Code Editing. Redefined.",
			},
			lists: map[string][]string{
				"Categories": {"TextEditor", "Development", "IDE"},
				"MimeType":   {"application/x-code-workspace", "text/plain"},
				"Keywords":   {"vscode"},
				"Actions":    {"new-empty-window"},
			},
			actions:    []string{"new-empty-window"},
			actionExec: []string{"/usr/share/code/code --new-window %F"},
		},
		{
			name: "Firefox with actions",
			file: "firefox.desktop",
			groups: []string{
				"Desktop Entry", "Desktop Action new-window", "Desktop Action new-private-window",
				"Desktop Action profile-manager-window",
			},
			strings: map[string]string{"Name": "Firefox", "Exec": "firefox %u", "Name[de]": "Firefox Webbrowser"},
			lists: map[string][]string{
				"Keywords":     {"Internet", "WWW", "Browser", "Web", "Explorer"},
				"Keywords[de]": {"Internet", "WWW", "Browser", "Web", "Explorer", "Webseite"},
				"MimeType":     {"text/html", "text/xml", "application/xhtml+xml", "x-scheme-handler/http", "x-scheme-handler/https"},
				"Actions":      {"new-window", "new-private-window", "profile-manager-window"},
				"Categories":   {"Network", "WebBrowser"},
				"OnlyShowIn":   nil,
			},
			actions: []string{"new-window", "new-private-window", "profile-manager-window"},
			actionExec: []string{
				"firefox --new-window %u", "firefox --private-window %u", "firefox --ProfileManager",
			},
		},
		{
			name:   "quoted Exec",
			file:   "quoted-exec.desktop",
			groups: []string{"Desktop Entry", "Desktop Action geo"},
			strings: map[string]string{
				"Exec": `"/opt/My App/bin/app" --flag "a b" %F`,
			},
			actions: []string{"geo"},
			actionExec: []string{
				`kde-geo-uri-handler --coordinate-template "https://example.org/?lat=<LAT>&lon=<LON>" --fallback "https://example.org" %u`,
			},
		},
		{
			name:    "carriage returns are dropped",
			file:    "crlf.desktop",
			groups:  []string{"Desktop Entry"},
			strings: map[string]string{"Name": "Windows Line Ends", "Exec": "app"},
		},
		{
			name:   "comments, spaces around = and escapes",
			file:   "comments.desktop",
			groups: []string{"Desktop Entry", "X-Extra Group"},
			strings: map[string]string{
				"Type": "Application", "Name": "Spaced Out", "Name[de_DE@euro]": "Abstand",
				"Comment": "Line one\nLine two\\ with a backslash", "Path": " leading and trailing ",
			},
			lists: map[string][]string{"Keywords": {"a;b", "c", ""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := parseFixture(t, tt.file)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := groupNames(e); !reflect.DeepEqual(got, tt.groups) {
				t.Errorf("groups = %q, want %q", got, tt.groups)
			}
			main := e.Main()
			if main == nil {
				t.Fatal("no main group")
			}
			for key, want := range tt.strings {
				if got := main.String(key); got != want {
					t.Errorf("String(%q) = %q, want %q", key, got, want)
				}
			}
			for key, want := range tt.lists {
				if got := main.Strings(key); !reflect.DeepEqual(got, want) {
					t.Errorf("Strings(%q) = %q, want %q", key, got, want)
				}
			}
			var ids, execs []string
			for _, g := range e.Actions() {
				id, ok := g.ActionID()
				if !ok {
					t.Errorf("group %q is not an action", g.Name())
				}
				ids = append(ids, id)
				execs = append(execs, g.String("Exec"))
			}
			if !reflect.DeepEqual(ids, tt.actions) {
				t.Errorf("Actions() ids = %q, want %q", ids, tt.actions)
			}
			if !reflect.DeepEqual(execs, tt.actionExec) {
				t.Errorf("Actions() Exec = %q, want %q", execs, tt.actionExec)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		route string
		line  int
	}{
		{"key before any group", "broken-key-before-group.desktop", "Parse.KeyBeforeGroup", 2},
		{"line without equals sign", "broken-malformed-line.desktop", "Parse.MalformedLine", 3},
		{"group header without closing bracket", "broken-bad-group-header.desktop", "Parse.MalformedLine", 1},
		{"key name with a space", "broken-bad-key-name.desktop", "Parse.MalformedLine", 3},
		{"duplicate group", "broken-duplicate-group.desktop", "Parse.DuplicateGroup", 5},
		{"duplicate key", "broken-duplicate-key.desktop", "Parse.DuplicateKey", 4},
		{"invalid UTF-8", "broken-invalid-utf8.desktop", "Parse.InvalidUTF8", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFixture(t, tt.file)
			if err == nil {
				t.Fatal("expected an error")
			}
			appErr := requireRoute(t, err, tt.route)
			if got := appErr.Metadata["line"]; got != tt.line {
				t.Errorf("line = %v, want %d", got, tt.line)
			}
		})
	}
}

func TestParseErrorsInline(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		route string
		line  int
	}{
		{"empty group name", "[]\n", "Parse.MalformedLine", 1},
		{"group name with a bracket", "[a[b]\n", "Parse.MalformedLine", 1},
		{"text after the group header", "[Desktop Entry] x\n", "Parse.MalformedLine", 1},
		{"empty key name", "[G]\n=x\n", "Parse.MalformedLine", 2},
		{"empty locale", "[G]\nName[]=x\n", "Parse.MalformedLine", 2},
		{"locale without closing bracket", "[G]\nName[de=x\n", "Parse.MalformedLine", 2},
		{"NUL in a line", "[G]\nName=a\x00b\n", "Parse.MalformedLine", 2},
		{"same localized key twice", "[G]\nName[de]=a\nName[de]=b\n", "Parse.DuplicateKey", 3},
		{"same key in two groups is fine, the group twice is not", "[A]\n[B]\n[A]\n", "Parse.DuplicateGroup", 3},
		{"line 1 is checked for UTF-8 too", "[G\xff]\n", "Parse.InvalidUTF8", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.text))
			if err == nil {
				t.Fatal("expected an error")
			}
			appErr := requireRoute(t, err, tt.route)
			if got := appErr.Metadata["line"]; got != tt.line {
				t.Errorf("line = %v, want %d", got, tt.line)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("disk on fire") }

func TestParseReadFailed(t *testing.T) {
	_, err := Parse(failingReader{})
	if err == nil {
		t.Fatal("expected an error")
	}
	checkRoute(t, err, "Parse.ReadFailed")
}

func TestParseKeepsFileOrder(t *testing.T) {
	e := mustParse(t, "# top\n\n[B]\nz=1\na=2\n\n[A]\nm=3\n")
	if got, want := groupNames(e), []string{"B", "A"}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups = %q, want %q", got, want)
	}
	keys := e.Lookup("B").Keys()
	if len(keys) != 2 || keys[0].Name != "z" || keys[1].Name != "a" || keys[0].Raw != "1" {
		t.Errorf("keys = %+v", keys)
	}
	if e.Lookup("C") != nil {
		t.Error("Lookup of a missing group should be nil")
	}
	if e.Main() != nil {
		t.Error("Main of an entry with no [Desktop Entry] should be nil")
	}
}

func TestParseEmptyAndNoFinalNewline(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		groups []string
	}{
		{"empty file", "", nil},
		{"only comments", "# a\n\n# b\n", nil},
		{"no final newline", "[A]\nk=v", []string{"A"}},
		{"header without newline", "[A]", []string{"A"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := mustParse(t, tt.text)
			if got := groupNames(e); !reflect.DeepEqual(got, tt.groups) {
				t.Errorf("groups = %q, want %q", got, tt.groups)
			}
		})
	}
}

func TestKeysSplitLocale(t *testing.T) {
	e := mustParse(t, "[G]\nName=a\nName[de_DE@euro]=b\n")
	got := e.Lookup("G").Keys()
	want := []Key{{Name: "Name", Raw: "a"}, {Name: "Name", Locale: "de_DE@euro", Raw: "b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %+v, want %+v", got, want)
	}
	if got[1].FullName() != "Name[de_DE@euro]" || got[0].FullName() != "Name" {
		t.Errorf("FullName = %q, %q", got[0].FullName(), got[1].FullName())
	}
}

func TestActionsOrderAndGaps(t *testing.T) {
	text := "[Desktop Entry]\nActions=b;missing;a;b;\n\n[Desktop Action a]\nName=A\n\n[Desktop Action b]\nName=B\n\n[Desktop Action unlisted]\nName=U\n"
	e := mustParse(t, text)
	var ids []string
	for _, g := range e.Actions() {
		id, _ := g.ActionID()
		ids = append(ids, id)
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("Actions() = %q, want %q", ids, want)
	}
	if _, ok := e.Main().ActionID(); ok {
		t.Error("the main group is not an action")
	}
	if got := mustParse(t, "[X]\n").Actions(); got != nil {
		t.Errorf("Actions() without a main group = %v", got)
	}
}

// TestFixturesExist guards against a fixture being renamed without its test.
func TestFixturesExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Errorf("only %d fixtures in testdata", len(files))
	}
	if _, err := os.Stat(filepath.Join("testdata", "built-entry.golden")); err != nil {
		t.Error(err)
	}
}
