package icons

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/errors"
)

func TestLookup(t *testing.T) {
	root := fixtureRoot(t)
	dirs := fixtureDirs(t, "")
	tests := []struct {
		name   string
		theme  string
		value  string
		want   string // below testdata/fake; "" means absolute, see wantAbs
		format Format
		size   int
	}{
		// Absolute paths.
		{name: "absolute png", theme: "Test", value: filepath.Join(root, "abs/icon.png"), want: "abs/icon.png", format: FormatPNG},
		{name: "absolute svg", theme: "Test", value: filepath.Join(root, "abs/icon.svg"), want: "abs/icon.svg", format: FormatSVG},
		{name: "absolute xpm, upper case extension", theme: "Test", value: filepath.Join(root, "abs/ICON.XPM"), want: "abs/ICON.XPM", format: FormatXPM},

		// A name with an extension is the name.
		{name: "name with .png finds the svg", theme: "Test", value: "scalable-wins.png", want: "sys/icons/Test/scalable/apps/scalable-wins.svg", format: FormatSVG},
		{name: "name with .svg", theme: "Test", value: "fixed-largest.svg", want: "sys/icons/Test/256x256/apps/fixed-largest.png", format: FormatPNG, size: 256},
		{name: "dots that are not an extension stay", theme: "Test", value: "org.example.App", want: "sys/icons/hicolor/48x48/apps/org.example.App.png", format: FormatPNG, size: 48},

		// Size choice inside a theme.
		{name: "scalable svg beats a 256px png", theme: "Test", value: "scalable-wins", want: "sys/icons/Test/scalable/apps/scalable-wins.svg", format: FormatSVG},
		{name: "largest fixed size", theme: "Test", value: "fixed-largest", want: "sys/icons/Test/256x256/apps/fixed-largest.png", format: FormatPNG, size: 256},
		{name: "256px png beats a 48px svg", theme: "Test", value: "big-png-beats-small-svg", want: "sys/icons/Test/256x256/apps/big-png-beats-small-svg.png", format: FormatPNG, size: 256},
		{name: "svg before png at the same size", theme: "Test", value: "svg-before-png", want: "sys/icons/Test/48x48/apps/svg-before-png.svg", format: FormatSVG, size: 48},
		{name: "svg before png at the same size in another directory", theme: "Test", value: "tie", want: "sys/icons/Test/48x48/extra/tie.svg", format: FormatSVG, size: 48},
		{name: "svg in a fixed directory", theme: "Test", value: "svg-in-fixed", want: "sys/icons/Test/48x48/apps/svg-in-fixed.svg", format: FormatSVG, size: 48},
		{name: "size 128 at scale 2 counts as 256 and beats 16", theme: "Test", value: "scaled", want: "sys/icons/Test/128x128@2/apps/scaled.png", format: FormatPNG, size: 256},
		{name: "size 256 at scale 2 beats 256", theme: "Test", value: "scaled-more", want: "sys/icons/Test/256x256@2/apps/scaled-more.png", format: FormatPNG, size: 512},
		{name: "png in a scalable directory counts at its MaxSize", theme: "Test", value: "png-in-scalable", want: "sys/icons/Test/scalable/apps/png-in-scalable.png", format: FormatPNG, size: 512},
		{name: "only a small png", theme: "Test", value: "small-only", want: "sys/icons/Test/48x48/apps/small-only.png", format: FormatPNG, size: 48},

		// The theme chain.
		{name: "icon only in the parent", theme: "Test", value: "parent-only", want: "sys/icons/Parent/32x32/apps/parent-only.png", format: FormatPNG, size: 32},
		{name: "the first theme wins, even with a smaller icon", theme: "Test", value: "both-themes", want: "sys/icons/Test/16x16/apps/both-themes.png", format: FormatPNG, size: 16},
		{name: "the child's png beats the parent's svg", theme: "Test", value: "parent-scalable", want: "sys/icons/Test/256x256/apps/parent-scalable.png", format: FormatPNG, size: 256},
		{name: "icon only in hicolor, through the parent", theme: "Test", value: "hicolor-only", want: "sys/icons/hicolor/48x48/apps/hicolor-only.png", format: FormatPNG, size: 48},
		{name: "scalable icon only in hicolor", theme: "Test", value: "hicolor-only-svg", want: "sys/icons/hicolor/scalable/apps/hicolor-only-svg.svg", format: FormatSVG},
		{name: "hicolor is the last theme", theme: "Test", value: "test-and-hicolor", want: "sys/icons/Test/16x16/apps/test-and-hicolor.png", format: FormatPNG, size: 16},
		{name: "a theme that does not exist falls back to hicolor", theme: "Nope", value: "hicolor-only", want: "sys/icons/hicolor/48x48/apps/hicolor-only.png", format: FormatPNG, size: 48},
		{name: "an invalid theme name falls back to hicolor", theme: "../Test", value: "hicolor-only", want: "sys/icons/hicolor/48x48/apps/hicolor-only.png", format: FormatPNG, size: 48},
		{name: "an empty theme name falls back to hicolor", theme: "", value: "hicolor-only", want: "sys/icons/hicolor/48x48/apps/hicolor-only.png", format: FormatPNG, size: 48},
		{name: "hicolor asked for by name", theme: "hicolor", value: "hicolor-only", want: "sys/icons/hicolor/48x48/apps/hicolor-only.png", format: FormatPNG, size: 48},
		{name: "loop in Inherits ends and hicolor is still reached", theme: "LoopA", value: "loop-icon", want: "sys/icons/hicolor/48x48/apps/loop-icon.png", format: FormatPNG, size: 48},
		{name: "theme with no index.theme is skipped", theme: "NoIndex", value: "noindex-and-hicolor", want: "sys/icons/hicolor/48x48/apps/noindex-and-hicolor.png", format: FormatPNG, size: 48},

		// Base directories, in order.
		{name: "~/.icons comes first", theme: "Test", value: "dup", want: "home/.icons/Test/48x48/apps/dup.png", format: FormatPNG, size: 48},
		{name: "then $XDG_DATA_HOME/icons", theme: "Test", value: "dup-data", want: "data/icons/Test/48x48/apps/dup-data.png", format: FormatPNG, size: 48},
		{name: "a theme's folders are merged across base directories", theme: "Test", value: "in-dot-icons", want: "home/.icons/Test/48x48/apps/in-dot-icons.png", format: FormatPNG, size: 48},
		{name: "the parent's index.theme is in another base directory than its icons", theme: "Test", value: "dot-icons-parent", want: "home/.icons/Parent/32x32/apps/dot-icons-parent.png", format: FormatPNG, size: 32},

		// XPM.
		{name: "xpm only", theme: "Test", value: "xpm-only", want: "sys/icons/Test/48x48/apps/xpm-only.xpm", format: FormatXPM, size: 48},
		{name: "a png in the pixmaps folder beats a themed xpm", theme: "Test", value: "xpm-vs-pixmap", want: "sys/pixmaps/xpm-vs-pixmap.png", format: FormatPNG},
		{name: "an unthemed png beats a themed xpm", theme: "Test", value: "xpm-vs-unthemed", want: "sys/icons/xpm-vs-unthemed.png", format: FormatPNG},

		// The unthemed fallback.
		{name: "unthemed svg before png", theme: "Test", value: "unthemed-svg", want: "sys/icons/unthemed-svg.svg", format: FormatSVG},
		{name: "unthemed png", theme: "Test", value: "unthemed-png", want: "sys/icons/unthemed-png.png", format: FormatPNG},
		{name: "unthemed xpm", theme: "Test", value: "unthemed-xpm", want: "sys/icons/unthemed-xpm.xpm", format: FormatXPM},
		{name: "unthemed in $XDG_DATA_HOME/icons", theme: "Test", value: "unthemed-data", want: "data/icons/unthemed-data.png", format: FormatPNG},
		{name: "a theme folder is not an icon", theme: "Test", value: "Test", want: "sys/icons/Test.png", format: FormatPNG},
		{name: "pixmaps", theme: "Test", value: "pixmap-only", want: "sys/pixmaps/pixmap-only.png", format: FormatPNG},
		{name: "pixmaps svg before png", theme: "Test", value: "pixmap-svg", want: "sys/pixmaps/pixmap-svg.svg", format: FormatSVG},
		{name: "pixmaps xpm", theme: "Test", value: "pixmap-xpm", want: "sys/pixmaps/pixmap-xpm.xpm", format: FormatXPM},
		{name: "base directories come before pixmaps", theme: "Test", value: "shadowed", want: "sys/icons/shadowed.png", format: FormatPNG},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lookup(context.Background(), dirs, tt.theme, tt.value)
			if err != nil {
				t.Fatalf("Lookup(%q, %q): %v", tt.theme, tt.value, err)
			}
			want := Icon{Path: filepath.Join(root, tt.want), Format: tt.format, Size: tt.size}
			if got != want {
				t.Errorf("Lookup(%q, %q) = %+v, want %+v", tt.theme, tt.value, got, want)
			}
		})
	}
}

