package ui

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Limits tab the user reads (SPEC §7.7).
const (
	limitsTitleText     = "Limits"
	limitsIntroText     = "How much of the computer the app may use. A switch that is off puts no limit."
	memoryOffText       = "Off: the app may use all the memory it can get."
	memoryOnText        = "The most memory the app can ask for. When it asks for more, it fails. Apps that map a lot of address space (browsers, Electron) fail at small limits."
	memoryNotNumberText = "Type a number, such as 512 or 1.5."
	memoryZeroText      = "The memory limit must be more than 0. Turn the switch off for no limit."
	memoryHugeText      = "That is more memory than firejail can count."
	cpuTitleText        = "CPU cores"
	cpuIntroText        = "The cores the app may run on. With none checked it may use all of them."
	niceOffText         = "Off: the app runs at the normal priority."
	niceOnText          = "From -20 to 19. A higher number gives the app less of the processor when others want it. Firejail turns a negative number into 0 for a user who is not root, which you are."
	niceNotNumberText   = "Type a whole number between -20 and 19."
	niceNegativeText    = "This number is negative: firejail will use 0 instead."
	timeoutOffText      = "Off: the app runs until it ends."
	timeoutOnText       = "Firejail stops the app after this long, as hours:minutes:seconds, under 100 hours."
	timeoutFormatText   = "Type the time as hh:mm:ss, for example 01:30:00. Minutes and seconds are below 60."
	timeoutZeroText     = "The time limit must be more than 0. Turn the switch off for no limit."
	filesOffText        = "Off: no limit on open files."
	filesOnText         = "The most files, sockets and pipes the app can have open at once."
	procsOffText        = "Off: no limit on processes."
	procsOnText         = "The most processes the app, and everything it starts, can have at once."
	whole0Text          = "Type a whole number more than 0. Turn the switch off for no limit."
	limitsFieldWidth    = 180.0
	cpuTileWidth        = 130.0
	maxCPUTiles         = 32
)

// Sizes of memory.
const (
	mebibyte = int64(1) << 20
	gibibyte = int64(1) << 30
)

// The units of the memory field.
const (
	unitMB = "MB"
	unitGB = "GB"
)

// memoryUnitFor is the unit a memory limit is shown in: GB when it is a whole number of
// them, MB otherwise.
func memoryUnitFor(bytes int64) string {
	if bytes >= gibibyte && bytes%gibibyte == 0 {
		return unitGB
	}
	return unitMB
}

// memoryText writes bytes as a number of the unit; an empty text for no limit.
func memoryText(bytes int64, unit string) string {
	if bytes <= 0 {
		return ""
	}
	size := mebibyte
	if unit == unitGB {
		size = gibibyte
	}
	return strconv.FormatFloat(float64(bytes)/float64(size), 'f', -1, 64)
}

// timeoutText writes a time limit as hh:mm:ss; an empty text for no limit.
func timeoutText(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	secs := int64(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", secs/3600, secs/60%60, secs%60)
}

// parseTimeout reads hh:mm:ss: three numbers separated by colons, the minutes and the
// seconds below 60. ok is false for any other text.
func parseTimeout(text string) (d time.Duration, ok bool) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) != 3 {
		return 0, false
	}
	var n [3]int64
	for i, p := range parts {
		if p == "" || len(p) > 4 || strings.Trim(p, "0123456789") != "" {
			return 0, false
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return 0, false
		}
		n[i] = v
	}
	if n[1] >= 60 || n[2] >= 60 {
		return 0, false
	}
	return time.Duration(n[0]*3600+n[1]*60+n[2]) * time.Second, true
}

// withCPU returns the cores with core checked or unchecked, in ascending order.
func withCPU(cores []int, core int, on bool) []int {
	out := slices.DeleteFunc(slices.Clone(cores), func(c int) bool { return c == core })
	if on {
		out = append(out, core)
	}
	slices.Sort(out)
	return out
}

// coreTiles is how many checkboxes the tab shows: the cores of the machine, at most the
// 32 firejail knows, and at least up to the highest core the settings name.
func coreTiles(machine int, chosen []int) int {
	n := max(machine, 1)
	for _, c := range chosen {
		n = max(n, c+1)
	}
	return min(n, maxCPUTiles)
}

// limitInput is the text field of a limit and why its text is not stored.
type limitInput struct {
	ctl *m.TextEditingController
	// problem is why the text cannot be stored, "" when it can.
	problem string
}

