package ui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// maxTryLines is how many lines of firejail's output a Try keeps; when there are more
// the oldest go and the panel says so.
const maxTryLines = 5000

// tryDropLines is how many of the oldest lines go at once when the output is over
// [maxTryLines], so the output is not copied for every new line.
const tryDropLines = 1000

// tryGrace is how long a stop may take before the controller gives up waiting for it.
const tryGrace = 15 * time.Second

// TryPhase is where a Try is.
type TryPhase int

// The phases of a Try.
const (
	// TryIdle is no Try: nothing ran, or the panel was closed.
	TryIdle TryPhase = iota
	// TryStarting is firejail being started.
	TryStarting
	// TryRunning is a sandbox that runs.
	TryRunning
	// TryEnded is a Try that has ended; its output stays until the panel is closed.
	TryEnded
)

// TrySnapshot is what a Try shows at one moment.
type TrySnapshot struct {
	// Owner is the editor the Try was started for.
	Owner *Editor
	// Phase says where the Try is.
	Phase TryPhase
	// Name is the sandbox name of the build that is tried.
	Name string
	// Lines is the output so far; do not change it.
	Lines []string
	// Dropped is how many of the oldest lines were dropped.
	Dropped int
	// Status is how firejail exited; valid when Phase is [TryEnded] and StartErr is nil.
	Status firejail.ExitStatus
	// Nested is true when firejail found an existing sandbox and ran the program
	// unsandboxed.
	Nested bool
	// StartErr is why the Try could not start; nil otherwise. The user's words are
	// [TrySnapshot.Failure].
	StartErr error
	// Failure is the message for the user when the Try could not start or could not be
	// waited for; "" otherwise.
	Failure string
	// Stopping is true from the moment Stop is pressed until the Try ends.
	Stopping bool
}

// TryController runs one Try at a time for the window (SPEC §11.2): it starts it off the
// UI thread, keeps its output, and stops it when the editor closes, another app is
// selected or the window closes. Any goroutine may call its methods.
type TryController struct {
	backend Backend
	logger  *slog.Logger

	mu      sync.Mutex
	snap    TrySnapshot
	session TrySession
	cancel  context.CancelFunc
	// done is closed when the run has ended; nil when no run is active.
	done chan struct{}
	// stopRequested is set by Stop while the session is still starting.
	stopRequested bool
	// notify is called, without the lock, after every change; notifyOwner says who set it.
	notify      func()
	notifyOwner any
}

// NewTryController creates the controller of a window.
func NewTryController(backend Backend, logger *slog.Logger) *TryController {
	return &TryController{backend: backend, logger: logger}
}

// Bind sets the function called after every change of the Try; owner identifies the
// caller, and only [TryController.Unbind] with the same owner removes it.
func (c *TryController) Bind(owner any, notify func()) {
	c.mu.Lock()
	c.notify, c.notifyOwner = notify, owner
	c.mu.Unlock()
}

// Unbind removes the function set by Bind(owner, ...); it does nothing when another owner
// has bound since.
func (c *TryController) Unbind(owner any) {
	c.mu.Lock()
	if c.notifyOwner == owner {
		c.notify, c.notifyOwner = nil, nil
	}
	c.mu.Unlock()
}

// changed calls the bound function; it is called without the lock held.
func (c *TryController) changed() {
	c.mu.Lock()
	notify := c.notify
	c.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// Snapshot returns what the Try shows now.
func (c *TryController) Snapshot() TrySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap
}

// Active reports whether a Try is starting or running. Only one Try runs at a time.
func (c *TryController) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap.Phase == TryStarting || c.snap.Phase == TryRunning
}

// Start begins a Try of the request for the editor, off the UI thread. It does nothing
// while a Try is active.
func (c *TryController) Start(owner *Editor, req BuildRequest) {
	c.mu.Lock()
	if c.snap.Phase == TryStarting || c.snap.Phase == TryRunning {
		c.mu.Unlock()
		return
	}
	// The session lives until it is stopped, whatever ends the call that started it.
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	c.stopRequested = false
	c.snap = TrySnapshot{Owner: owner, Phase: TryStarting, Name: req.Settings.SandboxName}
	done := c.done
	c.mu.Unlock()
	c.changed()

	go c.run(ctx, cancel, done, req)
}

