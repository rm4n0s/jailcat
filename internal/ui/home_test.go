package ui

import (
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"

	"github.com/rm4n0s/jailcat/internal/apps"
)

func TestListShowsRows(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()

	// The hidden app is left out; the other fourteen are in the list: the menu apps first,
	// then the binaries, each in the order of the scan.
	want := []string{"Archive Manager", "Calculator", "Files", "Firefox", "GIMP", "LibreOffice Writer", "No Icon", "Old Tool",
		"Slack", "Terminal", "Visual Studio Code", "curl", "htop", "wget"}
	if got := p.names(); !equalStrings(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
	rows := p.ts.Count(gluttertest.ByType[*AppRow]())
	if rows < 5 || rows >= len(want) {
		t.Errorf("%d rows are built; want the visible ones only (the list is lazy)", rows)
	}
	for _, text := range []string{"Archive Manager", "Archiver", "Calculator", "Do sums", "File Manager", "Image Editor"} {
		if n := p.ts.Count(gluttertest.ByText(text)); n != 1 {
			t.Errorf("the text %q is shown %d times, want 1", text, n)
		}
	}
	// The empty state is on the right, and no banner or progress shows.
	if !p.ts.Exists(gluttertest.ByText(emptyStateText)) {
		t.Error("the empty state is missing")
	}
	if p.ts.Exists(gluttertest.ByType[*m.LinearProgressIndicator]()) {
		t.Error("the progress indicator is still shown")
	}
	if p.ts.Exists(gluttertest.ByText(firejailMissingText)) {
		t.Error("the firejail banner is shown")
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"by name", "fire", []string{"Firefox"}},
		{"ignores case", "CALC", []string{"Calculator"}},
		{"by generic name", "browser", []string{"Firefox"}},
		{"by binary name", "htop", []string{"htop"}},
		{"by binary path", "/usr/bin/wg", []string{"wget"}},
		{"nothing matches", "zzzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPage(t, fixtureBackend(t))
			p.ready()
			p.ts.EnterText(gluttertest.ByType[*m.TextField](), tt.query)
			p.ts.Pump()
			if got := p.names(); !equalStrings(got, tt.want) {
				t.Errorf("names = %q, want %q", got, tt.want)
			}
			if tt.want == nil && !p.ts.Exists(gluttertest.ByText("No apps match.")) {
				t.Error("the empty-result text is missing")
			}
		})
	}
}

func TestFilters(t *testing.T) {
	tests := []struct {
		chip   string
		filter apps.Filter
		want   []string
	}{
		{"Menu apps", apps.FilterMenu, []string{"Archive Manager", "Calculator", "Files", "Firefox", "GIMP",
			"LibreOffice Writer", "No Icon", "Old Tool", "Slack", "Terminal", "Visual Studio Code"}},
		{"Binaries", apps.FilterBinaries, []string{"curl", "htop", "wget"}},
		{"Built", apps.FilterBuilt, []string{"Firefox", "Visual Studio Code"}},
		{"All", apps.FilterAll, []string{"Archive Manager", "Calculator", "Files", "Firefox", "GIMP", "LibreOffice Writer", "No Icon",
			"Old Tool", "Slack", "Terminal", "Visual Studio Code", "curl", "htop", "wget"}},
	}
	for _, tt := range tests {
		t.Run(tt.chip, func(t *testing.T) {
			p := newPage(t, fixtureBackend(t))
			p.ready()
			// Leave All first, so tapping All is a change too.
			if tt.chip == "All" {
				p.ts.Tap(gluttertest.ByText("Binaries"))
				p.ts.Pump()
			}
			p.ts.Tap(gluttertest.ByText(tt.chip))
			p.ts.Pump()
			if got := p.names(); !equalStrings(got, tt.want) {
				t.Errorf("names = %q, want %q", got, tt.want)
			}
			if got := p.list().filter; got != tt.filter {
				t.Errorf("filter = %v, want %v", got, tt.filter)
			}
		})
	}
}

func TestBuiltFilterWithoutBuilds(t *testing.T) {
	backend := fixtureBackend(t)
	backend.builds = nil
	p := newPage(t, backend)
	p.ready()
	p.ts.Tap(gluttertest.ByText("Built"))
	p.ts.Pump()
	if len(p.names()) != 0 || !p.ts.Exists(gluttertest.ByText("You have not built any sandboxes yet.")) {
		t.Errorf("names = %q, and the text is missing", p.names())
	}
}

