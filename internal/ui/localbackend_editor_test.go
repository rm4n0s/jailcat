package ui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// profileSystem is a LocalBackend whose firejail folders (system, user, imported) are in
// a temporary folder.
type profileSystem struct {
	b    *LocalBackend
	st   *store.Store
	sys  string
	user string
	imp  string
}

// newProfileSystem makes the backend. The system folder starts as a copy of nothing;
// tests put profiles in it with put.
func newProfileSystem(t *testing.T) profileSystem {
	t.Helper()
	s := newSystem(t, true)
	root := filepath.Join(s.root, "firejail")
	s.dirs.FirejailSystem = filepath.Join(root, "etc")
	s.dirs.FirejailUser = filepath.Join(root, "user")
	s.dirs.Imported = filepath.Join(root, "imported")
	b, st := s.newBackendWith(t, s.dirs)
	return profileSystem{b: b, st: st, sys: s.dirs.FirejailSystem, user: s.dirs.FirejailUser, imp: s.dirs.Imported}
}

// put writes files into a folder.
func put(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		writeFile(t, filepath.Join(dir, n), "# "+n+"\n", 0o644)
	}
}

// entryNames lists the entries as "<group>/<name>".
func entryNames(c ProfileChoices) []string {
	var out []string
	for _, e := range c.Entries {
		out = append(out, string(e.Group)+"/"+e.Label)
	}
	return out
}

func TestLocalBackendProfileChoices(t *testing.T) {
	code := apps.App{ID: "code.desktop", BinaryName: "code"}
	tests := []struct {
		name string
		sys  []string
		user []string
		imp  []string
		app  apps.App
		want []string
		// def is the path relative to the folders ("sys/..." or "user/...") of the default
		// choice, "" for None.
		def string
	}{
		{
			name: "the app's own profile comes first and is the default",
			sys:  []string{"code.profile", "default.profile", "Zed.profile", "blink.profile", "notes.txt", "base.inc"},
			app:  code,
			want: []string{"For this app/code (firejail)", "None/None", "Default/Default",
				"Firejail profiles/blink", "Firejail profiles/code", "Firejail profiles/default", "Firejail profiles/Zed"},
			def: "sys/code.profile",
		},
		{
			name: "the user's profile of the app wins over the system's",
			sys:  []string{"code.profile", "default.profile"},
			user: []string{"code.profile", "mine.profile"},
			app:  code,
			want: []string{"For this app/code (yours)", "None/None", "Default/Default",
				"Your profiles/code", "Your profiles/mine", "Firejail profiles/code", "Firejail profiles/default"},
			def: "user/code.profile",
		},
		{
			name: "no profile of the app: Default is the default, the user's first",
			sys:  []string{"default.profile", "other.profile"},
			user: []string{"default.profile"},
			imp:  []string{"imported-one.profile"},
			app:  code,
			want: []string{"None/None", "Default/Default", "Your profiles/default", "Imported/imported-one",
				"Firejail profiles/default", "Firejail profiles/other"},
			def: "user/default.profile",
		},
		{
			name: "no default.profile: None",
			sys:  []string{"other.profile"},
			app:  apps.App{ID: "x", BinaryName: "x"},
			want: []string{"None/None", "Firejail profiles/other"},
			def:  "",
		},
		{
			name: "nothing at all",
			app:  code,
			want: []string{"None/None"},
			def:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newProfileSystem(t)
			put(t, s.sys, tt.sys...)
			put(t, s.user, tt.user...)
			put(t, s.imp, tt.imp...)
			got, err := s.b.ProfileChoices(context.Background(), tt.app)
			if err != nil {
				t.Fatal(err)
			}
			if names := entryNames(got); !slices.Equal(names, tt.want) {
				t.Errorf("entries = %q, want %q", names, tt.want)
			}
			wantDef := ""
			switch {
			case strings.HasPrefix(tt.def, "sys/"):
				wantDef = filepath.Join(s.sys, strings.TrimPrefix(tt.def, "sys/"))
			case strings.HasPrefix(tt.def, "user/"):
				wantDef = filepath.Join(s.user, strings.TrimPrefix(tt.def, "user/"))
			}
			if got.Default.Path != wantDef {
				t.Errorf("default = %+v, want path %q", got.Default, wantDef)
			}
			// None has no path, every other entry an absolute one.
			for _, e := range got.Entries {
				if (e.Group == GroupNone) != (e.Path == "") || (e.Path != "" && !filepath.IsAbs(e.Path)) {
					t.Errorf("entry %+v has a wrong path", e)
				}
			}
		})
	}
}

