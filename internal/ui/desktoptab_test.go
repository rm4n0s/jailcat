package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

func TestDesktopTabSessionBus(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	if e.Final().SessionBus != firejail.BusFull || p.ts.Exists(key("bus:notifications")) || !p.ts.Exists(key("session-pick")) {
		t.Fatalf("a fresh build: bus %q", e.Final().SessionBus)
	}
	for _, mode := range []firejail.BusMode{firejail.BusFiltered, firejail.BusNone, firejail.BusFull} {
		p.tap("session-bus:" + string(mode))
		if e.Final().SessionBus != mode {
			t.Fatalf("after tapping %s the bus is %q", mode, e.Final().SessionBus)
		}
		changed := mode != firejail.BusFull
		if p.tabDot(firejail.TabDesktop) != changed || p.ts.Exists(key("mark:session_bus")) != changed {
			t.Errorf("%s: dot %v, mark %v, want %v", mode, p.tabDot(firejail.TabDesktop), p.ts.Exists(key("mark:session_bus")), changed)
		}
		if shown := p.ts.Exists(key("bus:notifications")); shown != (mode == firejail.BusFiltered) {
			t.Errorf("%s: the checkboxes are shown: %v", mode, shown)
		}
	}
	p.tap("session-bus:none")
	p.reset("mark:session_bus")
	if e.Final().SessionBus != firejail.BusFull || e.Changed(firejail.FieldSessionBus) {
		t.Errorf("Reset left the bus %q", e.Final().SessionBus)
	}
}

func TestDesktopTabBusCheckboxes(t *testing.T) {
	tests := []struct {
		key   string
		name  string
		field firejail.Field
		own   bool
	}{
		{"notifications", firejail.DBusNotifications, firejail.FieldSessionTalk, false},
		{"portals", firejail.DBusPortals, firejail.FieldSessionTalk, false},
		{"keyring", firejail.DBusKeyring, firejail.FieldSessionTalk, false},
		{"tray", firejail.DBusTray, firejail.FieldSessionTalk, false},
		{"media-keys", firejail.MPRISName("Calculator"), firejail.FieldSessionOwn, true},
	}
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	p.tap("session-bus:filtered")
	// The Filtered choice is a change; the names are measured on their own.
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			list := func() []string {
				if tt.own {
					return e.Final().SessionOwn
				}
				return e.Final().SessionTalk
			}
			if slices.Contains(list(), tt.name) || e.Changed(tt.field) {
				t.Fatal("the box starts checked")
			}
			p.tap("bus:" + tt.key)
			if !slices.Contains(list(), tt.name) || !e.Changed(tt.field) || !p.ts.Exists(key("mark:"+string(tt.field))) {
				t.Fatalf("checking the box left %q, changed %v", list(), e.Changed(tt.field))
			}
			// The checkbox's own name is not in the list of more names.
			if p.ts.Exists(gluttertest.Descendant(key(map[bool]string{false: "talk", true: "own"}[tt.own]+":row:0"), gluttertest.ByText(tt.name))) {
				t.Error("the name of the checkbox is also listed as a further name")
			}
			p.tap("bus:" + tt.key)
			if slices.Contains(list(), tt.name) || e.Changed(tt.field) {
				t.Errorf("unchecking left %q", list())
			}
		})
	}
	// Reset of the talk names puts back what the profile had.
	p.tap("bus:notifications")
	p.tap("bus:keyring")
	p.reset("mark:session_talk")
	if len(e.Final().SessionTalk) != 0 || e.Changed(firejail.FieldSessionTalk) {
		t.Errorf("Reset left %q", e.Final().SessionTalk)
	}
}

func TestDesktopTabBusCheckboxesFollowTheSettings(t *testing.T) {
	// A build that already allows a name shows the box checked.
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.SessionBus = firejail.BusFiltered
			s.SessionTalk = []string{firejail.DBusTray, "org.example.Chat"}
		})
	})
	if !p.checked("bus:tray") || p.checked("bus:notifications") {
		t.Error("the boxes do not follow the settings")
	}
	if got := e.Final().DesktopChecks("Calculator"); !got.Tray || got.Notifications {
		t.Errorf("DesktopChecks = %+v", got)
	}
	if !p.ts.Exists(gluttertest.Descendant(key("talk:row:0"), gluttertest.ByText("org.example.Chat"))) {
		t.Error("the further name is not listed")
	}
}

