package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

func TestNetworkTabChoice(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	for _, text := range []string{networkTitleText, networkFullText, networkNoneText} {
		if !p.ts.Exists(gluttertest.ByText(text)) {
			t.Errorf("the Network tab lacks %q", text)
		}
	}
	if e.Final().Network != firejail.NetFull || p.dots() != 0 || p.ts.Exists(key("mark:network")) {
		t.Fatal("a fresh build shows a network change")
	}

	p.tap("network:none")
	if e.Final().Network != firejail.NetNone || !e.Changed(firejail.FieldNetwork) {
		t.Fatalf("network = %q", e.Final().Network)
	}
	if !p.tabDot(firejail.TabNetwork) || !p.ts.Exists(key("mark:network")) {
		t.Error("no dot on the tab or no mark next to the choice")
	}

	p.reset("mark:network")
	if e.Final().Network != firejail.NetFull || e.Changed(firejail.FieldNetwork) || p.tabDot(firejail.TabNetwork) || p.ts.Exists(key("mark:network")) {
		t.Errorf("Reset left the network %q", e.Final().Network)
	}

	// Tapping the choice that is already selected changes nothing.
	p.tap("network:full")
	if e.HasChanges() {
		t.Error("tapping the selected choice made a change")
	}
}

func TestNetworkTabDisabledWhileNone(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	fd := p.ts.FakeDesktop()
	for _, k := range []string{"reason:dns", "reason:hosts", "reason:firewall"} {
		if p.ts.Exists(key(k)) {
			t.Errorf("%s is shown while the network is full", k)
		}
	}
	if !p.pressable("hosts:choose") || !p.pressable("dns:add") {
		t.Error("the buttons are disabled while the network is full")
	}

	p.tap("network:none")
	for _, k := range []string{"reason:dns", "reason:hosts", "reason:firewall"} {
		if !p.ts.Exists(key(k)) {
			t.Errorf("%s: the reason is not shown", k)
		}
	}
	if got := p.ts.Count(gluttertest.ByText(networkOffReason)); got != 3 {
		t.Errorf("the reason is shown %d times, want 3", got)
	}
	if p.pressable("hosts:choose") || p.pressable("dns:add") {
		t.Error("a button is enabled while the network is None")
	}
	before := e.Final().Firewall
	p.tap("hosts:choose")
	p.tap("toggle:" + string(firejail.FieldFirewall))
	if fd.PendingFileChoosers() != 0 || e.Final().Firewall != before {
		t.Error("a disabled control worked")
	}

	p.tap("network:full")
	if p.ts.Exists(gluttertest.ByText(networkOffReason)) || !p.pressable("hosts:choose") {
		t.Error("the controls stay disabled after the network is back")
	}
}

