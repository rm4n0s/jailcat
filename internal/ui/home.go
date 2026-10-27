package ui

import (
	"context"
	"log/slog"

	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"
	gui "github.com/rm4n0s/glutter/ui"

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
	// openingText is the right pane while the editor of an app is being prepared.
	openingText = "Opening…"
	// openFailedText is what the user reads when the editor of an app could not be opened.
	openFailedText = "Jailcat could not open this app. Choose it again to try once more."
	// draftFailedText is what the user reads when the unsaved changes of the editor could
	// not be kept.
	draftFailedText = "Jailcat could not keep your unsaved changes."
)

// HomePage is the main window (SPEC §5): the app list on the left, the build editor
// (or an empty state) on the right, and a banner on top when firejail is missing. It
// scans the apps, reads the builds and looks for firejail off the UI thread. Selecting
// another app keeps the unsaved changes of the editor as a draft (SPEC §5), and so does
// closing the window.
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

	// apps, builds, drafts and revision are what the last finished scan found: the number
	// of built builds and of never-built ones (drafts) of each app.
	apps     []apps.App
	builds   map[string]int
	drafts   map[string]int
	revision int
	// allBuilds are the builds the counts were made from; missingBase holds the apps with a
	// build whose base profile is gone (SPEC §13.3).
	allBuilds   []store.Build
	missingBase map[string]bool
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
	// editor is the editor of the selected app, nil while it is being opened.
	editor *Editor
	// opening is true while the editor is being prepared; openErr is the message for the
	// user when it could not be.
	opening bool
	openErr string
	// openGen numbers the openings, so the answer of one that was replaced is dropped.
	openGen int
	// notice is a message about keeping the unsaved changes.
	notice string
	// cancelOpen stops the reads of the opening that runs; lastSave is closed when the last
	// draft being written is done, so a draft is in the database before the next opening
	// looks for builds. Both are used on the UI thread only.
	cancelOpen context.CancelFunc
	lastSave   <-chan struct{}

	// lifecycle gets the request to close the window. exiting is true while the draft is
	// written before the window closes; closeAnyway says the next request closes it even if
	// the draft could not be written.
	binding     *m.WidgetsBinding
	lifecycle   *m.AppLifecycleListener
	exiting     bool
	closeAnyway bool
	// try runs the Try of the window.
	try *TryController
}

func (s *homeState) InitState() {
	s.try = NewTryController(s.Widget().backend, s.Widget().logger)
	s.binding = m.WidgetsBindingOf(s.Context())
	s.lifecycle = m.NewAppLifecycleListener(s.binding, m.AppLifecycleListenerOpts{OnExitRequested: s.onExitRequested})
	s.startScan()
	s.startFirejail()
}

func (s *homeState) Dispose() {
	s.lifecycle.Dispose()
	s.try.Stop()
}

// onExitRequested is the close button of the window. Unsaved changes of the editor are
// kept as a draft first, off the UI thread, so the request is answered with Cancel and
// the app exits itself when the draft is written (SPEC §5). If the draft cannot be
// written the window stays open with a message, and a second close request closes it.
func (s *homeState) onExitRequested() gui.AppExitResponse {
	if s.closeAnyway {
		return gui.AppExitResponseExit
	}
	var plan DraftPlan
	if s.editor != nil {
		plan = s.editor.DraftPlan()
	}
	trying := s.try.Active()
	if plan.Action == DraftNothing && !trying {
		return gui.AppExitResponseExit
	}
	if s.exiting {
		return gui.AppExitResponseCancel
	}
	s.exiting = true
	backend, logger, try := s.Widget().backend, s.Widget().logger, s.try
	s.Go(func(ctx context.Context) {
		// A sandbox that is left running belongs to nobody once Jailcat is gone.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tryGrace)
		try.StopWait(stopCtx)
		cancel()
		var err error
		if plan.Action != DraftNothing {
			err = plan.Run(context.WithoutCancel(ctx), backend)
		}
		if err != nil {
			logger.Error("keeping the draft before closing failed", "err", err)
		}
		s.binding.Post(func() {
			s.exiting = false
			if err != nil {
				s.closeAnyway = true
				s.SetState(func() { s.notice = draftFailedText + " Close the window again to leave without them." })
				return
			}
			s.binding.ExitApplication(gui.AppExitTypeRequired)
		})
	})
	return gui.AppExitResponseCancel
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
		var missing map[string]bool
		if scanErr == nil {
			builds, buildsErr = backend.ListBuilds(ctx)
			if buildsErr == nil {
				missing = missingBaseApps(ctx, backend, builds)
			}
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
				s.setApps(res.Apps, builds, missing)
			}
			if buildsErr != nil {
				s.buildsErr = buildsFailedText
			}
			s.revision++
		})
	})
}

