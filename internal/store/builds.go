package store

import (
	"context"
	"time"

	"github.com/rm4n0s/errors"
	"gorm.io/gorm"
)

// CreateBuild saves b as a new build and returns it with its ID, CreatedAt and
// UpdatedAt set (the ID and times in b are ignored). It fails with [TagSandboxNameTaken]
// when another build has the sandbox name.
func (s *Store) CreateBuild(ctx context.Context, b Build) (Build, error) {
	row, err := newRow(b)
	if err != nil {
		return Build{}, err
	}
	now := time.Now().UTC()
	row.ID = 0
	row.CreatedAt = now
	row.UpdatedAt = now
	err = s.db.WithContext(ctx).Create(&row).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return Build{}, errors.New(TagSandboxNameTaken, "another build has this sandbox name",
			"sandboxName", b.SandboxName)
	}
	if err != nil {
		return Build{}, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("sandboxName", b.SandboxName)
	}
	b.ID = row.ID
	b.CreatedAt = now
	b.UpdatedAt = now
	return b, nil
}

// GetBuild returns the build with the ID, or fails with [TagBuildNotFound].
func (s *Store) GetBuild(ctx context.Context, id int64) (Build, error) {
	var row buildRow
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Build{}, errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	if err != nil {
		return Build{}, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("id", id)
	}
	return row.build()
}

// GetBuildByName returns the build with the sandbox name, or fails with [TagBuildNotFound].
func (s *Store) GetBuildByName(ctx context.Context, name string) (Build, error) {
	var row buildRow
	err := s.db.WithContext(ctx).Where("sandbox_name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Build{}, errors.New(TagBuildNotFound, "no build has this sandbox name", "sandboxName", name)
	}
	if err != nil {
		return Build{}, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("sandboxName", name)
	}
	return row.build()
}

// ListBuilds returns every build, ordered by sandbox name.
func (s *Store) ListBuilds(ctx context.Context) ([]Build, error) {
	var rows []buildRow
	if err := s.db.WithContext(ctx).Order("sandbox_name").Find(&rows).Error; err != nil {
		return nil, errors.NewErr(tagDatabaseQueryFailed, err)
	}
	return buildsOf(rows)
}

// ListBuildsForApp returns the builds of one app, ordered by sandbox name: for
// [AppKindDesktop] the builds whose DesktopFileID is appID, for [AppKindBinary] those
// whose BinaryPath is appID. Any other kind fails with an error tagged AppKindUnknown.
func (s *Store) ListBuildsForApp(ctx context.Context, kind, appID string) ([]Build, error) {
	var column string
	switch kind {
	case AppKindDesktop:
		column = "desktop_file_id"
	case AppKindBinary:
		column = "binary_path"
	default:
		return nil, errors.New(tagAppKindUnknown, "unknown app kind", "kind", kind)
	}
	var rows []buildRow
	err := s.db.WithContext(ctx).Where("app_kind = ? AND "+column+" = ?", kind, appID).
		Order("sandbox_name").Find(&rows).Error
	if err != nil {
		return nil, errors.NewErr(tagDatabaseQueryFailed, err).SetMetadata("kind", kind, "appID", appID)
	}
	return buildsOf(rows)
}

// buildsOf turns rows into builds. Its errors come from [buildRow.build].
func buildsOf(rows []buildRow) ([]Build, error) {
	out := make([]Build, 0, len(rows))
	for _, row := range rows {
		b, err := row.build()
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// UpdateBuild saves the editable columns of b (everything but the ID, CreatedAt, the
// draft, and what [Store.RecordInstall] and [Store.MarkUninstalled] own: BuiltAt,
// FirejailVersion and Installed), sets UpdatedAt and returns the build as stored. It
// fails with [TagBuildNotFound], or with [TagSandboxNameTaken] when the new sandbox name
// is another build's.
func (s *Store) UpdateBuild(ctx context.Context, b Build) (Build, error) {
	row, err := newRow(b)
	if err != nil {
		return Build{}, err
	}
	res := s.db.WithContext(ctx).Model(&buildRow{}).Where("id = ?", b.ID).Updates(map[string]any{
		"sandbox_name":    row.SandboxName,
		"app_kind":        row.AppKind,
		"desktop_file_id": row.DesktopFileID,
		"binary_path":     row.BinaryPath,
		"base_profile":    row.BaseProfile,
		"overrides":       row.Overrides,
		"extra_rules":     row.ExtraRules,
		"icon_mode":       row.IconMode,
		"icon_seed":       row.IconSeed,
		"updated_at":      time.Now().UTC(),
	})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return Build{}, errors.New(TagSandboxNameTaken, "another build has this sandbox name",
			"sandboxName", b.SandboxName)
	}
	if res.Error != nil {
		return Build{}, errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", b.ID)
	}
	if res.RowsAffected == 0 {
		return Build{}, errors.New(TagBuildNotFound, "no build has this ID", "id", b.ID)
	}
	return s.GetBuild(ctx, b.ID)
}

// SaveDraft keeps d as the build's draft, replacing the one it had. It does not change
// UpdatedAt. It fails with [TagBuildNotFound].
func (s *Store) SaveDraft(ctx context.Context, id int64, d Draft) error {
	text, err := encodeDraft(&d)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Model(&buildRow{}).Where("id = ?", id).UpdateColumn("draft", text)
	if res.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", id)
	}
	if res.RowsAffected == 0 {
		return errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	return nil
}

// ClearDraft removes the build's draft; a build with no draft stays as it is. It fails
// with [TagBuildNotFound].
func (s *Store) ClearDraft(ctx context.Context, id int64) error {
	res := s.db.WithContext(ctx).Model(&buildRow{}).Where("id = ?", id).UpdateColumn("draft", "")
	if res.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", id)
	}
	if res.RowsAffected == 0 {
		return errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	return nil
}

// DeleteBuild removes the build and, with it, its installed files' rows. It fails with
// [TagBuildNotFound].
func (s *Store) DeleteBuild(ctx context.Context, id int64) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&buildRow{})
	if res.Error != nil {
		return errors.NewErr(tagDatabaseQueryFailed, res.Error).SetMetadata("id", id)
	}
	if res.RowsAffected == 0 {
		return errors.New(TagBuildNotFound, "no build has this ID", "id", id)
	}
	return nil
}

const (
	// TagBuildNotFound is the tag of the error for a build that does not exist.
	TagBuildNotFound = "BuildNotFound"
	// TagSandboxNameTaken is the tag of the error for a sandbox name that another build has.
	TagSandboxNameTaken = "SandboxNameTaken"

	tagAppKindUnknown = "AppKindUnknown"
)
