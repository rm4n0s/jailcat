package desktopentry

import (
	"strings"

	"github.com/rm4n0s/errors"
)

// String returns the value of key with the general escapes (\s \n \t \r \\)
// decoded, or "" when the group has no such key. A localized key is given with its
// postfix, as in "Name[de]". An escape the specification does not define is kept
// as it is. Use Has to tell a missing key from an empty one.
func (g *Group) String(key string) string {
	raw, _ := g.raw(key)
	return unescape(raw)
}

// LocaleString returns the value of key for the given locale (the value of
// LC_MESSAGES, such as "de_DE.UTF-8@euro"), in the order of the specification:
// lang_COUNTRY@MODIFIER, lang_COUNTRY, lang@MODIFIER, lang, then the key with no
// locale. The encoding of the locale is ignored. It returns "" when none matches.
func (g *Group) LocaleString(key, locale string) string {
	raw, _ := g.localeRaw(key, locale)
	return unescape(raw)
}

// Strings returns the value of key as a list: items are separated by ';', a ';' in
// an item is written '\;', and the last ';' is optional. It returns nil when the key
// is missing or empty.
func (g *Group) Strings(key string) []string {
	raw, _ := g.raw(key)
	return splitList(raw)
}

// LocaleStrings is Strings for the value LocaleString would pick.
func (g *Group) LocaleStrings(key, locale string) []string {
	raw, _ := g.localeRaw(key, locale)
	return splitList(raw)
}

// Bool returns the value of key as a boolean. The only values are "true" and
// "false"; a missing key is false. Any other value is an error.
func (g *Group) Bool(key string) (bool, error) {
	raw, ok := g.raw(key)
	if !ok {
		return false, nil
	}
	switch unescape(raw) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, errors.New(tagInvalidBool, "a boolean must be true or false", "key", key, "value", raw)
}

// localeRaw finds the raw value of key that matches locale best.
func (g *Group) localeRaw(key, locale string) (string, bool) {
	for _, candidate := range localeCandidates(locale) {
		full := key
		if candidate != "" {
			full = key + "[" + candidate + "]"
		}
		if raw, ok := g.raw(full); ok {
			return raw, true
		}
	}
	return "", false
}

// localeCandidates lists the locale postfixes to try, best first, ending with ""
// for the key with no locale (Desktop Entry Specification, "Localized values for
// keys"). The encoding is dropped; a missing country or modifier means no
// candidate with one.
func localeCandidates(locale string) []string {
	lang, country, modifier := splitLocale(locale)
	var out []string
	if lang != "" {
		if country != "" && modifier != "" {
			out = append(out, lang+"_"+country+"@"+modifier)
		}
		if country != "" {
			out = append(out, lang+"_"+country)
		}
		if modifier != "" {
			out = append(out, lang+"@"+modifier)
		}
		out = append(out, lang)
	}
	return append(out, "")
}

// splitLocale splits lang_COUNTRY.ENCODING@MODIFIER; every part but lang may be
// missing and the encoding is dropped.
func splitLocale(locale string) (lang, country, modifier string) {
	if i := strings.IndexByte(locale, '@'); i >= 0 {
		locale, modifier = locale[:i], locale[i+1:]
	}
	if i := strings.IndexByte(locale, '.'); i >= 0 {
		locale = locale[:i]
	}
	if i := strings.IndexByte(locale, '_'); i >= 0 {
		locale, country = locale[:i], locale[i+1:]
	}
	return locale, country, modifier
}

// unescape decodes the general string escapes of the specification: \s \n \t \r
// and \\. Any other backslash sequence is kept as it is.
func unescape(raw string) string {
	if !strings.Contains(raw, `\`) {
		return raw
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 == len(raw) {
			b.WriteByte(c)
			continue
		}
		i++
		switch raw[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(raw[i])
		}
	}
	return b.String()
}

// splitList splits a raw list value at its unescaped ';' and decodes each item
// (the general escapes and \;). The last ';' is a terminator, not the start of an
// empty item.
func splitList(raw string) []string {
	var items []string
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == ';':
			items = append(items, b.String())
			b.Reset()
		case c == '\\' && i+1 < len(raw):
			i++
			switch raw[i] {
			case 's':
				b.WriteByte(' ')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\', ';':
				b.WriteByte(raw[i])
			default:
				b.WriteByte('\\')
				b.WriteByte(raw[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		items = append(items, b.String())
	}
	return items
}

// EscapeValue escapes s for the value of a key: a backslash is doubled, a line
// feed, tab and carriage return are written \n, \t and \r, so a value stays on its
// line, and a space at either end is written \s, because the reader drops the
// spaces around a value.
func EscapeValue(s string) string {
	s = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
	first := len(s) - len(strings.TrimLeft(s, " "))
	last := len(strings.TrimRight(s, " "))
	if first == 0 && last == len(s) {
		return s
	}
	if first == len(s) {
		return strings.Repeat(`\s`, len(s))
	}
	return strings.Repeat(`\s`, first) + s[first:last] + strings.Repeat(`\s`, len(s)-last)
}

// EscapeList escapes items for a key that holds a list of strings: every item is
// escaped as EscapeValue does and its ';' as '\;', and every item, the last one
// too, ends with a ';' (an empty last item cannot be told apart otherwise).
func EscapeList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(strings.ReplaceAll(EscapeValue(item), ";", `\;`))
		b.WriteByte(';')
	}
	return b.String()
}

const tagInvalidBool = "InvalidBool"
