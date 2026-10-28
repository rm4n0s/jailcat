package ui

import (
	"context"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
)

// build is the Build button (SPEC §11.3): it checks the Extra rules, makes the files off
// the UI thread, asks about the files that are in the way, and installs. It runs on the
// UI thread.
func (s *editorPageState) build() {
	s.outcome = nil
	s.checkThen(func(req BuildRequest) {
		s.busy = busyPreparing
		s.prepareThenBuild(req)
	})
}

// prepareThenBuild makes the files of the request and builds them, or asks about the
// conflicts first. It runs on the UI thread.
func (s *editorPageState) prepareThenBuild(req BuildRequest) {
	w := s.Widget()
	s.Go(func(ctx context.Context) {
		prepared, err := w.backend.PrepareBuild(ctx, req)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			if err != nil {
				s.fail("preparing the build", prepareFailedText(err), err)
				return
			}
			if len(prepared.Conflicts) > 0 {
				s.busy = busyNone
				s.askConflicts(prepared)
				return
			}
			s.runBuild(prepared, nil)
		})
	})
}

// askConflicts shows the files that are in the way and does what the user chooses. It
// runs on the UI thread.
func (s *editorPageState) askConflicts(prepared *PreparedBuild) {
	home := s.Widget().backend.HomeDir()
	result := m.ShowDialog[ConflictAnswer](s.Context(), func(m.BuildContext) m.Widget {
		return NewConflictsDialog(prepared.Conflicts, prepared.Owners, home)
	})
	s.Go(func(ctx context.Context) {
		select {
		case answer := <-result:
			s.SetState(func() {
				switch answer.Action {
				case ConflictOverwrite:
					s.runBuild(prepared, answer.Overwrite)
				case ConflictRename:
					s.tab = firejail.TabLauncher
					s.editor().FocusSandboxName()
				}
			})
		case <-ctx.Done():
		}
	})
}

// runBuild installs the prepared build off the UI thread. A Build that has begun is not
// cancelled when the page goes away: it ends either installed or not at all. It runs on
// the UI thread.
func (s *editorPageState) runBuild(prepared *PreparedBuild, allow map[string]bool) {
	w := s.Widget()
	s.busy = busyBuilding
	binding, onChanged := s.binding, w.o.OnBuildsChanged
	s.Go(func(ctx context.Context) {
		res, err := w.backend.Build(context.WithoutCancel(ctx), prepared, allow)
		if err == nil && onChanged != nil {
			binding.Post(onChanged)
		}
		s.SetState(func() {
			if err != nil {
				s.fail("building", buildFailedText(err), err)
				return
			}
			s.busy = busyNone
			s.editor().Built(res.Build)
			s.outcome = newBuiltOutcome(res, w.backend.HomeDir())
		})
	})
}

// uninstall is the Uninstall button (SPEC §13.2): it reads the files recorded for the
// build and asks what to do. It runs on the UI thread.
func (s *editorPageState) uninstall() {
	w := s.Widget()
	build := s.editor().Build()
	s.busy, s.outcome, s.actionFailure = busyUninstalling, nil, ""
	s.Go(func(ctx context.Context) {
		files, err := w.backend.InstalledFiles(ctx, build.ID)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			if err != nil {
				s.fail("reading the files of the build", "Jailcat could not read which files this build installed: "+plainReason(err)+".", err)
				return
			}
			s.busy = busyNone
			s.askUninstall(build, files)
		})
	})
}

// askUninstall shows the Uninstall dialog and runs what the user chose. It runs on the UI
// thread.
func (s *editorPageState) askUninstall(build store.Build, files []store.InstalledFile) {
	home := s.Widget().backend.HomeDir()
	result := m.ShowDialog[UninstallAnswer](s.Context(), func(m.BuildContext) m.Widget {
		return NewUninstallDialog(build.SandboxName, files, home)
	})
	s.Go(func(ctx context.Context) {
		select {
		case answer := <-result:
			if !answer.Chosen {
				return
			}
			s.SetState(func() {
				s.runUninstall(build, UninstallChoice{DeleteHome: answer.DeleteHome, DeleteAppData: answer.DeleteAppData, KeepSettings: answer.KeepSettings}, install.UninstallResult{})
			})
		case <-ctx.Done():
		}
	})
}

// runUninstall removes the files of the build off the UI thread. earlier is what a
// previous run of the same uninstall removed. It runs on the UI thread.
func (s *editorPageState) runUninstall(build store.Build, choice UninstallChoice, earlier install.UninstallResult) {
	w := s.Widget()
	s.busy = busyUninstalling
	binding, onChanged := s.binding, w.o.OnBuildsChanged
	s.Go(func(ctx context.Context) {
		res, err := w.backend.Uninstall(context.WithoutCancel(ctx), build, choice)
		res.Removed = append(append([]install.File(nil), earlier.Removed...), res.Removed...)
		res.HomeKept = !choice.DeleteHome && (earlier.HomeKept || res.HomeKept)
		res.AppDataKept = !choice.DeleteAppData && (earlier.AppDataKept || res.AppDataKept)
		done := err == nil && UninstallDone(res, choice)
		if done && onChanged != nil {
			binding.Post(onChanged)
		}
		s.SetState(func() {
			switch {
			case err != nil:
				s.fail("uninstalling", "Jailcat could not finish the uninstall: "+plainReason(err)+".", err)
			case !done:
				s.busy = busyNone
				s.askKept(build, choice, res)
			default:
				s.busy = busyNone
				s.uninstalled(build, choice, res)
			}
		})
	})
}

// askKept lists the edited files an uninstall left and removes them or leaves them as the
// user says. It runs on the UI thread.
func (s *editorPageState) askKept(build store.Build, choice UninstallChoice, res install.UninstallResult) {
	home := s.Widget().backend.HomeDir()
	result := m.ShowDialog[string](s.Context(), func(m.BuildContext) m.Widget { return NewKeptFilesDialog(res.Kept, home) })
	s.Go(func(ctx context.Context) {
		select {
		case answer := <-result:
			s.SetState(func() {
				if answer == RemoveKept {
					choice.Force = map[string]bool{}
					for _, f := range res.Kept {
						choice.Force[f.Path] = true
					}
				} else {
					choice.LeaveKept = true
				}
				s.runUninstall(build, choice, res)
			})
		case <-ctx.Done():
		}
	})
}

// uninstalled shows the end of an uninstall. A build whose settings are kept stays in the
// editor as not installed; otherwise there is nothing left to edit. It runs on the UI
// thread.
func (s *editorPageState) uninstalled(build store.Build, choice UninstallChoice, res install.UninstallResult) {
	w := s.Widget()
	if !choice.KeepSettings {
		if w.o.OnUninstalled != nil {
			w.o.OnUninstalled()
		}
		return
	}
	s.editor().SetInstalled(false)
	s.outcome = newUninstalledOutcome(build.SandboxName, res, w.backend.HomeDir())
}
