package ui

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/rm4n0s/glutter/gluttertest"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// limitsPage opens the Limits tab of a new build of the app on a machine with cpus cores.
func limitsPage(t *testing.T, app string, cpus int) *page {
	t.Helper()
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds, f.cpus = nil, cpus })
	p := newPage(t, backend)
	p.ts.SetViewSize(gui.Size{Width: 1100, Height: 2400})
	p.ready()
	p.openApp(app)
	p.selectTab(firejail.TabLimits)
	return p
}

// typeInto replaces the text of the field and draws the answer.
func (p *page) typeInto(k, text string) {
	p.t.Helper()
	p.ts.EnterText(key(k), text)
	p.ts.Pump()
}

func TestLimitsTabTypedLimits(t *testing.T) {
	tests := []struct {
		field     firejail.Field
		prefix    string
		defText   string
		validText string
		on        func(s firejail.Settings) bool
		stored    func(s firejail.Settings) any
		want      any
		bad       []struct{ text, message string }
	}{
		{firejail.FieldMemory, "memory", "1", "512",
			func(s firejail.Settings) bool { return s.MemoryBytes > 0 }, func(s firejail.Settings) any { return s.MemoryBytes }, int64(512 << 30),
			[]struct{ text, message string }{{"abc", "Type a number"}, {"", "Type a number"}, {"0", "more than 0"}, {"-5", "negative"}, {"1e30", "more memory"}, {"1,5", "Type a number"}}},
		{firejail.FieldNice, "nice", "10", "5",
			func(s firejail.Settings) bool { return s.Nice != nil }, func(s firejail.Settings) any {
				if s.Nice == nil {
					return nil
				}
				return *s.Nice
			}, 5,
			[]struct{ text, message string }{{"abc", "whole number"}, {"", "whole number"}, {"20", "between -20 and 19"}, {"-21", "between -20 and 19"}, {"1.5", "whole number"}}},
		{firejail.FieldTimeout, "timeout", "01:00:00", "02:30:15",
			func(s firejail.Settings) bool { return s.Timeout > 0 }, func(s firejail.Settings) any { return s.Timeout }, 2*time.Hour + 30*time.Minute + 15*time.Second,
			[]struct{ text, message string }{{"abc", "hh:mm:ss"}, {"", "hh:mm:ss"}, {"1:30", "hh:mm:ss"}, {"00:61:00", "hh:mm:ss"}, {"00:00:60", "hh:mm:ss"}, {"00:00:00", "more than 0"}, {"100:00:00", "under 100 hours"}, {"-1:00:00", "hh:mm:ss"}}},
		{firejail.FieldMaxFiles, "max_files", "1024", "4096",
			func(s firejail.Settings) bool { return s.MaxFiles > 0 }, func(s firejail.Settings) any { return s.MaxFiles }, int64(4096),
			[]struct{ text, message string }{{"abc", "whole number"}, {"", "whole number"}, {"0", "more than 0"}, {"-3", "negative"}, {"1.5", "whole number"}}},
		{firejail.FieldMaxProcs, "max_procs", "256", "100",
			func(s firejail.Settings) bool { return s.MaxProcs > 0 }, func(s firejail.Settings) any { return s.MaxProcs }, int64(100),
			[]struct{ text, message string }{{"abc", "whole number"}, {"0", "more than 0"}, {"-3", "negative"}}},
	}
	for _, tt := range tests {
		t.Run(string(tt.field), func(t *testing.T) {
			p := limitsPage(t, "Calculator", 4)
			e := p.home().editor
			sw, mark, field := "toggle:"+string(tt.field), "mark:"+string(tt.field), tt.prefix+":field"
			if !p.ts.Exists(gluttertest.Descendant(key(sw), gluttertest.ByText(settingLabel(tt.field)))) {
				t.Fatalf("no switch labelled %q", settingLabel(tt.field))
			}
			if p.switchedOn(sw) || p.ts.Exists(key(field)) || tt.on(e.Final()) || p.dots() != 0 {
				t.Fatal("a fresh build is not off")
			}

			p.tap(sw)
			if !p.switchedOn(sw) || !tt.on(e.Final()) || !e.Changed(tt.field) || !p.ts.Exists(key(mark)) || !p.tabDot(firejail.TabLimits) {
				t.Fatal("the switch did not turn the limit on")
			}
			if got := p.textOf(field); got != tt.defText {
				t.Errorf("the field starts with %q, want %q", got, tt.defText)
			}

			p.typeInto(field, tt.validText)
			if got := tt.stored(e.Final()); got != tt.want {
				t.Fatalf("after typing %q the setting is %v, want %v", tt.validText, got, tt.want)
			}
			if p.ts.Exists(key("problem:" + tt.prefix)) {
				t.Error("a valid text shows a message")
			}

			for _, bad := range tt.bad {
				p.typeInto(field, bad.text)
				if !p.problemShows("problem:"+tt.prefix, bad.message) {
					t.Errorf("%q: no message with %q", bad.text, bad.message)
				}
				if got := tt.stored(e.Final()); got != tt.want {
					t.Errorf("%q was stored: %v", bad.text, got)
				}
				if p.textOf(field) != bad.text {
					t.Errorf("the field lost the text %q", bad.text)
				}
			}
			// A valid text clears the message and is stored.
			p.typeInto(field, tt.validText)
			if p.ts.Exists(key("problem:"+tt.prefix)) || tt.stored(e.Final()) != tt.want {
				t.Error("a valid text after a wrong one is not stored")
			}

			// A refused text does not survive Reset to profile.
			p.typeInto(field, "abc")
			p.reset(mark)
			if tt.on(e.Final()) || e.Changed(tt.field) || p.ts.Exists(key(field)) || p.ts.Exists(key("problem:"+tt.prefix)) || p.switchedOn(sw) || p.tabDot(firejail.TabLimits) {
				t.Fatal("Reset did not turn the limit off")
			}
			p.tap(sw)
			if got := p.textOf(field); got != tt.defText {
				t.Errorf("after Reset and on again the field has %q, want %q", got, tt.defText)
			}

			// Off removes it, with a refused text too.
			p.typeInto(field, "abc")
			p.tap(sw)
			if tt.on(e.Final()) || p.ts.Exists(key(field)) || p.ts.Exists(key("problem:"+tt.prefix)) || e.HasChanges() {
				t.Error("the switch did not turn the limit off")
			}
		})
	}
}

