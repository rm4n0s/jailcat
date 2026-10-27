package desktopentry

import (
	"bufio"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
)

// Parse reads a desktop entry file. It keeps the groups, keys, comments and blank
// lines in file order, so writing the entry back gives the same file apart from the
// spaces around '=' and a missing final line feed. A line may end with a carriage
// return, which is dropped; a line may start with spaces or tabs, which are dropped,
// as are the spaces around the '=' and at the end of a value.
//
// It returns an error, with the line number in its metadata, for a key before any
// group, a line that is not a comment, a group header or a key, a group or a key
// that is repeated, and a line that is not valid UTF-8. Group names may hold any
// character but '[', ']' and control characters.
func Parse(r io.Reader) (*Entry, error) {
	e := NewEntry()
	br := bufio.NewReader(r)
	var cur *Group
	keys := map[string]bool{}
	for n := 1; ; n++ {
		text, readErr := br.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, errors.NewErr(tagReadFailed, readErr).SetMetadata("line", n)
		}
		if text == "" && readErr == io.EOF {
			return e, nil
		}
		text = strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r")
		if !utf8.ValidString(text) {
			return nil, errors.New(tagInvalidUTF8, "the file is not valid UTF-8", "line", n)
		}
		trimmed := strings.TrimLeft(text, " \t")
		switch {
		case strings.IndexByte(trimmed, 0) >= 0:
			return nil, errors.New(tagMalformedLine, "the line has a NUL", "line", n)
		case trimmed == "":
			cur.addLine(e, line{kind: lineBlank})
		case trimmed[0] == '#':
			cur.addLine(e, line{kind: lineComment, text: trimmed})
		case trimmed[0] == '[':
			header := strings.TrimRight(trimmed, " \t")
			name := strings.TrimSuffix(strings.TrimPrefix(header, "["), "]")
			if !strings.HasSuffix(header, "]") || !validGroupName(name) {
				return nil, errors.New(tagMalformedLine, "not a valid group header", "line", n, "text", text)
			}
			if e.Lookup(name) != nil {
				return nil, errors.New(tagDuplicateGroup, "the group is already in the file", "line", n, "group", name)
			}
			cur = &Group{name: name}
			e.groups = append(e.groups, cur)
			keys = map[string]bool{}
		default:
			eq := strings.IndexByte(trimmed, '=')
			if eq < 0 {
				return nil, errors.New(tagMalformedLine, "not a comment, a group header or key=value", "line", n, "text", text)
			}
			key := strings.TrimRight(trimmed[:eq], " \t")
			if _, _, ok := splitKey(key); !ok {
				return nil, errors.New(tagMalformedLine, "not a valid key name", "line", n, "key", key)
			}
			if cur == nil {
				return nil, errors.New(tagKeyBeforeGroup, "a key comes before the first group", "line", n, "key", key)
			}
			if keys[key] {
				return nil, errors.New(tagDuplicateKey, "the key is already in the group", "line", n, "key", key, "group", cur.name)
			}
			keys[key] = true
			cur.lines = append(cur.lines, line{kind: lineKey, key: key, raw: strings.Trim(trimmed[eq+1:], " \t")})
		}
		if readErr == io.EOF {
			return e, nil
		}
	}
}

// addLine adds a comment or blank line to the group, or to the lines before the
// first group when g is nil.
func (g *Group) addLine(e *Entry, l line) {
	if g == nil {
		e.preamble = append(e.preamble, l)
		return
	}
	g.lines = append(g.lines, l)
}

// validGroupName reports whether s can be a group name: not empty, no '[', ']' or
// control character.
func validGroupName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '[' || r == ']' || r < ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

const (
	tagReadFailed     = "ReadFailed"
	tagInvalidUTF8    = "InvalidUTF8"
	tagMalformedLine  = "MalformedLine"
	tagKeyBeforeGroup = "KeyBeforeGroup"
	tagDuplicateGroup = "DuplicateGroup"
	tagDuplicateKey   = "DuplicateKey"
)
