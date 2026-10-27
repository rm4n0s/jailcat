package ui

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/icons"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// LocalBackend is the [Backend] that reads the real system: the desktop entries, the
// icon themes and firejail through the packages of Jailcat, and the builds from the
// database.
type LocalBackend struct {
	dirs   xdg.Dirs
	store  *store.Store
	locale string
	logger *slog.Logger

	// iconMu guards the three fields below: the app list looks up many icons at once, and
	// the same icon is asked again every time its row scrolls into view.
	iconMu sync.Mutex
	// theme is the icon theme, read on first use; themeSet says it was.
	theme    string
	themeSet bool
	// iconCache keeps the answers of icons.Lookup by Icon= value.
	iconCache map[string]iconResult
}

// iconResult is a remembered answer of [icons.Lookup].
type iconResult struct {
	icon icons.Icon
	err  error
}

// NewLocalBackend returns the backend for the folders dirs and the open database st.
// locale is the locale for the localized keys of desktop entries (see
// [apps.LocaleFromEnv]).
func NewLocalBackend(dirs xdg.Dirs, st *store.Store, locale string, logger *slog.Logger) *LocalBackend {
	return &LocalBackend{dirs: dirs, store: st, locale: locale, logger: logger, iconCache: map[string]iconResult{}}
}

// ScanApps scans the desktop entries and the $PATH folders.
func (b *LocalBackend) ScanApps(ctx context.Context) (apps.Result, error) {
	return apps.Scan(ctx, b.dirs, b.locale)
}

// ListBuilds returns the builds of the database.
func (b *LocalBackend) ListBuilds(ctx context.Context) ([]store.Build, error) {
	return b.store.ListBuilds(ctx)
}

// LookupIcon finds the app's icon in the user's current icon theme, hicolor and the
// pixmaps folder. The answer, a failure included, is kept for the next call with the
// same Icon= value; a cancelled lookup is not.
func (b *LocalBackend) LookupIcon(ctx context.Context, app apps.App) (icons.Icon, error) {
	b.iconMu.Lock()
	res, ok := b.iconCache[app.Icon]
	theme, themeSet := b.theme, b.themeSet
	b.iconMu.Unlock()
	if ok {
		return res.icon, res.err
	}

	if !themeSet {
		theme = icons.CurrentTheme(ctx, b.dirs)
		if ctx.Err() == nil {
			b.iconMu.Lock()
			b.theme, b.themeSet = theme, true
			b.iconMu.Unlock()
		}
	}
	icon, err := icons.Lookup(ctx, b.dirs, theme, app.Icon)
	if err != nil {
		if appErr, isApp := errors.FromError(err); isApp && appErr.Tag == icons.TagIconNotFound {
			b.logger.Debug("no icon found", "app", app.ID, "icon", app.Icon, "err", err)
			b.remember(app.Icon, iconResult{err: err})
		}
		return icons.Icon{}, err
	}
	b.remember(app.Icon, iconResult{icon: icon})
	return icon, nil
}

// remember keeps the answer for an Icon= value.
func (b *LocalBackend) remember(value string, res iconResult) {
	b.iconMu.Lock()
	b.iconCache[value] = res
	b.iconMu.Unlock()
}

// Firejail finds firejail on $PATH and runs "firejail --version". A missing firejail
// gives Missing and no error; a firejail that cannot report its version gives its path
// and the error.
func (b *LocalBackend) Firejail(ctx context.Context) (FirejailInfo, error) {
	path, err := firejail.FindFirejail(b.dirs)
	if err != nil {
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == firejail.TagFirejailNotFound {
			b.logger.Debug("firejail is not installed", "err", err)
			return FirejailInfo{Missing: true}, nil
		}
		return FirejailInfo{}, err
	}
	version, err := firejail.Version(ctx, path)
	if err != nil {
		return FirejailInfo{Path: path}, err
	}
	return FirejailInfo{Path: path, Version: version}, nil
}

// CPUCount returns the number of logical cores the program can use ([runtime.NumCPU]).
func (b *LocalBackend) CPUCount() int { return runtime.NumCPU() }

// BinOnPath reports whether the bin folder is one of the folders of $PATH.
func (b *LocalBackend) BinOnPath() bool {
	return xdg.OnPath(b.dirs, b.dirs.Bin)
}

// ProgramOnPath reports whether name is an executable regular file (a symbolic link is
// followed) in one of the absolute folders of $PATH. A name with a path separator is not
// a program name and is never found.
func (b *LocalBackend) ProgramOnPath(ctx context.Context, name string) bool {
	if name == "" || filepath.Base(name) != name {
		return false
	}
	for _, dir := range b.dirs.Path {
		if ctx.Err() != nil {
			return false
		}
		if !filepath.IsAbs(dir) {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return true
		}
	}
	return false
}
