package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installed installs the VS Code-like build on a scratch home and returns the files as
// the store would record them.
func installed(t *testing.T, k *kit) []File {
	t.Helper()
	p := k.prepare(privateHome2(k))
	files, err := Install(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	return files
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestUninstallRemovesMatchingFiles(t *testing.T) {
	k := newKit(t)
	files := installed(t, k)
	home := mustFind(t, files, KindHome).Path
	writeText(t, filepath.Join(home, "data.txt"), "private data")

	res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Kind == KindHome {
			continue
		}
		if exists(f.Path) {
			t.Errorf("%s still exists", f.Path)
		}
	}
	if len(res.Removed) != 4 || len(res.Kept) != 0 || len(res.Missing) != 0 {
		t.Errorf("result = %+v", res)
	}
	// The order: desktop entry, icon, launcher, profile.
	var got []Kind
	for _, f := range res.Removed {
		got = append(got, f.Kind)
	}
	if strings.Join(kindStrings(got), ",") != "desktop,icon,launcher,profile" {
		t.Errorf("removed in the order %v", got)
	}
	// The private home is kept by default.
	if !res.HomeKept || readText(t, filepath.Join(home, "data.txt")) != "private data" {
		t.Errorf("the private home was not kept: %+v", res)
	}
	// Other files of the same folders are untouched, and so are the folders.
	if !exists(k.dirs.Applications) || !exists(k.dirs.Icons) || !exists(k.dirs.Bin) {
		t.Error("a folder was removed")
	}
}

func TestUninstallDeleteHome(t *testing.T) {
	k := newKit(t)
	files := installed(t, k)
	home := mustFind(t, files, KindHome).Path
	writeText(t, filepath.Join(home, "sub", "data.txt"), "private data")
	other := filepath.Join(k.dirs.Homes, "firejail-other")
	writeText(t, filepath.Join(other, "x"), "another build's home")

	res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
	if err != nil {
		t.Fatal(err)
	}
	if exists(home) {
		t.Error("the private home is still there")
	}
	if res.HomeKept || len(res.Removed) != 5 || res.Removed[4].Kind != KindHome {
		t.Errorf("result = %+v", res)
	}
	if !exists(filepath.Join(other, "x")) || !exists(k.dirs.Homes) {
		t.Error("another home or the homes folder was removed")
	}
}

func TestUninstallKeepsEditedFiles(t *testing.T) {
	tests := []struct {
		name   string
		force  func(edited string) map[string]bool
		gone   bool
		kept   int
		remove int
	}{
		{"not forced", func(string) map[string]bool { return nil }, false, 1, 3},
		{"forced", func(p string) map[string]bool { return map[string]bool{p: true} }, true, 0, 4},
		{"another file forced", func(string) map[string]bool { return map[string]bool{"/elsewhere": true} }, false, 1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newKit(t)
			files := installed(t, k)
			launcher := mustFind(t, files, KindLauncher).Path
			writeText(t, launcher, "#!/bin/sh\nmy own launcher\n")
			res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, Force: tt.force(launcher)})
			if err != nil {
				t.Fatal(err)
			}
			if exists(launcher) == tt.gone {
				t.Errorf("the edited launcher exists = %v, want %v", exists(launcher), !tt.gone)
			}
			if len(res.Kept) != tt.kept || len(res.Removed) != tt.remove {
				t.Errorf("result = %+v", res)
			}
			if tt.kept == 1 && (res.Kept[0].Path != launcher || res.Kept[0].Kind != KindLauncher) {
				t.Errorf("Kept = %+v", res.Kept)
			}
		})
	}
}

