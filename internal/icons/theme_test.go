package icons

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rm4n0s/errors"
)

func TestBaseDirsAndPixmapDirs(t *testing.T) {
	dirs := resolve(t, "/h", map[string]string{
		"XDG_DATA_HOME": "/dh",
		"XDG_DATA_DIRS": "/a:/b",
	})
	wantBases := []string{"/h/.icons", "/dh/icons", "/a/icons", "/b/icons"}
	if got := BaseDirs(dirs); !reflect.DeepEqual(got, wantBases) {
		t.Errorf("BaseDirs = %q, want %q", got, wantBases)
	}
	wantPix := []string{"/a/pixmaps", "/b/pixmaps"}
	if got := PixmapDirs(dirs); !reflect.DeepEqual(got, wantPix) {
		t.Errorf("PixmapDirs = %q, want %q", got, wantPix)
	}
}

func TestLoadTheme(t *testing.T) {
	root := fixtureRoot(t)
	dirs := fixtureDirs(t, "")
	test := func(rel string, d Directory) Directory {
		d.Path = rel
		return d
	}
	tests := []struct {
		name         string
		theme        string
		wantInherits []string
		wantDirs     []Directory
		wantRoots    []string
	}{
		{
			name:         "Test: bad lines and bad groups skipped, ScaledDirectories merged",
			theme:        "Test",
			wantInherits: []string{"Parent"},
			wantDirs: []Directory{
				test("16x16/apps", Directory{Size: 16, Scale: 1, Type: DirFixed, MinSize: 16, MaxSize: 16, Threshold: 2}),
				test("48x48/apps", Directory{Size: 48, Scale: 1, Type: DirThreshold, MinSize: 48, MaxSize: 48, Threshold: 4}),
				test("48x48/extra", Directory{Size: 48, Scale: 1, Type: DirFixed, MinSize: 48, MaxSize: 48, Threshold: 2}),
				test("256x256/apps", Directory{Size: 256, Scale: 1, Type: DirFixed, MinSize: 256, MaxSize: 256, Threshold: 2}),
				test("128x128@2/apps", Directory{Size: 128, Scale: 2, Type: DirFixed, MinSize: 128, MaxSize: 128, Threshold: 2}),
				test("scalable/apps", Directory{Size: 48, Scale: 1, Type: DirScalable, MinSize: 8, MaxSize: 512, Threshold: 2}),
				test("256x256@2/apps", Directory{Size: 256, Scale: 2, Type: DirFixed, MinSize: 256, MaxSize: 256, Threshold: 2}),
			},
			wantRoots: []string{
				filepath.Join(root, "home/.icons/Test"),
				filepath.Join(root, "data/icons/Test"),
				filepath.Join(root, "sys/icons/Test"),
			},
		},
		{
			name:         "Parent inherits hicolor; its folder in ~/.icons has no index.theme",
			theme:        "Parent",
			wantInherits: []string{"hicolor"},
			wantDirs: []Directory{
				test("32x32/apps", Directory{Size: 32, Scale: 1, Type: DirFixed, MinSize: 32, MaxSize: 32, Threshold: 2}),
				test("scalable/apps", Directory{Size: 64, Scale: 1, Type: DirScalable, MinSize: 16, MaxSize: 256, Threshold: 2}),
			},
			wantRoots: []string{
				filepath.Join(root, "home/.icons/Parent"),
				filepath.Join(root, "sys/icons/Parent"),
			},
		},
		{
			name:         "invalid parent names are dropped",
			theme:        "LoopA",
			wantInherits: []string{"LoopB", "Missing"},
			wantDirs: []Directory{
				test("48x48/apps", Directory{Size: 48, Scale: 1, Type: DirThreshold, MinSize: 48, MaxSize: 48, Threshold: 2}),
			},
			wantRoots: []string{filepath.Join(root, "sys/icons/LoopA")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, err := LoadTheme(context.Background(), dirs, tt.theme)
			if err != nil {
				t.Fatal(err)
			}
			if th.Name != tt.theme {
				t.Errorf("Name = %q", th.Name)
			}
			if !reflect.DeepEqual(th.Inherits, tt.wantInherits) {
				t.Errorf("Inherits = %q, want %q", th.Inherits, tt.wantInherits)
			}
			if !reflect.DeepEqual(th.Directories, tt.wantDirs) {
				t.Errorf("Directories = %+v\nwant %+v", th.Directories, tt.wantDirs)
			}
			if !reflect.DeepEqual(th.roots, tt.wantRoots) {
				t.Errorf("roots = %q, want %q", th.roots, tt.wantRoots)
			}
		})
	}
}

