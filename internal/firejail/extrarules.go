package firejail

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// The tags of the [Problem]s that [CheckExtraRules] reports.
const (
	// ProblemExtraCommand is a row whose command firejail does not have.
	ProblemExtraCommand = "ExtraCommandUnknown"
	// ProblemExtraCondition is a "?CONDITION:" prefix firejail does not know, one with
	// no colon or no line after it, or "quiet" or "include" under a condition.
	ProblemExtraCondition = "ExtraConditionInvalid"
	// ProblemExtraInclude is an include of a file that is not in firejail's system
	// folder, its user folder or Jailcat's imported profiles.
	ProblemExtraInclude = "ExtraIncludeOutside"
	// ProblemExtraArgs is a row whose arguments are missing, are too many or are not
	// well formed (a number, a list, an address).
	ProblemExtraArgs = "ExtraArgumentsInvalid"
	// ProblemExtraCovered is a row that a setting of the form already covers. It is a
	// warning: the form is the one place for that setting.
	ProblemExtraCovered = "ExtraRowCovered"
)

// extraChecker holds what the check of one set of rows needs.
type extraChecker struct {
	dirs  xdg.Dirs
	base  Base
	cover coverage
	out   []Problem
}

// add records a problem of a row.
func (c *extraChecker) add(row int, sev Severity, tag string, f Field, path, format string, args ...any) {
	c.out = append(c.out, Problem{Field: f, Path: path, Tag: tag, Severity: sev, Row: row,
		Message: fmt.Sprintf("Row %d: %s", row, fmt.Sprintf(format, args...))})
}

// CheckExtraRules checks the rows of the Extra rules tab (SPEC §7.9), one problem per
// thing wrong, in the order of the rows. Row numbers start at 1 and count the empty rows.
//
// Each row is first normalized as firejail does (comments and blank rows are fine). A
// "?CONDITION:" prefix must name a condition firejail knows. The command must be one of
// [Commands]. "include" may name only a file in dirs.FirejailSystem, dirs.FirejailUser or
// dirs.Imported (a bare name is looked up in the first two, as firejail does). A row with
// a new line or a NUL is an error. A row that a field of the form covers
// (the commands the writer uses for it, see [Generate]) is a warning that names the
// field; so is an "ignore" row that removes a line of base that a field owns. Arguments
// are checked for the commands whose arguments the model reads (numbers, lists, DNS
// addresses, environment variables); the file system is not looked at.
func CheckExtraRules(rows []string, dirs xdg.Dirs, base Base) []Problem {
	c := &extraChecker{dirs: dirs, base: base, cover: coveredBy()}
	for i, raw := range rows {
		c.row(i+1, raw)
	}
	return c.out
}

// row checks one row.
func (c *extraChecker) row(n int, raw string) {
	if reason := rawRowReason(raw); reason != "" {
		c.add(n, SeverityError, ProblemValue, "", "", "The row cannot be used: it has %s.", reason)
		return
	}
	text := Normalize(raw)
	if text == "" {
		return
	}
	text, ok := c.conditionals(n, text)
	if !ok {
		return
	}
	cmd, args := splitCommand(text)
	if !IsCommand(cmd) {
		c.add(n, SeverityError, ProblemExtraCommand, "", "", "Firejail has no command %q.", cmd)
		return
	}
	if msg := argsProblem(cmd, args); msg != "" {
		c.add(n, SeverityError, ProblemExtraArgs, "", "", "%s", msg)
		return
	}
	switch cmd {
	case "include":
		c.include(n, args)
	case "ignore":
		c.ignore(n, args)
	}
	if f, ok := c.cover.field(text); ok {
		c.add(n, SeverityWarning, ProblemExtraCovered, f, "",
			"%q is the setting %q of the %s tab. Change it there, so the form shows it.", text, fieldLabel(f), tabLabel(f.Tab()))
	}
}

