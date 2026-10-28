package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
)

// vscodeOn turns the three VS Code switches on, with the home choice.
func vscodeOn(home firejail.HomeMode, edit ...func(s *firejail.Settings)) func(s *firejail.Settings) {
	return func(s *firejail.Settings) {
		s.Home = home
		if home == firejail.HomeChosen {
			s.Folders = []firejail.Folder{{Path: "${HOME}/Projects", Access: firejail.AccessReadWrite}}
		}
		s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles = true, true, true
		for _, e := range edit {
			e(s)
		}
	}
}

// configFiles writes the three shared settings of the user's VS Code in the scratch
// home and returns their paths.
func configFiles(t *testing.T, k *kit) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range firejail.VSCodeSharedNames() {
		path := filepath.Join(k.dirs.ConfigHome, "Code", "User", name)
		if name == "snippets" {
			writeText(t, filepath.Join(path, "go.json"), "{}")
		} else {
			writeText(t, path, "mine "+name)
		}
		out[name] = path
	}
	return out
}

func TestPrepareAppData(t *testing.T) {
	tests := []struct {
		name string
		app  func(t *testing.T) apps.App
		edit func(s *firejail.Settings)
		// plan is whether Prepared.AppData is set; file whether the folder is planned.
		plan, file bool
		link       bool
		userData   bool
	}{
		{"full", vscodeApp, vscodeOn(firejail.HomeFull), true, true, true, true},
		{"chosen", vscodeApp, vscodeOn(firejail.HomeChosen), true, true, true, true},
		{"empty home: the flag but no folder", vscodeApp, vscodeOn(firejail.HomeEmpty), false, false, false, true},
		{"private home: the flag but no folder", vscodeApp, vscodeOn(firejail.HomePrivate, func(s *firejail.Settings) {
			s.PrivateDir = fakeHome + "/.local/share/jailcat/homes/firejail-demo"
		}), false, false, false, true},
		{"full without own data: only the links are looked after", vscodeApp,
			vscodeOn(firejail.HomeFull, func(s *firejail.Settings) { s.VSCodeOwnData = false }), true, false, false, false},
		{"full without sharing", vscodeApp,
			vscodeOn(firejail.HomeFull, func(s *firejail.Settings) { s.VSCodeShareSettings = false }), true, true, false, true},
		{"not a VS Code app", func(t *testing.T) apps.App {
			a := vscodeApp(t)
			a.Binary = "/usr/bin/vim"
			return a
		}, vscodeOn(firejail.HomeFull), false, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := prepareApp(t, tt.app(t), tt.edit)
			dirs := fakeDirs(t)
			want := filepath.Join(dirs.AppData, "firejail-demo")
			if (p.AppData != nil) != tt.plan {
				t.Fatalf("AppData = %+v, want plan %v", p.AppData, tt.plan)
			}
			if tt.plan {
				if p.AppData.Dir != want || p.AppData.ConfigDir != filepath.Join(dirs.ConfigHome, "Code") ||
					p.AppData.Create != tt.file || p.AppData.Link != tt.link {
					t.Errorf("AppData = %+v", p.AppData)
				}
			}
			var folder *File
			for i := range p.Files {
				if p.Files[i].Kind == KindAppData {
					folder = &p.Files[i]
				}
			}
			if (folder != nil) != tt.file {
				t.Fatalf("the plan has the data folder: %v, want %v", folder != nil, tt.file)
			}
			if folder != nil && (folder.Path != want || folder.Mode != 0o700 || folder.Content != nil || folder.SHA256 != "") {
				t.Errorf("data folder file = %+v", *folder)
			}
			launcher := string(file(t, p, KindLauncher).Content)
			flag := "'--user-data-dir' '" + want + "'"
			if got := strings.Contains(launcher, flag); got != tt.userData {
				t.Errorf("launcher has the flag: %v, want %v\n%s", got, tt.userData, launcher)
			}
			if tt.userData && p.UserDataDir != want || !tt.userData && p.UserDataDir != "" {
				t.Errorf("UserDataDir = %q", p.UserDataDir)
			}
			// Right after the binary, before the extra arguments.
			if tt.userData && !strings.Contains(launcher, "-- '/usr/bin/code' "+flag+" \"$@\"") {
				t.Errorf("the flag is not right after the binary:\n%s", launcher)
			}
		})
	}
}

