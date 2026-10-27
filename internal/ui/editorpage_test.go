package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// openApp taps the row of the app in the list and waits for its editor.
func (p *page) openApp(name string) *Editor {
	p.t.Helper()
	p.selectRow(name)
	return p.editorReady()
}

// selectRow taps the row of the app in the list; when the row is not on screen (the list
// is lazy) it searches for the app first.
func (p *page) selectRow(name string) {
	p.t.Helper()
	row := gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.ByText(name))
	if !p.ts.Exists(row) {
		p.ts.EnterText(gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.ByType[*m.TextField]()), name)
		p.ts.Pump()
	}
	p.ts.Tap(row)
	p.ts.Pump()
}

// editorState is the state of the editor screen.
func (p *page) editorState() *editorPageState {
	return gluttertest.StateOf[*editorPageState](p.ts, gluttertest.ByType[*EditorPage]())
}

// change changes the editor the way the tabs will: through the state of the page.
func (p *page) change(apply func(e *Editor)) {
	p.t.Helper()
	e := p.home().editor
	p.editorState().SetState(func() { apply(e) })
	p.ts.Pump()
}

// tabDot reports whether the tab shows a change dot.
func (p *page) tabDot(tab firejail.Tab) bool {
	return p.ts.Exists(gluttertest.Descendant(gluttertest.ByKey(m.NewValueKey("tab:"+string(tab))), gluttertest.ByType[*ChangeDot]()))
}

// dots counts the change dots of the tab row.
func (p *page) dots() int {
	return p.ts.Count(gluttertest.Descendant(gluttertest.ByType[*m.TabBar](), gluttertest.ByType[*ChangeDot]()))
}

// selectTab taps a tab by name.
func (p *page) selectTab(tab firejail.Tab) {
	p.t.Helper()
	p.ts.Tap(gluttertest.ByKey(m.NewValueKey("tab:" + string(tab))))
	p.ts.Pump()
}

// profileButtonText is the text on the profile selector's button.
func (p *page) profileButtonText() string {
	p.t.Helper()
	w := p.ts.Widget(gluttertest.Descendant(gluttertest.ByKey(m.NewValueKey("profile-selector")), gluttertest.ByType[*m.Text]()))
	return w.(*m.Text).Data().OrElse("")
}

// openProfileDialog taps the selector's button.
func (p *page) openProfileDialog() {
	p.t.Helper()
	p.ts.Tap(gluttertest.ByKey(m.NewValueKey("profile-selector")))
	p.ts.Pump()
	if !p.ts.Exists(gluttertest.ByType[*ProfileDialog]()) {
		p.t.Fatal("the profile dialog did not open")
	}
}

// inDialog finds f in the profile dialog.
func inDialog(f gluttertest.Finder) gluttertest.Finder {
	return gluttertest.Descendant(gluttertest.ByType[*ProfileDialog](), f)
}

// pickProfile taps the entry of the dialog.
func (p *page) pickProfile(text string) {
	p.t.Helper()
	p.ts.Tap(inDialog(gluttertest.ByText(text)))
	p.ts.Pump()
}