func TestDirectoryPixels(t *testing.T) {
	tests := []struct {
		name string
		d    Directory
		want int
	}{
		{"fixed", Directory{Type: DirFixed, Size: 48, Scale: 1}, 48},
		{"scaled", Directory{Type: DirFixed, Size: 48, Scale: 2}, 96},
		{"threshold", Directory{Type: DirThreshold, Size: 32, Scale: 1, MaxSize: 99}, 32},
		{"scalable counts at MaxSize", Directory{Type: DirScalable, Size: 48, Scale: 1, MaxSize: 512}, 512},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.pixels(); got != tt.want {
				t.Errorf("pixels = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLoadThemeErrors(t *testing.T) {
	dirs := fixtureDirs(t, "")
	tests := []struct {
		name  string
		theme string
	}{
		{"folder without index.theme", "NoIndex"},
		{"no such theme", "Nope"},
		{"not a name", "../Test"},
		{"empty name", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadTheme(context.Background(), dirs, tt.theme)
			appErr := requireRoute(t, err, "LoadTheme.ThemeNotFound")
			if appErr.Tag != TagThemeNotFound {
				t.Errorf("tag = %s", appErr.Tag)
			}
			if appErr.Metadata["theme"] != tt.theme {
				t.Errorf("metadata = %v", appErr.Metadata)
			}
		})
	}
}

func TestLoadThemeIOErrors(t *testing.T) {
	t.Run("index.theme that cannot be read", func(t *testing.T) {
		dirs, tmp := tempDirs(t)
		// A folder where the file should be: reading it fails, and it is not "not found".
		writeFile(t, filepath.Join(tmp, "sys/icons/Broken/index.theme/x"), "")
		_, err := LoadTheme(context.Background(), dirs, "Broken")
		_ = requireRoute(t, err, "LoadTheme.IndexThemeReadFailed")
	})
	t.Run("theme folder that cannot be examined", func(t *testing.T) {
		dirs, tmp := tempDirs(t)
		symlinkToSelf(t, filepath.Join(tmp, "sys/icons/Loop"))
		_, err := LoadTheme(context.Background(), dirs, "Loop")
		_ = requireRoute(t, err, "LoadTheme.ThemeStatFailed")
	})
	t.Run("a file where the theme folder should be is skipped", func(t *testing.T) {
		dirs, tmp := tempDirs(t)
		writeFile(t, filepath.Join(tmp, "home/.icons/Theme"), "a file")
		writeFile(t, filepath.Join(tmp, "sys/icons/Theme/index.theme"), "[Icon Theme]\nDirectories=\n")
		th, err := LoadTheme(context.Background(), dirs, "Theme")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{filepath.Join(tmp, "sys/icons/Theme")}; !reflect.DeepEqual(th.roots, want) {
			t.Errorf("roots = %q, want %q", th.roots, want)
		}
	})
}

func TestLoadThemeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadTheme(ctx, fixtureDirs(t, ""), "Test")
	appErr := requireRoute(t, err, "LoadTheme.LookupCanceled")
	if appErr.Tag != TagLookupCanceled {
		t.Errorf("tag = %s", appErr.Tag)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("the error does not wrap context.Canceled: %v", err)
	}
}