func TestLocalBackendProfileChoicesFindAndFilter(t *testing.T) {
	s := newProfileSystem(t)
	put(t, s.sys, "code.profile", "default.profile", "firefox.profile")
	c, err := s.b.ProfileChoices(context.Background(), apps.App{BinaryName: "code"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.sys, "code.profile")
	// The first entry with the path: the app's own.
	if e, ok := c.Find(path); !ok || e.Group != GroupForApp || c.LabelFor(path) != "code (firejail)" {
		t.Errorf("Find = %+v, %v; label %q", e, ok, c.LabelFor(path))
	}
	if e, ok := c.Find(""); !ok || e.Group != GroupNone || c.LabelFor("") != "None" {
		t.Errorf("Find(\"\") = %+v", e)
	}
	// A profile that is not listed is shown by its file name.
	if got := c.LabelFor("/somewhere/else/mine.profile"); got != "mine" {
		t.Errorf("LabelFor = %q", got)
	}
	var names []string
	for _, e := range c.Filter("  FIRE ") {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"firefox"}) {
		t.Errorf("Filter = %q", names)
	}
	if got := c.Filter(""); len(got) != len(c.Entries) {
		t.Errorf("an empty filter keeps %d of %d", len(got), len(c.Entries))
	}
}

func TestLocalBackendProfileChoicesManyProfiles(t *testing.T) {
	s := newProfileSystem(t)
	var names []string
	for i := range 1500 {
		names = append(names, "p"+strconv.Itoa(i)+".profile")
	}
	put(t, s.sys, names...)
	c, err := s.b.ProfileChoices(context.Background(), apps.App{BinaryName: "nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 1501 {
		t.Fatalf("%d entries, want 1501 (None and the profiles)", len(c.Entries))
	}
	// Sorted by name: p0, p1, p10, p100, ...
	if c.Entries[1].Name != "p0" || c.Entries[2].Name != "p1" || c.Entries[3].Name != "p10" {
		t.Errorf("order = %q %q %q", c.Entries[1].Name, c.Entries[2].Name, c.Entries[3].Name)
	}
}

func TestLocalBackendProfileChoicesErrors(t *testing.T) {
	t.Run("a folder that is a file", func(t *testing.T) {
		s := newProfileSystem(t)
		writeFile(t, s.user, "not a folder", 0o644)
		_, err := s.b.ProfileChoices(context.Background(), apps.App{BinaryName: "code"})
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("expected *errors.Error, got %T: %v", err, err)
		}
		if !appErr.HasRoute("LocalBackend.ProfileChoices->profilesIn." + tagProfileListFailed) {
			t.Errorf("unexpected failure path: %s", appErr.Route())
		}
		if appErr.Metadata["dir"] != s.user {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		s := newProfileSystem(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := s.b.ProfileChoices(ctx, apps.App{BinaryName: "code"})
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("LocalBackend.ProfileChoices->profilesIn."+tagProfileListCancelled) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestLocalBackendReadBase(t *testing.T) {
	f := newFixtureFirejail(t)
	s := newProfileSystem(t)
	s.b.dirs = f.dirs
	ctx := context.Background()

	none, profile, err := s.b.ReadBase(ctx, "")
	if err != nil || profile != nil || !none.Settings.Seccomp || !none.Settings.Sound {
		t.Errorf("None: %+v, %v, %v", none.Settings.Seccomp, profile, err)
	}

	code, profile, err := s.b.ReadBase(ctx, f.path("code.profile"))
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || len(profile.Lines) == 0 || code.Settings.Sound || len(code.Settings.Folders) == 0 {
		t.Errorf("code.profile: lines %d, sound %v, folders %d", len(profile.Lines), code.Settings.Sound, len(code.Settings.Folders))
	}
	// The same as reading it by hand.
	byHand, _ := f.base("code.profile")
	if code.Settings.Sound != byHand.Settings.Sound || len(code.Settings.Folders) != len(byHand.Settings.Folders) {
		t.Error("ReadBase differs from Read and BaseValues")
	}

	_, _, err = s.b.ReadBase(ctx, filepath.Join(f.dirs.FirejailSystem, "missing.profile"))
	appErr, ok := errors.FromError(err)
	if !ok || !appErr.HasRoute("LocalBackend.ReadBase->Read") {
		t.Errorf("a missing profile: %v", err)
	}
}

func TestLocalBackendReadBaseConfigError(t *testing.T) {
	f := newFixtureFirejail(t)
	s := newProfileSystem(t)
	// A firejail.config with a value that is neither yes nor no stops firejail, so it stops us.
	writeFile(t, filepath.Join(s.sys, "firejail.config"), "browser-disable-u2f maybe\n", 0o644)
	writeFile(t, filepath.Join(s.sys, "x.profile"), "nosound\n", 0o644)
	_ = f
	_, _, err := s.b.ReadBase(context.Background(), filepath.Join(s.sys, "x.profile"))
	appErr, ok := errors.FromError(err)
	if !ok || !appErr.HasRoute("LocalBackend.ReadBase->ReadConfig") {
		t.Errorf("err = %v", err)
	}
}

func TestLocalBackendImportProfile(t *testing.T) {
	s := newProfileSystem(t)
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "my tool.profile")
	writeFile(t, src, "# mine\nnosound\n", 0o600)

	var got []string
	for range 3 {
		path, err := s.b.ImportProfile(ctx, src)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.Base(path))
		if filepath.Dir(path) != s.imp {
			t.Errorf("imported into %q, want %q", filepath.Dir(path), s.imp)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "# mine\nnosound\n" {
			t.Errorf("the copy = %q, %v", data, err)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v, %v", info.Mode(), err)
		}
	}
	if want := []string{"my tool.profile", "my tool-2.profile", "my tool-3.profile"}; !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
	// They are listed under Imported.
	c, err := s.b.ProfileChoices(ctx, apps.App{BinaryName: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if names := entryNames(c); !slices.Equal(names, []string{"None/None", "Imported/my tool", "Imported/my tool-2", "Imported/my tool-3"}) {
		t.Errorf("entries = %q", names)
	}
}

func TestLocalBackendImportProfileErrors(t *testing.T) {
	big := strings.Repeat("# x\n", maxImportSize/4+10)
	tests := []struct {
		name  string
		setup func(t *testing.T, s profileSystem) (ctx context.Context, src string)
		route string
	}{
		{"not a profile", func(t *testing.T, s profileSystem) (context.Context, string) {
			src := filepath.Join(t.TempDir(), "notes.txt")
			writeFile(t, src, "x", 0o644)
			return context.Background(), src
		}, "LocalBackend.ImportProfile." + tagImportNotProfile},
		{"a folder named .profile", func(t *testing.T, s profileSystem) (context.Context, string) {
			src := filepath.Join(t.TempDir(), "dir.profile")
			if err := os.Mkdir(src, 0o755); err != nil {
				t.Fatal(err)
			}
			return context.Background(), src
		}, "LocalBackend.ImportProfile." + tagImportNotProfile},
		{"too large", func(t *testing.T, s profileSystem) (context.Context, string) {
			src := filepath.Join(t.TempDir(), "big.profile")
			writeFile(t, src, big, 0o644)
			return context.Background(), src
		}, "LocalBackend.ImportProfile." + tagImportTooLarge},
		{"missing", func(t *testing.T, s profileSystem) (context.Context, string) {
			return context.Background(), filepath.Join(t.TempDir(), "gone.profile")
		}, "LocalBackend.ImportProfile." + tagImportReadFailed},
		{"cancelled", func(t *testing.T, s profileSystem) (context.Context, string) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, "/x.profile"
		}, "LocalBackend.ImportProfile." + tagImportCancelled},
		{"the imported folder cannot be made", func(t *testing.T, s profileSystem) (context.Context, string) {
			src := filepath.Join(t.TempDir(), "a.profile")
			writeFile(t, src, "x", 0o644)
			writeFile(t, filepath.Dir(s.imp)+"/imported-blocker", "", 0o644)
			// The parent of the imported folder is a file.
			if err := os.RemoveAll(filepath.Dir(s.imp)); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Dir(s.imp), "a file", 0o644)
			return context.Background(), src
		}, "LocalBackend.ImportProfile." + tagImportWriteFailed},
		{"every name is taken", func(t *testing.T, s profileSystem) (context.Context, string) {
			src := filepath.Join(t.TempDir(), "a.profile")
			writeFile(t, src, "x", 0o644)
			names := []string{"a.profile"}
			for n := 2; n <= maxImportCopies; n++ {
				names = append(names, "a-"+strconv.Itoa(n)+".profile")
			}
			put(t, s.imp, names...)
			return context.Background(), src
		}, "LocalBackend.ImportProfile." + tagImportNameExhausted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newProfileSystem(t)
			ctx, src := tt.setup(t, s)
			path, err := s.b.ImportProfile(ctx, src)
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T: %v (path %q)", err, err, path)
			}
			if !appErr.HasRoute(tt.route) {
				t.Errorf("unexpected failure path: %s, want %s", appErr.Route(), tt.route)
			}
		})
	}
}

func TestSandboxNameFor(t *testing.T) {
	tests := []struct{ binary, want string }{
		{"code", "firejail-code"},
		{"Code", "firejail-code"},
		{"code-oss", "firejail-code-oss"},
		{"python3.11", "firejail-python3-11"},
		{"my tool!", "firejail-my-tool"},
		{"a__b", "firejail-a-b"},
		{"-lead-and-trail-", "firejail-lead-and-trail"},
		{"é", "firejail-app"},
		{"", "firejail-app"},
		{"---", "firejail-app"},
	}
	for _, tt := range tests {
		t.Run(tt.binary, func(t *testing.T) {
			got := SandboxNameFor(tt.binary)
			if got != tt.want {
				t.Errorf("SandboxNameFor(%q) = %q, want %q", tt.binary, got, tt.want)
			}
			if problems := firejail.Validate(firejail.Settings{SandboxName: got}, ""); slices.ContainsFunc(problems, func(p firejail.Problem) bool { return p.Tag == firejail.ProblemSandboxName }) {
				t.Errorf("%q is not a valid sandbox name", got)
			}
		})
	}
}

func TestUniqueSandboxName(t *testing.T) {
	tests := []struct {
		taken []string
		want  string
	}{
		{nil, "firejail-code"},
		{[]string{"firejail-code"}, "firejail-code-2"},
		{[]string{"firejail-code", "firejail-code-2", "firejail-code-4"}, "firejail-code-3"},
		{[]string{"firejail-code-2"}, "firejail-code"},
	}
	for _, tt := range tests {
		taken := map[string]bool{}
		for _, n := range tt.taken {
			taken[n] = true
		}
		if got := UniqueSandboxName("firejail-code", taken); got != tt.want {
			t.Errorf("taken %q: %q, want %q", tt.taken, got, tt.want)
		}
	}
}

func TestLocalBackendNewBuildDefaults(t *testing.T) {
	s := newProfileSystem(t)
	ctx := context.Background()
	code := apps.App{ID: "code.desktop", Kind: apps.KindDesktop, Name: "Visual Studio Code", Comment: "Code Editing", Binary: "/usr/bin/code", BinaryName: "code"}

	first, err := s.b.NewBuildDefaults(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if first.SandboxName != "firejail-code" || first.MenuName != "Visual Studio Code (sandboxed)" || first.Comment != "Code Editing" || first.Terminal || first.IconSeed == 0 {
		t.Errorf("defaults = %+v", first)
	}
	// Other fields are left for the base.
	if first.Network != "" || first.Seccomp {
		t.Errorf("settings besides the launcher fields are set: %+v", first)
	}

	for _, name := range []string{"firejail-code", "firejail-code-2"} {
		if _, err := s.st.CreateBuild(ctx, store.Build{SandboxName: name, AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge}); err != nil {
			t.Fatal(err)
		}
	}
	second, err := s.b.NewBuildDefaults(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if second.SandboxName != "firejail-code-3" {
		t.Errorf("SandboxName = %q, want firejail-code-3", second.SandboxName)
	}
	if second.IconSeed == first.IconSeed {
		t.Error("the icon seed is not random")
	}

	// A binary runs in a terminal; a menu app as its entry says.
	tool := apps.App{ID: "mytool", Kind: apps.KindBinary, Name: "mytool", Binary: "/bin/mytool", BinaryName: "mytool", Terminal: true}
	d, err := s.b.NewBuildDefaults(ctx, tool)
	if err != nil || !d.Terminal || d.SandboxName != "firejail-mytool" || d.MenuName != "mytool (sandboxed)" {
		t.Errorf("binary defaults = %+v, %v", d, err)
	}
	termApp := code
	termApp.Terminal = true
	if d, _ := s.b.NewBuildDefaults(ctx, termApp); !d.Terminal {
		t.Error("a menu app that asks for a terminal does not get one")
	}
}

func TestLocalBackendBuildsForApp(t *testing.T) {
	s := newProfileSystem(t)
	ctx := context.Background()
	code := apps.App{ID: "code.desktop", Kind: apps.KindDesktop, Binary: "/usr/bin/code"}
	tool := apps.App{ID: "mytool", Kind: apps.KindBinary, Binary: "/bin/mytool"}
	for _, b := range []store.Build{
		{SandboxName: "firejail-code", AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge},
		{SandboxName: "firejail-code-2", AppKind: store.AppKindDesktop, DesktopFileID: "code.desktop", IconMode: store.IconModeBadge},
		{SandboxName: "firejail-mytool", AppKind: store.AppKindBinary, BinaryPath: "/bin/mytool", IconMode: store.IconModeBadge},
	} {
		if _, err := s.b.CreateBuild(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.b.BuildsForApp(ctx, code)
	if err != nil || len(got) != 2 || got[0].SandboxName != "firejail-code" {
		t.Errorf("code builds = %+v, %v", got, err)
	}
	got, err = s.b.BuildsForApp(ctx, tool)
	if err != nil || len(got) != 1 || got[0].SandboxName != "firejail-mytool" {
		t.Errorf("tool builds = %+v, %v", got, err)
	}
	got, err = s.b.BuildsForApp(ctx, apps.App{ID: "other.desktop"})
	if err != nil || len(got) != 0 {
		t.Errorf("other builds = %+v, %v", got, err)
	}
}

func TestDraftPlanRun(t *testing.T) {
	s := newProfileSystem(t)
	ctx := context.Background()
	f := newFixtureFirejail(t)
	e := newEditor(t, f, "code.profile")
	app := e.App()

	// Nothing to keep: nothing is written.
	if plan := e.DraftPlan(); plan.Action != DraftNothing {
		t.Fatalf("plan of a clean editor = %+v", plan)
	}
	if err := (DraftPlan{}).Run(ctx, s.b); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.b.ListBuilds(ctx); len(all) != 0 {
		t.Fatalf("%d builds after an empty plan", len(all))
	}

	// A new build with changes: created as it opened, the changes in its draft.
	e.Set(func(s *firejail.Settings) { s.Network = firejail.NetNone })
	plan := e.DraftPlan()
	if plan.Action != DraftSave || plan.Create == nil || plan.BuildID != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if err := plan.Run(ctx, s.b); err != nil {
		t.Fatal(err)
	}
	builds, err := s.b.BuildsForApp(ctx, app)
	if err != nil || len(builds) != 1 {
		t.Fatalf("builds = %+v, %v", builds, err)
	}
	b := builds[0]
	if b.BuiltAt != nil || b.Installed || b.Draft == nil || !b.Draft.Overrides.Changed(firejail.FieldNetwork) || b.Overrides.Changed(firejail.FieldNetwork) {
		t.Errorf("stored build = %+v", b)
	}

	// Opening it gives the draft back; undoing the change clears the draft.
	base, p := f.base("code.profile")
	opened := NewEditor(app, EditorOpts{Home: f.dirs.Home, BaseProfile: BuildBaseProfile(b), Profile: p, Base: base, Build: b})
	if !opened.FromDraft() || opened.Final().Network != firejail.NetNone {
		t.Fatalf("FromDraft %v, network %q", opened.FromDraft(), opened.Final().Network)
	}
	opened.Reset(firejail.FieldNetwork)
	plan = opened.DraftPlan()
	if plan.Action != DraftClear || plan.BuildID != b.ID {
		t.Fatalf("plan = %+v", plan)
	}
	if err := plan.Run(ctx, s.b); err != nil {
		t.Fatal(err)
	}
	builds, _ = s.b.BuildsForApp(ctx, app)
	if len(builds) != 1 || builds[0].Draft != nil {
		t.Errorf("the draft is still stored: %+v", builds)
	}

	// A change of a stored build writes the draft only.
	opened2 := NewEditor(app, EditorOpts{Home: f.dirs.Home, BaseProfile: builds[0].BaseProfile, Profile: p, Base: base, Build: builds[0]})
	opened2.SetExtraRules([]string{"nosound"})
	plan = opened2.DraftPlan()
	if plan.Action != DraftSave || plan.Create != nil || plan.BuildID != b.ID {
		t.Fatalf("plan = %+v", plan)
	}
	if err := plan.Run(ctx, s.b); err != nil {
		t.Fatal(err)
	}
	if builds, _ = s.b.BuildsForApp(ctx, app); len(builds) != 1 || builds[0].Draft == nil || !slices.Equal(builds[0].Draft.ExtraRules, []string{"nosound"}) || len(builds[0].ExtraRules) != 0 {
		t.Errorf("builds = %+v", builds)
	}
}

func TestBuildAndDraftCounts(t *testing.T) {
	built := fixtureBackend(t).builds[0].BuiltAt
	builds := []store.Build{
		{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop", BuiltAt: built, Installed: true},
		{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop", BuiltAt: built, Installed: true},
		{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop"},
		// Built and uninstalled with its settings kept: not installed any more.
		{AppKind: store.AppKindDesktop, DesktopFileID: "a.desktop", BuiltAt: built},
		{AppKind: store.AppKindBinary, BinaryPath: "/bin/b"},
	}
	if got := BuildCounts(builds); len(got) != 1 || got["desktop:a.desktop"] != 2 {
		t.Errorf("BuildCounts = %v", got)
	}
	if got := DraftCounts(builds); len(got) != 2 || got["desktop:a.desktop"] != 2 || got["binary:/bin/b"] != 1 {
		t.Errorf("DraftCounts = %v", got)
	}
}
