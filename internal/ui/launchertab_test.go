package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// launcherPage opens the Launcher tab of a new build of the app.
func launcherPage(t *testing.T, app string) *page {
	t.Helper()
	return tabPage(t, app, firejail.TabLauncher)
}

// sandboxName is the sandbox name the editor has.
func sandboxName(p *page) string { return p.home().editor.Final().SandboxName }

// textHas reports whether the Text with the key holds the part.
func (p *page) textHas(k, part string) bool {
	p.t.Helper()
	if !p.ts.Exists(key(k)) {
		return false
	}
	text, ok := p.ts.Widget(key(k)).(*m.Text)
	return ok && strings.Contains(text.Data().OrElse(""), part)
}

// previewAsks returns the settings PreviewIcon was asked for.
func (p *page) previewAsks() []firejail.Settings {
	p.backend.mu.Lock()
	defer p.backend.mu.Unlock()
	return slices.Clone(p.backend.previewAsks)
}

// nameAsks returns the names SandboxNameTaken was asked about.
func (p *page) nameAsks() []string {
	p.backend.mu.Lock()
	defer p.backend.mu.Unlock()
	return slices.Clone(p.backend.nameAsks)
}

// waitPreview waits until the preview of the icon is on screen.
func (p *page) waitPreview() {
	p.t.Helper()
	p.waitFor("the icon preview", func() bool {
		return p.ts.Exists(gluttertest.Descendant(key("icon:preview"), gluttertest.ByType[*svg.SvgPicture]()))
	})
}

func TestLauncherTabShowsTheBuild(t *testing.T) {
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	p.waitPreview()

	if got := p.textOf("sandbox:field"); got != "firejail-calculator" {
		t.Errorf("sandbox name field = %q", got)
	}
	if got := p.textOf("menu:field"); got != "Calculator (sandboxed)" {
		t.Errorf("menu name field = %q", got)
	}
	if got := p.textOf("comment:field"); got != "Do sums" {
		t.Errorf("comment field = %q", got)
	}
	want := map[firejail.Field]bool{
		firejail.FieldInstallMenu: true, firejail.FieldInstallBin: true, firejail.FieldOpenFileTypes: false,
		firejail.FieldSingleInstance: false, firejail.FieldTerminal: false, firejail.FieldQuiet: true,
	}
	for f, on := range want {
		if got := p.switchedOn("toggle:" + string(f)); got != on {
			t.Errorf("%s is %v, want %v", f, got, on)
		}
		if !p.ts.Exists(gluttertest.Descendant(key("toggle:"+string(f)), gluttertest.ByText(settingLabel(f)))) {
			t.Errorf("no switch labelled %q", settingLabel(f))
		}
	}
	if p.dots() != 0 || p.ts.Exists(gluttertest.ByText(changedText)) || e.HasChanges() {
		t.Error("a fresh build shows a change")
	}
	// Nothing is wrong, and the old name is not mentioned.
	for _, k := range []string{"problem:sandbox", "problem:menu", "problem:install", "note:rename", "icon:fallback", "icon:failed"} {
		if p.ts.Exists(key(k)) {
			t.Errorf("%s is shown for a fresh build", k)
		}
	}
	// The sandbox name field has no mark: the name says which build this is, not a change.
	if p.ts.Exists(key("mark:" + string(firejail.FieldSandboxName))) {
		t.Error("the sandbox name has a mark")
	}
}

func TestLauncherTabSandboxNameIsChecked(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		message string
	}{
		{"capital letters", "Firejail-Code", "lowercase letters"},
		{"a space", "my sandbox", "lowercase letters"},
		{"an underscore", "my_sandbox", "lowercase letters"},
		{"slash", "../evil", "lowercase letters"},
		{"empty", "", "at least one character"},
		{"non-ASCII", "firejail-café", "lowercase letters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := launcherPage(t, "Calculator")
			p.typeInto("sandbox:field", tt.text)
			if !p.problemShows("problem:sandbox", tt.message) {
				t.Fatalf("no message with %q for %q", tt.message, tt.text)
			}
			if got := sandboxName(p); got != "firejail-calculator" {
				t.Errorf("the refused name %q was stored as %q", tt.text, got)
			}
			if got := p.textOf("sandbox:field"); got != tt.text {
				t.Errorf("the field lost the text %q (it shows %q)", tt.text, got)
			}
			if asks := p.nameAsks(); len(asks) != 0 {
				t.Errorf("the backend was asked about %q", asks)
			}
		})
	}
}

