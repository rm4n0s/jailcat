package ui

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	m "github.com/rm4n0s/glutter/material"
	"github.com/rm4n0s/glutter/svg"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Launcher tab the user reads (SPEC §7.8).
const (
	launcherTitleText      = "Launcher"
	launcherIntroText      = "How the sandboxed app is installed and found: its name, its place in the menu and its icon."
	sandboxNameLabel       = "Sandbox name"
	sandboxNameHelp        = "The command and the file name of the sandboxed app. Lowercase letters, digits and -."
	sandboxNameTakenText   = "Another build already has this name. Choose another one."
	sandboxNameCheckedText = "Jailcat could not check whether another build has this name: "
	privateHomeFollowsText = "Its own private home moves with the name: "
	menuNameLabel          = "Menu name"
	menuNameHelp           = "The name the application menu shows."
	menuNameEmptyText      = "A menu entry needs a name."
	commentLabel           = "Comment"
	commentHelp            = "The line of text under the name in some menus. It can be empty."
	iconTitleText          = "Icon"
	iconIntroText          = "The icon of the sandboxed app, so it is easy to tell from the original."
	iconBadgeTitle         = "App icon with badge"
	iconBadgeText          = "The app's own icon with the firejail badge over its corner."
	iconRandomTitle        = "Random icon"
	iconRandomText         = "An icon Jailcat makes for this build, with the badge. It stays the same on every rebuild."
	newRandomIconText      = "New random icon"
	iconFallbackPrefix     = "Jailcat made a random icon instead, because "
	iconFailedText         = "Jailcat could not make the icon: "
	installTitleText       = "Where it is installed"
	installMenuText        = "Adds the sandboxed app to your application menu."
	installBinText         = "Puts a command with the sandbox name in your bin folder, so you can start the app from a terminal."
	binNotOnPathText       = "The bin folder is not on your PATH, so the command cannot be started by its name in a terminal. Add the folder to PATH, or start the app from the menu."
	openFileTypesText      = "The sandboxed copy opens the file types of the original app. Off: your files keep opening in the original."
	singleInstanceText     = "Starting the app again joins the sandbox that already runs it, instead of starting a second sandbox."
	terminalText           = "Runs the app in a terminal window. Programs that print to a terminal need it."
	quietText              = "Hides the messages firejail prints when the sandbox starts and stops."
	launcherFieldWidth     = 480.0
	iconPreviewSize        = 96.0
)

// launcherSwitch describes one switch of the tab.
type launcherSwitch struct {
	field firejail.Field
	text  string
	get   func(s firejail.Settings) bool
	set   func(s *firejail.Settings, on bool)
}

// launcherSwitches are the switches of SPEC §7.8 under the icon, in order.
func launcherSwitches() []launcherSwitch {
	return []launcherSwitch{
		{firejail.FieldInstallMenu, installMenuText,
			func(s firejail.Settings) bool { return s.InstallMenu },
			func(s *firejail.Settings, on bool) { s.InstallMenu = on }},
		{firejail.FieldInstallBin, installBinText,
			func(s firejail.Settings) bool { return s.InstallBin },
			func(s *firejail.Settings, on bool) { s.InstallBin = on }},
		{firejail.FieldOpenFileTypes, openFileTypesText,
			func(s firejail.Settings) bool { return s.OpenFileTypes },
			func(s *firejail.Settings, on bool) { s.OpenFileTypes = on }},
		{firejail.FieldSingleInstance, singleInstanceText,
			func(s firejail.Settings) bool { return s.SingleInstance },
			func(s *firejail.Settings, on bool) { s.SingleInstance = on }},
		{firejail.FieldTerminal, terminalText,
			func(s firejail.Settings) bool { return s.Terminal },
			func(s *firejail.Settings, on bool) { s.Terminal = on }},
		{firejail.FieldQuiet, quietText,
			func(s firejail.Settings) bool { return s.Quiet },
			func(s *firejail.Settings, on bool) { s.Quiet = on }},
	}
}

