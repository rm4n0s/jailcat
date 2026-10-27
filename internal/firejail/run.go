package firejail

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rm4n0s/errors"
)

// TagNestedSandbox is the tag of the error [Session.Wait] and [Runner.Check] return when
// firejail printed "an existing sandbox was detected": Jailcat itself runs inside another
// sandbox, so firejail ran the program without any sandboxing. The UI matches it to say
// that Try and the build check cannot sandbox from inside another sandbox. The program's
// exit status is still returned along with the error.
const TagNestedSandbox = "NestedSandbox"

// nestedMarker is the part of firejail's warning that does not change between versions
// and programs ("Warning: an existing sandbox was detected. <program> will run without
// any additional sandboxing features").
const nestedMarker = "an existing sandbox was detected"

// tryPrefix starts the name of every sandbox Try makes (SPEC §11.2).
const tryPrefix = "jailcat-try-"

const (
	// defaultGrace is how long Stop waits for the sandbox to end after "firejail
	// --shutdown" before it kills the process group.
	defaultGrace = 5 * time.Second
	// defaultShutdownTimeout is how long "firejail --shutdown" itself may run.
	defaultShutdownTimeout = 10 * time.Second
	// defaultDrain is how long the output of a session is read after firejail has
	// exited, for output that a program left in the pipe or still holds open.
	defaultDrain = 2 * time.Second
	// defaultCheckTimeout is the limit of [Runner.Check] (SPEC §11.3 step 5).
	defaultCheckTimeout = 30 * time.Second
	// waitDelay is how long a killed process may keep its output open before Go stops
	// waiting for it.
	waitDelay = time.Second

	// lineBuffer is how many lines a session keeps for a reader that is slow; beyond
	// it the oldest lines are dropped.
	lineBuffer = 1024
	// readBuffer is the longest piece of output taken as one line; a longer line comes
	// as several.
	readBuffer = 64 * 1024
	// outputLimit is how much of the output of [Runner.Check] and of "firejail
	// --shutdown" is kept.
	outputLimit = 64 * 1024
)

// Runner runs firejail for Try (SPEC §11.2) and for the check after a build (SPEC §11.3
// step 5). It holds no state besides the path of firejail.
type Runner struct {
	// Firejail is the absolute path of the firejail executable ([FindFirejail]).
	Firejail string

	// The durations below are zero in the program, which means the defaults; the tests
	// of this package shorten them.
	grace           time.Duration
	shutdownTimeout time.Duration
	drain           time.Duration
	checkTimeout    time.Duration
}

// timeouts are the durations of a [Runner] with the defaults filled in.
type timeouts struct {
	grace, shutdown, drain, check time.Duration
}

// timeouts returns the durations of r.
func (r Runner) timeouts() timeouts {
	pick := func(v, def time.Duration) time.Duration {
		if v > 0 {
			return v
		}
		return def
	}
	return timeouts{
		grace:    pick(r.grace, defaultGrace),
		shutdown: pick(r.shutdownTimeout, defaultShutdownTimeout),
		drain:    pick(r.drain, defaultDrain),
		check:    pick(r.checkTimeout, defaultCheckTimeout),
	}
}

// TryRequest is what [Runner.Try] needs to run a build without installing it.
type TryRequest struct {
	// Name is the sandbox name of the build, made of [a-z0-9-]. It names the profile
	// file and, with a prefix, the firejail sandbox ([TrySandboxName]).
	Name string
	// ProfileText is the generated profile, as [Generated.String] returns it.
	ProfileText []byte
	// Binary is the absolute, clean path of the app's executable.
	Binary string
	// Args are the words after the binary (SPEC §7.6, the extra arguments).
	Args []string
	// CacheDir is Jailcat's cache folder, [xdg.Dirs.Cache]; the profile goes to
	// <CacheDir>/try/<Name>.profile.
	CacheDir string
	// PrivateHome is the folder of Own private home; it is created with mode 0700 when
	// it does not exist. Empty when the build has none.
	PrivateHome string
}

// TrySandboxName returns the name of the firejail sandbox that Try makes for the build
// name, "jailcat-try-<name>": the name "firejail --list" shows and "--shutdown" takes.
func TrySandboxName(name string) string {
	return tryPrefix + name
}

