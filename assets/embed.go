// Package assets holds the files embedded into Jailcat: the badge drawn over the corner of a
// build's icon and Jailcat's own icon.
package assets

import _ "embed"

//go:embed badge.svg
var badge string

//go:embed jailcat.svg
var icon string

// Badge returns the SVG source of the badge (viewBox 0 0 64 64): a shield with jail bars.
// Every call returns a new copy.
func Badge() []byte { return []byte(badge) }

// Icon returns the SVG source of Jailcat's own icon (viewBox 0 0 128 128): a cat face behind
// jail bars. Every call returns a new copy.
func Icon() []byte { return []byte(icon) }
