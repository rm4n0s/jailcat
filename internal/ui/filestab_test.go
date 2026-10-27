package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// filesPage opens the editor of a new build of the app on the Files tab, in a window tall
// enough to show the whole tab, so a test can tap anything on it.
func filesPage(t *testing.T, app string) *page {
	t.Helper()
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ts.SetViewSize(gui.Size{Width: 1100, Height: 1800})
	p.ready()
	p.openApp(app)
	return p
}

// key finds a widget by its key.
func key(k string) gluttertest.Finder { return gluttertest.ByKey(m.NewValueKey(k)) }

// tap taps a widget by its key and draws the answer.
func (p *page) tap(k string) {
	p.t.Helper()
	p.ts.Tap(key(k))
	p.ts.Pump()
}

// hasFolder reports whether the editor's list has the path.
func hasFolder(e *Editor, path string) bool {
	return slices.ContainsFunc(e.Final().Folders, func(f firejail.Folder) bool { return f.Path == path })
}

func folderOf(t *testing.T, e *Editor, path string) firejail.Folder {
	t.Helper()
	for _, f := range e.Final().Folders {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no row for %s in %+v", path, e.Final().Folders)
	return firejail.Folder{}
}

func TestFilesTabHomeChoices(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	if got := e.Base().Settings.Home; got != firejail.HomeFull {
		t.Fatalf("the base home is %q, want full", got)
	}
	for _, text := range []string{homeTitleText, homeFullText, homeChosenText, homeEmptyText} {
		if !p.ts.Exists(gluttertest.ByText(text)) {
			t.Errorf("the Files tab lacks %q", text)
		}
	}
	if p.ts.Exists(key("mark:home")) || p.dots() != 0 {
		t.Error("a fresh build shows a change")
	}

	tests := []struct {
		mode firejail.HomeMode
		dot  bool
	}{
		{firejail.HomeChosen, true},
		{firejail.HomeEmpty, true},
		{firejail.HomePrivate, true},
		{firejail.HomeFull, false},
	}
	for _, tt := range tests {
		p.tap("home:" + string(tt.mode))
		if got := e.Final().Home; got != tt.mode {
			t.Errorf("after tapping %s the home is %q", tt.mode, got)
		}
		if p.tabDot(firejail.TabFiles) != tt.dot || p.ts.Exists(key("mark:home")) != tt.dot {
			t.Errorf("%s: dot %v, mark %v, want %v", tt.mode, p.tabDot(firejail.TabFiles), p.ts.Exists(key("mark:home")), tt.dot)
		}
	}

	// Reset next to the group puts the home back.
	p.tap("home:empty")
	p.ts.Tap(gluttertest.Descendant(key("mark:home"), gluttertest.ByText(resetText)))
	p.ts.Pump()
	if e.Final().Home != firejail.HomeFull || e.HomeChanged() || p.tabDot(firejail.TabFiles) {
		t.Errorf("Reset left home %q", e.Final().Home)
	}
}

func TestFilesTabOwnPrivateHomeShowsItsPath(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	want := "Its own home, kept between runs: ~/.local/share/jailcat/homes/firejail-calculator"
	// The path is shown before the choice is made, and it is the one the build uses after.
	if !p.ts.Exists(gluttertest.ByText(want)) {
		t.Errorf("the path is not shown before the choice")
	}
	p.tap("home:private")
	if !p.ts.Exists(gluttertest.ByText(want)) {
		t.Errorf("the path is not shown after the choice")
	}
	if got, dir := e.Final().PrivateDir, filepath.Join(p.backend.HomesDir(), "firejail-calculator"); got != dir {
		t.Errorf("PrivateDir = %q, want %q", got, dir)
	}
	// Going back to another choice leaves no change of the private folder.
	p.tap("home:full")
	if e.HasChanges() {
		t.Error("the private folder is still a change after going back to Full home")
	}
}

func TestFilesTabChosenWithoutFoldersShowsTheProblem(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	if len(e.Final().Folders) != 0 {
		t.Fatalf("the Default profile has folders: %+v", e.Final().Folders)
	}
	if !p.ts.Exists(gluttertest.ByText(noFoldersText)) || p.ts.Exists(key("problem:folders")) {
		t.Error("the empty list shows a problem with Full home")
	}
	p.tap("home:chosen")
	var message string
	for _, pr := range e.Problems() {
		if pr.Tag == firejail.ProblemChosenNoFolder {
			message = pr.Message
		}
	}
	if message == "" {
		t.Fatal("Validate has no problem for Only the folders I choose with no folder")
	}
	if !p.ts.Exists(gluttertest.Descendant(key("problem:folders"), gluttertest.ByText(message))) {
		t.Error("the problem is not shown under the list")
	}
	// A folder solves it.
	p.change(func(e *Editor) { e.AddFolder("/mnt/data") })
	if p.ts.Exists(key("problem:folders")) {
		t.Error("the problem is still shown with a folder in the list")
	}
}

func TestFilesTabAddsFoldersThroughTheChooser(t *testing.T) {
	p := filesPage(t, "Calculator")
	fd := p.ts.FakeDesktop()
	e := p.home().editor
	home := p.backend.HomeDir()

	p.tap("add-folder")
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser("/mnt/data", filepath.Join(home, "Projects"))
	p.waitFor("both folders", func() bool { return hasFolder(e, "/mnt/data") && hasFolder(e, "${HOME}/Projects") })

	for _, path := range []string{"/mnt/data", "${HOME}/Projects"} {
		if f := folderOf(t, e, path); f.Access != firejail.AccessReadWrite || f.FromProfile {
			t.Errorf("%s was added as %+v", path, f)
		}
		if !p.ts.Exists(key("folder:"+path)) || !p.ts.Exists(key("mark:"+path)) {
			t.Errorf("the row or the mark of %s is not shown", path)
		}
	}
	if !p.ts.Exists(gluttertest.ByText("~/Projects")) || !p.tabDot(firejail.TabFiles) {
		t.Error("the home is not written with ~, or the tab has no dot")
	}

	// One more chooser: a folder that is in the list is refused and the message says so.
	p.tap("add-folder")
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.AnswerFileChooser("/mnt/data")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining("in the list already")) })
	if n := len(e.Final().Folders); n != 2 {
		t.Errorf("%d rows, want 2", n)
	}
}

