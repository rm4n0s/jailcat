package firejail

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/rm4n0s/errors"
)

// OverridesVersion is the schema version [Overrides] writes (SPEC §12). A reader of an
// older version upgrades it in internal/store before it reaches this package.
const OverridesVersion = 1

// FolderOverride is the user's change to one row of the folder list, by path.
type FolderOverride struct {
	// Path is the row's path as written in profiles.
	Path string `json:"path"`
	// Access is the access level the row has now: a new row, or a base row whose access
	// was changed. It is empty when Removed is set.
	Access Access `json:"access,omitempty"`
	// Removed is set when a row of the base was removed.
	Removed bool `json:"removed,omitempty"`
}

// Overrides is what the user changed from a base: the fields whose value differs and
// the folder rows that were added, changed or removed (SPEC §12). It applies to any
// base ([Overrides.Apply]) and is stored as JSON.
//
// The stored JSON is {"version": 1, "fields": {"<field>": <value>, ...}, "folders":
// [...]}. A field name or a top-level key that this version does not know is kept in
// the Overrides and written back unchanged, but otherwise ignored, so a database written
// by a newer Jailcat opens in an older one without losing data.
type Overrides struct {
	// Version is the schema version. [Diff] and the JSON decoder set it to
	// [OverridesVersion]; the zero value is written as that version.
	Version int
	// Folders are the folder row changes, in the order they were made.
	Folders []FolderOverride

	// values holds the new value of each changed field, as get of its row returns it.
	values map[Field]any
	// unknown holds the stored value of each field name this version does not know.
	unknown map[Field]json.RawMessage
	// extra holds the top-level keys of the stored JSON that this version does not know.
	extra map[string]json.RawMessage
}

// Diff returns the overrides that turn base into edited: every field whose value
// differs, and for the folder list a change per path (a row edited has it, a row of the
// base that edited lacks is Removed). Folder paths are compared as written. A Locked row
// is recorded like any other; [Overrides.Apply] leaves it and [Overrides.Validate]
// reports it.
func Diff(base, edited Settings) Overrides {
	o := Overrides{Version: OverridesVersion}
	for _, row := range fieldTable() {
		if row.equal(&base, &edited) {
			continue
		}
		if o.values == nil {
			o.values = map[Field]any{}
		}
		o.values[row.field] = row.get(&edited)
	}

	baseRows := map[string]Folder{}
	for _, f := range base.Folders {
		baseRows[f.Path] = f
	}
	seen := map[string]bool{}
	for _, f := range edited.Folders {
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		if b, ok := baseRows[f.Path]; !ok || b.Access != f.Access {
			o.Folders = append(o.Folders, FolderOverride{Path: f.Path, Access: f.Access})
		}
	}
	for _, f := range base.Folders {
		if !seen[f.Path] {
			seen[f.Path] = true
			o.Folders = append(o.Folders, FolderOverride{Path: f.Path, Removed: true})
		}
	}
	return o
}

// Apply returns base with the overrides on top. base is not changed. A folder override
// of a Locked row is left out: a locked row stays as the base has it.
func (o Overrides) Apply(base Settings) Settings {
	out := base.Clone()
	for _, row := range fieldTable() {
		if v, ok := o.values[row.field]; ok {
			row.set(&out, v)
		}
	}
	for _, fo := range o.Folders {
		i := slices.IndexFunc(out.Folders, func(f Folder) bool { return f.Path == fo.Path })
		switch {
		case i >= 0 && out.Folders[i].Locked:
			// Locked rows cannot be removed or loosened (SPEC §9.1).
		case i >= 0 && fo.Removed:
			out.Folders = slices.Delete(out.Folders, i, i+1)
		case i >= 0:
			out.Folders[i].Access = fo.Access
		case !fo.Removed:
			out.Folders = append(out.Folders, Folder{Path: fo.Path, Access: fo.Access})
		}
	}
	return out
}

// Changed reports whether the overrides change the field. A name that is not a known
// field is never changed (it may be kept, see [Overrides]).
func (o Overrides) Changed(f Field) bool {
	_, ok := o.values[f]
	return ok
}

// ChangedFields returns the fields the overrides change, in the order of the tabs.
func (o Overrides) ChangedFields() []Field {
	var out []Field
	for _, row := range fieldTable() {
		if _, ok := o.values[row.field]; ok {
			out = append(out, row.field)
		}
	}
	return out
}

// ChangedTabs returns the tabs that have at least one override, for the dots on the
// tab names. Folder overrides belong to the Files tab.
func (o Overrides) ChangedTabs() map[Tab]bool {
	tabs := map[Tab]bool{}
	for _, f := range o.ChangedFields() {
		tabs[f.Tab()] = true
	}
	if len(o.Folders) > 0 {
		tabs[TabFiles] = true
	}
	return tabs
}

// IsEmpty reports whether the overrides change nothing.
func (o Overrides) IsEmpty() bool {
	return len(o.values) == 0 && len(o.Folders) == 0
}

