package firejail

import (
	"path/filepath"
	"slices"
)

// vscodeFlavors are the VS Code programs of SPEC §7.1: the base name of the binary and
// the name of its config folder under $XDG_CONFIG_HOME, in matching order.
var vscodeFlavors = [...]struct{ binary, config string }{
	{"code", "Code"},
	{"code-insiders", "Code - Insiders"},
	{"code-oss", "Code - OSS"},
	{"codium", "VSCodium"},
	{"codium-insiders", "VSCodium - Insiders"},
	{"vscodium", "VSCodium"},
}

// VSCodeConfigFolder returns the name of the config folder (under $XDG_CONFIG_HOME) of
// the VS Code program at binary, found by the binary's base name (SPEC §7.1). The
// second result is false when the binary is not a VS Code program.
func VSCodeConfigFolder(binary string) (string, bool) {
	name := filepath.Base(binary)
	for _, f := range vscodeFlavors {
		if f.binary == name {
			return f.config, true
		}
	}
	return "", false
}

// IsVSCode reports whether the binary is a VS Code program (see [VSCodeConfigFolder]).
func IsVSCode(binary string) bool {
	_, ok := VSCodeConfigFolder(binary)
	return ok
}

// VSCodeSpec names the two folders the profile lines of the VS Code switches (SPEC §7.1)
// are about. [ProfileSpec.VSCode] is nil for an app that is not VS Code.
type VSCodeSpec struct {
	// DataDir is the absolute path of the build's own data folder,
	// <data>/appdata/<sandbox name>.
	DataDir string
	// ConfigDir is the absolute path of the user's VS Code config folder, for example
	// ~/.config/Code.
	ConfigDir string
}

// NewVSCodeSpec returns the folders of the VS Code switches for the program at binary
// and the build named sandbox: the data folder is appData/sandbox and the config folder
// is configHome plus the program's config folder name. Both arguments are absolute
// folders ([xdg.Dirs] AppData and ConfigHome). It returns nil when binary is not a
// VS Code program.
func NewVSCodeSpec(binary, appData, configHome, sandbox string) *VSCodeSpec {
	folder, ok := VSCodeConfigFolder(binary)
	if !ok {
		return nil
	}
	return &VSCodeSpec{
		DataDir:   filepath.Join(appData, sandbox),
		ConfigDir: filepath.Join(configHome, folder),
	}
}

// VSCodeSharedNames returns the names, in the data folder's User/ folder, of the files
// and folders that "Share settings with my VS Code" links to the config folder's
// (SPEC §7.1).
func VSCodeSharedNames() []string {
	return []string{"settings.json", "keybindings.json", "snippets"}
}

// VSCodeShellFiles returns the shell startup files and folders the "Shell files for the
// terminal" switch shows read-only, relative to the home (SPEC §7.1).
func VSCodeShellFiles() []string {
	return []string{
		".bashrc", ".bash_profile", ".bash_login", ".profile", ".bash_aliases", ".inputrc",
		".bash_completion", ".local/share/bash-completion", ".zshenv", ".zprofile", ".zshrc",
		".zlogin", ".config/fish", ".config/starship.toml",
	}
}

// WithAppDefaults returns base with the base values of the settings that Jailcat itself
// gives an app (SPEC §7.1): the three VS Code switches are on for a VS Code program and
// off for any other. They are not lines of a profile, so no profile sets them; the base
// value is the value of a new build, which is also what a build made before the switches
// existed gets when it is opened. The Base is not changed.
func WithAppDefaults(base Base, binary string) Base {
	base.Settings = base.Settings.Clone()
	on := IsVSCode(binary)
	base.Settings.VSCodeOwnData = on
	base.Settings.VSCodeShareSettings = on
	base.Settings.VSCodeShellFiles = on
	return base
}

// vscodeHomeAllowsData reports whether Jailcat makes the data folder and writes its
// profile lines: with Full home and Only the folders I choose. With an empty or private
// home the same path names a folder of the sandbox's own home.
func vscodeHomeAllowsData(h HomeMode) bool {
	return slices.Contains([]HomeMode{HomeFull, HomeChosen}, h)
}

// VSCodeDataOnDisk reports whether Jailcat makes the VS Code data folder for the
// settings: "Own VS Code data folder" is on with Full home or Only the folders I choose.
func (s Settings) VSCodeDataOnDisk() bool {
	return s.VSCodeOwnData && vscodeHomeAllowsData(s.Home)
}

// VSCodeLinksSettings reports whether the data folder's settings, keybindings and
// snippets are links to the user's own: "Share settings with my VS Code" is on, in a
// data folder Jailcat makes.
func (s Settings) VSCodeLinksSettings() bool {
	return s.VSCodeDataOnDisk() && s.VSCodeShareSettings
}

// VSCodeShowsShellFiles reports whether the sandbox shows the shell startup files:
// "Shell files for the terminal" is on with Only the folders I choose.
func (s Settings) VSCodeShowsShellFiles() bool {
	return s.VSCodeShellFiles && s.Home == HomeChosen
}