func TestPrepareAppDataExtraArgsComeAfter(t *testing.T) {
	p, _ := prepareApp(t, vscodeApp(t), vscodeOn(firejail.HomeFull, func(s *firejail.Settings) {
		s.ExtraArgs = []string{"--ozone-platform=wayland"}
	}))
	launcher := string(file(t, p, KindLauncher).Content)
	if !strings.Contains(launcher, "'/usr/bin/code' '--user-data-dir' '"+fakeHome+"/.local/share/jailcat/appdata/firejail-demo' '--ozone-platform=wayland' \"$@\"") {
		t.Errorf("launcher = %s", launcher)
	}
}

func TestPrepareAppDataOutsideTheAppDataFolder(t *testing.T) {
	// The folder is below the appdata folder by construction; a Dirs without one is the
	// only way it is not.
	for _, appData := range []string{"", "relative/appdata", "/"} {
		t.Run(appData, func(t *testing.T) {
			dirs := fakeDirs(t)
			dirs.AppData = appData
			s := firejail.DefaultSettings()
			s.SandboxName = "firejail-demo"
			s.MenuName = "Demo"
			vscodeOn(firejail.HomeFull)(&s)
			_, err := Prepare(context.Background(), dirs, Request{
				BuildID: 3, App: vscodeApp(t), Settings: s, Profile: stubProfile(3), Firejail: "/usr/bin/firejail",
			})
			if appData == "/" {
				// "/" is a folder: a clean path below it is inside.
				if err != nil {
					t.Fatalf("Prepare: %v", err)
				}
				return
			}
			requireRoute(t, err, TagAppDataOutside, "Prepare->appDataFile."+TagAppDataOutside)
		})
	}
}