func TestLimitsTabMemoryUnits(t *testing.T) {
	p := limitsPage(t, "Calculator", 4)
	e := p.home().editor
	// chip says whether the chip of the unit is the selected one: the tab keeps the unit.
	chip := func(unit string) bool {
		p.t.Helper()
		if !p.ts.Exists(key("memory:unit:" + unit)) {
			p.t.Fatalf("no chip for %s", unit)
		}
		return gluttertest.StateOf[*limitsTabState](p.ts, gluttertest.ByType[*LimitsTab]()).memUnit == unit
	}
	p.tap("toggle:memory")
	if e.Final().MemoryBytes != 1<<30 || p.textOf("memory:field") != "1" || !chip("GB") || chip("MB") {
		t.Fatalf("on: %d bytes, text %q", e.Final().MemoryBytes, p.textOf("memory:field"))
	}
	p.tap("memory:unit:MB")
	if e.Final().MemoryBytes != 1<<20 || !chip("MB") || chip("GB") {
		t.Errorf("1 MB is %d bytes", e.Final().MemoryBytes)
	}
	p.typeInto("memory:field", "512")
	if e.Final().MemoryBytes != 512<<20 {
		t.Errorf("512 MB is %d bytes", e.Final().MemoryBytes)
	}
	p.tap("memory:unit:GB")
	if e.Final().MemoryBytes != 512<<30 || p.textOf("memory:field") != "512" {
		t.Errorf("512 GB is %d bytes", e.Final().MemoryBytes)
	}
	p.typeInto("memory:field", "1.5")
	if e.Final().MemoryBytes != 3<<29 {
		t.Errorf("1.5 GB is %d bytes", e.Final().MemoryBytes)
	}
	// 2048 MB stays in MB while the user is in it, though it is 2 GB.
	p.tap("memory:unit:MB")
	p.typeInto("memory:field", "2048")
	if e.Final().MemoryBytes != 2<<30 || !chip("MB") || p.textOf("memory:field") != "2048" {
		t.Errorf("2048 MB: %d bytes, MB chip %v, text %q", e.Final().MemoryBytes, chip("MB"), p.textOf("memory:field"))
	}
	// A unit with a text that is wrong shows the message and stores nothing.
	p.typeInto("memory:field", "x")
	p.tap("memory:unit:GB")
	if e.Final().MemoryBytes != 2<<30 || !p.problemShows("problem:memory", "Type a number") {
		t.Errorf("a wrong text with a new unit: %d bytes", e.Final().MemoryBytes)
	}
}