func TestUninstallMissingFiles(t *testing.T) {
	k := newKit(t)
	files := installed(t, k)
	icon := mustFind(t, files, KindIcon).Path
	home := mustFind(t, files, KindHome).Path
	for _, p := range []string{icon, home} {
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 2 || len(res.Removed) != 3 || res.HomeKept {
		t.Errorf("result = %+v", res)
	}
	// Twice: everything is missing and it is still fine.
	res, err = Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
	if err != nil || len(res.Missing) != 5 || len(res.Removed) != 0 {
		t.Errorf("second run = %+v, %v", res, err)
	}
}

func TestUninstallNotARegularFile(t *testing.T) {
	k := newKit(t)
	files := installed(t, k)
	profile := mustFind(t, files, KindProfile).Path
	if err := os.Remove(profile); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profile, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Even forced, a folder in place of a recorded file is not removed.
	res, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, Force: map[string]bool{profile: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kept) != 1 || !exists(filepath.Join(profile, "inner")) {
		t.Errorf("result = %+v", res)
	}
}

func TestUninstallIgnoresContentInTheRecords(t *testing.T) {
	// A record that still holds Content (a plan's file) works the same.
	k := newKit(t)
	p := k.prepare(nil)
	if _, err := Install(context.Background(), p, nil, nil); err != nil {
		t.Fatal(err)
	}
	res, err := Uninstall(context.Background(), p.Files, UninstallOpts{Dirs: k.dirs})
	if err != nil || len(res.Removed) != 4 {
		t.Errorf("result = %+v, %v", res, err)
	}
	for _, f := range res.Removed {
		if f.Content != nil {
			t.Errorf("%s: the result holds content", f.Kind)
		}
	}
}

func TestUninstallRoutes(t *testing.T) {
	t.Run("unknown kind removes nothing", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		files = append(files, File{Kind: "spell", Path: filepath.Join(k.root, "x")})
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
		requireRoute(t, err, TagUninstallKindUnknown, "Uninstall."+TagUninstallKindUnknown)
		if !exists(mustFind(t, files, KindProfile).Path) {
			t.Error("a file was removed before the error")
		}
	})
	t.Run("home outside the homes folder", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		victim := filepath.Join(k.root, "precious")
		writeText(t, filepath.Join(victim, "file"), "do not delete")
		for i := range files {
			if files[i].Kind == KindHome {
				files[i].Path = victim
			}
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
		requireRoute(t, err, TagUninstallHomeOutside, "Uninstall."+TagUninstallHomeOutside)
		if !exists(filepath.Join(victim, "file")) || !exists(mustFind(t, files, KindProfile).Path) {
			t.Error("something was removed before the error")
		}
		// Without DeleteHome the same record is harmless.
		if _, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs}); err != nil {
			t.Errorf("Uninstall without DeleteHome: %v", err)
		}
		if !exists(filepath.Join(victim, "file")) {
			t.Error("the folder was removed without DeleteHome")
		}
	})
	t.Run("a link inside the homes folder leads out of it", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		victim := filepath.Join(k.root, "precious")
		writeText(t, filepath.Join(victim, "deep", "file"), "do not delete")
		link := filepath.Join(k.dirs.Homes, "linked")
		if err := os.Symlink(victim, link); err != nil {
			t.Fatal(err)
		}
		for i := range files {
			if files[i].Kind == KindHome {
				files[i].Path = filepath.Join(link, "deep")
			}
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
		requireRoute(t, err, TagUninstallHomeOutside, "Uninstall."+TagUninstallHomeOutside)
		if !exists(filepath.Join(victim, "deep", "file")) {
			t.Error("the folder behind the link was removed")
		}
	})
	t.Run("a home that is a link is removed as a link", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		victim := filepath.Join(k.root, "precious")
		writeText(t, filepath.Join(victim, "file"), "do not delete")
		home := mustFind(t, files, KindHome).Path
		if err := os.RemoveAll(home); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, home); err != nil {
			t.Fatal(err)
		}
		if _, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true}); err != nil {
			t.Fatal(err)
		}
		if exists(home) || !exists(filepath.Join(victim, "file")) {
			t.Error("the link was not the only thing removed")
		}
	})
	t.Run("the homes folder itself is not a home", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		for i := range files {
			if files[i].Kind == KindHome {
				files[i].Path = k.dirs.Homes
			}
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
		requireRoute(t, err, TagUninstallHomeOutside, "Uninstall."+TagUninstallHomeOutside)
	})
	t.Run("no homes folder known", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		_, err := Uninstall(context.Background(), files, UninstallOpts{DeleteHome: true})
		requireRoute(t, err, TagUninstallHomeOutside, "Uninstall."+TagUninstallHomeOutside)
	})
	t.Run("canceled", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Uninstall(ctx, files, UninstallOpts{Dirs: k.dirs})
		requireRoute(t, err, tagUninstallCanceled, "Uninstall."+tagUninstallCanceled)
	})
	t.Run("a parent that is a file", func(t *testing.T) {
		k := newKit(t)
		files := []File{{Kind: KindProfile, Path: filepath.Join(k.root, "afile", "x.profile")}}
		writeText(t, filepath.Join(k.root, "afile"), "not a folder")
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
		requireRoute(t, err, tagUninstallStatFailed, "Uninstall."+tagUninstallStatFailed)
	})
	t.Run("unreadable file", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		profile := mustFind(t, files, KindProfile).Path
		if err := os.Chmod(profile, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := os.ReadFile(profile); err == nil {
			t.Skip("the test runs as a user that reads files without permission")
		}
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
		appErr := requireRouteMeta(t, err, tagUninstallReadFailed, "Uninstall."+tagUninstallReadFailed)
		// The files before it (desktop entry, icon, launcher) were removed and are listed.
		if removed, _ := appErr.Metadata["removed"].([]string); len(removed) != 3 {
			t.Errorf("removed = %v", appErr.Metadata["removed"])
		}
	})
	t.Run("a folder that cannot be changed", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		if err := os.Chmod(k.dirs.Applications, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(k.dirs.Applications, 0o755) })
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs})
		if err == nil {
			t.Skip("the test runs as a user that may write into read-only folders")
		}
		requireRoute(t, err, tagUninstallRemoveFailed, "Uninstall."+tagUninstallRemoveFailed)
	})
	t.Run("a home that cannot be removed", func(t *testing.T) {
		k := newKit(t)
		files := installed(t, k)
		home := mustFind(t, files, KindHome).Path
		writeText(t, filepath.Join(home, "sub", "x"), "x")
		if err := os.Chmod(filepath.Join(home, "sub"), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, "sub"), 0o755) })
		_, err := Uninstall(context.Background(), files, UninstallOpts{Dirs: k.dirs, DeleteHome: true})
		if err == nil {
			t.Skip("the test runs as a user that may write into read-only folders")
		}
		requireRoute(t, err, tagUninstallRemoveFailed, "Uninstall."+tagUninstallRemoveFailed)
	})
}
