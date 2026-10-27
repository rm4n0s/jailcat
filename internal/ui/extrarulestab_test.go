package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// extraPage opens the Extra rules tab of a new build of the app, in a tall window.
func extraPage(t *testing.T, app string) *page {
	t.Helper()
	return tabPage(t, app, TabExtraRules)
}

// extraState is the state of the Extra rules tab.
func (p *page) extraState() *extraRulesTabState {
	return gluttertest.StateOf[*extraRulesTabState](p.ts, gluttertest.ByType[*ExtraRulesTab]())
}

// countKeyed counts the widgets whose key has the prefix.
func (p *page) countKeyed(prefix string) int {
	return p.ts.Count(gluttertest.ByPredicate("a key starting with "+prefix, func(w m.Widget) bool {
		k := w.Key()
		return k != nil && strings.HasPrefix(fmt.Sprint(k), "[<'"+prefix)
	}))
}

// typeRows types the rows and waits until the check of them has answered.
func (p *page) typeRows(rows ...string) {
	p.t.Helper()
	p.typeInto("extra:field", strings.Join(rows, "\n"))
	want := rowsOfText(strings.Join(rows, "\n"))
	p.waitFor("the check of the rows", func() bool {
		return p.home().editor.ExtraRulesChecked() && slices.Equal(p.home().editor.ExtraRules(), want)
	})
}

// checks returns the rows the backend was asked to check.
func (p *page) checks() [][]string {
	p.backend.mu.Lock()
	defer p.backend.mu.Unlock()
	return slices.Clone(p.backend.checkAsks)
}

