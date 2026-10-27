package ui

import (
	"path/filepath"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// uninstallPage opens the installed build of Visual Studio Code, which has the files and
// the private home the tests list.
func uninstallPage(t *testing.T, withHome bool) (*page, []store.InstalledFile) {
	t.Helper()
	p := newPage(t, fixtureBackend(t))
	home := p.backend.fj.dirs.Home
	files := []store.InstalledFile{
		{Kind: store.FileKindProfile, Path: filepath.Join(home, ".local/share/jailcat/profiles/firejail-code.profile")},
		{Kind: store.FileKindLauncher, Path: filepath.Join(home, ".local/bin/firejail-code")},
		{Kind: store.FileKindIcon, Path: filepath.Join(home, ".local/share/icons/hicolor/scalable/apps/firejail-code.svg")},
		{Kind: store.FileKindDesktop, Path: filepath.Join(home, ".local/share/applications/firejail-code.desktop")},
	}
	if withHome {
		files = append(files, store.InstalledFile{Kind: store.FileKindHome, Path: filepath.Join(home, ".local/share/jailcat/homes/firejail-code")})
	}
	removed := make([]install.File, 0, len(files))
	for _, f := range files {
		if f.Kind != store.FileKindHome {
			removed = append(removed, install.File{Kind: install.Kind(f.Kind), Path: f.Path})
		}
	}
	p.backend.set(func(f *fakeBackend) {
		f.installedFiles = files
		f.uninstallResults = []install.UninstallResult{{Removed: removed}}
	})
	p.ready()
	p.openApp("Visual Studio Code")
	return p, files
}

// waitUninstallDialog presses Uninstall and waits for the dialog.
func (p *page) waitUninstallDialog() {
	p.t.Helper()
	p.tap("button:uninstall")
	p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*UninstallDialog]()) })
}

