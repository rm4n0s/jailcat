package ui

import (
	"context"
	"slices"
	"time"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// busyKind says what the button bar is working on.
type busyKind int

// What the page can be busy with. Only one thing at a time.
const (
	busyNone busyKind = iota
	// busyChecking is the check of the Extra rules before Try and Build.
	busyChecking
	// busyPreparing is the making of the files for Preview or Build.
	busyPreparing
	// busyBuilding is Build writing the files.
	busyBuilding
	// busyUninstalling is Uninstall removing the files.
	busyUninstalling
)

// The buttons of the bar and the texts around them.
const (
	uninstallText = "Uninstall"
	previewText   = "Preview"
	tryText       = "Try"
	buildText     = "Build"

	firejailUpdatedText = "firejail updated"
	baseMissingText     = "base profile missing"

	busyCheckingText  = "Checking the Extra rules…"
	busyPreparingText = "Preparing the files…"
	busyBuildingText  = "Building…"
	busyUninstallText = "Uninstalling…"
	problemsBlockText = "Fix the problems listed above first: they stop Try and Build."
)

// hasErrors reports whether the editor has a problem that stops Try and Build.
func (s *editorPageState) hasErrors() bool {
	return slices.ContainsFunc(s.editor().Problems(), func(p firejail.Problem) bool { return p.Severity == firejail.SeverityError })
}

// canRun reports whether Try and Build may be pressed.
func (s *editorPageState) canRun() bool {
	return s.busy == busyNone && !s.Widget().o.FirejailMissing && !s.hasErrors()
}

// buttons is the bar at the bottom: Uninstall (for an installed build) on the left,
// Preview, Try and Build on the right.
func (s *editorPageState) buttons(bc m.BuildContext) m.Widget {
	var left m.Widget = m.NewSizedBox(nil, m.SizedBoxOpts{})
	if s.editor().Build().Installed {
		var onUninstall func()
		if s.busy == busyNone {
			onUninstall = func() { s.SetState(s.uninstall) }
		}
		left = m.NewOutlinedButton(onUninstall, m.NewText(uninstallText), m.OutlinedButtonOpts{Key: m.NewValueKey("button:uninstall")})
	}
	// A button press changes the state of the page: the work starts inside SetState.
	press := func(fn func()) func() { return func() { s.SetState(fn) } }
	var onPreview, onTry, onBuild func()
	if s.busy == busyNone {
		onPreview = press(s.preview)
	}
	if s.canRun() {
		onBuild = press(s.build)
		if !s.try.Active() {
			onTry = press(s.startTry)
		}
	}
	return m.NewPadding(m.NewEdgeInsetsSymmetric(12, 16), m.NewRow([]m.Widget{
		left,
		m.NewSpacer(),
		m.NewOutlinedButton(onPreview, m.NewText(previewText), m.OutlinedButtonOpts{Key: m.NewValueKey("button:preview")}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewOutlinedButton(onTry, m.NewText(tryText), m.OutlinedButtonOpts{Key: m.NewValueKey("button:try")}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewFilledButton(onBuild, m.NewText(buildText), m.FilledButtonOpts{Key: m.NewValueKey("button:build")}),
	}))
}

// progress is the thin bar over the buttons with what is going on, while one works.
func (s *editorPageState) progress(bc m.BuildContext) m.Widget {
	var text string
	switch s.busy {
	case busyChecking:
		text = busyCheckingText
	case busyPreparing:
		text = busyPreparingText
	case busyBuilding:
		text = busyBuildingText
	case busyUninstalling:
		text = busyUninstallText
	default:
		return m.NewSizedBox(nil, m.SizedBoxOpts{})
	}
	return m.NewColumn([]m.Widget{
		m.NewLinearProgressIndicator(m.LinearProgressIndicatorOpts{Key: m.NewValueKey("progress")}),
		m.NewPadding(m.NewEdgeInsetsFromLTRB(16, 6, 16, 0), mutedText(bc, text, m.TextOpts{Key: m.NewValueKey("progress:text")})),
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// fail shows a message about a button that could not do its work and logs the details.
// It runs on the UI thread.
func (s *editorPageState) fail(what string, text string, err error) {
	s.busy = busyNone
	s.actionFailure = text
	s.Widget().logger.Error(what+" failed", "sandbox", s.editor().Final().SandboxName, "err", err)
}

// --- Preview -----------------------------------------------------------------------

// preview makes the files off the UI thread and shows them in the Preview dialog.
func (s *editorPageState) preview() {
	w := s.Widget()
	req := w.editor.Request()
	s.busy, s.actionFailure = busyPreparing, ""
	s.Go(func(ctx context.Context) {
		prepared, err := w.backend.PrepareBuild(ctx, req)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			s.busy = busyNone
			if err != nil {
				s.fail("preparing the preview", prepareFailedText(err), err)
				return
			}
			m.ShowDialog[string](s.Context(), func(m.BuildContext) m.Widget { return NewPreviewDialog(prepared) })
		})
	})
}

// --- Try ---------------------------------------------------------------------------

// tryChanged is called by the controller, on any goroutine, when the Try changed.
func (s *editorPageState) tryChanged() {
	s.SetState(func() {
		s.tryAtEnd = s.scrolledToEnd()
		if s.tryAtEnd {
			s.binding.RunPostFrame(func(time.Duration) { s.followTry() })
		}
	})
}

// scrolledToEnd reports whether the output of the Try is scrolled to its last line, or
// has nothing to scroll.
func (s *editorPageState) scrolledToEnd() bool {
	if !s.tryScroll.HasClients() {
		return true
	}
	pos := s.tryScroll.Position()
	return pos.Pixels() >= pos.MaxScrollExtent()-tryLineExtent
}

// followTry scrolls the output to its end.
func (s *editorPageState) followTry() {
	if !s.Mounted() || !s.tryScroll.HasClients() {
		return
	}
	pos := s.tryScroll.Position()
	if max := pos.MaxScrollExtent(); pos.Pixels() < max {
		s.tryScroll.JumpTo(max)
	}
}

// startTry checks the Extra rules and starts the Try.
func (s *editorPageState) startTry() {
	s.checkThen(func(req BuildRequest) {
		s.try.Start(s.editor(), req)
	})
}

// tryPanel is the panel of the Try of this editor; nil when it has none.
func (s *editorPageState) tryPanel() m.Widget {
	snap := s.try.Snapshot()
	if snap.Owner != s.editor() || snap.Phase == TryIdle {
		return nil
	}
	return NewTryPanel(snap, TryPanelOpts{
		Scroll:  s.tryScroll,
		OnStop:  s.try.Stop,
		OnClose: s.try.Clear,
	})
}

// --- Checking the Extra rules --------------------------------------------------------

// checkThen checks the Extra rules again when the editor has no answer for the rows it
// has, and calls then, on the UI thread, with the request of the editor when none of
// them is an error (SPEC §7.9). Otherwise it shows why it did not.
func (s *editorPageState) checkThen(then func(req BuildRequest)) {
	w := s.Widget()
	e := w.editor
	s.actionFailure = ""
	if e.ExtraRulesChecked() {
		then(e.Request())
		return
	}
	rows, base := e.ExtraRules(), e.Base()
	s.busy = busyChecking
	s.Go(func(ctx context.Context) {
		found := w.backend.CheckExtraRules(ctx, rows, base)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			s.busy = busyNone
			if slices.Equal(e.ExtraRules(), rows) {
				e.SetExtraProblems(rows, found)
			}
			if s.hasErrors() {
				s.actionFailure = problemsBlockText
				return
			}
			then(e.Request())
		})
	})
}

// --- Error texts -------------------------------------------------------------------

// prepareFailedText is what the user reads when the files could not be made.
func prepareFailedText(err error) string {
	if text := knownFailure(err); text != "" {
		return text
	}
	return "Jailcat could not make the files of this build: " + plainReason(err) + "."
}

// buildFailedText is what the user reads when Build failed.
func buildFailedText(err error) string {
	if text := knownFailure(err); text != "" {
		return text
	}
	return "Jailcat could not build this sandbox: " + plainReason(err) + "."
}

// knownFailure explains the failures the user can do something about; "" for others.
func knownFailure(err error) string {
	appErr, ok := errors.FromError(err)
	if !ok {
		return ""
	}
	switch appErr.Tag {
	case firejail.TagFirejailNotFound:
		return firejailMissingShort
	case install.TagSandboxNameInvalid:
		return "The sandbox name must have at least one character, and only a-z, 0-9 and -. Change it in the Launcher tab."
	case store.TagSandboxNameTaken:
		return "Another build already has this sandbox name. Choose another one in the Launcher tab."
	case install.TagConflictsRemain:
		return "Some files are in the way. Press Build again to see which."
	}
	return ""
}