func TestLimitsTabFollowsTheSettings(t *testing.T) {
	// A value that something other than typing set (a profile, Reset) is put in the field.
	p := limitsPage(t, "Calculator", 4)
	e := p.home().editor
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.MemoryBytes, s.Timeout, s.MaxFiles, s.MaxProcs = 3<<30, 90*time.Minute, 2048, 64
			n := -3
			s.Nice = &n
		})
	})
	for k, want := range map[string]string{
		"memory:field": "3", "timeout:field": "01:30:00", "max_files:field": "2048", "max_procs:field": "64", "nice:field": "-3",
	} {
		if got := p.textOf(k); got != want {
			t.Errorf("%s shows %q, want %q", k, got, want)
		}
	}
	if !p.ts.Exists(key("warning:nice")) || !p.switchedOn("toggle:memory") {
		t.Error("the negative priority is not warned about")
	}
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) { s.MemoryBytes, s.Timeout = 1536<<20, 2*time.Hour })
	})
	if p.textOf("memory:field") != "1536" || p.textOf("timeout:field") != "02:00:00" {
		t.Errorf("memory %q, timeout %q", p.textOf("memory:field"), p.textOf("timeout:field"))
	}
	if !e.Changed(firejail.FieldNice) {
		t.Error("no change")
	}
}

func TestLimitsTabNiceNote(t *testing.T) {
	p := limitsPage(t, "Calculator", 4)
	p.tap("toggle:nice")
	if !p.ts.Exists(gluttertest.ByTextContaining("negative number into 0")) {
		t.Error("the field does not say that firejail turns a negative value into 0")
	}
	if p.ts.Exists(key("warning:nice")) {
		t.Error("a positive priority shows the warning")
	}
	p.typeInto("nice:field", "-5")
	if !p.ts.Exists(key("warning:nice")) {
		t.Error("a negative priority does not show the warning")
	}
	// Zero is a limit that is set, not off.
	p.typeInto("nice:field", "0")
	e := p.home().editor
	if e.Final().Nice == nil || *e.Final().Nice != 0 || !e.Changed(firejail.FieldNice) || p.ts.Exists(key("warning:nice")) {
		t.Errorf("Nice = %v", e.Final().Nice)
	}
	for _, edge := range []struct {
		text string
		want int
	}{{"-20", -20}, {"19", 19}} {
		p.typeInto("nice:field", edge.text)
		if e.Final().Nice == nil || *e.Final().Nice != edge.want {
			t.Errorf("%s: Nice = %v", edge.text, e.Final().Nice)
		}
	}
}

func TestLimitsTabCPUs(t *testing.T) {
	tests := []struct {
		name   string
		cpus   int
		chosen []int
		shown  int
	}{
		{"the cores of the machine", 8, nil, 8},
		{"one core", 1, nil, 1},
		{"no answer", 0, nil, 1},
		{"more than firejail knows", 40, nil, 32},
		{"a core the machine lacks", 2, []int{5}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := limitsPage(t, "Calculator", tt.cpus)
			if len(tt.chosen) > 0 {
				p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.CPUs = tt.chosen }) })
			}
			n := 0
			for i := range 40 {
				if p.ts.Exists(key("cpu:" + strconv.Itoa(i))) {
					n++
				}
			}
			if n != tt.shown {
				t.Errorf("%d checkboxes, want %d", n, tt.shown)
			}
			if tt.shown < 32 && p.ts.Exists(key("cpu:"+strconv.Itoa(tt.shown))) {
				t.Errorf("core %d is shown", tt.shown)
			}
			for _, c := range tt.chosen {
				if !p.checked("cpu:" + strconv.Itoa(c)) {
					t.Errorf("core %d is not checked", c)
				}
			}
		})
	}
}

func TestLimitsTabCPUChecks(t *testing.T) {
	p := limitsPage(t, "Calculator", 8)
	e := p.home().editor
	if e.Final().CPUs != nil || p.dots() != 0 {
		t.Fatal("a fresh build has cores")
	}
	for i := range 8 {
		if p.checked("cpu:" + strconv.Itoa(i)) {
			t.Fatalf("core %d starts checked", i)
		}
	}
	p.tap("cpu:" + strconv.Itoa(5))
	p.tap("cpu:" + strconv.Itoa(1))
	p.tap("cpu:" + strconv.Itoa(7))
	if want := []int{1, 5, 7}; !slices.Equal(e.Final().CPUs, want) {
		t.Fatalf("CPUs = %v, want %v", e.Final().CPUs, want)
	}
	if !p.ts.Exists(key("mark:cpus")) || !p.tabDot(firejail.TabLimits) || !p.checked("cpu:"+strconv.Itoa(5)) {
		t.Error("the cores are not shown as a change")
	}
	p.tap("cpu:" + strconv.Itoa(5))
	if want := []int{1, 7}; !slices.Equal(e.Final().CPUs, want) || p.checked("cpu:"+strconv.Itoa(5)) {
		t.Errorf("CPUs = %v, want %v", e.Final().CPUs, want)
	}
	p.reset("mark:cpus")
	if len(e.Final().CPUs) != 0 || e.Changed(firejail.FieldCPUs) || p.checked("cpu:"+strconv.Itoa(1)) {
		t.Errorf("Reset left %v", e.Final().CPUs)
	}
	// Unchecking every box is off again, not a change.
	p.tap("cpu:" + strconv.Itoa(3))
	p.tap("cpu:" + strconv.Itoa(3))
	if e.Changed(firejail.FieldCPUs) {
		t.Error("check and uncheck left a change")
	}
}

