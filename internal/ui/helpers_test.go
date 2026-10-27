package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	"golang.org/x/image/font/gofont/gomono"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/store"
)

// windowSize is the size of the main window in the tests and the goldens.
var windowSize = m.Size{Width: 1100, Height: 720}

// useLinux makes the framework behave as on the Linux desktop, which Jailcat is for: the
// tests default to Android.
func useLinux(t *testing.T) {
	t.Helper()
	if !foundation.DebugMode {
		t.Skip("the platform override is a debug-only feature")
	}
	foundation.SetDebugDefaultTargetPlatformOverride(foundation.Some(foundation.TargetPlatformLinux))
	t.Cleanup(func() { foundation.SetDebugDefaultTargetPlatformOverride(foundation.None[foundation.TargetPlatform]()) })
}

// syncBuffer is a log destination that two goroutines can use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeBackend is a [Backend] that answers from fields, so a test decides what the system
// looks like. Set the fields before the page is pumped; change them later only through
// set.
type fakeBackend struct {
	mu        sync.Mutex
	result    apps.Result
	scanErr   error
	builds    []store.Build
	buildsErr error
	firejail  FirejailInfo
	fjErr     error
	binOnPath bool
	// cpus is the answer of CPUCount.
	cpus int
	// icons maps an Icon= value to the file; a value that is not here is not found.
	icons map[string]icons.Icon
	// gate, when not nil, holds ScanApps until it is closed.
	gate chan struct{}
	// scans counts the calls of ScanApps.
	scans int
	// programs maps a program name to whether ProgramOnPath finds it; programAsks lists
	// the names it was asked for.
	programs    map[string]bool
	programAsks []string

	// fj answers ProfileChoices, ReadBase and ImportProfile from the fixture folders of
	// firejail (the real code of [LocalBackend], no database).
	fj *LocalBackend
	// choices, when not nil, replaces the answer of ProfileChoices.
	choices *ProfileChoices
	// readGate, when not nil, holds ReadBase until it is closed.
	readGate chan struct{}
	// readErr maps a profile path to the error ReadBase returns for it.
	readErr map[string]error
	// reads lists the paths ReadBase was asked for.
	reads []string
	// imported lists the sources ImportProfile was asked for; importErr is its error.
	imported  []string
	importErr error
	// createErr and saveErr are the errors of CreateBuild and SaveDraft.
	createErr error
	saveErr   error
	// saves lists the calls that kept a draft or created a build, in order, as text.
	saves []string
	// lookups counts the calls of BuildsForApp.
	lookups int

	// nameAsks lists the names SandboxNameTaken was asked about; nameErr is its error and
	// nameGate, when not nil, holds it until it is closed.
	nameAsks []string
	nameErr  error
	nameGate chan struct{}
	// previewAsks lists the settings PreviewIcon was asked for. previewSVG, when not nil,
	// is its answer with previewFallback; previewErr is its error; otherwise it makes the
	// icon from the icons map like install.RenderIcon.
	previewAsks     []firejail.Settings
	previewSVG      []byte
	previewFallback string
	previewErr      error
	// checkAsks lists the rows CheckExtraRules was asked about.
	checkAsks [][]string
}

// set changes the fields under the lock.
func (f *fakeBackend) set(change func(f *fakeBackend)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

func (f *fakeBackend) ScanApps(ctx context.Context) (apps.Result, error) {
	f.mu.Lock()
	gate := f.gate
	f.scans++
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return apps.Result{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.result, f.scanErr
}

func (f *fakeBackend) ListBuilds(context.Context) ([]store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.builds, f.buildsErr
}

func (f *fakeBackend) LookupIcon(_ context.Context, app apps.App) (icons.Icon, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if icon, ok := f.icons[app.Icon]; ok {
		return icon, nil
	}
	return icons.Icon{}, errors.New(icons.TagIconNotFound, "no icon in the fake", "icon", app.Icon)
}

func (f *fakeBackend) Firejail(context.Context) (FirejailInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.firejail, f.fjErr
}

func (f *fakeBackend) BinOnPath() bool { return f.binOnPath }

// CPUCount answers the cpus field.
func (f *fakeBackend) CPUCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cpus
}

// ProgramOnPath answers from the programs map; a program that is not in it is missing.
func (f *fakeBackend) ProgramOnPath(_ context.Context, name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.programAsks = append(f.programAsks, name)
	return f.programs[name]
}

// testIcon returns an icon of testdata/icons.
func testIcon(t *testing.T, file string, format icons.Format) icons.Icon {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "icons", file))
	if err != nil {
		t.Fatal(err)
	}
	return icons.Icon{Path: path, Format: format}
}

