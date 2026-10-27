package ui

import (
	"strconv"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/store"
)

// What the Keep/Start over dialog pops with. Closing the dialog pops with nothing.
const (
	// KeepChanges keeps the user's overrides on the new profile.
	KeepChanges = "keep"
	// StartOver removes every override.
	StartOver = "start-over"
)

// Texts of the dialogs.
const (
	keepButtonText      = "Keep my changes"
	startOverButtonText = "Start over"
	cancelButtonText    = "Cancel"
	newBuildButtonText  = "New build"
)

// KeepChangesDialog asks what to do with the user's changes when the base profile
// changes (SPEC §6.1). It pops with [KeepChanges] or [StartOver]; Cancel pops with
// nothing.
type KeepChangesDialog struct {
	m.StatelessWidget
	changes int
	profile string
}

// NewKeepChangesDialog creates the dialog. changes is the number of settings the user
// changed, profile the name of the profile they are switching to.
func NewKeepChangesDialog(changes int, profile string, opts ...m.WidgetOpts) *KeepChangesDialog {
	return &KeepChangesDialog{StatelessWidget: m.NewStatelessWidgetBase(opts...), changes: changes, profile: profile}
}

// Build builds the dialog.
func (w *KeepChangesDialog) Build(bc m.BuildContext) m.Widget {
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title:   m.NewText("Keep your changes?"),
		Content: m.NewSizedBox(m.NewText(keepDialogText(w.changes, w.profile)), m.SizedBoxOpts{Width: m.Some(420.0)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText(cancelButtonText)),
			m.NewTextButton(func() { m.NavigatorPop(bc, StartOver) }, m.NewText(startOverButtonText)),
			m.NewFilledButton(func() { m.NavigatorPop(bc, KeepChanges) }, m.NewText(keepButtonText)),
		},
	})
}

// keepDialogText explains the two choices.
func keepDialogText(changes int, profile string) string {
	what := strconv.Itoa(changes) + " settings"
	if changes == 1 {
		what = "1 setting"
	}
	return "You changed " + what + " from the current profile. With " + profile + " as the base profile, " +
		"Keep my changes leaves them as they are and takes the rest from " + profile + "; " +
		"Start over removes them all."
}

// BuildChoice is what the build chooser pops with. The zero value is "closed without a
// choice".
type BuildChoice struct {
	// Build is the build the user picked; valid when Chosen is set and New is not.
	Build store.Build
	// New is true when the user asked for a new build.
	New bool
	// Chosen is true when the user picked a build or asked for a new one.
	Chosen bool
}

// BuildChooserDialog lists the builds of an app that has several, and **New build**
// (SPEC §5). It pops with a [BuildChoice].
type BuildChooserDialog struct {
	m.StatelessWidget
	app    apps.App
	builds []store.Build
}

// NewBuildChooserDialog creates the dialog for the builds of app.
func NewBuildChooserDialog(app apps.App, builds []store.Build, opts ...m.WidgetOpts) *BuildChooserDialog {
	return &BuildChooserDialog{StatelessWidget: m.NewStatelessWidgetBase(opts...), app: app, builds: builds}
}

// buildStatusText says when a build was built, or that it never was.
func buildStatusText(b store.Build) string {
	text := "never built"
	if b.BuiltAt != nil {
		text = "built " + b.BuiltAt.Local().Format("2 Jan 2006")
	}
	if b.Draft != nil {
		text += ", unsaved draft"
	}
	return text
}

// Build builds the dialog.
func (w *BuildChooserDialog) Build(bc m.BuildContext) m.Widget {
	var tiles []m.Widget
	for _, b := range w.builds {
		tiles = append(tiles, m.NewListTile(m.ListTileOpts{
			Key:      m.NewValueKey("build:" + b.SandboxName),
			Title:    m.NewText(b.SandboxName),
			Subtitle: m.NewText(buildStatusText(b)),
			OnTap:    func() { m.NavigatorPop(bc, BuildChoice{Build: b, Chosen: true}) },
		}))
	}
	return m.NewAlertDialog(m.AlertDialogOpts{
		Title:   m.NewText(w.app.Name + " has " + strconv.Itoa(len(w.builds)) + " builds"),
		Content: m.NewSizedBox(m.NewListView(tiles, m.ListViewOpts{ShrinkWrap: true}), m.SizedBoxOpts{Width: m.Some(420.0)}),
		Actions: []m.Widget{
			m.NewTextButton(func() { m.NavigatorPop(bc) }, m.NewText(cancelButtonText)),
			m.NewFilledButton(func() { m.NavigatorPop(bc, BuildChoice{New: true, Chosen: true}) }, m.NewText(newBuildButtonText)),
		},
	})
}

// showBuildChooser shows the build chooser above bc and returns the channel of its
// answer.
func showBuildChooser(bc m.BuildContext, app apps.App, builds []store.Build) <-chan BuildChoice {
	return m.ShowDialog[BuildChoice](bc, func(m.BuildContext) m.Widget { return NewBuildChooserDialog(app, builds) })
}
