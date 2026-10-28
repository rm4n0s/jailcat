package xdg

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rm4n0s/errors"
)

// requireRoute fails the test unless err is an *errors.Error with the given route.
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

func TestResolve(t *testing.T) {
	const home = "/home/u"
	tests := []struct {
		name string
		vars map[string]string
		want Dirs
	}{
		{
			name: "nothing set uses the defaults",
			vars: nil,
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/usr/local/share", "/usr/share"}, Path: nil,
				Data: "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "every variable set",
			vars: map[string]string{
				"XDG_DATA_HOME": "/x/data", "XDG_CONFIG_HOME": "/x/config", "XDG_CACHE_HOME": "/x/cache",
				"XDG_BIN_HOME": "/x/bin", "XDG_DATA_DIRS": "/a/share:/b/share", "PATH": "/x/bin:/usr/bin::/bin",
			},
			want: Dirs{
				Home: home, DataHome: "/x/data", ConfigHome: "/x/config",
				DataDirs: []string{"/a/share", "/b/share"}, Path: []string{"/x/bin", "/usr/bin", "/bin"},
				Data: "/x/data/jailcat", Database: "/x/data/jailcat/jailcat.db",
				Profiles: "/x/data/jailcat/profiles", Imported: "/x/data/jailcat/profiles/imported",
				Homes: "/x/data/jailcat/homes", AppData: "/x/data/jailcat/appdata", Launchers: "/x/data/jailcat/launchers",
				Cache: "/x/cache/jailcat", Bin: "/x/bin",
				Applications: "/x/data/applications", Icons: "/x/data/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail",
				// firejail ignores XDG_CONFIG_HOME
				FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "empty values use the defaults",
			vars: map[string]string{
				"XDG_DATA_HOME": "", "XDG_CONFIG_HOME": "", "XDG_CACHE_HOME": "", "XDG_BIN_HOME": "",
				"XDG_DATA_DIRS": "", "PATH": "",
			},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/usr/local/share", "/usr/share"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "relative values are ignored",
			vars: map[string]string{
				"XDG_DATA_HOME": "data", "XDG_CONFIG_HOME": "./config", "XDG_CACHE_HOME": "../cache",
				"XDG_BIN_HOME": "bin",
			},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/usr/local/share", "/usr/share"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "XDG_BIN_HOME only",
			vars: map[string]string{"XDG_BIN_HOME": "/opt/bin/"},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/usr/local/share", "/usr/share"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/opt/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "XDG_DATA_DIRS relative entries are dropped",
			vars: map[string]string{"XDG_DATA_DIRS": "rel/share:/abs/share::./x:/other/"},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/abs/share", "/other"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "XDG_DATA_DIRS all relative uses the default list",
			vars: map[string]string{"XDG_DATA_DIRS": "rel/share:other"},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/home/u/.config",
				DataDirs: []string{"/usr/local/share", "/usr/share"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
		{
			name: "XDG_CONFIG_HOME moves ConfigHome but not the firejail user folder",
			vars: map[string]string{"XDG_CONFIG_HOME": "/cfg"},
			want: Dirs{
				Home: home, DataHome: "/home/u/.local/share", ConfigHome: "/cfg",
				DataDirs: []string{"/usr/local/share", "/usr/share"},
				Data:     "/home/u/.local/share/jailcat", Database: "/home/u/.local/share/jailcat/jailcat.db",
				Profiles: "/home/u/.local/share/jailcat/profiles", Imported: "/home/u/.local/share/jailcat/profiles/imported",
				Homes: "/home/u/.local/share/jailcat/homes", AppData: "/home/u/.local/share/jailcat/appdata", Launchers: "/home/u/.local/share/jailcat/launchers",
				Cache: "/home/u/.cache/jailcat", Bin: "/home/u/.local/bin",
				Applications:   "/home/u/.local/share/applications",
				Icons:          "/home/u/.local/share/icons/hicolor/scalable/apps",
				FirejailSystem: "/etc/firejail", FirejailUser: "/home/u/.config/firejail",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(NewEnv(home, tt.vars))
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Resolve =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestResolveHomeNotAbsolute(t *testing.T) {
	for _, home := range []string{"", "home/u", ".", "~"} {
		t.Run(home, func(t *testing.T) {
			_, err := Resolve(NewEnv(home, nil))
			requireRoute(t, err, "Resolve.HomeNotAbsolute")
		})
	}
}

func TestResolveCleansHome(t *testing.T) {
	got, err := Resolve(NewEnv("/home/u/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.Home != "/home/u" || got.Bin != "/home/u/.local/bin" {
		t.Errorf("home not cleaned: %q, bin %q", got.Home, got.Bin)
	}
}

func TestZeroEnvHasNoVariables(t *testing.T) {
	var env Env
	if _, ok := env.Lookup("PATH"); ok {
		t.Error("zero Env must have no variables")
	}
	_, err := Resolve(env)
	requireRoute(t, err, "Resolve.HomeNotAbsolute")
}

func TestNewEnvFunc(t *testing.T) {
	env := NewEnvFunc("/home/u", func(name string) (string, bool) { return "v-" + name, true })
	if v, ok := env.Lookup("A"); !ok || v != "v-A" {
		t.Errorf("Lookup = %q, %v", v, ok)
	}
	if env.Home() != "/home/u" {
		t.Errorf("Home = %q", env.Home())
	}
}

func TestFromOS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_BIN_HOME", "/some/bin")
	env, err := FromOS()
	if err != nil {
		t.Fatal(err)
	}
	if env.Home() != home {
		t.Errorf("Home = %q, want %q", env.Home(), home)
	}
	if v, ok := env.Lookup("XDG_BIN_HOME"); !ok || v != "/some/bin" {
		t.Errorf("Lookup = %q, %v", v, ok)
	}
}

func TestFromOSNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := FromOS()
	requireRoute(t, err, "FromOS.HomeLookupFailed")
}

// mkdirs creates the folders (names relative to home) under home.
func mkdirs(t *testing.T, home string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(home, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeUserDirs(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user-dirs.dirs"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUserDirs(t *testing.T) {
	tests := []struct {
		name     string
		userDirs string // content of user-dirs.dirs; "-" means no file
		folders  []string
		want     map[string]string // macro -> relative
	}{
		{
			name: "user-dirs.dirs with existing folders",
			userDirs: `# comment
XDG_DESKTOP_DIR="$HOME/Skrivbord"
XDG_DOWNLOAD_DIR="$HOME/Hent"
XDG_MUSIC_DIR="$HOME/Musikk"
XDG_PICTURES_DIR="$HOME/Bilder2"
XDG_VIDEOS_DIR="$HOME/Filmer"
XDG_DOCUMENTS_DIR="$HOME/Dok"
`,
			folders: []string{"Skrivbord", "Hent", "Musikk", "Bilder2", "Filmer", "Dok"},
			want: map[string]string{
				MacroDesktop: "Skrivbord", MacroDownloads: "Hent", MacroMusic: "Musikk",
				MacroPictures: "Bilder2", MacroVideos: "Filmer", MacroDocuments: "Dok",
			},
		},
		{
			name:     "comments, blanks and indentation",
			userDirs: "\n   \n# XDG_DOWNLOAD_DIR=\"$HOME/Nope\"\n\t\n  \tXDG_DOWNLOAD_DIR=\"$HOME/Dl\"\n",
			folders:  []string{"Dl", "Nope"},
			want:     map[string]string{MacroDownloads: "Dl"},
		},
		{
			name:     "folder missing falls back to the translations",
			userDirs: `XDG_DOWNLOAD_DIR="$HOME/Gone"` + "\n",
			folders:  []string{"Downloads"},
			want:     map[string]string{MacroDownloads: "Downloads"},
		},
		{
			name:     "absolute value does not match firejail's prefix and falls back",
			userDirs: `XDG_DOWNLOAD_DIR="/mnt/data/dl"` + "\n",
			folders:  []string{"Загрузки"},
			want:     map[string]string{MacroDownloads: "Загрузки"},
		},
		{
			name:     "plain $HOME with no slash falls back",
			userDirs: `XDG_DOWNLOAD_DIR="$HOME"` + "\n",
			folders:  []string{"Downloads"},
			want:     map[string]string{MacroDownloads: "Downloads"},
		},
		{
			name:     "empty value is unresolved even if translations exist",
			userDirs: `XDG_DOWNLOAD_DIR="$HOME/"` + "\n",
			folders:  []string{"Downloads"},
			want:     map[string]string{MacroDownloads: "Downloads"},
		},
		{
			name:     "no closing quote keeps reading",
			userDirs: "XDG_DOWNLOAD_DIR=\"$HOME/Broken\nXDG_DOWNLOAD_DIR=\"$HOME/Good\"\n",
			folders:  []string{"Good"},
			want:     map[string]string{MacroDownloads: "Good"},
		},
		{
			name:     "the first matching line decides",
			userDirs: "XDG_DOWNLOAD_DIR=\"$HOME/First\"\nXDG_DOWNLOAD_DIR=\"$HOME/Second\"\n",
			folders:  []string{"Second"},
			want:     map[string]string{},
		},
		{
			name:     "nested value",
			userDirs: `XDG_DOCUMENTS_DIR="$HOME/Work/Docs"` + "\n",
			folders:  []string{"Work/Docs"},
			want:     map[string]string{MacroDocuments: "Work/Docs"},
		},
		{
			name:     "no user-dirs.dirs, only the French folder",
			userDirs: "-",
			folders:  []string{"Téléchargement"},
			want:     map[string]string{MacroDownloads: "Téléchargement"},
		},
		{
			name:     "translation order: the first one that exists wins",
			userDirs: "-",
			folders:  []string{"Descargas", "Scaricati"},
			want:     map[string]string{MacroDownloads: "Scaricati"},
		},
		{
			name:     "all translations",
			userDirs: "-",
			folders: []string{
				"Musik", "Vídeos", "Bilder", "Área de trabalho", "Dokumente",
			},
			want: map[string]string{
				MacroMusic: "Musik", MacroVideos: "Vídeos", MacroPictures: "Bilder",
				MacroDesktop: "Área de trabalho", MacroDocuments: "Dokumente",
			},
		},
		{
			name:     "Russian desktop",
			userDirs: "-",
			folders:  []string{"Рабочий стол"},
			want:     map[string]string{MacroDesktop: "Рабочий стол"},
		},
		{
			name:     "nothing exists",
			userDirs: "-",
			want:     map[string]string{},
		},
		{
			name:     "user-dirs.dirs but nothing exists",
			userDirs: `XDG_DOWNLOAD_DIR="$HOME/Downloads"` + "\n",
			want:     map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			mkdirs(t, home, tt.folders...)
			if tt.userDirs != "-" {
				writeUserDirs(t, home, tt.userDirs)
			}
			got, err := UserDirs(context.Background(), NewEnv(home, nil))
			if err != nil {
				t.Fatalf("UserDirs: %v", err)
			}
			rel := make(map[string]string)
			for macro, d := range got {
				rel[macro] = d.Relative
				if d.Path != filepath.Join(home, d.Relative) {
					t.Errorf("%s: Path = %q, want under %q", macro, d.Path, home)
				}
			}
			if !reflect.DeepEqual(rel, tt.want) {
				t.Errorf("UserDirs = %v, want %v", rel, tt.want)
			}
		})
	}
}

func TestUserDirsIgnoresXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	other := t.TempDir()
	mkdirs(t, home, "Dl")
	writeUserDirs(t, home, `XDG_DOWNLOAD_DIR="$HOME/Dl"`+"\n")
	if err := os.WriteFile(filepath.Join(other, "user-dirs.dirs"), []byte(`XDG_DOWNLOAD_DIR="$HOME/Other"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UserDirs(context.Background(), NewEnv(home, map[string]string{"XDG_CONFIG_HOME": other}))
	if err != nil {
		t.Fatal(err)
	}
	if got[MacroDownloads].Relative != "Dl" {
		t.Errorf("got %+v", got)
	}
}

func TestUserDirsErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name  string
		setup func(t *testing.T) (context.Context, Env)
		route string
	}{
		{
			name: "relative home",
			setup: func(t *testing.T) (context.Context, Env) {
				return context.Background(), NewEnv("rel", nil)
			},
			route: "UserDirs.UserDirsHomeNotAbsolute",
		},
		{
			name: "canceled",
			setup: func(t *testing.T) (context.Context, Env) {
				return canceled, NewEnv(t.TempDir(), nil)
			},
			route: "UserDirs.UserDirsCanceled",
		},
		{
			name: "user-dirs.dirs is a folder",
			setup: func(t *testing.T) (context.Context, Env) {
				home := t.TempDir()
				mkdirs(t, home, ".config/user-dirs.dirs")
				return context.Background(), NewEnv(home, nil)
			},
			route: "UserDirs.UserDirsReadFailed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, env := tt.setup(t)
			_, err := UserDirs(ctx, env)
			requireRoute(t, err, tt.route)
		})
	}
}

func testDirs(t *testing.T) Dirs {
	t.Helper()
	d, err := Resolve(NewEnv(t.TempDir(), nil))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a folder", path)
	}
	return info.Mode().Perm()
}

func TestEnsureDirsModes(t *testing.T) {
	d := testDirs(t)
	old := umaskForTest(0o077) // a strict umask must not change the modes
	defer umaskForTest(old)
	if err := EnsureDirs(context.Background(), d); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	tests := []struct {
		name string
		path string
		want os.FileMode
	}{
		{"Data", d.Data, 0o700},
		{"Homes", d.Homes, 0o700},
		{"AppData", d.AppData, 0o700},
		{"Profiles", d.Profiles, 0o755},
		{"Imported", d.Imported, 0o755},
		{"Launchers", d.Launchers, 0o755},
		{"Cache", d.Cache, 0o755},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mode(t, tt.path); got != tt.want {
				t.Errorf("mode = %o, want %o", got, tt.want)
			}
		})
	}
	for _, p := range []string{d.Bin, d.Applications, d.Icons} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must not be created by EnsureDirs (stat err: %v)", p, err)
		}
	}
	// a second call changes nothing and succeeds
	if err := EnsureDirs(context.Background(), d); err != nil {
		t.Fatalf("second EnsureDirs: %v", err)
	}
}

func TestEnsureDirsKeepsExistingModes(t *testing.T) {
	d := testDirs(t)
	if err := os.MkdirAll(d.Data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d.Data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(d.Cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d.Cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirs(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if got := mode(t, d.Data); got != 0o755 {
		t.Errorf("existing Data mode = %o, want 755", got)
	}
	if got := mode(t, d.Cache); got != 0o700 {
		t.Errorf("existing Cache mode = %o, want 700", got)
	}
	if got := mode(t, d.Homes); got != 0o700 {
		t.Errorf("new Homes mode = %o, want 700", got)
	}
}

func TestEnsureDirsErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name  string
		setup func(t *testing.T) (context.Context, Dirs)
		route string
	}{
		{
			name: "file where a folder should be",
			setup: func(t *testing.T) (context.Context, Dirs) {
				d := testDirs(t)
				if err := os.MkdirAll(d.Data, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(d.Homes, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return context.Background(), d
			},
			route: "EnsureDirs.DirNotADirectory",
		},
		{
			name: "file where a parent folder should be",
			setup: func(t *testing.T) (context.Context, Dirs) {
				d := testDirs(t)
				blocker := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(blocker, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				d.Data = filepath.Join(blocker, "jailcat")
				return context.Background(), d
			},
			route: "EnsureDirs.DirStatFailed",
		},
		{
			name: "parent folder is read-only",
			setup: func(t *testing.T) (context.Context, Dirs) {
				if os.Geteuid() == 0 {
					t.Skip("root ignores folder permissions")
				}
				d := testDirs(t)
				parent := filepath.Join(t.TempDir(), "ro")
				if err := os.Mkdir(parent, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
				d.Data = filepath.Join(parent, "jailcat")
				return context.Background(), d
			},
			route: "EnsureDirs.DirCreateFailed",
		},
		{
			name: "canceled",
			setup: func(t *testing.T) (context.Context, Dirs) {
				return canceled, testDirs(t)
			},
			route: "EnsureDirs.EnsureDirsCanceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, d := tt.setup(t)
			requireRoute(t, EnsureDirs(ctx, d), tt.route)
		})
	}
}

func TestOnPath(t *testing.T) {
	d := Dirs{Path: []string{"/usr/bin", "/home/u/.local/bin/", "/opt/./tools", "/a//b"}}
	tests := []struct {
		dir  string
		want bool
	}{
		{"/usr/bin", true},
		{"/usr/bin/", true},
		{"/usr/./bin", true},
		{"/usr//bin", true},
		{"/usr/lib/../bin", true},
		{"/home/u/.local/bin", true},
		{"/home/u/.local/bin/", true},
		{"/opt/tools", true},
		{"/opt/tools/.", true},
		{"/a/b", true},
		{"/usr", false},
		{"/usr/bin/x", false},
		{"/home/u/.local", false},
		{"usr/bin", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			if got := OnPath(d, tt.dir); got != tt.want {
				t.Errorf("OnPath(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
	if OnPath(Dirs{}, "/usr/bin") {
		t.Error("empty Path must hold nothing")
	}
}
