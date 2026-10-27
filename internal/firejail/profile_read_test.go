package firejail

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// env is a scratch firejail setup: a system folder, a user folder and a home, all in a
// temporary folder.
type env struct {
	t    *testing.T
	home string
	sys  string
	user string
	cfg  Config
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		t:    t,
		home: filepath.Join(root, "home"),
		sys:  filepath.Join(root, "etc-firejail"),
		cfg:  DefaultConfig(),
	}
	if bad := invalidFilename(root); bad != "" {
		// firejail refuses these characters in a profile path (t.TempDir keeps "," and "%"
		// from the test name), so the test name must not have them.
		t.Fatalf("the temporary folder %q is not a valid firejail path (%s): rename the test", root, bad)
	}
	e.user = filepath.Join(e.home, ".config", "firejail")
	for _, d := range []string{e.home, e.sys, e.user} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *env) dirs() xdg.Dirs {
	return xdg.Dirs{Home: e.home, FirejailSystem: e.sys, FirejailUser: e.user}
}

func (e *env) opts() ReadOpts {
	return ReadOpts{Dirs: e.dirs(), Config: e.cfg}
}

// sysFile writes a file into the system folder and returns its path.
func (e *env) sysFile(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.sys, name)
	writeFile(e.t, p, content)
	return p
}

// userFile writes a file into the user folder and returns its path.
func (e *env) userFile(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.user, name)
	writeFile(e.t, p, content)
	return p
}

// read reads a profile made of the given lines, written into the system folder.
func (e *env) read(lines ...string) (*Profile, error) {
	e.t.Helper()
	p := e.sysFile("top.profile", strings.Join(lines, "\n")+"\n")
	return Read(context.Background(), p, e.opts())
}

// mustRead is read that fails the test on an error.
func (e *env) mustRead(lines ...string) *Profile {
	e.t.Helper()
	prof, err := e.read(lines...)
	if err != nil {
		e.t.Fatalf("Read: %v", err)
	}
	return prof
}

func texts(lines []Line) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

func requireTexts(t *testing.T, what string, got []Line, want ...string) {
	t.Helper()
	if g := texts(got); !slices.Equal(g, want) {
		t.Errorf("%s = %q, want %q", what, g, want)
	}
}

