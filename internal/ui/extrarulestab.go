package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Extra rules tab the user reads (SPEC §7.9).
const (
	extraTitleText     = "Extra rules"
	extraIntroText     = "Firejail profile lines of your own, one in each row, written after the settings of the other tabs. For example: ignore nosound, noblacklist /mnt or env EDITOR=vim. A line a setting covers is better changed in its tab. Rows with errors stop Try and Build."
	baseTitleText      = "From the base profile"
	baseIntroText      = "The lines of the base profile and the files it includes that no setting covers. You cannot edit them here. They change when firejail is updated, in your own .local files, or with ignore and noblacklist rows above."
	baseNoProfileText  = "There is no base profile, so there are no lines to list."
	baseNoLinesText    = "Every line of the base profile is covered by a setting."
	baseInactiveText   = "conditional (not active)"
	extraFieldMinLines = 6
	// maxLinesUnlimited is the MaxLines of a text field that grows with its text.
	maxLinesUnlimited = 0
	// monoFamily is the generic name of the system's monospace font.
	monoFamily = "monospace"
	// monoSize and monoHeight give the field and the row numbers the same line height.
	monoSize   = 13.0
	monoHeight = 1.6
	// baseItemExtent is the height of a row of the list of the base profile.
	baseItemExtent = 26.0
)

// ExtraRulesTabOpts are the optional parameters of [NewExtraRulesTab].
type ExtraRulesTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// ExtraRulesTab is the Extra rules tab of the build editor (SPEC §7.9): a text box for
// profile lines, one in each row, with the problems of each row under it, and the
// read-only lines of the base profile, grouped by file. The rows are checked off the UI
// thread as the user types. The list of the base profile is lazy: only the rows on screen
// are built, so the thousands of lines of a big profile cost nothing. It edits an
// [Editor]; the whole tab scrolls.
type ExtraRulesTab struct {
	m.StatefulWidget
	editor  *Editor
	backend Backend
	change  func(apply func())
}

// NewExtraRulesTab creates the tab for editor. backend checks the rows. change runs a
// function that changes the editor (or the tab) inside the SetState of the page that
// shows the tab, and may be called from any goroutine.
func NewExtraRulesTab(editor *Editor, backend Backend, change func(apply func()), opts ...ExtraRulesTabOpts) *ExtraRulesTab {
	var o ExtraRulesTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &ExtraRulesTab{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), editor: editor, backend: backend, change: change}
}

// CreateState creates the state of the tab.
func (w *ExtraRulesTab) CreateState() m.State { return &extraRulesTabState{} }

// rowsOfText splits the text of the box into rows. The empty rows at the end are not
// rows: the text a user is about to continue on a new line is not a change.
func rowsOfText(text string) []string {
	rows := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1]) == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

type extraRulesTabState struct {
	m.StateOf[*ExtraRulesTab]
	scroll *m.ScrollControllerBase
	ctl    *m.TextEditingController
	// problems are the answer of the last check; checkSeq numbers the checks.
	problems []firejail.Problem
	checkSeq int
	// checkedBase is the base profile the last check was made for.
	checkedBase string
	// toggled holds what the user did to a group of the base profile list: file -> open.
	toggled map[string]bool
}

func (s *extraRulesTabState) InitState() {
	e := s.Widget().editor
	s.scroll = m.NewScrollController()
	s.ctl = m.NewTextEditingController()
	s.ctl.SetText(strings.Join(e.ExtraRules(), "\n"))
	s.toggled = map[string]bool{}
	s.problems = e.ExtraProblems()
	if !e.ExtraRulesChecked() {
		s.check()
	}
	s.checkedBase = e.BaseProfile()
}

func (s *extraRulesTabState) Dispose() {
	s.scroll.Dispose()
	s.ctl.Dispose()
}

