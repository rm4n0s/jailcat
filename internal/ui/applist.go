package ui

import (
	"sort"
	"strconv"
	"strings"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/apps"
)

// rowExtent is the height of a row of the app list. Every row has the same height, so the
// list can lay out 3,000 apps without measuring them.
const rowExtent = 72.0

// AppListOpts are the optional parameters of [NewAppList].
type AppListOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// Builds counts the builds of each app by [AppKey] that were built ([BuildCounts]); an
	// app with a count above zero shows a "built" chip and passes the Built filter.
	Builds map[string]int
	// Drafts counts the builds that were never built ([DraftCounts]); an app that has
	// only those shows a "draft" chip.
	Drafts map[string]int
	// Updated holds the [AppKey] of the apps with a build made with another firejail
	// version than the installed one (SPEC §13.3): the row shows "firejail updated".
	Updated map[string]bool
	// BaseGone holds the [AppKey] of the apps with a build whose base profile file is
	// gone (SPEC §13.3): the row shows "base profile missing".
	BaseGone map[string]bool
	// Revision changes whenever Apps or Builds do, so the list knows to filter again.
	Revision int
	// Loading shows a progress indicator: the apps are being scanned.
	Loading bool
	// Error is a message for the user, in plain words, when the scan failed. It replaces
	// the list when there are no apps, and is shown above it otherwise.
	Error string
	// Selected is the [AppKey] of the selected app, or "".
	Selected string
	// OnSelect is called with the app the user taps. An app that is already sandboxed
	// cannot be selected.
	OnSelect func(app apps.App)
	// OnRefresh is called when the user presses Refresh.
	OnRefresh func()
}

// AppList is the left pane of the main window (SPEC §5): a search box, the filters of
// SPEC §4.3, the "Show hidden apps" switch, a Refresh button and the list of apps with
// their icons.
type AppList struct {
	m.StatefulWidget
	backend Backend
	apps    []apps.App
	o       AppListOpts
}

// NewAppList creates the list of appList. backend draws the icons.
func NewAppList(backend Backend, appList []apps.App, opts ...AppListOpts) *AppList {
	var o AppListOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &AppList{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), backend: backend, apps: appList, o: o}
}

// Apps are all the apps the list was given, before the search and the filter.
func (w *AppList) Apps() []apps.App { return w.apps }

// CreateState creates the state of the list.
func (w *AppList) CreateState() m.State { return &appListState{} }

type appListState struct {
	m.StateOf[*AppList]
	search     *m.TextEditingController
	filter     apps.Filter
	showHidden bool
	// shown are the apps that pass the search and the filter, in the order to draw them.
	shown []apps.App
}

func (s *appListState) InitState() {
	s.search = m.NewTextEditingController()
	s.refilter()
}

func (s *appListState) Dispose() { s.search.Dispose() }

func (s *appListState) DidUpdateWidget(old *AppList) {
	if old.o.Revision != s.Widget().o.Revision || len(old.apps) != len(s.Widget().apps) {
		s.refilter()
	}
}

// refilter computes the apps to draw from the search text, the filter and the switch.
func (s *appListState) refilter() {
	w := s.Widget()
	builds := w.o.Builds
	s.shown = apps.Search(w.apps, s.search.Text(), s.filter, apps.SearchOpts{
		ShowHidden: s.showHidden,
		Built:      func(a apps.App) bool { return builds[AppKey(a)] > 0 },
	})
	// Without a search, the menu apps come before the binaries, as in the mock of SPEC §5:
	// a system has hundreds of binaries, many of which start with a symbol or a digit.
	if s.filter == apps.FilterAll && strings.TrimSpace(s.search.Text()) == "" {
		sort.SliceStable(s.shown, func(i, j int) bool {
			return s.shown[i].Kind == apps.KindDesktop && s.shown[j].Kind == apps.KindBinary
		})
	}
}

