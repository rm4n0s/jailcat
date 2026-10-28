package firejail

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
)

// notInProfile are the fields no profile line holds: the launcher script and the desktop
// entry do. Quiet is read from Profile.Quiet, not from the base values. The VS Code
// switches are Jailcat's own: the generator writes their lines with the folder rows, and
// no base profile gives them a value.
func notInProfile(f Field) bool {
	switch f {
	case FieldSandboxName, FieldMenuName, FieldComment, FieldIcon, FieldIconSeed,
		FieldInstallMenu, FieldInstallBin, FieldOpenFileTypes, FieldTerminal, FieldExtraArgs, FieldQuiet,
		FieldVSCodeOwnData, FieldVSCodeShareSettings, FieldVSCodeShellFiles:
		return true
	}
	return false
}

// diffRead compares what firejail would read from a generated profile with the settings
// the user chose. It returns one line per field that differs.
func diffRead(final Settings, prof *Profile, got Base, home string) []string {
	var out []string
	m := lineMaker{home: home}
	want := final.Clone()
	if want.Nice != nil && *want.Nice < 0 {
		n := 0 // profile.c: a user who is not root gets 0 for a negative nice
		want.Nice = &n
	}
	for _, row := range fieldTable() {
		f := row.field
		if notInProfile(f) {
			continue
		}
		if f == FieldPrivateDir {
			if final.Home == HomePrivate && got.Settings.PrivateDir != m.canon(final.PrivateDir) {
				out = append(out, fmt.Sprintf("%s = %q, want %q", f, got.Settings.PrivateDir, m.canon(final.PrivateDir)))
			}
			continue
		}
		if !row.equal(&got.Settings, &want) {
			out = append(out, fmt.Sprintf("%s = %v, want %v", f, row.get(&got.Settings), row.get(&want)))
		}
	}
	if prof.Quiet != final.Quiet {
		out = append(out, fmt.Sprintf("quiet = %v, want %v", prof.Quiet, final.Quiet))
	}
	rows := func(folders []Folder, canon bool) map[string]Access {
		r := map[string]Access{}
		for _, f := range folders {
			p := f.Path
			if canon {
				p = m.canon(p)
			}
			r[p] = f.Access
		}
		return r
	}
	if g, w := rows(got.Settings.Folders, false), rows(final.Folders, true); !maps.Equal(g, w) {
		out = append(out, fmt.Sprintf("folders = %v, want %v", g, w))
	}
	return out
}

// roundTrip generates the profile of spec, writes it, reads it back the way firejail
// does and compares the base values of what was read with the settings the user chose.
// It returns the generated text and the differences. A Generate error is returned as it
// is.
func (e *writerEnv) roundTrip(spec ProfileSpec) (string, []string, error) {
	e.t.Helper()
	g, err := Generate(spec)
	if err != nil {
		return "", nil, err
	}
	prof, err := e.read(e.write(g))
	if err != nil {
		e.t.Fatalf("Read of the generated profile: %v\n%s", err, g)
	}
	got := BaseValues(prof, e.home)
	final := spec.Overrides.Apply(spec.Base.Settings)
	return g.String(), diffRead(final, prof, got, e.home), nil
}

