package firejail

import (
	"slices"
	"testing"
)

func TestVSCodeConfigFolder(t *testing.T) {
	tests := []struct {
		binary string
		want   string
		ok     bool
	}{
		{"/usr/bin/code", "Code", true},
		{"code", "Code", true},
		{"/usr/bin/code-insiders", "Code - Insiders", true},
		{"/usr/bin/code-oss", "Code - OSS", true},
		{"/usr/bin/codium", "VSCodium", true},
		{"/opt/vscodium/bin/codium-insiders", "VSCodium - Insiders", true},
		{"/usr/bin/vscodium", "VSCodium", true},
		{"/home/u/bin/code", "Code", true},
		{"/usr/bin/codec", "", false},
		{"/usr/bin/code2", "", false},
		{"/usr/bin/Code", "", false},
		{"/usr/bin/code/bin", "", false},
		{"/usr/bin/vim", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.binary, func(t *testing.T) {
			got, ok := VSCodeConfigFolder(tt.binary)
			if got != tt.want || ok != tt.ok {
				t.Errorf("VSCodeConfigFolder(%q) = %q, %v; want %q, %v", tt.binary, got, ok, tt.want, tt.ok)
			}
			if IsVSCode(tt.binary) != tt.ok {
				t.Errorf("IsVSCode(%q) = %v", tt.binary, !tt.ok)
			}
		})
	}
}

func TestNewVSCodeSpec(t *testing.T) {
	got := NewVSCodeSpec("/usr/bin/code-oss", "/d/jailcat/appdata", "/c", "firejail-code-oss")
	want := &VSCodeSpec{DataDir: "/d/jailcat/appdata/firejail-code-oss", ConfigDir: "/c/Code - OSS"}
	if got == nil || *got != *want {
		t.Errorf("NewVSCodeSpec = %+v, want %+v", got, want)
	}
	if NewVSCodeSpec("/usr/bin/vim", "/d", "/c", "firejail-vim") != nil {
		t.Errorf("a program that is not VS Code has a spec")
	}
}

func TestWithAppDefaults(t *testing.T) {
	tests := []struct {
		binary string
		want   bool
	}{{"/usr/bin/code", true}, {"/usr/bin/codium", true}, {"/usr/bin/firefox", false}}
	for _, tt := range tests {
		t.Run(tt.binary, func(t *testing.T) {
			in := BaseNone()
			out := WithAppDefaults(in, tt.binary)
			s := out.Settings
			if s.VSCodeOwnData != tt.want || s.VSCodeShareSettings != tt.want || s.VSCodeShellFiles != tt.want {
				t.Errorf("settings = %v %v %v, want all %v", s.VSCodeOwnData, s.VSCodeShareSettings, s.VSCodeShellFiles, tt.want)
			}
			if in.Settings.VSCodeOwnData {
				t.Errorf("the input Base was changed")
			}
		})
	}
}

func TestVSCodeSettingsHelpers(t *testing.T) {
	all := Settings{VSCodeOwnData: true, VSCodeShareSettings: true, VSCodeShellFiles: true}
	tests := []struct {
		name                   string
		edit                   func(s *Settings)
		data, links, shellFile bool
	}{
		{"full", func(s *Settings) { s.Home = HomeFull }, true, true, false},
		{"chosen", func(s *Settings) { s.Home = HomeChosen }, true, true, true},
		{"empty", func(s *Settings) { s.Home = HomeEmpty }, false, false, false},
		{"private", func(s *Settings) { s.Home = HomePrivate }, false, false, false},
		{"full without own data", func(s *Settings) { s.Home = HomeFull; s.VSCodeOwnData = false }, false, false, false},
		{"full without sharing", func(s *Settings) { s.Home = HomeFull; s.VSCodeShareSettings = false }, true, false, false},
		{"chosen without shell files", func(s *Settings) { s.Home = HomeChosen; s.VSCodeShellFiles = false }, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := all
			tt.edit(&s)
			if s.VSCodeDataOnDisk() != tt.data || s.VSCodeLinksSettings() != tt.links || s.VSCodeShowsShellFiles() != tt.shellFile {
				t.Errorf("data %v links %v shell %v; want %v %v %v", s.VSCodeDataOnDisk(), s.VSCodeLinksSettings(),
					s.VSCodeShowsShellFiles(), tt.data, tt.links, tt.shellFile)
			}
		})
	}
}

func TestVSCodeSharedLists(t *testing.T) {
	if got := VSCodeSharedNames(); !slices.Equal(got, []string{"settings.json", "keybindings.json", "snippets"}) {
		t.Errorf("VSCodeSharedNames = %v", got)
	}
	if got := VSCodeShellFiles(); len(got) != 14 || got[0] != ".bashrc" || got[13] != ".config/starship.toml" {
		t.Errorf("VSCodeShellFiles = %v", got)
	}
	// The lists are fresh slices.
	VSCodeSharedNames()[0] = "x"
	if VSCodeSharedNames()[0] != "settings.json" {
		t.Errorf("VSCodeSharedNames shares its slice")
	}
}

func TestVSCodeFieldsDiffApply(t *testing.T) {
	fields := []struct {
		field Field
		get   func(s Settings) bool
	}{
		{FieldVSCodeOwnData, func(s Settings) bool { return s.VSCodeOwnData }},
		{FieldVSCodeShareSettings, func(s Settings) bool { return s.VSCodeShareSettings }},
		{FieldVSCodeShellFiles, func(s Settings) bool { return s.VSCodeShellFiles }},
	}
	for _, tt := range fields {
		t.Run(string(tt.field), func(t *testing.T) {
			if tt.field.Tab() != TabFiles {
				t.Errorf("Tab = %q", tt.field.Tab())
			}
			base := WithAppDefaults(BaseNone(), "/usr/bin/code").Settings
			edited := base.Clone()
			row, _ := findRow(tt.field)
			row.set(&edited, false)

			o := Diff(base, edited)
			if got := o.ChangedFields(); !slices.Equal(got, []Field{tt.field}) {
				t.Fatalf("ChangedFields = %v", got)
			}
			if tt.get(o.Apply(base)) {
				t.Errorf("Apply did not turn %s off", tt.field)
			}
			// Through the stored JSON.
			data, err := o.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			var back Overrides
			if err := back.UnmarshalJSON(data); err != nil {
				t.Fatal(err)
			}
			if tt.get(back.Apply(base)) {
				t.Errorf("the JSON round trip lost %s", tt.field)
			}
			// Without an override a build made before the switches (no override at all)
			// has the base value: on for VS Code.
			if !tt.get((Overrides{}).Apply(base)) {
				t.Errorf("an old build does not get %s on", tt.field)
			}
			if tt.field.Equal(base, edited) {
				t.Errorf("Equal says the values are the same")
			}
		})
	}
}
