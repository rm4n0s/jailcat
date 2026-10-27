package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// addItem types text in the field of the list and presses Add.
func (p *page) addItem(prefix, text string) {
	p.t.Helper()
	p.ts.EnterText(key(prefix+":field"), text)
	p.ts.Pump()
	p.tap(prefix + ":add")
}

// problemShows reports whether the widget with the key has a text with the part in it.
func (p *page) problemShows(k, text string) bool {
	return p.ts.Exists(gluttertest.Descendant(key(k), gluttertest.ByTextContaining(text)))
}

func TestProgramsTabRestrictedLists(t *testing.T) {
	tests := []struct {
		name   string
		field  firejail.Field
		prefix string
		get    func(s firejail.Settings) []string
		item   string
		bad    []struct{ text, message string }
	}{
		{"bin", firejail.FieldPrivateBin, "bin", func(s firejail.Settings) []string { return s.PrivateBin }, "git", []struct{ text, message string }{
			{"/usr/bin/git", "without a /"}, {"a,b", "comma"}, {"git", "in the list already"},
		}},
		{"etc", firejail.FieldPrivateEtc, "etc", func(s firejail.Settings) []string { return s.PrivateEtc }, "ssl", []struct{ text, message string }{
			{"/etc/ssl", "has a / in it"}, {"ssl/certs", "has a / in it"}, {"a,b", "comma"}, {"ssl", "in the list already"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tabPage(t, "Calculator", firejail.TabPrograms)
			e := p.home().editor
			sw := "toggle:" + string(tt.field)
			mark := "mark:" + string(tt.field)
			if p.switchedOn(sw) || p.ts.Exists(key(tt.prefix+":field")) || p.dots() != 0 {
				t.Fatal("a fresh build is not off")
			}

			// On with an empty list is not a change: it restricts nothing yet.
			p.tap(sw)
			if !p.switchedOn(sw) || !p.ts.Exists(key(tt.prefix+":field")) || e.HasChanges() || p.ts.Exists(key(mark)) {
				t.Fatal("the switch did not show the list")
			}

			p.addItem(tt.prefix, "  "+tt.item+" ")
			if got := tt.get(e.Final()); !slices.Equal(got, []string{tt.item}) {
				t.Fatalf("the list is %q after adding %q", got, tt.item)
			}
			if !p.ts.Exists(key(mark)) || !p.tabDot(firejail.TabPrograms) || !p.ts.Exists(gluttertest.Descendant(key(tt.prefix+":row:0"), gluttertest.ByText(tt.item))) {
				t.Error("the item is not shown as a change")
			}
			if p.textOf(tt.prefix+":field") != "" {
				t.Error("the field keeps the text after Add")
			}

			for _, bad := range tt.bad {
				p.addItem(tt.prefix, bad.text)
				if !p.problemShows(tt.prefix+":problem", bad.message) {
					t.Errorf("%q: no message with %q", bad.text, bad.message)
				}
				if got := tt.get(e.Final()); !slices.Equal(got, []string{tt.item}) {
					t.Errorf("%q was stored: %q", bad.text, got)
				}
				p.ts.EnterText(key(tt.prefix+":field"), "")
				p.ts.Pump()
				if p.ts.Exists(key(tt.prefix + ":problem")) {
					t.Errorf("%q: the message stays after the text is cleared", bad.text)
				}
			}

			// Removing the last item leaves the switch on and nothing changed.
			p.tap(tt.prefix + ":remove:0")
			if len(tt.get(e.Final())) != 0 || e.HasChanges() || !p.switchedOn(sw) {
				t.Errorf("removing left %q, changes %v", tt.get(e.Final()), e.HasChanges())
			}

			// Off clears the list.
			p.addItem(tt.prefix, tt.item)
			p.tap(sw)
			if len(tt.get(e.Final())) != 0 || p.switchedOn(sw) || p.ts.Exists(key(tt.prefix+":field")) || e.HasChanges() {
				t.Error("the switch did not turn the list off")
			}

			// Reset puts the switch back off too.
			p.tap(sw)
			p.addItem(tt.prefix, tt.item)
			p.reset(mark)
			if len(tt.get(e.Final())) != 0 || p.switchedOn(sw) || e.Changed(tt.field) || p.tabDot(firejail.TabPrograms) {
				t.Error("Reset did not turn the list off")
			}
		})
	}
}

func TestProgramsTabListOfTheProfile(t *testing.T) {
	// A build that already has a list shows the switch on, and turning it off is the change.
	p := tabPage(t, "Calculator", firejail.TabPrograms)
	e := p.home().editor
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.PrivateBin = []string{"sh", "git"} }) })
	if !p.switchedOn("toggle:private_bin") || !p.ts.Exists(key("bin:row:1")) {
		t.Fatal("the list is not shown")
	}
	p.tap("bin:remove:0")
	if want := []string{"git"}; !slices.Equal(e.Final().PrivateBin, want) {
		t.Fatalf("PrivateBin = %q, want %q", e.Final().PrivateBin, want)
	}
	p.tap("toggle:private_bin")
	if len(e.Final().PrivateBin) != 0 || p.switchedOn("toggle:private_bin") {
		t.Error("the switch did not clear the list")
	}
	p.change(func(e *Editor) { e.Reset(firejail.FieldPrivateBin) })
	if e.Changed(firejail.FieldPrivateBin) || p.switchedOn("toggle:private_bin") {
		t.Error("Reset left the list")
	}
}

