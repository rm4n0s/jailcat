package store

import (
	"context"
	"time"

	"github.com/rm4n0s/errors"
	"gorm.io/gorm"
)

// A migration creates or changes the schema. Migrations are applied in order,
// each in its own transaction, and recorded in schema_migrations by ID.
//
// Migrations use their own frozen copies of the table structs (see
// migration001Init) so that a later change to a model cannot rewrite history.
// A migration is never edited or reordered once released (SPEC §12).
type migration struct {
	id string
	up func(tx *gorm.DB) error
}

type appliedMigration struct {
	ID        string `gorm:"primaryKey"`
	AppliedAt time.Time
}

func (appliedMigration) TableName() string { return "schema_migrations" }

// migrations lists every migration in the order it must run. Add new ones at
// the end; never edit or reorder an existing entry.
func migrations() []migration {
	return []migration{
		{id: "001_init", up: migration001Init},
	}
}

func (s *Store) migrate(ctx context.Context) error {
	return s.runMigrations(ctx, migrations())
}

func (s *Store) runMigrations(ctx context.Context, list []migration) error {
	db := s.db.WithContext(ctx)
	if !db.Migrator().HasTable(&appliedMigration{}) {
		if err := db.Migrator().CreateTable(&appliedMigration{}); err != nil {
			return errors.NewErr(tagMigrationFailed, err).SetMetadata("migration", "schema_migrations")
		}
	}
	for _, m := range list {
		var count int64
		if err := db.Model(&appliedMigration{}).Where(&appliedMigration{ID: m.id}).Count(&count).Error; err != nil {
			return errors.NewErr(tagMigrationFailed, err).SetMetadata("migration", m.id)
		}
		if count > 0 {
			continue
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := m.up(tx); err != nil {
				return err
			}
			return tx.Create(&appliedMigration{ID: m.id, AppliedAt: time.Now()}).Error
		})
		if err != nil {
			return errors.NewErr(tagMigrationFailed, err).SetMetadata("migration", m.id)
		}
	}
	return nil
}

// buildV1 is the builds table of migration 001 (SPEC §12).
type buildV1 struct {
	ID              int64  `gorm:"primaryKey"`
	SandboxName     string `gorm:"not null;uniqueIndex:ux_builds_sandbox_name"`
	AppKind         string `gorm:"not null"`
	DesktopFileID   string `gorm:"not null;default:''"`
	BinaryPath      string `gorm:"not null;default:''"`
	BaseProfile     string `gorm:"not null;default:''"`
	Overrides       string `gorm:"not null"`
	ExtraRules      string `gorm:"not null;default:''"`
	IconMode        string `gorm:"not null"`
	IconSeed        int64  `gorm:"not null;default:0"`
	Draft           string `gorm:"not null;default:''"`
	FirejailVersion string `gorm:"not null;default:''"`
	Installed       bool   `gorm:"not null;default:false"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	BuiltAt         *time.Time
}

func (buildV1) TableName() string { return "builds" }

// installedFileV1 is the installed_files table of migration 001. Its rows go with their
// build (ON DELETE CASCADE), and a path belongs to one row only.
type installedFileV1 struct {
	ID      int64    `gorm:"primaryKey"`
	BuildID int64    `gorm:"not null;index"`
	Kind    string   `gorm:"not null"`
	Path    string   `gorm:"not null;uniqueIndex:ux_installed_files_path"`
	SHA256  string   `gorm:"column:sha256;not null;default:''"`
	Build   *buildV1 `gorm:"foreignKey:BuildID;references:ID;constraint:OnDelete:CASCADE"`
}

func (installedFileV1) TableName() string { return "installed_files" }

func migration001Init(tx *gorm.DB) error {
	return tx.Migrator().CreateTable(&buildV1{}, &installedFileV1{})
}

const tagMigrationFailed = "MigrationFailed"
