package ui

import (
	"strconv"

	"github.com/rm4n0s/errors"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
)

// pageOutcome is what the banner under the tabs says about the last Build or Uninstall.
type pageOutcome struct {
	// title is the first line.
	title string
	// lines are the files, one each.
	lines []string
	// notes are plain sentences after the files (the icon, the kept home).
	notes []string
	// warning is what the check found, in plain words; "" when it passed.
	warning string
	// output is firejail's output of the check, shown on request; "" for none.
	output string
	// showOutput says that the output is open.
	showOutput bool
}

// Texts of the banner the user reads (SPEC §11.3).
const (
	installedSuffix        = " is installed"
	uninstalledSuffix      = " was uninstalled"
	checkNestedText        = "The files are installed, but Jailcat could not test the new sandbox: it runs inside another sandbox, and firejail cannot start a sandbox from inside one. Test the app from the menu or a terminal outside of it."
	checkFailedText        = "The files are installed, but firejail could not start a test program in the new sandbox, so the app may not start. See what firejail said."
	checkSkippedPrefix     = "The files are installed. "
	checkUnknownPrefix     = "The files are installed, but the test of the sandbox could not run: "
	showOutputText         = "Show firejail's output"
	hideOutputText         = "Hide firejail's output"
	dismissText            = "Dismiss"
	outcomeMaxHeight       = 240.0
	removedOldText         = "Removed the files of the old name: "
	leftInPlaceText        = "Left in place, as you changed them: "
	nothingRemovedFileText = "No file was left to remove."
)

// fileLine is one line of the banner: what a file is and where it is.
func fileLine(f install.File, home string) string {
	return conflictKindName(f.Kind) + ": " + displayPlainPath(f.Path, home)
}

// newBuiltOutcome is the banner of a Build that finished.
func newBuiltOutcome(res BuildResult, home string) *pageOutcome {
	o := &pageOutcome{title: res.Build.SandboxName + installedSuffix}
	for _, f := range res.Files {
		o.lines = append(o.lines, fileLine(f, home))
	}
	if res.IconFallback != "" {
		o.notes = append(o.notes, iconFallbackPrefix+res.IconFallback+".")
	}
	for _, f := range res.Removed {
		o.notes = append(o.notes, removedOldText+displayPlainPath(f.Path, home))
	}
	switch {
	case res.CheckErr != nil:
		if isNested(res.CheckErr) {
			o.warning, o.output = checkNestedText, res.Check.Output
		} else {
			o.warning = checkUnknownPrefix + plainReason(res.CheckErr) + "."
			o.output = res.Check.Output
		}
	case res.Check.Skipped:
		o.warning = checkSkippedPrefix + capitalize(res.Check.Reason) + "."
	case !res.Check.OK:
		o.warning, o.output = checkFailedText, res.Check.Output
	}
	if o.warning == checkNestedText {
		// Firejail's output only repeats its warning.
		o.output = ""
	}
	return o
}

// newUninstalledOutcome is the banner of an Uninstall that kept the build's settings.
func newUninstalledOutcome(name string, res install.UninstallResult, home string) *pageOutcome {
	o := &pageOutcome{title: name + uninstalledSuffix}
	for _, f := range res.Removed {
		o.lines = append(o.lines, fileLine(f, home))
	}
	if len(o.lines) == 0 {
		o.notes = append(o.notes, nothingRemovedFileText)
	}
	for _, f := range res.Kept {
		o.notes = append(o.notes, leftInPlaceText+displayPlainPath(f.Path, home))
	}
	if res.HomeKept {
		o.notes = append(o.notes, uninstallHomeKeptText+"its files are still in the private homes folder.")
	}
	return o
}

// isNested reports whether err says firejail found an existing sandbox.
func isNested(err error) bool {
	appErr, ok := errors.FromError(err)
	return ok && appErr.Tag == firejail.TagNestedSandbox
}

// capitalize makes the first letter of s a capital.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-'a'+'A') + s[1:]
	}
	return s
}

// outcomeBanner is the banner of the last Build or Uninstall.
func (s *editorPageState) outcomeBanner(bc m.BuildContext) m.Widget {
	o := s.outcome
	theme := m.ThemeOf(bc)
	scheme := theme.ColorScheme

	rows := []m.Widget{m.NewRow([]m.Widget{
		m.NewIcon(m.Icons.CheckCircle(), m.IconOpts{Color: m.Some(scheme.OnSecondaryContainer())}),
		m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
		m.NewExpanded(m.NewText(o.title, m.TextOpts{Key: m.NewValueKey("outcome:title"), Style: theme.TextTheme.TitleSmall.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSecondaryContainer())})})),
		m.NewTextButton(func() { s.SetState(func() { s.outcome = nil }) }, m.NewText(dismissText), m.TextButtonOpts{Key: m.NewValueKey("outcome:dismiss")}),
	}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter})}
	text := theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSecondaryContainer())})
	for i, line := range o.lines {
		rows = append(rows, m.NewText(line, m.TextOpts{Style: text, Key: m.NewValueKey("outcome:file:" + strconv.Itoa(i))}))
	}
	for i, note := range o.notes {
		rows = append(rows, m.NewText(note, m.TextOpts{Style: text, Key: m.NewValueKey("outcome:note:" + strconv.Itoa(i))}))
	}
	if o.warning != "" {
		rows = append(rows, m.NewSizedBox(nil, m.SizedBoxOpts{Height: m.Some(6.0)}), m.NewRow([]m.Widget{
			m.NewIcon(m.Icons.Warning(), m.IconOpts{Size: m.Some(18.0), Color: m.Some(scheme.Error())}),
			m.NewSizedBox(nil, m.SizedBoxOpts{Width: m.Some(8.0)}),
			m.NewExpanded(m.NewText(o.warning, m.TextOpts{Key: m.NewValueKey("outcome:warning"), Style: theme.TextTheme.BodySmall.CopyWith(m.TextStyleCopy{Color: m.Some(scheme.OnSecondaryContainer())})})),
		}, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentStart}))
		if o.output != "" {
			label := showOutputText
			if o.showOutput {
				label = hideOutputText
			}
			rows = append(rows, m.NewAlign(m.NewTextButton(func() { s.SetState(func() { o.showOutput = !o.showOutput }) },
				m.NewText(label), m.TextButtonOpts{Key: m.NewValueKey("outcome:toggle-output")}), m.AlignOpts{Alignment: m.AlignmentCenterLeft}))
			if o.showOutput {
				rows = append(rows, m.NewContainer(m.NewSelectionArea(m.NewText(o.output, m.TextOpts{
					Key: m.NewValueKey("outcome:output"), Style: monoStyle(bc, scheme.OnSurface()),
				})), m.ContainerOpts{
					Padding: m.NewEdgeInsetsAll(8),
					Decoration: m.NewBoxDecoration(m.BoxDecorationOpts{
						Color: m.Some(scheme.Surface()), BorderRadius: m.NewBorderRadiusCircular(6),
					}),
				}))
			}
		}
	}
	return m.NewConstrainedBox(m.NewBoxConstraints(m.BoxConstraintsOpts{MaxHeight: m.Some(outcomeMaxHeight)}),
		m.NewSingleChildScrollView(m.NewContainer(m.NewColumn(rows, m.ColumnOpts{CrossAxisAlignment: m.CrossAxisAlignmentStretch}), m.ContainerOpts{
			Key:     m.NewValueKey("outcome:banner"),
			Color:   m.Some(scheme.SecondaryContainer()),
			Padding: m.NewEdgeInsetsAll(12),
			Margin:  m.NewEdgeInsetsSymmetric(4, 12),
		})))
}