func TestReadFixtureCodeProfile(t *testing.T) {
	sys := fixtureDir(t)
	user := t.TempDir()
	home := t.TempDir()
	cfg, err := ReadConfig(context.Background(), filepath.Join(sys, "firejail.config"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sys, "code.profile")
	prof, err := Read(context.Background(), path, ReadOpts{
		Dirs:   xdg.Dirs{Home: home, FirejailSystem: sys, FirejailUser: user},
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if prof.Path != path {
		t.Errorf("Path = %q, want %q", prof.Path, path)
	}
	// The counts below were made by reading the fixture by hand: code.profile has 6
	// lines of its own that take effect, electron-common.profile 6, blink-common.profile
	// 7, allow-common-devel.inc 30, disable-common.inc 620 and disable-programs.inc 1314.
	if got, want := len(prof.Lines), 6+6+7+30+620+1314; got != want {
		t.Errorf("len(Lines) = %d, want %d", got, want)
	}
	if got := len(prof.Ignores); got != 15 {
		t.Errorf("len(Ignores) = %d, want 15", got)
	}
	if got := len(prof.Ignored); got != 15 {
		t.Errorf("len(Ignored) = %d, want 15", got)
	}
	if prof.Quiet {
		t.Error("Quiet = true, want false")
	}
	if len(prof.Skipped) != 0 {
		t.Errorf("Skipped = %v, want none", prof.Skipped)
	}

	// The lines of code.profile come first, then allow-common-devel.inc's, in order.
	first := prof.Lines[0]
	if first.Text != "noblacklist ${HOME}/.config/Code" || first.File != path || first.Number != 25 || len(first.IncludedFrom) != 0 {
		t.Errorf("first line = %+v", first)
	}

	find := func(text, file string) (int, Line) {
		t.Helper()
		for i, l := range prof.Lines {
			if l.Text == text && filepath.Base(l.File) == file {
				return i, l
			}
		}
		t.Fatalf("line %q of %s not found", text, file)
		return 0, Line{}
	}
	iSound, sound := find("nosound", "code.profile")
	if sound.Number != 33 || sound.Command != "nosound" || sound.Args != "" {
		t.Errorf("nosound = %+v", sound)
	}
	_, nou2f := find("nou2f", "electron-common.profile")
	if nou2f.Number != 18 || !slices.Equal(nou2f.IncludedFrom, []string{path}) {
		t.Errorf("nou2f = %+v", nou2f)
	}
	find("novideo", "electron-common.profile")
	find("private-dev", "electron-common.profile")
	iNoexec, noexec := find("noexec /tmp", "code.profile")
	if noexec.Command != "noexec" || noexec.Args != "/tmp" {
		t.Errorf("noexec = %+v", noexec)
	}
	iElectron, _ := find("noblacklist ${HOME}/.config/Electron", "electron-common.profile")
	iDevel, _ := find("noblacklist ${HOME}/.config/Code - OSS", "code.profile")
	if iDevel >= iSound || iSound >= iNoexec || iNoexec >= iElectron {
		t.Errorf("order wrong: devel=%d sound=%d noexec=%d electron=%d", iDevel, iSound, iNoexec, iElectron)
	}
	// A line of disable-common.inc was included by blink-common.profile, which
	// electron-common.profile included, which code.profile included.
	var deep Line
	for _, l := range prof.Lines {
		if filepath.Base(l.File) == "disable-common.inc" {
			deep = l
			break
		}
	}
	wantChain := []string{path, filepath.Join(sys, "electron-common.profile"), filepath.Join(sys, "blink-common.profile")}
	if !slices.Equal(deep.IncludedFrom, wantChain) {
		t.Errorf("disable-common.inc IncludedFrom = %v, want %v", deep.IncludedFrom, wantChain)
	}
	// "dbus-user none" of electron-common.profile was ignored, and the ignore line is
	// code.profile's.
	for _, l := range prof.Lines {
		if strings.HasPrefix(l.Text, "dbus-user ") || strings.HasPrefix(l.Text, "dbus-system ") || l.Text == "apparmor" || l.Text == "disable-mnt" {
			t.Errorf("line %+v should have been ignored", l)
		}
	}
	var dbus *Ignored
	for i, ig := range prof.Ignored {
		if ig.Line.Text == "dbus-user none" {
			dbus = &prof.Ignored[i]
		}
	}
	if dbus == nil {
		t.Fatal("dbus-user none is not in Ignored")
	}
	if dbus.Line.File != filepath.Join(sys, "electron-common.profile") || dbus.Line.Number != 24 {
		t.Errorf("ignored line = %+v", dbus.Line)
	}
	if dbus.By.File != path || dbus.By.Number != 22 || dbus.By.Text != "ignore dbus-user none" || dbus.By.Command != "ignore" || dbus.By.Args != "dbus-user none" {
		t.Errorf("By = %+v", dbus.By)
	}
	// The ignore lines in order.
	wantIgnores := []string{
		"ignore include disable-devel.inc", "ignore include disable-exec.inc",
		"ignore include disable-interpreters.inc", "ignore include disable-xdg.inc",
		"ignore whitelist ${DOWNLOADS}", "ignore whitelist ${HOME}/.config/Electron",
		"ignore whitelist ${HOME}/.config/electron*-flag*.conf", "ignore include whitelist-common.inc",
		"ignore include whitelist-runuser-common.inc", "ignore include whitelist-usr-share-common.inc",
		"ignore include whitelist-var-common.inc", "ignore apparmor", "ignore disable-mnt",
		"ignore dbus-user none", "ignore dbus-system none",
	}
	requireTexts(t, "Ignores", prof.Ignores, wantIgnores...)

	// The ignored includes were not followed; the others were.
	var resolved []string
	for _, inc := range prof.Includes {
		resolved = append(resolved, inc.Resolved)
		if strings.Contains(inc.Resolved, "disable-xdg.inc") || strings.Contains(inc.Resolved, "whitelist-") || strings.Contains(inc.Resolved, "disable-devel.inc") {
			t.Errorf("ignored include was followed: %+v", inc)
		}
	}
	foundXDG := false
	for _, ig := range prof.Ignored {
		if ig.Line.Text == "include disable-xdg.inc" {
			foundXDG = true
			if ig.By.Text != "ignore include disable-xdg.inc" || ig.By.File != path {
				t.Errorf("disable-xdg.inc ignored by %+v", ig.By)
			}
		}
	}
	if !foundXDG {
		t.Error("include disable-xdg.inc is not in Ignored")
	}
	wantIncludes := []string{
		"code.local", "globals.local", filepath.Join(sys, "allow-common-devel.inc"), "allow-common-devel.local",
		filepath.Join(sys, "electron-common.profile"), "electron-common.local", filepath.Join(sys, "blink-common.profile"),
		"blink-common.local", filepath.Join(sys, "disable-common.inc"), "disable-common.local",
		filepath.Join(sys, "disable-programs.inc"), "disable-programs.local",
	}
	if !slices.Equal(resolved, wantIncludes) {
		t.Errorf("Includes resolved = %q\nwant %q", resolved, wantIncludes)
	}
	// The .local files do not exist, so firejail skips them.
	wantMissing := []string{
		"code.local", "globals.local", "allow-common-devel.local", "electron-common.local",
		"blink-common.local", "disable-common.local", "disable-programs.local",
	}
	if !slices.Equal(prof.Missing, wantMissing) {
		t.Errorf("Missing = %q, want %q", prof.Missing, wantMissing)
	}
	for _, inc := range prof.Includes {
		wantSkipped := ""
		if slices.Contains(wantMissing, inc.Resolved) {
			wantSkipped = "missing"
		}
		if inc.Skipped != wantSkipped {
			t.Errorf("include %q Skipped = %q, want %q", inc.Resolved, inc.Skipped, wantSkipped)
		}
	}
}

func TestReadFixtureDefaultProfile(t *testing.T) {
	sys := fixtureDir(t)
	prof, err := Read(context.Background(), filepath.Join(sys, "default.profile"), ReadOpts{
		Dirs:   xdg.Dirs{Home: t.TempDir(), FirejailSystem: sys, FirejailUser: t.TempDir()},
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// default.profile has 12 lines of its own, landlock-common.inc 1 more than its
	// "include landlock-common.local", disable-common.inc 620 and disable-programs.inc 1314.
	own := 0
	byFile := map[string]int{}
	for _, l := range prof.Lines {
		byFile[filepath.Base(l.File)]++
		if filepath.Base(l.File) == "default.profile" {
			own++
		}
	}
	if own != 12 {
		t.Errorf("default.profile lines = %d, want 12 (%v)", own, byFile)
	}
	if byFile["disable-common.inc"] != 620 || byFile["disable-programs.inc"] != 1314 {
		t.Errorf("lines per file = %v", byFile)
	}
	if len(prof.Ignores) != 0 || len(prof.Ignored) != 0 {
		t.Errorf("Ignores=%d Ignored=%d, want none", len(prof.Ignores), len(prof.Ignored))
	}
	found := false
	for _, l := range prof.Lines {
		if l.Text == "protocol unix,inet,inet6" && l.Command == "protocol" && l.Args == "unix,inet,inet6" {
			found = true
		}
	}
	if !found {
		t.Error("protocol line not found")
	}
}

func TestReadUserLocalFile(t *testing.T) {
	sys := fixtureDir(t)
	user := t.TempDir()
	home := t.TempDir()
	local := filepath.Join(user, "code.local")
	writeFile(t, local, "# my changes\nnogroups\n")
	globals := filepath.Join(user, "globals.local")
	writeFile(t, globals, "ignore nosound\n")

	path := filepath.Join(sys, "code.profile")
	prof, err := Read(context.Background(), path, ReadOpts{
		Dirs:   xdg.Dirs{Home: home, FirejailSystem: sys, FirejailUser: user},
		Config: DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var added *Line
	for i, l := range prof.Lines {
		if l.Text == "nogroups" && l.File == local {
			added = &prof.Lines[i]
		}
		if l.Text == "nosound" {
			t.Errorf("nosound should be ignored by globals.local: %+v", l)
		}
	}
	if added == nil {
		t.Fatal("nogroups of code.local is not in Lines")
	}
	if added.Number != 2 || !slices.Equal(added.IncludedFrom, []string{path}) {
		t.Errorf("nogroups = %+v", *added)
	}
	// code.local is read first (before the ignore lines of code.profile), so it is the
	// first line of the result.
	if prof.Lines[0].File != local {
		t.Errorf("first line is from %s, want %s", prof.Lines[0].File, local)
	}
	// The ignore of globals.local took effect for the later nosound of code.profile.
	var by Line
	for _, ig := range prof.Ignored {
		if ig.Line.Text == "nosound" {
			by = ig.By
		}
	}
	if by.File != globals || by.Text != "ignore nosound" {
		t.Errorf("nosound ignored by %+v", by)
	}
	if slices.Contains(prof.Missing, "code.local") || slices.Contains(prof.Missing, "globals.local") {
		t.Errorf("Missing = %q, but both .local files exist", prof.Missing)
	}
	if len(prof.Ignores) != 16 {
		t.Errorf("len(Ignores) = %d, want 15 + 1 of globals.local", len(prof.Ignores))
	}
}

func TestReadIncludeSearch(t *testing.T) {
	t.Run("user folder wins over the system folder", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("base.profile", "nosound\n")
		userBase := e.userFile("base.profile", "notv\n")
		prof := e.mustRead("include base.profile")
		requireTexts(t, "Lines", prof.Lines, "notv")
		if prof.Includes[0].Resolved != userBase {
			t.Errorf("Resolved = %q, want %q", prof.Includes[0].Resolved, userBase)
		}
	})
	t.Run("system folder when the user folder has none", func(t *testing.T) {
		e := newEnv(t)
		sysBase := e.sysFile("base.profile", "nosound\n")
		prof := e.mustRead("include base.profile")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		if prof.Includes[0].Resolved != sysBase {
			t.Errorf("Resolved = %q, want %q", prof.Includes[0].Resolved, sysBase)
		}
	})
	t.Run("a name with a slash is used as it is", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("base.profile", "nosound\n")
		other := filepath.Join(t.TempDir(), "mine.inc")
		writeFile(t, other, "notv\n")
		prof := e.mustRead("include "+other, "include "+e.user+"/../../../etc-firejail/base.profile")
		requireTexts(t, "Lines", prof.Lines, "notv", "nosound")
	})
	t.Run("macros in the include path", func(t *testing.T) {
		e := newEnv(t)
		writeFile(t, filepath.Join(e.home, "profiles", "mine.inc"), "notv\n")
		e.sysFile("sys.inc", "nosound\n")
		writeFile(t, filepath.Join(e.home, "tilde.inc"), "nodvd\n")
		prof := e.mustRead("include ${HOME}/profiles/mine.inc", "include ${CFG}/sys.inc", "include ~/tilde.inc")
		requireTexts(t, "Lines", prof.Lines, "notv", "nosound", "nodvd")
		wantFiles := []string{
			filepath.Join(e.home, "profiles", "mine.inc"), filepath.Join(e.sys, "sys.inc"), filepath.Join(e.home, "tilde.inc"),
		}
		for i, inc := range prof.Includes {
			if inc.Resolved != wantFiles[i] {
				t.Errorf("Includes[%d].Resolved = %q, want %q", i, inc.Resolved, wantFiles[i])
			}
		}
	})
	t.Run("the include argument keeps its inner spaces", func(t *testing.T) {
		e := newEnv(t)
		writeFile(t, filepath.Join(e.home, "my profiles", "a b.inc"), "notv\n")
		prof := e.mustRead("include ${HOME}/my  profiles/a\t b.inc")
		requireTexts(t, "Lines", prof.Lines, "notv")
	})
	t.Run("a bare name is not looked for in the working folder", func(t *testing.T) {
		e := newEnv(t)
		if _, err := e.read("include read_test_file_in_cwd.inc"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestReadMissingFiles(t *testing.T) {
	t.Run("a missing .local is skipped", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("include top.local", "include ${HOME}/x.local", "nosound")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		wantMissing := []string{"top.local", filepath.Join(e.home, "x.local")}
		if !slices.Equal(prof.Missing, wantMissing) {
			t.Errorf("Missing = %q, want %q", prof.Missing, wantMissing)
		}
		if len(prof.Includes) != 2 || prof.Includes[0].Skipped != "missing" || prof.Includes[1].Skipped != "missing" {
			t.Errorf("Includes = %+v", prof.Includes)
		}
	})
	t.Run("a missing top-level .local is skipped", func(t *testing.T) {
		e := newEnv(t)
		p := filepath.Join(e.sys, "nothing.local")
		prof, err := Read(context.Background(), p, e.opts())
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(prof.Lines) != 0 || !slices.Equal(prof.Missing, []string{p}) {
			t.Errorf("profile = %+v", prof)
		}
	})
	t.Run("the other missing files are errors", func(t *testing.T) {
		tests := []struct {
			name    string
			include string
		}{
			{"bare name", "include nothing.inc"},
			{"absolute path", "include /nonexistent-jailcat-test/nothing.inc"},
			{"home path", "include ${HOME}/nothing.inc"},
			{"two .local in a name", "include nothing.local.local"},
			{"name that only starts with .local", "include nothing.localx"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := newEnv(t)
				_, err := e.read(tt.include)
				if err == nil {
					t.Fatal("expected an error")
				}
				requireRoute(t, err, "Read->reader.readFile->reader.readLine->reader.readInclude->reader.readFile.ProfileReadFailed")
			})
		}
	})
	t.Run("a missing top-level profile is an error", func(t *testing.T) {
		e := newEnv(t)
		_, err := Read(context.Background(), filepath.Join(e.sys, "nothing.profile"), e.opts())
		requireRoute(t, err, "Read->reader.readFile.ProfileReadFailed")
	})
	t.Run("a directory is an error even when named .local", func(t *testing.T) {
		e := newEnv(t)
		if err := os.Mkdir(filepath.Join(e.sys, "dir.local"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := e.read("include dir.local")
		requireRoute(t, err, "reader.readInclude->reader.readFile.ProfileReadFailed")
	})
	t.Run("a .local that cannot be read is an error", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can read any file")
		}
		e := newEnv(t)
		p := e.sysFile("secret.local", "nosound\n")
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		_, err := e.read("include secret.local")
		requireRoute(t, err, "reader.readInclude->reader.readFile.ProfileReadFailed")
	})
	t.Run("a dangling symlink in a firejail folder is found, then fails", func(t *testing.T) {
		e := newEnv(t)
		if err := os.Symlink(filepath.Join(e.sys, "gone"), filepath.Join(e.sys, "link.inc")); err != nil {
			t.Fatal(err)
		}
		_, err := e.read("include link.inc")
		requireRoute(t, err, "reader.readInclude->reader.readFile.ProfileReadFailed")
	})
	t.Run("a dangling symlink named .local is skipped", func(t *testing.T) {
		e := newEnv(t)
		if err := os.Symlink(filepath.Join(e.sys, "gone"), filepath.Join(e.sys, "link.local")); err != nil {
			t.Fatal(err)
		}
		prof := e.mustRead("include link.local", "nosound")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		if !slices.Equal(prof.Missing, []string{filepath.Join(e.sys, "link.local")}) {
			t.Errorf("Missing = %q", prof.Missing)
		}
	})
}

func TestReadIncludeErrors(t *testing.T) {
	t.Run("$HOME is not allowed", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read("include $HOME/x.inc")
		requireRoute(t, err, "reader.readInclude->reader.expandMacros.HomeMacroNotAllowed")
	})
	t.Run("rejected characters", func(t *testing.T) {
		for _, name := range []string{"/tmp/a*b.inc", "/tmp/a?b.inc", "/tmp/a,b.inc", "/tmp/a[b.inc", "/tmp/a;b.inc"} {
			e := newEnv(t)
			_, err := e.read("include " + name)
			if err == nil {
				t.Fatalf("%s: expected an error", name)
			}
			requireRoute(t, err, "reader.readInclude->reader.readFile.ProfilePathInvalid")
		}
	})
	t.Run("a bare name with a rejected character", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read("include a*b.inc")
		requireRoute(t, err, "reader.readFile.ProfilePathInvalid")
	})
	t.Run("a cycle between two files", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("a.inc", "include b.inc\n")
		e.sysFile("b.inc", "include a.inc\n")
		_, err := e.read("include a.inc")
		requireRoute(t, err, "reader.readInclude->reader.readFile.IncludeCycle")
	})
	t.Run("a file that includes itself", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("self.profile", "nosound\ninclude self.profile\n")
		_, err := Read(context.Background(), p, e.opts())
		requireRoute(t, err, "reader.readInclude->reader.readFile.IncludeCycle")
	})
	t.Run("a cycle ended by an ignore is not a cycle", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("a.inc", "ignore include a.inc\nnosound\ninclude a.inc\n")
		prof := e.mustRead("include a.inc")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		if len(prof.Ignored) != 1 || prof.Ignored[0].Line.Text != "include a.inc" {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
	})
	t.Run("a chain at the include limit is read", func(t *testing.T) {
		e := newEnv(t)
		// top.profile is level 0 and f1.inc to f16.inc are levels 1 to 16.
		prof := e.mustRead(chain(e, 16)...)
		requireTexts(t, "Lines", prof.Lines, "nosound")
	})
	t.Run("a chain one past the include limit is an error", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read(chain(e, 17)...)
		if err == nil {
			t.Fatal("expected an error")
		}
		requireRoute(t, err, "reader.readInclude->reader.readFile.IncludeTooDeep")
	})
}

// chain writes f1.inc ... fN.inc, each including the next, the last holding "nosound",
// and returns the lines of the top profile that include f1.inc.
func chain(e *env, n int) []string {
	for i := 1; i <= n; i++ {
		content := "nosound\n"
		if i < n {
			content = fmt.Sprintf("include f%d.inc\n", i+1)
		}
		e.sysFile(fmt.Sprintf("f%d.inc", i), content)
	}
	return []string{"include f1.inc"}
}

func TestReadIgnore(t *testing.T) {
	t.Run("an ignore acts on the lines after it only", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("nosound", "ignore nosound", "nosound", "notv")
		requireTexts(t, "Lines", prof.Lines, "nosound", "notv")
		if len(prof.Ignored) != 1 || prof.Ignored[0].Line.Number != 3 || prof.Ignored[0].By.Number != 2 {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
	})
	t.Run("whitelist skips every whitelist but not whitelist-ro", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore whitelist", "whitelist /a", "whitelist ${HOME}/b", "nowhitelist /c", "whitelist-ro /d", "whitelist-roo /e")
		requireTexts(t, "Lines", prof.Lines, "nowhitelist /c", "whitelist /d", "read-only /d", "whitelist-roo /e")
		if len(prof.Ignored) != 2 {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
	})
	t.Run("the Code and Code - OSS case", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead(
			"ignore whitelist ${HOME}/.config/Code",
			"whitelist ${HOME}/.config/Code",
			"whitelist ${HOME}/.config/Code - OSS",
			"whitelist ${HOME}/.config/Code2",
			"whitelist ${HOME}/.config/Co",
		)
		requireTexts(t, "Lines", prof.Lines, "whitelist ${HOME}/.config/Code2", "whitelist ${HOME}/.config/Co")
		requireTexts(t, "Ignored", linesOf(prof.Ignored), "whitelist ${HOME}/.config/Code", "whitelist ${HOME}/.config/Code - OSS")
	})
	t.Run("macros are not expanded before the match", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore whitelist ${HOME}/x", "whitelist "+e.home+"/x", "whitelist ${HOME}/x")
		requireTexts(t, "Lines", prof.Lines, "whitelist "+e.home+"/x")
	})
	t.Run("the text is matched after normalization", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore   net \t none  # no network", "net    none", "net none")
		requireTexts(t, "Lines", prof.Lines)
		if len(prof.Ignored) != 2 || prof.Ignores[0].Text != "ignore net none" {
			t.Errorf("Ignored = %+v Ignores = %+v", prof.Ignored, prof.Ignores)
		}
	})
	t.Run("an ignore line can be ignored", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore ignore foo", "ignore foo", "foo bar", "ignore foo")
		requireTexts(t, "Lines", prof.Lines, "foo bar")
		requireTexts(t, "Ignores", prof.Ignores, "ignore ignore foo")
		requireTexts(t, "Ignored", linesOf(prof.Ignored), "ignore foo", "ignore foo")
	})
	t.Run("an ignore entry skips the first entry that matches", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore net", "ignore net none", "net none")
		if len(prof.Ignored) != 1 {
			t.Fatalf("Ignored = %+v", prof.Ignored)
		}
		// "ignore net none" itself matches the first entry "net", so it is skipped, and
		// the first entry is the one that matches the line.
		if got := prof.Ignored[0].By.Text; got != "ignore net" {
			t.Errorf("By = %q", got)
		}
	})
	t.Run("a bare ignore and a bare include are plain lines", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore", "include", "ignore  ")
		requireTexts(t, "Lines", prof.Lines, "ignore", "include", "ignore")
		if len(prof.Ignores) != 0 || len(prof.Includes) != 0 {
			t.Errorf("Ignores = %v Includes = %v", prof.Ignores, prof.Includes)
		}
	})
	t.Run("an ignored include is not read and needs no file", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore include nothing.inc", "include nothing.inc", "nosound")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		if len(prof.Includes) != 0 {
			t.Errorf("Includes = %+v", prof.Includes)
		}
		requireTexts(t, "Ignored", linesOf(prof.Ignored), "include nothing.inc")
	})
	t.Run("ignore include matches by the text as written", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("x.inc", "nosound\n")
		prof := e.mustRead("ignore include ${CFG}/x.inc", "include x.inc", "include ${CFG}/x.inc")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		if len(prof.Includes) != 1 || prof.Includes[0].Line.Text != "include x.inc" {
			t.Errorf("Includes = %+v", prof.Includes)
		}
	})
	t.Run("an ignore in an included file reaches the lines after the include", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("x.inc", "ignore nosound\n")
		prof := e.mustRead("include x.inc", "nosound", "notv")
		requireTexts(t, "Lines", prof.Lines, "notv")
	})
	t.Run("a command line entry ignores the lines and counts", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("top.profile", "nosound\nnotv\n")
		opts := e.opts()
		opts.Cmdline = []string{"nosound"}
		prof, err := Read(context.Background(), p, opts)
		if err != nil {
			t.Fatal(err)
		}
		requireTexts(t, "Lines", prof.Lines, "notv")
		if len(prof.Ignored) != 1 || prof.Ignored[0].By.File != "" || prof.Ignored[0].By.Number != 0 || prof.Ignored[0].By.Text != "ignore nosound" {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
		if len(prof.Ignores) != 0 {
			t.Errorf("Ignores = %+v, want none: the entry came from the command line", prof.Ignores)
		}
	})
}

