package firejail

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

const (
	// maxIncludeLevel is MAX_INCLUDE_LEVEL of firejail.h: profile_read stops when the
	// include level is above it.
	maxIncludeLevel = 16
	// maxProfileIgnore is MAX_PROFILE_IGNORE of firejail.h.
	maxProfileIgnore = 32
	// metaChars are the characters reject_meta_chars (lib/common.c) refuses in a
	// profile file name when globbing is off.
	metaChars = "\\&!?\"<>%^{};,*[]"
)

// ReadOpts are the inputs of [Read] besides the profile path.
type ReadOpts struct {
	// Dirs gives the home folder ("~", "${HOME}"), firejail's system folder
	// ("${CFG}", the last place a bare include name is looked for) and its user
	// folder (the first place).
	Dirs xdg.Dirs
	// Config gives the values of the conditionals BROWSER_DISABLE_U2F,
	// BROWSER_ALLOW_DRM and ALLOW_TRAY. The zero value is all false: use
	// [DefaultConfig] or [ReadConfig] for firejail's real values.
	Config Config
	// Cmdline holds the entries of "--ignore=<entry>" options of the firejail command
	// line (the text after "ignore "). They are in the ignore list before the profile is
	// read and count toward the limit of 32.
	Cmdline []string
}

// Read reads the profile at path and everything it includes the way firejail 0.9.80
// does (profile.c: profile_read, profile_check_line), and returns the lines that take
// effect in order, with their file and line number, and the lines "ignore" removed.
//
// The checks run in firejail's order: a "?CONDITION:" prefix first, then the ignore
// list, then "ignore" itself, then the other commands. Unknown commands are kept as
// lines: Read only reads. Errors are the ones on which firejail exits.
func Read(ctx context.Context, path string, opts ReadOpts) (*Profile, error) {
	r := &reader{opts: opts, prof: &Profile{Path: path}}
	// main.c adds the --ignore options before it reads any profile.
	for _, entry := range opts.Cmdline {
		if err := r.addIgnore(entry, Line{Text: "ignore " + entry, Command: "ignore", Args: entry}); err != nil {
			return nil, err
		}
	}
	if _, err := r.readFile(ctx, path, nil, false); err != nil {
		return nil, err
	}
	return r.prof, nil
}

// ignoreEntry is one item of firejail's profile_ignore array.
type ignoreEntry struct {
	text string
	by   Line
}

// reader holds the state firejail keeps in globals while it reads profiles.
type reader struct {
	opts ReadOpts
	prof *Profile

	// ignores is cfg.profile_ignore of firejail.h, at most maxProfileIgnore entries.
	ignores []ignoreEntry

	// The flags below are the arg_* globals that the conditionals of profile.c read.
	// profile_check_line sets them while it reads, so they hold only what the lines read
	// so far did.

	// hasNet is arg_nonetwork || any_bridge_configured(). "net none" sets arg_nonetwork
	// and nothing clears it; "net <device>" configures a bridge, and a later "net none"
	// sets arg_nonetwork, so once either line took effect the condition stays true.
	hasNet bool
	// hasNoDBus is arg_dbus_user != DBUS_POLICY_ALLOW || arg_dbus_system !=
	// DBUS_POLICY_ALLOW. "nodbus" sets both policies to block, and "dbus-user" or
	// "dbus-system" with the value "filter" or "none" sets that policy to filter or
	// block. The policy is never set back to allow while profiles are read.
	hasNoDBus bool
	// hasNoSound is arg_nosound, set by "nosound".
	hasNoSound bool
	// hasPrivate is arg_private, set by "private", "private-home <list>" and
	// "private <dir>".
	hasPrivate bool
	// hasX11 is arg_x11_block || arg_x11_xorg (|| env FIREJAIL_X11, which Jailcat's
	// reader never has). "x11 none" sets the first and "x11 xorg" the second. The other
	// "x11" forms start a nested X server and make firejail exit, so they set nothing.
	hasX11 bool
	// allowDebuggers is arg_allow_debuggers, set by "allow-debuggers".
	allowDebuggers bool
}

