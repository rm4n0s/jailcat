package ui

import (
	"context"
	"log/slog"
	"strings"

	"github.com/rm4n0s/glutter/desktop"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the Files tab the user reads (SPEC §7.1).
const (
	homeTitleText        = "Home folder"
	homeIntroText        = "What the app sees as its home folder. This is the most important choice of a build."
	homeFullTitle        = "Full home"
	homeFullText         = "The app sees your whole home, minus what the profile hides"
	homeChosenTitle      = "Only the folders I choose"
	homeChosenText       = "Only the folders below; the rest of your home looks empty"
	homeEmptyTitle       = "Empty home"
	homeEmptyText        = "A new empty home every time; nothing is kept"
	homePrivateTitle     = "Own private home"
	foldersTitleText     = "Folders"
	addFolderText        = "Add folder…"
	typePathText         = "Type a path…"
	noFoldersText        = "No folders in the list."
	fromProfileText      = "from profile"
	removedText          = "removed"
	restoreText          = "Restore"
	togglesTitleText     = "Quick settings"
	readWriteText        = "Read-write"
	readOnlyText         = "Read-only"
	hiddenText           = "Hidden"
	chooserTitleText     = "Choose folders for the app"
	chooserAcceptText    = "Add"
	chooserFailedText    = "The file chooser of your desktop could not be opened. Use Type a path… instead."
	downloadsOffReason   = "Not available with an empty or own private home: the Downloads folder is in your real home, which they keep out of the sandbox."
	lockedReasonText     = "Locked: the profile lets the app see this folder read-only (whitelist-ro), so it cannot be loosened or removed."
	foldersHintFull      = "What the app may do with each folder. Folders that are not listed keep the profile's rules."
	foldersHintChosen    = "The app sees only these folders (and the ones the profile lists); the rest of your home looks empty."
	foldersHintKeepsOut  = "Only folders outside your home can be added: an empty or own private home keeps the real home out of the sandbox."
	pathDialogTitleText  = "Type a folder path"
	pathDialogHintText   = "/mnt/data, ~/Projects or ${DOWNLOADS}/books"
	pathDialogAddText    = "Add"
	pathDialogFieldLabel = "Folder"
)

// homeTexts returns the title and the one-line explanation of a home choice.
func homeTexts(mode firejail.HomeMode) (title, text string) {
	switch mode {
	case firejail.HomeFull:
		return homeFullTitle, homeFullText
	case firejail.HomeChosen:
		return homeChosenTitle, homeChosenText
	case firejail.HomeEmpty:
		return homeEmptyTitle, homeEmptyText
	}
	return homePrivateTitle, ""
}

// accessLabel is how a folder row's access level is written on the dropdown.
func accessLabel(a firejail.Access) string {
	switch a {
	case firejail.AccessReadWrite:
		return readWriteText
	case firejail.AccessReadOnly:
		return readOnlyText
	case firejail.AccessHidden:
		return hiddenText
	}
	return string(a)
}

// toggle describes one of the quick settings.
type toggle struct {
	field firejail.Field
	text  string
	get   func(s firejail.Settings) bool
	set   func(s *firejail.Settings, on bool)
}

// quickToggles are the quick settings of the tab, in the order of SPEC §7.1.
func quickToggles() []toggle {
	return []toggle{
		{firejail.FieldDownloadsReadWrite, "The app can read and write your Downloads folder.",
			func(s firejail.Settings) bool { return s.DownloadsReadWrite },
			func(s *firejail.Settings, on bool) { s.DownloadsReadWrite = on }},
		{firejail.FieldHideRemovable, "USB sticks and other drives under /mnt, /media and /run/media are not visible.",
			func(s firejail.Settings) bool { return s.HideRemovable },
			func(s *firejail.Settings, on bool) { s.HideRemovable = on }},
		{firejail.FieldNoExecHome, "Files in your home cannot be run as programs.",
			func(s firejail.Settings) bool { return s.NoExecHome },
			func(s *firejail.Settings, on bool) { s.NoExecHome = on }},
		{firejail.FieldPrivateTmp, "The app gets its own empty /tmp, thrown away when it closes.",
			func(s firejail.Settings) bool { return s.PrivateTmp },
			func(s *firejail.Settings, on bool) { s.PrivateTmp = on }},
		{firejail.FieldPrivateDev, "The app sees only a short list of devices in /dev.",
			func(s firejail.Settings) bool { return s.PrivateDev },
			func(s *firejail.Settings, on bool) { s.PrivateDev = on }},
		{firejail.FieldPrivateCache, "The app gets an empty cache folder (~/.cache), thrown away when it closes.",
			func(s firejail.Settings) bool { return s.PrivateCache },
			func(s *firejail.Settings, on bool) { s.PrivateCache = on }},
	}
}

// FilesTabOpts are the optional parameters of [NewFilesTab].
type FilesTabOpts struct {
	// Key is the key of the widget.
	Key m.Key
}

// FilesTab is the Files tab of the build editor (SPEC §7.1): the home folder choice at
// the top, the folder list with **Add folder…** and **Type a path…**, and the quick
// toggles. It edits an [Editor]; its content scrolls when it is taller than the window.
type FilesTab struct {
	m.StatefulWidget
	editor *Editor
	logger *slog.Logger
	change func(apply func())
}

// NewFilesTab creates the tab for editor. change runs a function that changes the
// editor (or the tab) inside the SetState of the page that shows the tab, so the page
// redraws its dots and problems; it may be called from any goroutine. logger gets what
// the user must not read.
func NewFilesTab(editor *Editor, logger *slog.Logger, change func(apply func()), opts ...FilesTabOpts) *FilesTab {
	var o FilesTabOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &FilesTab{
		StatefulWidget: m.NewStatefulWidgetBase(m.WidgetOpts{Key: o.Key}),
		editor:         editor, logger: logger, change: change,
	}
}

// CreateState creates the state of the tab.
func (w *FilesTab) CreateState() m.State { return &filesTabState{} }

type filesTabState struct {
	m.StateOf[*FilesTab]
	// message is why a folder was not added, or why the chooser failed; "" for none.
	message string
	// choosing is true while the folder chooser is open.
	choosing bool
	// scroll scrolls the tab and its scrollbar.
	scroll *m.ScrollControllerBase
}

func (s *filesTabState) InitState() { s.scroll = m.NewScrollController() }

func (s *filesTabState) Dispose() { s.scroll.Dispose() }

// apply runs fn as a change of the editor, in the SetState of the page.
func (s *filesTabState) apply(fn func(e *Editor)) {
	w := s.Widget()
	w.change(func() { fn(w.editor) })
}

// chooseFolders asks for folders with the chooser of the desktop and adds them.
func (s *filesTabState) chooseFolders() {
	if s.choosing {
		return
	}
	w := s.Widget()
	bc := s.Context()
	w.change(func() { s.choosing, s.message = true, "" })
	s.Go(func(ctx context.Context) {
		res, err := desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{
			Title: chooserTitleText, AcceptLabel: chooserAcceptText, Directory: true, Multiple: true,
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.Error("the file chooser failed", "err", err)
			w.change(func() { s.choosing, s.message = false, chooserFailedText })
			return
		}
		w.change(func() {
			s.choosing = false
			if !res.Cancelled {
				s.addAll(res.Paths)
			}
		})
	})
}

// addAll adds the folders the chooser answered. A folder that cannot be added is left
// out, and the message says why. It runs on the UI thread.
func (s *filesTabState) addAll(paths []string) {
	e := s.Widget().editor
	var refused []string
	for _, p := range paths {
		canonical, problem := e.CheckFolder(p)
		if problem != "" {
			refused = append(refused, problem)
			continue
		}
		e.AddFolder(canonical)
	}
	s.message = strings.Join(refused, " ")
}

// typePath asks for a path in a dialog and adds it.
func (s *filesTabState) typePath() {
	w := s.Widget()
	e := w.editor
	result := m.ShowDialog[string](s.Context(), func(m.BuildContext) m.Widget { return NewPathDialog(e.CheckFolder) })
	s.Go(func(ctx context.Context) {
		select {
		case path := <-result:
			if path == "" {
				return
			}
			w.change(func() {
				s.message = ""
				e.AddFolder(path)
			})
		case <-ctx.Done():
		}
	})
}

func (s *filesTabState) Build(bc m.BuildContext) m.Widget {
	e := s.Widget().editor
	final := e.Final()
	children := []m.Widget{s.homeSection(bc, e, final), s.foldersSection(bc, e, final), s.togglesSection(bc, e, final)}
	return tabList(firejail.TabFiles, s.scroll, children)
}

// sectionTitle is the heading of a section, with the mark when the section changed.
func sectionTitle(bc m.BuildContext, text string, mark m.Widget, trailing ...m.Widget) m.Widget {
	theme := m.ThemeOf(bc)
	row := []m.Widget{m.NewText(text, m.TextOpts{Style: theme.TextTheme.TitleMedium})}
	if mark != nil {
		row = append(row, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(12.0)}), mark)
	}
	row = append(row, m.NewSpacer())
	row = append(row, trailing...)
	return m.NewPadding(m.NewEdgeInsetsFromLTRB(0, 12, 0, 4), m.NewRow(row, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
}

// mutedText is a line of explanation.
func mutedText(bc m.BuildContext, text string, opts ...m.TextOpts) m.Widget {
	theme := m.ThemeOf(bc)
	var o m.TextOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	o.Style = theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())})
	return m.NewText(text, o)
}