// Without returns the overrides without the one of the field ("Reset to profile"). The
// receiver is not changed.
func (o Overrides) Without(f Field) Overrides {
	out := o.clone()
	delete(out.values, f)
	return out
}

// WithoutFolder returns the overrides without the folder override of the path. The
// receiver is not changed.
func (o Overrides) WithoutFolder(path string) Overrides {
	out := o.clone()
	out.Folders = slices.DeleteFunc(out.Folders, func(fo FolderOverride) bool { return fo.Path == path })
	return out
}

// WithUnknownFrom returns o with the field values and the top-level keys that old holds
// but this version does not know (see [Overrides]) added, so a database written by a
// newer Jailcat loses nothing when an older one recomputes the overrides with [Diff].
// What o has itself wins: a name o already holds is not replaced. Neither o nor old is
// changed.
func (o Overrides) WithUnknownFrom(old Overrides) Overrides {
	out := o.clone()
	for name, raw := range old.unknown {
		if _, ok := out.values[name]; ok {
			continue
		}
		if _, ok := out.unknown[name]; ok {
			continue
		}
		if out.unknown == nil {
			out.unknown = map[Field]json.RawMessage{}
		}
		out.unknown[name] = raw
	}
	for key, raw := range old.extra {
		if _, ok := out.extra[key]; ok {
			continue
		}
		if out.extra == nil {
			out.extra = map[string]json.RawMessage{}
		}
		out.extra[key] = raw
	}
	return out
}

// clone copies the maps and the folder list, so the copy can be changed.
func (o Overrides) clone() Overrides {
	out := o
	out.Folders = slices.Clone(o.Folders)
	out.values = maps.Clone(o.values)
	out.unknown = maps.Clone(o.unknown)
	out.extra = maps.Clone(o.extra)
	return out
}

// MarshalJSON writes the stored form of the overrides (SPEC §12). Keys are sorted, so
// equal overrides give equal bytes. Names and keys that were read but not known are
// written back as they were.
func (o Overrides) MarshalJSON() ([]byte, error) {
	if o.Version != 0 && o.Version != OverridesVersion {
		return nil, errors.New(tagOverridesVersion, "the overrides have a version this Jailcat cannot write",
			"version", o.Version, "supported", OverridesVersion)
	}
	fields := map[string]any{}
	for name, raw := range o.unknown {
		fields[string(name)] = raw
	}
	for name, v := range o.values {
		fields[string(name)] = v
	}
	folders := o.Folders
	if folders == nil {
		folders = []FolderOverride{}
	}
	top := map[string]any{}
	for key, raw := range o.extra {
		top[key] = raw
	}
	top["version"] = OverridesVersion
	top["fields"] = fields
	top["folders"] = folders
	data, err := json.Marshal(top)
	if err != nil {
		return nil, errors.NewErr(tagOverridesEncode, err)
	}
	return data, nil
}

// UnmarshalJSON reads the stored form of the overrides. The version must be
// [OverridesVersion]; a field this version knows must have a value of its type; a field
// name or a top-level key it does not know is kept (see [Overrides]).
func (o *Overrides) UnmarshalJSON(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return errors.NewErr(tagOverridesDecode, err)
	}

	var (
		version int
		fields  map[string]json.RawMessage
		folders []FolderOverride
	)
	targets := map[string]any{"version": &version, "fields": &fields, "folders": &folders}
	extra := map[string]json.RawMessage{}
	for _, key := range slices.Sorted(maps.Keys(top)) {
		dst, known := targets[key]
		if !known {
			extra[key] = top[key]
			continue
		}
		if err := json.Unmarshal(top[key], dst); err != nil {
			return errors.NewErr(tagOverridesDecode, err).SetMetadata("key", key)
		}
	}
	if version != OverridesVersion {
		return errors.New(tagOverridesVersion, "the overrides have a version this Jailcat cannot read",
			"version", version, "supported", OverridesVersion)
	}

	values := map[Field]any{}
	unknown := map[Field]json.RawMessage{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		row, known := findRow(Field(name))
		if !known {
			unknown[Field(name)] = fields[name]
			continue
		}
		v, err := row.decode(fields[name])
		if err != nil {
			return errors.NewErr(tagOverridesFieldInvalid, err).SetMetadata("field", name)
		}
		values[row.field] = v
	}

	*o = Overrides{Version: version}
	if len(folders) > 0 {
		o.Folders = folders
	}
	if len(values) > 0 {
		o.values = values
	}
	if len(unknown) > 0 {
		o.unknown = unknown
	}
	if len(extra) > 0 {
		o.extra = extra
	}
	return nil
}

const (
	tagOverridesVersion      = "OverridesVersionUnsupported"
	tagOverridesDecode       = "OverridesDecodeFailed"
	tagOverridesFieldInvalid = "OverridesFieldInvalid"
	tagOverridesEncode       = "OverridesEncodeFailed"
)