func linesOf(ig []Ignored) []Line {
	var out []Line
	for _, i := range ig {
		out = append(out, i.Line)
	}
	return out
}

func TestReadIgnoreLimit(t *testing.T) {
	ignoreLines := func(n int) []string {
		var out []string
		for i := 1; i <= n; i++ {
			out = append(out, fmt.Sprintf("ignore cmd%d", i))
		}
		return out
	}
	t.Run("32 entries are accepted", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead(ignoreLines(32)...)
		if len(prof.Ignores) != 32 {
			t.Errorf("len(Ignores) = %d", len(prof.Ignores))
		}
	})
	t.Run("33 entries are an error", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read(ignoreLines(33)...)
		if err == nil {
			t.Fatal("expected an error")
		}
		requireRoute(t, err, "Read->reader.readFile->reader.readLine->reader.checkLine->reader.addIgnore.TooManyIgnores")
		appErr, _ := errors.FromError(err)
		if appErr.Metadata["entry"] != "cmd33" || appErr.Metadata["line"] != 33 {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
	})
	t.Run("the same entry twice counts twice", func(t *testing.T) {
		e := newEnv(t)
		lines := slices.Repeat([]string{"ignore nosound"}, 33)
		_, err := e.read(lines...)
		requireRoute(t, err, "reader.addIgnore.TooManyIgnores")
	})
	t.Run("an ignore line in an included file counts", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("x.inc", strings.Join(ignoreLines(32), "\n")+"\n")
		_, err := e.read("include x.inc", "ignore one-more")
		requireRoute(t, err, "reader.addIgnore.TooManyIgnores")
	})
	t.Run("command line entries count", func(t *testing.T) {
		for _, tt := range []struct {
			cmdline, lines int
			ok             bool
		}{{0, 32, true}, {2, 30, true}, {30, 2, true}, {32, 0, true}, {2, 31, false}, {32, 1, false}, {33, 0, false}} {
			e := newEnv(t)
			opts := e.opts()
			for i := 0; i < tt.cmdline; i++ {
				opts.Cmdline = append(opts.Cmdline, fmt.Sprintf("cli%d", i))
			}
			p := e.sysFile("top.profile", strings.Join(ignoreLines(tt.lines), "\n")+"\n")
			_, err := Read(context.Background(), p, opts)
			if tt.ok && err != nil {
				t.Errorf("cmdline=%d lines=%d: %v", tt.cmdline, tt.lines, err)
			}
			if !tt.ok {
				if err == nil {
					t.Errorf("cmdline=%d lines=%d: expected an error", tt.cmdline, tt.lines)
				} else {
					requireRoute(t, err, "addIgnore.TooManyIgnores")
				}
			}
		}
	})
	t.Run("an empty command line entry is an error", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("top.profile", "nosound\n")
		opts := e.opts()
		opts.Cmdline = []string{"ok", ""}
		_, err := Read(context.Background(), p, opts)
		requireRoute(t, err, "Read->reader.addIgnore.IgnoreEmpty")
	})
}

