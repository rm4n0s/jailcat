package ui

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// TestProofBuild makes one real build, through [LocalBackend], into the scratch home of
// the manual proofs (CLAUDE.md "Commands"), so the running app can be shown with a built
// app. It does nothing unless JAILCAT_PROOF_BUILD is set, and refuses to run unless the
// XDG variables point under a ".jailcat-test" folder: nothing of it may land in the real
// home. Put a firejail that does not sandbox first on the PATH (see docs/IMPLEMENTATION.md
// proof method B), so no sandbox is started either.
//
// JAILCAT_PROOF_APP is the ID of the desktop entry to build, "org.gnome.Calculator.desktop"
// by default.
func TestProofBuild(t *testing.T) {
	if os.Getenv("JAILCAT_PROOF_BUILD") == "" {
		t.Skip("JAILCAT_PROOF_BUILD is not set")
	}
	ctx := context.Background()
	env, err := xdg.FromOS()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := xdg.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{dirs.Data, dirs.Cache, dirs.Bin, dirs.Applications, dirs.Icons} {
		if !strings.Contains(folder, "/.jailcat-test/") {
			t.Fatalf("%s is not in the scratch home: set XDG_DATA_HOME, XDG_CACHE_HOME and XDG_BIN_HOME under .jailcat-test/", folder)
		}
	}
	if err := xdg.EnsureDirs(ctx, dirs); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dirs.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	backend := NewLocalBackend(dirs, st, "", logger)

	id := os.Getenv("JAILCAT_PROOF_APP")
	if id == "" {
		id = "org.gnome.Calculator.desktop"
	}
	res, err := backend.ScanApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range res.Apps {
		if app.ID != id {
			continue
		}
		e, err := OpenEditor(ctx, backend, logger, app, nil)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := backend.PrepareBuild(ctx, e.Request())
		if err != nil {
			t.Fatal(err)
		}
		built, err := backend.Build(ctx, prepared, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("built %s (check ok %v)", built.Build.SandboxName, built.Check.OK)
		return
	}
	t.Fatalf("no app with the ID %s", id)
}
