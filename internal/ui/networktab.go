package ui

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
	"strings"

	"github.com/rm4n0s/glutter/desktop"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Network tab the user reads (SPEC §7.2).
const (
	networkTitleText   = "Network"
	networkIntroText   = "Whether the app can reach the network and the internet."
	networkFullTitle   = "Full"
	networkFullText    = "The app uses the network of your computer, like any other program"
	networkNoneTitle   = "None"
	networkNoneText    = "The app has no network at all: it cannot connect anywhere"
	networkOffReason   = "Not available while the network is None: the app has no network to use it on."
	dnsHintText        = "The servers the app asks for the addresses of websites. With none, it uses the ones of your system."
	dnsFieldLabel      = "DNS server"
	dnsFieldHint       = "1.1.1.1 or 2606:4700:4700::1111"
	dnsFullText        = "Four servers are the most firejail uses. Remove one to add another."
	hostsHintText      = "A file the app reads in place of /etc/hosts. With none, it reads the one of your system."
	hostsFieldLabel    = "File"
	hostsFieldHint     = "/home/me/hosts"
	hostsChooseText    = "Choose…"
	hostsClearText     = "Clear"
	hostsChooserTitle  = "Choose a hosts file"
	hostsChooserAccept = "Use"
	hostsChooserFailed = "The file chooser of your desktop could not be opened. Type the path instead."
	firewallText       = "Firejail's default client filter (netfilter): the app can connect out, nothing can connect in to it. Firejail applies it only when the app has a network of its own."
	maxDNSServers      = 4
	dnsDuplicateText   = "That server is in the list already."
	dnsEmptyText       = "No servers: the app uses the ones of your system."
	dnsNotAddressText  = " is not an IP address. Type an IPv4 or IPv6 address."
)

// checkDNS is the [StringListCheck] of the DNS list: the text must be an IP address that
// is not in the list yet.
func checkDNS(text string, items []string) (item, problem string) {
	text = strings.TrimSpace(text)
	addr, err := netip.ParseAddr(text)
	if err != nil || addr.Zone() != "" {
		return "", "“" + text + "”" + dnsNotAddressText
	}
	item = addr.String()
	if slices.Contains(items, item) {
		return "", dnsDuplicateText
	}
	return item, ""
}

// NetworkTabOpts are the optional parameters of [NewNetworkTab].
type NetworkTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// NetworkTab is the Network tab of the build editor (SPEC §7.2): Full or None, the DNS
// servers, the hosts file and the firewall. The last three are disabled while the network
// is None. It edits an [Editor]; its content scrolls when it is taller than the window.
type NetworkTab struct {
	m.StatefulWidget
	editor *Editor
	logger *slog.Logger
	change func(apply func())
}

// NewNetworkTab creates the tab for editor. change runs a function that changes the
// editor (or the tab) inside the SetState of the page that shows the tab, and may be
// called from any goroutine. logger gets what the user must not read.
func NewNetworkTab(editor *Editor, logger *slog.Logger, change func(apply func()), opts ...NetworkTabOpts) *NetworkTab {
	var o NetworkTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &NetworkTab{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		editor:         editor, logger: logger, change: change,
	}
}

// CreateState creates the state of the tab.
func (w *NetworkTab) CreateState() m.State { return &networkTabState{} }

type networkTabState struct {
	m.StateOf[*NetworkTab]
	// hosts is the text of the hosts file field.
	hosts *m.TextEditingController
	// message is why the chooser failed; "" for none.
	message string
	// choosing is true while the file chooser is open.
	choosing bool
	scroll   *m.ScrollControllerBase
}

func (s *networkTabState) InitState() {
	s.scroll = m.NewScrollController()
	s.hosts = m.NewTextEditingController()
	s.hosts.SetText(s.Widget().editor.Final().HostsFile)
}

func (s *networkTabState) Dispose() {
	s.scroll.Dispose()
	s.hosts.Dispose()
}

// DidUpdateWidget puts the model's hosts file in the field when something other than
// typing changed it: the chooser, Clear, Reset to profile or another profile.
func (s *networkTabState) DidUpdateWidget(*NetworkTab) {
	if want := s.Widget().editor.Final().HostsFile; s.hosts.Text() != want {
		s.hosts.SetText(want)
	}
}

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *networkTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

// chooseHosts asks for the hosts file with the chooser of the desktop.
func (s *networkTabState) chooseHosts() {
	if s.choosing {
		return
	}
	w := s.Widget()
	bc := s.Context()
	w.change(func() { s.choosing, s.message = true, "" })
	s.Go(func(ctx context.Context) {
		res, err := desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{
			Title: hostsChooserTitle, AcceptLabel: hostsChooserAccept,
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.Error("the file chooser failed", "err", err)
			w.change(func() { s.choosing, s.message = false, hostsChooserFailed })
			return
		}
		w.change(func() {
			s.choosing = false
			if !res.Cancelled && len(res.Paths) > 0 {
				w.editor.Set(func(st *firejail.Settings) { st.HostsFile = res.Paths[0] })
			}
		})
	})
}

