package xdg

import "path/filepath"

// OnPath reports whether dir, cleaned, is one of the folders of d.Path, cleaned. It
// compares paths as text and does not follow symbolic links.
func OnPath(d Dirs, dir string) bool {
	want := filepath.Clean(dir)
	for _, p := range d.Path {
		if filepath.Clean(p) == want {
			return true
		}
	}
	return false
}
