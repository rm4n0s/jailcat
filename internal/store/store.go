package store

import (
	"context"
	"net/url"
	"os"

	"github.com/rm4n0s/errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fileMode is the mode of the database file (SPEC §12).
const fileMode = 0o600

// connParams are the options of every connection: a busy timeout, and foreign keys on.
// SQLite keeps "PRAGMA foreign_keys" per connection, so it is set in the DSN, which the
// driver applies to each connection the pool opens.
const connParams = "_busy_timeout=5000&_foreign_keys=on"

// Store wraps an open, migrated database. There is no package-level database
// variable, so tests build their own Store with OpenMemory.
type Store struct {
	db *gorm.DB
}

// Open opens the database file at path, creating it with mode 0600 if needed
// (and tightening an existing file to 0600), then runs the migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, errors.NewErr(tagDatabaseOpenFailed, err).SetMetadata("path", path)
	}
	if err := f.Chmod(fileMode); err != nil {
		_ = f.Close()
		return nil, errors.NewErr(tagDatabaseOpenFailed, err).SetMetadata("path", path)
	}
	if err := f.Close(); err != nil {
		return nil, errors.NewErr(tagDatabaseOpenFailed, err).SetMetadata("path", path)
	}
	dsn := (&url.URL{Scheme: "file", Opaque: (&url.URL{Path: path}).EscapedPath(), RawQuery: connParams}).String()
	return openDSN(ctx, dsn, false)
}

// OpenMemory opens a private in-memory database. It is meant for tests.
func OpenMemory(ctx context.Context) (*Store, error) {
	return openDSN(ctx, "file::memory:?"+connParams, true)
}

func openDSN(ctx context.Context, dsn string, single bool) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Discard,
		TranslateError: true,
	})
	if err != nil {
		return nil, errors.NewErr(tagDatabaseOpenFailed, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.NewErr(tagDatabaseOpenFailed, err)
	}
	if single {
		// Every connection to ":memory:" is its own database.
		sqlDB.SetMaxOpenConns(1)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, errors.NewErr(tagDatabaseOpenFailed, err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err)
	}
	if err := sqlDB.Close(); err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err)
	}
	return nil
}

const (
	tagDatabaseOpenFailed  = "DatabaseOpenFailed"
	tagDatabaseQueryFailed = "DatabaseQueryFailed"
)
