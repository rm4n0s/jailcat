package apps

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// fakeSystem is the system of testdata/fake built in a temporary folder.
type fakeSystem struct {
	// root is the folder everything lives in.
	root string
	// dirs has the user's data home, two system data folders and a $PATH of three
	// folders, as Scan gets them.
	dirs xdg.Dirs
}

// path joins elements below the fake root.
func (s fakeSystem) path(elem ...string) string {
	return filepath.Join(append([]string{s.root}, elem...)...)
}

// newFakeSystem copies testdata/fake into a temporary folder (replacing @ROOT@ in every
// file with the folder) and adds what git cannot keep: executables, modes and symbolic
// links. The $PATH is bin1:bin2:bin3, a folder that does not exist, and a relative one.
func newFakeSystem(t *testing.T) fakeSystem {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("testdata", "fake")
	walkErr := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, []byte(strings.ReplaceAll(string(data), "@ROOT@", root)), 0o644)
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}

	s := fakeSystem{root: root}
	for _, d := range []string{"bin1", "bin2", "bin3", "fakefj", "bin1/adir"} {
		mkdir(t, s.path(d))
	}
	exe := func(p string) { writeFile(t, s.path(p), "#!/bin/sh\n", 0o755) }
	for _, p := range []string{
		"fakefj/firejail",
		"bin1/prog", "bin1/dup", "bin1/envprog",
		"bin2/dup", "bin2/firecfgapp", "bin2/flatpak",
		"bin3/runbin", "bin3/onlybin3", "bin3/plainbin", "bin3/sandboxme", "bin3/firecfgbare",
	} {
		exe(p)
	}
	writeFile(t, s.path("bin1/notexec"), "not executable\n", 0o644)
	link := func(target, name string) {
		if err := os.Symlink(target, s.path(name)); err != nil {
			t.Fatal(err)
		}
	}
	// firecfg links: they point to the fake firejail and shadow real binaries in later folders.
	link("../fakefj/firejail", "bin1/firecfgapp")
	link("../fakefj/firejail", "bin1/firecfgbare")
	link("../fakefj/firejail", "bin1/firecfgonly")
	link("../fakefj/firejail", "bin1/sandboxme")
	// A link to a link to firejail is a firecfg link too.
	link("firecfgonly", "bin1/firecfgchain")
	// Links to ordinary files, and one to nothing.
	link("../bin1/prog", "bin3/proglink")
	link("../bin1/dup", "bin3/dupalias")
	link("../bin3/nothing-here", "bin3/brokenlink")

	s.dirs = xdg.Dirs{
		Home:     s.path("home"),
		DataHome: s.path("user"),
		DataDirs: []string{s.path("sys1"), s.path("sys2"), s.path("sys3")},
		Path: []string{
			s.path("bin1"), s.path("bin2"), s.path("bin3"), s.path("bin4"), "relative/bin",
		},
	}
	return s
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile keeps the mode of an existing file and applies the umask to a new one.
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

// byID returns the app with the given ID.
func byID(t *testing.T, apps []App, id string) App {
	t.Helper()
	for _, a := range apps {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no app %q in %v", id, ids(apps))
	return App{}
}

// ids returns the IDs of apps in order.
func ids(apps []App) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}
