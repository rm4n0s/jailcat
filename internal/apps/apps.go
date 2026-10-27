package apps

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// Kind tells where an [App] comes from.
type Kind uint8

const (
	// KindDesktop is an app with a desktop entry (SPEC §4.1).
	KindDesktop Kind = iota
	// KindBinary is an executable found on $PATH that no desktop entry runs (SPEC §4.2).
	KindBinary
)

// String returns "desktop" or "binary".
func (k Kind) String() string {
	if k == KindBinary {
		return "binary"
	}
	return "desktop"
}

const (
	// SandboxFlatpak is the SandboxKind of an app started with "flatpak run".
	SandboxFlatpak = "flatpak"
	// SandboxSnap is the SandboxKind of an app whose program is under /snap/bin/.
	SandboxSnap = "snap"
)

// App is something the user can sandbox: a desktop entry or an executable on $PATH.
type App struct {
	// ID is the desktop file ID (such as "code.desktop"), or the name of a binary.
	ID string
	// Kind says whether the app has a desktop entry or is a bare binary.
	Kind Kind
	// Name is the localized Name of the entry; for a binary, its file name.
	Name string
	// GenericName is the localized GenericName of the entry.
	GenericName string
	// Comment is the localized Comment of the entry.
	Comment string
	// Keywords is the localized Keywords of the entry.
	Keywords []string
	// Icon is the raw Icon= value of the entry (a name or an absolute path); empty for
	// a binary.
	Icon string
	// Binary is the absolute path of the program the app runs.
	Binary string
	// BinaryName is the base name of Binary.
	BinaryName string
	// Args is the rest of the main Exec after the program, field codes included, as
	// [desktopentry.SplitExec] returns them.
	Args []string
	// DesktopPath is the path of the .desktop file; empty for a binary.
	DesktopPath string
	// UsesFirejail is true when the Exec starts the program through a firejail wrapper.
	UsesFirejail bool
	// AlreadySandboxed is true for a Flatpak or Snap app: it is listed but cannot be built.
	AlreadySandboxed bool
	// SandboxKind is SandboxFlatpak or SandboxSnap when AlreadySandboxed, else "".
	SandboxKind string
	// NoDisplay is true when the entry asks not to be shown in the menu.
	NoDisplay bool
	// Terminal is true when the app runs in a terminal; always true for a binary.
	Terminal bool
	// Actions are the entry's desktop actions, in the order of its Actions key.
	Actions []Action
}

// Action is a desktop action of an entry ([Desktop Action <id>]).
type Action struct {
	// ID is the action's identifier.
	ID string
	// Name is the localized Name of the action.
	Name string
	// Binary is the program the action runs, resolved like the app's own; empty when it
	// could not be resolved.
	Binary string
	// Args is the rest of the action's Exec after the program.
	Args []string
	// SameBinary is true when Binary is the same file as the app's Binary (SPEC §9.3
	// copies only those actions).
	SameBinary bool
}

// Skipped explains a desktop file or a $PATH folder that is missing from a [Result].
type Skipped struct {
	// Path is the file or folder.
	Path string
	// Reason says why, in words for a person.
	Reason string
}

// Result is what [Scan] found.
type Result struct {
	// Apps are sorted by Name (case-insensitive), then ID.
	Apps []App
	// Skipped lists the desktop files and folders that were dropped, with the reason.
	// A Hidden entry and an ID that an earlier folder already has are not listed:
	// they are dropped on purpose.
	Skipped []Skipped
}

// LocaleFromEnv returns the locale to read localized keys in: the first non-empty of
// LC_ALL, LC_MESSAGES and LANG. "C" and "POSIX" (with or without an encoding) mean no
// locale and give "".
func LocaleFromEnv(env xdg.Env) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v, _ := env.Lookup(name)
		if v == "" {
			continue
		}
		lang := v
		if i := strings.IndexAny(lang, ".@"); i >= 0 {
			lang = lang[:i]
		}
		if lang == "C" || lang == "POSIX" {
			return ""
		}
		return v
	}
	return ""
}

// folder is one folder Scan reads.
type folder struct {
	path string
	// binaries is true for a $PATH folder, false for an applications folder.
	binaries bool
}

// Scan finds every app the user can sandbox (SPEC §4.1, §4.2): the desktop entries
// under <DataHome>/applications and each <DataDirs[i]>/applications, then the
// executables of the folders of dirs.Path. locale is the locale for the localized
// keys (see [LocaleFromEnv]). It reads files and never runs anything.
//
// A file or folder that cannot be read is not an error: it goes to Result.Skipped, and
// a folder that does not exist is ignored. The only error is a cancelled ctx, checked
// before every folder (tag [TagScanCancelled]).
func Scan(ctx context.Context, dirs xdg.Dirs, locale string) (Result, error) {
	folders := []folder{{path: filepath.Join(dirs.DataHome, "applications")}}
	for _, d := range dirs.DataDirs {
		folders = append(folders, folder{path: filepath.Join(d, "applications")})
	}
	for _, d := range dirs.Path {
		// A relative $PATH entry depends on the working directory: the shell would use
		// it, but a desktop app cannot know what it means.
		if filepath.IsAbs(d) {
			folders = append(folders, folder{path: d, binaries: true})
		}
	}

	var res Result
	seenIDs := map[string]bool{}
	seenNames := map[string]bool{}
	var binaries []App
	for _, f := range folders {
		if err := ctx.Err(); err != nil {
			return Result{}, errors.NewErr(tagScanCancelled, err).SetMetadata("folder", f.path)
		}
		if f.binaries {
			found, skipped := scanPathFolder(f.path, seenNames)
			binaries = append(binaries, found...)
			res.Skipped = append(res.Skipped, skipped...)
			continue
		}
		found, skipped := scanApplications(f.path, dirs, locale, seenIDs)
		res.Apps = append(res.Apps, found...)
		res.Skipped = append(res.Skipped, skipped...)
	}

	// A binary a desktop entry runs belongs to that app. A Flatpak or Snap launcher
	// does not count: "flatpak" itself is still a program the user may sandbox.
	runByEntry := map[string]bool{}
	for _, a := range res.Apps {
		if !a.AlreadySandboxed {
			runByEntry[evalOrPlain(a.Binary)] = true
		}
	}
	for _, b := range binaries {
		if !runByEntry[evalOrPlain(b.Binary)] {
			res.Apps = append(res.Apps, b)
		}
	}

	sort.SliceStable(res.Apps, func(i, j int) bool {
		a, b := res.Apps[i], res.Apps[j]
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if an != bn {
			return an < bn
		}
		return a.ID < b.ID
	})
	return res, nil
}

// TagScanCancelled is the tag of the error Scan returns when its context is cancelled.
const TagScanCancelled = "ScanCancelled"

const tagScanCancelled = TagScanCancelled
