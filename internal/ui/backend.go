package ui

import (
	"context"
	"path/filepath"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// Backend is everything the widgets need from the rest of Jailcat. The real one is
// [LocalBackend]; tests give the widgets a fake. Every method that does I/O takes a
// context, and the widgets call them off the UI thread (SPEC §4.3, §13 of Glutter).
// Later steps add methods.
type Backend interface {
	// ScanApps finds the apps of the system (SPEC §4.1, §4.2).
	ScanApps(ctx context.Context) (apps.Result, error)
	// ListBuilds returns every saved build (SPEC §12).
	ListBuilds(ctx context.Context) ([]store.Build, error)
	// LookupIcon finds the image file of an app's own icon. The error has the tag
	// [icons.TagIconNotFound] when the app has none.
	LookupIcon(ctx context.Context, app apps.App) (icons.Icon, error)
	// Firejail finds firejail and reads its version (SPEC §3). A firejail that is not
	// installed is not an error: the result has Missing set.
	Firejail(ctx context.Context) (FirejailInfo, error)
	// BinOnPath reports whether the folder the launchers go to is on $PATH (SPEC §3.1).
	BinOnPath() bool
	// ProgramOnPath reports whether an executable file called name is in one of the
	// folders of $PATH (SPEC §7.4: the X11 isolation programs).
	ProgramOnPath(ctx context.Context, name string) bool
	// CPUCount returns how many cores this machine has, for the checkboxes of the Limits
	// tab (SPEC §7.7). It does no I/O.
	CPUCount() int
	// HomeDir returns the user's home folder, to recognize folders inside it.
	HomeDir() string
	// HomesDir returns the folder of Jailcat's private homes (SPEC §3.1).
	HomesDir() string

	// ProfileChoices lists the profiles the selector offers for the app, group by group
	// (SPEC §6.1).
	ProfileChoices(ctx context.Context, app apps.App) (ProfileChoices, error)
	// ReadBase reads the base profile at path the way firejail does and computes its base
	// values (SPEC §8). The path "" is base None, with no profile to read.
	ReadBase(ctx context.Context, path string) (firejail.Base, *firejail.Profile, error)
	// ImportProfile copies the .profile file src into Jailcat's profiles/imported/ folder
	// under a name that is free, and returns the path of the copy (SPEC §6.1).
	ImportProfile(ctx context.Context, src string) (string, error)
	// BuildsForApp returns the builds of the app, ordered by sandbox name.
	BuildsForApp(ctx context.Context, app apps.App) ([]store.Build, error)
	// CreateBuild stores a new build.
	CreateBuild(ctx context.Context, b store.Build) (store.Build, error)
	// UpdateBuild stores the editable columns of a build.
	UpdateBuild(ctx context.Context, b store.Build) (store.Build, error)
	// SaveDraft keeps the unsaved state of a build's editor (SPEC §5).
	SaveDraft(ctx context.Context, id int64, d store.Draft) error
	// ClearDraft removes the draft of a build.
	ClearDraft(ctx context.Context, id int64) error
	// NewBuildDefaults returns the launcher settings of a new build of the app (SPEC
	// §7.8): a sandbox name no other build has, the menu name, the comment, the terminal
	// choice and a random icon seed. The other fields are zero.
	NewBuildDefaults(ctx context.Context, app apps.App) (firejail.Settings, error)
	// SandboxNameTaken reports whether a saved build other than the one with the ID
	// exceptBuildID (0 for a build that is not saved yet) has the sandbox name (SPEC §7.8).
	SandboxNameTaken(ctx context.Context, name string, exceptBuildID int64) (bool, error)
	// PreviewIcon makes the SVG icon a build of the app with the settings would install
	// (SPEC §10), without writing it. fallbackReason says, in words, why a random icon was
	// made when the settings ask for the app's icon with the badge and it cannot be used;
	// it is "" otherwise.
	PreviewIcon(ctx context.Context, app apps.App, s firejail.Settings) (svg []byte, fallbackReason string, err error)
	// CheckExtraRules checks the rows of the Extra rules tab against the base (SPEC §7.9).
	CheckExtraRules(ctx context.Context, rows []string, base firejail.Base) []firejail.Problem

	// PrepareBuild generates the profile and the files of the build the request describes
	// and checks their targets (SPEC §9.5), without writing anything: it feeds Preview and
	// the conflicts dialog. A firejail that is not installed is an error with the tag
	// [firejail.TagFirejailNotFound].
	PrepareBuild(ctx context.Context, req BuildRequest) (*PreparedBuild, error)
	// Build installs a prepared build (SPEC §11.3): it stores the build when it is new,
	// writes the files, removes the files of an old name, records the build and the files,
	// and runs the check. allowOverwrite names the conflicting targets the user agreed to
	// overwrite. A failed check does not undo the install; see [BuildResult].
	Build(ctx context.Context, prepared *PreparedBuild, allowOverwrite map[string]bool) (BuildResult, error)
	// Try runs the build in a sandbox without installing it (SPEC §11.2). The session
	// ends when it is stopped, when the program ends, or when ctx is cancelled.
	Try(ctx context.Context, req BuildRequest) (TrySession, error)
	// Uninstall removes the recorded files of the build and, by choice, its private home
	// and its place in the database (SPEC §13.2).
	Uninstall(ctx context.Context, build store.Build, choice UninstallChoice) (install.UninstallResult, error)
	// InstalledFiles returns the files recorded for the build.
	InstalledFiles(ctx context.Context, buildID int64) ([]store.InstalledFile, error)
	// BaseProfileMissing reports whether the base profile file at path is gone (SPEC
	// §13.3). The path "" (base None) is never missing.
	BaseProfileMissing(ctx context.Context, path string) bool
}

// FirejailInfo says which firejail Jailcat found.
type FirejailInfo struct {
	// Path is the absolute path of the firejail program; empty when it is missing.
	Path string
	// Version is the version firejail reported, such as "0.9.80".
	Version string
	// Missing is true when no firejail is installed.
	Missing bool
}

// AppKey returns the key that names an app in the UI: "desktop:<desktop file ID>" for a
// menu app and "binary:<absolute path>" for a binary. It is the same key [BuildKey] gives
// the builds of the app.
func AppKey(a apps.App) string {
	if a.Kind == apps.KindBinary {
		return store.AppKindBinary + ":" + a.Binary
	}
	return store.AppKindDesktop + ":" + a.ID
}

// BuildKey returns the [AppKey] of the app a build belongs to.
func BuildKey(b store.Build) string {
	if b.AppKind == store.AppKindBinary {
		return store.AppKindBinary + ":" + filepath.Clean(b.BinaryPath)
	}
	return store.AppKindDesktop + ":" + b.DesktopFileID
}

// isBuilt reports whether the build is installed now: it was built and not uninstalled
// since (SPEC §13.2).
func isBuilt(b store.Build) bool { return b.BuiltAt != nil && b.Installed }

// BuildCounts counts the builds of each app that are built, by [AppKey]. A build that was
// never built, or was uninstalled with its settings kept, is a draft, see [DraftCounts].
func BuildCounts(builds []store.Build) map[string]int {
	counts := map[string]int{}
	for _, b := range builds {
		if isBuilt(b) {
			counts[BuildKey(b)]++
		}
	}
	return counts
}

// DraftCounts counts the builds of each app that are not installed (SPEC §5, §13.2): they
// hold the editor's state of an app the user started on but did not build, or built and
// uninstalled with its settings kept, and the app list marks them "draft".
func DraftCounts(builds []store.Build) map[string]int {
	counts := map[string]int{}
	for _, b := range builds {
		if !isBuilt(b) {
			counts[BuildKey(b)]++
		}
	}
	return counts
}
