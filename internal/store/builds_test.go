package store

import (
	"context"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
)

func TestCreateAndGetBuild(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		before := time.Now().Add(-time.Second)
		want := testBuild("firejail-code")
		want.ID = 99                           // ignored
		want.CreatedAt = time.Unix(1, 0).UTC() // ignored
		got, err := s.CreateBuild(ctx, want)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID <= 0 || got.ID == 99 {
			t.Errorf("ID = %d, want a new one", got.ID)
		}
		if got.CreatedAt.Before(before) || !got.UpdatedAt.Equal(got.CreatedAt) {
			t.Errorf("times = %v, %v", got.CreatedAt, got.UpdatedAt)
		}
		want.ID, want.CreatedAt, want.UpdatedAt = got.ID, got.CreatedAt, got.UpdatedAt
		sameBuild(t, got, want)

		byID, err := s.GetBuild(ctx, got.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameBuild(t, byID, want)
		if byID.Installed || byID.BuiltAt != nil || byID.Draft != nil {
			t.Errorf("a new build is %+v, want not installed, not built, no draft", byID)
		}

		byName, err := s.GetBuildByName(ctx, "firejail-code")
		if err != nil {
			t.Fatal(err)
		}
		sameBuild(t, byName, want)
	})
}

func TestBuildExtraRulesRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		rules []string
	}{
		{"none", nil},
		{"one", []string{"noroot"}},
		{"one empty row", []string{""}},
		{"empty rows around", []string{"", "net none", "", ""}},
		{"spaces and non-ASCII", []string{"  # comment é ", "whitelist /tmp/ünï code"}},
	}
	eachStore(t, func(t *testing.T, s *Store) {
		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := testBuild("firejail-rules-" + string(rune('a'+i)))
				b.ExtraRules = tt.rules
				got := mustCreate(t, s, b)
				read, err := s.GetBuild(context.Background(), got.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !sameStrings(read.ExtraRules, tt.rules) {
					t.Errorf("ExtraRules = %q, want %q", read.ExtraRules, tt.rules)
				}
			})
		}
	})
}

func TestBuildOverridesRoundTrip(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		want := testOverrides()
		if want.IsEmpty() {
			t.Fatal("the test overrides are empty")
		}
		b := testBuild("firejail-code")
		b.Overrides = want
		got := mustCreate(t, s, b)
		read, err := s.GetBuild(ctx, got.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameOverrides(t, read.Overrides, want)
		if fields := read.Overrides.ChangedFields(); len(fields) != len(want.ChangedFields()) || len(fields) == 0 {
			t.Errorf("ChangedFields() = %v, want %v", fields, want.ChangedFields())
		}
		if len(read.Overrides.Folders) != 1 || read.Overrides.Folders[0].Path != "/mnt/data" {
			t.Errorf("Folders = %+v", read.Overrides.Folders)
		}

		// No overrides are stored as an empty overrides value, never as NULL.
		empty := testBuild("firejail-empty")
		empty.Overrides = otherOverrides()
		empty.Overrides = empty.Overrides.Without("network").Without("nice")
		e := mustCreate(t, s, empty)
		var stored string
		if err := s.db.Raw("SELECT overrides FROM builds WHERE id = ?", e.ID).Scan(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored == "" {
			t.Error("empty overrides stored as an empty text")
		}
		readEmpty, err := s.GetBuild(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !readEmpty.Overrides.IsEmpty() {
			t.Errorf("empty overrides read back as %+v", readEmpty.Overrides)
		}
	})
}

func TestZeroBuildOverrides(t *testing.T) {
	s := newTestStore(t)
	b := Build{SandboxName: "firejail-bare", AppKind: AppKindBinary, IconMode: IconModeBadge}
	got := mustCreate(t, s, b)
	read, err := s.GetBuild(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Overrides.IsEmpty() || read.ExtraRules != nil || read.Draft != nil {
		t.Errorf("bare build = %+v", read)
	}
}

func TestGetBuildNotFound(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		_, err := s.GetBuild(ctx, 12345)
		assertRoute(t, err, "Store.GetBuild.BuildNotFound")
		_, err = s.GetBuildByName(ctx, "firejail-nothing")
		assertRoute(t, err, "Store.GetBuildByName.BuildNotFound")
		appErr, _ := errors.FromError(err)
		if appErr.Tag != TagBuildNotFound || appErr.Metadata["sandboxName"] != "firejail-nothing" {
			t.Errorf("error = %+v", appErr)
		}
		if !errors.Is(err, &errors.Error{Tag: TagBuildNotFound}) {
			t.Error("errors.Is with the sentinel is false")
		}
	})
}