// homeSection is the home folder choice: four radio tiles, two by two.
func (s *filesTabState) homeSection(bc m.BuildContext, e *Editor, final firejail.Settings) m.Widget {
	var mark m.Widget
	if e.HomeChanged() {
		mark = NewChangedMark(func() { s.apply(func(e *Editor) { e.ResetHome() }) }, ChangedMarkOpts{Key: m.NewValueKey("mark:home")})
	}
	tile := func(mode firejail.HomeMode) m.Widget {
		title, text := homeTexts(mode)
		if mode == firejail.HomePrivate {
			text = "Its own home, kept between runs: " + displayPlainPath(e.PrivateHomePath(), e.home)
		}
		return radioTile(bc, "home:"+string(mode), mode, title, text, final.Home == mode, true)
	}
	grid := m.NewColumn([]m.Widget{
		m.NewIntrinsicHeight(m.NewRow([]m.Widget{m.NewExpanded(tile(firejail.HomeFull)), m.NewExpanded(tile(firejail.HomeChosen))}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})),
		m.NewIntrinsicHeight(m.NewRow([]m.Widget{m.NewExpanded(tile(firejail.HomeEmpty)), m.NewExpanded(tile(firejail.HomePrivate))}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})),
	})
	group := m.NewRadioGroup(func(v m.Opt[firejail.HomeMode]) {
		mode, ok := v.Get()
		if !ok || mode == final.Home {
			return
		}
		s.apply(func(e *Editor) {
			s.message = ""
			e.SetHome(mode)
		})
	}, grid, m.RadioGroupOpts[firejail.HomeMode]{GroupValue: m.Some(final.Home)})
	return m.NewColumn([]m.Widget{
		sectionTitle(bc, homeTitleText, mark),
		mutedText(bc, homeIntroText),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(8.0)}),
		group,
	}, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// foldersHint explains the folder list for the home choice.