// conditionals checks the "?CONDITION:" prefixes of a normalized row (profile_check_conditional
// of profile.c) and returns the line after them.
func (c *extraChecker) conditionals(n int, text string) (string, bool) {
	for strings.HasPrefix(text, "?") {
		ptr := text[1:]
		var name string
		for _, cond := range conditionals() {
			if strings.HasPrefix(ptr, cond.name) {
				name = cond.name
				break
			}
		}
		if name == "" {
			got, _, _ := strings.Cut(ptr, ":")
			c.add(n, SeverityError, ProblemExtraCondition, "", "", "Firejail does not know the condition %q.", got)
			return "", false
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(ptr, name), " ")
		rest, hasColon := strings.CutPrefix(rest, ":")
		switch {
		case !hasColon:
			c.add(n, SeverityError, ProblemExtraCondition, "", "", "Put a colon after the condition %s.", name)
			return "", false
		case rest == "":
			c.add(n, SeverityError, ProblemExtraCondition, "", "", "Put a line after the condition %s.", name)
			return "", false
		}
		rest = strings.TrimPrefix(rest, " ")
		if strings.HasPrefix(rest, "quiet") || strings.HasPrefix(rest, "include") {
			c.add(n, SeverityError, ProblemExtraCondition, "", "", "Firejail does not allow quiet or include under a condition.")
			return "", false
		}
		text = rest
	}
	return text, true
}

// include checks the file an include row names: it must be in one of the three folders.
// A bare name is looked up in the user folder and the system folder, so it is always
// in one of them (profile_read of profile.c).
func (c *extraChecker) include(n int, args string) {
	if strings.HasPrefix(args, "$HOME") {
		c.add(n, SeverityError, ProblemExtraInclude, "", "", "Firejail does not accept $HOME in a profile; write ${HOME}.")
		return
	}
	path := args
	switch {
	case strings.HasPrefix(path, "${HOME}"):
		path = c.dirs.Home + path[len("${HOME}"):]
	case strings.HasPrefix(path, "~"):
		path = c.dirs.Home + path[1:]
	case strings.HasPrefix(path, "${CFG}"):
		path = c.dirs.FirejailSystem + path[len("${CFG}"):]
	}
	if bad := invalidFilename(path); bad != "" {
		c.add(n, SeverityError, ProblemExtraInclude, "", "", "Firejail does not accept %q as a file name: %s.", args, bad)
		return
	}
	if !strings.Contains(path, "/") {
		return
	}
	dir := filepath.Dir(filepath.Clean(path))
	for _, allowed := range []string{c.dirs.FirejailSystem, c.dirs.FirejailUser, c.dirs.Imported} {
		if allowed != "" && dir == filepath.Clean(allowed) {
			return
		}
	}
	c.add(n, SeverityError, ProblemExtraInclude, "", "",
		"An include can name only a file in %s, %s or %s, or a bare file name.",
		c.dirs.FirejailSystem, c.dirs.FirejailUser, c.dirs.Imported)
}

// ignore warns when an ignore row removes a line that a field of the base owns: the
// field is the place to change it.
func (c *extraChecker) ignore(n int, args string) {
	seen := map[Field]bool{}
	for _, f := range Fields() {
		for _, l := range c.base.Sources[f] {
			if !isLocked(l) && Matches(args, effectiveText(l)) && !seen[f] {
				seen[f] = true
				c.add(n, SeverityWarning, ProblemExtraCovered, f, "",
					"Ignoring %q removes a line of the base profile that the setting %q of the %s tab owns. Change it there.",
					args, fieldLabel(f), tabLabel(f.Tab()))
			}
		}
	}
	paths := make([]string, 0, len(c.base.FolderSources))
	for path := range c.base.FolderSources {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		for _, l := range c.base.FolderSources[path] {
			if !isLocked(l) && Matches(args, effectiveText(l)) {
				c.add(n, SeverityWarning, ProblemExtraCovered, "", path,
					"Ignoring %q removes a line of the folder %s. Change the folder list instead.", args, path)
				break
			}
		}
	}
}

// needsArgument lists the commands that firejail rejects without an argument. The ones
// that take none are the bare commands of [bareCommands] and "quiet".
func needsArgument() []string {
	return []string{
		"blacklist", "blacklist-nolog", "cpu", "dbus-system", "dbus-user", "dbus-system.own",
		"dbus-system.talk", "dbus-user.own", "dbus-user.talk", "dns", "env", "hosts-file",
		"ignore", "include", "join-or-start", "mkdir", "mkfile", "net", "nice", "noblacklist",
		"noexec", "nowhitelist", "private-bin", "private-etc", "protocol", "read-only",
		"read-write", "rlimit-as", "rlimit-cpu", "rlimit-fsize", "rlimit-nofile", "rlimit-nproc",
		"rlimit-sigpending", "timeout", "tmpfs", "whitelist", "whitelist-ro",
	}
}