func TestSandboxNameTaken(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		first := mustCreate(t, s, testBuild("firejail-code"))
		other := mustCreate(t, s, testBuild("firejail-other"))

		_, err := s.CreateBuild(ctx, testBuild("firejail-code"))
		assertRoute(t, err, "Store.CreateBuild.SandboxNameTaken")
		if appErr, _ := errors.FromError(err); appErr.Tag != TagSandboxNameTaken || appErr.Metadata["sandboxName"] != "firejail-code" {
			t.Errorf("error = %+v", appErr)
		}

		other.SandboxName = "firejail-code"
		_, err = s.UpdateBuild(ctx, other)
		assertRoute(t, err, "Store.UpdateBuild.SandboxNameTaken")

		// Nothing changed.
		got, err := s.ListBuilds(ctx)
		if err != nil || len(got) != 2 || got[0].ID != first.ID || got[1].SandboxName != "firejail-other" {
			t.Errorf("ListBuilds() = %+v, %v", got, err)
		}

		// A build can be saved under its own name.
		if _, err := s.UpdateBuild(ctx, first); err != nil {
			t.Errorf("UpdateBuild with its own name: %v", err)
		}
	})
}

func TestListBuilds(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		got, err := s.ListBuilds(ctx)
		if err != nil || len(got) != 0 {
			t.Fatalf("ListBuilds() on an empty database = %v, %v", got, err)
		}
		for _, name := range []string{"firejail-zed", "firejail-alpha", "firejail-mid"} {
			mustCreate(t, s, testBuild(name))
		}
		got, err = s.ListBuilds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, b := range got {
			names = append(names, b.SandboxName)
		}
		if !sameStrings(names, []string{"firejail-alpha", "firejail-mid", "firejail-zed"}) {
			t.Errorf("names = %v, want them ordered", names)
		}
	})
}

func TestListBuildsForApp(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		mk := func(name, kind, desktopID, binary string) {
			b := testBuild(name)
			b.AppKind, b.DesktopFileID, b.BinaryPath = kind, desktopID, binary
			mustCreate(t, s, b)
		}
		mk("firejail-code-work", AppKindDesktop, "code.desktop", "/usr/bin/code")
		mk("firejail-code", AppKindDesktop, "code.desktop", "/usr/bin/code")
		mk("firejail-firefox", AppKindDesktop, "firefox.desktop", "/usr/bin/firefox")
		mk("firejail-code-bin", AppKindBinary, "", "/usr/bin/code")
		mk("firejail-curl", AppKindBinary, "", "/usr/bin/curl")

		tests := []struct {
			name, kind, appID string
			want              []string
		}{
			{"desktop by desktop file ID", AppKindDesktop, "code.desktop", []string{"firejail-code", "firejail-code-work"}},
			{"desktop, one build", AppKindDesktop, "firefox.desktop", []string{"firejail-firefox"}},
			{"desktop, no build", AppKindDesktop, "gimp.desktop", nil},
			{"desktop is not looked up by path", AppKindDesktop, "/usr/bin/code", nil},
			{"binary by path", AppKindBinary, "/usr/bin/code", []string{"firejail-code-bin"}},
			{"binary, no build", AppKindBinary, "/usr/bin/nothing", nil},
			{"binary is not looked up by desktop file ID", AppKindBinary, "code.desktop", nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := s.ListBuildsForApp(ctx, tt.kind, tt.appID)
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, b := range got {
					names = append(names, b.SandboxName)
				}
				if !sameStrings(names, tt.want) {
					t.Errorf("names = %v, want %v", names, tt.want)
				}
			})
		}

		_, err := s.ListBuildsForApp(ctx, "flatpak", "x")
		assertRoute(t, err, "Store.ListBuildsForApp.AppKindUnknown")
	})
}