func TestReadQuiet(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		quiet bool
		want  []string
	}{
		{"no quiet", []string{"nosound"}, false, []string{"nosound"}},
		{"quiet", []string{"quiet", "nosound"}, true, []string{"nosound"}},
		{"quiet with blanks and a comment", []string{"  quiet  # shh", "nosound"}, true, []string{"nosound"}},
		{"ignore quiet first", []string{"ignore quiet", "quiet"}, false, nil},
		{"ignore quiet after", []string{"quiet", "ignore quiet"}, true, nil},
		{"quiet then ignored quiet", []string{"quiet", "ignore quiet", "quiet"}, false, nil},
		{"quiet with an argument is a plain line", []string{"quiet now"}, false, []string{"quiet now"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prof := newEnv(t).mustRead(tt.lines...)
			if prof.Quiet != tt.quiet {
				t.Errorf("Quiet = %v, want %v", prof.Quiet, tt.quiet)
			}
			requireTexts(t, "Lines", prof.Lines, tt.want...)
		})
	}
}

func TestReadDerivedLines(t *testing.T) {
	t.Run("whitelist-ro becomes two lines that no ignore reaches", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore whitelist-ro /a", "ignore read-only", "whitelist-ro /a", "whitelist-ro  ${HOME}/b")
		requireTexts(t, "Lines", prof.Lines, "whitelist /a", "read-only /a", "whitelist ${HOME}/b", "read-only ${HOME}/b")
		for i, l := range prof.Lines {
			wantOrigin := "whitelist-ro /a"
			if i >= 2 {
				wantOrigin = "whitelist-ro ${HOME}/b"
			}
			if l.Origin != wantOrigin || l.Number != 3+i/2 {
				t.Errorf("Lines[%d] = %+v", i, l)
			}
		}
		if len(prof.Ignored) != 0 {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
	})
	t.Run("noprinters adds two blacklist lines", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("noprinters", "ignore blacklist", "notv")
		requireTexts(t, "Lines", prof.Lines, "noprinters", "blacklist /dev/lp*", "blacklist /run/cups/cups.sock", "notv")
		if prof.Lines[0].Origin != "" || prof.Lines[1].Origin != "noprinters" || prof.Lines[2].Origin != "noprinters" {
			t.Errorf("Lines = %+v", prof.Lines)
		}
	})
	t.Run("an ignored noprinters adds nothing", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore noprinters", "noprinters")
		requireTexts(t, "Lines", prof.Lines)
	})
}

