package firejail

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

// fixtureProfile reads a profile of the fixture copy of /etc/firejail the way Jailcat
// does, with an empty user folder, and returns it with the home it used.
func fixtureProfile(t *testing.T, name string) (*Profile, string) {
	t.Helper()
	sys := fixtureDir(t)
	home := t.TempDir()
	cfg, err := ReadConfig(context.Background(), filepath.Join(sys, "firejail.config"))
	if err != nil {
		t.Fatal(err)
	}
	prof, err := Read(context.Background(), filepath.Join(sys, name), ReadOpts{
		Dirs:   xdg.Dirs{Home: home, FirejailSystem: sys, FirejailUser: t.TempDir()},
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("Read(%s): %v", name, err)
	}
	return prof, home
}

// wantSettings is firejail's defaults with the changes of mod.
func wantSettings(mod func(s *Settings)) Settings {
	s := firejailDefaults()
	if mod != nil {
		mod(&s)
	}
	return s
}

func requireSettings(t *testing.T, got, want Settings) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	for _, row := range fieldTable() {
		if !row.equal(&got, &want) {
			t.Errorf("%s = %#v, want %#v", row.field, row.get(&got), row.get(&want))
		}
	}
	if !slices.Equal(got.Folders, want.Folders) {
		t.Errorf("Folders = %+v, want %+v", got.Folders, want.Folders)
	}
}