// rtValues returns the values to try for a field, other than its current one: every
// choice of a mode, the flipped checkbox, a list with something added and one emptied.
func rtValues(t *testing.T, f Field, cur any) []any {
	t.Helper()
	switch f {
	case FieldHome:
		var out []any
		for _, m := range []HomeMode{HomeFull, HomeChosen, HomeEmpty, HomePrivate} {
			if m != cur.(HomeMode) {
				out = append(out, m)
			}
		}
		return out
	case FieldPrivateDir:
		return []any{"${HOME}/.local/share/jailcat/homes/other"}
	case FieldNetwork:
		return []any{map[NetMode]NetMode{NetFull: NetNone, NetNone: NetFull}[cur.(NetMode)]}
	case FieldSessionBus, FieldSystemBus:
		var out []any
		for _, m := range []BusMode{BusFull, BusFiltered, BusNone} {
			if m != cur.(BusMode) {
				out = append(out, m)
			}
		}
		return out
	case FieldX11:
		var out []any
		for _, m := range []X11Mode{X11Off, X11Xpra, X11Xephyr, X11Xvfb} {
			if m != cur.(X11Mode) {
				out = append(out, m)
			}
		}
		return out
	case FieldDNS:
		return []any{[]string{"9.9.9.9"}, []string{"1.1.1.1", "2606:4700:4700::1111"}, []string(nil)}
	case FieldHostsFile:
		return []any{"/etc/hosts.jailcat", ""}
	case FieldTimeout:
		return []any{90 * time.Second, 2 * time.Hour, time.Duration(0)}
	case FieldMemory:
		return []any{int64(1 << 30), int64(0)}
	case FieldMaxFiles, FieldMaxProcs:
		return []any{int64(500), int64(0)}
	case FieldCPUs:
		return []any{[]int{0, 1, 5}, []int(nil)}
	case FieldNice:
		five, minus := 5, -5
		return []any{&five, &minus, (*int)(nil)}
	case FieldEnv:
		return []any{[]EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "two words"}}, []EnvVar(nil)}
	case FieldSandboxName, FieldMenuName, FieldComment, FieldIcon, FieldIconSeed,
		FieldInstallMenu, FieldInstallBin, FieldOpenFileTypes, FieldTerminal, FieldExtraArgs:
		return nil
	}
	out := []any{alternative(t, f, cur)}
	if rv := reflect.ValueOf(cur); rv.Kind() == reflect.Slice && rv.Len() > 0 {
		out = append(out, reflect.Zero(rv.Type()).Interface())
	}
	return out
}

// setField stores a value of a field into s.
func setField(t *testing.T, s *Settings, f Field, v any) {
	t.Helper()
	row, ok := findRow(f)
	if !ok {
		t.Fatalf("no field %s", f)
	}
	row.set(s, v)
}

// settle makes edited settings valid the way the editor would: a private home has a
// folder, "Only the folders I choose" has a folder to allow, and an empty or private
// home has no Downloads and no folder of the real home.
func settle(s *Settings) {
	if s.Home == HomePrivate && s.PrivateDir == "" {
		s.PrivateDir = "${HOME}/.local/share/jailcat/homes/x"
	}
	switch s.Home {
	case HomeEmpty, HomePrivate:
		s.DownloadsReadWrite = false
		s.Folders = slices.DeleteFunc(s.Folders, func(f Folder) bool {
			return !f.FromProfile && inHome(f.Path, "", false)
		})
	case HomeChosen:
		allows := s.DownloadsReadWrite
		for _, f := range s.Folders {
			allows = allows || (f.Access != AccessHidden && inHome(f.Path, "", true))
		}
		if !allows {
			s.Folders = append(s.Folders, Folder{Path: "${HOME}/Allowed", Access: AccessReadWrite})
		}
	}
}

// bases returns the base profiles the round trip runs over, as paths ("" is base None):
// VS Code's chain and the default profile of the fixtures, base None, and small profiles
// written for the cases the fixtures lack: Downloads and a folder in a restricted home,
// a private home with mkdir, and lines that serve two fields or are made by firejail.
func (e *writerEnv) bases() []string {
	return []string{
		filepath.Join(e.sys, "code.profile"),
		filepath.Join(e.sys, "default.profile"),
		"",
		e.profile("downloads.profile",
			"noblacklist ${DOWNLOADS}", "whitelist ${DOWNLOADS}",
			"noblacklist ${HOME}/a", "whitelist ${HOME}/a", "read-only ${HOME}/a",
			"noblacklist ${HOME}/b", "whitelist ${HOME}/b", "nosound"),
		e.profile("private.profile",
			"private ${HOME}/p", "mkdir ${HOME}/m", "mkfile ${HOME}/f", "noblacklist ${HOME}/c", "nosound", "novideo"),
		e.profile("emptyhome.profile", "private", "noblacklist ${HOME}/c", "private-cache"),
		e.profile("misc.profile",
			"quiet", "nodbus", "noprinters", "protocol unix,inet", "protocol inet6", "private-bin a,b", "private-bin c",
			"private-etc hosts", "dns 1.1.1.1", "dns 8.8.8.8", "hosts-file /etc/hosts", "rlimit-as 1G", "rlimit-nofile 100",
			"cpu 0,1", "nice 5", "timeout 01:00:00", "x11 xpra", "env A=1", "env B=two words", "dbus-user.talk org.a",
			"dbus-user.own org.b.*", "caps.drop all", "seccomp", "landlock.enforce", "join-or-start foo",
			"net none", "noexec ${HOME}", "disable-mnt", "memory-deny-write-execute"),
		e.profile("conditional.profile", "net none", "?HAS_NET: nou2f", "noblacklist ${HOME}/a", "?HAS_NET: noblacklist ${HOME}/a b"),
	}
}

