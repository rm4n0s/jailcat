package ui

import (
	"strconv"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// UninstallAnswer is what the Uninstall dialog pops with. The zero value is "closed
// without a choice".
type UninstallAnswer struct {
	// Chosen is true when the user pressed Uninstall.
	Chosen bool
	// DeleteHome is the state of "Also delete its private home".
	DeleteHome bool
	// KeepSettings is the state of "Keep its settings in Jailcat".
	KeepSettings bool
}

// What the dialog of the edited files pops with.
const (
	// RemoveKept removes the edited files anyway.
	RemoveKept = "remove"
	// LeaveKept leaves them in place.
	LeaveKept = "leave"
)

// Texts of the Uninstall dialogs (SPEC §13.2).
const (
	uninstallTitlePrefix  = "Uninstall "
	uninstallIntroText    = "This removes the files Jailcat made for this build:"
	uninstallHomeText     = "Also delete its private home"
	uninstallKeepText     = "Keep its settings in Jailcat"
	uninstallKeepHelp     = "The build stays in the list as not installed, so you can build it again."
	uninstallConfirmText  = "Uninstall"
	uninstallKeptTitle    = "Some files were changed"
	uninstallKeptText     = "These files were changed since Jailcat wrote them, so Jailcat left them where they are:"
	uninstallRemoveText   = "Remove them anyway"
	uninstallLeaveText    = "Leave them"
	uninstallDialogWidth  = 560.0
	uninstallHomeKeptText = "Its private home was kept: "
)

// UninstallDialog asks what to do when a build is uninstalled (SPEC §13.2): the files it
// removes, whether to also delete the private home (off) and whether to keep the build's
// settings (on). It pops with an [UninstallAnswer].
type UninstallDialog struct {
	m.StatefulWidget
	name  string
	files []store.InstalledFile
	home  string
}

// NewUninstallDialog creates the dialog for the build with the sandbox name and the files
// recorded for it; home is the user's home folder, which the paths are shown relative to.
func NewUninstallDialog(name string, files []store.InstalledFile, home string, opts ...m.WidgetOpts) *UninstallDialog {
	return &UninstallDialog{StatefulWidget: m.NewStatefulWidgetBase(opts...), name: name, files: files, home: home}
}

// CreateState creates the state of the dialog.
func (w *UninstallDialog) CreateState() m.State { return &uninstallDialogState{keep: true} }

type uninstallDialogState struct {
	m.StateOf[*UninstallDialog]
	deleteHome bool
	keep       bool
}

func (s *uninstallDialogState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)

	var list []m.Widget
	homePath := ""
	for i, f := range w.files {
		if f.Kind == store.FileKindHome {
			homePath = f.Path
			continue
		}
		list = append(list, m.NewText(conflictKindName(install.Kind(f.Kind))+": "+displayPlainPath(f.Path, w.home),
			m.TextOpts{Style: theme.TextTheme.BodySmall, Key: m.NewValueKey("uninstall:file:" + strconv.Itoa(i))}))
	}
	content := []m.Widget{m.NewText(uninstallIntroText, m.TextOpts{Style: theme.TextTheme.BodyMedium}), m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(6.0)})}
	if len(list) == 0 {
		content = append(content, mutedText(bc, "No file is recorded for this build."))
	}
	content = append(content, list...)
	content = append(content, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(12.0)}))
	if homePath != "" {
		content = append(content, m.NewCheckboxListTile(m.Some(s.deleteHome), func(v m.Opt[bool]) {
			on, _ := v.Get()
			s.SetState(func() { s.deleteHome = on })
		}, m.CheckboxListTileOpts{
			Key: m.NewValueKey("uninstall:home"), Title: m.NewText(uninstallHomeText),
			Subtitle: m.NewText(displayPlainPath(homePath, w.home)), Dense: m.Some(true),
			ContentPadding: m.NewEdgeInsetsSymmetric(0, 0), ControlAffinity: m.ListTileControlAffinityLeading,
		}))
	}
	content = append(content, m.NewCheckboxListTile(m.Some(s.keep), func(v m.Opt[bool]) {
		on, _ := v.Get()
		s.SetState(func() { s.keep = on })
	}, m.CheckboxListTileOpts{
		Key: m.NewValueKey("uninstall:keep"), Title: m.NewText(uninstallKeepText), Subtitle: m.NewText(uninstallKeepHelp),
		Dense: m.Some(true), ContentPadding: m.NewEdgeInsetsSymmetric(0, 0), ControlAffinity: m.ListTileControlAffinityLeading,
	}))

	return m.NewAlertDialog(m.AlertDialogOpts{
		Title: m.NewText(uninstallTitlePrefix + w.name + "?"),
		Content: m.NewSizedBox(m.NewListView(content, m.ListViewOpts{ShrinkWrap: true}),
			m.SizedBoxOpts{Width: m.Some(uninstallDialogWidth)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText(cancelButtonText), m.TextButtonOpts{Key: m.NewValueKey("uninstall:cancel")}),
			m.NewFilledButton(func() {
				m.NavigatorPop(bc, UninstallAnswer{Chosen: true, DeleteHome: s.deleteHome, KeepSettings: s.keep})
			}, m.NewText(uninstallConfirmText), m.FilledButtonOpts{Key: m.NewValueKey("uninstall:confirm")}),
		},
	})
}

// KeptFilesDialog lists the edited files an uninstall left in place and asks whether to
// remove them anyway. It pops with [RemoveKept] or [LeaveKept]; closing it leaves them.
type KeptFilesDialog struct {
	m.StatelessWidget
	kept []install.File
	home string
}

// NewKeptFilesDialog creates the dialog for the files that were kept.
func NewKeptFilesDialog(kept []install.File, home string, opts ...m.WidgetOpts) *KeptFilesDialog {
	return &KeptFilesDialog{StatelessWidget: m.NewStatelessWidgetBase(opts...), kept: kept, home: home}
}

// Build builds the dialog.
func (w *KeptFilesDialog) Build(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	content := []m.Widget{m.NewText(uninstallKeptText, m.TextOpts{Style: theme.TextTheme.BodyMedium}), m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(6.0)})}
	for i, f := range w.kept {
		content = append(content, m.NewText(conflictKindName(f.Kind)+": "+displayPlainPath(f.Path, w.home),
			m.TextOpts{Style: theme.TextTheme.BodySmall, Key: m.NewValueKey("kept:file:" + strconv.Itoa(i))}))
	}
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title: m.NewText(uninstallKeptTitle),
		Content: m.NewSizedBox(m.NewListView(content, m.ListViewOpts{ShrinkWrap: true}),
			m.SizedBoxOpts{Width: m.Some(uninstallDialogWidth)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc, LeaveKept) }, m.NewText(uninstallLeaveText), m.TextButtonOpts{Key: m.NewValueKey("kept:leave")}),
			m.NewFilledButton(func() { m.NavigatorPop(bc, RemoveKept) }, m.NewText(uninstallRemoveText), m.FilledButtonOpts{Key: m.NewValueKey("kept:remove")}),
		},
	})
}
