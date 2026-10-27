package install

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind says what a [File] of a build is. The values are the ones the database records
// (SPEC §12), so a caller can convert them with Kind(s) and string(k).
type Kind string

// The kinds of file a build installs.
const (
	// KindProfile is the generated firejail profile, in Jailcat's profiles folder.
	KindProfile Kind = "profile"
	// KindLauncher is the launcher script, in the bin folder or Jailcat's launchers folder.
	KindLauncher Kind = "launcher"
	// KindDesktop is the desktop entry in the application menu folder.
	KindDesktop Kind = "desktop"
	// KindIcon is the SVG icon in the user's scalable icon folder.
	KindIcon Kind = "icon"
	// KindHome is the build's private home folder; it has no content and no SHA-256.
	KindHome Kind = "home"
)

// File is one file (or, for [KindHome], one folder) a build installs.
type File struct {
	// Kind says what the file is.
	Kind Kind
	// Path is the absolute path of the target.
	Path string
	// Content is what is written; nil for [KindHome]. A [File] that [Install] or
	// [Uninstall] returns has no Content.
	Content []byte
	// Mode is the mode the file is given: 0644 for a profile, a desktop entry and an
	// icon, 0755 for a launcher, 0700 for a private home.
	Mode os.FileMode
	// SHA256 is the hex SHA-256 of Content; "" for [KindHome].
	SHA256 string
}

// installRank is the order [Install] writes the kinds in (SPEC §9.5): the desktop entry
// comes last, so the menu never shows an entry whose launcher, icon or home is missing.
func installRank(k Kind) int {
	switch k {
	case KindProfile:
		return 0
	case KindLauncher:
		return 1
	case KindIcon:
		return 2
	case KindHome:
		return 3
	case KindDesktop:
		return 4
	}
	return 5
}

// uninstallRank is the order [Uninstall] removes the kinds in: the desktop entry first,
// so the menu never shows an entry whose launcher or icon is already gone.
func uninstallRank(k Kind) int {
	switch k {
	case KindDesktop:
		return 0
	case KindIcon:
		return 1
	case KindLauncher:
		return 2
	case KindProfile:
		return 3
	case KindHome:
		return 4
	}
	return 5
}

// validSandboxName reports whether name is a non-empty name of the characters [a-z0-9-],
// the sandbox names of SPEC §7.8: such a name cannot hold a path separator or a dot.
func validSandboxName(name string) bool {
	return name != "" && strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") == ""
}

// strictlyInside reports whether path is a clean path below dir (not dir itself). Neither
// path is resolved against the file system.
func strictlyInside(dir, path string) bool {
	if dir == "" || !filepath.IsAbs(dir) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
