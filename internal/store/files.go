package store

import (
	"context"
	"time"

	"github.com/rm4n0s/errors"
	"gorm.io/gorm"
)

// RecordInstall records a Build, in one transaction: the build's installed files become
// exactly files (the BuildID and ID in them are ignored), and its BuiltAt,
// FirejailVersion and Installed are set. If any step fails, the build and its old files
// stay as they were. It fails with [TagBuildNotFound], or with [TagFileOwnedByOtherBuild]
// when a path is already recorded for another build or twice in files.
func (s *Store) RecordInstall(ctx context.Context, id int64, files []InstalledFile, firejailVersion string, builtAt time.Time) error {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, tx.Error).SetMetadata("id", id)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()

	now := time.Now().UTC()
	builtAt = builtAt.UTC()
	res := tx.Model(&buildRow{}).Where("id = ?", id).UpdateColumns(map[string]any{
		"built_at":         builtAt,
		"firejail_version": firejailVersion,
		"installed":        true,
		"updated_at":       now,
	})
	if res.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", id)
	}
	if res.RowsAffected == 0 {
		return errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	if err := tx.Where("build_id = ?", id).Delete(&installedFileRow{}).Error; err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	if len(files) > 0 {
		rows := make([]installedFileRow, len(files))
		for i, f := range files {
			rows[i] = installedFileRow{BuildID: id, Kind: f.Kind, Path: f.Path, SHA256: f.SHA256}
		}
		err := tx.Create(&rows).Error
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return errors.New(TagFileOwnedByOtherBuild, "a file is already recorded for another build or twice",
				"id", id)
		}
		if err != nil {
			return errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	committed = true
	return nil
}

// InstalledFiles returns the files recorded for the build, in the order they were
// recorded. A build with none, or one that does not exist, gives an empty list.
func (s *Store) InstalledFiles(ctx context.Context, id int64) ([]InstalledFile, error) {
	var rows []installedFileRow
	if err := s.db.WithContext(ctx).Where("build_id = ?", id).Order("id").Find(&rows).Error; err != nil {
		return nil, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	out := make([]InstalledFile, len(rows))
	for i, row := range rows {
		out[i] = row.file()
	}
	return out, nil
}

// FileOwner returns the recorded file with the path and true, or false when no build
// owns the path (SPEC §9.5).
func (s *Store) FileOwner(ctx context.Context, path string) (InstalledFile, bool, error) {
	var row installedFileRow
	err := s.db.WithContext(ctx).Where("path = ?", path).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return InstalledFile{}, false, nil
	}
	if err != nil {
		return InstalledFile{}, false, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("path", path)
	}
	return row.file(), true, nil
}

// MarkUninstalled removes the build's installed files' rows and sets Installed to false,
// but keeps the build, its settings and BuiltAt ("Keep settings", SPEC §13.2). It fails
// with [TagBuildNotFound].
func (s *Store) MarkUninstalled(ctx context.Context, id int64) error {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, tx.Error).SetMetadata("id", id)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()

	res := tx.Model(&buildRow{}).Where("id = ?", id).UpdateColumns(map[string]any{
		"installed":  false,
		"updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", id)
	}
	if res.RowsAffected == 0 {
		return errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	if err := tx.Where("build_id = ?", id).Delete(&installedFileRow{}).Error; err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	if err := tx.Commit().Error; err != nil {
		return errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	committed = true
	return nil
}

// TagFileOwnedByOtherBuild is the tag of the error for a path that is already recorded
// for another build (or twice in one list of files).
const TagFileOwnedByOtherBuild = "FileOwnedByOtherBuild"
