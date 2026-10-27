package ui

import (
	"context"

	"github.com/rm4n0s/jailcat/internal/store"
)

// missingBaseApps returns the [AppKey] of every app that has a build whose base profile
// file is gone (SPEC §13.3). It looks at each profile file once, so it is called off the UI
// thread.
func missingBaseApps(ctx context.Context, backend Backend, builds []store.Build) map[string]bool {
	out := map[string]bool{}
	checked := map[string]bool{}
	for _, b := range builds {
		if b.BaseProfile == "" {
			continue
		}
		missing, known := checked[b.BaseProfile]
		if !known {
			missing = backend.BaseProfileMissing(ctx, b.BaseProfile)
			checked[b.BaseProfile] = missing
		}
		if missing {
			out[BuildKey(b)] = true
		}
	}
	return out
}

// updatedApps returns the [AppKey] of every app that has an installed build made with
// another firejail version than running (SPEC §13.3). A build without a version, or an
// unknown running version, is not updated.
func updatedApps(builds []store.Build, running string) map[string]bool {
	out := map[string]bool{}
	for _, b := range builds {
		if b.Installed && firejailUpdatedFor(b.FirejailVersion, running) {
			out[BuildKey(b)] = true
		}
	}
	return out
}