// lineCtx is where a line was read.
type lineCtx struct {
	file   string
	number int
	chain  []string
}

func (c lineCtx) line(text string, conds []string) Line {
	cmd, args := splitCommand(text)
	return Line{
		Text: text, Command: cmd, Args: args, File: c.file, Number: c.number,
		Conditional: strings.Join(conds, ","), IncludedFrom: c.chain,
	}
}

// readFile is profile_read of profile.c. It returns "" when the file was read, and
// the reason when it was skipped ("missing", "allow-debuggers"). parent holds the
// files that include this one, outermost first; its length is firejail's include_level.
// notFound makes the file behave as one that does not exist, for a bare include name
// that no firejail folder holds (see readInclude).
func (r *reader) readFile(ctx context.Context, fname string, parent []string, notFound bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.NewErr(tagProfileReadCanceled, err).SetMetadata("file", fname)
	}

	// profile_read: exit if the maximum include level was reached.
	if len(parent) > maxIncludeLevel {
		// firejail has no cycle check: a cycle ends here with this error. The tag tells a
		// cycle (the file is already being read) from a legitimately deep chain.
		if slices.Contains(parent, filepath.Clean(fname)) || slices.Contains(parent, fname) {
			return "", errors.New(tagIncludeCycle, "the profile includes itself",
				"file", fname, "chain", parent)
		}
		return "", errors.New(tagIncludeTooDeep, "the profiles include each other too deeply",
			"file", fname, "max", maxIncludeLevel, "chain", parent)
	}

	// profile_read: invalid_filename(fname, 0), then the empty name and directory checks.
	if bad := invalidFilename(fname); bad != "" {
		return "", errors.New(tagProfilePathInvalid, "the profile file name has a character firejail rejects",
			"file", fname, "reason", bad)
	}
	if fname == "" {
		return "", errors.New(tagProfileReadFailed, "the profile file name is empty", "file", fname)
	}
	if !notFound {
		if st, err := os.Stat(fname); err == nil && st.IsDir() {
			return "", errors.New(tagProfileReadFailed, "the profile is a directory", "file", fname)
		}
	}

	// profile_read: access(fname, R_OK). "if the file ends in .local, do not exit":
	//   const char *ptr = strstr(base, ".local");
	//   if (ptr && strlen(ptr) == 6 && errsv != EACCES) return;
	// Any other file that cannot be accessed stops firejail with an error.
	var data []byte
	var readErr error
	if notFound {
		readErr = &fs.PathError{Op: "open", Path: fname, Err: fs.ErrNotExist}
	} else {
		data, readErr = os.ReadFile(fname)
	}
	if readErr != nil {
		if endsWithLocal(fname) && !errors.Is(readErr, fs.ErrPermission) {
			r.prof.Missing = append(r.prof.Missing, fname)
			return "missing", nil
		}
		return "", errors.NewErr(tagProfileReadFailed, readErr).SetMetadata("file", fname)
	}

	// profile_read: --allow-debuggers skips disable-devel.inc. (--appimage skips
	// disable-shell.inc, but Jailcat never runs with it: HAS_APPIMAGE is false.)
	if r.allowDebuggers && strings.Contains(fname, "/") && gnuBasename(fname) == "disable-devel.inc" {
		return "allow-debuggers", nil
	}

	chain := slices.Clone(parent)
	inner := append(slices.Clone(parent), fname)
	lineno := 0
	for _, chunk := range fgetsChunks(data) {
		lineno++
		text := Normalize(chunk)
		if text == "" {
			continue
		}
		c := lineCtx{file: fname, number: lineno, chain: chain}
		if err := r.readLine(ctx, text, c, inner); err != nil {
			return "", err
		}
	}
	return "", nil
}

