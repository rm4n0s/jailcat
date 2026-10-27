package apps

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// isExecutable reports whether path, following symbolic links, is a regular file with
// any execute bit.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// isFirecfgLink reports whether path is a symbolic link whose fully resolved target
// has the base name "firejail": what firecfg makes in /usr/local/bin so that a
// program starts inside firejail.
func isFirecfgLink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return filepath.Base(target) == "firejail"
}

// isCandidate reports whether path is an executable file that is not a firecfg link.
func isCandidate(path string) bool {
	return isExecutable(path) && !isFirecfgLink(path)
}

// lookPath returns the first executable regular file called name in the absolute
// folders of dirs.Path, in order, following symbolic links and skipping firecfg links.
// name must be a bare file name. Nothing is run.
func lookPath(name string, dirs xdg.Dirs) (string, bool) {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return "", false
	}
	for _, dir := range dirs.Path {
		if !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, name)
		if isCandidate(p) {
			return p, true
		}
	}
	return "", false
}

// evalOrPlain returns path with its symbolic links resolved, or path cleaned when
// they cannot be.
func evalOrPlain(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	return filepath.Clean(path)
}