// fixtureBackend is a fake system with sixteen apps: thirteen menu apps (two hidden, one
// Flatpak, one that wraps firejail, three with a missing, XPM or unusable icon) and three
// binaries; Firefox has two builds and Visual Studio Code one. firejail is installed.
func fixtureBackend(t *testing.T) *fakeBackend {
	t.Helper()
	desktop := func(name, generic, icon string) apps.App {
		bin := "/usr/bin/" + filepath.Base(name)
		return apps.App{
			ID: name + ".desktop", Kind: apps.KindDesktop, Name: name, GenericName: generic, Icon: icon,
			Binary: bin, BinaryName: filepath.Base(bin),
		}
	}
	binary := func(name string) apps.App {
		return apps.App{ID: name, Kind: apps.KindBinary, Name: name, Binary: "/usr/bin/" + name, BinaryName: name, Terminal: true}
	}

	calculator := desktop("Calculator", "", "blue")
	calculator.Comment = "Do sums"
	hidden := desktop("Secret Helper", "Helper", "blue")
	hidden.NoDisplay = true
	// Its icon file is not an SVG; the svg package reports an error when it draws it, so the
	// app is hidden and only the test that wants the error asks for it.
	brokenIcon := desktop("Zed Broken Icon", "Test", "broken")
	brokenIcon.NoDisplay = true
	slack := desktop("Slack", "Chat", "green")
	slack.Binary, slack.BinaryName = "/usr/bin/flatpak", "flatpak"
	slack.AlreadySandboxed, slack.SandboxKind = true, apps.SandboxFlatpak
	terminal := desktop("Terminal", "Terminal Emulator", "red")
	terminal.UsesFirejail = true

	// Visual Studio Code runs "code", which has a profile in the fixture of firejail.
	code := desktop("Visual Studio Code", "Text Editor", "blue")
	code.Binary, code.BinaryName, code.Comment = "/usr/bin/code", "code", "Code Editing. Redefined."

	all := []apps.App{
		desktop("Archive Manager", "Archiver", "green"),
		calculator,
		binary("curl"),
		desktop("Files", "File Manager", "orange"),
		desktop("Firefox", "Web Browser", "red"),
		desktop("GIMP", "Image Editor", "teal"),
		binary("htop"),
		desktop("LibreOffice Writer", "Word Processor", "purple"),
		desktop("No Icon", "Test", "missing"),
		desktop("Old Tool", "Test", "old"),
		hidden,
		slack,
		terminal,
		code,
		binary("wget"),
		brokenIcon,
	}
	built := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	fj := newFixtureFirejail(t)
	return &fakeBackend{
		fj:     NewLocalBackend(fj.dirs, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil))),
		result: apps.Result{Apps: all},
		builds: []store.Build{
			{ID: 1, SandboxName: "firejail-firefox", AppKind: store.AppKindDesktop, DesktopFileID: "Firefox.desktop", BuiltAt: &built},
			{ID: 2, SandboxName: "firejail-firefox-2", AppKind: store.AppKindDesktop, DesktopFileID: "Firefox.desktop", BuiltAt: &built},
			{ID: 3, SandboxName: "firejail-code", AppKind: store.AppKindDesktop, DesktopFileID: "Visual Studio Code.desktop", BuiltAt: &built},
		},
		firejail: FirejailInfo{Path: "/usr/bin/firejail", Version: "0.9.80"},
		cpus:     8,
		// Xpra and Xvfb are installed, Xephyr is not.
		programs: map[string]bool{"xpra": true, "Xvfb": true},
		icons: map[string]icons.Icon{
			"blue":   testIcon(t, "blue.svg", icons.FormatSVG),
			"green":  testIcon(t, "green.svg", icons.FormatSVG),
			"red":    testIcon(t, "red.svg", icons.FormatSVG),
			"purple": testIcon(t, "purple.svg", icons.FormatSVG),
			"orange": testIcon(t, "orange.png", icons.FormatPNG),
			"teal":   testIcon(t, "teal.png", icons.FormatPNG),
			"broken": testIcon(t, "broken.svg", icons.FormatSVG),
			"old":    testIcon(t, "old.xpm", icons.FormatXPM),
		},
	}
}

