package firejail

import (
	"path/filepath"
	"slices"

	"github.com/rm4n0s/errors"
)

// vscode writes the lines of the VS Code switches (SPEC §7.1, §9.1) with the folder
// rows: the data folder and the three shared settings as Read-write rows, the shell
// startup files as Read-only rows and "tab". They are the same lines a folder row of
// that access writes, so "Only the folders I choose" whitelists them. A line the profile
// has already is not written twice.
func (g *generator) vscode() error {
	v := g.spec.VSCode
	if v == nil {
		return nil
	}
	if !isCleanAbs(v.DataDir) || !isCleanAbs(v.ConfigDir) {
		return errors.New(tagVSCodeSpecInvalid, "the VS Code data and config folders must be absolute, clean paths",
			"data_dir", v.DataDir, "config_dir", v.ConfigDir)
	}
	s := g.final

	if s.VSCodeDataOnDisk() {
		if err := g.vscodeRow(FieldVSCodeOwnData, v.DataDir, AccessReadWrite); err != nil {
			return err
		}
	}
	if s.VSCodeLinksSettings() {
		for _, name := range VSCodeSharedNames() {
			if err := g.vscodeRow(FieldVSCodeShareSettings, filepath.Join(v.ConfigDir, "User", name), AccessReadWrite); err != nil {
				return err
			}
		}
	}
	if s.VSCodeShowsShellFiles() {
		if err := g.emit(FieldVSCodeShellFiles, "", "tab"); err != nil {
			return err
		}
		for _, name := range VSCodeShellFiles() {
			if err := g.vscodeRow(FieldVSCodeShellFiles, "${HOME}/"+name, AccessReadOnly); err != nil {
				return err
			}
		}
	}
	return nil
}

// vscodeRow writes the lines of one folder row of the VS Code switch f.
func (g *generator) vscodeRow(f Field, path string, access Access) error {
	text, err := g.maker.path(path)
	if err != nil {
		return err
	}
	cmds, err := g.maker.rowCommands(access, g.chosenFinal)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		line := c + " " + text
		if slices.ContainsFunc(g.settings, func(l GeneratedLine) bool { return l.Text == line }) {
			continue
		}
		if err := g.emit(f, "", line); err != nil {
			return err
		}
	}
	return nil
}

const tagVSCodeSpecInvalid = "VSCodeSpecInvalid"
