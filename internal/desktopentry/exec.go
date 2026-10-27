package desktopentry

import (
	"strings"

	"github.com/rm4n0s/errors"
)

const (
	// execReserved are the characters an Exec argument may not hold unquoted
	// (Desktop Entry Specification, "The Exec key"), without the space and the tab
	// that separate arguments.
	execReserved = "\n\"'\\><~|&;$*?#()`"
	// execQuoteEscaped are the characters escaped by a backslash inside a quoted
	// argument.
	execQuoteEscaped = "\"`$\\"
	// execFieldCodes are the field codes that may be written in an Exec value.
	execFieldCodes = "fFuUick"
	// execDeprecatedCodes are the deprecated field codes, removed on reading.
	execDeprecatedCodes = "dDnNvm"
)

// SplitExec splits an Exec value, with the general string escapes already decoded
// (what Group.String returns), into the program and its arguments, by the
// specification's rules:
//
//   - arguments are separated by spaces (or tabs)
//   - an argument that has a reserved character is in double quotes; inside them \"
//     \` \$ and \\ stand for the character after the backslash, and any other
//     backslash is kept
//   - an unquoted argument with a reserved character, a quote that does not start
//     an argument and text right after a closing quote are errors
//   - "%%" becomes "%"; the field codes %f %F %u %U %i %c %k stay in the argument
//     where they are, so a bare "%F" is its own element; the deprecated codes are
//     removed (an argument that held only one disappears); any other "%" is an error
//
// A literal "%F" argument (written "%%F") therefore comes out like the field code.
// The result is empty for an empty value. The rule that a command line has at most
// one of %f %F %u %U, and that %F, %U and %i stand alone, is not checked.
func SplitExec(value string) ([]string, error) {
	sc := execScanner{src: value}
	var args []string
	for {
		sc.skipSeparators()
		if sc.pos >= len(sc.src) {
			return args, nil
		}
		arg, keep, err := sc.next()
		if err != nil {
			return nil, err
		}
		if keep {
			args = append(args, arg)
		}
	}
}

// execScanner reads the arguments of an Exec value one by one.
type execScanner struct {
	src string
	pos int
}

// skipSeparators moves past the spaces and tabs between arguments.
func (sc *execScanner) skipSeparators() {
	for sc.pos < len(sc.src) && (sc.src[sc.pos] == ' ' || sc.src[sc.pos] == '\t') {
		sc.pos++
	}
}

// next reads one argument. keep is false for an unquoted argument that held only
// deprecated field codes.
func (sc *execScanner) next() (arg string, keep bool, err error) {
	var b strings.Builder
	dropped := false
	if sc.src[sc.pos] == '"' {
		start := sc.pos
		sc.pos++
		closed := false
		for sc.pos < len(sc.src) && !closed {
			c := sc.src[sc.pos]
			switch {
			case c == '\\' && sc.pos+1 < len(sc.src) && strings.IndexByte(execQuoteEscaped, sc.src[sc.pos+1]) >= 0:
				b.WriteByte(sc.src[sc.pos+1])
				sc.pos += 2
			case c == '"':
				closed = true
				sc.pos++
			case c == '%':
				if err := sc.percent(&b, &dropped); err != nil {
					return "", false, err
				}
			default:
				b.WriteByte(c)
				sc.pos++
			}
		}
		if !closed {
			return "", false, errors.New(tagUnterminatedQuote, "a quoted Exec argument has no closing quote",
				"value", sc.src, "offset", start)
		}
		if sc.pos < len(sc.src) && sc.src[sc.pos] != ' ' && sc.src[sc.pos] != '\t' {
			return "", false, errors.New(tagTextAfterQuote, "text follows the closing quote of an Exec argument",
				"value", sc.src, "offset", sc.pos)
		}
		return b.String(), true, nil
	}
	for sc.pos < len(sc.src) && sc.src[sc.pos] != ' ' && sc.src[sc.pos] != '\t' {
		c := sc.src[sc.pos]
		switch {
		case strings.IndexByte(execReserved, c) >= 0:
			return "", false, errors.New(tagReservedCharacter,
				"an Exec argument with a reserved character must be in double quotes",
				"value", sc.src, "offset", sc.pos, "character", string(c))
		case c == '%':
			if err := sc.percent(&b, &dropped); err != nil {
				return "", false, err
			}
		default:
			b.WriteByte(c)
			sc.pos++
		}
	}
	return b.String(), b.Len() > 0 || !dropped, nil
}

// percent reads the '%' at the current position and what follows it.
func (sc *execScanner) percent(b *strings.Builder, dropped *bool) error {
	if sc.pos+1 >= len(sc.src) {
		return errors.New(tagInvalidFieldCode, "an Exec value ends with a lone %", "value", sc.src, "offset", sc.pos)
	}
	code := sc.src[sc.pos+1]
	switch {
	case code == '%':
		b.WriteByte('%')
	case strings.IndexByte(execFieldCodes, code) >= 0:
		b.WriteByte('%')
		b.WriteByte(code)
	case strings.IndexByte(execDeprecatedCodes, code) >= 0:
		*dropped = true
	default:
		return errors.New(tagInvalidFieldCode, "an Exec value has a field code the specification does not list",
			"value", sc.src, "offset", sc.pos, "code", "%"+string(code))
	}
	sc.pos += 2
	return nil
}

// QuoteExecArg writes arg as one argument of an Exec value, the inverse of what
// SplitExec reads: a "%" is doubled so it is not a field code, and an argument that
// is empty or has a reserved character (including a space, tab or line feed) is put
// in double quotes with ", `, $ and \ escaped by a backslash.
//
// The result is the value after the general string escapes are decoded; Group.Set
// and EscapeValue add that layer, so a backslash in a quoted argument becomes four
// in the file, as the specification says. A line feed is written as it is inside the
// quotes (the specification lists it as a character to quote), and EscapeValue turns
// it into \n in the file.
func QuoteExecArg(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if arg != "" && !strings.ContainsAny(arg, " \t\r"+execReserved) {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(arg); i++ {
		if strings.IndexByte(execQuoteEscaped, arg[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(arg[i])
	}
	b.WriteByte('"')
	return b.String()
}

// JoinExec builds an Exec value: the arguments quoted by QuoteExecArg, then the
// field codes (such as "%F"), unquoted, each separated by a space. A field code
// other than %f %F %u %U %i %c %k is an error.
func JoinExec(args []string, fieldCodes ...string) (string, error) {
	parts := make([]string, 0, len(args)+len(fieldCodes))
	for _, arg := range args {
		parts = append(parts, QuoteExecArg(arg))
	}
	for _, code := range fieldCodes {
		if len(code) != 2 || code[0] != '%' || strings.IndexByte(execFieldCodes, code[1]) < 0 {
			return "", errors.New(tagInvalidFieldCode, "not a field code the specification lists", "code", code)
		}
		parts = append(parts, code)
	}
	return strings.Join(parts, " "), nil
}

const (
	tagUnterminatedQuote = "UnterminatedQuote"
	tagTextAfterQuote    = "TextAfterQuote"
	tagReservedCharacter = "ReservedCharacter"
	tagInvalidFieldCode  = "InvalidFieldCode"
)
