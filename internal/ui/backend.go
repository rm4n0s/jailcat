package ui

import (
	"context"
	"path/filepath"

	"github.com/rm4n0s/jailcat/internal/apps"
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

// BuildCounts counts the builds of each app, by [AppKey].
func BuildCounts(builds []store.Build) map[string]int {
	counts := map[string]int{}
	for _, b := range builds {
		counts[BuildKey(b)]++
	}
	return counts
}