// DidUpdateWidget puts the editor's rows in the box when something other than typing
// changed them (Discard draft), and checks the rows again when the base profile changed.
func (s *extraRulesTabState) DidUpdateWidget(*ExtraRulesTab) {
	e := s.Widget().editor
	rows := e.ExtraRules()
	if !slices.Equal(rows, rowsOfText(s.ctl.Text())) {
		s.ctl.SetText(strings.Join(rows, "\n"))
		s.check()
	} else if e.BaseProfile() != s.checkedBase {
		s.check()
	}
	s.checkedBase = e.BaseProfile()
}

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *extraRulesTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

// check asks the backend about the rows, off the UI thread. An answer for rows that are
// not the editor's any more is dropped.
func (s *extraRulesTabState) check() {
	w := s.Widget()
	rows, base := w.editor.ExtraRules(), w.editor.Base()
	s.checkSeq++
	seq := s.checkSeq
	s.Go(func(ctx context.Context) {
		found := w.backend.CheckExtraRules(ctx, rows, base)
		if ctx.Err() != nil {
			return
		}
		w.change(func() {
			if s.checkSeq != seq {
				return
			}
			s.problems = found
			w.editor.SetExtraProblems(rows, found)
		})
	})
}

// typed stores the rows of the box and checks them.
func (s *extraRulesTabState) typed(text string) {
	rows := rowsOfText(text)
	s.apply(func(e *Editor) {
		e.SetExtraRules(rows)
		s.check()
	})
}

func (s *extraRulesTabState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	e := w.editor
	groups := baseGroups(e.Base())

	head := []m.Widget{
		sectionTitle(bc, extraTitleText, nil),
		mutedText(bc, extraIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		s.editorBox(bc),
	}
	head = append(head, s.problemRows(bc)...)
	head = append(head, sectionTitle(bc, baseTitleText, nil), mutedText(bc, baseIntroText), m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}))
	if len(groups) == 0 {
		text := baseNoLinesText
		if e.BaseProfile() == "" {
			text = baseNoProfileText
		}
		head = append(head, mutedText(bc, text, m.TextOpts{Key: m.NewValueKey("base-empty")}))
	}

	slivers := []m.Widget{
		m.NewSliverPadding(m.NewEdgeInsetsSymmetric(8, 16), m.NewSliverToBoxAdapter(
			m.NewColumn(head, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}))),
	}
	if len(groups) > 0 {
		count := s.itemCount(groups)
		slivers = append(slivers, m.NewSliverPadding(m.NewEdgeInsetsSymmetric(0, 16), m.NewSliverFixedExtentListBuilder(
			func(bc m.BuildContext, i int) m.Widget { return s.baseItem(bc, groups, i) },
			baseItemExtent, m.SliverBuilderOpts{ItemCount: m.Some(count)})))
		slivers = append(slivers, m.NewSliverToBoxAdapter(m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(16.0)})))
	}
	return m.NewScrollbar(m.NewCustomScrollView(slivers, m.CustomScrollViewOpts{
		Key: m.NewValueKey("body:" + string(TabExtraRules)), Controller: s.scroll,
	}), m.ScrollbarOpts{Controller: s.scroll})
}

// monoStyle is the style of the rows and their numbers: the same size and line height, so
// each number stands beside its row.
func monoStyle(bc m.BuildContext, color m.Color) *m.TextStyle {
	return m.ThemeOf(bc).TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{
		FontFamily: monoFamily, FontSize: m.Some(monoSize), Height: m.Some(monoHeight), Color: m.Some(color),
	})
}

