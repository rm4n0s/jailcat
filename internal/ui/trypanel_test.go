package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// tryPage opens a new build of the Calculator and gives the backend a session to run.
func tryPage(t *testing.T) (*page, *fakeSession) {
	t.Helper()
	p := buildPage(t, "Calculator")
	session := newFakeSession()
	p.backend.set(func(f *fakeBackend) { f.session = session })
	t.Cleanup(func() { session.finish(firejail.ExitStatus{Code: -1, Signal: "killed"}, nil) })
	return p, session
}

// try is the controller of the open editor's page.
func (p *page) try() *TryController { return p.editorState().try }

// startTry presses Try and waits for the sandbox to run.
func (p *page) startTry() {
	p.t.Helper()
	p.tap("button:try")
	p.waitFor("the session", func() bool { return p.try().Snapshot().Phase == TryRunning })
}

func TestTryStreamsTheOutputAndStops(t *testing.T) {
	p, session := tryPage(t)
	if p.ts.Exists(key("try:panel")) {
		t.Fatal("the panel shows before a Try")
	}
	p.startTry()
	if !p.ts.Exists(key("try:panel")) {
		t.Fatal("no panel")
	}
	if !p.text(tryTitlePrefix+"firejail-calculator") || !p.text(tryNoOutputText) {
		t.Error("the panel is not about the build or says nothing about the empty output")
	}
	// One Try at a time, and Build waits for nothing.
	if p.enabled("button:try") {
		t.Error("a second Try can be started")
	}
	if !p.enabled("button:build") {
		t.Error("Build is disabled by a Try")
	}
	if got := p.backend.tries; len(got) != 1 || got[0].Settings.SandboxName != "firejail-calculator" {
		t.Errorf("Try was asked for %+v", got)
	}

	session.emit("Parent pid 4242, child pid 4243")
	session.emit("hello from the sandbox")
	p.waitFor("the lines", func() bool { return p.ts.Exists(gluttertest.ByText("hello from the sandbox")) })
	if !p.ts.Exists(gluttertest.ByText("Parent pid 4242, child pid 4243")) {
		t.Error("the first line is not shown")
	}
	if !p.text(tryRunningText) || !p.enabled("try:stop") {
		t.Error("the panel does not say it runs, or Stop does not work")
	}

	p.tap("try:stop")
	p.waitFor("the end", func() bool { return p.try().Snapshot().Phase == TryEnded })
	if session.stopCount() != 1 {
		t.Errorf("Stop was called %d times", session.stopCount())
	}
	p.waitFor("the status", func() bool { return p.text(tryEndedPrefix + "terminated") })
	if !p.ts.Exists(key("try:close")) || p.ts.Exists(key("try:stop")) {
		t.Error("the buttons of an ended Try are wrong")
	}
	// Nothing was built, stored or installed.
	if len(p.backend.buildAsks) != 0 || len(p.backend.savesList()) != 0 {
		t.Errorf("a Try changed things: %v", p.backend.savesList())
	}
	p.tap("try:close")
	if p.ts.Exists(key("try:panel")) {
		t.Error("Close kept the panel")
	}
	if !p.enabled("button:try") {
		t.Error("Try stays disabled after the panel is closed")
	}
}

func TestTryEndsByItself(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	session.emit("done")
	session.finish(firejail.ExitStatus{Code: 3}, nil)
	p.waitFor("the status", func() bool { return p.text(tryEndedPrefix + "exit status 3") })
	if !p.ts.Exists(gluttertest.ByText("done")) {
		t.Error("the last line is lost")
	}
}

func TestTryKeepsLongOutputLazy(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	for i := range 3000 {
		session.emit("line " + strconv.Itoa(i))
	}
	p.waitFor("the lines", func() bool { return len(p.try().Snapshot().Lines) == 3000 })
	// The panel follows the end, and only the lines that fit are built.
	p.waitFor("the end of the output", func() bool { return p.ts.Exists(gluttertest.ByText("line 2999")) })
	shown := 0
	for i := range 3000 {
		if p.ts.Exists(gluttertest.ByText("line " + strconv.Itoa(i))) {
			shown++
		}
	}
	if shown == 0 || shown > 40 {
		t.Errorf("%d of 3000 lines are built", shown)
	}
}

