package xdg

import (
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
)

// Dirs holds every folder of SPEC §3.1, resolved once from an [Env]. Every field is an
// absolute, cleaned path.
type Dirs struct {
	// Home is the user's home folder.
	Home string
	// DataHome is $XDG_DATA_HOME (default $HOME/.local/share).
	DataHome string
	// ConfigHome is $XDG_CONFIG_HOME (default $HOME/.config).
	ConfigHome string
	// DataDirs is $XDG_DATA_DIRS (default /usr/local/share, /usr/share), absolute entries only.
	DataDirs []string
	// Path is $PATH split on ":", empty entries dropped.
	Path []string

	// Data is Jailcat's data folder, <DataHome>/jailcat.
	Data string
	// Database is the SQLite file, <Data>/jailcat.db.
	Database string
	// Profiles holds the generated profiles, <Data>/profiles.
	Profiles string
	// Imported holds the imported profiles, <Data>/profiles/imported.
	Imported string
	// Homes holds the private homes, <Data>/homes.
	Homes string
	// Launchers holds the launcher scripts when they are not installed in Bin, <Data>/launchers.
	Launchers string
	// Cache is Jailcat's cache folder, $XDG_CACHE_HOME/jailcat (default $HOME/.cache/jailcat).
	Cache string
	// Bin is $XDG_BIN_HOME, else $HOME/.local/bin.
	Bin string
	// Applications is the user's application menu folder, <DataHome>/applications.
	Applications string
	// Icons is the user's scalable icon folder, <DataHome>/icons/hicolor/scalable/apps.
	Icons string
	// FirejailSystem is firejail's system config folder, /etc/firejail.
	FirejailSystem string
	// FirejailUser is firejail's user config folder. It is always $HOME/.config/firejail:
	// firejail ignores $XDG_CONFIG_HOME (profile.c).
	FirejailUser string
}

// Resolve computes every folder of SPEC §3.1 from env. A variable that is unset, empty
// or not absolute is ignored and its default is used (XDG Base Directory Specification);
// in $XDG_DATA_DIRS the relative entries are dropped, and when none is left the default
// list is used. The home folder must be absolute.
func Resolve(env Env) (Dirs, error) {
	home := env.Home()
	if !filepath.IsAbs(home) {
		return Dirs{}, errors.New(tagHomeNotAbsolute, "the home folder is not an absolute path", "home", home)
	}
	home = filepath.Clean(home)

	dataHome, ok := env.absVar("XDG_DATA_HOME")
	if !ok {
		dataHome = filepath.Join(home, ".local", "share")
	}
	configHome, ok := env.absVar("XDG_CONFIG_HOME")
	if !ok {
		configHome = filepath.Join(home, ".config")
	}
	cacheHome, ok := env.absVar("XDG_CACHE_HOME")
	if !ok {
		cacheHome = filepath.Join(home, ".cache")
	}
	bin, ok := env.absVar("XDG_BIN_HOME")
	if !ok {
		bin = filepath.Join(home, ".local", "bin")
	}

	data := filepath.Join(dataHome, "jailcat")
	profiles := filepath.Join(data, "profiles")
	return Dirs{
		Home:           home,
		DataHome:       dataHome,
		ConfigHome:     configHome,
		DataDirs:       dataDirs(env),
		Path:           pathDirs(env),
		Data:           data,
		Database:       filepath.Join(data, "jailcat.db"),
		Profiles:       profiles,
		Imported:       filepath.Join(profiles, "imported"),
		Homes:          filepath.Join(data, "homes"),
		Launchers:      filepath.Join(data, "launchers"),
		Cache:          filepath.Join(cacheHome, "jailcat"),
		Bin:            bin,
		Applications:   filepath.Join(dataHome, "applications"),
		Icons:          filepath.Join(dataHome, "icons", "hicolor", "scalable", "apps"),
		FirejailSystem: "/etc/firejail",
		FirejailUser:   filepath.Join(home, ".config", "firejail"),
	}, nil
}

// dataDirs returns $XDG_DATA_DIRS without its relative and empty entries, or the
// default list when nothing valid is left.
func dataDirs(env Env) []string {
	var dirs []string
	if v, ok := env.Lookup("XDG_DATA_DIRS"); ok {
		for _, e := range strings.Split(v, ":") {
			if e != "" && filepath.IsAbs(e) {
				dirs = append(dirs, filepath.Clean(e))
			}
		}
	}
	if len(dirs) == 0 {
		return []string{"/usr/local/share", "/usr/share"}
	}
	return dirs
}

// pathDirs returns $PATH split on ":" with the empty entries dropped.
func pathDirs(env Env) []string {
	v, _ := env.Lookup("PATH")
	var dirs []string
	for _, e := range strings.Split(v, ":") {
		if e != "" {
			dirs = append(dirs, e)
		}
	}
	return dirs
}

const tagHomeNotAbsolute = "HomeNotAbsolute"
