package firejail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	jerrors "github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

func TestFindFirejail(t *testing.T) {
	root := t.TempDir()
	mk := func(name string) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	empty := mk("empty")
	plain := mk("plain")
	notExec := mk("notexec")
	dirNamed := mk("dirnamed")
	linked := mk("linked")
	broken := mk("broken")
	second := mk("second")
	target := fakeScript(t, mk("real"), "firejail", "exit 0\n")
	if err := os.WriteFile(filepath.Join(notExec, "firejail"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dirNamed, "firejail"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linked, "firejail")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(broken, "firejail")); err != nil {
		t.Fatal(err)
	}
	fakeScript(t, plain, "firejail", "exit 0\n")
	fakeScript(t, second, "firejail", "exit 0\n")

	tests := []struct {
		name string
		path []string
		want string
	}{
		{"found", []string{empty, plain}, filepath.Join(plain, "firejail")},
		{"first of two wins", []string{plain, second}, filepath.Join(plain, "firejail")},
		{"symlink", []string{linked}, filepath.Join(linked, "firejail")},
		{"not executable is skipped", []string{notExec, second}, filepath.Join(second, "firejail")},
		{"directory is skipped", []string{dirNamed, second}, filepath.Join(second, "firejail")},
		{"broken symlink is skipped", []string{broken, second}, filepath.Join(second, "firejail")},
		{"relative entry is skipped", []string{"relative", second}, filepath.Join(second, "firejail")},
		{"missing folder is skipped", []string{filepath.Join(root, "missing"), second}, filepath.Join(second, "firejail")},
		{"unclean entry is cleaned", []string{plain + "/../plain/."}, filepath.Join(plain, "firejail")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindFirejail(xdg.Dirs{Path: tt.path})
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("FindFirejail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindFirejailNotFound(t *testing.T) {
	root := t.TempDir()
	notExec := filepath.Join(root, "notexec")
	if err := os.Mkdir(notExec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notExec, "firejail"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string][]string{
		"empty path":     nil,
		"missing folder": {filepath.Join(root, "missing")},
		"not executable": {notExec},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := FindFirejail(xdg.Dirs{Path: path})
			appErr, ok := jerrors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T", err)
			}
			if appErr.Tag != TagFirejailNotFound {
				t.Errorf("tag = %s, want %s", appErr.Tag, TagFirejailNotFound)
			}
			if !appErr.HasRoute("FindFirejail.FirejailNotFound") {
				t.Errorf("unexpected failure path: %s", appErr.Route())
			}
		})
	}
}

func TestVersion(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"real first line", "printf 'firejail version 0.9.80\\n\\nCompile time support:\\n\\t- AppArmor support is enabled\\n'\n", "0.9.80"},
		{"only the first line", "echo 'firejail version 0.9.72'\necho 'firejail version 9.9.9'\n", "0.9.72"},
		{"no newline", "printf 'firejail version 0.9.80'\n", "0.9.80"},
		{"release candidate", "echo 'firejail version 0.9.81-rc1'\n", "0.9.81-rc1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := fakeScript(t, t.TempDir(), "firejail", tt.body)
			got, err := Version(context.Background(), fake)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionPassesTheFlagOnly(t *testing.T) {
	dir := t.TempDir()
	fake := fakeScript(t, dir, "firejail", `[ "$#" = 1 ] && [ "$1" = --version ] || exit 3`+"\necho 'firejail version 1.2.3'\n")
	if got, err := Version(context.Background(), fake); err != nil || got != "1.2.3" {
		t.Errorf("Version = %q, %v", got, err)
	}
}

func TestVersionErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		path    string
		timeout time.Duration
		route   string
	}{
		{"garbage", fakeScript(t, dir, "garbage", "echo hello\n"), 5 * time.Second, "version.FirejailVersionUnparsable"},
		{"empty output", fakeScript(t, dir, "empty", "exit 0\n"), 5 * time.Second, "version.FirejailVersionUnparsable"},
		{"no number", fakeScript(t, dir, "nonumber", "echo 'firejail version'\n"), 5 * time.Second, "version.FirejailVersionUnparsable"},
		{"not a number", fakeScript(t, dir, "notnum", "echo 'firejail version abc'\n"), 5 * time.Second, "version.FirejailVersionUnparsable"},
		{"first line only counts", fakeScript(t, dir, "late", "echo hi\necho 'firejail version 0.9.80'\n"), 5 * time.Second, "version.FirejailVersionUnparsable"},
		{"exit 1", fakeScript(t, dir, "fail", "echo 'firejail version 0.9.80'\nexit 1\n"), 5 * time.Second, "version.FirejailVersionFailed"},
		{"missing program", filepath.Join(dir, "missing"), 5 * time.Second, "version.FirejailVersionFailed"},
		{"timeout", fakeScript(t, dir, "slow", "exec sleep 30\n"), 200 * time.Millisecond, "version.FirejailVersionTimeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := version(context.Background(), tt.path, tt.timeout)
			appErr, ok := jerrors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
			}
			if !appErr.HasRoute(tt.route) {
				t.Errorf("unexpected failure path: %s", appErr.Route())
			}
		})
	}
}

func TestVersionCancelledByCaller(t *testing.T) {
	fake := fakeScript(t, t.TempDir(), "firejail", "exec sleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := Version(ctx, fake)
	appErr, ok := jerrors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if !appErr.HasRoute("Version->version.FirejailVersionFailed") {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
}
