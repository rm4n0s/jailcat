package firejail

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
)

// alternative returns a value of the same type as v that differs from it, for any type
// a field of the table has. A new type of field fails the test until it is added here.
func alternative(t *testing.T, f Field, v any) any {
	t.Helper()
	rv := reflect.ValueOf(v)
	out := reflect.New(rv.Type()).Elem()
	switch rv.Kind() {
	case reflect.Bool:
		out.SetBool(!rv.Bool())
	case reflect.String:
		out.SetString(rv.String() + "-alt")
	case reflect.Int64:
		out.SetInt(rv.Int() + 3)
	case reflect.Slice:
		var elem reflect.Value
		switch rv.Type().Elem() {
		case reflect.TypeOf(""):
			elem = reflect.ValueOf("alt")
		case reflect.TypeOf(0):
			elem = reflect.ValueOf(7)
		case reflect.TypeOf(EnvVar{}):
			elem = reflect.ValueOf(EnvVar{Name: "ALT", Value: "v"})
		default:
			t.Fatalf("field %s: no alternative for a slice of %s: add it to alternative()", f, rv.Type().Elem())
		}
		out.Set(reflect.Append(rv, elem))
	case reflect.Pointer:
		if rv.Type().Elem().Kind() != reflect.Int {
			t.Fatalf("field %s: no alternative for %s: add it to alternative()", f, rv.Type())
		}
		if !rv.IsNil() {
			return reflect.Zero(rv.Type()).Interface() // off
		}
		n := 7
		return &n
	default:
		t.Fatalf("field %s: no alternative for kind %s: add it to alternative()", f, rv.Kind())
	}
	return out.Interface()
}

// equalSettings compares two Settings with the table and the folder list.
func equalSettings(a, b Settings) bool {
	for _, row := range fieldTable() {
		if !row.equal(&a, &b) {
			return false
		}
	}
	return slices.Equal(a.Folders, b.Folders)
}

func TestFieldTable(t *testing.T) {
	table := fieldTable()
	seen := map[Field]bool{}
	tabs := map[Tab]bool{}
	for _, row := range table {
		if seen[row.field] {
			t.Errorf("field %s is in the table twice", row.field)
		}
		seen[row.field] = true
		tabs[row.tab] = true
		if row.field == "" || row.tab == "" {
			t.Errorf("row %+v has no field or tab", row.field)
		}
	}
	for _, tab := range []Tab{TabFiles, TabNetwork, TabDevices, TabDesktop, TabSecurity, TabPrograms, TabLimits, TabLauncher} {
		if !tabs[tab] {
			t.Errorf("no field on the %s tab", tab)
		}
	}
	// Every settings field except the folder list has a row: count the struct's fields.
	if got, want := len(table), reflect.TypeFor[Settings]().NumField()-1; got != want {
		t.Errorf("the table has %d rows, Settings has %d fields besides Folders", got, want)
	}
	if got := Fields(); len(got) != len(table) {
		t.Errorf("len(Fields()) = %d, want %d", len(got), len(table))
	}
	// A fresh slice on every call: changing one does not change the next.
	fieldTable()[0].field = "changed"
	if fieldTable()[0].field == "changed" {
		t.Error("fieldTable shares its slice between calls")
	}
	if FieldSound.Tab() != TabDevices || Field("nope").Tab() != "" {
		t.Errorf("Tab(): sound -> %q, nope -> %q", FieldSound.Tab(), Field("nope").Tab())
	}
	a, b := DefaultSettings(), DefaultSettings()
	b.Sound = false
	if FieldSound.Equal(a, b) || !FieldCamera.Equal(a, b) || Field("nope").Equal(a, a) {
		t.Error("Field.Equal gives a wrong answer")
	}
}