func TestLookupNotFound(t *testing.T) {
	root := fixtureRoot(t)
	dirs := fixtureDirs(t, "")
	tests := []struct {
		name  string
		theme string
		value string
		route string
	}{
		{"no such name", "Test", "no-such-icon", "Lookup.IconNotFound"},
		{"empty value", "Test", "", "Lookup.IconNotFound"},
		{"name with a slash", "Test", "apps/hicolor-only", "Lookup.IconNotFound"},
		{"relative path", "Test", "./hicolor-only", "Lookup.IconNotFound"},
		{"dot dot", "Test", "..", "Lookup.IconNotFound"},
		{"extension only", "Test", ".png", "Lookup.IconNotFound"},
		{"icon only in a theme with no index.theme", "NoIndex", "noindex-only", "Lookup.IconNotFound"},
		{"a loop of themes", "LoopA", "no-such-icon", "Lookup.IconNotFound"},
		{"absolute path that is missing", "Test", filepath.Join(root, "abs/missing.png"), "Lookup->lookupAbsolute.IconNotFound"},
		{"absolute path of another format", "Test", filepath.Join(root, "abs/icon.txt"), "Lookup->lookupAbsolute.IconNotFound"},
		{"absolute path without a theme folder", "Test", filepath.Join(root, "abs"), "Lookup->lookupAbsolute.IconNotFound"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lookup(context.Background(), dirs, tt.theme, tt.value)
			if err == nil {
				t.Fatalf("Lookup = %+v, want an error", got)
			}
			appErr := requireRoute(t, err, tt.route)
			if appErr.Tag != TagIconNotFound {
				t.Errorf("tag = %s", appErr.Tag)
			}
			if appErr.Metadata["name"] != tt.value {
				t.Errorf("metadata = %v, want name %q", appErr.Metadata, tt.value)
			}
			if got != (Icon{}) {
				t.Errorf("Icon = %+v on error", got)
			}
		})
	}
}

