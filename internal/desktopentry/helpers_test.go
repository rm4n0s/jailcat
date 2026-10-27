package desktopentry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"
)

// requireRoute fails the test unless err is an *errors.Error whose route has route.
// It returns the error for a test that also checks the metadata.
func requireRoute(t *testing.T, err error, route string) *errors.Error {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
	return appErr
}

// checkRoute is requireRoute for a test that does not use the error.
func checkRoute(t *testing.T, err error, route string) {
	t.Helper()
	_ = requireRoute(t, err, route)
}

// parseFixture parses testdata/name.
func parseFixture(t *testing.T, name string) (*Entry, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Error(cerr)
		}
	}()
	return Parse(f)
}

// mustParse parses text and fails the test on an error.
func mustParse(t *testing.T, text string) *Entry {
	t.Helper()
	e, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return e
}

// mustWrite writes e to a string and fails the test on an error.
func mustWrite(t *testing.T, e *Entry) string {
	t.Helper()
	var b strings.Builder
	if err := Write(&b, e); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return b.String()
}
