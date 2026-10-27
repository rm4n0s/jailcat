package ui

import (
	"slices"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

func TestSecurityTabSwitches(t *testing.T) {
	tests := []struct {
		field firejail.Field
		get   func(s firejail.Settings) bool
	}{
		{firejail.FieldSeccomp, func(s firejail.Settings) bool { return s.Seccomp }},
		{firejail.FieldDropCaps, func(s firejail.Settings) bool { return s.DropCaps }},
		{firejail.FieldNoNewPrivs, func(s firejail.Settings) bool { return s.NoNewPrivs }},
		{firejail.FieldNoRoot, func(s firejail.Settings) bool { return s.NoRoot }},
		{firejail.FieldRestrictNamespaces, func(s firejail.Settings) bool { return s.RestrictNamespaces }},
		{firejail.FieldMemoryDenyWriteExecute, func(s firejail.Settings) bool { return s.MemoryDenyWriteExecute }},
		{firejail.FieldLandlock, func(s firejail.Settings) bool { return s.Landlock }},
	}
	if got := len(securityToggles()); got != len(tests) {
		t.Fatalf("the tab has %d switches, the test %d", got, len(tests))
	}
	p := tabPage(t, "Calculator", firejail.TabSecurity)
	e := p.home().editor
	if p.dots() != 0 {
		t.Fatal("a fresh build shows a change")
	}
	for _, tt := range tests {
		t.Run(string(tt.field), func(t *testing.T) {
			before := tt.get(e.Final())
			if !p.ts.Exists(gluttertest.Descendant(key("toggle:"+string(tt.field)), gluttertest.ByText(settingLabel(tt.field)))) {
				t.Fatalf("no switch labelled %q", settingLabel(tt.field))
			}
			if p.switchedOn("toggle:"+string(tt.field)) != before {
				t.Fatal("the switch does not show the setting")
			}
			p.tap("toggle:" + string(tt.field))
			if p.switchedOn("toggle:"+string(tt.field)) == before || tt.get(e.Final()) == before || !e.Changed(tt.field) || !p.tabDot(firejail.TabSecurity) {
				t.Fatalf("the switch did not change %s", tt.field)
			}
			mark := "mark:" + string(tt.field)
			if !p.ts.Exists(key(mark)) {
				t.Fatal("no changed mark")
			}
			p.reset(mark)
			if tt.get(e.Final()) != before || e.Changed(tt.field) || p.ts.Exists(key(mark)) || p.tabDot(firejail.TabSecurity) {
				t.Error("Reset did not put the switch back")
			}
			// Turning it back by hand leaves no change either.
			p.tap("toggle:" + string(tt.field))
			p.tap("toggle:" + string(tt.field))
			if e.Changed(tt.field) {
				t.Error("two taps left a change")
			}
		})
	}
	if e.HasChanges() {
		t.Errorf("the switches left changes: %+v", e.Overrides())
	}
}

func TestSecurityTabMemoryWarning(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabSecurity)
	warning := gluttertest.Descendant(key("toggle:memory_deny_write_execute"), gluttertest.ByTextContaining("JIT"))
	if !p.ts.Exists(warning) {
		t.Fatal("the switch does not warn about JIT engines")
	}
	for _, name := range []string{"Electron", "browsers", "Java", "VS Code"} {
		if !p.ts.Exists(gluttertest.Descendant(key("toggle:memory_deny_write_execute"), gluttertest.ByTextContaining(name))) {
			t.Errorf("the warning does not name %s", name)
		}
	}
}