// ExitStatus is how a process ended.
type ExitStatus struct {
	// Code is the exit code; -1 when a signal ended the process.
	Code int
	// Signal is the name of the signal that ended the process ("killed",
	// "terminated"); empty when the process exited by itself.
	Signal string
}

// Success reports whether the process exited by itself with code 0.
func (e ExitStatus) Success() bool {
	return e.Signal == "" && e.Code == 0
}

// String describes the status in words, "exit status 3" or "killed".
func (e ExitStatus) String() string {
	if e.Signal != "" {
		return e.Signal
	}
	return "exit status " + strconv.Itoa(e.Code)
}

// ContainsNestedSandboxWarning reports whether text holds firejail's warning that it
// found an existing sandbox. The tests that run the real firejail use it to fail
// instead of passing on a program that ran unsandboxed.
func ContainsNestedSandboxWarning(text string) bool {
	return strings.Contains(text, nestedMarker)
}

// Session is a running Try: firejail with the build's profile, its output as lines and
// the means to stop it. It is made by [Runner.Try]; every goroutine it starts has ended
// when [Session.Wait] or [Session.Stop] returns.
type Session struct {
	runner Runner
	name   string
	t      timeouts
	cmd    *exec.Cmd

	// kill makes exec.CommandContext kill the process group of firejail.
	kill  context.CancelFunc
	lines chan string
	// done is closed when firejail has exited and its output has been read; status and
	// waitErr are written before it.
	done    chan struct{}
	wg      sync.WaitGroup
	nested  atomic.Bool
	status  ExitStatus
	waitErr error
}

// Try writes the profile of a build and runs it (SPEC §11.2): "firejail
// --profile=<CacheDir>/try/<Name>.profile --name=jailcat-try-<Name> -- <Binary> <Args>",
// started with exec.CommandContext in its own process group, with the output of the
// program and of firejail merged into the lines of [Session.Lines]. Nothing is installed.
// When ctx is cancelled the session stops itself like [Session.Stop].
//
// A name, binary or cache folder that is not valid, a profile or private home that
// cannot be written and a firejail that cannot be started each have their own tag.
func (r Runner) Try(ctx context.Context, req TryRequest) (*Session, error) {
	if !validTryName(req.Name) {
		return nil, errors.New(tagTryNameInvalid, "the sandbox name must have at least one character, and only a-z, 0-9 and -",
			"name", req.Name)
	}
	if !isCleanAbs(req.Binary) {
		return nil, errors.New(tagTryBinaryInvalid, "the path of the app's binary is not absolute and clean",
			"path", req.Binary)
	}
	if !filepath.IsAbs(req.CacheDir) {
		return nil, errors.New(tagTryCacheDirInvalid, "the cache folder is not an absolute path", "path", req.CacheDir)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr(tagTryCanceled, err)
	}

	dir := filepath.Join(req.CacheDir, "try")
	profile := filepath.Join(dir, req.Name+".profile")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.NewErr(tagTryProfileWriteFailed, err).SetMetadata("path", dir, "step", "mkdir")
	}
	if err := os.WriteFile(profile, req.ProfileText, 0o600); err != nil {
		return nil, errors.NewErr(tagTryProfileWriteFailed, err).SetMetadata("path", profile, "step", "write")
	}
	// WriteFile keeps the mode of a profile that was already there.
	if err := os.Chmod(profile, 0o600); err != nil {
		return nil, errors.NewErr(tagTryProfileWriteFailed, err).SetMetadata("path", profile, "step", "chmod")
	}
	if req.PrivateHome != "" {
		if err := os.MkdirAll(req.PrivateHome, 0o700); err != nil {
			return nil, errors.NewErr(tagTryHomeCreateFailed, err).SetMetadata("path", req.PrivateHome)
		}
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, errors.NewErr(tagTryStartFailed, err).SetMetadata("firejail", r.Firejail, "step", "pipe")
	}

	// The process outlives a cancelled ctx for as long as the graceful stop takes, so
	// it runs under a context of its own that only a hard kill cancels.
	procCtx, kill := context.WithCancel(context.WithoutCancel(ctx))
	args := []string{"--profile=" + profile, "--name=" + TrySandboxName(req.Name), "--", req.Binary}
	args = append(args, req.Args...)
	cmd := exec.CommandContext(procCtx, r.Firejail, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = waitDelay
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		kill()
		closeQuietly(pr)
		closeQuietly(pw)
		return nil, errors.NewErr(tagTryStartFailed, err).SetMetadata("firejail", r.Firejail, "step", "start")
	}
	// The child holds its own copy of the write end; ours must go so that the reader
	// sees the end of the output.
	closeQuietly(pw)

	s := &Session{
		runner: r,
		name:   req.Name,
		t:      r.timeouts(),
		cmd:    cmd,
		kill:   kill,
		lines:  make(chan string, lineBuffer),
		done:   make(chan struct{}),
	}
	readerDone := make(chan struct{})
	s.wg.Add(3)
	go s.readLines(pr, readerDone)
	go s.reap(pr, readerDone)
	go s.watch(ctx)
	return s, nil
}

