package desktopentry

import (
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
)

// FileID returns the desktop file ID of the file at path, which must be inside
// appsDir, an "applications" folder of the data directories: the path below appsDir
// with each '/' turned into '-', so kde/foo.desktop is kde-foo.desktop. Neither path
// is resolved against the file system. A path that is appsDir itself, is outside it,
// or cannot be made relative to it is an error.
func FileID(appsDir, path string) (string, error) {
	rel, err := filepath.Rel(appsDir, path)
	if err != nil {
		return "", errors.NewErr(tagNotInApplicationsDir, err).SetMetadata("appsDir", appsDir, "path", path)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New(tagNotInApplicationsDir, "the file is not below the applications folder",
			"appsDir", appsDir, "path", path)
	}
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", "-"), nil
}

const tagNotInApplicationsDir = "NotInApplicationsDir"
