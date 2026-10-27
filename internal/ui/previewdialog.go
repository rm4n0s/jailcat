package ui

import (
	"strconv"
	"time"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Preview dialog (SPEC §11.1).
const (
	previewTitleText       = "Preview"
	previewProfileTab      = "Profile"
	previewLauncherTab     = "Launcher"
	previewDesktopTab      = "Desktop entry"
	previewCloseText       = "Close"
	previewNoDesktopText   = "No desktop entry is written: Install in the menu is off for this build."
	previewLegendSetting   = "from your settings"
	previewLegendRewritten = "written again, so an ignore line cannot skip it"
	previewLegendIgnore    = "undoes a line of the base profile"
	previewLegendExtra     = "from your Extra rules"
	previewDialogWidth     = 780.0
	previewDialogHeight    = 440.0
)

// PreviewDialog shows what Build would write (SPEC §11.1): read-only tabs for the profile,
// the launcher script and the desktop entry. In the profile, the lines that come from the
// user's settings and Extra rules are highlighted. It pops with nothing.
type PreviewDialog struct {
	m.StatefulWidget
	prepared *PreparedBuild
}

// NewPreviewDialog creates the dialog for a prepared build.
func NewPreviewDialog(prepared *PreparedBuild, opts ...m.WidgetOpts) *PreviewDialog {
	return &PreviewDialog{StatefulWidget: m.NewStatefulWidgetBase(opts...), prepared: prepared}
}

// CreateState creates the state of the dialog.
func (w *PreviewDialog) CreateState() m.State { return &previewDialogState{} }

type previewDialogState struct {
	m.StateOf[*PreviewDialog]
	tab    int
	scroll *m.ScrollControllerBase
}

func (s *previewDialogState) InitState() { s.scroll = m.NewScrollController() }

func (s *previewDialogState) Dispose() { s.scroll.Dispose() }

// previewTabs are the titles of the tabs, in order.
func previewTabs() []string {
	return []string{previewProfileTab, previewLauncherTab, previewDesktopTab}
}

// highlight is the background and text color of a kind of profile line; ok is false for
// the kinds that are not highlighted.
func highlight(scheme *m.ColorScheme, kind firejail.LineKind) (background, foreground m.Color, ok bool) {
	switch kind {
	case firejail.LineSetting:
		return scheme.TertiaryContainer(), scheme.OnTertiaryContainer(), true
	case firejail.LineRewritten:
		return scheme.SecondaryContainer(), scheme.OnSecondaryContainer(), true
	case firejail.LineIgnore:
		return scheme.ErrorContainer(), scheme.OnErrorContainer(), true
	case firejail.LineExtra:
		return scheme.PrimaryContainer(), scheme.OnPrimaryContainer(), true
	}
	return m.Color{}, m.Color{}, false
}

func (s *previewDialogState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)

	titles := previewTabs()
	var tabWidgets []m.Widget
	for _, t := range titles {
		tabWidgets = append(tabWidgets, m.NewTab(m.TabOpts{Key: m.NewValueKey("preview:tab:" + t), Child: m.NewText(t)}))
	}
	bar := m.NewDefaultTabController(len(titles), m.NewTabBar(tabWidgets, m.TabBarOpts{
		TabAlignment: m.TabAlignmentStart, IsScrollable: true,
		OnTap: func(i int) { s.SetState(func() { s.tab = i }) },
	}), m.DefaultTabControllerOpts{InitialIndex: s.tab, AnimationDuration: m.Some(time.Duration(0))})

	children := []m.Widget{}
	if reason := w.prepared.Prepared.IconFallback; reason != "" {
		children = append(children, m.NewPadding(m.NewEdgeInsetsSymmetric(0, 0),
			infoNote(bc, iconFallbackPrefix+reason+".", m.NewValueKey("preview:icon-fallback"))))
	}
	children = append(children, bar)
	if s.tab == 0 {
		children = append(children, s.legend(bc))
	}
	children = append(children, m.NewExpanded(s.body(bc)))

	return m.NewAlertDialog(m.AlertDialogOpts{
		Title: m.NewText(previewTitleText+": "+w.prepared.Request.Settings.SandboxName, m.TextOpts{Style: theme.TextTheme.TitleLarge}),
		Content: m.NewSizedBox(m.NewColumn(children, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}),
			m.SizedBoxOpts{Width: m.Some(previewDialogWidth), Height: m.Some(previewDialogHeight)}),
		Actions: []m.Widget{
			m.NewFilledButton(func() { m.NavigatorPop(bc) }, m.NewText(previewCloseText), m.FilledButtonOpts{Key: m.NewValueKey("preview:close")}),
		},
	})
}

