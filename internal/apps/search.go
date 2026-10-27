package apps

import (
	"sort"
	"strings"
)

// Filter narrows the apps a search looks at (SPEC §4.3).
type Filter uint8

const (
	// FilterAll keeps every app.
	FilterAll Filter = iota
	// FilterMenu keeps the apps with a desktop entry.
	FilterMenu
	// FilterBinaries keeps the bare binaries.
	FilterBinaries
	// FilterBuilt keeps the apps SearchOpts.Built says yes to.
	FilterBuilt
)

// SearchOpts are the options of [Search].
type SearchOpts struct {
	// ShowHidden keeps the apps whose entry has NoDisplay=true.
	ShowHidden bool
	// Built tells whether an app already has a build; FilterBuilt keeps those. A nil
	// Built means none does.
	Built func(App) bool
}

// Search returns the apps that pass filter and match query, in a new slice. An app with
// NoDisplay is left out unless opts.ShowHidden. The trimmed query matches, ignoring
// case and by substring, the Name, GenericName, Keywords, BinaryName and Binary; an
// empty query matches everything. The apps whose Name starts with the query come first;
// each group keeps the order of apps.
func Search(apps []App, query string, filter Filter, opts SearchOpts) []App {
	q := strings.ToLower(strings.TrimSpace(query))
	out := []App{}
	for _, a := range apps {
		if a.NoDisplay && !opts.ShowHidden {
			continue
		}
		if !passesFilter(a, filter, opts) || !matches(a, q) {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return nameStarts(out[i], q) && !nameStarts(out[j], q)
	})
	return out
}

// passesFilter reports whether a is in the set filter selects.
func passesFilter(a App, filter Filter, opts SearchOpts) bool {
	switch filter {
	case FilterMenu:
		return a.Kind == KindDesktop
	case FilterBinaries:
		return a.Kind == KindBinary
	case FilterBuilt:
		return opts.Built != nil && opts.Built(a)
	}
	return true
}

// matches reports whether the lower-case query q is in a field of a.
func matches(a App, q string) bool {
	if q == "" {
		return true
	}
	fields := append([]string{a.Name, a.GenericName, a.BinaryName, a.Binary}, a.Keywords...)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// nameStarts reports whether a's Name starts with the lower-case query q.
func nameStarts(a App, q string) bool {
	return strings.HasPrefix(strings.ToLower(a.Name), q)
}
