package firejail

import (
	"slices"
	"time"
)

// HomeMode is the home folder choice of SPEC §7.1. The zero value, [HomeUnspecified],
// is not a valid mode.
type HomeMode string

// The home choices of SPEC §7.1.
const (
	// HomeUnspecified is the zero value: no choice was made. It is never valid.
	HomeUnspecified HomeMode = ""
	// HomeFull is the real home, minus what the base profile hides.
	HomeFull HomeMode = "full"
	// HomeChosen is "Only the folders I choose": whitelist lines under the home.
	HomeChosen HomeMode = "chosen"
	// HomeEmpty is an empty home that is thrown away when the sandbox closes (private).
	HomeEmpty HomeMode = "empty"
	// HomePrivate is a private home kept between runs (private DIR).
	HomePrivate HomeMode = "private"
)

// Access is the access level of a folder row (SPEC §7.1).
type Access string

// The access levels of a folder row.
const (
	// AccessReadWrite lets the app read and write the folder.
	AccessReadWrite Access = "read-write"
	// AccessReadOnly lets the app read the folder only.
	AccessReadOnly Access = "read-only"
	// AccessHidden hides the folder from the app.
	AccessHidden Access = "hidden"
)

// Folder is one row of the folder list of SPEC §7.1.
type Folder struct {
	// Path is the folder as written in profiles: it may start with ${HOME}, "~" or a
	// user-folder macro such as ${DOWNLOADS}, or be absolute.
	Path string
	// Access is what the app may do with the folder.
	Access Access
	// FromProfile is true for a row that the base profile makes.
	FromProfile bool
	// Locked is true for a row made from a whitelist-ro line (SPEC §9.1): it cannot be
	// removed or loosened.
	Locked bool
}

// NetMode is the network choice of SPEC §7.2. The zero value is not valid.
type NetMode string

// The network choices.
const (
	// NetFull is the host's network (firejail's default).
	NetFull NetMode = "full"
	// NetNone is no network at all (net none).
	NetNone NetMode = "none"
)

// BusMode is the D-Bus choice of SPEC §7.4. The zero value is not valid.
type BusMode string

// The D-Bus choices.
const (
	// BusFull is the whole bus (firejail's default).
	BusFull BusMode = "full"
	// BusFiltered is a filtered bus (dbus-user filter, dbus-system filter). The
	// session bus offers it; the system bus only shows it, read-only, when a base
	// profile sets it.
	BusFiltered BusMode = "filtered"
	// BusNone is no bus (dbus-user none, dbus-system none).
	BusNone BusMode = "none"
)

// X11Mode is the X11 isolation choice of SPEC §7.4. The zero value is not valid.
type X11Mode string

// The X11 isolation choices. "x11 none" and "x11 xorg" are not choices of the form;
// a base profile that sets them has them in the read-only list.
const (
	// X11Off is no isolation (firejail's default).
	X11Off X11Mode = "off"
	// X11Xpra isolates with an Xpra server (x11 xpra).
	X11Xpra X11Mode = "xpra"
	// X11Xephyr isolates with an Xephyr server (x11 xephyr).
	X11Xephyr X11Mode = "xephyr"
	// X11Xvfb isolates with an Xvfb server (x11 xvfb).
	X11Xvfb X11Mode = "xvfb"
)

// IconMode is the icon choice of SPEC §7.8. The zero value is not valid.
type IconMode string

// The icon choices.
const (
	// IconBadge is the app's own icon with the firejail badge.
	IconBadge IconMode = "badge"
	// IconRandom is a random icon made from [Settings.IconSeed].
	IconRandom IconMode = "random"
)

// EnvVar is one environment variable of the Programs tab (env NAME=value).
type EnvVar struct {
	// Name is the variable's name.
	Name string `json:"name"`
	// Value is its value.
	Value string `json:"value"`
}

// The well-known D-Bus names of the checkboxes of SPEC §7.4. A checkbox is on when its
// name is in [Settings.SessionTalk] (or, for the media keys, [Settings.SessionOwn]).
const (
	// DBusNotifications is the Notifications checkbox (dbus-user.talk).
	DBusNotifications = "org.freedesktop.Notifications"
	// DBusPortals is the "File chooser and other portals" checkbox (dbus-user.talk).
	DBusPortals = "org.freedesktop.portal.*"
	// DBusKeyring is the Keyring (Secret Service) checkbox (dbus-user.talk).
	DBusKeyring = "org.freedesktop.secrets"
	// DBusTray is the Tray icon checkbox (dbus-user.talk).
	DBusTray = "org.kde.StatusNotifierWatcher"
)