func TestDesktopTabFurtherNames(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	p.tap("session-bus:filtered")

	add := func(list, text string) {
		p.t.Helper()
		p.ts.EnterText(key(list+":field"), text)
		p.ts.Pump()
		p.tap(list + ":add")
	}
	add("talk", "org.example.Chat")
	add("talk", "org.example.*")
	add("own", "org.example.Player")
	if want := []string{"org.example.Chat", "org.example.*"}; !slices.Equal(e.Final().SessionTalk, want) {
		t.Errorf("talk = %q, want %q", e.Final().SessionTalk, want)
	}
	if want := []string{"org.example.Player"}; !slices.Equal(e.Final().SessionOwn, want) {
		t.Errorf("own = %q, want %q", e.Final().SessionOwn, want)
	}
	if !p.ts.Exists(key("mark:session_talk")) || !p.ts.Exists(key("mark:session_own")) {
		t.Error("the marks of the lists are missing")
	}

	// A name of a checkbox, or one in the list, is refused with its own message.
	tests := []struct{ list, text, message string }{
		{"talk", firejail.DBusKeyring, "one of the checkboxes"},
		{"talk", "org.example.Chat", "in the list already"},
		{"own", firejail.MPRISName("Calculator"), "one of the checkboxes"},
	}
	for _, tt := range tests {
		add(tt.list, tt.text)
		if !p.ts.Exists(gluttertest.Descendant(key(tt.list+":problem"), gluttertest.ByTextContaining(tt.message))) {
			t.Errorf("%q: no message with %q", tt.text, tt.message)
		}
		p.ts.EnterText(key(tt.list+":field"), "")
		p.ts.Pump()
	}

	p.tap("talk:remove:0")
	if want := []string{"org.example.*"}; !slices.Equal(e.Final().SessionTalk, want) {
		t.Errorf("after the removal talk = %q, want %q", e.Final().SessionTalk, want)
	}
	// A box and a further name together: removing the name leaves the box.
	p.tap("bus:tray")
	p.tap("talk:remove:0")
	if want := []string{firejail.DBusTray}; !slices.Equal(e.Final().SessionTalk, want) {
		t.Errorf("talk = %q, want %q", e.Final().SessionTalk, want)
	}
	p.tap("own:remove:0")
	if len(e.Final().SessionOwn) != 0 {
		t.Errorf("own = %q", e.Final().SessionOwn)
	}
}

func TestBusNameProblem(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
		part string // a part of the message
	}{
		{"org.example.App", true, ""},
		{"org.freedesktop.portal.*", true, ""},
		{"org.example_x.My-App2", true, ""},
		{"com.example.*", true, ""},
		{"a.b", true, ""},
		{"", false, "Type a D-Bus name"},
		{"org.example App", false, "no blanks"},
		{"org.example\tApp", false, "no blanks"},
		{"org.example\nApp", false, "no blanks"},
		{"org", false, "at least two parts"},
		{"*", false, "at least two parts"},
		{"org..example", false, "empty part"},
		{".org.example", false, "empty part"},
		{"org.example.", false, "empty part"},
		{"org.*.example", false, "only be the last part"},
		{"org.exa*mple.App", false, "letters, digits"},
		{"org.1example.App", false, "start with a digit"},
		{"org.exämple.App", false, "letters, digits"},
		{"org.example.App#x", false, "letters, digits"},
		{"org.example.A$p", false, "letters, digits"},
		{strings.Repeat("a", 250) + ".example", false, "at most 255"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := busNameProblem(tt.name)
			if (got == "") != tt.ok {
				t.Fatalf("busNameProblem(%q) = %q, want ok %v", tt.name, got, tt.ok)
			}
			if !tt.ok && !strings.Contains(got, tt.part) {
				t.Errorf("busNameProblem(%q) = %q, want it to contain %q", tt.name, got, tt.part)
			}
		})
	}
}

