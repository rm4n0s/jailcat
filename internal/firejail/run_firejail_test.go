//go:build jailcat_firejail

package firejail

// These tests run the real firejail through Runner. They skip without /usr/bin/firejail
// and fail, instead of passing, when firejail runs inside another sandbox ("an existing
// sandbox was detected"): they must be run outside Claude Code's sandbox.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// realFirejail is the firejail these tests run.
const realFirejail = "/usr/bin/firejail"

// requireRealFirejail skips the test when firejail is not installed and fails it when
// firejail runs inside another sandbox.
func requireRealFirejail(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(realFirejail); err != nil {
		t.Skipf("%s is not installed", realFirejail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// No --quiet: firejail only prints its warning without it.
	out, err := exec.CommandContext(ctx, realFirejail, "--noprofile", "--", "/bin/true").CombinedOutput()
	if ContainsNestedSandboxWarning(string(out)) {
		t.Fatalf("firejail runs inside another sandbox and would not sandbox anything: run the jailcat_firejail tests outside Claude Code's sandbox\n%s", out)
	}
	if err != nil {
		t.Fatalf("firejail does not start: %v\n%s", err, out)
	}
}

// sandboxListed reports whether "firejail --list" shows a sandbox with the name. A line
// of the list is "<pid>:<user>:<name>:<command>".
func sandboxListed(t *testing.T, name string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, realFirejail, "--list").Output()
	if err != nil {
		t.Fatalf("firejail --list: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ":"+name+":") {
			return true
		}
	}
	return false
}

// waitFor polls cond every 100 ms until it is true or the time is up.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Try starts a sandbox that "firejail --list" shows under jailcat-try-<name>, and Stop
// ends it.
//
// Probe: the program is "sleep 300", which only ends when the sandbox is shut down. The
// name is unique to the run, so another sandbox of the machine cannot be mistaken for
// this one.
func TestFirejailTryStartsAndStopEndsTheSandbox(t *testing.T) {
	requireRealFirejail(t)
	name := "try-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	sandbox := TrySandboxName(name)
	cache := filepath.Join(t.TempDir(), "cache")

	runner := Runner{Firejail: realFirejail}
	s, err := runner.Try(context.Background(), TryRequest{
		Name:        name,
		ProfileText: []byte("quiet\n"),
		Binary:      "/bin/sleep",
		Args:        []string{"300"},
		CacheDir:    cache,
	})
	if err != nil {
		t.Fatalf("Try: %v", err)
	}
	t.Cleanup(func() {
		// Whatever the test did, no sandbox of this run is left.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			t.Errorf("Stop in cleanup: %v", err)
		}
		if _, err := s.Wait(); err != nil {
			t.Errorf("Wait in cleanup: %v", err)
		}
		if sandboxListed(t, sandbox) {
			t.Errorf("the sandbox %s is still listed", sandbox)
		}
	})

	// Reading the lines shows firejail's own messages in the failure text; the goroutine
	// ends when the session does.
	var mu sync.Mutex
	var seen []string
	go func() {
		for line := range s.Lines() {
			mu.Lock()
			seen = append(seen, line)
			mu.Unlock()
		}
	}()

	listed := waitFor(20*time.Second, func() bool {
		select {
		case <-s.done:
			return true
		default:
			return sandboxListed(t, sandbox)
		}
	})
	select {
	case <-s.done:
		status, werr := s.Wait()
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("the sandbox ended by itself: %v, %v; firejail said %q", status, werr, seen)
	default:
	}
	if !listed {
		t.Fatalf("firejail --list does not show %s", sandbox)
	}
	if s.Nested() {
		t.Fatal("firejail runs inside another sandbox: run the jailcat_firejail tests outside Claude Code's sandbox")
	}
	profile, err := os.ReadFile(filepath.Join(cache, "try", name+".profile"))
	if err != nil || string(profile) != "quiet\n" {
		t.Errorf("profile = %q, %v", profile, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := s.Wait(); err != nil {
		t.Errorf("Wait: %v", err)
	}
	if !waitFor(10*time.Second, func() bool { return !sandboxListed(t, sandbox) }) {
		t.Errorf("the sandbox %s is still listed after Stop", sandbox)
	}
}

// Try with a profile that restricts something shows the program's output through Lines.
//
// Probe: "net none" and a program that prints the interfaces of its network namespace.
func TestFirejailTryStreamsTheProgramsOutput(t *testing.T) {
	requireRealFirejail(t)
	name := "try-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not installed")
	}
	s, err := Runner{Firejail: realFirejail}.Try(context.Background(), TryRequest{
		Name:        name,
		ProfileText: []byte("quiet\nnet none\n"),
		Binary:      sh,
		Args:        []string{"-c", "echo interfaces=$(sed 1,2d /proc/net/dev | cut -d: -f1 | tr -d ' ' | tr '\\n' ',')"},
		CacheDir:    filepath.Join(t.TempDir(), "cache"),
	})
	if err != nil {
		t.Fatalf("Try: %v", err)
	}
	var lines []string
	for line := range s.Lines() {
		lines = append(lines, line)
	}
	status, werr := s.Wait()
	if werr != nil {
		t.Fatalf("Wait: %v (firejail said %q)", werr, lines)
	}
	if !status.Success() {
		t.Errorf("status = %v; firejail said %q", status, lines)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "interfaces=lo,") {
		t.Errorf("want the interfaces \"lo,\" in the output, got %q", lines)
	}
}

// Check passes for a profile that firejail accepts, and reports failure, with firejail's
// words, for one it rejects.
func TestFirejailCheck(t *testing.T) {
	requireRealFirejail(t)
	dir := t.TempDir()
	tests := []struct {
		name    string
		profile string
		wantOK  bool
	}{
		{"good profile", "quiet\nnet none\n", true},
		{"unknown command", "quiet\nthis-is-not-a-firejail-command\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".profile")
			if err := os.WriteFile(path, []byte(tc.profile), 0o600); err != nil {
				t.Fatal(err)
			}
			res, err := Runner{Firejail: realFirejail}.Check(context.Background(), path, CheckOpts{})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if res.OK != tc.wantOK {
				t.Errorf("OK = %v (status %v), want %v; output %q", res.OK, res.Status, tc.wantOK, res.Output)
			}
			if !tc.wantOK && strings.TrimSpace(res.Output) == "" {
				t.Error("a failed check has no output")
			}
		})
	}
}

// A build that limits the programs to "sh" has no "true" in its sandbox, so its check
// fails; with IgnorePrivateBin the check passes.
func TestFirejailCheckIgnoresPrivateBin(t *testing.T) {
	requireRealFirejail(t)
	path := filepath.Join(t.TempDir(), "private-bin.profile")
	if err := os.WriteFile(path, []byte("quiet\nprivate-bin sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Firejail: realFirejail}
	plain, err := runner.Check(context.Background(), path, CheckOpts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if plain.OK {
		t.Logf("without the ignore the check passes on this machine (output %q)", plain.Output)
	}
	res, err := runner.Check(context.Background(), path, CheckOpts{IgnorePrivateBin: true, IgnoreCount: 0})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.OK || res.Skipped {
		t.Errorf("Check with IgnorePrivateBin = %+v", res)
	}
}