func TestReadAllowDebuggers(t *testing.T) {
	t.Run("disable-devel.inc is not read after allow-debuggers", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("disable-devel.inc", "blacklist /usr/bin/gdb\n")
		prof := e.mustRead("allow-debuggers", "include disable-devel.inc", "notv")
		requireTexts(t, "Lines", prof.Lines, "allow-debuggers", "notv")
		if len(prof.Includes) != 1 || prof.Includes[0].Skipped != "allow-debuggers" {
			t.Errorf("Includes = %+v", prof.Includes)
		}
	})
	t.Run("disable-devel.inc is read otherwise", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("disable-devel.inc", "blacklist /usr/bin/gdb\n")
		prof := e.mustRead("include disable-devel.inc", "allow-debuggers")
		requireTexts(t, "Lines", prof.Lines, "blacklist /usr/bin/gdb", "allow-debuggers")
	})
	t.Run("an ignored allow-debuggers does not count", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("disable-devel.inc", "blacklist /usr/bin/gdb\n")
		prof := e.mustRead("ignore allow-debuggers", "allow-debuggers", "include disable-devel.inc")
		requireTexts(t, "Lines", prof.Lines, "blacklist /usr/bin/gdb")
	})
	t.Run("other files are read", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("disable-exec.inc", "noexec /tmp\n")
		prof := e.mustRead("allow-debuggers", "include disable-exec.inc")
		requireTexts(t, "Lines", prof.Lines, "allow-debuggers", "noexec /tmp")
	})
}

