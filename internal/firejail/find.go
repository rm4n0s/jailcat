package firejail

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

// TagFirejailNotFound is the tag of the error [FindFirejail] returns when no executable
// "firejail" is on the path. The UI matches it to show its banner (SPEC §3).
const TagFirejailNotFound = "FirejailNotFound"

// versionTimeout is how long [Version] waits for "firejail --version".
const versionTimeout = 5 * time.Second

// versionKillDelay is how long a killed "firejail --version" may keep its output pipe
// open before Go stops waiting for it.
const versionKillDelay = time.Second

// FindFirejail returns the absolute, clean path of the first "firejail" on dirs.Path that
// is an executable regular file (symlinks are followed, and the path found is returned,
// not the link's target). Relative entries of the path are skipped: the result must be
// absolute. If there is none the error has the tag [TagFirejailNotFound].
func FindFirejail(dirs xdg.Dirs) (string, error) {
	for _, dir := range dirs.Path {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, "firejail")
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New(TagFirejailNotFound, "firejail is not installed in any folder of the path",
		"path", strings.Join(dirs.Path, ":"))
}

// Version runs "firejail --version" (no shell, 5 seconds at most) and returns the version
// on its first line, "firejail version 0.9.80" giving "0.9.80". It is the only process
// Jailcat starts to learn about firejail. A failed run, a timeout and a first line that
// is not of that form have each their own tag.
func Version(ctx context.Context, firejail string) (string, error) {
	return version(ctx, firejail, versionTimeout)
}

// version is [Version] with the timeout as a parameter, for the tests.
func version(ctx context.Context, firejail string, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, firejail, "--version")
	cmd.WaitDelay = versionKillDelay
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// Only our own deadline is a timeout; a cancelled or expired parent context is
		// a failed run.
		if ctx.Err() == nil && runCtx.Err() == context.DeadlineExceeded {
			return "", errors.NewErr(tagFirejailVersionTimeout, err).SetMetadata("firejail", firejail,
				"timeout", timeout.String())
		}
		return "", errors.NewErr(tagFirejailVersionFailed, err).SetMetadata("firejail", firejail)
	}

	first, _, _ := strings.Cut(stdout.String(), "\n")
	fields := strings.Fields(first)
	if len(fields) != 3 || fields[0] != "firejail" || fields[1] != "version" ||
		fields[2][0] < '0' || fields[2][0] > '9' {
		return "", errors.New(tagFirejailVersionUnparsable, "the first line of firejail --version is not "+
			`"firejail version <number>"`, "firejail", firejail, "line", first)
	}
	return fields[2], nil
}

const (
	tagFirejailVersionFailed     = "FirejailVersionFailed"
	tagFirejailVersionTimeout    = "FirejailVersionTimeout"
	tagFirejailVersionUnparsable = "FirejailVersionUnparsable"
)
