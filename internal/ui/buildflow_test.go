package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
)

// buildPage opens the editor of a new build of the app on a fixture backend whose
// builds are the ones the test keeps.
func buildPage(t *testing.T, app string) *page {
	t.Helper()
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.openApp(app)
	return p
}

// newVSCodePage opens the editor of a new build of Visual Studio Code, on its own firejail
// profile (the fixture's stored build of it has no launcher fields).
func newVSCodePage(t *testing.T) *page {
	t.Helper()
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	return p
}

// enabled reports whether the button of the bar works.
func (p *page) enabled(k string) bool {
	p.t.Helper()
	return p.pressable(k)
}

// waitOutcome waits for the banner of a Build or an Uninstall.
func (p *page) waitOutcome() *pageOutcome {
	p.t.Helper()
	p.waitFor("the banner", func() bool { return p.editorState().outcome != nil })
	return p.editorState().outcome
}

// waitIdle waits until no button is working.
func (p *page) waitIdle() {
	p.t.Helper()
	p.waitFor("the buttons to stop working", func() bool { return p.editorState().busy == busyNone })
}

// text reports whether the text is on screen.
func (p *page) text(s string) bool { return p.ts.Exists(gluttertest.ByText(s)) }

func TestButtonsAreEnabledWhenTheyCanWork(t *testing.T) {
	tests := []struct {
		name  string
		app   string
		setup func(f *fakeBackend)
		// change is applied to the open editor.
		change func(e *Editor)
		// preview, try and build say which buttons work; uninstall that Uninstall is shown.
		preview, try, build, uninstall bool
	}{
		{name: "a new build", app: "Calculator", preview: true, try: true, build: true},
		{name: "an installed build", app: "Visual Studio Code", preview: true, try: true, build: true, uninstall: true},
		{
			name: "a problem that is an error", app: "Calculator",
			change:  func(e *Editor) { e.Set(func(s *firejail.Settings) { s.SandboxName = "Not Valid" }) },
			preview: true,
		},
		{
			name: "firejail is not installed", app: "Calculator",
			setup:   func(f *fakeBackend) { f.firejail = FirejailInfo{Missing: true} },
			preview: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := fixtureBackend(t)
			if tt.setup != nil {
				tt.setup(backend)
			}
			p := newPage(t, backend)
			p.ready()
			p.openApp(tt.app)
			if tt.change != nil {
				p.change(tt.change)
			}
			if got := p.enabled("button:preview"); got != tt.preview {
				t.Errorf("Preview works = %v, want %v", got, tt.preview)
			}
			if got := p.enabled("button:try"); got != tt.try {
				t.Errorf("Try works = %v, want %v", got, tt.try)
			}
			if got := p.enabled("button:build"); got != tt.build {
				t.Errorf("Build works = %v, want %v", got, tt.build)
			}
			if got := p.ts.Exists(key("button:uninstall")); got != tt.uninstall {
				t.Errorf("Uninstall shown = %v, want %v", got, tt.uninstall)
			}
			if tt.uninstall && !p.enabled("button:uninstall") {
				t.Error("Uninstall does not work")
			}
		})
	}
}

