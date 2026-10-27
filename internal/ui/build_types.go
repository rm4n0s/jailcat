package ui

import (
	"context"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// BuildRequest is the state of the editor that Preview, Try and Build work from (SPEC
// §11). It holds copies, so the backend can use it off the UI thread.
type BuildRequest struct {
	// App is the app the build is for.
	App apps.App
	// Build is the stored build; its ID is 0 when the build is not stored yet.
	Build store.Build
	// BaseProfile is the path of the base profile, or "" for None.
	BaseProfile string
	// Profile is the base profile as it was read; nil for None.
	Profile *firejail.Profile
	// Base is the base value of every setting.
	Base firejail.Base
	// Overrides are what the user changed from the base values.
	Overrides firejail.Overrides
	// Settings are the settings the user sees: the overrides on the base values.
	Settings firejail.Settings
	// ExtraRules are the rows of the Extra rules tab.
	ExtraRules []string
}

// PreparedBuild is what Build would write, for Preview and for the conflicts dialog
// ([Backend.PrepareBuild]). Nothing of it is written yet.
type PreparedBuild struct {
	// Request is the state it was made from; Build makes the files again from it when the
	// build gets its ID.
	Request BuildRequest
	// Generated is the profile, line by line, with where each line came from.
	Generated *firejail.Generated
	// Prepared holds the files of the build.
	Prepared *install.Prepared
	// Firejail is the absolute path of firejail the launcher runs.
	Firejail string
	// Conflicts are the targets that are taken (SPEC §9.5); none means Build may write all.
	Conflicts []install.Conflict
	// Owners maps the ID of another build that owns a conflicting file to its sandbox name.
	Owners map[int64]string
	// Stale are the recorded files of the installed build that the new name does not
	// write again: a rename removes them (SPEC §13.1).
	Stale []install.File
}

// BuildResult is what [Backend.Build] did.
type BuildResult struct {
	// Build is the build as stored after the Build.
	Build store.Build
	// Files are the files that were written, without their content.
	Files []install.File
	// Removed are the files of the old name that a rename removed.
	Removed []install.File
	// Check is the outcome of the check that ran the sandbox with "true" (SPEC §11.3).
	Check firejail.CheckResult
	// CheckErr is why the check could not tell; it has the tag [firejail.TagNestedSandbox]
	// when Jailcat itself runs inside another sandbox. The install stands either way.
	CheckErr error
	// IconFallback says why a random icon was made instead of the app's icon; "" if none.
	IconFallback string
}

// TrySession is a running Try (SPEC §11.2): the output of firejail as lines and the means
// to stop it. [firejail.Session] is one.
type TrySession interface {
	// Lines is the output, a line at a time; it is closed when the session has ended.
	Lines() <-chan string
	// Wait waits for the end and returns how firejail exited; the error has the tag
	// [firejail.TagNestedSandbox] when firejail ran the program unsandboxed.
	Wait() (firejail.ExitStatus, error)
	// Stop ends the sandbox and returns when it has ended.
	Stop(ctx context.Context) error
	// Nested reports whether firejail has said that it found an existing sandbox.
	Nested() bool
}

// UninstallChoice is what the user chose in the Uninstall dialog (SPEC §13.2).
type UninstallChoice struct {
	// DeleteHome also removes the private home folder.
	DeleteHome bool
	// KeepSettings keeps the build in the database as "not installed"; off removes it.
	KeepSettings bool
	// Force lists the paths of edited files to remove anyway.
	Force map[string]bool
	// LeaveKept finishes the uninstall although edited files were kept in place.
	LeaveKept bool
}

// UninstallDone reports whether an uninstall that returned result with choice is
// finished: the build is gone from the database or marked not installed. It is not while
// edited files were kept and the user has not said to leave them.
func UninstallDone(result install.UninstallResult, choice UninstallChoice) bool {
	return len(result.Kept) == 0 || choice.LeaveKept
}
