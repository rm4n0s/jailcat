package firejail

import (
	"slices"
	"strings"
	"testing"
)

const (
	testDataDir   = testHome + "/.local/share/jailcat/appdata/firejail-test"
	testConfigDir = testHome + "/.config/Code"
)

// vscodeSpec is goldenSpec for a build of VS Code: the base values are the defaults of
// a VS Code app (the switches on), edit changes what the user sees, and the spec names
// the data and config folders.
func vscodeSpec(t *testing.T, base string, edit func(s *Settings), extras ...string) ProfileSpec {
	t.Helper()
	e := newWriterEnv(t, testHome)
	spec := e.spec(base, nil, extras...)
	spec.Base = WithAppDefaults(spec.Base, "/usr/bin/code")
	spec.Base.Settings.SandboxName = "firejail-test"
	edited := spec.Base.Settings.Clone()
	if edit != nil {
		edit(&edited)
	}
	spec.Overrides = Diff(spec.Base.Settings, edited)
	if base != "" {
		spec.BaseProfile = "/etc/firejail/" + base
	}
	spec.VSCode = &VSCodeSpec{DataDir: testDataDir, ConfigDir: testConfigDir}
	return spec
}

func TestGenerateVSCodeGolden(t *testing.T) {
	const privateDir = testHome + "/.local/share/jailcat/homes/firejail-test"
	tests := []struct {
		file string
		edit func(s *Settings)
	}{
		{"code-vscode-chosen.profile", func(s *Settings) {
			s.Home = HomeChosen
			s.Folders = append(s.Folders, Folder{Path: testHome + "/Projects", Access: AccessReadWrite})
		}},
		{"code-vscode-full.profile", func(s *Settings) { s.Home = HomeFull }},
		{"code-vscode-empty.profile", func(s *Settings) { s.Home = HomeEmpty }},
		{"code-vscode-private.profile", func(s *Settings) { s.Home = HomePrivate; s.PrivateDir = privateDir }},
		{"code-vscode-chosen-off.profile", func(s *Settings) {
			s.Home = HomeChosen
			s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles = false, false, false
			s.Folders = append(s.Folders, Folder{Path: testHome + "/Projects", Access: AccessReadWrite})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			g := mustGenerate(t, vscodeSpec(t, "code.profile", tt.edit))
			golden(t, tt.file, g.String())
		})
	}
}

