package icons

import (
	"context"
	"os"
	"path/filepath"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// CurrentTheme returns the name of the user's icon theme. It reads, in this order and
// without running anything: gtk-icon-theme-name in the [Settings] group of
// <ConfigHome>/gtk-4.0/settings.ini and of <ConfigHome>/gtk-3.0/settings.ini, then Theme
// in the [Icons] group of <ConfigHome>/kdeglobals. When none names a theme, it is
// "hicolor". A file that cannot be read is skipped, and a cancelled ctx gives "hicolor".
func CurrentTheme(ctx context.Context, dirs xdg.Dirs) string {
	sources := []struct{ file, group, key string }{
		{filepath.Join("gtk-4.0", "settings.ini"), "Settings", "gtk-icon-theme-name"},
		{filepath.Join("gtk-3.0", "settings.ini"), "Settings", "gtk-icon-theme-name"},
		{"kdeglobals", "Icons", "Theme"},
	}
	for _, s := range sources {
		if ctx.Err() != nil {
			return defaultTheme
		}
		data, err := os.ReadFile(filepath.Join(dirs.ConfigHome, s.file))
		if err != nil {
			continue
		}
		if v, ok := parseINI(data).get(s.group, s.key); ok && validName(v) {
			return v
		}
	}
	return defaultTheme
}
