package desktopentry

import (
	"io"
	"strings"

	"github.com/rm4n0s/errors"
)

// Write writes the entry as a desktop entry file: the comments before the first
// group, then each group with its keys, comments and blank lines in the order they
// are stored, as "Key=value" lines. A group name that is empty or has a '[', ']' or
// a control character is refused, and nothing is written. A nil entry writes nothing.
func Write(w io.Writer, e *Entry) error {
	var b strings.Builder
	if e != nil {
		for _, l := range e.preamble {
			writeLine(&b, l)
		}
		for _, g := range e.groups {
			if !validGroupName(g.name) {
				return errors.New(tagInvalidGroupName, "a group name may not be empty or have '[', ']' or control characters",
					"group", g.name)
			}
			b.WriteString("[" + g.name + "]\n")
			for _, l := range g.lines {
				writeLine(&b, l)
			}
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return errors.NewErr(tagWriteFailed, err)
	}
	return nil
}

// writeLine writes one line of a group, with its line feed.
func writeLine(b *strings.Builder, l line) {
	switch l.kind {
	case lineBlank:
	case lineComment:
		b.WriteString(l.text)
	case lineKey:
		b.WriteString(l.key + "=" + l.raw)
	}
	b.WriteByte('\n')
}

const (
	tagInvalidGroupName = "InvalidGroupName"
	tagWriteFailed      = "WriteFailed"
)
