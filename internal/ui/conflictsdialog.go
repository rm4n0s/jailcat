package ui

import (
	"strconv"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/install"
)

// ConflictAction is what the user chose in the conflicts dialog. The zero value is
// "closed without a choice".
type ConflictAction int

// The answers of the conflicts dialog.
const (
	// ConflictCancel is Cancel, or a closed dialog: nothing is written.
	ConflictCancel ConflictAction = iota
	// ConflictOverwrite is **Overwrite and build**.
	ConflictOverwrite
	// ConflictRename is **Choose another sandbox name**.
	ConflictRename
)

// ConflictAnswer is what the conflicts dialog pops with.
type ConflictAnswer struct {
	// Action says which button was pressed.
	Action ConflictAction
	// Overwrite holds the paths the user agreed to overwrite; set with [ConflictOverwrite].
	Overwrite map[string]bool
}

// Texts of the conflicts dialog (SPEC §9.5).
const (
	conflictsTitleText     = "Some files are in the way"
	conflictsIntroText     = "Build writes nothing until every file below is dealt with."
	conflictsForeignText   = "Jailcat will not overwrite a file it did not make. Choose another sandbox name, or move that file away yourself and build again."
	conflictsOverwriteText = "Overwrite and build"
	conflictsRenameText    = "Choose another sandbox name"
	conflictsOverwriteBox  = "Overwrite this file"
	conflictsDialogWidth   = 640.0
)

// conflictKindText says in plain words why a file is in the way. owner is the sandbox
// name of the build that owns it, when it is another build's.
func conflictKindText(c install.Conflict, owner string) string {
	switch c.Kind {
	case install.ConflictEdited:
		return "Jailcat wrote this file for this build, but it was changed since."
	case install.ConflictOtherBuild:
		if owner != "" {
			return "This file belongs to the build " + owner + "."
		}
		return "This file belongs to another build."
	case install.ConflictForeignJailcat:
		return "This file says Jailcat made it, but no build here owns it."
	}
	switch c.File.Kind {
	case install.KindHome:
		return "A file stands where the private home folder should be."
	case install.KindAppData:
		return "A file stands where the VS Code data folder should be."
	}
	return "This file already exists, and Jailcat did not make it."
}

// conflictKindName is the plain name of what a file is.
func conflictKindName(k install.Kind) string {
	switch k {
	case install.KindProfile:
		return "Profile"
	case install.KindLauncher:
		return "Launcher"
	case install.KindDesktop:
		return "Menu entry"
	case install.KindIcon:
		return "Icon"
	case install.KindHome:
		return "Private home"
	case install.KindAppData:
		return "VS Code data"
	}
	return string(k)
}

// ConflictsDialog lists the targets of a Build that are taken (SPEC §9.5) with a checkbox
// for each one that may be overwritten, and the buttons **Overwrite and build**,
// **Choose another sandbox name** and **Cancel**. It pops with a [ConflictAnswer].
type ConflictsDialog struct {
	m.StatefulWidget
	conflicts []install.Conflict
	owners    map[int64]string
	home      string
}

// NewConflictsDialog creates the dialog. owners maps the ID of a build to its sandbox
// name; home is the user's home folder, which the paths are shown relative to.
func NewConflictsDialog(conflicts []install.Conflict, owners map[int64]string, home string, opts ...m.WidgetOpts) *ConflictsDialog {
	return &ConflictsDialog{StatefulWidget: m.NewStatefulWidgetBase(opts...), conflicts: conflicts, owners: owners, home: home}
}

// CreateState creates the state of the dialog.
func (w *ConflictsDialog) CreateState() m.State {
	return &conflictsDialogState{checked: map[string]bool{}}
}

type conflictsDialogState struct {
	m.StateOf[*ConflictsDialog]
	checked map[string]bool
}

// blocked reports whether a conflict cannot be overwritten, whatever the user ticks.
func (s *conflictsDialogState) blocked() bool {
	for _, c := range s.Widget().conflicts {
		if !c.Overwritable {
			return true
		}
	}
	return false
}

// allChecked reports whether every overwritable conflict is ticked.
func (s *conflictsDialogState) allChecked() bool {
	for _, c := range s.Widget().conflicts {
		if c.Overwritable && !s.checked[c.File.Path] {
			return false
		}
	}
	return true
}

func (s *conflictsDialogState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme

	var rows []m.Widget
	for i, c := range w.conflicts {
		path := c.File.Path
		head := m.NewText(conflictKindName(c.File.Kind)+": "+displayPlainPath(path, w.home), m.TextOpts{
			Style: theme.TextTheme.TitleSmall, Key: m.NewValueKey("conflict:path:" + strconv.Itoa(i)),
		})
		why := mutedText(bc, conflictKindText(c, w.owners[c.Owner]), m.TextOpts{Key: m.NewValueKey("conflict:why:" + strconv.Itoa(i))})
		parts := []m.Widget{head, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(2.0)}), why}
		if c.Overwritable {
			parts = append(parts, m.NewCheckboxListTile(m.Some(s.checked[path]), func(v m.Opt[bool]) {
				on, _ := v.Get()
				s.SetState(func() { s.checked[path] = on })
			}, m.CheckboxListTileOpts{
				Key: m.NewValueKey("conflict:overwrite:" + strconv.Itoa(i)), Title: m.NewText(conflictsOverwriteBox),
				Dense: m.Some(true), ContentPadding: m.NewEdgeInsetsSymmetric(0, 0),
				ControlAffinity: m.ListTileControlAffinityLeading,
			}))
		}
		rows = append(rows, m.NewContainer(m.NewColumn(parts, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}), m.ContainerOpts{
			Padding: m.NewEdgeInsetsAll(10), Margin: m.NewEdgeInsetsSymmetric(4, 0),
			Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
				Border:       m.NewBorderAll(m.BorderSideOpts{Color: m.Some(scheme.OutlineVariant())}),
				BorderRadius: m.NewBorderRadiusCircular(8),
			}),
		}))
	}

	content := []m.Widget{m.NewText(conflictsIntroText, m.TextOpts{Style: theme.TextTheme.BodyMedium})}
	if s.blocked() {
		content = append(content, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
			warningText(bc, conflictsForeignText, m.NewValueKey("conflict:blocked")))
	}
	content = append(content, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}))
	content = append(content, rows...)

	var onOverwrite func()
	if !s.blocked() && s.allChecked() {
		onOverwrite = func() {
			allow := map[string]bool{}
			for _, c := range w.conflicts {
				allow[c.File.Path] = true
			}
			m.NavigatorPop(bc, ConflictAnswer{Action: ConflictOverwrite, Overwrite: allow})
		}
	}
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title: m.NewText(conflictsTitleText),
		Content: m.NewSizedBox(m.NewListView(content, m.ListViewOpts{ShrinkWrap: true}),
			m.SizedBoxOpts{Width: m.Some(conflictsDialogWidth)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText(cancelButtonText), m.TextButtonOpts{Key: m.NewValueKey("conflict:cancel")}),
			m.NewOutlinedButton(func() { m.NavigatorPop(bc, ConflictAnswer{Action: ConflictRename}) },
				m.NewText(conflictsRenameText), m.OutlinedButtonOpts{Key: m.NewValueKey("conflict:rename")}),
			m.NewFilledButton(onOverwrite, m.NewText(conflictsOverwriteText), m.FilledButtonOpts{Key: m.NewValueKey("conflict:build")}),
		},
	})
}
