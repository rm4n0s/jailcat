package apps

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveCommand(t *testing.T) {
	s := newFakeSystem(t)
	prog := s.path("bin1/prog")
	tests := []struct {
		name   string
		words  []string
		want   command
		reason string // a part of the reason; "" when the command resolves
	}{
		{"bare name", []string{"prog", "%U"}, command{binary: prog, args: []string{"%U"}}, ""},
		{"absolute path", []string{prog, "-x"}, command{binary: prog, args: []string{"-x"}}, ""},
		{"env with assignments", []string{"env", "A=1", "B=2", "prog", "x"}, command{binary: prog, args: []string{"x"}}, ""},
		{"env by absolute path", []string{"/usr/bin/env", "prog"}, command{binary: prog}, ""},
		{"env options", []string{"env", "-i", "-0", "-", "--ignore-environment", "--null", "prog"}, command{binary: prog}, ""},
		{"env -u NAME", []string{"env", "-u", "HOME", "prog"}, command{binary: prog}, ""},
		{"env -uNAME", []string{"env", "-uHOME", "prog"}, command{binary: prog}, ""},
		{"env --unset NAME", []string{"env", "--unset", "HOME", "prog"}, command{binary: prog}, ""},
		{"env --unset=NAME", []string{"env", "--unset=HOME", "prog"}, command{binary: prog}, ""},
		{"env -- ends the options", []string{"env", "--", "A=b", "prog"}, command{binary: prog}, ""},
		{"env twice", []string{"env", "A=b", "env", "C=d", "prog"}, command{binary: prog}, ""},
		{"env with a program that has = in its arguments", []string{"env", "prog", "A=b"}, command{binary: prog, args: []string{"A=b"}}, ""},
		{"env with an unknown option", []string{"env", "-C", "/tmp", "prog"}, command{}, "env option"},
		{"env with nothing after", []string{"env", "A=b"}, command{}, "no program"},
		{"env alone", []string{"env"}, command{}, "no program"},
		{"env -u at the end", []string{"env", "-u"}, command{}, "no program"},
		{"firejail wrapper", []string{"firejail", "--private", "--net=none", "prog", "%U"}, command{binary: prog, args: []string{"%U"}, usesFirejail: true}, ""},
		{"firejail by path, no options", []string{"/usr/bin/firejail", "prog"}, command{binary: prog, usesFirejail: true}, ""},
		{"firejail -- ends the options", []string{"firejail", "--quiet", "--", "prog"}, command{binary: prog, usesFirejail: true}, ""},
		{"firejail then env", []string{"firejail", "env", "A=b", "prog"}, command{binary: prog, usesFirejail: true}, ""},
		{"env then firejail", []string{"env", "A=b", "firejail", "--x", "prog"}, command{binary: prog, usesFirejail: true}, ""},
		{"firejail alone", []string{"firejail", "--private"}, command{}, "no program"},
		{"flatpak run", []string{"flatpak", "run", "org.x.Y"}, command{
			binary: s.path("bin2/flatpak"), args: []string{"run", "org.x.Y"}, sandbox: SandboxFlatpak,
		}, ""},
		{"flatpak with an option before run", []string{"flatpak", "--user", "run", "org.x.Y"}, command{
			binary: s.path("bin2/flatpak"), args: []string{"--user", "run", "org.x.Y"}, sandbox: SandboxFlatpak,
		}, ""},
		{"env flatpak run", []string{"env", "A=b", "flatpak", "run", "x"}, command{
			binary: s.path("bin2/flatpak"), args: []string{"run", "x"}, sandbox: SandboxFlatpak,
		}, ""},
		{"flatpak without run is an ordinary program", []string{"flatpak", "list"}, command{
			binary: s.path("bin2/flatpak"), args: []string{"list"},
		}, ""},
		{"snap program", []string{"/snap/bin/firefox", "%u"}, command{
			binary: "/snap/bin/firefox", args: []string{"%u"}, sandbox: SandboxSnap,
		}, ""},
		{"snap program behind env", []string{"env", "BAMF=/x.desktop", "/snap/bin/firefox"}, command{
			binary: "/snap/bin/firefox", sandbox: SandboxSnap,
		}, ""},
		{"firecfg link by path resolves past it", []string{s.path("bin1/firecfgapp")}, command{binary: s.path("bin2/firecfgapp")}, ""},
		{"firecfg link by name resolves past it", []string{"firecfgapp"}, command{binary: s.path("bin2/firecfgapp")}, ""},
		{"firecfg link to a link", []string{s.path("bin1/firecfgchain")}, command{}, "firecfg link"},
		{"firecfg link with nothing behind it", []string{"firecfgonly"}, command{}, "not an executable on $PATH"},
		{"program not found", []string{"nothing-like-this"}, command{}, "not an executable on $PATH"},
		{"absolute path that does not exist", []string{s.path("bin1/gone")}, command{}, "not an executable file"},
		{"absolute path without execute bits", []string{s.path("bin1/notexec")}, command{}, "not an executable file"},
		{"relative path", []string{"bin1/prog"}, command{}, "relative"},
		{"empty program", []string{""}, command{}, "not an executable on $PATH"},
		{"no words", nil, command{}, "no program"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := resolveCommand(tt.words, s.dirs)
			if tt.reason != "" {
				if !strings.Contains(reason, tt.reason) {
					t.Errorf("reason = %q, want it to mention %q", reason, tt.reason)
				}
				if !reflect.DeepEqual(got, command{}) {
					t.Errorf("command = %+v with a reason", got)
				}
				return
			}
			if reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			if len(got.args) == 0 {
				got.args = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("command = %+v\nwant      %+v", got, tt.want)
			}
		})
	}
}

func TestTryExecFound(t *testing.T) {
	s := newFakeSystem(t)
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"name on PATH", "prog", true},
		{"absolute executable", s.path("bin1/prog"), true},
		{"absolute without execute bits", s.path("bin1/notexec"), false},
		{"absolute missing", s.path("bin1/gone"), false},
		{"absolute folder", s.path("bin1/adir"), false},
		{"name not on PATH", "nothing-like-this", false},
		{"only a firecfg link", "firecfgonly", false},
		{"relative path", "bin1/prog", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tryExecFound(tt.value, s.dirs); got != tt.want {
				t.Errorf("tryExecFound(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
