//go:build jailcat_firejail

package install

// These tests build sandboxes into a temporary home and run the installed launcher with
// the real firejail, with a small probe command in place of the app, and check what the
// sandbox does. They need /usr/bin/firejail (they skip without it) and must run outside
// any other sandbox: firejail then prints "an existing sandbox was detected" and runs the
// probe unsandboxed, and these tests fail on that warning instead of passing.
//
// Two facts of firejail shape every probe:
//
//   - firejail takes "${HOME}" and the home it hides or replaces from the user's account,
//     never from $HOME or from Jailcat's temporary home. So no probe names a folder under
//     the temporary home in a profile; the profile is generated with a home that nothing
//     is under, and the folders it names are absolute paths below /tmp. (The one
//     exception, "Only the folders I choose", names a folder of the real home that does
//     not exist, and never writes there.)
//   - the profiles here have base None (or a small base profile in the temporary folder),
//     so they hide nothing but what the case asks for: /tmp is the host's /tmp, and the
//     folders of a probe are visible inside the sandbox unless the profile changes that.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// realFirejail is the firejail these tests run.
const realFirejail = "/usr/bin/firejail"

// noHome is the home folder the profiles are generated with. No folder of a probe is
// under it, so no path of a profile is written with ${HOME}, which firejail would read as
// the real home.
const noHome = "/jailcat-test-no-such-home"

