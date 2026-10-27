package ui

import (
	"path/filepath"
	"slices"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// SetSandboxName gives the build a new sandbox name (SPEC §7.8). The name is not checked
// here: the Launcher tab checks it before it calls this. An Own private home that is at
// the default place, a folder of the private homes named like the sandbox, moves with the
// name (SPEC §7.1); a folder that comes from the base profile, or one the user chose, stays.
func (e *Editor) SetSandboxName(name string) {
	follows := e.PrivateHomeFollowsName()
	e.Set(func(s *firejail.Settings) {
		s.SandboxName = name
		if follows && name != "" {
			s.PrivateDir = filepath.Join(e.homesDir, name)
		}
	})
}

// PrivateHomeFollowsName reports whether the build has an Own private home at the default
// place, a folder of the private homes named like the sandbox: the home then moves when
// the sandbox is renamed.
func (e *Editor) PrivateHomeFollowsName() bool {
	return e.final.Home == firejail.HomePrivate && e.homesDir != "" && e.final.SandboxName != "" &&
		e.final.PrivateDir == filepath.Join(e.homesDir, e.final.SandboxName)
}

// RenamedFrom returns the sandbox name an installed build has now when the editor gives
// it another one: Build then removes the files under the old name and installs the
// build under the new one (SPEC §13.1). It is "" for a build that is not stored, not
// installed, or not renamed.
func (e *Editor) RenamedFrom() string {
	if e.build.ID == 0 || !e.build.Installed || e.build.SandboxName == "" || e.build.SandboxName == e.final.SandboxName {
		return ""
	}
	return e.build.SandboxName
}

// SetExtraProblems keeps what the check of the Extra rules found for rows (the answer of
// [Backend.CheckExtraRules]). The errors in it make [Editor.Problems] list them for as
// long as the rows are the editor's rows; once the rows change, the answer is out of
// date and is ignored until a new one comes.
func (e *Editor) SetExtraProblems(rows []string, problems []firejail.Problem) {
	e.extraDone = true
	e.extraRows = slices.Clone(rows)
	e.extraProblems = slices.Clone(problems)
}

// ExtraRulesChecked reports whether the last [Editor.SetExtraProblems] was made for the
// rows the editor has now. Build checks the rows again when it is false.
func (e *Editor) ExtraRulesChecked() bool {
	return e.extraDone && slices.Equal(e.extraRows, e.extra)
}

// ExtraProblems returns the problems of the Extra rules, errors and warnings, when
// [Editor.ExtraRulesChecked]; else none.
func (e *Editor) ExtraProblems() []firejail.Problem {
	if !e.ExtraRulesChecked() {
		return nil
	}
	return slices.Clone(e.extraProblems)
}
