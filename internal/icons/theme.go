package icons

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// DirType is the Type key of a directory in index.theme.
type DirType string

// The values of the Type key (Icon Theme Specification, "File Formats").
const (
	// DirFixed holds icons of exactly Size.
	DirFixed DirType = "Fixed"
	// DirScalable holds scalable icons that suit sizes from MinSize to MaxSize.
	DirScalable DirType = "Scalable"
	// DirThreshold holds icons of Size that suit sizes within Threshold of it. It is the
	// default when the key is missing or unknown.
	DirThreshold DirType = "Threshold"
)

// Directory is one directory of a [Theme], from its group in index.theme.
type Directory struct {
	// Path is the directory's path below the theme folder, e.g. "48x48/apps".
	Path string
	// Size is the nominal size in pixels, before Scale.
	Size int
	// Scale is the Scale key (default 1): 2 means the images are for HiDPI screens.
	Scale int
	// Type is the Type key (default [DirThreshold]).
	Type DirType
	// MinSize is the smallest size a [DirScalable] directory suits (default Size).
	MinSize int
	// MaxSize is the largest size a [DirScalable] directory suits (default Size).
	MaxSize int
	// Threshold is the range of a [DirThreshold] directory (default 2).
	Threshold int
}

// pixels is the size of the images of d in real pixels, the number "largest first"
// compares. A scalable directory counts at its MaxSize.
func (d Directory) pixels() int {
	if d.Type == DirScalable {
		return d.MaxSize * d.Scale
	}
	return d.Size * d.Scale
}

// Theme is an icon theme: its index.theme and the folders that hold its icons.
type Theme struct {
	// Name is the theme's folder name in the icon base directories.
	Name string
	// Inherits is the Inherits key: the themes to look in when this one lacks an icon.
	Inherits []string
	// Directories are the Directories and ScaledDirectories of index.theme, in its order.
	// A directory with no group in index.theme, or with no valid Size, is left out.
	Directories []Directory

	// roots are the existing <base>/<Name> folders, in the order of [BaseDirs]. The
	// directories of a theme are looked up in every one of them.
	roots []string
}

// BaseDirs returns the base directories an icon theme or an unthemed icon is looked up
// in, in the order of the Icon Theme Specification: $HOME/.icons, <DataHome>/icons and
// <dir>/icons for each of DataDirs. /usr/share/pixmaps, the last place, is not among
// them (see [PixmapDirs]).
func BaseDirs(dirs xdg.Dirs) []string {
	bases := make([]string, 0, 2+len(dirs.DataDirs))
	bases = append(bases, filepath.Join(dirs.Home, ".icons"), filepath.Join(dirs.DataHome, "icons"))
	for _, d := range dirs.DataDirs {
		bases = append(bases, filepath.Join(d, "icons"))
	}
	return bases
}

// PixmapDirs returns the pixmaps folders, <dir>/pixmaps for each of DataDirs, where
// /usr/share/pixmaps is on a standard system. The unthemed fallback looks in them after
// all of [BaseDirs]. They come from DataDirs so the one place that knows the system's
// folders is [xdg.Dirs].
func PixmapDirs(dirs xdg.Dirs) []string {
	out := make([]string, 0, len(dirs.DataDirs))
	for _, d := range dirs.DataDirs {
		out = append(out, filepath.Join(d, "pixmaps"))
	}
	return out
}

