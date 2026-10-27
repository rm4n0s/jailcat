package ui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// homeVar is the variable that stands for the user's home in the paths of profiles.
const homeVar = "${HOME}"

// userMacroNames are the user-folder macros of firejail, as a profile writes them.
func userMacroNames() []string {
	return []string{xdg.MacroDownloads, xdg.MacroMusic, xdg.MacroVideos, xdg.MacroPictures, xdg.MacroDesktop, xdg.MacroDocuments}
}

// HomesDir is the folder of Jailcat's private homes.
func (e *Editor) HomesDir() string { return e.homesDir }

// PrivateHomePath returns the folder the Own private home choice uses: the one the
// settings hold when the home is already private, else the base profile's own, else a
// folder of the private homes named like the sandbox (SPEC §7.1).
func (e *Editor) PrivateHomePath() string {
	if e.final.Home == firejail.HomePrivate && e.final.PrivateDir != "" {
		return e.final.PrivateDir
	}
	return e.defaultPrivateDir(e.final.SandboxName)
}

// defaultPrivateDir is the private home of a sandbox: the base profile's own folder
// when it has one, else a folder of the private homes.
func (e *Editor) defaultPrivateDir(sandbox string) string {
	if b := e.base.Settings; b.Home == firejail.HomePrivate && b.PrivateDir != "" {
		return b.PrivateDir
	}
	if e.homesDir == "" || sandbox == "" {
		return ""
	}
	return filepath.Join(e.homesDir, sandbox)
}

// SetHome makes mode the home folder choice. The private home folder follows it: Own
// private home gets its folder, and every other choice gives the folder back to the
// base profile's value, so only the home counts as changed.
func (e *Editor) SetHome(mode firejail.HomeMode) {
	e.Set(func(s *firejail.Settings) {
		s.Home = mode
		if mode == firejail.HomePrivate {
			s.PrivateDir = e.defaultPrivateDir(s.SandboxName)
			return
		}
		s.PrivateDir = e.base.Settings.PrivateDir
	})
}

// HomeChanged reports whether the home choice, or the folder of the private home,
// differs from the base profile's.
func (e *Editor) HomeChanged() bool {
	return e.Changed(firejail.FieldHome) || e.Changed(firejail.FieldPrivateDir)
}

// ResetHome puts the home choice and its private folder back to the base profile's.
func (e *Editor) ResetHome() {
	e.Reset(firejail.FieldHome)
	e.Reset(firejail.FieldPrivateDir)
}

// KeepsHomeOut reports whether the home choice keeps the real home out of the sandbox:
// Empty home and Own private home.
func (e *Editor) KeepsHomeOut() bool {
	return e.final.Home == firejail.HomeEmpty || e.final.Home == firejail.HomePrivate
}

// canonicalFolderPath writes a folder path the way the folder list keeps it: no slash at
// the end, and the home as ${HOME} whether it was typed as ~ or as the real path, so the
// same folder is never listed twice under two spellings.
func canonicalFolderPath(path, home string) string {
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "/"
		}
	}
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/') {
		return homeVar + rest
	}
	if home != "" && filepath.IsAbs(home) {
		h := filepath.Clean(home)
		if path == h {
			return homeVar
		}
		if rest, ok := strings.CutPrefix(path, h+"/"); ok {
			return homeVar + "/" + rest
		}
	}
	return path
}

// displayFolderPath writes a folder path for the user: the home as ~, and a user-folder
// macro as the name of the folder. hint is the path as the profile writes it when that
// differs (a macro), else "".
func displayFolderPath(path, home string) (text, hint string) {
	path = canonicalFolderPath(path, home)
	for _, macro := range userMacroNames() {
		if rest, ok := strings.CutPrefix(path, macro); ok && (rest == "" || rest[0] == '/') {
			name := strings.ToLower(strings.Trim(macro, "${}"))
			return strings.ToUpper(name[:1]) + name[1:] + rest, path
		}
	}
	if rest, ok := strings.CutPrefix(path, homeVar); ok && (rest == "" || rest[0] == '/') {
		return "~" + rest, ""
	}
	return path, ""
}

// displayPlainPath writes an absolute path with ~ for the home.
func displayPlainPath(path, home string) string {
	text, _ := displayFolderPath(path, home)
	return text
}

// CheckFolder checks a folder the user wants to add, typed or chosen. It returns the
// path as the list keeps it, or, when the folder cannot be added, a message for the
// user and an empty path. The rules are those of [firejail.Validate] (a full path,
// nothing firejail refuses, nothing inside the real home with an empty or private home),
// and that the folder is not in the list already.
func (e *Editor) CheckFolder(path string) (canonical, problem string) {
	canonical = canonicalFolderPath(path, e.home)
	text, _ := displayFolderPath(canonical, e.home)
	probe := firejail.Settings{Home: e.final.Home, Folders: []firejail.Folder{{Path: canonical, Access: firejail.AccessReadWrite}}}
	for _, p := range firejail.Validate(probe, e.home) {
		if p.Field == "" && p.Path == canonical {
			return "", strings.ReplaceAll(p.Message, canonical, text)
		}
	}
	if canonical == homeVar {
		return "", "The home folder itself cannot be added to the list. Choose a folder inside it."
	}
	if slices.ContainsFunc(e.final.Folders, func(f firejail.Folder) bool { return f.Path == canonical }) {
		return "", text + " is in the list already."
	}
	return canonical, ""
}

// AddFolder adds a folder, read-write, to the list. path is the answer of
// [Editor.CheckFolder]. A row of the base profile that the user removed comes back as it
// was instead.
func (e *Editor) AddFolder(path string) {
	inBase := slices.ContainsFunc(e.base.Settings.Folders, func(f firejail.Folder) bool { return f.Path == path })
	inFinal := slices.ContainsFunc(e.final.Folders, func(f firejail.Folder) bool { return f.Path == path })
	if inBase && !inFinal {
		e.ResetFolder(path)
		return
	}
	e.Set(func(s *firejail.Settings) {
		s.Folders = append(s.Folders, firejail.Folder{Path: path, Access: firejail.AccessReadWrite})
	})
}

// SetFolderAccess changes the access of a row. A locked row (SPEC §9.1) cannot be
// loosened, so it is left as it is.
func (e *Editor) SetFolderAccess(path string, access firejail.Access) {
	e.Set(func(s *firejail.Settings) {
		for i := range s.Folders {
			if s.Folders[i].Path == path && !s.Folders[i].Locked {
				s.Folders[i].Access = access
			}
		}
	})
}

// RemoveFolder removes a row from the list. A row of the base profile stays known as
// removed (see [Editor.ChangedFolders]); a locked row cannot be removed.
func (e *Editor) RemoveFolder(path string) {
	e.Set(func(s *firejail.Settings) {
		s.Folders = slices.DeleteFunc(s.Folders, func(f firejail.Folder) bool { return f.Path == path && !f.Locked })
	})
}

// RemovedFolders returns the rows of the base profile the user removed, as the base has
// them, in the order they were removed.
func (e *Editor) RemovedFolders() []firejail.Folder {
	var out []firejail.Folder
	for _, fo := range e.ChangedFolders() {
		if !fo.Removed {
			continue
		}
		i := slices.IndexFunc(e.base.Settings.Folders, func(f firejail.Folder) bool { return f.Path == fo.Path })
		if i >= 0 {
			out = append(out, e.base.Settings.Folders[i])
		}
	}
	return out
}
