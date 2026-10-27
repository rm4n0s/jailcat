package ui

import (
	"context"
	"log/slog"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

// OpenEditor makes the editor of app: of the stored build, or of a new build when build
// is nil. A new build starts on the profile selector's default profile (SPEC §6.1); a
// stored build on its base profile, or its draft's. It reads the profile, so it is
// called off the UI thread.
//
// A profile that cannot be read does not stop the editor: it opens on base None, with a
// notice that says so, and the user picks another profile. The details go to logger.
// The errors are the backend's, for what is needed to open the editor at all.
func OpenEditor(ctx context.Context, backend Backend, logger *slog.Logger, app apps.App, build *store.Build) (*Editor, error) {
	choices, err := backend.ProfileChoices(ctx, app)
	if err != nil {
		return nil, err
	}
	opts := EditorOpts{Home: backend.HomeDir(), HomesDir: backend.HomesDir(), Choices: choices}
	if build == nil {
		opts.BaseProfile = choices.Default.Path
		if opts.Defaults, err = backend.NewBuildDefaults(ctx, app); err != nil {
			return nil, err
		}
	} else {
		opts.Build = *build
		opts.BaseProfile = BuildBaseProfile(*build)
	}

	var notice string
	baseMissing := false
	base, profile, err := backend.ReadBase(ctx, opts.BaseProfile)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		baseMissing = build != nil && backend.BaseProfileMissing(ctx, opts.BaseProfile)
		logger.Error("reading the base profile failed", "profile", opts.BaseProfile, "err", err)
		notice = openReadFailedText(choices.LabelFor(opts.BaseProfile), err)
		opts.BaseProfile = ""
		base, profile = firejail.BaseNone(), nil
	}
	opts.Base, opts.Profile = base, profile
	e := NewEditor(app, opts)
	e.SetNotice(notice)
	e.SetBaseMissing(baseMissing)
	if build != nil && build.Installed {
		// A firejail that does not answer is not a reason not to open the build; the
		// list and the Build button say what is wrong with it.
		if info, err := backend.Firejail(ctx); err == nil {
			e.SetFirejailUpdated(firejailUpdatedFor(build.FirejailVersion, info.Version))
		}
	}
	return e, nil
}

// openReadFailedText is what the user reads when the base profile of a build could not
// be read as the editor opened.
func openReadFailedText(name string, err error) string {
	return "Jailcat could not read the profile " + name + ": " + plainReason(err) + ". Choose another profile."
}
