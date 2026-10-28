package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// vscodeConfig writes the user's VS Code settings in the scratch config folder and returns
// the folder.
func vscodeConfig(t *testing.T, bs buildSystem) string {
	t.Helper()
	dir := filepath.Join(bs.sys.dirs.ConfigHome, "Code", "User")
	writeFile(t, filepath.Join(dir, "settings.json"), "{}", 0o644)
	writeFile(t, filepath.Join(dir, "keybindings.json"), "[]", 0o644)
	return dir
}

func TestLocalBackendBuildVSCodeOwnData(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	config := vscodeConfig(t, bs)
	e := bs.editor(t, nil)
	if !e.IsVSCode() {
		t.Fatal("the fixture app is not VS Code")
	}
	res := bs.build(t, e)

	dir := filepath.Join(bs.sys.dirs.AppData, "firejail-code")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("the data folder: %v, %v", info, err)
	}
	// Links for the settings that exist, none for the ones that do not.
	for name, want := range map[string]bool{"settings.json": true, "keybindings.json": true, "snippets": false} {
		target, err := os.Readlink(filepath.Join(dir, "User", name))
		if want != (err == nil) || (want && target != filepath.Join(config, name)) {
			t.Errorf("%s: link %q, %v; want a link %v", name, target, err, want)
		}
	}
	launcher, err := os.ReadFile(filepath.Join(bs.sys.dirs.Bin, "firejail-code"))
	if err != nil || !strings.Contains(string(launcher), "'--user-data-dir' '"+dir+"' \"$@\"") {
		t.Errorf("the launcher: %v\n%s", err, launcher)
	}
	profile, err := os.ReadFile(filepath.Join(bs.sys.dirs.Profiles, "firejail-code.profile"))
	if err != nil || !strings.Contains(string(profile), "noblacklist ${HOME}/.local/share/jailcat/appdata/firejail-code") {
		t.Errorf("the profile lacks the data folder: %v\n%s", err, profile)
	}
	for _, line := range []string{
		"noblacklist ${HOME}/.config/Code/User/settings.json",
		"noblacklist ${HOME}/.config/Code/User/keybindings.json",
	} {
		if !strings.Contains(string(profile), line) {
			t.Errorf("the profile lacks %q\n%s", line, profile)
		}
	}
	recorded, err := bs.store.InstalledFiles(ctx, res.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range recorded {
		found = found || (f.Kind == "appdata" && f.Path == dir && f.SHA256 == "")
	}
	if !found {
		t.Errorf("the data folder is not recorded: %+v", recorded)
	}

	// A rebuild with Share settings off removes the links Jailcat made and keeps the folder.
	stored, err := bs.store.GetBuild(ctx, res.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	again := bs.editor(t, &stored)
	again.Set(func(s *firejail.Settings) { s.VSCodeShareSettings = false })
	bs.build(t, again)
	if _, err := os.Lstat(filepath.Join(dir, "User", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("the link is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config, "settings.json")); err != nil {
		t.Errorf("the user's settings are gone: %v", err)
	}
}

func TestLocalBackendBuildVSCodeSwitchedOff(t *testing.T) {
	bs := newBuildSystem(t)
	e := bs.editor(t, nil)
	e.Set(func(s *firejail.Settings) { s.VSCodeOwnData = false })
	res := bs.build(t, e)
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.AppData, "firejail-code")); !os.IsNotExist(err) {
		t.Errorf("a data folder was made: %v", err)
	}
	launcher, err := os.ReadFile(filepath.Join(bs.sys.dirs.Bin, "firejail-code"))
	if err != nil || strings.Contains(string(launcher), "--user-data-dir") {
		t.Errorf("the launcher passes a data folder: %v\n%s", err, launcher)
	}
	for _, f := range res.Files {
		if f.Kind == "appdata" {
			t.Errorf("the data folder is recorded")
		}
	}
}

func TestLocalBackendBuildVSCodeEmptyHome(t *testing.T) {
	// The flag is passed in every home choice; the folder is made only for Full home and
	// Only the folders I choose.
	bs := newBuildSystem(t)
	e := bs.editor(t, nil)
	e.SetHome(firejail.HomeEmpty)
	bs.build(t, e)
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.AppData, "firejail-code")); !os.IsNotExist(err) {
		t.Errorf("a data folder was made for an empty home: %v", err)
	}
	launcher, err := os.ReadFile(filepath.Join(bs.sys.dirs.Bin, "firejail-code"))
	dir := filepath.Join(bs.sys.dirs.AppData, "firejail-code")
	if err != nil || !strings.Contains(string(launcher), "'--user-data-dir' '"+dir+"'") {
		t.Errorf("the launcher: %v\n%s", err, launcher)
	}
	profile, err := os.ReadFile(filepath.Join(bs.sys.dirs.Profiles, "firejail-code.profile"))
	if err != nil || strings.Contains(string(profile), "appdata") {
		t.Errorf("the profile names the data folder: %v\n%s", err, profile)
	}
}

