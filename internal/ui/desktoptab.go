package ui

import (
	"context"
	"slices"
	"strings"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Desktop tab the user reads (SPEC §7.4).
const (
	sessionTitleText    = "Session D-Bus"
	sessionIntroText    = "The message bus that apps use to talk to your desktop: notifications, the file chooser, the keyring and more."
	sessionFullText     = "The app can talk to everything on your desktop"
	sessionFilteredText = "The app can talk only to the names you choose below"
	sessionNoneText     = "The app has no session bus; notifications, portals and the keyring do not work"
	sessionPickText     = "Choose Filtered to pick the names the app may use."
	talkTitleText       = "Names the app may talk to"
	ownTitleText        = "Names the app may own"
	talkMoreLabel       = "More names to talk to"
	ownMoreLabel        = "More names to own"
	busNameHint         = "org.example.App or org.example.*"
	talkEmptyText       = "No more names."
	systemTitleText     = "System D-Bus"
	systemIntroText     = "The bus for the services of the system: power, network and more."
	systemFullText      = "The app can talk to the services of the system"
	systemNoneText      = "The app has no system bus"
	systemFilteredText  = "Filtered by the profile"
	systemFilteredNote  = "The profile filters the system bus (dbus-system filter). Jailcat cannot edit that filter; choose Full or None to replace it."
	x11TitleText        = "X11 isolation"
	x11IntroText        = "Gives the app an X11 display of its own, so it cannot read other windows or what you type in them."
	x11NetNoteText      = "Xpra, Xephyr and Xvfb need a network namespace of their own to hide the X11 abstract socket (man firejail, --x11). Jailcat does not set one up."
	x11OffText          = "No isolation: the app shares the X11 display with your other programs"
	x11XpraText         = "The app runs on its own display, which Xpra shows on yours"
	x11XephyrText       = "The app runs on its own display, shown in a window by Xephyr"
	x11XvfbText         = "The app runs on a virtual display that is not shown anywhere"
	x11LookingText      = "Looking for the program…"
)

// busCheckbox describes one checkbox of the Filtered session bus (SPEC §7.4).
type busCheckbox struct {
	key   string
	title string
	text  string
	name  string
	// own is true for a dbus-user.own name; false for a dbus-user.talk one.
	own bool
}

// busCheckboxes are the checkboxes of SPEC §7.4 for the app whose binary name is binary.
func busCheckboxes(binary string) []busCheckbox {
	return []busCheckbox{
		{"notifications", "Notifications", "The app can show desktop notifications.", firejail.DBusNotifications, false},
		{"portals", "File chooser and other portals", "The app can use the file chooser of your desktop, and the other portals (opening links, printing).", firejail.DBusPortals, false},
		{"keyring", "Keyring (Secret Service)", "The app can read and store passwords in your keyring.", firejail.DBusKeyring, false},
		{"tray", "Tray icon", "The app can show an icon in the system tray.", firejail.DBusTray, false},
		{"media-keys", "Media keys (MPRIS)", "The media keys of your keyboard can control the app.", firejail.MPRISName(binary), true},
	}
}

// busNameProblem checks a D-Bus name the user typed, and returns why it cannot be used,
// or "" when it can. A name has at least two parts separated by dots; a part is made of
// letters, digits, "_" and "-" and does not start with a digit; the last part may be "*"
// (every name that starts with the others).
func busNameProblem(name string) string {
	switch {
	case name == "":
		return "Type a D-Bus name, such as org.example.App."
	case strings.ContainsAny(name, " \t\r\n"):
		return "A D-Bus name has no blanks in it."
	case len(name) > 255:
		return "A D-Bus name has at most 255 characters."
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return "A D-Bus name has at least two parts separated by dots, such as org.example.App."
	}
	for i, part := range parts {
		switch {
		case part == "":
			return "A D-Bus name has no empty part: look for two dots in a row, or a dot at either end."
		case part == "*":
			if i != len(parts)-1 {
				return "The * can only be the last part of a D-Bus name, such as org.example.*."
			}
		case part[0] >= '0' && part[0] <= '9':
			return "A part of a D-Bus name cannot start with a digit: " + part + "."
		case strings.IndexFunc(part, func(r rune) bool { return !isBusNameRune(r) }) >= 0:
			return "A part of a D-Bus name has only letters, digits, “_” and “-”: " + part + "."
		}
	}
	return ""
}

// isBusNameRune reports whether r may be in a part of a D-Bus name.
func isBusNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
}

