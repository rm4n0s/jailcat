package firejail

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// requireRoute fails the test unless err is an *errors.Error whose route contains route.
func requireRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain with newline", "nosound\n", "nosound"},
		{"no newline", "nosound", "nosound"},
		{"empty", "", ""},
		{"only newline", "\n", ""},
		{"only blanks", "  \t \t\n", ""},
		{"leading blanks", "  \t nosound\n", "nosound"},
		{"run of spaces", "net    none\n", "net none"},
		{"tabs between words", "net\t\tnone\n", "net none"},
		{"mixed tab and space run", "net \t \tnone\n", "net none"},
		{"trailing space", "nosound \n", "nosound"},
		{"trailing run of blanks", "nosound \t \t\n", "nosound"},
		{"trailing blanks without newline", "nosound  ", "nosound"},
		{"comment only", "# a comment\n", ""},
		{"indented comment", "   # a comment\n", ""},
		{"comment after the command", "nosound # why\n", "nosound"},
		{"hash in the middle of a word", "whitelist ${HOME}/a#b\n", "whitelist ${HOME}/a"},
		{"hash right after the command", "nosound#x\n", "nosound"},
		{"blanks before the comment", "nosound   \t # x\n", "nosound"},
		{"carriage return ends the line", "nosound\r\n", "nosound"},
		{"carriage return in the middle", "a\rb\n", "a"},
		{"nul ends the line", "a\x00b\n", "a"},
		{"form feed is not a blank", "a\fb\n", "a\fb"},
		{"no-break space is not a blank", " nosound\n", " nosound"},
		{"one inner space is kept", "noblacklist ${HOME}/.config/Code - OSS\n", "noblacklist ${HOME}/.config/Code - OSS"},
		{"unicode", "whitelist ${HOME}/Загрузки\n", "whitelist ${HOME}/Загрузки"},
		{"conditional", "?HAS_NET:   net none\n", "?HAS_NET: net none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.raw); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name   string
		ignore string
		line   string
		want   bool
	}{
		{"same command", "nosound", "nosound", true},
		{"command with other command after it", "nosound", "nosound x", true},
		{"longer word is not a match", "nosound", "nosounds", false},
		{"hyphenated word is not a match", "whitelist", "whitelist-ro /a", false},
		{"whitelist matches every whitelist", "whitelist", "whitelist ${HOME}/a", true},
		{"whitelist matches another whitelist", "whitelist", "whitelist /usr/share/x", true},
		{"whitelist does not match noblacklist", "whitelist", "nowhitelist /a", false},
		{"path with a longer name after a space", "whitelist ${HOME}/.config/Code", "whitelist ${HOME}/.config/Code - OSS", true},
		{"path that continues without a space", "whitelist ${HOME}/.config/Code", "whitelist ${HOME}/.config/Code2", false},
		{"exact path", "whitelist ${HOME}/.config/Code", "whitelist ${HOME}/.config/Code", true},
		{"macro text against an expanded path", "whitelist ${HOME}/x", "whitelist /home/me/x", false},
		{"expanded path against a macro", "whitelist /home/me/x", "whitelist ${HOME}/x", false},
		{"entry longer than the line", "net none", "net", false},
		{"first word of the entry only", "net", "net none", true},
		{"net none against net eth0", "net none", "net eth0", false},
		{"the case matters", "NoSound", "nosound", false},
		{"entry includes the include", "include disable-xdg.inc", "include disable-xdg.inc", true},
		{"include of another file", "include disable-xdg.inc", "include disable-xdg.local", false},
		{"dbus-user does not skip dbus-user.talk", "dbus-user", "dbus-user.talk org.x", false},
		{"dbus-user none against dbus-user filter", "dbus-user none", "dbus-user filter", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(tt.ignore, tt.line); got != tt.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tt.ignore, tt.line, got, tt.want)
			}
		})
	}
}

func TestFgetsChunks(t *testing.T) {
	long := strings.Repeat("a", 9000)
	tests := []struct {
		name string
		data string
		want []int // the length of each chunk
	}{
		{"empty", "", nil},
		{"two short lines", "ab\ncd\n", []int{3, 3}},
		{"no final newline", "ab\ncd", []int{3, 2}},
		{"exactly 8191 bytes then a newline", strings.Repeat("a", 8191) + "\nb\n", []int{8191, 1, 2}},
		{"a long line is cut at 8191 bytes", long + "\nb\n", []int{8191, 810, 2}},
		{"a nul cuts the chunk", "ab\x00cd\nef\n", []int{2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, c := range fgetsChunks([]byte(tt.data)) {
				got = append(got, len(c))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("chunk lengths = %v, want %v", got, tt.want)
			}
		})
	}
}

