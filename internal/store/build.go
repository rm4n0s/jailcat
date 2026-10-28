package store

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// The values of [Build.AppKind].
const (
	// AppKindDesktop is a build of an app with a desktop entry.
	AppKindDesktop = "desktop"
	// AppKindBinary is a build of an executable found on $PATH.
	AppKindBinary = "binary"
)

// The values of [Build.IconMode].
const (
	// IconModeBadge is the app's own icon with the firejail badge.
	IconModeBadge = "badge"
	// IconModeRandom is a random icon made from [Build.IconSeed].
	IconModeRandom = "random"
)

// The values of [InstalledFile.Kind].
const (
	// FileKindProfile is the build's firejail profile.
	FileKindProfile = "profile"
	// FileKindLauncher is the launcher script in the bin folder.
	FileKindLauncher = "launcher"
	// FileKindDesktop is the desktop entry.
	FileKindDesktop = "desktop"
	// FileKindIcon is the SVG icon.
	FileKindIcon = "icon"
	// FileKindHome is the build's private home folder; it has no SHA-256.
	FileKindHome = "home"
	// FileKindAppData is the build's VS Code data folder; it has no SHA-256.
	FileKindAppData = "appdata"
)

// Build is one sandbox of one app (SPEC §12). It is a plain value: nothing in it refers
// to the database.
type Build struct {
	// ID is the build's key; [Store.CreateBuild] sets it.
	ID int64
	// SandboxName is the build's name, unique among builds.
	SandboxName string
	// AppKind is [AppKindDesktop] or [AppKindBinary].
	AppKind string
	// DesktopFileID is the desktop file ID of a desktop app.
	DesktopFileID string
	// BinaryPath is the path of the program, as resolved at the last Build.
	BinaryPath string
	// BaseProfile is the absolute path of the base profile, or empty for None.
	BaseProfile string
	// Overrides is what the user changed from the base.
	Overrides firejail.Overrides
	// ExtraRules are the user's own profile lines. A row has no newline.
	ExtraRules []string
	// IconMode is [IconModeBadge] or [IconModeRandom].
	IconMode string
	// IconSeed is the seed of a random icon.
	IconSeed int64
	// Draft is the unsaved state of the editor, or nil when there is none.
	Draft *Draft
	// FirejailVersion is the version of firejail at the last Build.
	FirejailVersion string
	// CreatedAt is when the build was created.
	CreatedAt time.Time
	// UpdatedAt is when the build was last changed (a draft does not change it).
	UpdatedAt time.Time
	// BuiltAt is the time of the last Build, nil before the first one.
	BuiltAt *time.Time
	// Installed is true while the build's files are installed.
	Installed bool
}

// Draft is the unsaved state of a build's editor (SPEC §5): what [Build] holds, as the
// user has it on screen. It is installed only by a Build.
type Draft struct {
	// Overrides are the unsaved overrides.
	Overrides firejail.Overrides
	// ExtraRules are the unsaved extra rules.
	ExtraRules []string
	// BaseProfile is the unsaved base profile.
	BaseProfile string
}

// InstalledFile is a file or folder that a build installed and owns (SPEC §9.5).
type InstalledFile struct {
	// ID is the row's key.
	ID int64
	// BuildID is the build that owns the file.
	BuildID int64
	// Kind is one of the FileKind values.
	Kind string
	// Path is the absolute path; it belongs to one build only.
	Path string
	// SHA256 is the hex SHA-256 of the file as written, empty for [FileKindHome] and [FileKindAppData].
	SHA256 string
}

// buildRow is the live model of the builds table. It never leaves the package.
type buildRow struct {
	ID              int64 `gorm:"primaryKey"`
	SandboxName     string
	AppKind         string
	DesktopFileID   string
	BinaryPath      string
	BaseProfile     string
	Overrides       string
	ExtraRules      string
	IconMode        string
	IconSeed        int64
	Draft           string
	FirejailVersion string
	Installed       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	BuiltAt         *time.Time
}

func (buildRow) TableName() string { return "builds" }

// installedFileRow is the live model of the installed_files table.
type installedFileRow struct {
	ID      int64
	BuildID int64
	Kind    string
	Path    string
	SHA256  string `gorm:"column:sha256"`
}

func (installedFileRow) TableName() string { return "installed_files" }

func (r installedFileRow) file() InstalledFile {
	return InstalledFile(r)
}

