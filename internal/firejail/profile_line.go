package firejail

import (
	"bytes"
	"strings"
)

// maxReadLine is the size of firejail's line buffer, MAX_READ in profile.c and
// checkcfg.c. fgets(buf, MAX_READ, fp) reads at most MAX_READ-1 bytes, so a longer
// line arrives as several "lines".
const maxReadLine = 8192

// Line is one profile line as firejail sees it after reading: normalized, with the
// macros as written.
type Line struct {
	// Text is the normalized line (see [Normalize]), macros NOT expanded. For a
	// conditional line it is the text after the "?NAME:" prefix.
	Text string
	// Command is the first word of Text.
	Command string
	// Args is the rest of Text after the first space, empty when there is none.
	Args string
	// File is the profile file the line was read from, as firejail names it.
	File string
	// Number is the 1-based line number in File. firejail counts one line per fgets
	// call, so a line longer than 8191 bytes counts as several.
	Number int
	// Conditional is the conditional(s) the line was written under, for example
	// "HAS_NET"; nested conditionals are joined with ",", outermost first. It is empty
	// when the line is not conditional.
	Conditional string
	// IncludedFrom lists the files that led to File, the top-level profile first and
	// the file that holds the include line last. It is empty for the top-level
	// profile. The slice is shared between lines: do not modify it.
	IncludedFrom []string
	// Origin is empty when the line stands as written. When firejail itself made the
	// line from another one, Origin is the text of that line: profile.c turns
	// "whitelist-ro X" into "whitelist X" and "read-only X", and "noprinters" adds
	// "blacklist /dev/lp*" and "blacklist /run/cups/cups.sock". Such lines are added
	// to firejail's list without going through the ignore list.
	Origin string
}

// Ignored is a line that an "ignore" line skipped.
type Ignored struct {
	// Line is the line that was skipped.
	Line Line
	// By is the "ignore" line whose entry matched it. For an entry that came from
	// [ReadOpts.Cmdline], By has no File and Number 0.
	By Line
}

// Skipped is a conditional line whose condition was false, so firejail drops it
// without looking at what follows the colon.
type Skipped struct {
	// Line is the dropped line; Text is what followed the "?NAME:" prefix and
	// Conditional is the condition that was false (with the enclosing ones).
	Line Line
	// Reason says which condition was false and why.
	Reason string
}

// Include is an "include" line that took effect (one an "ignore" skipped is in
// [Profile.Ignored] instead).
type Include struct {
	// Line is the include line.
	Line Line
	// Resolved is the path firejail read, or tried to read when the file is missing.
	// For a bare name that no firejail folder holds it is the name itself.
	Resolved string
	// Skipped is empty when the file was read. It is "missing" for a ".local" file that
	// could not be accessed (firejail skips those), and "allow-debuggers" for
	// disable-devel.inc after an "allow-debuggers" line took effect.
	Skipped string
}

// Profile is the result of [Read]: what firejail would apply, in order.
type Profile struct {
	// Path is the profile that was read.
	Path string
	// Lines are the lines that take effect, in the order firejail applies them,
	// including the ones written under a true conditional. It holds neither "ignore",
	// "include" nor "quiet" lines: those are in Ignores, Includes and Quiet.
	Lines []Line
	// Ignored are the lines that "ignore" entries skipped, in the order they were read.
	Ignored []Ignored
	// Ignores is every "ignore" line that took effect, in order.
	Ignores []Line
	// Includes are the include lines that took effect, in the order they were read.
	Includes []Include
	// Missing are the include targets that could not be accessed and were skipped like
	// firejail skips them (only ".local" files are skipped; any other missing file is
	// an error).
	Missing []string
	// Skipped are the conditional lines whose condition was false.
	Skipped []Skipped
	// Quiet is true when a "quiet" line took effect (profile.c, profile_read).
	Quiet bool
}

// Normalize returns the text firejail works with for one profile line: everything
// from the first "#" is a comment, then line_remove_spaces (util.c) runs. It drops the
// leading blanks, stops at the first newline or carriage return, turns every run of
// spaces and tabs into one space, and strips the last character if it is a space, so
// no trailing blank is left. The result is empty for a blank or comment-only line.
// Other whitespace (form feed, vertical tab, no-break space) is not a blank to
// firejail and is kept.
func Normalize(raw string) string {
	// profile.c, profile_read: the buffer is a C string, so a NUL ends it, and
	// strchr(buf, '#') cuts the comment before line_remove_spaces runs.
	if i := strings.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	return removeSpaces(raw)
}

// removeSpaces is line_remove_spaces of util.c, without the comment handling.
func removeSpaces(buf string) string {
	i := 0
	for i < len(buf) && (buf[i] == ' ' || buf[i] == '\t') {
		i++
	}
	var out []byte
	for i < len(buf) {
		c := buf[i]
		if c == '\n' || c == '\r' {
			break
		}
		if c == ' ' || c == '\t' {
			out = append(out, ' ')
			for i < len(buf) && (buf[i] == ' ' || buf[i] == '\t') {
				i++
			}
			continue
		}
		out = append(out, c)
		i++
	}
	if n := len(out); n > 0 && out[n-1] == ' ' {
		out = out[:n-1]
	}
	return string(out)
}

// Matches reports whether the "ignore" entry ignoreText skips the profile line
// lineText, the way is_in_ignore_list of profile.c does: lineText starts with
// ignoreText and what follows is the end of the line or a space. Both are compared as
// written (normalized, macros not expanded), so "whitelist ${HOME}/.config/Code"
// also skips "whitelist ${HOME}/.config/Code - OSS" but not "...Code2", and "nosound"
// skips only "nosound".
func Matches(ignoreText, lineText string) bool {
	if !strings.HasPrefix(lineText, ignoreText) {
		return false
	}
	rest := lineText[len(ignoreText):]
	return rest == "" || rest[0] == ' '
}

// fgetsChunks splits data into the pieces firejail's fgets(buf, 8192, fp) loop gets:
// up to 8191 bytes, ending after a newline. Each piece stops at its first NUL, as a C
// string does.
func fgetsChunks(data []byte) []string {
	var out []string
	for len(data) > 0 {
		n := min(len(data), maxReadLine-1)
		if i := bytes.IndexByte(data[:n], '\n'); i >= 0 {
			n = i + 1
		}
		chunk := data[:n]
		data = data[n:]
		if i := bytes.IndexByte(chunk, 0); i >= 0 {
			chunk = chunk[:i]
		}
		out = append(out, string(chunk))
	}
	return out
}

// splitCommand splits a normalized line at its first space.
func splitCommand(text string) (command, args string) {
	command, args, _ = strings.Cut(text, " ")
	return command, args
}