func TestRowsOfText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"empty", "", nil},
		{"one row", "net none", []string{"net none"}},
		{"two rows", "a\nb", []string{"a", "b"}},
		{"trailing new lines are not rows", "a\n\n\n", []string{"a"}},
		{"trailing blanks are not rows", "a\n   \n\t", []string{"a"}},
		{"empty rows inside stay", "a\n\nb", []string{"a", "", "b"}},
		{"leading empty row stays", "\na", []string{"", "a"}},
		{"CRLF", "a\r\nb\r\n", []string{"a", "b"}},
		{"only blanks", "\n \n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowsOfText(tt.text); !slices.Equal(got, tt.want) {
				t.Errorf("rowsOfText(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtraRulesTabTypingRows(t *testing.T) {
	p := extraPage(t, "Calculator")
	e := p.home().editor
	if p.tabDot(TabExtraRules) || len(e.ExtraRules()) != 0 {
		t.Fatal("a fresh build has extra rules")
	}
	// The box is empty and shows six numbered rows.
	if p.textOf("extra:field") != "" {
		t.Error("the box is not empty")
	}
	for i := 1; i <= 6; i++ {
		if !p.ts.Exists(key("extra:number:" + strconv.Itoa(i))) {
			t.Errorf("no number %d beside the box", i)
		}
	}
	if p.ts.Exists(key("extra:number:7")) {
		t.Error("a seventh number is shown for an empty box")
	}

	p.typeRows("noblacklist /mnt", "", "ignore apparmor")
	if want := []string{"noblacklist /mnt", "", "ignore apparmor"}; !slices.Equal(e.ExtraRules(), want) {
		t.Fatalf("ExtraRules = %q, want %q", e.ExtraRules(), want)
	}
	if !p.tabDot(TabExtraRules) {
		t.Error("the tab shows no dot for rows")
	}
	if last := p.checks(); !slices.Equal(last[len(last)-1], e.ExtraRules()) {
		t.Errorf("the last check was for %q", last[len(last)-1])
	}

	// Eight rows give eight numbers.
	p.typeRows("a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8")
	if !p.ts.Exists(key("extra:number:8")) || p.ts.Exists(key("extra:number:9")) {
		t.Error("the numbers do not follow the rows")
	}

	// A new line at the end gets a number and no row.
	p.typeInto("extra:field", "ignore apparmor\n\n\n\n\n\n\n")
	p.waitFor("the rows", func() bool { return len(e.ExtraRules()) == 1 })
	if !p.ts.Exists(key("extra:number:8")) {
		t.Error("the empty rows at the end have no numbers")
	}

	// Emptying the box removes the rows and the dot.
	p.typeInto("extra:field", "")
	p.waitFor("no rows", func() bool { return len(e.ExtraRules()) == 0 && e.ExtraRulesChecked() })
	if p.tabDot(TabExtraRules) {
		t.Error("the dot stays for an empty box")
	}
}

func TestExtraRulesTabProblems(t *testing.T) {
	p := extraPage(t, "Calculator")
	e := p.home().editor
	p.typeRows("bogus 1", "net none", "noblacklist /mnt")

	// One error and one warning, each with the number of its row.
	probs := e.ExtraProblems()
	if len(probs) != 2 {
		t.Fatalf("problems = %+v", probs)
	}
	if probs[0].Row != 1 || probs[0].Severity != firejail.SeverityError || probs[0].Tag != firejail.ProblemExtraCommand {
		t.Errorf("first problem = %+v", probs[0])
	}
	if probs[1].Row != 2 || probs[1].Severity != firejail.SeverityWarning || probs[1].Tag != firejail.ProblemExtraCovered {
		t.Errorf("second problem = %+v", probs[1])
	}
	in := func(k string, f gluttertest.Finder) bool { return p.ts.Exists(gluttertest.Descendant(key(k), f)) }
	if !in("extra:problem:0", gluttertest.ByIcon(m.Icons.Error())) || !in("extra:problem:0", gluttertest.ByTextContaining("Row 1:")) {
		t.Error("the error is not shown with its icon and row")
	}
	if !in("extra:problem:1", gluttertest.ByIcon(m.Icons.Warning())) || !in("extra:problem:1", gluttertest.ByTextContaining("Row 2:")) ||
		!in("extra:problem:1", gluttertest.ByTextContaining("Network")) {
		t.Error("the warning is not shown with its icon, row and the setting it names")
	}
	if p.ts.Exists(key("extra:problem:2")) {
		t.Error("a row without a problem is listed")
	}

	// The error reaches the problems of the editor, the warning does not.
	all := e.Problems()
	if len(all) != 1 || all[0].Row != 1 || all[0].Severity != firejail.SeverityError {
		t.Errorf("Problems = %+v", all)
	}
	if !p.ts.Exists(gluttertest.ByTextContaining(all[0].Message)) {
		t.Error("the page does not list the error")
	}

	// Fixing the row removes the error and keeps the warning.
	p.typeRows("noblacklist /mnt", "net none")
	if got := e.Problems(); len(got) != 0 {
		t.Errorf("Problems after the fix = %+v", got)
	}
	if got := e.ExtraProblems(); len(got) != 1 || got[0].Row != 2 || got[0].Severity != firejail.SeverityWarning {
		t.Errorf("ExtraProblems after the fix = %+v", got)
	}
	if p.ts.Exists(key("extra:problem:1")) || !p.ts.Exists(key("extra:problem:0")) {
		t.Error("the list does not follow the rows")
	}
}

func TestExtraRulesTabKindsOfProblems(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		tag  string
		sev  firejail.Severity
	}{
		{"unknown command", []string{"frobnicate"}, firejail.ProblemExtraCommand, firejail.SeverityError},
		{"missing argument", []string{"blacklist"}, firejail.ProblemExtraArgs, firejail.SeverityError},
		{"include outside", []string{"include /tmp/evil.inc"}, firejail.ProblemExtraInclude, firejail.SeverityError},
		{"unknown condition", []string{"?NOPE: nosound"}, firejail.ProblemExtraCondition, firejail.SeverityError},
		{"a line a field covers", []string{"nosound"}, firejail.ProblemExtraCovered, firejail.SeverityWarning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := extraPage(t, "Calculator")
			p.typeRows(append([]string{"# a comment"}, tt.rows...)...)
			probs := p.home().editor.ExtraProblems()
			if len(probs) != 1 || probs[0].Tag != tt.tag || probs[0].Severity != tt.sev || probs[0].Row != 2 {
				t.Fatalf("problems = %+v", probs)
			}
			wantErrors := 0
			if tt.sev == firejail.SeverityError {
				wantErrors = 1
			}
			if got := len(p.home().editor.Problems()); got != wantErrors {
				t.Errorf("Problems has %d entries, want %d", got, wantErrors)
			}
		})
	}
}

