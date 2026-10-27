package ui

import (
	"slices"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Security tab the user reads (SPEC §7.5).
const (
	securityTitleText     = "Security"
	securityIntroText     = "The restrictions firejail puts on the app. A switch that is on makes the sandbox stricter."
	securityElectronNote  = "The profile you chose leaves most of these switches off. Electron and Chromium apps (VS Code, Chrome, Slack and the like) run a sandbox of their own, and some of these switches can stop them from starting. Press Try before you build."
	protocolsTitleText    = "Socket types"
	protocolsIntroText    = "The kinds of network socket the app may open."
	protocolsSwitchText   = "Restrict socket types"
	protocolsOffText      = "Not restricted: the app can open every kind of socket."
	protocolsOnText       = "The app can open only the kinds you check."
	protocolsLastText     = "At least one kind stays checked. Turn the switch off to allow every kind."
	securityElectronMin   = 4
	memoryDenyWarningText = "Breaks JIT engines: Electron, browsers, Java and VS Code may not start."
)

// securityToggles are the switches of the tab, in the order of SPEC §7.5 (the socket
// types come after the first five, and Landlock is last). A switch that is on is a
// restriction in force; the label of each is [settingLabel].
func securityToggles() []toggle {
	return []toggle{
		{firejail.FieldSeccomp, "Blocks the system calls that apps seldom need and attackers use.",
			func(s firejail.Settings) bool { return s.Seccomp },
			func(s *firejail.Settings, on bool) { s.Seccomp = on }},
		{firejail.FieldDropCaps, "The app runs without any of the special powers of root (capabilities).",
			func(s firejail.Settings) bool { return s.DropCaps },
			func(s *firejail.Settings, on bool) { s.DropCaps = on }},
		{firejail.FieldNoNewPrivs, "The app and the programs it starts can never gain more rights than they have.",
			func(s firejail.Settings) bool { return s.NoNewPrivs },
			func(s *firejail.Settings, on bool) { s.NoNewPrivs = on }},
		{firejail.FieldNoRoot, "The app sees no root user: it cannot become root even inside the sandbox.",
			func(s firejail.Settings) bool { return s.NoRoot },
			func(s *firejail.Settings, on bool) { s.NoRoot = on }},
		{firejail.FieldRestrictNamespaces, "The app cannot make new namespaces, which are the tools of other sandboxes.",
			func(s firejail.Settings) bool { return s.RestrictNamespaces },
			func(s *firejail.Settings, on bool) { s.RestrictNamespaces = on }},
		{firejail.FieldMemoryDenyWriteExecute, "The app cannot run memory it can also write, which stops some attacks.",
			func(s firejail.Settings) bool { return s.MemoryDenyWriteExecute },
			func(s *firejail.Settings, on bool) { s.MemoryDenyWriteExecute = on }},
		{firejail.FieldLandlock, "Adds the Landlock filter of the Linux kernel on the files the app may reach (needs kernel 5.13 or newer).",
			func(s firejail.Settings) bool { return s.Landlock },
			func(s *firejail.Settings, on bool) { s.Landlock = on }},
	}
}

// protocolChoices are the socket types of the checkboxes, in the order of SPEC §7.5.
func protocolChoices() []string {
	return []string{"unix", "inet", "inet6", "netlink", "packet", "bluetooth"}
}

// protocolText is what a socket type is, for the checkbox.
func protocolText(name string) string {
	switch name {
	case "unix":
		return "Local sockets between programs of this computer."
	case "inet":
		return "Internet connections (IPv4)."
	case "inet6":
		return "Internet connections (IPv6)."
	case "netlink":
		return "Talking to the kernel about the network."
	case "packet":
		return "Raw network packets."
	case "bluetooth":
		return "Bluetooth."
	}
	return ""
}

// defaultProtocols are the socket types a restriction starts with: the ones of firejail's
// default profile.
func defaultProtocols() []string { return []string{"unix", "inet", "inet6"} }

// withProtocol returns list with the socket type name checked or unchecked. The result is
// in the order of the base profile when it has the same kinds, and in the order of the
// checkboxes otherwise, so that checking a box again gives back what the profile had.
func withProtocol(list []string, base []string, name string, on bool) []string {
	set := slices.DeleteFunc(slices.Clone(list), func(p string) bool { return p == name })
	if on {
		set = append(set, name)
	}
	out := make([]string, 0, len(set))
	for _, p := range protocolChoices() {
		if slices.Contains(set, p) {
			out = append(out, p)
		}
	}
	for _, p := range set { // a kind the checkboxes do not know stays
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if len(out) == len(base) && !slices.ContainsFunc(out, func(p string) bool { return !slices.Contains(base, p) }) {
		return slices.Clone(base)
	}
	return out
}

// electronNote reports whether the base profile leaves most of the restrictions off, so
// that the tab warns about Electron and Chromium (SPEC §7.5).
func electronNote(base firejail.Settings) bool {
	off := 0
	for _, on := range []bool{base.Seccomp, base.DropCaps, base.NoNewPrivs, base.NoRoot, base.RestrictNamespaces, len(base.Protocols) > 0} {
		if !on {
			off++
		}
	}
	return off >= securityElectronMin
}

// SecurityTabOpts are the optional parameters of [NewSecurityTab].
type SecurityTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// SecurityTab is the Security tab of the build editor (SPEC §7.5): a switch for each
// restriction and the socket types. It edits an [Editor]; its content scrolls when it is
// taller than the window.
type SecurityTab struct {
	m.StatefulWidget
	editor *Editor
	change func(apply func())
}

// NewSecurityTab creates the tab for editor. change runs a function that changes the
// editor inside the SetState of the page that shows the tab.
func NewSecurityTab(editor *Editor, change func(apply func()), opts ...SecurityTabOpts) *SecurityTab {
	var o SecurityTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &SecurityTab{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), editor: editor, change: change}
}

// CreateState creates the state of the tab.
func (w *SecurityTab) CreateState() m.State { return &securityTabState{} }

type securityTabState struct {
	m.StateOf[*SecurityTab]
	scroll *m.ScrollControllerBase
}

func (s *securityTabState) InitState() { s.scroll = m.NewScrollController() }

func (s *securityTabState) Dispose() { s.scroll.Dispose() }

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *securityTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

func (s *securityTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	children := []m.Widget{sectionTitle(bc, securityTitleText, nil), mutedText(bc, securityIntroText)}
	if electronNote(e.Base().Settings) {
		children = append(children, infoNote(bc, securityElectronNote, m.NewValueKey("note:electron")))
	}
	children = append(children, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}))
	for _, t := range securityToggles() {
		t := t
		var subtitle m.Widget = m.NewText(t.text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis})
		if t.field == firejail.FieldMemoryDenyWriteExecute {
			subtitle = m.NewColumn([]m.Widget{
				subtitle, warningText(bc, memoryDenyWarningText, m.NewValueKey("warning:"+string(t.field))),
			}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart})
		}
		children = append(children, m.NewSwitchListTile(t.get(final), func(on bool) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { t.set(st, on) }) })
		}, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(t.field)),
			Title:    markedTitle(settingLabel(t.field), fieldMark(e, t.field, s.apply)),
			Subtitle: subtitle,
			Dense:    m.Some(true),
		}))
	}
	children = append(children, s.protocolsSection(bc, e, final)...)
	return tabList(firejail.TabSecurity, s.scroll, children)
}