func TestSecurityTabProtocols(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabSecurity)
	e := p.home().editor
	// The Default profile restricts the socket types to unix, inet and inet6.
	if got := e.Final().Protocols; !slices.Equal(got, []string{"unix", "inet", "inet6"}) {
		t.Fatalf("the base has %q", got)
	}
	if !p.switchedOn("toggle:protocols") {
		t.Fatal("the switch is off for a restricted base")
	}
	for _, name := range protocolChoices() {
		if !p.ts.Exists(key("protocol:" + name)) {
			t.Fatalf("no checkbox for %s", name)
		}
	}
	for name, want := range map[string]bool{"unix": true, "inet": true, "inet6": true, "netlink": false, "packet": false, "bluetooth": false} {
		if p.checked("protocol:"+name) != want {
			t.Errorf("%s is checked: %v, want %v", name, !want, want)
		}
	}

	p.tap("protocol:bluetooth")
	p.tap("protocol:netlink")
	if want := []string{"unix", "inet", "inet6", "netlink", "bluetooth"}; !slices.Equal(e.Final().Protocols, want) {
		t.Fatalf("Protocols = %q, want %q", e.Final().Protocols, want)
	}
	if !e.Changed(firejail.FieldProtocols) || !p.ts.Exists(key("mark:protocols")) || !p.tabDot(firejail.TabSecurity) {
		t.Error("the added kinds are not shown as a change")
	}
	p.tap("protocol:bluetooth")
	p.tap("protocol:netlink")
	if e.Changed(firejail.FieldProtocols) {
		t.Errorf("checking and unchecking left %q changed", e.Final().Protocols)
	}

	// Removing one kind and checking it again gives back the profile's order.
	p.tap("protocol:inet")
	if want := []string{"unix", "inet6"}; !slices.Equal(e.Final().Protocols, want) {
		t.Fatalf("Protocols = %q, want %q", e.Final().Protocols, want)
	}
	p.tap("protocol:inet")
	if e.Changed(firejail.FieldProtocols) {
		t.Errorf("checking inet again left %q changed", e.Final().Protocols)
	}

	// Reset puts the profile's list back.
	p.tap("protocol:packet")
	p.reset("mark:protocols")
	if e.Changed(firejail.FieldProtocols) || p.checked("protocol:packet") {
		t.Error("Reset did not put the socket types back")
	}
}

func TestSecurityTabProtocolsLastBoxStays(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabSecurity)
	e := p.home().editor
	p.tap("protocol:inet")
	p.tap("protocol:inet6")
	if want := []string{"unix"}; !slices.Equal(e.Final().Protocols, want) {
		t.Fatalf("Protocols = %q, want %q", e.Final().Protocols, want)
	}
	// The last kind cannot be unchecked: an empty list would not restrict anything.
	p.tap("protocol:unix")
	if want := []string{"unix"}; !slices.Equal(e.Final().Protocols, want) || !p.checked("protocol:unix") {
		t.Errorf("the last box was unchecked: %q", e.Final().Protocols)
	}
	if !p.ts.Exists(gluttertest.ByTextContaining("Turn the switch off")) {
		t.Error("the tab does not say why the last box stays")
	}
}

func TestSecurityTabProtocolsNotRestricted(t *testing.T) {
	// The profile of Visual Studio Code does not restrict the socket types.
	p := tabPage(t, "Visual Studio Code", firejail.TabSecurity)
	e := p.home().editor
	if e.Final().Protocols != nil {
		t.Fatalf("the base has %q", e.Final().Protocols)
	}
	if p.switchedOn("toggle:protocols") || p.ts.Exists(key("protocol:unix")) || !p.ts.Exists(gluttertest.ByTextContaining("Not restricted")) {
		t.Fatal("the not restricted state is not shown")
	}
	if p.dots() != 0 {
		t.Fatal("the base shows a change")
	}

	p.tap("toggle:protocols")
	if want := []string{"unix", "inet", "inet6"}; !slices.Equal(e.Final().Protocols, want) {
		t.Fatalf("Protocols = %q, want %q", e.Final().Protocols, want)
	}
	if !p.switchedOn("toggle:protocols") || !p.checked("protocol:unix") || p.checked("protocol:packet") || !p.ts.Exists(key("mark:protocols")) {
		t.Error("the switch did not turn the checkboxes on")
	}
	p.tap("toggle:protocols")
	if e.Final().Protocols != nil || e.Changed(firejail.FieldProtocols) || p.ts.Exists(key("protocol:unix")) {
		t.Errorf("the switch did not turn the restriction off: %q", e.Final().Protocols)
	}

	// Reset goes back to not restricted.
	p.tap("toggle:protocols")
	p.tap("protocol:packet")
	p.reset("mark:protocols")
	if e.Final().Protocols != nil || p.switchedOn("toggle:protocols") {
		t.Errorf("Reset left %q", e.Final().Protocols)
	}
}

