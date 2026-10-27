package ui

import (
	m "github.com/rm4n0s/glutter/material"
)

// Texts of the marks that show a setting differs from its base profile (SPEC §6).
const (
	// changedText is the small label next to a setting the user changed.
	changedText = "changed"
	// resetText is the button that puts a changed setting back to the profile's value.
	resetText = "Reset to profile"
)

// ChangedMarkOpts are the optional parameters of [NewChangedMark].
type ChangedMarkOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// ChangedMark is the small "changed" label with the **Reset to profile** button that
// stands next to a setting whose value differs from the base profile's (SPEC §6). Every
// tab uses it for each of its fields.
type ChangedMark struct {
	m.StatelessWidget
	onReset func()
}

// NewChangedMark creates the mark. onReset is called when the user presses Reset to
// profile; the caller removes the override.
func NewChangedMark(onReset func(), opts ...ChangedMarkOpts) *ChangedMark {
	var o ChangedMarkOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &ChangedMark{StatelessWidget: m.NewStatelessWidgetBase(m.WidgetOpts{Key: o.Key}), onReset: onReset}
}

// Build builds the mark.
func (w *ChangedMark) Build(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	compact := m.TextButtonStyleFrom(m.ButtonStyleFromOpts{
		Padding:     m.NewEdgeInsetsSymmetric(0, 8),
		MinimumSize: m.Some(m.Size{Width: 0, Height: 28}),
		TextStyle:   theme.TextTheme.LabelMedium,
	})
	return m.NewRow([]m.Widget{
		badge(bc, changedText, scheme.TertiaryContainer(), scheme.OnTertiaryContainer()),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(4.0)}),
		m.NewTextButton(w.onReset, m.NewText(resetText), m.TextButtonOpts{Style: compact}),
	}, m.RowOpts{MainAxisSize: m.MainAxisSizeMin})
}

// ChangeDot is the dot on the name of a tab that has a changed setting (SPEC §6).
type ChangeDot struct {
	m.StatelessWidget
}

// NewChangeDot creates the dot.
func NewChangeDot(opts ...m.WidgetOpts) *ChangeDot {
	return &ChangeDot{StatelessWidget: m.NewStatelessWidgetBase(opts...)}
}

// Build builds the dot.
func (w *ChangeDot) Build(bc m.BuildContext) m.Widget {
	return m.NewContainer(nil, m.ContainerOpts{
		Width:  m.Some(8.0),
		Height: m.Some(8.0),
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color: m.Some(m.ThemeOf(bc).ColorScheme.Tertiary()),
			Shape: m.BoxShapeCircle,
		}),
	})
}
