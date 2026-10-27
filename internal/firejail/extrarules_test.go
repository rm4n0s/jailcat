package firejail

import (
	"slices"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

func extraDirs() xdg.Dirs {
	return xdg.Dirs{
		Home:           "/home/me",
		FirejailSystem: "/etc/firejail",
		FirejailUser:   "/home/me/.config/firejail",
		Imported:       "/home/me/.local/share/jailcat/profiles/imported",
	}
}

func TestCheckExtraRules(t *testing.T) {
	prof, home := fixtureProfile(t, "code.profile")
	code := BaseValues(prof, home)

	type want struct {
		sev   Severity
		tag   string
		field Field
		path  string
	}
	errTag := func(tag string) want { return want{sev: SeverityError, tag: tag} }
	warn := func(f Field) want { return want{sev: SeverityWarning, tag: ProblemExtraCovered, field: f} }

	tests := []struct {
		name string
		row  string
		want []want
	}{
		// Rows that are fine.
		{name: "empty", row: ""},
		{name: "blanks", row: "   \t  "},
		{name: "comment", row: "# a comment"},
		{name: "comment after blanks", row: "   # a comment"},
		{name: "a rule with a trailing comment", row: "blacklist /mnt/x # not this"},
		{name: "blacklist", row: "blacklist /mnt/x"},
		{name: "noblacklist", row: "noblacklist ${HOME}/.config/foo"},
		{name: "whitelist", row: "whitelist ${HOME}/x"},
		{name: "mkdir", row: "mkdir ${HOME}/x"},
		{name: "net with an interface", row: "net eth0"},
		{name: "noexec of another folder", row: "noexec /tmp"},
		{name: "seccomp with a list", row: "seccomp !chroot"},
		{name: "x11 none", row: "x11 none"},
		{name: "condition", row: "?HAS_X11: blacklist /mnt/x"},
		{name: "ignore of a line no field owns", row: "ignore whitelist ${HOME}/.config/Electron"},
		{name: "ignore of the include", row: "ignore include disable-xdg.inc"},
		{name: "caps.keep", row: "caps.keep sys_admin"},
		{name: "extra spaces are normalized", row: "  blacklist     /mnt/x  "},

		// Commands.
		{name: "unknown command", row: "nosuchcommand foo", want: []want{errTag(ProblemExtraCommand)}},
		{name: "removed command shell", row: "shell none", want: []want{errTag(ProblemExtraCommand)}},
		{name: "removed command keep-hostname", row: "keep-hostname", want: []want{errTag(ProblemExtraCommand)}},
		{name: "command is case sensitive", row: "NoSound", want: []want{errTag(ProblemExtraCommand)}},

		// Conditions.
		{name: "unknown condition", row: "?NOPE: nosound", want: []want{errTag(ProblemExtraCondition)}},
		{name: "condition without a colon", row: "?HAS_NET nosound", want: []want{errTag(ProblemExtraCondition)}},
		{name: "condition without a line", row: "?HAS_NET:", want: []want{errTag(ProblemExtraCondition)}},
		{name: "include under a condition", row: "?HAS_NET: include foo.inc", want: []want{errTag(ProblemExtraCondition)}},
		{name: "quiet under a condition", row: "?HAS_NET: quiet", want: []want{errTag(ProblemExtraCondition)}},
		{name: "nested conditions", row: "?HAS_NET: ?HAS_X11: blacklist /mnt/x"},
		{name: "nested with a bad one", row: "?HAS_NET: ?NOPE: blacklist /mnt/x", want: []want{errTag(ProblemExtraCondition)}},
		{name: "unknown command under a condition", row: "?HAS_NET: nope", want: []want{errTag(ProblemExtraCommand)}},
		{name: "covered under a condition", row: "?HAS_NET: nosound", want: []want{warn(FieldSound)}},

		// include.
		{name: "include a bare name", row: "include globals.local"},
		{name: "include in the system folder", row: "include /etc/firejail/disable-xdg.inc"},
		{name: "include ${CFG}", row: "include ${CFG}/disable-xdg.inc"},
		{name: "include in the user folder", row: "include /home/me/.config/firejail/mine.profile"},
		{name: "include ${HOME} user folder", row: "include ${HOME}/.config/firejail/mine.profile"},
		{name: "include ~ user folder", row: "include ~/.config/firejail/mine.profile"},
		{name: "include an imported profile", row: "include /home/me/.local/share/jailcat/profiles/imported/a.profile"},
		{name: "include elsewhere", row: "include /tmp/evil.inc", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include in a subfolder", row: "include /etc/firejail/sub/x.inc", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include with ..", row: "include /etc/firejail/../passwd", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include $HOME", row: "include $HOME/.config/firejail/x", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include in the home", row: "include ${HOME}/x.inc", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include ${RUNUSER}", row: "include ${RUNUSER}/x.inc", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include with a glob", row: "include /etc/firejail/a*b", want: []want{errTag(ProblemExtraInclude)}},
		{name: "include with no file", row: "include", want: []want{errTag(ProblemExtraArgs)}},

		// Arguments.
		{name: "ignore with no text", row: "ignore", want: []want{errTag(ProblemExtraArgs)}},
		{name: "whitelist with no path", row: "whitelist", want: []want{errTag(ProblemExtraArgs)}},
		{name: "a bare command with an argument", row: "nosound now", want: []want{errTag(ProblemExtraArgs)}},
		{name: "dns not an address", row: "dns example.com", want: []want{errTag(ProblemExtraArgs)}},
		{name: "nice not a number", row: "nice high", want: []want{errTag(ProblemExtraArgs)}},
		{name: "timeout wrong format", row: "timeout 1:2", want: []want{errTag(ProblemExtraArgs)}},
		{name: "rlimit-as zero", row: "rlimit-as 0", want: []want{errTag(ProblemExtraArgs)}},
		{name: "rlimit-nofile not a number", row: "rlimit-nofile many", want: []want{errTag(ProblemExtraArgs)}},
		{name: "cpu out of range", row: "cpu 40", want: []want{errTag(ProblemExtraArgs)}},
		{name: "env without =", row: "env FOO", want: []want{errTag(ProblemExtraArgs)}},
		{name: "env with a bad name", row: "env 1A=x", want: []want{errTag(ProblemExtraArgs)}},
		{name: "rlimit-fsize with a unit", row: "rlimit-fsize 10M"},

		// Values.
		{name: "new line in a row", row: "blacklist /a\nblacklist /b", want: []want{errTag(ProblemValue)}},
		{name: "carriage return in a row", row: "blacklist /a\r", want: []want{errTag(ProblemValue)}},
		{name: "NUL in a row", row: "blacklist /a\x00", want: []want{errTag(ProblemValue)}},

		// Rows a field covers.
		{name: "nosound", row: "nosound", want: []want{warn(FieldSound)}},
		{name: "novideo", row: "novideo", want: []want{warn(FieldCamera)}},
		{name: "no3d", row: "no3d", want: []want{warn(FieldGPU)}},
		{name: "noprinters", row: "noprinters", want: []want{warn(FieldPrinters)}},
		{name: "net none", row: "net none", want: []want{warn(FieldNetwork)}},
		{name: "dns", row: "dns 1.1.1.1", want: []want{warn(FieldDNS)}},
		{name: "hosts-file", row: "hosts-file /etc/hosts2", want: []want{warn(FieldHostsFile)}},
		{name: "netfilter", row: "netfilter", want: []want{warn(FieldFirewall)}},
		{name: "dbus-user none", row: "dbus-user none", want: []want{warn(FieldSessionBus)}},
		{name: "dbus-user filter", row: "dbus-user filter", want: []want{warn(FieldSessionBus)}},
		{name: "dbus-user.talk", row: "dbus-user.talk org.x", want: []want{warn(FieldSessionTalk)}},
		{name: "dbus-user.own", row: "dbus-user.own org.x", want: []want{warn(FieldSessionOwn)}},
		{name: "dbus-system none", row: "dbus-system none", want: []want{warn(FieldSystemBus)}},
		{name: "x11 xpra", row: "x11 xpra", want: []want{warn(FieldX11)}},
		{name: "private", row: "private", want: []want{warn(FieldHome)}},
		{name: "private DIR", row: "private ${HOME}/x", want: []want{warn(FieldPrivateDir)}},
		{name: "disable-mnt", row: "disable-mnt", want: []want{warn(FieldHideRemovable)}},
		{name: "noexec ${HOME}", row: "noexec ${HOME}", want: []want{warn(FieldNoExecHome)}},
		{name: "private-tmp", row: "private-tmp", want: []want{warn(FieldPrivateTmp)}},
		{name: "private-dev", row: "private-dev", want: []want{warn(FieldPrivateDev)}},
		{name: "private-cache", row: "private-cache", want: []want{warn(FieldPrivateCache)}},
		{name: "seccomp", row: "seccomp", want: []want{warn(FieldSeccomp)}},
		{name: "caps.drop all", row: "caps.drop all", want: []want{warn(FieldDropCaps)}},
		{name: "nonewprivs", row: "nonewprivs", want: []want{warn(FieldNoNewPrivs)}},
		{name: "noroot", row: "noroot", want: []want{warn(FieldNoRoot)}},
		{name: "restrict-namespaces", row: "restrict-namespaces", want: []want{warn(FieldRestrictNamespaces)}},
		{name: "memory-deny-write-execute", row: "memory-deny-write-execute", want: []want{warn(FieldMemoryDenyWriteExecute)}},
		{name: "landlock.enforce", row: "landlock.enforce", want: []want{warn(FieldLandlock)}},
		{name: "protocol", row: "protocol unix", want: []want{warn(FieldProtocols)}},
		{name: "private-bin", row: "private-bin sh", want: []want{warn(FieldPrivateBin)}},
		{name: "private-etc", row: "private-etc hosts", want: []want{warn(FieldPrivateEtc)}},
		{name: "env", row: "env A=b", want: []want{warn(FieldEnv)}},
		{name: "rlimit-as", row: "rlimit-as 1G", want: []want{warn(FieldMemory)}},
		{name: "cpu", row: "cpu 0,1", want: []want{warn(FieldCPUs)}},
		{name: "nice", row: "nice 5", want: []want{warn(FieldNice)}},
		{name: "timeout", row: "timeout 01:00:00", want: []want{warn(FieldTimeout)}},
		{name: "rlimit-nofile", row: "rlimit-nofile 10", want: []want{warn(FieldMaxFiles)}},
		{name: "rlimit-nproc", row: "rlimit-nproc 10", want: []want{warn(FieldMaxProcs)}},
		{name: "join-or-start", row: "join-or-start x", want: []want{warn(FieldSingleInstance)}},
		{name: "quiet", row: "quiet", want: []want{warn(FieldQuiet)}},
		{name: "noblacklist ${DOWNLOADS}", row: "noblacklist ${DOWNLOADS}", want: []want{warn(FieldDownloadsReadWrite)}},
		{name: "ignore of a line a field owns", row: "ignore nosound", want: []want{warn(FieldSound)}},
		{name: "ignore of a line another field owns", row: "ignore private-tmp", want: []want{warn(FieldPrivateTmp)}},
		{name: "ignore of a folder row", row: "ignore noblacklist ${HOME}/.vscode",
			want: []want{{sev: SeverityWarning, tag: ProblemExtraCovered, path: "${HOME}/.vscode"}}},
		{name: "ignore that also hits the row Code - OSS", row: "ignore noblacklist ${HOME}/.config/Code",
			want: []want{
				{sev: SeverityWarning, tag: ProblemExtraCovered, path: "${HOME}/.config/Code"},
				{sev: SeverityWarning, tag: ProblemExtraCovered, path: "${HOME}/.config/Code - OSS"},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckExtraRules([]string{tt.row}, extraDirs(), code)
			if len(got) != len(tt.want) {
				t.Fatalf("CheckExtraRules(%q) = %+v, want %d problems", tt.row, got, len(tt.want))
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Severity != w.sev || g.Tag != w.tag || g.Field != w.field || g.Path != w.path {
					t.Errorf("problem %d = {%s %s field %q path %q}, want {%s %s field %q path %q}",
						i, g.Severity, g.Tag, g.Field, g.Path, w.sev, w.tag, w.field, w.path)
				}
				if g.Row != 1 || !strings.HasPrefix(g.Message, "Row 1: ") {
					t.Errorf("problem %d: Row %d, message %q", i, g.Row, g.Message)
				}
			}
		})
	}
}

func TestCheckExtraRulesRowNumbers(t *testing.T) {
	rows := []string{"", "# note", "nosound", "", "nope", "blacklist /x", "include /tmp/x"}
	got := CheckExtraRules(rows, extraDirs(), BaseNone())
	var summary []string
	for _, p := range got {
		summary = append(summary, p.Tag+"@"+string(rune('0'+p.Row)))
	}
	if want := []string{ProblemExtraCovered + "@3", ProblemExtraCommand + "@5", ProblemExtraInclude + "@7"}; !slices.Equal(summary, want) {
		t.Errorf("problems = %v, want %v", summary, want)
	}
	if CheckExtraRules(nil, extraDirs(), BaseNone()) != nil {
		t.Errorf("no rows must give no problems")
	}
}

func TestExtraRulesErrorsAndWarnings(t *testing.T) {
	// Only errors block Build and Try.
	got := CheckExtraRules([]string{"nosound", "nope"}, extraDirs(), BaseNone())
	if len(got) != 2 || got[0].Severity != SeverityWarning || got[1].Severity != SeverityError {
		t.Fatalf("problems = %+v", got)
	}
}

// TestCoverageMatchesTheWriter checks the rows the Extra rules tab calls covered are the
// lines the writer produces: every line of every field maps back to its field.
func TestCoverageMatchesTheWriter(t *testing.T) {
	cv := coveredBy()
	m := lineMaker{home: "/home/probe"}
	seen := map[Field]bool{}
	for _, s := range probeSettings() {
		for _, f := range Fields() {
			lines, err := m.fieldLines(f, &s)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			for _, text := range lines {
				if cmd, _ := splitCommand(text); cmd == "include" {
					continue
				}
				seen[f] = true
				if got, ok := cv.field(text); !ok || got != f {
					t.Errorf("%q: covered by %q (%v), want %s", text, got, ok, f)
				}
			}
		}
	}
	// Every field that has a line in the profile is covered: the launcher fields that
	// write nothing are the only ones left out.
	for _, f := range Fields() {
		if !seen[f] && !notInProfile(f) {
			t.Errorf("the probe never writes a line for %s", f)
		}
	}
	if !seen[FieldQuiet] {
		t.Errorf("quiet is not covered")
	}
}

func TestFieldLabelsAndTabs(t *testing.T) {
	for _, f := range Fields() {
		if notInProfile(f) {
			continue
		}
		if fieldLabel(f) == string(f) {
			t.Errorf("field %s has no label for the Extra rules warnings", f)
		}
	}
	if tabLabel(TabDevices) != "Devices" || tabLabel("") != "" {
		t.Errorf("tabLabel = %q, %q", tabLabel(TabDevices), tabLabel(""))
	}
}
