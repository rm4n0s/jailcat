package firejail

import (
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// LineKind says what a [GeneratedLine] is, so the Preview can highlight the lines of a
// setting (SPEC §11.1).
type LineKind string

// The kinds of generated lines.
const (
	// LineHeader is one of the two header lines of SPEC §9.
	LineHeader LineKind = "header"
	// LineQuiet is the "quiet" line, the first command.
	LineQuiet LineKind = "quiet"
	// LineSetting is a line a setting or a folder row writes.
	LineSetting LineKind = "setting"
	// LineRewritten is a line of the base profile (or an Extra rule) that an "ignore"
	// of the profile would also skip, written again before the ignores.
	LineRewritten LineKind = "rewritten"
	// LineIgnore is an "ignore" line that undoes a line of the base profile.
	LineIgnore LineKind = "ignore"
	// LineExtra is a row of the Extra rules tab.
	LineExtra LineKind = "extra"
	// LineInclude is the "include" of the base profile.
	LineInclude LineKind = "include"
	// LineComment is a section title.
	LineComment LineKind = "comment"
	// LineBlank is an empty line between sections.
	LineBlank LineKind = "blank"
)

// ProfileSpec is everything [Generate] needs to write the profile of a build.
type ProfileSpec struct {
	// BuildID is the id of the build, written in the header (SPEC §9).
	BuildID int64
	// BaseProfile is the absolute path of the base profile that the generated one
	// includes; "" for base None.
	BaseProfile string
	// Profile is the base chain as [Read] returned it; nil for base None.
	Profile *Profile
	// Base is the base value of every setting, with the lines each came from:
	// [BaseValues] of Profile, or [BaseNone].
	Base Base
	// Overrides are the user's changes from the base.
	Overrides Overrides
	// ExtraRules are the rows of the Extra rules tab.
	ExtraRules []string
	// Home is the user's home folder, to write the paths under it with ${HOME}.
	Home string
	// VSCode names the folders of the VS Code switches (SPEC §7.1); nil when the app is
	// not VS Code, which writes none of their lines.
	VSCode *VSCodeSpec
}

// GeneratedLine is one line of the generated profile and where it came from.
type GeneratedLine struct {
	// Text is the line, without a line ending.
	Text string
	// Field is the setting that caused the line; empty for structural lines, for the
	// lines of a folder row (see Folder) and for lines no single setting owns.
	Field Field
	// Folder is the path of the folder row that caused the line, as the row names it.
	Folder string
	// Kind says what the line is.
	Kind LineKind
}

// IgnoreCount says how many "ignore" entries the generated profile adds to firejail's
// limit of 32 (SPEC §8.1, §9.1), and where they come from.
type IgnoreCount struct {
	// Base is the number of ignore entries of the base chain.
	Base int
	// Extra is the number of ignore rows in the Extra rules.
	Extra int
	// Jailcat is the number of ignore lines Jailcat writes.
	Jailcat int
	// Fields splits Jailcat by the setting that needs them.
	Fields map[Field]int
	// Folders is the part of Jailcat that folder rows need.
	Folders int
	// Total is Base + Extra + Jailcat.
	Total int
}

// Generated is the profile text of a build, as lines.
type Generated struct {
	// Lines are the lines of the file, in order.
	Lines []GeneratedLine
	// IgnoreCount is how many ignore entries the profile adds.
	IgnoreCount IgnoreCount
}

// String returns the profile text; it ends with a newline.
func (g *Generated) String() string {
	var b strings.Builder
	for _, l := range g.Lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// WriteTo writes the profile text to w and returns the number of bytes written.
func (g *Generated) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, g.String())
	if err != nil {
		return int64(n), errors.NewErr(tagProfileWriteFailed, err)
	}
	return int64(n), nil
}