func TestUpdateBuild(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		created := mustCreate(t, s, testBuild("firejail-code"))
		draft := Draft{Overrides: otherOverrides(), ExtraRules: []string{"noroot"}, BaseProfile: "/etc/firejail/x.profile"}
		if err := s.SaveDraft(ctx, created.ID, draft); err != nil {
			t.Fatal(err)
		}
		builtAt := fixedTime()
		if err := s.RecordInstall(ctx, created.ID, []InstalledFile{{Kind: FileKindLauncher, Path: "/b/firejail-code", SHA256: "aa"}}, "0.9.80", builtAt); err != nil {
			t.Fatal(err)
		}

		edit := created
		edit.SandboxName = "firejail-code-renamed"
		edit.AppKind = AppKindBinary
		edit.DesktopFileID = ""
		edit.BinaryPath = "/usr/local/bin/code"
		edit.BaseProfile = ""
		edit.Overrides = otherOverrides()
		edit.ExtraRules = []string{"seccomp"}
		edit.IconMode = IconModeBadge
		edit.IconSeed = 0
		// These are not editable here: they must not change.
		edit.FirejailVersion = "9.9.9"
		edit.Installed = false
		edit.BuiltAt = nil
		edit.Draft = nil
		edit.CreatedAt = time.Unix(5, 0).UTC()

		got, err := s.UpdateBuild(ctx, edit)
		if err != nil {
			t.Fatal(err)
		}
		if !got.UpdatedAt.After(created.UpdatedAt) {
			t.Errorf("UpdatedAt = %v, was %v", got.UpdatedAt, created.UpdatedAt)
		}
		want := edit
		want.CreatedAt = created.CreatedAt
		want.UpdatedAt = got.UpdatedAt
		want.FirejailVersion = "0.9.80"
		want.Installed = true
		want.BuiltAt = &builtAt
		want.Draft = &draft
		sameBuild(t, got, want)

		read, err := s.GetBuild(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		sameBuild(t, read, want)

		_, err = s.UpdateBuild(ctx, Build{ID: 4242, SandboxName: "firejail-x"})
		assertRoute(t, err, "Store.UpdateBuild.BuildNotFound")
	})
}

func TestDrafts(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		b := mustCreate(t, s, testBuild("firejail-code"))
		original, err := s.GetBuild(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}

		d := Draft{Overrides: otherOverrides(), ExtraRules: []string{"", "noroot"}, BaseProfile: "/etc/firejail/code.profile"}
		if err := s.SaveDraft(ctx, b.ID, d); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetBuild(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Draft == nil {
			t.Fatal("the draft was not saved")
		}
		sameDraft(t, *got.Draft, d)
		// The draft is not part of the build: the rest, UpdatedAt included, is as it was.
		want := original
		want.Draft = got.Draft
		sameBuild(t, got, want)

		// A second draft replaces the first; a draft with no changes is still a draft.
		d2 := Draft{}
		if err := s.SaveDraft(ctx, b.ID, d2); err != nil {
			t.Fatal(err)
		}
		got, err = s.GetBuild(ctx, b.ID)
		if err != nil || got.Draft == nil {
			t.Fatalf("GetBuild() = %+v, %v; want an (empty) draft", got, err)
		}
		sameDraft(t, *got.Draft, d2)

		// Drafts of other builds are not touched.
		other := mustCreate(t, s, testBuild("firejail-other"))
		if err := s.SaveDraft(ctx, other.ID, d); err != nil {
			t.Fatal(err)
		}
		if err := s.ClearDraft(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
		got, err = s.GetBuild(ctx, b.ID)
		if err != nil || got.Draft != nil {
			t.Errorf("after ClearDraft: %+v, %v; want no draft", got.Draft, err)
		}
		if err := s.ClearDraft(ctx, b.ID); err != nil {
			t.Errorf("ClearDraft twice: %v", err)
		}
		got, err = s.GetBuild(ctx, other.ID)
		if err != nil || got.Draft == nil {
			t.Errorf("the other draft = %+v, %v; want it kept", got.Draft, err)
		}

		assertRoute(t, s.SaveDraft(ctx, 4242, d), "Store.SaveDraft.BuildNotFound")
		assertRoute(t, s.ClearDraft(ctx, 4242), "Store.ClearDraft.BuildNotFound")
	})
}

