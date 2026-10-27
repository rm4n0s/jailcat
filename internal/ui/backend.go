package ui

import (
	"context"
	"path/filepath"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
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

// BuildCounts counts the builds of each app that were built, by [AppKey]. A build that
// was never built is a draft, see [DraftCounts].
func BuildCounts(builds []store.Build) map[string]int {
	counts := map[string]int{}
	for _, b := range builds {
		if b.BuiltAt != nil {
			counts[BuildKey(b)]++
		}
	}
	return counts
}

// DraftCounts counts the builds of each app that were never built (SPEC §5): they hold
// the editor's state of an app the user started on but did not build, and the app list
// marks them "draft".
func DraftCounts(builds []store.Build) map[string]int {
	counts := map[string]int{}
	for _, b := range builds {
		if b.BuiltAt == nil {
			counts[BuildKey(b)]++
		}
	}
	return counts
}