func TestExtraRulesTabFollowsTheEditor(t *testing.T) {
	p := extraPage(t, "Calculator")
	e := p.home().editor
	// A draft that is discarded, or any other change of the rows, reaches the box.
	p.change(func(e *Editor) { e.SetExtraRules([]string{"ignore apparmor", "bogus"}) })
	if got := p.textOf("extra:field"); got != "ignore apparmor\nbogus" {
		t.Errorf("the box shows %q", got)
	}
	p.waitFor("the check", func() bool { return e.ExtraRulesChecked() && len(e.ExtraProblems()) == 1 })
	p.change(func(e *Editor) { e.SetExtraRules(nil) })
	if p.textOf("extra:field") != "" {
		t.Error("the box keeps the rows")
	}
	p.waitFor("the check of no rows", func() bool { return e.ExtraRulesChecked() && len(e.ExtraProblems()) == 0 })
	if p.ts.Exists(key("extra:problem:0")) {
		t.Error("a problem is shown for no rows")
	}
}

func TestExtraRulesTabChecksAgainWhenTheProfileChanges(t *testing.T) {
	p := extraPage(t, "Calculator")
	e := p.home().editor
	// "ignore nosound" removes a line of the base only when the base has one.
	p.typeRows("ignore nosound")
	if got := e.ExtraProblems(); len(got) != 0 {
		t.Fatalf("on the default profile: %+v", got)
	}
	n := len(p.checks())
	base, profile := p.backend.fj.fixtureRead(t, "code.profile")
	p.change(func(e *Editor) {
		e.SwitchProfile(filepath.Join(p.backend.fj.dirs.FirejailSystem, "code.profile"), base, profile, true)
	})
	p.waitFor("the new check", func() bool { return len(p.checks()) > n && e.ExtraRulesChecked() })
	if got := p.checks(); !slices.Equal(got[len(got)-1], []string{"ignore nosound"}) {
		t.Errorf("the check after the switch was for %q", got[len(got)-1])
	}
}

// fixtureRead reads a profile of the fixture folders.
func (b *LocalBackend) fixtureRead(t *testing.T, name string) (firejail.Base, *firejail.Profile) {
	t.Helper()
	base, profile, err := b.ReadBase(t.Context(), filepath.Join(b.dirs.FirejailSystem, name))
	if err != nil {
		t.Fatal(err)
	}
	return base, profile
}

func TestEditorExtraProblemsAreForTheirRows(t *testing.T) {
	f := newFixtureFirejail(t)
	e := newEditor(t, f, "default.profile")
	rows := []string{"bogus", "net none"}
	problems := []firejail.Problem{
		{Row: 1, Severity: firejail.SeverityError, Tag: firejail.ProblemExtraCommand, Message: "Row 1: no"},
		{Row: 2, Severity: firejail.SeverityWarning, Tag: firejail.ProblemExtraCovered, Message: "Row 2: covered"},
	}
	if e.ExtraRulesChecked() || len(e.ExtraProblems()) != 0 {
		t.Fatal("a new editor has a check")
	}
	e.SetExtraRules(rows)
	e.SetExtraProblems(rows, problems)
	if !e.ExtraRulesChecked() || len(e.ExtraProblems()) != 2 {
		t.Fatal("the answer for the rows is not kept")
	}
	got := e.Problems()
	if len(got) != 1 || got[0].Row != 1 {
		t.Errorf("Problems = %+v, want the error only", got)
	}

	// An answer for other rows is out of date: it is not listed.
	e.SetExtraRules([]string{"bogus", "net none", "more"})
	if e.ExtraRulesChecked() || len(e.Problems()) != 0 || len(e.ExtraProblems()) != 0 {
		t.Error("an out-of-date answer is still listed")
	}

	// So is the answer for the old profile.
	e.SetExtraRules(rows)
	if !e.ExtraRulesChecked() {
		t.Fatal("the same rows are not the checked ones")
	}
	base, p := f.base("code.profile")
	e.SwitchProfile(f.path("code.profile"), base, p, true)
	if e.ExtraRulesChecked() || len(e.Problems()) != 0 {
		t.Error("the answer survived a switch of the base profile")
	}

	// The settings' problems are listed with the errors of the rows.
	e.SetExtraProblems(rows, problems)
	e.Set(func(s *firejail.Settings) { s.SandboxName = "Bad Name" })
	if got := e.Problems(); len(got) != 2 || got[0].Tag != firejail.ProblemSandboxName || got[1].Row != 1 {
		t.Errorf("Problems = %+v", got)
	}
}

