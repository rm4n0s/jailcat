//go:build jailcat_firejail

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// fakeCode is a program named "code" that tells its first two arguments and then runs the
// script that follows "-c", so the probes of the lab run in place of VS Code.
const fakeCode = `#!/bin/sh
echo "arg1=$1"
echo "arg2=$2"
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-c" ]; then
		exec /bin/sh -c "$2"
	fi
	shift
done
`

// vscodeLab makes the fake "code" in a folder of its own and returns the app.
func vscodeLab(t *testing.T, l *lab) (apps.App, string) {
	t.Helper()
	bin := filepath.Join(l.root, "codebin")
	writeText(t, filepath.Join(bin, "code"), fakeCode)
	if err := os.Chmod(filepath.Join(bin, "code"), 0o755); err != nil {
		t.Fatal(err)
	}
	app := apps.App{
		ID: "code", Kind: apps.KindBinary, Name: "code", Binary: filepath.Join(bin, "code"), BinaryName: "code", Terminal: true,
	}
	return app, bin
}

// A build of VS Code with Only the folders I choose: the launcher passes the build's own
// data folder; the folder is writable and holds links to the settings of the user's VS
// Code; a folder that was not chosen is not there; the shell startup files, when the
// user has some, cannot be written.
//
// The probe names the real home only through "${HOME}" in the lines of the shell files
// and one folder that does not exist; nothing is written there, and the probe only asks
// whether a file can be written ("test -w"), never writes to it.
func TestFirejailVSCodeOwnDataFolder(t *testing.T) {
	l := newLab(t)
	env, err := xdg.FromOS()
	if err != nil {
		t.Fatal(err)
	}
	realHome := filepath.Clean(env.Home())
	if !filepath.IsAbs(realHome) || realHome == "/" {
		t.Fatalf("the home folder %q cannot be used for this test", realHome)
	}
	l.profileHome = realHome
	app, bin := vscodeLab(t, l)
	other := l.data("other")
	writeText(t, filepath.Join(other, "file.txt"), "other\n")
	config := filepath.Join(l.dirs.ConfigHome, "Code", "User")
	writeText(t, filepath.Join(config, "settings.json"), "{\"mine\": true}\n")

	launcher, files := l.buildApp(app, "", func(s *firejail.Settings) {
		s.Home = firejail.HomeChosen
		s.Folders = []firejail.Folder{
			{Path: filepath.Join(realHome, ".jailcat-test-no-such-folder"), Access: firejail.AccessReadWrite},
			{Path: bin, Access: firejail.AccessReadOnly},
		}
		s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles = true, true, true
	})
	dir := mustFind(t, files, KindAppData).Path
	if want := filepath.Join(l.dirs.AppData, "firejail-probe-a"); dir != want {
		t.Fatalf("the data folder is %s, want %s", dir, want)
	}
	profile := l.profileOf(files)
	for _, want := range []string{"\ntab\n", "\nwhitelist " + dir + "\n", "\nread-only ${HOME}/.bashrc\n"} {
		if !strings.Contains(profile, want) {
			t.Fatalf("the profile lacks %q:\n%s", strings.TrimSpace(want), profile)
		}
	}
	launcherText := readText(t, launcher)
	if !strings.Contains(launcherText, "'--user-data-dir' "+shq(t, dir)+" \"$@\"") {
		t.Fatalf("the launcher does not pass the data folder:\n%s", launcherText)
	}

	out := l.run(launcher, strings.Join([]string{
		"touch " + shq(t, filepath.Join(dir, "probe")) + " && echo data-writable=yes || echo data-writable=no",
		"echo settings=$(cat " + shq(t, filepath.Join(dir, "User", "settings.json")) + " | tr -d '\\n ')",
		"test -e " + shq(t, filepath.Join(other, "file.txt")) + " && echo other=there || echo other=missing",
		`if [ -e "$HOME/.bashrc" ]; then if [ -w "$HOME/.bashrc" ]; then echo bashrc=writable; else echo bashrc=read-only; fi; else echo bashrc=none; fi`,
		`echo home-list=$(ls -A "$HOME" | sort | tr '\n' ',')`,
	}, "\n"))

	expectProbe(t, out, "arg1", "--user-data-dir")
	expectProbe(t, out, "arg2", dir)
	expectProbe(t, out, "data-writable", "yes")
	if !hostHas(filepath.Join(dir, "probe")) {
		t.Error("the file the sandbox wrote in the data folder is not on the host")
	}
	expectProbe(t, out, "settings", "{\"mine\":true}")
	expectProbe(t, out, "other", "missing")
	if got := out["bashrc"]; got == "writable" || got == "" {
		t.Errorf("bashrc = %q: the shell startup file is not read-only", got)
	} else if got == "none" {
		t.Log("the user has no ~/.bashrc: the read-only part of the probe had nothing to look at")
	}

	// The home shows only what was chosen: the shell startup files and the folders that
	// lead to them. Anything else of the real home would be a leak.
	allowed := map[string]bool{".local": true, ".config": true}
	for _, name := range firejail.VSCodeShellFiles() {
		allowed[strings.SplitN(name, "/", 2)[0]] = true
	}
	for _, name := range strings.Split(out["home-list"], ",") {
		if name != "" && !allowed[name] {
			t.Errorf("the home of the sandbox shows %q, which was not chosen (home list: %s)", name, out["home-list"])
		}
	}
}

// With a home the real one is kept out of (an empty home) the launcher still passes the
// data folder; the profile has no line about it, and the sandbox starts.
func TestFirejailVSCodeOwnDataWithEmptyHome(t *testing.T) {
	l := newLab(t)
	app, _ := vscodeLab(t, l)
	launcher, files := l.buildApp(app, "", func(s *firejail.Settings) {
		s.Home = firejail.HomeEmpty
		s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles = true, true, true
	})
	for _, f := range files {
		if f.Kind == KindAppData {
			t.Errorf("a data folder was made for an empty home: %s", f.Path)
		}
	}
	profile := l.profileOf(files)
	for _, bad := range []string{"appdata", "tab\n", ".bashrc"} {
		if strings.Contains(profile, bad) {
			t.Errorf("the profile has %q:\n%s", bad, profile)
		}
	}
	dir := filepath.Join(l.dirs.AppData, "firejail-probe-a")
	out := l.run(launcher, "echo done=yes")
	expectProbe(t, out, "arg1", "--user-data-dir")
	expectProbe(t, out, "arg2", dir)
	expectProbe(t, out, "done", "yes")
}
