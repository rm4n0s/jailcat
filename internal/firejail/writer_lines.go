package firejail

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// lineMaker writes the profile lines of one setting. It is the table of SPEC §9.1 that
// maps each field to the firejail commands of "man firejail-profile"; the checks of the
// Extra rules tab read the same table (see [coveredBy]). The lines are those that
// establish the value of the field on top of firejail's own default, so a field at its
// default has none.
type lineMaker struct {
	// home is the cleaned home folder, or "" when unknown.
	home string
}

// canon writes a path the way profiles do: a path under the home starts with ${HOME};
// macros and every other path are kept as written.
func (m lineMaker) canon(p string) string {
	if m.home == "" || m.home == "/" {
		return p
	}
	if p == m.home {
		return "${HOME}"
	}
	if rest, ok := strings.CutPrefix(p, m.home+"/"); ok {
		return "${HOME}/" + rest
	}
	return p
}

// path is a path ready for a profile line: under ${HOME} when it is in the home, and
// passed through [profileValue].
func (m lineMaker) path(p string) (string, error) {
	return profileValue(m.canon(p))
}

// rowCommands returns the commands that give a folder row its access, in the order they
// are written (SPEC §9.1): Read-write is "noblacklist" (and "whitelist" when the home is
// "Only the folders I choose"), Read-only adds "read-only", Hidden is "blacklist".
func (m lineMaker) rowCommands(a Access, chosen bool) ([]string, error) {
	var out []string
	switch a {
	case AccessReadWrite, AccessReadOnly:
		out = append(out, "noblacklist")
		if chosen {
			out = append(out, "whitelist")
		}
		if a == AccessReadOnly {
			out = append(out, "read-only")
		}
	case AccessHidden:
		out = append(out, "blacklist")
	default:
		return nil, errors.New(tagModeInvalid, "a folder row has an access level that is not one of the three",
			"access", string(a))
	}
	return out, nil
}

// rowLines returns the lines of one folder row.
func (m lineMaker) rowLines(path string, a Access, chosen bool) ([]string, error) {
	cmds, err := m.rowCommands(a, chosen)
	if err != nil {
		return nil, err
	}
	p, err := m.path(path)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c + " " + p
	}
	return out, nil
}

// lineIf returns the line when on is true and no line otherwise.
func lineIf(on bool, line string) []string {
	if on {
		return []string{line}
	}
	return nil
}

