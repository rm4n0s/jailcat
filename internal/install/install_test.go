package install

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/desktopentry"
	"github.com/rm4n0s/jailcat/internal/firejail"
)

// mode returns the permission bits of a path.
func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestInstallVSCode(t *testing.T) {
	k := newKit(t)
	p := k.prepare(privateHome2(k))
	if conflicts, err := Check(context.Background(), p, nil); err != nil || len(conflicts) != 0 {
		t.Fatalf("Check = %v, %v", conflicts, err)
	}
	written, err := Install(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// The returned files are what the store records, in the order they were written.
	var kinds []Kind
	for _, f := range written {
		kinds = append(kinds, f.Kind)
		if f.Content != nil {
			t.Errorf("%s: the returned file holds its content", f.Kind)
		}
	}
	wantKinds := []Kind{KindProfile, KindLauncher, KindIcon, KindHome, KindDesktop}
	if strings.Join(kindStrings(kinds), ",") != strings.Join(kindStrings(wantKinds), ",") {
		t.Errorf("written kinds = %v, want %v", kinds, wantKinds)
	}

	for _, planned := range p.Files {
		got := mustFind(t, written, planned.Kind)
		if got.Path != planned.Path || got.SHA256 != planned.SHA256 || got.Mode != planned.Mode {
			t.Errorf("%s: written %+v differs from the plan %+v", planned.Kind, got, planned)
		}
		if planned.Kind == KindHome {
			if mode(t, planned.Path) != 0o700 {
				t.Errorf("home mode = %v, want 0700", mode(t, planned.Path))
			}
			continue
		}
		disk := readText(t, planned.Path)
		if disk != string(planned.Content) {
			t.Errorf("%s: the file on disk differs from the plan", planned.Kind)
		}
		if sumOf(disk) != planned.SHA256 {
			t.Errorf("%s: SHA-256 on disk differs from the recorded one", planned.Kind)
		}
		if mode(t, planned.Path) != planned.Mode {
			t.Errorf("%s: mode = %v, want %v", planned.Kind, mode(t, planned.Path), planned.Mode)
		}
	}

	// The three texts are the golden ones, apart from the scratch folders.
	golden(t, "vscode.profile", k.normalize(readText(t, file(t, p, KindProfile).Path)))
	golden(t, "vscode.sh", k.normalize(readText(t, file(t, p, KindLauncher).Path)))
	golden(t, "vscode.desktop", k.normalize(readText(t, file(t, p, KindDesktop).Path)))

	// The launcher runs: the shell parses it (it is only read, firejail is not run).
	if mode(t, file(t, p, KindLauncher).Path)&0o111 == 0 {
		t.Error("the launcher is not executable")
	}

	// The icon is XML and the app's own icon is in it.
	icon := readText(t, file(t, p, KindIcon).Path)
	dec := xml.NewDecoder(strings.NewReader(icon))
	for {
		if _, err := dec.Token(); err != nil {
			if err != io.EOF {
				t.Fatalf("the icon is not XML: %v", err)
			}
			break
		}
	}
	if !strings.Contains(icon, `fill="#2d8cff"`) {
		t.Errorf("the icon does not hold the app's icon:\n%s", icon)
	}

	// The desktop entry parses back, and its Exec splits to the launcher path.
	e, err := desktopentry.Parse(strings.NewReader(readText(t, file(t, p, KindDesktop).Path)))
	if err != nil {
		t.Fatal(err)
	}
	words, err := desktopentry.SplitExec(e.Main().String("Exec"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := file(t, p, KindLauncher).Path
	if len(words) != 2 || words[0] != launcher || words[1] != "%F" {
		t.Errorf("Exec words = %q, want [%s %%F]", words, launcher)
	}
	if got := e.Main().String("Icon"); got != "firejail-code" {
		t.Errorf("Icon = %q", got)
	}
	if got := e.Main().String("X-Jailcat-Build"); got != "7" {
		t.Errorf("X-Jailcat-Build = %q", got)
	}
	for _, a := range e.Actions() {
		w, err := desktopentry.SplitExec(a.String("Exec"))
		if err != nil || w[0] != launcher {
			t.Errorf("action Exec words = %q, %v", w, err)
		}
	}

	// Folders: Jailcat's data and homes are private, the user's own are 0755.
	for dir, want := range map[string]os.FileMode{
		k.dirs.Data: 0o700, k.dirs.Homes: 0o700, k.dirs.Profiles: 0o755,
		k.dirs.Bin: 0o755, k.dirs.Applications: 0o755, k.dirs.Icons: 0o755,
	} {
		if got := mode(t, dir); got != want {
			t.Errorf("%s mode = %v, want %v", dir, got, want)
		}
	}

	// No temporary file is left behind.
	for _, dir := range []string{k.dirs.Profiles, k.dirs.Bin, k.dirs.Applications, k.dirs.Icons} {
		matches, _ := filepath.Glob(filepath.Join(dir, ".jailcat-*"))
		if len(matches) != 0 {
			t.Errorf("temporary files left in %s: %v", dir, matches)
		}
	}
}

func kindStrings(kinds []Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

func mustFind(t *testing.T, files []File, kind Kind) File {
	t.Helper()
	for _, f := range files {
		if f.Kind == kind {
			return f
		}
	}
	t.Fatalf("no %s file in %+v", kind, files)
	return File{}
}

func TestInstallWithoutMenuOrBin(t *testing.T) {
	k := newKit(t)
	p := k.prepare(func(s *firejail.Settings) { s.InstallMenu = false; s.InstallBin = false })
	if _, err := Install(context.Background(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(k.dirs.Applications, "firejail-code.desktop")); !os.IsNotExist(err) {
		t.Errorf("a desktop entry was written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(k.dirs.Bin, "firejail-code")); !os.IsNotExist(err) {
		t.Errorf("a launcher was written in the bin folder: %v", err)
	}
	if mode(t, filepath.Join(k.dirs.Launchers, "firejail-code")) != 0o755 {
		t.Error("the launcher is not in the launchers folder with 0755")
	}
	if _, err := os.Stat(k.dirs.Bin); !os.IsNotExist(err) {
		t.Errorf("the bin folder was made without a use: %v", err)
	}
}

func TestInstallKeepsAnExistingHome(t *testing.T) {
	k := newKit(t)
	p := k.prepare(privateHome2(k))
	home := file(t, p, KindHome).Path
	if err := os.MkdirAll(filepath.Join(home, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(home, "data", "keep.txt"), "kept")
	if _, err := Install(context.Background(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if readText(t, filepath.Join(home, "data", "keep.txt")) != "kept" {
		t.Error("the private home lost a file")
	}
	if got := mode(t, home); got != 0o750 {
		t.Errorf("an existing home has mode %v, want it kept as 0750", got)
	}
}

func TestInstallRefusesUnconfirmedConflicts(t *testing.T) {
	k := newKit(t)
	p := k.prepare(nil)
	foreign := file(t, p, KindDesktop).Path
	writeText(t, foreign, "[Desktop Entry]\nName=Mine\n")
	theirs := file(t, p, KindLauncher).Path
	writeText(t, theirs, "#!/bin/sh\n"+firejail.HeaderLine+"\n")

	tests := []struct {
		name  string
		allow map[string]bool
	}{
		{"nothing confirmed", nil},
		{"only the Jailcat file confirmed", map[string]bool{theirs: true}},
		{"a foreign file cannot be confirmed", map[string]bool{theirs: true, foreign: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written, err := Install(context.Background(), p, nil, tt.allow)
			if err == nil {
				t.Fatalf("Install succeeded: %v", written)
			}
			appErr := requireRouteMeta(t, err, TagConflictsRemain, "Install."+TagConflictsRemain)
			paths, _ := appErr.Metadata["paths"].([]string)
			if len(paths) == 0 {
				t.Errorf("metadata paths = %v", appErr.Metadata["paths"])
			}
			// Nothing was written, not even the files that were free.
			if _, err := os.Stat(file(t, p, KindProfile).Path); !os.IsNotExist(err) {
				t.Errorf("the profile was written: %v", err)
			}
			if readText(t, foreign) != "[Desktop Entry]\nName=Mine\n" {
				t.Error("the foreign file was changed")
			}
			if !strings.Contains(readText(t, theirs), firejail.HeaderLine) || strings.Contains(readText(t, theirs), "exec") {
				t.Error("the Jailcat file was changed")
			}
		})
	}
}

func TestInstallOverwritesConfirmedJailcatFile(t *testing.T) {
	k := newKit(t)
	p := k.prepare(nil)
	launcher := file(t, p, KindLauncher)
	writeText(t, launcher.Path, "#!/bin/sh\n"+firejail.HeaderLine+"\n# jailcat-build: 2\nold\n")
	if err := os.Chmod(launcher.Path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), p, nil, map[string]bool{launcher.Path: true, "/unrelated": true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if readText(t, launcher.Path) != string(launcher.Content) {
		t.Error("the confirmed file was not overwritten")
	}
	if mode(t, launcher.Path) != 0o755 {
		t.Errorf("mode = %v, want 0755", mode(t, launcher.Path))
	}
}

func TestInstallRebuildOverwritesOwnFiles(t *testing.T) {
	k := newKit(t)
	p := k.prepare(nil)
	written, err := Install(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]record{}
	for _, f := range written {
		records[f.Path] = record{7, f.SHA256}
	}
	// A rebuild with another comment replaces the desktop entry; the files are ours and unchanged.
	p2 := k.prepare(func(s *firejail.Settings) { s.Comment = "Changed" })
	if _, err := Install(context.Background(), p2, owners(records), nil); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if !strings.Contains(readText(t, file(t, p2, KindDesktop).Path), "Comment=Changed") {
		t.Error("the desktop entry was not replaced")
	}

	// Without the records the same files are Jailcat-foreign conflicts, so the rebuild stops.
	_, err = Install(context.Background(), k.prepare(nil), nil, nil)
	requireRoute(t, err, TagConflictsRemain, "Install."+TagConflictsRemain)
}

func TestInstallStalePlan(t *testing.T) {
	k := newKit(t)
	p := k.prepare(nil)
	// The plan was checked and was clean...
	if conflicts, err := Check(context.Background(), p, nil); err != nil || len(conflicts) != 0 {
		t.Fatalf("Check = %v, %v", conflicts, err)
	}
	// ...then someone put their own file in the way.
	icon := file(t, p, KindIcon).Path
	writeText(t, icon, "<svg>mine</svg>")
	_, err := Install(context.Background(), p, nil, map[string]bool{})
	requireRoute(t, err, TagConflictsRemain, "Install."+TagConflictsRemain)
	if readText(t, icon) != "<svg>mine</svg>" {
		t.Error("the stale plan overwrote the file")
	}
	// A confirmation of a foreign file does not help either.
	_, err = Install(context.Background(), p, nil, map[string]bool{icon: true})
	requireRoute(t, err, TagConflictsRemain, "Install."+TagConflictsRemain)
}

func TestInstallRoutes(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		k := newKit(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Install(ctx, k.prepare(nil), nil, nil)
		requireRoute(t, err, tagCheckCanceled, "Install->Check."+tagCheckCanceled)
	})
	t.Run("canceled between files", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(nil)
		// Err is nil for the calls of Check and EnsureDirs (one per file, one per folder),
		// then reports the cancel when the first file is about to be written.
		ctx := &cancelAfter{Context: context.Background(), calls: len(p.Files) + 7}
		_, err := Install(ctx, p, nil, nil)
		appErr := requireRouteMeta(t, err, tagInstallCanceled, "Install."+tagInstallCanceled)
		if w, _ := appErr.Metadata["written"].([]string); len(w) != 0 {
			t.Errorf("written = %v, want none", w)
		}
		if _, err := os.Stat(file(t, p, KindProfile).Path); !os.IsNotExist(err) {
			t.Errorf("a file was written: %v", err)
		}
	})
	t.Run("canceled after two files", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(nil)
		ctx := &cancelAfter{Context: context.Background(), calls: len(p.Files) + 7 + 2}
		_, err := Install(ctx, p, nil, nil)
		appErr := requireRouteMeta(t, err, tagInstallCanceled, "Install."+tagInstallCanceled)
		w, _ := appErr.Metadata["written"].([]string)
		if len(w) != 2 || w[0] != file(t, p, KindProfile).Path || w[1] != file(t, p, KindLauncher).Path {
			t.Errorf("written = %v, want the profile and the launcher", w)
		}
	})
	t.Run("a folder cannot be made", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(nil)
		// $XDG_DATA_HOME exists but may not be written, so the icons folder cannot be made.
		if err := os.MkdirAll(k.dirs.DataHome, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(k.dirs.DataHome, "icons"), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(k.dirs.DataHome, "icons"), 0o755) })
		if _, err := os.Stat(k.dirs.Icons); !os.IsNotExist(err) {
			t.Fatalf("the icons folder exists: %v", err)
		}
		_, err := Install(context.Background(), p, nil, nil)
		if err == nil {
			t.Skip("the test runs as a user that may write into read-only folders")
		}
		requireRoute(t, err, tagDirCreateFailed, "Install->ensureFolders->ensureDir."+tagDirCreateFailed)
		if _, err := os.Stat(file(t, p, KindProfile).Path); !os.IsNotExist(err) {
			t.Errorf("a file was written before the folders were ready: %v", err)
		}
	})
	t.Run("jailcat folders cannot be made", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(nil)
		// A file where the cache folder should be.
		writeText(t, k.dirs.Cache, "not a folder")
		_, err := Install(context.Background(), p, nil, nil)
		requireRoute(t, err, "DirNotADirectory", "Install->ensureFolders->EnsureDirs")
	})
	t.Run("a write fails midway", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(nil)
		// The applications folder exists but is read-only: profile, launcher and icon are
		// replaced, then the desktop entry cannot get its temporary file.
		if err := os.MkdirAll(k.dirs.Applications, 0o755); err != nil {
			t.Fatal(err)
		}
		writeText(t, filepath.Join(k.dirs.Applications, "other.desktop"), "keep")
		if err := os.Chmod(k.dirs.Applications, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(k.dirs.Applications, 0o755) })
		_, err := Install(context.Background(), p, nil, nil)
		if err == nil {
			t.Skip("the test runs as a user that may write into read-only folders")
		}
		appErr := requireRouteMeta(t, err, tagFileWriteFailed, "Install->writeAtomic."+tagFileWriteFailed)
		written, _ := appErr.Metadata["written"].([]string)
		want := []string{file(t, p, KindProfile).Path, file(t, p, KindLauncher).Path, file(t, p, KindIcon).Path}
		if strings.Join(written, "\n") != strings.Join(want, "\n") {
			t.Errorf("written = %v, want %v", written, want)
		}
		if appErr.Metadata["step"] != "create" || appErr.Metadata["path"] != file(t, p, KindDesktop).Path {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
		for _, path := range want {
			if readText(t, path) != string(mustFind(t, p.Files, kindOf(p, path)).Content) {
				t.Errorf("%s was not replaced", path)
			}
		}
		matches, _ := filepath.Glob(filepath.Join(k.dirs.Applications, ".jailcat-*"))
		if len(matches) != 0 {
			t.Errorf("temporary files left: %v", matches)
		}
	})
	t.Run("the home cannot be made", func(t *testing.T) {
		k := newKit(t)
		p := k.prepare(privateHome2(k))
		home := file(t, p, KindHome).Path
		// The homes folder is read-only (made by EnsureDirs first, then locked: do it by hand).
		if err := os.MkdirAll(k.dirs.Homes, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(k.dirs.Homes, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(k.dirs.Homes, 0o700) })
		_, err := Install(context.Background(), p, nil, nil)
		if err == nil {
			t.Skip("the test runs as a user that may write into read-only folders")
		}
		appErr := requireRouteMeta(t, err, tagDirCreateFailed, "Install->ensureDir."+tagDirCreateFailed)
		if appErr.Metadata["path"] != home {
			t.Errorf("metadata = %v", appErr.Metadata)
		}
		if w, _ := appErr.Metadata["written"].([]string); len(w) != 3 {
			t.Errorf("written = %v, want profile, launcher and icon", w)
		}
		if _, err := os.Stat(file(t, p, KindDesktop).Path); !os.IsNotExist(err) {
			t.Errorf("the desktop entry was written after the home failed: %v", err)
		}
	})
}

// kindOf returns the kind of the planned file at path.
func kindOf(p *Prepared, path string) Kind {
	for _, f := range p.Files {
		if f.Path == path {
			return f.Kind
		}
	}
	return ""
}

func TestWriteAtomicRenameFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeAtomic(File{Kind: KindProfile, Path: target, Content: []byte("x"), Mode: 0o644})
	appErr := requireRouteMeta(t, err, tagFileWriteFailed, "writeAtomic."+tagFileWriteFailed)
	if appErr.Metadata["step"] != "rename" {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".jailcat-*"))
	if len(matches) != 0 {
		t.Errorf("the temporary file was left: %v", matches)
	}
}

// cancelAfter is a context whose Err is nil for the first calls and reports Canceled
// after them.
type cancelAfter struct {
	context.Context
	calls int
}

func (c *cancelAfter) Err() error {
	if c.calls > 0 {
		c.calls--
		return nil
	}
	return context.Canceled
}