func TestDeleteBuild(t *testing.T) {
	eachStore(t, func(t *testing.T, s *Store) {
		ctx := context.Background()
		a := mustCreate(t, s, testBuild("firejail-a"))
		b := mustCreate(t, s, testBuild("firejail-b"))
		if err := s.DeleteBuild(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetBuild(ctx, a.ID); err == nil {
			t.Error("the build is still there")
		}
		if _, err := s.GetBuild(ctx, b.ID); err != nil {
			t.Errorf("the other build: %v", err)
		}
		assertRoute(t, s.DeleteBuild(ctx, a.ID), "Store.DeleteBuild.BuildNotFound")

		// The name is free again.
		mustCreate(t, s, testBuild("firejail-a"))
	})
}

// The data of a build is checked when it is read: a damaged column is an error with the
// route of the code that found it, and the errors of the firejail package come back as
// they are.
func TestDamagedColumns(t *testing.T) {
	tests := []struct {
		name   string
		column string
		value  string
		route  string
	}{
		{"overrides not JSON", "overrides", "{", "buildRow.build->Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"overrides empty", "overrides", "", "buildRow.build->Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"overrides version", "overrides", `{"version":7,"fields":{},"folders":[]}`, "buildRow.build->Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
		{"draft not JSON", "draft", "{", "buildRow.build->decodeDraft.DraftDecodeFailed"},
		{"draft overrides missing", "draft", `{"extra_rules":[]}`, "buildRow.build->decodeDraft->Overrides.UnmarshalJSON.OverridesDecodeFailed"},
		{"draft overrides version", "draft", `{"overrides":{"version":7},"extra_rules":[]}`, "buildRow.build->decodeDraft->Overrides.UnmarshalJSON.OverridesVersionUnsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			b := mustCreate(t, s, testBuild("firejail-code"))
			if err := s.db.Exec("UPDATE builds SET "+tt.column+" = ? WHERE id = ?", tt.value, b.ID).Error; err != nil {
				t.Fatal(err)
			}
			_, err := s.GetBuild(ctx, b.ID)
			assertRoute(t, err, "Store.GetBuild->"+tt.route)
			_, err = s.GetBuildByName(ctx, "firejail-code")
			assertRoute(t, err, "Store.GetBuildByName->"+tt.route)
			_, err = s.ListBuilds(ctx)
			assertRoute(t, err, "Store.ListBuilds->buildsOf->"+tt.route)
			_, err = s.ListBuildsForApp(ctx, AppKindDesktop, "code.desktop")
			assertRoute(t, err, "Store.ListBuildsForApp->buildsOf->"+tt.route)
			if tt.column == "draft" {
				// UpdateBuild rewrites the overrides, so only a damaged draft shows in what it returns.
				_, err = s.UpdateBuild(ctx, b)
				assertRoute(t, err, "Store.UpdateBuild->Store.GetBuild->"+tt.route)
			}
		})
	}
}

func TestWriteErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	b := mustCreate(t, s, testBuild("firejail-code"))

	t.Run("overrides of a version this Jailcat cannot write", func(t *testing.T) {
		bad := testBuild("firejail-bad")
		bad.Overrides.Version = 7
		_, err := s.CreateBuild(ctx, bad)
		assertRoute(t, err, "Store.CreateBuild->newRow->Overrides.MarshalJSON.OverridesVersionUnsupported")
		bad.ID = b.ID
		_, err = s.UpdateBuild(ctx, bad)
		assertRoute(t, err, "Store.UpdateBuild->newRow->Overrides.MarshalJSON.OverridesVersionUnsupported")
		err = s.SaveDraft(ctx, b.ID, Draft{Overrides: bad.Overrides})
		assertRoute(t, err, "Store.SaveDraft->encodeDraft->Overrides.MarshalJSON.OverridesVersionUnsupported")
		err = s.SaveDraft(ctx, b.ID, Draft{Overrides: bad.Overrides})
		appErr, ok := errors.FromError(err)
		if !ok || appErr.Tag != "OverridesVersionUnsupported" {
			t.Errorf("error = %v; want the firejail error, untouched", err)
		}
	})

	t.Run("an extra rule with a line break", func(t *testing.T) {
		for _, rule := range []string{"noroot\nnet none", "noroot\r", "\n"} {
			bad := testBuild("firejail-bad")
			bad.ExtraRules = []string{"ok", rule}
			_, err := s.CreateBuild(ctx, bad)
			assertRoute(t, err, "Store.CreateBuild->newRow->encodeExtraRules.ExtraRuleHasLineBreak")
			if appErr, _ := errors.FromError(err); appErr.Metadata["row"] != 1 {
				t.Errorf("metadata = %v", appErr.Metadata)
			}
			bad.ID = b.ID
			_, err = s.UpdateBuild(ctx, bad)
			assertRoute(t, err, "Store.UpdateBuild->newRow->encodeExtraRules.ExtraRuleHasLineBreak")
		}
		if got, err := s.ListBuilds(ctx); err != nil || len(got) != 1 {
			t.Errorf("ListBuilds() = %v, %v; want only the first build", got, err)
		}
	})

	t.Run("a draft that cannot be written", func(t *testing.T) {
		_, err := marshalDraft([]byte("{"), Draft{})
		assertRoute(t, err, "marshalDraft.DraftEncodeFailed")
	})
}

