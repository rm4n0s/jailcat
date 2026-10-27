package icons

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// Format is the file format of an [Icon].
type Format string

// The formats Lookup returns. The value is the file extension.
const (
	// FormatSVG is a scalable vector image (.svg).
	FormatSVG Format = "svg"
	// FormatPNG is a PNG image (.png).
	FormatPNG Format = "png"
	// FormatXPM is an X pixmap (.xpm), the last resort: Jailcat cannot draw it.
	FormatXPM Format = "xpm"
)

// Icon is the file [Lookup] chose for an icon.
type Icon struct {
	// Path is the absolute path of the image file.
	Path string
	// Format is the file's format.
	Format Format
	// Size is the image's size in real pixels (the directory's Size times its Scale). It is
	// 0 for a scalable icon and for an icon whose size is not known: an absolute path and
	// an unthemed icon.
	Size int
}

// defaultTheme is the theme every lookup ends with, and the theme in use when no
// setting names one.
const defaultTheme = "hicolor"

// Lookup finds the image file for an application's Icon= value.
//
// An absolute iconValue is returned as it is when the file exists; its format comes from
// its extension. Anything else is an icon name (a trailing .svg, .png or .xpm is dropped,
// as many desktops do) that is looked up in the theme themeName, the themes it inherits
// from, and hicolor; then, unthemed, directly in the base directories and the pixmaps
// folders (see [BaseDirs] and [PixmapDirs]). An .xpm file is used only when no SVG or PNG
// exists anywhere.
//
// This is deliberately "largest first", not the Icon Theme Specification's "closest to a
// requested size": Jailcat draws the icon at several sizes and uses it as the source of a
// build's icon (SPEC §10.2), so the best file is the one with the most detail. In a theme
// that is, in this order: an SVG in a Scalable directory; then the largest size of the
// Fixed and Threshold directories (a Scale 2 directory counts twice its Size), where an
// SVG wins over a PNG of the same size.
//
// When nothing is found the error has the tag [TagIconNotFound]. A cancelled ctx stops the
// scan with [TagLookupCanceled]. Lookup keeps no state between calls.
func Lookup(ctx context.Context, dirs xdg.Dirs, themeName, iconValue string) (Icon, error) {
	if filepath.IsAbs(iconValue) {
		return lookupAbsolute(iconValue)
	}
	name := stripExt(iconValue)
	if !validName(name) {
		return Icon{}, errors.New(tagIconNotFound, "not a valid icon name", "name", iconValue)
	}
	if !validName(themeName) {
		themeName = defaultTheme
	}

	w := &chain{dirs: dirs, seen: map[string]bool{}}
	for _, exts := range [][]Format{{FormatSVG, FormatPNG}, {FormatXPM}} {
		var icon Icon
		var ok bool
		var err error
		if len(w.order) == 0 {
			icon, ok, err = w.walk(ctx, themeName, name, exts)
			if err == nil && !ok {
				icon, ok, err = w.walk(ctx, defaultTheme, name, exts)
			}
		} else {
			// The first pass found nothing, so it walked the whole chain.
			for _, th := range w.order {
				icon, ok, err = findInTheme(ctx, th, name, exts)
				if err != nil || ok {
					break
				}
			}
		}
		if err != nil {
			return Icon{}, err
		}
		if ok {
			return icon, nil
		}
		icon, ok, err = findUnthemed(ctx, dirs, name, exts)
		if err != nil {
			return Icon{}, err
		}
		if ok {
			return icon, nil
		}
	}
	return Icon{}, errors.New(tagIconNotFound, "no icon of that name", "name", iconValue, "theme", themeName)
}

// lookupAbsolute returns the file at path when it exists and has an extension of a known
// format.
func lookupAbsolute(path string) (Icon, error) {
	f, ok := formatOf(path)
	if !ok {
		return Icon{}, errors.New(tagIconNotFound, "the file is not an SVG, PNG or XPM image", "name", path)
	}
	isFile, err := fileExists(path)
	if err != nil {
		return Icon{}, errors.NewErr(tagIconStatFailed, err).SetMetadata("path", path)
	}
	if !isFile {
		return Icon{}, errors.New(tagIconNotFound, "the file does not exist", "name", path)
	}
	return Icon{Path: path, Format: f}, nil
}

// chain walks a theme and its parents, loading each index.theme once.
type chain struct {
	dirs xdg.Dirs
	seen map[string]bool
	// order holds the themes loaded so far, in the order they were searched.
	order []Theme
}