// validTryName reports whether name is a non-empty string of [a-z0-9-].
func validTryName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// closeQuietly closes f. The files it is used for are pipe ends whose close can only
// fail when they were closed already, which leaves nothing to handle.
func closeQuietly(f *os.File) {
	_ = f.Close()
}

// killGroup kills the process group of cmd, which was started with Setpgid. It is the
// Cancel function of every process this package starts.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// SandboxName returns the name of the firejail sandbox of the session, as
// [TrySandboxName] gives it.
func (s *Session) SandboxName() string {
	return TrySandboxName(s.name)
}

// Lines returns the output of the session, firejail's and the program's, one line at a
// time, without the line ending. The channel is closed when the session has ended and
// its output has been read. A reader that falls more than a thousand lines behind loses
// the oldest ones; the program never waits for it.
func (s *Session) Lines() <-chan string {
	return s.lines
}

// Nested reports whether firejail has said so far that it found an existing sandbox.
func (s *Session) Nested() bool {
	return s.nested.Load()
}

// Wait waits until the session has ended and returns how firejail exited. It may be
// called more than once. If firejail printed the warning of an existing sandbox the
// error has the tag [TagNestedSandbox] and the exit status is returned along with it.
// Every goroutine of the session has ended when Wait returns.
func (s *Session) Wait() (ExitStatus, error) {
	<-s.done
	s.wg.Wait()
	if s.waitErr != nil {
		return s.status, s.waitErr
	}
	if s.nested.Load() {
		return s.status, errors.New(TagNestedSandbox,
			"firejail found an existing sandbox and ran the program without sandboxing it",
			"sandbox", s.SandboxName())
	}
	return s.status, nil
}

// Stop ends the session (SPEC §11.2): it runs "firejail --shutdown=jailcat-try-<Name>",
// and if the sandbox is still there after a grace period, or ctx ends first, it kills the
// process group. It returns once the session has ended, and does nothing if it already
// has. If "firejail --shutdown" failed and the group had to be killed, the error has the
// tag of that failure; the session is over either way.
func (s *Session) Stop(ctx context.Context) error {
	err := s.stop(ctx)
	s.wg.Wait()
	return err
}

// stop is [Session.Stop] without waiting for the goroutines, which the watcher calls
// from one of them.
func (s *Session) stop(ctx context.Context) error {
	select {
	case <-s.done:
		return nil
	default:
	}
	shutdownErr := s.shutdown(ctx)

	timer := time.NewTimer(s.t.grace)
	defer timer.Stop()
	select {
	case <-s.done:
		return nil
	case <-timer.C:
	case <-ctx.Done():
	}
	s.kill()
	<-s.done
	return shutdownErr
}

// shutdown runs "firejail --shutdown=<sandbox>".
func (s *Session) shutdown(ctx context.Context) error {
	runCtx, cancel := context.WithTimeout(ctx, s.t.shutdown)
	defer cancel()

	cmd := exec.CommandContext(runCtx, s.runner.Firejail, "--shutdown="+s.SandboxName())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = waitDelay
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return errors.NewErr(tagTryShutdownFailed, err).SetMetadata("firejail", s.runner.Firejail,
			"sandbox", s.SandboxName(), "output", out.String())
	}
	return nil
}