// iconPreview is the icon the build will install, as the backend made it.
type iconPreview struct {
	// key says which settings the picture is for (see [previewKey]).
	key string
	// seq numbers the requests, so an answer that is no longer wanted is dropped.
	seq int
	// version changes with every new picture, so the widget draws it again.
	version int
	// svg is the picture; nil until the first answer.
	svg []byte
	// fallback says why a random icon was made instead of the app's icon.
	fallback string
	// failure is the message for the user when the icon could not be made.
	failure string
}

// previewKey is the part of the settings the preview depends on: the mode and the seed
// always, the menu name (the letter of a random icon) only when the picture can be random.
func previewKey(s firejail.Settings, random bool) string {
	key := fmt.Sprintf("%s/%d", s.Icon, s.IconSeed)
	if s.Icon == firejail.IconRandom || random {
		key += "/" + s.MenuName
	}
	return key
}

// LauncherTabOpts are the optional parameters of [NewLauncherTab].
type LauncherTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// LauncherTab is the Launcher tab of the build editor (SPEC §7.8): the sandbox name,
// the menu name and comment, the icon with its preview, and where the build is installed.
// A sandbox name is stored only when it is well formed and no other build has it, which
// the backend tells off the UI thread. It edits an [Editor]; its content scrolls when it
// is taller than the window.
type LauncherTab struct {
	m.StatefulWidget
	editor  *Editor
	backend Backend
	logger  *slog.Logger
	change  func(apply func())
}

// NewLauncherTab creates the tab for editor. backend checks the sandbox name against the
// other builds and makes the icon preview; logger gets what the user must not read.
// change runs a function that changes the editor (or the tab) inside the SetState of the
// page that shows the tab, and may be called from any goroutine.
func NewLauncherTab(editor *Editor, backend Backend, logger *slog.Logger, change func(apply func()), opts ...LauncherTabOpts) *LauncherTab {
	var o LauncherTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &LauncherTab{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		editor:         editor, backend: backend, logger: logger, change: change,
	}
}

// CreateState creates the state of the tab.
func (w *LauncherTab) CreateState() m.State { return &launcherTabState{} }

type launcherTabState struct {
	m.StateOf[*LauncherTab]
	scroll                    *m.ScrollControllerBase
	nameCtl, menuCtl, commCtl *m.TextEditingController
	// nameFocus is the focus of the sandbox name field; autofocusName says the field takes
	// the focus when it first shows, because the user was sent here to choose another name.
	nameFocus     *m.FocusNode
	autofocusName bool
	// nameProblem is why the typed sandbox name is not stored; "" for none.
	nameProblem string
	// namePending is true from the moment a well-formed name is typed until the backend
	// has said whether another build has it; nameSeq numbers the checks.
	namePending bool
	nameSeq     int
	preview     iconPreview
}

func (s *launcherTabState) InitState() {
	final := s.Widget().editor.Final()
	s.scroll = m.NewScrollController()
	s.nameCtl, s.menuCtl, s.commCtl = m.NewTextEditingController(), m.NewTextEditingController(), m.NewTextEditingController()
	s.nameCtl.SetText(final.SandboxName)
	s.menuCtl.SetText(final.MenuName)
	s.commCtl.SetText(final.Comment)
	s.nameFocus = m.NewFocusNode()
	s.autofocusName = s.Widget().editor.TakeNameFocus()
	s.refreshPreview()
}

func (s *launcherTabState) Dispose() {
	s.nameFocus.Dispose()
	s.scroll.Dispose()
	s.nameCtl.Dispose()
	s.menuCtl.Dispose()
	s.commCtl.Dispose()
}

// DidUpdateWidget puts the model's values in the fields when something other than typing
// changed them (Reset to profile, a draft), and makes the preview again when it changed.
// A sandbox name that is being checked or was refused stays in its field.
func (s *launcherTabState) DidUpdateWidget(*LauncherTab) {
	if s.Widget().editor.TakeNameFocus() {
		s.nameFocus.RequestFocus()
	}
	final := s.Widget().editor.Final()
	if !s.namePending && s.nameProblem == "" && s.nameCtl.Text() != final.SandboxName {
		s.nameCtl.SetText(final.SandboxName)
	}
	if s.menuCtl.Text() != final.MenuName {
		s.menuCtl.SetText(final.MenuName)
	}
	if s.commCtl.Text() != final.Comment {
		s.commCtl.SetText(final.Comment)
	}
	s.refreshPreview()
}

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *launcherTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