// MPRISName is the name of the Media keys checkbox of SPEC §7.4 for an app: it is in
// [Settings.SessionOwn] (dbus-user.own).
func MPRISName(binary string) string {
	return "org.mpris.MediaPlayer2." + binary + ".*"
}

// Settings is everything a build chooses, grouped by tab (SPEC §7.1 to §7.8). Folders
// are rows; every other setting is one field of the table in [Fields], so the
// difference between two Settings ([Diff]), applying overrides ([Overrides.Apply]) and
// their JSON are built from that table.
type Settings struct {
	// Files tab (SPEC §7.1).

	// Home is the home folder choice.
	Home HomeMode
	// PrivateDir is the folder of [HomePrivate] (private DIR).
	PrivateDir string
	// Folders is the folder list.
	Folders []Folder
	// DownloadsReadWrite makes ${DOWNLOADS} read-write.
	DownloadsReadWrite bool
	// HideRemovable hides removable drives (disable-mnt).
	HideRemovable bool
	// NoExecHome forbids running programs from the home (noexec ${HOME}).
	NoExecHome bool
	// PrivateTmp gives the app its own /tmp (private-tmp).
	PrivateTmp bool
	// PrivateDev gives the app its own /dev (private-dev).
	PrivateDev bool
	// PrivateCache gives the app its own cache folder (private-cache).
	PrivateCache bool

	// Network tab (SPEC §7.2).

	// Network is the network choice.
	Network NetMode
	// DNS lists up to four DNS servers (dns); empty is the system's.
	DNS []string
	// HostsFile is a file to use as /etc/hosts (hosts-file); empty is the system's.
	HostsFile string
	// Firewall turns on firejail's default client filter (netfilter).
	Firewall bool

	// Devices tab (SPEC §7.3): true allows the device, false blocks it.

	// Sound allows sound and the microphone (blocked by nosound).
	Sound bool
	// Camera allows cameras (blocked by novideo).
	Camera bool
	// GPU allows 3D acceleration (blocked by no3d).
	GPU bool
	// Printers allows printers (blocked by noprinters).
	Printers bool
	// U2F allows security keys (blocked by nou2f).
	U2F bool
	// Input allows input devices (blocked by noinput).
	Input bool
	// DVD allows DVD drives (blocked by nodvd).
	DVD bool
	// TV allows TV and radio tuners (blocked by notv).
	TV bool
	// TPM allows the TPM (blocked by notpm).
	TPM bool

	// Desktop tab (SPEC §7.4).

	// SessionBus is the session D-Bus choice.
	SessionBus BusMode
	// SessionTalk lists the names the app may talk to on a filtered session bus
	// (dbus-user.talk).
	SessionTalk []string
	// SessionOwn lists the names the app may own on a filtered session bus
	// (dbus-user.own).
	SessionOwn []string
	// SystemBus is the system D-Bus choice: Full or None in the form; a base profile
	// may set Filtered, which the form shows read-only.
	SystemBus BusMode
	// X11 is the X11 isolation choice.
	X11 X11Mode

	// Security tab (SPEC §7.5).

	// Seccomp turns on the seccomp filter (seccomp).
	Seccomp bool
	// DropCaps drops all capabilities (caps.drop all).
	DropCaps bool
	// NoNewPrivs forbids gaining privileges (nonewprivs).
	NoNewPrivs bool
	// NoRoot denies the root user in the sandbox (noroot).
	NoRoot bool
	// RestrictNamespaces blocks new namespaces (restrict-namespaces).
	RestrictNamespaces bool
	// MemoryDenyWriteExecute blocks writable-and-executable memory
	// (memory-deny-write-execute).
	MemoryDenyWriteExecute bool
	// Landlock turns on Landlock (landlock.enforce).
	Landlock bool
	// Protocols are the allowed socket types (protocol); empty means not restricted.
	Protocols []string

	// Programs tab (SPEC §7.6).

	// PrivateBin lists the only programs the app sees (private-bin); empty is off.
	PrivateBin []string
	// PrivateEtc lists the only /etc files the app sees (private-etc); empty is off.
	PrivateEtc []string
	// Env lists environment variables (env NAME=value).
	Env []EnvVar
	// ExtraArgs are words added to the app's command line before the user's own.
	ExtraArgs []string

	// Limits tab (SPEC §7.7).

	// MemoryBytes is the memory limit in bytes (rlimit-as); 0 is off.
	MemoryBytes int64
	// CPUs lists the cores the app may use (cpu); empty is off.
	CPUs []int
	// Nice is the priority (nice); nil is off.
	Nice *int
	// Timeout kills the sandbox after this long (timeout hh:mm:ss); 0 is off.
	Timeout time.Duration
	// MaxFiles is the maximum number of open files (rlimit-nofile); 0 is off.
	MaxFiles int64
	// MaxProcs is the maximum number of processes (rlimit-nproc); 0 is off.
	MaxProcs int64

	// Launcher tab (SPEC §7.8).

	// SandboxName is the name of the build, made of [a-z0-9-].
	SandboxName string
	// MenuName is the name shown in the application menu.
	MenuName string
	// Comment is the desktop entry's comment.
	Comment string
	// Icon is the icon choice.
	Icon IconMode
	// IconSeed is the seed of a random icon.
	IconSeed int64
	// InstallMenu installs a desktop entry in the application menu.
	InstallMenu bool
	// InstallBin installs the launcher in the bin folder.
	InstallBin bool
	// OpenFileTypes lets the entry take over the app's file types (MimeType).
	OpenFileTypes bool
	// SingleInstance joins a running sandbox of the build (join-or-start).
	SingleInstance bool
	// Terminal runs the app in a terminal (Terminal=true).
	Terminal bool
	// Quiet hides firejail's messages (quiet).
	Quiet bool
}