func (s *appListState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)

	var progress m.Widget = m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)})
	if w.o.Loading {
		progress = m.NewSizedBox(m.NewLinearProgressIndicator(), m.SizedBoxOpts{Height: m.Some(4.0)})
	}

	children := []m.Widget{
		m.NewPadding(m.NewEdgeInsetsFromLTRB(12, 12, 4, 0), s.searchRow()),
		m.NewPadding(m.NewEdgeInsetsFromLTRB(12, 4, 12, 0), s.filterChips()),
		m.NewSwitchListTile(s.showHidden, func(v bool) {
			s.SetState(func() {
				s.showHidden = v
				s.refilter()
			})
		}, m.SwitchListTileOpts{
			Title:          m.NewText("Show hidden apps", m.TextOpts{Style: theme.TextTheme.BodyMedium}),
			Dense:          m.Some(true),
			ContentPadding: m.NewEdgeInsetsSymmetric(0, 16),
		}),
		progress,
	}
	if w.o.Error != "" && len(w.apps) > 0 {
		children = append(children, notice(bc, w.o.Error))
	}
	children = append(children, m.NewExpanded(s.body(bc)))
	return m.NewColumn(children, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// searchRow is the search field and the Refresh button.
func (s *appListState) searchRow() m.Widget {
	w := s.Widget()
	decoration := m.NewInputDecoration(m.InputDecorationOpts{
		HintText:   m.Some("Search apps"),
		PrefixIcon: m.NewIcon(m.Icons.Search()),
		IsDense:    m.Some(true),
		Border:     m.NewOutlineInputBorder(),
	})
	return m.NewRow([]m.Widget{
		m.NewExpanded(m.NewTextField(m.TextFieldOpts{
			Controller: s.search,
			Decoration: &decoration,
			OnChanged: func(string) {
				s.SetState(s.refilter)
			},
		})),
		m.NewIconButton(w.o.OnRefresh, m.NewIcon(m.Icons.Refresh()), m.IconButtonOpts{Tooltip: "Refresh"}),
	})
}

// filterChips are the filters All, Menu apps, Binaries and Built (SPEC §4.3).
func (s *appListState) filterChips() m.Widget {
	filters := []struct {
		label  string
		filter apps.Filter
	}{
		{"All", apps.FilterAll},
		{"Menu apps", apps.FilterMenu},
		{"Binaries", apps.FilterBinaries},
		{"Built", apps.FilterBuilt},
	}
	var chips []m.Widget
	for _, f := range filters {
		chips = append(chips, m.NewFilterChip(m.NewText(f.label), m.FilterChipOpts{
			Selected:      s.filter == f.filter,
			ShowCheckmark: m.Some(false),
			OnSelected: func(bool) {
				s.SetState(func() {
					s.filter = f.filter
					s.refilter()
				})
			},
		}))
	}
	return m.NewWrap(chips, m.WrapOpts{Spacing: 6})
}

// body is the list, or the text that stands in for it.
func (s *appListState) body(bc m.BuildContext) m.Widget {
	w := s.Widget()
	switch {
	case len(s.shown) > 0:
		return m.NewListViewBuilder(func(bc m.BuildContext, i int) m.Widget {
			if i >= len(s.shown) {
				return nil
			}
			app := s.shown[i]
			key := AppKey(app)
			return NewAppRow(w.backend, app, AppRowOpts{
				Key:      m.NewValueKey(key),
				Selected: key == w.o.Selected,
				Builds:   w.o.Builds[key],
				Drafts:   w.o.Drafts[key],
				Updated:  w.o.Updated[key],
				BaseGone: w.o.BaseGone[key],
				OnTap: func() {
					if w.o.OnSelect != nil {
						w.o.OnSelect(app)
					}
				},
			})
		}, m.ListViewOpts{ItemCount: m.Some(len(s.shown)), ItemExtent: m.Some(rowExtent)})
	case w.o.Loading && len(w.apps) == 0:
		return centeredText(bc, "Looking for apps…")
	case w.o.Error != "" && len(w.apps) == 0:
		return m.NewAlign(notice(bc, w.o.Error), m.AlignOpts{Alignment: m.AlignmentTopCenter})
	case len(w.apps) == 0:
		return centeredText(bc, "No apps were found.")
	case s.filter == apps.FilterBuilt && strings.TrimSpace(s.search.Text()) == "":
		return centeredText(bc, "You have not built any sandboxes yet.")
	}
	return centeredText(bc, "No apps match.")
}

// centeredText is a short message in the middle of a pane.
func centeredText(bc m.BuildContext, text string) m.Widget {
	theme := m.ThemeOf(bc)
	return m.NewCenter(m.NewPadding(m.NewEdgeInsetsAll(24), m.NewText(text, m.TextOpts{
		TextAlign: m.TextAlignCenter,
		Style:     theme.TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())}),
	})))
}

// notice is a message about a failure, in the error colours.
func notice(bc m.BuildContext, text string) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	return m.NewContainer(m.NewRow([]m.Widget{
		m.NewIcon(m.Icons.ErrorOutline(), m.IconOpts{Color: m.Some(scheme.OnErrorContainer())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewExpanded(m.NewText(text, m.TextOpts{
			Style: m.ThemeOf(bc).TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnErrorContainer())}),
		})),
	}), m.ContainerOpts{
		Color:   m.Some(scheme.ErrorContainer()),
		Padding: m.NewEdgeInsetsAll(12),
		Margin:  m.NewEdgeInsetsSymmetric(4, 12),
	})
}

// builtLabel is the text of the "built" chip: the word, and the count when there are
// several builds.
func builtLabel(n int) string {
	if n <= 1 {
		return "built"
	}
	return "built ×" + strconv.Itoa(n)
}
