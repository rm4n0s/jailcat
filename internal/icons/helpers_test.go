package icons

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// requireRoute fails the test unless err is an *errors.Error whose route has route.
func requireRoute(t *testing.T, err error, route string) *errors.Error {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
	return appErr
}

// fixtureRoot is the absolute path of testdata/fake, a fake system that is only read:
//
//	home/.icons   the user's ~/.icons
//	data/icons    $XDG_DATA_HOME/icons
//	sys/icons     the one $XDG_DATA_DIRS entry's icons (themes Test -> Parent -> hicolor, ...)
//	sys/pixmaps   its pixmaps
//	config/*      $XDG_CONFIG_HOME folders for CurrentTheme
//	abs/          files for absolute Icon= values
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "fake"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// fixtureDirs points an xdg.Dirs at the fake system; config is the folder below
// testdata/fake/config to use as $XDG_CONFIG_HOME ("" for one that does not exist).
func fixtureDirs(t *testing.T, config string) xdg.Dirs {
	t.Helper()
	root := fixtureRoot(t)
	vars := map[string]string{
		"XDG_DATA_HOME": filepath.Join(root, "data"),
		"XDG_DATA_DIRS": filepath.Join(root, "sys"),
	}
	if config == "" {
		config = "nothing-here"
	}
	vars["XDG_CONFIG_HOME"] = filepath.Join(root, "config", config)
	return resolve(t, filepath.Join(root, "home"), vars)
}

// tempDirs returns an xdg.Dirs whose folders are all below a new temporary folder, and
// the folder: icons are looked up in <tmp>/home/.icons, <tmp>/data/icons and
// <tmp>/sys/icons, pixmaps in <tmp>/sys/pixmaps.
func tempDirs(t *testing.T) (xdg.Dirs, string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolve(t, filepath.Join(tmp, "home"), map[string]string{
		"XDG_DATA_HOME":   filepath.Join(tmp, "data"),
		"XDG_DATA_DIRS":   filepath.Join(tmp, "sys"),
		"XDG_CONFIG_HOME": filepath.Join(tmp, "config"),
	}), tmp
}

func resolve(t *testing.T, home string, vars map[string]string) xdg.Dirs {
	t.Helper()
	dirs, err := xdg.Resolve(xdg.NewEnv(home, vars))
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

// writeFile writes content to path, making its folders.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// symlinkToSelf makes path a symbolic link that points to itself, so that any stat of it
// fails with ELOOP: an I/O error that is not "does not exist", and that does not depend
// on who runs the test.
func symlinkToSelf(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(path), path); err != nil {
		t.Fatal(err)
	}
}

// cancelAt is a context whose Err reports cancellation from its n-th call on, so a test
// can stop a scan at a chosen check. calls counts the calls to Err.
type cancelAt struct {
	context.Context
	n     int64
	calls atomic.Int64
}

func newCancelAt(n int) *cancelAt {
	return &cancelAt{Context: context.Background(), n: int64(n)}
}

func (c *cancelAt) Err() error {
	if c.calls.Add(1) >= c.n {
		return context.Canceled
	}
	return nil
}