// generator holds the state while one profile is made.
type generator struct {
	spec  ProfileSpec
	maker lineMaker

	// final is what the user sees: the overrides on the base values. inc is what firejail
	// would do with only the include: the base values of a real base profile, or
	// firejail's own defaults for base None.
	final, inc Settings
	// sources and folderSources are the lines the include baseline came from; both are
	// empty for base None.
	sources       map[Field][]Line
	folderSources map[string][]Line

	chosenInc, chosenFinal bool

	// cancelled holds the keys of the base lines an ignore is meant to skip.
	cancelled map[string]bool
	ignores   []writtenIgnore
	ignoreAt  map[string]int

	settings []GeneratedLine
}

// writtenIgnore is one "ignore" line Jailcat writes.
type writtenIgnore struct {
	text   string
	field  Field
	folder string
	// all is set when every base line the entry matches is meant to go (mkdir, mkfile).
	all bool
}

// extraRow is a row of the Extra rules in the form it is written.
type extraRow struct {
	// text is the normalized row.
	text string
	// inner is the row without its "?CONDITION:" prefixes: the text firejail checks
	// against the ignore list.
	inner string
}

// maxIgnores is firejail's limit of ignore entries (MAX_PROFILE_IGNORE).
const maxIgnores = maxProfileIgnore

// Generate turns a build (base profile, overrides and Extra rules) into the profile text
// of SPEC §9.1, and says which lines come from which setting.
//
// What the user sees is the overrides on the base values. What firejail would do with
// only the include is the include baseline: the base values of a real base profile, and
// firejail's own defaults for base None, so that with None Jailcat's security defaults
// are written as lines (SPEC §7.5). For every field whose final value differs from the
// include baseline, every base line of the field is cancelled with an "ignore", and the
// lines of the final value are written. The positive lines come before the ignores, so
// no ignore can skip a line Jailcat wrote; a base line that an ignore would also skip
// without being meant to go is written again before the ignores (SPEC §9.1).
//
// Generate does not check what [Validate] checks; it refuses what it cannot write
// faithfully: a value that [profileValue] refuses, a limit out of firejail's range, a
// home that would not be restricted, a base line that no ignore can remove, and more
// than 32 ignore entries.
//
// Conditional lines: the lines Jailcat writes are read before the base chain, so they
// can change what a "?HAS_NET:", "?HAS_NODBUS:", "?HAS_NOSOUND:", "?HAS_PRIVATE:" or
// "?HAS_X11:" line of the chain sees. A conditional line that took effect when the base
// values were read, and whose state the lines of the build can change, is written again
// without its condition, so it keeps the effect the user sees in the form. A conditional
// line that was false in the base is not looked at again: if Jailcat's lines make its
// condition true, firejail applies it.
func Generate(spec ProfileSpec) (*Generated, error) {
	g := &generator{
		spec:          spec,
		sources:       spec.Base.Sources,
		folderSources: spec.Base.FolderSources,
		cancelled:     map[string]bool{},
		ignoreAt:      map[string]int{},
	}
	if home := strings.TrimSpace(spec.Home); home != "" {
		g.maker.home = filepath.Clean(home)
	}
	if (spec.BaseProfile == "") != (spec.Profile == nil) || (spec.BaseProfile != "" && !filepath.IsAbs(spec.BaseProfile)) {
		return nil, errors.New(tagSpecInvalid,
			"the base profile must be an absolute path with the profile read, or neither for base None",
			"base_profile", spec.BaseProfile, "has_profile", spec.Profile != nil)
	}

	g.final = spec.Overrides.Apply(spec.Base.Settings)
	if spec.Profile == nil {
		g.inc = firejailDefaults()
	} else {
		g.inc = spec.Base.Settings.Clone()
	}
	g.chosenInc = g.inc.Home == HomeChosen
	g.chosenFinal = g.final.Home == HomeChosen

	if err := g.home(); err != nil {
		return nil, err
	}
	if err := g.rows(); err != nil {
		return nil, err
	}
	if err := g.fields(); err != nil {
		return nil, err
	}
	g.quiet()
	extras, err := g.extraRows()
	if err != nil {
		return nil, err
	}
	rewritten := g.rewritten(extras)
	count, err := g.countIgnores(extras)
	if err != nil {
		return nil, err
	}
	include := ""
	if spec.BaseProfile != "" {
		if include, err = g.maker.path(spec.BaseProfile); err != nil {
			return nil, err
		}
	}
	return g.assemble(extras, rewritten, include, count), nil
}