func baseName(b string) string {
	if b == "" {
		return "none"
	}
	return filepath.Base(b)
}

// requireRoundTrip generates, reads back and fails the test on a difference.
func (e *writerEnv) requireRoundTrip(what string, spec ProfileSpec) {
	e.t.Helper()
	text, diffs, err := e.roundTrip(spec)
	if err != nil {
		e.t.Errorf("%s: Generate: %v", what, err)
		return
	}
	if len(diffs) > 0 {
		e.t.Errorf("%s: the profile does not read back as chosen:\n  %s\n%s", what, strings.Join(diffs, "\n  "), text)
	}
}

// TestRoundTripFields changes each field of the table on its own, over each base, with
// each value from rtValues, and checks the generated profile reads back as chosen.
func TestRoundTripFields(t *testing.T) {
	e := newWriterEnv(t, "")
	for _, base := range e.bases() {
		cur := e.specFor(base, nil).Base.Settings
		for _, f := range Fields() {
			row, _ := findRow(f)
			for i, v := range rtValues(t, f, row.get(&cur)) {
				what := fmt.Sprintf("%s: %s -> %v (#%d)", baseName(base), f, v, i)
				e.requireRoundTrip(what, e.specFor(base, func(s *Settings) {
					setField(t, s, f, v)
					settle(s)
				}))
			}
		}
	}
}

// TestRoundTripNoChange checks that a build with no override reads back as its base, over
// each base, and that with base None only Jailcat's defaults are written.
func TestRoundTripNoChange(t *testing.T) {
	e := newWriterEnv(t, "")
	for _, base := range e.bases() {
		e.requireRoundTrip(baseName(base), e.specFor(base, nil))
	}
}

// TestRoundTripHomeModes checks each home choice over each base, with and without folder
// rows and Downloads.
func TestRoundTripHomeModes(t *testing.T) {
	e := newWriterEnv(t, "")
	for _, base := range e.bases() {
		for _, mode := range []HomeMode{HomeFull, HomeChosen, HomeEmpty, HomePrivate} {
			for _, rows := range []string{"none", "read-write", "read-only", "hidden", "outside", "all"} {
				for _, downloads := range []bool{false, true} {
					what := fmt.Sprintf("%s: %s, rows %s, downloads %v", baseName(base), mode, rows, downloads)
					e.requireRoundTrip(what, e.specFor(base, func(s *Settings) {
						s.Home = mode
						s.PrivateDir = "${HOME}/.local/share/jailcat/homes/x"
						s.DownloadsReadWrite = downloads
						add := func(p string, a Access) { s.Folders = append(s.Folders, Folder{Path: p, Access: a}) }
						switch rows {
						case "read-write":
							add("${HOME}/Projects", AccessReadWrite)
						case "read-only":
							add("${HOME}/Projects", AccessReadOnly)
						case "hidden":
							add("${HOME}/Projects", AccessHidden)
						case "outside":
							add("/mnt/data", AccessReadWrite)
							add("/opt/ro", AccessReadOnly)
						case "all":
							add("${HOME}/Projects", AccessReadWrite)
							add("${HOME}/Notes", AccessReadOnly)
							add("${HOME}/Secret", AccessHidden)
							add("/mnt/data", AccessReadWrite)
						}
						settle(s)
					}))
				}
			}
		}
	}
}

// TestRoundTripFolderRows changes, removes and adds rows over the base rows of VS Code.
func TestRoundTripFolderRows(t *testing.T) {
	e := newWriterEnv(t, "")
	code := e.spec("code.profile", nil).Base.Settings
	var paths []string
	for _, f := range code.Folders {
		paths = append(paths, f.Path)
	}
	if len(paths) < 5 {
		t.Fatalf("expected the base rows of VS Code, got %v", paths)
	}
	for _, mode := range []HomeMode{HomeFull, HomeChosen, HomeEmpty, HomePrivate} {
		for i, path := range paths {
			for _, a := range []Access{AccessReadWrite, AccessReadOnly, AccessHidden} {
				e.requireRoundTrip(fmt.Sprintf("%s: row %s -> %s", mode, path, a), e.spec("code.profile", func(s *Settings) {
					s.Home = mode
					s.Folders[i].Access = a
					settle(s)
				}))
			}
			e.requireRoundTrip(fmt.Sprintf("%s: row %s removed", mode, path), e.spec("code.profile", func(s *Settings) {
				s.Home = mode
				s.Folders = slices.Delete(s.Folders, i, i+1)
				settle(s)
			}))
		}
		e.requireRoundTrip(fmt.Sprintf("%s: every base row removed", mode), e.spec("code.profile", func(s *Settings) {
			s.Home = mode
			s.Folders = nil
			settle(s)
		}))
	}
}