func TestPreviewShowsTheFiles(t *testing.T) {
	p := newVSCodePage(t)
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.Network = firejail.NetNone
			s.Sound = true
		})
		e.SetExtraRules([]string{"dbus-user.talk org.freedesktop.Notifications"})
	})
	p.tap("button:preview")
	p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*PreviewDialog]()) })

	if !p.text(previewTitleText + ": firejail-code") {
		t.Error("the dialog is not about firejail-code")
	}
	for _, tab := range previewTabs() {
		if !p.ts.Exists(key("preview:tab:" + tab)) {
			t.Errorf("tab %q is missing", tab)
		}
	}
	// The profile tab: the lines of the settings, an ignore and the Extra rule are drawn on
	// a background; the include and the comments are not.
	kinds := map[string]bool{}
	for i := 0; ; i++ {
		w := p.ts.Widgets(key("preview:line:" + strconv.Itoa(i)))
		if len(w) == 0 {
			break
		}
		text := w[0].(*m.Container).Child().(*m.Text).Data().OrElse("")
		highlighted := w[0].(*m.Container).Color().IsSome()
		switch {
		case strings.HasPrefix(text, "net none"), strings.HasPrefix(text, "ignore "), strings.HasPrefix(text, "dbus-user.talk"):
			kinds[strings.Fields(text)[0]] = highlighted
		case strings.HasPrefix(text, "include "), strings.HasPrefix(text, "#"):
			if highlighted {
				t.Errorf("%q is highlighted", text)
			}
		}
	}
	for _, line := range []string{"net", "ignore", "dbus-user.talk"} {
		if highlighted, found := kinds[line]; !found || !highlighted {
			t.Errorf("the line %q: found %v, highlighted %v", line, found, highlighted)
		}
	}
	if !p.text(previewLegendSetting) || !p.text(previewLegendExtra) {
		t.Error("the legend lacks a color")
	}

	// The launcher and the desktop entry.
	p.ts.Tap(key("preview:tab:" + previewLauncherTab))
	p.ts.Pump()
	if !p.ts.Exists(gluttertest.ByTextContaining("exec '/usr/bin/firejail'")) {
		t.Error("the launcher tab lacks the exec line")
	}
	p.ts.Tap(key("preview:tab:" + previewDesktopTab))
	p.ts.Pump()
	if !p.text("[Desktop Entry]") || !p.ts.Exists(gluttertest.ByTextContaining("Name=Visual Studio Code (sandboxed)")) {
		t.Error("the desktop entry tab is wrong")
	}

	// Nothing was written or stored by the preview.
	if got := p.backend.savesList(); len(got) != 0 {
		t.Errorf("saves = %v", got)
	}
	if len(p.backend.buildAsks) != 0 {
		t.Error("Preview built")
	}
	p.tap("preview:close")
	if p.ts.Exists(gluttertest.ByType[*PreviewDialog]()) {
		t.Error("the dialog did not close")
	}
}

func TestPreviewWithoutAMenuEntryAndWithAnIconFallback(t *testing.T) {
	p := buildPage(t, "No Icon")
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.InstallMenu = false }) })
	p.tap("button:preview")
	p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*PreviewDialog]()) })
	if !p.ts.Exists(key("preview:icon-fallback")) {
		t.Error("the icon fallback is not explained")
	}
	p.ts.Tap(key("preview:tab:" + previewDesktopTab))
	p.ts.Pump()
	if !p.text(previewNoDesktopText) {
		t.Error("the missing entry is not explained")
	}
}

func TestPreviewFailureIsShownInWords(t *testing.T) {
	p := buildPage(t, "Calculator")
	p.backend.set(func(f *fakeBackend) { f.firejail = FirejailInfo{Missing: true} })
	p.tap("button:preview")
	p.waitFor("the message", func() bool { return p.text(firejailMissingShort) })
	if p.ts.Exists(gluttertest.ByType[*PreviewDialog]()) {
		t.Error("a dialog opened")
	}
	if !strings.Contains(p.log.String(), "FirejailNotFound") {
		t.Errorf("the log lacks the details:\n%s", p.log.String())
	}
	p.waitIdle()
}

func TestBuildWithoutConflicts(t *testing.T) {
	p := buildPage(t, "Calculator")
	e := p.home().editor
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone }) })
	if !e.Dirty() {
		t.Fatal("the editor is not dirty before the build")
	}
	p.tap("button:build")
	o := p.waitOutcome()

	if o.title != "firejail-calculator is installed" || o.warning != "" {
		t.Errorf("outcome = %+v", o)
	}
	if len(o.lines) != 4 {
		t.Errorf("the banner lists %d files, want 4: %v", len(o.lines), o.lines)
	}
	if !p.text("firejail-calculator is installed") || !p.ts.Exists(gluttertest.ByTextContaining("Launcher: ")) {
		t.Error("the banner is not shown")
	}
	// The editor holds what is stored: nothing to keep, an installed build with an ID.
	if e.Dirty() || e.Build().ID == 0 || !e.Build().Installed {
		t.Errorf("Dirty %v, ID %d, installed %v", e.Dirty(), e.Build().ID, e.Build().Installed)
	}
	if !p.ts.Exists(key("button:uninstall")) {
		t.Error("Uninstall is not offered for the installed build")
	}
	if plan := e.DraftPlan(); plan.Action != DraftNothing {
		t.Errorf("leaving the editor would write %v", plan.Action)
	}
	// The list shows it as built.
	p.waitFor("the list", func() bool { return p.home().builds[AppKey(e.App())] == 1 })
	if got := p.backend.buildAsks; len(got) != 1 || got[0] != nil {
		t.Errorf("Build was called with %v", got)
	}
	if len(p.backend.prepareAsks) != 1 || p.backend.prepareAsks[0].Settings.Network != firejail.NetNone {
		t.Error("Build did not prepare what the editor holds")
	}
	p.tap("outcome:dismiss")
	if p.editorState().outcome != nil {
		t.Error("Dismiss kept the banner")
	}
}

