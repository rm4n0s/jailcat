package ui

import (
	"context"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/store"
)

// selectApp is the user picking an app of the list (SPEC §5). The unsaved changes of
// the editor on screen are kept as a draft first; then the app's build is opened, or a
// new one. It runs on the UI thread.
func (s *homeState) selectApp(app apps.App) {
	if s.selected != nil && AppKey(*s.selected) == AppKey(app) {
		return
	}
	var plan DraftPlan
	if s.editor != nil {
		plan = s.editor.DraftPlan()
	}
	s.selected = &app
	s.editor, s.openErr, s.notice = nil, "", ""
	s.beginOpening()
	gen := s.openGen
	openCtx := s.openContext()

	wait := s.lastSave
	done := make(chan struct{})
	s.lastSave = done
	backend, logger := s.Widget().backend, s.Widget().logger
	s.Go(func(ctx context.Context) {
		defer close(done)
		if wait != nil {
			select {
			case <-wait:
			case <-ctx.Done():
				return
			}
		}
		var saveErr error
		if plan.Action != DraftNothing {
			// The draft is written even when the user has moved on to a third app.
			if saveErr = plan.Run(ctx, backend); saveErr != nil {
				logger.Error("keeping the draft failed", "app", app.ID, "err", saveErr)
			}
		}
		all, listErr := backend.ListBuilds(ctx)
		var missing map[string]bool
		if listErr != nil {
			logger.Error("reading the builds failed", "err", listErr)
		} else {
			missing = missingBaseApps(ctx, backend, all)
		}
		builds, err := backend.BuildsForApp(openCtx, app)
		if openCtx.Err() != nil || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Error("reading the builds of the app failed", "app", app.ID, "err", err)
		}
		s.SetState(func() {
			if listErr == nil {
				s.setBuilds(all, missing)
				s.revision++
			}
			if saveErr != nil {
				s.notice = draftFailedText
			}
			if s.openGen != gen {
				return
			}
			if err != nil {
				s.opening, s.openErr = false, openFailedText
				return
			}
			switch len(builds) {
			case 0:
				s.loadEditor(app, nil, gen)
			case 1:
				s.loadEditor(app, &builds[0], gen)
			default:
				s.chooseBuild(app, builds, gen)
			}
		})
	})
}

// beginOpening starts a new opening: the answer of the one that ran is dropped.
func (s *homeState) beginOpening() {
	if s.cancelOpen != nil {
		s.cancelOpen()
		s.cancelOpen = nil
	}
	s.openGen++
	s.opening = true
}

// openContext returns the context of the reads of the opening that began last.
func (s *homeState) openContext() context.Context {
	ctx, cancel := context.WithCancel(s.Lifetime())
	s.cancelOpen = cancel
	return ctx
}

// chooseBuild asks which of the builds of the app to open, or for a new one. It runs on
// the UI thread.
func (s *homeState) chooseBuild(app apps.App, builds []store.Build, gen int) {
	result := showBuildChooser(s.Context(), app, builds)
	s.Go(func(ctx context.Context) {
		select {
		case choice := <-result:
			s.SetState(func() {
				if s.openGen != gen {
					return
				}
				switch {
				case !choice.Chosen:
					// Closed without a choice: back to the empty state.
					s.selected, s.opening = nil, false
				case choice.New:
					s.loadEditor(app, nil, gen)
				default:
					s.loadEditor(app, &choice.Build, gen)
				}
			})
		case <-ctx.Done():
		}
	})
}

// loadEditor opens the editor of the build (nil for a new one) off the UI thread. It
// runs on the UI thread.
func (s *homeState) loadEditor(app apps.App, build *store.Build, gen int) {
	s.opening = true
	openCtx := s.cancelOpenContext()
	backend, logger := s.Widget().backend, s.Widget().logger
	s.Go(func(ctx context.Context) {
		e, err := OpenEditor(openCtx, backend, logger, app, build)
		if openCtx.Err() != nil || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Error("opening the editor failed", "app", app.ID, "err", err)
		}
		s.SetState(func() {
			if s.openGen != gen {
				return
			}
			s.opening = false
			if err != nil {
				s.openErr = openFailedText
				return
			}
			s.editor = e
		})
	})
}

// cancelOpenContext replaces the context of the reads with a fresh one, without
// starting a new opening.
func (s *homeState) cancelOpenContext() context.Context {
	if s.cancelOpen != nil {
		s.cancelOpen()
	}
	return s.openContext()
}

// discardDraft throws the draft of the open build away and opens the build again from
// what is stored. It runs on the UI thread.
func (s *homeState) discardDraft() {
	e := s.editor
	if e == nil || e.Build().ID == 0 {
		return
	}
	app, id := e.App(), e.Build().ID
	s.SetState(func() {
		s.editor, s.notice = nil, ""
		s.beginOpening()
		gen := s.openGen
		openCtx := s.openContext()
		backend, logger := s.Widget().backend, s.Widget().logger
		s.Go(func(ctx context.Context) {
			err := backend.ClearDraft(openCtx, id)
			var builds []store.Build
			if err == nil {
				builds, err = backend.BuildsForApp(openCtx, app)
			}
			var stored *store.Build
			for i := range builds {
				if builds[i].ID == id {
					stored = &builds[i]
				}
			}
			var opened *Editor
			if err == nil && stored != nil {
				opened, err = OpenEditor(openCtx, backend, logger, app, stored)
			}
			if openCtx.Err() != nil || ctx.Err() != nil {
				return
			}
			if err != nil {
				logger.Error("discarding the draft failed", "app", app.ID, "err", err)
			}
			s.SetState(func() {
				if s.openGen != gen {
					return
				}
				s.opening = false
				if err != nil || opened == nil {
					s.openErr = openFailedText
					return
				}
				s.editor = opened
			})
		})
	})
}