// limitSpec describes one limit that is off or a typed value.
type limitSpec struct {
	field   firejail.Field
	prefix  string
	offText string
	onText  string
	// on says whether the limit is set.
	on func(s firejail.Settings) bool
	// turnOn sets the limit to the value it starts with; turnOff removes it.
	turnOn  func(st *firejail.Settings)
	turnOff func(st *firejail.Settings)
	// show writes the limit as the field shows it.
	show func(s firejail.Settings) string
}

// LimitsTabOpts are the optional parameters of [NewLimitsTab].
type LimitsTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// LimitsTab is the Limits tab of the build editor (SPEC §7.7): memory, CPU cores,
// priority, time limit and the numbers of open files and processes. Text the user typed
// that is not valid is explained under its field and not stored. It edits an [Editor];
// its content scrolls when it is taller than the window.
type LimitsTab struct {
	m.StatefulWidget
	editor  *Editor
	backend Backend
	change  func(apply func())
}

// NewLimitsTab creates the tab for editor. backend tells how many cores the machine has.
// change runs a function that changes the editor inside the SetState of the page that
// shows the tab.
func NewLimitsTab(editor *Editor, backend Backend, change func(apply func()), opts ...LimitsTabOpts) *LimitsTab {
	var o LimitsTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &LimitsTab{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), editor: editor, backend: backend, change: change}
}

// CreateState creates the state of the tab.
func (w *LimitsTab) CreateState() m.State { return &limitsTabState{} }

type limitsTabState struct {
	m.StateOf[*LimitsTab]
	inputs map[firejail.Field]*limitInput
	// memUnit is the unit of the memory field.
	memUnit string
	scroll  *m.ScrollControllerBase
}

func (s *limitsTabState) InitState() {
	s.scroll = m.NewScrollController()
	s.inputs = map[firejail.Field]*limitInput{}
	final := s.Widget().editor.Final()
	s.memUnit = memoryUnitFor(final.MemoryBytes)
	for _, sp := range s.specs() {
		in := &limitInput{ctl: m.NewTextEditingController()}
		in.ctl.SetText(sp.show(final))
		s.inputs[sp.field] = in
	}
}

func (s *limitsTabState) Dispose() {
	s.scroll.Dispose()
	for _, in := range s.inputs {
		in.ctl.Dispose()
	}
}

// DidUpdateWidget puts the model's value in a field when something other than typing
// changed it (Reset to profile, another profile). A field whose text was refused keeps it.
func (s *limitsTabState) DidUpdateWidget(*LimitsTab) { s.sync() }

// sync rewrites the text of every field whose stored value its text no longer says.
func (s *limitsTabState) sync() {
	final := s.Widget().editor.Final()
	for _, sp := range s.specs() {
		in := s.inputs[sp.field]
		if in.problem != "" || !sp.on(final) {
			continue
		}
		if set, problem := s.parse(sp.field, in.ctl.Text()); problem == "" {
			applied := final
			set(&applied)
			if sp.show(applied) == sp.show(final) {
				continue
			}
		}
		if sp.field == firejail.FieldMemory {
			s.memUnit = memoryUnitFor(final.MemoryBytes)
		}
		if want := sp.show(final); in.ctl.Text() != want {
			in.ctl.SetText(want)
		}
	}
}

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *limitsTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

