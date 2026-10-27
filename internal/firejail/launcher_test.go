package firejail

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	jerrors "github.com/rm4n0s/errors"
)

// launcherCases are the golden launchers of testdata/launchers.
var launcherCases = []struct {
	file string
	spec LauncherSpec
}{
	{"code.sh", LauncherSpec{
		BuildID:  7,
		Firejail: "/usr/bin/firejail",
		Profile:  "/home/u/.local/share/jailcat/profiles/firejail-code.profile",
		Binary:   "/usr/bin/code",
	}},
	{"code-extra.sh", LauncherSpec{
		BuildID:   7,
		Firejail:  "/usr/bin/firejail",
		Profile:   "/home/u/.local/share/jailcat/profiles/firejail-code.profile",
		Binary:    "/usr/bin/code",
		ExtraArgs: []string{"--ozone-platform=wayland", "extra"},
	}},
	{"spaces-quotes.sh", LauncherSpec{
		BuildID:  12,
		Firejail: "/opt/fire jail/it's/firejail",
		Profile:  "/home/my user/.local/share/jailcat/profiles/firejail-my-app.profile",
		Binary:   "/opt/My \"App\"/bin/it's here",
	}},
	{"special-args.sh", LauncherSpec{
		BuildID:  3,
		Firejail: "/usr/bin/firejail",
		Profile:  "/home/u/profiles/firejail-x.profile",
		Binary:   "/usr/bin/x",
		ExtraArgs: []string{
			"", "a b", "it's", "''", `"q"`, "$HOME", "${X}", "$(id)", "`id`", `a\b`, "a!b",
			"*", "~", "a\nb", "a\tb", "ünï 日本", "%F", ";", "&&", "|", ">", "#",
		},
	}},
}

func goldenLauncher(t *testing.T, name, text string) {
	t.Helper()
	path := filepath.Join("testdata", "launchers", name)
	if *updateGoldens {
		writeFile(t, path, text)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update-goldens to make it)", err)
	}
	if string(want) != text {
		t.Errorf("%s differs from the golden file.\n--- want\n%s--- got\n%s", name, want, text)
	}
}

func launcherText(t *testing.T, spec LauncherSpec) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteLauncher(&b, spec); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWriteLauncherGolden(t *testing.T) {
	for _, tt := range launcherCases {
		t.Run(tt.file, func(t *testing.T) {
			goldenLauncher(t, tt.file, launcherText(t, tt.spec))
		})
	}
}

func TestWriteLauncherHeader(t *testing.T) {
	got := launcherText(t, launcherCases[0].spec)
	lines := strings.Split(got, "\n")
	if lines[0] != "#!/bin/sh" || lines[1] != HeaderLine || lines[2] != BuildLine(7) {
		t.Errorf("unexpected header: %q", lines[:3])
	}
	if !strings.HasSuffix(got, " \"$@\"\n") {
		t.Errorf("the launcher does not end with \"$@\" and a newline: %q", got)
	}
}

func TestLauncherShellSyntax(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not installed")
	}
	files, err := filepath.Glob(filepath.Join("testdata", "launchers", "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < len(launcherCases) {
		t.Fatalf("found %d golden launchers, want %d", len(files), len(launcherCases))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			out, err := exec.CommandContext(context.Background(), sh, "-n", f).CombinedOutput()
			if err != nil {
				t.Errorf("sh -n failed: %v\n%s", err, out)
			}
		})
	}
}