func (s *networkTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	off := final.Network == firejail.NetNone
	children := []m.Widget{
		s.networkSection(bc, e, final),
		s.dnsSection(bc, e, final, off),
		s.hostsSection(bc, e, final, off),
		s.firewallSection(bc, e, final, off),
	}
	return tabList(firejail.TabNetwork, s.scroll, children)
}

// networkSection is the choice between the network of the computer and none.
func (s *networkTabState) networkSection(bc m.BuildContext, e *Editor, final firejail.Settings) m.Widget {
	tile := func(mode firejail.NetMode, title, text string) m.Widget {
		return radioTile(bc, "network:"+string(mode), mode, title, text, final.Network == mode, true)
	}
	group := choiceGroup(final.Network,
		tileRow(tile(firejail.NetFull, networkFullTitle, networkFullText), tile(firejail.NetNone, networkNoneTitle, networkNoneText)),
		func(mode firejail.NetMode) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.Network = mode }) })
		})
	return m.NewColumn([]m.Widget{
		sectionTitle(bc, networkTitleText, fieldMark(e, firejail.FieldNetwork, s.apply)),
		mutedText(bc, networkIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		group,
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// explanation is the muted line under a heading: the hint, or the reason the group is
// disabled.
func explanation(bc m.BuildContext, hint string, off bool, key string) m.Widget {
	if off {
		return mutedText(bc, networkOffReason, m.TextOpts{Key: m.NewValueKey("reason:" + key)})
	}
	return mutedText(bc, hint)
}

// dnsSection is the list of DNS servers.
func (s *networkTabState) dnsSection(bc m.BuildContext, e *Editor, final firejail.Settings, off bool) m.Widget {
	col := []m.Widget{
		sectionTitle(bc, settingLabel(firejail.FieldDNS), fieldMark(e, firejail.FieldDNS, s.apply)),
		explanation(bc, dnsHintText, off, "dns"),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}),
		NewStringList(final.DNS, checkDNS,
			func(item string) {
				s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.DNS = append(slices.Clone(st.DNS), item) }) })
			},
			func(i int) {
				s.apply(func(e *Editor) {
					e.Set(func(st *firejail.Settings) { st.DNS = slices.Delete(slices.Clone(st.DNS), i, i+1) })
				})
			},
			StringListOpts{
				KeyPrefix: "dns", Label: dnsFieldLabel, Hint: dnsFieldHint, Empty: dnsEmptyText,
				Max: maxDNSServers, FullText: dnsFullText, Disabled: off,
			}),
	}
	for _, msg := range problemsOf(e, firejail.FieldDNS) {
		col = append(col, problemLine(bc, msg, nil))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// hostsSection is the hosts file: a field, the chooser and Clear.
func (s *networkTabState) hostsSection(bc m.BuildContext, e *Editor, final firejail.Settings, off bool) m.Widget {
	decoration := m.NewInputDecoration(m.InputDecorationOpts{
		HintText:  m.Some(hostsFieldHint),
		LabelText: m.Some(hostsFieldLabel),
		Border:    m.NewOutlineInputBorder(),
		IsDense:   m.Some(true),
	})
	field := m.NewTextField(m.TextFieldOpts{
		Key:        m.NewValueKey("hosts:field"),
		Controller: s.hosts,
		Decoration: &decoration,
		Enabled:    m.Some(!off),
		OnChanged: func(text string) {
			if text != e.Final().HostsFile {
				s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.HostsFile = text }) })
			}
		},
	})
	var choose, clear func()
	if !off {
		choose = s.chooseHosts
		if final.HostsFile != "" {
			clear = func() { s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.HostsFile = "" }) }) }
		}
	}
	col := []m.Widget{
		sectionTitle(bc, settingLabel(firejail.FieldHostsFile), fieldMark(e, firejail.FieldHostsFile, s.apply)),
		explanation(bc, hostsHintText, off, "hosts"),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
			m.NewExpanded(field),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewOutlinedButton(choose, m.NewText(hostsChooseText), m.OutlinedButtonOpts{Key: m.NewValueKey("hosts:choose")}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewTextButton(clear, m.NewText(hostsClearText), m.TextButtonOpts{Key: m.NewValueKey("hosts:clear")}),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})),
	}
	if s.message != "" {
		col = append(col, notice(bc, s.message))
	}
	for _, msg := range problemsOf(e, firejail.FieldHostsFile) {
		col = append(col, problemLine(bc, msg, m.NewValueKey("problem:hosts")))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// firewallSection is the switch for firejail's default client filter.
func (s *networkTabState) firewallSection(bc m.BuildContext, e *Editor, final firejail.Settings, off bool) m.Widget {
	text, textKey := firewallText, m.Key(nil)
	var onChanged func(bool)
	if off {
		text, textKey = networkOffReason, m.NewValueKey("reason:firewall")
	} else {
		onChanged = func(v bool) { s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.Firewall = v }) }) }
	}
	return m.NewColumn([]m.Widget{
		m.NewSwitchListTile(final.Firewall, onChanged, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(firejail.FieldFirewall)),
			Title:    markedTitle(settingLabel(firejail.FieldFirewall), fieldMark(e, firejail.FieldFirewall, s.apply)),
			Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(3), Overflow: m.TextOverflowEllipsis, Key: textKey}),
			Dense:    m.Some(true),
		}),
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}