func TestFilesTabChooserCancelledAndFailed(t *testing.T) {
	p := filesPage(t, "Calculator")
	fd := p.ts.FakeDesktop()
	e := p.home().editor

	p.tap("add-folder")
	fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
	fd.CancelFileChooser()
	p.waitFor("the chooser to end", func() bool {
		return !gluttertest.StateOf[*filesTabState](p.ts, gluttertest.ByType[*FilesTab]()).choosing
	})
	if len(e.Final().Folders) != 0 || p.ts.Exists(gluttertest.ByType[*m.Card]()) {
		t.Error("a cancelled chooser added something")
	}

	fd.DisableFileChooser()
	p.tap("add-folder")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByText(chooserFailedText)) })
	if strings.Contains(p.log.String(), "FileChooser") == false {
		t.Errorf("the log lacks the failure:\n%s", p.log.String())
	}
}

func TestFilesTabRefusesTheRealHomeInPrivateModes(t *testing.T) {
	for _, mode := range []firejail.HomeMode{firejail.HomeEmpty, firejail.HomePrivate} {
		t.Run(string(mode), func(t *testing.T) {
			p := filesPage(t, "Visual Studio Code")
			fd := p.ts.FakeDesktop()
			e := p.home().editor
			profileRows := len(e.Final().Folders)
			p.tap("home:" + string(mode))
			home := p.backend.HomeDir()

			// The chooser: the folder in the home is refused, the one outside is added.
			p.tap("add-folder")
			fd.WaitFor("the file chooser", func() bool { return fd.PendingFileChoosers() == 1 })
			fd.AnswerFileChooser(filepath.Join(home, "Projects"), "/opt/projects")
			p.waitFor("the folder outside", func() bool { return hasFolder(e, "/opt/projects") })
			if hasFolder(e, "${HOME}/Projects") {
				t.Error("a folder inside the home was added")
			}
			if !p.ts.Exists(gluttertest.ByTextContaining("~/Projects is inside your home folder")) {
				t.Error("the refusal is not shown in plain words")
			}

			// Typed: refused in the dialog, which stays open.
			p.tap("type-path")
			p.ts.EnterText(key("path-field"), "~/Documents")
			p.tap("path-add")
			if !p.ts.Exists(gluttertest.ByType[*PathDialog]()) || !p.ts.Exists(gluttertest.ByTextContaining("~/Documents is inside your home folder")) {
				t.Error("the dialog did not refuse the folder")
			}
			p.cancelPathDialog()
			if p.ts.Exists(gluttertest.ByType[*PathDialog]()) {
				t.Error("Cancel did not close the dialog")
			}
			// Nothing else was added, and the profile's rows stay.
			if n := len(e.Final().Folders); n != profileRows+1 {
				t.Errorf("%d rows, want %d", n, profileRows+1)
			}
			if !p.ts.Exists(gluttertest.ByText("~/.vscode")) || !hasFolder(e, "${HOME}/.vscode") {
				t.Error("a row of the profile is gone")
			}
			if len(e.Problems()) != 0 {
				t.Errorf("problems = %+v", e.Problems())
			}
		})
	}
}