func TestReadFileShape(t *testing.T) {
	t.Run("line numbers count blank and comment lines", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("# header", "", "   ", "nosound  # sound", "\t# indented", "notv")
		if len(prof.Lines) != 2 || prof.Lines[0].Number != 4 || prof.Lines[1].Number != 6 {
			t.Errorf("Lines = %+v", prof.Lines)
		}
	})
	t.Run("CRLF line ends", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("crlf.profile", "nosound\r\nnet  none\r\n")
		prof, err := Read(context.Background(), p, e.opts())
		if err != nil {
			t.Fatal(err)
		}
		requireTexts(t, "Lines", prof.Lines, "nosound", "net none")
	})
	t.Run("no newline at the end", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("nonl.profile", "nosound\nnotv")
		prof, err := Read(context.Background(), p, e.opts())
		if err != nil {
			t.Fatal(err)
		}
		requireTexts(t, "Lines", prof.Lines, "nosound", "notv")
	})
	t.Run("a long line counts as several lines", func(t *testing.T) {
		e := newEnv(t)
		// 8191 bytes: "env " plus filler, so fgets returns it without the newline; the
		// newline is then a line of its own, and the next line is number 3.
		long := "env " + strings.Repeat("a", 8191-len("env "))
		p := e.sysFile("long.profile", long+"\nnosound\n")
		prof, err := Read(context.Background(), p, e.opts())
		if err != nil {
			t.Fatal(err)
		}
		if len(prof.Lines) != 2 || prof.Lines[0].Number != 1 || prof.Lines[1].Number != 3 {
			t.Errorf("Lines = %d, numbers = %d %d", len(prof.Lines), prof.Lines[0].Number, prof.Lines[len(prof.Lines)-1].Number)
		}
	})
	t.Run("unknown commands are kept", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("frobnicate all the things", "warn careful", "keep-hostname")
		requireTexts(t, "Lines", prof.Lines, "frobnicate all the things", "warn careful", "keep-hostname")
		if l := prof.Lines[0]; l.Command != "frobnicate" || l.Args != "all the things" {
			t.Errorf("Lines[0] = %+v", l)
		}
	})
	t.Run("command and args", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("nosound", "net none", "noblacklist ${HOME}/.config/Code - OSS")
		want := [][2]string{{"nosound", ""}, {"net", "none"}, {"noblacklist", "${HOME}/.config/Code - OSS"}}
		for i, l := range prof.Lines {
			if l.Command != want[i][0] || l.Args != want[i][1] {
				t.Errorf("Lines[%d] = %+v", i, l)
			}
		}
	})
	t.Run("an empty file", func(t *testing.T) {
		e := newEnv(t)
		p := e.sysFile("empty.profile", "")
		prof, err := Read(context.Background(), p, e.opts())
		if err != nil {
			t.Fatal(err)
		}
		if len(prof.Lines) != 0 {
			t.Errorf("Lines = %+v", prof.Lines)
		}
	})
}

func TestReadCanceled(t *testing.T) {
	e := newEnv(t)
	p := e.sysFile("top.profile", "nosound\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Read(ctx, p, e.opts())
	if err == nil {
		t.Fatal("expected an error")
	}
	requireRoute(t, err, "Read->reader.readFile.ProfileReadCanceled")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false for %v", err)
	}
}

func TestReadCanceledBetweenFiles(t *testing.T) {
	e := newEnv(t)
	e.sysFile("a.inc", "nosound\n")
	p := e.sysFile("top.profile", "include a.inc\n")
	ctx, cancel := context.WithCancel(context.Background())
	// The context reports canceled from its second Err call on: the top-level file is
	// read, and the check before the included file fails.
	cc := &cancelAfter{Context: ctx, after: 1, cancel: cancel}
	_, err := Read(cc, p, e.opts())
	if err == nil {
		t.Fatal("expected an error")
	}
	requireRoute(t, err, "reader.readInclude->reader.readFile.ProfileReadCanceled")
}

// cancelAfter is a context whose Err turns to Canceled after a number of calls.
type cancelAfter struct {
	context.Context
	after  int
	cancel context.CancelFunc
	calls  int
}

func (c *cancelAfter) Err() error {
	c.calls++
	if c.calls > c.after {
		c.cancel()
	}
	return c.Context.Err()
}