func TestEditorOpensForANewApp(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	e := p.openApp("Calculator")

	// A new build, on the Default profile (the app has none of its own).
	if e.Build().ID != 0 || e.BaseProfile() != filepath.Join(p.backend.fj.dirs.FirejailSystem, "default.profile") {
		t.Errorf("build %d on %q", e.Build().ID, e.BaseProfile())
	}
	if got := p.profileButtonText(); got != "Default" {
		t.Errorf("the profile button says %q, want Default", got)
	}
	if n := p.ts.Count(gluttertest.ByType[*m.Tab]()); n != 9 {
		t.Errorf("%d tabs, want 9", n)
	}
	for _, name := range []string{"Files", "Network", "Devices", "Desktop", "Security", "Programs", "Limits", "Launcher", "Extra rules"} {
		if !p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*m.TabBar](), gluttertest.ByText(name))) {
			t.Errorf("the tab %q is missing", name)
		}
	}
	if p.dots() != 0 || p.ts.Exists(gluttertest.ByText(changedText)) {
		t.Errorf("a fresh build shows %d dots", p.dots())
	}
	// The header: the app, the arrow and the sandbox name.
	if !p.ts.Exists(gluttertest.ByText("firejail-calculator")) || !p.ts.Exists(gluttertest.ByType[*AppIcon]()) {
		t.Error("the header is incomplete")
	}
	// The button bar: Preview, Try and Build, disabled; no Uninstall for a build that is not installed.
	for _, kind := range []gluttertest.Finder{gluttertest.ByType[*m.FilledButton](), gluttertest.ByType[*m.OutlinedButton]()} {
		for _, w := range p.ts.Widgets(gluttertest.Descendant(gluttertest.ByType[*EditorPage](), kind)) {
			switch b := w.(type) {
			case *m.FilledButton:
				if b.OnPressed() != nil {
					t.Error("the Build button is enabled")
				}
			case *m.OutlinedButton:
				// The profile selector is the one button that works.
				if b.OnPressed() != nil && b.Key() != m.NewValueKey("profile-selector") {
					t.Error("a button of the bar is enabled")
				}
			}
		}
	}
	if p.ts.Exists(gluttertest.ByText(uninstallText)) {
		t.Error("Uninstall is shown for a build that is not installed")
	}
	for _, name := range []string{previewText, tryText, buildText} {
		if !p.ts.Exists(gluttertest.ByText(name)) {
			t.Errorf("the button %q is missing", name)
		}
	}
	if p.ts.Exists(gluttertest.ByTextContaining("step 4")) {
		t.Error("the buttons mention a step")
	}
	// The Files tab lists its settings.
	if !p.ts.Exists(gluttertest.ByText("Home folder")) || !p.ts.Exists(gluttertest.ByText("Folders")) {
		t.Error("the Files tab does not list its settings")
	}
	if p.ts.Exists(gluttertest.ByText(draftNoteText)) {
		t.Error("a new build shows the draft note")
	}
}

func TestEditorStartsOnTheProfileOfTheApp(t *testing.T) {
	backend := fixtureBackend(t)
	backend.builds = nil
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Visual Studio Code")

	if e.BaseProfile() != filepath.Join(backend.fj.dirs.FirejailSystem, "code.profile") {
		t.Errorf("base profile = %q", e.BaseProfile())
	}
	if got := p.profileButtonText(); got != "code (firejail)" {
		t.Errorf("the profile button says %q, want \"code (firejail)\"", got)
	}
	// code.profile says nosound and novideo.
	if e.Final().Sound {
		t.Error("sound is allowed: the base values are not those of code.profile")
	}
	if !p.ts.Exists(gluttertest.ByText("firejail-code")) {
		t.Error("the sandbox name is missing")
	}
}

func TestEditorOpensTheStoredBuild(t *testing.T) {
	backend := fixtureBackend(t)
	code := filepath.Join(backend.fj.dirs.FirejailSystem, "code.profile")
	backend.set(func(f *fakeBackend) {
		f.builds[2].BaseProfile = code
		f.builds[2].Installed = true
	})
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Visual Studio Code")

	if e.Build().ID != 3 || e.BaseProfile() != code || e.Final().SandboxName != "firejail-code" {
		t.Errorf("build %d on %q as %q", e.Build().ID, e.BaseProfile(), e.Final().SandboxName)
	}
	// Uninstall is for an installed build.
	if !p.ts.Exists(gluttertest.ByText(uninstallText)) {
		t.Error("Uninstall is missing for an installed build")
	}
	if p.ts.Exists(gluttertest.ByText(draftNoteText)) {
		t.Error("the draft note is shown for a build with no draft")
	}
}

func TestChangedDotAndMark(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp("Calculator")

	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	if !p.tabDot(firejail.TabNetwork) || p.dots() != 1 {
		t.Errorf("dots = %d, Network dot = %v", p.dots(), p.tabDot(firejail.TabNetwork))
	}
	// The Files tab is on screen: no mark there. The Network tab shows it.
	if p.ts.Exists(gluttertest.ByText(changedText)) {
		t.Error("a mark is shown on the Files tab")
	}
	p.selectTab(firejail.TabNetwork)
	if n := p.ts.Count(gluttertest.ByText(changedText)); n != 1 {
		t.Fatalf("%d marks on the Network tab, want 1", n)
	}
	if !p.ts.Exists(key("network:none")) || !p.ts.Exists(key("mark:network")) {
		t.Error("the Network tab does not show the choice and its mark")
	}

	p.ts.Tap(gluttertest.ByText(resetText))
	p.ts.Pump()
	if p.ts.Exists(gluttertest.ByText(changedText)) || p.dots() != 0 {
		t.Errorf("after Reset: marks %v, dots %d", p.ts.Exists(gluttertest.ByText(changedText)), p.dots())
	}
	if e := p.home().editor; e.Final().Network != firejail.NetFull || e.Changed(firejail.FieldNetwork) {
		t.Errorf("network = %q", e.Final().Network)
	}
}