func TestFilesTabTypedPath(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	submit := func(text string) {
		p.t.Helper()
		p.ts.EnterText(key("path-field"), text)
		p.ts.Pump()
		p.tap("path-add")
	}

	// A valid path closes the dialog and adds the row, the home as ${HOME}.
	p.tap("type-path")
	submit("~/Projects/")
	p.waitFor("the folder", func() bool { return hasFolder(e, "${HOME}/Projects") })
	if p.ts.Exists(gluttertest.ByType[*PathDialog]()) || !p.ts.Exists(gluttertest.ByText("~/Projects")) {
		t.Error("the dialog stayed open or the row is not shown")
	}

	// A macro: shown as the folder's name, with the macro as the hint.
	p.tap("type-path")
	submit("${DOWNLOADS}/books")
	p.waitFor("the macro folder", func() bool { return hasFolder(e, "${DOWNLOADS}/books") })
	if !p.ts.Exists(gluttertest.ByText("Downloads/books")) || !p.ts.Exists(gluttertest.ByText("${DOWNLOADS}/books")) {
		t.Error("the macro row does not show the name and the hint")
	}

	// Invalid paths: the message of Validate, the dialog open, and typing clears it.
	tests := []struct{ text, message string }{
		{"projects", "must be a full path"},
		{"/mnt/../etc", `".."`},
		{"~/Projects", "in the list already"},
	}
	rows := len(e.Final().Folders)
	for _, tt := range tests {
		p.tap("type-path")
		submit(tt.text)
		if !p.ts.Exists(gluttertest.ByType[*PathDialog]()) || !p.ts.Exists(gluttertest.ByTextContaining(tt.message)) {
			t.Errorf("%q: no message with %q", tt.text, tt.message)
		}
		p.ts.EnterText(key("path-field"), "x")
		p.ts.Pump()
		if p.ts.Exists(gluttertest.ByTextContaining(tt.message)) {
			t.Errorf("%q: the message stays after typing", tt.text)
		}
		p.cancelPathDialog()
	}
	if len(e.Final().Folders) != rows {
		t.Errorf("an invalid path was added: %+v", e.Final().Folders)
	}
}

// cancelPathDialog presses Cancel in the dialog of the typed path.
func (p *page) cancelPathDialog() {
	p.t.Helper()
	p.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*PathDialog](), gluttertest.ByText(cancelButtonText)))
	p.ts.Pump()
}

// pickAccess opens the access menu of the row and picks a level.
func (p *page) pickAccess(path string, access firejail.Access) {
	p.t.Helper()
	p.tap("access:" + path)
	p.ts.Tap(gluttertest.Last(key("access:" + string(access) + ":" + path)))
	p.ts.Pump()
}