func TestReadConditionals(t *testing.T) {
	type want struct {
		lines   []string
		skipped []string
	}
	tests := []struct {
		name  string
		cfg   *Config
		lines []string
		want  want
	}{
		// HAS_APPIMAGE
		{"appimage is never true", nil, []string{"?HAS_APPIMAGE: nosound"}, want{nil, []string{"nosound"}}},
		// HAS_NET
		{"net before any net line", nil, []string{"?HAS_NET: notv"}, want{nil, []string{"notv"}}},
		{"net after net none", nil, []string{"net none", "?HAS_NET: notv"}, want{[]string{"net none", "notv"}, nil}},
		{"net after net device", nil, []string{"net eth0", "?HAS_NET: notv"}, want{[]string{"net eth0", "notv"}, nil}},
		{"net after net none then a device", nil, []string{"net none", "net br0", "?HAS_NET: notv"}, want{[]string{"net none", "net br0", "notv"}, nil}},
		{"net after a device then net none", nil, []string{"net br0", "net none", "?HAS_NET: notv"}, want{[]string{"net br0", "net none", "notv"}, nil}},
		{"net is not set by netfilter or netns", nil, []string{"netfilter", "netns x", "netmask 1.2.3.4", "?HAS_NET: notv"}, want{[]string{"netfilter", "netns x", "netmask 1.2.3.4"}, []string{"notv"}}},
		{"net is not set by an ignored net none", nil, []string{"ignore net none", "net none", "?HAS_NET: notv"}, want{nil, []string{"notv"}}},
		{"net lo sets nothing", nil, []string{"net lo", "?HAS_NET: notv"}, want{[]string{"net lo"}, []string{"notv"}}},
		// HAS_NODBUS
		{"nodbus before", nil, []string{"?HAS_NODBUS: notv"}, want{nil, []string{"notv"}}},
		{"nodbus after nodbus", nil, []string{"nodbus", "?HAS_NODBUS: notv"}, want{[]string{"nodbus", "notv"}, nil}},
		{"nodbus after dbus-user none", nil, []string{"dbus-user none", "?HAS_NODBUS: notv"}, want{[]string{"dbus-user none", "notv"}, nil}},
		{"nodbus after dbus-user filter", nil, []string{"dbus-user filter", "?HAS_NODBUS: notv"}, want{[]string{"dbus-user filter", "notv"}, nil}},
		{"nodbus after dbus-system none", nil, []string{"dbus-system none", "?HAS_NODBUS: notv"}, want{[]string{"dbus-system none", "notv"}, nil}},
		{"nodbus after dbus-system filter", nil, []string{"dbus-system filter", "?HAS_NODBUS: notv"}, want{[]string{"dbus-system filter", "notv"}, nil}},
		{"nodbus is not set by a dbus-user.talk", nil, []string{"dbus-user.talk org.x", "dbus-system.own org.y", "?HAS_NODBUS: notv"}, want{[]string{"dbus-user.talk org.x", "dbus-system.own org.y"}, []string{"notv"}}},
		{"nodbus is not set by an ignored line", nil, []string{"ignore dbus-user none", "dbus-user none", "?HAS_NODBUS: notv"}, want{nil, []string{"notv"}}},
		// HAS_NOSOUND
		{"nosound before", nil, []string{"?HAS_NOSOUND: notv", "nosound"}, want{[]string{"nosound"}, []string{"notv"}}},
		{"nosound after", nil, []string{"nosound", "?HAS_NOSOUND: notv"}, want{[]string{"nosound", "notv"}, nil}},
		{"nosound is not set by an ignored nosound", nil, []string{"ignore nosound", "nosound", "?HAS_NOSOUND: notv"}, want{nil, []string{"notv"}}},
		// HAS_PRIVATE
		{"private before", nil, []string{"?HAS_PRIVATE: notv"}, want{nil, []string{"notv"}}},
		{"private after private", nil, []string{"private", "?HAS_PRIVATE: notv"}, want{[]string{"private", "notv"}, nil}},
		{"private after private dir", nil, []string{"private /tmp/x", "?HAS_PRIVATE: notv"}, want{[]string{"private /tmp/x", "notv"}, nil}},
		{"private after private-home", nil, []string{"private-home .mozilla,.config", "?HAS_PRIVATE: notv"}, want{[]string{"private-home .mozilla,.config", "notv"}, nil}},
		{"private is not set by the other private commands", nil, []string{"private-dev", "private-tmp", "private-etc a", "private-bin b", "private-cache", "private-cwd", "private-lib", "private-opt none", "private-srv none", "?HAS_PRIVATE: notv"}, want{[]string{"private-dev", "private-tmp", "private-etc a", "private-bin b", "private-cache", "private-cwd", "private-lib", "private-opt none", "private-srv none"}, []string{"notv"}}},
		// HAS_X11
		{"x11 before", nil, []string{"?HAS_X11: notv"}, want{nil, []string{"notv"}}},
		{"x11 after x11 none", nil, []string{"x11 none", "?HAS_X11: notv"}, want{[]string{"x11 none", "notv"}, nil}},
		{"x11 after x11 xorg", nil, []string{"x11 xorg", "?HAS_X11: notv"}, want{[]string{"x11 xorg", "notv"}, nil}},
		{"x11 is not set by the other x11 forms", nil, []string{"x11", "x11 xephyr", "x11 xpra", "x11 xvfb", "xephyr-screen 640x480", "?HAS_X11: notv"}, want{[]string{"x11", "x11 xephyr", "x11 xpra", "x11 xvfb", "xephyr-screen 640x480"}, []string{"notv"}}},
		// firejail.config
		{"u2f by default", nil, []string{"?BROWSER_DISABLE_U2F: nou2f"}, want{[]string{"nou2f"}, nil}},
		{"u2f turned off", &Config{}, []string{"?BROWSER_DISABLE_U2F: nou2f"}, want{nil, []string{"nou2f"}}},
		{"drm by default", nil, []string{"?BROWSER_ALLOW_DRM: ignore nou2f"}, want{nil, []string{"ignore nou2f"}}},
		{"drm turned on", &Config{BrowserAllowDRM: true}, []string{"?BROWSER_ALLOW_DRM: notv"}, want{[]string{"notv"}, nil}},
		{"tray by default", nil, []string{"?ALLOW_TRAY: notv"}, want{nil, []string{"notv"}}},
		{"tray turned on", &Config{AllowTray: true}, []string{"?ALLOW_TRAY: notv"}, want{[]string{"notv"}, nil}},
		// syntax that firejail accepts
		{"a space before the colon", nil, []string{"nosound", "?HAS_NOSOUND : notv"}, want{[]string{"nosound", "notv"}, nil}},
		{"no space after the colon", nil, []string{"nosound", "?HAS_NOSOUND:notv"}, want{[]string{"nosound", "notv"}, nil}},
		{"many blanks around", nil, []string{"nosound", "  ?HAS_NOSOUND \t :  \t notv  "}, want{[]string{"nosound", "notv"}, nil}},
		{"include and quiet are checked only when the condition is true", nil, []string{"?HAS_NET: include x.inc", "?HAS_NET: quiet"}, want{nil, []string{"include x.inc", "quiet"}}},
		{"a conditional with arguments", nil, []string{"nosound", "?HAS_NOSOUND: noblacklist ${HOME}/a - b"}, want{[]string{"nosound", "noblacklist ${HOME}/a - b"}, nil}},
		{"a conditional whitelist-ro is not rewritten", nil, []string{"nosound", "?HAS_NOSOUND: whitelist-ro /a"}, want{[]string{"nosound", "whitelist-ro /a"}, nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			if tt.cfg != nil {
				e.cfg = *tt.cfg
			}
			prof := e.mustRead(tt.lines...)
			// The text of a conditional line is the part after the colon.
			requireTexts(t, "Lines", prof.Lines, tt.want.lines...)
			var skipped []string
			for _, s := range prof.Skipped {
				skipped = append(skipped, s.Line.Text)
				if s.Reason == "" || s.Line.Conditional == "" || s.Line.File == "" || s.Line.Number == 0 {
					t.Errorf("Skipped entry incomplete: %+v", s)
				}
			}
			if !slices.Equal(skipped, tt.want.skipped) {
				t.Errorf("Skipped = %q, want %q", skipped, tt.want.skipped)
			}
		})
	}
}

func TestReadConditionalLineFields(t *testing.T) {
	e := newEnv(t)
	prof := e.mustRead("nosound", "net none", "?HAS_NOSOUND: ?HAS_NET: notv", "?HAS_NOSOUND: ?HAS_X11: nodvd", "?HAS_X11: nogroups")
	if len(prof.Lines) != 3 {
		t.Fatalf("Lines = %+v", prof.Lines)
	}
	got := prof.Lines[2]
	if got.Text != "notv" || got.Conditional != "HAS_NOSOUND,HAS_NET" || got.Number != 3 || got.Command != "notv" {
		t.Errorf("conditional line = %+v", got)
	}
	if len(prof.Skipped) != 2 {
		t.Fatalf("Skipped = %+v", prof.Skipped)
	}
	if s := prof.Skipped[0]; s.Line.Text != "nodvd" || s.Line.Conditional != "HAS_NOSOUND,HAS_X11" || s.Line.Number != 4 {
		t.Errorf("Skipped[0] = %+v", s)
	}
	if s := prof.Skipped[1]; s.Line.Text != "nogroups" || s.Line.Conditional != "HAS_X11" || s.Line.Number != 5 {
		t.Errorf("Skipped[1] = %+v", s)
	}
	if prof.Lines[0].Conditional != "" {
		t.Errorf("plain line has Conditional %q", prof.Lines[0].Conditional)
	}
}

