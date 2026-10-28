package ui

import (
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// Texts of the VS Code section of the Files tab (SPEC §7.1).
const (
	vscodeTitleText      = "VS Code"
	vscodeIntroText      = "VS Code runs one instance per data folder. Without a folder of its own, the sandboxed VS Code finds your open VS Code and opens its window there, outside the sandbox."
	vscodeOwnDataText    = "The sandbox is a VS Code of its own, with its own window state, recent files and logins."
	vscodeOwnDataKeeps   = " With this home, VS Code makes the folder inside the sandbox's own home."
	vscodeShareText      = "Settings, keybindings and snippets are links to your VS Code's, so the sandbox looks and behaves like it. Your extensions are shared by VS Code itself."
	vscodeShareNeedsHome = "Not available with an empty or own private home: your real home stays out of the sandbox, so there is nothing to link to."
	vscodeShareNeedsData = "Not available without its own VS Code data folder: the links live in it."
	vscodeShellText      = "The integrated terminal sees your shell startup files (.bashrc, .zshrc and the like) read-only, and Tab completion works. Anything these files export, such as API keys, is visible in the sandbox."
	vscodeShellNeedsMode = "Not available unless the home is Only the folders I choose: with the full home the files are visible already, and an empty or own private home keeps them out."
)

// IsVSCode reports whether the app is a VS Code program (SPEC §7.1): the Files tab then
// shows the VS Code switches.
func (e *Editor) IsVSCode() bool { return firejail.IsVSCode(e.app.Binary) }

// vscodeSwitch describes one switch of the VS Code section.
type vscodeSwitch struct {
	field firejail.Field
	text  string
	// on is the value the switch shows: off when the home choice does not allow it.
	on bool
	// reason is why the switch is disabled; "" when it can be changed.
	reason string
	set    func(s *firejail.Settings, on bool)
}

// vscodeSwitches returns the three switches with what they show for the settings.
func vscodeSwitches(final firejail.Settings) []vscodeSwitch {
	own := vscodeOwnDataText
	if final.Home == firejail.HomeEmpty || final.Home == firejail.HomePrivate {
		own += vscodeOwnDataKeeps
	}
	share := vscodeSwitch{
		field: firejail.FieldVSCodeShareSettings, text: vscodeShareText, on: final.VSCodeLinksSettings(),
		set: func(s *firejail.Settings, on bool) { s.VSCodeShareSettings = on },
	}
	switch {
	case final.Home != firejail.HomeFull && final.Home != firejail.HomeChosen:
		share.reason = vscodeShareNeedsHome
	case !final.VSCodeOwnData:
		share.reason = vscodeShareNeedsData
	}
	shell := vscodeSwitch{
		field: firejail.FieldVSCodeShellFiles, text: vscodeShellText, on: final.VSCodeShowsShellFiles(),
		set: func(s *firejail.Settings, on bool) { s.VSCodeShellFiles = on },
	}
	if final.Home != firejail.HomeChosen {
		shell.reason = vscodeShellNeedsMode
	}
	return []vscodeSwitch{
		{
			field: firejail.FieldVSCodeOwnData, text: own, on: final.VSCodeOwnData,
			set: func(s *firejail.Settings, on bool) { s.VSCodeOwnData = on },
		},
		share, shell,
	}
}

// vscodeSection is the VS Code section: the three switches of SPEC §7.1. A switch that
// the home choice does not allow shows off and says why.
func (s *filesTabState) vscodeSection(bc m.BuildContext, e *Editor, final firejail.Settings) m.Widget {
	col := []m.Widget{sectionTitle(bc, vscodeTitleText, nil), mutedText(bc, vscodeIntroText)}
	for _, sw := range vscodeSwitches(final) {
		sw := sw
		text := sw.text
		var onChanged func(bool)
		if sw.reason != "" {
			text = sw.reason
		} else {
			onChanged = func(v bool) { s.apply(func(e *Editor) { e.Set(func(st *firejail.Settings) { sw.set(st, v) }) }) }
		}
		var mark m.Widget
		if e.Changed(sw.field) {
			field := sw.field
			mark = NewChangedMark(func() { s.apply(func(e *Editor) { e.Reset(field) }) }, ChangedMarkOpts{Key: m.NewValueKey("mark:" + string(field))})
		}
		col = append(col, m.NewSwitchListTile(sw.on, onChanged, m.SwitchListTileOpts{
			Key:      m.NewValueKey("toggle:" + string(sw.field)),
			Title:    markedTitle(settingLabel(sw.field), mark),
			Subtitle: m.NewText(text, m.TextOpts{MaxLines: m.Some(3), Overflow: m.TextOverflowEllipsis}),
			Dense:    m.Some(true),
		}))
	}
	return m.NewColumn(col, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch})
}