func TestChangedFolderRowAndExtraRulesDot(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.Folders = append(s.Folders, firejail.Folder{Path: "/mnt/data", Access: firejail.AccessReadOnly})
		})
		e.SetExtraRules([]string{"net none"})
	})
	if !p.tabDot(firejail.TabFiles) || !p.tabDot(TabExtraRules) || p.dots() != 2 {
		t.Errorf("dots = %d", p.dots())
	}
	if !p.ts.Exists(gluttertest.ByText("/mnt/data")) || p.ts.Count(gluttertest.ByText(changedText)) != 1 {
		t.Error("the added folder row and its mark are missing")
	}
	p.ts.Tap(gluttertest.ByText(resetText))
	p.ts.Pump()
	if p.ts.Exists(gluttertest.ByText("/mnt/data")) || p.tabDot(firejail.TabFiles) {
		t.Error("the folder row is still there after Reset")
	}
	p.selectTab(TabExtraRules)
	if !p.ts.Exists(gluttertest.ByText("net none")) {
		t.Error("the extra rule is not listed")
	}
}

func TestProblemsAreListed(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.SandboxName = "Bad Name" }) })
	probs := p.home().editor.Problems()
	if len(probs) != 1 {
		t.Fatalf("problems = %+v", probs)
	}
	if !p.ts.Exists(gluttertest.ByText(probs[0].Message)) || !p.ts.Exists(gluttertest.ByIcon(m.Icons.Error())) {
		t.Errorf("the problem %q is not shown", probs[0].Message)
	}
}

func TestProfileDialogLists(t *testing.T) {
	backend := fixtureBackend(t)
	backend.builds = nil
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.openProfileDialog()

	state := gluttertest.StateOf[*profileDialogState](p.ts, gluttertest.ByType[*ProfileDialog]())
	var headers []string
	for _, r := range state.rows {
		if r.header != "" {
			headers = append(headers, string(r.header))
		}
	}
	want := []string{"For this app", "Firejail profiles"}
	if !equalStrings(headers, want) {
		t.Errorf("groups = %q, want %q", headers, want)
	}
	for _, text := range []string{"code (firejail)", "None", "Default"} {
		if !p.ts.Exists(inDialog(gluttertest.ByText(text))) {
			t.Errorf("%q is not shown", text)
		}
	}
	// The list is lazy: the rows further down are in the state, not yet on screen.
	var names []string
	for _, r := range state.rows {
		if r.header == "" && r.choice.Group == GroupFirejail {
			names = append(names, r.choice.Name)
		}
	}
	if !equalStrings(names, []string{"blink-common", "code", "default", "electron-common"}) {
		t.Errorf("Firejail profiles = %q", names)
	}
}

func TestProfileDialogSearchesAndSelects(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp("Calculator")
	p.openProfileDialog()

	p.ts.EnterText(inDialog(gluttertest.ByType[*m.TextField]()), "ELECTRON")
	p.ts.Pump()
	state := gluttertest.StateOf[*profileDialogState](p.ts, gluttertest.ByType[*ProfileDialog]())
	if len(state.rows) != 2 || state.rows[0].header != GroupFirejail || state.rows[1].choice.Name != "electron-common" {
		t.Fatalf("rows = %+v", state.rows)
	}
	p.ts.EnterText(inDialog(gluttertest.ByType[*m.TextField]()), "zzzz")
	p.ts.Pump()
	if !p.ts.Exists(inDialog(gluttertest.ByText(noProfilesText))) {
		t.Error("the empty-result text is missing")
	}
	p.ts.EnterText(inDialog(gluttertest.ByType[*m.TextField]()), "electron")
	p.ts.Pump()
	p.pickProfile("electron-common")
	p.waitFor("the profile", func() bool {
		return p.home().editor.BaseProfile() != "" && !p.editorState().reading &&
			strings.HasSuffix(p.home().editor.BaseProfile(), "electron-common.profile")
	})
	if p.ts.Exists(gluttertest.ByType[*ProfileDialog]()) {
		t.Error("the dialog is still open")
	}
	if got := p.profileButtonText(); got != "electron-common" {
		t.Errorf("the profile button says %q", got)
	}
	// No change was made, so nothing was asked.
	if p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) {
		t.Error("Keep/Start over was asked without changes")
	}
}