func TestBaseGroups(t *testing.T) {
	line := func(file string, n int, text, cond string) firejail.Line {
		return firejail.Line{File: file, Number: n, Text: text, Conditional: cond}
	}
	base := firejail.Base{
		Uncovered: []firejail.UncoveredGroup{
			{File: "/etc/firejail/a.inc", Lines: []firejail.Line{line("/etc/firejail/a.inc", 3, "blacklist /x", ""), line("/etc/firejail/a.inc", 9, "noexec /y", "HAS_NET")}},
			{File: "/etc/firejail/b.profile", Lines: []firejail.Line{line("/etc/firejail/b.profile", 1, "tracelog", "")}},
		},
		Conditional: []firejail.Skipped{
			{Line: line("/etc/firejail/a.inc", 5, "nosound", "HAS_NOSOUND"), Reason: "no sound line"},
			{Line: line("/etc/firejail/c.inc", 2, "x11 none", "HAS_X11,HAS_NET"), Reason: "x11 is off"},
		},
	}
	got := baseGroups(base)
	var files []string
	for _, g := range got {
		files = append(files, g.file)
	}
	if want := []string{"/etc/firejail/a.inc", "/etc/firejail/b.profile", "/etc/firejail/c.inc"}; !slices.Equal(files, want) {
		t.Fatalf("files = %q, want %q", files, want)
	}
	a := got[0].lines
	if len(a) != 3 || a[0].number != 3 || a[1].number != 5 || a[2].number != 9 {
		t.Fatalf("group a is %+v, want the lines 3, 5, 9 in order", a)
	}
	if a[0].inactive || !a[1].inactive || a[1].reason != "no sound line" || a[2].inactive {
		t.Errorf("only line 5 is inactive: %+v", a)
	}
	if a[1].text != "?HAS_NOSOUND: nosound" || a[2].text != "?HAS_NET: noexec /y" {
		t.Errorf("conditional lines keep their prefix: %q, %q", a[1].text, a[2].text)
	}
	if c := got[2].lines[0]; c.text != "?HAS_X11: ?HAS_NET: x11 none" || !c.inactive {
		t.Errorf("a line under two conditions is %+v", c)
	}
	if len(baseGroups(firejail.BaseNone())) != 0 {
		t.Error("no base has groups")
	}
}

func TestExtraRulesTabBaseGroupsOfVSCode(t *testing.T) {
	p := extraPage(t, "Visual Studio Code")
	e := p.home().editor
	groups := baseGroups(e.Base())
	if len(groups) != 6 {
		t.Fatalf("the base has %d groups, want 6 (the VS Code chain of the fixture)", len(groups))
	}
	if e.BaseProfile() != filepath.Join(p.backend.fj.dirs.FirejailSystem, "code.profile") {
		t.Fatalf("base profile %q", e.BaseProfile())
	}
	// Every group has a header with its file; the file's path is shown.
	for i, g := range groups {
		header := key("base-group:" + strconv.Itoa(i))
		if !p.ts.Exists(gluttertest.Descendant(header, gluttertest.ByTextContaining(filepath.Base(g.file)))) {
			t.Errorf("group %d has no header with %s", i, filepath.Base(g.file))
		}
	}
	if !p.ts.Exists(gluttertest.Descendant(key("base-group:3"), gluttertest.ByText("620 lines"))) ||
		!p.ts.Exists(gluttertest.Descendant(key("base-group:1"), gluttertest.ByText("1 line"))) {
		t.Error("the headers do not count the lines")
	}

	// Only the first group is open.
	first := groups[0].lines
	if !p.ts.Exists(key(fmt.Sprintf("base-line:0:%d", first[0].number))) || !p.ts.Exists(key(fmt.Sprintf("base-line:0:%d", first[len(first)-1].number))) {
		t.Error("the lines of the first group are not shown")
	}
	for i := 1; i < len(groups); i++ {
		if p.ts.Exists(key(fmt.Sprintf("base-line:%d:%d", i, groups[i].lines[0].number))) {
			t.Errorf("group %d is open before the user opened it", i)
		}
	}
	// A line is its number and its text.
	row := key(fmt.Sprintf("base-line:0:%d", first[0].number))
	if !p.ts.Exists(gluttertest.Descendant(row, gluttertest.ByText(strconv.Itoa(first[0].number)))) ||
		!p.ts.Exists(gluttertest.Descendant(row, gluttertest.ByText(first[0].text))) {
		t.Errorf("the row of line %d does not show its number and text %q", first[0].number, first[0].text)
	}

	// Opening the second group shows its line; closing the first hides its lines.
	p.tap("base-group:1")
	if !p.ts.Exists(key(fmt.Sprintf("base-line:1:%d", groups[1].lines[0].number))) {
		t.Error("opening a group does not show its line")
	}
	p.tap("base-group:0")
	if p.ts.Exists(key(fmt.Sprintf("base-line:0:%d", first[0].number))) {
		t.Error("closing the first group leaves its lines")
	}
	if !p.ts.Exists(key("base-group:0")) || !p.ts.Exists(key(fmt.Sprintf("base-line:1:%d", groups[1].lines[0].number))) {
		t.Error("closing a group changed the others")
	}
	p.tap("base-group:0")
	if !p.ts.Exists(key(fmt.Sprintf("base-line:0:%d", first[0].number))) {
		t.Error("the first group does not open again")
	}
	// What the user changed is kept when the rows change.
	p.typeRows("ignore apparmor")
	if !p.ts.Exists(key(fmt.Sprintf("base-line:1:%d", groups[1].lines[0].number))) {
		t.Error("typing closed a group")
	}
}

