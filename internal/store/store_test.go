package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesPrivateFileAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jailcat.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := s.CreateBuild(ctx, testBuild("firejail-code")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}

	// Reopening runs the migrations again without error and keeps the data.
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := s.ListBuilds(ctx)
	if err != nil || len(got) != 1 || got[0].SandboxName != "firejail-code" {
		t.Errorf("ListBuilds() = %v, %v; want the saved build", got, err)
	}
}

func TestOpenTightensExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jailcat.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}
}

func TestOpenFailure(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing", "jailcat.db")
	_, err := Open(context.Background(), missingDir)
	assertRoute(t, err, "Open.DatabaseOpenFailed")
}

// TestOpenNotADatabase gives Open a file that SQLite cannot read.
func TestOpenNotADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jailcat.db")
	if err := os.WriteFile(path, []byte("this is not a SQLite database, but it is long enough to be tried"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("Open() of a text file succeeded")
	}
	assertRoute(t, err, "Open->openDSN.DatabaseOpenFailed")
}

// TestForeignKeysOnEveryConnection checks that the pragma is on for each connection of the
// pool, not just the first one.
func TestForeignKeysOnEveryConnection(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "jailcat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	sqlDB, err := s.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	var conns [3]interface{ Close() error }
	for i := range conns {
		// Held at once, so the pool has to open a new connection for each.
		c, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
		var on int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
			t.Fatal(err)
		}
		if on != 1 {
			t.Errorf("connection %d: foreign_keys = %d, want 1", i, on)
		}
	}
	for _, c := range conns {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestMemoryDatabasesArePrivate(t *testing.T) {
	ctx := context.Background()
	a, b := newTestStore(t), newTestStore(t)
	mustCreate(t, a, testBuild("firejail-a"))
	got, err := b.ListBuilds(ctx)
	if err != nil || len(got) != 0 {
		t.Errorf("second memory database has %v, %v; want nothing", got, err)
	}
}
