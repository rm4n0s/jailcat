package ui

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// vscodeSwitchesOn returns what the three switches show, in the order of the section.
func vscodeSwitchesOn(e *Editor) [3]bool {
	var out [3]bool
	for i, sw := range vscodeSwitches(e.Final()) {
		out[i] = sw.on
	}
	return out
}

func TestFilesTabVSCodeSectionOnlyForVSCode(t *testing.T) {
	tests := []struct {
		app  string
		want bool
	}{{"Visual Studio Code", true}, {"Calculator", false}, {"curl", false}}
	for _, tt := range tests {
		t.Run(tt.app, func(t *testing.T) {
			p := filesPage(t, tt.app)
			for _, f := range []firejail.Field{firejail.FieldVSCodeOwnData, firejail.FieldVSCodeShareSettings, firejail.FieldVSCodeShellFiles} {
				if got := p.ts.Exists(key("toggle:" + string(f))); got != tt.want {
					t.Errorf("%s: the %s switch is shown: %v, want %v", tt.app, f, got, tt.want)
				}
			}
			if got := p.ts.Exists(gluttertest.ByText(vscodeTitleText)); got != tt.want {
				t.Errorf("%s: the VS Code title is shown: %v", tt.app, got)
			}
			e := p.home().editor
			if e.IsVSCode() != tt.want {
				t.Errorf("IsVSCode = %v", e.IsVSCode())
			}
			s := e.Final()
			if got := s.VSCodeOwnData && s.VSCodeShareSettings && s.VSCodeShellFiles; got != tt.want {
				t.Errorf("the new build has the switches on: %v, want %v", got, tt.want)
			}
			if e.HasChanges() {
				t.Errorf("a new build has changes: %v", e.Overrides().ChangedFields())
			}
		})
	}
}

// TestFilesTabVSCodeSwitchesFollowTheHome checks which switches the home choice allows,
// what they show, and what the disabled ones say.
func TestFilesTabVSCodeSwitchesFollowTheHome(t *testing.T) {
	tests := []struct {
		home firejail.HomeMode
		// shows is what the three switches show; enabled which of them can be changed.
		shows   [3]bool
		enabled [3]bool
		reasons [3]string
	}{
		{firejail.HomeFull, [3]bool{true, true, false}, [3]bool{true, true, false}, [3]string{"", "", vscodeShellNeedsMode}},
		{firejail.HomeChosen, [3]bool{true, true, true}, [3]bool{true, true, true}, [3]string{}},
		{firejail.HomeEmpty, [3]bool{true, false, false}, [3]bool{true, false, false}, [3]string{"", vscodeShareNeedsHome, vscodeShellNeedsMode}},
		{firejail.HomePrivate, [3]bool{true, false, false}, [3]bool{true, false, false}, [3]string{"", vscodeShareNeedsHome, vscodeShellNeedsMode}},
	}
	fields := [3]firejail.Field{firejail.FieldVSCodeOwnData, firejail.FieldVSCodeShareSettings, firejail.FieldVSCodeShellFiles}
	for _, tt := range tests {
		t.Run(string(tt.home), func(t *testing.T) {
			p := filesPage(t, "Visual Studio Code")
			e := p.home().editor
			p.tap("home:" + string(tt.home))
			if got := vscodeSwitchesOn(e); got != tt.shows {
				t.Errorf("the switches show %v, want %v", got, tt.shows)
			}
			for i, sw := range vscodeSwitches(e.Final()) {
				if (sw.reason == "") != tt.enabled[i] || sw.reason != tt.reasons[i] {
					t.Errorf("%s: reason %q, enabled %v; want reason %q, enabled %v", fields[i], sw.reason, sw.reason == "", tt.reasons[i], tt.enabled[i])
				}
				if sw.reason != "" && !p.ts.Exists(gluttertest.ByText(sw.reason)) {
					t.Errorf("%s: the reason %q is not on screen", fields[i], sw.reason)
				}
			}
			// Changing the home choice is not a change of the switches: the user's own
			// values stay, so going back gives them back.
			s := e.Final()
			if !s.VSCodeOwnData || !s.VSCodeShareSettings || !s.VSCodeShellFiles {
				t.Errorf("the stored values changed with the home: %+v", s)
			}
		})
	}
}