// busNameCheck returns the [StringListCheck] of a list of names: the text must be a valid
// name that is not allowed yet. all is every name of the setting, the checkboxes' too.
func busNameCheck(all, boxes []string) StringListCheck {
	return func(text string, _ []string) (item, problem string) {
		name := strings.TrimSpace(text)
		if problem := busNameProblem(name); problem != "" {
			return "", problem
		}
		switch {
		case slices.Contains(boxes, name):
			return "", "That name is one of the checkboxes above."
		case slices.Contains(all, name):
			return "", "That name is in the list already."
		}
		return name, ""
	}
}

// setBusName turns a D-Bus name on or off in the talk list or, with own, the own list.
func setBusName(st *firejail.Settings, own bool, name string, on bool) {
	if own {
		st.SessionOwn = firejail.WithName(st.SessionOwn, name, on)
		return
	}
	st.SessionTalk = firejail.WithName(st.SessionTalk, name, on)
}

// x11Choice describes one X11 isolation choice.
type x11Choice struct {
	mode    firejail.X11Mode
	title   string
	text    string
	program string
}

// x11Choices are the choices of SPEC §7.4. The program of Off is empty: it needs none.
func x11Choices() []x11Choice {
	return []x11Choice{
		{firejail.X11Off, "Off", x11OffText, ""},
		{firejail.X11Xpra, "Xpra", x11XpraText, "xpra"},
		{firejail.X11Xephyr, "Xephyr", x11XephyrText, "Xephyr"},
		{firejail.X11Xvfb, "Xvfb", x11XvfbText, "Xvfb"},
	}
}

// x11UnavailableText is why an isolation choice cannot be picked.
func x11UnavailableText(program string) string {
	return "Not available: " + program + " is not installed (it is in none of the folders of your PATH)."
}

// DesktopTabOpts are the optional parameters of [NewDesktopTab].
type DesktopTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// DesktopTab is the Desktop tab of the build editor (SPEC §7.4): the session D-Bus with
// the names of the Filtered choice, the system D-Bus and the X11 isolation. The programs
// the X11 choices need are looked for off the UI thread when the tab opens. It edits an
// [Editor]; its content scrolls when it is taller than the window.
type DesktopTab struct {
	m.StatefulWidget
	editor  *Editor
	backend Backend
	change  func(apply func())
}

// NewDesktopTab creates the tab for editor. backend answers which programs are installed.
// change runs a function that changes the editor (or the tab) inside the SetState of the
// page that shows the tab, and may be called from any goroutine.
func NewDesktopTab(editor *Editor, backend Backend, change func(apply func()), opts ...DesktopTabOpts) *DesktopTab {
	var o DesktopTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &DesktopTab{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		editor:         editor, backend: backend, change: change,
	}
}

// CreateState creates the state of the tab.
func (w *DesktopTab) CreateState() m.State { return &desktopTabState{} }

type desktopTabState struct {
	m.StateOf[*DesktopTab]
	// programs says, for each program of the X11 choices, whether it is installed; nil
	// until the search has answered.
	programs map[string]bool
	scroll   *m.ScrollControllerBase
}

func (s *desktopTabState) InitState() {
	s.scroll = m.NewScrollController()
	w := s.Widget()
	s.Go(func(ctx context.Context) {
		found := map[string]bool{}
		for _, c := range x11Choices() {
			if c.program != "" {
				found[c.program] = w.backend.ProgramOnPath(ctx, c.program)
			}
		}
		if ctx.Err() != nil {
			return
		}
		w.change(func() { s.programs = found })
	})
}

func (s *desktopTabState) Dispose() { s.scroll.Dispose() }

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *desktopTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

func (s *desktopTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	children := s.sessionSection(bc, e, final)
	children = append(children, s.systemSection(bc, e, final)...)
	children = append(children, s.x11Section(bc, e, final)...)
	return tabList(firejail.TabDesktop, s.scroll, children)
}

// sessionSection is the session D-Bus: the choice and, when it is Filtered, the names.
func (s *desktopTabState) sessionSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	tile := func(mode firejail.BusMode, title, text string) m.Widget {
		return radioTile(bc, "session-bus:"+string(mode), mode, title, text, final.SessionBus == mode, true)
	}
	group := choiceGroup(final.SessionBus,
		tileRow(tile(firejail.BusFull, "Full", sessionFullText), tile(firejail.BusFiltered, "Filtered", sessionFilteredText), tile(firejail.BusNone, "None", sessionNoneText)),
		func(mode firejail.BusMode) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.SessionBus = mode }) })
		})
	out := []m.Widget{
		sectionTitle(bc, sessionTitleText, fieldMark(e, firejail.FieldSessionBus, s.apply)),
		mutedText(bc, sessionIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		group,
	}
	if final.SessionBus != firejail.BusFiltered {
		return append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(8, 4), mutedText(bc, sessionPickText, m.TextOpts{Key: m.NewValueKey("session-pick")})))
	}
	out = append(out, s.namesSection(bc, e, final, false)...)
	return append(out, s.namesSection(bc, e, final, true)...)
}