// requireFirejail skips the test when firejail is not installed, and fails it when
// firejail runs inside another sandbox.
func requireFirejail(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(realFirejail); err != nil {
		t.Skipf("%s is not installed", realFirejail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// No --quiet: firejail only prints its warning without it.
	out, err := exec.CommandContext(ctx, realFirejail, "--noprofile", "--", "/bin/true").CombinedOutput()
	if firejail.ContainsNestedSandboxWarning(string(out)) {
		t.Fatalf("firejail runs inside another sandbox and would not sandbox anything: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", out)
	}
	if err != nil {
		t.Fatalf("firejail does not start: %v\n%s", err, out)
	}
}

// lab is a temporary folder with a home for Jailcat's files in it and the folders of
// the probes beside it. The folder is below /tmp, because the "mkdir" and "mkfile"
// commands of a profile and the folders the probes whitelist have to be.
type lab struct {
	t    *testing.T
	root string
	home string
	sys  string
	dirs xdg.Dirs
	// profileHome is the home the profiles are generated with: [noHome], except in the
	// case that needs a folder under the real home.
	profileHome string
	// builds counts the builds, to give each its own id and name.
	builds int
}

// newLab makes the folders of a test: <root>/home is the home Jailcat installs into,
// <root>/data holds the folders of the probes.
func newLab(t *testing.T) *lab {
	t.Helper()
	requireFirejail(t)
	root := t.TempDir()
	// firejail rejects some characters in paths, and mkdir/whitelist work below /tmp only.
	if !strings.HasPrefix(root, "/tmp/") || strings.ContainsAny(root, " \t\"'\\$`") {
		var err error
		if root, err = os.MkdirTemp("/tmp", "jailcat-firejail-"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(root); err != nil {
				t.Error(err)
			}
		})
	}
	l := &lab{t: t, root: root, home: filepath.Join(root, "home"), sys: filepath.Join(root, "etc-firejail"), profileHome: noHome}
	for _, d := range []string{l.home, l.sys, l.data("")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := xdg.Resolve(xdg.NewEnv(l.home, map[string]string{
		"XDG_DATA_DIRS": filepath.Join(root, "share"),
		"PATH":          "/usr/bin:/bin",
	}))
	if err != nil {
		t.Fatal(err)
	}
	l.dirs = dirs
	return l
}

// data returns a path below the folder of the probes.
func (l *lab) data(parts ...string) string {
	return filepath.Join(append([]string{l.root, "data"}, parts...)...)
}

// baseProfile writes a base profile of the test and returns its path.
func (l *lab) baseProfile(name string, lines ...string) string {
	l.t.Helper()
	path := filepath.Join(l.root, "base", name)
	writeText(l.t, path, strings.Join(lines, "\n")+"\n")
	return path
}

// build generates, prepares and installs a build whose app is /bin/sh and returns the
// installed launcher and the files. base is the path of a base profile, or "" for base
// None. edit changes the settings, which start with every optional protection off, so a
// probe sees the effect of its own lines only.
func (l *lab) build(base string, edit func(s *firejail.Settings)) (launcher string, files []File) {
	l.t.Helper()
	app := apps.App{ID: "sh", Kind: apps.KindBinary, Name: "sh", Binary: "/bin/sh", BinaryName: "sh", Terminal: true}
	return l.buildApp(app, base, edit)
}

// buildApp is build for another app.
func (l *lab) buildApp(app apps.App, base string, edit func(s *firejail.Settings)) (launcher string, files []File) {
	l.t.Helper()
	ctx := context.Background()
	l.builds++
	name := "firejail-probe-" + string(rune('a'+l.builds-1))

	baseValues := firejail.BaseNone()
	var prof *firejail.Profile
	if base != "" {
		var err error
		prof, err = firejail.Read(ctx, base, firejail.ReadOpts{
			Dirs:   xdg.Dirs{Home: l.profileHome, FirejailSystem: l.sys, FirejailUser: filepath.Join(l.root, "user-firejail")},
			Config: firejail.DefaultConfig(),
		})
		if err != nil {
			l.t.Fatalf("Read(%s): %v", base, err)
		}
		baseValues = firejail.BaseValues(prof, l.profileHome)
	}
	baseValues = firejail.WithAppDefaults(baseValues, app.Binary)
	baseValues.Settings.SandboxName = name

	s := baseValues.Settings.Clone()
	s.Seccomp, s.DropCaps, s.NoNewPrivs, s.NoRoot, s.RestrictNamespaces = false, false, false, false, false
	s.Protocols = nil
	s.Quiet = true
	s.MenuName = name
	s.Icon = firejail.IconRandom
	s.IconSeed = 1
	s.InstallMenu = false
	s.InstallBin = true
	if edit != nil {
		edit(&s)
	}

	gen, err := firejail.Generate(firejail.ProfileSpec{
		BuildID:     int64(l.builds),
		BaseProfile: base,
		Profile:     prof,
		Base:        baseValues,
		Overrides:   firejail.Diff(baseValues.Settings, s),
		Home:        l.profileHome,
		VSCode:      firejail.NewVSCodeSpec(app.Binary, l.dirs.AppData, l.dirs.ConfigHome, name),
	})
	if err != nil {
		l.t.Fatalf("Generate: %v", err)
	}
	p, err := Prepare(ctx, l.dirs, Request{
		BuildID: int64(l.builds), App: app, Settings: s, Profile: gen, Firejail: realFirejail, IconTheme: "hicolor",
	})
	if err != nil {
		l.t.Fatalf("Prepare: %v", err)
	}
	files, err = Install(ctx, p, nil, nil)
	if err != nil {
		l.t.Fatalf("Install: %v", err)
	}
	for _, f := range files {
		if f.Kind == KindLauncher {
			launcher = f.Path
		}
	}
	if launcher == "" {
		l.t.Fatal("the build installed no launcher")
	}
	return launcher, files
}

// profileOf returns the text of the installed profile of a build.
func (l *lab) profileOf(files []File) string {
	l.t.Helper()
	for _, f := range files {
		if f.Kind == KindProfile {
			return readText(l.t, f.Path)
		}
	}
	l.t.Fatal("the build installed no profile")
	return ""
}

// run starts the installed launcher with "-c script", so the launcher's "$@" hands the
// script to /bin/sh inside the sandbox, and returns what the script printed as
// key=value lines. The launcher runs in its own process group under a time limit, from /
// (the folder it starts in may not exist in the sandbox). The test fails if firejail
// reports an existing sandbox.
func (l *lab) run(launcher, script string) map[string]string {
	l.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, launcher, "-c", script)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if firejail.ContainsNestedSandboxWarning(stdout.String() + stderr.String()) {
		l.t.Fatalf("firejail runs inside another sandbox: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", stderr.String())
	}
	if err != nil {
		l.t.Fatalf("the launcher failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	l.t.Logf("probe output:\n%sfirejail said:\n%s", stdout.String(), stderr.String())

	out := map[string]string{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

// shq returns s as a shell word.
func shq(t *testing.T, s string) string {
	t.Helper()
	q, err := firejail.ShellQuote(s)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// expectProbe fails the test unless out has key with the value.
func expectProbe(t *testing.T, out map[string]string, key, want string) {
	t.Helper()
	if got, ok := out[key]; !ok || got != want {
		t.Errorf("%s = %q (present: %v), want %q; probe output: %v", key, got, ok, want, out)
	}
}

// hostHas reports whether a path exists on the host.
func hostHas(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// A folder set to Hidden is not visible in the sandbox.
//
// Probe: the folder holds a file; "test -e" on the file must fail inside, while the same
// test on a file of a folder the profile does not mention (the control) succeeds. A
// hidden folder is replaced by one nobody can enter, so the test is on the file inside.
func TestFirejailHiddenFolderIsNotVisible(t *testing.T) {
	l := newLab(t)
	hidden, visible := l.data("hidden"), l.data("visible")
	writeText(t, filepath.Join(hidden, "secret.txt"), "secret\n")
	writeText(t, filepath.Join(visible, "open.txt"), "open\n")

	launcher, files := l.build("", func(s *firejail.Settings) {
		s.Folders = []firejail.Folder{{Path: hidden, Access: firejail.AccessHidden}}
	})
	if profile := l.profileOf(files); !strings.Contains(profile, "blacklist "+hidden+"\n") {
		t.Fatalf("the profile has no blacklist line:\n%s", profile)
	}
	out := l.run(launcher, strings.Join([]string{
		"test -e " + shq(t, filepath.Join(hidden, "secret.txt")) + " && echo hidden-file=visible || echo hidden-file=missing",
		"cat " + shq(t, filepath.Join(hidden, "secret.txt")) + " >/dev/null 2>&1 && echo hidden-read=yes || echo hidden-read=no",
		"test -e " + shq(t, filepath.Join(visible, "open.txt")) + " && echo control-file=visible || echo control-file=missing",
	}, "\n"))
	expectProbe(t, out, "hidden-file", "missing")
	expectProbe(t, out, "hidden-read", "no")
	expectProbe(t, out, "control-file", "visible")
	if !hostHas(filepath.Join(hidden, "secret.txt")) {
		t.Error("the host's file is gone")
	}
}

// A Read-only folder can be read and cannot be written; a Read-write folder (the
// control) can be written.
//
// Probe: read the file of the read-only folder, try to create a file in it, and try the
// same in the read-write folder. Afterwards the host shows which file was really made.
func TestFirejailReadOnlyFolderCannotBeWritten(t *testing.T) {
	l := newLab(t)
	ro, rw := l.data("ro"), l.data("rw")
	writeText(t, filepath.Join(ro, "file.txt"), "readable\n")
	if err := os.MkdirAll(rw, 0o755); err != nil {
		t.Fatal(err)
	}

	launcher, _ := l.build("", func(s *firejail.Settings) {
		s.Folders = []firejail.Folder{
			{Path: ro, Access: firejail.AccessReadOnly},
			{Path: rw, Access: firejail.AccessReadWrite},
		}
	})
	out := l.run(launcher, strings.Join([]string{
		"echo ro-read=$(cat " + shq(t, filepath.Join(ro, "file.txt")) + ")",
		"( : > " + shq(t, filepath.Join(ro, "new.txt")) + " ) 2>/dev/null && echo ro-write=yes || echo ro-write=no",
		"( : > " + shq(t, filepath.Join(rw, "new.txt")) + " ) 2>/dev/null && echo rw-write=yes || echo rw-write=no",
	}, "\n"))
	expectProbe(t, out, "ro-read", "readable")
	expectProbe(t, out, "ro-write", "no")
	expectProbe(t, out, "rw-write", "yes")
	if hostHas(filepath.Join(ro, "new.txt")) {
		t.Error("a file was made in the read-only folder")
	}
	if !hostHas(filepath.Join(rw, "new.txt")) {
		t.Error("the read-write folder was not written")
	}
}

// With "Only the folders I choose", a folder outside the list is missing and a folder in
// the list is there.
//
// Probe: the choice writes "whitelist" lines, and Jailcat refuses it unless one of them
// is under the home. The folders of the probe cannot be under the real home (see the top
// of the file; the test must not write there), so they are below /tmp, where a whitelist
// hides the rest of /tmp in the same way as one under the home hides the rest of the
// home: the listed folder is shown, its sibling is not. The one row under the real home
// names a folder that does not exist there, so it only satisfies the rule and shows
// nothing; the profile is generated with the real home for that. The home listing is
// logged, not asserted: firejail may put a few files of its own in the empty home.
func TestFirejailOnlyTheChosenFoldersAreThere(t *testing.T) {
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
	inHome := filepath.Join(realHome, ".jailcat-test-no-such-folder")
	listed, other := l.data("listed"), l.data("other")
	writeText(t, filepath.Join(listed, "file.txt"), "listed\n")
	writeText(t, filepath.Join(other, "file.txt"), "other\n")

	launcher, files := l.build("", func(s *firejail.Settings) {
		s.Home = firejail.HomeChosen
		s.Folders = []firejail.Folder{
			{Path: inHome, Access: firejail.AccessReadWrite},
			{Path: listed, Access: firejail.AccessReadWrite},
		}
	})
	profile := l.profileOf(files)
	for _, want := range []string{"\nwhitelist ${HOME}/.jailcat-test-no-such-folder\n", "\nwhitelist " + listed + "\n"} {
		if !strings.Contains(profile, want) {
			t.Fatalf("the profile lacks %q:\n%s", strings.TrimSpace(want), profile)
		}
	}
	out := l.run(launcher, strings.Join([]string{
		"test -e " + shq(t, filepath.Join(listed, "file.txt")) + " && echo listed=there || echo listed=missing",
		"test -e " + shq(t, filepath.Join(other, "file.txt")) + " && echo other=there || echo other=missing",
		`echo home-list=$(ls -A "$HOME" | sort | tr '\n' ',')`,
	}, "\n"))
	expectProbe(t, out, "listed", "there")
	expectProbe(t, out, "other", "missing")
}

// netInterfaces is the probe for the network: the interface names of the sandbox's own
// network namespace from /proc/net/dev (always the reader's namespace), and from
// /sys/class/net. Both are printed as comma-separated lists.
const netInterfaces = `echo proc-net=$(sed 1,2d /proc/net/dev | cut -d: -f1 | tr -d ' ' | sort | tr '\n' ',')
echo sys-net=$(ls /sys/class/net | sort | tr '\n' ',')
`

// "net none" leaves only the loopback interface.
func TestFirejailNetNoneLeavesOnlyLoopback(t *testing.T) {
	l := newLab(t)
	launcher, files := l.build("", func(s *firejail.Settings) { s.Network = firejail.NetNone })
	if profile := l.profileOf(files); !strings.Contains(profile, "\nnet none\n") {
		t.Fatalf("the profile has no net line:\n%s", profile)
	}
	out := l.run(launcher, netInterfaces)
	expectProbe(t, out, "proc-net", "lo,")
	expectProbe(t, out, "sys-net", "lo,")
}

// An "ignore" written before the "include" undoes a line of the base profile.
//
// Probe: the base profile has "net none". A build that keeps it has only "lo" (the
// control, which also shows that the base is read at all). A build that sets the network
// to Full writes "ignore net none" above the include, and its sandbox has the host's
// other interfaces. The case is skipped on a machine with no interface but "lo".
func TestFirejailIgnoreUndoesALineOfTheBase(t *testing.T) {
	host, err := os.ReadDir("/sys/class/net")
	if err != nil {
		t.Fatal(err)
	}
	var others []string
	for _, e := range host {
		if e.Name() != "lo" {
			others = append(others, e.Name())
		}
	}
	if len(others) == 0 {
		t.Skip("the machine has no network interface besides lo")
	}

	l := newLab(t)
	base := l.baseProfile("net-none.profile", "net none")

	keep, _ := l.build(base, nil)
	out := l.run(keep, netInterfaces)
	expectProbe(t, out, "proc-net", "lo,")

	undo, files := l.build(base, func(s *firejail.Settings) { s.Network = firejail.NetFull })
	profile := l.profileOf(files)
	ignoreAt, includeAt := strings.Index(profile, "\nignore net none\n"), strings.Index(profile, "\ninclude "+base+"\n")
	if ignoreAt < 0 || includeAt < 0 || ignoreAt > includeAt {
		t.Fatalf("want \"ignore net none\" before the include, got:\n%s", profile)
	}
	out = l.run(undo, netInterfaces)
	got := strings.Split(strings.TrimSuffix(out["proc-net"], ","), ",")
	if !slices.ContainsFunc(got, func(n string) bool { return n != "lo" && n != "" }) {
		t.Errorf("the sandbox has the interfaces %q, want one besides lo (the host has %v)", got, others)
	}
}

// With Own private home, $HOME inside the sandbox is the private folder, nothing of the
// real home is there, and the "mkdir" and "mkfile" lines of the base profile make
// nothing on the host.
//
// Probe, home: the private folder holds a marker file. Inside, "ls -A $HOME" must list
// it and nothing else but what firejail itself puts in a home ($HOME is the real
// user's home, whatever Jailcat's temporary home is, so a leak of the real home would
// show as extra entries); a file made in $HOME must appear in the private folder on the
// host; a marker in Jailcat's temporary home, outside the private folder, is not listed.
// Probe, mkdir: the base profile has "mkdir" and "mkfile" lines for paths below /tmp.
// The private-home build must not make them (it ends with "ignore mkdir" and "ignore
// mkfile"); a build of the same base with the full home does make them, which shows that
// the lines work at all.
func TestFirejailOwnPrivateHome(t *testing.T) {
	l := newLab(t)
	madeDir, madeFile := l.data("made-by-mkdir"), l.data("made-by-mkfile")
	base := l.baseProfile("mkdir.profile", "mkdir "+madeDir, "mkfile "+madeFile)
	writeText(t, filepath.Join(l.home, "outside-marker"), "outside\n")

	var private string
	launcher, files := l.build(base, func(s *firejail.Settings) {
		s.Home = firejail.HomePrivate
		s.PrivateDir = filepath.Join(l.dirs.Homes, s.SandboxName)
		private = s.PrivateDir
	})
	profile := l.profileOf(files)
	for _, want := range []string{"\nprivate " + private + "\n", "\nignore mkdir\n", "\nignore mkfile\n"} {
		if !strings.Contains(profile, want) {
			t.Fatalf("the profile lacks %q:\n%s", strings.TrimSpace(want), profile)
		}
	}
	if info, err := os.Stat(private); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("Install did not make the private home: %v %v", info, err)
	}
	writeText(t, filepath.Join(private, "inside-marker"), "inside\n")

	out := l.run(launcher, strings.Join([]string{
		`echo home-list=$(ls -A "$HOME" | sort | tr '\n' ',')`,
		`echo marker=$(cat "$HOME/inside-marker")`,
		`: > "$HOME/written-inside" && echo write=ok`,
	}, "\n"))
	expectProbe(t, out, "marker", "inside")
	expectProbe(t, out, "write", "ok")
	for _, name := range strings.Split(strings.TrimSuffix(out["home-list"], ","), ",") {
		switch name {
		case "inside-marker":
		case ".Xauthority", ".config":
			// Made by firejail in a private home (X authority, pulseaudio client.conf).
		case ".bashrc", ".zshrc", ".cshrc", ".inputrc":
			// Copied from /etc/skel or made by firejail in a fresh home (fs_home.c, skel).
		default:
			t.Errorf("$HOME holds %q, which is not in the private folder; listing: %q", name, out["home-list"])
		}
	}
	if strings.Contains(out["home-list"], "outside-marker") {
		t.Errorf("a file of the temporary home is in $HOME: %q", out["home-list"])
	}
	if !hostHas(filepath.Join(private, "written-inside")) {
		t.Error("the file made in $HOME is not in the private folder")
	}
	if hostHas(madeDir) || hostHas(madeFile) {
		t.Errorf("the mkdir/mkfile lines of the base ran: dir %v, file %v", hostHas(madeDir), hostHas(madeFile))
	}

	// Control: with the full home the same base does make them.
	full, _ := l.build(base, nil)
	l.run(full, "true")
	if !hostHas(madeDir) || !hostHas(madeFile) {
		t.Errorf("the control build did not run the mkdir/mkfile lines (dir %v, file %v): the probe proves nothing",
			hostHas(madeDir), hostHas(madeFile))
	}
}