// bigBase is a base with a group of n lines and a small group after it.
func bigBase(n int) firejail.Base {
	base := firejail.BaseNone()
	var lines []firejail.Line
	for i := 1; i <= n; i++ {
		lines = append(lines, firejail.Line{File: "/etc/firejail/big.inc", Number: i, Text: "blacklist /big/" + strconv.Itoa(i)})
	}
	base.Uncovered = []firejail.UncoveredGroup{
		{File: "/etc/firejail/big.inc", Lines: lines},
		{File: "/etc/firejail/small.inc", Lines: []firejail.Line{{File: "/etc/firejail/small.inc", Number: 1, Text: "tracelog"}}},
	}
	return base
}

func TestExtraRulesTabBaseListIsVirtualized(t *testing.T) {
	const lines = 3000
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend) // the window of the goldens: 1100x720
	p.ready()
	p.openApp("Calculator")
	p.selectTab(TabExtraRules)
	p.change(func(e *Editor) { e.SwitchProfile("", bigBase(lines), nil, true) })

	built := p.countKeyed("base-line:")
	if built == 0 || built > 60 {
		t.Fatalf("%d of %d lines are built; a lazy list builds only the rows on screen", built, lines)
	}
	if !p.ts.Exists(key("base-line:0:1")) || p.ts.Exists(key("base-line:0:"+strconv.Itoa(lines))) {
		t.Fatal("the list does not start at the first line")
	}

	// Scrolling the tab builds the lines that come into view and drops the ones that left.
	s := p.extraState()
	max := s.scroll.Position().MaxScrollExtent()
	want := float64(lines)*baseItemExtent + 4*baseItemExtent
	if max < want-2000 {
		t.Fatalf("the tab can scroll only %v pixels for %d lines", max, lines)
	}
	s.scroll.JumpTo(max / 2)
	p.ts.Pump()
	if built := p.countKeyed("base-line:"); built == 0 || built > 60 {
		t.Errorf("%d lines are built in the middle", built)
	}
	if p.ts.Exists(key("base-line:0:1")) {
		t.Error("the first line is still built in the middle of the list")
	}
	mid := lines / 2
	if !p.ts.Exists(key("base-line:0:" + strconv.Itoa(mid))) {
		// The middle of the scroll is near the middle of the lines.
		found := false
		for d := -40; d <= 40 && !found; d++ {
			found = p.ts.Exists(key("base-line:0:" + strconv.Itoa(mid+d)))
		}
		if !found {
			t.Error("no line near the middle is built after scrolling to the middle")
		}
	}

	s.scroll.JumpTo(max)
	p.ts.Pump()
	if !p.ts.Exists(key("base-group:1")) {
		t.Error("the header of the last group is not reached at the end")
	}
	if p.ts.Exists(key("base-line:0:1")) || p.countKeyed("base-line:") > 60 {
		t.Error("the end of the list builds too much")
	}
	if !p.ts.Exists(key("base-line:0:" + strconv.Itoa(lines))) {
		t.Error("the last line of the big group is not built at the end")
	}

	// Closing the big group leaves its header and the other group's.
	s.scroll.JumpTo(0)
	p.ts.Pump()
	p.tap("base-group:0")
	if p.countKeyed("base-line:") != 0 || !p.ts.Exists(key("base-group:1")) {
		t.Error("closing the big group leaves its lines")
	}
}

