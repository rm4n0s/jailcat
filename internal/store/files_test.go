package store

import (
	"context"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
)

func filesOf(dir string) []InstalledFile {
	return []InstalledFile{
		{Kind: FileKindProfile, Path: dir + "/profiles/x.profile", SHA256: "11"},
		{Kind: FileKindLauncher, Path: dir + "/bin/x", SHA256: "22"},
		{Kind: FileKindDesktop, Path: dir + "/applications/x.desktop", SHA256: "33"},
		{Kind: FileKindIcon, Path: dir + "/icons/x.svg", SHA256: "44"},
		{Kind: FileKindHome, Path: dir + "/homes/x"},
	}
}

func sameFiles(t *testing.T, got []InstalledFile, buildID int64, want []InstalledFile) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID <= 0 || got[i].BuildID != buildID || got[i].Kind != want[i].Kind ||
			got[i].Path != want[i].Path || got[i].SHA256 != want[i].SHA256 {
			t.Errorf("file %d = %+v, want %+v of build %d", i, got[i], want[i], buildID)
		}
	}
}

func TestRecordInstall(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		b := mustCreate(t, s, testBuild("firejail-code"))
		files := filesOf("/h/x")
		files[0].BuildID = 777 // ignored
		files[0].ID = 777      // ignored
		builtAt := fixedTime()

		if err := s.RecordInstall(ctx, b.ID, files, "firejail version 0.9.80", builtAt); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetBuild(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Installed || got.FirejailVersion != "firejail version 0.9.80" || got.BuiltAt == nil || !got.BuiltAt.Equal(builtAt) {
			t.Errorf("build = %+v, want installed with the version and time", got)
		}
		if !got.UpdatedAt.After(b.UpdatedAt) {
			t.Errorf("UpdatedAt = %v, was %v", got.UpdatedAt, b.UpdatedAt)
		}
		stored, err := s.InstalledFiles(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameFiles(t, stored, b.ID, filesOf("/h/x"))

		// A rebuild replaces the files: some stay, some go, some are new.
		rebuilt := []InstalledFile{filesOf("/h/x")[1], {Kind: FileKindIcon, Path: "/h/x/icons/new.svg", SHA256: "55"}}
		later := builtAt.Add(time.Hour)
		if err := s.RecordInstall(ctx, b.ID, rebuilt, "firejail version 0.9.82", later); err != nil {
			t.Fatal(err)
		}
		stored, err = s.InstalledFiles(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameFiles(t, stored, b.ID, rebuilt)
		got, err = s.GetBuild(ctx, b.ID)
		if err != nil || got.FirejailVersion != "firejail version 0.9.82" || !got.BuiltAt.Equal(later) {
			t.Errorf("build = %+v, %v", got, err)
		}

		// An empty list records a build with no files.
		if err := s.RecordInstall(ctx, b.ID, nil, "v", later); err != nil {
			t.Fatal(err)
		}
		if stored, err = s.InstalledFiles(ctx, b.ID); err != nil || len(stored) != 0 {
			t.Errorf("InstalledFiles() = %+v, %v; want none", stored, err)
		}

		assertRoute(t, s.RecordInstall(ctx, 4242, files, "v", later), "Store.RecordInstall.BuildNotFound")
		if owner, ok, err := s.FileOwner(ctx, files[1].Path); err != nil || ok {
			t.Errorf("a failed RecordInstall left %+v, %v, %v", owner, ok, err)
		}
	})
}

func TestRecordInstallIsAtomic(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		b := mustCreate(t, s, testBuild("firejail-code"))
		builtAt := fixedTime()
		if err := s.RecordInstall(ctx, b.ID, filesOf("/h/x"), "0.9.80", builtAt); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetBuild(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}

		// A duplicate path inside the new list fails the insert after the old rows were
		// deleted and the build was updated: all of it is undone.
		dup := []InstalledFile{
			{Kind: FileKindLauncher, Path: "/h/y/bin/y", SHA256: "aa"},
			{Kind: FileKindDesktop, Path: "/h/y/applications/y.desktop", SHA256: "bb"},
			{Kind: FileKindIcon, Path: "/h/y/bin/y", SHA256: "cc"},
		}
		err = s.RecordInstall(ctx, b.ID, dup, "0.9.99", builtAt.Add(time.Hour))
		assertRoute(t, err, "Store.RecordInstall.FileOwnedByOtherBuild")
		if appErr, _ := errors.FromError(err); appErr.Tag != TagFileOwnedByOtherBuild {
			t.Errorf("tag = %s", appErr.Tag)
		}

		stored, err := s.InstalledFiles(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameFiles(t, stored, b.ID, filesOf("/h/x"))
		after, err := s.GetBuild(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameBuild(t, after, before)
		if _, ok, err := s.FileOwner(ctx, "/h/y/bin/y"); err != nil || ok {
			t.Errorf("FileOwner of a rolled back path = %v, %v", ok, err)
		}
	})
}

func TestFileOwnedByOtherBuild(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		a := mustCreate(t, s, testBuild("firejail-a"))
		b := mustCreate(t, s, testBuild("firejail-b"))
		if err := s.RecordInstall(ctx, a.ID, filesOf("/h/a"), "v", fixedTime()); err != nil {
			t.Fatal(err)
		}

		mine := []InstalledFile{{Kind: FileKindLauncher, Path: "/h/b/bin/b", SHA256: "x"}, filesOf("/h/a")[3]}
		err := s.RecordInstall(ctx, b.ID, mine, "v", fixedTime())
		assertRoute(t, err, "Store.RecordInstall.FileOwnedByOtherBuild")

		// Neither build changed.
		gotB, err := s.GetBuild(ctx, b.ID)
		if err != nil || gotB.Installed || gotB.BuiltAt != nil || gotB.FirejailVersion != "" {
			t.Errorf("build b = %+v, %v; want it untouched", gotB, err)
		}
		if files, err := s.InstalledFiles(ctx, b.ID); err != nil || len(files) != 0 {
			t.Errorf("files of b = %+v, %v", files, err)
		}
		files, err := s.InstalledFiles(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameFiles(t, files, a.ID, filesOf("/h/a"))
	})
}