func TestNetworkTabDNS(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	add := func(text string) {
		p.t.Helper()
		p.ts.EnterText(key("dns:field"), text)
		p.ts.Pump()
		p.tap("dns:add")
	}
	if !p.ts.Exists(gluttertest.ByText(dnsEmptyText)) {
		t.Error("an empty list does not say so")
	}

	add("  1.1.1.1 ")
	add("2606:4700:4700:0:0:0:0:1111")
	if want := []string{"1.1.1.1", "2606:4700:4700::1111"}; !slices.Equal(e.Final().DNS, want) {
		t.Fatalf("DNS = %q, want %q", e.Final().DNS, want)
	}
	if !p.ts.Exists(key("dns:row:1")) || !p.tabDot(firejail.TabNetwork) || !p.ts.Exists(key("mark:dns")) {
		t.Error("the rows, the dot or the mark are missing")
	}

	tests := []struct{ text, message string }{
		{"dns.example.com", "is not an IP address"},
		{"8.8.8", "is not an IP address"},
		{"fe80::1%eth0", "is not an IP address"},
		{"1.1.1.1", "in the list already"},
	}
	for _, tt := range tests {
		add(tt.text)
		if !p.ts.Exists(gluttertest.Descendant(key("dns:problem"), gluttertest.ByTextContaining(tt.message))) {
			t.Errorf("%q: no message with %q", tt.text, tt.message)
		}
		p.ts.EnterText(key("dns:field"), "x")
		p.ts.Pump()
		if p.ts.Exists(key("dns:problem")) {
			t.Errorf("%q: the message stays after typing", tt.text)
		}
	}
	if len(e.Final().DNS) != 2 {
		t.Errorf("a refused text was added: %q", e.Final().DNS)
	}
	if problems := problemsOf(e, firejail.FieldDNS); len(problems) != 0 {
		t.Errorf("the list has problems: %q", problems)
	}

	// Four are the most; then the field is replaced by the reason.
	p.ts.EnterText(key("dns:field"), "")
	add("9.9.9.9")
	add("8.8.8.8")
	if len(e.Final().DNS) != 4 || p.ts.Exists(key("dns:field")) || !p.ts.Exists(gluttertest.ByText(dnsFullText)) {
		t.Fatalf("DNS = %q, field shown %v", e.Final().DNS, p.ts.Exists(key("dns:field")))
	}

	p.tap("dns:remove:0")
	if want := []string{"2606:4700:4700::1111", "9.9.9.9", "8.8.8.8"}; !slices.Equal(e.Final().DNS, want) {
		t.Errorf("after the removal DNS = %q, want %q", e.Final().DNS, want)
	}
	if !p.ts.Exists(key("dns:field")) {
		t.Error("the field does not come back after a removal")
	}

	p.reset("mark:dns")
	if len(e.Final().DNS) != 0 || e.Changed(firejail.FieldDNS) || p.tabDot(firejail.TabNetwork) {
		t.Errorf("Reset left DNS %q", e.Final().DNS)
	}
}

func TestNetworkTabHostsFileChooser(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	fd := p.ts.FakeDesktop()
	if p.pressable("hosts:clear") {
		t.Error("Clear is enabled with no file")
	}

	p.tap("hosts:choose")
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser("/srv/hosts")
	p.waitFor("the hosts file", func() bool { return e.Final().HostsFile == "/srv/hosts" })
	if p.textOf("hosts:field") != "/srv/hosts" || !p.ts.Exists(key("mark:hosts_file")) || !p.tabDot(firejail.TabNetwork) {
		t.Errorf("field %q, mark %v, dot %v", p.textOf("hosts:field"), p.ts.Exists(key("mark:hosts_file")), p.tabDot(firejail.TabNetwork))
	}

	// Clear empties the file and the field.
	p.tap("hosts:clear")
	if e.Final().HostsFile != "" || p.textOf("hosts:field") != "" || p.ts.Exists(key("mark:hosts_file")) {
		t.Errorf("Clear left %q in the model and %q in the field", e.Final().HostsFile, p.textOf("hosts:field"))
	}

	// Cancelled: nothing changes.
	p.tap("hosts:choose")
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.CancelFileChooser()
	p.waitFor("the chooser to end", func() bool {
		return !gluttertest.StateOf[*networkTabState](p.ts, gluttertest.ByType[*NetworkTab]()).choosing
	})
	if e.Final().HostsFile != "" {
		t.Error("a cancelled chooser set the file")
	}

	// Reset puts the base back and empties the field.
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.HostsFile = "/srv/other" }) })
	if p.textOf("hosts:field") != "/srv/other" {
		t.Fatalf("the field shows %q after a change from outside", p.textOf("hosts:field"))
	}
	p.reset("mark:hosts_file")
	if e.Final().HostsFile != "" || p.textOf("hosts:field") != "" {
		t.Errorf("Reset left %q in the model and %q in the field", e.Final().HostsFile, p.textOf("hosts:field"))
	}

	// A chooser that fails: the message, and the error in the log.
	fd.DisableFileChooser()
	p.tap("hosts:choose")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByText(hostsChooserFailed)) })
	if !strings.Contains(p.log.String(), "FileChooser") {
		t.Errorf("the failure is not in the log: %s", p.log.String())
	}
}