func TestLookupAbsoluteDirectory(t *testing.T) {
	// A folder called icon.png is not an icon.
	dirs, tmp := tempDirs(t)
	writeFile(t, filepath.Join(tmp, "icon.png", "x"), "")
	_, err := Lookup(context.Background(), dirs, "hicolor", filepath.Join(tmp, "icon.png"))
	_ = requireRoute(t, err, "Lookup->lookupAbsolute.IconNotFound")
}

func TestLookupIOErrors(t *testing.T) {
	const themeIndex = "[Icon Theme]\nDirectories=48x48/apps,scalable/apps\n\n[48x48/apps]\nSize=48\n\n[scalable/apps]\nSize=48\nType=Scalable\n"
	tests := []struct {
		name  string
		setup func(t *testing.T, tmp string) (themeName, value string)
		route string
	}{
		{
			name: "absolute path that cannot be examined",
			setup: func(t *testing.T, tmp string) (string, string) {
				symlinkToSelf(t, filepath.Join(tmp, "loop.png"))
				return "hicolor", filepath.Join(tmp, "loop.png")
			},
			route: "Lookup->lookupAbsolute.IconStatFailed",
		},
		{
			name: "fixed directory",
			setup: func(t *testing.T, tmp string) (string, string) {
				writeFile(t, filepath.Join(tmp, "sys/icons/T/index.theme"), themeIndex)
				symlinkToSelf(t, filepath.Join(tmp, "sys/icons/T/48x48/apps/loop.png"))
				return "T", "loop"
			},
			route: "Lookup->chain.walk->findInTheme.IconStatFailed",
		},
		{
			name: "scalable directory",
			setup: func(t *testing.T, tmp string) (string, string) {
				writeFile(t, filepath.Join(tmp, "sys/icons/T/index.theme"), themeIndex)
				symlinkToSelf(t, filepath.Join(tmp, "sys/icons/T/scalable/apps/loop.svg"))
				return "T", "loop"
			},
			route: "Lookup->chain.walk->findInTheme.IconStatFailed",
		},
		{
			name: "unthemed",
			setup: func(t *testing.T, tmp string) (string, string) {
				symlinkToSelf(t, filepath.Join(tmp, "sys/icons/loop.svg"))
				return "T", "loop"
			},
			route: "Lookup->findUnthemed.IconStatFailed",
		},
		{
			name: "theme index.theme",
			setup: func(t *testing.T, tmp string) (string, string) {
				writeFile(t, filepath.Join(tmp, "sys/icons/T/index.theme/x"), "")
				return "T", "anything"
			},
			route: "Lookup->chain.walk->LoadTheme.IndexThemeReadFailed",
		},
		{
			name: "theme folder",
			setup: func(t *testing.T, tmp string) (string, string) {
				symlinkToSelf(t, filepath.Join(tmp, "sys/icons/T"))
				return "T", "anything"
			},
			route: "Lookup->chain.walk->LoadTheme.ThemeStatFailed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs, tmp := tempDirs(t)
			theme, value := tt.setup(t, tmp)
			_, err := Lookup(context.Background(), dirs, theme, value)
			if err == nil {
				t.Fatal("Lookup succeeded")
			}
			_ = requireRoute(t, err, tt.route)
		})
	}
}