func TestProfileDialogIsLazy(t *testing.T) {
	backend := fixtureBackend(t)
	var entries []ProfileChoice
	entries = append(entries, ProfileChoice{Group: GroupNone, Name: "None", Label: "None"})
	for i := range 1400 {
		name := "profile-" + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('0'+i/676))
		entries = append(entries, ProfileChoice{Group: GroupFirejail, Name: name, Path: "/etc/firejail/" + name + ".profile", Label: name})
	}
	backend.choices = &ProfileChoices{Entries: entries, Default: entries[0]}
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.openProfileDialog()
	if n := p.ts.Count(inDialog(gluttertest.ByType[*m.ListTile]())); n == 0 || n > 30 {
		t.Errorf("%d rows are built for 1,400 profiles, want only the visible ones", n)
	}
	p.ts.EnterText(inDialog(gluttertest.ByType[*m.TextField]()), "profile-ab")
	p.ts.Pump()
	state := gluttertest.StateOf[*profileDialogState](p.ts, gluttertest.ByType[*ProfileDialog]())
	if len(state.rows) < 2 || len(state.rows) > 100 {
		t.Errorf("%d rows after the search", len(state.rows))
	}
}

func TestSwitchingProfileWithoutChangesDoesNotAsk(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp("Calculator")
	p.openProfileDialog()
	p.pickProfile("None")
	p.waitFor("None", func() bool { return p.home().editor.BaseProfile() == "" && !p.editorState().reading })
	if got := p.profileButtonText(); got != "None" {
		t.Errorf("the profile button says %q", got)
	}
	if p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) {
		t.Error("Keep/Start over was asked without changes")
	}
	// None has Jailcat's security defaults.
	if !p.home().editor.Final().Seccomp {
		t.Error("seccomp is off on None")
	}
}

func TestSwitchingProfileWithChangesAsks(t *testing.T) {
	tests := []struct {
		name       string
		button     string
		wantChange bool
	}{
		{"keep", keepButtonText, true},
		{"start over", startOverButtonText, false},
		{"cancel", cancelButtonText, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPage(t, fixtureBackend(t))
			p.ready()
			e := p.openApp("Calculator")
			before := e.BaseProfile()
			p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
			p.openProfileDialog()
			p.pickProfile("None")
			p.waitFor("the question", func() bool { return p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) })
			if !p.ts.Exists(gluttertest.ByText(keepButtonText)) || !p.ts.Exists(gluttertest.ByText(startOverButtonText)) {
				t.Fatal("the choices are missing")
			}
			if !p.ts.Exists(gluttertest.ByTextContaining("1 setting")) {
				t.Error("the question does not say how many settings were changed")
			}
			// Nothing happens until the user answers.
			if e.BaseProfile() != before {
				t.Error("the profile changed before the answer")
			}
			p.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*KeepChangesDialog](), gluttertest.ByText(tt.button)))
			p.ts.Pump()
			p.waitFor("the answer", func() bool { return !p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) })

			if tt.button == cancelButtonText {
				if e.BaseProfile() != before || e.Final().Network != firejail.NetNone {
					t.Errorf("cancel changed something: %q, network %q", e.BaseProfile(), e.Final().Network)
				}
				return
			}
			if e.BaseProfile() != "" {
				t.Errorf("base profile = %q, want None", e.BaseProfile())
			}
			if got := e.Final().Network == firejail.NetNone; got != tt.wantChange || e.Changed(firejail.FieldNetwork) != tt.wantChange {
				t.Errorf("network none = %v, want %v", got, tt.wantChange)
			}
			if p.tabDot(firejail.TabNetwork) != tt.wantChange {
				t.Errorf("the Network dot = %v", p.tabDot(firejail.TabNetwork))
			}
		})
	}
}

