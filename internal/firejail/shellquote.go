package firejail

import (
	"strings"

	"github.com/rm4n0s/errors"
)

// ShellQuote returns s as one POSIX shell word in single quotes. Each single quote in s
// is written as quote, backslash, quote, quote (closing the word, an escaped quote,
// opening it again); everything else, a newline included, is written as is, because
// nothing is special inside single quotes. The empty string is two quotes. A NUL byte is an error: no shell can
// pass one. This is the only shell-quoting function of the code base (CLAUDE.md).
func ShellQuote(s string) (string, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return "", errors.New(tagShellWordHasNul, "a shell word cannot hold a NUL byte", "word", s)
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}

const tagShellWordHasNul = "ShellWordHasNul"