func TestBuildChecksTheExtraRulesAgain(t *testing.T) {
	p := buildPage(t, "Calculator")
	e := p.home().editor
	// Rows the tab never checked, as when a draft opens.
	p.change(func(e *Editor) { e.SetExtraRules([]string{"nosound"}) })
	asks := len(p.backend.checkAsks)
	p.tap("button:build")
	p.waitOutcome()
	if len(p.backend.checkAsks) != asks+1 {
		t.Errorf("the rows were checked %d times, want %d", len(p.backend.checkAsks)-asks, 1)
	}
	if !e.ExtraRulesChecked() {
		t.Error("the answer was not kept")
	}

	// A row with an error stops the Build.
	p2 := buildPage(t, "Calculator")
	p2.change(func(e *Editor) { e.SetExtraRules([]string{"this-is-not-a-command"}) })
	p2.tap("button:build")
	p2.waitFor("the message", func() bool { return p2.text(problemsBlockText) })
	if len(p2.backend.prepareAsks) != 0 {
		t.Error("a build with an error in the Extra rules was prepared")
	}
	p2.waitIdle()
}

func TestBuildFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
		log  string
	}{
		{"the name is taken", errors.New("SandboxNameTaken", "another build has this sandbox name"), "Another build already has this sandbox name", "SandboxNameTaken"},
		{"files remain in the way", errors.New("ConflictsRemain", "some targets are taken"), "Some files are in the way", "ConflictsRemain"},
		{"anything else", errors.New("FileWriteFailed", "the disk is full", "path", "/x"), "Jailcat could not build this sandbox: the disk is full.", "FileWriteFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildPage(t, "Calculator")
			p.backend.set(func(f *fakeBackend) { f.buildErr = tt.err })
			p.tap("button:build")
			p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining(tt.want)) })
			if p.ts.Exists(gluttertest.ByTextContaining(tt.log)) {
				t.Error("the tag is shown to the user")
			}
			if !strings.Contains(p.log.String(), tt.log) {
				t.Errorf("the log lacks the details:\n%s", p.log.String())
			}
			p.waitIdle()
			if !p.enabled("button:build") {
				t.Error("Build stays disabled")
			}
			if p.home().editor.Build().ID != 0 || p.editorState().outcome != nil {
				t.Error("a failed Build changed the editor")
			}
		})
	}
}

func TestBuildShowsProgressWhileItWorks(t *testing.T) {
	p := buildPage(t, "Calculator")
	gate := make(chan struct{})
	p.backend.set(func(f *fakeBackend) { f.buildGate = gate })
	p.tap("button:build")
	p.waitFor("the build", func() bool { return p.editorState().busy == busyBuilding })
	if !p.ts.Exists(key("progress")) || !p.text(busyBuildingText) {
		t.Error("no progress is shown")
	}
	if p.enabled("button:build") || p.enabled("button:try") || p.enabled("button:preview") {
		t.Error("a button works while Build runs")
	}
	close(gate)
	p.waitOutcome()
	if p.ts.Exists(key("progress")) {
		t.Error("the progress stays")
	}
}