func TestProfileThatCannotBeReadKeepsThePrevious(t *testing.T) {
	backend := fixtureBackend(t)
	blink := filepath.Join(backend.fj.dirs.FirejailSystem, "blink-common.profile")
	backend.readErr = map[string]error{blink: errors.New("ProfileReadFailed", "the profile includes a file that does not exist", "file", "blink-common.inc")}
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Calculator")
	before := e.BaseProfile()
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })

	p.openProfileDialog()
	p.pickProfile("blink-common")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining("could not read the profile")) })

	if e.BaseProfile() != before || e.Final().Network != firejail.NetNone {
		t.Error("the profile or the changes were lost")
	}
	if p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) {
		t.Error("the question was asked for a profile that could not be read")
	}
	// In plain words: the profile's name and what is wrong, no tag or route.
	if !p.ts.Exists(gluttertest.ByTextContaining("blink-common: the profile includes a file that does not exist")) {
		t.Error("the message does not name the profile and the reason")
	}
	if p.ts.Exists(gluttertest.ByTextContaining("ProfileReadFailed")) {
		t.Error("the tag is shown to the user")
	}
	if !strings.Contains(p.log.String(), "reading the base profile failed") || !strings.Contains(p.log.String(), "ProfileReadFailed") {
		t.Errorf("the log lacks the details:\n%s", p.log.String())
	}
	// The next choice clears the message.
	p.openProfileDialog()
	p.pickProfile("None")
	p.waitFor("the question", func() bool { return p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) })
	if p.ts.Exists(gluttertest.ByTextContaining("could not read the profile")) {
		t.Error("the old message is still shown")
	}
}

func TestReadingShowsProgress(t *testing.T) {
	backend := fixtureBackend(t)
	backend.readGate = make(chan struct{})
	p := newPage(t, backend)
	p.ready()
	// Opening reads the default profile: let it through, then hold the next read.
	close(backend.readGate)
	p.openApp("Calculator")
	backend.set(func(f *fakeBackend) { f.readGate = make(chan struct{}) })
	gate := backend.readGate

	p.openProfileDialog()
	p.pickProfile("None")
	p.ts.Pump()
	if !p.ts.Exists(gluttertest.ByText(readingProfileText)) || !p.ts.Exists(gluttertest.ByType[*m.CircularProgressIndicator]()) {
		t.Error("no progress while the profile is read")
	}
	close(gate)
	p.waitFor("the read", func() bool { return !p.editorState().reading })
	if p.ts.Exists(gluttertest.ByText(readingProfileText)) {
		t.Error("the progress stays")
	}
}

func TestBuildChooser(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.selectRow("Firefox")
	p.waitFor("the chooser", func() bool { return p.ts.Exists(gluttertest.ByType[*BuildChooserDialog]()) })
	if p.home().editor != nil {
		t.Error("an editor is open before the choice")
	}
	for _, text := range []string{"firejail-firefox", "firejail-firefox-2", newBuildButtonText} {
		if !p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*BuildChooserDialog](), gluttertest.ByText(text))) {
			t.Errorf("%q is missing in the chooser", text)
		}
	}
	if n := p.ts.Count(gluttertest.ByTextContaining("built 20 Oct 2026")); n != 2 {
		t.Errorf("the built date is shown %d times, want 2", n)
	}

	// A build.
	p.ts.Tap(gluttertest.ByText("firejail-firefox-2"))
	p.ts.Pump()
	e := p.editorReady()
	if e.Build().ID != 2 || e.Final().SandboxName != "firejail-firefox-2" {
		t.Errorf("opened build %d as %q", e.Build().ID, e.Final().SandboxName)
	}
}

func TestBuildChooserNewBuild(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.selectRow("Firefox")
	p.waitFor("the chooser", func() bool { return p.ts.Exists(gluttertest.ByType[*BuildChooserDialog]()) })
	p.ts.Tap(gluttertest.ByText(newBuildButtonText))
	p.ts.Pump()
	e := p.editorReady()
	// The name continues the numbering of the builds the app has.
	if e.Build().ID != 0 || e.Final().SandboxName != "firejail-firefox-3" {
		t.Errorf("new build %d named %q", e.Build().ID, e.Final().SandboxName)
	}
}

