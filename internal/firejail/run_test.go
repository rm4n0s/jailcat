package firejail

import (
	"context"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
)

// nestedWarning is the line firejail prints when it runs inside another sandbox.
const nestedWarning = "Warning: an existing sandbox was detected. x will run without any additional sandboxing features"

// fakeFirejail is a shell script that stands in for firejail. A call with
// "--shutdown=..." is recorded in shutdown.args and kills the script of the running call
// (unless the file shutdown-ignored or shutdown-fails exists); any other call is
// recorded in run.args, writes its pid to the file pid and runs the body of the test.
type fakeFirejail struct {
	dir  string
	path string
	// sleepArg is the duration the fake sleeps for, unique to the test, so that the leak
	// check can find a "sleep" it left behind.
	sleepArg string
}

// newFake writes the fake into a temporary folder. In the body, SLEEPARG stands for the
// unique duration. When the test is over, no process may be left whose command line has
// the folder or the unique duration.
func newFake(t *testing.T, body string) *fakeFirejail {
	t.Helper()
	dir := t.TempDir()
	h := fnv.New32a()
	if _, err := h.Write([]byte(dir)); err != nil {
		t.Fatal(err)
	}
	f := &fakeFirejail{dir: dir, path: filepath.Join(dir, "firejail"), sleepArg: "300." + strconv.Itoa(int(h.Sum32()%900000)+100000)}
	q, err := ShellQuote(dir)
	if err != nil {
		t.Fatal(err)
	}
	script := "DIR=" + q + `
case "$1" in
--shutdown=*)
	printf '%s\n' "$@" > "$DIR/shutdown.args"
	if [ -f "$DIR/shutdown-fails" ]; then echo "Error: cannot find the sandbox" >&2; exit 1; fi
	if [ -f "$DIR/shutdown-ignored" ]; then exit 0; fi
	kill "$(cat "$DIR/pid")"
	exit 0
	;;
esac
printf '%s\n' "$@" > "$DIR/run.args"
echo $$ > "$DIR/pid"
` + strings.ReplaceAll(body, "SLEEPARG", f.sleepArg)
	if err := os.WriteFile(f.path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.requireNoProcesses(t) })
	return f
}