// lineKey identifies a base line: the file and the line number, and the text, which
// tells apart the lines firejail made from one line (Origin).
func lineKey(l Line) string {
	return l.File + "\x00" + strconv.Itoa(l.Number) + "\x00" + l.Text + "\x00" + l.Origin
}

// effectiveText is the text an ignore sees for a line: a line firejail made from
// another one lives and dies with it.
func effectiveText(l Line) string {
	if l.Origin != "" {
		return l.Origin
	}
	return l.Text
}

// isLocked reports whether no ignore can remove the line: profile_read turns
// "whitelist-ro X" into two lines before it looks at the ignore list (SPEC §8.1).
func isLocked(l Line) bool {
	return strings.HasPrefix(l.Origin, "whitelist-ro ")
}

// emit adds a positive line for a field or a folder row.
func (g *generator) emit(f Field, folder, text string) error {
	if len(text) >= maxReadLine-1 {
		return errors.New(tagLineTooLong, "a profile line is longer than firejail reads in one piece",
			"field", string(f), "folder", folder, "length", len(text), "max", maxReadLine-1)
	}
	g.settings = append(g.settings, GeneratedLine{Text: text, Field: f, Folder: folder, Kind: LineSetting})
	return nil
}

// addIgnore records an "ignore" line; the same text is written once.
func (g *generator) addIgnore(text string, f Field, folder string, all bool) {
	if i, ok := g.ignoreAt[text]; ok {
		g.ignores[i].all = g.ignores[i].all || all
		return
	}
	g.ignoreAt[text] = len(g.ignores)
	g.ignores = append(g.ignores, writtenIgnore{text: text, field: f, folder: folder, all: all})
}

// cancel writes an "ignore" for each of the lines, for the field or folder row that
// changes. A line firejail made from another one is cancelled through that one.
func (g *generator) cancel(f Field, folder string, lines []Line) error {
	for _, l := range lines {
		if isLocked(l) {
			return errors.New(tagLineUncancellable,
				"the base profile reads this line from a whitelist-ro line, which no ignore can remove",
				"line", l.Text, "origin", l.Origin, "file", l.File, "number", l.Number,
				"field", string(f), "folder", folder)
		}
		g.cancelled[lineKey(l)] = true
		g.addIgnore(effectiveText(l), f, folder, false)
	}
	return nil
}