func TestBuildChooserCancelled(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.selectRow("Firefox")
	p.waitFor("the chooser", func() bool { return p.ts.Exists(gluttertest.ByType[*BuildChooserDialog]()) })
	p.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*BuildChooserDialog](), gluttertest.ByText(cancelButtonText)))
	p.ts.Pump()
	p.waitFor("the empty state", func() bool { return p.ts.Exists(gluttertest.ByText(emptyStateText)) })
	if p.home().selected != nil {
		t.Error("the app is still selected")
	}
}

func TestDraftIsSavedWhenAnotherAppIsSelected(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })

	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the draft", func() bool { return len(backend.savesList()) == 2 })

	// The new build was created, not installed, and holds the changes as its draft.
	if got := backend.savesList(); !equalStrings(got, []string{"create firejail-calculator", "draft firejail-calculator"}) {
		t.Fatalf("saves = %q", got)
	}
	b, ok := backend.buildNamed("firejail-calculator")
	if !ok || b.BuiltAt != nil || b.Installed || b.Draft == nil {
		t.Fatalf("build = %+v", b)
	}
	if !b.Draft.Overrides.Changed(firejail.FieldNetwork) || b.Overrides.Changed(firejail.FieldNetwork) {
		t.Errorf("the change is in the build, not only in the draft: %v / %v", b.Overrides.ChangedFields(), b.Draft.Overrides.ChangedFields())
	}
	// The row shows "draft".
	p.waitFor("the chip", func() bool { return p.ts.Exists(gluttertest.ByText(draftLabel)) })
	if n := p.ts.Count(gluttertest.ByText(draftLabel)); n != 1 {
		t.Errorf("\"draft\" is shown %d times, want 1", n)
	}
	if got := p.home().builds["desktop:Visual Studio Code.desktop"]; got != 1 {
		t.Errorf("Visual Studio Code has %d built builds, want 1", got)
	}
}

func TestDraftIsRestoredAndCanBeDiscarded(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
		e.SetExtraRules([]string{"nosound"})
	})
	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the draft", func() bool { return len(backend.savesList()) == 2 })

	// Back to Calculator: its one build opens on the draft.
	p.selectRow("Calculator")
	e := p.editorReady()
	p.waitFor("the draft note", func() bool { return p.ts.Exists(gluttertest.ByText(draftNoteText)) })
	if e.Build().ID == 0 || e.Final().Network != firejail.NetNone || len(e.ExtraRules()) != 1 || !e.Dirty() {
		t.Errorf("build %d, network %q, extra %q, dirty %v", e.Build().ID, e.Final().Network, e.ExtraRules(), e.Dirty())
	}
	if !p.tabDot(firejail.TabNetwork) || !p.tabDot(TabExtraRules) {
		t.Error("the dots of the draft are missing")
	}

	p.ts.Tap(gluttertest.ByText(discardDraftText))
	p.ts.Pump()
	e = p.editorReady()
	if p.ts.Exists(gluttertest.ByText(draftNoteText)) || e.Final().Network != firejail.NetFull || len(e.ExtraRules()) != 0 || e.Dirty() {
		t.Errorf("after Discard: network %q, extra %q, dirty %v", e.Final().Network, e.ExtraRules(), e.Dirty())
	}
	if p.dots() != 0 {
		t.Errorf("%d dots after Discard", p.dots())
	}
	if got := backend.savesList(); got[len(got)-1] != "clear firejail-calculator" {
		t.Errorf("saves = %q", got)
	}
	if b, _ := backend.buildNamed("firejail-calculator"); b.Draft != nil {
		t.Error("the draft is still stored")
	}
}

func TestDraftOfAStoredBuild(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.GPU = false }) })
	p.selectRow("Calculator")
	p.editorReady()
	p.waitFor("the draft", func() bool { return len(backend.savesList()) == 1 })
	// The build exists: only a draft is written, and it stays "built".
	if got := backend.savesList(); !equalStrings(got, []string{"draft firejail-code"}) {
		t.Errorf("saves = %q", got)
	}
	b, _ := backend.buildNamed("firejail-code")
	if b.Draft == nil || !b.Draft.Overrides.Changed(firejail.FieldGPU) || b.Overrides.Changed(firejail.FieldGPU) || b.BuiltAt == nil {
		t.Errorf("build = %+v", b)
	}
}