// takesNoArgument lists the commands that firejail rejects with an argument: it compares
// the whole line with the command (profile_check_line of profile.c). "seccomp",
// "netfilter" and "restrict-namespaces" are not here: they take an optional argument.
func takesNoArgument() []string {
	return []string{
		"disable-mnt", "landlock.enforce", "memory-deny-write-execute", "no3d", "nodvd", "noinput",
		"nonewprivs", "noprinters", "noroot", "nosound", "notpm", "notv", "nou2f", "novideo",
		"private-cache", "private-dev", "private-tmp", "quiet",
	}
}

// argsProblem says what is wrong with the arguments of a command, or returns "". It
// knows the commands whose arguments the model reads; for the others only a missing
// argument of the list above is reported.
func argsProblem(cmd, args string) string {
	if args != "" && slices.Contains(takesNoArgument(), cmd) {
		return fmt.Sprintf("%s takes no argument.", cmd)
	}
	if args == "" {
		if slices.Contains(needsArgument(), cmd) {
			return fmt.Sprintf("%s needs an argument.", cmd)
		}
		return ""
	}
	switch cmd {
	case "dns":
		if addr, err := netip.ParseAddr(args); err != nil || addr.Zone() != "" {
			return fmt.Sprintf("%q is not an IP address.", args)
		}
	case "nice":
		if _, err := strconv.Atoi(args); err != nil {
			return fmt.Sprintf("%q is not a number.", args)
		}
	case "timeout":
		if _, ok := parseTimeout(args); !ok {
			return fmt.Sprintf("%q is not a time as hh:mm:ss.", args)
		}
	case "rlimit-as", "rlimit-fsize":
		if _, ok := parseSize(args); !ok {
			return fmt.Sprintf("%q is not a size.", args)
		}
	case "rlimit-nofile", "rlimit-nproc", "rlimit-cpu", "rlimit-sigpending":
		if _, ok := parseUnsigned(args); !ok {
			return fmt.Sprintf("%q is not a number.", args)
		}
	case "cpu":
		if _, ok := parseCPUs(args); !ok {
			return fmt.Sprintf("%q is not a list of cores between 0 and %d.", args, maxCPU-1)
		}
	case "env":
		if name, _, ok := strings.Cut(args, "="); !ok || !validEnvName(name) {
			return fmt.Sprintf("%q is not NAME=value.", args)
		}
	}
	return ""
}

// coverage says which lines the form already writes, derived from the table of
// [lineMaker] by writing every setting with a value other than its default.
type coverage struct {
	// exact maps a line of a setting that has a fixed set of lines (a checkbox, a
	// choice) to the field.
	exact map[string]Field
	// free maps a command of a setting that takes any value (a list, a number, a path)
	// to the field.
	free map[string]Field
}

// field returns the field the line is covered by.
func (cv coverage) field(text string) (Field, bool) {
	if f, ok := cv.exact[text]; ok {
		return f, true
	}
	cmd, _ := splitCommand(text)
	f, ok := cv.free[cmd]
	return f, ok
}

// coveredBy derives the coverage from the lines [lineMaker.fieldLines] writes for
// settings that are all away from firejail's defaults, in every choice of a mode field.
func coveredBy() coverage {
	cv := coverage{exact: map[string]Field{}, free: map[string]Field{}}
	m := lineMaker{home: "/home/probe"}
	for _, s := range probeSettings() {
		for _, row := range fieldTable() {
			lines, err := m.fieldLines(row.field, &s)
			if err != nil {
				continue
			}
			fixed := false
			switch row.get(&s).(type) {
			case bool, HomeMode, NetMode, BusMode, X11Mode:
				fixed = true
			}
			// A checkbox whose line holds something else the user chose (the sandbox name
			// of "join-or-start") takes any argument.
			other := s.Clone()
			other.SandboxName = "probe-two"
			if again, err := m.fieldLines(row.field, &other); err != nil || !slices.Equal(lines, again) {
				fixed = false
			}
			for _, text := range lines {
				cmd, _ := splitCommand(text)
				switch {
				case cmd == "include" || cmd == "ignore":
				case fixed:
					cv.exact[text] = row.field
				default:
					cv.free[cmd] = row.field
				}
			}
		}
	}
	return cv
}