func TestLauncherTabSandboxNameUniqueness(t *testing.T) {
	p := launcherPage(t, "Calculator")
	p.backend.set(func(f *fakeBackend) {
		f.builds = append(f.builds, storeBuild("firejail-taken", 9))
	})

	// A name another build has is explained and not stored.
	p.typeInto("sandbox:field", "firejail-taken")
	p.waitFor("the name to be refused", func() bool { return p.ts.Exists(key("problem:sandbox")) })
	if !p.problemShows("problem:sandbox", "Another build already has this name") {
		t.Error("the message does not say another build has the name")
	}
	if got := sandboxName(p); got != "firejail-calculator" {
		t.Errorf("a taken name was stored: %q", got)
	}
	if p.textOf("sandbox:field") != "firejail-taken" {
		t.Error("the field lost the taken name")
	}

	// A free name is stored once the backend has answered, and the message goes.
	p.typeInto("sandbox:field", "firejail-calc")
	p.waitFor("the name to be stored", func() bool { return sandboxName(p) == "firejail-calc" })
	if p.ts.Exists(key("problem:sandbox")) {
		t.Error("the message stays for a free name")
	}
	if !p.ts.Exists(gluttertest.ByText("firejail-calc")) {
		t.Error("the header does not show the new name")
	}
	if asks := p.nameAsks(); !slices.Equal(asks, []string{"firejail-taken", "firejail-calc"}) {
		t.Errorf("asked about %q", asks)
	}

	// The name the build has is not asked about.
	p.typeInto("sandbox:field", "firejail-calc")
	if asks := p.nameAsks(); len(asks) != 2 {
		t.Errorf("the own name was asked about: %q", asks)
	}
}

func TestLauncherTabSandboxNameCheckFails(t *testing.T) {
	p := launcherPage(t, "Calculator")
	p.backend.set(func(f *fakeBackend) { f.nameErr = errors.New("DatabaseGone", "the database is gone") })
	p.typeInto("sandbox:field", "firejail-other")
	p.waitFor("the message", func() bool { return p.ts.Exists(key("problem:sandbox")) })
	if !p.problemShows("problem:sandbox", "could not check") || !p.problemShows("problem:sandbox", "the database is gone") {
		t.Error("the message does not say the check failed")
	}
	if got := sandboxName(p); got != "firejail-calculator" {
		t.Errorf("a name that was not checked was stored: %q", got)
	}
	if !strings.Contains(p.log.String(), "DatabaseGone") {
		t.Errorf("the failure is not in the log: %s", p.log.String())
	}
}

func TestLauncherTabLaterTypingWins(t *testing.T) {
	p := launcherPage(t, "Calculator")
	gate := make(chan struct{})
	p.backend.set(func(f *fakeBackend) { f.nameGate = gate })
	p.typeInto("sandbox:field", "firejail-one")
	p.typeInto("sandbox:field", "firejail-two")
	if got := p.textOf("sandbox:field"); got != "firejail-two" {
		t.Fatalf("the field says %q while a check is running", got)
	}
	close(gate)
	p.waitFor("the second name", func() bool { return sandboxName(p) == "firejail-two" })
	p.ts.Pump()
	if sandboxName(p) != "firejail-two" || p.textOf("sandbox:field") != "firejail-two" {
		t.Errorf("the name is %q, the field %q", sandboxName(p), p.textOf("sandbox:field"))
	}
}

func TestLauncherTabRenameOfAnInstalledBuild(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds[2].Installed = true })
	p := newPage(t, backend)
	p.ts.SetViewSize(gui.Size{Width: 1100, Height: 2400})
	p.ready()
	e := p.openApp("Visual Studio Code")
	p.selectTab(firejail.TabLauncher)
	if e.Build().ID != 3 || sandboxName(p) != "firejail-code" || p.ts.Exists(key("note:rename")) {
		t.Fatalf("build %d named %q, or the note is shown already", e.Build().ID, sandboxName(p))
	}

	// Another build's name is refused even though the build itself is stored.
	p.typeInto("sandbox:field", "firejail-firefox")
	p.waitFor("the refusal", func() bool { return p.ts.Exists(key("problem:sandbox")) })

	// The build's own id is excepted, so a new name is free.
	p.typeInto("sandbox:field", "firejail-code-2")
	p.waitFor("the new name", func() bool { return sandboxName(p) == "firejail-code-2" })
	if !p.ts.Exists(key("note:rename")) || !p.problemShows("note:rename", "firejail-code") ||
		!p.problemShows("note:rename", "remove its files") {
		t.Error("the note about the rename is missing")
	}
	if e.RenamedFrom() != "firejail-code" {
		t.Errorf("RenamedFrom = %q", e.RenamedFrom())
	}

	// The old name again is no rename.
	p.typeInto("sandbox:field", "firejail-code")
	p.waitFor("the old name", func() bool { return sandboxName(p) == "firejail-code" })
	if p.ts.Exists(key("note:rename")) {
		t.Error("the note stays after the old name came back")
	}
}