func TestNoDraftWithoutChanges(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.selectRow("Files")
	p.editorReady()
	if got := backend.savesList(); len(got) != 0 {
		t.Errorf("saves = %q, want none", got)
	}
	if p.ts.Exists(gluttertest.ByText(draftLabel)) {
		t.Error("a draft chip appeared")
	}

	// A change that is undone is no change.
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Firewall = !s.Firewall }) })
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Firewall = !s.Firewall }) })
	p.selectRow("Calculator")
	p.editorReady()
	if got := backend.savesList(); len(got) != 0 {
		t.Errorf("saves = %q, want none", got)
	}
}

func TestUndoneDraftIsCleared(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the draft", func() bool { return len(backend.savesList()) == 2 })
	p.selectRow("Calculator")
	p.editorReady()

	// The user undoes the change by hand and leaves.
	p.change(func(e *Editor) { e.Reset(firejail.FieldNetwork) })
	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the clear", func() bool { return len(backend.savesList()) == 3 })
	if b, _ := backend.buildNamed("firejail-calculator"); b.Draft != nil {
		t.Error("the draft is still stored")
	}
}

func TestDraftThatCannotBeSavedIsReported(t *testing.T) {
	backend := fixtureBackend(t)
	backend.saveErr = errors.New("DatabaseQueryFailed", "the database is locked")
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByText(draftFailedText)) })
	if !strings.Contains(p.log.String(), "keeping the draft failed") || !strings.Contains(p.log.String(), "DatabaseQueryFailed") {
		t.Errorf("the log lacks the details:\n%s", p.log.String())
	}
	if p.ts.Exists(gluttertest.ByTextContaining("locked")) {
		t.Error("the details are shown to the user")
	}
}

func TestDraftIsSavedWhenTheWindowCloses(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()

	// With nothing to keep, closing goes through.
	p.openApp("Calculator")
	if !p.ts.RequestWindowClose(gui.ImplicitViewID) {
		t.Fatal("the window did not close without changes")
	}

	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	if p.ts.RequestWindowClose(gui.ImplicitViewID) {
		t.Fatal("the window closed before the draft was written")
	}
	p.waitFor("the exit", func() bool { return p.ts.ExitRequests() == 1 })
	if got := backend.savesList(); !equalStrings(got, []string{"create firejail-calculator", "draft firejail-calculator"}) {
		t.Errorf("saves = %q", got)
	}
}

func TestWindowStaysOpenWhenTheDraftCannotBeSaved(t *testing.T) {
	backend := fixtureBackend(t)
	backend.saveErr = errors.New("DatabaseQueryFailed", "the database is locked")
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	if p.ts.RequestWindowClose(gui.ImplicitViewID) {
		t.Fatal("the window closed before the draft was written")
	}
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining(draftFailedText)) })
	if p.ts.ExitRequests() != 0 {
		t.Error("the app exited although the draft was lost")
	}
	// Closing again leaves without it.
	if !p.ts.RequestWindowClose(gui.ImplicitViewID) {
		t.Error("the second request did not close the window")
	}
}