func TestCheckWarnings(t *testing.T) {
	nested := errors.New(firejail.TagNestedSandbox, "firejail found an existing sandbox", "output", "Warning: an existing sandbox was detected")
	tests := []struct {
		name   string
		result firejail.CheckResult
		err    error
		// warning is what the banner says; output whether firejail's output is offered.
		warning string
		output  bool
	}{
		{"passes", firejail.CheckResult{OK: true}, nil, "", false},
		{"fails", firejail.CheckResult{Status: firejail.ExitStatus{Code: 1}, Output: "Error: cannot open profile\n"}, nil, checkFailedText, true},
		{"is skipped", firejail.CheckResult{Skipped: true, Reason: "the check was skipped: too many ignore entries"}, nil, "The files are installed. The check was skipped: too many ignore entries.", false},
		{"inside another sandbox", firejail.CheckResult{OK: true, Output: "Warning: an existing sandbox was detected"}, nested, checkNestedText, false},
		{"cannot run", firejail.CheckResult{}, errors.New("CheckTimeout", "the check took too long"), checkUnknownPrefix + "the check took too long.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildPage(t, "Calculator")
			p.backend.set(func(f *fakeBackend) { f.checkResult, f.checkErr = tt.result, tt.err })
			p.tap("button:build")
			o := p.waitOutcome()
			if o.warning != tt.warning {
				t.Errorf("warning = %q, want %q", o.warning, tt.warning)
			}
			// The install stands whatever the check says.
			if !p.text("firejail-calculator is installed") || !p.home().editor.Build().Installed {
				t.Error("the build is not shown as installed")
			}
			if got := p.ts.Exists(key("outcome:warning")); got != (tt.warning != "") {
				t.Errorf("warning shown = %v", got)
			}
			if got := p.ts.Exists(key("outcome:toggle-output")); got != tt.output {
				t.Errorf("output offered = %v, want %v", got, tt.output)
			}
			if tt.output {
				if p.ts.Exists(key("outcome:output")) {
					t.Error("the output is open before it is asked for")
				}
				p.tap("outcome:toggle-output")
				if !p.ts.Exists(gluttertest.ByTextContaining("cannot open profile")) {
					t.Error("firejail's output is not shown")
				}
				p.tap("outcome:toggle-output")
				if p.ts.Exists(key("outcome:output")) {
					t.Error("the output does not close")
				}
			}
		})
	}
}

func TestBuildTellsWhyTheIconIsRandom(t *testing.T) {
	p := buildPage(t, "Calculator")
	p.backend.set(func(f *fakeBackend) { f.iconFallback = "the app's icon \"blue\" was not found" })
	p.tap("button:build")
	o := p.waitOutcome()
	if len(o.notes) != 1 || !strings.Contains(o.notes[0], "was not found") {
		t.Errorf("notes = %v", o.notes)
	}
}

// conflict returns a conflict of the launcher of the Calculator build.
func conflict(p *page, kind install.ConflictKind, overwritable bool, owner int64) install.Conflict {
	path := filepath.Join(p.backend.fj.dirs.Bin, "firejail-calculator")
	return install.Conflict{File: install.File{Kind: install.KindLauncher, Path: path}, Kind: kind, Owner: owner, Overwritable: overwritable}
}

