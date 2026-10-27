package ui

import (
	"context"
	"log/slog"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/desktop"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/apps"
)

// Texts of the profile selector.
const (
	// profileDialogTitle is the title of the selector's dialog.
	profileDialogTitle = "Choose a profile"
	// profileSearchHint is the hint of the selector's search field.
	profileSearchHint = "Search profiles"
	// importButtonText is the button that adds a profile file of the user's.
	importButtonText = "Import profile…"
	// noProfilesText is shown when the search matches no profile.
	noProfilesText = "No profile matches."
)

// profileRowExtent is the height of a row of the selector's list (headers included), so
// the list can lay out 1,400 profiles without measuring them.
const profileRowExtent = 44.0

// ProfileDialogResult is what the profile selector's dialog pops with. The zero value
// is "closed without a choice".
type ProfileDialogResult struct {
	// Choice is the entry the user picked; its Group is empty when there was no choice.
	Choice ProfileChoice
	// Choices is the new content of the selector when the dialog imported a profile;
	// the zero value otherwise.
	Choices ProfileChoices
	// Imported is true when Choices is new.
	Imported bool
}

// ProfileDialog is the dialog of the profile selector (SPEC §6.1): a search field, the
// profiles in their groups, and **Import profile…**. The list is virtual: the Firejail
// profiles are about 1,400 and only the visible rows are built. It pops with a
// [ProfileDialogResult].
type ProfileDialog struct {
	m.StatefulWidget
	backend Backend
	logger  *slog.Logger
	app     apps.App
	choices ProfileChoices
	current string
}

// NewProfileDialog creates the dialog for the app. choices is what the selector lists,
// current is the path of the profile in use ("" is None).
func NewProfileDialog(backend Backend, logger *slog.Logger, app apps.App, choices ProfileChoices, current string, opts ...m.WidgetOpts) *ProfileDialog {
	return &ProfileDialog{
		StatefulWidget: m.NewStatefulWidgetBase(opts...),
		backend:        backend, logger: logger, app: app, choices: choices, current: current,
	}
}

// CreateState creates the state of the dialog.
func (w *ProfileDialog) CreateState() m.State { return &profileDialogState{} }

// profileRow is a row of the selector's list: a group header or an entry.
type profileRow struct {
	header ProfileGroup
	choice ProfileChoice
}

type profileDialogState struct {
	m.StateOf[*ProfileDialog]
	search *m.TextEditingController
	rows   []profileRow
	// busy is true while a file is chosen and imported; failure is the message to show.
	busy    bool
	failure string
}

func (s *profileDialogState) InitState() {
	s.search = m.NewTextEditingController()
	s.refilter()
}

func (s *profileDialogState) Dispose() { s.search.Dispose() }

// refilter computes the rows from the search text: a header before the first match of
// each group.
func (s *profileDialogState) refilter() {
	s.rows = s.rows[:0]
	var group ProfileGroup
	for _, c := range s.Widget().choices.Filter(s.search.Text()) {
		if c.Group != group {
			group = c.Group
			// None and Default are one entry each: the entry says it.
			if group != GroupNone && group != GroupDefault {
				s.rows = append(s.rows, profileRow{header: group})
			}
		}
		s.rows = append(s.rows, profileRow{choice: c})
	}
}

// importProfile asks for a .profile file, copies it into Jailcat's folder and picks it.
func (s *profileDialogState) importProfile() {
	if s.busy {
		return
	}
	s.busy, s.failure = true, ""
	w := s.Widget()
	bc := s.Context()
	s.Go(func(ctx context.Context) {
		res, err := desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{
			Title:       "Import a firejail profile",
			AcceptLabel: "Import",
			Filters:     []desktop.FileFilter{{Name: "Firejail profiles", Patterns: []string{"*.profile"}}},
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.Error("the file chooser failed", "err", err)
			s.SetState(func() { s.busy, s.failure = false, importChooserFailedText })
			return
		}
		if res.Cancelled || len(res.Paths) == 0 {
			s.SetState(func() { s.busy = false })
			return
		}
		src := res.Paths[0]
		path, err := w.backend.ImportProfile(ctx, src)
		var choices ProfileChoices
		if err == nil {
			choices, err = w.backend.ProfileChoices(ctx, w.app)
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.Error("importing the profile failed", "src", src, "err", err)
			s.SetState(func() { s.busy, s.failure = false, importFailedText(src, err) })
			return
		}
		s.SetState(func() {
			s.busy = false
			choice := ProfileChoice{Group: GroupImported, Name: profileName(path), Path: path, Label: profileName(path)}
			for _, c := range choices.Entries {
				if c.Group == GroupImported && c.Path == path {
					choice = c
				}
			}
			m.NavigatorPop(s.Context(), ProfileDialogResult{Choice: choice, Choices: choices, Imported: true})
		})
	})
}