func TestExtraRulesTabBaseConditionalLines(t *testing.T) {
	p := extraPage(t, "Calculator")
	base := firejail.BaseNone()
	file := "/etc/firejail/cond.inc"
	base.Uncovered = []firejail.UncoveredGroup{{File: file, Lines: []firejail.Line{{File: file, Number: 1, Text: "blacklist /a"}}}}
	base.Conditional = []firejail.Skipped{{
		Line:   firejail.Line{File: file, Number: 2, Text: "nosound", Conditional: "HAS_NET"},
		Reason: "HAS_NET is false",
	}}
	p.change(func(e *Editor) { e.SwitchProfile("", base, nil, true) })
	if !p.ts.Exists(key("base-line:0:1")) || p.ts.Exists(key("base-inactive:0:1")) {
		t.Error("a line that counts is marked")
	}
	if !p.ts.Exists(gluttertest.Descendant(key("base-inactive:0:2"), gluttertest.ByText(baseInactiveText))) {
		t.Fatal("a conditional line that is not active is not marked")
	}
	if !p.ts.Exists(gluttertest.Descendant(key("base-line:0:2"), gluttertest.ByText("?HAS_NET: nosound"))) {
		t.Error("the conditional line does not show its condition")
	}
}

func TestExtraRulesTabEmptyBase(t *testing.T) {
	p := extraPage(t, "Calculator")
	p.change(func(e *Editor) { e.SwitchProfile("", firejail.BaseNone(), nil, true) })
	if !p.textHas("base-empty", "no base profile") || p.countKeyed("base-group:") != 0 {
		t.Error("base None does not say there is nothing to list")
	}
	p.change(func(e *Editor) { e.SwitchProfile("/some/profile", firejail.BaseNone(), nil, true) })
	if !p.textHas("base-empty", "covered by a setting") {
		t.Error("a profile with no free lines does not say so")
	}
}

// goldenExtraRulesPage opens the Extra rules tab of Visual Studio Code with one warning (row
// 1) and one error (row 3). The file paths of the fixture are in a temporary folder: they are
// shown as /etc/firejail, and one conditional line that is not active is added, which the
// fixture has none of.
func goldenExtraRulesPage(t *testing.T) *page {
	t.Helper()
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ready()
	p.openApp("Visual Studio Code")
	p.clearSearch()
	p.selectTab(TabExtraRules)
	p.change(func(e *Editor) {
		base := e.Base()
		sys := p.backend.fj.dirs.FirejailSystem
		var groups []firejail.UncoveredGroup
		for _, g := range base.Uncovered {
			ng := firejail.UncoveredGroup{File: strings.Replace(g.File, sys, "/etc/firejail", 1)}
			for _, l := range g.Lines {
				l.File = ng.File
				ng.Lines = append(ng.Lines, l)
			}
			groups = append(groups, ng)
		}
		base.Uncovered = groups
		base.Conditional = []firejail.Skipped{{
			Line:   firejail.Line{File: groups[0].File, Number: 12, Text: "nosound", Conditional: "HAS_NET"},
			Reason: "HAS_NET is false",
		}}
		e.SwitchProfile(e.BaseProfile(), base, e.Profile(), true)
	})
	p.typeInto("extra:field", "net none\nnoblacklist /mnt\nfrobnicate")
	p.waitFor("the check", func() bool { return p.home().editor.ExtraRulesChecked() && len(p.home().editor.ExtraProblems()) == 2 })
	return p
}

func TestGoldenExtraRulesTab(t *testing.T) {
	p := goldenExtraRulesPage(t)
	// Scrolled until the first lines of the base profile are in view, with the rows and
	// their problems still above them.
	p.extraState().scroll.JumpTo(90)
	p.ts.Pump()
	p.golden("extra-rules-tab", "4.6-extra-rules.png")
}

func TestGoldenExtraRulesGroups(t *testing.T) {
	p := goldenExtraRulesPage(t)
	// The end of the tab: the other five files of the chain, closed.
	s := p.extraState()
	s.scroll.JumpTo(s.scroll.Position().MaxScrollExtent())
	p.ts.Pump()
	p.golden("extra-rules-groups", "")
}