func TestProgramsTabBinNote(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabPrograms)
	if p.ts.Exists(key("note:bin")) {
		t.Fatal("the note about the check shows while the list is off")
	}
	p.tap("toggle:private_bin")
	if !p.ts.Exists(key("note:bin")) || !p.ts.Exists(gluttertest.ByTextContaining("check after Build")) {
		t.Error("the note about the build check is missing")
	}
}

func TestProgramsTabEnv(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabPrograms)
	e := p.home().editor
	add := func(name, value string) {
		p.t.Helper()
		p.ts.EnterText(key("env:name"), name)
		p.ts.EnterText(key("env:value"), value)
		p.tap("env:add")
	}
	if !p.ts.Exists(gluttertest.ByText(envEmptyText)) {
		t.Error("an empty list says so")
	}
	add(" LANG ", "C")
	add("EDITOR_OPTS", "--wait now")
	want := []firejail.EnvVar{{Name: "LANG", Value: "C"}, {Name: "EDITOR_OPTS", Value: "--wait now"}}
	if got := e.Final().Env; !slices.Equal(got, want) {
		t.Fatalf("Env = %+v, want %+v", got, want)
	}
	if !p.ts.Exists(key("mark:env")) || !p.tabDot(firejail.TabPrograms) {
		t.Error("the variables are not shown as a change")
	}
	if !p.ts.Exists(gluttertest.Descendant(key("env:row:1"), gluttertest.ByText("EDITOR_OPTS=--wait now"))) {
		t.Error("the row does not show NAME=value")
	}
	if p.textOf("env:name") != "" || p.textOf("env:value") != "" {
		t.Error("the fields keep their text after Add")
	}

	tests := []struct{ name, value, message string }{
		{"1BAD", "x", "not a variable name"},
		{"has space", "x", "not a variable name"},
		{"", "x", "not a variable name"},
		{"GOOD", "has # hash", "#"},
		{"GOOD", " leading", "blank"},
		{"LANG", "other", "in the list already"},
	}
	for _, tt := range tests {
		add(tt.name, tt.value)
		if !p.problemShows("env:problem", tt.message) {
			t.Errorf("%q=%q: no message with %q", tt.name, tt.value, tt.message)
		}
		if len(e.Final().Env) != 2 {
			t.Errorf("%q=%q was stored: %+v", tt.name, tt.value, e.Final().Env)
		}
		// The text stays so the user can fix it; typing clears the message.
		p.ts.EnterText(key("env:name"), "X")
		if p.ts.Exists(key("env:problem")) {
			t.Errorf("%q: the message stays after typing", tt.name)
		}
		p.ts.EnterText(key("env:name"), "")
		p.ts.EnterText(key("env:value"), "")
	}

	// A variable with an empty value is allowed.
	add("EMPTY", "")
	if len(e.Final().Env) != 3 || e.Final().Env[2].Name != "EMPTY" {
		t.Errorf("Env = %+v", e.Final().Env)
	}

	p.tap("env:remove:0")
	if got := e.Final().Env; len(got) != 2 || got[0].Name != "EDITOR_OPTS" {
		t.Errorf("after the removal Env = %+v", got)
	}
	p.reset("mark:env")
	if len(e.Final().Env) != 0 || e.Changed(firejail.FieldEnv) {
		t.Errorf("Reset left %+v", e.Final().Env)
	}
}