// fakeScript writes an executable shell script into dir and returns its path.
func fakeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLauncherPassesArgumentsUnchanged(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not installed")
	}
	dir := t.TempDir()
	fake := fakeScript(t, dir, "firejail", `printf '%s\0' "$@"`+"\n")
	extra := []string{"", "--flag=a b", "it's", "$HOME", "`id`", "a\nb"}
	spec := LauncherSpec{
		BuildID:   1,
		Firejail:  fake,
		Profile:   filepath.Join(dir, "my profile's.profile"),
		Binary:    "/usr/bin/some app",
		ExtraArgs: extra,
	}
	launcher := filepath.Join(dir, "launcher")
	if err := os.WriteFile(launcher, []byte(launcherText(t, spec)), 0o755); err != nil {
		t.Fatal(err)
	}

	user := []string{"a b", "it's", `"dq"`, "$HOME", "*", "line1\nline2", "", "~", "`id`", `\n`, "-x", "--", "ünï"}
	cmd := exec.CommandContext(context.Background(), sh, append([]string{launcher}, user...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running the launcher: %v\n%s", err, stderr.String())
	}

	want := append([]string{"--profile=" + spec.Profile, "--", spec.Binary}, extra...)
	want = append(want, user...)
	got := strings.Split(strings.TrimSuffix(stdout.String(), "\x00"), "\x00")
	if len(got) != len(want) {
		t.Fatalf("got %d arguments, want %d:\n%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argument %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLauncherWithoutUserArguments(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not installed")
	}
	dir := t.TempDir()
	fake := fakeScript(t, dir, "firejail", `printf '%s\0' "$@"`+"\n")
	spec := LauncherSpec{BuildID: 1, Firejail: fake, Profile: "/p.profile", Binary: "/bin/x"}
	launcher := filepath.Join(dir, "launcher")
	if err := os.WriteFile(launcher, []byte(launcherText(t, spec)), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(context.Background(), sh, launcher).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "--profile=/p.profile\x00--\x00/bin/x\x00"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestWriteLauncherErrors(t *testing.T) {
	good := LauncherSpec{BuildID: 1, Firejail: "/usr/bin/firejail", Profile: "/p/x.profile", Binary: "/usr/bin/x"}
	with := func(edit func(s *LauncherSpec)) LauncherSpec {
		s := good
		edit(&s)
		return s
	}
	tests := []struct {
		name  string
		spec  LauncherSpec
		route string
	}{
		{"firejail relative", with(func(s *LauncherSpec) { s.Firejail = "firejail" }), "WriteLauncher.LauncherFirejailInvalid"},
		{"firejail empty", with(func(s *LauncherSpec) { s.Firejail = "" }), "WriteLauncher.LauncherFirejailInvalid"},
		{"firejail unclean", with(func(s *LauncherSpec) { s.Firejail = "/usr/bin/../bin/firejail" }), "WriteLauncher.LauncherFirejailInvalid"},
		{"profile relative", with(func(s *LauncherSpec) { s.Profile = "x.profile" }), "WriteLauncher.LauncherProfileInvalid"},
		{"profile trailing slash", with(func(s *LauncherSpec) { s.Profile = "/p/x.profile/" }), "WriteLauncher.LauncherProfileInvalid"},
		{"binary relative", with(func(s *LauncherSpec) { s.Binary = "code" }), "WriteLauncher.LauncherBinaryInvalid"},
		{"binary double slash", with(func(s *LauncherSpec) { s.Binary = "/usr//bin/x" }), "WriteLauncher.LauncherBinaryInvalid"},
		{"NUL in binary", with(func(s *LauncherSpec) { s.Binary = "/usr/bin/x\x00y" }), "WriteLauncher->ShellQuote.ShellWordHasNul"},
		{"NUL in extra argument", with(func(s *LauncherSpec) { s.ExtraArgs = []string{"a\x00b"} }), "WriteLauncher->ShellQuote.ShellWordHasNul"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			err := WriteLauncher(&b, tt.spec)
			appErr, ok := jerrors.FromError(err)
			if !ok {
				t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
			}
			if !appErr.HasRoute(tt.route) {
				t.Errorf("unexpected failure path: %s", appErr.Route())
			}
			if b.Len() != 0 {
				t.Errorf("a failed WriteLauncher wrote %q", b.String())
			}
		})
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriteLauncherWriteFailed(t *testing.T) {
	cause := errors.New("disk full")
	err := WriteLauncher(failingWriter{cause}, launcherCases[0].spec)
	appErr, ok := jerrors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if !appErr.HasRoute("WriteLauncher.LauncherWriteFailed") {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
	if !jerrors.Is(err, cause) {
		t.Error("the cause of the write error is lost")
	}
}