func TestDesktopTabFurtherNameMessages(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	p.tap("session-bus:filtered")
	for _, text := range []string{"org.example App", "org", "org.*.x", "org.1x.y"} {
		p.ts.EnterText(key("talk:field"), text)
		p.ts.Pump()
		p.tap("talk:add")
		if !p.ts.Exists(key("talk:problem")) {
			t.Errorf("%q: no message", text)
		}
	}
	if len(e.Final().SessionTalk) != 0 {
		t.Errorf("a refused name was added: %q", e.Final().SessionTalk)
	}
	// Whitespace around a good name is trimmed.
	p.ts.EnterText(key("talk:field"), "  org.example.Chat  ")
	p.ts.Pump()
	p.tap("talk:add")
	if want := []string{"org.example.Chat"}; !slices.Equal(e.Final().SessionTalk, want) {
		t.Errorf("talk = %q, want %q", e.Final().SessionTalk, want)
	}
}

func TestDesktopTabSystemBus(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	if p.ts.Exists(key("system-bus:filtered")) || p.ts.Exists(key("note:system-bus")) {
		t.Error("the Filtered choice is shown for a base that does not filter")
	}
	p.tap("system-bus:none")
	if e.Final().SystemBus != firejail.BusNone || !p.ts.Exists(key("mark:system_bus")) || !p.tabDot(firejail.TabDesktop) {
		t.Fatalf("system bus %q", e.Final().SystemBus)
	}
	p.reset("mark:system_bus")
	if e.Final().SystemBus != firejail.BusFull || e.Changed(firejail.FieldSystemBus) {
		t.Errorf("Reset left %q", e.Final().SystemBus)
	}
}

func TestDesktopTabSystemBusFilteredByTheProfile(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	e.SwitchProfile(e.BaseProfile(), withBase(e, func(s *firejail.Settings) { s.SystemBus = firejail.BusFiltered }), e.Profile(), true)
	p.change(func(*Editor) {})
	if !p.ts.Exists(key("system-bus:filtered")) || !p.ts.Exists(key("note:system-bus")) || p.ts.Exists(key("mark:system_bus")) {
		t.Fatal("the profile's filter is not shown read-only with its note")
	}
	// The read-only choice cannot be picked again once left.
	p.tap("system-bus:none")
	if e.Final().SystemBus != firejail.BusNone {
		t.Fatalf("system bus %q", e.Final().SystemBus)
	}
	p.tap("system-bus:filtered")
	if e.Final().SystemBus != firejail.BusNone {
		t.Error("the read-only choice was picked")
	}
	if !p.ts.Exists(key("system-bus:filtered")) {
		t.Error("the read-only choice disappears once left")
	}
	p.reset("mark:system_bus")
	if e.Final().SystemBus != firejail.BusFiltered {
		t.Errorf("Reset gave %q", e.Final().SystemBus)
	}
}

func TestDesktopTabX11(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabDesktop)
	e := p.home().editor
	p.waitFor("the programs", func() bool {
		return gluttertest.StateOf[*desktopTabState](p.ts, gluttertest.ByType[*DesktopTab]()).programs != nil
	})
	asked := func() []string {
		var out []string
		p.backend.set(func(f *fakeBackend) { out = slices.Clone(f.programAsks) })
		slices.Sort(out)
		return out
	}
	if want := []string{"Xephyr", "Xvfb", "xpra"}; !slices.Equal(asked(), want) {
		t.Errorf("the backend was asked for %q, want %q", asked(), want)
	}
	if e.Final().X11 != firejail.X11Off || p.ts.Exists(key("mark:x11")) {
		t.Fatal("a fresh build shows an X11 change")
	}

	// Xephyr is not installed: its reason is shown and it cannot be picked.
	if !p.ts.Exists(gluttertest.ByText(x11UnavailableText("Xephyr"))) {
		t.Error("the reason for Xephyr is not shown")
	}
	if p.ts.Exists(gluttertest.ByText(x11UnavailableText("xpra"))) || p.ts.Exists(gluttertest.ByText(x11UnavailableText("Xvfb"))) {
		t.Error("a reason is shown for a program that is installed")
	}
	p.tap("x11:xephyr")
	if e.Final().X11 != firejail.X11Off {
		t.Errorf("a program that is not installed was picked: %q", e.Final().X11)
	}

	for _, mode := range []firejail.X11Mode{firejail.X11Xpra, firejail.X11Xvfb, firejail.X11Off} {
		p.tap("x11:" + string(mode))
		if e.Final().X11 != mode {
			t.Fatalf("after tapping %s the mode is %q", mode, e.Final().X11)
		}
		changed := mode != firejail.X11Off
		if p.ts.Exists(key("mark:x11")) != changed || p.tabDot(firejail.TabDesktop) != changed {
			t.Errorf("%s: mark %v, dot %v, want %v", mode, p.ts.Exists(key("mark:x11")), p.tabDot(firejail.TabDesktop), changed)
		}
	}
	p.tap("x11:xpra")
	p.reset("mark:x11")
	if e.Final().X11 != firejail.X11Off || e.Changed(firejail.FieldX11) {
		t.Errorf("Reset left %q", e.Final().X11)
	}
	if !p.ts.Exists(gluttertest.ByTextContaining("network namespace")) {
		t.Error("the note about the network namespace is missing")
	}
}

