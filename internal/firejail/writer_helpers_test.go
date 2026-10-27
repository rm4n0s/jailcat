package firejail

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// testHome is the home folder the golden tests pretend to have; nothing is written
// there.
const testHome = "/home/jailcat-test"

// writerEnv is a firejail setup over the fixture copy of /etc/firejail: the system
// folder is testdata/etc-firejail, the user folder and the home are empty folders.
type writerEnv struct {
	t    *testing.T
	home string
	sys  string
	user string
	opts ReadOpts
	// out holds the profiles the tests generate.
	out string
	n   int
}

// newWriterEnv makes the setup. A home of "" makes a temporary one; the golden tests
// pass testHome, which is never touched.
func newWriterEnv(t *testing.T, home string) *writerEnv {
	t.Helper()
	root := t.TempDir()
	if home == "" {
		home = filepath.Join(root, "home")
	}
	sys := fixtureDir(t)
	cfg, err := ReadConfig(context.Background(), filepath.Join(sys, "firejail.config"))
	if err != nil {
		t.Fatal(err)
	}
	e := &writerEnv{t: t, home: home, sys: sys, user: filepath.Join(root, "user"), out: filepath.Join(root, "out")}
	e.opts = ReadOpts{Dirs: xdg.Dirs{Home: home, FirejailSystem: sys, FirejailUser: e.user}, Config: cfg}
	for _, d := range []string{e.user, e.out} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if bad := invalidFilename(root); bad != "" {
		t.Fatalf("the temporary folder %q is not a valid firejail path (%s): rename the test", root, bad)
	}
	return e
}

// read reads a file the way Jailcat does.
func (e *writerEnv) read(path string) (*Profile, error) {
	e.t.Helper()
	return Read(context.Background(), path, e.opts)
}

// spec makes the spec of a build on a fixture base profile ("" is base None). edit
// changes the settings the user sees; the overrides are the difference from the base
// values. BaseProfile is the real path of the fixture.
func (e *writerEnv) spec(base string, edit func(s *Settings), extras ...string) ProfileSpec {
	e.t.Helper()
	if base == "" {
		return e.specFor("", edit, extras...)
	}
	return e.specFor(filepath.Join(e.sys, base), edit, extras...)
}

// profile writes a base profile of the test into the out folder and returns its path.
func (e *writerEnv) profile(name string, lines ...string) string {
	e.t.Helper()
	path := filepath.Join(e.out, name)
	writeFile(e.t, path, strings.Join(lines, "\n")+"\n")
	return path
}

// specFor is spec for a base profile at path ("" is base None).
func (e *writerEnv) specFor(path string, edit func(s *Settings), extras ...string) ProfileSpec {
	e.t.Helper()
	spec := ProfileSpec{BuildID: 7, Home: e.home, ExtraRules: extras, Base: BaseNone()}
	if path != "" {
		prof, err := e.read(path)
		if err != nil {
			e.t.Fatalf("Read(%s): %v", path, err)
		}
		spec.Profile = prof
		spec.BaseProfile = path
		spec.Base = BaseValues(prof, e.home)
	}
	spec.Base.Settings.SandboxName = "firejail-test"
	edited := spec.Base.Settings.Clone()
	if edit != nil {
		edit(&edited)
	}
	spec.Overrides = Diff(spec.Base.Settings, edited)
	return spec
}

// write writes a generated profile into the out folder, as a ".profile" file, and
// returns its path.
func (e *writerEnv) write(g *Generated) string {
	e.t.Helper()
	e.n++
	path := filepath.Join(e.out, "gen-"+strconv.Itoa(e.n)+".profile")
	if err := os.WriteFile(path, []byte(g.String()), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return path
}

// mustGenerate generates the profile of spec and fails the test on an error.
func mustGenerate(t *testing.T, spec ProfileSpec) *Generated {
	t.Helper()
	g, err := Generate(spec)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return g
}

// requireTag fails the test unless err is an *errors.Error with the route and the tag.
func requireTag(t *testing.T, err error, tag, route string) {
	t.Helper()
	_ = requireTagErr(t, err, tag, route)
}

// requireTagErr is requireTag that returns the error, to look at its metadata.
func requireTagErr(t *testing.T, err error, tag, route string) *errors.Error {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if appErr.Tag != tag {
		t.Errorf("Tag = %q, want %q (route %s)", appErr.Tag, tag, appErr.Route())
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
	return appErr
}
