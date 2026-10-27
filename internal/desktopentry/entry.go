package desktopentry

import (
	"strings"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
)

const (
	// MainGroup is the name of the group every desktop entry must have.
	MainGroup = "Desktop Entry"
	// ActionGroupPrefix starts the name of a desktop action group; the action's
	// identifier follows it.
	ActionGroupPrefix = "Desktop Action "
)

type lineKind uint8

const (
	lineBlank lineKind = iota
	lineComment
	lineKey
)

// line is one line of a group (or of the lines before the first group).
type line struct {
	kind lineKind
	// text is the whole comment line, including its leading '#' (comments only).
	text string
	// key is the full key name, with its [locale] postfix (keys only).
	key string
	// raw is the value as written in the file, with its escapes not decoded (keys only).
	raw string
}

// Entry is a desktop entry file: the groups in file order, and the comments and
// blank lines around them. The zero value is an empty entry ready to use.
type Entry struct {
	preamble []line
	groups   []*Group
}

// Group is a group of a desktop entry: its name and its keys, comments and blank
// lines in file order.
type Group struct {
	name  string
	lines []line
}

// Key is one key of a group, as stored: the key name without its locale, the
// locale (empty for the unlocalized key) and the raw value with its escapes
// still in place.
type Key struct {
	Name   string
	Locale string
	Raw    string
}

// FullName returns the key as written in the file: Name, or Name[Locale].
func (k Key) FullName() string {
	if k.Locale == "" {
		return k.Name
	}
	return k.Name + "[" + k.Locale + "]"
}

// NewEntry returns an empty entry.
func NewEntry() *Entry {
	return &Entry{}
}

// Groups returns the groups in file order.
func (e *Entry) Groups() []*Group {
	return append([]*Group(nil), e.groups...)
}

// Lookup returns the group with the given name, or nil when there is none.
func (e *Entry) Lookup(name string) *Group {
	for _, g := range e.groups {
		if g.name == name {
			return g
		}
	}
	return nil
}

// Group returns the group with the given name and creates it, after the last
// one, when the entry has none. The name is checked when the entry is written.
func (e *Entry) Group(name string) *Group {
	if g := e.Lookup(name); g != nil {
		return g
	}
	if n := len(e.groups); n > 0 {
		prev := e.groups[n-1]
		if len(prev.lines) == 0 || prev.lines[len(prev.lines)-1].kind != lineBlank {
			prev.lines = append(prev.lines, line{kind: lineBlank})
		}
	}
	g := &Group{name: name}
	e.groups = append(e.groups, g)
	return g
}

// Main returns the [Desktop Entry] group, or nil when the entry has none.
func (e *Entry) Main() *Group {
	return e.Lookup(MainGroup)
}

// Actions returns the [Desktop Action <id>] groups named by the Actions key of the
// main group, in the order of that key. An identifier with no group is skipped, and
// so is a repeated one; a group no identifier names is not returned.
func (e *Entry) Actions() []*Group {
	var out []*Group
	seen := map[string]bool{}
	for _, id := range e.Main().Strings("Actions") {
		if seen[id] {
			continue
		}
		seen[id] = true
		if g := e.Lookup(ActionGroupPrefix + id); g != nil {
			out = append(out, g)
		}
	}
	return out
}

// Comment adds a comment line before the first group; text with line breaks makes
// one comment line per line. A comment with a NUL, a carriage return or invalid
// UTF-8 is refused.
func (e *Entry) Comment(text string) error {
	lines, err := commentLines(text)
	if err != nil {
		return err
	}
	e.preamble = append(e.preamble, lines...)
	return nil
}

// Name returns the group's name.
func (g *Group) Name() string {
	if g == nil {
		return ""
	}
	return g.name
}

// ActionID returns the identifier of a [Desktop Action <id>] group and true, or
// "" and false for any other group.
func (g *Group) ActionID() (string, bool) {
	if g == nil || !strings.HasPrefix(g.name, ActionGroupPrefix) {
		return "", false
	}
	return strings.TrimPrefix(g.name, ActionGroupPrefix), true
}

// Keys returns the group's keys in file order.
func (g *Group) Keys() []Key {
	if g == nil {
		return nil
	}
	var keys []Key
	for _, l := range g.lines {
		if l.kind != lineKey {
			continue
		}
		name, locale, _ := splitKey(l.key)
		keys = append(keys, Key{Name: name, Locale: locale, Raw: l.raw})
	}
	return keys
}

// Has reports whether the group has the key. A localized key is given with its
// postfix, as in "Name[de]".
func (g *Group) Has(key string) bool {
	_, ok := g.raw(key)
	return ok
}