func TestDesktopTabX11NothingInstalled(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil; f.programs = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	p.selectTab(firejail.TabDesktop)
	p.waitFor("the programs", func() bool {
		return gluttertest.StateOf[*desktopTabState](p.ts, gluttertest.ByType[*DesktopTab]()).programs != nil
	})
	for _, program := range []string{"xpra", "Xephyr", "Xvfb"} {
		if !p.ts.Exists(gluttertest.ByText(x11UnavailableText(program))) {
			t.Errorf("no reason for %s", program)
		}
	}
	p.tap("x11:xvfb")
	if p.home().editor.Final().X11 != firejail.X11Off {
		t.Error("Xvfb was picked though it is not installed")
	}
}

func TestDesktopTabX11WaitsForTheSearch(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Calculator")
	// The search runs off the UI thread: right after the tab opens its answer is not in yet,
	// and the choices say so instead of offering a program that may not exist.
	p.ts.Tap(key("tab:" + string(firejail.TabDesktop)))
	p.ts.Pump()
	state := gluttertest.StateOf[*desktopTabState](p.ts, gluttertest.ByType[*DesktopTab]())
	if state.programs == nil && !p.ts.Exists(gluttertest.ByText(x11LookingText)) {
		t.Error("the choices do not say they are looking")
	}
	p.waitFor("the programs", func() bool { return state.programs != nil })
	if p.ts.Exists(gluttertest.ByText(x11LookingText)) {
		t.Error("the choices still say they are looking")
	}
}

func TestDesktopTabX11OfAProfileThatIsNotInstalled(t *testing.T) {
	// A base profile may choose a program this computer lacks: the choice shows selected,
	// with its reason, and the user can pick Off.
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Calculator")
	p.selectTab(firejail.TabDesktop)
	e.SwitchProfile(e.BaseProfile(), withBase(e, func(s *firejail.Settings) { s.X11 = firejail.X11Xephyr }), e.Profile(), true)
	p.change(func(*Editor) {})
	p.waitFor("the programs", func() bool {
		return gluttertest.StateOf[*desktopTabState](p.ts, gluttertest.ByType[*DesktopTab]()).programs != nil
	})
	if e.Final().X11 != firejail.X11Xephyr || !p.ts.Exists(gluttertest.ByText(x11UnavailableText("Xephyr"))) {
		t.Fatal("the profile's choice is not shown with its reason")
	}
	p.tap("x11:off")
	if e.Final().X11 != firejail.X11Off || !p.ts.Exists(key("mark:x11")) {
		t.Errorf("Off was not picked: %q", e.Final().X11)
	}
}

func TestGoldenDesktopTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabDesktop)
	p.waitFor("the programs", func() bool {
		return gluttertest.StateOf[*desktopTabState](p.ts, gluttertest.ByType[*DesktopTab]()).programs != nil
	})
	// One change: the session bus is filtered, with the notifications allowed.
	p.tap("session-bus:filtered")
	p.tap("bus:notifications")
	p.golden("desktop-tab", "4.4-desktop.png")
}