func sourceFields(b Base) []Field {
	var out []Field
	for f := range b.Sources {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

func uncoveredTexts(b Base) []string {
	var out []string
	for _, g := range b.Uncovered {
		out = append(out, texts(g.Lines)...)
	}
	return out
}

func sorted(fields ...Field) []Field {
	out := slices.Clone(fields)
	slices.Sort(out)
	return out
}

func TestBaseValuesVSCode(t *testing.T) {
	prof, home := fixtureProfile(t, "code.profile")
	b := BaseValues(prof, home)

	want := wantSettings(func(s *Settings) {
		s.Sound = false       // nosound (code.profile)
		s.Camera = false      // novideo (electron-common.profile)
		s.U2F = false         // nou2f (electron-common.profile)
		s.DVD = false         // nodvd (blink-common.profile)
		s.Input = false       // noinput (blink-common.profile)
		s.TV = false          // notv (blink-common.profile)
		s.PrivateDev = true   // electron-common.profile
		s.PrivateTmp = true   // electron-common.profile
		s.PrivateCache = true // blink-common.profile
		s.Firewall = true     // netfilter (blink-common.profile)
		s.Folders = []Folder{ // the noblacklist lines of the .profile files
			{Path: "${HOME}/.config/Code", Access: AccessReadWrite, FromProfile: true},
			{Path: "${HOME}/.config/Code - OSS", Access: AccessReadWrite, FromProfile: true},
			{Path: "${HOME}/.vscode", Access: AccessReadWrite, FromProfile: true},
			{Path: "${HOME}/.vscode-oss", Access: AccessReadWrite, FromProfile: true},
			{Path: "${HOME}/.config/Electron", Access: AccessReadWrite, FromProfile: true},
		}
		// Not set, though blink-common.profile has them: "ignore dbus-user none" and
		// "ignore dbus-system none" skip the dbus lines, "ignore disable-mnt" and
		// "ignore apparmor" skip those, "ignore whitelist ${DOWNLOADS}" skips the
		// downloads and "ignore whitelist ${HOME}/.config/Electron" the whitelists, so
		// the home stays full. The security switches are firejail's defaults (off).
	})
	requireSettings(t, b.Settings, want)

	// Every field that has a line, and the file the line came from.
	wantSources := map[Field]string{
		FieldSound: "code.profile", FieldCamera: "electron-common.profile",
		FieldU2F: "electron-common.profile", FieldDVD: "blink-common.profile",
		FieldInput: "blink-common.profile", FieldTV: "blink-common.profile",
		FieldPrivateDev: "electron-common.profile", FieldPrivateTmp: "electron-common.profile",
		FieldPrivateCache: "blink-common.profile", FieldFirewall: "blink-common.profile",
	}
	var wantFields []Field
	for f, file := range wantSources {
		wantFields = append(wantFields, f)
		got := b.Sources[f]
		if len(got) != 1 || filepath.Base(got[0].File) != file {
			t.Errorf("Sources[%s] = %+v, want one line of %s", f, got, file)
		}
	}
	if got := sourceFields(b); !slices.Equal(got, sorted(wantFields...)) {
		t.Errorf("source fields = %v, want %v", got, sorted(wantFields...))
	}
	if got := b.Sources[FieldSound][0]; got.Text != "nosound" {
		t.Errorf("Sources[sound][0].Text = %q", got.Text)
	}

	// The folder rows keep their lines.
	for _, f := range want.Folders {
		src := b.FolderSources[f.Path]
		if len(src) != 1 || src[0].Command != "noblacklist" || src[0].Args != f.Path {
			t.Errorf("FolderSources[%q] = %+v", f.Path, src)
		}
	}
	if len(b.FolderSources) != len(want.Folders) {
		t.Errorf("len(FolderSources) = %d, want %d", len(b.FolderSources), len(want.Folders))
	}

	// What no field covers, by file in the order of the files: noexec /tmp, the glob of
	// electron-common.profile, caps.keep and nogroups.
	type group struct {
		file string
		n    int
	}
	wantGroups := []group{
		{"allow-common-devel.inc", 30}, {"code.profile", 1}, {"electron-common.profile", 1},
		{"disable-common.inc", 620}, {"disable-programs.inc", 1314}, {"blink-common.profile", 2},
	}
	var gotGroups []group
	for _, g := range b.Uncovered {
		gotGroups = append(gotGroups, group{filepath.Base(g.File), len(g.Lines)})
		for _, l := range g.Lines {
			if l.File != g.File {
				t.Errorf("line %q of %s is in the group of %s", l.Text, l.File, g.File)
			}
		}
	}
	if !slices.Equal(gotGroups, wantGroups) {
		t.Errorf("uncovered groups = %v, want %v", gotGroups, wantGroups)
	}
	for _, g := range b.Uncovered {
		switch filepath.Base(g.File) {
		case "code.profile":
			requireTexts(t, "code.profile uncovered", g.Lines, "noexec /tmp")
		case "electron-common.profile":
			requireTexts(t, "electron-common.profile uncovered", g.Lines, "noblacklist ${HOME}/.config/electron*-flag*.conf")
		case "blink-common.profile":
			requireTexts(t, "blink-common.profile uncovered", g.Lines, "caps.keep sys_admin,sys_chroot", "nogroups")
		}
	}
	if len(b.Conditional) != 0 {
		t.Errorf("Conditional = %+v, want none", b.Conditional)
	}
}

func TestBaseValuesDefaultProfile(t *testing.T) {
	prof, home := fixtureProfile(t, "default.profile")
	b := BaseValues(prof, home)

	want := wantSettings(func(s *Settings) {
		s.Seccomp = true
		s.DropCaps = true
		s.NoNewPrivs = true
		s.NoRoot = true
		s.RestrictNamespaces = true
		s.Protocols = []string{"unix", "inet", "inet6"}
		s.Camera = false // novideo
		s.Input = false  // noinput
		s.TV = false     // notv
		s.PrivateDev = true
		s.PrivateTmp = true
		s.Firewall = true
	})
	requireSettings(t, b.Settings, want)

	wantFields := sorted(FieldSeccomp, FieldDropCaps, FieldNoNewPrivs, FieldNoRoot,
		FieldRestrictNamespaces, FieldProtocols, FieldCamera, FieldInput, FieldTV,
		FieldPrivateDev, FieldPrivateTmp, FieldFirewall)
	if got := sourceFields(b); !slices.Equal(got, wantFields) {
		t.Errorf("source fields = %v, want %v", got, wantFields)
	}
	if len(b.Settings.Folders) != 0 {
		t.Errorf("Folders = %+v, want none: the lines are in .inc files", b.Settings.Folders)
	}
	// disable-common.inc, disable-programs.inc and landlock-common.inc (no
	// landlock.enforce line there, only landlock.fs rules).
	var files []string
	for _, g := range b.Uncovered {
		files = append(files, filepath.Base(g.File))
	}
	if want := []string{"disable-common.inc", "disable-programs.inc", "landlock-common.inc"}; !slices.Equal(files, want) {
		t.Errorf("uncovered files = %v, want %v", files, want)
	}
}

func TestBaseNone(t *testing.T) {
	b := BaseNone()
	requireSettings(t, b.Settings, DefaultSettings())
	if len(b.Sources) != 0 || len(b.FolderSources) != 0 || len(b.Uncovered) != 0 || len(b.Conditional) != 0 {
		t.Errorf("BaseNone has more than the defaults: %+v", b)
	}
	nilBase := BaseValues(nil, "/home/me")
	requireSettings(t, nilBase.Settings, DefaultSettings())
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	want := Settings{
		Home: HomeFull, Network: NetFull,
		Sound: true, Camera: true, GPU: true, Printers: true, U2F: true, Input: true,
		DVD: true, TV: true, TPM: true,
		SessionBus: BusFull, SystemBus: BusFull, X11: X11Off,
		Seccomp: true, DropCaps: true, NoNewPrivs: true, NoRoot: true, RestrictNamespaces: true,
		Protocols: []string{"unix", "inet", "inet6"},
		Icon:      IconBadge, InstallMenu: true, InstallBin: true, Quiet: true,
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("DefaultSettings() = %+v, want %+v", s, want)
	}
	// The firejail defaults have none of Jailcat's security switches.
	f := firejailDefaults()
	if f.Seccomp || f.DropCaps || f.NoNewPrivs || f.NoRoot || f.RestrictNamespaces || f.Protocols != nil {
		t.Errorf("firejailDefaults() has security switches on: %+v", f)
	}
	if DefaultSettings().Home == HomeUnspecified || (Settings{}).Home != HomeUnspecified {
		t.Error("the zero HomeMode must be HomeUnspecified and DefaultSettings must not use it")
	}
	// Each call is a fresh value.
	a, b := DefaultSettings(), DefaultSettings()
	a.Protocols[0] = "changed"
	if b.Protocols[0] != "unix" {
		t.Error("DefaultSettings shares its Protocols slice between calls")
	}
}

func intp(n int) *int { return &n }

func TestBaseValuesCommands(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		// files are written into the system folder before the profile is read.
		files    map[string]string
		want     func(s *Settings)
		fields   []Field  // the fields that have a source
		unc      []string // texts of the uncovered lines, in order
		folderBy map[string][]string
	}{
		{
			name: "no lines is firejail's defaults", lines: []string{"quiet"},
		},
		{
			name: "nosound", lines: []string{"nosound"},
			want: func(s *Settings) { s.Sound = false }, fields: []Field{FieldSound},
		},
		{
			name: "all device commands",
			lines: []string{"nosound", "novideo", "no3d", "noprinters", "nou2f", "noinput",
				"nodvd", "notv", "notpm"},
			want: func(s *Settings) {
				s.Sound, s.Camera, s.GPU, s.Printers, s.U2F = false, false, false, false, false
				s.Input, s.DVD, s.TV, s.TPM = false, false, false, false
			},
			fields: []Field{FieldSound, FieldCamera, FieldGPU, FieldPrinters, FieldU2F,
				FieldInput, FieldDVD, FieldTV, FieldTPM},
		},
		{
			name: "noprinters and the lines firejail adds", lines: []string{"noprinters"},
			want: func(s *Settings) { s.Printers = false }, fields: []Field{FieldPrinters},
		},
		{
			name:  "toggles of the files tab",
			lines: []string{"disable-mnt", "noexec ${HOME}", "private-tmp", "private-dev", "private-cache"},
			want: func(s *Settings) {
				s.HideRemovable, s.NoExecHome, s.PrivateTmp, s.PrivateDev, s.PrivateCache = true, true, true, true, true
			},
			fields: []Field{FieldHideRemovable, FieldNoExecHome, FieldPrivateTmp, FieldPrivateDev, FieldPrivateCache},
		},
		{
			name: "noexec tilde is noexec of the home", lines: []string{"noexec ~"},
			want: func(s *Settings) { s.NoExecHome = true }, fields: []Field{FieldNoExecHome},
		},
		{
			name: "noexec of another folder is uncovered", lines: []string{"noexec /tmp", "noexec ${HOME}/bin"},
			unc: []string{"noexec /tmp", "noexec ${HOME}/bin"},
		},
		{
			name: "net none", lines: []string{"net none"},
			want: func(s *Settings) { s.Network = NetNone }, fields: []Field{FieldNetwork},
		},
		{
			name: "net on a device is uncovered", lines: []string{"net eth0"}, unc: []string{"net eth0"},
		},
		{
			name: "dns lines add up to four", lines: []string{"dns 1.1.1.1", "dns 9.9.9.9", "dns 2606:4700:4700::1111", "dns 8.8.8.8", "dns 8.8.4.4"},
			want:   func(s *Settings) { s.DNS = []string{"1.1.1.1", "9.9.9.9", "2606:4700:4700::1111", "8.8.8.8"} },
			fields: []Field{FieldDNS},
		},
		{
			name: "dns that is not an address is uncovered", lines: []string{"dns example.com"}, unc: []string{"dns example.com"},
		},
		{
			name: "hosts-file", lines: []string{"hosts-file ${HOME}/hosts"},
			want: func(s *Settings) { s.HostsFile = "${HOME}/hosts" }, fields: []Field{FieldHostsFile},
		},
		{
			name: "netfilter without a file", lines: []string{"netfilter"},
			want: func(s *Settings) { s.Firewall = true }, fields: []Field{FieldFirewall},
		},
		{
			name: "netfilter with a file is uncovered", lines: []string{"netfilter /etc/f.net"}, unc: []string{"netfilter /etc/f.net"},
		},
		{
			name: "dbus-user none", lines: []string{"dbus-user none"},
			want: func(s *Settings) { s.SessionBus = BusNone }, fields: []Field{FieldSessionBus},
		},
		{
			name: "dbus-user filter with names", lines: []string{
				"dbus-user filter", "dbus-user.talk org.freedesktop.Notifications",
				"dbus-user.talk org.freedesktop.portal.*", "dbus-user.talk org.freedesktop.Notifications",
				"dbus-user.own org.mpris.MediaPlayer2.code.*", "dbus-user.see org.x.Y"},
			want: func(s *Settings) {
				s.SessionBus = BusFiltered
				s.SessionTalk = []string{"org.freedesktop.Notifications", "org.freedesktop.portal.*"}
				s.SessionOwn = []string{"org.mpris.MediaPlayer2.code.*"}
			},
			fields: []Field{FieldSessionBus, FieldSessionTalk, FieldSessionOwn}, unc: []string{"dbus-user.see org.x.Y"},
		},
		{
			name: "dbus-user filter cannot relax none", lines: []string{"dbus-user none", "dbus-user filter"},
			want: func(s *Settings) { s.SessionBus = BusNone }, fields: []Field{FieldSessionBus},
		},
		{
			name: "dbus-system filter", lines: []string{"dbus-system filter", "dbus-system.talk org.x.Y"},
			want: func(s *Settings) { s.SystemBus = BusFiltered }, fields: []Field{FieldSystemBus},
			unc: []string{"dbus-system.talk org.x.Y"},
		},
		{
			name: "nodbus", lines: []string{"nodbus"},
			want:   func(s *Settings) { s.SessionBus, s.SystemBus = BusNone, BusNone },
			fields: []Field{FieldSessionBus, FieldSystemBus},
		},
		{
			name: "dbus-user with an unknown policy is uncovered", lines: []string{"dbus-user maybe"}, unc: []string{"dbus-user maybe"},
		},
		{
			name: "x11 servers", lines: []string{"x11 xpra"},
			want: func(s *Settings) { s.X11 = X11Xpra }, fields: []Field{FieldX11},
		},
		{
			name: "x11 xvfb after xephyr wins", lines: []string{"x11 xephyr", "x11 xvfb"},
			want: func(s *Settings) { s.X11 = X11Xvfb }, fields: []Field{FieldX11},
		},
		{
			name: "x11 none and xorg and plain x11 are uncovered", lines: []string{"x11 none", "x11 xorg", "x11"},
			unc: []string{"x11 none", "x11 xorg", "x11"},
		},
		{
			name: "security switches",
			lines: []string{"seccomp", "caps.drop all", "nonewprivs", "noroot", "restrict-namespaces",
				"memory-deny-write-execute", "landlock.enforce"},
			want: func(s *Settings) {
				s.Seccomp, s.DropCaps, s.NoNewPrivs, s.NoRoot = true, true, true, true
				s.RestrictNamespaces, s.MemoryDenyWriteExecute, s.Landlock = true, true, true
			},
			fields: []Field{FieldSeccomp, FieldDropCaps, FieldNoNewPrivs, FieldNoRoot,
				FieldRestrictNamespaces, FieldMemoryDenyWriteExecute, FieldLandlock},
		},
		{
			name:  "other caps and seccomp forms are uncovered",
			lines: []string{"caps.drop sys_admin", "caps.keep net_raw", "caps", "seccomp.drop mount", "restrict-namespaces user", "landlock.fs.read /"},
			unc: []string{"caps.drop sys_admin", "caps.keep net_raw", "caps", "seccomp.drop mount",
				"restrict-namespaces user", "landlock.fs.read /"},
		},
		{
			name: "protocol lists merge", lines: []string{"protocol unix,inet", "protocol inet,inet6,,"},
			want: func(s *Settings) { s.Protocols = []string{"unix", "inet", "inet6"} }, fields: []Field{FieldProtocols},
		},
		{
			name: "protocol minus plus and reset", lines: []string{"protocol unix,inet,inet6", "protocol -inet,+netlink", "protocol =packet,unix"},
			want: func(s *Settings) { s.Protocols = []string{"packet", "unix"} }, fields: []Field{FieldProtocols},
		},
		{
			name: "protocol emptied is not restricted", lines: []string{"protocol unix", "protocol -unix"},
			fields: []Field{FieldProtocols},
		},
		{
			name: "private-bin lists merge", lines: []string{"private-bin sh,bash", "private-bin bash,ls,"},
			want: func(s *Settings) { s.PrivateBin = []string{"sh", "bash", "ls"} }, fields: []Field{FieldPrivateBin},
		},
		{
			name: "private-etc lists merge", lines: []string{"private-etc hosts", "private-etc ssl,hosts"},
			want: func(s *Settings) { s.PrivateEtc = []string{"hosts", "ssl"} }, fields: []Field{FieldPrivateEtc},
		},
		{
			name: "private-etc without a list is uncovered", lines: []string{"private-etc"}, unc: []string{"private-etc"},
		},
		{
			name: "env", lines: []string{"env EDITOR=vim", "env A=1", "env EDITOR=nano", "env EMPTY=", "env BAD"},
			want: func(s *Settings) {
				s.Env = []EnvVar{{"EDITOR", "nano"}, {"A", "1"}, {"EMPTY", ""}}
			},
			fields: []Field{FieldEnv}, unc: []string{"env BAD"},
		},
		{
			name: "rlimit-as units", lines: []string{"rlimit-as 512M"},
			want: func(s *Settings) { s.MemoryBytes = 512 << 20 }, fields: []Field{FieldMemory},
		},
		{
			name: "rlimit-as G lower case", lines: []string{"rlimit-as 2g"},
			want: func(s *Settings) { s.MemoryBytes = 2 << 30 }, fields: []Field{FieldMemory},
		},
		{
			name: "rlimit-as K and plain bytes", lines: []string{"rlimit-as 4k", "rlimit-as 123456789012"},
			want: func(s *Settings) { s.MemoryBytes = 123456789012 }, fields: []Field{FieldMemory},
		},
		{
			name:  "rlimit-as that firejail rejects is uncovered",
			lines: []string{"rlimit-as 0", "rlimit-as 5T", "rlimit-as -1", "rlimit-as M", "rlimit-as 1.5G", "rlimit-as 9999999999G"},
			unc:   []string{"rlimit-as 0", "rlimit-as 5T", "rlimit-as -1", "rlimit-as M", "rlimit-as 1.5G", "rlimit-as 9999999999G"},
		},
		{
			name: "rlimit-nofile and nproc", lines: []string{"rlimit-nofile 1024", "rlimit-nproc 200"},
			want: func(s *Settings) { s.MaxFiles, s.MaxProcs = 1024, 200 }, fields: []Field{FieldMaxFiles, FieldMaxProcs},
		},
		{
			name:  "other rlimits and bad numbers are uncovered",
			lines: []string{"rlimit-cpu 10", "rlimit-fsize 1024", "rlimit-sigpending 5", "rlimit-nofile abc"},
			unc:   []string{"rlimit-cpu 10", "rlimit-fsize 1024", "rlimit-sigpending 5", "rlimit-nofile abc"},
		},
		{
			name: "cpu lines add up", lines: []string{"cpu 0,1", "cpu 3,1"},
			want: func(s *Settings) { s.CPUs = []int{0, 1, 3} }, fields: []Field{FieldCPUs},
		},
		{
			name: "cpu out of range is uncovered", lines: []string{"cpu 32", "cpu a", "cpu ,"}, unc: []string{"cpu 32", "cpu a", "cpu ,"},
		},
		{
			name: "nice", lines: []string{"nice 10"},
			want: func(s *Settings) { s.Nice = intp(10) }, fields: []Field{FieldNice},
		},
		{
			name: "nice below zero is zero for a user", lines: []string{"nice -5"},
			want: func(s *Settings) { s.Nice = intp(0) }, fields: []Field{FieldNice},
		},
		{
			name: "nice that is not a number is uncovered", lines: []string{"nice high"}, unc: []string{"nice high"},
		},
		{
			name: "timeout", lines: []string{"timeout 01:02:03"},
			want: func(s *Settings) { s.Timeout = time.Hour + 2*time.Minute + 3*time.Second }, fields: []Field{FieldTimeout},
		},
		{
			name:  "timeout that firejail rejects is uncovered",
			lines: []string{"timeout 00:00:00", "timeout 100:00:00", "timeout 10:00", "timeout 1:2:x"},
			unc:   []string{"timeout 00:00:00", "timeout 100:00:00", "timeout 10:00", "timeout 1:2:x"},
		},
		{
			name: "join-or-start", lines: []string{"join-or-start code"},
			want: func(s *Settings) { s.SingleInstance = true }, fields: []Field{FieldSingleInstance},
		},
		{
			name: "private is an empty home", lines: []string{"private"},
			want: func(s *Settings) { s.Home = HomeEmpty }, fields: []Field{FieldHome},
		},
		{
			name: "private DIR is an own private home", lines: []string{"private ${HOME}/.local/share/jailcat/homes/x"},
			want: func(s *Settings) {
				s.Home = HomePrivate
				s.PrivateDir = "${HOME}/.local/share/jailcat/homes/x"
			},
			fields: []Field{FieldHome, FieldPrivateDir},
		},
		{
			name: "private DIR wins over private", lines: []string{"private ${HOME}/p", "private"},
			want: func(s *Settings) {
				s.Home = HomePrivate
				s.PrivateDir = "${HOME}/p"
			},
			fields: []Field{FieldHome, FieldPrivateDir},
		},
		{
			name: "private-home is uncovered", lines: []string{"private-home .config"}, unc: []string{"private-home .config"},
		},
		{
			name: "whitelist under the home chooses the folders", lines: []string{"whitelist ${HOME}/Projects", "whitelist ~/Notes", "whitelist ${DOCUMENTS}/work"},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.Folders = []Folder{
					{Path: "${HOME}/Projects", Access: AccessReadWrite, FromProfile: true},
					{Path: "~/Notes", Access: AccessReadWrite, FromProfile: true},
					{Path: "${DOCUMENTS}/work", Access: AccessReadWrite, FromProfile: true},
				}
			},
			fields:   []Field{FieldHome},
			folderBy: map[string][]string{"${HOME}/Projects": {"whitelist ${HOME}/Projects"}},
		},
		{
			name: "whitelist of the absolute home path", lines: []string{"whitelist @HOME@/Projects"},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.Folders = []Folder{{Path: "@HOME@/Projects", Access: AccessReadWrite, FromProfile: true}}
			},
			fields: []Field{FieldHome},
		},
		{
			name: "whitelist outside the home does not choose the folders", lines: []string{"whitelist /mnt/data"},
			want: func(s *Settings) {
				s.Folders = []Folder{{Path: "/mnt/data", Access: AccessReadWrite, FromProfile: true}}
			},
		},
		{
			name: "whitelist of the downloads", lines: []string{"whitelist ${DOWNLOADS}"},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.DownloadsReadWrite = true
			},
			fields: []Field{FieldHome, FieldDownloadsReadWrite},
		},
		{
			name: "noblacklist of the downloads", lines: []string{"noblacklist ${DOWNLOADS}"},
			want: func(s *Settings) { s.DownloadsReadWrite = true }, fields: []Field{FieldDownloadsReadWrite},
		},
		{
			name:  "private and a whitelist: private wins",
			lines: []string{"whitelist ${HOME}/a", "private"},
			want: func(s *Settings) {
				s.Home = HomeEmpty
				s.Folders = []Folder{{Path: "${HOME}/a", Access: AccessReadWrite, FromProfile: true}}
			},
			fields: []Field{FieldHome},
		},
		{
			name: "folder access levels",
			lines: []string{
				"whitelist ${HOME}/a", "read-only ${HOME}/a",
				"noblacklist ${HOME}/b",
				"read-only /opt/ro",
				"read-write /opt/rw",
				"blacklist ${HOME}/secret", "blacklist-nolog /mnt/hidden",
				"blacklist ${HOME}/c", "noblacklist ${HOME}/c",
				"read-only ${HOME}/d", "noblacklist ${HOME}/d",
			},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.Folders = []Folder{
					{Path: "${HOME}/a", Access: AccessReadOnly, FromProfile: true},
					{Path: "${HOME}/b", Access: AccessReadWrite, FromProfile: true},
					{Path: "/opt/ro", Access: AccessReadOnly, FromProfile: true},
					{Path: "/opt/rw", Access: AccessReadWrite, FromProfile: true},
					{Path: "${HOME}/secret", Access: AccessHidden, FromProfile: true},
					{Path: "/mnt/hidden", Access: AccessHidden, FromProfile: true},
					{Path: "${HOME}/c", Access: AccessReadWrite, FromProfile: true},
					{Path: "${HOME}/d", Access: AccessReadOnly, FromProfile: true},
				}
			},
			fields: []Field{FieldHome},
			folderBy: map[string][]string{
				"${HOME}/a": {"whitelist ${HOME}/a", "read-only ${HOME}/a"},
				"${HOME}/c": {"blacklist ${HOME}/c", "noblacklist ${HOME}/c"},
			},
		},
		{
			name:  "paths that are not folders the user knows are uncovered",
			lines: []string{"blacklist /dev/dri", "noblacklist /proc/1", "blacklist /sys", "whitelist /run/user/1000/x", "blacklist /tmp/x", "blacklist /usr/bin/su", "blacklist ${HOME}/*.key", "blacklist ${PATH}/su", "blacklist /var", "blacklist ${HOME}", "blacklist ../x"},
			unc:   []string{"blacklist /dev/dri", "noblacklist /proc/1", "blacklist /sys", "whitelist /run/user/1000/x", "blacklist /tmp/x", "blacklist /usr/bin/su", "blacklist ${HOME}/*.key", "blacklist ${PATH}/su", "blacklist /var", "blacklist ${HOME}", "blacklist ../x"},
		},
		{
			name:   "a glob under the home still chooses the folders and is listed",
			lines:  []string{"whitelist ${HOME}/.config/app*.conf"},
			want:   func(s *Settings) { s.Home = HomeChosen },
			fields: []Field{FieldHome}, unc: []string{"whitelist ${HOME}/.config/app*.conf"},
		},
		{
			name:  "whitelist-ro makes a locked read-only row",
			lines: []string{"whitelist-ro ${HOME}/.config/ro"},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.Folders = []Folder{{Path: "${HOME}/.config/ro", Access: AccessReadOnly, FromProfile: true, Locked: true}}
			},
			fields:   []Field{FieldHome},
			folderBy: map[string][]string{"${HOME}/.config/ro": {"whitelist ${HOME}/.config/ro", "read-only ${HOME}/.config/ro"}},
		},
		{
			name:  "lines of .inc files are not rows",
			lines: []string{"include shared.inc"},
			files: map[string]string{"shared.inc": "noblacklist ${HOME}/.gitconfig\nwhitelist ${HOME}/shared\nnosound\nblacklist /mnt/x\n"},
			want: func(s *Settings) {
				s.Home = HomeChosen
				s.Sound = false
			},
			fields: []Field{FieldHome, FieldSound},
			unc:    []string{"noblacklist ${HOME}/.gitconfig", "whitelist ${HOME}/shared", "blacklist /mnt/x"},
		},
		{
			name:  "lines of .local files are rows",
			lines: []string{"include mine.local"},
			files: map[string]string{"mine.local": "noblacklist ${HOME}/.mine\n"},
			want: func(s *Settings) {
				s.Folders = []Folder{{Path: "${HOME}/.mine", Access: AccessReadWrite, FromProfile: true}}
			},
		},
		{
			name:  "lines of another .profile are rows",
			lines: []string{"include other.profile"},
			files: map[string]string{"other.profile": "read-only /srv/data\n"},
			want: func(s *Settings) {
				s.Folders = []Folder{{Path: "/srv/data", Access: AccessReadOnly, FromProfile: true}}
			},
		},
		{
			name: "ignore removes the line before it is mapped", lines: []string{"ignore nosound", "nosound", "novideo", "ignore whitelist", "whitelist ${HOME}/x"},
			want: func(s *Settings) { s.Camera = false }, fields: []Field{FieldCamera},
		},
		{
			name: "commands no field covers", lines: []string{"tracelog", "hostname box", "allusers", "keep-dev-shm"},
			unc: []string{"tracelog", "hostname box", "allusers", "keep-dev-shm"},
		},
		{
			name: "a bare command with an argument is uncovered", lines: []string{"nosound now", "private-tmp x"},
			unc: []string{"nosound now", "private-tmp x"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			for name, content := range tc.files {
				e.sysFile(name, content)
			}
			lines := make([]string, len(tc.lines))
			for i, l := range tc.lines {
				lines[i] = strings.ReplaceAll(l, "@HOME@", e.home)
			}
			prof := e.mustRead(lines...)
			b := BaseValues(prof, e.home)

			want := wantSettings(tc.want)
			for i := range want.Folders {
				want.Folders[i].Path = strings.ReplaceAll(want.Folders[i].Path, "@HOME@", e.home)
			}
			requireSettings(t, b.Settings, want)

			if got := sourceFields(b); !slices.Equal(got, sorted(tc.fields...)) {
				t.Errorf("source fields = %v, want %v", got, sorted(tc.fields...))
			}
			wantUnc := make([]string, len(tc.unc))
			for i, u := range tc.unc {
				wantUnc[i] = strings.ReplaceAll(u, "@HOME@", e.home)
			}
			if got := uncoveredTexts(b); !slices.Equal(got, wantUnc) {
				t.Errorf("uncovered = %q, want %q", got, wantUnc)
			}
			for path, wantLines := range tc.folderBy {
				if got := texts(b.FolderSources[path]); !slices.Equal(got, wantLines) {
					t.Errorf("FolderSources[%q] = %q, want %q", path, got, wantLines)
				}
			}
			wantRows := 0
			for _, f := range want.Folders {
				wantRows++
				if _, ok := b.FolderSources[f.Path]; !ok {
					t.Errorf("FolderSources has no entry for the row %q", f.Path)
				}
			}
			if len(b.FolderSources) != wantRows {
				t.Errorf("len(FolderSources) = %d, want %d", len(b.FolderSources), wantRows)
			}
		})
	}
}