func TestBuiltChips(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.ts.Tap(gluttertest.ByText("Built"))
	p.ts.Pump()
	// Firefox has two builds and Visual Studio Code one.
	if n := p.ts.Count(gluttertest.ByText("built ×2")); n != 1 {
		t.Errorf("\"built ×2\" is shown %d times, want 1", n)
	}
	if n := p.ts.Count(gluttertest.ByText("built")); n != 1 {
		t.Errorf("\"built\" is shown %d times, want 1", n)
	}
}

func TestShowHiddenApps(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	if got := len(p.names()); got != 14 {
		t.Fatalf("%d apps are shown, want 14", got)
	}
	p.ts.Tap(gluttertest.ByText("Show hidden apps"))
	p.ts.Pump()
	names := p.names()
	if len(names) != 16 {
		t.Fatalf("%d apps are shown, want 16: %q", len(names), names)
	}
	for _, hidden := range []string{"Secret Helper", "Zed Broken Icon"} {
		found := false
		for _, n := range names {
			found = found || n == hidden
		}
		if !found {
			t.Errorf("the hidden app %q is not listed: %q", hidden, names)
		}
	}
	p.ts.Tap(gluttertest.ByText("Show hidden apps"))
	p.ts.Pump()
	if got := len(p.names()); got != 14 {
		t.Errorf("%d apps are shown after turning it off, want 14", got)
	}
}

func TestSelectingARow(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.ts.Tap(gluttertest.ByText("Calculator"))
	p.ts.Pump()

	if sel := p.home().selected; sel == nil || sel.Name != "Calculator" {
		t.Fatalf("selected = %+v", sel)
	}
	if p.ts.Exists(gluttertest.ByText(emptyStateText)) {
		t.Error("the empty state is still shown")
	}
	p.editorReady()
	// Its name in the editor, next to the list's row; the sandbox name after the arrow.
	if n := p.ts.Count(gluttertest.ByText("Calculator")); n != 2 {
		t.Errorf("\"Calculator\" is shown %d times, want 2 (row and editor)", n)
	}
	if n := p.ts.Count(gluttertest.ByText("firejail-calculator")); n != 1 {
		t.Errorf("the sandbox name is shown %d times, want 1", n)
	}
	var selectedRows int
	for _, w := range p.ts.Widgets(gluttertest.ByType[*AppRow]()) {
		if row := w.(*AppRow); row.o.Selected {
			selectedRows++
			if row.App().Name != "Calculator" {
				t.Errorf("the highlighted row is %q", row.App().Name)
			}
		}
	}
	if selectedRows != 1 {
		t.Errorf("%d rows are highlighted, want 1", selectedRows)
	}

	// Another tap moves the selection ("Files" is also the name of a tab: tap the row).
	p.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.ByText("Files")))
	p.ts.Pump()
	if sel := p.home().selected; sel == nil || sel.Name != "Files" {
		t.Errorf("selected = %+v after the second tap", sel)
	}
}

func TestSelectingABinary(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.ts.EnterText(gluttertest.ByType[*m.TextField](), "curl")
	p.ts.Pump()
	p.ts.Tap(gluttertest.ByText("curl"))
	p.ts.Pump()
	if sel := p.home().selected; sel == nil || sel.Name != "curl" || AppKey(*sel) != "binary:/usr/bin/curl" {
		t.Errorf("selected = %+v", sel)
	}
}

func TestAlreadySandboxedRowCannotBeSelected(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.ts.EnterText(gluttertest.ByType[*m.TextField](), "slack")
	p.ts.Pump()
	if !p.ts.Exists(gluttertest.ByText("already sandboxed: Flatpak")) {
		t.Error("the reason is missing")
	}
	p.ts.Tap(gluttertest.ByText("Slack"))
	p.ts.Pump()
	if p.home().selected != nil {
		t.Errorf("the Flatpak app was selected: %+v", p.home().selected)
	}
	if !p.ts.Exists(gluttertest.ByText(emptyStateText)) {
		t.Error("the empty state is gone")
	}
}

func TestUsesFirejailMarker(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.ts.EnterText(gluttertest.ByType[*m.TextField](), "terminal")
	p.ts.Pump()
	if n := p.ts.Count(gluttertest.ByText("uses firejail")); n != 1 {
		t.Errorf("\"uses firejail\" is shown %d times, want 1", n)
	}
	// The row can still be selected.
	p.ts.Tap(gluttertest.ByText("Terminal"))
	p.ts.Pump()
	if p.home().selected == nil {
		t.Error("an app that uses firejail cannot be selected")
	}
}