// watch stops the session when ctx is cancelled. It ends when the session does.
func (s *Session) watch(ctx context.Context) {
	defer s.wg.Done()
	select {
	case <-ctx.Done():
		// Nobody is left to receive an error here; the session ends the same way, and
		// Wait reports how.
		_ = s.stop(context.WithoutCancel(ctx))
	case <-s.done:
	}
}

// readLines turns the output of firejail into lines, and closes the channel at the end.
func (s *Session) readLines(pr *os.File, readerDone chan<- struct{}) {
	defer s.wg.Done()
	defer close(readerDone)
	defer close(s.lines)
	defer closeQuietly(pr)

	br := bufio.NewReaderSize(pr, readBuffer)
	for {
		chunk, _, err := br.ReadLine()
		if err != nil {
			// The only ends are the end of the output and the read deadline that reap
			// sets when a program holds the output open; neither is a failure.
			return
		}
		line := strings.ToValidUTF8(string(chunk), "�")
		if ContainsNestedSandboxWarning(line) {
			s.nested.Store(true)
		}
		s.emit(line)
	}
}

// emit sends line to the channel; when the channel is full the oldest line goes, so that
// a reader that does not read never holds up the program. Only readLines sends, so the
// second send finds room.
func (s *Session) emit(line string) {
	select {
	case s.lines <- line:
		return
	default:
	}
	select {
	case <-s.lines:
	default:
	}
	s.lines <- line
}

// reap waits for firejail to exit, then for its output to be read, then marks the
// session as done.
func (s *Session) reap(pr *os.File, readerDone <-chan struct{}) {
	defer s.wg.Done()
	waitErr := s.cmd.Wait()
	// The process is gone; releasing the context frees its resources.
	s.kill()

	timer := time.NewTimer(s.t.drain)
	select {
	case <-readerDone:
	case <-timer.C:
		// A program that daemonized still holds the output open.
		// A deadline that cannot be set means the reader closed the pipe already.
		_ = pr.SetReadDeadline(time.Now())
		<-readerDone
	}
	timer.Stop()

	s.status, s.waitErr = waitResult(s.cmd, waitErr)
	close(s.done)
}

// waitResult returns how cmd ended. waitErr is what cmd.Wait or Run returned: an
// *exec.ExitError for a non-zero exit is not a failure, so it is not looked at when the
// process left a state; without a state the process was never waited for.
func waitResult(cmd *exec.Cmd, waitErr error) (ExitStatus, error) {
	state := cmd.ProcessState
	if state == nil {
		if waitErr == nil {
			return ExitStatus{Code: -1}, errors.New(tagRunWaitFailed, "the process left no exit status")
		}
		return ExitStatus{Code: -1}, errors.NewErr(tagRunWaitFailed, waitErr)
	}
	status := ExitStatus{Code: state.ExitCode()}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		status.Signal = ws.Signal().String()
	}
	return status, nil
}

// CheckOpts are the options of [Runner.Check].
type CheckOpts struct {
	// IgnorePrivateBin passes "--ignore=private-bin" to firejail, so that "true" is
	// reachable in a build that limits the programs (private-bin) and does not list it.
	IgnorePrivateBin bool
	// IgnoreCount is the number of ignore entries the build already has: the Total of
	// [Generated.IgnoreCount]. The extra command-line ignore counts toward firejail's
	// limit of 32.
	IgnoreCount int
}

// CheckResult is the outcome of [Runner.Check].
type CheckResult struct {
	// OK is true when firejail ran "true" in the sandbox and exited with status 0. A
	// skipped check is not OK.
	OK bool
	// Skipped is true when firejail was not run, because the check would exceed
	// firejail's limit of ignore entries; Reason says so in words for the UI.
	Skipped bool
	// Reason says why the check was skipped; empty otherwise.
	Reason string
	// Status is how firejail exited.
	Status ExitStatus
	// Output is what firejail printed (standard output and error together), cut at
	// 64 KiB.
	Output string
}

