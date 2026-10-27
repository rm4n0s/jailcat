package ui

import (
	"context"
	"log/slog"

	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"

	"github.com/rm4n0s/jailcat/assets"
	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/store"
)

// listWidth is the width of the left pane, in logical pixels.
const listWidth = 360.0

// Texts the user reads. They are constants so the tests and the screens agree.
const (
	// firejailMissingText is the banner shown when firejail is not installed (SPEC §3).
	firejailMissingText = "firejail is not installed: Jailcat can't try or build sandboxes. Install it with your package manager."
	// scanFailedText is what the user reads when the scan of the apps failed; the error
	// itself goes to the log.
	scanFailedText = "Jailcat could not read the list of apps. Press Refresh to try again."
	// buildsFailedText is what the user reads when the saved builds could not be read.
	buildsFailedText = "Jailcat could not read your saved builds, so none is marked as built. Press Refresh to try again."
	// emptyStateText is the right pane while no app is selected.
	emptyStateText = "Choose an app to build its sandbox"
)

// HomePage is the main window (SPEC §5): the app list on the left, the build editor
// (an empty state until step 4.2) on the right, and a banner on top when firejail is
// missing. It scans the apps, reads the builds and looks for firejail off the UI thread.
type HomePage struct {
	m.StatefulWidget
	backend Backend
	logger  *slog.Logger
}

// NewHomePage creates the main window's content.
func NewHomePage(backend Backend, logger *slog.Logger, opts ...m.WidgetOpts) *HomePage {
	return &HomePage{StatefulWidget: m.NewStatefulWidgetBase(opts...), backend: backend, logger: logger}
}

// CreateState creates the state of the page.
func (w *HomePage) CreateState() m.State { return &homeState{} }

type homeState struct {
	m.StateOf[*HomePage]

	// apps, builds and revision are what the last finished scan found.
	apps     []apps.App
	builds   map[string]int
	revision int
	// loading is true while a scan runs; scanErr and buildsErr are the messages for the
	// user when the last scan or the read of the builds failed.
	loading   bool
	scanErr   string
	buildsErr string
	// generation numbers the scans, so the answer of one that was replaced is dropped;
	// cancelScan stops the running one. Both are used on the UI thread only.
	generation int
	cancelScan context.CancelFunc
	// firejail is what the backend found; known says it answered.
	firejail FirejailInfo
	known    bool
	// selected is the app the user picked, nil for none.
	selected *apps.App
}

func (s *homeState) InitState() {
	s.startScan()
	s.startFirejail()
}

// startScan begins a scan, replacing the one that runs. It runs on the UI thread.
func (s *homeState) startScan() {
	if s.cancelScan != nil {
		s.cancelScan()
	}
	ctx, cancel := context.WithCancel(s.Lifetime())
	s.cancelScan = cancel
	s.generation++
	gen := s.generation
	s.loading = true

	backend, logger := s.Widget().backend, s.Widget().logger
	s.Go(func(context.Context) {
		res, scanErr := backend.ScanApps(ctx)
		var builds []store.Build
		var buildsErr error
		if scanErr == nil {
			builds, buildsErr = backend.ListBuilds(ctx)
		}
		if ctx.Err() != nil {
			// Replaced by a newer scan, or the window is closing.
			return
		}
		if scanErr != nil {
			logger.Error("scanning the apps failed", "err", scanErr)
		}
		if buildsErr != nil {
			logger.Error("reading the builds failed", "err", buildsErr)
		}
		s.SetState(func() {
			if s.generation != gen {
				return
			}
			s.loading = false
			s.scanErr, s.buildsErr = "", ""
			if scanErr != nil {
				s.scanErr = scanFailedText
			} else {
				s.setApps(res.Apps, builds)
			}
			if buildsErr != nil {
				s.buildsErr = buildsFailedText
			}
			s.revision++
		})
	})
}

