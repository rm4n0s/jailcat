package apps

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rm4n0s/errors"
)

// scanPathFolder lists the executables of one $PATH folder (SPEC §4.2) as binary apps.
// seen holds the names an earlier folder already has; the names found here are added.
// A name is taken only by an executable file that is not a firecfg link, so a
// non-executable file, a folder or a firecfg link leaves the name to a later folder,
// as the shell does. A folder that does not exist is ignored; one that cannot be read
// is reported.
func scanPathFolder(dir string, seen map[string]bool) ([]App, []Skipped) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []Skipped{{Path: dir, Reason: "cannot read the folder: " + err.Error()}}
	}
	var apps []App
	for _, e := range entries {
		name := e.Name()
		if seen[name] {
			continue
		}
		path := filepath.Join(dir, name)
		if !isCandidate(path) {
			continue
		}
		seen[name] = true
		apps = append(apps, App{
			ID:         name,
			Kind:       KindBinary,
			Name:       name,
			Binary:     path,
			BinaryName: name,
			Terminal:   true,
		})
	}
	return apps, nil
}
