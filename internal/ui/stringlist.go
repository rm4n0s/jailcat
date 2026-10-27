package ui

import (
	"strconv"
	"strings"

	m "github.com/rm4n0s/glutter/material"
)

// Texts of the list editor.
const (
	addItemText = "Add"
)

// StringListOpts are the optional parameters of [NewStringList].
type StringListOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// KeyPrefix starts the keys of the parts: "<prefix>:row:<i>", "<prefix>:remove:<i>",
	// "<prefix>:field", "<prefix>:add", "<prefix>:problem" and "<prefix>:full".
	KeyPrefix string
	// Label is the label of the field that adds an item.
	Label string
	// Hint is the hint inside the field.
	Hint string
	// Empty is the line shown when the list has no item; "" shows nothing.
	Empty string
	// Max is the most items the list holds; 0 is no limit. A full list shows FullText in
	// place of the field.
	Max int
	// FullText is shown instead of the field when the list holds Max items.
	FullText string
	// Disabled turns the field and the buttons off; the items stay readable.
	Disabled bool
}

// StringList is a list of short texts the user can add to and remove from: a row for each
// item with a remove button, and a field with an **Add** button under them. The text is
// checked before it is added and the reason is shown when it is refused, so the list
// never holds an item that [StringListCheck] refused. The widget keeps no items: the
// caller owns the list and changes it in onAdd and onRemove.
type StringList struct {
	m.StatefulWidget
	items    []string
	check    StringListCheck
	onAdd    func(item string)
	onRemove func(index int)
	o        StringListOpts
}

// StringListCheck checks the text the user typed against the items of the list. It
// returns the item to add, or a message for the user when the text cannot be added.
type StringListCheck func(text string, items []string) (item, problem string)

// NewStringList creates the list for items. check is called when the user presses Add or
// Enter; onAdd gets the item it accepted and onRemove the index of the row whose button
// was pressed. Both run on the UI thread.
func NewStringList(items []string, check StringListCheck, onAdd func(item string), onRemove func(index int), opts ...StringListOpts) *StringList {
	var o StringListOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &StringList{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		items:          items, check: check, onAdd: onAdd, onRemove: onRemove, o: o,
	}
}

// CreateState creates the state of the list.
func (w *StringList) CreateState() m.State { return &stringListState{} }

type stringListState struct {
	m.StateOf[*StringList]
	field *m.TextEditingController
	// failure is why the last text was refused; "" for none.
	failure string
}

func (s *stringListState) InitState() { s.field = m.NewTextEditingController() }

func (s *stringListState) Dispose() { s.field.Dispose() }

// submit checks the text and adds it.
func (s *stringListState) submit() {
	w := s.Widget()
	if strings.TrimSpace(s.field.Text()) == "" {
		return
	}
	item, problem := w.check(s.field.Text(), w.items)
	if problem != "" {
		s.SetState(func() { s.failure = problem })
		return
	}
	s.field.Clear()
	s.SetState(func() { s.failure = "" })
	w.onAdd(item)
}

func (s *stringListState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	theme := m.ThemeOf(bc)
	prefix := w.o.KeyPrefix
	var col []m.Widget

	if len(w.items) == 0 && w.o.Empty != "" {
		col = append(col, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 4), mutedText(bc, w.o.Empty)))
	}
	for i, item := range w.items {
		i := i
		var remove func()
		if !w.o.Disabled {
			remove = func() { w.onRemove(i) }
		}
		col = append(col, m.NewPadding(m.NewEdgeInsetsSymmetric(0, 4), m.NewRow([]m.Widget{
			m.NewExpanded(m.NewText(item, m.TextOpts{Style: theme.TextTheme.BodyMedium, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})),
			m.NewIconButton(remove, m.NewIcon(m.Icons.Delete()), m.IconButtonOpts{
				Key: m.NewValueKey(prefix + ":remove:" + strconv.Itoa(i)), Tooltip: "Remove from the list",
			}),
		}, m.RowOpts{Key: m.NewValueKey(prefix + ":row:" + strconv.Itoa(i)), CrossAxisAlignment: m.CrossAxisAlignmentCenter})))
	}

	if w.o.Max > 0 && len(w.items) >= w.o.Max {
		if w.o.FullText != "" {
			col = append(col, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 4), mutedText(bc, w.o.FullText, m.TextOpts{Key: m.NewValueKey(prefix + ":full")})))
		}
		return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
	}

	enabled := !w.o.Disabled
	decoration := m.NewInputDecoration(m.InputDecorationOpts{
		HintText:  m.Some(w.o.Hint),
		LabelText: m.Some(w.o.Label),
		Border:    m.NewOutlineInputBorder(),
		IsDense:   m.Some(true),
	})
	field := m.NewTextField(m.TextFieldOpts{
		Key:         m.NewValueKey(prefix + ":field"),
		Controller:  s.field,
		Decoration:  &decoration,
		Enabled:     m.Some(enabled),
		OnChanged:   func(string) { s.SetState(func() { s.failure = "" }) },
		OnSubmitted: func(string) { s.submit() },
	})
	var add func()
	if enabled {
		add = s.submit
	}
	col = append(col, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
		m.NewExpanded(field),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewOutlinedButton(add, m.NewText(addItemText), m.OutlinedButtonOpts{Key: m.NewValueKey(prefix + ":add")}),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})))
	if s.failure != "" {
		col = append(col, problemLine(bc, s.failure, m.NewValueKey(prefix+":problem")))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}
