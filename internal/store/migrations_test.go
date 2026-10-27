package store

import (
	"context"
	"slices"
	"testing"

	"github.com/rm4n0s/errors"
	"gorm.io/gorm"
)

func appliedIDs(t *testing.T, s *Store) []string {
	t.Helper()
	var ids []string
	if err := s.db.Model(&appliedMigration{}).Order("applied_at, id").Pluck("id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestMigrationsAreRecorded(t *testing.T) {
	s := newTestStore(t)
	if got, want := appliedIDs(t, s), []string{"001_init"}; !slices.Equal(got, want) {
		t.Fatalf("applied = %v, want %v", got, want)
	}
	for _, table := range []string{"builds", "installed_files", "schema_migrations"} {
		if !s.db.Migrator().HasTable(table) {
			t.Errorf("table %s was not created", table)
		}
	}
}

func TestSchema(t *testing.T) {
	s := newTestStore(t)

	// The unique indexes.
	for _, tt := range []struct{ table, column string }{
		{"builds", "sandbox_name"},
		{"installed_files", "path"},
	} {
		var indexes []struct {
			Name   string
			Unique int
		}
		if err := s.db.Raw("SELECT name, \"unique\" AS \"unique\" FROM pragma_index_list(?)", tt.table).Scan(&indexes).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ix := range indexes {
			if ix.Unique != 1 {
				continue
			}
			var cols []string
			if err := s.db.Raw("SELECT name FROM pragma_index_info(?)", ix.Name).Scan(&cols).Error; err != nil {
				t.Fatal(err)
			}
			if slices.Equal(cols, []string{tt.column}) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s.%s has no unique index (indexes: %+v)", tt.table, tt.column, indexes)
		}
	}

	// The foreign key, with ON DELETE CASCADE.
	var fks []struct {
		Table    string
		From     string
		To       string
		OnDelete string
	}
	if err := s.db.Raw(`SELECT "table" AS "table", "from" AS "from", "to" AS "to", on_delete FROM pragma_foreign_key_list('installed_files')`).Scan(&fks).Error; err != nil {
		t.Fatal(err)
	}
	if len(fks) != 1 || fks[0].Table != "builds" || fks[0].From != "build_id" || fks[0].To != "id" || fks[0].OnDelete != "CASCADE" {
		t.Errorf("foreign keys of installed_files = %+v", fks)
	}
}

func TestMigrationsTwiceIsNoop(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	b := mustCreate(t, s, testBuild("firejail-code"))
	before := appliedIDs(t, s)
	var appliedAt []string
	if err := s.db.Model(&appliedMigration{}).Pluck("applied_at", &appliedAt).Error; err != nil {
		t.Fatal(err)
	}

	if err := s.migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if got := appliedIDs(t, s); !slices.Equal(got, before) {
		t.Errorf("applied = %v, want %v", got, before)
	}
	var after []string
	if err := s.db.Model(&appliedMigration{}).Pluck("applied_at", &after).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(after, appliedAt) {
		t.Errorf("applied_at changed: %v -> %v", appliedAt, after)
	}
	if got, err := s.GetBuild(ctx, b.ID); err != nil || got.SandboxName != b.SandboxName {
		t.Errorf("GetBuild() = %+v, %v; want the build kept", got, err)
	}
}

func TestRunMigrations(t *testing.T) {
	failing := errors.New("Inner", "boom")
	tests := []struct {
		name      string
		list      []migration
		wantRoute string
		wantApply []string
	}{
		{
			name:      "applies each migration once",
			list:      []migration{{id: "a", up: func(*gorm.DB) error { return nil }}},
			wantApply: []string{"001_init", "a"},
		},
		{
			name:      "failing migration is rolled back and reported",
			list:      []migration{{id: "bad", up: func(*gorm.DB) error { return failing }}},
			wantRoute: "Store.runMigrations.MigrationFailed",
			wantApply: []string{"001_init"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			err := s.runMigrations(ctx, tt.list)
			if tt.wantRoute != "" {
				assertRoute(t, err, tt.wantRoute)
			} else {
				if err != nil {
					t.Fatal(err)
				}
				// Running the same list again must not apply anything twice.
				if err := s.runMigrations(ctx, tt.list); err != nil {
					t.Fatalf("second run: %v", err)
				}
			}
			if got := appliedIDs(t, s); !slices.Equal(got, tt.wantApply) {
				t.Errorf("applied = %v, want %v", got, tt.wantApply)
			}
		})
	}
}

func TestMigrationFailureOnClosedDatabase(t *testing.T) {
	s := closedStore(t)
	assertRoute(t, s.migrate(context.Background()), "Store.migrate->Store.runMigrations.MigrationFailed")
}
