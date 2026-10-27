package firejail

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// The tags of [Problem]. A problem is not an error: [Validate] returns a list for the
// editor to show, and the tag lets the editor and the tests tell the rules apart.
const (
	// ProblemMode is a mode field (home, network, buses, X11, icon) with a value that
	// is not one of its choices, the zero value included.
	ProblemMode = "ModeInvalid"
	// ProblemSandboxName is a sandbox name that is empty or has a character outside
	// [a-z0-9-].
	ProblemSandboxName = "SandboxNameInvalid"
	// ProblemInstallNothing is a build that installs neither a menu entry nor a bin
	// launcher.
	ProblemInstallNothing = "InstallNothing"
	// ProblemPath is a path that is not absolute and does not start with ${HOME}, "~" or
	// a user-folder macro, or that firejail would reject (a ".." or a character
	// reject_meta_chars refuses). It is used for folder rows (Path is set), the private
	// home folder and the hosts file.
	ProblemPath = "PathInvalid"
	// ProblemFolderAccess is a folder row whose access is not one of the three levels.
	ProblemFolderAccess = "FolderAccessInvalid"
	// ProblemFolderInHome is a folder row inside the real home while the home is Empty
	// or Own private (SPEC §7.1).
	ProblemFolderInHome = "FolderInHome"
	// ProblemFolderDuplicate is a second row for a path.
	ProblemFolderDuplicate = "FolderDuplicate"
	// ProblemFolderLocked is a locked row (made from whitelist-ro) that is not
	// read-only, or an override that removes or loosens one.
	ProblemFolderLocked = "FolderLocked"
	// ProblemDNSCount is more than four DNS servers.
	ProblemDNSCount = "DNSTooMany"
	// ProblemDNSAddress is a DNS server that is not an IP address.
	ProblemDNSAddress = "DNSAddressInvalid"
	// ProblemProtocol is a socket type that firejail does not know.
	ProblemProtocol = "ProtocolInvalid"
	// ProblemListItem is a list item that is empty or has a comma (a list is written
	// separated by commas).
	ProblemListItem = "ListItemInvalid"
	// ProblemPrivateEtc is a private-etc item with a "/" (firejail wants names relative
	// to /etc).
	ProblemPrivateEtc = "PrivateEtcSlash"
	// ProblemBusName is a D-Bus name that has a blank.
	ProblemBusName = "BusNameInvalid"
	// ProblemEnvName is an environment variable name outside ^[A-Za-z_][A-Za-z0-9_]*$.
	ProblemEnvName = "EnvNameInvalid"
	// ProblemNice is a priority outside -20 to 19.
	ProblemNice = "NiceRange"
	// ProblemTimeout is a time limit that is negative, not a whole number of seconds,
	// or 100 hours or more (firejail reads hours with two digits).
	ProblemTimeout = "TimeoutRange"
	// ProblemCPU is a core number outside 0 to 31 (firejail's range) or listed twice.
	ProblemCPU = "CPUInvalid"
	// ProblemNumber is a limit that is negative.
	ProblemNumber = "NumberNegative"
	// ProblemChosenNoFolder is the home set to "Only the folders I choose" with no folder
	// row and no Downloads: firejail would not restrict the home at all.
	ProblemChosenNoFolder = "ChosenHomeNoFolder"
	// ProblemDownloadsHome is Downloads read-write with an empty or private home, which
	// keep the real home (where the Downloads folder is) out of the sandbox.
	ProblemDownloadsHome = "DownloadsNeedRealHome"
	// ProblemValue is a value that cannot be written in a profile line (SPEC §9.1): a
	// newline, a NUL, a "#", a blank at the start or the end, two blanks in a row or a
	// tab.
	ProblemValue = "ValueForbidden"
)

// Severity says whether a [Problem] blocks Build and Try.
type Severity string

// The severities of a [Problem].
const (
	// SeverityError blocks Build and Try.
	SeverityError Severity = "error"
	// SeverityWarning is shown to the user and blocks nothing.
	SeverityWarning Severity = "warning"
)

