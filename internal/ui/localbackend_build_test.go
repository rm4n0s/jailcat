package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// scriptFJ is a firejail that does not sandbox: it answers --version, ends at once for
// the check ("... -- true"), stops a Try at --shutdown and otherwise prints a line and
// waits, so a test can watch a run and stop it.
const scriptFJ = `#!/bin/sh
case "$1" in
--version) echo 'firejail version 0.9.80'; exit 0;;
--shutdown=*) kill "$(cat "$FAKE_PIDFILE")"; exit 0;;
esac
for last; do :; done
case "$last" in */true) echo "checked: $*"; exit 0;; esac
echo $$ > "$FAKE_PIDFILE"
echo "started: $*"
exec sleep 60
`

// buildSystem is a system for the Build flows: Visual Studio Code's fixture app, the
// fixture profiles of firejail, a database and the script above as firejail.
type buildSystem struct {
	sys     system
	backend *LocalBackend
	store   *store.Store
	app     apps.App
	logs    *syncBuffer
}

func newBuildSystem(t *testing.T) buildSystem {
	t.Helper()
	sys := newSystem(t, false)
	sys.dirs.FirejailSystem = newFixtureFirejail(t).dirs.FirejailSystem
	writeFile(t, filepath.Join(sys.root, "bin", "firejail"), scriptFJ, 0o755)
	t.Setenv("FAKE_PIDFILE", filepath.Join(sys.root, "fake.pid"))
	logs := &syncBuffer{}
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "jailcat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	b := NewLocalBackend(sys.dirs, st, "", slog.New(slog.NewTextHandler(logs, nil)))
	res, err := b.ScanApps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var app apps.App
	for _, a := range res.Apps {
		if a.ID == "code.desktop" {
			app = a
		}
	}
	if app.ID == "" {
		t.Fatal("the fixture app is not found")
	}
	return buildSystem{sys: sys, backend: b, store: st, app: app, logs: logs}
}