func TestLocalBackendTryVSCode(t *testing.T) {
	bs := newBuildSystem(t)
	ctx := context.Background()
	config := vscodeConfig(t, bs)
	e := bs.editor(t, nil)
	e.Set(func(s *firejail.Settings) { s.ExtraArgs = []string{"--ozone-platform=wayland"} })
	session, err := bs.backend.Try(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Stop(context.Background()) })
	var first string
	select {
	case first = <-session.Lines():
	case <-time.After(10 * time.Second):
		t.Fatal("no output")
	}
	dir := filepath.Join(bs.sys.dirs.AppData, "firejail-code")
	if !strings.HasSuffix(first, " --user-data-dir "+dir+" --ozone-platform=wayland") {
		t.Errorf("the arguments: %q", first)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the data folder: %v, %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "User", "settings.json")); err != nil || target != filepath.Join(config, "settings.json") {
		t.Errorf("the link: %q, %v", target, err)
	}
	// Nothing was installed or stored.
	if n := countBuilds(t, bs); n != 0 {
		t.Errorf("a Try stored %d builds", n)
	}
}

func TestLocalBackendTryVSCodeOff(t *testing.T) {
	bs := newBuildSystem(t)
	e := bs.editor(t, nil)
	e.Set(func(s *firejail.Settings) { s.VSCodeOwnData = false })
	session, err := bs.backend.Try(context.Background(), e.Request())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Stop(context.Background()) })
	first := <-session.Lines()
	if strings.Contains(first, "--user-data-dir") {
		t.Errorf("the arguments: %q", first)
	}
	if _, err := os.Stat(filepath.Join(bs.sys.dirs.AppData, "firejail-code")); !os.IsNotExist(err) {
		t.Errorf("a data folder was made: %v", err)
	}
}

func TestLocalBackendUninstallVSCodeData(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		choice     UninstallChoice
		wantFolder bool
		wantKept   bool
	}{
		{"kept by default", UninstallChoice{KeepSettings: true}, true, true},
		{"the private home tick does not delete it", UninstallChoice{KeepSettings: true, DeleteHome: true}, true, true},
		{"deleted with its own tick", UninstallChoice{KeepSettings: true, DeleteAppData: true}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := newBuildSystem(t)
			config := vscodeConfig(t, bs)
			built := bs.build(t, bs.editor(t, nil))
			dir := filepath.Join(bs.sys.dirs.AppData, "firejail-code")
			writeFile(t, filepath.Join(dir, "logs", "main.log"), "log", 0o644)
			res, err := bs.backend.Uninstall(ctx, built.Build, tt.choice)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(dir); (err == nil) != tt.wantFolder {
				t.Errorf("data folder present = %v, want %v (%v)", err == nil, tt.wantFolder, err)
			}
			if res.AppDataKept != tt.wantKept {
				t.Errorf("AppDataKept = %v, want %v", res.AppDataKept, tt.wantKept)
			}
			if _, err := os.Stat(filepath.Join(config, "settings.json")); err != nil {
				t.Errorf("the user's settings are gone: %v", err)
			}
			if !UninstallDone(res, tt.choice) {
				t.Errorf("the uninstall is not done: %+v", res)
			}
		})
	}
}