// DefaultSettings returns the defaults of SPEC §7 for base None: Jailcat's security
// defaults on (§7.5), every device allowed, full home, network and buses, and the
// launcher defaults (quiet, install in the menu and the bin folder). Names and the
// comment are left empty: the editor fills them from the app.
func DefaultSettings() Settings {
	s := firejailDefaults()
	s.Seccomp = true
	s.DropCaps = true
	s.NoNewPrivs = true
	s.NoRoot = true
	s.RestrictNamespaces = true
	s.Protocols = []string{"unix", "inet", "inet6"}
	return s
}

// firejailDefaults is what firejail does with no line at all: nothing restricted. A
// build on a base profile starts from it, not from [DefaultSettings], whose security
// switches are Jailcat's own (SPEC §7.5). The launcher fields do not come from a
// profile; they have Jailcat's defaults in both.
func firejailDefaults() Settings {
	return Settings{
		Home:        HomeFull,
		Network:     NetFull,
		Sound:       true,
		Camera:      true,
		GPU:         true,
		Printers:    true,
		U2F:         true,
		Input:       true,
		DVD:         true,
		TV:          true,
		TPM:         true,
		SessionBus:  BusFull,
		SystemBus:   BusFull,
		X11:         X11Off,
		Icon:        IconBadge,
		InstallMenu: true,
		InstallBin:  true,
		Quiet:       true,
	}
}

// Clone returns a copy of s that shares no slice or pointer with it.
func (s Settings) Clone() Settings {
	out := s
	out.Folders = slices.Clone(s.Folders)
	for _, row := range fieldTable() {
		row.set(&out, row.get(&s))
	}
	return out
}

// DesktopChecks says which checkboxes of the Desktop tab (SPEC §7.4) are on.
type DesktopChecks struct {
	// Notifications is on when [DBusNotifications] is in SessionTalk.
	Notifications bool
	// Portals is on when [DBusPortals] is in SessionTalk.
	Portals bool
	// Keyring is on when [DBusKeyring] is in SessionTalk.
	Keyring bool
	// Tray is on when [DBusTray] is in SessionTalk.
	Tray bool
	// MediaKeys is on when [MPRISName] of the binary is in SessionOwn.
	MediaKeys bool
}

// DesktopChecks returns the checkboxes of the Desktop tab that s turns on, for the
// app whose binary name is binary (the Media keys name contains it).
func (s Settings) DesktopChecks(binary string) DesktopChecks {
	return DesktopChecks{
		Notifications: slices.Contains(s.SessionTalk, DBusNotifications),
		Portals:       slices.Contains(s.SessionTalk, DBusPortals),
		Keyring:       slices.Contains(s.SessionTalk, DBusKeyring),
		Tray:          slices.Contains(s.SessionTalk, DBusTray),
		MediaKeys:     slices.Contains(s.SessionOwn, MPRISName(binary)),
	}
}

// WithName returns a copy of list with name in it (appended when missing) when on is
// true, and without name when on is false. It is how a checkbox of the Desktop tab
// changes SessionTalk or SessionOwn; the other names stay.
func WithName(list []string, name string, on bool) []string {
	out := slices.Clone(list)
	switch has := slices.Contains(out, name); {
	case on && !has:
		out = append(out, name)
	case !on && has:
		out = slices.DeleteFunc(out, func(n string) bool { return n == name })
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