// home writes the lines of the home choice (SPEC §7.1, §9.1): Empty and Own private
// home write "private", the others write nothing here (the folder rows write the
// whitelist lines of "Only the folders I choose"), and a base that restricts the home is
// undone when the choice leaves it.
func (g *generator) home() error {
	if !slices.Contains([]HomeMode{HomeFull, HomeChosen, HomeEmpty, HomePrivate}, g.final.Home) {
		return errors.New(tagModeInvalid, "the home choice is not one of the four",
			"field", string(FieldHome), "value", string(g.final.Home))
	}
	changed := g.final.Home != g.inc.Home ||
		(g.final.Home == HomePrivate && g.maker.canon(g.final.PrivateDir) != g.maker.canon(g.inc.PrivateDir))
	if !changed && g.final.Home != HomePrivate {
		return nil
	}

	homeLines := slices.Concat(g.sources[FieldHome], g.sources[FieldPrivateDir])
	switch g.final.Home {
	case HomeFull:
		return g.cancel(FieldHome, "", homeLines)
	case HomeChosen:
		// Only the private lines leave: the whitelist lines are what chooses.
		return g.cancel(FieldHome, "", slices.DeleteFunc(slices.Clone(homeLines), func(l Line) bool {
			return l.Command != "private"
		}))
	}
	if changed {
		if err := g.cancel(FieldHome, "", homeLines); err != nil {
			return err
		}
	}
	if g.final.Home == HomeEmpty {
		return g.emit(FieldHome, "", "private")
	}
	// HomePrivate. The man page's workaround for firejail's bug (#903): without these
	// two ignores the "mkdir" and "mkfile" lines of the base profile make folders and
	// files in the real home. With base None there is no base profile to undo.
	if g.spec.Profile != nil {
		g.addIgnore("mkdir", FieldHome, "", true)
		g.addIgnore("mkfile", FieldHome, "", true)
	}
	if !changed {
		return nil
	}
	lines, err := g.maker.fieldLines(FieldPrivateDir, &g.final)
	if err != nil {
		return err
	}
	for _, text := range lines {
		if err := g.emit(FieldPrivateDir, "", text); err != nil {
			return err
		}
	}
	return nil
}

// rows writes the lines of the folder rows and of the Downloads toggle, which is a row
// for ${DOWNLOADS}. A row is written again only when its access changed, or when the
// home choice changed whether it needs a "whitelist" line.
func (g *generator) rows() error {
	if g.final.DownloadsReadWrite && (g.final.Home == HomeEmpty || g.final.Home == HomePrivate) {
		return errors.New(tagDownloadsHome,
			"the Downloads folder is in the real home, which an empty or private home keeps out of the sandbox")
	}
	err := g.row(FieldDownloadsReadWrite, "", xdg.MacroDownloads,
		g.inc.DownloadsReadWrite, AccessReadWrite, g.final.DownloadsReadWrite, AccessReadWrite,
		g.sources[FieldDownloadsReadWrite])
	if err != nil {
		return err
	}

	finalRows := map[string]Folder{}
	var order []string
	for _, f := range g.final.Folders {
		if _, dup := finalRows[f.Path]; dup {
			return errors.New(tagFolderDuplicate, "a folder is in the list twice", "path", f.Path)
		}
		finalRows[f.Path] = f
		order = append(order, f.Path)
	}
	incRows := map[string]Folder{}
	for _, f := range g.inc.Folders {
		incRows[f.Path] = f
		if _, ok := finalRows[f.Path]; !ok {
			order = append(order, f.Path)
		}
	}
	for _, path := range order {
		fin, finOK := finalRows[path]
		inc, incOK := incRows[path]
		if err := g.row("", path, path, incOK, inc.Access, finOK, fin.Access, g.folderSources[path]); err != nil {
			return err
		}
	}
	if err := g.vscode(); err != nil {
		return err
	}
	return g.checkChosen()
}

// row writes one folder row (or the Downloads toggle). baseLines are the lines of the
// include baseline that make the row.
func (g *generator) row(f Field, folder, path string, baseOK bool, baseAccess Access, finalOK bool, finalAccess Access, baseLines []Line) error {
	touched := slices.ContainsFunc(baseLines, func(l Line) bool { return g.cancelled[lineKey(l)] })
	if baseOK && finalOK && baseAccess == finalAccess && g.chosenInc == g.chosenFinal && !touched {
		return nil
	}

	var want []string
	var text string
	if finalOK {
		var err error
		if want, err = g.maker.rowCommands(finalAccess, g.chosenFinal); err != nil {
			return err
		}
		if text, err = g.maker.path(path); err != nil {
			return err
		}
	}
	have := map[string]bool{}
	var drop []Line
	for _, l := range baseLines {
		if g.cancelled[lineKey(l)] {
			continue
		}
		have[l.Command] = true
		if !slices.Contains(want, l.Command) {
			drop = append(drop, l)
		}
	}
	for _, c := range want {
		if have[c] {
			continue
		}
		if err := g.emit(f, folder, c+" "+text); err != nil {
			return err
		}
	}
	return g.cancel(f, folder, drop)
}