// editorBox is the text box with the number of each row beside it. The number of a row
// that has an error is red and of one that has a warning is amber.
func (s *extraRulesTabState) editorBox(bc m.BuildContext) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	lines := strings.Count(s.ctl.Text(), "\n") + 1
	lines = max(lines, extraFieldMinLines)
	severity := map[int]firejail.Severity{}
	for _, p := range s.problems {
		if cur, ok := severity[p.Row]; !ok || p.Severity == firejail.SeverityError && cur != firejail.SeverityError {
			severity[p.Row] = p.Severity
		}
	}
	numbers := make([]m.Widget, lines)
	for i := range numbers {
		color := scheme.OnSurfaceVariant()
		if sev, ok := severity[i+1]; ok {
			color = scheme.Tertiary()
			if sev == firejail.SeverityError {
				color = scheme.Error()
			}
		}
		numbers[i] = m.NewText(strconv.Itoa(i+1), m.TextOpts{
			Key: m.NewValueKey("extra:number:" + strconv.Itoa(i+1)), Style: monoStyle(bc, color), TextAlign: m.TextAlignRight, MaxLines: m.Some(1),
		})
	}
	style := monoStyle(bc, scheme.OnSurface())
	field := m.NewTextField(m.TextFieldOpts{
		Key: m.NewValueKey("extra:field"), Controller: s.ctl, NoDecoration: true, Style: style,
		MinLines: m.Some(extraFieldMinLines), MaxLines: m.Some(maxLinesUnlimited),
		OnChanged: s.typed,
	})
	return m.NewContainer(m.NewRow([]m.Widget{
		m.NewSizedBox(m.NewColumn(numbers, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}), m.SizedBoxOpts{Width: m.Some(36.0)}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
		m.NewExpanded(field),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}), m.ContainerOpts{
		Padding: m.NewEdgeInsetsAll(10),
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Border:       m.NewBorderAll(m.BorderSideOpts{Color: m.Some(scheme.Outline())}),
			BorderRadius: m.NewBorderRadiusCircular(8),
		}),
	})
}

// problemRows lists what the check found, one line each, with the severity icon.
func (s *extraRulesTabState) problemRows(bc m.BuildContext) []m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	if len(s.problems) == 0 {
		return nil
	}
	rows := []m.Widget{m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)})}
	for i, p := range s.problems {
		icon, color := m.Icons.Error(), scheme.Error()
		if p.Severity == firejail.SeverityWarning {
			icon, color = m.Icons.Warning(), scheme.Tertiary()
		}
		rows = append(rows, m.NewPadding(m.NewEdgeInsetsSymmetric(2, 0), m.NewRow([]m.Widget{
			m.NewIcon(icon, m.IconOpts{Size: m.Some(18.0), Color: m.Some(color)}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewExpanded(m.NewText(p.Message, m.TextOpts{Style: theme.TextTheme.BodySmall})),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}), m.PaddingOpts{Key: m.NewValueKey("extra:problem:" + strconv.Itoa(i))}))
	}
	return rows
}

// baseLine is one line of the read-only list.
type baseLine struct {
	number int
	text   string
	// inactive is true for a conditional line whose condition was false; reason says why.
	inactive bool
	reason   string
}

// baseGroup is the lines of one file of the read-only list.
type baseGroup struct {
	file  string
	lines []baseLine
}

// baseGroups returns the lines of the base profile that no setting covers, grouped by
// file in the order the files first appear: the lines that count, and the conditional
// lines that do not (marked inactive), each group ordered by line number.
func baseGroups(b firejail.Base) []baseGroup {
	var groups []baseGroup
	at := map[string]int{}
	add := func(file string, l baseLine) {
		i, ok := at[file]
		if !ok {
			i = len(groups)
			at[file] = i
			groups = append(groups, baseGroup{file: file})
		}
		groups[i].lines = append(groups[i].lines, l)
	}
	for _, g := range b.Uncovered {
		for _, l := range g.Lines {
			add(g.File, baseLine{number: l.Number, text: conditionalText(l)})
		}
	}
	for _, sk := range b.Conditional {
		add(sk.Line.File, baseLine{number: sk.Line.Number, text: conditionalText(sk.Line), inactive: true, reason: sk.Reason})
	}
	for i := range groups {
		slices.SortStableFunc(groups[i].lines, func(a, c baseLine) int { return a.number - c.number })
	}
	return groups
}