func TestUninstallDialogOffersTheOptions(t *testing.T) {
	t.Run("a build with a private home", func(t *testing.T) {
		p, files := uninstallPage(t, true)
		p.waitUninstallDialog()
		for i := range 4 {
			if !p.ts.Exists(key("uninstall:file:" + string(rune('0'+i)))) {
				t.Errorf("file %d is not listed", i)
			}
		}
		if !p.ts.Exists(gluttertest.ByTextContaining("Launcher: ")) {
			t.Error("the launcher is not listed")
		}
		// The home is offered with its path, off; the settings are kept, on.
		if !p.ts.Exists(key("uninstall:home")) || p.checked("uninstall:home") {
			t.Error("the home option is missing or on")
		}
		if !p.ts.Exists(gluttertest.ByTextContaining(".local/share/jailcat/homes/firejail-code")) {
			t.Errorf("the path of the home is not shown (%s)", files[4].Path)
		}
		if !p.checked("uninstall:keep") {
			t.Error("Keep its settings is off")
		}
	})
	t.Run("a build without one", func(t *testing.T) {
		p, _ := uninstallPage(t, false)
		p.waitUninstallDialog()
		if p.ts.Exists(key("uninstall:home")) {
			t.Error("the home option is offered for a build with no private home")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		p, _ := uninstallPage(t, true)
		p.waitUninstallDialog()
		p.tap("uninstall:cancel")
		p.waitFor("the dialog to close", func() bool { return !p.ts.Exists(gluttertest.ByType[*UninstallDialog]()) })
		p.waitIdle()
		if len(p.backend.uninstallAsks) != 0 {
			t.Error("Cancel uninstalled")
		}
	})
}

func TestUninstallChoices(t *testing.T) {
	tests := []struct {
		name string
		// tick toggles these checkboxes before Uninstall is pressed.
		tick []string
		want UninstallChoice
		// kept says the build stays in the editor as not installed.
		kept bool
	}{
		{name: "the defaults", want: UninstallChoice{KeepSettings: true}, kept: true},
		{name: "delete the private home", tick: []string{"uninstall:home"}, want: UninstallChoice{KeepSettings: true, DeleteHome: true}, kept: true},
		{name: "do not keep the settings", tick: []string{"uninstall:keep"}, want: UninstallChoice{}},
		{name: "both", tick: []string{"uninstall:home", "uninstall:keep"}, want: UninstallChoice{DeleteHome: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := uninstallPage(t, true)
			e := p.home().editor
			p.waitUninstallDialog()
			for _, k := range tt.tick {
				p.tap(k)
			}
			p.tap("uninstall:confirm")
			if tt.kept {
				p.waitFor("the banner", func() bool { return p.editorState().outcome != nil })
			} else {
				p.waitFor("the editor to close", func() bool { return p.home().editor == nil })
			}
			asks := p.backend.uninstallAsks
			if len(asks) != 1 || asks[0].DeleteHome != tt.want.DeleteHome || asks[0].KeepSettings != tt.want.KeepSettings || len(asks[0].Force) != 0 {
				t.Fatalf("Uninstall was called with %+v, want %+v", asks, tt.want)
			}
			if tt.kept {
				o := p.editorState().outcome
				if o.title != "firejail-code was uninstalled" || len(o.lines) != 4 {
					t.Errorf("outcome = %+v", o)
				}
				if e.Build().Installed || p.ts.Exists(key("button:uninstall")) {
					t.Error("the build is still shown as installed")
				}
				// The row is a draft now, not built.
				p.waitFor("the list", func() bool { return p.home().builds[AppKey(e.App())] == 0 && p.home().drafts[AppKey(e.App())] == 1 })
				return
			}
			p.waitFor("the list", func() bool { return p.home().builds[AppKey(e.App())] == 0 })
			if !p.ts.Exists(gluttertest.ByText(emptyStateText)) {
				t.Error("the editor of a removed build is still shown")
			}
			if p.home().drafts[AppKey(e.App())] != 0 {
				t.Error("the removed build is still a draft")
			}
		})
	}
}

func TestUninstallKeepsEditedFilesUntilTheUserSaysSo(t *testing.T) {
	launcher := func(p *page) install.File {
		return install.File{Kind: install.KindLauncher, Path: filepath.Join(p.backend.fj.dirs.Home, ".local/bin/firejail-code")}
	}
	tests := []struct {
		name   string
		button string
		// want is the choice of the second call.
		check func(t *testing.T, p *page, second UninstallChoice)
	}{
		{
			name: "remove them anyway", button: "kept:remove",
			check: func(t *testing.T, p *page, second UninstallChoice) {
				if !second.Force[launcher(p).Path] || second.LeaveKept {
					t.Errorf("second call = %+v", second)
				}
			},
		},
		{
			name: "leave them", button: "kept:leave",
			check: func(t *testing.T, p *page, second UninstallChoice) {
				if len(second.Force) != 0 || !second.LeaveKept {
					t.Errorf("second call = %+v", second)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := uninstallPage(t, false)
			p.backend.set(func(f *fakeBackend) {
				f.uninstallResults = []install.UninstallResult{
					{Removed: []install.File{{Kind: install.KindDesktop, Path: "/x/a.desktop"}}, Kept: []install.File{launcher(p)}},
					{},
				}
			})
			p.waitUninstallDialog()
			p.tap("uninstall:confirm")
			p.waitFor("the question", func() bool { return p.ts.Exists(gluttertest.ByType[*KeptFilesDialog]()) })
			if !p.ts.Exists(gluttertest.ByTextContaining("Launcher: ")) {
				t.Error("the edited file is not listed")
			}
			// Nothing is finished while the files are kept: the build is still installed.
			if !p.home().editor.Build().Installed || len(p.backend.savesList()) != 0 {
				t.Error("the uninstall finished before the user decided")
			}
			p.tap(tt.button)
			p.waitFor("the banner", func() bool { return p.editorState().outcome != nil })
			if len(p.backend.uninstallAsks) != 2 {
				t.Fatalf("Uninstall was called %d times", len(p.backend.uninstallAsks))
			}
			tt.check(t, p, p.backend.uninstallAsks[1])
			if p.home().editor.Build().Installed {
				t.Error("the build is still installed")
			}
			// What the first run removed is in the banner.
			if o := p.editorState().outcome; len(o.lines) != 1 {
				t.Errorf("outcome lines = %v", o.lines)
			}
		})
	}
}

func TestUninstallFailure(t *testing.T) {
	p, _ := uninstallPage(t, false)
	p.backend.set(func(f *fakeBackend) {
		f.uninstallErr = errors.New("UninstallRemoveFailed", "the file is on a read-only disk")
	})
	p.waitUninstallDialog()
	p.tap("uninstall:confirm")
	p.waitFor("the message", func() bool { return p.ts.Exists(gluttertest.ByTextContaining("could not finish the uninstall")) })
	if p.ts.Exists(gluttertest.ByTextContaining("UninstallRemoveFailed")) {
		t.Error("the tag is shown to the user")
	}
	p.waitIdle()
	if !p.home().editor.Build().Installed {
		t.Error("a failed uninstall changed the build")
	}
}