// Set sets key to value, replacing the key's value in place or adding the key after
// the last one. The value is the text itself: Set escapes it (EscapeValue), so it
// never needs escaping by the caller. A key name outside [A-Za-z0-9-], a value with a
// NUL and a value that is not valid UTF-8 are refused.
func (g *Group) Set(key, value string) error {
	return g.setRaw(key, "", EscapeValue(value))
}

// SetLocale sets the localized key key[locale] to value, as Set does. A locale that
// is empty or holds a bracket, a space or a control character is refused.
func (g *Group) SetLocale(key, locale, value string) error {
	if !validLocale(locale) {
		return errors.New(tagInvalidLocale, "locale is empty or has a bracket, a space or a control character",
			"key", key, "locale", locale)
	}
	return g.setRaw(key, locale, EscapeValue(value))
}

// SetStrings sets key to a list of strings: every item is escaped (EscapeList) and
// ends with a semicolon.
func (g *Group) SetStrings(key string, items []string) error {
	return g.setRaw(key, "", EscapeList(items))
}

// SetBool sets key to true or false.
func (g *Group) SetBool(key string, value bool) error {
	raw := "false"
	if value {
		raw = "true"
	}
	return g.setRaw(key, "", raw)
}

// Comment adds a comment line to the group, after its last key; text with line
// breaks makes one comment line per line. A comment with a NUL, a carriage return or
// invalid UTF-8 is refused.
func (g *Group) Comment(text string) error {
	lines, err := commentLines(text)
	if err != nil {
		return err
	}
	for _, l := range lines {
		g.insert(l)
	}
	return nil
}

// setRaw stores an already escaped value under key (and locale).
func (g *Group) setRaw(key, locale, raw string) error {
	if !validKeyName(key) {
		return errors.New(tagInvalidKeyName, "a key name may only have the characters A-Za-z0-9-",
			"key", key)
	}
	if strings.IndexByte(raw, 0) >= 0 || !utf8.ValidString(raw) {
		return errors.New(tagInvalidValue, "a value may not hold a NUL or invalid UTF-8", "key", key)
	}
	full := key
	if locale != "" {
		full = key + "[" + locale + "]"
	}
	for i := range g.lines {
		if g.lines[i].kind == lineKey && g.lines[i].key == full {
			g.lines[i].raw = raw
			return nil
		}
	}
	g.insert(line{kind: lineKey, key: full, raw: raw})
	return nil
}

// insert adds a line after the last line that is not blank, so the blank lines that
// separate this group from the next stay at its end.
func (g *Group) insert(l line) {
	n := len(g.lines)
	for n > 0 && g.lines[n-1].kind == lineBlank {
		n--
	}
	g.lines = append(g.lines, line{})
	copy(g.lines[n+1:], g.lines[n:])
	g.lines[n] = l
}

// raw returns the stored value of the full key name.
func (g *Group) raw(full string) (string, bool) {
	if g == nil {
		return "", false
	}
	for _, l := range g.lines {
		if l.kind == lineKey && l.key == full {
			return l.raw, true
		}
	}
	return "", false
}

// commentLines turns text into comment lines, one per line of text.
func commentLines(text string) ([]line, error) {
	if strings.IndexByte(text, 0) >= 0 || strings.IndexByte(text, '\r') >= 0 || !utf8.ValidString(text) {
		return nil, errors.New(tagInvalidComment, "a comment may not hold a NUL, a carriage return or invalid UTF-8")
	}
	var out []line
	for _, part := range strings.Split(text, "\n") {
		if part == "" {
			out = append(out, line{kind: lineComment, text: "#"})
			continue
		}
		out = append(out, line{kind: lineComment, text: "# " + part})
	}
	return out, nil
}

// validKeyName reports whether s is a non-empty name of the characters A-Za-z0-9-.
func validKeyName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// validLocale reports whether s can sit between the brackets of a localized key.
func validLocale(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '[' || r == ']' || r == '=' || r <= ' ' || r == 0x7f || r == utf8.RuneError {
			return false
		}
	}
	return true
}

// splitKey splits a full key name into its name and locale. ok is false when the
// key is not a valid name with an optional valid [locale] postfix.
func splitKey(full string) (name, locale string, ok bool) {
	i := strings.IndexByte(full, '[')
	if i < 0 {
		return full, "", validKeyName(full)
	}
	if !strings.HasSuffix(full, "]") || !validKeyName(full[:i]) {
		return "", "", false
	}
	locale = full[i+1 : len(full)-1]
	if !validLocale(locale) {
		return "", "", false
	}
	return full[:i], locale, true
}

const (
	tagInvalidKeyName = "InvalidKeyName"
	tagInvalidValue   = "InvalidValue"
	tagInvalidLocale  = "InvalidLocale"
	tagInvalidComment = "InvalidComment"
)
