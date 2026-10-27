package install

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/assets"
	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// setupDirs resolves the folders of a fresh home in t.TempDir().
func setupDirs(t *testing.T) xdg.Dirs {
	t.Helper()
	dirs, err := xdg.Resolve(xdg.NewEnv(filepath.Join(t.TempDir(), "home"), nil))
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

func TestSetupDesktopGolden(t *testing.T) {
	dirs := setupDirs(t)
	res, err := SetupDesktop(context.Background(), dirs, "/home/jailcat-test/go/bin/jailcat")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "setup.desktop", readText(t, res.DesktopFile))
}

func TestSetupDesktopWritesBothFiles(t *testing.T) {
	tests := []struct {
		name string
		exe  string
	}{
		{"plain path", "/usr/local/bin/jailcat"},
		{"spaces", "/opt/my apps/jailcat"},
		{"quotes dollar backtick backslash", `/opt/a"b'c$d` + "`e\\f/jailcat"},
		{"percent", "/opt/100%/jailcat"},
		{"non-ASCII", "/opt/γάτα/jailcat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := setupDirs(t)
			res, err := SetupDesktop(context.Background(), dirs, tt.exe)
			if err != nil {
				t.Fatal(err)
			}
			wantDesktop := filepath.Join(dirs.DataHome, "applications", "io.github.rm4n0s.jailcat.desktop")
			wantIcon := filepath.Join(dirs.DataHome, "icons", "hicolor", "scalable", "apps", "io.github.rm4n0s.jailcat.svg")
			if res.DesktopFile != wantDesktop || res.IconFile != wantIcon {
				t.Errorf("result = %+v, want %s and %s", res, wantDesktop, wantIcon)
			}
			if readText(t, res.IconFile) != string(assets.Icon()) {
				t.Error("the icon is not Jailcat's own icon")
			}
			for _, p := range []string{res.DesktopFile, res.IconFile} {
				if m := mode(t, p); m != 0o644 {
					t.Errorf("%s has mode %v, want 0644", p, m)
				}
			}
			for _, d := range []string{dirs.Applications, dirs.Icons} {
				if m := mode(t, d); m != 0o755 {
					t.Errorf("%s has mode %v, want 0755", d, m)
				}
			}

			f, err := os.Open(res.DesktopFile)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			entry, err := desktopentry.Parse(f)
			if err != nil {
				t.Fatalf("the entry does not parse back: %v", err)
			}
			main := entry.Main()
			args, err := desktopentry.SplitExec(main.String("Exec"))
			if err != nil {
				t.Fatalf("SplitExec: %v", err)
			}
			if !reflect.DeepEqual(args, []string{tt.exe}) {
				t.Errorf("Exec splits to %q, want [%q]", args, tt.exe)
			}
			for key, want := range map[string]string{
				"Name": "Jailcat", "Icon": "io.github.rm4n0s.jailcat", "StartupWMClass": "io.github.rm4n0s.jailcat",
				"Comment": "Build firejail sandboxes for your applications", "Type": "Application",
			} {
				if got := main.String(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if got := main.Strings("Categories"); !reflect.DeepEqual(got, []string{"System", "Security", "Utility"}) {
				t.Errorf("Categories = %q", got)
			}
			if main.Has("X-Jailcat-Build") || strings.Contains(readText(t, res.DesktopFile), "X-Jailcat-Build") {
				t.Error("the entry has an X-Jailcat-Build key")
			}
		})
	}
}

func TestSetupDesktopTwiceReplacesFiles(t *testing.T) {
	dirs := setupDirs(t)
	ctx := context.Background()
	if _, err := SetupDesktop(ctx, dirs, "/old/place/jailcat"); err != nil {
		t.Fatal(err)
	}
	res, err := SetupDesktop(ctx, dirs, "/new/place/jailcat")
	if err != nil {
		t.Fatal(err)
	}
	if got := readText(t, res.DesktopFile); !strings.Contains(got, "Exec=/new/place/jailcat\n") || strings.Contains(got, "/old/") {
		t.Errorf("the entry was not replaced:\n%s", got)
	}
	// No temporary file is left behind.
	for _, d := range []string{dirs.Applications, dirs.Icons} {
		list, err := os.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Errorf("%s has %d entries, want 1: %v", d, len(list), list)
		}
	}
}

func TestSetupDesktopErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name  string
		ctx   context.Context
		exe   string
		spoil func(t *testing.T, dirs xdg.Dirs)
		tag   string
		route string
	}{
		{"relative path", context.Background(), "jailcat", nil,
			tagSetupExeNotAbsolute, "SetupDesktop." + tagSetupExeNotAbsolute},
		{"canceled", canceled, "/usr/bin/jailcat", nil,
			tagSetupCanceled, "SetupDesktop." + tagSetupCanceled},
		{"a file where the icon folder should be", context.Background(), "/usr/bin/jailcat",
			func(t *testing.T, dirs xdg.Dirs) {
				writeText(t, filepath.Join(dirs.DataHome, "icons"), "x")
			},
			tagDirCreateFailed, "SetupDesktop->ensureDir." + tagDirCreateFailed},
		{"an Exec with a NUL", context.Background(), "/usr/bin/a\x00b", nil,
			"", "SetupDesktop->Group.Set"},
		{"an unwritable applications folder", context.Background(), "/usr/bin/jailcat",
			func(t *testing.T, dirs xdg.Dirs) {
				if err := os.MkdirAll(dirs.Applications, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(dirs.Applications, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dirs.Applications, 0o755) })
				if f, err := os.CreateTemp(dirs.Applications, "probe"); err == nil {
					_ = f.Close()
					t.Skip("the folder is writable despite mode 0500 (running as root?)")
				}
			},
			tagFileWriteFailed, "SetupDesktop->writeAtomic." + tagFileWriteFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := setupDirs(t)
			if tt.spoil != nil {
				tt.spoil(t, dirs)
			}
			_, err := SetupDesktop(tt.ctx, dirs, tt.exe)
			if err == nil {
				t.Fatal("SetupDesktop succeeded")
			}
			if tt.tag == "" {
				// An error of package desktopentry is returned as it is.
				requireRoute(t, err, "InvalidValue", tt.route)
				return
			}
			requireRoute(t, err, tt.tag, tt.route)
		})
	}
}