// fieldLines returns the lines that give the field the value it has in s. The folder
// rows are not a field and are written by rowLines. Fields that no profile line holds
// (the names, icon, install and terminal choices of the Launcher tab, and the extra
// arguments) have none. A name that is not a known field is an error, so a new field
// cannot be left out of the profile by accident.
func (m lineMaker) fieldLines(f Field, s *Settings) ([]string, error) {
	switch f {
	case FieldHome:
		return lineIf(s.Home == HomeEmpty, "private"), nil
	case FieldPrivateDir:
		if s.Home != HomePrivate {
			return nil, nil
		}
		p, err := m.path(s.PrivateDir)
		if err != nil {
			return nil, err
		}
		return []string{"private " + p}, nil
	case FieldDownloadsReadWrite:
		if !s.DownloadsReadWrite {
			return nil, nil
		}
		return m.rowLines(xdg.MacroDownloads, AccessReadWrite, s.Home == HomeChosen)
	case FieldHideRemovable:
		return lineIf(s.HideRemovable, "disable-mnt"), nil
	case FieldNoExecHome:
		return lineIf(s.NoExecHome, "noexec ${HOME}"), nil
	case FieldPrivateTmp:
		return lineIf(s.PrivateTmp, "private-tmp"), nil
	case FieldPrivateDev:
		return lineIf(s.PrivateDev, "private-dev"), nil
	case FieldPrivateCache:
		return lineIf(s.PrivateCache, "private-cache"), nil

	case FieldNetwork:
		switch s.Network {
		case NetFull:
			return nil, nil
		case NetNone:
			return []string{"net none"}, nil
		}
		return nil, errors.New(tagModeInvalid, "the network choice is not one of the two", "field", string(f), "value", string(s.Network))
	case FieldDNS:
		var out []string
		for _, d := range s.DNS {
			v, err := profileValue(d)
			if err != nil {
				return nil, err
			}
			out = append(out, "dns "+v)
		}
		return out, nil
	case FieldHostsFile:
		if s.HostsFile == "" {
			return nil, nil
		}
		p, err := m.path(s.HostsFile)
		if err != nil {
			return nil, err
		}
		return []string{"hosts-file " + p}, nil
	case FieldFirewall:
		return lineIf(s.Firewall, "netfilter"), nil

	case FieldSound:
		return lineIf(!s.Sound, "nosound"), nil
	case FieldCamera:
		return lineIf(!s.Camera, "novideo"), nil
	case FieldGPU:
		return lineIf(!s.GPU, "no3d"), nil
	case FieldPrinters:
		return lineIf(!s.Printers, "noprinters"), nil
	case FieldU2F:
		return lineIf(!s.U2F, "nou2f"), nil
	case FieldInput:
		return lineIf(!s.Input, "noinput"), nil
	case FieldDVD:
		return lineIf(!s.DVD, "nodvd"), nil
	case FieldTV:
		return lineIf(!s.TV, "notv"), nil
	case FieldTPM:
		return lineIf(!s.TPM, "notpm"), nil

	case FieldSessionBus:
		switch s.SessionBus {
		case BusFull:
			return nil, nil
		case BusFiltered:
			return []string{"dbus-user filter"}, nil
		case BusNone:
			return []string{"dbus-user none"}, nil
		}
		return nil, errors.New(tagModeInvalid, "the session D-Bus choice is not one of the three", "field", string(f), "value", string(s.SessionBus))
	case FieldSessionTalk:
		var out []string
		for _, n := range s.SessionTalk {
			v, err := profileValue(n)
			if err != nil {
				return nil, err
			}
			out = append(out, "dbus-user.talk "+v)
		}
		return out, nil
	case FieldSessionOwn:
		var out []string
		for _, n := range s.SessionOwn {
			v, err := profileValue(n)
			if err != nil {
				return nil, err
			}
			out = append(out, "dbus-user.own "+v)
		}
		return out, nil
	case FieldSystemBus:
		switch s.SystemBus {
		case BusFull:
			return nil, nil
		case BusFiltered:
			return []string{"dbus-system filter"}, nil
		case BusNone:
			return []string{"dbus-system none"}, nil
		}
		return nil, errors.New(tagModeInvalid, "the system D-Bus choice is not one of the three", "field", string(f), "value", string(s.SystemBus))
	case FieldX11:
		switch s.X11 {
		case X11Off:
			return nil, nil
		case X11Xpra, X11Xephyr, X11Xvfb:
			return []string{"x11 " + string(s.X11)}, nil
		}
		return nil, errors.New(tagModeInvalid, "the X11 isolation choice is not one of the four", "field", string(f), "value", string(s.X11))

	case FieldSeccomp:
		return lineIf(s.Seccomp, "seccomp"), nil
	case FieldDropCaps:
		return lineIf(s.DropCaps, "caps.drop all"), nil
	case FieldNoNewPrivs:
		return lineIf(s.NoNewPrivs, "nonewprivs"), nil
	case FieldNoRoot:
		return lineIf(s.NoRoot, "noroot"), nil
	case FieldRestrictNamespaces:
		return lineIf(s.RestrictNamespaces, "restrict-namespaces"), nil
	case FieldMemoryDenyWriteExecute:
		return lineIf(s.MemoryDenyWriteExecute, "memory-deny-write-execute"), nil
	case FieldLandlock:
		if !s.Landlock {
			return nil, nil
		}
		return []string{"landlock.enforce", "include ${CFG}/landlock-common.inc"}, nil
	case FieldProtocols:
		if len(s.Protocols) == 0 {
			return nil, nil
		}
		list, err := profileList(s.Protocols)
		if err != nil {
			return nil, err
		}
		return []string{"protocol " + list}, nil

	case FieldPrivateBin:
		if len(s.PrivateBin) == 0 {
			return nil, nil
		}
		list, err := profileList(s.PrivateBin)
		if err != nil {
			return nil, err
		}
		return []string{"private-bin " + list}, nil
	case FieldPrivateEtc:
		if len(s.PrivateEtc) == 0 {
			return nil, nil
		}
		list, err := profileList(s.PrivateEtc)
		if err != nil {
			return nil, err
		}
		return []string{"private-etc " + list}, nil
	case FieldEnv:
		var out []string
		for _, e := range s.Env {
			if !validEnvName(e.Name) {
				return nil, errors.New(tagProfileValueInvalid, "an environment variable name is not a name",
					"value", e.Name, "reason", "a name that is not a variable name")
			}
			if e.Value != "" {
				if _, err := profileValue(e.Value); err != nil {
					return nil, err
				}
			}
			out = append(out, "env "+e.Name+"="+e.Value)
		}
		return out, nil

	case FieldMemory:
		if s.MemoryBytes < 0 {
			return nil, errors.New(tagValueRange, "the memory limit is negative", "field", string(f), "value", s.MemoryBytes)
		}
		if s.MemoryBytes == 0 {
			return nil, nil
		}
		return []string{"rlimit-as " + strconv.FormatInt(s.MemoryBytes, 10)}, nil
	case FieldCPUs:
		if len(s.CPUs) == 0 {
			return nil, nil
		}
		items := make([]string, len(s.CPUs))
		for i, c := range s.CPUs {
			if c < 0 || c >= maxCPU {
				return nil, errors.New(tagValueRange, "a CPU core is outside firejail's range of 0 to 31", "field", string(f), "value", c)
			}
			items[i] = strconv.Itoa(c)
		}
		return []string{"cpu " + strings.Join(items, ",")}, nil
	case FieldNice:
		if s.Nice == nil {
			return nil, nil
		}
		if *s.Nice < -20 || *s.Nice > 19 {
			return nil, errors.New(tagValueRange, "the priority is outside -20 to 19", "field", string(f), "value", *s.Nice)
		}
		return []string{"nice " + strconv.Itoa(*s.Nice)}, nil
	case FieldTimeout:
		if s.Timeout == 0 {
			return nil, nil
		}
		if s.Timeout < 0 || s.Timeout >= 100*time.Hour || s.Timeout%time.Second != 0 {
			return nil, errors.New(tagValueRange, "the time limit is not a whole number of seconds under 100 hours",
				"field", string(f), "value", s.Timeout.String())
		}
		secs := int64(s.Timeout / time.Second)
		return []string{fmt.Sprintf("timeout %02d:%02d:%02d", secs/3600, secs%3600/60, secs%60)}, nil
	case FieldMaxFiles:
		if s.MaxFiles < 0 {
			return nil, errors.New(tagValueRange, "the open files limit is negative", "field", string(f), "value", s.MaxFiles)
		}
		if s.MaxFiles == 0 {
			return nil, nil
		}
		return []string{"rlimit-nofile " + strconv.FormatInt(s.MaxFiles, 10)}, nil
	case FieldMaxProcs:
		if s.MaxProcs < 0 {
			return nil, errors.New(tagValueRange, "the processes limit is negative", "field", string(f), "value", s.MaxProcs)
		}
		if s.MaxProcs == 0 {
			return nil, nil
		}
		return []string{"rlimit-nproc " + strconv.FormatInt(s.MaxProcs, 10)}, nil

	case FieldSingleInstance:
		if !s.SingleInstance {
			return nil, nil
		}
		name, err := profileValue(s.SandboxName)
		if err != nil {
			return nil, err
		}
		return []string{"join-or-start " + name}, nil
	case FieldQuiet:
		return lineIf(s.Quiet, "quiet"), nil

	case FieldVSCodeOwnData, FieldVSCodeShareSettings, FieldVSCodeShellFiles:
		// Jailcat's own settings: the generator writes their lines with the folder rows
		// ([generator.vscode]), because they depend on the home choice and on the folders
		// of the build, not on a profile line they undo.
		return nil, nil
	case FieldSandboxName, FieldMenuName, FieldComment, FieldIcon, FieldIconSeed,
		FieldInstallMenu, FieldInstallBin, FieldOpenFileTypes, FieldTerminal, FieldExtraArgs:
		// The launcher script and the desktop entry hold these, not the profile.
		return nil, nil
	}
	return nil, errors.New(tagFieldUnknown, "the profile writer has no lines for this field", "field", string(f))
}

const (
	tagModeInvalid  = "ProfileModeInvalid"
	tagValueRange   = "ProfileValueOutOfRange"
	tagFieldUnknown = "ProfileFieldUnknown"
)
