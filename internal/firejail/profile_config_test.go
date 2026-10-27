package firejail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	want := Config{BrowserDisableU2F: true, BrowserAllowDRM: false, AllowTray: false}
	if got := DefaultConfig(); got != want {
		t.Errorf("DefaultConfig() = %+v, want %+v", got, want)
	}
}

func TestReadConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
		want    Config
	}{
		{name: "missing file gives the defaults", missing: true, want: DefaultConfig()},
		{name: "empty file", content: "", want: DefaultConfig()},
		{name: "only comments and blank lines", content: "# a\n\n   \n# browser-allow-drm yes\n", want: DefaultConfig()},
		{name: "drm yes", content: "browser-allow-drm yes\n", want: Config{BrowserDisableU2F: true, BrowserAllowDRM: true}},
		{name: "u2f no", content: "browser-disable-u2f no\n", want: Config{}},
		{name: "tray yes", content: "allow-tray yes\n", want: Config{BrowserDisableU2F: true, AllowTray: true}},
		{
			name:    "all three",
			content: "allow-tray yes\nbrowser-allow-drm yes\nbrowser-disable-u2f no\n",
			want:    Config{AllowTray: true, BrowserAllowDRM: true},
		},
		{
			name:    "explicit defaults",
			content: "allow-tray no\nbrowser-allow-drm no\nbrowser-disable-u2f yes\n",
			want:    DefaultConfig(),
		},
		{name: "last line wins", content: "browser-allow-drm yes\nbrowser-allow-drm no\n", want: DefaultConfig()},
		{name: "extra blanks and tabs", content: "  browser-allow-drm \t  yes\n", want: Config{BrowserDisableU2F: true, BrowserAllowDRM: true}},
		{name: "no final newline", content: "browser-allow-drm yes", want: Config{BrowserDisableU2F: true, BrowserAllowDRM: true}},
		{name: "indented comment is not a key", content: "  # allow-tray yes\n", want: DefaultConfig()},
		{
			name:    "other keys are ignored",
			content: "network no\nx11 no\nprivate-bin yes\nnetfilter-default /etc/x\nsome-new-key banana\n",
			want:    DefaultConfig(),
		},
		{name: "other key with a bad value is ignored", content: "network maybe\n", want: DefaultConfig()},
		{name: "a key that only starts like ours", content: "allow-tray-x yes\nbrowser-allow-drm-x yes\n", want: DefaultConfig()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "firejail.config")
			if !tt.missing {
				writeFile(t, path, tt.content)
			}
			got, err := ReadConfig(context.Background(), path)
			if err != nil {
				t.Fatalf("ReadConfig: %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadConfig = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReadConfigFixture(t *testing.T) {
	// firejail 0.9.80's own firejail.config has only comments, so it gives the defaults.
	got, err := ReadConfig(context.Background(), filepath.Join(fixtureDir(t), "firejail.config"))
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if got != DefaultConfig() {
		t.Errorf("ReadConfig = %+v, want the defaults", got)
	}
}

func TestReadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	badValue := filepath.Join(dir, "bad-value.config")
	writeFile(t, badValue, "browser-allow-drm\tmaybe\n")
	comment := filepath.Join(dir, "comment.config")
	writeFile(t, comment, "allow-tray yes # because\n")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name  string
		ctx   context.Context
		path  string
		route string
	}{
		{"value is not yes or no", context.Background(), badValue, "ReadConfig.ConfigLineInvalid"},
		{"a trailing comment is part of the value", context.Background(), comment, "ReadConfig.ConfigLineInvalid"},
		{"path is a folder", context.Background(), dir, "ReadConfig.ConfigReadFailed"},
		{"context is canceled", canceled, badValue, "ReadConfig.ConfigReadCanceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadConfig(tt.ctx, tt.path)
			if err == nil {
				t.Fatal("expected an error")
			}
			requireRoute(t, err, tt.route)
		})
	}
}

// writeFile writes content to path, making the parent folders.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixtureDir is the copy of firejail 0.9.80's /etc/firejail in testdata.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "etc-firejail"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