func TestBaseValuesNoprintersSources(t *testing.T) {
	e := newEnv(t)
	b := BaseValues(e.mustRead("noprinters"), e.home)
	src := b.Sources[FieldPrinters]
	requireTexts(t, "Sources[printers]", src, "noprinters", "blacklist /dev/lp*", "blacklist /run/cups/cups.sock")
	if src[1].Origin != "noprinters" {
		t.Errorf("Origin = %q, want noprinters", src[1].Origin)
	}
	if len(b.Uncovered) != 0 || len(b.Settings.Folders) != 0 {
		t.Errorf("the lines of noprinters leaked: uncovered %+v, folders %+v", b.Uncovered, b.Settings.Folders)
	}
}

func TestBaseValuesSourcesKeepEveryLine(t *testing.T) {
	e := newEnv(t)
	b := BaseValues(e.mustRead("nosound", "private", "nosound", "dns 1.1.1.1", "dns 8.8.8.8", "whitelist ${HOME}/a", "private /x"), "")
	requireTexts(t, "Sources[sound]", b.Sources[FieldSound], "nosound", "nosound")
	requireTexts(t, "Sources[dns]", b.Sources[FieldDNS], "dns 1.1.1.1", "dns 8.8.8.8")
	// Every line that decides the home is a source, whatever the mode that wins.
	requireTexts(t, "Sources[home]", b.Sources[FieldHome], "private", "whitelist ${HOME}/a", "private /x")
	requireTexts(t, "Sources[private_dir]", b.Sources[FieldPrivateDir], "private /x")
	if b.Sources[FieldSound][0].Number != 1 || b.Sources[FieldSound][1].Number != 3 {
		t.Errorf("the sources lost their line numbers: %+v", b.Sources[FieldSound])
	}
}