func TestLauncherTabRenameMovesThePrivateHome(t *testing.T) {
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	homes := p.backend.HomesDir()
	p.change(func(e *Editor) { e.SetHome(firejail.HomePrivate) })
	if got := e.Final().PrivateDir; got != filepath.Join(homes, "firejail-calculator") {
		t.Fatalf("private dir = %q", got)
	}
	if !p.ts.Exists(key("note:private-home")) {
		t.Error("no note that the private home follows the name")
	}
	p.typeInto("sandbox:field", "firejail-sums")
	p.waitFor("the new name", func() bool { return sandboxName(p) == "firejail-sums" })
	if got := e.Final().PrivateDir; got != filepath.Join(homes, "firejail-sums") {
		t.Errorf("after the rename the private dir is %q", got)
	}
	if !p.textHas("note:private-home", "firejail-sums") {
		t.Error("the note does not show the new folder")
	}
}

func TestEditorSetSandboxNamePrivateHome(t *testing.T) {
	f := newFixtureFirejail(t)
	homes := filepath.Join(f.dirs.Home, ".local", "share", "jailcat", "homes")
	privateBase := firejail.BaseNone()
	privateBase.Settings.Home = firejail.HomePrivate
	privateBase.Settings.PrivateDir = "/data/base-home"

	tests := []struct {
		name  string
		setup func(e *Editor)
		want  string
		moves bool
	}{
		{"default folder of the old name", func(e *Editor) { e.SetHome(firejail.HomePrivate) }, filepath.Join(homes, "new-name"), true},
		{"folder of the base profile", func(e *Editor) {
			e.SwitchProfile("", privateBase, nil, false)
			e.SetHome(firejail.HomePrivate)
		}, "/data/base-home", false},
		{"folder the user chose", func(e *Editor) {
			e.SetHome(firejail.HomePrivate)
			e.Set(func(s *firejail.Settings) { s.PrivateDir = "/data/mine" })
		}, "/data/mine", false},
		{"home that is not private", func(e *Editor) {}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, p := f.base("default.profile")
			e := NewEditor(codeApp(), EditorOpts{
				Home: f.dirs.Home, HomesDir: homes, BaseProfile: f.path("default.profile"), Profile: p, Base: base, Defaults: codeDefaults(),
			})
			tt.setup(e)
			if got := e.PrivateHomeFollowsName(); got != tt.moves {
				t.Errorf("PrivateHomeFollowsName = %v, want %v", got, tt.moves)
			}
			e.SetSandboxName("new-name")
			if got := e.Final().SandboxName; got != "new-name" {
				t.Errorf("name = %q", got)
			}
			if got := e.Final().PrivateDir; got != tt.want {
				t.Errorf("PrivateDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLauncherTabTextFields(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		field  firejail.Field
		get    func(s firejail.Settings) string
		def    string
		typed  string
	}{
		{"menu name", "menu", firejail.FieldMenuName, func(s firejail.Settings) string { return s.MenuName }, "Calculator (sandboxed)", "My Calculator"},
		{"comment", "comment", firejail.FieldComment, func(s firejail.Settings) string { return s.Comment }, "Do sums", "Quick sums, with “quotes”, \"double\", $HOME and `ticks`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := launcherPage(t, "Calculator")
			e := p.home().editor
			mark := "mark:" + string(tt.field)
			p.typeInto(tt.prefix+":field", tt.typed)
			if got := tt.get(e.Final()); got != tt.typed || !e.Changed(tt.field) || !p.ts.Exists(key(mark)) || !p.tabDot(firejail.TabLauncher) {
				t.Fatalf("typing did not change the setting: %q", got)
			}
			p.reset(mark)
			if got := tt.get(e.Final()); got != tt.def || e.Changed(tt.field) || p.ts.Exists(key(mark)) || p.tabDot(firejail.TabLauncher) {
				t.Errorf("Reset left %q", got)
			}
			if got := p.textOf(tt.prefix + ":field"); got != tt.def {
				t.Errorf("after Reset the field shows %q, want %q", got, tt.def)
			}
		})
	}
}

func TestLauncherTabMenuNameCannotBeEmpty(t *testing.T) {
	p := launcherPage(t, "Calculator")
	p.typeInto("menu:field", "  ")
	if !p.problemShows("problem:menu", "needs a name") {
		t.Fatal("an empty menu name is not explained")
	}
	// Without a menu entry the name is not used.
	p.tap("toggle:" + string(firejail.FieldInstallMenu))
	if p.ts.Exists(key("problem:menu")) {
		t.Error("the message stays although no menu entry is installed")
	}
	p.typeInto("menu:field", "Calc")
	p.tap("toggle:" + string(firejail.FieldInstallMenu))
	if p.ts.Exists(key("problem:menu")) {
		t.Error("the message stays for a menu name")
	}
}

func TestLauncherTabSwitches(t *testing.T) {
	tests := []struct {
		field firejail.Field
		get   func(s firejail.Settings) bool
	}{
		{firejail.FieldInstallMenu, func(s firejail.Settings) bool { return s.InstallMenu }},
		{firejail.FieldInstallBin, func(s firejail.Settings) bool { return s.InstallBin }},
		{firejail.FieldOpenFileTypes, func(s firejail.Settings) bool { return s.OpenFileTypes }},
		{firejail.FieldSingleInstance, func(s firejail.Settings) bool { return s.SingleInstance }},
		{firejail.FieldTerminal, func(s firejail.Settings) bool { return s.Terminal }},
		{firejail.FieldQuiet, func(s firejail.Settings) bool { return s.Quiet }},
	}
	if got := len(launcherSwitches()); got != len(tests) {
		t.Fatalf("the tab has %d switches, the test %d", got, len(tests))
	}
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	for _, tt := range tests {
		t.Run(string(tt.field), func(t *testing.T) {
			before := tt.get(e.Final())
			p.tap("toggle:" + string(tt.field))
			if p.switchedOn("toggle:"+string(tt.field)) == before || tt.get(e.Final()) == before || !e.Changed(tt.field) || !p.tabDot(firejail.TabLauncher) {
				t.Fatalf("the switch did not change %s", tt.field)
			}
			mark := "mark:" + string(tt.field)
			if !p.ts.Exists(key(mark)) {
				t.Fatal("no changed mark")
			}
			p.reset(mark)
			if tt.get(e.Final()) != before || e.Changed(tt.field) || p.ts.Exists(key(mark)) || p.tabDot(firejail.TabLauncher) {
				t.Error("Reset did not put the switch back")
			}
		})
	}
	if e.HasChanges() {
		t.Errorf("the switches left changes: %+v", e.Overrides())
	}
}

func TestLauncherTabTerminalDefaultFollowsTheApp(t *testing.T) {
	// A binary starts in a terminal; turning it off is a change.
	p := launcherPage(t, "curl")
	e := p.home().editor
	if !p.switchedOn("toggle:terminal") || e.Changed(firejail.FieldTerminal) {
		t.Fatal("a binary does not start with Run in a terminal on")
	}
	p.tap("toggle:terminal")
	if p.switchedOn("toggle:terminal") || !e.Changed(firejail.FieldTerminal) || !p.ts.Exists(key("mark:terminal")) {
		t.Error("turning the terminal off is not a change")
	}
}

func TestLauncherTabNeedsOneInstall(t *testing.T) {
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	p.tap("toggle:" + string(firejail.FieldInstallMenu))
	if p.ts.Exists(key("problem:install")) || len(e.Problems()) != 0 {
		t.Fatal("one install target on is reported as a problem")
	}
	p.tap("toggle:" + string(firejail.FieldInstallBin))
	if !p.problemShows("problem:install", "one of them must be on") {
		t.Fatal("both off shows no message")
	}
	probs := e.Problems()
	if len(probs) != 1 || probs[0].Tag != firejail.ProblemInstallNothing || probs[0].Severity != firejail.SeverityError {
		t.Errorf("Problems = %+v", probs)
	}
	p.reset("mark:" + string(firejail.FieldInstallBin))
	if p.ts.Exists(key("problem:install")) {
		t.Error("the message stays after Reset")
	}
}

func TestLauncherTabBinNotOnPathNote(t *testing.T) {
	tests := []struct {
		name      string
		onPath    bool
		installed bool
		want      bool
	}{
		{"not on PATH", false, true, true},
		{"on PATH", true, true, false},
		{"not on PATH, not installed there", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := fixtureBackend(t)
			backend.set(func(f *fakeBackend) { f.builds, f.binOnPath = nil, tt.onPath })
			p := newPage(t, backend)
			p.ts.SetViewSize(gui.Size{Width: 1100, Height: 2400})
			p.ready()
			p.openApp("Calculator")
			p.selectTab(firejail.TabLauncher)
			if !tt.installed {
				p.tap("toggle:" + string(firejail.FieldInstallBin))
			}
			if got := p.ts.Exists(key("note:bin-path")); got != tt.want {
				t.Errorf("note shown = %v, want %v", got, tt.want)
			}
			if tt.want && !p.problemShows("note:bin-path", "PATH") {
				t.Error("the note does not name PATH")
			}
		})
	}
}

func TestLauncherTabIconPreview(t *testing.T) {
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	p.waitPreview()

	// The badge mode asks for the badge icon and shows it at 96 pixels.
	asks := p.previewAsks()
	if len(asks) != 1 || asks[0].Icon != firejail.IconBadge || asks[0].IconSeed != 1234 {
		t.Fatalf("PreviewIcon was asked for %+v", asks)
	}
	pic := p.ts.Widget(gluttertest.Descendant(key("icon:preview"), gluttertest.ByType[*svg.SvgPicture]())).(*svg.SvgPicture)
	if pic.Width().OrElse(0) != 96 || pic.Height().OrElse(0) != 96 {
		t.Errorf("the preview is %vx%v", pic.Width().OrElse(0), pic.Height().OrElse(0))
	}
	if p.ts.Exists(key("icon:fallback")) || p.ts.Exists(key("icon:new-random")) {
		t.Error("the badge mode shows a fallback note or the New random icon button")
	}
	if e.Final().Icon != firejail.IconBadge {
		t.Error("the badge is not the icon of a new build")
	}

	// Random mode asks again, with the mode, and shows the button.
	p.tap("icon:random")
	p.waitFor("the second preview", func() bool { return len(p.previewAsks()) == 2 })
	if e.Final().Icon != firejail.IconRandom || !e.Changed(firejail.FieldIcon) || !p.ts.Exists(key("mark:"+string(firejail.FieldIcon))) {
		t.Fatal("the radio did not change the icon")
	}
	if got := p.previewAsks()[1]; got.Icon != firejail.IconRandom || got.MenuName != "Calculator (sandboxed)" {
		t.Errorf("asked for %+v", got)
	}
	if !p.ts.Exists(key("icon:new-random")) || !p.pressable("icon:new-random") {
		t.Fatal("no New random icon button in the random mode")
	}

	// New random icon draws a new seed and a new preview.
	before := e.Final().IconSeed
	p.tap("icon:new-random")
	p.waitFor("the third preview", func() bool { return len(p.previewAsks()) == 3 })
	if after := e.Final().IconSeed; after == before {
		t.Errorf("the seed is still %d", after)
	}
	if e.Changed(firejail.FieldIconSeed) || p.previewAsks()[2].IconSeed != e.Final().IconSeed {
		t.Error("the new seed is not the one the preview was made for")
	}

	// Typing the menu name changes the letter of a random icon, so the preview is made again.
	p.typeInto("menu:field", "Zed")
	p.waitFor("the fourth preview", func() bool { return len(p.previewAsks()) == 4 })
	if p.previewAsks()[3].MenuName != "Zed" {
		t.Errorf("asked for %+v", p.previewAsks()[3])
	}

	// Reset goes back to the badge.
	p.reset("mark:" + string(firejail.FieldIcon))
	p.waitFor("the badge again", func() bool { return len(p.previewAsks()) == 5 })
	if e.Final().Icon != firejail.IconBadge || p.ts.Exists(key("icon:new-random")) {
		t.Error("Reset did not put the badge back")
	}
}

func TestLauncherTabIconFallback(t *testing.T) {
	// "No Icon" names an icon the fake system does not have.
	p := launcherPage(t, "No Icon")
	p.waitFor("the fallback note", func() bool { return p.ts.Exists(key("icon:fallback")) })
	if !p.textHas("icon:fallback", "random icon instead") || !p.textHas("icon:fallback", "was not found") {
		t.Error("the note does not say why the icon is random")
	}
	// The mode is still the badge: only the picture is random.
	if p.home().editor.Final().Icon != firejail.IconBadge || p.ts.Exists(key("icon:new-random")) {
		t.Error("the fallback changed the mode")
	}
	// The fallback icon depends on the menu name, so typing it asks again.
	n := len(p.previewAsks())
	p.typeInto("menu:field", "Other")
	p.waitFor("another preview", func() bool { return len(p.previewAsks()) > n })

	// In the random mode the note goes.
	p.tap("icon:random")
	p.waitFor("the note to go", func() bool { return !p.ts.Exists(key("icon:fallback")) })
}

func TestLauncherTabIconPreviewFails(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) {
		f.builds = nil
		f.previewErr = errors.New("IconUnreadable", "the icon file cannot be read.")
	})
	p := newPage(t, backend)
	p.ts.SetViewSize(gui.Size{Width: 1100, Height: 2400})
	p.ready()
	p.openApp("Calculator")
	p.selectTab(firejail.TabLauncher)
	p.waitFor("the failure", func() bool { return p.ts.Exists(key("icon:failed")) })
	if !p.textHas("icon:failed", "the icon file cannot be read") {
		t.Error("the failure is not explained")
	}
	if !strings.Contains(p.log.String(), "IconUnreadable") {
		t.Errorf("the failure is not in the log: %s", p.log.String())
	}
	if p.ts.Exists(gluttertest.Descendant(key("icon:preview"), gluttertest.ByType[*svg.SvgPicture]())) {
		t.Error("a picture is drawn for an icon that failed")
	}
}