func TestFilesTabAccessOfARow(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	e := p.home().editor
	const profileRow = "${HOME}/.vscode"
	if f := folderOf(t, e, profileRow); f.Access != firejail.AccessReadWrite || !f.FromProfile {
		t.Fatalf("the profile row = %+v", f)
	}
	if !p.ts.Exists(gluttertest.Descendant(key("folder:"+profileRow), gluttertest.ByText(fromProfileText))) {
		t.Error("the row has no from-profile mark")
	}
	if p.ts.Exists(key("mark:" + profileRow)) {
		t.Error("an unchanged row shows the changed mark")
	}

	for _, access := range []firejail.Access{firejail.AccessReadOnly, firejail.AccessHidden} {
		p.pickAccess(profileRow, access)
		if got := folderOf(t, e, profileRow).Access; got != access {
			t.Fatalf("the access is %q, want %q", got, access)
		}
		if !p.ts.Exists(key("mark:"+profileRow)) || !e.FolderChanged(profileRow) || !p.tabDot(firejail.TabFiles) {
			t.Errorf("%s: no mark or dot", access)
		}
	}
	// Reset of the row puts the profile's access back, and only that row.
	p.ts.Tap(gluttertest.Descendant(key("mark:"+profileRow), gluttertest.ByText(resetText)))
	p.ts.Pump()
	if got := folderOf(t, e, profileRow).Access; got != firejail.AccessReadWrite || e.HasChanges() {
		t.Errorf("after Reset: access %q, changes %v", got, e.HasChanges())
	}

	// A row the user added: changing it to the same level as before is no override.
	p.change(func(e *Editor) { e.AddFolder("/mnt/data") })
	p.pickAccess("/mnt/data", firejail.AccessReadOnly)
	if got := folderOf(t, e, "/mnt/data").Access; got != firejail.AccessReadOnly {
		t.Errorf("the added row's access is %q", got)
	}
}

func TestFilesTabRemoveAndRestoreAProfileRow(t *testing.T) {
	p := filesPage(t, "Visual Studio Code")
	e := p.home().editor
	const row = "${HOME}/.vscode"
	rows := len(e.Final().Folders)

	p.tap("remove:" + row)
	if hasFolder(e, row) || len(e.Final().Folders) != rows-1 {
		t.Fatal("the row was not removed")
	}
	// It stays listed, struck through, with Restore.
	removed := key("folder:" + row)
	if !p.ts.Exists(removed) || !p.ts.Exists(gluttertest.Descendant(removed, gluttertest.ByText(restoreText))) {
		t.Fatal("the removed row is not listed with Restore")
	}
	text := p.ts.Widget(gluttertest.Descendant(removed, gluttertest.ByText("~/.vscode"))).(*m.Text)
	if style := text.Style(); style == nil || !style.Decoration.OrElse(0).Contains(m.TextDecorationLineThrough) {
		t.Error("the removed row is not struck through")
	}
	if !p.tabDot(firejail.TabFiles) || !e.FolderChanged(row) {
		t.Error("the removal is not a change")
	}
	p.tap("restore:" + row)
	if !hasFolder(e, row) || e.HasChanges() || p.ts.Exists(key("restore:"+row)) || p.tabDot(firejail.TabFiles) {
		t.Error("Restore did not bring the row back as it was")
	}

	// A row the user added is removed for good.
	p.change(func(e *Editor) { e.AddFolder("/mnt/data") })
	p.tap("remove:/mnt/data")
	if p.ts.Exists(key("folder:/mnt/data")) || hasFolder(e, "/mnt/data") {
		t.Error("an added row is still there after remove")
	}
}

func TestFilesTabLockedRow(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	p.change(func(e *Editor) {
		locked := firejail.Folder{Path: "/opt/docs", Access: firejail.AccessReadOnly, FromProfile: true, Locked: true}
		e.base.Settings.Folders = append(e.base.Settings.Folders, locked)
		e.baseline.Folders = append(e.baseline.Folders, locked)
		e.final.Folders = append(e.final.Folders, locked)
	})
	row := key("folder:/opt/docs")
	if !p.ts.Exists(row) || !p.ts.Exists(key("locked:/opt/docs")) || !p.ts.Exists(gluttertest.ByIcon(m.Icons.Lock())) {
		t.Fatal("the locked row shows no lock")
	}
	if !p.ts.Exists(gluttertest.ByTextContaining("whitelist-ro")) {
		t.Error("the reason is not given")
	}
	if p.ts.Exists(key("access:/opt/docs")) || p.ts.Exists(key("remove:/opt/docs")) {
		t.Error("a locked row has a dropdown or a remove button")
	}
	p.change(func(e *Editor) {
		e.SetFolderAccess("/opt/docs", firejail.AccessHidden)
		e.RemoveFolder("/opt/docs")
	})
	if f := folderOf(t, e, "/opt/docs"); f.Access != firejail.AccessReadOnly || e.HasChanges() {
		t.Errorf("the locked row changed: %+v", f)
	}
}