// Check runs the command of the build check (SPEC §11.3 step 5), "firejail
// [--ignore=private-bin] --profile=<profilePath> -- true", and returns how it ended and
// what it printed. There is no --quiet: with it firejail does not print its warning of an
// existing sandbox, and the profile's own "quiet" line silences the banner. "true" is the
// first one on the path, else /usr/bin/true. The run is limited to 30 seconds and is
// started in its own process group. A non-zero exit is not an error: it is a result with
// OK false, for the UI to show with the output.
//
// With opts.IgnorePrivateBin the command line has "--ignore=private-bin" first. If the
// build already has 32 ignore entries that would be one too many, so firejail is not run
// and the result is Skipped, with the reason.
//
// If the output has firejail's warning of an existing sandbox, the result comes back
// along with an error that has the tag [TagNestedSandbox].
func (r Runner) Check(ctx context.Context, profilePath string, opts CheckOpts) (CheckResult, error) {
	if !isCleanAbs(profilePath) {
		return CheckResult{}, errors.New(tagCheckProfileInvalid, "the path of the profile is not absolute and clean",
			"path", profilePath)
	}
	if opts.IgnorePrivateBin && opts.IgnoreCount >= maxProfileIgnore {
		return CheckResult{
			Skipped: true,
			Reason: "the check was skipped: the build already has " + strconv.Itoa(opts.IgnoreCount) +
				" ignore entries, and the check needs one more to run a program in a sandbox that limits the " +
				"programs, but firejail accepts at most " + strconv.Itoa(maxProfileIgnore),
		}, nil
	}
	t := r.timeouts()
	runCtx, cancel := context.WithTimeout(ctx, t.check)
	defer cancel()

	var args []string
	if opts.IgnorePrivateBin {
		args = append(args, "--ignore=private-bin")
	}
	args = append(args, "--profile="+profilePath, "--", trueBinary())
	cmd := exec.CommandContext(runCtx, r.Firejail, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = waitDelay
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	output := out.String()
	if runErr != nil {
		// Only our own deadline is a timeout; a cancelled parent context is a
		// cancelled check.
		if ctx.Err() != nil {
			return CheckResult{Output: output}, errors.NewErr(tagCheckCanceled, runErr).SetMetadata("firejail", r.Firejail)
		}
		if runCtx.Err() == context.DeadlineExceeded {
			return CheckResult{Output: output}, errors.NewErr(tagCheckTimeout, runErr).SetMetadata("firejail", r.Firejail,
				"timeout", t.check.String(), "output", output)
		}
		if cmd.ProcessState == nil {
			return CheckResult{}, errors.NewErr(tagCheckStartFailed, runErr).SetMetadata("firejail", r.Firejail)
		}
	}
	status, err := waitResult(cmd, runErr)
	if err != nil {
		return CheckResult{Output: output}, err
	}
	res := CheckResult{OK: status.Success(), Status: status, Output: output}
	if ContainsNestedSandboxWarning(output) {
		return res, errors.New(TagNestedSandbox,
			"firejail found an existing sandbox and ran the check without sandboxing it", "output", output)
	}
	return res, nil
}

// trueBinary returns the absolute path of "true" on the path, or /usr/bin/true.
func trueBinary() string {
	if p, err := exec.LookPath("true"); err == nil && filepath.IsAbs(p) {
		return p
	}
	return "/usr/bin/true"
}

// limitedBuffer keeps the first outputLimit bytes written to it and drops the rest.
type limitedBuffer struct {
	buf bytes.Buffer
}

// Write keeps what fits and reports all of p as written, so that the process never
// blocks or fails on its output.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := outputLimit - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// String returns what was kept, as valid UTF-8.
func (b *limitedBuffer) String() string {
	return strings.ToValidUTF8(b.buf.String(), "�")
}

const (
	tagTryNameInvalid        = "TryNameInvalid"
	tagTryBinaryInvalid      = "TryBinaryInvalid"
	tagTryCacheDirInvalid    = "TryCacheDirInvalid"
	tagTryCanceled           = "TryCanceled"
	tagTryProfileWriteFailed = "TryProfileWriteFailed"
	tagTryHomeCreateFailed   = "TryHomeCreateFailed"
	tagTryStartFailed        = "TryStartFailed"
	tagTryShutdownFailed     = "TryShutdownFailed"
	tagRunWaitFailed         = "RunWaitFailed"
	tagCheckProfileInvalid   = "CheckProfileInvalid"
	tagCheckCanceled         = "CheckCanceled"
	tagCheckTimeout          = "CheckTimeout"
	tagCheckStartFailed      = "CheckStartFailed"
)
