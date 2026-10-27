package ui

import (
	m "github.com/rm4n0s/glutter/material"
)

// Texts of the Try panel the user reads (SPEC §11.2).
const (
	tryTitlePrefix  = "Try: "
	tryStartingText = "Starting…"
	tryRunningText  = "Running"
	tryStoppingText = "Stopping…"
	tryEndedPrefix  = "Ended: "
	tryNoOutputText = "No output yet."
	tryDroppedText  = "Earlier lines were dropped to keep the panel light."
	tryStopText     = "Stop"
	tryCloseText    = "Close"
	// tryNestedText explains why a Try inside another sandbox shows nothing about the
	// build (firejail's "an existing sandbox was detected").
	tryNestedText = "Jailcat itself runs inside another sandbox, and firejail cannot sandbox a program from inside one: " +
		"it started the program without any sandbox. This Try tells you nothing about the build. " +
		"Start Jailcat outside of the other sandbox to try it."
	// tryPanelHeight is the height of the output of the panel.
	tryPanelHeight = 200.0
	// tryLineExtent is the height of a line of output; every line has it, so the list lays
	// out thousands of lines without measuring them.
	tryLineExtent = monoSize * monoHeight
)

// TryPanelOpts are the optional parameters of [NewTryPanel].
type TryPanelOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// Scroll is the controller of the output, so the owner can keep it at the end.
	Scroll *m.ScrollControllerBase
	// OnStop is called when the user presses Stop.
	OnStop func()
	// OnClose is called when the user closes the panel of a Try that has ended.
	OnClose func()
}

// TryPanel is the panel at the bottom of the editor while a Try runs and after it
// (SPEC §11.2): firejail's output as it comes, its state and a **Stop** button. The
// output is a lazy list, so a long output costs only the lines on screen.
type TryPanel struct {
	m.StatelessWidget
	snap TrySnapshot
	o    TryPanelOpts
}

// NewTryPanel creates the panel for what the Try shows now.
func NewTryPanel(snap TrySnapshot, opts ...TryPanelOpts) *TryPanel {
	var o TryPanelOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &TryPanel{StatelessWidget: m.NewStatelessWidgetBase(m.WidgetOpts{Key: o.Key}), snap: snap, o: o}
}

// statusText says in words where the Try is.
func (w *TryPanel) statusText() string {
	switch w.snap.Phase {
	case TryStarting:
		return tryStartingText
	case TryRunning:
		if w.snap.Stopping {
			return tryStoppingText
		}
		return tryRunningText
	case TryEnded:
		if w.snap.StartErr != nil {
			return "Could not start"
		}
		return tryEndedPrefix + w.snap.Status.String()
	}
	return ""
}

// Build builds the panel.
func (w *TryPanel) Build(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	snap := w.snap
	active := snap.Phase == TryStarting || snap.Phase == TryRunning

	var action m.Widget
	if active {
		var onStop func()
		if !snap.Stopping {
			onStop = w.o.OnStop
		}
		action = m.NewOutlinedButton(onStop, m.NewText(tryStopText), m.OutlinedButtonOpts{Key: m.NewValueKey("try:stop")})
	} else {
		action = m.NewTextButton(w.o.OnClose, m.NewText(tryCloseText), m.TextButtonOpts{Key: m.NewValueKey("try:close")})
	}
	header := m.NewRow([]m.Widget{
		m.NewText(tryTitlePrefix+snap.Name, m.TextOpts{Style: theme.TextTheme.TitleSmall}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
		m.NewExpanded(m.NewText(w.statusText(), m.TextOpts{
			Key:   m.NewValueKey("try:status"),
			Style: theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSurfaceVariant())}),
		})),
		action,
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})

	children := []m.Widget{header}
	if snap.Nested {
		children = append(children, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), infoNote(bc, tryNestedText, m.NewValueKey("try:nested"))))
	}
	if snap.Failure != "" {
		children = append(children, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), warningText(bc, snap.Failure, m.NewValueKey("try:failure"))))
	}
	if snap.Dropped > 0 {
		children = append(children, mutedText(bc, tryDroppedText, m.TextOpts{Key: m.NewValueKey("try:dropped")}))
	}
	children = append(children, m.NewSizedBox(w.output(bc), m.SizedBoxOpts{Height: m.Some(tryPanelHeight)}))
	return m.NewContainer(m.NewColumn(children, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}), m.ContainerOpts{
		Key:     m.NewValueKey("try:panel"),
		Color:   m.Some(scheme.SurfaceContainerLow()),
		Padding: m.NewEdgeInsetsFromLTRB(16, 8, 16, 8),
	})
}

// output is the lines of firejail's output.
func (w *TryPanel) output(bc m.BuildContext) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	lines := w.snap.Lines
	var body m.Widget
	if len(lines) == 0 {
		body = m.NewPadding(m.NewEdgeInsetsAll(8), mutedText(bc, tryNoOutputText, m.TextOpts{Key: m.NewValueKey("try:empty")}))
	} else {
		style := monoStyle(bc, scheme.OnSurface())
		body = m.NewScrollbar(m.NewListViewBuilder(func(bc m.BuildContext, i int) m.Widget {
			if i >= len(lines) {
				return nil
			}
			return m.NewText(lines[i], m.TextOpts{Style: style, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})
		}, m.ListViewOpts{
			Key: m.NewValueKey("try:lines"), ItemCount: m.Some(len(lines)), ItemExtent: m.Some(tryLineExtent),
			Controller: w.o.Scroll, Padding: m.NewEdgeInsetsSymmetric(4, 8),
		}), m.ScrollbarOpts{Controller: w.o.Scroll})
	}
	return m.NewContainer(body, m.ContainerOpts{
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color:        m.Some(scheme.Surface()),
			Border:       m.NewBorderAll(m.BorderSideOpts{Color: m.Some(scheme.OutlineVariant())}),
			BorderRadius: m.NewBorderRadiusCircular(6),
		}),
	})
}