// Problem is one thing wrong with a [Settings] or with the Extra rules, written for the
// user.
type Problem struct {
	// Field is the setting that has the problem; it is empty for a folder row, which
	// has Path (the Files tab).
	Field Field
	// Path is the path of the folder row, for a problem of one row.
	Path string
	// Message tells the user what is wrong and what to do.
	Message string
	// Tag is one of the Problem constants.
	Tag string
	// Severity is [SeverityError] for every problem of [Validate]; the checks of the
	// Extra rules ([CheckExtraRules]) also report [SeverityWarning].
	Severity Severity
	// Row is the 1-based row of the Extra rules that has the problem, 0 for a problem of
	// a setting or a folder row.
	Row int
}

// validator collects the problems of one [Validate] call.
type validator struct {
	home string
	out  []Problem
}

func (v *validator) add(f Field, path, tag, format string, args ...any) {
	v.out = append(v.out, Problem{Field: f, Path: path, Tag: tag, Severity: SeverityError, Message: fmt.Sprintf(format, args...)})
}

// value checks that a string can be a value of a profile line (SPEC §9.1).
func (v *validator) value(f Field, path, what, s string) bool {
	if reason := forbiddenValue(s); reason != "" {
		v.add(f, path, ProblemValue, "%s cannot be used: it has %s.", what, reason)
		return false
	}
	return true
}

// Validate checks a Settings and returns what is wrong with it, in the order of the
// tabs; the list is empty when it is valid. home is the user's home folder, to
// recognize folder rows inside it.
func Validate(s Settings, home string) []Problem {
	v := &validator{home: home}
	v.files(s)
	v.network(s)
	v.desktop(s)
	v.security(s)
	v.programs(s)
	v.limits(s)
	v.launcher(s)
	return v.out
}

func (v *validator) files(s Settings) {
	if !slices.Contains([]HomeMode{HomeFull, HomeChosen, HomeEmpty, HomePrivate}, s.Home) {
		v.add(FieldHome, "", ProblemMode, "Choose what the app sees as its home folder.")
	}
	if s.PrivateDir != "" {
		v.path(FieldHome, "", "The private home folder", s.PrivateDir)
	}

	keepsHomeOut := s.Home == HomeEmpty || s.Home == HomePrivate
	if s.Home == HomeChosen && len(s.Folders) == 0 && !s.DownloadsReadWrite {
		v.add(FieldHome, "", ProblemChosenNoFolder,
			"Add at least one folder to the list. With none, the app would see the whole home, not only the folders you choose.")
	}
	if keepsHomeOut && s.DownloadsReadWrite {
		v.add(FieldDownloadsReadWrite, "", ProblemDownloadsHome,
			"The Downloads folder is in your home folder, which an empty or private home keeps out of the sandbox. Turn Downloads off, or choose another home.")
	}
	seen := map[string]bool{}
	for _, f := range s.Folders {
		if seen[f.Path] {
			v.add("", f.Path, ProblemFolderDuplicate, "%s is in the list twice.", f.Path)
			continue
		}
		seen[f.Path] = true

		if !slices.Contains([]Access{AccessReadWrite, AccessReadOnly, AccessHidden}, f.Access) {
			v.add("", f.Path, ProblemFolderAccess, "Choose read-write, read-only or hidden for %s.", f.Path)
		}
		pathOK := v.path("", f.Path, "The folder "+f.Path, f.Path)
		// The real home stays out of an empty or private home (SPEC §7.1). The rows the
		// base profile makes are the profile's own business: the user cannot be asked
		// to remove them to choose a home.
		if pathOK && keepsHomeOut && !f.FromProfile && inHome(f.Path, v.home, false) {
			v.add("", f.Path, ProblemFolderInHome,
				"%s is inside your home folder. With an empty or private home the real home stays out of the sandbox: choose a folder outside it.", f.Path)
		}
		if f.Locked && f.Access != AccessReadOnly {
			v.add("", f.Path, ProblemFolderLocked,
				"%s is read-only in the profile and cannot be loosened.", f.Path)
		}
	}
}

// path checks the form of a path (SPEC §9.1): absolute, or starting with ${HOME}, "~" or
// a user-folder macro; and that firejail accepts it. It reports whether the path is fine.
func (v *validator) path(f Field, row, what, p string) bool {
	if !v.value(f, row, what, p) {
		return false
	}
	rest, ok := cutPathRoot(p)
	switch {
	case !ok:
		v.add(f, row, ProblemPath,
			"%s must be a full path, or start with ${HOME}, ~ or a user folder such as ${DOWNLOADS}.", what)
	case strings.Contains(p, ".."):
		v.add(f, row, ProblemPath, `%s cannot have ".." in it.`, what)
	case invalidFilename(rest) != "":
		v.add(f, row, ProblemPath, "%s has a character that firejail does not accept in a path.", what)
	default:
		return true
	}
	return false
}

