package ui

import (
	"path/filepath"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
)

func TestGoldenPreviewDialog(t *testing.T) {
	p := newVSCodePage(t)
	p.change(func(e *Editor) {
		e.Set(func(s *firejail.Settings) {
			s.Network = firejail.NetNone
			s.Sound = true
			s.Camera = true
			s.GPU = true
			s.Printers = true
			s.U2F = true
			s.Input = true
			s.DVD = true
			s.TV = true
		})
		e.SetExtraRules([]string{
			"dbus-user.talk org.freedesktop.Notifications", "dbus-user.talk org.freedesktop.portal.Desktop",
			"dbus-user.talk org.freedesktop.secrets", "dbus-user.talk org.kde.StatusNotifierWatcher",
		})
	})
	p.tap("button:preview")
	p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*PreviewDialog]()) })
	// The profile is longer than the dialog, so its last line, the include of the base
	// profile with the path of the fixture folder on this machine, is not in the picture.
	prepared, err := p.backend.PrepareBuild(t.Context(), p.home().editor.Request())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(prepared.Generated.Lines); n < 22 {
		t.Fatalf("the profile has %d lines: the include would show in the golden", n)
	}
	p.golden("preview-dialog", "4.7-preview.png")
}

func TestGoldenConflictsDialog(t *testing.T) {
	p := buildPage(t, "Calculator")
	home := p.backend.fj.dirs.Home
	p.backend.set(func(f *fakeBackend) {
		f.conflicts = []install.Conflict{
			{File: install.File{Kind: install.KindLauncher, Path: filepath.Join(home, ".local/bin/firejail-calculator")}, Kind: install.ConflictEdited, Overwritable: true},
			{File: install.File{Kind: install.KindDesktop, Path: filepath.Join(home, ".local/share/applications/firejail-calculator.desktop")}, Kind: install.ConflictForeign},
		}
	})
	p.tap("button:build")
	p.waitFor("the dialog", func() bool { return p.ts.Exists(gluttertest.ByType[*ConflictsDialog]()) })
	p.golden("conflicts-dialog", "4.7-conflict.png")
}

func TestGoldenEditorAfterABuild(t *testing.T) {
	p := buildPage(t, "Calculator")
	p.tap("button:build")
	p.waitOutcome()
	p.waitFor("the list", func() bool { return p.home().builds[AppKey(p.home().editor.App())] == 1 })
	p.golden("editor-built", "")
}
