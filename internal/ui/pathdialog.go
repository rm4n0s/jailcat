package ui

import (
	m "github.com/rm4n0s/glutter/material"
)

// PathDialog asks for the path of a folder to add to the folder list (SPEC §7.1). It
// checks the path when the user presses **Add** or Enter, and stays open with the reason
// when the path cannot be added. It pops with the path as the list keeps it; Cancel pops
// with nothing.
type PathDialog struct {
	m.StatefulWidget
	check func(path string) (canonical, problem string)
}

// NewPathDialog creates the dialog. check is [Editor.CheckFolder]: it returns the path
// to add, or a message for the user when the path cannot be added.
func NewPathDialog(check func(path string) (canonical, problem string), opts ...m.WidgetOpts) *PathDialog {
	return &PathDialog{StatefulWidget: m.NewStatefulWidgetBase(opts...), check: check}
}

// CreateState creates the state of the dialog.
func (w *PathDialog) CreateState() m.State { return &pathDialogState{} }

type pathDialogState struct {
	m.StateOf[*PathDialog]
	field *m.TextEditingController
	// failure is why the last path was refused; "" for none.
	failure string
}

func (s *pathDialogState) InitState() { s.field = m.NewTextEditingController() }

func (s *pathDialogState) Dispose() { s.field.Dispose() }

// submit checks the path and, when it is fine, closes the dialog with it.
func (s *pathDialogState) submit() {
	canonical, problem := s.Widget().check(s.field.Text())
	if problem != "" {
		s.SetState(func() { s.failure = problem })
		return
	}
	m.NavigatorPop(s.Context(), canonical)
}

func (s *pathDialogState) Build(bc m.BuildContext) m.Widget {
	decoration := m.NewInputDecoration(m.InputDecorationOpts{
		HintText:  m.Some(pathDialogHintText),
		LabelText: m.Some(pathDialogFieldLabel),
		Border:    m.NewOutlineInputBorder(),
	})
	field := m.NewTextField(m.TextFieldOpts{
		Key:         m.NewValueKey("path-field"),
		Controller:  s.field,
		Decoration:  &decoration,
		Autofocus:   true,
		OnChanged:   func(string) { s.SetState(func() { s.failure = "" }) },
		OnSubmitted: func(string) { s.submit() },
	})
	content := []m.Widget{field}
	if s.failure != "" {
		content = append(content, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}), notice(bc, s.failure))
	}
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title:   m.NewText(pathDialogTitleText),
		Content: m.NewSizedBox(m.NewColumn(content, m.ColumnOpts{MainAxisSize: m.MainAxisSizeMin, CrossAxisAlignment: m.CrossAxisAlignmentStretch}), m.SizedBoxOpts{Width: m.Some(480.0)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText(cancelButtonText)),
			m.NewFilledButton(s.submit, m.NewText(pathDialogAddText), m.FilledButtonOpts{Key: m.NewValueKey("path-add")}),
		},
	})
}
