package ui

import (
	"context"

	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/icons"
)

// defaultIconSize is the size of an app's icon in the list, in logical pixels.
const defaultIconSize = 36.0

// AppIconOpts are the optional parameters of [NewAppIcon].
type AppIconOpts struct {
	// Key is the key of the widget.
	Key m.Key
	// Size is the width and height of the icon; the default is 36.
	Size float64
}

// AppIcon draws the icon of an app (SPEC §10.4). It looks the icon up through the backend
// off the UI thread and draws an SVG with Glutter's svg package and a PNG with its image
// widget. While the lookup runs, when the app has no icon, for an XPM and when the
// picture cannot be drawn it draws a generic icon: a terminal for a binary, a grid for
// a menu app.
type AppIcon struct {
	m.StatelessWidget
	backend Backend
	app     apps.App
	size    float64
}

// NewAppIcon creates the icon of app.
func NewAppIcon(backend Backend, app apps.App, opts ...AppIconOpts) *AppIcon {
	var o AppIconOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Size <= 0 {
		o.Size = defaultIconSize
	}
	return &AppIcon{StatelessWidget: m.NewStatelessWidgetBase(m.WidgetOpts{Key: o.Key}), backend: backend, app: app, size: o.Size}
}

// App is the app whose icon is drawn.
func (w *AppIcon) App() apps.App { return w.app }

// Build builds the icon.
func (w *AppIcon) Build(bc m.BuildContext) m.Widget {
	generic := genericIcon(w.app, w.size)
	// A binary has no Icon= and a menu app may have none: there is nothing to look up.
	if w.app.Kind == apps.KindBinary || w.app.Icon == "" {
		return generic
	}
	backend, app, size := w.backend, w.app, w.size
	lookup := func(ctx context.Context) (icons.Icon, error) { return backend.LookupIcon(ctx, app) }
	return m.NewFutureBuilder(lookup, func(bc m.BuildContext, snap m.AsyncSnapshot[icons.Icon]) m.Widget {
		if !snap.HasData() {
			return generic
		}
		icon := snap.Data()
		switch icon.Format {
		case icons.FormatSVG:
			return svg.NewSvgPictureFile(icon.Path, svg.SvgPictureOpts{
				Width:                m.Some(size),
				Height:               m.Some(size),
				ExcludeFromSemantics: true,
				PlaceholderBuilder:   func(m.BuildContext) m.Widget { return generic },
				ErrorBuilder:         func(m.BuildContext, error) m.Widget { return generic },
			})
		case icons.FormatPNG:
			return m.NewImageFile(icon.Path, m.ImageFileOpts{
				ImageOpts: m.ImageOpts{
					Width:                m.Some(size),
					Height:               m.Some(size),
					Fit:                  m.BoxFitContain,
					ExcludeFromSemantics: true,
					ErrorBuilder:         func(m.BuildContext, error) m.Widget { return generic },
				},
				CacheWidth: m.Some(int(size * 2)),
			})
		}
		// XPM and anything else Jailcat cannot draw.
		return generic
	}, m.FutureBuilderOpts[icons.Icon]{Identity: app.Icon})
}

// genericIcon is the icon of an app that has no picture to show.
func genericIcon(app apps.App, size float64) m.Widget {
	data := m.Icons.Apps()
	if app.Kind == apps.KindBinary {
		data = m.Icons.Terminal()
	}
	return m.NewSizedBox(m.NewIcon(data, m.IconOpts{Size: m.Some(size * 0.78)}),
		m.SizedBoxOpts{Width: m.Some(size), Height: m.Some(size)})
}