// refreshPreview asks the backend for the icon when the settings it depends on are not
// the ones of the picture the tab has. The old picture stays until the new one arrives.
func (s *launcherTabState) refreshPreview() {
	w := s.Widget()
	final := w.editor.Final()
	key := previewKey(final, s.preview.fallback != "")
	if key == s.preview.key {
		return
	}
	s.preview.key = key
	s.preview.seq++
	seq := s.preview.seq
	app := w.editor.App()
	s.Go(func(ctx context.Context) {
		svgBytes, fallback, err := w.backend.PreviewIcon(ctx, app, final)
		if ctx.Err() != nil {
			return
		}
		w.change(func() {
			if s.preview.seq != seq {
				return
			}
			s.preview.version++
			if err != nil {
				w.logger.Error("the icon preview failed", "app", app.ID, "err", err)
				s.preview.svg, s.preview.fallback, s.preview.failure = nil, "", plainReason(err)
				return
			}
			s.preview.svg, s.preview.fallback, s.preview.failure = svgBytes, fallback, ""
		})
	})
}

// nameTyped handles the sandbox name field changing. A name that is not well formed is
// explained; a well formed one is asked about off the UI thread and stored when no other
// build has it.
func (s *launcherTabState) nameTyped(text string) {
	w := s.Widget()
	e := w.editor
	s.nameSeq++
	seq := s.nameSeq
	if text == e.Final().SandboxName {
		s.SetState(func() { s.namePending, s.nameProblem = false, "" })
		return
	}
	if msgs := candidateProblems(e, firejail.FieldSandboxName, func(st *firejail.Settings) { st.SandboxName = text }); len(msgs) > 0 {
		s.SetState(func() { s.namePending, s.nameProblem = false, msgs[0] })
		return
	}
	s.SetState(func() { s.namePending, s.nameProblem = true, "" })
	id := e.Build().ID
	s.Go(func(ctx context.Context) {
		taken, err := w.backend.SandboxNameTaken(ctx, text, id)
		if ctx.Err() != nil {
			return
		}
		w.change(func() {
			if s.nameSeq != seq {
				return
			}
			s.namePending = false
			switch {
			case err != nil:
				w.logger.Error("checking the sandbox name failed", "name", text, "err", err)
				s.nameProblem = sandboxNameCheckedText + plainReason(err) + "."
			case taken:
				s.nameProblem = sandboxNameTakenText
			default:
				s.nameProblem = ""
				e.SetSandboxName(text)
			}
		})
	})
}

func (s *launcherTabState) Build(bc m.BuildContext) m.Widget {
	w := s.Widget()
	e := w.editor
	final := e.Final()
	children := []m.Widget{sectionTitle(bc, launcherTitleText, nil), mutedText(bc, launcherIntroText)}
	children = append(children, s.nameSection(bc, e, final)...)
	children = append(children, s.textSection(bc, e, firejail.FieldMenuName, menuNameLabel, menuNameHelp, "menu", s.menuCtl,
		func(st *firejail.Settings, text string) { st.MenuName = text }, menuNameProblem(final))...)
	children = append(children, s.textSection(bc, e, firejail.FieldComment, commentLabel, commentHelp, "comment", s.commCtl,
		func(st *firejail.Settings, text string) { st.Comment = text }, "")...)
	children = append(children, s.iconSection(bc, e, final)...)
	children = append(children, s.installSection(bc, e, final)...)
	return tabList(firejail.TabLauncher, s.scroll, children)
}

// menuNameProblem is what is wrong with the menu name; "" when nothing is.
func menuNameProblem(s firejail.Settings) string {
	if s.InstallMenu && strings.TrimSpace(s.MenuName) == "" {
		return menuNameEmptyText
	}
	return ""
}