func TestGenerateVSCodeLines(t *testing.T) {
	has := func(g *Generated, text string) bool { return indexOf(g, text) >= 0 }
	dataLines := []string{"noblacklist " + "${HOME}/.local/share/jailcat/appdata/firejail-test"}
	shareLines := []string{
		"noblacklist ${HOME}/.config/Code/User/settings.json",
		"noblacklist ${HOME}/.config/Code/User/keybindings.json",
		"noblacklist ${HOME}/.config/Code/User/snippets",
	}
	shellRO := []string{"noblacklist ${HOME}/.bashrc", "read-only ${HOME}/.bashrc", "read-only ${HOME}/.config/fish",
		"read-only ${HOME}/.local/share/bash-completion", "read-only ${HOME}/.config/starship.toml"}
	tests := []struct {
		name string
		home HomeMode
		// present and absent are line texts.
		present, absent []string
	}{
		{"full", HomeFull, slices.Concat(dataLines, shareLines),
			[]string{"tab", "whitelist ${HOME}/.local/share/jailcat/appdata/firejail-test", "read-only ${HOME}/.bashrc"}},
		{"chosen", HomeChosen, slices.Concat(dataLines, shareLines, shellRO, []string{
			"tab", "whitelist ${HOME}/.local/share/jailcat/appdata/firejail-test",
			"whitelist ${HOME}/.config/Code/User/settings.json", "whitelist ${HOME}/.bashrc", "whitelist ${HOME}/.zshrc"}), nil},
		{"empty", HomeEmpty, nil, slices.Concat(dataLines, shareLines, shellRO, []string{"tab"})},
		{"private", HomePrivate, nil, slices.Concat(dataLines, shareLines, shellRO, []string{"tab"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mustGenerate(t, vscodeSpec(t, "code.profile", func(s *Settings) {
				s.Home = tt.home
				if tt.home == HomePrivate {
					s.PrivateDir = testHome + "/.local/share/jailcat/homes/firejail-test"
				}
				s.Folders = append(s.Folders, Folder{Path: testHome + "/Projects", Access: AccessReadWrite})
				if tt.home == HomeEmpty || tt.home == HomePrivate {
					s.Folders = nil
				}
			}))
			for _, l := range tt.present {
				if !has(g, l) {
					t.Errorf("missing %q\n%s", l, g)
				}
			}
			for _, l := range tt.absent {
				if has(g, l) {
					t.Errorf("unexpected %q\n%s", l, g)
				}
			}
		})
	}
}

// TestGenerateVSCodeAttribution checks the Preview can tell which setting wrote a line.
func TestGenerateVSCodeAttribution(t *testing.T) {
	g := mustGenerate(t, vscodeSpec(t, "code.profile", func(s *Settings) { s.Home = HomeChosen }))
	want := map[string]Field{
		"whitelist ${HOME}/.local/share/jailcat/appdata/firejail-test": FieldVSCodeOwnData,
		"noblacklist ${HOME}/.config/Code/User/keybindings.json":       FieldVSCodeShareSettings,
		"tab":                                FieldVSCodeShellFiles,
		"read-only ${HOME}/.zshrc":           FieldVSCodeShellFiles,
		"whitelist ${HOME}/.config/fish":     FieldVSCodeShellFiles,
		"noblacklist ${HOME}/.inputrc":       FieldVSCodeShellFiles,
		"read-only ${HOME}/.bash_profile":    FieldVSCodeShellFiles,
		"whitelist ${HOME}/.bash_aliases":    FieldVSCodeShellFiles,
		"noblacklist ${HOME}/.zlogin":        FieldVSCodeShellFiles,
		"read-only ${HOME}/.bash_login":      FieldVSCodeShellFiles,
		"whitelist ${HOME}/.zshenv":          FieldVSCodeShellFiles,
		"noblacklist ${HOME}/.zprofile":      FieldVSCodeShellFiles,
		"whitelist ${HOME}/.bash_completion": FieldVSCodeShellFiles,
		"read-only ${HOME}/.profile":         FieldVSCodeShellFiles,
	}
	for text, field := range want {
		i := indexOf(g, text)
		if i < 0 {
			t.Errorf("missing %q", text)
			continue
		}
		if l := g.Lines[i]; l.Field != field || l.Kind != LineSetting || l.Folder != "" {
			t.Errorf("%q = {field %q, kind %q, folder %q}, want field %q", text, l.Field, l.Kind, l.Folder, field)
		}
	}
}

// TestGenerateVSCodeNeedsTheSpec checks that a build that is not a VS Code app writes
// none of the lines, whatever the switches say.
func TestGenerateVSCodeNeedsTheSpec(t *testing.T) {
	spec := vscodeSpec(t, "code.profile", func(s *Settings) { s.Home = HomeChosen })
	spec.VSCode = nil
	g := mustGenerate(t, spec)
	for _, l := range g.Lines {
		if strings.Contains(l.Text, "appdata") || l.Text == "tab" || strings.Contains(l.Text, ".bashrc") {
			t.Errorf("unexpected line %q", l.Text)
		}
	}
}

// TestGenerateVSCodeReadsBack checks firejail reads the lines as written: the generated
// profile of every home choice reads back as the settings.
func TestGenerateVSCodeReadsBack(t *testing.T) {
	e := newWriterEnv(t, testHome)
	for _, home := range []HomeMode{HomeFull, HomeChosen} {
		g := mustGenerate(t, vscodeSpec(t, "code.profile", func(s *Settings) { s.Home = home }))
		prof, err := e.read(e.write(g))
		if err != nil {
			t.Fatalf("%s: %v", home, err)
		}
		var cmds []string
		for _, l := range prof.Lines {
			cmds = append(cmds, l.Command)
		}
		for _, c := range []string{"noblacklist", "whitelist", "read-only", "tab"} {
			if home == HomeFull && (c == "whitelist" || c == "tab" || c == "read-only") {
				continue
			}
			if !slices.Contains(cmds, c) {
				t.Errorf("%s: the profile reads back without a %q line", home, c)
			}
		}
	}
}

func TestGenerateVSCodeErrors(t *testing.T) {
	const gen = "Generate->generator."
	tests := []struct {
		name  string
		vs    *VSCodeSpec
		tag   string
		route string
	}{
		{"relative data folder", &VSCodeSpec{DataDir: "appdata/x", ConfigDir: testConfigDir},
			tagVSCodeSpecInvalid, gen + "rows->generator.vscode.VSCodeSpecInvalid"},
		{"relative config folder", &VSCodeSpec{DataDir: testDataDir, ConfigDir: ".config/Code"},
			tagVSCodeSpecInvalid, gen + "rows->generator.vscode.VSCodeSpecInvalid"},
		{"unclean data folder", &VSCodeSpec{DataDir: testDataDir + "/../x", ConfigDir: testConfigDir},
			tagVSCodeSpecInvalid, gen + "rows->generator.vscode.VSCodeSpecInvalid"},
		{"data folder with two blanks", &VSCodeSpec{DataDir: "/home/jailcat-test/a  b", ConfigDir: testConfigDir},
			tagProfileValueInvalid, gen + "rows->generator.vscode->generator.vscodeRow->lineMaker.path->profileValue.ProfileValueInvalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := vscodeSpec(t, "code.profile", func(s *Settings) { s.Home = HomeChosen })
			spec.VSCode = tt.vs
			_, err := Generate(spec)
			if err == nil {
				t.Fatal("Generate succeeded")
			}
			requireTag(t, err, tt.tag, tt.route)
		})
	}
}