// conditionalText writes a line as the profile has it: a conditional line keeps its
// "?CONDITION:" prefix.
func conditionalText(l firejail.Line) string {
	if l.Conditional == "" {
		return l.Text
	}
	var sb strings.Builder
	for _, c := range strings.Split(l.Conditional, ",") {
		sb.WriteString("?" + c + ": ")
	}
	sb.WriteString(l.Text)
	return sb.String()
}

// open reports whether the group is open: the first one is until the user closes it, the
// others are closed until the user opens them.
func (s *extraRulesTabState) open(index int, g baseGroup) bool {
	if v, ok := s.toggled[g.file]; ok {
		return v
	}
	return index == 0
}

// itemCount is the number of rows of the list: a header for each group and the lines of
// the open groups.
func (s *extraRulesTabState) itemCount(groups []baseGroup) int {
	n := 0
	for i, g := range groups {
		n++
		if s.open(i, g) {
			n += len(g.lines)
		}
	}
	return n
}

// baseItem builds the row i of the list: a group's header or one of its lines.
func (s *extraRulesTabState) baseItem(bc m.BuildContext, groups []baseGroup, i int) m.Widget {
	home := s.Widget().editor.home
	for gi, g := range groups {
		if i == 0 {
			return s.groupHeader(bc, gi, g, home)
		}
		i--
		if !s.open(gi, g) {
			continue
		}
		if i < len(g.lines) {
			return s.lineRow(bc, gi, g.lines[i])
		}
		i -= len(g.lines)
	}
	return nil
}

// groupHeader is the row of a file: its path, the number of lines, and an arrow that
// tells whether the lines are shown.
func (s *extraRulesTabState) groupHeader(bc m.BuildContext, index int, g baseGroup, home string) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	open := s.open(index, g)
	arrow := m.Icons.ChevronRight()
	if open {
		arrow = m.Icons.ExpandMore()
	}
	count := strconv.Itoa(len(g.lines)) + " lines"
	if len(g.lines) == 1 {
		count = "1 line"
	}
	return m.NewInkWell(m.NewContainer(m.NewRow([]m.Widget{
		m.NewIcon(arrow, m.IconOpts{Size: m.Some(20.0), Color: m.Some(scheme.OnSurfaceVariant())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(4.0)}),
		m.NewFlexible(m.NewText(displayPlainPath(g.file, home), m.TextOpts{Style: theme.TextTheme.TitleSmall, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
		mutedText(bc, count),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}), m.ContainerOpts{
		Height: m.Some(baseItemExtent), Padding: m.NewEdgeInsetsSymmetric(0, 4),
	}), m.InkWellOpts{
		Key:   m.NewValueKey("base-group:" + strconv.Itoa(index)),
		OnTap: func() { s.SetState(func() { s.toggled[g.file] = !open }) },
	})
}

// lineRow is one line of a file: its number and its text.
func (s *extraRulesTabState) lineRow(bc m.BuildContext, group int, l baseLine) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	key := strconv.Itoa(group) + ":" + strconv.Itoa(l.number)
	row := []m.Widget{
		m.NewSizedBox(m.NewText(strconv.Itoa(l.number), m.TextOpts{Style: monoStyle(bc, scheme.OnSurfaceVariant()), TextAlign: m.TextAlignRight, MaxLines: m.Some(1)}),
			m.SizedBoxOpts{Width: m.Some(48.0)}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(16.0)}),
	}
	color := scheme.OnSurface()
	if l.inactive {
		color = scheme.OnSurfaceVariant()
	}
	row = append(row, m.NewFlexible(m.NewText(l.text, m.TextOpts{Style: monoStyle(bc, color), MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})))
	if l.inactive {
		row = append(row, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}),
			m.NewContainer(badge(bc, baseInactiveText, scheme.SecondaryContainer(), scheme.OnSecondaryContainer()), m.ContainerOpts{Key: m.NewValueKey("base-inactive:" + key)}))
	}
	return m.NewSizedBox(m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
		m.SizedBoxOpts{Key: m.NewValueKey("base-line:" + key), Height: m.Some(baseItemExtent)})
}