func TestInstallAppData(t *testing.T) {
	k := newKit(t)
	config := configFiles(t, k)
	p := k.prepare(vscodeOn(firejail.HomeChosen))
	written, err := Install(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	var kinds []Kind
	for _, f := range written {
		kinds = append(kinds, f.Kind)
	}
	if got := strings.Join(kindStrings(kinds), ","); got != "profile,launcher,icon,appdata,desktop" {
		t.Errorf("written kinds = %s", got)
	}
	dir := filepath.Join(k.dirs.AppData, "firejail-code")
	folder := mustFind(t, written, KindAppData)
	if folder.Path != dir || folder.SHA256 != "" || folder.Content != nil {
		t.Errorf("appdata file = %+v", folder)
	}
	if mode(t, dir) != 0o700 || mode(t, k.dirs.AppData) != 0o700 {
		t.Errorf("modes = %v (folder), %v (appdata)", mode(t, dir), mode(t, k.dirs.AppData))
	}
	for name, target := range config {
		link := filepath.Join(dir, "User", name)
		got, err := os.Readlink(link)
		if err != nil || got != target {
			t.Errorf("%s: link = %q, %v; want %q", name, got, err, target)
		}
	}
	// Reading through the link gives the user's file.
	if got := readText(t, filepath.Join(dir, "User", "settings.json")); got != "mine settings.json" {
		t.Errorf("settings through the link = %q", got)
	}
	// Nothing was written in the user's config folder.
	if got := readText(t, config["keybindings.json"]); got != "mine keybindings.json" {
		t.Errorf("the config file changed: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(k.dirs.ConfigHome, "Code", "User"))
	if len(entries) != 3 {
		t.Errorf("the config folder holds %d entries after Install, want the 3 of the user", len(entries))
	}
}

func TestInstallAppDataLinksOnlyExistingTargets(t *testing.T) {
	k := newKit(t)
	writeText(t, filepath.Join(k.dirs.ConfigHome, "Code", "User", "settings.json"), "{}")
	p := k.prepare(vscodeOn(firejail.HomeFull))
	if _, err := Install(context.Background(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(k.dirs.AppData, "firejail-code", "User")
	if !exists(filepath.Join(dir, "settings.json")) {
		t.Error("no link for the settings that exist")
	}
	for _, name := range []string{"keybindings.json", "snippets"} {
		if exists(filepath.Join(dir, name)) {
			t.Errorf("a link for %s, whose target is missing", name)
		}
	}
}

func TestInstallAppDataDoesNotClobber(t *testing.T) {
	k := newKit(t)
	config := configFiles(t, k)
	dir := filepath.Join(k.dirs.AppData, "firejail-code")
	// A settings file of its own, a keybindings link that points elsewhere, and a broken
	// snippets link: all stay as they are.
	writeText(t, filepath.Join(dir, "User", "settings.json"), "the sandbox's own")
	if err := os.Symlink("/elsewhere/keys.json", filepath.Join(dir, "User", "keybindings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/gone/snippets", filepath.Join(dir, "User", "snippets")); err != nil {
		t.Fatal(err)
	}
	p := k.prepare(vscodeOn(firejail.HomeFull))
	if _, err := Install(context.Background(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, filepath.Join(dir, "User", "settings.json")); got != "the sandbox's own" {
		t.Errorf("settings.json = %q", got)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "User", "keybindings.json")); got != "/elsewhere/keys.json" {
		t.Errorf("keybindings link = %q", got)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "User", "snippets")); got != "/gone/snippets" {
		t.Errorf("snippets link = %q", got)
	}
	if got := readText(t, config["settings.json"]); got != "mine settings.json" {
		t.Errorf("the user's file changed: %q", got)
	}
}

func TestInstallAppDataUnlinksOnlyItsLinks(t *testing.T) {
	k := newKit(t)
	config := configFiles(t, k)
	ctx := context.Background()
	if _, err := Install(ctx, k.prepare(vscodeOn(firejail.HomeFull)), nil, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(k.dirs.AppData, "firejail-code")
	user := filepath.Join(dir, "User")
	// The user replaced one link by a file and another by a link to somewhere else.
	for _, name := range []string{"keybindings.json", "snippets"} {
		if err := os.Remove(filepath.Join(user, name)); err != nil {
			t.Fatal(err)
		}
	}
	writeText(t, filepath.Join(user, "keybindings.json"), "their own keys")
	if err := os.Symlink("/elsewhere/snippets", filepath.Join(user, "snippets")); err != nil {
		t.Fatal(err)
	}

	off := k.prepare(vscodeOn(firejail.HomeFull, func(s *firejail.Settings) { s.VSCodeShareSettings = false }))
	again := map[string]bool{}
	for _, f := range off.Files {
		again[f.Path] = true
	}
	if _, err := Install(ctx, off, nil, again); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(user, "settings.json")) {
		t.Error("the link to the shared settings is still there")
	}
	if got := readText(t, filepath.Join(user, "keybindings.json")); got != "their own keys" {
		t.Errorf("a file of theirs was changed: %q", got)
	}
	if got, _ := os.Readlink(filepath.Join(user, "snippets")); got != "/elsewhere/snippets" {
		t.Errorf("a link to somewhere else was removed: %q", got)
	}
	for name, path := range config {
		if !exists(path) {
			t.Errorf("the user's %s is gone", name)
		}
	}
	// Turning the switch off for a data folder that is not there does nothing.
	other := &AppData{Dir: filepath.Join(k.dirs.AppData, "nobody"), ConfigDir: filepath.Join(k.dirs.ConfigHome, "Code")}
	if err := SyncAppData(ctx, other); err != nil || exists(other.Dir) {
		t.Errorf("SyncAppData on a missing folder: %v (exists %v)", err, exists(other.Dir))
	}
}

func TestInstallAppDataKeepsAnExistingFolder(t *testing.T) {
	k := newKit(t)
	dir := filepath.Join(k.dirs.AppData, "firejail-code")
	writeText(t, filepath.Join(dir, "logs", "main.log"), "log")
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), k.prepare(vscodeOn(firejail.HomeFull)), nil, nil); err != nil {
		t.Fatal(err)
	}
	if readText(t, filepath.Join(dir, "logs", "main.log")) != "log" || mode(t, dir) != 0o755 {
		t.Error("an existing data folder was changed")
	}
}

func TestSyncAppDataForTry(t *testing.T) {
	k := newKit(t)
	config := configFiles(t, k)
	p := k.prepare(vscodeOn(firejail.HomeChosen))
	if err := SyncAppData(context.Background(), p.AppData); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(k.dirs.AppData, "firejail-code")
	if mode(t, dir) != 0o700 {
		t.Errorf("mode = %v", mode(t, dir))
	}
	if got, _ := os.Readlink(filepath.Join(dir, "User", "snippets")); got != config["snippets"] {
		t.Errorf("snippets link = %q", got)
	}
	if err := SyncAppData(context.Background(), nil); err != nil {
		t.Errorf("a nil plan: %v", err)
	}
	// Twice is fine.
	if err := SyncAppData(context.Background(), p.AppData); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAppDataFolder(t *testing.T) {
	k := newKit(t)
	p := k.prepare(vscodeOn(firejail.HomeFull))
	folder := file(t, p, KindAppData)
	// An existing folder is no conflict.
	writeText(t, filepath.Join(folder.Path, "x"), "x")
	if c, err := Check(context.Background(), p, nil); err != nil || len(c) != 0 {
		t.Fatalf("Check = %v, %v", c, err)
	}
	// A file where the folder should be is.
	if err := os.RemoveAll(folder.Path); err != nil {
		t.Fatal(err)
	}
	writeText(t, folder.Path, "a file")
	c, err := Check(context.Background(), p, nil)
	if err != nil || len(c) != 1 || c[0].Kind != ConflictForeign || c[0].File.Kind != KindAppData {
		t.Errorf("Check = %+v, %v", c, err)
	}
}

func TestUninstallAppData(t *testing.T) {
	setup := func(t *testing.T) (*kit, []File, string, map[string]string) {
		k := newKit(t)
		config := configFiles(t, k)
		p := k.prepare(vscodeOn(firejail.HomeFull))
		files, err := Install(context.Background(), p, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		dir := mustFind(t, files, KindAppData).Path
		writeText(t, filepath.Join(dir, "logs", "main.log"), "log")
		return k, files, dir, config
	}
	t.Run("kept by default", func(t *testing.T) {
		k, files, dir, _ := setup(t)
		res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
		if err != nil {
			t.Fatal(err)
		}
		if !res.AppDataKept || res.HomeKept || !exists(filepath.Join(dir, "logs", "main.log")) {
			t.Errorf("result = %+v", res)
		}
		for _, f := range res.Removed {
			if f.Kind == KindAppData {
				t.Error("the data folder is in Removed")
			}
		}
	})
	t.Run("deleted with the tick", func(t *testing.T) {
		k, files, dir, config := setup(t)
		other := filepath.Join(k.dirs.AppData, "firejail-other")
		writeText(t, filepath.Join(other, "x"), "another build's data")
		res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteAppData: true})
		if err != nil {
			t.Fatal(err)
		}
		if exists(dir) || res.AppDataKept {
			t.Errorf("the data folder is still there: %+v", res)
		}
		if !exists(filepath.Join(other, "x")) || !exists(k.dirs.AppData) {
			t.Error("another folder or appdata was removed")
		}
		// The links in it did not lead the removal into the user's config.
		for name, path := range config {
			if !exists(path) {
				t.Errorf("the user's %s was removed", name)
			}
		}
		if got := res.Removed[len(res.Removed)-1]; got.Kind != KindAppData {
			t.Errorf("last removed = %+v", got)
		}
	})
	t.Run("the private home tick does not delete it", func(t *testing.T) {
		k, files, dir, _ := setup(t)
		if _, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true}); err != nil {
			t.Fatal(err)
		}
		if !exists(dir) {
			t.Error("DeleteHome removed the data folder")
		}
	})
	t.Run("outside the appdata folder", func(t *testing.T) {
		k, files, _, _ := setup(t)
		victim := filepath.Join(k.root, "precious")
		writeText(t, filepath.Join(victim, "file"), "do not delete")
		for i := range files {
			if files[i].Kind == KindAppData {
				files[i].Path = victim
			}
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteAppData: true})
		requireRoute(t, err, TagUninstallAppDataOutside, "Uninstall."+TagUninstallAppDataOutside)
		if !exists(filepath.Join(victim, "file")) || !exists(mustFind(t, files, KindProfile).Path) {
			t.Error("something was removed before the error")
		}
		if _, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs}); err != nil {
			t.Errorf("without DeleteAppData the record is harmless: %v", err)
		}
		if !exists(filepath.Join(victim, "file")) {
			t.Error("the folder was removed without DeleteAppData")
		}
	})
	t.Run("a link inside appdata leads out of it", func(t *testing.T) {
		k, files, _, _ := setup(t)
		victim := filepath.Join(k.root, "precious")
		writeText(t, filepath.Join(victim, "deep", "file"), "do not delete")
		link := filepath.Join(k.dirs.AppData, "linked")
		if err := os.Symlink(victim, link); err != nil {
			t.Fatal(err)
		}
		for i := range files {
			if files[i].Kind == KindAppData {
				files[i].Path = filepath.Join(link, "deep")
			}
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteAppData: true})
		requireRoute(t, err, TagUninstallAppDataOutside, "Uninstall."+TagUninstallAppDataOutside)
		if !exists(filepath.Join(victim, "deep", "file")) {
			t.Error("the folder behind the link was removed")
		}
	})
}