func TestQueryFailures(t *testing.T) {
	ctx := context.Background()
	s := closedStore(t)
	tests := []struct {
		name  string
		route string
		call  func() error
	}{
		{"CreateBuild", "Store.CreateBuild.DatabaseQueryFailed", func() error { _, err := s.CreateBuild(ctx, testBuild("x")); return err }},
		{"GetBuild", "Store.GetBuild.DatabaseQueryFailed", func() error { _, err := s.GetBuild(ctx, 1); return err }},
		{"GetBuildByName", "Store.GetBuildByName.DatabaseQueryFailed", func() error { _, err := s.GetBuildByName(ctx, "x"); return err }},
		{"ListBuilds", "Store.ListBuilds.DatabaseQueryFailed", func() error { _, err := s.ListBuilds(ctx); return err }},
		{"ListBuildsForApp", "Store.ListBuildsForApp.DatabaseQueryFailed", func() error { _, err := s.ListBuildsForApp(ctx, AppKindDesktop, "x"); return err }},
		{"UpdateBuild", "Store.UpdateBuild.DatabaseQueryFailed", func() error { _, err := s.UpdateBuild(ctx, testBuild("x")); return err }},
		{"SaveDraft", "Store.SaveDraft.DatabaseQueryFailed", func() error { return s.SaveDraft(ctx, 1, Draft{}) }},
		{"ClearDraft", "Store.ClearDraft.DatabaseQueryFailed", func() error { return s.ClearDraft(ctx, 1) }},
		{"DeleteBuild", "Store.DeleteBuild.DatabaseQueryFailed", func() error { return s.DeleteBuild(ctx, 1) }},
		{"RecordInstall", "Store.RecordInstall.DatabaseQueryFailed", func() error { return s.RecordInstall(ctx, 1, nil, "v", time.Now()) }},
		{"InstalledFiles", "Store.InstalledFiles.DatabaseQueryFailed", func() error { _, err := s.InstalledFiles(ctx, 1); return err }},
		{"FileOwner", "Store.FileOwner.DatabaseQueryFailed", func() error { _, _, err := s.FileOwner(ctx, "/x"); return err }},
		{"MarkUninstalled", "Store.MarkUninstalled.DatabaseQueryFailed", func() error { return s.MarkUninstalled(ctx, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assertRoute(t, tt.call(), tt.route) })
	}
}

func TestCanceledContext(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.ListBuilds(ctx)
	assertRoute(t, err, "Store.ListBuilds.DatabaseQueryFailed")
}