func foldersHint(final firejail.Settings) string {
	switch final.Home {
	case firejail.HomeChosen:
		return foldersHintChosen
	case firejail.HomeEmpty, firejail.HomePrivate:
		return foldersHintKeepsOut
	}
	return foldersHintFull
}

// foldersSection is the folder list: the header with the two ways to add a folder, one
// row per folder, the rows of the profile the user removed, and what is wrong.
func (s *filesTabState) foldersSection(bc m.BuildContext, e *Editor, final firejail.Settings) m.Widget {
	add := m.NewTextButtonIcon(s.chooseFolders, m.NewText(addFolderText), m.TextButtonIconOpts{
		Icon:                  m.NewIcon(m.Icons.CreateNewFolder()),
		ButtonStyleButtonOpts: m.ButtonStyleButtonOpts{Key: m.NewValueKey("add-folder")},
	})
	typed := m.NewTextButtonIcon(s.typePath, m.NewText(typePathText), m.TextButtonIconOpts{
		Icon:                  m.NewIcon(m.Icons.Edit()),
		ButtonStyleButtonOpts: m.ButtonStyleButtonOpts{Key: m.NewValueKey("type-path")},
	})
	col := []m.Widget{
		sectionTitle(bc, foldersTitleText, nil, add, typed),
		mutedText(bc, foldersHint(final)),
		m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(4.0)}),
	}
	if s.message != "" {
		col = append(col, notice(bc, s.message))
	}

	problems := map[string][]string{}
	var listProblem string
	for _, p := range e.Problems() {
		switch {
		case p.Tag == firejail.ProblemChosenNoFolder:
			listProblem = p.Message
		case p.Field == "" && p.Path != "":
			problems[p.Path] = append(problems[p.Path], p.Message)
		}
	}

	removed := e.RemovedFolders()
	if len(final.Folders) == 0 && len(removed) == 0 {
		col = append(col, m.NewPadding(m.NewEdgeInsetsSymmetric(8, 0), mutedText(bc, noFoldersText)))
	}
	// The rows the user added come first, where a new row can be seen; then the profile's.
	for _, fromProfile := range []bool{false, true} {
		for _, f := range final.Folders {
			if f.FromProfile == fromProfile {
				col = append(col, s.folderRow(bc, e, f, problems[f.Path]))
			}
		}
	}
	for _, f := range removed {
		col = append(col, s.removedRow(bc, e, f))
	}
	if listProblem != "" {
		col = append(col, problemLine(bc, listProblem, m.NewValueKey("problem:folders")))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}

