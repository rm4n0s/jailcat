package ui

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// system is a small system in a temporary folder: one menu app (code), one binary
// (mytool) and a fake firejail on the $PATH, an icon theme setting and icons.
type system struct {
	root string
	home string
	dirs xdg.Dirs
	env  xdg.Env
}

const (
	codeDesktop = "[Desktop Entry]\nType=Application\nName=Code\nGenericName=Text Editor\nIcon=code\nExec=code %F\n"
	toolDesktop = "[Desktop Entry]\nType=Application\nName=Tool\nIcon=nothing-like-it\nExec=tool\n"
	fakeFJ      = "#!/bin/sh\necho 'firejail version 0.9.80'\n"
)

func writeFile(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

// newSystem builds the system. withFirejail puts the fake firejail on the $PATH.
func newSystem(t *testing.T, withFirejail bool) system {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	share := filepath.Join(root, "share")
	bin := filepath.Join(root, "bin")
	writeFile(t, filepath.Join(share, "applications", "code.desktop"), codeDesktop, 0o644)
	writeFile(t, filepath.Join(share, "applications", "tool.desktop"), toolDesktop, 0o644)
	writeFile(t, filepath.Join(bin, "code"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(bin, "tool"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(bin, "mytool"), "#!/bin/sh\n", 0o755)
	if withFirejail {
		writeFile(t, filepath.Join(bin, "firejail"), fakeFJ, 0o755)
	}
	// The icon theme is named in the GTK settings; its icon is in that theme.
	writeFile(t, filepath.Join(home, ".config", "gtk-3.0", "settings.ini"), "[Settings]\ngtk-icon-theme-name=Fancy\n", 0o644)
	writeFile(t, filepath.Join(share, "icons", "Fancy", "index.theme"),
		"[Icon Theme]\nName=Fancy\nDirectories=scalable/apps\n\n[scalable/apps]\nSize=48\nType=Scalable\nMinSize=1\nMaxSize=512\n", 0o644)
	writeFile(t, filepath.Join(share, "icons", "Fancy", "scalable", "apps", "code.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\"/>", 0o644)

	env := xdg.NewEnv(home, map[string]string{"XDG_DATA_DIRS": share, "PATH": bin, "LANG": "C"})
	dirs, err := xdg.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	return system{root: root, home: home, dirs: dirs, env: env}
}

// newBackend returns the backend of the system with a database in a temporary folder.
func (s system) newBackend(t *testing.T) (*LocalBackend, *store.Store) {
	t.Helper()
	return s.newBackendWith(t, s.dirs)
}

// newBackendWith is newBackend with other folders than the system's own.
func (s system) newBackendWith(t *testing.T, dirs xdg.Dirs) (*LocalBackend, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "jailcat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return NewLocalBackend(dirs, st, apps.LocaleFromEnv(s.env), slog.New(slog.NewTextHandler(io.Discard, nil))), st
}

func TestLocalBackendScanApps(t *testing.T) {
	b, _ := newSystem(t, true).newBackend(t)
	res, err := b.ScanApps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]apps.App{}
	for _, a := range res.Apps {
		got[a.Name] = a
	}
	if len(res.Apps) != 4 {
		// code and tool (menu apps), mytool and firejail (binaries).
		t.Errorf("found %d apps, want 4: %+v", len(res.Apps), res.Apps)
	}
	if a, ok := got["Code"]; !ok || a.Kind != apps.KindDesktop || a.ID != "code.desktop" || a.GenericName != "Text Editor" {
		t.Errorf("Code = %+v", a)
	}
	if a, ok := got["mytool"]; !ok || a.Kind != apps.KindBinary {
		t.Errorf("mytool = %+v", a)
	}
}

func TestLocalBackendScanAppsCancelled(t *testing.T) {
	b, _ := newSystem(t, true).newBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.ScanApps(ctx)
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute("LocalBackend.ScanApps->Scan." + apps.TagScanCancelled) {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
}

func TestLocalBackendLookupIcon(t *testing.T) {
	sys := newSystem(t, true)
	b, _ := sys.newBackend(t)
	ctx := context.Background()

	// The icon of the theme named in the GTK settings.
	icon, err := b.LookupIcon(ctx, apps.App{ID: "code.desktop", Icon: "code"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sys.root, "share", "icons", "Fancy", "scalable", "apps", "code.svg")
	if icon.Path != want || icon.Format != icons.FormatSVG {
		t.Errorf("icon = %+v, want %s as SVG", icon, want)
	}

	// An answer is kept: the file can go and the next call still answers.
	if err := os.Remove(want); err != nil {
		t.Fatal(err)
	}
	if again, err := b.LookupIcon(ctx, apps.App{Icon: "code"}); err != nil || again != icon {
		t.Errorf("second lookup = %+v, %v; want the kept answer", again, err)
	}

	// A missing icon is an error with the tag the widgets match, and it is kept too.
	for range 2 {
		_, err = b.LookupIcon(ctx, apps.App{ID: "tool.desktop", Icon: "nothing-like-it"})
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("expected *errors.Error, got %T: %v", err, err)
		}
		if appErr.Tag != icons.TagIconNotFound || !appErr.HasRoute("LocalBackend.LookupIcon->Lookup."+icons.TagIconNotFound) {
			t.Errorf("tag %s, route %s", appErr.Tag, appErr.Route())
		}
	}
}

func TestLocalBackendLookupIconCancelledIsNotKept(t *testing.T) {
	sys := newSystem(t, true)
	b, _ := sys.newBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.LookupIcon(ctx, apps.App{Icon: "code"}); err == nil {
		t.Fatal("a cancelled lookup succeeded")
	}
	if _, err := b.LookupIcon(context.Background(), apps.App{Icon: "code"}); err != nil {
		t.Errorf("the cancelled answer was kept: %v", err)
	}
}

func TestLocalBackendListBuilds(t *testing.T) {
	b, st := newSystem(t, true).newBackend(t)
	ctx := context.Background()
	got, err := b.ListBuilds(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("ListBuilds on a new database = %v, %v", got, err)
	}
	for _, build := range []store.Build{
		{SandboxName: "firejail-code", AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge},
		{SandboxName: "firejail-mytool", AppKind: store.AppKindBinary, BinaryPath: "/bin/mytool", IconMode: store.IconModeBadge},
	} {
		if _, err := st.CreateBuild(ctx, build); err != nil {
			t.Fatal(err)
		}
	}
	got, err = b.ListBuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing was built: both are drafts.
	drafts, built := DraftCounts(got), BuildCounts(got)
	if len(got) != 2 || drafts["desktop:code.desktop"] != 1 || drafts["binary:/bin/mytool"] != 1 || len(built) != 0 {
		t.Errorf("builds = %+v, drafts = %v, built = %v", got, drafts, built)
	}
}

func TestLocalBackendFirejail(t *testing.T) {
	tests := []struct {
		name string
		with bool
		want FirejailInfo
	}{
		{"installed", true, FirejailInfo{Version: "0.9.80"}},
		{"missing", false, FirejailInfo{Missing: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys := newSystem(t, tt.with)
			b, _ := sys.newBackend(t)
			got, err := b.Firejail(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tt.with {
				tt.want.Path = filepath.Join(sys.root, "bin", "firejail")
			}
			if got != tt.want {
				t.Errorf("Firejail() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLocalBackendFirejailThatFails(t *testing.T) {
	sys := newSystem(t, true)
	writeFile(t, filepath.Join(sys.root, "bin", "firejail"), "#!/bin/sh\nexit 3\n", 0o755)
	b, _ := sys.newBackend(t)
	got, err := b.Firejail(context.Background())
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute("LocalBackend.Firejail->Version->version.FirejailVersionFailed") {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
	if got.Missing || got.Path == "" {
		t.Errorf("a firejail that fails is not a missing one: %+v", got)
	}
}

func TestLocalBackendBinOnPath(t *testing.T) {
	sys := newSystem(t, true)
	b, _ := sys.newBackend(t)
	if b.BinOnPath() {
		t.Error("BinOnPath() = true, but the bin folder is not on the PATH")
	}
	env := xdg.NewEnv(sys.home, map[string]string{"PATH": filepath.Join(sys.home, ".local", "bin") + ":/usr/bin"})
	dirs, err := xdg.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if !NewLocalBackend(dirs, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil))).BinOnPath() {
		t.Error("BinOnPath() = false, but the bin folder is on the PATH")
	}
}

func TestKeys(t *testing.T) {
	tests := []struct {
		name  string
		app   apps.App
		build store.Build
		key   string
	}{
		{"menu app", apps.App{Kind: apps.KindDesktop, ID: "code.desktop"},
			store.Build{AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop"}, "desktop:code.desktop"},
		{"binary", apps.App{Kind: apps.KindBinary, ID: "curl", Binary: "/usr/bin/curl"},
			store.Build{AppKind: store.AppKindBinary, BinaryPath: "/usr/bin//curl"}, "binary:/usr/bin/curl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AppKey(tt.app); got != tt.key {
				t.Errorf("AppKey = %q, want %q", got, tt.key)
			}
			if got := BuildKey(tt.build); got != tt.key {
				t.Errorf("BuildKey = %q, want %q", got, tt.key)
			}
		})
	}
}

func TestLocalBackendProgramOnPath(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	writeFile(t, filepath.Join(first, "plain"), "x", 0o644)
	writeFile(t, filepath.Join(first, "tool"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(second, "later"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(root, "rel", "relative"), "#!/bin/sh\n", 0o755)
	if err := os.MkdirAll(filepath.Join(first, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(second, "later"), filepath.Join(first, "link")); err != nil {
		t.Fatal(err)
	}

	sys := newSystem(t, true)
	dirs := sys.dirs
	dirs.Path = []string{first, "rel", "", second}
	b, _ := sys.newBackendWith(t, dirs)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"tool", context.Background(), true},
		{"later", context.Background(), true},
		{"link", context.Background(), true},
		{"plain", context.Background(), false},
		{"folder", context.Background(), false},
		{"missing", context.Background(), false},
		{"relative", context.Background(), false},
		{"", context.Background(), false},
		{"../first/tool", context.Background(), false},
		{"first/tool", context.Background(), false},
		{"tool", cancelled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.ProgramOnPath(tt.ctx, tt.name); got != tt.want {
				t.Errorf("ProgramOnPath(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