// namesSection is one of the two halves of the Filtered choice: the names the app may
// talk to (own false) or own (own true), as checkboxes and a list of more names.
func (s *desktopTabState) namesSection(bc m.BuildContext, e *Editor, final firejail.Settings, own bool) []m.Widget {
	title, label, field, all, prefix := talkTitleText, talkMoreLabel, firejail.FieldSessionTalk, final.SessionTalk, "talk"
	if own {
		title, label, field, all, prefix = ownTitleText, ownMoreLabel, firejail.FieldSessionOwn, final.SessionOwn, "own"
	}
	out := []m.Widget{subTitle(bc, title, fieldMark(e, field, s.apply))}

	var boxes []string
	for _, c := range busCheckboxes(e.App().BinaryName) {
		if c.own != own {
			continue
		}
		c := c
		boxes = append(boxes, c.name)
		out = append(out, m.NewCheckboxListTile(m.Some(slices.Contains(all, c.name)), func(v m.Opt[bool]) {
			on, ok := v.Get()
			if !ok {
				return
			}
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { setBusName(st, own, c.name, on) }) })
		}, m.CheckboxListTileOpts{
			Key:             m.NewValueKey("bus:" + c.key),
			Title:           m.NewText(c.title),
			Subtitle:        m.NewText(c.text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:           m.Some(true),
			ControlAffinity: m.ListTileControlAffinityLeading,
		}))
	}

	var more []string
	for _, name := range all {
		if !slices.Contains(boxes, name) {
			more = append(more, name)
		}
	}
	out = append(out, NewStringList(more, busNameCheck(all, boxes),
		func(name string) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { setBusName(st, own, name, true) }) })
		},
		func(i int) {
			name := more[i]
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { setBusName(st, own, name, false) }) })
		},
		StringListOpts{KeyPrefix: prefix, Label: label, Hint: busNameHint, Empty: talkEmptyText}))
	for _, msg := range problemsOf(e, field) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}

// systemSection is the system D-Bus: Full or None. A base profile that filters it shows a
// third, read-only choice.
func (s *desktopTabState) systemSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	tile := func(mode firejail.BusMode, title, text string, enabled bool) m.Widget {
		return radioTile(bc, "system-bus:"+string(mode), mode, title, text, final.SystemBus == mode, enabled)
	}
	tiles := []m.Widget{tile(firejail.BusFull, "Full", systemFullText, true), tile(firejail.BusNone, "None", systemNoneText, true)}
	filtered := final.SystemBus == firejail.BusFiltered || e.Base().Settings.SystemBus == firejail.BusFiltered
	if filtered {
		tiles = append(tiles, tile(firejail.BusFiltered, "Filtered", systemFilteredText, false))
	}
	group := choiceGroup(final.SystemBus, tileRow(tiles...),
		func(mode firejail.BusMode) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.SystemBus = mode }) })
		})
	out := []m.Widget{
		sectionTitle(bc, systemTitleText, fieldMark(e, firejail.FieldSystemBus, s.apply)),
		mutedText(bc, systemIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		group,
	}
	if filtered {
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(8, 4), mutedText(bc, systemFilteredNote, m.TextOpts{Key: m.NewValueKey("note:system-bus")})))
	}
	return out
}

// x11Section is the X11 isolation: Off and the programs that are installed.
func (s *desktopTabState) x11Section(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	var tiles []m.Widget
	for _, c := range x11Choices() {
		text, enabled := c.text, true
		if c.program != "" {
			switch {
			case s.programs == nil:
				text, enabled = x11LookingText, false
			case !s.programs[c.program]:
				text, enabled = x11UnavailableText(c.program), false
			}
		}
		tiles = append(tiles, radioTile(bc, "x11:"+string(c.mode), c.mode, c.title, text, final.X11 == c.mode, enabled))
	}
	group := choiceGroup(final.X11,
		m.NewColumn([]m.Widget{tileRow(tiles[0], tiles[1]), tileRow(tiles[2], tiles[3])}),
		func(mode firejail.X11Mode) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.X11 = mode }) })
		})
	out := []m.Widget{
		sectionTitle(bc, x11TitleText, fieldMark(e, firejail.FieldX11, s.apply)),
		mutedText(bc, x11IntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		group,
		m.NewPadding(m.NewEdgeInsetsSymmetric(8, 4), mutedText(bc, x11NetNoteText, m.TextOpts{Key: m.NewValueKey("note:x11-network")})),
	}
	for _, msg := range problemsOf(e, firejail.FieldX11) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}