// problemLine is a problem of the settings, written under the part of the tab it is about.
func problemLine(bc m.BuildContext, text string, key m.Key) m.Widget {
	theme := m.ThemeOf(bc)
	return m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
		m.NewIcon(m.Icons.Error(), m.IconOpts{Size: m.Some(18.0), Color: m.Some(theme.ColorScheme.Error())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewExpanded(m.NewText(text, m.TextOpts{
			Style: theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.Error())}),
		})),
	}), m.PaddingOpts{Key: key})
}

// pathLabel is the path of a row, with the user folder's macro as a hint under it.
func pathLabel(bc m.BuildContext, e *Editor, path string, strike bool, trailing ...m.Widget) m.Widget {
	theme := m.ThemeOf(bc)
	text, hint := displayFolderPath(path, e.home)
	style := theme.TextTheme.BodyMedium
	if strike {
		style = style.CopyWith(m.TextStyleCopy{
			Decoration: m.Some(m.TextDecorationLineThrough),
			Color:      m.Some(theme.ColorScheme.OnSurfaceVariant()),
		})
	}
	first := []m.Widget{m.NewFlexible(m.NewText(text, m.TextOpts{Style: style, MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis}))}
	for _, t := range trailing {
		first = append(first, m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}), t)
	}
	col := []m.Widget{m.NewRow(first, m.RowOpts{MainAxisSize: m.MainAxisSizeMin, CrossAxisAlignment: m.CrossAxisAlignmentCenter})}
	if hint != "" {
		col = append(col, mutedText(bc, hint, m.TextOpts{MaxLines: m.Some(1), Overflow: m.TextOverflowEllipsis}))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart, MainAxisSize: m.MainAxisSizeMin})
}