func TestBaseValuesConditional(t *testing.T) {
	e := newEnv(t)
	prof := e.mustRead("?HAS_NET: nosound", "net none", "?HAS_NET: novideo", "?HAS_NOSOUND: no3d")
	b := BaseValues(prof, e.home)
	// HAS_NET is false at the first line (the Sound stays allowed) and true at the third;
	// HAS_NOSOUND is false because nosound was skipped.
	requireSettings(t, b.Settings, wantSettings(func(s *Settings) {
		s.Network = NetNone
		s.Camera = false
	}))
	if len(b.Conditional) != 2 {
		t.Fatalf("Conditional = %+v, want 2", b.Conditional)
	}
	if b.Conditional[0].Line.Text != "nosound" || b.Conditional[0].Line.Conditional != "HAS_NET" {
		t.Errorf("Conditional[0] = %+v", b.Conditional[0])
	}
	if b.Conditional[1].Line.Text != "no3d" {
		t.Errorf("Conditional[1] = %+v", b.Conditional[1])
	}
	// The false conditionals are not uncovered lines.
	if len(b.Uncovered) != 0 {
		t.Errorf("Uncovered = %+v, want none", b.Uncovered)
	}
}

func TestBaseValuesSharedSlicesAreCopied(t *testing.T) {
	prof, home := fixtureProfile(t, "default.profile")
	b := BaseValues(prof, home)
	b.Settings.Protocols[0] = "changed"
	again := BaseValues(prof, home)
	if again.Settings.Protocols[0] != "unix" {
		t.Error("two BaseValues calls share a slice")
	}
}