func TestSelectionSurvivesRefresh(t *testing.T) {
	backend := fixtureBackend(t)
	p := newPage(t, backend)
	p.ready()
	p.ts.Tap(gluttertest.ByText("Calculator"))
	p.ts.Pump()

	p.ts.Tap(gluttertest.ByTooltip("Refresh"))
	p.ts.Pump()
	p.ready()
	if sel := p.home().selected; sel == nil || sel.Name != "Calculator" {
		t.Errorf("selected = %+v after Refresh", sel)
	}
	if backend.scans != 2 {
		t.Errorf("%d scans, want 2", backend.scans)
	}

	// The app goes away: the selection with it.
	backend.set(func(f *fakeBackend) { f.result.Apps = f.result.Apps[:1] })
	p.ts.Tap(gluttertest.ByTooltip("Refresh"))
	p.ts.Pump()
	p.ready()
	if p.home().selected != nil || !p.ts.Exists(gluttertest.ByText(emptyStateText)) {
		t.Errorf("selected = %+v after the app disappeared", p.home().selected)
	}
}

func TestScanInProgress(t *testing.T) {
	backend := fixtureBackend(t)
	backend.gate = make(chan struct{})
	p := newPage(t, backend)
	p.ts.Pump()
	if !p.ts.Exists(gluttertest.ByType[*m.LinearProgressIndicator]()) {
		t.Error("no progress indicator while scanning")
	}
	if !p.ts.Exists(gluttertest.ByText("Looking for apps…")) {
		t.Error("the waiting text is missing")
	}
	if p.ts.Exists(gluttertest.ByType[*AppRow]()) {
		t.Error("rows are shown before the scan ends")
	}
	close(backend.gate)
	p.ready()
	if p.ts.Exists(gluttertest.ByType[*m.LinearProgressIndicator]()) {
		t.Error("the progress indicator stays after the scan")
	}
	if !p.ts.Exists(gluttertest.ByType[*AppRow]()) {
		t.Error("no rows after the scan")
	}
}

func TestScanError(t *testing.T) {
	backend := fixtureBackend(t)
	backend.scanErr = errors.New("ScanFailed", "the folder is unreadable", "folder", "/usr/share/applications")
	p := newPage(t, backend)
	p.ready()

	if !p.ts.Exists(gluttertest.ByText(scanFailedText)) {
		t.Error("the message for the user is missing")
	}
	if p.ts.Exists(gluttertest.ByType[*AppRow]()) {
		t.Error("rows are shown after a failed scan")
	}
	if p.ts.Exists(gluttertest.ByTextContaining("unreadable")) {
		t.Error("the details are shown to the user")
	}
	// The details are in the log, with the tag and the route.
	log := p.log.String()
	for _, want := range []string{"scanning the apps failed", "ScanFailed", "the folder is unreadable"} {
		if !strings.Contains(log, want) {
			t.Errorf("the log does not say %q:\n%s", want, log)
		}
	}

	// Refresh after the cause is gone.
	backend.set(func(f *fakeBackend) { f.scanErr = nil })
	p.ts.Tap(gluttertest.ByTooltip("Refresh"))
	p.ts.Pump()
	p.ready()
	if p.ts.Exists(gluttertest.ByText(scanFailedText)) || !p.ts.Exists(gluttertest.ByType[*AppRow]()) {
		t.Error("the list did not recover")
	}
}

func TestBuildsError(t *testing.T) {
	backend := fixtureBackend(t)
	backend.buildsErr = errors.New("DatabaseQueryFailed", "the database is locked")
	p := newPage(t, backend)
	p.ready()
	// The apps are there, the message is above them, and nothing is marked as built.
	if !p.ts.Exists(gluttertest.ByText(buildsFailedText)) || !p.ts.Exists(gluttertest.ByType[*AppRow]()) {
		t.Error("expected the apps and the message about the builds")
	}
	if p.ts.Exists(gluttertest.ByTextContaining("built")) && p.ts.Count(gluttertest.ByText("built")) != 0 {
		t.Error("an app is marked as built")
	}
	if !strings.Contains(p.log.String(), "reading the builds failed") {
		t.Errorf("the log does not say it:\n%s", p.log.String())
	}
}