// checkChosen refuses "Only the folders I choose" when nothing would be whitelisted
// under the home: firejail would then not restrict the home at all.
func (g *generator) checkChosen() error {
	if !g.chosenFinal {
		return nil
	}
	for _, l := range g.settings {
		cmd, arg := splitCommand(l.Text)
		if cmd == "whitelist" && inHome(arg, g.maker.home, true) {
			return nil
		}
	}
	for _, l := range g.sources[FieldHome] {
		if l.Command == "whitelist" && !g.cancelled[lineKey(l)] {
			return nil
		}
	}
	return errors.New(tagHomeNotRestricted,
		"the home is set to only the folders you choose, but no folder in the home is allowed, so the home would not be restricted")
}

// genericField reports whether a field is written by the one rule for every field. The
// home choice, the private folder and Downloads have their own (home and rows), the
// VS Code switches are written with the rows ([generator.vscode]), and "quiet" is
// written first.
func genericField(f Field) bool {
	switch f {
	case FieldHome, FieldPrivateDir, FieldDownloadsReadWrite, FieldQuiet,
		FieldVSCodeOwnData, FieldVSCodeShareSettings, FieldVSCodeShellFiles:
		return false
	}
	return true
}

// fields writes every other setting whose final value differs from the include
// baseline: the base lines of the field are cancelled, then the lines of the final value
// are written.
func (g *generator) fields() error {
	changed := map[Field]bool{}
	byKey := map[string][]Field{}
	for _, f := range Fields() {
		if !genericField(f) {
			continue
		}
		if !f.Equal(g.inc, g.final) {
			changed[f] = true
		}
		for _, l := range g.sources[f] {
			byKey[lineKey(l)] = append(byKey[lineKey(l)], f)
		}
	}
	// A line that serves two fields ("nodbus" sets both buses) goes when either changes,
	// so the other field is written again from its final value.
	for grew := true; grew; {
		grew = false
		for f := range changed {
			for _, l := range g.sources[f] {
				for _, other := range byKey[lineKey(l)] {
					if !changed[other] {
						changed[other] = true
						grew = true
					}
				}
			}
		}
	}
	for _, f := range Fields() {
		if !changed[f] {
			continue
		}
		if err := g.cancel(f, "", g.sources[f]); err != nil {
			return err
		}
		lines, err := g.maker.fieldLines(f, &g.final)
		if err != nil {
			return err
		}
		for _, text := range lines {
			if err := g.emit(f, "", text); err != nil {
				return err
			}
		}
	}
	return nil
}

// quiet undoes the "quiet" of the base chain when the build wants firejail's messages.
func (g *generator) quiet() {
	if !g.final.Quiet && g.spec.Profile != nil && g.spec.Profile.Quiet {
		g.addIgnore("quiet", FieldQuiet, "", true)
	}
}

// extraRows normalizes the rows of the Extra rules. A row with a new line or a NUL would
// lose its end silently when normalized, so it is an error.
func (g *generator) extraRows() ([]extraRow, error) {
	var out []extraRow
	for i, raw := range g.spec.ExtraRules {
		if reason := rawRowReason(raw); reason != "" {
			return nil, errors.New(tagExtraRowInvalid, "an Extra rules row cannot be written: it has "+reason,
				"row", i+1, "reason", reason)
		}
		text := Normalize(raw)
		if text == "" {
			continue
		}
		out = append(out, extraRow{text: text, inner: stripConditionals(text)})
	}
	return out, nil
}

// stripConditionals removes the "?CONDITION:" prefixes of a normalized line.
func stripConditionals(text string) string {
	for strings.HasPrefix(text, "?") {
		_, rest, ok := strings.Cut(text, ":")
		if !ok {
			return text
		}
		text = strings.TrimPrefix(rest, " ")
	}
	return text
}