func TestLookupBrokenIndexThemeIsNotFatal(t *testing.T) {
	dirs, tmp := tempDirs(t)
	// Garbage, a directory group without a Size, and a Size that is not a number: nothing
	// usable, and the lookup goes on to the next theme.
	writeFile(t, filepath.Join(tmp, "sys/icons/Broken/index.theme"),
		"\xff\xfe garbage\n[Icon Theme]\nInherits=hicolor\nDirectories=a,b,c\n[a]\nType=Fixed\n[b]\nSize=big\n")
	writeFile(t, filepath.Join(tmp, "sys/icons/Broken/a/x.png"), "")
	writeFile(t, filepath.Join(tmp, "sys/icons/hicolor/index.theme"), "[Icon Theme]\nDirectories=48x48/apps\n[48x48/apps]\nSize=48\n")
	writeFile(t, filepath.Join(tmp, "sys/icons/hicolor/48x48/apps/x.png"), "")
	got, err := Lookup(context.Background(), dirs, "Broken", "x")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(tmp, "sys/icons/hicolor/48x48/apps/x.png"); got.Path != want || got.Size != 48 {
		t.Errorf("Lookup = %+v, want %s at 48", got, want)
	}
}

func TestLookupCancelled(t *testing.T) {
	dirs := fixtureDirs(t, "")

	// How many times a full scan of a missing icon asks the context.
	probe := newCancelAt(1 << 30)
	if _, err := Lookup(probe, dirs, "Test", "no-such-icon"); err == nil {
		t.Fatal("Lookup found an icon that does not exist")
	}
	total := int(probe.calls.Load())

	tests := []struct {
		name  string
		after int // the call of ctx.Err that reports the cancellation
		route string
	}{
		{"before anything", 1, "Lookup->chain.walk->LoadTheme.LookupCanceled"},
		{"while loading the theme", 3, "Lookup->chain.walk->LoadTheme.LookupCanceled"},
		{"at the scalable directory", 4, "Lookup->chain.walk->findInTheme.LookupCanceled"},
		{"between the directories of a theme", 8, "Lookup->chain.walk->findInTheme.LookupCanceled"},
		{"in the unthemed fallback", total, "Lookup->findUnthemed.LookupCanceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Lookup(newCancelAt(tt.after), dirs, "Test", "no-such-icon")
			if err == nil {
				t.Fatal("Lookup ignored the cancelled context")
			}
			appErr := requireRoute(t, err, tt.route)
			if appErr.Tag != TagLookupCanceled {
				t.Errorf("tag = %s", appErr.Tag)
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("the error does not wrap context.Canceled: %v", err)
			}
		})
	}

	t.Run("a context cancelled before the call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Lookup(ctx, dirs, "Test", "scalable-wins")
		_ = requireRoute(t, err, "Lookup->chain.walk->LoadTheme.LookupCanceled")
	})
}

func TestLookupIsStateless(t *testing.T) {
	// Two calls with a file added in between: nothing is cached.
	dirs, tmp := tempDirs(t)
	if _, err := Lookup(context.Background(), dirs, "hicolor", "late"); err == nil {
		t.Fatal("found an icon that does not exist yet")
	}
	writeFile(t, filepath.Join(tmp, "sys/pixmaps/late.png"), "")
	got, err := Lookup(context.Background(), dirs, "hicolor", "late")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(tmp, "sys/pixmaps/late.png"); got.Path != want {
		t.Errorf("Path = %s, want %s", got.Path, want)
	}
}