func TestConflictsDialog(t *testing.T) {
	tests := []struct {
		name         string
		kind         install.ConflictKind
		overwritable bool
		owner        int64
		// why is a part of the sentence that explains the conflict.
		why string
	}{
		{"edited", install.ConflictEdited, true, 5, "was changed since"},
		{"another build's", install.ConflictOtherBuild, false, 9, "belongs to the build firejail-other"},
		{"foreign with the header", install.ConflictForeignJailcat, true, 0, "no build here owns it"},
		{"foreign", install.ConflictForeign, false, 0, "Jailcat did not make it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildPage(t, "Calculator")
			c := conflict(p, tt.kind, tt.overwritable, tt.owner)
			p.backend.set(func(f *fakeBackend) {
				f.conflicts, f.owners = []install.Conflict{c}, map[int64]string{9: "firejail-other"}
			})
			p.tap("button:build")
			p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) })

			if !p.ts.Exists(gluttertest.ByTextContaining("~/.local/bin/firejail-calculator")) &&
				!p.ts.Exists(gluttertest.ByTextContaining("firejail-calculator")) {
				t.Error("the path is not shown")
			}
			if !p.ts.Exists(gluttertest.ByTextContaining(tt.why)) {
				t.Errorf("the dialog does not say why (%q)", tt.why)
			}
			if got := p.ts.Exists(key("conflict:overwrite:0")); got != tt.overwritable {
				t.Errorf("checkbox shown = %v, want %v", got, tt.overwritable)
			}
			if got := p.ts.Exists(key("conflict:blocked")); got == tt.overwritable {
				t.Errorf("the explanation of a file Jailcat will not overwrite is shown = %v", got)
			}
			if p.enabled("conflict:build") {
				t.Error("Overwrite and build works before anything is ticked")
			}
			if len(p.backend.buildAsks) != 0 {
				t.Fatal("Build ran before the conflict was dealt with")
			}
			if !tt.overwritable {
				// Nothing can make it work: the file stays.
				p.tap("conflict:cancel")
				p.waitIdle()
				if len(p.backend.buildAsks) != 0 {
					t.Error("Cancel built")
				}
				return
			}
			p.tap("conflict:overwrite:0")
			if !p.enabled("conflict:build") {
				t.Fatal("Overwrite and build does not work after the file is ticked")
			}
			p.tap("conflict:build")
			o := p.waitOutcome()
			if o.title != "firejail-calculator is installed" {
				t.Errorf("title = %q", o.title)
			}
			if got := p.backend.buildAsks; len(got) != 1 || !got[0][c.File.Path] {
				t.Errorf("Build was told to overwrite %v", got)
			}
		})
	}
}

func TestConflictsCancelAndRename(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		p := buildPage(t, "Calculator")
		p.backend.set(func(f *fakeBackend) { f.conflicts = []install.Conflict{conflict(p, install.ConflictEdited, true, 0)} })
		p.tap("button:build")
		p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) })
		p.tap("conflict:cancel")
		p.waitIdle()
		if p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) || len(p.backend.buildAsks) != 0 {
			t.Error("Cancel did not stop the build")
		}
		if p.editorState().outcome != nil || !p.enabled("button:build") {
			t.Error("the editor is not as it was")
		}
	})
	t.Run("choose another sandbox name", func(t *testing.T) {
		p := buildPage(t, "Calculator")
		p.backend.set(func(f *fakeBackend) { f.conflicts = []install.Conflict{conflict(p, install.ConflictForeign, false, 0)} })
		p.tap("button:build")
		p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) })
		p.tap("conflict:rename")
		p.waitFor("the Launcher tab", func() bool { return p.ts.Exists(key("sandbox:field")) })
		if p.editorState().tab != firejail.TabLauncher {
			t.Errorf("the tab is %q", p.editorState().tab)
		}
		field := p.ts.Widget(key("sandbox:field")).(*m.TextField)
		p.waitFor("the focus", func() bool { return field.FocusNode().HasFocus() })
		if len(p.backend.buildAsks) != 0 {
			t.Error("the build ran")
		}
		p.waitIdle()
	})
	t.Run("choose another sandbox name on the Launcher tab", func(t *testing.T) {
		p := buildPage(t, "Calculator")
		p.selectTab(firejail.TabLauncher)
		p.backend.set(func(f *fakeBackend) { f.conflicts = []install.Conflict{conflict(p, install.ConflictForeign, false, 0)} })
		p.tap("button:build")
		p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) })
		p.tap("conflict:rename")
		field := p.ts.Widget(key("sandbox:field")).(*m.TextField)
		p.waitFor("the focus", func() bool { return field.FocusNode().HasFocus() })
	})
}

func TestRenamedBuildNamesTheRemovedFiles(t *testing.T) {
	p := buildPage(t, "Visual Studio Code")
	// The fake does not remove files; the banner lists what the result says was.
	o := newBuiltOutcome(BuildResult{
		Build:   p.home().editor.Build(),
		Removed: []install.File{{Kind: install.KindLauncher, Path: filepath.Join(p.backend.fj.dirs.Home, ".local", "bin", "firejail-old")}},
	}, p.backend.fj.dirs.Home)
	if len(o.notes) != 1 || o.notes[0] != removedOldText+"~/.local/bin/firejail-old" {
		t.Errorf("notes = %v", o.notes)
	}
}