func TestTryDropsTheOldestLines(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	total := maxTryLines + 300
	for i := range total {
		session.emit("n" + strconv.Itoa(i))
	}
	p.waitFor("the lines", func() bool {
		snap := p.try().Snapshot()
		return snap.Dropped+len(snap.Lines) == total
	})
	snap := p.try().Snapshot()
	if snap.Dropped != tryDropLines || len(snap.Lines) > maxTryLines || snap.Lines[len(snap.Lines)-1] != "n"+strconv.Itoa(total-1) {
		t.Errorf("dropped %d, kept %d", snap.Dropped, len(snap.Lines))
	}
	p.waitFor("the note", func() bool { return p.ts.Exists(key("try:dropped")) })
}

func TestTryInsideAnotherSandbox(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	session.emit("Warning: an existing sandbox was detected. app will run without any additional sandboxing features")
	session.finish(firejail.ExitStatus{}, errors.New(firejail.TagNestedSandbox, "firejail found an existing sandbox"))
	p.waitFor("the explanation", func() bool { return p.ts.Exists(key("try:nested")) })
	if !p.text(tryNestedText) {
		t.Error("the nested case is not explained in words")
	}
	if p.ts.Exists(key("try:failure")) {
		t.Error("the nested case is shown as a failure")
	}
}

func TestTryThatCannotStart(t *testing.T) {
	p, _ := tryPage(t)
	p.backend.set(func(f *fakeBackend) { f.tryErr = errors.New("TryStartFailed", "the program could not be started") })
	p.tap("button:try")
	p.waitFor("the failure", func() bool { return p.ts.Exists(key("try:failure")) })
	if !p.ts.Exists(gluttertest.ByTextContaining("the program could not be started")) {
		t.Error("the reason is not shown")
	}
	if p.ts.Exists(gluttertest.ByTextContaining("TryStartFailed")) {
		t.Error("the tag is shown to the user")
	}
	if !strings.Contains(p.log.String(), "TryStartFailed") {
		t.Errorf("the log lacks the details:\n%s", p.log.String())
	}
	if !p.text("Could not start") {
		t.Error("the status does not say it")
	}
}

func TestTryChecksTheExtraRulesFirst(t *testing.T) {
	p, _ := tryPage(t)
	p.change(func(e *Editor) { e.SetExtraRules([]string{"this-is-not-a-command"}) })
	p.tap("button:try")
	p.waitFor("the message", func() bool { return p.text(problemsBlockText) })
	if len(p.backend.tries) != 0 {
		t.Error("a Try started with an error in the Extra rules")
	}
}

func TestTryStopsWhenAnotherAppIsSelected(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	p.selectRow("Files")
	p.editorReady()
	p.waitFor("the stop", func() bool { return session.stopCount() == 1 })
	p.waitFor("the end", func() bool { return p.home().try.Snapshot().Phase == TryIdle })
	if p.ts.Exists(key("try:panel")) {
		t.Error("the panel of the old editor shows on the new one")
	}
}

func TestTryStopsWhenTheWindowCloses(t *testing.T) {
	p, session := tryPage(t)
	p.startTry()
	if p.ts.RequestWindowClose(gui.ImplicitViewID) {
		t.Fatal("the window closed with a Try running")
	}
	p.waitFor("the exit", func() bool { return p.ts.ExitRequests() == 1 })
	if session.stopCount() != 1 {
		t.Errorf("Stop was called %d times", session.stopCount())
	}
	if p.home().try.Active() {
		t.Error("the Try still runs")
	}
}

func TestTryStopsWhenTheEditorCloses(t *testing.T) {
	// The editor of an uninstalled build goes, and its Try with it.
	p := buildPage(t, "Visual Studio Code")
	session := newFakeSession()
	p.backend.set(func(f *fakeBackend) { f.session = session })
	t.Cleanup(func() { session.finish(firejail.ExitStatus{Code: -1, Signal: "killed"}, nil) })
	p.startTry()
	p.home().uninstalled()
	p.ts.Pump()
	p.waitFor("the stop", func() bool { return session.stopCount() == 1 })
}