func TestProgramsTabExtraArgs(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabPrograms)
	e := p.home().editor
	p.addItem("args", "--disable-gpu")
	p.addItem("args", `it's "quoted" $HOME`)
	p.addItem("args", "--disable-gpu") // a word can be given twice
	p.addItem("args", "two words")
	want := []string{"--disable-gpu", `it's "quoted" $HOME`, "--disable-gpu", "two words"}
	if got := e.Final().ExtraArgs; !slices.Equal(got, want) {
		t.Fatalf("ExtraArgs = %q, want %q", got, want)
	}
	if !p.ts.Exists(key("mark:extra_args")) || !p.tabDot(firejail.TabPrograms) {
		t.Error("the arguments are not shown as a change")
	}
	// The rows show the words as the launcher quotes them.
	for i, shown := range []string{`'--disable-gpu'`, `'it'\''s "quoted" $HOME'`, `'--disable-gpu'`, `'two words'`} {
		if !p.ts.Exists(gluttertest.Descendant(key("args:row:"+string(rune('0'+i))), gluttertest.ByText(shown))) {
			t.Errorf("row %d does not show %s", i, shown)
		}
	}

	// A word the launcher cannot write is refused.
	p.addItem("args", "bad\x00word")
	if !p.problemShows("args:problem", "NUL") || len(e.Final().ExtraArgs) != 4 {
		t.Errorf("a NUL was not refused: %q", e.Final().ExtraArgs)
	}

	p.tap("args:remove:1")
	if want := []string{"--disable-gpu", "--disable-gpu", "two words"}; !slices.Equal(e.Final().ExtraArgs, want) {
		t.Errorf("after the removal ExtraArgs = %q", e.Final().ExtraArgs)
	}
	p.reset("mark:extra_args")
	if len(e.Final().ExtraArgs) != 0 || e.Changed(firejail.FieldExtraArgs) {
		t.Errorf("Reset left %q", e.Final().ExtraArgs)
	}
}

func TestQuoteForDisplay(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"two words", "'two words'"},
		{"it's", `'it'\''s'`},
		{"$HOME `id`", "'$HOME `id`'"},
		{"", "''"},
		{"новый", "'новый'"},
		{"line\nbreak", "'line\nbreak'"},
		{"nul\x00here", "nul\x00here"}, // cannot be quoted: shown as it is, the list refuses it
	}
	for _, tt := range tests {
		if got := quoteForDisplay(tt.in); got != tt.want {
			t.Errorf("quoteForDisplay(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestListCheck(t *testing.T) {
	e := tabPage(t, "Calculator", firejail.TabPrograms).home().editor
	check := listCheck(e, firejail.FieldPrivateBin, true, nil, "dup", addTo(binList().list))
	tests := []struct {
		text    string
		items   []string
		item    string
		message string
	}{
		{" git ", nil, "git", ""},
		{"git", []string{"git"}, "", "dup"},
		{"a,b", nil, "", "comma"},
		{"a#b", nil, "", "#"},
		{"two  spaces", nil, "", "two blanks"},
	}
	for _, tt := range tests {
		item, problem := check(tt.text, tt.items)
		if item != tt.item || !strings.Contains(problem, tt.message) {
			t.Errorf("check(%q) = %q, %q; want %q, a message with %q", tt.text, item, problem, tt.item, tt.message)
		}
	}
}

func TestGoldenProgramsTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabPrograms)
	// One change: only the program git.
	p.tap("toggle:" + string(firejail.FieldPrivateBin))
	p.addItem("bin", "git")
	p.golden("programs-tab", "4.5-programs.png")
}