// page is a main window under test.
type page struct {
	t       *testing.T
	ts      *gluttertest.Tester
	backend *fakeBackend
	log     *syncBuffer
}

// newPage pumps the app on backend at the size of the window. The scan has not finished.
func newPage(t *testing.T, backend *fakeBackend) *page {
	t.Helper()
	useLinux(t)
	log := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(log, nil))
	// The generic "monospace" of the Extra rules tab is Go Mono in the tests: a test does not
	// use the fonts of the machine, so the goldens are the same everywhere.
	ts := gluttertest.New(t, gluttertest.Opts{Size: windowSize, Fonts: []gluttertest.Font{{Family: monoFamily, Data: gomono.TTF}}})
	ts.PumpWidget(NewApp(backend, logger))
	return &page{t: t, ts: ts, backend: backend, log: log}
}

// home is the state of the page.
func (p *page) home() *homeState {
	return gluttertest.StateOf[*homeState](p.ts, gluttertest.ByType[*HomePage]())
}

// list is the state of the app list.
func (p *page) list() *appListState {
	return gluttertest.StateOf[*appListState](p.ts, gluttertest.ByType[*AppList]())
}

// waitFor pumps frames, letting the goroutines that load things hand their results over,
// until cond is true.
func (p *page) waitFor(what string, cond func() bool) {
	p.t.Helper()
	for range 500 {
		p.ts.Pump(10 * time.Millisecond)
		if cond() {
			// The state may have changed after the frame began: draw what it says.
			p.ts.Pump()
			return
		}
		time.Sleep(time.Millisecond)
	}
	p.t.Fatalf("timed out waiting for %s", what)
}

// ready waits until the scan and the search for firejail have answered.
func (p *page) ready() {
	p.t.Helper()
	p.waitFor("the scan and firejail", func() bool { h := p.home(); return !h.loading && h.known })
}

// editorReady waits until the editor of the selected app is on screen.
func (p *page) editorReady() *Editor {
	p.t.Helper()
	p.waitFor("the editor", func() bool { return p.home().editor != nil })
	return p.home().editor
}

// names are the names of the apps the list shows after the search and the filter.
func (p *page) names() []string {
	var out []string
	for _, a := range p.list().shown {
		out = append(out, a.Name)
	}
	return out
}

// settled pumps until the picture stops changing: the icons load on goroutines and no
// signal says they are all drawn.
func (p *page) settled() {
	p.t.Helper()
	var prev [sha256.Size]byte
	stable := 0
	for range 600 {
		p.ts.Pump(16 * time.Millisecond)
		time.Sleep(3 * time.Millisecond)
		cur := sha256.Sum256(p.ts.Screenshot().Pix)
		if cur == prev {
			stable++
			if stable >= 8 {
				return
			}
		} else {
			stable, prev = 0, cur
		}
	}
	p.t.Fatal("the picture never settled")
}

// golden settles the picture, compares it with testdata/goldens/<name>.png and, when
// JAILCAT_PROOF_DIR is set, also saves it there as proofFile.
func (p *page) golden(name, proofFile string) {
	p.t.Helper()
	p.settled()
	p.ts.MatchesGolden(name)
	dir, ok := os.LookupEnv("JAILCAT_PROOF_DIR")
	if !ok || proofFile == "" {
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, p.ts.Screenshot()); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, proofFile), buf.Bytes(), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func equalStrings(a, b []string) bool {
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
