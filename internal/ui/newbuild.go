package ui

import (
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
)

// SandboxNameFor returns the sandbox name of a first build of the program binary
// (SPEC §7.8): "firejail-" and the name with every character outside [a-z0-9-] turned
// into a dash (runs of dashes become one).
func SandboxNameFor(binary string) string {
	var sb strings.Builder
	sb.WriteString("firejail-")
	dash := true
	for _, r := range strings.ToLower(binary) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			dash = false
		case !dash:
			sb.WriteByte('-')
			dash = true
		}
	}
	name := strings.TrimRight(sb.String(), "-")
	if name == "firejail" {
		return "firejail-app"
	}
	return name
}

// UniqueSandboxName returns name, or name-2, name-3, ... for the first one that is not
// in taken (SPEC §7.8).
func UniqueSandboxName(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	for n := 2; ; n++ {
		candidate := name + "-" + strconv.Itoa(n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// launcherDefaults returns the launcher fields that come from the app and are the same
// every time: the menu name, the comment and the terminal choice (SPEC §7.8). The
// sandbox name and the icon seed depend on the other builds and on chance, so they are
// not here.
func launcherDefaults(app apps.App) firejail.Settings {
	return firejail.Settings{
		MenuName: app.Name + " (sandboxed)",
		Comment:  app.Comment,
		Terminal: app.Kind == apps.KindBinary || app.Terminal,
	}
}

// newBuildDefaults returns the launcher fields of a new build of the app whose sandbox
// name must differ from the names in taken.
func newBuildDefaults(app apps.App, taken map[string]bool) firejail.Settings {
	s := launcherDefaults(app)
	s.SandboxName = UniqueSandboxName(SandboxNameFor(app.BinaryName), taken)
	s.IconSeed = rand.Int64()
	return s
}