func TestSecurityTabProtocolsTurnedOff(t *testing.T) {
	// Turning the restriction off where the profile restricts it is a change.
	p := tabPage(t, "Calculator", firejail.TabSecurity)
	e := p.home().editor
	p.tap("toggle:protocols")
	if e.Final().Protocols != nil || !e.Changed(firejail.FieldProtocols) || !p.ts.Exists(key("mark:protocols")) {
		t.Fatalf("Protocols = %q, changed %v", e.Final().Protocols, e.Changed(firejail.FieldProtocols))
	}
	p.reset("mark:protocols")
	if !slices.Equal(e.Final().Protocols, []string{"unix", "inet", "inet6"}) || !p.switchedOn("toggle:protocols") {
		t.Errorf("Reset left %q", e.Final().Protocols)
	}
}

func TestSecurityTabElectronNote(t *testing.T) {
	// The profile of Visual Studio Code leaves the restrictions off; the Default one has them.
	p := tabPage(t, "Visual Studio Code", firejail.TabSecurity)
	note := gluttertest.Descendant(key("note:electron"), gluttertest.ByTextContaining("Electron and Chromium"))
	if !p.ts.Exists(note) || !p.ts.Exists(gluttertest.Descendant(key("note:electron"), gluttertest.ByTextContaining("Try"))) {
		t.Error("Visual Studio Code does not get the note about Electron")
	}
	p = tabPage(t, "Calculator", firejail.TabSecurity)
	if p.ts.Exists(key("note:electron")) {
		t.Error("the note is shown for a profile that has the restrictions")
	}
}

func TestElectronNote(t *testing.T) {
	on := firejail.Settings{Seccomp: true, DropCaps: true, NoNewPrivs: true, NoRoot: true, RestrictNamespaces: true, Protocols: []string{"unix"}}
	tests := []struct {
		name   string
		mutate func(s *firejail.Settings)
		want   bool
	}{
		{"all on", func(*firejail.Settings) {}, false},
		{"three off", func(s *firejail.Settings) { s.Seccomp, s.DropCaps, s.NoRoot = false, false, false }, false},
		{"four off", func(s *firejail.Settings) { s.Seccomp, s.DropCaps, s.NoRoot, s.NoNewPrivs = false, false, false, false }, true},
		{"sockets count", func(s *firejail.Settings) { s.Seccomp, s.DropCaps, s.NoRoot, s.Protocols = false, false, false, nil }, true},
		{"all off", func(s *firejail.Settings) { *s = firejail.Settings{} }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := on
			tt.mutate(&s)
			if got := electronNote(s); got != tt.want {
				t.Errorf("electronNote = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithProtocol(t *testing.T) {
	tests := []struct {
		name string
		list []string
		base []string
		kind string
		on   bool
		want []string
	}{
		{"add in the order of the boxes", []string{"unix"}, nil, "inet", true, []string{"unix", "inet"}},
		{"add before", []string{"inet6"}, nil, "unix", true, []string{"unix", "inet6"}},
		{"remove", []string{"unix", "inet", "inet6"}, nil, "inet", false, []string{"unix", "inet6"}},
		{"back to the base order", []string{"unix", "inet6"}, []string{"inet6", "unix", "inet"}, "inet", true, []string{"inet6", "unix", "inet"}},
		{"not the same kinds as the base", []string{"unix"}, []string{"inet"}, "bluetooth", true, []string{"unix", "bluetooth"}},
		{"already checked", []string{"unix"}, nil, "unix", true, []string{"unix"}},
		{"an unknown kind stays", []string{"future", "unix"}, nil, "inet", true, []string{"unix", "inet", "future"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := slices.Clone(tt.list)
			if got := withProtocol(list, tt.base, tt.kind, tt.on); !slices.Equal(got, tt.want) {
				t.Errorf("withProtocol = %q, want %q", got, tt.want)
			}
			if !slices.Equal(list, tt.list) {
				t.Errorf("the list was changed to %q", list)
			}
		})
	}
}

func TestGoldenSecurityTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabSecurity)
	// One change: seccomp is turned on.
	p.tap("toggle:" + string(firejail.FieldSeccomp))
	p.golden("security-tab", "4.5-security.png")
}
