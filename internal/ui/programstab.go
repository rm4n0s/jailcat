package ui

import (
	"slices"
	"strconv"
	"strings"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Programs tab the user reads (SPEC §7.6).
const (
	programsTitleText = "Programs and files"
	programsIntroText = "What the app finds in the system, the variables it starts with and the words you add to its command line."
	binSwitchText     = "Only these programs"
	binOffText        = "Off: the app sees every program of the system."
	binOnText         = "The app sees only the programs in the list, and its own. Names, not paths: git, not /usr/bin/git."
	binCheckNoteText  = "The check after Build runs with this switched off, so a missing program does not stop it. Press Try to see whether the app works."
	binEmptyText      = "No program yet: with an empty list the switch does nothing."
	binFieldLabel     = "Program"
	binFieldHint      = "git"
	binSlashText      = "Give the name of the program without a /, such as git. The app finds it in the usual folders."
	binDuplicateText  = "That program is in the list already."
	etcSwitchText     = "Only these /etc files"
	etcOffText        = "Off: the app sees every file in /etc."
	etcOnText         = "The app sees only the files and folders of /etc in the list. Names relative to /etc, such as hosts or ssl."
	etcEmptyText      = "No file yet: with an empty list the switch does nothing."
	etcFieldLabel     = "File or folder in /etc"
	etcFieldHint      = "ssl"
	etcDuplicateText  = "That name is in the list already."
	envTitleText      = "Environment variables"
	envIntroText      = "Variables the app starts with, on top of the ones your desktop gives it."
	envEmptyText      = "No variables."
	envNameLabel      = "Name"
	envNameHint       = "LANG"
	envValueLabel     = "Value"
	envValueHint      = "C"
	envDuplicateText  = " is in the list already. Remove it first to give it another value."
	argsTitleText     = "Extra arguments"
	argsIntroText     = "Words added to the app's command line before the ones you type when you start it. Each row is one word; it is quoted for the shell as shown."
	argsEmptyText     = "No extra arguments."
	argsFieldLabel    = "Argument"
	argsFieldHint     = "--disable-gpu"
	programsGap       = 8.0
)

// quoteForDisplay writes an extra argument the way the launcher will quote it. A word
// the launcher cannot quote (one with a NUL) is shown as it is: the list refuses it.
func quoteForDisplay(word string) string {
	q, err := firejail.ShellQuote(word)
	if err != nil {
		return word
	}
	return q
}

// listCheck makes the [StringListCheck] of a list of the tab: the item is trimmed (when
// trim is set), looked at by pre, refused when dup is not "" and it is in the list, and
// then checked by [firejail.Validate] with the item added by add, so the message is the
// one the build would give.
func listCheck(e *Editor, f firejail.Field, trim bool, pre func(item string) string, dup string, add func(st *firejail.Settings, item string)) StringListCheck {
	return func(text string, items []string) (item, problem string) {
		item = text
		if trim {
			item = strings.TrimSpace(text)
		}
		if pre != nil {
			if msg := pre(item); msg != "" {
				return "", msg
			}
		}
		if dup != "" && slices.Contains(items, item) {
			return "", dup
		}
		if msgs := candidateProblems(e, f, func(st *firejail.Settings) { add(st, item) }); len(msgs) > 0 {
			return "", msgs[0]
		}
		return item, ""
	}
}

// addTo returns a function that adds an item to a list field of the settings.
func addTo(list func(st *firejail.Settings) *[]string) func(st *firejail.Settings, item string) {
	return func(st *firejail.Settings, item string) {
		l := list(st)
		*l = append(slices.Clone(*l), item)
	}
}

// programsList describes one of the three string lists of the tab.
type programsList struct {
	field firejail.Field
	// prefix starts the keys of the list.
	prefix string
	list   func(st *firejail.Settings) *[]string
}

func binList() programsList {
	return programsList{firejail.FieldPrivateBin, "bin", func(st *firejail.Settings) *[]string { return &st.PrivateBin }}
}

func etcList() programsList {
	return programsList{firejail.FieldPrivateEtc, "etc", func(st *firejail.Settings) *[]string { return &st.PrivateEtc }}
}

func argsList() programsList {
	return programsList{firejail.FieldExtraArgs, "args", func(st *firejail.Settings) *[]string { return &st.ExtraArgs }}
}

// ProgramsTabOpts are the optional parameters of [NewProgramsTab].
type ProgramsTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// ProgramsTab is the Programs tab of the build editor (SPEC §7.6): the programs and the
// /etc files the app may see, its environment variables and the extra arguments of its
// command line. It edits an [Editor]; its content scrolls when it is taller than the
// window.
type ProgramsTab struct {
	m.StatefulWidget
	editor *Editor
	change func(apply func())
}

// NewProgramsTab creates the tab for editor. change runs a function that changes the
// editor inside the SetState of the page that shows the tab.
func NewProgramsTab(editor *Editor, change func(apply func()), opts ...ProgramsTabOpts) *ProgramsTab {
	var o ProgramsTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &ProgramsTab{StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}), editor: editor, change: change}
}

