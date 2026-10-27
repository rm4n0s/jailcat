package firejail

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// validSettings is a Settings that passes Validate.
func validSettings() Settings {
	s := DefaultSettings()
	s.SandboxName = "firejail-code"
	return s
}

func TestValidateDefaultsPass(t *testing.T) {
	if got := Validate(validSettings(), "/home/me"); len(got) != 0 {
		t.Errorf("Validate(valid) = %+v, want none", got)
	}
	// The base of firejail's own defaults and the base of a read profile also pass once
	// they have a name.
	for _, name := range []string{"code.profile", "default.profile"} {
		prof, home := fixtureProfile(t, name)
		s := BaseValues(prof, home).Settings
		s.SandboxName = "x"
		if got := Validate(s, home); len(got) != 0 {
			t.Errorf("Validate(base of %s) = %+v, want none", name, got)
		}
	}
}

func TestValidate(t *testing.T) {
	const home = "/home/me"
	tests := []struct {
		name string
		edit func(s *Settings)
		// want is the tag, the field and the path of each problem, in order.
		want []Problem
	}{
		// Modes: the zero value is not valid.
		{name: "home unspecified", edit: func(s *Settings) { s.Home = HomeUnspecified },
			want: []Problem{{Field: FieldHome, Tag: ProblemMode}}},
		{name: "home unknown", edit: func(s *Settings) { s.Home = "everything" },
			want: []Problem{{Field: FieldHome, Tag: ProblemMode}}},
		{name: "network unspecified", edit: func(s *Settings) { s.Network = "" },
			want: []Problem{{Field: FieldNetwork, Tag: ProblemMode}}},
		{name: "session bus unspecified", edit: func(s *Settings) { s.SessionBus = "" },
			want: []Problem{{Field: FieldSessionBus, Tag: ProblemMode}}},
		{name: "system bus filtered is allowed", edit: func(s *Settings) { s.SystemBus = BusFiltered }},
		{name: "system bus unknown", edit: func(s *Settings) { s.SystemBus = "some" },
			want: []Problem{{Field: FieldSystemBus, Tag: ProblemMode}}},
		{name: "x11 unspecified", edit: func(s *Settings) { s.X11 = "" },
			want: []Problem{{Field: FieldX11, Tag: ProblemMode}}},
		{name: "x11 xpra", edit: func(s *Settings) { s.X11 = X11Xpra }},
		{name: "icon unspecified", edit: func(s *Settings) { s.Icon = "" },
			want: []Problem{{Field: FieldIcon, Tag: ProblemMode}}},
		{name: "icon random", edit: func(s *Settings) { s.Icon = IconRandom }},

		// Sandbox name.
		{name: "name empty", edit: func(s *Settings) { s.SandboxName = "" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with a capital", edit: func(s *Settings) { s.SandboxName = "Firejail-code" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with a dot", edit: func(s *Settings) { s.SandboxName = "firejail.code" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with a slash", edit: func(s *Settings) { s.SandboxName = "../x" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with a space", edit: func(s *Settings) { s.SandboxName = "a b" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with a newline", edit: func(s *Settings) { s.SandboxName = "a\nb" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with non-ASCII letters", edit: func(s *Settings) { s.SandboxName = "café" },
			want: []Problem{{Field: FieldSandboxName, Tag: ProblemSandboxName}}},
		{name: "name with digits and dashes", edit: func(s *Settings) { s.SandboxName = "firejail-code-2" }},

		// Install.
		{name: "only the menu", edit: func(s *Settings) { s.InstallBin = false }},
		{name: "only the bin folder", edit: func(s *Settings) { s.InstallMenu = false }},
		{name: "install nothing", edit: func(s *Settings) { s.InstallMenu, s.InstallBin = false, false },
			want: []Problem{{Field: FieldInstallMenu, Tag: ProblemInstallNothing}}},

		// Folder rows.
		{name: "folder absolute", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/data", Access: AccessReadOnly}}
		}},
		{name: "folder ${HOME}", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${HOME}/Projects", Access: AccessReadWrite}}
		}},
		{name: "folder tilde", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "~/Projects", Access: AccessHidden}}
		}},
		{name: "folder macro", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${DOWNLOADS}/x", Access: AccessReadWrite}, {Path: "${MUSIC}", Access: AccessReadOnly}}
		}},
		{name: "folder with a space", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${HOME}/.config/Code - OSS", Access: AccessReadWrite}}
		}},
		{name: "folder relative", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "Projects", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "Projects"}}},
		{name: "folder empty", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: ""}}},
		{name: "folder $HOME without braces", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "$HOME/x", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "$HOME/x"}}},
		{name: "folder with a macro that is not a folder", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${PATH}/x", Access: AccessReadWrite}, {Path: "${HOMEX}/y", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "${PATH}/x"}, {Tag: ProblemPath, Path: "${HOMEX}/y"}}},
		{name: "folder with dot dot", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/../etc", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "/mnt/../etc"}}},
		{name: "folder with a glob", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/*", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "/mnt/*"}}},
		{name: "folder with a comma", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/a,b", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemPath, Path: "/mnt/a,b"}}},
		{name: "folder with a newline", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/a\nb", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemValue, Path: "/mnt/a\nb"}}},
		{name: "folder with a hash", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/a#b", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemValue, Path: "/mnt/a#b"}}},
		{name: "folder access unknown", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/a", Access: "rw"}, {Path: "/mnt/b"}}
		}, want: []Problem{{Tag: ProblemFolderAccess, Path: "/mnt/a"}, {Tag: ProblemFolderAccess, Path: "/mnt/b"}}},
		{name: "folder twice", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "/mnt/a", Access: AccessReadWrite}, {Path: "/mnt/a", Access: AccessHidden}}
		}, want: []Problem{{Tag: ProblemFolderDuplicate, Path: "/mnt/a"}}},
		{name: "locked row read-only", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${HOME}/x", Access: AccessReadOnly, FromProfile: true, Locked: true}}
		}},
		{name: "locked row loosened", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${HOME}/x", Access: AccessReadWrite, FromProfile: true, Locked: true}}
		}, want: []Problem{{Tag: ProblemFolderLocked, Path: "${HOME}/x"}}},

		// The real home stays out of an empty or private home.
		{name: "full home keeps rows in the home", edit: func(s *Settings) {
			s.Folders = []Folder{{Path: "${HOME}/a", Access: AccessReadWrite}}
		}},
		{name: "chosen home keeps rows in the home", edit: func(s *Settings) {
			s.Home = HomeChosen
			s.Folders = []Folder{{Path: "~/a", Access: AccessReadWrite}}
		}},
		{name: "empty home refuses ${HOME}", edit: func(s *Settings) {
			s.Home = HomeEmpty
			s.Folders = []Folder{{Path: "${HOME}/a", Access: AccessReadWrite}}
		}, want: []Problem{{Tag: ProblemFolderInHome, Path: "${HOME}/a"}}},
		{name: "empty home refuses ~", edit: func(s *Settings) {
			s.Home = HomeEmpty
			s.Folders = []Folder{{Path: "~/a", Access: AccessReadOnly}}
		}, want: []Problem{{Tag: ProblemFolderInHome, Path: "~/a"}}},
		{name: "private home refuses a user folder macro", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.Folders = []Folder{{Path: "${DOCUMENTS}/a", Access: AccessReadOnly}}
		}, want: []Problem{{Tag: ProblemFolderInHome, Path: "${DOCUMENTS}/a"}}},
		{name: "private home refuses the absolute home", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.Folders = []Folder{{Path: "/home/me/a", Access: AccessHidden}}
		}, want: []Problem{{Tag: ProblemFolderInHome, Path: "/home/me/a"}}},
		{name: "private home refuses the home itself", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.Folders = []Folder{{Path: "/home/me", Access: AccessReadOnly}}
		}, want: []Problem{{Tag: ProblemFolderInHome, Path: "/home/me"}}},
		{name: "private home takes folders outside", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.Folders = []Folder{{Path: "/mnt/data", Access: AccessReadWrite}, {Path: "/home/mexico/x", Access: AccessReadOnly}}
		}},
		{name: "private home leaves the profile's own rows", edit: func(s *Settings) {
			s.Home = HomeEmpty
			s.Folders = []Folder{{Path: "${HOME}/.config/Code", Access: AccessReadWrite, FromProfile: true}}
		}},
		{name: "private dir", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.PrivateDir = "${HOME}/.local/share/jailcat/homes/x"
		}},
		{name: "private dir relative", edit: func(s *Settings) {
			s.Home = HomePrivate
			s.PrivateDir = "homes/x"
		}, want: []Problem{{Field: FieldHome, Tag: ProblemPath}}},

		// Network.
		{name: "four DNS servers", edit: func(s *Settings) { s.DNS = []string{"1.1.1.1", "8.8.8.8", "9.9.9.9", "::1"} }},
		{name: "five DNS servers", edit: func(s *Settings) {
			s.DNS = []string{"1.1.1.1", "8.8.8.8", "9.9.9.9", "::1", "2.2.2.2"}
		}, want: []Problem{{Field: FieldDNS, Tag: ProblemDNSCount}}},
		{name: "DNS name", edit: func(s *Settings) { s.DNS = []string{"dns.example.com"} },
			want: []Problem{{Field: FieldDNS, Tag: ProblemDNSAddress}}},
		{name: "DNS with a zone", edit: func(s *Settings) { s.DNS = []string{"fe80::1%eth0"} },
			want: []Problem{{Field: FieldDNS, Tag: ProblemDNSAddress}}},
		{name: "DNS empty", edit: func(s *Settings) { s.DNS = []string{""} },
			want: []Problem{{Field: FieldDNS, Tag: ProblemDNSAddress}}},
		{name: "hosts file", edit: func(s *Settings) { s.HostsFile = "${HOME}/hosts" }},
		{name: "hosts file relative", edit: func(s *Settings) { s.HostsFile = "hosts" },
			want: []Problem{{Field: FieldHostsFile, Tag: ProblemPath}}},
		{name: "hosts file with a hash", edit: func(s *Settings) { s.HostsFile = "/etc/hosts#x" },
			want: []Problem{{Field: FieldHostsFile, Tag: ProblemValue}}},

		// Desktop.
		{name: "bus names", edit: func(s *Settings) {
			s.SessionBus = BusFiltered
			s.SessionTalk = []string{DBusNotifications, DBusPortals}
			s.SessionOwn = []string{MPRISName("code")}
		}},
		{name: "bus name with a blank", edit: func(s *Settings) { s.SessionTalk = []string{"org.a b"} },
			want: []Problem{{Field: FieldSessionTalk, Tag: ProblemBusName}}},
		{name: "bus name empty", edit: func(s *Settings) { s.SessionOwn = []string{""} },
			want: []Problem{{Field: FieldSessionOwn, Tag: ProblemBusName}}},
		{name: "bus name with a hash", edit: func(s *Settings) { s.SessionTalk = []string{"org.a#b"} },
			want: []Problem{{Field: FieldSessionTalk, Tag: ProblemValue}}},

		// Security.
		{name: "all protocols", edit: func(s *Settings) {
			s.Protocols = []string{"unix", "inet", "inet6", "netlink", "packet", "bluetooth"}
		}},
		{name: "protocol unknown", edit: func(s *Settings) { s.Protocols = []string{"unix", "ipx"} },
			want: []Problem{{Field: FieldProtocols, Tag: ProblemProtocol}}},
		{name: "protocol with a comma", edit: func(s *Settings) { s.Protocols = []string{"unix,inet"} },
			want: []Problem{{Field: FieldProtocols, Tag: ProblemListItem}}},
		{name: "protocol empty", edit: func(s *Settings) { s.Protocols = []string{""} },
			want: []Problem{{Field: FieldProtocols, Tag: ProblemListItem}}},
		{name: "protocol in capitals", edit: func(s *Settings) { s.Protocols = []string{"UNIX"} },
			want: []Problem{{Field: FieldProtocols, Tag: ProblemProtocol}}},

		// Programs.
		{name: "private-bin", edit: func(s *Settings) { s.PrivateBin = []string{"sh", "code"} }},
		{name: "private-bin with a comma", edit: func(s *Settings) { s.PrivateBin = []string{"sh,bash"} },
			want: []Problem{{Field: FieldPrivateBin, Tag: ProblemListItem}}},
		{name: "private-bin empty item", edit: func(s *Settings) { s.PrivateBin = []string{"sh", ""} },
			want: []Problem{{Field: FieldPrivateBin, Tag: ProblemListItem}}},
		{name: "private-bin with a hash", edit: func(s *Settings) { s.PrivateBin = []string{"s#h"} },
			want: []Problem{{Field: FieldPrivateBin, Tag: ProblemValue}}},
		{name: "private-etc", edit: func(s *Settings) { s.PrivateEtc = []string{"hosts", "ssl"} }},
		{name: "private-etc with a slash", edit: func(s *Settings) { s.PrivateEtc = []string{"ssl/certs"} },
			want: []Problem{{Field: FieldPrivateEtc, Tag: ProblemPrivateEtc}}},
		{name: "private-etc absolute", edit: func(s *Settings) { s.PrivateEtc = []string{"/etc/hosts"} },
			want: []Problem{{Field: FieldPrivateEtc, Tag: ProblemPrivateEtc}}},
		{name: "private-etc with a comma", edit: func(s *Settings) { s.PrivateEtc = []string{"a,b"} },
			want: []Problem{{Field: FieldPrivateEtc, Tag: ProblemListItem}}},
		{name: "env", edit: func(s *Settings) {
			s.Env = []EnvVar{{"_A1", "x y"}, {"b", ""}, {"EDITOR", "/usr/bin/vim -f"}}
		}},
		{name: "env name with a dash", edit: func(s *Settings) { s.Env = []EnvVar{{"A-B", "x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemEnvName}}},
		{name: "env name starting with a digit", edit: func(s *Settings) { s.Env = []EnvVar{{"1A", "x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemEnvName}}},
		{name: "env name empty", edit: func(s *Settings) { s.Env = []EnvVar{{"", "x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemEnvName}}},
		{name: "env name with an equals sign", edit: func(s *Settings) { s.Env = []EnvVar{{"A=B", "x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemEnvName}}},
		{name: "env value with a newline", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "x\ny"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with a NUL", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "x\x00y"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with a hash", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "#x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with a tab", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "x\ty"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with a leading blank", edit: func(s *Settings) { s.Env = []EnvVar{{"A", " x"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with a trailing blank", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "x "}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "env value with two blanks", edit: func(s *Settings) { s.Env = []EnvVar{{"A", "x  y"}} },
			want: []Problem{{Field: FieldEnv, Tag: ProblemValue}}},
		{name: "extra arguments may have anything but NUL", edit: func(s *Settings) {
			s.ExtraArgs = []string{"--flag=a b", "#x", " ", "", "a\nb"}
		}},
		{name: "extra argument with a NUL", edit: func(s *Settings) { s.ExtraArgs = []string{"a\x00b"} },
			want: []Problem{{Field: FieldExtraArgs, Tag: ProblemValue}}},

		// Limits.
		{name: "limits", edit: func(s *Settings) {
			s.MemoryBytes, s.MaxFiles, s.MaxProcs = 1<<30, 1024, 100
			s.CPUs = []int{0, 1, 31}
			s.Nice = intp(-20)
			s.Timeout = 99*time.Hour + 59*time.Minute + 59*time.Second
		}},
		{name: "nice 19", edit: func(s *Settings) { s.Nice = intp(19) }},
		{name: "nice 20", edit: func(s *Settings) { s.Nice = intp(20) },
			want: []Problem{{Field: FieldNice, Tag: ProblemNice}}},
		{name: "nice -21", edit: func(s *Settings) { s.Nice = intp(-21) },
			want: []Problem{{Field: FieldNice, Tag: ProblemNice}}},
		{name: "timeout 100h", edit: func(s *Settings) { s.Timeout = 100 * time.Hour },
			want: []Problem{{Field: FieldTimeout, Tag: ProblemTimeout}}},
		{name: "timeout negative", edit: func(s *Settings) { s.Timeout = -time.Second },
			want: []Problem{{Field: FieldTimeout, Tag: ProblemTimeout}}},
		{name: "timeout with a fraction of a second", edit: func(s *Settings) { s.Timeout = 1500 * time.Millisecond },
			want: []Problem{{Field: FieldTimeout, Tag: ProblemTimeout}}},
		{name: "cpu negative", edit: func(s *Settings) { s.CPUs = []int{-1} },
			want: []Problem{{Field: FieldCPUs, Tag: ProblemCPU}}},
		{name: "cpu 32", edit: func(s *Settings) { s.CPUs = []int{32} },
			want: []Problem{{Field: FieldCPUs, Tag: ProblemCPU}}},
		{name: "cpu twice", edit: func(s *Settings) { s.CPUs = []int{2, 3, 2} },
			want: []Problem{{Field: FieldCPUs, Tag: ProblemCPU}}},
		{name: "memory negative", edit: func(s *Settings) { s.MemoryBytes = -1 },
			want: []Problem{{Field: FieldMemory, Tag: ProblemNumber}}},
		{name: "files negative", edit: func(s *Settings) { s.MaxFiles = -1 },
			want: []Problem{{Field: FieldMaxFiles, Tag: ProblemNumber}}},
		{name: "processes negative", edit: func(s *Settings) { s.MaxProcs = -5 },
			want: []Problem{{Field: FieldMaxProcs, Tag: ProblemNumber}}},

		// Several problems come back together, in the order of the tabs.
		{name: "many at once", edit: func(s *Settings) {
			s.Timeout = -1
			s.DNS = []string{"x"}
			s.Home = ""
			s.SandboxName = ""
		}, want: []Problem{
			{Field: FieldHome, Tag: ProblemMode},
			{Field: FieldDNS, Tag: ProblemDNSAddress},
			{Field: FieldTimeout, Tag: ProblemTimeout},
			{Field: FieldSandboxName, Tag: ProblemSandboxName},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validSettings()
			tc.edit(&s)
			got := Validate(s, home)
			if len(got) != len(tc.want) {
				t.Fatalf("Validate() = %+v, want %d problems", got, len(tc.want))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Tag != w.Tag || g.Field != w.Field || g.Path != w.Path {
					t.Errorf("problem %d = {Field %q Path %q Tag %q}, want {Field %q Path %q Tag %q}",
						i, g.Field, g.Path, g.Tag, w.Field, w.Path, w.Tag)
				}
				if g.Message == "" {
					t.Errorf("problem %d has no message", i)
				}
			}
		})
	}
}

func TestValidateRelativeHomeAndNoHome(t *testing.T) {
	// With no home given, an absolute path is not recognized as inside it; the macros
	// still are.
	s := validSettings()
	s.Home = HomeEmpty
	s.Folders = []Folder{{Path: "/home/me/a", Access: AccessReadOnly}, {Path: "${HOME}/b", Access: AccessReadOnly}}
	got := Validate(s, "")
	if len(got) != 1 || got[0].Path != "${HOME}/b" || got[0].Tag != ProblemFolderInHome {
		t.Errorf("Validate() = %+v", got)
	}
}

func TestValidateMessagesAreForTheUser(t *testing.T) {
	s := validSettings()
	s.Home = HomeEmpty
	s.Folders = []Folder{{Path: "${HOME}/a", Access: AccessReadOnly}}
	got := Validate(s, "/home/me")
	if len(got) != 1 || !strings.Contains(got[0].Message, "inside your home folder") {
		t.Errorf("message = %+v", got)
	}
}

func TestForbiddenValue(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"plain", false},
		{"with space", false},
		{"/path/with spaces/and-dashes_1.2", false},
		{"unicode é中", false},
		{"a\nb", true},
		{"a\rb", true},
		{"a\x00b", true},
		{"a#b", true},
		{"#", true},
		{"a\tb", true},
		{" a", true},
		{"a ", true},
		{"a  b", true},
		{" ", true},
	}
	for _, tc := range tests {
		if got := forbiddenValue(tc.in) != ""; got != tc.want {
			t.Errorf("forbiddenValue(%q) forbidden = %v, want %v", tc.in, got, tc.want)
		}
	}
	// A value that passes is read back as written: Normalize does not change it.
	for _, in := range []string{"plain", "with space", "/a b/c"} {
		if got := Normalize(in); got != in {
			t.Errorf("Normalize(%q) = %q: forbiddenValue would have to refuse it", in, got)
		}
	}
	// And one that is refused is changed by Normalize or ends the line.
	for _, in := range []string{"a#b", " a", "a ", "a  b", "a\tb", "a\nb"} {
		if Normalize(in) == in {
			t.Errorf("Normalize(%q) keeps the value, but forbiddenValue refuses it", in)
		}
	}
}

func TestDesktopChecks(t *testing.T) {
	s := DefaultSettings()
	if got := s.DesktopChecks("code"); got != (DesktopChecks{}) {
		t.Errorf("DesktopChecks() = %+v, want none", got)
	}
	s.SessionTalk = []string{DBusNotifications, "org.example.Other", DBusKeyring}
	s.SessionOwn = []string{MPRISName("code")}
	want := DesktopChecks{Notifications: true, Keyring: true, MediaKeys: true}
	if got := s.DesktopChecks("code"); got != want {
		t.Errorf("DesktopChecks(code) = %+v, want %+v", got, want)
	}
	// The media keys name has the binary's name.
	want.MediaKeys = false
	if got := s.DesktopChecks("other"); got != want {
		t.Errorf("DesktopChecks(other) = %+v, want %+v", got, want)
	}
	s.SessionTalk = append(s.SessionTalk, DBusPortals, DBusTray)
	if got := s.DesktopChecks("other"); !got.Portals || !got.Tray {
		t.Errorf("DesktopChecks() = %+v, want portals and tray", got)
	}
	if MPRISName("firefox") != "org.mpris.MediaPlayer2.firefox.*" {
		t.Errorf("MPRISName = %q", MPRISName("firefox"))
	}
}

func TestWithName(t *testing.T) {
	tests := []struct {
		name string
		list []string
		item string
		on   bool
		want []string
	}{
		{"add to nil", nil, "a", true, []string{"a"}},
		{"add to a list", []string{"x"}, "a", true, []string{"x", "a"}},
		{"already there stays in place", []string{"a", "x"}, "a", true, []string{"a", "x"}},
		{"remove", []string{"x", "a", "y"}, "a", false, []string{"x", "y"}},
		{"remove the last", []string{"a"}, "a", false, nil},
		{"remove what is not there", []string{"x"}, "a", false, []string{"x"}},
		{"remove from nil", nil, "a", false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := slices.Clone(tc.list)
			got := WithName(tc.list, tc.item, tc.on)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("WithName() = %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(tc.list, orig) {
				t.Errorf("WithName changed its argument: %v, was %v", tc.list, orig)
			}
		})
	}
}
