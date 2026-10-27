package ui

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the editor the user reads.
const (
	// profileLabelText is the label before the profile selector's button.
	profileLabelText = "Profile:"
	// draftNoteText marks an editor that opened on a draft (SPEC §5).
	draftNoteText = "Unsaved draft"
	// discardDraftText is the button that throws a draft away.
	discardDraftText = "Discard draft"
	// readingProfileText is shown while a profile is read.
	readingProfileText = "Reading the profile…"
)

// EditorPageOpts are the optional parameters of [NewEditorPage].
type EditorPageOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// OnDiscardDraft is called when the user presses Discard draft.
	OnDiscardDraft func()
}

// EditorPage is the build editor of SPEC §6: the header, the profile selector, the row
// of tabs with a dot on the ones that have changes, the body of the tab, the problems
// and the buttons. It shows and changes an [Editor]. The Files tab is a [FilesTab]; the other
// bodies list the settings with their values for now.
type EditorPage struct {
	m.StatefulWidget
	backend Backend
	logger  *slog.Logger
	editor  *Editor
	o       EditorPageOpts
}

// NewEditorPage creates the editor screen for editor. backend serves the icon, the
// profile selector and the profiles; logger gets what the user must not read.
func NewEditorPage(backend Backend, logger *slog.Logger, editor *Editor, opts ...EditorPageOpts) *EditorPage {
	var o EditorPageOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &EditorPage{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		backend:        backend, logger: logger, editor: editor, o: o,
	}
}

// Editor is the editor the page shows.
func (w *EditorPage) Editor() *Editor { return w.editor }

// CreateState creates the state of the page.
func (w *EditorPage) CreateState() m.State { return &editorPageState{tab: firejail.TabFiles} }

type editorPageState struct {
	m.StateOf[*EditorPage]
	// tab is the selected tab.
	tab firejail.Tab
	// reading is true while a profile is read; failure is the message for the user when
	// the last profile could not be read.
	reading bool
	failure string
}

func (s *editorPageState) editor() *Editor { return s.Widget().editor }

// openProfiles shows the profile selector and switches to the profile the user picks.
func (s *editorPageState) openProfiles() {
	w := s.Widget()
	result := m.ShowDialog[ProfileDialogResult](s.Context(), func(m.BuildContext) m.Widget {
		return NewProfileDialog(w.backend, w.logger, w.editor.App(), w.editor.Choices(), w.editor.BaseProfile())
	})
	s.Go(func(ctx context.Context) {
		select {
		case r := <-result:
			if r.Choice.Group == "" {
				return
			}
			s.SetState(func() { s.chooseProfile(r) })
		case <-ctx.Done():
		}
	})
}

// chooseProfile reads the profile off the UI thread and, when it is read, makes it the
// base profile. The previous profile stays when it cannot be read. It runs on the UI
// thread.
func (s *editorPageState) chooseProfile(r ProfileDialogResult) {
	w := s.Widget()
	if r.Imported {
		w.editor.SetChoices(r.Choices)
	}
	choice := r.Choice
	if choice.Path == w.editor.BaseProfile() {
		return
	}
	s.reading, s.failure = true, ""
	s.Go(func(ctx context.Context) {
		base, profile, err := w.backend.ReadBase(ctx, choice.Path)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			s.reading = false
			if err != nil {
				w.logger.Error("reading the base profile failed", "profile", choice.Path, "err", err)
				s.failure = readFailedText(choice, err)
				return
			}
			if !w.editor.HasChanges() {
				w.editor.SwitchProfile(choice.Path, base, profile, true)
				return
			}
			s.askKeep(choice, func(keep bool) { w.editor.SwitchProfile(choice.Path, base, profile, keep) })
		})
	})
}

// readFailedText is what the user reads when a profile could not be read.
func readFailedText(choice ProfileChoice, err error) string {
	return "Jailcat could not read the profile " + choice.Name + ": " + plainReason(err) + ". The previous profile is still in use."
}

// askKeep asks whether to keep the user's changes on the new profile and calls apply
// with the answer; Cancel calls nothing. It runs on the UI thread.
func (s *editorPageState) askKeep(choice ProfileChoice, apply func(keep bool)) {
	count := s.editor().ChangeCount()
	result := m.ShowDialog[string](s.Context(), func(m.BuildContext) m.Widget {
		return NewKeepChangesDialog(count, choice.Name)
	})
	s.Go(func(ctx context.Context) {
		select {
		case v := <-result:
			if v == "" {
				return
			}
			s.SetState(func() { apply(v == KeepChanges) })
		case <-ctx.Done():
		}
	})
}