func TestLauncherTabIsRebuiltFromADraft(t *testing.T) {
	p := launcherPage(t, "Calculator")
	e := p.home().editor
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) { s.MenuName, s.Comment = "From a draft", "Draft comment" })
	})
	if p.textOf("menu:field") != "From a draft" || p.textOf("comment:field") != "Draft comment" {
		t.Errorf("the fields show %q and %q", p.textOf("menu:field"), p.textOf("comment:field"))
	}
	if !e.Changed(firejail.FieldMenuName) {
		t.Error("the change is not seen")
	}
}

func TestGoldenLauncherTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.clearSearch()
	p.selectTab(firejail.TabLauncher)
	// One change: the menu name. The bin folder is not on the PATH of the fake system.
	p.typeInto("menu:field", "Calculator in a jail")
	p.waitPreview()
	p.golden("launcher-tab", "4.6-launcher.png")
}

// storeBuild is a stored build of another app, to take a sandbox name.
func storeBuild(name string, id int64) store.Build {
	return store.Build{ID: id, SandboxName: name, AppKind: store.AppKindDesktop, DesktopFileID: "other.desktop"}
}

func TestEditorRenamedFrom(t *testing.T) {
	f := newFixtureFirejail(t)
	base, p := f.base("default.profile")
	open := func(b store.Build) *Editor {
		return NewEditor(codeApp(), EditorOpts{
			Home: f.dirs.Home, BaseProfile: f.path("default.profile"), Profile: p, Base: base, Defaults: codeDefaults(), Build: b,
		})
	}
	stored := store.Build{ID: 3, SandboxName: "firejail-code", AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge}
	installed := stored
	installed.Installed = true

	tests := []struct {
		name  string
		build store.Build
		want  string
	}{
		{"a new build", store.Build{}, ""},
		{"a stored build that is not installed", stored, ""},
		{"an installed build", installed, "firejail-code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := open(tt.build)
			if got := e.RenamedFrom(); got != "" {
				t.Errorf("before a rename: %q", got)
			}
			e.SetSandboxName("firejail-other")
			if got := e.RenamedFrom(); got != tt.want {
				t.Errorf("after a rename: %q, want %q", got, tt.want)
			}
			e.SetSandboxName(tt.build.SandboxName)
			if tt.build.ID != 0 && e.RenamedFrom() != "" {
				t.Error("the old name again is a rename")
			}
		})
	}
}
