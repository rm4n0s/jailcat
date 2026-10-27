package ui

import (
	"context"
	"fmt"
	"slices"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/store"
)

func (f *fakeBackend) HomeDir() string { return f.fj.dirs.Home }

func (f *fakeBackend) ProfileChoices(ctx context.Context, app apps.App) (ProfileChoices, error) {
	f.mu.Lock()
	override := f.choices
	f.mu.Unlock()
	if override != nil {
		return *override, nil
	}
	return f.fj.ProfileChoices(ctx, app)
}

func (f *fakeBackend) ReadBase(ctx context.Context, path string) (firejail.Base, *firejail.Profile, error) {
	f.mu.Lock()
	gate, readErr := f.readGate, f.readErr[path]
	f.reads = append(f.reads, path)
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return firejail.Base{}, nil, ctx.Err()
		}
	}
	if readErr != nil {
		return firejail.Base{}, nil, readErr
	}
	return f.fj.ReadBase(ctx, path)
}

func (f *fakeBackend) ImportProfile(ctx context.Context, src string) (string, error) {
	f.mu.Lock()
	f.imported = append(f.imported, src)
	err := f.importErr
	f.mu.Unlock()
	if err != nil {
		return "", err
	}
	return f.fj.ImportProfile(ctx, src)
}

func (f *fakeBackend) BuildsForApp(_ context.Context, app apps.App) ([]store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	var out []store.Build
	for _, b := range f.builds {
		if BuildKey(b) == AppKey(app) {
			out = append(out, b)
		}
	}
	return out, f.buildsErr
}

func (f *fakeBackend) CreateBuild(_ context.Context, b store.Build) (store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return store.Build{}, f.createErr
	}
	for _, other := range f.builds {
		if other.SandboxName == b.SandboxName {
			return store.Build{}, errors.New(store.TagSandboxNameTaken, "taken", "sandboxName", b.SandboxName)
		}
	}
	var id int64
	for _, other := range f.builds {
		id = max(id, other.ID)
	}
	b.ID = id + 1
	f.builds = append(f.builds, b)
	f.saves = append(f.saves, "create "+b.SandboxName)
	return b, nil
}

func (f *fakeBackend) UpdateBuild(_ context.Context, b store.Build) (store.Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.builds {
		if f.builds[i].ID == b.ID {
			f.builds[i] = b
			return b, nil
		}
	}
	return store.Build{}, errors.New(store.TagBuildNotFound, "no build", "id", b.ID)
}

func (f *fakeBackend) SaveDraft(_ context.Context, id int64, d store.Draft) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	for i := range f.builds {
		if f.builds[i].ID == id {
			f.builds[i].Draft = &d
			f.saves = append(f.saves, fmt.Sprintf("draft %s", f.builds[i].SandboxName))
			return nil
		}
	}
	return errors.New(store.TagBuildNotFound, "no build", "id", id)
}

func (f *fakeBackend) ClearDraft(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.builds {
		if f.builds[i].ID == id {
			f.builds[i].Draft = nil
			f.saves = append(f.saves, fmt.Sprintf("clear %s", f.builds[i].SandboxName))
			return nil
		}
	}
	return errors.New(store.TagBuildNotFound, "no build", "id", id)
}

func (f *fakeBackend) NewBuildDefaults(_ context.Context, app apps.App) (firejail.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	taken := map[string]bool{}
	for _, b := range f.builds {
		taken[b.SandboxName] = true
	}
	s := newBuildDefaults(app, taken)
	s.IconSeed = 1234
	return s, nil
}

// savesList returns a copy of the saves log.
func (f *fakeBackend) savesList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.saves)
}

// buildNamed returns the stored build with the sandbox name.
func (f *fakeBackend) buildNamed(name string) (store.Build, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.builds {
		if b.SandboxName == name {
			return b, true
		}
	}
	return store.Build{}, false
}
