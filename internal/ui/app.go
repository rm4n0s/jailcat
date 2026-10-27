package ui

import (
	"log/slog"

	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"
)

// App is Jailcat's Material app: Material 3 with a light and a dark theme that follow the
// system, one SVG cache for all the icons (SPEC §10.4), and the [HomePage].
type App struct {
	m.StatelessWidget
	backend Backend
	logger  *slog.Logger
	cache   *svg.Cache
}

// NewApp creates the app. backend serves the pages, logger gets what the user must not
// read.
func NewApp(backend Backend, logger *slog.Logger, opts ...m.WidgetOpts) *App {
	return &App{
		StatelessWidget: m.NewStatelessWidgetBase(opts...),
		backend:         backend,
		logger:          logger,
		cache:           svg.NewCache(),
	}
}

// seedColor is the colour both themes are made from.
var seedColor = m.Colors.DeepOrange.Primary

// Build builds the app.
func (w *App) Build(m.BuildContext) m.Widget {
	return svg.NewSvgCacheScope(w.cache, m.NewMaterialApp(m.MaterialAppOpts{
		Title: "Jailcat",
		Theme: m.NewThemeData(m.ThemeDataOpts{
			UseMaterial3: m.Some(true), ColorSchemeSeed: m.Some(seedColor), Brightness: m.BrightnessLight,
		}),
		DarkTheme: m.NewThemeData(m.ThemeDataOpts{
			UseMaterial3: m.Some(true), ColorSchemeSeed: m.Some(seedColor), Brightness: m.BrightnessDark,
		}),
		ThemeMode:                  m.ThemeModeSystem,
		DebugShowCheckedModeBanner: m.Some(false),
		Home:                       NewHomePage(w.backend, w.logger),
	}))
}
