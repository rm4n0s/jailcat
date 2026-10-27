package ui

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

func TestLocalBackendSandboxNameTaken(t *testing.T) {
	b, st := newSystem(t, true).newBackend(t)
	ctx := context.Background()
	first, err := st.CreateBuild(ctx, store.Build{SandboxName: "firejail-code", AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateBuild(ctx, store.Build{SandboxName: "firejail-tool", AppKind: store.AppKindDesktop, DesktopFileID: "tool.desktop", IconMode: store.IconModeBadge})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		check  string
		except int64
		want   bool
	}{
		{"a free name", "firejail-free", 0, false},
		{"a free name for a stored build", "firejail-free", first.ID, false},
		{"a name another build has", "firejail-code", 0, true},
		{"a name another build has, for a stored build", "firejail-code", second.ID, true},
		{"the name of the build itself", "firejail-code", first.ID, false},
		{"another build's name is not the first build's", "firejail-tool", first.ID, true},
		{"a name that differs by a letter", "firejail-cod", 0, false},
		{"a name with a SQL wildcard", "firejail-%", 0, false},
		{"a name with a quote", "firejail-code' OR '1'='1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.SandboxNameTaken(ctx, tt.check, tt.except)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("SandboxNameTaken(%q, %d) = %v, want %v", tt.check, tt.except, got, tt.want)
			}
		})
	}
}

func TestLocalBackendSandboxNameTakenFails(t *testing.T) {
	b, _ := newSystem(t, true).newBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.SandboxNameTaken(ctx, "firejail-code", 0)
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute("LocalBackend.SandboxNameTaken->Store.GetBuildByName.DatabaseQueryFailed") {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
}

func TestLocalBackendPreviewIcon(t *testing.T) {
	sys := newSystem(t, true)
	b, _ := sys.newBackend(t)
	ctx := context.Background()
	blue, err := filepath.Abs(filepath.Join("testdata", "icons", "blue.svg"))
	if err != nil {
		t.Fatal(err)
	}
	badged, err := icons.BadgeIcon(ctx, icons.Icon{Path: blue, Format: icons.FormatSVG})
	if err != nil {
		t.Fatal(err)
	}
	app := func(icon string) apps.App { return apps.App{ID: "code.desktop", Name: "Code", Icon: icon} }
	settings := func(mode firejail.IconMode, seed int64, menu string) firejail.Settings {
		s := firejail.DefaultSettings()
		s.Icon, s.IconSeed, s.MenuName = mode, seed, menu
		return s
	}
	tests := []struct {
		name         string
		app          apps.App
		s            firejail.Settings
		want         []byte
		wantFallback string
	}{
		{"the app's icon with the badge", app(blue), settings(firejail.IconBadge, 5, "Code"), badged, ""},
		{"a random icon", app(blue), settings(firejail.IconRandom, 5, "Code"), icons.RandomIcon(5, "Code"), ""},
		{"a random icon with another seed", app(blue), settings(firejail.IconRandom, 6, "Code"), icons.RandomIcon(6, "Code"), ""},
		{"a random icon with another menu name", app(blue), settings(firejail.IconRandom, 5, "Zed"), icons.RandomIcon(5, "Zed"), ""},
		{"an icon that is not found", app("nothing-like-it"), settings(firejail.IconBadge, 5, "Code"), icons.RandomIcon(5, "Code"), "was not found"},
		{"no icon", app(""), settings(firejail.IconBadge, 9, "Tool"), icons.RandomIcon(9, "Tool"), "has no icon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, fallback, err := b.PreviewIcon(ctx, tt.app, tt.s)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("the icon is not the expected one (%d bytes, want %d)", len(got), len(tt.want))
			}
			if (tt.wantFallback == "") != (fallback == "") || !bytes.Contains([]byte(fallback), []byte(tt.wantFallback)) {
				t.Errorf("fallback = %q, want it to hold %q", fallback, tt.wantFallback)
			}
			// It is what a build installs: install.RenderIcon is the one place that makes it.
			want, wantReason, err := install.RenderIcon(ctx, sys.dirs, icons.CurrentTheme(ctx, sys.dirs), tt.app, tt.s)
			if err != nil || !bytes.Equal(got, want) || fallback != wantReason {
				t.Errorf("PreviewIcon differs from install.RenderIcon: %v", err)
			}
		})
	}

	// The icon of the theme the GTK settings name is found, as for the app list.
	if _, fallback, err := b.PreviewIcon(ctx, app("code"), settings(firejail.IconBadge, 1, "Code")); err != nil || bytes.Contains([]byte(fallback), []byte("was not found")) {
		t.Errorf("the icon of the current theme was not found: %q, %v", fallback, err)
	}
}

func TestLocalBackendPreviewIconFails(t *testing.T) {
	b, _ := newSystem(t, true).newBackend(t)
	ctx := context.Background()
	s := firejail.DefaultSettings()
	s.Icon = "neon"
	_, _, err := b.PreviewIcon(ctx, apps.App{ID: "code.desktop"}, s)
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if appErr.Tag != install.TagIconModeInvalid || !appErr.HasRoute("LocalBackend.PreviewIcon->RenderIcon."+install.TagIconModeInvalid) {
		t.Errorf("tag %s, route %s", appErr.Tag, appErr.Route())
	}

	// A cancelled lookup is an error, not a random icon.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	blue, err := filepath.Abs(filepath.Join("testdata", "icons", "blue.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.PreviewIcon(cancelled, apps.App{Icon: blue}, firejail.DefaultSettings()); err == nil {
		t.Error("a cancelled preview succeeded")
	}
}

func TestLocalBackendCheckExtraRules(t *testing.T) {
	sys := newSystem(t, true)
	b, _ := sys.newBackend(t)
	rows := []string{
		"# a comment",
		"bogus 1",
		"include " + filepath.Join(sys.dirs.FirejailUser, "mine.inc"),
		"include /tmp/evil.inc",
		"net none",
		"noblacklist /mnt",
	}
	got := b.CheckExtraRules(context.Background(), rows, firejail.BaseNone())
	type want struct {
		row int
		tag string
		sev firejail.Severity
	}
	wants := []want{
		{2, firejail.ProblemExtraCommand, firejail.SeverityError},
		{4, firejail.ProblemExtraInclude, firejail.SeverityError},
		{5, firejail.ProblemExtraCovered, firejail.SeverityWarning},
	}
	if len(got) != len(wants) {
		t.Fatalf("problems = %+v", got)
	}
	for i, w := range wants {
		if got[i].Row != w.row || got[i].Tag != w.tag || got[i].Severity != w.sev {
			t.Errorf("problem %d = %+v, want row %d %s %v", i, got[i], w.row, w.tag, w.sev)
		}
	}
	if len(b.CheckExtraRules(context.Background(), nil, firejail.BaseNone())) != 0 {
		t.Error("no rows have a problem")
	}
}