// run is the Try: it starts the session, reads its lines and records how it ended.
func (c *TryController) run(ctx context.Context, cancel context.CancelFunc, done chan struct{}, req BuildRequest) {
	defer close(done)
	defer cancel()

	session, err := c.backend.Try(ctx, req)
	if err != nil {
		c.logger.Error("starting the try failed", "sandbox", req.Settings.SandboxName, "err", err)
		c.mu.Lock()
		c.snap.Phase, c.snap.StartErr, c.snap.Failure = TryEnded, err, tryStartFailedText(err)
		c.snap.Stopping = false
		c.mu.Unlock()
		c.changed()
		return
	}
	c.mu.Lock()
	c.session = session
	c.snap.Phase = TryRunning
	stop := c.stopRequested
	c.mu.Unlock()
	c.changed()
	if stop {
		c.stopSession(session)
	}

	pending := 0
	for line := range session.Lines() {
		c.mu.Lock()
		c.addLine(line)
		c.mu.Unlock()
		pending++
		// Tell the window once the burst is over, or every few lines of a long one.
		if len(session.Lines()) == 0 || pending >= 64 {
			c.changed()
			pending = 0
		}
	}
	status, waitErr := session.Wait()

	c.mu.Lock()
	c.snap.Phase, c.snap.Status, c.snap.Stopping = TryEnded, status, false
	c.snap.Nested = session.Nested()
	if waitErr != nil {
		if appErr, ok := errors.FromError(waitErr); ok && appErr.Tag == firejail.TagNestedSandbox {
			c.snap.Nested = true
		} else {
			c.snap.Failure = tryWaitFailedText(waitErr)
			c.logger.Error("the try ended with an error", "sandbox", req.Settings.SandboxName, "err", waitErr)
		}
	}
	c.session = nil
	c.mu.Unlock()
	c.changed()
}

// addLine appends a line of output; the caller holds the lock.
func (c *TryController) addLine(line string) {
	if len(c.snap.Lines) >= maxTryLines {
		// A new slice: a snapshot that is being drawn keeps the old one.
		kept := make([]string, len(c.snap.Lines)-tryDropLines, maxTryLines)
		copy(kept, c.snap.Lines[tryDropLines:])
		c.snap.Lines = kept
		c.snap.Dropped += tryDropLines
	}
	c.snap.Lines = append(c.snap.Lines, line)
}

// stopSession stops a session and logs a failure to do it cleanly: the session is over
// either way.
func (c *TryController) stopSession(session TrySession) {
	ctx, cancel := context.WithTimeout(context.Background(), tryGrace)
	defer cancel()
	if err := session.Stop(ctx); err != nil {
		c.logger.Error("stopping the try failed", "err", err)
	}
}

// Stop asks the running Try to end and returns at once; the panel shows when it has. It
// does nothing when no Try is active.
func (c *TryController) Stop() {
	c.mu.Lock()
	if c.snap.Phase != TryStarting && c.snap.Phase != TryRunning {
		c.mu.Unlock()
		return
	}
	if c.snap.Stopping {
		c.mu.Unlock()
		return
	}
	c.snap.Stopping = true
	session := c.session
	if session == nil {
		c.stopRequested = true
	}
	c.mu.Unlock()
	c.changed()
	if session != nil {
		go c.stopSession(session)
	}
}

// StopWait stops the Try and waits until it has ended, or until ctx is done.
func (c *TryController) StopWait(ctx context.Context) {
	c.mu.Lock()
	done := c.done
	active := c.snap.Phase == TryStarting || c.snap.Phase == TryRunning
	c.mu.Unlock()
	if !active || done == nil {
		return
	}
	c.Stop()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Clear closes the panel of a Try that has ended. It does nothing while one is active.
func (c *TryController) Clear() {
	c.mu.Lock()
	if c.snap.Phase == TryEnded {
		c.snap = TrySnapshot{}
	}
	c.mu.Unlock()
	c.changed()
}

// Release stops the Try that belongs to the editor, if any, and closes its panel as soon
// as it has ended: the editor is gone.
func (c *TryController) Release(owner *Editor) {
	c.mu.Lock()
	mine := c.snap.Owner == owner && c.snap.Phase != TryIdle
	active := c.snap.Phase == TryStarting || c.snap.Phase == TryRunning
	done := c.done
	c.mu.Unlock()
	if !mine {
		return
	}
	if !active {
		c.Clear()
		return
	}
	c.Stop()
	go func() {
		<-done
		c.mu.Lock()
		if c.snap.Owner == owner && c.snap.Phase == TryEnded {
			c.snap = TrySnapshot{}
		}
		c.mu.Unlock()
		c.changed()
	}()
}

// tryStartFailedText is what the user reads when a Try could not start.
func tryStartFailedText(err error) string {
	if appErr, ok := errors.FromError(err); ok && appErr.Tag == firejail.TagFirejailNotFound {
		return firejailMissingShort
	}
	return "Jailcat could not start the sandbox: " + plainReason(err) + "."
}

// tryWaitFailedText is what the user reads when firejail could not be waited for.
func tryWaitFailedText(err error) string {
	return "Jailcat lost track of the sandbox: " + plainReason(err) + "."
}