func TestSettingsClone(t *testing.T) {
	s := DefaultSettings()
	s.Folders = []Folder{{Path: "/a", Access: AccessReadOnly}}
	s.DNS = []string{"1.1.1.1"}
	s.Nice = intp(3)
	s.Env = []EnvVar{{"A", "b"}}
	c := s.Clone()
	if !reflect.DeepEqual(s, c) {
		t.Fatalf("Clone() = %+v, want %+v", c, s)
	}
	c.Folders[0].Path = "/b"
	c.DNS[0] = "9.9.9.9"
	*c.Nice = 9
	c.Env[0].Value = "x"
	c.Protocols[0] = "changed"
	if s.Folders[0].Path != "/a" || s.DNS[0] != "1.1.1.1" || *s.Nice != 3 || s.Env[0].Value != "b" || s.Protocols[0] != "unix" {
		t.Errorf("Clone shares memory with the original: %+v", s)
	}
}

func TestDiffApplyEveryField(t *testing.T) {
	for _, row := range fieldTable() {
		t.Run(string(row.field), func(t *testing.T) {
			base := DefaultSettings()
			edited := base.Clone()
			row.set(&edited, alternative(t, row.field, row.get(&base)))
			if row.equal(&base, &edited) {
				t.Fatal("the alternative equals the base value")
			}

			o := Diff(base, edited)
			if o.Version != OverridesVersion {
				t.Errorf("Version = %d", o.Version)
			}
			if got := o.ChangedFields(); !slices.Equal(got, []Field{row.field}) {
				t.Errorf("ChangedFields() = %v, want [%s]", got, row.field)
			}
			for _, other := range Fields() {
				if o.Changed(other) != (other == row.field) {
					t.Errorf("Changed(%s) = %v", other, o.Changed(other))
				}
			}
			if got := o.ChangedTabs(); !reflect.DeepEqual(got, map[Tab]bool{row.tab: true}) {
				t.Errorf("ChangedTabs() = %v, want only %s", got, row.tab)
			}
			if o.IsEmpty() {
				t.Error("IsEmpty() = true")
			}
			if got := o.Apply(base); !equalSettings(got, edited) {
				t.Errorf("Apply(Diff(base, edited)) differs from edited in %s: got %#v", row.field, row.get(&got))
			}

			// The same overrides apply to another base: only this field is taken from them.
			other := base.Clone()
			other.Network = NetNone
			wantOther := other.Clone()
			row.set(&wantOther, row.get(&edited))
			if got := o.Apply(other); !equalSettings(got, wantOther) {
				t.Errorf("Apply on another base: got %+v, want %+v", got, wantOther)
			}

			// JSON round trip.
			data, err := json.Marshal(o)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var back Overrides
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if !reflect.DeepEqual(back, o) {
				t.Errorf("round trip changed the overrides:\n got %#v\nwant %#v\njson %s", back, o, data)
			}
			if got := back.Apply(base); !equalSettings(got, edited) {
				t.Errorf("Apply after the round trip differs in %s", row.field)
			}
			again, err := json.Marshal(back)
			if err != nil || string(again) != string(data) {
				t.Errorf("Marshal is not stable: %s then %s (%v)", data, again, err)
			}

			// Diffing the applied settings again gives the same overrides.
			if again := Diff(base, o.Apply(base)); !reflect.DeepEqual(again, o) {
				t.Errorf("Diff(base, Apply) = %#v, want %#v", again, o)
			}
			if !o.Without(row.field).IsEmpty() {
				t.Error("Without(field) still has the override")
			}
			if o.IsEmpty() {
				t.Error("Without changed its receiver")
			}
		})
	}
}

func TestDiffNothing(t *testing.T) {
	s := DefaultSettings()
	s.Folders = []Folder{{Path: "/a", Access: AccessReadWrite}}
	o := Diff(s, s.Clone())
	if !o.IsEmpty() || len(o.ChangedTabs()) != 0 || o.Version != OverridesVersion {
		t.Errorf("Diff(s, s) = %+v", o)
	}
	// nil and empty lists are the same value.
	e := s.Clone()
	e.DNS = []string{}
	e.Nice = nil
	if got := Diff(s, e); !got.IsEmpty() {
		t.Errorf("an empty list differs from a nil one: %v", got.ChangedFields())
	}
}

