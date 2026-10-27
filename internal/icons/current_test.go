package icons

import (
	"context"
	"testing"
)

func TestCurrentTheme(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{"gtk-4.0 comes first, over gtk-3.0", "gtk4", "FromGtk4"},
		{"gtk-3.0, spaces around =", "gtk3", "FromGtk3"},
		{"kdeglobals [Icons] Theme, not another group's Theme", "kde", "FromKde"},
		{"settings without the key", "none", "hicolor"},
		{"empty key, wrong group, malformed group: kdeglobals is next", "bad", "FromKdeAfterBad"},
		{"a value that is not a theme name", "escape", "hicolor"},
		{"no files at all", "missing", "hicolor"},
		{"config folder that does not exist", "", "hicolor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CurrentTheme(context.Background(), fixtureDirs(t, tt.config)); got != tt.want {
				t.Errorf("CurrentTheme = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCurrentThemeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := CurrentTheme(ctx, fixtureDirs(t, "gtk4")); got != "hicolor" {
		t.Errorf("CurrentTheme = %q, want hicolor", got)
	}
}