func TestMemoryText(t *testing.T) {
	tests := []struct {
		bytes int64
		unit  string
		want  string
	}{
		{0, unitMB, ""},
		{1 << 30, unitGB, "1"},
		{512 << 20, unitMB, "512"},
		{3 << 29, unitGB, "1.5"},
		{3 << 29, unitMB, "1536"},
		{2 << 30, unitMB, "2048"},
		{2 << 30, unitGB, "2"},
	}
	for _, tt := range tests {
		if got := memoryText(tt.bytes, tt.unit); got != tt.want {
			t.Errorf("memoryText(%d, %s) = %q, want %q", tt.bytes, tt.unit, got, tt.want)
		}
	}
	units := []struct {
		bytes int64
		want  string
	}{{0, unitMB}, {512 << 20, unitMB}, {1 << 30, unitGB}, {3 << 29, unitMB}, {2 << 30, unitGB}, {1<<30 + 1, unitMB}}
	for _, tt := range units {
		if got := memoryUnitFor(tt.bytes); got != tt.want {
			t.Errorf("memoryUnitFor(%d) = %s, want %s", tt.bytes, got, tt.want)
		}
	}
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		text string
		want time.Duration
		ok   bool
	}{
		{"01:30:00", 90 * time.Minute, true},
		{"00:00:01", time.Second, true},
		{"1:2:3", time.Hour + 2*time.Minute + 3*time.Second, true},
		{" 99:59:59 ", 99*time.Hour + 59*time.Minute + 59*time.Second, true},
		{"100:00:00", 100 * time.Hour, true},
		{"00:60:00", 0, false},
		{"00:00:60", 0, false},
		{"01:30", 0, false},
		{"::", 0, false},
		{"a:b:c", 0, false},
		{"-1:00:00", 0, false},
		{"+1:00:00", 0, false},
		{"", 0, false},
		{"12345:00:00", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseTimeout(tt.text)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseTimeout(%q) = %v, %v; want %v, %v", tt.text, got, ok, tt.want, tt.ok)
		}
	}
	for _, d := range []time.Duration{time.Second, 90 * time.Minute, 99*time.Hour + time.Minute} {
		if back, ok := parseTimeout(timeoutText(d)); !ok || back != d {
			t.Errorf("%v does not survive %q", d, timeoutText(d))
		}
	}
	if timeoutText(0) != "" || timeoutText(-time.Second) != "" {
		t.Error("no limit is not written as a time")
	}
}

func TestWithCPUAndCoreTiles(t *testing.T) {
	if got := withCPU([]int{5, 1}, 3, true); !slices.Equal(got, []int{1, 3, 5}) {
		t.Errorf("withCPU = %v", got)
	}
	if got := withCPU([]int{1, 3, 5}, 3, false); !slices.Equal(got, []int{1, 5}) {
		t.Errorf("withCPU = %v", got)
	}
	cores := []int{1, 3}
	_ = withCPU(cores, 0, true)
	if !slices.Equal(cores, []int{1, 3}) {
		t.Errorf("withCPU changed its argument: %v", cores)
	}
	tests := []struct {
		machine int
		chosen  []int
		want    int
	}{{8, nil, 8}, {0, nil, 1}, {64, nil, 32}, {2, []int{9}, 10}, {2, []int{31}, 32}, {40, []int{31}, 32}}
	for _, tt := range tests {
		if got := coreTiles(tt.machine, tt.chosen); got != tt.want {
			t.Errorf("coreTiles(%d, %v) = %d, want %d", tt.machine, tt.chosen, got, tt.want)
		}
	}
}

func TestGoldenLimitsTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(firejail.TabLimits)
	// One change: the memory is limited to 2 GB.
	p.tap("toggle:" + string(firejail.FieldMemory))
	p.typeInto("memory:field", "2")
	p.golden("limits-tab", "4.5-limits.png")
}