// walk searches the theme called theme, then, depth first, the themes it inherits from.
// A theme already visited (an Inherits loop, or hicolor reached twice) is skipped, and so
// is a theme with no index.theme.
func (w *chain) walk(ctx context.Context, theme, name string, exts []Format) (Icon, bool, error) {
	if w.seen[theme] {
		return Icon{}, false, nil
	}
	w.seen[theme] = true
	th, err := LoadTheme(ctx, w.dirs, theme)
	if err != nil {
		if appErr, ok := errors.FromError(err); ok && appErr.Tag == tagThemeNotFound {
			return Icon{}, false, nil
		}
		return Icon{}, false, err
	}
	w.order = append(w.order, th)
	icon, ok, err := findInTheme(ctx, th, name, exts)
	if err != nil || ok {
		return icon, ok, err
	}
	for _, parent := range th.Inherits {
		icon, ok, err = w.walk(ctx, parent, name, exts)
		if err != nil || ok {
			return icon, ok, err
		}
	}
	return Icon{}, false, nil
}

// findInTheme looks for name in one theme, in every base directory that has the theme's
// folder. exts are in order of preference at the same size.
func findInTheme(ctx context.Context, th Theme, name string, exts []Format) (Icon, bool, error) {
	if slices.Contains(exts, FormatSVG) {
		for _, d := range th.Directories {
			if d.Type != DirScalable {
				continue
			}
			if err := ctx.Err(); err != nil {
				return Icon{}, false, errors.NewErr(tagLookupCanceled, err).SetMetadata("theme", th.Name, "dir", d.Path)
			}
			for _, root := range th.roots {
				path := filepath.Join(root, d.Path, name+".svg")
				isFile, err := fileExists(path)
				if err != nil {
					return Icon{}, false, errors.NewErr(tagIconStatFailed, err).SetMetadata("path", path)
				}
				if isFile {
					return Icon{Path: path, Format: FormatSVG}, true, nil
				}
			}
		}
	}

	// The remaining directories, the largest first. A scalable directory is searched only
	// for formats other than SVG, which the loop above already tried.
	dirs := slices.Clone(th.Directories)
	sort.SliceStable(dirs, func(i, j int) bool { return dirs[i].pixels() > dirs[j].pixels() })
	for start := 0; start < len(dirs); {
		end := start
		for end < len(dirs) && dirs[end].pixels() == dirs[start].pixels() {
			end++
		}
		// Directories of the same size: the formats in order of preference first, so an SVG
		// in any of them beats a PNG in any of them.
		for _, ext := range exts {
			for _, d := range dirs[start:end] {
				if d.Type == DirScalable && ext == FormatSVG {
					continue
				}
				if err := ctx.Err(); err != nil {
					return Icon{}, false, errors.NewErr(tagLookupCanceled, err).SetMetadata("theme", th.Name, "dir", d.Path)
				}
				for _, root := range th.roots {
					path := filepath.Join(root, d.Path, name+"."+string(ext))
					isFile, err := fileExists(path)
					if err != nil {
						return Icon{}, false, errors.NewErr(tagIconStatFailed, err).SetMetadata("path", path)
					}
					if isFile {
						return Icon{Path: path, Format: ext, Size: d.pixels()}, true, nil
					}
				}
			}
		}
		start = end
	}
	return Icon{}, false, nil
}

// findUnthemed looks for <folder>/<name>.<ext> in every base directory, then in every
// pixmaps folder (Icon Theme Specification, "LookupFallbackIcon").
func findUnthemed(ctx context.Context, dirs xdg.Dirs, name string, exts []Format) (Icon, bool, error) {
	folders := append(BaseDirs(dirs), PixmapDirs(dirs)...)
	for _, folder := range folders {
		if err := ctx.Err(); err != nil {
			return Icon{}, false, errors.NewErr(tagLookupCanceled, err).SetMetadata("dir", folder)
		}
		for _, ext := range exts {
			path := filepath.Join(folder, name+"."+string(ext))
			isFile, err := fileExists(path)
			if err != nil {
				return Icon{}, false, errors.NewErr(tagIconStatFailed, err).SetMetadata("path", path)
			}
			if isFile {
				return Icon{Path: path, Format: ext}, true, nil
			}
		}
	}
	return Icon{}, false, nil
}

// fileExists reports whether path is a file (or a link to one). A path that is not there
// is not an error; any other failure is returned as the system gave it.
func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if isAbsent(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

// formatOf returns the format of the file called path, from its extension.
func formatOf(path string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return FormatSVG, true
	case ".png":
		return FormatPNG, true
	case ".xpm":
		return FormatXPM, true
	}
	return "", false
}

// stripExt drops a trailing .svg, .png or .xpm from an icon name. Other dots are part of
// the name (org.gnome.Calculator).
func stripExt(name string) string {
	if _, ok := formatOf(name); ok {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

const tagIconStatFailed = "IconStatFailed"

// TagIconNotFound is the tag of the error [Lookup] returns when no icon file exists for
// the value. The error's metadata "name" is the value that was asked for. The UI shows a
// generic icon for it.
const TagIconNotFound = "IconNotFound"

const tagIconNotFound = TagIconNotFound