func TestReadConditionalsAndIgnore(t *testing.T) {
	t.Run("the ignore list is checked on the text after the colon", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore notv", "nosound", "?HAS_NOSOUND: notv", "?HAS_NOSOUND: nodvd")
		requireTexts(t, "Lines", prof.Lines, "nosound", "nodvd")
		if len(prof.Ignored) != 1 || prof.Ignored[0].Line.Text != "notv" || prof.Ignored[0].Line.Conditional != "HAS_NOSOUND" || prof.Ignored[0].By.Text != "ignore notv" {
			t.Errorf("Ignored = %+v", prof.Ignored)
		}
	})
	t.Run("an entry with the conditional prefix matches nothing", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("ignore ?HAS_NOSOUND: notv", "nosound", "?HAS_NOSOUND: notv")
		requireTexts(t, "Lines", prof.Lines, "nosound", "notv")
	})
	t.Run("a conditional ignore line adds an entry", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("nosound", "?HAS_NOSOUND: ignore notv", "?HAS_X11: ignore nodvd", "notv", "nodvd")
		requireTexts(t, "Lines", prof.Lines, "nosound", "nodvd")
		if len(prof.Ignores) != 1 || prof.Ignores[0].Conditional != "HAS_NOSOUND" || prof.Ignores[0].Text != "ignore notv" {
			t.Errorf("Ignores = %+v", prof.Ignores)
		}
	})
	t.Run("a false conditional does not need a valid line", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("?HAS_NET: ?BOGUS: x", "nosound")
		requireTexts(t, "Lines", prof.Lines, "nosound")
	})
	t.Run("a true conditional checks the nested one", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read("nosound", "?HAS_NOSOUND: ?BOGUS: x")
		requireRoute(t, err, "reader.checkLine->reader.checkConditional.UnknownConditional")
	})
	t.Run("a nested include is not allowed", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.read("nosound", "?HAS_NOSOUND: ?HAS_NOSOUND: include x.inc")
		requireRoute(t, err, "reader.checkConditional.ConditionalForbidsCommand")
	})
}

func TestReadConditionalErrors(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		route string
	}{
		{"no colon", []string{"?HAS_NET net none"}, "Read->reader.readFile->reader.readLine->reader.checkLine->reader.checkConditional.ConditionalNoColon"},
		{"no colon and the name only", []string{"?HAS_NET"}, "reader.checkConditional.ConditionalNoColon"},
		{"a longer name", []string{"?HAS_NETX: nosound"}, "reader.checkConditional.ConditionalNoColon"},
		{"two spaces make one and then no colon", []string{"?HAS_NET  net none"}, "reader.checkConditional.ConditionalNoColon"},
		{"no line after the colon", []string{"?HAS_NET:"}, "reader.checkConditional.ConditionalNoLine"},
		{"no line after the colon and a blank", []string{"?HAS_NET: "}, "reader.checkConditional.ConditionalNoLine"},
		{"no line after the colon when true", []string{"nosound", "?HAS_NOSOUND:"}, "reader.checkConditional.ConditionalNoLine"},
		{"include when true", []string{"nosound", "?HAS_NOSOUND: include x.inc"}, "reader.checkConditional.ConditionalForbidsCommand"},
		{"quiet when true", []string{"nosound", "?HAS_NOSOUND: quiet"}, "reader.checkConditional.ConditionalForbidsCommand"},
		{"a word that starts with quiet when true", []string{"nosound", "?HAS_NOSOUND: quieter"}, "reader.checkConditional.ConditionalForbidsCommand"},
		{"unknown name", []string{"?HAS_FOO: nosound"}, "reader.checkConditional.UnknownConditional"},
		{"empty name", []string{"?: nosound"}, "reader.checkConditional.UnknownConditional"},
		{"just a question mark", []string{"?"}, "reader.checkConditional.UnknownConditional"},
		{"lower case", []string{"?has_net: nosound"}, "reader.checkConditional.UnknownConditional"},
		{"unknown name with no colon", []string{"?FOO"}, "reader.checkConditional.UnknownConditional"},
		{"unknown name is checked even in an ignored-looking line", []string{"ignore ?FOO: x", "?FOO: x"}, "reader.checkConditional.UnknownConditional"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newEnv(t).read(tt.lines...)
			if err == nil {
				t.Fatal("expected an error")
			}
			requireRoute(t, err, tt.route)
		})
	}
	t.Run("unknown conditional metadata", func(t *testing.T) {
		_, err := newEnv(t).read("nosound", "?HAS_FOO: nosound")
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("got %T", err)
		}
		if appErr.Metadata["conditional"] != "HAS_FOO" || appErr.Metadata["line"] != 2 {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
	})
	t.Run("in an included file the error names the file", func(t *testing.T) {
		e := newEnv(t)
		inc := e.sysFile("x.inc", "nosound\n?BOGUS: x\n")
		_, err := e.read("include x.inc")
		appErr, ok := errors.FromError(err)
		if !ok {
			t.Fatalf("got %T", err)
		}
		if appErr.Metadata["file"] != inc || appErr.Metadata["line"] != 2 {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
	})
}

func TestReadConditionalState(t *testing.T) {
	t.Run("the state is global and not per file", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("a.inc", "nosound\n")
		e.sysFile("b.inc", "?HAS_NOSOUND: notv\n")
		prof := e.mustRead("include a.inc", "include b.inc")
		requireTexts(t, "Lines", prof.Lines, "nosound", "notv")
	})
	t.Run("a line before the include does not see a later one", func(t *testing.T) {
		e := newEnv(t)
		e.sysFile("a.inc", "nosound\n")
		prof := e.mustRead("?HAS_NOSOUND: notv", "include a.inc")
		requireTexts(t, "Lines", prof.Lines, "nosound")
		requireTexts(t, "Skipped", linesOfSkipped(prof.Skipped), "notv")
	})
	t.Run("a line skipped by the ignore of its own file", func(t *testing.T) {
		e := newEnv(t)
		prof := e.mustRead("nosound", "ignore nosound", "nosound", "?HAS_NOSOUND: notv")
		requireTexts(t, "Lines", prof.Lines, "nosound", "notv")
	})
}

func linesOfSkipped(s []Skipped) []Line {
	var out []Line
	for _, i := range s {
		out = append(out, i.Line)
	}
	return out
}
