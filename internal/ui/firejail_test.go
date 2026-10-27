//go:build jailcat_firejail

package ui

// These tests build a sandbox through the backend of the window ([LocalBackend]: prepare,
// build, check, try, stop), with the real firejail, into a temporary home, and check what
// the installed launcher and the Try really do. They need /usr/bin/firejail (they skip
// without it) and must run outside any other sandbox: firejail then prints "an existing
// sandbox was detected" and runs the program unsandboxed, and these tests fail on that
// warning instead of passing. As in internal/install/firejail_test.go, no folder of a
// probe is under the temporary home (firejail takes "${HOME}" from the user's account), so
// the folders are below /tmp, beside it.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// realFirejail is the firejail these tests run.
const realFirejail = "/usr/bin/firejail"

// requireFirejail skips the test when firejail is not installed, and fails it when
// firejail runs inside another sandbox.
func requireFirejail(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(realFirejail); err != nil {
		t.Skipf("%s is not installed", realFirejail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, realFirejail, "--noprofile", "--", "/bin/true").CombinedOutput()
	if firejail.ContainsNestedSandboxWarning(string(out)) {
		t.Fatalf("firejail runs inside another sandbox and would not sandbox anything: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", out)
	}
	if err != nil {
		t.Fatalf("firejail does not start: %v\n%s", err, out)
	}
}

// firejailSystem is a backend over the real firejail with a home in /tmp.
type firejailSystem struct {
	root    string
	dirs    xdg.Dirs
	backend *LocalBackend
	app     apps.App
}

// newFirejailSystem makes the folders, the database and the backend. The app is /bin/sh.
func newFirejailSystem(t *testing.T) firejailSystem {
	t.Helper()
	requireFirejail(t)
	root, err := os.MkdirTemp("/tmp", "jailcat-ui-firejail-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	dirs, err := xdg.Resolve(xdg.NewEnv(home, map[string]string{"PATH": "/usr/bin:/bin", "XDG_DATA_DIRS": filepath.Join(root, "share")}))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(root, "jailcat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return firejailSystem{
		root: root, dirs: dirs,
		backend: NewLocalBackend(dirs, st, "", slog.New(slog.NewTextHandler(io.Discard, nil))),
		app:     apps.App{ID: "sh", Kind: apps.KindBinary, Name: "sh", Binary: "/bin/sh", BinaryName: "sh", Terminal: true},
	}
}

// editor opens a new build of the app on base None with the protections that would get in
// a probe's way switched off; edit changes the rest.
func (fs firejailSystem) editor(edit func(s *firejail.Settings)) *Editor {
	defaults := newBuildDefaults(fs.app, nil)
	e := NewEditor(fs.app, EditorOpts{
		Home: fs.dirs.Home, HomesDir: fs.dirs.Homes, Base: firejail.BaseNone(), Defaults: defaults,
	})
	e.Set(func(s *firejail.Settings) {
		s.Seccomp, s.DropCaps, s.NoNewPrivs, s.NoRoot, s.RestrictNamespaces = false, false, false, false, false
		s.Protocols = nil
		s.Quiet = true
		s.Icon = firejail.IconRandom
		s.InstallMenu = false
		s.InstallBin = true
		if edit != nil {
			edit(s)
		}
	})
	return e
}

// run starts the installed launcher with "-c script" and returns what the script printed
// as key=value lines. It fails if firejail reports an existing sandbox.
func runLauncher(t *testing.T, launcher, script string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, launcher, "-c", script)
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if firejail.ContainsNestedSandboxWarning(stdout.String() + stderr.String()) {
		t.Fatalf("firejail runs inside another sandbox: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", stderr.String())
	}
	if err != nil {
		t.Fatalf("the launcher failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	out := map[string]string{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

// TestFirejailBackendBuildsASandboxThatWorks builds the app through PrepareBuild and Build,
// then runs the installed launcher: the hidden folder is not visible, the sibling is, and
// the network has only the loopback interface.
func TestFirejailBackendBuildsASandboxThatWorks(t *testing.T) {
	fs := newFirejailSystem(t)
	ctx := context.Background()
	hidden, visible := filepath.Join(fs.root, "data", "hidden"), filepath.Join(fs.root, "data", "visible")
	writeFile(t, filepath.Join(hidden, "secret.txt"), "secret\n", 0o644)
	writeFile(t, filepath.Join(visible, "open.txt"), "open\n", 0o644)

	e := fs.editor(func(s *firejail.Settings) {
		s.Network = firejail.NetNone
		s.Folders = []firejail.Folder{{Path: hidden, Access: firejail.AccessHidden}}
	})
	prepared, err := fs.backend.PrepareBuild(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Conflicts) != 0 {
		t.Fatalf("conflicts: %+v", prepared.Conflicts)
	}
	res, err := fs.backend.Build(ctx, prepared, nil)
	if err != nil {
		t.Fatal(err)
	}
	if appErr, ok := errors.FromError(res.CheckErr); ok && appErr.Tag == firejail.TagNestedSandbox {
		t.Fatalf("firejail runs inside another sandbox: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", res.Check.Output)
	}
	if res.CheckErr != nil || !res.Check.OK {
		t.Fatalf("the check did not pass: %+v, %v", res.Check, res.CheckErr)
	}
	if res.Build.ID == 0 || !res.Build.Installed || res.Build.FirejailVersion == "" {
		t.Errorf("stored build = %+v", res.Build)
	}

	launcher := filepath.Join(fs.dirs.Bin, res.Build.SandboxName)
	profile, err := os.ReadFile(filepath.Join(fs.dirs.Profiles, res.Build.SandboxName+".profile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), "blacklist "+hidden+"\n") || !strings.Contains(string(profile), firejail.BuildLine(res.Build.ID)) {
		t.Fatalf("the profile is not the one of the build:\n%s", profile)
	}
	out := runLauncher(t, launcher, strings.Join([]string{
		"test -e " + shellWord(t, filepath.Join(hidden, "secret.txt")) + " && echo hidden-file=visible || echo hidden-file=missing",
		"test -e " + shellWord(t, filepath.Join(visible, "open.txt")) + " && echo control-file=visible || echo control-file=missing",
		"echo proc-net=$(sed 1,2d /proc/net/dev | cut -d: -f1 | tr -d ' ' | sort | tr '\\n' ',')",
	}, "\n"))
	for key, want := range map[string]string{"hidden-file": "missing", "control-file": "visible", "proc-net": "lo,"} {
		if got := out[key]; got != want {
			t.Errorf("%s = %q, want %q (probe output: %v)", key, got, want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(hidden, "secret.txt")); err != nil {
		t.Errorf("the host's file is gone: %v", err)
	}

	// A rebuild of the stored build, then an uninstall, leave no file behind.
	stored, err := fs.backend.store.GetBuild(ctx, res.Build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.backend.Uninstall(ctx, stored, UninstallChoice{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(launcher); !os.IsNotExist(err) {
		t.Errorf("the launcher is still there: %v", err)
	}
}

// shellWord returns s as a shell word.
func shellWord(t *testing.T, s string) string {
	t.Helper()
	q, err := firejail.ShellQuote(s)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// TestFirejailBackendTryAndStop runs the app in the sandbox with Try, reads its output,
// and stops it: nothing is installed or stored, and no sandbox is left in firejail's list.
func TestFirejailBackendTryAndStop(t *testing.T) {
	fs := newFirejailSystem(t)
	ctx := context.Background()
	e := fs.editor(func(s *firejail.Settings) {
		s.Network = firejail.NetNone
		s.ExtraArgs = []string{"-c", "echo hello-from-try; exec sleep 60"}
	})
	session, err := fs.backend.Try(ctx, e.Request())
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			if err := session.Stop(context.Background()); err != nil {
				t.Errorf("Stop: %v", err)
			}
		}
	})

	deadline := time.After(time.Minute)
	seen := false
	for !seen {
		select {
		case line, ok := <-session.Lines():
			if !ok {
				t.Fatal("the session ended before it printed")
			}
			if firejail.ContainsNestedSandboxWarning(line) {
				t.Fatalf("firejail runs inside another sandbox: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", line)
			}
			seen = strings.Contains(line, "hello-from-try")
		case <-deadline:
			t.Fatal("no output from the sandbox")
		}
	}
	list, err := exec.CommandContext(ctx, realFirejail, "--list").CombinedOutput()
	if err != nil || !strings.Contains(string(list), firejail.TrySandboxName("firejail-sh")) {
		t.Errorf("the sandbox is not in firejail's list: %v\n%s", err, list)
	}

	if err := session.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stopped = true
	status, err := session.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if status.Success() {
		t.Logf("the sandbox exited by itself: %v", status)
	}
	list, err = exec.CommandContext(ctx, realFirejail, "--list").CombinedOutput()
	if err != nil || strings.Contains(string(list), "jailcat-try-") {
		t.Errorf("a sandbox is left in firejail's list: %v\n%s", err, list)
	}

	builds, err := fs.backend.store.ListBuilds(ctx)
	if err != nil || len(builds) != 0 {
		t.Errorf("a Try stored builds: %v, %v", builds, err)
	}
	if _, err := os.Stat(filepath.Join(fs.dirs.Bin, "firejail-sh")); !os.IsNotExist(err) {
		t.Errorf("a Try installed the launcher: %v", err)
	}
}
