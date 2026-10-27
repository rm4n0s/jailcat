package ui

import (
	"path/filepath"
	"strings"
)

// ProfileGroup is one of the groups of the profile selector (SPEC §6.1).
type ProfileGroup string

// The groups of the profile selector, in the order the selector shows them.
const (
	// GroupForApp is the profile firejail itself would pick for the app's binary.
	GroupForApp ProfileGroup = "For this app"
	// GroupNone is "no base profile".
	GroupNone ProfileGroup = "None"
	// GroupDefault is firejail's default.profile.
	GroupDefault ProfileGroup = "Default"
	// GroupYours is the *.profile files of the firejail user config folder.
	GroupYours ProfileGroup = "Your profiles"
	// GroupImported is the *.profile files Jailcat imported.
	GroupImported ProfileGroup = "Imported"
	// GroupFirejail is the *.profile files of /etc/firejail.
	GroupFirejail ProfileGroup = "Firejail profiles"
)

// ProfileChoice is one entry of the profile selector.
type ProfileChoice struct {
	// Group is the group the entry is listed in.
	Group ProfileGroup
	// Name is what the list shows and the search matches: the file name without
	// ".profile", or "None" and "Default".
	Name string
	// Path is the absolute path of the profile file; empty for None.
	Path string
	// Label is the text of the selector's button when the entry is chosen: Name, with
	// where the profile is from for the app's own one ("code (firejail)").
	Label string
}

// ProfileChoices is the content of the profile selector for one app.
type ProfileChoices struct {
	// Entries are all the entries, group by group in the order of the groups; inside a
	// group, sorted by name.
	Entries []ProfileChoice
	// Default is the entry a new build starts on: the app's own profile if it has one,
	// else Default.
	Default ProfileChoice
}

// Find returns the first entry with the path; "" finds None.
func (c ProfileChoices) Find(path string) (ProfileChoice, bool) {
	for _, e := range c.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return ProfileChoice{}, false
}

// LabelFor returns the text for the selector's button for the profile at path. A path
// that is not in the list (the file is gone, or lives elsewhere) is shown by its file
// name.
func (c ProfileChoices) LabelFor(path string) string {
	if e, ok := c.Find(path); ok {
		return e.Label
	}
	return profileName(path)
}

// Filter returns the entries whose name contains text, ignoring case, in the order of
// the list. An empty text keeps all of them.
func (c ProfileChoices) Filter(text string) []ProfileChoice {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return c.Entries
	}
	var out []ProfileChoice
	for _, e := range c.Entries {
		if strings.Contains(strings.ToLower(e.Name), text) {
			out = append(out, e)
		}
	}
	return out
}

// profileName is the file name of a profile without ".profile".
func profileName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".profile")
}