// textField is a single-line field of the tab.
func textField(key, label string, ctl *m.TextEditingController, onChanged func(text string)) m.Widget {
	d := m.NewInputDecoration(m.InputDecorationOpts{LabelText: m.Some(label), Border: m.NewOutlineInputBorder(), IsDense: m.Some(true)})
	return m.NewTextField(m.TextFieldOpts{Key: m.NewValueKey(key), Controller: ctl, Decoration: &d, OnChanged: onChanged})
}

// nameField is the field of the sandbox name, which takes the focus when the user was sent
// here to choose another name.
func (s *launcherTabState) nameField() m.Widget {
	d := m.NewInputDecoration(m.InputDecorationOpts{LabelText: m.Some(sandboxNameLabel), Border: m.NewOutlineInputBorder(), IsDense: m.Some(true)})
	return m.NewTextField(m.TextFieldOpts{
		Key: m.NewValueKey("sandbox:field"), Controller: s.nameCtl, Decoration: &d, OnChanged: s.nameTyped,
		FocusNode: s.nameFocus, Autofocus: s.autofocusName,
	})
}

// fieldRow puts a field and the mark of its setting side by side.
func fieldRow(field m.Widget, mark m.Widget) m.Widget {
	row := []m.Widget{m.NewSizedBox(field, m.SizedBoxOpts{Width: m.Some(launcherFieldWidth)})}
	if mark != nil {
		row = append(row, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}), mark)
	}
	return m.NewPadding(m.NewEdgeInsetsFromLTRB(4, 4, 4, 2), m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
}

// nameSection is the sandbox name: its field, why a typed name is not stored, and the
// notes about a private home that follows it and a rename of an installed build.
func (s *launcherTabState) nameSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	out := []m.Widget{
		fieldRow(s.nameField(), nil),
		m.NewPadding(m.NewEdgeInsetsSymmetric(0, 8), mutedText(bc, sandboxNameHelp)),
	}
	if s.nameProblem != "" {
		out = append(out, problemLine(bc, s.nameProblem, m.NewValueKey("problem:sandbox")))
	}
	if e.PrivateHomeFollowsName() {
		out = append(out, m.NewPadding(m.NewEdgeInsetsFromLTRB(8, 2, 8, 0),
			mutedText(bc, privateHomeFollowsText+displayPlainPath(e.PrivateHomePath(), e.home)+".", m.TextOpts{Key: m.NewValueKey("note:private-home")})))
	}
	if old := e.RenamedFrom(); old != "" {
		out = append(out, infoNote(bc, "This build is installed as "+old+". Build will remove its files and install the build as "+final.SandboxName+".", m.NewValueKey("note:rename")))
	}
	return out
}

// textSection is a section of one text field that is stored as typed.
func (s *launcherTabState) textSection(bc m.BuildContext, e *Editor, f firejail.Field, label, help, prefix string,
	ctl *m.TextEditingController, set func(st *firejail.Settings, text string), problem string) []m.Widget {
	out := []m.Widget{
		fieldRow(textField(prefix+":field", label, ctl, func(text string) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { set(st, text) }) })
		}), fieldMark(e, f, s.apply)),
		m.NewPadding(m.NewEdgeInsetsSymmetric(0, 8), mutedText(bc, help)),
	}
	if problem != "" {
		out = append(out, problemLine(bc, problem, m.NewValueKey("problem:"+prefix)))
	}
	return out
}

