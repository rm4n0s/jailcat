package ui

import (
	"slices"

	"github.com/rm4n0s/jailcat/internal/store"
)

// Request returns the state Preview, Try and Build work from (SPEC §11): the app, the
// stored build, the base profile and what the user chose on top of it. The copy is the
// caller's, so it can be used off the UI thread.
func (e *Editor) Request() BuildRequest {
	return BuildRequest{
		App: e.app, Build: e.build, BaseProfile: e.baseProfile, Profile: e.profile, Base: e.base,
		Overrides: e.overrides, Settings: e.Final(), ExtraRules: slices.Clone(e.extra),
	}
}

// Built is called with the build as stored after a successful Build: the editor now
// holds exactly what is stored, so it has no unsaved changes and no draft (SPEC §13.1).
func (e *Editor) Built(stored store.Build) {
	e.build = stored
	e.build.Draft = nil
	e.hasDraft, e.fromDraft = false, false
	e.saved = snapshot{
		baseProfile: e.baseProfile, key: e.overridesKey(), extra: slices.Clone(e.extra),
		sandboxName: e.final.SandboxName, icon: e.final.Icon, seed: e.final.IconSeed, overrides: e.overrides,
	}
	e.firejailUpdated = false
}

// SetInstalled records that the build is installed or, after an uninstall that keeps the
// settings, not installed (SPEC §13.2). The settings in the editor do not change.
func (e *Editor) SetInstalled(installed bool) {
	e.build.Installed = installed
	if !installed {
		e.firejailUpdated = false
	}
}

// FirejailUpdated reports whether firejail is another version than the one that built
// the build (SPEC §13.3): the list and the header say "firejail updated".
func (e *Editor) FirejailUpdated() bool { return e.firejailUpdated }

// SetFirejailUpdated sets the answer of [Editor.FirejailUpdated].
func (e *Editor) SetFirejailUpdated(v bool) { e.firejailUpdated = v }

// BaseMissing reports whether the stored base profile of the build is gone (SPEC §13.3):
// the editor opened on base None and asks the user to choose another profile.
func (e *Editor) BaseMissing() bool { return e.baseMissing }

// SetBaseMissing sets the answer of [Editor.BaseMissing]. Choosing a profile ends it.
func (e *Editor) SetBaseMissing(v bool) { e.baseMissing = v }

// FocusSandboxName asks the Launcher tab to put the cursor in the sandbox name field the
// next time it shows (the conflicts dialog's "Choose another sandbox name").
func (e *Editor) FocusSandboxName() { e.focusName = true }

// TakeNameFocus reports whether the sandbox name field was asked to take the focus, and
// forgets the request.
func (e *Editor) TakeNameFocus() bool {
	v := e.focusName
	e.focusName = false
	return v
}

// firejailUpdatedFor says whether a build made with the firejail version built is on
// another version than running. A build that was never built (no version) or an unknown
// running version is not "updated".
func firejailUpdatedFor(built, running string) bool {
	return built != "" && running != "" && built != running
}