// CreateState creates the state of the tab.
func (w *ProgramsTab) CreateState() m.State { return &programsTabState{} }

type programsTabState struct {
	m.StateOf[*ProgramsTab]
	// binOn and etcOn say that the switch of the list is on while the list is empty: the
	// model cannot tell an empty list that is on from one that is off.
	binOn, etcOn bool
	// name and value are the fields of a new variable; envFailure is why the last one was
	// refused.
	name, value *m.TextEditingController
	envFailure  string
	scroll      *m.ScrollControllerBase
}

func (s *programsTabState) InitState() {
	s.scroll = m.NewScrollController()
	s.name = m.NewTextEditingController()
	s.value = m.NewTextEditingController()
}

func (s *programsTabState) Dispose() {
	s.scroll.Dispose()
	s.name.Dispose()
	s.value.Dispose()
}

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *programsTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

func (s *programsTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	children := []m.Widget{sectionTitle(bc, programsTitleText, nil), mutedText(bc, programsIntroText)}
	children = append(children, s.restrictedList(bc, e, final, binList(), &s.binOn, binSwitchText, binOffText, binOnText, binEmptyText,
		binFieldLabel, binFieldHint, listCheck(e, firejail.FieldPrivateBin, true,
			func(item string) string {
				if strings.Contains(item, "/") {
					return binSlashText
				}
				return ""
			}, binDuplicateText, addTo(binList().list)), binCheckNoteText)...)
	children = append(children, s.restrictedList(bc, e, final, etcList(), &s.etcOn, etcSwitchText, etcOffText, etcOnText, etcEmptyText,
		etcFieldLabel, etcFieldHint, listCheck(e, firejail.FieldPrivateEtc, true, nil, etcDuplicateText, addTo(etcList().list)), "")...)
	children = append(children, s.envSection(bc, e, final)...)
	children = append(children, s.argsSection(bc, e, final)...)
	return tabList(firejail.TabPrograms, s.scroll, children)
}

// restrictedList is a part of the tab that is off or a list: a switch with the mark, and
// the list when it is on. note, when it is not "", is a line under the list.
func (s *programsTabState) restrictedList(bc m.BuildContext, e *Editor, final firejail.Settings, l programsList, flag *bool,
	title, offText, onText, emptyText, label, hint string, check StringListCheck, note string) []m.Widget {
	items := *l.list(&final)
	on := len(items) > 0 || *flag
	text := offText
	if on {
		text = onText
	}
	out := []m.Widget{
		m.NewSwitchListTile(on, func(v bool) {
			s.apply(func(e *Editor) {
				*flag = v
				if !v {
					e.Set(func(st *firejail.Settings) { *l.list(st) = nil })
				}
			})
		}, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(l.field)),
			Title:    markedTitle(title, fieldMarkThen(e, l.field, s.apply, func() { *flag = false })),
			Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(3), Overflow: m.TextOverflowEllipsis}),
			Dense:    m.Some(true),
		}),
	}
	if !on {
		return out
	}
	out = append(out, NewStringList(items, check,
		func(item string) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { addTo(l.list)(st, item) }) })
		},
		func(i int) {
			s.apply(func(e *Editor) {
				e.Set(func(st *firejail.Settings) { *l.list(st) = slices.Delete(slices.Clone(*l.list(st)), i, i+1) })
			})
		},
		StringListOpts{KeyPrefix: l.prefix, Label: label, Hint: hint, Empty: emptyText}))
	for _, msg := range problemsOf(e, l.field) {
		out = append(out, problemLine(bc, msg, nil))
	}
	if note != "" {
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(8, 4), mutedText(bc, note, m.TextOpts{Key: m.NewValueKey("note:" + l.prefix)})))
	}
	return out
}