func TestFileOwner(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		a := mustCreate(t, s, testBuild("firejail-a"))
		b := mustCreate(t, s, testBuild("firejail-b"))
		if err := s.RecordInstall(ctx, a.ID, filesOf("/h/a"), "v", fixedTime()); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordInstall(ctx, b.ID, filesOf("/h/b"), "v", fixedTime()); err != nil {
			t.Fatal(err)
		}
		got, ok, err := s.FileOwner(ctx, "/h/b/icons/x.svg")
		if err != nil || !ok {
			t.Fatalf("FileOwner() = %+v, %v, %v", got, ok, err)
		}
		if got.BuildID != b.ID || got.Kind != FileKindIcon || got.SHA256 != "44" || got.Path != "/h/b/icons/x.svg" || got.ID <= 0 {
			t.Errorf("FileOwner() = %+v", got)
		}
		if got, ok, err := s.FileOwner(ctx, "/h/c/none"); err != nil || ok || got != (InstalledFile{}) {
			t.Errorf("FileOwner of an unknown path = %+v, %v, %v", got, ok, err)
		}
	})
}

func TestInstalledFilesOfOtherBuilds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustCreate(t, s, testBuild("firejail-a"))
	b := mustCreate(t, s, testBuild("firejail-b"))
	if err := s.RecordInstall(ctx, a.ID, filesOf("/h/a"), "v", fixedTime()); err != nil {
		t.Fatal(err)
	}
	if files, err := s.InstalledFiles(ctx, b.ID); err != nil || len(files) != 0 {
		t.Errorf("files of b = %+v, %v", files, err)
	}
	if files, err := s.InstalledFiles(ctx, 4242); err != nil || len(files) != 0 {
		t.Errorf("files of no build = %+v, %v", files, err)
	}
}

func TestMarkUninstalled(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		a := mustCreate(t, s, testBuild("firejail-a"))
		b := mustCreate(t, s, testBuild("firejail-b"))
		builtAt := fixedTime()
		if err := s.RecordInstall(ctx, a.ID, filesOf("/h/a"), "0.9.80", builtAt); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordInstall(ctx, b.ID, filesOf("/h/b"), "0.9.80", builtAt); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveDraft(ctx, a.ID, Draft{BaseProfile: "/p"}); err != nil {
			t.Fatal(err)
		}
		before, err := s.GetBuild(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}

		if err := s.MarkUninstalled(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetBuild(ctx, a.ID)
		if err != nil {
			t.Fatalf("the build is gone: %v", err)
		}
		if got.Installed {
			t.Error("still installed")
		}
		// Settings are kept; only Installed (and UpdatedAt) change.
		want := before
		want.Installed = false
		want.UpdatedAt = got.UpdatedAt
		sameBuild(t, got, want)
		if files, err := s.InstalledFiles(ctx, a.ID); err != nil || len(files) != 0 {
			t.Errorf("files = %+v, %v; want none", files, err)
		}
		if _, ok, err := s.FileOwner(ctx, "/h/a/bin/x"); err != nil || ok {
			t.Errorf("FileOwner() = %v, %v; the path should be free", ok, err)
		}
		// The other build is untouched.
		other, err := s.InstalledFiles(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameFiles(t, other, b.ID, filesOf("/h/b"))

		// Its paths can be recorded for another build now, and for itself again.
		if err := s.RecordInstall(ctx, b.ID, filesOf("/h/a"), "v", builtAt); err != nil {
			t.Errorf("RecordInstall of the freed paths: %v", err)
		}
		if err := s.MarkUninstalled(ctx, a.ID); err != nil {
			t.Errorf("MarkUninstalled twice: %v", err)
		}
		assertRoute(t, s.MarkUninstalled(ctx, 4242), "Store.MarkUninstalled.BuildNotFound")
	})
}

func TestCascadeDelete(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		a := mustCreate(t, s, testBuild("firejail-a"))
		b := mustCreate(t, s, testBuild("firejail-b"))
		if err := s.RecordInstall(ctx, a.ID, filesOf("/h/a"), "v", fixedTime()); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordInstall(ctx, b.ID, filesOf("/h/b"), "v", fixedTime()); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteBuild(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := s.db.Model(&installedFileRow{}).Where("build_id = ?", a.ID).Count(&count).Error; err != nil || count != 0 {
			t.Errorf("rows left for the deleted build: %d, %v", count, err)
		}
		if err := s.db.Model(&installedFileRow{}).Count(&count).Error; err != nil || count != int64(len(filesOf("/h/b"))) {
			t.Errorf("rows in all: %d, %v; want the other build's", count, err)
		}
		if _, ok, err := s.FileOwner(ctx, "/h/a/bin/x"); err != nil || ok {
			t.Errorf("FileOwner() = %v, %v; the path should be free", ok, err)
		}
	})
}

// TestForeignKeyIsEnforced inserts a file of a build that does not exist.
func TestForeignKeyIsEnforced(t *testing.T) {
	s := newTestStore(t)
	err := s.db.Create(&installedFileRow{BuildID: 4242, Kind: FileKindIcon, Path: "/x"}).Error
	if err == nil {
		t.Fatal("a file of no build was accepted")
	}
}