func TestFirejailBanner(t *testing.T) {
	tests := []struct {
		name       string
		info       FirejailInfo
		err        error
		wantBanner bool
	}{
		{"installed", FirejailInfo{Path: "/usr/bin/firejail", Version: "0.9.80"}, nil, false},
		{"missing", FirejailInfo{Missing: true}, nil, true},
		{"installed but silent", FirejailInfo{Path: "/usr/bin/firejail"}, errors.New("FirejailVersionFailed", "no answer"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := fixtureBackend(t)
			backend.firejail, backend.fjErr = tt.info, tt.err
			p := newPage(t, backend)
			p.ready()
			if got := p.ts.Exists(gluttertest.ByText(firejailMissingText)); got != tt.wantBanner {
				t.Errorf("banner shown = %v, want %v", got, tt.wantBanner)
			}
			// The window works either way.
			if !p.ts.Exists(gluttertest.ByType[*AppRow]()) {
				t.Error("no rows")
			}
			if tt.err != nil && !strings.Contains(p.log.String(), "looking for firejail failed") {
				t.Errorf("the log does not say it:\n%s", p.log.String())
			}
		})
	}
}

// iconCounts counts what the rows of the list draw.
type iconCounts struct{ svg, png, generic, terminal int }

func (p *page) iconCounts() iconCounts {
	inList := func(f gluttertest.Finder) int {
		return p.ts.Count(gluttertest.Descendant(gluttertest.ByType[*AppList](), f))
	}
	return iconCounts{
		svg:      inList(gluttertest.ByType[*svg.SvgPicture]()),
		png:      inList(gluttertest.ByType[*m.Image]()),
		generic:  inList(gluttertest.ByIcon(m.Icons.Apps())),
		terminal: inList(gluttertest.ByIcon(m.Icons.Terminal())),
	}
}

func TestIcons(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  iconCounts
		// reports is the number of errors the svg package reports for a picture that cannot
		// be drawn.
		reports bool
		// hidden turns "Show hidden apps" on first: the app is hidden.
		hidden bool
	}{
		{"an SVG icon", "calculator", iconCounts{svg: 1}, false, false},
		{"a PNG icon", "files", iconCounts{png: 1}, false, false},
		{"an icon that is not found", "no icon", iconCounts{generic: 1}, false, false},
		{"an XPM icon", "old tool", iconCounts{generic: 1}, false, false},
		{"a binary", "curl", iconCounts{terminal: 1}, false, false},
		// The SvgPicture stays in the tree and shows the generic icon as its error widget.
		{"an SVG that cannot be drawn", "zed broken", iconCounts{svg: 1, generic: 1}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPage(t, fixtureBackend(t))
			p.ready()
			if tt.hidden {
				p.ts.Tap(gluttertest.ByText("Show hidden apps"))
				p.ts.Pump()
			}
			p.ts.EnterText(gluttertest.ByType[*m.TextField](), tt.query)
			p.ts.Pump()
			p.waitFor("the icon", func() bool { return p.iconCounts() == tt.want })
			if got := p.iconCounts(); got != tt.want {
				t.Errorf("icons = %+v, want %+v", got, tt.want)
			}
			if tt.reports {
				if errs := p.ts.TakeErrors(); len(errs) == 0 {
					t.Error("the svg package reported nothing for a file that is not an SVG")
				}
			}
		})
	}
}

func TestEveryFixtureIconLoads(t *testing.T) {
	// The first screen draws SVG and PNG icons together; none reports an error (the test
	// fails if one does).
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.waitFor("the icons", func() bool {
		c := p.iconCounts()
		return c.png > 0 && c.svg > 0
	})
}

func TestGoldenHome(t *testing.T) {
	p := newPage(t, fixtureBackend(t))
	p.ready()
	p.golden("home", "4.1-app-list.png")
}

func TestGoldenHomeSelectedWithBanner(t *testing.T) {
	backend := fixtureBackend(t)
	backend.firejail = FirejailInfo{Missing: true}
	p := newPage(t, backend)
	p.ready()
	p.ts.Tap(gluttertest.ByText("Calculator"))
	p.ts.Pump()
	p.editorReady()
	p.golden("home-selected-banner", "")
}

func TestGoldenHomeScanError(t *testing.T) {
	backend := fixtureBackend(t)
	backend.scanErr = errors.New("ScanFailed", "boom")
	p := newPage(t, backend)
	p.ready()
	p.golden("home-scan-error", "")
}
