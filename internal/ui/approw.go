package ui

import (
	"strings"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/apps"
)

// AppRowOpts are the optional parameters of [NewAppRow].
type AppRowOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// Selected highlights the row.
	Selected bool
	// Builds is the number of builds of the app that were built; above zero the row shows
	// a "built" chip.
	Builds int
	// Drafts is the number of builds of the app that were never built. An app with no
	// built build and a draft shows a "draft" chip.
	Drafts int
	// Updated shows the "firejail updated" chip: a build of the app was made with another
	// firejail version than the installed one (SPEC §13.3).
	Updated bool
	// BaseGone shows the "base profile missing" chip: a build's base profile file is gone
	// (SPEC §13.3).
	BaseGone bool
	// OnTap is called when the row is tapped. It is not called for an app that is
	// already sandboxed.
	OnTap func()
}

// AppRow is one row of the app list: the icon, the name, a second line, and chips for
// "built" and "uses firejail". An app that is already sandboxed (Flatpak, Snap) is drawn
// disabled and cannot be selected (SPEC §4.1).
type AppRow struct {
	m.StatelessWidget
	backend Backend
	app     apps.App
	o       AppRowOpts
}

// NewAppRow creates the row of app. backend draws the icon.
func NewAppRow(backend Backend, app apps.App, opts ...AppRowOpts) *AppRow {
	var o AppRowOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &AppRow{StatelessWidget: m.NewStatelessWidgetBase(m.WidgetOpts{Key: o.Key}), backend: backend, app: app, o: o}
}

// App is the app the row shows.
func (w *AppRow) App() apps.App { return w.app }

// Build builds the row.
func (w *AppRow) Build(bc m.BuildContext) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	app := w.app

	var specs []badgeSpec
	switch {
	case w.o.Builds > 0:
		specs = append(specs, badgeSpec{builtLabel(w.o.Builds), scheme.Primary(), scheme.OnPrimary()})
	case w.o.Drafts > 0:
		specs = append(specs, badgeSpec{draftLabel, scheme.SecondaryContainer(), scheme.OnSecondaryContainer()})
	}
	if app.UsesFirejail {
		specs = append(specs, badgeSpec{"uses firejail", scheme.TertiaryContainer(), scheme.OnTertiaryContainer()})
	}
	if w.o.Updated {
		specs = append(specs, badgeSpec{firejailUpdatedText, scheme.TertiaryContainer(), scheme.OnTertiaryContainer()})
	}
	if w.o.BaseGone {
		specs = append(specs, badgeSpec{baseMissingText, scheme.ErrorContainer(), scheme.OnErrorContainer()})
	}
	// The row has a fixed height: three chips or more are drawn thinner to fit.
	var badges []m.Widget
	for _, b := range specs {
		if len(specs) > 2 {
			badges = append(badges, denseBadge(bc, b.text, b.background, b.foreground))
		} else {
			badges = append(badges, badge(bc, b.text, b.background, b.foreground))
		}
	}
	var trailing m.Widget
	if len(badges) > 0 {
		var spaced []m.Widget
		for i, b := range badges {
			if i > 0 {
				spaced = append(spaced, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}))
			}
			spaced = append(spaced, b)
		}
		trailing = m.NewColumn(spaced, m.ColumnOpts{
			MainAxisAlignment:  m.MainAxisAlignmentCenter,
			MainAxisSize:       m.MainAxisSizeMin,
			CrossAxisAlignment: m.CrossAxisAlignmentEnd,
		})
	}

	onTap := w.o.OnTap
	if app.AlreadySandboxed {
		onTap = nil
	}
	return m.NewListTile(m.ListTileOpts{
		Leading:           NewAppIcon(w.backend, app),
		Title:             m.NewText(app.Name, m.TextOpts{MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis}),
		Subtitle:          m.NewText(secondLine(app), m.TextOpts{MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis}),
		Trailing:          trailing,
		Selected:          w.o.Selected,
		SelectedTileColor: m.Some(scheme.SecondaryContainer()),
		SelectedColor:     m.Some(scheme.OnSecondaryContainer()),
		Disabled:          app.AlreadySandboxed,
		OnTap:             onTap,
		ContentPadding:    m.NewEdgeInsetsSymmetric(0, 12),
	})
}

// draftLabel is the text of the chip of an app that has only builds that were never built.
const draftLabel = "draft"

// secondLine is the small text under an app's name: why it cannot be built, the path of a
// binary, or the generic name or comment of a menu app.
func secondLine(app apps.App) string {
	switch {
	case app.AlreadySandboxed:
		kind := app.SandboxKind
		if kind != "" {
			kind = strings.ToUpper(kind[:1]) + kind[1:]
		}
		return "already sandboxed: " + kind
	case app.Kind == apps.KindBinary:
		return app.Binary
	case app.GenericName != "":
		return app.GenericName
	case app.Comment != "":
		return app.Comment
	}
	return app.BinaryName
}

// badge is a small rounded label.
func badge(bc m.BuildContext, text string, background, foreground m.Color) m.Widget {
	style := m.ThemeOf(bc).TextTheme.LabelSmall.CopyWith(m.TextStyleCopy{Color: m.Some(foreground)})
	return m.NewContainer(m.NewText(text, m.TextOpts{Style: style, MaxLines: m.Some(1)}), m.ContainerOpts{
		Padding: m.NewEdgeInsetsSymmetric(2, 8),
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color:        m.Some(background),
			BorderRadius: m.NewBorderRadiusCircular(8),
		}),
	})
}

// badgeSpec is the text and colors of a chip before it is drawn.
type badgeSpec struct {
	text                   string
	background, foreground m.Color
}

// denseBadge is a [badge] without the space above and below its text.
func denseBadge(bc m.BuildContext, text string, background, foreground m.Color) m.Widget {
	style := m.ThemeOf(bc).TextTheme.LabelSmall.CopyWith(m.TextStyleCopy{Color: m.Some(foreground)})
	return m.NewContainer(m.NewText(text, m.TextOpts{Style: style, MaxLines: m.Some(1)}), m.ContainerOpts{
		Padding: m.NewEdgeInsetsSymmetric(0, 8),
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color:        m.Some(background),
			BorderRadius: m.NewBorderRadiusCircular(8),
		}),
	})
}
