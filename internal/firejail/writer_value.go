package firejail

import (
	"strings"

	"github.com/rm4n0s/errors"
)

// profileValue is the one function every value passes through before it is written into
// a profile line (CLAUDE.md, SPEC §9.1). Firejail normalizes every line it reads
// (SPEC §8.1), so a value with a new line, a NUL, a "#", a tab, a blank at either end or
// two blanks in a row would not be read back as written: these are errors, not escaped.
// An empty value is an error too, because the line would lose its argument. The value is
// returned unchanged when it is fine.
func profileValue(s string) (string, error) {
	if s == "" {
		return "", errors.New(tagProfileValueInvalid, "a profile line needs a value, and this one is empty",
			"value", s, "reason", "nothing")
	}
	if reason := forbiddenValue(s); reason != "" {
		return "", errors.New(tagProfileValueInvalid, "this value cannot be written in a profile line: it has "+reason,
			"value", s, "reason", reason)
	}
	return s, nil
}

// profileList joins the items of a comma separated list (protocol, private-bin,
// private-etc, cpu) after every item passed through [profileValue]. A comma inside an
// item would split it into two when firejail reads the line, so it is an error, and so
// is an empty item.
func profileList(items []string) (string, error) {
	for _, item := range items {
		if item == "" || strings.Contains(item, ",") {
			return "", errors.New(tagProfileListItemInvalid,
				"an item of a comma separated list cannot be empty or have a comma in it", "item", item)
		}
		if _, err := profileValue(item); err != nil {
			return "", err
		}
	}
	return strings.Join(items, ","), nil
}

const (
	tagProfileValueInvalid    = "ProfileValueInvalid"
	tagProfileListItemInvalid = "ProfileListItemInvalid"
)