// readLine handles one normalized line of a file: the checks of the loop in
// profile_read that come before profile_check_line.
func (r *reader) readLine(ctx context.Context, text string, c lineCtx, inner []string) error {
	// whitelist-ro X is rewritten into "whitelist X" and "read-only X" and added to the
	// list directly: neither the ignore list nor the conditionals see it.
	if rest, ok := strings.CutPrefix(text, "whitelist-ro "); ok {
		for _, cmd := range []string{"whitelist ", "read-only "} {
			l := c.line(cmd+rest, nil)
			l.Origin = text
			r.prof.Lines = append(r.prof.Lines, l)
		}
		return nil
	}

	// quiet: an "ignore quiet" entry turns quiet off, otherwise it turns it on (the
	// "arg_debug" case is for firejail's --debug, which Jailcat never has).
	if text == "quiet" {
		_, ignored := r.findIgnore(text)
		r.prof.Quiet = !ignored
		return nil
	}

	// include, unless the ignore list has it. An ignored include goes on to
	// profile_check_line, which records it as skipped.
	if strings.HasPrefix(text, "include ") {
		if _, ignored := r.findIgnore(text); !ignored {
			return r.readInclude(ctx, text, c, inner)
		}
	}

	return r.checkLine(text, c, nil)
}

// readInclude is the include branch of profile_read.
func (r *reader) readInclude(ctx context.Context, text string, c lineCtx, inner []string) error {
	line := c.line(text, nil)
	// expand macros in front of the include profile file
	newprofile, err := r.expandMacros(text[len("include "):])
	if err != nil {
		return err
	}

	r.prof.Includes = append(r.prof.Includes, Include{Line: line, Resolved: newprofile})
	idx := len(r.prof.Includes) - 1

	var skipped string
	if strings.Contains(newprofile, "/") {
		// a path with a / is read as it is
		skipped, err = r.readFile(ctx, newprofile, inner, false)
	} else {
		// profile path contains no / chars, do a search (profile_find_firejail): first
		// the user's folder, then the system one. profile_find reads the first directory
		// entry whose name is equal.
		found := ""
		for _, dir := range []string{r.opts.Dirs.FirejailUser, r.opts.Dirs.FirejailSystem} {
			if dir == "" {
				continue
			}
			if _, statErr := os.Lstat(filepath.Join(dir, newprofile)); statErr == nil {
				found = filepath.Join(dir, newprofile)
				break
			}
		}
		if found != "" {
			r.prof.Includes[idx].Resolved = found
			skipped, err = r.readFile(ctx, found, inner, false)
		} else {
			// profile.c: "maybe this is a file in the local working directory? it will stop
			// the sandbox if not! Note: if the file ends in .local it will not stop the
			// program". Jailcat does not look in a working directory it cannot know, so the
			// name is a file that does not exist.
			skipped, err = r.readFile(ctx, newprofile, inner, true)
		}
	}
	if err != nil {
		return err
	}
	r.prof.Includes[idx].Skipped = skipped
	return nil
}

// expandMacros is expand_macros of macros.c for an include path: "~", "${HOME}",
// "${CFG}" and "${RUNUSER}" at the start. "$HOME" is an error. The user-folder macros
// (${DOWNLOADS} and the like) only apply when they are the whole path, which names a
// folder and so can never be a profile file: left as written, the read fails the same
// way.
func (r *reader) expandMacros(path string) (string, error) {
	home := r.opts.Dirs.Home
	switch {
	case strings.HasPrefix(path, "$HOME"):
		return "", errors.New(tagHomeMacroNotAllowed,
			"$HOME is not allowed in profile files, use ${HOME}", "path", path)
	case strings.HasPrefix(path, "${HOME}"):
		return home + path[len("${HOME}"):], nil
	case strings.HasPrefix(path, "~"):
		return home + path[1:], nil
	case strings.HasPrefix(path, "${CFG}"):
		return r.opts.Dirs.FirejailSystem + path[len("${CFG}"):], nil
	case strings.HasPrefix(path, "${RUNUSER}"):
		return "/run/user/" + strconv.Itoa(os.Getuid()) + path[len("${RUNUSER}"):], nil
	}
	return path, nil
}

// invalidFilename is invalid_filename (macros.c) with reject_meta_chars (lib/common.c),
// globbing off. It returns why the name is rejected, or "". A leading ${HOME}, ${PATH}
// or ${RUNUSER} is not checked.
func invalidFilename(fname string) string {
	rest := fname
	for _, macro := range []string{"${HOME}", "${PATH}", "${RUNUSER}"} {
		if strings.HasPrefix(fname, macro) {
			rest = fname[len(macro):]
			break
		}
	}
	for _, ch := range []byte(rest) {
		if ch < 0x20 || ch == 0x7f {
			return "control character"
		}
	}
	if i := strings.IndexAny(rest, metaChars); i >= 0 {
		return "rejected character " + strconv.Quote(rest[i:i+1])
	}
	return ""
}

