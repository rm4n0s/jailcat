package ui

import (
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Devices tab the user reads (SPEC §7.3).
const (
	devicesTitleText = "Devices"
	devicesIntroText = "The hardware the app may use. A checked box is allowed; an unchecked one is blocked."
)

// deviceToggles are the checkboxes of the tab, in the order of SPEC §7.3. A checked box
// is a device the app may use; the label of each is [settingLabel].
func deviceToggles() []toggle {
	return []toggle{
		{firejail.FieldSound, "The app can play sound and use the microphone. Firejail cannot allow one without the other.",
			func(s firejail.Settings) bool { return s.Sound },
			func(s *firejail.Settings, on bool) { s.Sound = on }},
		{firejail.FieldCamera, "The app can use webcams.",
			func(s firejail.Settings) bool { return s.Camera },
			func(s *firejail.Settings, on bool) { s.Camera = on }},
		{firejail.FieldGPU, "The app can use the graphics card for 3D and video acceleration.",
			func(s firejail.Settings) bool { return s.GPU },
			func(s *firejail.Settings, on bool) { s.GPU = on }},
		{firejail.FieldPrinters, "The app can use printers.",
			func(s firejail.Settings) bool { return s.Printers },
			func(s *firejail.Settings, on bool) { s.Printers = on }},
		{firejail.FieldU2F, "The app can use U2F and FIDO security keys.",
			func(s firejail.Settings) bool { return s.U2F },
			func(s *firejail.Settings, on bool) { s.U2F = on }},
		{firejail.FieldInput, "The app can use game controllers and other raw input devices.",
			func(s firejail.Settings) bool { return s.Input },
			func(s *firejail.Settings, on bool) { s.Input = on }},
		{firejail.FieldDVD, "The app can use CD and DVD drives.",
			func(s firejail.Settings) bool { return s.DVD },
			func(s *firejail.Settings, on bool) { s.DVD = on }},
		{firejail.FieldTV, "The app can use TV and radio tuners.",
			func(s firejail.Settings) bool { return s.TV },
			func(s *firejail.Settings, on bool) { s.TV = on }},
		{firejail.FieldTPM, "The app can use the trusted platform module (TPM) of the computer.",
			func(s firejail.Settings) bool { return s.TPM },
			func(s *firejail.Settings, on bool) { s.TPM = on }},
	}
}

// DevicesTabOpts are the optional parameters of [NewDevicesTab].
type DevicesTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// DevicesTab is the Devices tab of the build editor (SPEC §7.3): one checkbox for each
// kind of device, checked when the app may use it. It edits an [Editor].
type DevicesTab struct {
	m.StatefulWidget
	editor *Editor
	change func(apply func())
}

// NewDevicesTab creates the tab for editor. change runs a function that changes the
// editor inside the SetState of the page that shows the tab.
func NewDevicesTab(editor *Editor, change func(apply func()), opts ...DevicesTabOpts) *DevicesTab {
	var o DevicesTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &DevicesTab{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), editor: editor, change: change}
}

// CreateState creates the state of the tab.
func (w *DevicesTab) CreateState() m.State { return &devicesTabState{} }

type devicesTabState struct {
	m.StateOf[*DevicesTab]
	scroll *m.ScrollControllerBase
}

func (s *devicesTabState) InitState() { s.scroll = m.NewScrollController() }

func (s *devicesTabState) Dispose() { s.scroll.Dispose() }

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *devicesTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

func (s *devicesTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	children := []m.Widget{sectionTitle(bc, devicesTitleText, nil), mutedText(bc, devicesIntroText), m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)})}
	for _, t := range deviceToggles() {
		t := t
		on := t.get(final)
		children = append(children, m.NewCheckboxListTile(m.Some(on), func(v m.Opt[bool]) {
			if allowed, ok := v.Get(); ok {
				s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { t.set(st, allowed) }) })
			}
		}, m.CheckboxListTileOpts{
			Key:             m.NewValueKey("toggle:" + string(t.field)),
			Title:           markedTitle(settingLabel(t.field), fieldMark(e, t.field, s.apply)),
			Subtitle:        m.NewText(t.text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:           m.Some(true),
			ControlAffinity: m.ListTileControlAffinityLeading,
		}))
	}
	return tabList(firejail.TabDevices, s.scroll, children)
}