func TestFilesTabToggles(t *testing.T) {
	tests := []struct {
		field firejail.Field
		get   func(s firejail.Settings) bool
	}{
		{firejail.FieldDownloadsReadWrite, func(s firejail.Settings) bool { return s.DownloadsReadWrite }},
		{firejail.FieldHideRemovable, func(s firejail.Settings) bool { return s.HideRemovable }},
		{firejail.FieldNoExecHome, func(s firejail.Settings) bool { return s.NoExecHome }},
		{firejail.FieldPrivateTmp, func(s firejail.Settings) bool { return s.PrivateTmp }},
		{firejail.FieldPrivateDev, func(s firejail.Settings) bool { return s.PrivateDev }},
		{firejail.FieldPrivateCache, func(s firejail.Settings) bool { return s.PrivateCache }},
	}
	p := filesPage(t, "Calculator")
	e := p.home().editor
	for _, tt := range tests {
		t.Run(string(tt.field), func(t *testing.T) {
			before := tt.get(e.Final())
			p.tap("toggle:" + string(tt.field))
			if tt.get(e.Final()) == before || !e.Changed(tt.field) || !p.tabDot(firejail.TabFiles) {
				t.Fatalf("the switch did not change %s", tt.field)
			}
			mark := key("mark:" + string(tt.field))
			if !p.ts.Exists(mark) {
				t.Fatal("no changed mark")
			}
			p.ts.Tap(gluttertest.Descendant(mark, gluttertest.ByText(resetText)))
			p.ts.Pump()
			if tt.get(e.Final()) != before || e.Changed(tt.field) || p.ts.Exists(mark) {
				t.Error("Reset did not put the switch back")
			}
		})
	}
	if e.HasChanges() {
		t.Errorf("the toggles left changes: %+v", e.Overrides())
	}
}

func TestFilesTabDownloadsNeedsTheRealHome(t *testing.T) {
	p := filesPage(t, "Calculator")
	e := p.home().editor
	for _, mode := range []firejail.HomeMode{firejail.HomeEmpty, firejail.HomePrivate} {
		p.tap("home:" + string(mode))
		if !p.ts.Exists(gluttertest.ByText(downloadsOffReason)) {
			t.Errorf("%s: the reason is not given", mode)
		}
		p.tap("toggle:" + string(firejail.FieldDownloadsReadWrite))
		if e.Final().DownloadsReadWrite {
			t.Errorf("%s: Downloads was turned on", mode)
		}
	}

	// With Downloads on first, the private home leaves it on, shows why that is a problem
	// and lets the user turn it off.
	p.tap("home:full")
	p.tap("toggle:" + string(firejail.FieldDownloadsReadWrite))
	p.tap("home:private")
	if !e.Final().DownloadsReadWrite || !p.ts.Exists(key("problem:downloads")) {
		t.Fatal("Downloads on with a private home shows no problem")
	}
	p.tap("toggle:" + string(firejail.FieldDownloadsReadWrite))
	if e.Final().DownloadsReadWrite || p.ts.Exists(key("problem:downloads")) {
		t.Error("Downloads could not be turned off")
	}
}

func TestFilesTabScrolls(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	// In the window of 720 points the last switch is below the edge: the list has not
	// built it. Scrolling brings it up and the first choice goes away.
	last := key("toggle:" + string(firejail.FieldPrivateCache))
	if p.ts.Exists(last) {
		t.Fatal("the last switch is on screen: the tab is not taller than the window")
	}
	p.ts.Scroll(key("body:"+string(firejail.TabFiles)), gui.Offset{Dx: 0, Dy: 3000})
	p.ts.Pump()
	if !p.ts.Exists(last) {
		t.Fatal("scrolling did not bring up the last switch")
	}
	if y := p.ts.BottomRight(last).Dy; y > windowSize.Height {
		t.Errorf("the last switch is at %v, below the window", y)
	}
	if p.ts.Exists(key("home:full")) {
		t.Error("the home choice is still on screen after scrolling to the end")
	}
}

func TestGoldenFilesTab(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	// Own private home, one folder of the user's and one row of the profile with another access.
	p.tap("home:private")
	p.change(func(e *Editor) {
		e.AddFolder("/mnt/projects")
		e.SetFolderAccess("${HOME}/.config/Code", firejail.AccessReadOnly)
	})
	p.golden("files-tab", "4.3-files-tab.png")
}