func TestImportProfileThroughTheFileChooser(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	fd := p.ts.FakeDesktop()
	p.ready()
	p.openApp("Calculator")

	src := filepath.Join(t.TempDir(), "my-tool.profile")
	if err := os.WriteFile(src, []byte("# my profile\nnosound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.openProfileDialog()
	p.ts.Tap(gluttertest.ByText(importButtonText))
	p.ts.Pump()
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser(src)
	p.waitFor("the import", func() bool {
		return !p.ts.Exists(gluttertest.ByType[*ProfileDialog]()) && !p.editorState().reading &&
			strings.HasSuffix(p.home().editor.BaseProfile(), "my-tool.profile")
	})

	imported := filepath.Join(backend.fj.dirs.Imported, "my-tool.profile")
	if got := p.home().editor.BaseProfile(); got != imported {
		t.Errorf("base profile = %q, want %q", got, imported)
	}
	if data, err := os.ReadFile(imported); err != nil || !strings.Contains(string(data), "nosound") {
		t.Errorf("the copy is wrong: %q, %v", data, err)
	}
	if got := p.profileButtonText(); got != "my-tool" {
		t.Errorf("the profile button says %q", got)
	}
	if p.home().editor.Final().Sound {
		t.Error("the imported profile's nosound is not in the base values")
	}
	// The selector lists it under Imported.
	p.openProfileDialog()
	state := gluttertest.StateOf[*profileDialogState](p.ts, gluttertest.ByType[*ProfileDialog]())
	var found bool
	for _, r := range state.rows {
		found = found || r.header == GroupImported
	}
	if !found {
		t.Error("the Imported group is missing")
	}
}

func TestImportCancelledAndFailed(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	fd := p.ts.FakeDesktop()
	p.ready()
	e := p.openApp("Calculator")
	before := e.BaseProfile()
	p.openProfileDialog()

	// Cancelled.
	p.ts.Tap(gluttertest.ByText(importButtonText))
	p.ts.Pump()
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.CancelFileChooser()
	p.waitFor("the end of the import", func() bool {
		return !gluttertest.StateOf[*profileDialogState](p.ts, gluttertest.ByType[*ProfileDialog]()).busy
	})
	if !p.ts.Exists(gluttertest.ByType[*ProfileDialog]()) || len(backend.imported) != 0 {
		t.Error("a cancelled chooser closed the dialog or imported something")
	}

	// A file that cannot be imported.
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.ts.Tap(gluttertest.ByText(importButtonText))
	p.ts.Pump()
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser(src)
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining("could not import")) })
	if e.BaseProfile() != before || !p.ts.Exists(gluttertest.ByType[*ProfileDialog]()) {
		t.Error("the failed import changed the profile or closed the dialog")
	}
	if p.ts.Exists(gluttertest.ByTextContaining("ProfileImportNotProfile")) {
		t.Error("the tag is shown to the user")
	}
	if !strings.Contains(p.log.String(), "ProfileImportNotProfile") {
		t.Errorf("the log lacks the details:\n%s", p.log.String())
	}
}

func TestEditorOpensOnNoneWhenTheStoredProfileIsGone(t *testing.T) {
	backend := fixtureBackend(t)
	gone := filepath.Join(backend.fj.dirs.FirejailSystem, "missing.profile")
	backend.set(func(f *fakeBackend) { f.builds[2].BaseProfile = gone })
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Visual Studio Code")
	if e.BaseProfile() != "" || !p.ts.Exists(gluttertest.ByTextContaining("could not read the profile missing")) {
		t.Errorf("base profile %q, notice %q", e.BaseProfile(), e.Notice())
	}
	if !p.ts.Exists(gluttertest.ByTextContaining("Choose another profile")) {
		t.Error("the notice does not say what to do")
	}
}

func TestSelectedAppThatDisappearsLeavesTheEditor(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	backend.set(func(f *fakeBackend) { f.result.Apps = f.result.Apps[:1] })
	p.ts.Tap(gluttertest.ByTooltip("Refresh"))
	p.ts.Pump()
	p.ready()
	if p.home().editor != nil || !p.ts.Exists(gluttertest.ByText(emptyStateText)) {
		t.Error("the editor of an app that is gone is still shown")
	}
}

func TestOpenFailure(t *testing.T) {
	backend := fixtureBackend(t)
	backend.buildsErr = errors.New("DatabaseQueryFailed", "the database is locked")
	p := newPage(t, backend)
	p.ready()
	p.selectRow("Calculator")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByText(openFailedText)) })
}

// clearSearch empties the search field of the app list, so the goldens show the list.
func (p *page) clearSearch() {
	p.t.Helper()
	p.ts.EnterText(gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.ByType[*m.TextField]()), "")
	p.ts.Pump()
}

func TestGoldenEditor(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	// The user turned the network off and added a folder: the dots show on both tabs, and
	// the Network tab shows the mark with Reset to profile.
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.Network = firejail.NetNone
			s.Folders = append(s.Folders, firejail.Folder{Path: "/mnt/projects", Access: firejail.AccessReadWrite})
		})
	})
	p.selectTab(firejail.TabNetwork)
	p.golden("editor", "4.2-editor.png")
}

func TestGoldenEditorFilesTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.golden("editor-files", "")
}

func TestGoldenProfileSelector(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.openProfileDialog()
	p.golden("profile-selector", "4.2-profile-selector.png")
}
