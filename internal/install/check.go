package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// ConflictKind says why a target cannot be written without the user's say (SPEC §9.5).
type ConflictKind string

// The kinds of conflict.
const (
	// ConflictEdited is a file recorded for this build that has changed on disk since
	// Jailcat wrote it.
	ConflictEdited ConflictKind = "edited"
	// ConflictOtherBuild is a file recorded for another build. It is never overwritten:
	// a path belongs to one build only.
	ConflictOtherBuild ConflictKind = "other-build"
	// ConflictForeignJailcat is a file Jailcat's header says Jailcat made, that no build
	// records (for example from a database that was lost).
	ConflictForeignJailcat ConflictKind = "foreign-jailcat"
	// ConflictForeign is anything else: a file that is not Jailcat's, a folder, a link.
	// It is never overwritten; the user picks another sandbox name.
	ConflictForeign ConflictKind = "foreign"
)

// Conflict is a target that [Check] found in the way.
type Conflict struct {
	// File is the planned file whose target is taken.
	File File
	// Kind says why.
	Kind ConflictKind
	// Owner is the build the file is recorded for: this build for [ConflictEdited],
	// another one for [ConflictOtherBuild]; 0 for the other kinds.
	Owner int64
	// Overwritable is true when the user may confirm the overwrite (SPEC §9.5): the file
	// carries Jailcat's header and is not another build's.
	Overwritable bool
}

// OwnerFunc tells which build a path is recorded for in the database: the build's id,
// the SHA-256 recorded for the path and ok true; ok false when no build records it. The
// caller makes it from the store; this package does not use the database.
type OwnerFunc func(path string) (buildID int64, sha string, ok bool, err error)

// headerScanBytes is how much of a file is searched for the header of SPEC §9.
const headerScanBytes = 2048

// headerScanLines is how many lines from the top are searched for the header: the
// launcher has it after its "#!" line, a desktop entry after "[Desktop Entry]", an icon
// after the XML declaration.
const headerScanLines = 6

// Check looks at the target of every planned file (SPEC §9.5) and returns the conflicts,
// in the order of the plan; none means [Install] may write everything.
//
//   - a missing target is fine
//   - a file recorded for this build (owner) whose SHA-256 on disk is the recorded one is
//     fine: it is overwritten
//   - recorded for this build but different on disk: [ConflictEdited]
//   - recorded for another build: [ConflictOtherBuild]
//   - not recorded, with Jailcat's header: [ConflictForeignJailcat]
//   - anything else, including a folder or a link in place of the file: [ConflictForeign]
//
// A private home folder or VS Code data folder that exists is fine, because it is kept
// between builds; a file in its place is a [ConflictForeign]. A nil owner means that no path is recorded.
func Check(ctx context.Context, p *Prepared, owner OwnerFunc) ([]Conflict, error) {
	var out []Conflict
	for _, f := range p.Files {
		if err := ctx.Err(); err != nil {
			return nil, errors.NewErr(tagCheckCanceled, err).SetMetadata("path", f.Path)
		}
		c, found, err := checkFile(p.BuildID, f, owner)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, c)
		}
	}
	return out, nil
}

// checkFile checks one target; found is false when there is no conflict.
func checkFile(buildID int64, f File, owner OwnerFunc) (c Conflict, found bool, err error) {
	info, err := os.Lstat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Conflict{}, false, nil
	}
	if err != nil {
		return Conflict{}, false, errors.NewErr(tagTargetStatFailed, err).SetMetadata("path", f.Path)
	}
	if f.Kind == KindHome || f.Kind == KindAppData {
		if info.IsDir() {
			return Conflict{}, false, nil
		}
		return Conflict{File: f, Kind: ConflictForeign}, true, nil
	}

	var (
		ownerID int64
		sum     string
		ok      bool
	)
	if owner != nil {
		ownerID, sum, ok, err = owner(f.Path)
		if err != nil {
			if _, tagged := errors.FromError(err); tagged {
				return Conflict{}, false, err
			}
			return Conflict{}, false, errors.NewErr(tagOwnerLookupFailed, err).SetMetadata("path", f.Path)
		}
	}
	if ok && ownerID != buildID {
		return Conflict{File: f, Kind: ConflictOtherBuild, Owner: ownerID}, true, nil
	}
	if !info.Mode().IsRegular() {
		return Conflict{File: f, Kind: ConflictForeign}, true, nil
	}

	file, err := os.Open(f.Path)
	if err != nil {
		return Conflict{}, false, errors.NewErr(tagTargetReadFailed, err).SetMetadata("path", f.Path)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, headerScanBytes)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Conflict{}, false, errors.NewErr(tagTargetReadFailed, err).SetMetadata("path", f.Path)
	}
	head = head[:n]
	h := sha256.New()
	h.Write(head)
	if _, err := io.Copy(h, file); err != nil {
		return Conflict{}, false, errors.NewErr(tagTargetReadFailed, err).SetMetadata("path", f.Path)
	}
	onDisk := hex.EncodeToString(h.Sum(nil))

	switch {
	case ok && onDisk == sum:
		return Conflict{}, false, nil
	case ok:
		return Conflict{File: f, Kind: ConflictEdited, Owner: buildID, Overwritable: hasHeader(f.Kind, head)}, true, nil
	case hasHeader(f.Kind, head):
		return Conflict{File: f, Kind: ConflictForeignJailcat, Overwritable: true}, true, nil
	}
	return Conflict{File: f, Kind: ConflictForeign}, true, nil
}

// hasHeader reports whether head, the start of a file of the kind, carries Jailcat's
// header (SPEC §9) in its first lines: [firejail.HeaderLine] in a profile, a launcher or
// a desktop entry, the same words as an XML comment in an icon.
func hasHeader(kind Kind, head []byte) bool {
	want := firejail.HeaderLine
	if kind == KindIcon {
		want = "<!-- " + strings.TrimPrefix(firejail.HeaderLine, "# ") + " -->"
	}
	for i, line := range bytes.SplitN(head, []byte("\n"), headerScanLines+1) {
		if i == headerScanLines {
			break
		}
		if string(bytes.TrimSuffix(line, []byte("\r"))) == want {
			return true
		}
	}
	return false
}

const (
	tagCheckCanceled     = "CheckCanceled"
	tagTargetStatFailed  = "TargetStatFailed"
	tagTargetReadFailed  = "TargetReadFailed"
	tagOwnerLookupFailed = "OwnerLookupFailed"
)