// probeSettings returns Settings that write every line of the table: every checkbox
// away from its default, every list and number set, and one Settings for each choice of
// the mode fields.
func probeSettings() []Settings {
	s := firejailDefaults()
	for _, row := range fieldTable() {
		if b, ok := row.get(&s).(bool); ok {
			row.set(&s, !b)
		}
	}
	nice := 1
	s.Network = NetNone
	s.DNS = []string{"1.1.1.1"}
	s.HostsFile = "/etc/hosts.probe"
	s.SessionTalk = []string{"org.probe.Talk"}
	s.SessionOwn = []string{"org.probe.Own"}
	s.Protocols = []string{"unix"}
	s.PrivateBin = []string{"probe"}
	s.PrivateEtc = []string{"probe"}
	s.Env = []EnvVar{{Name: "PROBE", Value: "1"}}
	s.MemoryBytes = 1
	s.CPUs = []int{0}
	s.Nice = &nice
	s.Timeout = 1000000000
	s.MaxFiles = 1
	s.MaxProcs = 1
	s.SandboxName = "probe"
	s.Quiet = true
	s.PrivateDir = "/home/probe/private"

	var out []Settings
	for _, home := range []HomeMode{HomeEmpty, HomePrivate, HomeChosen} {
		for _, bus := range []BusMode{BusFiltered, BusNone} {
			for _, x := range []X11Mode{X11Xpra, X11Xephyr, X11Xvfb} {
				v := s.Clone()
				v.Home, v.SessionBus, v.SystemBus, v.X11 = home, bus, bus, x
				out = append(out, v)
			}
		}
	}
	return out
}

// fieldLabel is the name of a field as the form shows it (SPEC §7).
func fieldLabel(f Field) string {
	labels := map[Field]string{
		FieldHome: "Home folder", FieldPrivateDir: "Home folder", FieldDownloadsReadWrite: "Downloads read-write",
		FieldHideRemovable: "Hide removable drives", FieldNoExecHome: "No programs from home",
		FieldPrivateTmp: "Private /tmp", FieldPrivateDev: "Private /dev", FieldPrivateCache: "Private cache",
		FieldNetwork: "Network", FieldDNS: "DNS servers", FieldHostsFile: "Hosts file", FieldFirewall: "Firewall",
		FieldSound: "Sound and microphone", FieldCamera: "Camera", FieldGPU: "GPU / 3D acceleration",
		FieldPrinters: "Printers", FieldU2F: "U2F security keys", FieldInput: "Input devices",
		FieldDVD: "DVD drives", FieldTV: "TV and radio tuners", FieldTPM: "TPM",
		FieldSessionBus: "Session D-Bus", FieldSessionTalk: "Session D-Bus names to talk to",
		FieldSessionOwn: "Session D-Bus names to own", FieldSystemBus: "System D-Bus", FieldX11: "X11 isolation",
		FieldSeccomp: "Seccomp filter", FieldDropCaps: "Drop all capabilities", FieldNoNewPrivs: "No new privileges",
		FieldNoRoot: "No root user in the sandbox", FieldRestrictNamespaces: "Restrict namespaces",
		FieldMemoryDenyWriteExecute: "Block writable-and-executable memory", FieldLandlock: "Landlock",
		FieldProtocols: "Socket types", FieldPrivateBin: "Only these programs", FieldPrivateEtc: "Only these /etc files",
		FieldEnv: "Environment variables", FieldMemory: "Memory", FieldCPUs: "CPU cores", FieldNice: "Priority",
		FieldTimeout: "Time limit", FieldMaxFiles: "Maximum open files", FieldMaxProcs: "Maximum processes",
		FieldSingleInstance: "Single instance", FieldQuiet: "Hide firejail's messages",
	}
	if l, ok := labels[f]; ok {
		return l
	}
	return string(f)
}

// tabLabel is the name of a tab as the editor shows it.
func tabLabel(t Tab) string {
	if t == "" {
		return ""
	}
	return strings.ToUpper(string(t)[:1]) + string(t)[1:]
}
