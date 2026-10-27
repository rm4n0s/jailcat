package ui

import (
	"context"
	"slices"
	"strings"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// TabExtraRules is the Extra rules tab (SPEC §7.9). It holds no settings, so firejail's
// Tab type has no value for it.
const TabExtraRules firejail.Tab = "extra"

// EditorTabs returns the tabs of the build editor in the order of SPEC §6.
func EditorTabs() []firejail.Tab {
	return []firejail.Tab{
		firejail.TabFiles, firejail.TabNetwork, firejail.TabDevices, firejail.TabDesktop, firejail.TabSecurity,
		firejail.TabPrograms, firejail.TabLimits, firejail.TabLauncher, TabExtraRules,
	}
}

// TabTitle returns the name of a tab as the editor shows it.
func TabTitle(t firejail.Tab) string {
	if t == TabExtraRules {
		return "Extra rules"
	}
	if t == "" {
		return ""
	}
	return strings.ToUpper(string(t)[:1]) + string(t)[1:]
}

// snapshot is the state of a build as it is stored, to tell whether the editor has
// unsaved changes.
type snapshot struct {
	baseProfile string
	// key is the canonical form of the overrides, see [Editor.overridesKey].
	key         string
	extra       []string
	sandboxName string
	icon        firejail.IconMode
	seed        int64
	overrides   firejail.Overrides
}

// EditorOpts are the inputs of [NewEditor].
type EditorOpts struct {
	// Home is the user's home folder, to recognize folders inside it.
	Home string
	// Choices is the profile selector's content; it names the base profile on the button.
	Choices ProfileChoices
	// BaseProfile is the path of the base profile Base was computed from, or "" for None.
	// For a stored build with a draft it is the draft's base profile ([BuildBaseProfile]).
	BaseProfile string
	// Profile is the profile that was read, nil for None.
	Profile *firejail.Profile
	// Base is the base value of every setting.
	Base firejail.Base
	// Defaults are the launcher fields of a new build ([Backend.NewBuildDefaults]); they
	// are ignored when Build is stored.
	Defaults firejail.Settings
	// Build is the stored build to edit; the zero value makes a new build. A build with a
	// draft opens on the draft's values.
	Build store.Build
}

// BuildBaseProfile returns the path of the base profile the editor of the build shows:
// the draft's when the build has one, else the stored one.
func BuildBaseProfile(b store.Build) string {
	if b.Draft != nil {
		return b.Draft.BaseProfile
	}
	return b.BaseProfile
}

// Editor is the state of the build editor of SPEC §6, without any widget: the app, the
// build, the base profile and its base values, what the user changed, and the Extra
// rules. Only the UI thread uses it.
//
// The settings the user sees are [Editor.Final]. What is stored is the difference from
// the base values ([Editor.Overrides]), so a change of the base profile reaches every
// field the user did not touch (SPEC §6.1).
type Editor struct {
	app  apps.App
	home string

	// build is the stored build; its ID is 0 until the build is stored.
	build   store.Build
	choices ProfileChoices

	baseProfile string
	profile     *firejail.Profile
	base        firejail.Base
	// baseline is what "changed" is measured against: the base values, and for the
	// launcher fields that come from the app (menu name, comment, terminal) their defaults.
	baseline firejail.Settings
	final    firejail.Settings
	// overrides is firejail.Diff(base.Settings, final), kept as loaded until the first
	// change so a stored build keeps what this version does not know.
	overrides firejail.Overrides
	extra     []string

	saved snapshot
	// hasDraft is true while the database holds a draft of the build; fromDraft that the
	// editor opened on one.
	hasDraft  bool
	fromDraft bool
	// notice is a message about opening the build, in plain words.
	notice string
}

// NewEditor creates the editor of a new build, or of the stored build in o.Build.
func NewEditor(app apps.App, o EditorOpts) *Editor {
	e := &Editor{
		app: app, home: o.Home, choices: o.Choices,
		baseProfile: o.BaseProfile, profile: o.Profile, base: o.Base,
	}
	e.baseline = e.makeBaseline(o.Base)

	if o.Build.ID == 0 {
		e.build = newBuildRow(app)
		e.final = o.Base.Settings.Clone()
		overlayLauncher(&e.final, o.Defaults)
		e.overrides = firejail.Diff(o.Base.Settings, e.final)
		e.saved = snapshot{
			baseProfile: e.baseProfile, key: e.overridesKey(), extra: nil,
			sandboxName: e.final.SandboxName, icon: e.final.Icon, seed: e.final.IconSeed, overrides: e.overrides,
		}
		return e
	}

	b := o.Build
	e.build = b
	overrides, extra := b.Overrides, b.ExtraRules
	if b.Draft != nil {
		overrides, extra = b.Draft.Overrides, b.Draft.ExtraRules
		e.hasDraft, e.fromDraft = true, true
	}
	e.final = overrides.Apply(o.Base.Settings)
	// A build stores its name, icon choice and seed in columns as well: they win when the
	// overrides lack them.
	if !overrides.Changed(firejail.FieldSandboxName) {
		e.final.SandboxName = b.SandboxName
	}
	if !overrides.Changed(firejail.FieldIconSeed) {
		e.final.IconSeed = b.IconSeed
	}
	if !overrides.Changed(firejail.FieldIcon) && b.IconMode == store.IconModeRandom {
		e.final.Icon = firejail.IconRandom
	}
	e.overrides = overrides
	e.extra = slices.Clone(extra)

	e.saved = snapshot{
		baseProfile: b.BaseProfile, extra: slices.Clone(b.ExtraRules), overrides: b.Overrides,
		sandboxName: b.SandboxName, icon: firejail.IconMode(b.IconMode), seed: b.IconSeed,
	}
	if b.Draft == nil {
		e.saved.key = e.overridesKey()
	} else if data, err := b.Overrides.MarshalJSON(); err == nil {
		// The stored overrides may be of another base than the editor's: no canonical form.
		e.saved.key = string(data)
	}
	return e
}

// newBuildRow is the part of a new build that comes from the app.
func newBuildRow(app apps.App) store.Build {
	b := store.Build{BinaryPath: app.Binary, IconMode: store.IconModeBadge}
	if app.Kind == apps.KindBinary {
		b.AppKind = store.AppKindBinary
	} else {
		b.AppKind = store.AppKindDesktop
		b.DesktopFileID = app.ID
	}
	return b
}

// overlayLauncher copies the launcher fields that a new build gets from the app and
// from chance.
func overlayLauncher(dst *firejail.Settings, src firejail.Settings) {
	dst.SandboxName = src.SandboxName
	dst.MenuName = src.MenuName
	dst.Comment = src.Comment
	dst.Terminal = src.Terminal
	dst.IconSeed = src.IconSeed
}

// makeBaseline returns the settings "changed" is measured against for a base.
func (e *Editor) makeBaseline(base firejail.Base) firejail.Settings {
	s := base.Settings.Clone()
	d := launcherDefaults(e.app)
	s.MenuName, s.Comment, s.Terminal = d.MenuName, d.Comment, d.Terminal
	return s
}

// overridesKey is the canonical text of the current overrides: two states with the same
// settings give the same text.
func (e *Editor) overridesKey() string {
	data, err := firejail.Diff(e.base.Settings, e.final).MarshalJSON()
	if err != nil {
		// Only a version this Jailcat cannot write fails, and Diff makes the current one.
		return ""
	}
	return string(data)
}

// App is the app the build is for.
func (e *Editor) App() apps.App { return e.app }

// Build is the build as stored; its ID is 0 until the build is stored.
func (e *Editor) Build() store.Build { return e.build }

// BaseProfile is the path of the base profile, or "" for None.
func (e *Editor) BaseProfile() string { return e.baseProfile }

// Profile is the base profile as it was read, nil for None.
func (e *Editor) Profile() *firejail.Profile { return e.profile }

// Base is the base value of every setting, with the lines each came from.
func (e *Editor) Base() firejail.Base { return e.base }

// Choices is the content of the profile selector.
func (e *Editor) Choices() ProfileChoices { return e.choices }

// SetChoices replaces the content of the profile selector (after an import).
func (e *Editor) SetChoices(c ProfileChoices) { e.choices = c }

// ProfileLabel is the text on the profile selector's button.
func (e *Editor) ProfileLabel() string { return e.choices.LabelFor(e.baseProfile) }

// Notice is a message about opening the build, in plain words; "" for none.
func (e *Editor) Notice() string { return e.notice }

// SetNotice sets the message of [Editor.Notice].
func (e *Editor) SetNotice(text string) { e.notice = text }

// Final returns the settings the user sees: the base values with the overrides on top.
// The copy is the caller's.
func (e *Editor) Final() firejail.Settings { return e.final.Clone() }

// Overrides returns what the user changed from the base values: what is stored.
func (e *Editor) Overrides() firejail.Overrides { return e.overrides }

// ExtraRules returns the rows of the Extra rules.
func (e *Editor) ExtraRules() []string { return slices.Clone(e.extra) }

// Set changes the settings: apply is called with a copy of them, and the overrides are
// computed again from what it leaves.
func (e *Editor) Set(apply func(s *firejail.Settings)) {
	s := e.final.Clone()
	apply(&s)
	e.final = s
	e.recompute()
}

// recompute makes the overrides the difference from the base values again. What the
// stored overrides hold that this version does not know is kept (SPEC §12).
func (e *Editor) recompute() {
	e.overrides = firejail.Diff(e.base.Settings, e.final).WithUnknownFrom(e.overrides)
}

// SetExtraRules replaces the rows of the Extra rules.
func (e *Editor) SetExtraRules(rows []string) { e.extra = slices.Clone(rows) }

// changes is what the user changed from the baseline. The sandbox name and the icon seed
// are not changes: they say which build this is, and have no value in a profile.
func (e *Editor) changes() firejail.Overrides {
	return firejail.Diff(e.baseline, e.final).Without(firejail.FieldSandboxName).Without(firejail.FieldIconSeed)
}

// Changed reports whether the field differs from its base value.
func (e *Editor) Changed(f firejail.Field) bool { return e.changes().Changed(f) }

// ChangedFolders returns the folder rows the user added, changed or removed.
func (e *Editor) ChangedFolders() []firejail.FolderOverride { return e.changes().Folders }

// FolderChanged reports whether the folder row of the path differs from the base's.
func (e *Editor) FolderChanged(path string) bool {
	return slices.ContainsFunc(e.changes().Folders, func(fo firejail.FolderOverride) bool { return fo.Path == path })
}

// ChangeCount is the number of settings and folder rows that differ from their base
// values.
func (e *Editor) ChangeCount() int {
	c := e.changes()
	return len(c.ChangedFields()) + len(c.Folders)
}

// HasChanges reports whether any setting or folder row differs from its base value.
func (e *Editor) HasChanges() bool { return !e.changes().IsEmpty() }

// ChangedTabs returns the tabs that show a dot: the ones with a changed setting, the
// Files tab also for a changed folder row, and the Extra rules tab when it has a row
// that is not empty.
func (e *Editor) ChangedTabs() map[firejail.Tab]bool {
	tabs := e.changes().ChangedTabs()
	for _, row := range e.extra {
		if strings.TrimSpace(row) != "" {
			tabs[TabExtraRules] = true
			break
		}
	}
	return tabs
}

// Reset puts the field back to its base value ("Reset to profile").
func (e *Editor) Reset(f firejail.Field) {
	e.resetTo(e.changes().Without(f))
}

// ResetFolder puts the folder row of the path back to the base's.
func (e *Editor) ResetFolder(path string) {
	e.resetTo(e.changes().WithoutFolder(path))
}

// resetTo makes the settings the baseline with the changes on top, and keeps the sandbox
// name and the icon seed.
func (e *Editor) resetTo(changes firejail.Overrides) {
	s := changes.Apply(e.baseline)
	s.SandboxName, s.IconSeed = e.final.SandboxName, e.final.IconSeed
	e.final = s
	e.recompute()
}

// Problems checks the settings (SPEC §6): a problem with the error severity blocks
// Build and Try.
func (e *Editor) Problems() []firejail.Problem { return firejail.Validate(e.final, e.home) }

// SwitchProfile makes the profile at path the base profile (SPEC §6.1); base and
// profile are what the backend read from it. With keep, the user's changes stay and
// the ones that equal the new base value are dropped; without it every change is
// removed ("Start over"). The sandbox name and the icon seed stay either way.
func (e *Editor) SwitchProfile(path string, base firejail.Base, profile *firejail.Profile, keep bool) {
	old := e.overrides
	e.baseProfile, e.base, e.profile = path, base, profile
	e.baseline = e.makeBaseline(base)
	if keep {
		e.final = old.Apply(base.Settings)
	} else {
		s := base.Settings.Clone()
		d := launcherDefaults(e.app)
		s.MenuName, s.Comment, s.Terminal = d.MenuName, d.Comment, d.Terminal
		s.SandboxName, s.IconSeed = e.final.SandboxName, e.final.IconSeed
		e.final = s
	}
	e.overrides = firejail.Diff(e.base.Settings, e.final).WithUnknownFrom(old)
}

// Dirty reports whether the editor holds changes that are not stored: another base
// profile, other overrides or other Extra rules than the stored build has.
func (e *Editor) Dirty() bool {
	return e.baseProfile != e.saved.baseProfile ||
		!slices.Equal(e.extra, e.saved.extra) ||
		e.overridesKey() != e.saved.key
}

// HasDraft reports whether the database holds a draft of the build.
func (e *Editor) HasDraft() bool { return e.hasDraft }

// FromDraft reports whether the editor opened on a draft: it shows the "Unsaved draft"
// note.
func (e *Editor) FromDraft() bool { return e.fromDraft }

// Draft returns the unsaved state to keep in the database (SPEC §5).
func (e *Editor) Draft() store.Draft {
	return store.Draft{Overrides: e.overrides, ExtraRules: slices.Clone(e.extra), BaseProfile: e.baseProfile}
}

// DraftAction says what leaving the editor does to the database (SPEC §5).
type DraftAction int

// The things leaving the editor does to the database.
const (
	// DraftNothing leaves the database as it is: no unsaved changes, no stored draft.
	DraftNothing DraftAction = iota
	// DraftSave keeps the unsaved changes as the build's draft, creating the build first
	// when it is not stored yet.
	DraftSave
	// DraftClear removes a stored draft the user has since undone.
	DraftClear
)

// DraftPlan is what to write when the user leaves the editor. It holds copies of the
// editor's state, so it can be run off the UI thread.
type DraftPlan struct {
	// Action says what to do.
	Action DraftAction
	// BuildID is the stored build; 0 when Create is set.
	BuildID int64
	// Create is the build to store first, for a build that is not stored yet.
	Create *store.Build
	// Draft is the unsaved state, for [DraftSave].
	Draft store.Draft
}

// DraftPlan returns what leaving the editor now writes: the unsaved changes become the
// draft of the build; a build that is not stored yet is created first, as it was when
// the editor opened, not installed and never built. An editor with no unsaved changes
// writes nothing, except that a draft it opened on and the user undid is removed.
func (e *Editor) DraftPlan() DraftPlan {
	if !e.Dirty() {
		if e.hasDraft && e.build.ID != 0 {
			return DraftPlan{Action: DraftClear, BuildID: e.build.ID}
		}
		return DraftPlan{}
	}
	plan := DraftPlan{Action: DraftSave, BuildID: e.build.ID, Draft: e.Draft()}
	if e.build.ID == 0 {
		b := e.SavedBuild()
		plan.Create = &b
	}
	return plan
}

// Run writes the plan with the backend. The errors are the backend's.
func (p DraftPlan) Run(ctx context.Context, backend Backend) error {
	switch p.Action {
	case DraftSave:
		id := p.BuildID
		if p.Create != nil {
			created, err := backend.CreateBuild(ctx, *p.Create)
			if err != nil {
				return err
			}
			id = created.ID
		}
		return backend.SaveDraft(ctx, id, p.Draft)
	case DraftClear:
		return backend.ClearDraft(ctx, p.BuildID)
	}
	return nil
}

// SavedBuild returns the build as it was when the editor opened, for a build that is
// not stored yet: the first save of a new build keeps this state and puts the changes
// in its draft, so a build that was never built stays "not installed".
func (e *Editor) SavedBuild() store.Build {
	b := e.build
	b.SandboxName = e.saved.sandboxName
	b.BaseProfile = e.saved.baseProfile
	b.Overrides = e.saved.overrides
	b.ExtraRules = slices.Clone(e.saved.extra)
	b.IconMode = string(e.saved.icon)
	if b.IconMode == "" {
		b.IconMode = store.IconModeBadge
	}
	b.IconSeed = e.saved.seed
	b.Draft = nil
	return b
}