func TestInHome(t *testing.T) {
	tests := []struct {
		path   string
		home   string
		strict bool
		want   bool
	}{
		{"${HOME}/x", "", true, true},
		{"${HOME}", "", true, false},
		{"${HOME}", "", false, true},
		{"~/x", "", true, true},
		{"~", "", false, true},
		{"~x", "", false, false},
		{"${HOMEX}/x", "", false, false},
		{"${DOWNLOADS}", "", true, true},
		{"${DOWNLOADS}/a", "", true, true},
		{"${DOWNLOADSX}", "", true, false},
		{"${PATH}/x", "", false, false},
		{"/home/me/x", "/home/me", true, true},
		{"/home/me", "/home/me", true, false},
		{"/home/me", "/home/me/", false, true},
		{"/home/mex", "/home/me", false, false},
		{"/home/me/x", "", false, false},
		{"/mnt/data", "/home/me", false, false},
	}
	for _, tc := range tests {
		if got := inHome(tc.path, tc.home, tc.strict); got != tc.want {
			t.Errorf("inHome(%q, %q, %v) = %v, want %v", tc.path, tc.home, tc.strict, got, tc.want)
		}
	}
}

func TestParsers(t *testing.T) {
	sizes := map[string]int64{"1": 1, "10K": 10 << 10, "3m": 3 << 20, "1G": 1 << 30, "9223372036854775807": 9223372036854775807}
	for in, want := range sizes {
		if got, ok := parseSize(in); !ok || got != want {
			t.Errorf("parseSize(%q) = %d, %v, want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "0K", "K", "-1", "+1", " 1", "1 ", "1T", "99999999999999999999", "8589934592G"} {
		if got, ok := parseSize(in); ok {
			t.Errorf("parseSize(%q) = %d, true, want false", in, got)
		}
	}
	timeouts := map[string]time.Duration{"00:00:01": time.Second, "99:59:59": 99*time.Hour + 59*time.Minute + 59*time.Second, "1:2:3": time.Hour + 2*time.Minute + 3*time.Second}
	for in, want := range timeouts {
		if got, ok := parseTimeout(in); !ok || got != want {
			t.Errorf("parseTimeout(%q) = %v, %v, want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "00:00:00", "100:00:00", "1:2", "1:2:3:4", "a:b:c", "-1:00:00", "001:00:00"} {
		if got, ok := parseTimeout(in); ok {
			t.Errorf("parseTimeout(%q) = %v, true, want false", in, got)
		}
	}
}