// specs are the limits that are off or a typed value, in the order of SPEC §7.7.
func (s *limitsTabState) specs() []limitSpec {
	return []limitSpec{
		{
			field: firejail.FieldMemory, prefix: "memory", offText: memoryOffText, onText: memoryOnText,
			on:      func(st firejail.Settings) bool { return st.MemoryBytes > 0 },
			turnOn:  func(st *firejail.Settings) { st.MemoryBytes = gibibyte },
			turnOff: func(st *firejail.Settings) { st.MemoryBytes = 0 },
			show:    func(st firejail.Settings) string { return memoryText(st.MemoryBytes, s.memUnit) },
		},
		{
			field: firejail.FieldNice, prefix: "nice", offText: niceOffText, onText: niceOnText,
			on:      func(st firejail.Settings) bool { return st.Nice != nil },
			turnOn:  func(st *firejail.Settings) { n := 10; st.Nice = &n },
			turnOff: func(st *firejail.Settings) { st.Nice = nil },
			show: func(st firejail.Settings) string {
				if st.Nice == nil {
					return ""
				}
				return strconv.Itoa(*st.Nice)
			},
		},
		{
			field: firejail.FieldTimeout, prefix: "timeout", offText: timeoutOffText, onText: timeoutOnText,
			on:      func(st firejail.Settings) bool { return st.Timeout > 0 },
			turnOn:  func(st *firejail.Settings) { st.Timeout = time.Hour },
			turnOff: func(st *firejail.Settings) { st.Timeout = 0 },
			show:    func(st firejail.Settings) string { return timeoutText(st.Timeout) },
		},
		{
			field: firejail.FieldMaxFiles, prefix: "max_files", offText: filesOffText, onText: filesOnText,
			on:      func(st firejail.Settings) bool { return st.MaxFiles > 0 },
			turnOn:  func(st *firejail.Settings) { st.MaxFiles = 1024 },
			turnOff: func(st *firejail.Settings) { st.MaxFiles = 0 },
			show:    func(st firejail.Settings) string { return strconv.FormatInt(max(st.MaxFiles, 0), 10) },
		},
		{
			field: firejail.FieldMaxProcs, prefix: "max_procs", offText: procsOffText, onText: procsOnText,
			on:      func(st firejail.Settings) bool { return st.MaxProcs > 0 },
			turnOn:  func(st *firejail.Settings) { st.MaxProcs = 256 },
			turnOff: func(st *firejail.Settings) { st.MaxProcs = 0 },
			show:    func(st firejail.Settings) string { return strconv.FormatInt(max(st.MaxProcs, 0), 10) },
		},
	}
}

// parse reads the text of a field. It returns the change that stores the value, or the
// message for the user when the text cannot be stored; for a value out of range the
// message is the one of [firejail.Validate].
func (s *limitsTabState) parse(f firejail.Field, text string) (set func(st *firejail.Settings), problem string) {
	e := s.Widget().editor
	text = strings.TrimSpace(text)
	checked := func(change func(st *firejail.Settings)) (func(st *firejail.Settings), string) {
		if msgs := candidateProblems(e, f, change); len(msgs) > 0 {
			return nil, msgs[0]
		}
		return change, ""
	}
	switch f {
	case firejail.FieldMemory:
		x, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, memoryNotNumberText
		}
		size := float64(mebibyte)
		if s.memUnit == unitGB {
			size = float64(gibibyte)
		}
		switch bytes := math.Round(x * size); {
		case x < 0:
			return checked(func(st *firejail.Settings) { st.MemoryBytes = -1 })
		case bytes < 1:
			return nil, memoryZeroText
		case bytes >= math.MaxInt64:
			return nil, memoryHugeText
		default:
			return checked(func(st *firejail.Settings) { st.MemoryBytes = int64(bytes) })
		}
	case firejail.FieldNice:
		n, err := strconv.Atoi(text)
		if err != nil {
			return nil, niceNotNumberText
		}
		return checked(func(st *firejail.Settings) { st.Nice = &n })
	case firejail.FieldTimeout:
		d, ok := parseTimeout(text)
		switch {
		case !ok:
			return nil, timeoutFormatText
		case d == 0:
			return nil, timeoutZeroText
		}
		return checked(func(st *firejail.Settings) { st.Timeout = d })
	default: // the two numbers
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n == 0 {
			return nil, whole0Text
		}
		return checked(func(st *firejail.Settings) {
			if f == firejail.FieldMaxFiles {
				st.MaxFiles = n
			} else {
				st.MaxProcs = n
			}
		})
	}
}

// typed handles the text of a field changing: a valid text is stored, an invalid one is
// explained and left out of the model.
func (s *limitsTabState) typed(f firejail.Field, text string) {
	in := s.inputs[f]
	set, problem := s.parse(f, text)
	if problem != "" {
		s.SetState(func() { in.problem = problem })
		return
	}
	s.apply(func(e *Editor) {
		in.problem = ""
		e.Set(set)
	})
}

// toggle turns a limit on with its starting value or off.
func (s *limitsTabState) toggle(sp limitSpec, on bool) {
	in := s.inputs[sp.field]
	s.apply(func(e *Editor) {
		in.problem = ""
		e.Set(func(st *firejail.Settings) {
			if !on {
				sp.turnOff(st)
				return
			}
			sp.turnOn(st)
		})
		if on {
			if sp.field == firejail.FieldMemory {
				s.memUnit = memoryUnitFor(e.Final().MemoryBytes)
			}
			in.ctl.SetText(sp.show(e.Final()))
		}
	})
}