// folderRow is one row of the list: the path, what marks it, the access and the button
// that removes it.
func (s *filesTabState) folderRow(bc m.BuildContext, e *Editor, f firejail.Folder, problems []string) m.Widget {
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	path := f.Path

	var badges []m.Widget
	if f.FromProfile {
		badges = append(badges, badge(bc, fromProfileText, scheme.SecondaryContainer(), scheme.OnSecondaryContainer()))
	}
	left := []m.Widget{pathLabel(bc, e, path, false, badges...)}
	if e.FolderChanged(path) {
		left = append(left, NewChangedMark(func() { s.apply(func(e *Editor) { e.ResetFolder(path) }) },
			ChangedMarkOpts{Key: m.NewValueKey("mark:" + path)}))
	}
	if f.Locked {
		left = append(left, m.NewRow([]m.Widget{
			m.NewIcon(m.Icons.Lock(), m.IconOpts{Size: m.Some(14.0), Color: m.Some(scheme.OnSurfaceVariant())}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(4.0)}),
			m.NewExpanded(mutedText(bc, lockedReasonText, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis})),
		}, m.RowOpts{Key: m.NewValueKey("locked:" + path)}))
	}
	for _, msg := range problems {
		left = append(left, problemLine(bc, msg, nil))
	}

	right := []m.Widget{}
	if f.Locked {
		right = append(right, m.NewText(accessLabel(f.Access), m.TextOpts{Style: theme.TextTheme.BodyMedium}))
	} else {
		right = append(right,
			s.accessDropdown(path, f.Access),
			m.NewIconButton(func() { s.apply(func(e *Editor) { e.RemoveFolder(path) }) }, m.NewIcon(m.Icons.Delete()),
				m.IconButtonOpts{Key: m.NewValueKey("remove:" + path), Tooltip: "Remove from the list"}))
	}
	return m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
		m.NewExpanded(m.NewColumn(left, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart})),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewRow(right, m.RowOpts{MainAxisSize: m.MainAxisSizeMin, CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
	}, m.RowOpts{Key: m.NewValueKey("folder:" + path), CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
}

// accessDropdown is the menu that changes the access of a row.
func (s *filesTabState) accessDropdown(path string, current firejail.Access) m.Widget {
	var items []*m.DropdownMenuItem[firejail.Access]
	for _, a := range []firejail.Access{firejail.AccessReadWrite, firejail.AccessReadOnly, firejail.AccessHidden} {
		items = append(items, m.NewDropdownMenuItem(m.NewText(accessLabel(a)), m.DropdownMenuItemOpts[firejail.Access]{
			Key: m.NewValueKey("access:" + string(a) + ":" + path), Value: m.Some(a),
		}))
	}
	return m.NewSizedBox(m.NewDropdownButton(items, func(v m.Opt[firejail.Access]) {
		if a, ok := v.Get(); ok && a != current {
			s.apply(func(e *Editor) { e.SetFolderAccess(path, a) })
		}
	}, m.DropdownButtonOpts[firejail.Access]{Key: m.NewValueKey("access:" + path), Value: m.Some(current), IsDense: true}),
		m.SizedBoxOpts{Width: m.Some(140.0)})
}

// removedRow is a row of the profile the user removed: struck through, with Restore.
func (s *filesTabState) removedRow(bc m.BuildContext, e *Editor, f firejail.Folder) m.Widget {
	path := f.Path
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme
	return m.NewPadding(m.NewEdgeInsetsSymmetric(4, 0), m.NewRow([]m.Widget{
		m.NewExpanded(pathLabel(bc, e, path, true)),
		badge(bc, removedText, scheme.TertiaryContainer(), scheme.OnTertiaryContainer()),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewTextButton(func() { s.apply(func(e *Editor) { e.ResetFolder(path) }) }, m.NewText(restoreText),
			m.TextButtonOpts{Key: m.NewValueKey("restore:" + path)}),
	}, m.RowOpts{Key: m.NewValueKey("folder:" + path), CrossAxisAlignment: m.CrossAxisAlignmentCenter}))
}

// togglesSection is the quick settings: one switch with an explanation for each.
func (s *filesTabState) togglesSection(bc m.BuildContext, e *Editor, final firejail.Settings) m.Widget {
	col := []m.Widget{sectionTitle(bc, togglesTitleText, nil)}
	for _, t := range quickToggles() {
		t := t
		on := t.get(final)
		text := t.text
		onChanged := func(v bool) { s.apply(func(e *Editor) { e.Set(func(s *firejail.Settings) { t.set(s, v) }) }) }
		if t.field == firejail.FieldDownloadsReadWrite && e.KeepsHomeOut() {
			if !on {
				// It cannot be turned on; one that is on can still be turned off, and the
				// problem under it says why it must be.
				text, onChanged = downloadsOffReason, nil
			}
		}
		var mark m.Widget
		if e.Changed(t.field) {
			field := t.field
			mark = NewChangedMark(func() { s.apply(func(e *Editor) { e.Reset(field) }) }, ChangedMarkOpts{Key: m.NewValueKey("mark:" + string(field))})
		}
		title := markedTitle(settingLabel(t.field), mark)
		col = append(col, m.NewSwitchListTile(on, onChanged, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(t.field)),
			Title:    title,
			Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis}),
			Dense:    m.Some(true),
		}))
		if t.field == firejail.FieldDownloadsReadWrite {
			for _, p := range e.Problems() {
				if p.Tag == firejail.ProblemDownloadsHome {
					col = append(col, problemLine(bc, p.Message, m.NewValueKey("problem:downloads")))
				}
			}
		}
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}
