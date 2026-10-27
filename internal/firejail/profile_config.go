package firejail

import (
	"context"
	"io/fs"
	"os"
	"strings"

	"github.com/rm4n0s/errors"
)

// Config holds the values of firejail.config that profile conditionals read.
type Config struct {
	// BrowserDisableU2F is "browser-disable-u2f": browser profiles disable U2F devices.
	BrowserDisableU2F bool
	// BrowserAllowDRM is "browser-allow-drm": browser profiles allow DRM.
	BrowserAllowDRM bool
	// AllowTray is "allow-tray": programs may show a tray icon.
	AllowTray bool
}

// DefaultConfig returns firejail's defaults, the values checkcfg() in checkcfg.c sets
// before it reads firejail.config: browser-disable-u2f yes, browser-allow-drm no and
// allow-tray no.
func DefaultConfig() Config {
	return Config{BrowserDisableU2F: true}
}

// ReadConfig reads the firejail.config at path. Lines are "key yes" or "key no"; only
// browser-disable-u2f, browser-allow-drm and allow-tray are used and other keys are
// ignored. A missing file gives [DefaultConfig], as firejail uses its defaults when
// the file is not found.
//
// Like checkcfg(), a line that starts with "#" or is empty is skipped, the rest is
// normalized with line_remove_spaces (a "#" later in the line is not a comment there),
// and a known key whose value is not exactly "yes" or "no" is an error.
func ReadConfig(ctx context.Context, path string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, errors.NewErr(tagConfigReadCanceled, err).SetMetadata("path", path)
	}
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, errors.NewErr(tagConfigReadFailed, err).SetMetadata("path", path)
	}

	fields := []struct {
		key string
		dst *bool
	}{
		{"allow-tray", &cfg.AllowTray},
		{"browser-allow-drm", &cfg.BrowserAllowDRM},
		{"browser-disable-u2f", &cfg.BrowserDisableU2F},
	}
	for i, chunk := range fgetsChunks(data) {
		if strings.HasPrefix(chunk, "#") || strings.HasPrefix(chunk, "\n") {
			continue
		}
		ptr := removeSpaces(chunk)
		for _, f := range fields {
			value, ok := strings.CutPrefix(ptr, f.key+" ")
			if !ok {
				continue
			}
			switch value {
			case "yes":
				*f.dst = true
			case "no":
				*f.dst = false
			default:
				return Config{}, errors.New(tagConfigLineInvalid,
					"invalid line in firejail configuration file",
					"path", path, "line", i+1, "key", f.key, "value", value)
			}
		}
	}
	return cfg, nil
}

const (
	tagConfigReadCanceled = "ConfigReadCanceled"
	tagConfigReadFailed   = "ConfigReadFailed"
	tagConfigLineInvalid  = "ConfigLineInvalid"
)
