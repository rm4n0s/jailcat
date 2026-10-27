package ui

import (
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// tabList is the body of a tab: the children in a list that scrolls with its scrollbar
// when it is taller than the window.
func tabList(tab firejail.Tab, scroll *m.ScrollControllerBase, children []m.Widget) m.Widget {
	return m.NewScrollbar(m.NewListView(children, m.ListViewOpts{
		Key: m.NewValueKey("body:" + string(tab)), Controller: scroll, Padding: m.NewEdgeInsetsSymmetric(8, 16),
	}), m.ScrollbarOpts{Controller: scroll})
}

// markedTitle is the title of a switch, a checkbox or a tile: the text, and the mark when
// the setting changed (mark is nil when it did not).
func markedTitle(label string, mark m.Widget) m.Widget {
	if mark == nil {
		return m.NewText(label)
	}
	return m.NewRow([]m.Widget{
		m.NewText(label), m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}), mark,
	}, m.RowOpts{MainAxisSize: m.MainAxisSizeMin, CrossAxisAlignment: m.CrossAxisAlignmentCenter})
}

// fieldMark is the [ChangedMark] of a field, or nil when the field has its base value.
// Reset runs through apply, which changes the editor inside the page's SetState.
func fieldMark(e *Editor, f firejail.Field, apply func(fn func(e *Editor))) m.Widget {
	return fieldMarkThen(e, f, apply, nil)
}

// fieldMarkThen is [fieldMark] for a field whose tab keeps a state of its own (a typed
// text, a switch): then runs before the reset, inside the same change, to clear it.
func fieldMarkThen(e *Editor, f firejail.Field, apply func(fn func(e *Editor)), then func()) m.Widget {
	if !e.Changed(f) {
		return nil
	}
	return NewChangedMark(func() {
		apply(func(e *Editor) {
			if then != nil {
				then()
			}
			e.Reset(f)
		})
	}, ChangedMarkOpts{Key: m.NewValueKey("mark:" + string(f))})
}

// candidateProblems returns the messages [firejail.Validate] gives about the field f for
// the editor's settings after change: it tells whether a value the user typed may be
// stored, without storing it.
func candidateProblems(e *Editor, f firejail.Field, change func(st *firejail.Settings)) []string {
	s := e.Final()
	change(&s)
	var out []string
	for _, p := range firejail.Validate(s, e.home) {
		if p.Field == f {
			out = append(out, p.Message)
		}
	}
	return out
}

// radioTile is one choice of a group of radio tiles: a rounded tile with a title and a
// line of explanation, outlined when it is selected. A tile that is not enabled is drawn
// dimmed and cannot be tapped.
func radioTile[T comparable](bc m.BuildContext, key string, value T, title, text string, selected, enabled bool) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	side := m.NewBorderSide(m.BorderSideOpts{Color: m.Some(scheme.OutlineVariant())})
	if selected {
		side = m.NewBorderSide(m.BorderSideOpts{Color: m.Some(scheme.Primary()), Width: m.Some(2.0)})
	}
	return m.NewPadding(m.NewEdgeInsetsAll(4), m.NewRadioListTile(value, m.RadioListTileOpts[T]{
		Key:               m.NewValueKey(key),
		Title:             m.NewText(title, m.TextOpts{Style: theme.TextTheme.TitleSmall}),
		Subtitle:          m.NewText(text, m.TextOpts{Style: theme.TextTheme.BodySmall, MaxLines: m.Some(3), Overflow: m.TextOverflowEllipsis}),
		Dense:             m.Some(true),
		Selected:          selected,
		Enabled:           m.Some(enabled),
		SelectedTileColor: m.Some(scheme.SecondaryContainer()),
		Shape:             m.NewRoundedRectangleBorder(m.RoundedRectangleBorderOpts{Side: side, BorderRadius: m.NewBorderRadiusCircular(12)}),
	}))
}

// tileRow puts radio tiles side by side, all as tall as the tallest.
func tileRow(tiles ...m.Widget) m.Widget {
	row := make([]m.Widget, len(tiles))
	for i, t := range tiles {
		row[i] = m.NewExpanded(t)
	}
	return m.NewIntrinsicHeight(m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}))
}

// choiceGroup makes the tiles a radio group. onChanged is called with the value the user
// taps when it differs from current.
func choiceGroup[T comparable](current T, child m.Widget, onChanged func(v T)) m.Widget {
	return m.NewRadioGroup(func(v m.Opt[T]) {
		if value, ok := v.Get(); ok && value != current {
			onChanged(value)
		}
	}, child, m.RadioGroupOpts[T]{GroupValue: m.Some(current)})
}

// subTitle is the heading of a part of a section, with the mark when that part changed.
func subTitle(bc m.BuildContext, text string, mark m.Widget) m.Widget {
	theme := m.ThemeOf(bc)
	row := []m.Widget{m.NewText(text, m.TextOpts{Style: theme.TextTheme.TitleSmall})}
	if mark != nil {
		row = append(row, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}), mark)
	}
	return m.NewPadding(m.NewEdgeInsetsFromLTRB(4, 12, 0, 0), m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
}

// problemsOf returns the messages of the problems about a field.
func problemsOf(e *Editor, f firejail.Field) []string {
	var out []string
	for _, p := range e.Problems() {
		if p.Field == f {
			out = append(out, p.Message)
		}
	}
	return out
}

// infoNote is a note that is not an error: a rounded box with an info icon.
func infoNote(bc m.BuildContext, text string, key m.Key) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	return m.NewContainer(m.NewRow([]m.Widget{
		m.NewIcon(m.Icons.Info(), m.IconOpts{Color: m.Some(scheme.OnSecondaryContainer())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewExpanded(m.NewText(text, m.TextOpts{
			Style: m.ThemeOf(bc).TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSecondaryContainer())}),
		})),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}), m.ContainerOpts{
		Key:     key,
		Color:   m.Some(scheme.SecondaryContainer()),
		Padding: m.NewEdgeInsetsAll(12),
		Margin:  m.NewEdgeInsetsSymmetric(4, 8),
	})
}

// warningText is a short line of caution, in the color of errors.
func warningText(bc m.BuildContext, text string, key m.Key) m.Widget {
	theme := m.ThemeOf(bc)
	return m.NewText(text, m.TextOpts{
		Key:      key,
		Style:    theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.Error())}),
		MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis,
	})
}