func TestApplyDoesNotChangeBase(t *testing.T) {
	base := DefaultSettings()
	base.DNS = []string{"1.1.1.1"}
	base.Folders = []Folder{{Path: "/a", Access: AccessReadOnly}}
	edited := base.Clone()
	edited.DNS[0] = "9.9.9.9"
	edited.Folders[0].Access = AccessHidden
	o := Diff(base, edited)

	got := o.Apply(base)
	got.DNS[0] = "changed"
	got.Folders[0].Path = "/changed"
	if base.DNS[0] != "1.1.1.1" || base.Folders[0].Path != "/a" || base.Folders[0].Access != AccessReadOnly {
		t.Errorf("Apply changed the base: %+v", base)
	}
	// And the overrides' own values are not shared with what Apply returned.
	again := o.Apply(base)
	if again.DNS[0] != "9.9.9.9" {
		t.Errorf("Apply shares the override's slice with its result: %v", again.DNS)
	}
}

func TestFolderOverrides(t *testing.T) {
	baseFolders := []Folder{
		{Path: "${HOME}/.config/Code", Access: AccessReadWrite, FromProfile: true},
		{Path: "${HOME}/.vscode", Access: AccessReadWrite, FromProfile: true},
		{Path: "${HOME}/.config/ro", Access: AccessReadOnly, FromProfile: true, Locked: true},
	}
	base := DefaultSettings()
	base.Folders = baseFolders

	tests := []struct {
		name string
		edit func(s *Settings)
		want []FolderOverride
		// result is the folder list after Apply, in order.
		result []Folder
	}{
		{
			name:   "no change",
			edit:   func(s *Settings) {},
			result: baseFolders,
		},
		{
			name: "add a row",
			edit: func(s *Settings) { s.Folders = append(s.Folders, Folder{Path: "/mnt/data", Access: AccessReadOnly}) },
			want: []FolderOverride{{Path: "/mnt/data", Access: AccessReadOnly}},
			result: append(slices.Clone(baseFolders),
				Folder{Path: "/mnt/data", Access: AccessReadOnly}),
		},
		{
			name: "change the access of a base row",
			edit: func(s *Settings) { s.Folders[1].Access = AccessHidden },
			want: []FolderOverride{{Path: "${HOME}/.vscode", Access: AccessHidden}},
			result: []Folder{baseFolders[0],
				{Path: "${HOME}/.vscode", Access: AccessHidden, FromProfile: true}, baseFolders[2]},
		},
		{
			name:   "remove a base row",
			edit:   func(s *Settings) { s.Folders = slices.Delete(s.Folders, 0, 1) },
			want:   []FolderOverride{{Path: "${HOME}/.config/Code", Removed: true}},
			result: []Folder{baseFolders[1], baseFolders[2]},
		},
		{
			name: "remove an added row again is no override",
			edit: func(s *Settings) {
				s.Folders = append(s.Folders, Folder{Path: "/x", Access: AccessHidden})
				s.Folders = s.Folders[:len(s.Folders)-1]
			},
			result: baseFolders,
		},
		{
			name: "remove and change and add",
			edit: func(s *Settings) {
				s.Folders = []Folder{baseFolders[1], baseFolders[2], {Path: "/opt/a", Access: AccessReadWrite}}
				s.Folders[0].Access = AccessReadOnly
			},
			want: []FolderOverride{
				{Path: "${HOME}/.vscode", Access: AccessReadOnly},
				{Path: "/opt/a", Access: AccessReadWrite},
				{Path: "${HOME}/.config/Code", Removed: true},
			},
			result: []Folder{
				{Path: "${HOME}/.vscode", Access: AccessReadOnly, FromProfile: true},
				baseFolders[2],
				{Path: "/opt/a", Access: AccessReadWrite},
			},
		},
		{
			name: "a locked row removed is left",
			edit: func(s *Settings) { s.Folders = s.Folders[:2] },
			want: []FolderOverride{{Path: "${HOME}/.config/ro", Removed: true}},
			// Apply leaves the locked row.
			result: baseFolders,
		},
		{
			name: "a locked row loosened is left",
			edit: func(s *Settings) { s.Folders[2].Access = AccessReadWrite },
			want: []FolderOverride{{Path: "${HOME}/.config/ro", Access: AccessReadWrite}},
			// Apply leaves the locked row.
			result: baseFolders,
		},
		{
			name: "a repeated path counts once",
			edit: func(s *Settings) {
				s.Folders = append(s.Folders, Folder{Path: "/z", Access: AccessReadOnly}, Folder{Path: "/z", Access: AccessHidden})
			},
			want: []FolderOverride{{Path: "/z", Access: AccessReadOnly}},
			result: append(slices.Clone(baseFolders),
				Folder{Path: "/z", Access: AccessReadOnly}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			edited := base.Clone()
			tc.edit(&edited)
			o := Diff(base, edited)
			if !slices.Equal(o.Folders, tc.want) {
				t.Errorf("Folders = %+v, want %+v", o.Folders, tc.want)
			}
			if got := o.ChangedTabs(); (len(tc.want) > 0) != got[TabFiles] || len(got) > 1 {
				t.Errorf("ChangedTabs() = %v", got)
			}
			if o.Changed(FieldHome) {
				t.Error("a folder override marks a field as changed")
			}
			got := o.Apply(base)
			if !slices.Equal(got.Folders, tc.result) {
				t.Errorf("Apply().Folders = %+v, want %+v", got.Folders, tc.result)
			}

			data, err := json.Marshal(o)
			if err != nil {
				t.Fatal(err)
			}
			var back Overrides
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatal(err)
			}
			if got := back.Apply(base); !slices.Equal(got.Folders, tc.result) {
				t.Errorf("after the round trip, Apply().Folders = %+v, want %+v", got.Folders, tc.result)
			}
		})
	}
}