// editor opens the editor of a new build of the app, or of the stored build.
func (bs buildSystem) editor(t *testing.T, build *store.Build) *Editor {
	t.Helper()
	e, err := OpenEditor(context.Background(), bs.backend, slog.New(slog.NewTextHandler(io.Discard, nil)), bs.app, build)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// build prepares and builds what the editor holds, with no conflicts.
func (bs buildSystem) build(t *testing.T, e *Editor) BuildResult {
	t.Helper()
	ctx := context.Background()
	p, err := bs.backend.PrepareBuild(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Conflicts) != 0 {
		t.Fatalf("conflicts: %+v", p.Conflicts)
	}
	res, err := bs.backend.Build(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sumOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestLocalBackendBuildNewBuild(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	e := bs.editor(t, nil)
	if e.Build().ID != 0 {
		t.Fatal("a new build has an ID")
	}
	res := bs.build(t, e)

	if res.Build.ID == 0 || !res.Build.Installed || res.Build.BuiltAt == nil || res.Build.FirejailVersion != "0.9.80" {
		t.Errorf("stored build = %+v", res.Build)
	}
	if res.Build.SandboxName != "firejail-code" {
		t.Errorf("sandbox name = %q", res.Build.SandboxName)
	}
	if !res.Check.OK || res.CheckErr != nil {
		t.Errorf("check = %+v, %v", res.Check, res.CheckErr)
	}
	recorded, err := bs.store.InstalledFiles(ctx, res.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, f := range recorded {
		kinds[f.Kind] = true
		if f.Kind == store.FileKindAppData {
			// A folder: the VS Code data folder, made with the build's defaults for VS Code.
			if info, err := os.Stat(f.Path); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || f.SHA256 != "" {
				t.Errorf("the data folder: %v, %v, %q", info, err, f.SHA256)
			}
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		if sumOf(data) != f.SHA256 {
			t.Errorf("%s: the recorded SHA-256 is not the file's", f.Path)
		}
		// The header carries the real ID, never 0.
		if f.Kind != store.FileKindIcon {
			want := firejail.BuildLine(res.Build.ID)
			if !bytes.Contains(data, []byte(want)) {
				t.Errorf("%s lacks %q:\n%s", f.Path, want, data)
			}
		}
		if bytes.Contains(data, []byte(firejail.BuildLine(0))) {
			t.Errorf("%s has the header of build 0", f.Path)
		}
	}
	for _, kind := range []string{store.FileKindProfile, store.FileKindLauncher, store.FileKindDesktop, store.FileKindIcon, store.FileKindAppData} {
		if !kinds[kind] {
			t.Errorf("no %s is recorded", kind)
		}
	}
	info, err := os.Stat(filepath.Join(bs.sys.dirs.Bin, "firejail-code"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("launcher: %v, %v", info, err)
	}
	// The build row was stored once, and the file of the profile names the right include.
	builds, err := bs.store.ListBuilds(ctx)
	if err != nil || len(builds) != 1 {
		t.Fatalf("builds = %v, %v", builds, err)
	}
	if builds[0].BaseProfile == "" || builds[0].Draft != nil {
		t.Errorf("stored base profile %q, draft %v", builds[0].BaseProfile, builds[0].Draft)
	}
}

func TestLocalBackendRebuildReplacesTheFiles(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	first := bs.build(t, bs.editor(t, nil))

	// A draft of the stored build goes when it is rebuilt.
	if err := bs.store.SaveDraft(ctx, first.Build.ID, store.Draft{Overrides: first.Build.Overrides, BaseProfile: first.Build.BaseProfile, ExtraRules: []string{"nosound"}}); err != nil {
		t.Fatal(err)
	}
	stored, err := bs.store.GetBuild(ctx, first.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := bs.editor(t, &stored)
	e.Set(func(s *firejail.Settings) { s.Comment = "changed" })
	second := bs.build(t, e)

	if second.Build.ID != first.Build.ID {
		t.Errorf("the rebuild stored a new build: %d, %d", second.Build.ID, first.Build.ID)
	}
	if second.Build.Draft != nil {
		t.Error("the draft was kept")
	}
	if len(second.Removed) != 0 {
		t.Errorf("Removed = %v", second.Removed)
	}
	desktop, err := os.ReadFile(filepath.Join(bs.sys.dirs.Applications, "firejail-code.desktop"))
	if err != nil || !bytes.Contains(desktop, []byte("Comment=changed")) {
		t.Errorf("the desktop entry was not replaced: %v\n%s", err, desktop)
	}
	recorded, err := bs.store.InstalledFiles(ctx, first.Build.ID)
	if err != nil || len(recorded) != len(first.Files) {
		t.Errorf("recorded = %d files (%v), want %d", len(recorded), err, len(first.Files))
	}
}

func TestLocalBackendRenameRemovesTheOldFiles(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	first := bs.build(t, bs.editor(t, nil))
	stored, err := bs.store.GetBuild(ctx, first.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := bs.editor(t, &stored)
	e.SetSandboxName("firejail-vscode")
	if e.RenamedFrom() != "firejail-code" {
		t.Fatalf("RenamedFrom = %q", e.RenamedFrom())
	}
	p, err := bs.backend.PrepareBuild(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stale) != len(first.Files) {
		t.Errorf("Stale = %d files, want %d", len(p.Stale), len(first.Files))
	}
	res, err := bs.backend.Build(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The VS Code data folder is not a file Build removes: like a private home it is kept,
	// and the build now has a data folder named like the new sandbox.
	if len(res.Removed) != len(first.Files)-1 {
		t.Errorf("Removed = %d files, want %d", len(res.Removed), len(first.Files)-1)
	}
	for _, f := range first.Files {
		if f.Kind == install.KindAppData {
			if _, err := os.Stat(f.Path); err != nil {
				t.Errorf("the old data folder was removed: %v", err)
			}
			continue
		}
		if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
			t.Errorf("the old file %s is still there: %v", f.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.Bin, "firejail-vscode")); err != nil {
		t.Errorf("the new launcher: %v", err)
	}
	recorded, err := bs.store.InstalledFiles(ctx, first.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range recorded {
		if !strings.Contains(f.Path, "firejail-vscode") {
			t.Errorf("an old file is still recorded: %s", f.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.AppData, "firejail-vscode")); err != nil {
		t.Errorf("the data folder of the new name: %v", err)
	}
	renamed, err := bs.store.GetBuild(ctx, first.Build.ID)
	if err != nil || renamed.SandboxName != "firejail-vscode" {
		t.Errorf("stored name = %q, %v", renamed.SandboxName, err)
	}
}

func TestLocalBackendConflicts(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		// setup puts the file in the way, after the first build when it returns true.
		setup func(t *testing.T, bs buildSystem, first BuildResult)
		built bool
		kind  install.ConflictKind
		// overwritable says whether the user may overwrite it.
		overwritable bool
		// owner is checked for ConflictOtherBuild.
		owner string
	}{
		{
			name: "a file that Jailcat did not make",
			setup: func(t *testing.T, bs buildSystem, _ BuildResult) {
				writeFile(t, filepath.Join(bs.sys.dirs.Bin, "firejail-code"), "#!/bin/sh\necho mine\n", 0o755)
			},
			kind: install.ConflictForeign,
		},
		{
			name: "a file with the header of Jailcat that no build owns",
			setup: func(t *testing.T, bs buildSystem, _ BuildResult) {
				writeFile(t, filepath.Join(bs.sys.dirs.Bin, "firejail-code"), "#!/bin/sh\n"+firejail.HeaderLine+"\n# jailcat-build: 99\n", 0o755)
			},
			kind: install.ConflictForeignJailcat, overwritable: true,
		},
		{
			name: "a file of this build that was edited",
			setup: func(t *testing.T, bs buildSystem, first BuildResult) {
				path := filepath.Join(bs.sys.dirs.Bin, "firejail-code")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, path, string(data)+"# edited\n", 0o755)
			},
			built: true, kind: install.ConflictEdited, overwritable: true,
		},
		{
			name: "a file that belongs to another build",
			setup: func(t *testing.T, bs buildSystem, first BuildResult) {
				other, err := bs.store.CreateBuild(ctx, store.Build{SandboxName: "firejail-other", AppKind: store.AppKindBinary, BinaryPath: "/usr/bin/other", IconMode: store.IconModeBadge})
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(bs.sys.dirs.Bin, "firejail-code")
				writeFile(t, path, "#!/bin/sh\n", 0o755)
				err = bs.store.RecordInstall(ctx, other.ID, []store.InstalledFile{{Kind: store.FileKindLauncher, Path: path, SHA256: sumOf([]byte("#!/bin/sh\n"))}}, "0.9.80", time.Now())
				if err != nil {
					t.Fatal(err)
				}
			},
			kind: install.ConflictOtherBuild, owner: "firejail-other",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := newBuildSystem(t)
			var first BuildResult
			e := bs.editor(t, nil)
			if tt.built {
				first = bs.build(t, e)
				stored, err := bs.store.GetBuild(ctx, first.Build.ID)
				if err != nil {
					t.Fatal(err)
				}
				e = bs.editor(t, &stored)
			}
			tt.setup(t, bs, first)

			p, err := bs.backend.PrepareBuild(ctx, e.Request())
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Conflicts) != 1 {
				t.Fatalf("conflicts = %+v", p.Conflicts)
			}
			c := p.Conflicts[0]
			if c.Kind != tt.kind || c.Overwritable != tt.overwritable {
				t.Errorf("conflict = %v overwritable %v, want %v %v", c.Kind, c.Overwritable, tt.kind, tt.overwritable)
			}
			if tt.owner != "" && p.Owners[c.Owner] != tt.owner {
				t.Errorf("owners = %v, want %s", p.Owners, tt.owner)
			}

			// Nothing is written until the user agrees.
			before := countBuilds(t, bs)
			_, err = bs.backend.Build(ctx, p, nil)
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T: %v", err, err)
			}
			if !appErr.HasRoute("LocalBackend.Build->Install.ConflictsRemain") {
				t.Errorf("unexpected failure path: %s", appErr.Route())
			}
			if after := countBuilds(t, bs); after != before {
				t.Errorf("a failed Build left %d builds, want %d", after, before)
			}
			if !tt.overwritable {
				return
			}
			res, err := bs.backend.Build(ctx, p, map[string]bool{c.File.Path: true})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(c.File.Path)
			if err != nil || !bytes.Contains(data, []byte(firejail.BuildLine(res.Build.ID))) {
				t.Errorf("the file was not overwritten: %v\n%s", err, data)
			}
		})
	}
}

// countBuilds is how many builds the database holds.
func countBuilds(t *testing.T, bs buildSystem) int {
	t.Helper()
	builds, err := bs.store.ListBuilds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(builds)
}

func TestLocalBackendBuildFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("firejail is not installed", func(t *testing.T) {
		bs := newBuildSystem(t)
		if err := os.Remove(filepath.Join(bs.sys.root, "bin", "firejail")); err != nil {
			t.Fatal(err)
		}
		e := bs.editor(t, nil)
		for name, call := range map[string]func() error{
			"PrepareBuild": func() error { _, err := bs.backend.PrepareBuild(ctx, e.Request()); return err },
			"Try":          func() error { _, err := bs.backend.Try(ctx, e.Request()); return err },
		} {
			err := call()
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("%s: expected *errors.Error, got %T: %v", name, err, err)
			}
			if !appErr.HasRoute("LocalBackend." + name + "->FindFirejail.FirejailNotFound") {
				t.Errorf("%s: unexpected failure path: %s", name, appErr.Route())
			}
		}
	})
	t.Run("a sandbox name that is not valid", func(t *testing.T) {
		bs := newBuildSystem(t)
		e := bs.editor(t, nil)
		e.Set(func(s *firejail.Settings) { s.SandboxName = "Not Valid" })
		_, err := bs.backend.PrepareBuild(ctx, e.Request())
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("expected *errors.Error, got %T: %v", err, err)
		}
		if !appErr.HasRoute("LocalBackend.PrepareBuild->LocalBackend.generate->Prepare.SandboxNameInvalid") {
			t.Errorf("unexpected failure path: %s", appErr.Route())
		}
	})
	t.Run("a sandbox name another build has", func(t *testing.T) {
		bs := newBuildSystem(t)
		bs.build(t, bs.editor(t, nil))
		e := bs.editor(t, nil)
		// The editor was opened before the name was taken: its name is the same.
		e.Set(func(s *firejail.Settings) { s.SandboxName = "firejail-code" })
		p, err := bs.backend.PrepareBuild(ctx, e.Request())
		if err != nil {
			t.Fatal(err)
		}
		// The files of the first build are in the way as well, so there are conflicts; the
		// name is checked first.
		_, err = bs.backend.Build(ctx, p, nil)
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("expected *errors.Error, got %T: %v", err, err)
		}
		if !appErr.HasRoute("LocalBackend.Build->Store.CreateBuild.SandboxNameTaken") {
			t.Errorf("unexpected failure path: %s", appErr.Route())
		}
	})
}

func TestLocalBackendUninstall(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		// own makes the build's home its own private home.
		own bool
		// edit changes the launcher after the build.
		edit   bool
		choice UninstallChoice
		// wantDone says whether the build is gone from the database or marked not installed.
		wantDone bool
		wantKept int
		// wantBuild says whether the build row is still there; wantInstalled its state.
		wantBuild, wantInstalled bool
		wantHome                 bool
	}{
		{name: "keep the settings", choice: UninstallChoice{KeepSettings: true}, wantDone: true, wantBuild: true},
		{name: "delete the build", choice: UninstallChoice{}, wantDone: true},
		{name: "the home is kept", own: true, choice: UninstallChoice{KeepSettings: true}, wantDone: true, wantBuild: true, wantHome: true},
		{name: "the home is deleted", own: true, choice: UninstallChoice{DeleteHome: true}, wantDone: true},
		{name: "an edited file stops it", edit: true, choice: UninstallChoice{KeepSettings: true}, wantKept: 1, wantBuild: true, wantInstalled: true},
		{name: "an edited file is forced", edit: true, wantDone: true, wantBuild: true, choice: UninstallChoice{KeepSettings: true, Force: map[string]bool{"launcher": true}}},
		{name: "an edited file is left", edit: true, wantDone: true, wantKept: 1, wantBuild: true, choice: UninstallChoice{KeepSettings: true, LeaveKept: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := newBuildSystem(t)
			e := bs.editor(t, nil)
			if tt.own {
				e.SetHome(firejail.HomePrivate)
			}
			built := bs.build(t, e)
			launcher := filepath.Join(bs.sys.dirs.Bin, "firejail-code")
			if tt.edit {
				data, err := os.ReadFile(launcher)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, launcher, string(data)+"# edited\n", 0o755)
			}
			choice := tt.choice
			if choice.Force != nil {
				choice.Force = map[string]bool{launcher: true}
			}
			res, err := bs.backend.Uninstall(ctx, built.Build, choice)
			if err != nil {
				t.Fatal(err)
			}
			if got := UninstallDone(res, choice); got != tt.wantDone {
				t.Errorf("UninstallDone = %v, want %v", got, tt.wantDone)
			}
			if len(res.Kept) != tt.wantKept {
				t.Errorf("Kept = %v, want %d", res.Kept, tt.wantKept)
			}
			stored, err := bs.store.GetBuild(ctx, built.Build.ID)
			if tt.wantBuild != (err == nil) {
				t.Fatalf("build row: %v, want present %v", err, tt.wantBuild)
			}
			if err == nil && stored.Installed != tt.wantInstalled {
				t.Errorf("Installed = %v, want %v", stored.Installed, tt.wantInstalled)
			}
			if tt.own {
				homes, err := os.ReadDir(bs.sys.dirs.Homes)
				if got := err == nil && len(homes) > 0; got != tt.wantHome {
					t.Errorf("private home present = %v, want %v (%v)", got, tt.wantHome, err)
				}
			}
			_, launcherErr := os.Stat(launcher)
			removed := os.IsNotExist(launcherErr)
			wantRemoved := !tt.edit || len(choice.Force) > 0
			if removed != wantRemoved {
				t.Errorf("launcher removed = %v, want %v", removed, wantRemoved)
			}
			if tt.wantDone && !tt.wantInstalled {
				recorded, err := bs.store.InstalledFiles(ctx, built.Build.ID)
				if err != nil || len(recorded) != 0 {
					t.Errorf("recorded files = %v, %v", recorded, err)
				}
			}
		})
	}
}