// requireNoProcesses fails the test when a process of the fake is still running after
// a short wait.
func (f *fakeFirejail) requireNoProcesses(t *testing.T) {
	t.Helper()
	pgrep, err := exec.LookPath("pgrep")
	if err != nil {
		t.Log("pgrep is not installed: the check for left-over processes is skipped")
		return
	}
	for _, pattern := range []string{f.dir, "sleep " + f.sleepArg} {
		deadline := time.Now().Add(3 * time.Second)
		for {
			out, err := exec.CommandContext(context.Background(), pgrep, "-af", "--", pattern).Output()
			if err != nil {
				break // pgrep exits 1 when nothing matches
			}
			if time.Now().After(deadline) {
				t.Errorf("processes left over for %q:\n%s", pattern, out)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// args returns the lines of a file of the fake that holds an argument list.
func (f *fakeFirejail) args(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// touch makes an empty file in the folder of the fake.
func (f *fakeFirejail) touch(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// serveBody runs until it is stopped: it prints "started", and ends with exit status 0
// when it gets a TERM (what "firejail --shutdown" sends), taking its sleep with it.
const serveBody = `sleep SLEEPARG &
child=$!
trap 'kill $child 2>/dev/null; exit 0' TERM
echo started
wait $child
`

// stubbornBody is serveBody that ignores TERM, so only a kill ends it.
const stubbornBody = `trap '' TERM
sleep SLEEPARG &
echo started
wait
`

// recv receives one line from ch, failing the test if none comes in time.
func recv(t *testing.T, ch <-chan string) (string, bool) {
	t.Helper()
	select {
	case line, ok := <-ch:
		return line, ok
	case <-time.After(10 * time.Second):
		t.Fatal("no line within 10 seconds")
		return "", false
	}
}

// drain reads ch until it is closed and returns the lines.
func drain(t *testing.T, ch <-chan string) []string {
	t.Helper()
	var out []string
	for {
		line, ok := recv(t, ch)
		if !ok {
			return out
		}
		out = append(out, line)
	}
}

// tryReq is a request for a build "my-app" in a temporary cache folder.
func tryReq(t *testing.T) TryRequest {
	t.Helper()
	return TryRequest{
		Name:        "my-app",
		ProfileText: []byte("quiet\nnet none\n"),
		Binary:      "/usr/bin/my-app",
		CacheDir:    filepath.Join(t.TempDir(), "cache"),
	}
}

func TestTryArguments(t *testing.T) {
	f := newFake(t, "echo hello\n")
	req := tryReq(t)
	req.Args = []string{"--flag", "a b", "$HOME", "it's", ""}
	req.PrivateHome = filepath.Join(t.TempDir(), "homes", "my-app")
	profile := filepath.Join(req.CacheDir, "try", "my-app.profile")

	s, err := Runner{Firejail: f.path}.Try(context.Background(), req)
	if err != nil {
		t.Fatalf("Try: %v", err)
	}
	lines := drain(t, s.Lines())
	status, err := s.Wait()
	if err != nil || !status.Success() {
		t.Fatalf("Wait = %v, %v", status, err)
	}

	if len(lines) != 1 || lines[0] != "hello" {
		t.Errorf("lines = %q", lines)
	}
	want := []string{"--profile=" + profile, "--name=jailcat-try-my-app", "--", "/usr/bin/my-app", "--flag", "a b", "$HOME", "it's", ""}
	got := f.args(t, "run.args")
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("arguments:\n got %q\nwant %q", got, want)
	}
	if s.SandboxName() != "jailcat-try-my-app" {
		t.Errorf("SandboxName = %q", s.SandboxName())
	}

	for _, c := range []struct {
		path string
		mode os.FileMode
	}{
		{profile, 0o600},
		{filepath.Dir(profile), 0o700},
		{req.PrivateHome, 0o700},
	} {
		info, err := os.Stat(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != c.mode {
			t.Errorf("%s has mode %v, want %v", c.path, info.Mode().Perm(), c.mode)
		}
	}
	text, err := os.ReadFile(profile)
	if err != nil || string(text) != "quiet\nnet none\n" {
		t.Errorf("profile = %q, %v", text, err)
	}
}

func TestTryReplacesTheProfileOfAnEarlierTry(t *testing.T) {
	f := newFake(t, "exit 0\n")
	req := tryReq(t)
	profile := filepath.Join(req.CacheDir, "try", "my-app.profile")
	if err := os.MkdirAll(filepath.Dir(profile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte("old profile that is longer than the new one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Runner{Firejail: f.path}.Try(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(profile)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(profile)
	if err != nil || string(text) != "quiet\nnet none\n" || info.Mode().Perm() != 0o600 {
		t.Errorf("profile = %q, mode %v, %v", text, info.Mode().Perm(), err)
	}
}

func TestTryKeepsAnExistingPrivateHome(t *testing.T) {
	f := newFake(t, "exit 0\n")
	req := tryReq(t)
	req.PrivateHome = filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(req.PrivateHome, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Runner{Firejail: f.path}.Try(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(req.PrivateHome)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("an existing home changed to %v", info.Mode().Perm())
	}
}

func TestTryStreamsLinesAsTheyCome(t *testing.T) {
	// "one" goes to standard output, "two" to standard error, and "two" is only
	// printed after the test has seen "one" while the program is still running.
	f := newFake(t, `echo one
while [ ! -f "$DIR/go" ]; do sleep 0.05; done
echo two >&2
exit 3
`)
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, ok := recv(t, s.Lines()); !ok || line != "one" {
		t.Fatalf("first line = %q, %v", line, ok)
	}
	select {
	case <-s.done:
		t.Fatal("the session ended before the program did")
	default:
	}
	f.touch(t, "go")
	if line, ok := recv(t, s.Lines()); !ok || line != "two" {
		t.Fatalf("second line = %q, %v", line, ok)
	}
	if _, ok := recv(t, s.Lines()); ok {
		t.Error("the channel was not closed at the end")
	}
	status, err := s.Wait()
	if err != nil || status != (ExitStatus{Code: 3}) {
		t.Errorf("Wait = %v, %v", status, err)
	}
	// Wait may be called again.
	if again, err := s.Wait(); err != nil || again != status {
		t.Errorf("second Wait = %v, %v", again, err)
	}
}

func TestTryLinesAreCleaned(t *testing.T) {
	f := newFake(t, `printf 'a\r\n\377b\n\nlast'`+"\n")
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	got := drain(t, s.Lines())
	want := []string{"a", "\uFFFDb", "", "last"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if _, err := s.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestTryNonZeroExit(t *testing.T) {
	f := newFake(t, "exit 7\n")
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Wait()
	if err != nil {
		t.Fatalf("a non-zero exit is not an error: %v", err)
	}
	if status != (ExitStatus{Code: 7}) || status.Success() || status.String() != "exit status 7" {
		t.Errorf("status = %+v (%s)", status, status)
	}
}

func TestTryDropsTheOldestLinesOfASlowReader(t *testing.T) {
	// Nobody reads the lines while the program runs: it must still end, and the reader
	// finds the newest lineBuffer lines.
	f := newFake(t, `i=0
while [ $i -lt 3000 ]; do echo "line $i"; i=$((i+1)); done
`)
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if status, err := s.Wait(); err != nil || !status.Success() {
		t.Fatalf("Wait = %v, %v", status, err)
	}
	got := drain(t, s.Lines())
	if len(got) != lineBuffer {
		t.Fatalf("%d lines kept, want %d", len(got), lineBuffer)
	}
	if got[0] != "line "+strconv.Itoa(3000-lineBuffer) || got[len(got)-1] != "line 2999" {
		t.Errorf("kept %q ... %q", got[0], got[len(got)-1])
	}
}

func TestTryStop(t *testing.T) {
	f := newFake(t, serveBody)
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	status, err := s.Wait()
	if err != nil || !status.Success() {
		t.Errorf("Wait = %v, %v", status, err)
	}
	if got := f.args(t, "shutdown.args"); len(got) != 1 || got[0] != "--shutdown=jailcat-try-my-app" {
		t.Errorf("shutdown arguments = %q", got)
	}
	if _, ok := recv(t, s.Lines()); ok {
		t.Error("the channel was not closed")
	}
	// Stopping a session that has ended does nothing.
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestTryStopKillsAProgramThatIgnoresTheShutdown(t *testing.T) {
	f := newFake(t, stubbornBody)
	f.touch(t, "shutdown-ignored")
	s, err := Runner{Firejail: f.path, grace: 200 * time.Millisecond}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	status, err := s.Wait()
	if err != nil || status.Signal != "killed" || status.Success() {
		t.Errorf("Wait = %+v, %v; want killed", status, err)
	}
}

func TestTryStopReportsAFailedShutdown(t *testing.T) {
	f := newFake(t, stubbornBody)
	f.touch(t, "shutdown-fails")
	s, err := Runner{Firejail: f.path, grace: 100 * time.Millisecond}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	err = s.Stop(context.Background())
	appErr := requireTagErr(t, err, tagTryShutdownFailed, "Session.Stop->Session.stop->Session.shutdown.TryShutdownFailed")
	if out, _ := appErr.Metadata["output"].(string); !strings.Contains(out, "cannot find the sandbox") {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	if appErr.Metadata["sandbox"] != "jailcat-try-my-app" {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	// The program was killed all the same.
	status, werr := s.Wait()
	if werr != nil || status.Signal != "killed" {
		t.Errorf("Wait = %+v, %v; want killed", status, werr)
	}
}

func TestTryStopWithACancelledContextKillsAtOnce(t *testing.T) {
	f := newFake(t, stubbornBody)
	f.touch(t, "shutdown-ignored")
	// A long grace period: only the cancelled context can end the wait early.
	s, err := Runner{Firejail: f.path, grace: time.Minute}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The shutdown cannot even start with this context, so Stop reports it.
	requireTag(t, s.Stop(ctx), tagTryShutdownFailed, "Session.Stop->Session.stop->Session.shutdown.TryShutdownFailed")
	status, werr := s.Wait()
	if werr != nil || status.Signal != "killed" {
		t.Errorf("Wait = %+v, %v; want killed", status, werr)
	}
}

func TestTryStopsItselfWhenTheContextIsCancelled(t *testing.T) {
	f := newFake(t, serveBody)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Runner{Firejail: f.path}.Try(ctx, tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	cancel()
	status, err := s.Wait()
	if err != nil || !status.Success() {
		t.Errorf("Wait = %+v, %v", status, err)
	}
	if got := f.args(t, "shutdown.args"); got[0] != "--shutdown=jailcat-try-my-app" {
		t.Errorf("shutdown arguments = %q", got)
	}
}

func TestTryKillsAStubbornProgramWhenTheContextIsCancelled(t *testing.T) {
	f := newFake(t, stubbornBody)
	f.touch(t, "shutdown-ignored")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Runner{Firejail: f.path, grace: 100 * time.Millisecond}.Try(ctx, tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := recv(t, s.Lines()); line != "started" {
		t.Fatalf("line = %q", line)
	}
	cancel()
	status, err := s.Wait()
	if err != nil || status.Signal != "killed" {
		t.Errorf("Wait = %+v, %v; want killed", status, err)
	}
}

func TestTryReportsANestedSandbox(t *testing.T) {
	f := newFake(t, "echo '"+nestedWarning+"'\nexit 0\n")
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := drain(t, s.Lines())
	if len(lines) != 1 || lines[0] != nestedWarning {
		t.Errorf("lines = %q", lines)
	}
	if !s.Nested() {
		t.Error("Nested is false")
	}
	status, err := s.Wait()
	requireTag(t, err, TagNestedSandbox, "Session.Wait.NestedSandbox")
	if !status.Success() {
		t.Errorf("the status comes with the error: %+v", status)
	}
}

func TestTryWithoutAWarningIsNotNested(t *testing.T) {
	f := newFake(t, "echo fine\n")
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s.Lines())
	if _, err := s.Wait(); err != nil || s.Nested() {
		t.Errorf("Wait error %v, Nested %v", err, s.Nested())
	}
}

func TestTryErrors(t *testing.T) {
	root := t.TempDir()
	aFile := filepath.Join(root, "a-file")
	if err := os.WriteFile(aFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fake := newFake(t, "exit 0\n")

	tests := []struct {
		name string
		edit func(r *TryRequest)
		run  Runner
		ctx  context.Context
		tag  string
	}{
		{"empty name", func(r *TryRequest) { r.Name = "" }, Runner{}, nil, tagTryNameInvalid},
		{"capital in name", func(r *TryRequest) { r.Name = "My-app" }, Runner{}, nil, tagTryNameInvalid},
		{"blank in name", func(r *TryRequest) { r.Name = "my app" }, Runner{}, nil, tagTryNameInvalid},
		{"slash in name", func(r *TryRequest) { r.Name = "../x" }, Runner{}, nil, tagTryNameInvalid},
		{"empty binary", func(r *TryRequest) { r.Binary = "" }, Runner{}, nil, tagTryBinaryInvalid},
		{"relative binary", func(r *TryRequest) { r.Binary = "bin/app" }, Runner{}, nil, tagTryBinaryInvalid},
		{"unclean binary", func(r *TryRequest) { r.Binary = "/usr/../bin/app" }, Runner{}, nil, tagTryBinaryInvalid},
		{"relative cache folder", func(r *TryRequest) { r.CacheDir = "cache" }, Runner{}, nil, tagTryCacheDirInvalid},
		{"cancelled context", func(r *TryRequest) {}, Runner{Firejail: fake.path}, cancelled, tagTryCanceled},
		{"cache folder below a file", func(r *TryRequest) { r.CacheDir = filepath.Join(aFile, "cache") }, Runner{}, nil, tagTryProfileWriteFailed},
		{"profile is a folder", func(r *TryRequest) {
			r.CacheDir = filepath.Join(root, "cache-with-folder")
			if err := os.MkdirAll(filepath.Join(r.CacheDir, "try", "my-app.profile"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, Runner{}, nil, tagTryProfileWriteFailed},
		{"home below a file", func(r *TryRequest) { r.PrivateHome = filepath.Join(aFile, "home") }, Runner{Firejail: fake.path}, nil, tagTryHomeCreateFailed},
		{"firejail missing", func(r *TryRequest) {}, Runner{Firejail: filepath.Join(root, "no-firejail")}, nil, tagTryStartFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tryReq(t)
			tc.edit(&req)
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			s, err := tc.run.Try(ctx, req)
			if s != nil {
				t.Errorf("a session came back with the error %v", err)
			}
			requireTag(t, err, tc.tag, "Runner.Try."+tc.tag)
		})
	}
}

func TestCheck(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "firejail-x.profile")
	trueBin := trueBinary()
	if !filepath.IsAbs(trueBin) || filepath.Base(trueBin) != "true" {
		t.Fatalf("trueBinary = %q", trueBin)
	}
	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus ExitStatus
		wantOutput string
	}{
		{"passes", "exit 0\n", true, ExitStatus{}, ""},
		{"fails with its output", "echo 'Error: bad line' >&2\necho more\nexit 1\n", false, ExitStatus{Code: 1}, "Error: bad line\nmore\n"},
		{"killed by a signal", "kill -TERM $$\nsleep 5\n", false, ExitStatus{Code: -1, Signal: "terminated"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, tc.body)
			res, err := Runner{Firejail: f.path}.Check(context.Background(), profile, CheckOpts{})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if res.OK != tc.wantOK || res.Status != tc.wantStatus || (tc.wantOutput != "" && res.Output != tc.wantOutput) {
				t.Errorf("Check = %+v", res)
			}
			want := []string{"--profile=" + profile, "--", trueBin}
			if got := f.args(t, "run.args"); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("arguments:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestCheckIgnoresPrivateBin(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "firejail-x.profile")
	f := newFake(t, "exit 0\n")
	res, err := Runner{Firejail: f.path}.Check(context.Background(), profile, CheckOpts{IgnorePrivateBin: true, IgnoreCount: 31})
	if err != nil || !res.OK || res.Skipped {
		t.Fatalf("Check = %+v, %v", res, err)
	}
	want := []string{"--ignore=private-bin", "--profile=" + profile, "--", trueBinary()}
	if got := f.args(t, "run.args"); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("arguments:\n got %q\nwant %q", got, want)
	}
}

func TestCheckIsSkippedAtTheIgnoreLimit(t *testing.T) {
	f := newFake(t, "exit 0\n")
	for _, count := range []int{32, 40} {
		res, err := Runner{Firejail: f.path}.Check(context.Background(), "/tmp/x.profile",
			CheckOpts{IgnorePrivateBin: true, IgnoreCount: count})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.OK || !res.Skipped || !strings.Contains(res.Reason, strconv.Itoa(count)) || !strings.Contains(res.Reason, "32") {
			t.Errorf("count %d: Check = %+v", count, res)
		}
		if _, err := os.Stat(filepath.Join(f.dir, "run.args")); err == nil {
			t.Error("firejail was run for a skipped check")
		}
	}
	// A build without private-bin needs no extra ignore, whatever its count.
	res, err := Runner{Firejail: f.path}.Check(context.Background(), "/tmp/x.profile", CheckOpts{IgnoreCount: 32})
	if err != nil || res.Skipped || !res.OK {
		t.Errorf("without IgnorePrivateBin: %+v, %v", res, err)
	}
}

func TestCheckNestedSandbox(t *testing.T) {
	f := newFake(t, "echo '"+nestedWarning+"'\nexit 0\n")
	res, err := Runner{Firejail: f.path}.Check(context.Background(), "/tmp/x.profile", CheckOpts{})
	appErr := requireTagErr(t, err, TagNestedSandbox, "Runner.Check.NestedSandbox")
	if out, _ := appErr.Metadata["output"].(string); !strings.Contains(out, "an existing sandbox") {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	if !res.OK || !strings.Contains(res.Output, nestedWarning) {
		t.Errorf("the result comes with the error: %+v", res)
	}
}

func TestCheckKeepsOnlyTheStartOfALongOutput(t *testing.T) {
	f := newFake(t, "i=0\nwhile [ $i -lt 3000 ]; do echo 'a line of output that is a bit long, 0123456789'; i=$((i+1)); done\n")
	res, err := Runner{Firejail: f.path}.Check(context.Background(), "/tmp/x.profile", CheckOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Output) != outputLimit {
		t.Errorf("output has %d bytes, want %d", len(res.Output), outputLimit)
	}
}

func TestCheckTimeout(t *testing.T) {
	f := newFake(t, "sleep SLEEPARG &\nwait\n")
	start := time.Now()
	_, err := Runner{Firejail: f.path, checkTimeout: 200 * time.Millisecond}.Check(context.Background(), "/tmp/x.profile", CheckOpts{})
	appErr := requireTagErr(t, err, tagCheckTimeout, "Runner.Check.CheckTimeout")
	if appErr.Metadata["timeout"] != "200ms" {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("the check did not stop at its limit")
	}
}

func TestCheckHasAThirtySecondLimit(t *testing.T) {
	if got := (Runner{}).timeouts().check; got != 30*time.Second {
		t.Errorf("check limit = %v", got)
	}
}

func TestCheckErrors(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fake := newFake(t, "exit 0\n")
	tests := []struct {
		name    string
		run     Runner
		ctx     context.Context
		profile string
		tag     string
	}{
		{"relative profile", Runner{Firejail: fake.path}, context.Background(), "x.profile", tagCheckProfileInvalid},
		{"unclean profile", Runner{Firejail: fake.path}, context.Background(), "/tmp/../x.profile", tagCheckProfileInvalid},
		{"cancelled", Runner{Firejail: fake.path}, cancelled, "/tmp/x.profile", tagCheckCanceled},
		{"firejail missing", Runner{Firejail: "/nonexistent/firejail"}, context.Background(), "/tmp/x.profile", tagCheckStartFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.run.Check(tc.ctx, tc.profile, CheckOpts{})
			requireTag(t, err, tc.tag, "Runner.Check."+tc.tag)
		})
	}
}

func TestCheckCancelledWhileRunning(t *testing.T) {
	f := newFake(t, "sleep SLEEPARG &\nwait\n")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, err := Runner{Firejail: f.path}.Check(ctx, "/tmp/x.profile", CheckOpts{})
	requireTag(t, err, tagCheckCanceled, "Runner.Check.CheckCanceled")
}

func TestWaitResultWithoutAState(t *testing.T) {
	for _, waitErr := range []error{io.ErrUnexpectedEOF, nil} {
		_, err := waitResult(&exec.Cmd{}, waitErr)
		requireTag(t, err, tagRunWaitFailed, "waitResult.RunWaitFailed")
	}
}

func TestNestedSandboxWarningIsRecognized(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{nestedWarning, true},
		{"Warning: cannot read /proc/1/comm\n" + nestedWarning + "\n", true},
		{"Parent pid 1234, child pid 1235", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := ContainsNestedSandboxWarning(tc.text); got != tc.want {
			t.Errorf("ContainsNestedSandboxWarning(%q) = %v", tc.text, got)
		}
	}
}

func TestTrySandboxName(t *testing.T) {
	if got := TrySandboxName("firejail-code"); got != "jailcat-try-firejail-code" {
		t.Errorf("TrySandboxName = %q", got)
	}
}

func TestTagsAreFoundByErrorsIs(t *testing.T) {
	f := newFake(t, "echo '"+nestedWarning+"'\n")
	s, err := Runner{Firejail: f.path}.Try(context.Background(), tryReq(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Wait()
	if !errors.Is(err, &errors.Error{Tag: TagNestedSandbox}) {
		t.Errorf("errors.Is does not match %v", err)
	}
}