func TestFolderOverridesApplyOnOtherBase(t *testing.T) {
	// A removal of a row the other base lacks does nothing; a change of a row it lacks
	// adds the row (the user's intent for that path is kept).
	o := Overrides{Version: 1, Folders: []FolderOverride{
		{Path: "/gone", Removed: true},
		{Path: "/new", Access: AccessHidden},
		{Path: "/there", Access: AccessReadOnly},
	}}
	base := DefaultSettings()
	base.Folders = []Folder{{Path: "/there", Access: AccessReadWrite, FromProfile: true}}
	got := o.Apply(base)
	want := []Folder{{Path: "/there", Access: AccessReadOnly, FromProfile: true}, {Path: "/new", Access: AccessHidden}}
	if !slices.Equal(got.Folders, want) {
		t.Errorf("Folders = %+v, want %+v", got.Folders, want)
	}
}

func TestOverridesWithoutFolder(t *testing.T) {
	o := Overrides{Version: 1, Folders: []FolderOverride{{Path: "/a", Access: AccessHidden}, {Path: "/b", Removed: true}}}
	got := o.WithoutFolder("/a")
	if !slices.Equal(got.Folders, []FolderOverride{{Path: "/b", Removed: true}}) {
		t.Errorf("WithoutFolder = %+v", got.Folders)
	}
	if len(o.Folders) != 2 {
		t.Errorf("WithoutFolder changed its receiver: %+v", o.Folders)
	}
}

func TestChangedTabs(t *testing.T) {
	base := DefaultSettings()
	edited := base.Clone()
	edited.Sound = false                                          // devices
	edited.Network = NetNone                                      // network
	edited.Timeout = time.Minute                                  // limits
	edited.Folders = []Folder{{Path: "/a", Access: AccessHidden}} // files
	o := Diff(base, edited)
	want := map[Tab]bool{TabDevices: true, TabNetwork: true, TabLimits: true, TabFiles: true}
	if got := o.ChangedTabs(); !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedTabs() = %v, want %v", got, want)
	}
	// Resetting the field takes its tab's dot away.
	if got := o.Without(FieldTimeout).ChangedTabs(); got[TabLimits] || len(got) != 3 {
		t.Errorf("after Without(timeout): %v", got)
	}
}