func (s *editorPageState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	e := w.editor

	tabs := EditorTabs()
	changed := e.ChangedTabs()
	var tabWidgets []m.Widget
	for _, t := range tabs {
		label := []m.Widget{m.NewText(TabTitle(t))}
		if changed[t] {
			label = append(label, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(6.0)}), NewChangeDot())
		}
		tabWidgets = append(tabWidgets, m.NewTab(m.TabOpts{
			Key:   m.NewValueKey("tab:" + string(t)),
			Child: m.NewRow(label, m.RowOpts{MainAxisSize: m.MainAxisSizeMin}),
		}))
	}
	index := 0
	for i, t := range tabs {
		if t == s.tab {
			index = i
		}
	}
	bar := m.NewDefaultTabController(len(tabs), m.NewTabBar(tabWidgets, m.TabBarOpts{
		IsScrollable: true,
		TabAlignment: m.TabAlignmentStart,
		// Nine tabs and their dots must fit the right pane of the default window.
		LabelPadding: m.NewEdgeInsetsSymmetric(0, 10),
		OnTap: func(i int) {
			s.SetState(func() { s.tab = tabs[i] })
		},
	}), m.DefaultTabControllerOpts{InitialIndex: index, AnimationDuration: m.Some(time.Duration(0))})

	act := tabActions{
		reset:  func(f firejail.Field) { s.SetState(func() { e.Reset(f) }) },
		change: func(apply func()) { s.SetState(apply) },
		logger: w.logger,
	}

	children := []m.Widget{
		s.header(bc),
		bar,
		m.NewExpanded(tabBody(bc, e, s.tab, act)),
	}
	if probs := s.problems(bc); probs != nil {
		children = append(children, probs)
	}
	children = append(children, m.NewDivider(m.DividerOpts{Height: m.Some(1.0)}), s.buttons(bc))
	return m.NewColumn(children, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// header is the icon, the names, the profile selector and the notes about the build.
func (s *editorPageState) header(bc m.BuildContext) m.Widget {
	w := s.Widget()
	e := w.editor
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	app := e.App()
	final := e.Final()

	title := m.NewRow([]m.Widget{
		m.NewFlexible(m.NewText(app.Name, m.TextOpts{Style: theme.TextTheme.HeadlineSmall, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewIcon(m.Icons.ArrowForward(), m.IconOpts{Size: m.Some(20.0), Color: m.Some(scheme.OnSurfaceVariant())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewFlexible(m.NewText(final.SandboxName, m.TextOpts{
			Style: theme.TextTheme.TitleMedium.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSurfaceVariant())}), MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis,
		})),
	})

	selector := m.NewOutlinedButton(s.openProfiles, m.NewRow([]m.Widget{
		m.NewFlexible(m.NewText(e.ProfileLabel(), m.TextOpts{MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})),
		m.NewIcon(m.Icons.ArrowDropDown()),
	}, m.RowOpts{MainAxisSize: m.MainAxisSizeMin}), m.OutlinedButtonOpts{Key: m.NewValueKey("profile-selector")})
	profileRow := []m.Widget{
		m.NewText(profileLabelText, m.TextOpts{Style: theme.TextTheme.BodyMedium}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewConstrainedBox(m.NewBoxConstraints(m.BoxConstraintsOpts{MaxWidth: m.Some(420.0)}), selector),
	}
	if s.reading {
		profileRow = append(profileRow, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
			m.NewSizedBox(m.NewCircularProgressIndicator(), m.SizedBoxOpts{Width: m.Some(18.0), Height: m.Some(18.0)}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewText(readingProfileText, m.TextOpts{Style: theme.TextTheme.BodySmall}))
	}

	col := []m.Widget{
		m.NewRow([]m.Widget{
			NewAppIcon(w.backend, app, AppIconOpts{Size: 48}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
			m.NewExpanded(title),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		m.NewRow(profileRow, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
	}
	if e.FromDraft() {
		col = append(col, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}), m.NewRow([]m.Widget{
			badge(bc, draftNoteText, scheme.SecondaryContainer(), scheme.OnSecondaryContainer()),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewTextButton(w.o.OnDiscardDraft, m.NewText(discardDraftText)),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
	}
	if text := e.Notice(); text != "" {
		col = append(col, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}), notice(bc, text))
	}
	if s.failure != "" {
		col = append(col, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}), notice(bc, s.failure))
	}
	return m.NewPadding(m.NewEdgeInsetsFromLTRB(16, 16, 16, 8), m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}))
}

// maxProblemsShown is how many problems are listed under the tabs before "and N more".
const maxProblemsShown = 3

// problems lists what is wrong with the settings, errors and warnings; nil when nothing is.
func (s *editorPageState) problems(bc m.BuildContext) m.Widget {
	probs := s.editor().Problems()
	if len(probs) == 0 {
		return nil
	}
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	var rows []m.Widget
	for i, p := range probs {
		if i == maxProblemsShown {
			rows = append(rows, m.NewText("and "+strconv.Itoa(len(probs)-maxProblemsShown)+" more", m.TextOpts{Style: theme.TextTheme.BodySmall}))
			break
		}
		icon, color := m.Icons.Error(), scheme.Error()
		if p.Severity == firejail.SeverityWarning {
			icon, color = m.Icons.Warning(), scheme.Tertiary()
		}
		rows = append(rows, m.NewRow([]m.Widget{
			m.NewIcon(icon, m.IconOpts{Size: m.Some(18.0), Color: m.Some(color)}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewExpanded(m.NewText(p.Message, m.TextOpts{Style: theme.TextTheme.BodySmall, MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis})),
		}))
	}
	return m.NewPadding(m.NewEdgeInsetsSymmetric(8, 16), m.NewColumn(rows, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}))
}

// The buttons of the bar. They are drawn disabled until step 4.7 gives them work.
const (
	uninstallText = "Uninstall"
	previewText   = "Preview"
	tryText       = "Try"
	buildText     = "Build"
)

// buttons is the bar at the bottom: Uninstall (for an installed build) on the left,
// Preview, Try and Build on the right.
func (s *editorPageState) buttons(bc m.BuildContext) m.Widget {
	// TODO(step 4.7): give the buttons their work; a nil callback draws them disabled.
	var left m.Widget = m.NewSizedBox(nil, m.SizedBoxOpts{})
	if s.editor().Build().Installed {
		left = m.NewOutlinedButton(nil, m.NewText(uninstallText))
	}
	return m.NewPadding(m.NewEdgeInsetsSymmetric(12, 16), m.NewRow([]m.Widget{
		left,
		m.NewSpacer(),
		m.NewOutlinedButton(nil, m.NewText(previewText)),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewOutlinedButton(nil, m.NewText(tryText)),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewFilledButton(nil, m.NewText(buildText)),
	}))
}
