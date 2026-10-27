package ui

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

func TestDevicesTabChecks(t *testing.T) {
	tests := []struct {
		field firejail.Field
		get   func(s firejail.Settings) bool
	}{
		{firejail.FieldSound, func(s firejail.Settings) bool { return s.Sound }},
		{firejail.FieldCamera, func(s firejail.Settings) bool { return s.Camera }},
		{firejail.FieldGPU, func(s firejail.Settings) bool { return s.GPU }},
		{firejail.FieldPrinters, func(s firejail.Settings) bool { return s.Printers }},
		{firejail.FieldU2F, func(s firejail.Settings) bool { return s.U2F }},
		{firejail.FieldInput, func(s firejail.Settings) bool { return s.Input }},
		{firejail.FieldDVD, func(s firejail.Settings) bool { return s.DVD }},
		{firejail.FieldTV, func(s firejail.Settings) bool { return s.TV }},
		{firejail.FieldTPM, func(s firejail.Settings) bool { return s.TPM }},
	}
	if got := len(deviceToggles()); got != len(tests) {
		t.Fatalf("the tab has %d checkboxes, the test %d", got, len(tests))
	}
	p := tabPage(t, "Calculator", firejail.TabDevices)
	e := p.home().editor
	if p.dots() != 0 {
		t.Fatal("a fresh build shows a change")
	}
	for _, tt := range tests {
		t.Run(string(tt.field), func(t *testing.T) {
			before := tt.get(e.Final())
			if !p.ts.Exists(gluttertest.Descendant(key("toggle:"+string(tt.field)), gluttertest.ByText(settingLabel(tt.field)))) {
				t.Fatalf("no checkbox labelled %q", settingLabel(tt.field))
			}
			if p.checked("toggle:"+string(tt.field)) != before {
				t.Fatal("the checkbox does not show the setting")
			}
			p.tap("toggle:" + string(tt.field))
			if p.checked("toggle:"+string(tt.field)) == before || tt.get(e.Final()) == before || !e.Changed(tt.field) || !p.tabDot(firejail.TabDevices) {
				t.Fatalf("the checkbox did not change %s", tt.field)
			}
			mark := "mark:" + string(tt.field)
			if !p.ts.Exists(key(mark)) {
				t.Fatal("no changed mark")
			}
			p.reset(mark)
			if tt.get(e.Final()) != before || e.Changed(tt.field) || p.ts.Exists(key(mark)) || p.tabDot(firejail.TabDevices) {
				t.Error("Reset did not put the checkbox back")
			}
		})
	}
	if e.HasChanges() {
		t.Errorf("the checkboxes left changes: %+v", e.Overrides())
	}
}

func TestDevicesTabCheckedMeansAllowed(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDevices)
	e := p.home().editor
	// The Default profile allows printers and blocks the camera (novideo).
	if s := e.Final(); !s.Printers || s.Camera {
		t.Fatalf("the base has Printers %v and Camera %v", s.Printers, s.Camera)
	}
	if !p.checked("toggle:printers") || p.checked("toggle:camera") {
		t.Error("the boxes do not show the base")
	}
	p.tap("toggle:" + string(firejail.FieldPrinters))
	if e.Final().Printers {
		t.Error("unchecking Printers left it allowed")
	}
	p.tap("toggle:" + string(firejail.FieldPrinters))
	if !e.Final().Printers || e.HasChanges() {
		t.Error("checking Printers again left a change")
	}
	// Allowing what the profile blocks is the change.
	p.tap("toggle:" + string(firejail.FieldCamera))
	if !e.Final().Camera || !p.ts.Exists(key("mark:camera")) {
		t.Error("allowing the camera is not shown as a change")
	}
}

func TestDevicesTabExplainsSound(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDevices)
	if !p.ts.Exists(gluttertest.ByTextContaining("cannot allow one without the other")) {
		t.Error("the sound checkbox does not say that firejail cannot separate the microphone")
	}
}

func TestDevicesTabShowsABlockingProfile(t *testing.T) {
	// The profile of Visual Studio Code blocks sound (nosound): the box starts unchecked
	// and without a mark, and checking it is the change.
	p := tabPage(t, "Visual Studio Code", firejail.TabDevices)
	e := p.home().editor
	if e.Final().Sound || p.ts.Exists(key("mark:sound")) || p.dots() != 0 {
		t.Fatalf("Sound = %v, mark %v", e.Final().Sound, p.ts.Exists(key("mark:sound")))
	}
	p.tap("toggle:" + string(firejail.FieldSound))
	if !e.Final().Sound || !p.ts.Exists(key("mark:sound")) || !p.tabDot(firejail.TabDevices) {
		t.Error("allowing sound is not shown as a change")
	}
	p.reset("mark:sound")
	if e.Final().Sound {
		t.Error("Reset did not block sound again")
	}
}

func TestGoldenDevicesTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabDevices)
	// One change: the camera is allowed.
	p.tap("toggle:" + string(firejail.FieldCamera))
	p.golden("devices-tab", "4.4-devices.png")
}
