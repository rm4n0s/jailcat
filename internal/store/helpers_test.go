package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return s
}

// closedStore returns a store whose database is already closed, so every query fails.
func closedStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return s
}

func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with route %q, got nil", route)
	}
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

func intp(n int) *int { return &n }

// testOverrides returns a non-trivial result of firejail.Diff.
func testOverrides() firejail.Overrides {
	base := firejail.DefaultSettings()
	edited := base.Clone()
	edited.Network = firejail.NetNone
	edited.Sound = false
	edited.Nice = intp(5)
	edited.DNS = []string{"1.1.1.1", "9.9.9.9"}
	edited.Folders = []firejail.Folder{{Path: "/mnt/data", Access: firejail.AccessReadOnly}}
	return firejail.Diff(base, edited)
}

// otherOverrides is a second non-trivial value, different from testOverrides.
func otherOverrides() firejail.Overrides {
	base := firejail.DefaultSettings()
	edited := base.Clone()
	edited.Network = firejail.NetNone
	edited.Nice = intp(-3)
	return firejail.Diff(base, edited)
}

func testBuild(name string) Build {
	return Build{
		SandboxName:   name,
		AppKind:       AppKindDesktop,
		DesktopFileID: "code.desktop",
		BinaryPath:    "/usr/bin/code",
		BaseProfile:   "/etc/firejail/code.profile",
		Overrides:     testOverrides(),
		ExtraRules:    []string{"noroot", "", "whitelist /tmp/x y"},
		IconMode:      IconModeRandom,
		IconSeed:      -1234567890123,
	}
}

// sameOverrides compares two overrides through what they write and what they change.
func sameOverrides(t *testing.T, got, want firejail.Overrides) {
	t.Helper()
	gj, err := got.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	wj, err := want.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(gj) != string(wj) {
		t.Errorf("overrides JSON = %s\nwant           %s", gj, wj)
	}
	base := firejail.DefaultSettings()
	if g, w := got.Apply(base), want.Apply(base); g.Network != w.Network || g.Sound != w.Sound ||
		len(g.Folders) != len(w.Folders) || len(g.DNS) != len(w.DNS) {
		t.Errorf("Apply(base) = %+v, want %+v", g, w)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameBuild compares every field of two builds.
func sameBuild(t *testing.T, got, want Build) {
	t.Helper()
	if got.ID != want.ID || got.SandboxName != want.SandboxName || got.AppKind != want.AppKind ||
		got.DesktopFileID != want.DesktopFileID || got.BinaryPath != want.BinaryPath ||
		got.BaseProfile != want.BaseProfile || got.IconMode != want.IconMode || got.IconSeed != want.IconSeed ||
		got.FirejailVersion != want.FirejailVersion || got.Installed != want.Installed {
		t.Errorf("build = %+v\nwant   %+v", got, want)
	}
	if !sameStrings(got.ExtraRules, want.ExtraRules) {
		t.Errorf("ExtraRules = %q, want %q", got.ExtraRules, want.ExtraRules)
	}
	sameOverrides(t, got.Overrides, want.Overrides)
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("times = %v, %v; want %v, %v", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
	if (got.BuiltAt == nil) != (want.BuiltAt == nil) || got.BuiltAt != nil && !got.BuiltAt.Equal(*want.BuiltAt) {
		t.Errorf("BuiltAt = %v, want %v", got.BuiltAt, want.BuiltAt)
	}
	if (got.Draft == nil) != (want.Draft == nil) {
		t.Errorf("Draft = %+v, want %+v", got.Draft, want.Draft)
	} else if got.Draft != nil {
		sameDraft(t, *got.Draft, *want.Draft)
	}
}

func sameDraft(t *testing.T, got, want Draft) {
	t.Helper()
	sameOverrides(t, got.Overrides, want.Overrides)
	if !sameStrings(got.ExtraRules, want.ExtraRules) || got.BaseProfile != want.BaseProfile {
		t.Errorf("draft = %+v, want %+v", got, want)
	}
}

func mustCreate(t *testing.T, s *Store, b Build) Build {
	t.Helper()
	got, err := s.CreateBuild(context.Background(), b)
	if err != nil {
		t.Fatalf("CreateBuild(%s) error = %v", b.SandboxName, err)
	}
	return got
}

func fixedTime() time.Time { return time.Date(2026, 10, 27, 12, 30, 45, 123456789, time.UTC) }

// eachStore runs fn on a fresh in-memory database and on a fresh file database in
// t.TempDir() (whose mode is checked).
func eachStore(t *testing.T, fn func(t *testing.T, s *Store)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { fn(t, newTestStore(t)) })
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "jailcat.db")
		s, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() {
			if err := s.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("file mode = %o, want 600", got)
		}
		fn(t, s)
	})
}