// rawRowReason says why a raw row cannot be normalized without losing something: a new
// line, a carriage return or a NUL cuts the row there.
func rawRowReason(raw string) string {
	switch {
	case strings.ContainsAny(raw, "\n\r"):
		return "a new line"
	case strings.ContainsRune(raw, 0):
		return "a NUL character"
	}
	return ""
}

// rewritten finds the lines that an ignore of the profile would skip without being meant
// to, and returns them to write again before the ignores (SPEC §9.1). They are the
// lines of the base chain, and the Extra rules, which come after the ignores.
func (g *generator) rewritten(extras []extraRow) []GeneratedLine {
	var out []GeneratedLine
	seen := map[string]bool{}
	for _, l := range g.settings {
		seen[l.Text] = true
	}
	add := func(text string, f Field, folder string) {
		if !seen[text] {
			seen[text] = true
			out = append(out, GeneratedLine{Text: text, Field: f, Folder: folder, Kind: LineRewritten})
		}
	}

	owner := map[string]GeneratedLine{}
	for f, lines := range g.sources {
		for _, l := range lines {
			owner[lineKey(l)] = GeneratedLine{Field: f}
		}
	}
	for path, lines := range g.folderSources {
		for _, l := range lines {
			if _, ok := owner[lineKey(l)]; !ok {
				owner[lineKey(l)] = GeneratedLine{Folder: path}
			}
		}
	}

	if g.spec.Profile != nil {
		touched := g.touchedStates(extras)
		for _, l := range g.spec.Profile.Lines {
			if isLocked(l) || g.cancelled[lineKey(l)] {
				continue
			}
			text := effectiveText(l)
			// A conditional line that took effect on the state the base values were read
			// with must keep taking effect, whatever Jailcat's lines do to that state:
			// it is written without its condition.
			if slices.ContainsFunc(strings.Split(l.Conditional, ","), func(c string) bool { return touched[c] }) {
				o := owner[lineKey(l)]
				add(text, o.Field, o.Folder)
				continue
			}
			matched, meant := false, false
			for _, e := range g.ignores {
				if Matches(e.text, text) {
					matched = true
					meant = meant || e.all
				}
			}
			if matched && !meant {
				o := owner[lineKey(l)]
				add(conditionalPrefix(l)+text, o.Field, o.Folder)
			}
		}
	}
	for _, x := range extras {
		if cmd, _ := splitCommand(x.inner); cmd == "include" {
			continue
		}
		for _, e := range g.ignores {
			if Matches(e.text, x.inner) {
				add(x.text, "", "")
				break
			}
		}
	}
	return out
}

// touchedStates says which conditions of profile.c (HAS_NET, HAS_NODBUS, HAS_NOSOUND,
// HAS_PRIVATE, HAS_X11) the lines Jailcat writes before the base profile can change:
// the positive lines, the ignores and the Extra rules that name the command the
// condition looks at. The other conditions come from firejail.config and do not change.
func (g *generator) touchedStates(extras []extraRow) map[string]bool {
	touched := map[string]bool{}
	note := func(text string) {
		cmd, args := splitCommand(text)
		switch cmd {
		case "net":
			touched["HAS_NET"] = true
		case "nodbus", "dbus-user", "dbus-system":
			touched["HAS_NODBUS"] = true
		case "nosound":
			touched["HAS_NOSOUND"] = true
		case "private", "private-home":
			touched["HAS_PRIVATE"] = true
		case "x11":
			if args == "none" || args == "xorg" {
				touched["HAS_X11"] = true
			}
		}
	}
	for _, l := range g.settings {
		note(l.Text)
	}
	for _, e := range g.ignores {
		note(e.text)
	}
	for _, x := range extras {
		if rest, ok := strings.CutPrefix(x.inner, "ignore "); ok {
			note(rest)
		} else {
			note(x.inner)
		}
	}
	return touched
}

