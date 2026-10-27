package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// fullDirs completes the folders of the firejail fixture with the ones a Build writes to,
// all in temporary folders under the same home.
func fullDirs(t *testing.T, base xdg.Dirs) xdg.Dirs {
	t.Helper()
	env := xdg.NewEnv(base.Home, map[string]string{"PATH": "/usr/bin:/bin", "LANG": "C"})
	dirs, err := xdg.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	dirs.FirejailSystem, dirs.FirejailUser, dirs.Imported = base.FirejailSystem, base.FirejailUser, base.Imported
	return dirs
}

// fakeSession is a Try that a test drives: it emits lines and ends when told to.
type fakeSession struct {
	lines chan string
	done  chan struct{}

	mu     sync.Mutex
	status firejail.ExitStatus
	err    error
	nested bool
	stops  int
	ended  bool
}

// newFakeSession returns a session that has not started to emit.
func newFakeSession() *fakeSession {
	return &fakeSession{lines: make(chan string, 4096), done: make(chan struct{})}
}

// emit sends a line of output.
func (s *fakeSession) emit(line string) { s.lines <- line }

// finish ends the session with the status; the lines channel closes.
func (s *fakeSession) finish(status firejail.ExitStatus, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.ended = true
	s.status, s.err = status, err
	close(s.lines)
	close(s.done)
}

func (s *fakeSession) Lines() <-chan string { return s.lines }

func (s *fakeSession) Wait() (firejail.ExitStatus, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.err
}

// Stop ends the session as firejail does when it is told to shut down.
func (s *fakeSession) Stop(context.Context) error {
	s.mu.Lock()
	s.stops++
	s.mu.Unlock()
	s.finish(firejail.ExitStatus{Code: -1, Signal: "terminated"}, nil)
	return nil
}

func (s *fakeSession) Nested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nested
}

// stopCount is how many times Stop was called.
func (s *fakeSession) stopCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stops
}

func (f *fakeBackend) PrepareBuild(ctx context.Context, req BuildRequest) (*PreparedBuild, error) {
	f.mu.Lock()
	f.prepareAsks = append(f.prepareAsks, req)
	err, conflicts, owners, info := f.prepareErr, f.conflicts, f.owners, f.firejail
	// An icon of the fake is found by its path, as a desktop entry with an absolute Icon=.
	icon, known := f.icons[req.App.Icon]
	f.mu.Unlock()
	if known {
		req.App.Icon = icon.Path
	}
	if err != nil {
		return nil, err
	}
	if info.Missing {
		return nil, errors.New(firejail.TagFirejailNotFound, "firejail is not installed in any folder of the path")
	}
	gen, prepared, err := f.fj.generate(ctx, req, info.Path)
	if err != nil {
		return nil, err
	}
	return &PreparedBuild{
		Request: req, Generated: gen, Prepared: prepared, Firejail: info.Path,
		Conflicts: conflicts, Owners: owners,
	}, nil
}

func (f *fakeBackend) Build(ctx context.Context, p *PreparedBuild, allow map[string]bool) (BuildResult, error) {
	f.mu.Lock()
	gate := f.buildGate
	f.buildAsks = append(f.buildAsks, allow)
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return BuildResult{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.buildErr != nil {
		return BuildResult{}, f.buildErr
	}
	row := buildRowOf(p.Request)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	row.BuiltAt, row.Installed, row.FirejailVersion = &now, true, f.firejail.Version
	if row.ID == 0 {
		var id int64
		for _, other := range f.builds {
			id = max(id, other.ID)
		}
		row.ID = id + 1
		f.builds = append(f.builds, row)
	} else {
		for i := range f.builds {
			if f.builds[i].ID == row.ID {
				row.Draft = nil
				f.builds[i] = row
			}
		}
	}
	files := make([]install.File, len(p.Prepared.Files))
	for i, file := range p.Prepared.Files {
		file.Content = nil
		files[i] = file
	}
	f.saves = append(f.saves, "build "+row.SandboxName)
	return BuildResult{Build: row, Files: files, Check: f.checkResult, CheckErr: f.checkErr, IconFallback: f.iconFallback}, nil
}

func (f *fakeBackend) Try(_ context.Context, req BuildRequest) (TrySession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tries = append(f.tries, req)
	if f.tryErr != nil {
		return nil, f.tryErr
	}
	if f.session == nil {
		f.session = newFakeSession()
	}
	return f.session, nil
}

func (f *fakeBackend) Uninstall(_ context.Context, build store.Build, choice UninstallChoice) (install.UninstallResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uninstallAsks = append(f.uninstallAsks, choice)
	if f.uninstallErr != nil {
		return install.UninstallResult{}, f.uninstallErr
	}
	var res install.UninstallResult
	if n := len(f.uninstallResults); n > 0 {
		i := min(len(f.uninstallAsks)-1, n-1)
		res = f.uninstallResults[i]
	}
	if !UninstallDone(res, choice) {
		return res, nil
	}
	for i := range f.builds {
		if f.builds[i].ID != build.ID {
			continue
		}
		if choice.KeepSettings {
			f.builds[i].Installed = false
		} else {
			f.builds = append(f.builds[:i], f.builds[i+1:]...)
		}
		break
	}
	f.saves = append(f.saves, "uninstall "+build.SandboxName)
	return res, nil
}

func (f *fakeBackend) InstalledFiles(context.Context, int64) ([]store.InstalledFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installedFiles, nil
}

func (f *fakeBackend) BaseProfileMissing(ctx context.Context, path string) bool {
	return f.fj.BaseProfileMissing(ctx, path)
}