// submitEnv adds the variable of the two fields.
func (s *programsTabState) submitEnv() {
	e := s.Widget().editor
	name, value := strings.TrimSpace(s.name.Text()), s.value.Text()
	if name == "" && value == "" {
		return
	}
	v := firejail.EnvVar{Name: name, Value: value}
	failure := ""
	switch {
	case slices.ContainsFunc(e.Final().Env, func(x firejail.EnvVar) bool { return x.Name == name }):
		failure = "“" + name + "”" + envDuplicateText
	default:
		if msgs := candidateProblems(e, firejail.FieldEnv, func(st *firejail.Settings) { st.Env = append(slices.Clone(st.Env), v) }); len(msgs) > 0 {
			failure = msgs[0]
		}
	}
	if failure != "" {
		s.SetState(func() { s.envFailure = failure })
		return
	}
	s.name.Clear()
	s.value.Clear()
	s.SetState(func() { s.envFailure = "" })
	s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.Env = append(slices.Clone(st.Env), v) }) })
}

// envSection is the environment variables: a row for each, and the fields of a new one.
func (s *programsTabState) envSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	theme := m.ThemeOf(bc)
	out := []m.Widget{
		sectionTitle(bc, envTitleText, fieldMark(e, firejail.FieldEnv, s.apply)),
		mutedText(bc, envIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}),
	}
	if len(final.Env) == 0 {
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 4), mutedText(bc, envEmptyText)))
	}
	for i, v := range final.Env {
		i := i
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(0, 4), m.NewRow([]m.Widget{
			m.NewExpanded(m.NewText(v.Name+"="+v.Value, m.TextOpts{Style: theme.TextTheme.BodyMedium, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis})),
			m.NewIconButton(func() {
				s.apply(func(e *Editor) {
					e.Set(func(st *firejail.Settings) { st.Env = slices.Delete(slices.Clone(st.Env), i, i+1) })
				})
			}, m.NewIcon(m.Icons.Delete()), m.IconButtonOpts{
				Key: m.NewValueKey("env:remove:" + strconv.Itoa(i)), Tooltip: "Remove from the list",
			}),
		}, m.RowOpts{Key: m.NewValueKey("env:row:" + strconv.Itoa(i)), CrossAxisAlignment: m.CrossAxisAlignmentCenter})))
	}
	field := func(key string, c *m.TextEditingController, label, hint string) m.Widget {
		d := m.NewInputDecoration(m.InputDecorationOpts{HintText: m.Some(hint), LabelText: m.Some(label), Border: m.NewOutlineInputBorder(), IsDense: m.Some(true)})
		return m.NewTextField(m.TextFieldOpts{
			Key: m.NewValueKey(key), Controller: c, Decoration: &d,
			OnChanged:   func(string) { s.SetState(func() { s.envFailure = "" }) },
			OnSubmitted: func(string) { s.submitEnv() },
		})
	}
	out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
		m.NewExpanded(field("env:name", s.name, envNameLabel, envNameHint)),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(programsGap)}),
		m.NewExpanded(field("env:value", s.value, envValueLabel, envValueHint)),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(programsGap)}),
		m.NewOutlinedButton(s.submitEnv, m.NewText(addItemText), m.OutlinedButtonOpts{Key: m.NewValueKey("env:add")}),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})))
	if s.envFailure != "" {
		out = append(out, problemLine(bc, s.envFailure, m.NewValueKey("env:problem")))
	}
	for _, msg := range problemsOf(e, firejail.FieldEnv) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}

// argsSection is the extra arguments, shown as the launcher will quote them.
func (s *programsTabState) argsSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	l := argsList()
	out := []m.Widget{
		sectionTitle(bc, argsTitleText, fieldMark(e, firejail.FieldExtraArgs, s.apply)),
		mutedText(bc, argsIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}),
		NewStringList(final.ExtraArgs, listCheck(e, l.field, false, nil, "", addTo(l.list)),
			func(item string) {
				s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { addTo(l.list)(st, item) }) })
			},
			func(i int) {
				s.apply(func(e *Editor) {
					e.Set(func(st *firejail.Settings) { st.ExtraArgs = slices.Delete(slices.Clone(st.ExtraArgs), i, i+1) })
				})
			},
			StringListOpts{KeyPrefix: l.prefix, Label: argsFieldLabel, Hint: argsFieldHint, Empty: argsEmptyText, Display: quoteForDisplay}),
	}
	for _, msg := range problemsOf(e, l.field) {
		out = append(out, problemLine(bc, msg, nil))
	}
	return out
}