// cutPathRoot checks that p starts the way a profile path may and returns what follows
// the macro, if there is one. It is the part invalidFilename should look at.
func cutPathRoot(p string) (rest string, ok bool) {
	if strings.HasPrefix(p, "/") {
		return p, true
	}
	macros := append(userMacros(), "${HOME}", "~")
	for _, m := range macros {
		if r, found := strings.CutPrefix(p, m); found && (r == "" || r[0] == '/') {
			return r, true
		}
	}
	return "", false
}

func (v *validator) network(s Settings) {
	if s.Network != NetFull && s.Network != NetNone {
		v.add(FieldNetwork, "", ProblemMode, "Choose full network or no network.")
	}
	if len(s.DNS) > maxDNS {
		v.add(FieldDNS, "", ProblemDNSCount, "Firejail uses at most %d DNS servers.", maxDNS)
	}
	for _, d := range s.DNS {
		if addr, err := netip.ParseAddr(d); err != nil || addr.Zone() != "" {
			v.add(FieldDNS, "", ProblemDNSAddress, "%q is not an IP address.", d)
		}
	}
	if s.HostsFile != "" {
		v.path(FieldHostsFile, "", "The hosts file", s.HostsFile)
	}
}

func (v *validator) desktop(s Settings) {
	for _, bus := range []struct {
		field Field
		mode  BusMode
	}{{FieldSessionBus, s.SessionBus}, {FieldSystemBus, s.SystemBus}} {
		if !slices.Contains([]BusMode{BusFull, BusFiltered, BusNone}, bus.mode) {
			v.add(bus.field, "", ProblemMode, "Choose full, filtered or no D-Bus access.")
		}
	}
	for _, names := range []struct {
		field Field
		list  []string
	}{{FieldSessionTalk, s.SessionTalk}, {FieldSessionOwn, s.SessionOwn}} {
		for _, name := range names.list {
			if name == "" || strings.ContainsAny(name, " \t") {
				v.add(names.field, "", ProblemBusName, "%q is not a D-Bus name.", name)
				continue
			}
			v.value(names.field, "", "The D-Bus name "+name, name)
		}
	}
	if !slices.Contains([]X11Mode{X11Off, X11Xpra, X11Xephyr, X11Xvfb}, s.X11) {
		v.add(FieldX11, "", ProblemMode, "Choose how to isolate X11.")
	}
}

// protocolNames are the socket types of "man firejail-profile" (protocol).
func protocolNames() []string {
	return []string{"unix", "inet", "inet6", "netlink", "packet", "bluetooth"}
}

func (v *validator) security(s Settings) {
	for _, p := range s.Protocols {
		switch {
		case p == "" || strings.Contains(p, ","):
			v.add(FieldProtocols, "", ProblemListItem, "A socket type cannot be empty or have a comma in it: %q.", p)
		case !slices.Contains(protocolNames(), p):
			v.add(FieldProtocols, "", ProblemProtocol,
				"%q is not a socket type. Use unix, inet, inet6, netlink, packet or bluetooth.", p)
		}
	}
}