// setApps stores the result of a scan and keeps the selection if the app is still there.
func (s *homeState) setApps(found []apps.App, builds []store.Build) {
	s.apps = found
	s.builds = BuildCounts(builds)
	if s.selected == nil {
		return
	}
	key := AppKey(*s.selected)
	s.selected = nil
	for i := range found {
		if AppKey(found[i]) == key {
			s.selected = &found[i]
			break
		}
	}
}

// startFirejail looks for firejail off the UI thread.
func (s *homeState) startFirejail() {
	backend, logger := s.Widget().backend, s.Widget().logger
	s.Go(func(ctx context.Context) {
		info, err := backend.Firejail(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// firejail is installed but did not answer: the log has why, and the banner
			// stays away, as it is only for a missing firejail.
			logger.Error("looking for firejail failed", "err", err)
		}
		s.SetState(func() {
			s.firejail = info
			s.known = true
		})
	})
}

func (s *homeState) refresh() {
	s.SetState(s.startScan)
}

func (s *homeState) Build(bc m.BuildContext) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	errText := s.scanErr
	if errText == "" {
		errText = s.buildsErr
	}
	selected := ""
	if s.selected != nil {
		selected = AppKey(*s.selected)
	}

	list := NewAppList(s.Widget().backend, s.apps, AppListOpts{
		Builds:   s.builds,
		Revision: s.revision,
		Loading:  s.loading,
		Error:    errText,
		Selected: selected,
		OnSelect: func(app apps.App) {
			s.SetState(func() { s.selected = &app })
		},
		OnRefresh: s.refresh,
	})
	panes := m.NewRow([]m.Widget{
		m.NewSizedBox(list, m.SizedBoxOpts{Width: m.Some(listWidth)}),
		m.NewVerticalDivider(m.VerticalDividerOpts{Width: m.Some(1.0)}),
		m.NewExpanded(s.rightPane(bc)),
	})

	var children []m.Widget
	if s.known && s.firejail.Missing {
		children = append(children, firejailBanner(bc))
	}
	children = append(children, m.NewExpanded(panes))
	return m.NewScaffold(m.ScaffoldOpts{
		BackgroundColor: m.Some(scheme.Surface()),
		Body:            m.NewColumn(children, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}),
	})
}

// rightPane is the empty state, or the name and the program of the selected app until
// the build editor replaces it (step 4.2).
func (s *homeState) rightPane(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	if s.selected == nil {
		return m.NewCenter(m.NewColumn([]m.Widget{
			svg.NewSvgPictureMemory(assets.Icon(), svg.SvgPictureOpts{
				Width: m.Some(96.0), Height: m.Some(96.0), ExcludeFromSemantics: true,
			}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(16.0)}),
			m.NewText(emptyStateText, m.TextOpts{
				Style: theme.TextTheme.TitleMedium.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())}),
			}),
		}, m.ColumnOpts{MainAxisSize: m.MainAxisSizeMin}))
	}
	// TODO(step 4.2): the build editor replaces this.
	return m.NewPadding(m.NewEdgeInsetsAll(24), m.NewColumn([]m.Widget{
		m.NewText(s.selected.Name, m.TextOpts{Style: theme.TextTheme.HeadlineSmall}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		m.NewText(s.selected.Binary, m.TextOpts{Style: theme.TextTheme.BodyMedium}),
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}))
}

// firejailBanner is the strip on top of the window when firejail is not installed.
func firejailBanner(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	return m.NewContainer(m.NewRow([]m.Widget{
		m.NewIcon(m.Icons.WarningAmber(), m.IconOpts{Color: m.Some(scheme.OnErrorContainer())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
		m.NewExpanded(m.NewText(firejailMissingText, m.TextOpts{
			Style: theme.TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnErrorContainer())}),
		})),
	}), m.ContainerOpts{
		Color:   m.Some(scheme.ErrorContainer()),
		Padding: m.NewEdgeInsetsSymmetric(12, 16),
	})
}