// endsWithLocal is the test in profile_read: the base name has ".local" at its first
// occurrence of the text, and nothing after it.
func endsWithLocal(fname string) bool {
	base := gnuBasename(fname)
	i := strings.Index(base, ".local")
	return i >= 0 && len(base)-i == len(".local")
}

// gnuBasename is gnu_basename of lib/common.c: what follows the last slash.
func gnuBasename(path string) string {
	return path[strings.LastIndexByte(path, '/')+1:]
}

// findIgnore is is_in_ignore_list of profile.c: the first entry that matches the line.
func (r *reader) findIgnore(text string) (ignoreEntry, bool) {
	for _, e := range r.ignores {
		if Matches(e.text, text) {
			return e, true
		}
	}
	return ignoreEntry{}, false
}

// addIgnore is profile_add_ignore of profile.c.
func (r *reader) addIgnore(entry string, by Line) error {
	if entry == "" {
		return errors.New(tagIgnoreEmpty, "invalid ignore option: it is empty")
	}
	if len(r.ignores) >= maxProfileIgnore {
		return errors.New(tagTooManyIgnores, "firejail accepts at most 32 ignore entries",
			"max", maxProfileIgnore, "entry", entry, "file", by.File, "line", by.Number)
	}
	r.ignores = append(r.ignores, ignoreEntry{text: entry, by: by})
	return nil
}

// checkLine is profile_check_line of profile.c, in its order: conditional, ignore
// list, "ignore", then the commands that set a state Jailcat's conditionals read. conds
// holds the conditionals the text is written under.
func (r *reader) checkLine(text string, c lineCtx, conds []string) error {
	if strings.HasPrefix(text, "?") {
		name, inner, value, err := r.checkConditional(text, c)
		if err != nil {
			return err
		}
		conds = append(slices.Clone(conds), name)
		if !value {
			// "tell caller to ignore": the rest of the line is not looked at.
			r.prof.Skipped = append(r.prof.Skipped, Skipped{
				Line:   c.line(inner, conds),
				Reason: name + " is false at this point of the profile",
			})
			return nil
		}
		return r.checkLine(inner, c, conds)
	}

	line := c.line(text, conds)

	// check ignore list
	if e, ok := r.findIgnore(text); ok {
		r.prof.Ignored = append(r.prof.Ignored, Ignored{Line: line, By: e.by})
		return nil
	}

	if rest, ok := strings.CutPrefix(text, "ignore "); ok {
		if err := r.addIgnore(rest, line); err != nil {
			return err
		}
		r.prof.Ignores = append(r.prof.Ignores, line)
		return nil
	}

	r.apply(text)
	r.prof.Lines = append(r.prof.Lines, line)
	if text == "noprinters" {
		// profile_check_line adds these two with profile_add, bypassing the ignore list.
		for _, extra := range []string{"blacklist /dev/lp*", "blacklist /run/cups/cups.sock"} {
			l := c.line(extra, conds)
			l.Origin = text
			r.prof.Lines = append(r.prof.Lines, l)
		}
	}
	return nil
}

// apply records what a line that took effect does to the state the conditionals read
// (profile_check_line of profile.c). The matches are exact, as the code's strcmp calls
// are. It assumes firejail.config enables the features (network, private-home, x11,
// dbus), which is the default.
func (r *reader) apply(text string) {
	switch text {
	case "nosound":
		r.hasNoSound = true
	case "private":
		r.hasPrivate = true
	case "nodbus", "dbus-user filter", "dbus-user none", "dbus-system filter", "dbus-system none":
		r.hasNoDBus = true
	case "x11 none", "x11 xorg":
		r.hasX11 = true
	case "net none":
		r.hasNet = true
	case "allow-debuggers":
		r.allowDebuggers = true
	}
	if strings.HasPrefix(text, "private-home ") || strings.HasPrefix(text, "private ") {
		r.hasPrivate = true
	}
	// "net <device>" (not "lo", which makes firejail exit) configures a bridge.
	if dev, ok := strings.CutPrefix(text, "net "); ok && dev != "none" && dev != "lo" {
		r.hasNet = true
	}
}