func (s *limitsTabState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	e := w.editor
	final := e.Final()
	children := []m.Widget{sectionTitle(bc, limitsTitleText, nil), mutedText(bc, limitsIntroText)}
	specs := s.specs()
	children = append(children, s.limit(bc, e, final, specs[0])...) // memory
	children = append(children, s.cpuSection(bc, e, final)...)
	for _, sp := range specs[1:] {
		children = append(children, s.limit(bc, e, final, sp)...)
	}
	return tabList(firejail.TabLimits, s.scroll, children)
}

// limit is one limit: its switch and, when it is on, its field.
func (s *limitsTabState) limit(bc m.BuildContext, e *Editor, final firejail.Settings, sp limitSpec) []m.Widget {
	in := s.inputs[sp.field]
	on := sp.on(final)
	text := sp.offText
	if on {
		text = sp.onText
	}
	out := []m.Widget{m.NewSwitchListTile(on, func(v bool) { s.toggle(sp, v) }, m.SwitchListTileOpts{
		Key: m.NewValueKey("toggle:" + string(sp.field)),
		Title: markedTitle(settingLabel(sp.field), fieldMarkThen(e, sp.field, s.apply, func() {
			in.problem = ""
		})),
		Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(3), Overflow: m.TextOverflowEllipsis}),
		Dense:    m.Some(true),
	})}
	if !on {
		return out
	}
	hint := map[firejail.Field]string{firejail.FieldMemory: "512", firejail.FieldNice: "10", firejail.FieldTimeout: "01:30:00",
		firejail.FieldMaxFiles: "1024", firejail.FieldMaxProcs: "256"}[sp.field]
	d := m.NewInputDecoration(m.InputDecorationOpts{HintText: m.Some(hint), Border: m.NewOutlineInputBorder(), IsDense: m.Some(true)})
	row := []m.Widget{m.NewSizedBox(m.NewTextField(m.TextFieldOpts{
		Key: m.NewValueKey(sp.prefix + ":field"), Controller: in.ctl, Decoration: &d,
		OnChanged: func(t string) { s.typed(sp.field, t) },
	}), m.SizedBoxOpts{Width: m.Some(limitsFieldWidth)})}
	if sp.field == firejail.FieldMemory {
		for _, unit := range []string{unitMB, unitGB} {
			unit := unit
			row = append(row, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}), m.NewChoiceChip(m.NewText(unit), s.memUnit == unit, m.ChoiceChipOpts{
				Key:           m.NewValueKey("memory:unit:" + unit),
				ShowCheckmark: m.Some(false),
				OnSelected: func(bool) {
					s.SetState(func() { s.memUnit = unit })
					s.typed(firejail.FieldMemory, in.ctl.Text())
				},
			}))
		}
	}
	out = append(out, m.NewPadding(m.NewEdgeInsetsFromLTRB(16, 0, 4, 4), m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})))
	if in.problem != "" {
		out = append(out, problemLine(bc, in.problem, m.NewValueKey("problem:"+sp.prefix)))
	}
	if sp.field == firejail.FieldNice && final.Nice != nil && *final.Nice < 0 {
		out = append(out, m.NewPadding(m.NewEdgeInsetsFromLTRB(20, 0, 4, 4), warningText(bc, niceNegativeText, m.NewValueKey("warning:nice"))))
	}
	return out
}

// cpuSection is the CPU cores: a checkbox for each core of the machine.
func (s *limitsTabState) cpuSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	n := coreTiles(s.Widget().backend.CPUCount(), final.CPUs)
	tiles := make([]m.Widget, n)
	for i := range tiles {
		core := i
		tiles[i] = m.NewSizedBox(m.NewCheckboxListTile(m.Some(slices.Contains(final.CPUs, core)), func(v m.Opt[bool]) {
			if on, ok := v.Get(); ok {
				s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.CPUs = withCPU(st.CPUs, core, on) }) })
			}
		}, m.CheckboxListTileOpts{
			Key:             m.NewValueKey("cpu:" + strconv.Itoa(core)),
			Title:           m.NewText(strconv.Itoa(core)),
			Dense:           m.Some(true),
			ControlAffinity: m.ListTileControlAffinityLeading,
		}), m.SizedBoxOpts{Width: m.Some(cpuTileWidth)})
	}
	out := []m.Widget{
		sectionTitle(bc, cpuTitleText, fieldMark(e, firejail.FieldCPUs, s.apply)),
		mutedText(bc, cpuIntroText),
		m.NewWrap(tiles, m.WrapOpts{}),
	}
	for _, msg := range problemsOf(e, firejail.FieldCPUs) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}
