package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

var updateGoldens = flag.Bool("update-goldens", false, "rewrite the golden files of testdata")

// fakeHome is the home the golden tests pretend to have; nothing is ever written there:
// Prepare writes nothing.
const fakeHome = "/home/jailcat-test"

// requireRoute fails the test unless err is an *errors.Error with the tag and a route
// that has route.
func requireRoute(t *testing.T, err error, tag, route string) {
	t.Helper()
	_ = requireRouteMeta(t, err, tag, route)
}

// requireRouteMeta is requireRoute that returns the error, to look at its metadata.
func requireRouteMeta(t *testing.T, err error, tag, route string) *errors.Error {
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

// absPath returns the absolute path of a file below testdata.
func absPath(t *testing.T, parts ...string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// golden compares text with testdata/golden/name, byte for byte.
func golden(t *testing.T, name, text string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update-goldens to make it)", err)
	}
	if string(want) != text {
		t.Errorf("%s differs from the golden file.\n--- want\n%s--- got\n%s", name, want, text)
	}
}

// writeText writes a file (and its folders) for a test.
func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readText reads a file for a test.
func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// vscodeProfile reads the fixture code.profile of package firejail and returns the
// settings of a build on it (edit changes them) with its generated profile. The profile
// includes /etc/firejail/code.profile, so its text does not depend on the machine.
func vscodeProfile(t *testing.T, home string, edit func(s *firejail.Settings)) (firejail.Settings, *firejail.Generated) {
	t.Helper()
	ctx := context.Background()
	sys, err := filepath.Abs(filepath.Join("..", "firejail", "testdata", "etc-firejail"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := firejail.ReadConfig(ctx, filepath.Join(sys, "firejail.config"))
	if err != nil {
		t.Fatal(err)
	}
	opts := firejail.ReadOpts{Dirs: xdg.Dirs{Home: home, FirejailSystem: sys, FirejailUser: t.TempDir()}, Config: cfg}
	prof, err := firejail.Read(ctx, filepath.Join(sys, "code.profile"), opts)
	if err != nil {
		t.Fatal(err)
	}
	base := firejail.BaseValues(prof, home)
	s := base.Settings.Clone()
	s.SandboxName = "firejail-code"
	s.MenuName = "Visual Studio Code (sandboxed)"
	s.Comment = "Code Editing. Redefined."
	s.Icon = firejail.IconBadge
	s.InstallMenu = true
	s.InstallBin = true
	if edit != nil {
		edit(&s)
	}
	gen, err := firejail.Generate(firejail.ProfileSpec{
		BuildID:     7,
		BaseProfile: "/etc/firejail/code.profile",
		Profile:     prof,
		Base:        base,
		Overrides:   firejail.Diff(base.Settings, s),
		Home:        home,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return s, gen
}

// stubProfile is a small generated profile with the header of build id, for tests that
// do not look at the profile.
func stubProfile(id int64) *firejail.Generated {
	g := &firejail.Generated{}
	for _, l := range firejail.Header(id) {
		g.Lines = append(g.Lines, firejail.GeneratedLine{Text: l, Kind: firejail.LineHeader})
	}
	g.Lines = append(g.Lines, firejail.GeneratedLine{Text: "quiet", Kind: firejail.LineQuiet})
	return g
}

// fakeDirs resolves the folders of the pretend home; nothing is written there.
func fakeDirs(t *testing.T) xdg.Dirs {
	t.Helper()
	dirs, err := xdg.Resolve(xdg.NewEnv(fakeHome, map[string]string{"XDG_DATA_DIRS": "/nonexistent"}))
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

// vscodeApp is the VS Code-like app of the goldens, by hand, so its paths do not depend
// on a temporary folder.
func vscodeApp(t *testing.T) apps.App {
	t.Helper()
	return apps.App{
		ID:          "code.desktop",
		Kind:        apps.KindDesktop,
		Name:        "Visual Studio Code",
		Comment:     "Code Editing. Redefined.",
		Icon:        absPath(t, "icons", "code.svg"),
		Binary:      "/usr/bin/code",
		BinaryName:  "code",
		Args:        []string{"%F"},
		DesktopPath: absPath(t, "apps", "code.desktop"),
		Actions: []apps.Action{
			{ID: "new-empty-window", Name: "New Empty Window", Binary: "/usr/bin/code", Args: []string{"--new-window", "%F"}, SameBinary: true},
			{ID: "open-docs", Name: "Open the Documentation", Binary: "/usr/bin/xdg-open", Args: []string{"https://code.visualstudio.com/docs"}},
		},
	}
}

// vscodeBuild is what the VS Code-like build chooses besides the defaults: its own private
// home below homes, no network and some security switches.
func vscodeBuild(homes string) func(s *firejail.Settings) {
	return func(s *firejail.Settings) {
		s.Home = firejail.HomePrivate
		s.PrivateDir = homes + "/" + s.SandboxName
		s.Network = firejail.NetNone
		s.Seccomp = true
		s.NoNewPrivs = true
	}
}

// vscodeRequest is the request of the VS Code-like build for the fake home.
func vscodeRequest(t *testing.T, edit func(s *firejail.Settings)) Request {
	t.Helper()
	s, gen := vscodeProfile(t, fakeHome, edit)
	return Request{BuildID: 7, App: vscodeApp(t), Settings: s, Profile: gen, Firejail: "/usr/bin/firejail", IconTheme: "hicolor"}
}

// kit is a scratch home in a temporary folder with the VS Code-like fixture installed
// in it: the desktop entry and its icon, and a fake code binary on PATH.
type kit struct {
	t      *testing.T
	root   string
	home   string
	dirs   xdg.Dirs
	binary string
}

func newKit(t *testing.T) *kit {
	t.Helper()
	root := t.TempDir()
	k := &kit{t: t, root: root, home: filepath.Join(root, "home"), binary: filepath.Join(root, "bin", "code")}
	writeText(t, k.binary, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(k.binary, 0o755); err != nil {
		t.Fatal(err)
	}
	sys := filepath.Join(root, "sys")
	writeText(t, filepath.Join(sys, "icons", "code.svg"), readText(t, absPath(t, "icons", "code.svg")))
	dirs, err := xdg.Resolve(xdg.NewEnv(k.home, map[string]string{
		"XDG_DATA_DIRS": sys,
		"PATH":          filepath.Join(root, "bin"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	k.dirs = dirs
	writeText(t, filepath.Join(dirs.Applications, "code.desktop"), readText(t, absPath(t, "apps", "code.desktop")))
	return k
}

// app finds the VS Code-like app by scanning the scratch home.
func (k *kit) app() apps.App {
	k.t.Helper()
	res, err := apps.Scan(context.Background(), k.dirs, "")
	if err != nil {
		k.t.Fatal(err)
	}
	for _, a := range res.Apps {
		if a.ID == "code.desktop" {
			return a
		}
	}
	k.t.Fatalf("the scan did not find code.desktop: %+v", res)
	return apps.App{}
}

// request makes the request of the VS Code-like build on the scratch home.
func (k *kit) request(edit func(s *firejail.Settings)) Request {
	k.t.Helper()
	s, gen := vscodeProfile(k.t, k.home, edit)
	return Request{BuildID: 7, App: k.app(), Settings: s, Profile: gen, Firejail: "/usr/bin/firejail", IconTheme: "hicolor"}
}

// prepare makes the plan of the VS Code-like build on the scratch home.
func (k *kit) prepare(edit func(s *firejail.Settings)) *Prepared {
	k.t.Helper()
	p, err := Prepare(context.Background(), k.dirs, k.request(edit))
	if err != nil {
		k.t.Fatalf("Prepare: %v", err)
	}
	return p
}

// normalize turns the scratch folders in text into the ones of the goldens.
func (k *kit) normalize(text string) string {
	return strings.NewReplacer(k.binary, "/usr/bin/code", k.home, fakeHome).Replace(text)
}

// owners is an OwnerFunc over a map of path -> record.
type record struct {
	build int64
	sha   string
}

func owners(m map[string]record) OwnerFunc {
	return func(path string) (int64, string, bool, error) {
		r, ok := m[path]
		return r.build, r.sha, ok, nil
	}
}

// file returns the planned file of a kind.
func file(t *testing.T, p *Prepared, kind Kind) File {
	t.Helper()
	for _, f := range p.Files {
		if f.Kind == kind {
			return f
		}
	}
	t.Fatalf("the plan has no %s file", kind)
	return File{}
}

// requireRouteOnly fails the test unless err is an *errors.Error whose route has route
// (for an error that another package tagged).
func requireRouteOnly(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

// sumOf is the hex SHA-256 of text.
func sumOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