// legend explains the colors of the profile tab, only for the kinds the profile has.
func (s *previewDialogState) legend(bc m.BuildContext) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	present := map[firejail.LineKind]bool{}
	for _, l := range s.Widget().prepared.Generated.Lines {
		present[l.Kind] = true
	}
	var chips []m.Widget
	for _, k := range []struct {
		kind firejail.LineKind
		text string
	}{
		{firejail.LineSetting, previewLegendSetting}, {firejail.LineRewritten, previewLegendRewritten},
		{firejail.LineIgnore, previewLegendIgnore}, {firejail.LineExtra, previewLegendExtra},
	} {
		if !present[k.kind] {
			continue
		}
		bg, fg, _ := highlight(scheme, k.kind)
		chips = append(chips, badge(bc, k.text, bg, fg))
	}
	if len(chips) == 0 {
		return m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)})
	}
	return m.NewPadding(m.NewEdgeInsetsSymmetric(8, 0), m.NewWrap(chips, m.WrapOpts{Spacing: 6, RunSpacing: 4}))
}

// body is the text of the selected tab.
func (s *previewDialogState) body(bc m.BuildContext) m.Widget {
	w := s.Widget()
	scheme := m.ThemeOf(bc).ColorScheme
	texts := w.prepared.Prepared.Preview()
	var lines []previewLine
	switch s.tab {
	case 0:
		for _, l := range w.prepared.Generated.Lines {
			lines = append(lines, previewLine{text: l.Text, kind: l.Kind})
		}
	case 1:
		lines = plainLines(texts.Launcher)
	default:
		if texts.Desktop == "" {
			return m.NewPadding(m.NewEdgeInsetsAll(12), mutedText(bc, previewNoDesktopText, m.TextOpts{Key: m.NewValueKey("preview:no-desktop")}))
		}
		lines = plainLines(texts.Desktop)
	}
	style := monoStyle(bc, scheme.OnSurface())
	list := m.NewListViewBuilder(func(bc m.BuildContext, i int) m.Widget {
		if i >= len(lines) {
			return nil
		}
		return previewRow(bc, lines[i], style, i)
	}, m.ListViewOpts{
		Key: m.NewValueKey("preview:lines:" + strconv.Itoa(s.tab)), ItemCount: m.Some(len(lines)),
		Controller: s.scroll, Padding: m.NewEdgeInsetsSymmetric(4, 0),
	})
	return m.NewContainer(m.NewScrollbar(list, m.ScrollbarOpts{Controller: s.scroll}), m.ContainerOpts{
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color:        m.Some(scheme.SurfaceContainerLowest()),
			Border:       m.NewBorderAll(m.BorderSideOpts{Color: m.Some(scheme.OutlineVariant())}),
			BorderRadius: m.NewBorderRadiusCircular(6),
		}),
	})
}

// previewLine is one line of a tab.
type previewLine struct {
	text string
	kind firejail.LineKind
}

// plainLines splits the text of a file into lines that are not highlighted.
func plainLines(text string) []previewLine {
	var out []previewLine
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, previewLine{text: text[start:i]})
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, previewLine{text: text[start:]})
	}
	return out
}

// previewRow is one line of text, with a background when its kind is highlighted. A long
// line wraps.
func previewRow(bc m.BuildContext, l previewLine, style *m.TextStyle, index int) m.Widget {
	text := l.text
	if text == "" {
		text = " "
	}
	key := m.NewValueKey("preview:line:" + strconv.Itoa(index))
	scheme := m.ThemeOf(bc).ColorScheme
	if bg, fg, ok := highlight(scheme, l.kind); ok {
		return m.NewContainer(m.NewText(text, m.TextOpts{Style: style.CopyWith(m.TextStyleCopy{Color: m.Some(fg)})}),
			m.ContainerOpts{Key: key, Color: m.Some(bg), Padding: m.NewEdgeInsetsSymmetric(0, 4)})
	}
	return m.NewContainer(m.NewText(text, m.TextOpts{Style: style}),
		m.ContainerOpts{Key: key, Padding: m.NewEdgeInsetsSymmetric(0, 4)})
}