func TestNetworkTabHostsFileTyped(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	p.ts.EnterText(key("hosts:field"), "/srv/hosts")
	p.ts.Pump()
	if e.Final().HostsFile != "/srv/hosts" || !e.Changed(firejail.FieldHostsFile) || p.ts.Exists(key("problem:hosts")) {
		t.Fatalf("typing set %q", e.Final().HostsFile)
	}
	if p.textOf("hosts:field") != "/srv/hosts" {
		t.Errorf("the field shows %q", p.textOf("hosts:field"))
	}

	// A path that is not a full path shows the message of Validate.
	p.ts.EnterText(key("hosts:field"), "hosts")
	p.ts.Pump()
	var message string
	for _, pr := range e.Problems() {
		if pr.Field == firejail.FieldHostsFile {
			message = pr.Message
		}
	}
	if message == "" || !p.ts.Exists(gluttertest.Descendant(key("problem:hosts"), gluttertest.ByText(message))) {
		t.Errorf("the problem %q is not shown", message)
	}

	// Emptying the field is back to the system's file.
	p.ts.EnterText(key("hosts:field"), "")
	p.ts.Pump()
	if e.Final().HostsFile != "" || e.Changed(firejail.FieldHostsFile) || p.ts.Exists(key("problem:hosts")) {
		t.Error("an empty field is still a change")
	}
}

func TestNetworkTabFirewall(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	if !p.ts.Exists(gluttertest.ByTextContaining("default client filter")) {
		t.Error("the firewall is not explained")
	}
	// The Default profile turns the firewall on (netfilter), so the switch starts on.
	base := e.Base().Settings.Firewall
	if e.Final().Firewall != base || p.ts.Exists(key("mark:firewall")) {
		t.Fatal("a fresh build shows a firewall change")
	}
	p.tap("toggle:" + string(firejail.FieldFirewall))
	if p.switchedOn("toggle:"+string(firejail.FieldFirewall)) == base || e.Final().Firewall == base || !p.tabDot(firejail.TabNetwork) || !p.ts.Exists(key("mark:firewall")) {
		t.Fatal("the switch did not change the firewall")
	}
	p.reset("mark:firewall")
	if e.Final().Firewall != base || p.ts.Exists(key("mark:firewall")) {
		t.Error("Reset did not put the firewall back")
	}
}

func TestNetworkTabShowsAProfilesValues(t *testing.T) {
	p := tabPage(t, "Calculator", firejail.TabNetwork)
	e := p.home().editor
	// A base that sets DNS shows its servers without a mark; removing one is a change.
	e.SwitchProfile(e.BaseProfile(), withBase(e, func(s *firejail.Settings) { s.DNS = []string{"9.9.9.9"} }), e.Profile(), true)
	p.change(func(*Editor) {})
	if !p.ts.Exists(key("dns:row:0")) || p.ts.Exists(key("mark:dns")) || p.tabDot(firejail.TabNetwork) {
		t.Fatal("the profile's server is not shown as the base")
	}
	p.tap("dns:remove:0")
	if len(e.Final().DNS) != 0 || !p.ts.Exists(key("mark:dns")) {
		t.Error("removing the profile's server is not a change")
	}
	p.reset("mark:dns")
	if !slices.Equal(e.Final().DNS, []string{"9.9.9.9"}) {
		t.Errorf("Reset gave %q", e.Final().DNS)
	}
}

// withBase returns the base values of e with change applied to the settings.
func withBase(e *Editor, change func(s *firejail.Settings)) firejail.Base {
	b := e.Base()
	s := b.Settings.Clone()
	change(&s)
	b.Settings = s
	return b
}

func TestGoldenNetworkTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabNetwork)
	// One change: a DNS server.
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.DNS = []string{"1.1.1.1"} }) })
	p.golden("network-tab", "4.4-network.png")
}
