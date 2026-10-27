package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// newFilesEditor opens a new build of codeApp on a fixture profile with a folder for the
// private homes.
func newFilesEditor(t *testing.T, profile string) *Editor {
	t.Helper()
	f := newFixtureFirejail(t)
	base, p := f.base(profile)
	return NewEditor(codeApp(), EditorOpts{
		Home: f.dirs.Home, HomesDir: f.dirs.Homes, BaseProfile: f.path(profile), Profile: p, Base: base, Defaults: codeDefaults(),
	})
}

func TestCanonicalAndDisplayFolderPath(t *testing.T) {
	const home = "/home/ann"
	tests := []struct {
		name, path     string
		canonical      string
		text, textHint string
	}{
		{"home", "/home/ann", "${HOME}", "~", ""},
		{"tilde", "~", "${HOME}", "~", ""},
		{"tilde folder", "~/Projects/", "${HOME}/Projects", "~/Projects", ""},
		{"real path under the home", "/home/ann/Projects", "${HOME}/Projects", "~/Projects", ""},
		{"variable", "${HOME}/.config/Code", "${HOME}/.config/Code", "~/.config/Code", ""},
		{"outside the home", "/mnt/data/", "/mnt/data", "/mnt/data", ""},
		{"a folder that starts like the home", "/home/anna/x", "/home/anna/x", "/home/anna/x", ""},
		{"root", "/", "/", "/", ""},
		{"macro", "${DOWNLOADS}", "${DOWNLOADS}", "Downloads", "${DOWNLOADS}"},
		{"macro folder", "${DOCUMENTS}/books", "${DOCUMENTS}/books", "Documents/books", "${DOCUMENTS}/books"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalFolderPath(tt.path, home)
			if got != tt.canonical {
				t.Errorf("canonicalFolderPath(%q) = %q, want %q", tt.path, got, tt.canonical)
			}
			text, hint := displayFolderPath(tt.path, home)
			if text != tt.text || hint != tt.textHint {
				t.Errorf("displayFolderPath(%q) = %q, %q, want %q, %q", tt.path, text, hint, tt.text, tt.textHint)
			}
		})
	}
}

func TestSetHomeKeepsThePrivateFolderInStep(t *testing.T) {
	e := newFilesEditor(t, "default.profile")
	want := filepath.Join(e.HomesDir(), "firejail-code")
	tests := []struct {
		name        string
		mode        firejail.HomeMode
		dir         string
		homeChanged bool
	}{
		{"own private home gets its folder", firejail.HomePrivate, want, true},
		{"empty home gives the folder back", firejail.HomeEmpty, "", true},
		{"only the folders I choose", firejail.HomeChosen, "", true},
		{"full home is the base again", firejail.HomeFull, "", false},
		{"own private home again", firejail.HomePrivate, want, true},
		{"full home again", firejail.HomeFull, "", false},
	}
	for _, tt := range tests {
		e.SetHome(tt.mode)
		s := e.Final()
		if s.Home != tt.mode || s.PrivateDir != tt.dir || e.HomeChanged() != tt.homeChanged {
			t.Errorf("%s: home %q, dir %q, changed %v; want %q, %q, %v",
				tt.name, s.Home, s.PrivateDir, e.HomeChanged(), tt.mode, tt.dir, tt.homeChanged)
		}
	}
	if e.HasChanges() {
		t.Errorf("going back to the base left changes: %+v", e.ChangedFolders())
	}

	e.SetHome(firejail.HomePrivate)
	if got := e.PrivateHomePath(); got != want {
		t.Errorf("PrivateHomePath = %q, want %q", got, want)
	}
	e.ResetHome()
	if e.HomeChanged() || e.Final().Home != e.Base().Settings.Home || e.Final().PrivateDir != "" {
		t.Errorf("ResetHome left home %q dir %q", e.Final().Home, e.Final().PrivateDir)
	}
	// The path the choice would use is known before it is chosen.
	if got := e.PrivateHomePath(); got != want {
		t.Errorf("PrivateHomePath before the choice = %q, want %q", got, want)
	}
}