// conditional is one entry of the conditionals[] table of profile.c.
type conditional struct {
	name  string
	check func(r *reader) bool
}

// conditionals returns the conditionals[] table of profile.c, in its order.
func conditionals() []conditional {
	return []conditional{
		// check_appimage: arg_appimage, which only --appimage sets.
		{"HAS_APPIMAGE", func(*reader) bool { return false }},
		{"HAS_NET", func(r *reader) bool { return r.hasNet }},
		{"HAS_NODBUS", func(r *reader) bool { return r.hasNoDBus }},
		{"HAS_NOSOUND", func(r *reader) bool { return r.hasNoSound }},
		{"HAS_PRIVATE", func(r *reader) bool { return r.hasPrivate }},
		{"HAS_X11", func(r *reader) bool { return r.hasX11 }},
		{"BROWSER_DISABLE_U2F", func(r *reader) bool { return r.opts.Config.BrowserDisableU2F }},
		{"BROWSER_ALLOW_DRM", func(r *reader) bool { return r.opts.Config.BrowserAllowDRM }},
		{"ALLOW_TRAY", func(r *reader) bool { return r.opts.Config.AllowTray }},
	}
}

// checkConditional is profile_check_conditional of profile.c for a text that starts
// with "?". It returns the name of the conditional, the text after the colon and the
// value of the condition now. Every error is one on which firejail exits.
func (r *reader) checkConditional(text string, c lineCtx) (name, inner string, value bool, err error) {
	ptr := text[1:]
	for _, cond := range conditionals() {
		// continue if not this conditional
		rest, ok := strings.CutPrefix(ptr, cond.name)
		if !ok {
			continue
		}
		rest = strings.TrimPrefix(rest, " ")
		rest, hasColon := strings.CutPrefix(rest, ":")
		if !hasColon {
			return "", "", false, errors.New(tagConditionalNoColon,
				"invalid conditional syntax: colon must come after conditional",
				"text", text, "file", c.file, "line", c.number)
		}
		if rest == "" {
			return "", "", false, errors.New(tagConditionalNoLine,
				"invalid conditional syntax: no profile line after conditional",
				"text", text, "file", c.file, "line", c.number)
		}
		rest = strings.TrimPrefix(rest, " ")

		value = cond.check(r)
		// The check for quiet and include only happens when the condition is true.
		if value && (strings.HasPrefix(rest, "quiet") || strings.HasPrefix(rest, "include")) {
			return "", "", false, errors.New(tagConditionalForbidsCommand,
				"invalid conditional syntax: quiet and include not allowed in conditionals",
				"text", text, "file", c.file, "line", c.number)
		}
		return cond.name, rest, value, nil
	}

	// this was a '?' prefix, but didn't match any of the conditionals
	got, _, _ := strings.Cut(ptr, ":")
	return "", "", false, errors.New(tagUnknownConditional, "invalid/unsupported conditional",
		"conditional", got, "file", c.file, "line", c.number)
}

const (
	tagProfileReadCanceled       = "ProfileReadCanceled"
	tagProfileReadFailed         = "ProfileReadFailed"
	tagProfilePathInvalid        = "ProfilePathInvalid"
	tagHomeMacroNotAllowed       = "HomeMacroNotAllowed"
	tagIncludeCycle              = "IncludeCycle"
	tagIncludeTooDeep            = "IncludeTooDeep"
	tagTooManyIgnores            = "TooManyIgnores"
	tagIgnoreEmpty               = "IgnoreEmpty"
	tagConditionalNoColon        = "ConditionalNoColon"
	tagConditionalNoLine         = "ConditionalNoLine"
	tagConditionalForbidsCommand = "ConditionalForbidsCommand"
	tagUnknownConditional        = "UnknownConditional"
)