// iconSection is the icon choice, the preview of the icon the build will install and, for
// a random icon, the button for a new one.
func (s *launcherTabState) iconSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	tile := func(mode firejail.IconMode, title, text string) m.Widget {
		return radioTile(bc, "icon:"+string(mode), mode, title, text, final.Icon == mode, true)
	}
	group := choiceGroup(final.Icon, m.NewColumn([]m.Widget{
		tile(firejail.IconBadge, iconBadgeTitle, iconBadgeText), tile(firejail.IconRandom, iconRandomTitle, iconRandomText),
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}),
		func(mode firejail.IconMode) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.Icon = mode }) })
		})

	side := []m.Widget{s.previewBox(bc)}
	if final.Icon == firejail.IconRandom {
		side = append(side, m.NewOutlinedButton(func() {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { st.IconSeed = rand.Int64() }) })
		}, m.NewText(newRandomIconText), m.OutlinedButtonOpts{Key: m.NewValueKey("icon:new-random")}))
	}
	out := []m.Widget{
		sectionTitle(bc, iconTitleText, fieldMark(e, firejail.FieldIcon, s.apply)),
		mutedText(bc, iconIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		m.NewRow([]m.Widget{
			m.NewExpanded(group),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(16.0)}),
			m.NewColumn(spaced(side, 8), m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}),
	}
	switch {
	case s.preview.failure != "":
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 8), warningText(bc, iconFailedText+s.preview.failure+".", m.NewValueKey("icon:failed"))))
	case s.preview.fallback != "" && final.Icon == firejail.IconBadge:
		out = append(out, m.NewPadding(m.NewEdgeInsetsSymmetric(4, 8),
			mutedText(bc, iconFallbackPrefix+s.preview.fallback+".", m.TextOpts{Key: m.NewValueKey("icon:fallback"), MaxLines: m.Some(4), Overflow: m.TextOverflowEllipsis})))
	}
	return out
}

// spaced puts a gap between the widgets.
func spaced(items []m.Widget, gap float64) []m.Widget {
	var out []m.Widget
	for i, it := range items {
		if i > 0 {
			out = append(out, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(gap)}))
		}
		out = append(out, it)
	}
	return out
}

// previewBox is the picture of the icon the build will install, in a rounded box.
func (s *launcherTabState) previewBox(bc m.BuildContext) m.Widget {
	scheme := m.ThemeOf(bc).ColorScheme
	var child m.Widget
	switch {
	case s.preview.svg != nil:
		child = svg.NewSvgPictureMemory(s.preview.svg, svg.SvgPictureOpts{
			Key:                  m.NewValueKey(fmt.Sprintf("icon:picture:%d", s.preview.version)),
			Width:                m.Some(iconPreviewSize),
			Height:               m.Some(iconPreviewSize),
			ExcludeFromSemantics: true,
			ErrorBuilder:         func(m.BuildContext, error) m.Widget { return genericIcon(s.Widget().editor.App(), iconPreviewSize) },
		})
	case s.preview.failure != "":
		child = genericIcon(s.Widget().editor.App(), iconPreviewSize)
	default:
		child = m.NewCenter(m.NewSizedBox(m.NewCircularProgressIndicator(), m.SizedBoxOpts{Width: m.Some(24.0), Height: m.Some(24.0)}))
	}
	return m.NewContainer(child, m.ContainerOpts{
		Key:     m.NewValueKey("icon:preview"),
		Width:   m.Some(iconPreviewSize + 16),
		Height:  m.Some(iconPreviewSize + 16),
		Padding: m.NewEdgeInsetsAll(8),
		Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
			Color:        m.Some(scheme.SurfaceContainerHighest()),
			BorderRadius: m.NewBorderRadiusCircular(16),
		}),
	})
}

// installSection is where the build is installed and the other options, as switches.
func (s *launcherTabState) installSection(bc m.BuildContext, e *Editor, final firejail.Settings) []m.Widget {
	out := []m.Widget{sectionTitle(bc, installTitleText, nil)}
	for _, sw := range launcherSwitches() {
		sw := sw
		out = append(out, m.NewSwitchListTile(sw.get(final), func(on bool) {
			s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { sw.set(st, on) }) })
		}, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(sw.field)),
			Title:    markedTitle(settingLabel(sw.field), fieldMark(e, sw.field, s.apply)),
			Subtitle: m.NewText(sw.text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:    m.Some(true),
		}))
		switch sw.field {
		case firejail.FieldInstallMenu:
			for _, msg := range problemsOf(e, firejail.FieldInstallMenu) {
				out = append(out, problemLine(bc, msg, m.NewValueKey("problem:install")))
			}
		case firejail.FieldInstallBin:
			if final.InstallBin && !s.Widget().backend.BinOnPath() {
				out = append(out, infoNote(bc, binNotOnPathText, m.NewValueKey("note:bin-path")))
			}
		}
	}
	return out
}
