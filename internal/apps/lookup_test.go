package apps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rm4n0s/jailcat/internal/xdg"
)

func TestLookPath(t *testing.T) {
	s := newFakeSystem(t)
	tests := []struct {
		name string
		path []string
		prog string
		want string // relative to the fake root; "" when not found
	}{
		{"first folder wins", nil, "dup", "bin1/dup"},
		{"later folder", nil, "plainbin", "bin3/plainbin"},
		{"symlink to an executable", nil, "dupalias", "bin3/dupalias"},
		{"firecfg link is skipped", nil, "firecfgapp", "bin2/firecfgapp"},
		{"firecfg link skipped, real binary last", nil, "sandboxme", "bin3/sandboxme"},
		{"firecfg link only", nil, "firecfgonly", ""},
		{"link to a firecfg link", nil, "firecfgchain", ""},
		{"file without execute bits", nil, "notexec", ""},
		{"folder", nil, "adir", ""},
		{"link to nothing", nil, "brokenlink", ""},
		{"name that is not there", nil, "nope", ""},
		{"name with a slash", nil, "bin1/prog", ""},
		{"empty name", nil, "", ""},
		{"dot dot", nil, "..", ""},
		{"order of PATH counts", []string{"bin2", "bin1"}, "dup", "bin2/dup"},
		{"folder that does not exist", []string{"bin4", "bin1"}, "prog", "bin1/prog"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := s.dirs
			if tt.path != nil {
				dirs.Path = nil
				for _, p := range tt.path {
					dirs.Path = append(dirs.Path, s.path(p))
				}
			}
			got, ok := lookPath(tt.prog, dirs)
			want := ""
			if tt.want != "" {
				want = s.path(tt.want)
			}
			if got != want || ok != (want != "") {
				t.Errorf("lookPath(%q) = %q, %v; want %q", tt.prog, got, ok, want)
			}
		})
	}
}

func TestLookPathSkipsRelativeFolders(t *testing.T) {
	s := newFakeSystem(t)
	// A relative $PATH entry is skipped even when it would resolve from the working directory.
	t.Chdir(s.path("bin1"))
	dirs := xdg.Dirs{Path: []string{".", "", "../bin1"}}
	if got, ok := lookPath("prog", dirs); ok {
		t.Errorf("lookPath found %q in a relative folder", got)
	}
}

func TestIsFirecfgLink(t *testing.T) {
	s := newFakeSystem(t)
	other := s.path("bin1/prog")
	if err := os.Symlink(other, s.path("bin3/toprog")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"link to firejail", "bin1/firecfgapp", true},
		{"link to a link to firejail", "bin1/firecfgchain", true},
		{"link to another program", "bin3/toprog", false},
		{"the firejail file itself", "fakefj/firejail", false},
		{"plain file", "bin1/prog", false},
		{"link to nothing", "bin3/brokenlink", false},
		{"missing", "bin3/gone", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFirecfgLink(s.path(tt.path)); got != tt.want {
				t.Errorf("isFirecfgLink(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestEvalOrPlain(t *testing.T) {
	s := newFakeSystem(t)
	tests := []struct {
		name string
		path string
		want string
	}{
		{"resolves a link", s.path("bin3/proglink"), s.path("bin1/prog")},
		{"plain file", s.path("bin1/prog"), s.path("bin1/prog")},
		{"missing file stays as it is, cleaned", s.path("bin1/../bin1/gone"), s.path("bin1/gone")},
		{"link to nothing stays as it is", s.path("bin3/brokenlink"), s.path("bin3/brokenlink")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evalOrPlain(tt.path); filepath.Clean(got) != tt.want {
				t.Errorf("evalOrPlain = %q, want %q", got, tt.want)
			}
		})
	}
}