func TestAppDataLinkRoutes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	ctx := context.Background()
	plan := func(k *kit, link bool) *AppData {
		return &AppData{
			Dir: filepath.Join(k.dirs.AppData, "firejail-code"), ConfigDir: filepath.Join(k.dirs.ConfigHome, "Code"),
			Create: true, Link: link,
		}
	}
	t.Run("canceled", func(t *testing.T) {
		k := newKit(t)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		requireRoute(t, SyncAppData(canceled, plan(k, true)), tagAppDataCanceled, "SyncAppData->syncLinks."+tagAppDataCanceled)
	})
	t.Run("the User folder is a file", func(t *testing.T) {
		k := newKit(t)
		configFiles(t, k)
		a := plan(k, true)
		writeText(t, filepath.Join(a.Dir, "User"), "a file")
		requireRoute(t, SyncAppData(ctx, a), tagAppDataStatFailed, "SyncAppData->syncLinks->makeLink."+tagAppDataStatFailed)
		a.Link = false
		requireRoute(t, SyncAppData(ctx, a), tagAppDataStatFailed, "SyncAppData->syncLinks->removeLink."+tagAppDataStatFailed)
	})
	t.Run("the User folder cannot be written", func(t *testing.T) {
		k := newKit(t)
		configFiles(t, k)
		a := plan(k, true)
		user := filepath.Join(a.Dir, "User")
		if err := os.MkdirAll(user, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(user, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(user, 0o700) })
		e := requireRouteMeta(t, SyncAppData(ctx, a), tagAppDataLinkFailed, "SyncAppData->syncLinks->makeLink."+tagAppDataLinkFailed)
		if e.Metadata["link"] == nil || e.Metadata["target"] == nil {
			t.Errorf("metadata = %v", e.Metadata)
		}

		// A link of ours in a folder that cannot be written cannot be removed.
		if err := os.Chmod(user, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := SyncAppData(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(user, 0o500); err != nil {
			t.Fatal(err)
		}
		a.Link = false
		requireRoute(t, SyncAppData(ctx, a), tagAppDataUnlinkFailed, "SyncAppData->syncLinks->removeLink."+tagAppDataUnlinkFailed)
	})
	t.Run("the data folder cannot be made", func(t *testing.T) {
		k := newKit(t)
		writeText(t, k.dirs.AppData, "a file")
		a := plan(k, false)
		requireRoute(t, SyncAppData(ctx, a), tagDirCreateFailed, "SyncAppData->ensureDir."+tagDirCreateFailed)
	})
}
