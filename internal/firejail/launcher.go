package firejail

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
)

// LauncherSpec is everything [WriteLauncher] needs to write the launcher script of a
// build (SPEC §9.2).
type LauncherSpec struct {
	// BuildID is the id of the build, written in the header (SPEC §9).
	BuildID int64
	// Firejail is the absolute, clean path of the firejail executable.
	Firejail string
	// Profile is the absolute, clean path of the build's generated profile.
	Profile string
	// Binary is the absolute, clean path of the app's executable.
	Binary string
	// ExtraArgs are the words that go between the binary and the user's arguments
	// (SPEC §7.6).
	ExtraArgs []string
}

// WriteLauncher writes the launcher script of SPEC §9.2 to w: the "#!/bin/sh" line, the
// header, and an "exec" of firejail with "--profile=<profile>", "--", the binary, the
// extra arguments and "$@". Every word goes through [ShellQuote]; "$@" is written as is,
// so the user's arguments arrive unchanged. The script is built whole and written with
// one call, so an error leaves w untouched; the file ends with a newline.
func WriteLauncher(w io.Writer, spec LauncherSpec) error {
	if !isCleanAbs(spec.Firejail) {
		return errors.New(tagLauncherFirejailInvalid, "the path of firejail is not absolute and clean",
			"path", spec.Firejail)
	}
	if !isCleanAbs(spec.Profile) {
		return errors.New(tagLauncherProfileInvalid, "the path of the profile is not absolute and clean",
			"path", spec.Profile)
	}
	if !isCleanAbs(spec.Binary) {
		return errors.New(tagLauncherBinaryInvalid, "the path of the app's binary is not absolute and clean",
			"path", spec.Binary)
	}

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	for _, line := range Header(spec.BuildID) {
		b.WriteString(line + "\n")
	}
	b.WriteString("exec")
	// "--" has nothing to quote and is written as is, the way SPEC §9.2 shows it.
	words := []string{spec.Firejail, "--profile=" + spec.Profile, "", spec.Binary}
	words = append(words, spec.ExtraArgs...)
	for i, word := range words {
		if i == 2 {
			b.WriteString(" --")
			continue
		}
		q, err := ShellQuote(word)
		if err != nil {
			return err
		}
		b.WriteString(" " + q)
	}
	b.WriteString(" \"$@\"\n")

	if _, err := io.WriteString(w, b.String()); err != nil {
		return errors.NewErr(tagLauncherWriteFailed, err)
	}
	return nil
}

// isCleanAbs reports whether p is an absolute path that [filepath.Clean] leaves as it is.
func isCleanAbs(p string) bool {
	return filepath.IsAbs(p) && filepath.Clean(p) == p
}

const (
	tagLauncherFirejailInvalid = "LauncherFirejailInvalid"
	tagLauncherProfileInvalid  = "LauncherProfileInvalid"
	tagLauncherBinaryInvalid   = "LauncherBinaryInvalid"
	tagLauncherWriteFailed     = "LauncherWriteFailed"
)