func TestCheckFolder(t *testing.T) {
	tests := []struct {
		name    string
		home    firejail.HomeMode
		path    string
		want    string // the path to add, "" when refused
		problem string // text the message has
	}{
		{name: "outside the home", home: firejail.HomeFull, path: "/mnt/data", want: "/mnt/data"},
		{name: "trailing slash", home: firejail.HomeFull, path: "/mnt/data/", want: "/mnt/data"},
		{name: "real path in the home", home: firejail.HomeFull, path: "HOME/Projects", want: "${HOME}/Projects"},
		{name: "tilde", home: firejail.HomeFull, path: "~/Projects", want: "${HOME}/Projects"},
		{name: "macro", home: firejail.HomeChosen, path: "${DOWNLOADS}/books", want: "${DOWNLOADS}/books"},
		{name: "relative", home: firejail.HomeFull, path: "Projects", problem: "must be a full path"},
		{name: "dots", home: firejail.HomeFull, path: "/mnt/../etc", problem: `".."`},
		{name: "empty", home: firejail.HomeFull, path: "", problem: "must be a full path"},
		{name: "a hash", home: firejail.HomeFull, path: "/mnt/a#b", problem: "# character"},
		{name: "the home itself", home: firejail.HomeFull, path: "~", problem: "home folder itself"},
		{name: "in the list already", home: firejail.HomeFull, path: "~/.vscode", problem: "in the list already"},
		{name: "in the list already, other spelling", home: firejail.HomeFull, path: "HOME/.vscode", problem: "in the list already"},
		{name: "empty home refuses the real home", home: firejail.HomeEmpty, path: "~/Projects", problem: "inside your home folder"},
		{name: "private home refuses the real home", home: firejail.HomePrivate, path: "HOME/Projects", problem: "inside your home folder"},
		{name: "private home refuses a macro", home: firejail.HomePrivate, path: "${DOWNLOADS}", problem: "inside your home folder"},
		{name: "private home refuses the home itself", home: firejail.HomePrivate, path: "~", problem: "inside your home folder"},
		{name: "private home takes a folder outside", home: firejail.HomePrivate, path: "/opt/projects", want: "/opt/projects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newFilesEditor(t, "code.profile")
			e.SetHome(tt.home)
			path := strings.Replace(tt.path, "HOME", e.home, 1)
			got, problem := e.CheckFolder(path)
			if got != tt.want || (tt.problem == "") != (problem == "") || !strings.Contains(problem, tt.problem) {
				t.Errorf("CheckFolder(%q) = %q, %q; want %q and a message with %q", path, got, problem, tt.want, tt.problem)
			}
			if strings.Contains(problem, "${HOME}/Projects") {
				t.Errorf("the message shows the variable, not the path the user knows: %q", problem)
			}
		})
	}
}

func TestEditorFolderChanges(t *testing.T) {
	e := newFilesEditor(t, "code.profile")
	const profileRow = "${HOME}/.vscode"
	rows := len(e.Final().Folders)

	e.SetFolderAccess(profileRow, firejail.AccessReadOnly)
	if !e.FolderChanged(profileRow) {
		t.Error("a profile row with another access is not changed")
	}
	e.SetFolderAccess(profileRow, firejail.AccessReadWrite)
	if e.FolderChanged(profileRow) || e.HasChanges() {
		t.Error("the row is back to the profile's access but is still changed")
	}

	e.RemoveFolder(profileRow)
	if got := e.RemovedFolders(); len(got) != 1 || got[0].Path != profileRow || !got[0].FromProfile {
		t.Fatalf("RemovedFolders = %+v", got)
	}
	// Adding a folder the profile has, and the user removed, brings the profile's row back.
	e.AddFolder(profileRow)
	if e.HasChanges() || len(e.Final().Folders) != rows || len(e.RemovedFolders()) != 0 {
		t.Errorf("the row did not come back as it was: %d rows, changes %v", len(e.Final().Folders), e.HasChanges())
	}

	e.AddFolder("/mnt/data")
	if got := e.Final().Folders; len(got) != rows+1 || got[rows].Access != firejail.AccessReadWrite || got[rows].FromProfile {
		t.Errorf("the added row = %+v", got)
	}
	e.RemoveFolder("/mnt/data")
	if e.HasChanges() {
		t.Error("an added row that was removed is still a change")
	}
}

func TestLockedFolderCannotChange(t *testing.T) {
	e := newFilesEditor(t, "default.profile")
	locked := firejail.Folder{Path: "/opt/ro", Access: firejail.AccessReadOnly, FromProfile: true, Locked: true}
	e.base.Settings.Folders = append(e.base.Settings.Folders, locked)
	e.baseline.Folders = append(e.baseline.Folders, locked)
	e.final.Folders = append(e.final.Folders, locked)

	e.SetFolderAccess("/opt/ro", firejail.AccessReadWrite)
	e.RemoveFolder("/opt/ro")
	got := e.Final().Folders
	if len(got) != 1 || got[0].Access != firejail.AccessReadOnly || e.HasChanges() {
		t.Errorf("a locked row changed: %+v", got)
	}
}