// protocolsSection is the socket types: a switch for the restriction and, when it is on,
// a checkbox for each kind.
func (s *securityTabState) protocolsSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	restricted := len(final.Protocols) > 0
	text := protocolsOffText
	if restricted {
		text = protocolsOnText
	}
	out := []m.Widget{
		sectionTitle(bc, protocolsTitleText, fieldMark(e, firejail.FieldProtocols, s.apply)),
		mutedText(bc, protocolsIntroText),
		m.NewSwitchListTile(restricted, func(on bool) {
			s.apply(func(e *Editor) {
				e.Set(func(st *firejail.Settings) {
					if on {
						st.Protocols = defaultProtocols()
					} else {
						st.Protocols = nil
					}
				})
			})
		}, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(firejail.FieldProtocols)),
			Title:    m.NewText(protocolsSwitchText),
			Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:    m.Some(true),
		}),
	}
	if !restricted {
		return out
	}
	for _, name := range protocolChoices() {
		name := name
		checked := slices.Contains(final.Protocols, name)
		var onChanged func(m.Opt[bool])
		subtitle := protocolText(name)
		if checked && len(final.Protocols) == 1 {
			subtitle = protocolsLastText
		} else {
			onChanged = func(v m.Opt[bool]) {
				if on, ok := v.Get(); ok {
					s.apply(func(e *Editor) {
						e.Set(func(st *firejail.Settings) {
							st.Protocols = withProtocol(st.Protocols, e.Base().Settings.Protocols, name, on)
						})
					})
				}
			}
		}
		out = append(out, m.NewCheckboxListTile(m.Some(checked), onChanged, m.CheckboxListTileOpts{
			Key:             m.NewValueKey("protocol:" + name),
			Title:           m.NewText(name),
			Subtitle:        m.NewText(subtitle, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:           m.Some(true),
			ControlAffinity: m.ListTileControlAffinityLeading,
		}))
	}
	for _, msg := range problemsOf(e, firejail.FieldProtocols) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}