// importChooserFailedText is what the user reads when the desktop's file dialog failed.
const importChooserFailedText = "The file chooser of your desktop could not be opened, so no profile was imported."

// importFailedText is what the user reads when a profile could not be imported.
func importFailedText(src string, err error) string {
	return "Jailcat could not import " + src + ": " + plainReason(err) + "."
}

// plainReason is the part of an error that is written for a person: the message of a
// tagged error, else the text of the error. The tag, the route and the metadata go to
// the log.
func plainReason(err error) string {
	if appErr, ok := errors.FromError(err); ok {
		return trimDot(appErr.Message)
	}
	return trimDot(err.Error())
}

// trimDot removes the full stop at the end of a message, so a sentence can end with one.
func trimDot(s string) string {
	for len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

func (s *profileDialogState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme

	decoration := m.NewInputDecoration(m.InputDecorationOpts{
		HintText:   m.Some(profileSearchHint),
		PrefixIcon: m.NewIcon(m.Icons.Search()),
		IsDense:    m.Some(true),
		Border:     m.NewOutlineInputBorder(),
	})
	current, _ := w.choices.Find(w.current)

	var body m.Widget
	if len(s.rows) == 0 {
		body = centeredText(bc, noProfilesText)
	} else {
		body = m.NewListViewBuilder(func(bc m.BuildContext, i int) m.Widget {
			if i >= len(s.rows) {
				return nil
			}
			row := s.rows[i]
			if row.header != "" {
				return m.NewPadding(m.NewEdgeInsetsFromLTRB(16, 0, 16, 0), m.NewAlign(
					m.NewText(string(row.header), m.TextOpts{
						Style: theme.TextTheme.LabelLarge.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.Primary())}),
					}), m.AlignOpts{Alignment: m.AlignmentCenterLeft}))
			}
			c := row.choice
			return m.NewListTile(m.ListTileOpts{
				Key:               m.NewValueKey("profile:" + string(c.Group) + ":" + c.Path),
				Title:             m.NewText(c.Label, m.TextOpts{MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis}),
				Dense:             m.Some(true),
				Selected:          c.Group == current.Group && c.Path == current.Path,
				SelectedTileColor: m.Some(scheme.SecondaryContainer()),
				SelectedColor:     m.Some(scheme.OnSecondaryContainer()),
				OnTap:             func() { m.NavigatorPop(bc, ProfileDialogResult{Choice: c}) },
			})
		}, m.ListViewOpts{ItemCount: m.Some(len(s.rows)), ItemExtent: m.Some(profileRowExtent)})
	}

	content := []m.Widget{
		m.NewTextField(m.TextFieldOpts{
			Controller: s.search,
			Decoration: &decoration,
			Autofocus:  true,
			OnChanged:  func(string) { s.SetState(s.refilter) },
		}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		m.NewExpanded(body),
	}
	if s.busy {
		content = append(content, m.NewLinearProgressIndicator())
	}
	if s.failure != "" {
		content = append(content, notice(bc, s.failure))
	}
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title: m.NewText(profileDialogTitle),
		Content: m.NewSizedBox(m.NewColumn(content, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}),
			m.SizedBoxOpts{Width: m.Some(520.0), Height: m.Some(440.0)}),
		Actions: []m.Widget{
			m.NewTextButtonIcon(s.importProfile, m.NewText(importButtonText), m.TextButtonIconOpts{Icon: m.NewIcon(m.Icons.FileOpen())}),
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText("Cancel")),
		},
	})
}