// draftJSON is the stored form of a [Draft].
type draftJSON struct {
	Overrides   json.RawMessage `json:"overrides"`
	ExtraRules  []string        `json:"extra_rules"`
	BaseProfile string          `json:"base_profile"`
}

// newRow turns a build into its row. The errors of the overrides come from the firejail
// package and are returned as they are.
func newRow(b Build) (buildRow, error) {
	overrides, err := b.Overrides.MarshalJSON()
	if err != nil {
		return buildRow{}, err
	}
	extra, err := encodeExtraRules(b.ExtraRules)
	if err != nil {
		return buildRow{}, err
	}
	draft, err := encodeDraft(b.Draft)
	if err != nil {
		return buildRow{}, err
	}
	return buildRow{
		ID:              b.ID,
		SandboxName:     b.SandboxName,
		AppKind:         b.AppKind,
		DesktopFileID:   b.DesktopFileID,
		BinaryPath:      b.BinaryPath,
		BaseProfile:     b.BaseProfile,
		Overrides:       string(overrides),
		ExtraRules:      extra,
		IconMode:        b.IconMode,
		IconSeed:        b.IconSeed,
		Draft:           draft,
		FirejailVersion: b.FirejailVersion,
		Installed:       b.Installed,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
		BuiltAt:         b.BuiltAt,
	}, nil
}

// build turns a row into a build, reading its JSON and text columns.
func (r buildRow) build() (Build, error) {
	b := Build{
		ID:              r.ID,
		SandboxName:     r.SandboxName,
		AppKind:         r.AppKind,
		DesktopFileID:   r.DesktopFileID,
		BinaryPath:      r.BinaryPath,
		BaseProfile:     r.BaseProfile,
		ExtraRules:      decodeExtraRules(r.ExtraRules),
		IconMode:        r.IconMode,
		IconSeed:        r.IconSeed,
		FirejailVersion: r.FirejailVersion,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		BuiltAt:         r.BuiltAt,
		Installed:       r.Installed,
	}
	if err := b.Overrides.UnmarshalJSON([]byte(r.Overrides)); err != nil {
		return Build{}, err
	}
	draft, err := decodeDraft(r.Draft)
	if err != nil {
		return Build{}, err
	}
	b.Draft = draft
	return b, nil
}

// encodeExtraRules writes the rows as text: every row is followed by a newline, so an
// empty list (no text) and a list of one empty row ("\n") stay different. A row with a
// line break would become two rows, so it is refused.
func encodeExtraRules(rows []string) (string, error) {
	var sb strings.Builder
	for i, row := range rows {
		if strings.ContainsAny(row, "\n\r") {
			return "", errors.New(tagExtraRuleLineBreak, "an extra rule has a line break", "row", i)
		}
		sb.WriteString(row)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// decodeExtraRules is the reverse of encodeExtraRules.
func decodeExtraRules(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// encodeDraft writes a draft as JSON, or an empty text for none.
func encodeDraft(d *Draft) (string, error) {
	if d == nil {
		return "", nil
	}
	overrides, err := d.Overrides.MarshalJSON()
	if err != nil {
		return "", err
	}
	return marshalDraft(overrides, *d)
}

// marshalDraft writes the stored form of a draft whose overrides are already JSON.
func marshalDraft(overrides json.RawMessage, d Draft) (string, error) {
	rules := d.ExtraRules
	if rules == nil {
		rules = []string{}
	}
	data, err := json.Marshal(draftJSON{Overrides: overrides, ExtraRules: rules, BaseProfile: d.BaseProfile})
	if err != nil {
		return "", errors.NewErr(tagDraftEncodeFailed, err)
	}
	return string(data), nil
}

// decodeDraft reads a stored draft; an empty text is no draft.
func decodeDraft(text string) (*Draft, error) {
	if text == "" {
		return nil, nil
	}
	var stored draftJSON
	if err := json.Unmarshal([]byte(text), &stored); err != nil {
		return nil, errors.NewErr(tagDraftDecodeFailed, err)
	}
	d := Draft{ExtraRules: stored.ExtraRules, BaseProfile: stored.BaseProfile}
	if len(d.ExtraRules) == 0 {
		d.ExtraRules = nil
	}
	if err := d.Overrides.UnmarshalJSON(stored.Overrides); err != nil {
		return nil, err
	}
	return &d, nil
}

const (
	tagExtraRuleLineBreak = "ExtraRuleHasLineBreak"
	tagDraftEncodeFailed  = "DraftEncodeFailed"
	tagDraftDecodeFailed  = "DraftDecodeFailed"
)
