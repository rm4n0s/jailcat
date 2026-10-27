package ui

import (
	"path/filepath"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// rowHas reports whether the row of the app shows the chip.
func (p *page) rowHas(app, chip string) bool {
	p.t.Helper()
	// The list is lazy: search for the app so that its row is built.
	if !p.ts.Exists(gluttertest.ByKey(m.NewValueKey("desktop:" + app + ".desktop"))) {
		p.ts.EnterText(gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.ByType[*m.TextField]()), app)
		p.ts.Pump()
	}
	return p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*AppList](), gluttertest.Descendant(
		gluttertest.ByKey(m.NewValueKey("desktop:"+app+".desktop")), gluttertest.ByText(chip))))
}

func TestFirejailUpdatedMark(t *testing.T) {
	tests := []struct {
		name string
		// version is the firejail version the build was made with; running is the installed one.
		version, running string
		want             bool
	}{
		{"the same version", "0.9.80", "0.9.80", false},
		{"another version", "0.9.72", "0.9.80", true},
		{"a build without a version", "", "0.9.80", false},
		{"an unknown running version", "0.9.72", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := fixtureBackend(t)
			backend.set(func(f *fakeBackend) {
				f.builds[2].FirejailVersion = tt.version
				f.firejail.Version = tt.running
			})
			p := newPage(t, backend)
			p.ready()
			p.waitFor("the row", func() bool { return p.rowHas("Visual Studio Code", "built") })
			if got := p.rowHas("Visual Studio Code", firejailUpdatedText); got != tt.want {
				t.Errorf("the row shows the mark = %v, want %v", got, tt.want)
			}
			if p.rowHas("Calculator", firejailUpdatedText) || p.rowHas("Firefox", firejailUpdatedText) {
				t.Error("an app without that build shows the mark")
			}
			p.openApp("Visual Studio Code")
			if got := p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*EditorPage](), gluttertest.ByText(firejailUpdatedText))); got != tt.want {
				t.Errorf("the header shows the mark = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirejailUpdatedMarkGoesAfterABuild(t *testing.T) {
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds[2].FirejailVersion = "0.9.72" })
	p := newPage(t, backend)
	p.ready()
	e := p.openApp("Visual Studio Code")
	if !e.FirejailUpdated() {
		t.Fatal("the editor does not know firejail was updated")
	}
	// The fixture's stored build has no launcher fields; a real one has them.
	p.change(func(e *Editor) { e.Set(func(s *firejail.Settings) { s.MenuName = "Code (sandboxed)" }) })
	p.tap("button:build")
	p.waitOutcome()
	p.waitFor("the mark to go", func() bool { return !p.rowHas("Visual Studio Code", firejailUpdatedText) })
	if p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*EditorPage](), gluttertest.ByText(firejailUpdatedText))) {
		t.Error("the header keeps the mark")
	}
}

func TestBaseProfileMissingMark(t *testing.T) {
	backend := fixtureBackend(t)
	gone := filepath.Join(backend.fj.dirs.FirejailSystem, "missing.profile")
	present := filepath.Join(backend.fj.dirs.FirejailSystem, "code.profile")
	backend.set(func(f *fakeBackend) {
		f.builds[2].BaseProfile = gone
		f.builds[0].BaseProfile = present
	})
	p := newPage(t, backend)
	p.ready()
	p.waitFor("the row", func() bool { return p.rowHas("Visual Studio Code", baseMissingText) })
	if p.rowHas("Firefox", baseMissingText) {
		t.Error("an app whose profile is there shows the mark")
	}
	e := p.openApp("Visual Studio Code")
	if !e.BaseMissing() || !p.ts.Exists(gluttertest.Descendant(gluttertest.ByType[*EditorPage](), gluttertest.ByText(baseMissingText))) {
		t.Error("the header does not show the mark")
	}
	if !p.ts.Exists(gluttertest.ByTextContaining("Choose another profile")) {
		t.Error("the editor does not ask for another profile")
	}

	// Choosing another profile ends it.
	p.openProfileDialog()
	p.ts.EnterText(inDialog(gluttertest.ByType[*m.TextField]()), "electron")
	p.ts.Pump()
	p.pickProfile("electron-common")
	// The fixture's stored build differs from the new profile's values: it asks.
	p.waitFor("the question", func() bool { return p.ts.Exists(gluttertest.ByType[*KeepChangesDialog]()) })
	p.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*KeepChangesDialog](), gluttertest.ByText(keepButtonText)))
	p.ts.Pump()
	p.waitFor("the profile", func() bool { return e.BaseProfile() != "" && !p.editorState().reading })
	inEditor := gluttertest.Descendant(gluttertest.ByType[*EditorPage](), gluttertest.ByText(baseMissingText))
	if e.BaseMissing() || p.ts.Exists(inEditor) {
		t.Error("the mark stays after another profile was chosen")
	}
}

func TestMarksOnlyForBuildsThatAreThere(t *testing.T) {
	backend := fixtureBackend(t)
	var gone, updated []store.Build
	gone = append(gone, store.Build{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop", BaseProfile: "/nope/x.profile"})
	updated = append(updated,
		store.Build{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop", Installed: true, FirejailVersion: "0.9.72"},
		// Not installed any more: nothing to refresh.
		store.Build{AppKind: store.AppKindDesktop, DesktopFileID: "b.desktop", FirejailVersion: "0.9.72"},
	)
	if got := missingBaseApps(t.Context(), backend, gone); !got["desktop:a.desktop"] || len(got) != 1 {
		t.Errorf("missingBaseApps = %v", got)
	}
	if got := updatedApps(updated, "0.9.80"); !got["desktop:a.desktop"] || len(got) != 1 {
		t.Errorf("updatedApps = %v", got)
	}
}