func TestLocalBackendBaseProfileMissing(t *testing.T) {
	bs := newBuildSystem(t)
	present := filepath.Join(bs.sys.dirs.FirejailSystem, "code.profile")
	tests := []struct {
		name, path string
		want       bool
	}{
		{"base None", "", false},
		{"a profile that is there", present, false},
		{"a profile that is gone", filepath.Join(bs.sys.dirs.FirejailSystem, "gone.profile"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bs.backend.BaseProfileMissing(context.Background(), tt.path); got != tt.want {
				t.Errorf("BaseProfileMissing(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if bs.backend.BaseProfileMissing(cancelled, filepath.Join(bs.sys.dirs.FirejailSystem, "gone.profile")) {
		t.Error("a cancelled call says the profile is missing")
	}
}

func TestLocalBackendTry(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	e := bs.editor(t, nil)
	session, err := bs.backend.Try(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = session.Stop(context.Background())
		}
	})
	var first string
	select {
	case first = <-session.Lines():
	case <-time.After(10 * time.Second):
		t.Fatal("no output")
	}
	profile := filepath.Join(bs.sys.dirs.Cache, "try", "firejail-code.profile")
	if !strings.HasPrefix(first, "started: --profile="+profile+" --name=jailcat-try-firejail-code -- ") {
		t.Errorf("first line = %q", first)
	}
	data, err := os.ReadFile(profile)
	if err != nil || !bytes.Contains(data, []byte("# jailcat-build: 0")) {
		t.Errorf("the profile: %v\n%s", err, data)
	}
	// Nothing was installed or stored.
	if n := countBuilds(t, bs); n != 0 {
		t.Errorf("a Try stored %d builds", n)
	}
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.Bin, "firejail-code")); !os.IsNotExist(err) {
		t.Errorf("a Try installed the launcher: %v", err)
	}
	if err := session.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stopped = true
	status, err := session.Wait()
	if err != nil || status.Success() {
		t.Errorf("Wait = %v, %v", status, err)
	}
	if !slices.Contains([]string{"terminated", "killed"}, status.Signal) {
		t.Errorf("signal = %q", status.Signal)
	}
}

func TestLocalBackendTryWithPrivateHome(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	e := bs.editor(t, nil)
	e.SetHome(firejail.HomePrivate)
	session, err := bs.backend.Try(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Stop(context.Background()) })
	<-session.Lines()
	home := filepath.Join(bs.sys.dirs.Homes, "firejail-code")
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Errorf("the private home was not made: %v", err)
	}
}