func TestFilesTabVSCodeSwitchesChangeAndReset(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	e := p.home().editor
	p.tap("home:chosen")
	p.change(func(e *Editor) { e.AddFolder("${HOME}/Projects") })
	for _, f := range []firejail.Field{firejail.FieldVSCodeOwnData, firejail.FieldVSCodeShareSettings, firejail.FieldVSCodeShellFiles} {
		t.Run(string(f), func(t *testing.T) {
			if p.ts.Exists(key("mark:" + string(f))) {
				t.Fatal("a mark on a fresh switch")
			}
			p.tap("toggle:" + string(f))
			if v, _ := f.Value(e.Final()); v != false {
				t.Errorf("tapping did not turn it off: %v", v)
			}
			if !e.Changed(f) || !p.ts.Exists(key("mark:"+string(f))) || !p.tabDot(firejail.TabFiles) {
				t.Errorf("no mark or dot after the change")
			}
			if !e.Overrides().Changed(f) {
				t.Errorf("the change is not stored in the overrides")
			}
			p.ts.Tap(gluttertest.Descendant(key("mark:"+string(f)), gluttertest.ByText(resetText)))
			p.ts.Pump()
			if v, _ := f.Value(e.Final()); v != true || e.Changed(f) {
				t.Errorf("Reset did not turn it on again: %v", v)
			}
		})
	}
}

func TestFilesTabVSCodeShareNeedsOwnData(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	e := p.home().editor
	p.tap("toggle:" + string(firejail.FieldVSCodeOwnData))
	var share vscodeSwitch
	for _, sw := range vscodeSwitches(e.Final()) {
		if sw.field == firejail.FieldVSCodeShareSettings {
			share = sw
		}
	}
	if share.on || share.reason != vscodeShareNeedsData {
		t.Errorf("share = %+v", share)
	}
}

// TestFilesTabVSCodeShellFilesSaysWhatIsVisible checks the switch says that what the
// files export is visible in the sandbox.
func TestFilesTabVSCodeShellFilesSaysWhatIsVisible(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	p.tap("home:chosen")
	if !p.ts.Exists(gluttertest.ByText(vscodeShellText)) {
		t.Error("the switch of the shell files does not warn about what they export")
	}
}

// TestEditorVSCodeDefaultsKeepOldBuilds checks that a stored build made before the
// switches existed (no override for them) opens with them on, and that one that turned
// them off keeps that.
func TestEditorVSCodeDefaultsKeepOldBuilds(t *testing.T) {
	tests := []struct {
		name      string
		app       apps.App
		overrides firejail.Overrides
		want      [3]bool
	}{
		{"old build of VS Code", codeApp(), firejail.Overrides{}, [3]bool{true, true, true}},
		{"build that turned one off", codeApp(), firejail.Diff(firejail.WithAppDefaults(firejail.BaseNone(), "/usr/bin/code").Settings, func() firejail.Settings {
			s := firejail.WithAppDefaults(firejail.BaseNone(), "/usr/bin/code").Settings
			s.VSCodeShellFiles = false
			return s
		}()), [3]bool{true, true, false}},
		{"old build of another app", apps.App{ID: "x.desktop", Binary: "/usr/bin/x", BinaryName: "x"}, firejail.Overrides{}, [3]bool{false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEditor(tt.app, EditorOpts{
				Base:  firejail.BaseNone(),
				Build: store.Build{ID: 3, SandboxName: "firejail-x", Overrides: tt.overrides, IconMode: store.IconModeBadge},
			})
			s := e.Final()
			if got := [3]bool{s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles}; got != tt.want {
				t.Errorf("switches = %v, want %v", got, tt.want)
			}
			if e.Dirty() {
				t.Errorf("opening the build made it dirty")
			}
			// A build of 1.0.0 stores nothing about the switches, and opening it does not
			// add anything.
			if e.Overrides().Changed(firejail.FieldVSCodeOwnData) {
				t.Errorf("the overrides hold the default")
			}
		})
	}
}

func TestGoldenFilesTabVSCode(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	// Only the folders I choose: all three switches can be changed; one of them was.
	p.tap("home:chosen")
	p.change(func(e *Editor) { e.AddFolder("${HOME}/Projects") })
	p.tap("toggle:" + string(firejail.FieldVSCodeShareSettings))
	p.golden("files-tab-vscode", "")
}

func TestGoldenFilesTabVSCodeEmptyHome(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	// Empty home: Share settings and Shell files are off and say why.
	p.tap("home:empty")
	p.golden("files-tab-vscode-empty", "")
}