// commandsOfBrief is the list the manager extracted from profile.c, minus keep-hostname.
const commandsOfBrief = `allow-bwrap allow-debuggers allusers apparmor apparmor-replace apparmor-stack bind
blacklist blacklist-nolog caps caps.drop caps.keep cpu dbus-system dbus-system.broadcast dbus-system.call
dbus-system.own dbus-system.see dbus-system.talk dbus-user dbus-user.broadcast dbus-user.call dbus-user.own
dbus-user.see dbus-user.talk defaultgw deterministic-exit-code deterministic-shutdown disable-mnt dns env
hostname hostname-randomize hosts-file ignore include ip ip6 ipc-namespace iprange join-or-start
keep-config-pulse keep-dev-ntsync keep-dev-shm keep-dev-tpm keep-fd keep-shell-rc keep-var-tmp
landlock.enforce landlock.fs.execute landlock.fs.makedev landlock.fs.makeipc landlock.fs.read
landlock.fs.write mac machine-id memory-deny-write-execute mkdir mkfile mtu name net netfilter netfilter6
netlock netmask netns nice no3d noautopulse noblacklist nodbus nodvd noexec nogroups noinput nonewprivs
noprinters noroot nosound notpm notv nou2f novideo nowhitelist private private-bin private-cache
private-cwd private-dev private-etc private-home private-lib private-opt private-srv private-tmp protocol
quiet read-only read-write restrict-namespaces rlimit-as rlimit-cpu rlimit-fsize rlimit-nofile
rlimit-nproc rlimit-sigpending rmenv seccomp seccomp.32 seccomp.32.drop seccomp.32.keep
seccomp.block-secondary seccomp.drop seccomp-error-action seccomp.keep timeout tmpfs tracelog veth-name
warn whitelist whitelist-ro writable-etc writable-run-user writable-var writable-var-log x11
xephyr-screen`

func TestCommands(t *testing.T) {
	got := Commands()
	want := strings.Fields(commandsOfBrief)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("Commands() = %v\nwant %v", got, want)
	}
	if !slices.IsSorted(got) {
		t.Error("Commands() is not sorted")
	}
	if len(slices.Compact(slices.Clone(got))) != len(got) {
		t.Error("Commands() has duplicates")
	}
	for _, not := range []string{"tab", "shell", "keep-hostname", "xephyr-extra-params", "rlimit", ""} {
		if IsCommand(not) {
			t.Errorf("IsCommand(%q) = true, want false", not)
		}
	}
	for _, yes := range []string{"nosound", "whitelist", "ignore", "dbus-user.talk", "seccomp.32.drop"} {
		if !IsCommand(yes) {
			t.Errorf("IsCommand(%q) = false, want true", yes)
		}
	}
}

func TestCommandsReturnsFreshSlice(t *testing.T) {
	a := Commands()
	a[0] = "changed"
	if b := Commands(); b[0] == "changed" {
		t.Error("Commands() shares its slice between calls")
	}
}

func TestEndsWithLocal(t *testing.T) {
	tests := []struct {
		fname string
		want  bool
	}{
		{"code.local", true},
		{"/etc/firejail/code.local", true},
		{"/home/u/.config/firejail/globals.local", true},
		{".local", true},
		{"code.localx", false},
		{"code.local.local", false},
		{"code.local.inc", false},
		{"code.profile", false},
		{"/a.local/code.profile", false},
		{"/a.local/", false},
	}
	for _, tt := range tests {
		t.Run(tt.fname, func(t *testing.T) {
			if got := endsWithLocal(tt.fname); got != tt.want {
				t.Errorf("endsWithLocal(%q) = %v, want %v", tt.fname, got, tt.want)
			}
		})
	}
}

func TestInvalidFilename(t *testing.T) {
	tests := []struct {
		fname string
		bad   bool
	}{
		{"/etc/firejail/code.profile", false},
		{"/home/me/my profiles/a b.inc", false},
		{"${PATH}/x.inc", false},
		{"/a/b*c", true},
		{"/a/b?c", true},
		{"/a/[b]", true},
		{"/a/b,c", true},
		{"/a/b;c", true},
		{"/a/b\\c", true},
		{"/a/b%c", true},
		{"/a/b\x01c", true},
		{"/a/b\x7fc", true},
		{"/a/b$c", false},
		{"/a/b'c", false},
	}
	for _, tt := range tests {
		t.Run(tt.fname, func(t *testing.T) {
			if got := invalidFilename(tt.fname) != ""; got != tt.bad {
				t.Errorf("invalidFilename(%q) rejected = %v, want %v", tt.fname, got, tt.bad)
			}
		})
	}
}
