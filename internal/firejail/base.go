package firejail

import (
	"math"
	"net/netip"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// UncoveredGroup is the lines of one profile file that no field covers (the read-only
// list of SPEC §7.9).
type UncoveredGroup struct {
	// File is the profile file the lines were read from.
	File string
	// Lines are the lines in the order firejail read them.
	Lines []Line
}

// Base is the base value of every setting that a profile gives (SPEC §8.2), with the
// lines each value came from. Step 2.3 uses the sources to write "ignore <line>" for
// an override.
type Base struct {
	// Settings holds the base values. The launcher fields are Jailcat's defaults: no
	// profile sets them (SingleInstance excepted).
	Settings Settings
	// Sources holds, for each field a profile line set, every line that touched it, in
	// the order read. A field with no line is absent. Lines firejail made itself carry
	// their Origin. The home choice lists every line that decides it: private lines
	// and whitelist lines under the home.
	Sources map[Field][]Line
	// FolderSources holds the lines of each folder row, by the row's Path.
	FolderSources map[string][]Line
	// Uncovered are the lines no field covers, grouped by file in the order the files
	// first appear.
	Uncovered []UncoveredGroup
	// Conditional are the conditional lines whose condition was false (Profile.Skipped).
	Conditional []Skipped
}

// BaseNone is the base for "no profile": [DefaultSettings] and nothing else.
func BaseNone() Base {
	return Base{
		Settings:      DefaultSettings(),
		Sources:       map[Field][]Line{},
		FolderSources: map[string][]Line{},
	}
}

// BaseValues computes the base values of a read profile (SPEC §8.2). The lines that
// remain after "ignore" are mapped to fields; a field with no line keeps firejail's own
// default (sound allowed, network full, nothing dropped, ...), not Jailcat's security
// defaults. A nil profile gives [BaseNone]. home is the user's home folder, to
// recognize the absolute path of the home in a profile; it may be empty.
//
// Folder rows (SPEC §7.1) come only from the lines of files whose name ends in
// ".profile" or ".local": the ".local" files are the user's own and are personal
// to the app, while the shared ".inc" files go to [Base.Uncovered]. A "whitelist"
// line under the home also decides the home choice wherever it is read; when it is
// not a folder row (a ".inc" file, a glob, a system path) it is listed in Uncovered
// as well, so the read-only list shows it.
func BaseValues(p *Profile, home string) Base {
	if p == nil {
		return BaseNone()
	}
	b := &baseBuilder{
		home: home,
		base: Base{
			Settings:      firejailDefaults(),
			Sources:       map[Field][]Line{},
			FolderSources: map[string][]Line{},
		},
		bare:     bareCommands(),
		rows:     map[string]int{},
		uncFiles: map[string]int{},
	}
	for _, l := range p.Lines {
		if !b.line(l) {
			b.addUncovered(l)
		}
	}
	b.finish()
	b.base.Conditional = slices.Clone(p.Skipped)
	return b.base
}

// bareCmd is a command without arguments that sets one boolean field.
type bareCmd struct {
	field Field
	set   func(s *Settings)
}

// bareCommands maps the commands that take no argument to the field they set. The
// device commands turn their device off; the others turn their field on. The match is
// exact, as the strcmp calls of profile.c are.
func bareCommands() map[string]bareCmd {
	off := func(f Field, set func(s *Settings)) bareCmd { return bareCmd{f, set} }
	return map[string]bareCmd{
		"nosound":    off(FieldSound, func(s *Settings) { s.Sound = false }),
		"novideo":    off(FieldCamera, func(s *Settings) { s.Camera = false }),
		"no3d":       off(FieldGPU, func(s *Settings) { s.GPU = false }),
		"noprinters": off(FieldPrinters, func(s *Settings) { s.Printers = false }),
		"nou2f":      off(FieldU2F, func(s *Settings) { s.U2F = false }),
		"noinput":    off(FieldInput, func(s *Settings) { s.Input = false }),
		"nodvd":      off(FieldDVD, func(s *Settings) { s.DVD = false }),
		"notv":       off(FieldTV, func(s *Settings) { s.TV = false }),
		"notpm":      off(FieldTPM, func(s *Settings) { s.TPM = false }),

		"disable-mnt":               off(FieldHideRemovable, func(s *Settings) { s.HideRemovable = true }),
		"private-tmp":               off(FieldPrivateTmp, func(s *Settings) { s.PrivateTmp = true }),
		"private-dev":               off(FieldPrivateDev, func(s *Settings) { s.PrivateDev = true }),
		"private-cache":             off(FieldPrivateCache, func(s *Settings) { s.PrivateCache = true }),
		"netfilter":                 off(FieldFirewall, func(s *Settings) { s.Firewall = true }),
		"seccomp":                   off(FieldSeccomp, func(s *Settings) { s.Seccomp = true }),
		"nonewprivs":                off(FieldNoNewPrivs, func(s *Settings) { s.NoNewPrivs = true }),
		"noroot":                    off(FieldNoRoot, func(s *Settings) { s.NoRoot = true }),
		"restrict-namespaces":       off(FieldRestrictNamespaces, func(s *Settings) { s.RestrictNamespaces = true }),
		"memory-deny-write-execute": off(FieldMemoryDenyWriteExecute, func(s *Settings) { s.MemoryDenyWriteExecute = true }),
		"landlock.enforce":          off(FieldLandlock, func(s *Settings) { s.Landlock = true }),
	}
}

// maxDNS is the number of DNS servers firejail keeps (cfg.dns1 to cfg.dns4 in
// profile.c); a fifth is ignored with a warning.
const maxDNS = 4

// maxCPU is one more than the highest core number firejail accepts (set_cpu in cpu.c).
const maxCPU = 32

// systemFolders are the first components of absolute paths that are not folders a user
// would recognize; their lines stay in the read-only list.
func systemFolders() []string {
	return []string{
		"/dev", "/proc", "/sys", "/run", "/tmp", "/etc", "/usr", "/var", "/boot",
		"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/root", "/lost+found",
	}
}

// baseBuilder holds the state while BaseValues walks the lines.
type baseBuilder struct {
	home string
	base Base
	bare map[string]bareCmd

	rows     map[string]int // folder path -> index in Settings.Folders
	uncFiles map[string]int // file -> index in Uncovered

	hasPrivate    bool   // a bare "private" line
	hasPrivateDir bool   // a "private DIR" line
	privateDir    string // the DIR of the last one
	chosen        bool   // a whitelist line under the home
}

// source records that a line touched a field.
func (b *baseBuilder) source(f Field, l Line) {
	b.base.Sources[f] = append(b.base.Sources[f], l)
}

// addUncovered puts a line into the read-only list, under its file.
func (b *baseBuilder) addUncovered(l Line) {
	i, ok := b.uncFiles[l.File]
	if !ok {
		i = len(b.base.Uncovered)
		b.uncFiles[l.File] = i
		b.base.Uncovered = append(b.base.Uncovered, UncoveredGroup{File: l.File})
	}
	b.base.Uncovered[i].Lines = append(b.base.Uncovered[i].Lines, l)
}

// finish settles what depends on all the lines: the home choice.
func (b *baseBuilder) finish() {
	s := &b.base.Settings
	switch {
	case b.hasPrivateDir:
		s.Home = HomePrivate
		s.PrivateDir = b.privateDir
	case b.hasPrivate:
		s.Home = HomeEmpty
	case b.chosen:
		s.Home = HomeChosen
	}
	if len(s.Protocols) == 0 {
		s.Protocols = nil
	}
}

// line maps one line to its field and reports whether a field covers it.
func (b *baseBuilder) line(l Line) bool {
	s := &b.base.Settings

	// noprinters adds two blacklist lines of its own (profile_check_line); they belong
	// to the Printers field.
	if l.Origin == "noprinters" {
		b.source(FieldPrinters, l)
		return true
	}

	if l.Args == "" {
		if c, ok := b.bare[l.Command]; ok {
			c.set(s)
			b.source(c.field, l)
			return true
		}
		switch l.Command {
		case "nodbus":
			s.SessionBus = BusNone
			s.SystemBus = BusNone
			b.source(FieldSessionBus, l)
			b.source(FieldSystemBus, l)
			return true
		case "private":
			b.hasPrivate = true
			b.source(FieldHome, l)
			return true
		}
		return false
	}

	switch l.Command {
	case "caps.drop":
		if l.Args == "all" {
			s.DropCaps = true
			b.source(FieldDropCaps, l)
			return true
		}
	case "net":
		if l.Args == "none" {
			s.Network = NetNone
			b.source(FieldNetwork, l)
			return true
		}
	case "dns":
		if addr, err := netip.ParseAddr(l.Args); err == nil && addr.Zone() == "" {
			if len(s.DNS) < maxDNS {
				s.DNS = append(s.DNS, l.Args)
			}
			b.source(FieldDNS, l)
			return true
		}
	case "hosts-file":
		s.HostsFile = l.Args
		b.source(FieldHostsFile, l)
		return true
	case "private":
		b.hasPrivateDir = true
		b.privateDir = l.Args
		b.source(FieldHome, l)
		b.source(FieldPrivateDir, l)
		return true
	case "dbus-user":
		return b.bus(l, &s.SessionBus, FieldSessionBus)
	case "dbus-system":
		return b.bus(l, &s.SystemBus, FieldSystemBus)
	case "dbus-user.talk":
		s.SessionTalk = appendUnique(s.SessionTalk, l.Args)
		b.source(FieldSessionTalk, l)
		return true
	case "dbus-user.own":
		s.SessionOwn = appendUnique(s.SessionOwn, l.Args)
		b.source(FieldSessionOwn, l)
		return true
	case "x11":
		switch X11Mode(l.Args) {
		case X11Xpra, X11Xephyr, X11Xvfb:
			s.X11 = X11Mode(l.Args)
			b.source(FieldX11, l)
			return true
		}
	case "protocol":
		// profile_list_augment: the lists of several lines merge.
		s.Protocols = augmentList(s.Protocols, l.Args)
		b.source(FieldProtocols, l)
		return true
	case "private-bin":
		s.PrivateBin = appendItems(s.PrivateBin, l.Args)
		b.source(FieldPrivateBin, l)
		return true
	case "private-etc":
		s.PrivateEtc = appendItems(s.PrivateEtc, l.Args)
		b.source(FieldPrivateEtc, l)
		return true
	case "env":
		if name, value, ok := strings.Cut(l.Args, "="); ok && name != "" {
			s.Env = setEnv(s.Env, EnvVar{Name: name, Value: value})
			b.source(FieldEnv, l)
			return true
		}
	case "rlimit-as":
		if n, ok := parseSize(l.Args); ok {
			s.MemoryBytes = n
			b.source(FieldMemory, l)
			return true
		}
	case "rlimit-nofile":
		if n, ok := parseUnsigned(l.Args); ok {
			s.MaxFiles = n
			b.source(FieldMaxFiles, l)
			return true
		}
	case "rlimit-nproc":
		if n, ok := parseUnsigned(l.Args); ok {
			s.MaxProcs = n
			b.source(FieldMaxProcs, l)
			return true
		}
	case "cpu":
		if cpus, ok := parseCPUs(l.Args); ok {
			s.CPUs = mergeInts(s.CPUs, cpus)
			b.source(FieldCPUs, l)
			return true
		}
	case "nice":
		if n, err := strconv.Atoi(l.Args); err == nil {
			// profile.c: "if (getuid() != 0 && cfg.nice < 0) cfg.nice = 0". Jailcat is
			// never root.
			n = max(n, 0)
			s.Nice = &n
			b.source(FieldNice, l)
			return true
		}
	case "timeout":
		if d, ok := parseTimeout(l.Args); ok {
			s.Timeout = d
			b.source(FieldTimeout, l)
			return true
		}
	case "join-or-start":
		s.SingleInstance = true
		b.source(FieldSingleInstance, l)
		return true
	case "noexec":
		if l.Args == "${HOME}" || l.Args == "~" {
			s.NoExecHome = true
			b.source(FieldNoExecHome, l)
			return true
		}
	case "whitelist", "read-only", "read-write", "blacklist", "blacklist-nolog", "noblacklist":
		return b.pathLine(l)
	}
	return false
}

// bus handles "dbus-user" and "dbus-system" lines (profile.c): "none" blocks, "filter"
// filters unless the bus is already blocked ("Cannot relax dbus-user policy").
func (b *baseBuilder) bus(l Line, mode *BusMode, f Field) bool {
	switch l.Args {
	case "none":
		*mode = BusNone
	case "filter":
		if *mode != BusNone {
			*mode = BusFiltered
		}
	default:
		return false
	}
	b.source(f, l)
	return true
}

// pathLine handles the commands that name a path.
func (b *baseBuilder) pathLine(l Line) bool {
	s := &b.base.Settings
	cmd, path := l.Command, l.Args

	if (cmd == "whitelist" || cmd == "noblacklist") && path == xdg.MacroDownloads {
		s.DownloadsReadWrite = true
		b.source(FieldDownloadsReadWrite, l)
		if cmd == "whitelist" {
			b.chosen = true
			b.source(FieldHome, l)
		}
		return true
	}

	underHome := cmd == "whitelist" && inHome(path, b.home, true)
	if underHome {
		b.chosen = true
		b.source(FieldHome, l)
	}

	if isFolderLine(l, b.home) {
		i, ok := b.rows[path]
		if !ok {
			i = len(s.Folders)
			b.rows[path] = i
			s.Folders = append(s.Folders, Folder{Path: path, FromProfile: true})
		}
		f := &s.Folders[i]
		switch cmd {
		case "read-only":
			f.Access = AccessReadOnly
		case "read-write":
			f.Access = AccessReadWrite
		case "blacklist", "blacklist-nolog":
			f.Access = AccessHidden
		default: // whitelist, noblacklist: a read-only row stays read-only
			if f.Access != AccessReadOnly {
				f.Access = AccessReadWrite
			}
		}
		if strings.HasPrefix(l.Origin, "whitelist-ro ") {
			f.Locked = true
		}
		b.base.FolderSources[path] = append(b.base.FolderSources[path], l)
		return true
	}

	if underHome {
		b.addUncovered(l)
		return true
	}
	return false
}

// isFolderLine reports whether a path line is a row of the folder list: it is read from
// a ".profile" or ".local" file and names a folder the user would recognize.
func isFolderLine(l Line, home string) bool {
	if !strings.HasSuffix(l.File, ".profile") && !strings.HasSuffix(l.File, ".local") {
		return false
	}
	path := l.Args
	if path == "" || strings.ContainsAny(path, "*?[]") || strings.Contains(path, "..") {
		return false
	}
	if path == "${HOME}" || path == "~" || (home != "" && path == filepath.Clean(home)) {
		return false
	}
	if inHome(path, home, true) {
		return true
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for _, sys := range systemFolders() {
		if path == sys || strings.HasPrefix(path, sys+"/") {
			return false
		}
	}
	return true
}

// userMacros are the user-folder macros of firejail (all are folders of the home).
func userMacros() []string {
	return []string{
		xdg.MacroDownloads, xdg.MacroMusic, xdg.MacroVideos,
		xdg.MacroPictures, xdg.MacroDesktop, xdg.MacroDocuments,
	}
}

// inHome reports whether path, as written in a profile, is inside the home: it starts
// with ${HOME}, "~", a user-folder macro or the absolute home. With strict set, the
// home itself does not count (a user-folder macro always does, it names a folder of
// the home).
func inHome(path, home string, strict bool) bool {
	for _, macro := range userMacros() {
		if path == macro || strings.HasPrefix(path, macro+"/") {
			return true
		}
	}
	prefixes := []string{"${HOME}", "~"}
	if home != "" && filepath.IsAbs(home) {
		prefixes = append(prefixes, filepath.Clean(home))
	}
	for _, p := range prefixes {
		if path == p {
			return !strict
		}
		if strings.HasPrefix(path, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

// appendUnique appends item unless the list has it.
func appendUnique(list []string, item string) []string {
	if slices.Contains(list, item) {
		return list
	}
	return append(list, item)
}

// appendItems appends the items of a comma separated list, skipping empty items and
// ones the list has.
func appendItems(list []string, args string) []string {
	for _, item := range strings.Split(args, ",") {
		if item != "" {
			list = appendUnique(list, item)
		}
	}
	return list
}

// augmentList is profile_list_augment and profile_list_compress of profile.c for a
// comma separated list of more items: "item" and "+item" add the item unless the list
// has it, "-item" removes it, "=item" clears the list and adds the item, and empty
// items are dropped.
func augmentList(list []string, args string) []string {
	out := slices.Clone(list)
	for _, item := range strings.Split(args, ",") {
		switch {
		case strings.HasPrefix(item, "-"):
			name := item[1:]
			if i := slices.Index(out, name); i >= 0 {
				out = slices.Delete(out, i, i+1)
			}
		case strings.HasPrefix(item, "="):
			out = nil
			if name := item[1:]; name != "" {
				out = []string{name}
			}
		default:
			if name := strings.TrimPrefix(item, "+"); name != "" {
				out = appendUnique(out, name)
			}
		}
	}
	return out
}

// setEnv sets a variable: a later line for the same name replaces the earlier value, as
// the later setenv does.
func setEnv(vars []EnvVar, v EnvVar) []EnvVar {
	for i := range vars {
		if vars[i].Name == v.Name {
			vars[i].Value = v.Value
			return vars
		}
	}
	return append(vars, v)
}

// mergeInts returns the sorted union of two lists of numbers (cfg.cpus is a bit mask).
func mergeInts(a, b []int) []int {
	out := slices.Concat(a, b)
	slices.Sort(out)
	return slices.Compact(out)
}

// allDigits reports whether s is a non-empty run of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseSize is parse_arg_size of util.c: digits and an optional K, M or G (any case),
// in powers of 1024. Zero, anything else and an overflow are not a size.
func parseSize(args string) (int64, bool) {
	digits := args
	mult := int64(1)
	if n := len(args); n > 0 {
		switch args[n-1] {
		case 'k', 'K':
			mult, digits = 1<<10, args[:n-1]
		case 'm', 'M':
			mult, digits = 1<<20, args[:n-1]
		case 'g', 'G':
			mult, digits = 1<<30, args[:n-1]
		}
	}
	n, ok := parseUnsigned(digits)
	if !ok || n == 0 || n > math.MaxInt64/mult {
		return 0, false
	}
	return n * mult, true
}

// parseUnsigned reads a run of digits as a number that fits an int64.
func parseUnsigned(args string) (int64, bool) {
	if !allDigits(args) {
		return 0, false
	}
	n, err := strconv.ParseInt(args, 10, 64)
	return n, err == nil
}

// parseCPUs is read_cpu_list of cpu.c: numbers 0 to 31 separated by commas; empty items
// are skipped. At least one number is required.
func parseCPUs(args string) ([]int, bool) {
	var out []int
	for _, item := range strings.Split(args, ",") {
		if item == "" {
			continue
		}
		n, ok := parseUnsigned(item)
		if !ok || n >= maxCPU {
			return nil, false
		}
		out = append(out, int(n))
	}
	if len(out) == 0 {
		return nil, false
	}
	slices.Sort(out)
	return slices.Compact(out), true
}

// parseTimeout is extract_timeout of util.c: hh:mm:ss, each part read with "%02u" (one
// or two digits), and not zero.
func parseTimeout(args string) (time.Duration, bool) {
	parts := strings.Split(args, ":")
	if len(parts) != 3 {
		return 0, false
	}
	var nums [3]int64
	for i, p := range parts {
		if len(p) > 2 {
			return 0, false
		}
		n, ok := parseUnsigned(p)
		if !ok {
			return 0, false
		}
		nums[i] = n
	}
	d := time.Duration(nums[0]*3600+nums[1]*60+nums[2]) * time.Second
	return d, d > 0
}
