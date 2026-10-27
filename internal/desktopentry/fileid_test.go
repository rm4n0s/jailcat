package desktopentry

import "testing"

func TestFileID(t *testing.T) {
	tests := []struct {
		name    string
		appsDir string
		path    string
		want    string
	}{
		{"file in the folder", "/usr/share/applications", "/usr/share/applications/firefox.desktop", "firefox.desktop"},
		{"one folder down", "/usr/share/applications", "/usr/share/applications/kde/foo.desktop", "kde-foo.desktop"},
		{"two folders down", "/usr/share/applications", "/usr/share/applications/a/b/c.desktop", "a-b-c.desktop"},
		{"folder name with a dash", "/usr/share/applications", "/usr/share/applications/x-y/z.desktop", "x-y-z.desktop"},
		{"reverse DNS name", "/usr/share/applications", "/usr/share/applications/org.mozilla.firefox.desktop", "org.mozilla.firefox.desktop"},
		{"trailing slash on the folder", "/usr/share/applications/", "/usr/share/applications/a.desktop", "a.desktop"},
		{"unclean paths", "/usr/share/./applications", "/usr/share/applications//kde/../kde/foo.desktop", "kde-foo.desktop"},
		{"user folder", "/home/u/.local/share/applications", "/home/u/.local/share/applications/firejail-code.desktop", "firejail-code.desktop"},
		{"relative paths", "testdata/applications", "testdata/applications/kde/foo.desktop", "kde-foo.desktop"},
		{"spaces", "/a b/applications", "/a b/applications/my app/x y.desktop", "my app-x y.desktop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FileID(tt.appsDir, tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("FileID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileIDErrors(t *testing.T) {
	tests := []struct {
		name    string
		appsDir string
		path    string
	}{
		{"other folder", "/usr/share/applications", "/usr/local/share/applications/a.desktop"},
		{"parent folder", "/usr/share/applications", "/usr/share/a.desktop"},
		{"sibling with the same prefix", "/usr/share/applications", "/usr/share/applications2/a.desktop"},
		{"the folder itself", "/usr/share/applications", "/usr/share/applications"},
		{"escapes with dot dot", "/usr/share/applications", "/usr/share/applications/../a.desktop"},
		{"absolute folder, relative path", "/usr/share/applications", "applications/a.desktop"},
		{"relative folder, absolute path", "applications", "/usr/share/applications/a.desktop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FileID(tt.appsDir, tt.path)
			if err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
			appErr := requireRoute(t, err, "FileID.NotInApplicationsDir")
			if appErr.Metadata["path"] != tt.path {
				t.Errorf("metadata = %v", appErr.Metadata)
			}
		})
	}
}