// TestRoundTripRandom changes many fields at once, over each base. The generator is
// seeded, so a failure repeats.
func TestRoundTripRandom(t *testing.T) {
	e := newWriterEnv(t, "")
	rng := rand.New(rand.NewPCG(7, 11))
	for _, base := range e.bases() {
		cur := e.specFor(base, nil).Base.Settings
		ok, tooMany := 0, 0
		for i := 0; i < 150; i++ {
			var picked []string
			spec := e.specFor(base, func(s *Settings) {
				for _, f := range Fields() {
					if rng.Float64() > 0.2 {
						continue
					}
					row, _ := findRow(f)
					vals := rtValues(t, f, row.get(&cur))
					if len(vals) == 0 {
						continue
					}
					v := vals[rng.IntN(len(vals))]
					setField(t, s, f, v)
					picked = append(picked, fmt.Sprintf("%s=%v", f, v))
				}
				pool := []string{"${HOME}/Projects", "${HOME}/Notes", "/mnt/data", "${HOME}/.config/Code", "${HOME}/.vscode"}
				for range rng.IntN(3) {
					path := pool[rng.IntN(len(pool))]
					a := []Access{AccessReadWrite, AccessReadOnly, AccessHidden}[rng.IntN(3)]
					if j := slices.IndexFunc(s.Folders, func(f Folder) bool { return f.Path == path }); j >= 0 {
						if rng.IntN(2) == 0 {
							s.Folders = slices.Delete(s.Folders, j, j+1)
						} else {
							s.Folders[j].Access = a
						}
						continue
					}
					s.Folders = append(s.Folders, Folder{Path: path, Access: a})
				}
				settle(s)
			})
			text, diffs, err := e.roundTrip(spec)
			if err != nil {
				appErr, isErr := errors.FromError(err)
				if isErr && appErr.Tag == tagIgnoreLimit {
					tooMany++
					continue
				}
				t.Errorf("%s #%d: Generate: %v (changed %v)", baseName(base), i, err, picked)
				continue
			}
			ok++
			if len(diffs) > 0 {
				t.Errorf("%s #%d: does not read back as chosen:\n  %s\nchanged: %v\n%s",
					baseName(base), i, strings.Join(diffs, "\n  "), picked, text)
			}
		}
		if ok < 50 {
			t.Errorf("%s: only %d of 150 random builds were generated (%d needed too many ignores)", baseName(base), ok, tooMany)
		}
	}
}

// TestGeneratedProfilesUseKnownCommands checks that every line Generate writes starts
// with a command firejail knows, with many fields changed at once.
func TestGeneratedProfilesUseKnownCommands(t *testing.T) {
	e := newWriterEnv(t, "")
	for _, base := range e.bases() {
		spec := e.specFor(base, func(s *Settings) {
			for _, f := range Fields() {
				row, _ := findRow(f)
				if vals := rtValues(t, f, row.get(s)); len(vals) > 0 && f != FieldHome {
					setField(t, s, f, vals[0])
				}
			}
			s.Home = HomeFull
			s.Folders = append(s.Folders, Folder{Path: "${HOME}/Projects", Access: AccessReadOnly})
		})
		g, err := Generate(spec)
		if err != nil {
			// Changing everything on VS Code's chain needs more than 32 ignores.
			requireTag(t, err, tagIgnoreLimit, "Generate->generator.countIgnores.IgnoreLimitExceeded")
			continue
		}
		for _, l := range g.Lines {
			if l.Kind == LineBlank || l.Kind == LineComment || l.Kind == LineHeader {
				continue
			}
			cmd, _ := splitCommand(stripConditionals(l.Text))
			if !IsCommand(cmd) {
				t.Errorf("%s: %q is not a firejail command", baseName(base), l.Text)
			}
		}
	}
}