// LoadTheme loads the theme called name. Its index.theme is the first one found in
// [BaseDirs]; its directories are then looked up in every base directory that has a
// folder of that name, as the Icon Theme Specification says. A broken index.theme is not
// an error: its bad lines and directory groups are skipped. When no base directory has an
// index.theme for name, the error has the tag [TagThemeNotFound].
func LoadTheme(ctx context.Context, dirs xdg.Dirs, name string) (Theme, error) {
	if !validName(name) {
		return Theme{}, errors.New(tagThemeNotFound, "not a valid theme name", "theme", name)
	}
	var roots []string
	var index []byte
	found := false
	for _, base := range BaseDirs(dirs) {
		if err := ctx.Err(); err != nil {
			return Theme{}, errors.NewErr(tagLookupCanceled, err).SetMetadata("theme", name, "dir", base)
		}
		root := filepath.Join(base, name)
		info, err := os.Stat(root)
		if err != nil {
			if isAbsent(err) {
				continue
			}
			return Theme{}, errors.NewErr(tagThemeStatFailed, err).SetMetadata("theme", name, "path", root)
		}
		if !info.IsDir() {
			continue
		}
		roots = append(roots, root)
		if found {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, "index.theme"))
		if err != nil {
			if isAbsent(err) {
				continue
			}
			return Theme{}, errors.NewErr(tagIndexThemeReadFailed, err).SetMetadata("theme", name, "path", filepath.Join(root, "index.theme"))
		}
		index, found = data, true
	}
	if !found {
		return Theme{}, errors.New(tagThemeNotFound, "the theme has no index.theme", "theme", name)
	}
	th := parseIndexTheme(index)
	th.Name = name
	th.roots = roots
	return th, nil
}

// parseIndexTheme reads the [Icon Theme] group and the group of every directory it lists.
func parseIndexTheme(data []byte) Theme {
	f := parseINI(data)
	const group = "Icon Theme"
	var th Theme
	for _, p := range f.list(group, "Inherits") {
		if validName(p) {
			th.Inherits = append(th.Inherits, p)
		}
	}
	seen := map[string]bool{}
	names := append(f.list(group, "Directories"), f.list(group, "ScaledDirectories")...)
	for _, p := range names {
		if seen[p] || !filepath.IsLocal(p) {
			continue
		}
		seen[p] = true
		if d, ok := parseDirectory(f, p); ok {
			th.Directories = append(th.Directories, d)
		}
	}
	return th
}

// parseDirectory reads the group named path. It fails when the group is missing or has no
// valid Size; any other bad value falls back to the default of its key.
func parseDirectory(f iniFile, path string) (Directory, bool) {
	size, ok := f.intValue(path, "Size")
	if !ok || size <= 0 {
		return Directory{}, false
	}
	d := Directory{Path: path, Size: size, Scale: 1, Type: DirThreshold, MinSize: size, MaxSize: size, Threshold: 2}
	if v, ok := f.intValue(path, "Scale"); ok && v > 0 {
		d.Scale = v
	}
	if v, ok := f.get(path, "Type"); ok {
		switch DirType(v) {
		case DirFixed, DirScalable, DirThreshold:
			d.Type = DirType(v)
		}
	}
	if v, ok := f.intValue(path, "MinSize"); ok && v > 0 {
		d.MinSize = v
	}
	if v, ok := f.intValue(path, "MaxSize"); ok && v > 0 {
		d.MaxSize = v
	}
	if v, ok := f.intValue(path, "Threshold"); ok && v >= 0 {
		d.Threshold = v
	}
	return d, true
}

// validName reports whether s can be a theme or icon name: one path element.
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\x00")
}

// isAbsent reports whether err says the path is not there: it does not exist, or a part
// of it is a file where a folder should be.
func isAbsent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

const (
	tagIndexThemeReadFailed = "IndexThemeReadFailed"
	tagThemeStatFailed      = "ThemeStatFailed"
)

// TagThemeNotFound is the tag of the error [LoadTheme] returns when a theme has no
// index.theme in any base directory.
const TagThemeNotFound = "ThemeNotFound"

const tagThemeNotFound = TagThemeNotFound

// TagLookupCanceled is the tag of the error [Lookup] and [LoadTheme] return when their
// context is cancelled; the error wraps the context's error.
const TagLookupCanceled = "LookupCanceled"

const tagLookupCanceled = TagLookupCanceled
