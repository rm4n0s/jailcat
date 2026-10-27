package xdg

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"
)

// The macros of firejail's user folders, as written in profiles (macros.c, macro[]).
const (
	MacroDownloads = "${DOWNLOADS}"
	MacroMusic     = "${MUSIC}"
	MacroVideos    = "${VIDEOS}"
	MacroPictures  = "${PICTURES}"
	MacroDesktop   = "${DESKTOP}"
	MacroDocuments = "${DOCUMENTS}"
)

// UserDir is a resolved user folder.
type UserDir struct {
	// Relative is the folder relative to the home, as firejail keeps it (for example
	// "Downloads"); firejail expands the macro to "<home>/<Relative>".
	Relative string
	// Path is the absolute path, "<home>/<Relative>".
	Path string
}

// userMacro is one row of firejail's macro[] table (macros.c).
type userMacro struct {
	name         string
	xdg          string
	translations []string
}

// userMacros copies the macro[] table of firejail 0.9.80, src/firejail/macros.c, entry
// for entry and in the same order (the first translation that exists wins).
func userMacros() []userMacro {
	return []userMacro{
		{MacroDownloads, `XDG_DOWNLOAD_DIR="$HOME/`,
			[]string{"Downloads", "Загрузки", "Téléchargement", "Scaricati", "Descargas"}},
		{MacroMusic, `XDG_MUSIC_DIR="$HOME/`,
			[]string{"Music", "Музыка", "Musique", "Musica", "Música", "Musik"}},
		{MacroVideos, `XDG_VIDEOS_DIR="$HOME/`,
			[]string{"Videos", "Видео", "Vidéos", "Video", "Vídeos"}},
		{MacroPictures, `XDG_PICTURES_DIR="$HOME/`,
			[]string{"Pictures", "Изображения", "Images", "Immagini", "Imágenes", "Imagens", "Bilder"}},
		{MacroDesktop, `XDG_DESKTOP_DIR="$HOME/`,
			[]string{"Desktop", "Рабочий стол", "Bureau", "Scrivania", "Escritorio", "Área de trabalho", "Schreibtisch"}},
		{MacroDocuments, `XDG_DOCUMENTS_DIR="$HOME/`,
			[]string{"Documents", "Документы", "Documenti", "Documentos", "Dokumente"}},
	}
}

// UserDirs resolves firejail's user folder macros (${DOWNLOADS}, ${MUSIC}, ${VIDEOS},
// ${PICTURES}, ${DESKTOP}, ${DOCUMENTS}) the way firejail 0.9.80 does. The map is keyed
// by the macro as written in a profile (the Macro constants); a macro that does not
// resolve is absent.
//
// It follows src/firejail/macros.c: resolve_macro tries resolve_xdg, then
// resolve_hardcoded. resolve_xdg reads $HOME/.config/user-dirs.dirs (always under the
// home, whatever $XDG_CONFIG_HOME says), skips blank lines and lines starting with '#',
// and takes, from the first line that starts with 'XDG_<NAME>_DIR="$HOME/' and has a
// closing '"', the text between them; the value counts only if it is not empty and
// $HOME/<value> exists; when it does not count, firejail stops reading the file and
// falls back to resolve_hardcoded. resolve_hardcoded
// returns the first entry of the macro's translation list that exists in the home.
//
// A missing user-dirs.dirs is not an error (firejail ignores it); one that exists and
// cannot be read is. The home folder must be absolute.
func UserDirs(ctx context.Context, env Env) (map[string]UserDir, error) {
	home := env.Home()
	if !filepath.IsAbs(home) {
		return nil, errors.New(tagUserDirsHomeNotAbsolute, "the home folder is not an absolute path", "home", home)
	}
	home = filepath.Clean(home)
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr(tagUserDirsCanceled, err)
	}

	file := filepath.Join(home, ".config", "user-dirs.dirs")
	var lines []string
	data, err := os.ReadFile(file)
	switch {
	case err == nil:
		lines = strings.Split(string(data), "\n")
	case errors.Is(err, fs.ErrNotExist):
		// like fopen failing in resolve_xdg: no xdg lines, fall back to the translations
	default:
		return nil, errors.NewErr(tagUserDirsReadFailed, err).SetMetadata("path", file)
	}

	out := make(map[string]UserDir)
	for _, m := range userMacros() {
		if err := ctx.Err(); err != nil {
			return nil, errors.NewErr(tagUserDirsCanceled, err)
		}
		rel, ok := resolveXDG(home, lines, m.xdg)
		if !ok {
			rel, ok = resolveHardcoded(home, m.translations)
		}
		if ok {
			out[m.name] = UserDir{Relative: rel, Path: filepath.Join(home, rel)}
		}
	}
	return out, nil
}

// resolveXDG follows resolve_xdg of macros.c over the lines of user-dirs.dirs.
func resolveXDG(home string, lines []string, prefix string) (string, bool) {
	for _, line := range lines {
		line = strings.TrimLeft(line, " \t")
		if line == "" || line[0] == '#' {
			continue
		}
		rest, found := strings.CutPrefix(line, prefix)
		if !found {
			continue
		}
		value, _, closed := strings.Cut(rest, `"`)
		if !closed {
			continue // firejail keeps reading the next lines
		}
		if value == "" {
			return "", false
		}
		if _, err := os.Stat(filepath.Join(home, value)); err != nil {
			return "", false
		}
		return value, true
	}
	return "", false
}

// resolveHardcoded follows resolve_hardcoded of macros.c: the first entry that exists
// in the home.
func resolveHardcoded(home string, entries []string) (string, bool) {
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(home, e)); err == nil {
			return e, true
		}
	}
	return "", false
}

const (
	tagUserDirsHomeNotAbsolute = "UserDirsHomeNotAbsolute"
	tagUserDirsCanceled        = "UserDirsCanceled"
	tagUserDirsReadFailed      = "UserDirsReadFailed"
)