func TestOverridesJSON(t *testing.T) {
	base := DefaultSettings()
	edited := base.Clone()
	edited.Network = NetNone
	edited.Sound = false
	edited.Nice = intp(5)
	edited.DNS = []string{"1.1.1.1"}
	edited.Folders = []Folder{{Path: "/mnt/data", Access: AccessReadOnly}}
	o := Diff(base, edited)
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"fields":{"dns":["1.1.1.1"],"network":"none","nice":5,"sound":false},"folders":[{"path":"/mnt/data","access":"read-only"}],"version":1}`
	if string(data) != want {
		t.Errorf("JSON = %s\nwant   %s", data, want)
	}

	// An empty value is "off" and still an override: a null, not a missing key.
	off := Diff(edited, base)
	data, err = json.Marshal(off)
	if err != nil {
		t.Fatal(err)
	}
	const wantOff = `{"fields":{"dns":null,"network":"full","nice":null,"sound":true},"folders":[{"path":"/mnt/data","removed":true}],"version":1}`
	if string(data) != wantOff {
		t.Errorf("JSON = %s\nwant   %s", data, wantOff)
	}
	var back Overrides
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.Apply(edited); !equalSettings(got, base) {
		t.Errorf("Apply of the off overrides = %+v, want the base", got)
	}

	// The zero Overrides is written as version 1 with nothing in it.
	data, err = json.Marshal(Overrides{})
	if err != nil || string(data) != `{"fields":{},"folders":[],"version":1}` {
		t.Errorf("zero Overrides = %s, %v", data, err)
	}
}

func TestOverridesKeepUnknown(t *testing.T) {
	// Written by a newer Jailcat: a field this version does not know, and a top-level key.
	const stored = `{"version":1,"future":{"a":[1,2]},"fields":{"sound":false,"warp_drive":{"level":9},"another":null},"folders":[]}`
	var o Overrides
	if err := json.Unmarshal([]byte(stored), &o); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !o.Changed(FieldSound) || o.Changed("warp_drive") || o.Changed("another") {
		t.Errorf("Changed: sound %v, warp_drive %v", o.Changed(FieldSound), o.Changed("warp_drive"))
	}
	if got := o.ChangedFields(); !slices.Equal(got, []Field{FieldSound}) {
		t.Errorf("ChangedFields() = %v", got)
	}
	if got := o.ChangedTabs(); !reflect.DeepEqual(got, map[Tab]bool{TabDevices: true}) {
		t.Errorf("ChangedTabs() = %v: an unknown field must not mark a tab", got)
	}
	// Ignored by Apply.
	got := o.Apply(DefaultSettings())
	want := DefaultSettings()
	want.Sound = false
	if !equalSettings(got, want) {
		t.Errorf("Apply = %+v, want only Sound off", got)
	}
	// Kept when written back.
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"fields":{"another":null,"sound":false,"warp_drive":{"level":9}},"folders":[],"version":1}`
	var gotM, wantM map[string]json.RawMessage
	if err := json.Unmarshal(data, &gotM); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &wantM); err != nil {
		t.Fatal(err)
	}
	if string(gotM["fields"]) != string(wantM["fields"]) {
		t.Errorf("fields = %s, want %s", gotM["fields"], wantM["fields"])
	}
	if string(gotM["future"]) != `{"a":[1,2]}` {
		t.Errorf("future = %s", gotM["future"])
	}
	// An edit on top of the stored overrides keeps what it did not understand.
	changed := o.Without(FieldSound)
	data, err = json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &gotM); err != nil {
		t.Fatal(err)
	}
	if string(gotM["fields"]) != `{"another":null,"warp_drive":{"level":9}}` {
		t.Errorf("after Without: fields = %s", gotM["fields"])
	}
}

func TestOverridesJSONErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		route string
	}{
		{"not JSON", `{"version":`, "Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"not an object", `[1,2]`, "Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"version of the wrong type", `{"version":"1"}`, "Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"fields of the wrong type", `{"version":1,"fields":[]}`, "Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"folders of the wrong type", `{"version":1,"folders":{"a":1}}`, "Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"a newer version", `{"version":2,"fields":{}}`, "Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
		{"no version", `{"fields":{}}`, "Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
		{"version zero", `{"version":0}`, "Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
		{"null", `null`, "Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
		{"a known field of the wrong type", `{"version":1,"fields":{"sound":"yes"}}`, "Overrides.UnmarshalJSON.OverridesFieldInvalid"},
		{"a list of the wrong type", `{"version":1,"fields":{"dns":"1.1.1.1"}}`, "Overrides.UnmarshalJSON.OverridesFieldInvalid"},
		{"a number that does not fit", `{"version":1,"fields":{"nice":"high"}}`, "Overrides.UnmarshalJSON.OverridesFieldInvalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Directly, and the way the store calls it.
			var direct Overrides
			requireRoute(t, direct.UnmarshalJSON([]byte(tc.input)), tc.route)

			// json.Unmarshal checks the syntax itself before it calls UnmarshalJSON.
			if !json.Valid([]byte(tc.input)) {
				return
			}
			var viaJSON Overrides
			requireRoute(t, json.Unmarshal([]byte(tc.input), &viaJSON), tc.route)
		})
	}
}

func TestOverridesJSONErrorMetadata(t *testing.T) {
	var o Overrides
	err := o.UnmarshalJSON([]byte(`{"version":7}`))
	appErr, ok := errors.FromError(err)
	if !ok || appErr.Tag != tagOverridesVersion || appErr.Metadata["version"] != 7 {
		t.Errorf("version error = %v", err)
	}
	err = o.UnmarshalJSON([]byte(`{"version":1,"fields":{"sound":3}}`))
	appErr, ok = errors.FromError(err)
	if !ok || appErr.Tag != tagOverridesFieldInvalid || appErr.Metadata["field"] != "sound" {
		t.Errorf("field error = %v", err)
	}
	if o.Changed(FieldSound) {
		t.Error("a failed Unmarshal changed the receiver")
	}
}

func TestOverridesMarshalErrors(t *testing.T) {
	// A version that is not ours cannot be written.
	_, err := Overrides{Version: 2}.MarshalJSON()
	requireRoute(t, err, "Overrides.MarshalJSON.OverridesVersionUnsupported")

	// A value json cannot encode. Diff never makes one: the test puts it in by hand.
	bad := Overrides{Version: 1, values: map[Field]any{FieldSound: make(chan int)}}
	_, err = bad.MarshalJSON()
	requireRoute(t, err, "Overrides.MarshalJSON.OverridesEncodeFailed")
}

func TestOverridesValidateLocked(t *testing.T) {
	base := DefaultSettings()
	base.Folders = []Folder{
		{Path: "/locked", Access: AccessReadOnly, FromProfile: true, Locked: true},
		{Path: "/free", Access: AccessReadWrite, FromProfile: true},
	}
	tests := []struct {
		name string
		o    Overrides
		want []string // paths with a FolderLocked problem
	}{
		{"nothing", Overrides{}, nil},
		{"free row changed", Overrides{Folders: []FolderOverride{{Path: "/free", Access: AccessHidden}, {Path: "/free", Removed: true}}}, nil},
		{"locked removed", Overrides{Folders: []FolderOverride{{Path: "/locked", Removed: true}}}, []string{"/locked"}},
		{"locked loosened", Overrides{Folders: []FolderOverride{{Path: "/locked", Access: AccessReadWrite}}}, []string{"/locked"}},
		{"locked hidden", Overrides{Folders: []FolderOverride{{Path: "/locked", Access: AccessHidden}}}, []string{"/locked"}},
		{"locked made read-only again", Overrides{Folders: []FolderOverride{{Path: "/locked", Access: AccessReadOnly}}}, nil},
		{"unknown path", Overrides{Folders: []FolderOverride{{Path: "/other", Removed: true}}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, p := range tc.o.Validate(base) {
				if p.Tag != ProblemFolderLocked || p.Message == "" {
					t.Errorf("problem = %+v", p)
				}
				got = append(got, p.Path)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate paths = %v, want %v", got, tc.want)
			}
			// Whatever the user tried, Apply leaves the locked row as it is.
			applied := tc.o.Apply(base)
			if applied.Folders[0] != base.Folders[0] {
				t.Errorf("Apply changed the locked row: %+v", applied.Folders[0])
			}
		})
	}
}