func (v *validator) programs(s Settings) {
	for _, l := range []struct {
		field Field
		what  string
		list  []string
	}{{FieldPrivateBin, "program name", s.PrivateBin}, {FieldPrivateEtc, "file name", s.PrivateEtc}} {
		for _, item := range l.list {
			if item == "" || strings.Contains(item, ",") {
				v.add(l.field, "", ProblemListItem, "A %s cannot be empty or have a comma in it: %q.", l.what, item)
				continue
			}
			if l.field == FieldPrivateEtc && strings.Contains(item, "/") {
				v.add(l.field, "", ProblemPrivateEtc,
					"%q has a / in it. Give names relative to /etc, such as hosts or ssl.", item)
				continue
			}
			v.value(l.field, "", "The "+l.what+" "+item, item)
		}
	}
	for _, e := range s.Env {
		if !validEnvName(e.Name) {
			v.add(FieldEnv, "", ProblemEnvName,
				"%q is not a variable name. Use letters, digits and _, and do not start with a digit.", e.Name)
			continue
		}
		v.value(FieldEnv, "", "The value of "+e.Name, e.Value)
	}
	for _, a := range s.ExtraArgs {
		// The launcher quotes every word (SPEC §9.2); a NUL cannot be in an argument.
		if strings.ContainsRune(a, 0) {
			v.add(FieldExtraArgs, "", ProblemValue, "An extra argument cannot be used: it has a NUL character.")
		}
	}
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func (v *validator) limits(s Settings) {
	for _, n := range []struct {
		field Field
		what  string
		value int64
	}{{FieldMemory, "The memory limit", s.MemoryBytes}, {FieldMaxFiles, "The open files limit", s.MaxFiles},
		{FieldMaxProcs, "The processes limit", s.MaxProcs}} {
		if n.value < 0 {
			v.add(n.field, "", ProblemNumber, "%s cannot be negative.", n.what)
		}
	}
	seen := map[int]bool{}
	for _, c := range s.CPUs {
		switch {
		case c < 0 || c >= maxCPU:
			v.add(FieldCPUs, "", ProblemCPU, "Core %d is not between 0 and %d.", c, maxCPU-1)
		case seen[c]:
			v.add(FieldCPUs, "", ProblemCPU, "Core %d is listed twice.", c)
		}
		seen[c] = true
	}
	if s.Nice != nil && (*s.Nice < -20 || *s.Nice > 19) {
		v.add(FieldNice, "", ProblemNice, "The priority must be between -20 and 19.")
	}
	if s.Timeout < 0 || s.Timeout >= 100*time.Hour || s.Timeout%time.Second != 0 {
		v.add(FieldTimeout, "", ProblemTimeout, "The time limit must be a whole number of seconds, under 100 hours.")
	}
}

func (v *validator) launcher(s Settings) {
	if s.SandboxName == "" || strings.Trim(s.SandboxName, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		v.add(FieldSandboxName, "", ProblemSandboxName,
			"The sandbox name must have at least one character, and only lowercase letters, digits and -.")
	}
	if !slices.Contains([]IconMode{IconBadge, IconRandom}, s.Icon) {
		v.add(FieldIcon, "", ProblemMode, "Choose the app's icon with the badge, or a random icon.")
	}
	if !s.InstallMenu && !s.InstallBin {
		v.add(FieldInstallMenu, "", ProblemInstallNothing,
			"Install in the menu, in the bin folder, or both: one of them must be on.")
	}
}

// Validate reports the overrides that cannot take effect on base: removing a Locked
// row, or giving it another access than read-only (SPEC §9.1). [Overrides.Apply] leaves
// such a row as it is; the editor shows these problems so the user knows why.
func (o Overrides) Validate(base Settings) []Problem {
	var out []Problem
	for _, fo := range o.Folders {
		i := slices.IndexFunc(base.Folders, func(f Folder) bool { return f.Path == fo.Path })
		if i < 0 || !base.Folders[i].Locked {
			continue
		}
		switch {
		case fo.Removed:
			out = append(out, Problem{Path: fo.Path, Tag: ProblemFolderLocked, Severity: SeverityError,
				Message: fmt.Sprintf("%s is read-only in the profile and cannot be removed.", fo.Path)})
		case fo.Access != AccessReadOnly:
			out = append(out, Problem{Path: fo.Path, Tag: ProblemFolderLocked, Severity: SeverityError,
				Message: fmt.Sprintf("%s is read-only in the profile and cannot be loosened.", fo.Path)})
		}
	}
	return out
}

// forbiddenValue says why a value cannot go into a profile line, or returns "". Firejail
// normalizes every line (SPEC §8.1), so a value with one of these would not be read
// back as written; they are errors, not escaped (SPEC §9.1).
func forbiddenValue(s string) string {
	switch {
	case strings.ContainsAny(s, "\n\r"):
		return "a new line"
	case strings.ContainsRune(s, 0):
		return "a NUL character"
	case strings.Contains(s, "#"):
		return "a # character"
	case strings.Contains(s, "\t"):
		return "a tab"
	case strings.HasPrefix(s, " ") || strings.HasSuffix(s, " "):
		return "a blank at the start or the end"
	case strings.Contains(s, "  "):
		return "two blanks in a row"
	}
	return ""
}
