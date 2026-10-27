package apps

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// snapBin is the folder of the launchers snapd makes.
const snapBin = "/snap/bin/"

// command is an Exec value reduced to the program it runs.
type command struct {
	// binary is the absolute path of the program.
	binary string
	// args are the words after the program.
	args []string
	// usesFirejail is true when a firejail wrapper started the program.
	usesFirejail bool
	// sandbox is SandboxFlatpak, SandboxSnap or "".
	sandbox string
}

// resolveCommand finds the program of an Exec value split into words (SPEC §4.1): it
// skips the "env" and firejail wrappers, then looks the program up. The second result
// is the reason the program cannot be used, or "".
func resolveCommand(words []string, dirs xdg.Dirs) (command, string) {
	var cmd command
	for {
		if len(words) == 0 {
			return command{}, "the Exec key has no program"
		}
		switch filepath.Base(words[0]) {
		case "env":
			rest, reason := skipEnv(words[1:])
			if reason != "" {
				return command{}, reason
			}
			words = rest
			continue
		case "firejail":
			cmd.usesFirejail = true
			words = skipFirejail(words[1:])
			continue
		}
		break
	}

	prog, args := words[0], words[1:]
	binary, reason := findProgram(prog, dirs)
	if reason != "" {
		return command{}, reason
	}
	cmd.binary, cmd.args = binary, args
	switch {
	case filepath.Base(prog) == "flatpak" && slices.Contains(args, "run"):
		cmd.sandbox = SandboxFlatpak
	case strings.HasPrefix(binary, snapBin):
		cmd.sandbox = SandboxSnap
	}
	return cmd, ""
}

// skipEnv skips the options and the NAME=value words that follow "env" and returns the
// words from the program on.
func skipEnv(words []string) ([]string, string) {
	i := 0
	for i < len(words) {
		w := words[i]
		switch {
		case w == "--":
			i++
			// The end of the options; NAME=value words may still follow.
			for i < len(words) && isAssignment(words[i]) {
				i++
			}
			return finishEnv(words[i:])
		case w == "-u" || w == "--unset":
			i += 2
		case w == "-i" || w == "-" || w == "-0" || w == "--ignore-environment" || w == "--null",
			strings.HasPrefix(w, "--unset="), strings.HasPrefix(w, "-u") && len(w) > 2 && !strings.HasPrefix(w, "--"):
			i++
		case strings.HasPrefix(w, "-"):
			return nil, fmt.Sprintf("the env option %q is not supported", w)
		case isAssignment(w):
			i++
		default:
			return finishEnv(words[i:])
		}
	}
	return finishEnv(nil)
}

// finishEnv checks that env was followed by a program.
func finishEnv(rest []string) ([]string, string) {
	if len(rest) == 0 {
		return nil, "env has no program"
	}
	return rest, ""
}

// isAssignment reports whether w is a NAME=value word of env.
func isAssignment(w string) bool {
	return strings.Index(w, "=") > 0
}

// skipFirejail skips the options of a firejail wrapper: the words that start with "-"
// up to the first one that does not, or up to "--", which is skipped too.
func skipFirejail(words []string) []string {
	i := 0
	for i < len(words) && strings.HasPrefix(words[i], "-") {
		if words[i] == "--" {
			i++
			break
		}
		i++
	}
	return words[i:]
}

// findProgram returns the absolute path of the program word prog. A bare name is looked
// up on $PATH; an absolute path must be an executable file, and when it is a firecfg
// link the name is looked up on $PATH past it. A path under /snap/bin/ is taken as it
// is, since Jailcat does not build those. The second result is the reason when the
// program cannot be found.
func findProgram(prog string, dirs xdg.Dirs) (string, string) {
	switch {
	case filepath.IsAbs(prog):
		p := filepath.Clean(prog)
		if strings.HasPrefix(p, snapBin) {
			return p, ""
		}
		if !isExecutable(p) {
			return "", fmt.Sprintf("%s is not an executable file", p)
		}
		if isFirecfgLink(p) {
			found, ok := lookPath(filepath.Base(p), dirs)
			if !ok {
				return "", fmt.Sprintf("%s is a firecfg link and %q is not on $PATH past it", p, filepath.Base(p))
			}
			return found, ""
		}
		return p, ""
	case strings.ContainsRune(prog, filepath.Separator):
		return "", fmt.Sprintf("%q is a relative path", prog)
	}
	found, ok := lookPath(prog, dirs)
	if !ok {
		return "", fmt.Sprintf("%q is not an executable on $PATH", prog)
	}
	return found, ""
}

// tryExecFound reports whether a TryExec value names an executable file: an absolute
// path, or a name on $PATH.
func tryExecFound(value string, dirs xdg.Dirs) bool {
	if filepath.IsAbs(value) {
		return isExecutable(value)
	}
	_, ok := lookPath(value, dirs)
	return ok
}