// conditionalPrefix returns the "?CONDITION: " prefixes of a conditional line.
func conditionalPrefix(l Line) string {
	if l.Conditional == "" {
		return ""
	}
	var b strings.Builder
	for _, c := range strings.Split(l.Conditional, ",") {
		b.WriteString("?" + c + ": ")
	}
	return b.String()
}

// countIgnores counts the ignore entries the profile adds to firejail's limit of 32: the
// base chain's, Jailcat's and the Extra rules', and fails when they are more.
func (g *generator) countIgnores(extras []extraRow) (IgnoreCount, error) {
	c := IgnoreCount{Jailcat: len(g.ignores), Fields: map[Field]int{}}
	if g.spec.Profile != nil {
		c.Base = len(g.spec.Profile.Ignores)
	}
	for _, x := range extras {
		if cmd, _ := splitCommand(x.inner); cmd == "ignore" {
			c.Extra++
		}
	}
	for _, e := range g.ignores {
		if e.folder != "" {
			c.Folders++
		} else {
			c.Fields[e.field]++
		}
	}
	c.Total = c.Base + c.Extra + c.Jailcat
	if c.Total > maxIgnores {
		byField := map[string]int{}
		for f, n := range c.Fields {
			byField[string(f)] = n
		}
		return c, errors.New(tagIgnoreLimit, "the profile needs more ignore lines than firejail accepts",
			"total", c.Total, "max", maxIgnores, "base", c.Base, "extra", c.Extra,
			"jailcat", c.Jailcat, "folders", c.Folders, "fields", byField)
	}
	return c, nil
}

// assemble puts the lines in the order of SPEC §9.1.
func (g *generator) assemble(extras []extraRow, rewritten []GeneratedLine, include string, count IgnoreCount) *Generated {
	var lines []GeneratedLine
	for _, h := range Header(g.spec.BuildID) {
		lines = append(lines, GeneratedLine{Text: h, Kind: LineHeader})
	}
	if g.final.Quiet {
		lines = append(lines, GeneratedLine{Text: "quiet", Field: FieldQuiet, Kind: LineQuiet})
	}
	section := func(title string, body []GeneratedLine) {
		if len(body) == 0 {
			return
		}
		lines = append(lines, GeneratedLine{Kind: LineBlank}, GeneratedLine{Text: "# " + title, Kind: LineComment})
		lines = append(lines, body...)
	}

	section("Settings", slices.Concat(g.settings, rewritten))

	var ignoreLines []GeneratedLine
	for _, e := range g.ignores {
		ignoreLines = append(ignoreLines, GeneratedLine{Text: "ignore " + e.text, Field: e.field, Folder: e.folder, Kind: LineIgnore})
	}
	section("Undo base profile lines", ignoreLines)

	var extraLines []GeneratedLine
	for _, x := range extras {
		extraLines = append(extraLines, GeneratedLine{Text: x.text, Kind: LineExtra})
	}
	section("Extra rules", extraLines)

	if include != "" {
		section("Base profile", []GeneratedLine{{Text: "include " + include, Kind: LineInclude}})
	}
	return &Generated{Lines: lines, IgnoreCount: count}
}

const (
	tagSpecInvalid        = "ProfileSpecInvalid"
	tagProfileWriteFailed = "ProfileWriteFailed"
	tagLineUncancellable  = "IgnoreImpossible"
	tagIgnoreLimit        = "IgnoreLimitExceeded"
	tagHomeNotRestricted  = "HomeNotRestricted"
	tagDownloadsHome      = "DownloadsNeedRealHome"
	tagFolderDuplicate    = "FolderRowDuplicate"
	tagLineTooLong        = "ProfileLineTooLong"
	tagExtraRowInvalid    = "ExtraRowInvalid"
)
