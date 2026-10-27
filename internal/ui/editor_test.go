package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// fixtureFirejail is a copy of the firejail folders for the tests: /etc/firejail is the
// fixture of internal/firejail (the profile chain of Visual Studio Code), the user
// folder and the home are empty temporary folders.
type fixtureFirejail struct {
	t    *testing.T
	dirs xdg.Dirs
	opts firejail.ReadOpts
}

// newFixtureFirejail makes the folders.
func newFixtureFirejail(t *testing.T) fixtureFirejail {
	t.Helper()
	sys, err := filepath.Abs(filepath.Join("..", "firejail", "testdata", "etc-firejail"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dirs := xdg.Dirs{
		Home: filepath.Join(root, "home"), FirejailSystem: sys, FirejailUser: filepath.Join(root, "user"),
		Imported: filepath.Join(root, "imported"),
		Homes:    filepath.Join(root, "home", ".local", "share", "jailcat", "homes"),
	}
	cfg, err := firejail.ReadConfig(context.Background(), filepath.Join(sys, "firejail.config"))
	if err != nil {
		t.Fatal(err)
	}
	return fixtureFirejail{t: t, dirs: dirs, opts: firejail.ReadOpts{Dirs: dirs, Config: cfg}}
}

// path is the path of a profile of the fixture; "" is base None.
func (f fixtureFirejail) path(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(f.dirs.FirejailSystem, name)
}

// base reads a profile of the fixture and computes its base values.
func (f fixtureFirejail) base(name string) (firejail.Base, *firejail.Profile) {
	f.t.Helper()
	if name == "" {
		return firejail.BaseNone(), nil
	}
	p, err := firejail.Read(context.Background(), f.path(name), f.opts)
	if err != nil {
		f.t.Fatal(err)
	}
	return firejail.BaseValues(p, f.dirs.Home), p
}

// codeApp is Visual Studio Code, a menu app.
func codeApp() apps.App {
	return apps.App{
		ID: "code.desktop", Kind: apps.KindDesktop, Name: "Visual Studio Code", Comment: "Code Editing. Redefined.",
		Binary: "/usr/bin/code", BinaryName: "code",
	}
}

// codeDefaults are the launcher fields of a new build of codeApp.
func codeDefaults() firejail.Settings {
	s := launcherDefaults(codeApp())
	s.SandboxName = "firejail-code"
	s.IconSeed = 42
	return s
}

// newEditor opens a new build of codeApp on the fixture profile ("" is None).
func newEditor(t *testing.T, f fixtureFirejail, profile string) *Editor {
	t.Helper()
	base, p := f.base(profile)
	return NewEditor(codeApp(), EditorOpts{
		Home: f.dirs.Home, BaseProfile: f.path(profile), Profile: p, Base: base, Defaults: codeDefaults(),
	})
}

func TestNewEditorStartsClean(t *testing.T) {
	f := newFixtureFirejail(t)
	for _, profile := range []string{"", "default.profile", "code.profile"} {
		t.Run(profile, func(t *testing.T) {
			e := newEditor(t, f, profile)
			s := e.Final()
			if s.SandboxName != "firejail-code" || s.MenuName != "Visual Studio Code (sandboxed)" ||
				s.Comment != "Code Editing. Redefined." || s.IconSeed != 42 || s.Terminal {
				t.Errorf("launcher fields = %q %q %q %d %v", s.SandboxName, s.MenuName, s.Comment, s.IconSeed, s.Terminal)
			}
			if got := e.ChangedTabs(); len(got) != 0 {
				t.Errorf("ChangedTabs = %v, want none", got)
			}
			if e.HasChanges() || e.ChangeCount() != 0 || e.Dirty() || e.HasDraft() {
				t.Errorf("HasChanges %v, ChangeCount %d, Dirty %v, HasDraft %v", e.HasChanges(), e.ChangeCount(), e.Dirty(), e.HasDraft())
			}
			// The launcher fields are stored with the build, as overrides of the base.
			for _, field := range []firejail.Field{firejail.FieldSandboxName, firejail.FieldMenuName, firejail.FieldComment, firejail.FieldIconSeed} {
				if !e.Overrides().Changed(field) {
					t.Errorf("%s is not in the stored overrides", field)
				}
			}
			if probs := e.Problems(); len(probs) != 0 {
				t.Errorf("Problems = %+v", probs)
			}
		})
	}
}

func TestEditorBaseValuesFollowTheProfile(t *testing.T) {
	f := newFixtureFirejail(t)
	// code.profile says nosound; Default does not.
	if newEditor(t, f, "code.profile").Final().Sound {
		t.Error("code.profile: sound is allowed")
	}
	if !newEditor(t, f, "default.profile").Final().Sound {
		t.Error("default.profile: sound is blocked")
	}
	// Base None has Jailcat's security defaults on.
	if s := newEditor(t, f, "").Final(); !s.Seccomp || !s.Sound {
		t.Errorf("None: seccomp %v, sound %v", s.Seccomp, s.Sound)
	}
}

func TestEditorSetAndReset(t *testing.T) {
	f := newFixtureFirejail(t)
	tests := []struct {
		name  string
		field firejail.Field
		tab   firejail.Tab
		set   func(s *firejail.Settings)
	}{
		{"network", firejail.FieldNetwork, firejail.TabNetwork, func(s *firejail.Settings) { s.Network = firejail.NetNone }},
		{"sound", firejail.FieldSound, firejail.TabDevices, func(s *firejail.Settings) { s.Sound = !s.Sound }},
		{"memory", firejail.FieldMemory, firejail.TabLimits, func(s *firejail.Settings) { s.MemoryBytes = 1 << 30 }},
		{"menu name", firejail.FieldMenuName, firejail.TabLauncher, func(s *firejail.Settings) { s.MenuName = "My Code" }},
		{"terminal", firejail.FieldTerminal, firejail.TabLauncher, func(s *firejail.Settings) { s.Terminal = true }},
		{"environment", firejail.FieldEnv, firejail.TabPrograms, func(s *firejail.Settings) {
			s.Env = []firejail.EnvVar{{Name: "A", Value: "b"}}
		}},
	}
	for _, profile := range []string{"", "code.profile"} {
		for _, tt := range tests {
			t.Run(tt.name+"/"+profile, func(t *testing.T) {
				e := newEditor(t, f, profile)
				before := e.Final()
				e.Set(tt.set)
				if !e.Changed(tt.field) {
					t.Fatalf("%s is not changed", tt.field)
				}
				if got := e.ChangedTabs(); len(got) != 1 || !got[tt.tab] {
					t.Errorf("ChangedTabs = %v, want only %s", got, tt.tab)
				}
				if !e.Dirty() || !e.HasChanges() || e.ChangeCount() != 1 {
					t.Errorf("Dirty %v, HasChanges %v, ChangeCount %d", e.Dirty(), e.HasChanges(), e.ChangeCount())
				}
				if !e.Overrides().Changed(tt.field) {
					t.Errorf("%s is not in the overrides", tt.field)
				}

				e.Reset(tt.field)
				if e.Changed(tt.field) || len(e.ChangedTabs()) != 0 || e.Dirty() || e.ChangeCount() != 0 {
					t.Errorf("after Reset: changed %v, tabs %v, dirty %v", e.Changed(tt.field), e.ChangedTabs(), e.Dirty())
				}
				after := e.Final()
				if !slices.Equal(after.Env, before.Env) || after.Network != before.Network || after.Sound != before.Sound ||
					after.MemoryBytes != before.MemoryBytes || after.MenuName != before.MenuName || after.Terminal != before.Terminal {
					t.Errorf("Final after Reset = %+v, want %+v", after, before)
				}
				if after.SandboxName != "firejail-code" || after.IconSeed != 42 {
					t.Errorf("Reset lost the sandbox name or the seed: %q %d", after.SandboxName, after.IconSeed)
				}
			})
		}
	}
}

func TestEditorSettingBackToTheBaseValueIsNotAChange(t *testing.T) {
	e := newEditor(t, newFixtureFirejail(t), "default.profile")
	orig := e.Final().Firewall
	e.Set(func(s *firejail.Settings) { s.Firewall = !orig })
	if !e.Changed(firejail.FieldFirewall) {
		t.Fatal("the change is not seen")
	}
	e.Set(func(s *firejail.Settings) { s.Firewall = orig })
	if e.Changed(firejail.FieldFirewall) || e.Dirty() {
		t.Errorf("changed %v, dirty %v", e.Changed(firejail.FieldFirewall), e.Dirty())
	}
}

func TestEditorSandboxNameAndSeedAreNotChanges(t *testing.T) {
	e := newEditor(t, newFixtureFirejail(t), "")
	e.Set(func(s *firejail.Settings) { s.SandboxName = "firejail-code-work"; s.IconSeed = 7 })
	if e.Changed(firejail.FieldSandboxName) || e.Changed(firejail.FieldIconSeed) || len(e.ChangedTabs()) != 0 {
		t.Errorf("tabs = %v", e.ChangedTabs())
	}
	// They are unsaved changes all the same.
	if !e.Dirty() {
		t.Error("not dirty after renaming")
	}
}

func TestEditorFolders(t *testing.T) {
	f := newFixtureFirejail(t)
	e := newEditor(t, f, "code.profile")
	baseRows := e.Final().Folders
	if len(baseRows) == 0 {
		t.Fatal("the code profile has no folder rows")
	}
	first := baseRows[0]

	e.Set(func(s *firejail.Settings) {
		s.Folders = append(s.Folders, firejail.Folder{Path: "/mnt/data", Access: firejail.AccessReadOnly})
		s.Folders = slices.DeleteFunc(s.Folders, func(r firejail.Folder) bool { return r.Path == first.Path })
	})
	tabs := e.ChangedTabs()
	if !tabs[firejail.TabFiles] || len(tabs) != 1 {
		t.Errorf("ChangedTabs = %v, want only Files", tabs)
	}
	if !e.FolderChanged("/mnt/data") || !e.FolderChanged(first.Path) || e.FolderChanged("/nowhere") {
		t.Errorf("FolderChanged wrong: %+v", e.ChangedFolders())
	}

	e.ResetFolder("/mnt/data")
	if e.FolderChanged("/mnt/data") || !e.FolderChanged(first.Path) {
		t.Errorf("after ResetFolder: %+v", e.ChangedFolders())
	}
	e.ResetFolder(first.Path)
	if len(e.ChangedFolders()) != 0 || e.Dirty() || len(e.ChangedTabs()) != 0 {
		t.Errorf("after both resets: %+v, dirty %v", e.ChangedFolders(), e.Dirty())
	}
	if got := e.Final().Folders; !slices.Equal(got, baseRows) {
		t.Errorf("folders = %+v, want %+v", got, baseRows)
	}
}

func TestEditorExtraRulesDot(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		dot  bool
	}{
		{"none", nil, false},
		{"only an empty row", []string{"", "  "}, false},
		{"a rule", []string{"", "net none"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEditor(t, newFixtureFirejail(t), "")
			e.SetExtraRules(tt.rows)
			if got := e.ChangedTabs()[TabExtraRules]; got != tt.dot {
				t.Errorf("dot = %v, want %v", got, tt.dot)
			}
			if got := e.Dirty(); got != (len(tt.rows) > 0) {
				t.Errorf("Dirty = %v for rows %q", got, tt.rows)
			}
			// Extra rules are not overrides.
			if e.HasChanges() {
				t.Error("HasChanges is true for Extra rules")
			}
		})
	}
}

func TestEditorProblems(t *testing.T) {
	e := newEditor(t, newFixtureFirejail(t), "")
	e.Set(func(s *firejail.Settings) { s.SandboxName = "Bad Name" })
	probs := e.Problems()
	if len(probs) != 1 || probs[0].Tag != firejail.ProblemSandboxName {
		t.Errorf("Problems = %+v", probs)
	}
}

func TestEditorSwitchProfile(t *testing.T) {
	f := newFixtureFirejail(t)
	defBase, defProfile := f.base("default.profile")
	codeBase, _ := f.base("code.profile")
	if codeBase.Settings.Sound == defBase.Settings.Sound {
		t.Fatal("the fixtures no longer differ in sound")
	}

	tests := []struct {
		name string
		keep bool
		// edit is what the user changes on code.profile first.
		edit func(s *firejail.Settings)
		// check looks at the final settings after the switch to default.profile.
		check func(t *testing.T, e *Editor)
	}{
		{
			name: "keep: a change stays, the rest follows the new base",
			keep: true,
			edit: func(s *firejail.Settings) { s.Network = firejail.NetNone },
			check: func(t *testing.T, e *Editor) {
				s := e.Final()
				if s.Network != firejail.NetNone || !e.Changed(firejail.FieldNetwork) {
					t.Error("the network change was lost")
				}
				if s.Sound != defBase.Settings.Sound {
					t.Errorf("sound = %v, want the Default profile's %v", s.Sound, defBase.Settings.Sound)
				}
				if e.Changed(firejail.FieldSound) {
					t.Error("sound is marked changed")
				}
			},
		},
		{
			name: "keep: a change that equals the new base value is dropped",
			keep: true,
			edit: func(s *firejail.Settings) { s.Sound = defBase.Settings.Sound },
			check: func(t *testing.T, e *Editor) {
				if e.Changed(firejail.FieldSound) || e.HasChanges() {
					t.Errorf("sound is still a change: %+v", e.Overrides().ChangedFields())
				}
				if e.Overrides().Changed(firejail.FieldSound) {
					t.Error("sound is still in the stored overrides")
				}
			},
		},
		{
			name: "keep: the menu name the user typed stays",
			keep: true,
			edit: func(s *firejail.Settings) { s.MenuName = "Work" },
			check: func(t *testing.T, e *Editor) {
				if e.Final().MenuName != "Work" || !e.Changed(firejail.FieldMenuName) {
					t.Errorf("menu name = %q", e.Final().MenuName)
				}
			},
		},
		{
			name: "start over: every change goes, the sandbox name stays",
			keep: false,
			edit: func(s *firejail.Settings) {
				s.Network = firejail.NetNone
				s.MenuName = "Work"
				s.SandboxName = "firejail-work"
				s.Folders = append(s.Folders, firejail.Folder{Path: "/mnt/x", Access: firejail.AccessReadWrite})
			},
			check: func(t *testing.T, e *Editor) {
				s := e.Final()
				if e.HasChanges() || len(e.ChangedTabs()) != 0 {
					t.Errorf("changes remain: %+v %v", e.Overrides().ChangedFields(), e.ChangedFolders())
				}
				if s.Network != defBase.Settings.Network || s.MenuName != "Visual Studio Code (sandboxed)" {
					t.Errorf("network %q, menu name %q", s.Network, s.MenuName)
				}
				if s.SandboxName != "firejail-work" || s.IconSeed != 42 {
					t.Errorf("sandbox name %q, seed %d", s.SandboxName, s.IconSeed)
				}
				if len(s.Folders) != len(defBase.Settings.Folders) {
					t.Errorf("%d folders, want the base's %d", len(s.Folders), len(defBase.Settings.Folders))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEditor(t, f, "code.profile")
			e.Set(tt.edit)
			e.SwitchProfile(f.path("default.profile"), defBase, defProfile, tt.keep)
			if e.BaseProfile() != f.path("default.profile") || e.Profile() != defProfile {
				t.Errorf("BaseProfile = %q", e.BaseProfile())
			}
			// The stored overrides are the difference from the new base.
			if !e.Overrides().Changed(firejail.FieldSandboxName) {
				t.Error("the sandbox name left the stored overrides")
			}
			tt.check(t, e)
			if !e.Dirty() {
				t.Error("a new base profile is an unsaved change")
			}
		})
	}
}

func TestEditorSwitchProfileToNone(t *testing.T) {
	f := newFixtureFirejail(t)
	e := newEditor(t, f, "code.profile")
	none, _ := f.base("")
	e.SwitchProfile("", none, nil, true)
	if e.BaseProfile() != "" || e.Profile() != nil || !e.Final().Seccomp {
		t.Errorf("BaseProfile %q, seccomp %v", e.BaseProfile(), e.Final().Seccomp)
	}
}

func TestEditorDraft(t *testing.T) {
	f := newFixtureFirejail(t)
	e := newEditor(t, f, "code.profile")
	e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
	e.SetExtraRules([]string{"net none"})

	d := e.Draft()
	if d.BaseProfile != f.path("code.profile") || !slices.Equal(d.ExtraRules, []string{"net none"}) || !d.Overrides.Changed(firejail.FieldNetwork) {
		t.Errorf("draft = %+v", d)
	}

	// The saved state of a new build is what it was when it opened, without the changes.
	saved := e.SavedBuild()
	if saved.SandboxName != "firejail-code" || saved.BaseProfile != f.path("code.profile") || saved.AppKind != store.AppKindDesktop ||
		saved.DesktopFileID != "code.desktop" || saved.Draft != nil || saved.BuiltAt != nil || saved.Installed || len(saved.ExtraRules) != 0 {
		t.Errorf("SavedBuild = %+v", saved)
	}
	if saved.Overrides.Changed(firejail.FieldNetwork) || !saved.Overrides.Changed(firejail.FieldSandboxName) {
		t.Errorf("saved overrides = %v", saved.Overrides.ChangedFields())
	}
}

func TestEditorSavedBuildOfABinary(t *testing.T) {
	app := apps.App{ID: "wget", Kind: apps.KindBinary, Name: "wget", Binary: "/usr/bin/wget", BinaryName: "wget", Terminal: true}
	e := NewEditor(app, EditorOpts{Base: firejail.BaseNone(), Defaults: newBuildDefaults(app, nil)})
	b := e.SavedBuild()
	if b.AppKind != store.AppKindBinary || b.BinaryPath != "/usr/bin/wget" || b.DesktopFileID != "" || b.SandboxName != "firejail-wget" {
		t.Errorf("SavedBuild = %+v", b)
	}
	if !e.Final().Terminal || e.Changed(firejail.FieldTerminal) {
		t.Errorf("terminal %v, changed %v", e.Final().Terminal, e.Changed(firejail.FieldTerminal))
	}
}

// storedBuild makes a stored build of codeApp from an editor's state, as the database
// would hand it back.
func storedBuild(e *Editor, withDraft bool) store.Build {
	b := e.SavedBuild()
	b.ID = 5
	if withDraft {
		d := e.Draft()
		b.Draft = &d
	}
	return b
}

func TestEditorOpensAStoredBuild(t *testing.T) {
	f := newFixtureFirejail(t)
	first := newEditor(t, f, "code.profile")
	first.Set(func(s *firejail.Settings) { s.GPU = false })
	// The stored build has the GPU change; no draft.
	stored := first.SavedBuild()
	stored.ID = 5
	stored.Overrides = first.Overrides()
	stored.ExtraRules = []string{"ignore apparmor"}
	base, p := f.base("code.profile")

	e := NewEditor(codeApp(), EditorOpts{Home: f.dirs.Home, BaseProfile: stored.BaseProfile, Profile: p, Base: base, Build: stored})
	if e.Build().ID != 5 || e.Final().GPU || !e.Changed(firejail.FieldGPU) || e.Final().SandboxName != "firejail-code" {
		t.Errorf("Final = %+v", e.Final())
	}
	if e.Dirty() || e.HasDraft() || e.FromDraft() {
		t.Errorf("Dirty %v, HasDraft %v, FromDraft %v", e.Dirty(), e.HasDraft(), e.FromDraft())
	}
	if !slices.Equal(e.ExtraRules(), []string{"ignore apparmor"}) || !e.ChangedTabs()[TabExtraRules] || !e.ChangedTabs()[firejail.TabDevices] {
		t.Errorf("extra rules %q, tabs %v", e.ExtraRules(), e.ChangedTabs())
	}

	// Change it and change it back: not dirty.
	e.Set(func(s *firejail.Settings) { s.GPU = true })
	if !e.Dirty() {
		t.Error("not dirty after a change")
	}
	e.Set(func(s *firejail.Settings) { s.GPU = false })
	if e.Dirty() {
		t.Error("dirty after the change was undone")
	}
}

func TestEditorOpensADraft(t *testing.T) {
	f := newFixtureFirejail(t)
	e1 := newEditor(t, f, "code.profile")
	e1.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
	e1.SetExtraRules([]string{"net none"})

	// The draft is on another base profile than the stored build.
	defBase, defProfile := f.base("default.profile")
	e1.SwitchProfile(f.path("default.profile"), defBase, defProfile, true)
	stored := storedBuild(e1, true)
	if BuildBaseProfile(stored) != f.path("default.profile") || stored.BaseProfile != f.path("code.profile") {
		t.Fatalf("BuildBaseProfile = %q", BuildBaseProfile(stored))
	}

	e2 := NewEditor(codeApp(), EditorOpts{
		Home: f.dirs.Home, BaseProfile: BuildBaseProfile(stored), Profile: defProfile, Base: defBase, Build: stored,
	})
	if !e2.FromDraft() || !e2.HasDraft() || !e2.Dirty() {
		t.Errorf("FromDraft %v, HasDraft %v, Dirty %v", e2.FromDraft(), e2.HasDraft(), e2.Dirty())
	}
	if e2.Final().Network != firejail.NetNone || !slices.Equal(e2.ExtraRules(), []string{"net none"}) || e2.BaseProfile() != f.path("default.profile") {
		t.Errorf("Final network %q, extra %q, base %q", e2.Final().Network, e2.ExtraRules(), e2.BaseProfile())
	}
}

func TestEditorTabs(t *testing.T) {
	got := EditorTabs()
	var titles []string
	for _, tab := range got {
		titles = append(titles, TabTitle(tab))
	}
	want := []string{"Files", "Network", "Devices", "Desktop", "Security", "Programs", "Limits", "Launcher", "Extra rules"}
	if !slices.Equal(titles, want) {
		t.Errorf("titles = %q, want %q", titles, want)
	}
}

// unknownJSON is what a newer Jailcat wrote: a field and a top-level key this version
// does not know.
const unknownJSON = `{"version":1,"future":{"a":[1,2]},"fields":{"warp_drive":{"level":9}},"folders":[]}`

// hasUnknown reports whether the overrides still write the unknown field and key.
func hasUnknown(t *testing.T, o firejail.Overrides) bool {
	t.Helper()
	data, err := o.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	return strings.Contains(s, `"warp_drive":{"level":9}`) && strings.Contains(s, `"future":{"a":[1,2]}`)
}

func TestEditorKeepsWhatItDoesNotKnow(t *testing.T) {
	f := newFixtureFirejail(t)
	base, p := f.base("code.profile")
	stored := newEditor(t, f, "code.profile").SavedBuild()
	stored.ID = 5
	if err := stored.Overrides.UnmarshalJSON([]byte(unknownJSON)); err != nil {
		t.Fatal(err)
	}
	stored.SandboxName = "firejail-code"
	open := func(b store.Build, profile string) *Editor {
		bs, pr := base, p
		if profile != f.path("code.profile") {
			bs, pr = f.base("default.profile")
		}
		return NewEditor(codeApp(), EditorOpts{Home: f.dirs.Home, BaseProfile: profile, Profile: pr, Base: bs, Build: b})
	}

	e := open(stored, stored.BaseProfile)
	if !hasUnknown(t, e.Overrides()) || e.Dirty() {
		t.Fatalf("opening lost it or made the editor dirty (dirty %v)", e.Dirty())
	}

	// An edit and a reset.
	e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
	if !hasUnknown(t, e.Overrides()) || !e.Overrides().Changed(firejail.FieldNetwork) {
		t.Error("an edit lost the unknown field")
	}
	e.Reset(firejail.FieldNetwork)
	if !hasUnknown(t, e.Overrides()) {
		t.Error("Reset lost the unknown field")
	}
	e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })

	// A draft round trip: the draft holds it, and the editor opened on it still does.
	d := e.Draft()
	if !hasUnknown(t, d.Overrides) {
		t.Fatal("the draft lost the unknown field")
	}
	withDraft := stored
	withDraft.Draft = &d
	e2 := open(withDraft, BuildBaseProfile(withDraft))
	if !hasUnknown(t, e2.Overrides()) || e2.Final().Network != firejail.NetNone {
		t.Error("opening a draft lost the unknown field")
	}
	e2.Set(func(s *firejail.Settings) { s.Sound = !s.Sound })
	if !hasUnknown(t, e2.Draft().Overrides) {
		t.Error("an edit of an opened draft lost the unknown field")
	}

	// Switching profile, keeping the changes, and starting over.
	defBase, defProfile := f.base("default.profile")
	for _, keep := range []bool{true, false} {
		e3 := open(stored, stored.BaseProfile)
		e3.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
		e3.SwitchProfile(f.path("default.profile"), defBase, defProfile, keep)
		if !hasUnknown(t, e3.Overrides()) {
			t.Errorf("SwitchProfile(keep=%v) lost the unknown field", keep)
		}
	}
}