// setApps stores the result of a scan and keeps the selection if the app is still there.
func (s *homeState) setApps(found []apps.App, builds []store.Build, missing map[string]bool) {
	s.apps = found
	s.setBuilds(builds, missing)
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
	if s.selected == nil {
		// The app is gone: so is its editor.
		s.editor, s.opening, s.openErr = nil, false, ""
		s.openGen++
	}
}

// setBuilds stores the builds the list marks come from.
func (s *homeState) setBuilds(builds []store.Build, missing map[string]bool) {
	s.allBuilds, s.missingBase = builds, missing
	s.builds, s.drafts = BuildCounts(builds), DraftCounts(builds)
}

// refreshBuilds reads the saved builds again, off the UI thread, so the list shows the
// effect of a Build or an Uninstall. It runs on the UI thread.
func (s *homeState) refreshBuilds() {
	backend, logger := s.Widget().backend, s.Widget().logger
	s.Go(func(ctx context.Context) {
		all, err := backend.ListBuilds(ctx)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("reading the builds failed", "err", err)
			}
			return
		}
		missing := missingBaseApps(ctx, backend, all)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			s.setBuilds(all, missing)
			s.revision++
		})
	})
}

// uninstalled is called when the open build was removed from Jailcat: its editor goes.
func (s *homeState) uninstalled() {
	s.SetState(func() {
		s.selected, s.editor, s.opening, s.openErr = nil, nil, false, ""
		s.openGen++
	})
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
		Drafts:   s.drafts,
		Updated:  s.updatedApps(),
		BaseGone: s.missingBase,
		Revision: s.revision,
		Loading:  s.loading,
		Error:    errText,
		Selected: selected,
		OnSelect: func(app apps.App) {
			s.SetState(func() { s.selectApp(app) })
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

// rightPane is the empty state, the build editor of the selected app, or what stands
// in for it while the editor opens.
func (s *homeState) rightPane(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	switch {
	case s.selected == nil:
		return m.NewCenter(m.NewColumn([]m.Widget{
			svg.NewSvgPictureMemory(assets.Icon(), svg.SvgPictureOpts{
				Width: m.Some(96.0), Height: m.Some(96.0), ExcludeFromSemantics: true,
			}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(16.0)}),
			m.NewText(emptyStateText, m.TextOpts{
				Style: theme.TextTheme.TitleMedium.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())}),
			}),
		}, m.ColumnOpts{MainAxisSize: m.MainAxisSizeMin}))
	case s.editor != nil:
		editorPage := NewEditorPage(s.Widget().backend, s.Widget().logger, s.editor, EditorPageOpts{
			Key:             m.NewValueKey(s.editor),
			OnDiscardDraft:  s.discardDraft,
			Try:             s.try,
			FirejailMissing: s.known && s.firejail.Missing,
			OnBuildsChanged: s.refreshBuilds,
			OnUninstalled:   s.uninstalled,
		})
		if s.notice == "" {
			return editorPage
		}
		return m.NewColumn([]m.Widget{notice(bc, s.notice), m.NewExpanded(editorPage)},
			m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
	case s.openErr != "":
		return m.NewAlign(notice(bc, s.openErr), m.AlignOpts{Alignment: m.AlignmentTopCenter})
	}
	return centeredText(bc, openingText)
}

// updatedApps returns the apps with a build made with another firejail than the one that
// is installed now (SPEC §13.3); none while the version is not known.
func (s *homeState) updatedApps() map[string]bool {
	if !s.known || s.firejail.Missing {
		return nil
	}
	return updatedApps(s.allBuilds, s.firejail.Version)
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
