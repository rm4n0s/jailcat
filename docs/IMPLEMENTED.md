# Implemented steps

This is the log of finished steps from `docs/IMPLEMENTATION.md`, newest at the bottom. The manager adds an entry on the step's branch, as part of the step's commit (`docs/IMPLEMENTATION.md`, rules 10–12). The next step to do is the first step in `docs/IMPLEMENTATION.md` that has no entry here.

## Entry format

Copy this block, fill it in, and keep every heading. Write "none" where something doesn't apply.

```markdown
## <id> <step title>

- **Date:** YYYY-MM-DD
- **Branch:** feat/step-<id>-<slug>
- **Summary:** 2–4 sentences on what now exists and works.
- **Main files:** the packages or files created or changed.
- **Tests:** what was added.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests ✅/none · display tests ✅/none
- **Proof:**
  ![<id> <name>](implemented-proof-images/<id>-<name>.png)
  One sentence describing what the image shows, as you verified it by viewing it.
- **Manual checks:** the user's answers to the step's manual checks, or "none".
- **Deviations and follow-ups:** differences from SPEC (also changed in SPEC), and TODOs left for later steps.
```

---

<!-- Entries start below this line. -->

## 0.1 Module, layout and docs

- **Date:** 2026-10-27
- **Branch:** feat/step-0.1-module-layout-docs
- **Summary:** The Go module `github.com/rm4n0s/jailcat` (go 1.27) exists with Glutter v1.9.0, rm4n0s/errors v0.2.0, GORM v1.31.2 and its SQLite driver v1.6.0. Every package of CLAUDE.md "Layout" has a `doc.go` naming the SPEC sections it implements. CLAUDE.md, the SPEC, this plan and the lint config are committed.
- **Main files:** `go.mod`, `go.sum`, `cmd/jailcat/main.go`, `internal/*/doc.go`, `assets/README.md`, `.gitignore`, `.golangci.yml`, `CLAUDE.md`, `docs/SPEC.md`, `docs/IMPLEMENTATION.md`, `docs/IMPLEMENTED.md`.
- **Tests:** none (no code yet).
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** `cmd/jailcat/main.go` holds the four dependencies with blank imports so `go mod tidy` keeps them; step 4.1 replaces them with real use.

## 1.1 Folders (`internal/xdg`)

- **Date:** 2026-10-27
- **Branch:** feat/step-1.1-folders
- **Summary:** `internal/xdg` resolves every folder of SPEC §3.1 from an `Env` that is passed in (`FromOS` is the only reader of the process environment), with the XDG rule that unset, empty or relative values fall back to defaults. Firejail's user config folder is always `$HOME/.config/firejail`, and the user-folder macros (`${DOWNLOADS}`, …) resolve exactly as firejail 0.9.80's `macros.c` does. `EnsureDirs` creates Jailcat's own folders with the modes of §3.1; `OnPath` says whether the bin folder is on `$PATH`.
- **Main files:** `internal/xdg/env.go`, `dirs.go`, `userdirs.go`, `ensure.go`, `onpath.go`, `xdg_test.go`, `umask_test.go`; `docs/SPEC.md` §3.1 and §8.1.
- **Tests:** table tests for `Resolve` (set, unset, empty, relative variables; `XDG_DATA_DIRS`; `XDG_BIN_HOME`; firejail's folder under `$HOME`), `UserDirs` (user-dirs.dirs cases, firejail's translations, prefix match, `XDG_CONFIG_HOME` ignored), `EnsureDirs` (modes under umask 077, existing modes kept, Bin/Applications/Icons not created), `OnPath`; route tests for all 9 tags.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** SPEC §3.1 corrected: firejail ignores `$XDG_CONFIG_HOME` (`profile.c` builds `$HOME/.config/firejail`), and §8.1 now says how firejail resolves the user-folder macros. `Dirs` also has `Launchers` (`<data>/launchers`, SPEC §9.3). An unreadable `user-dirs.dirs` is an error (`UserDirsReadFailed`) where firejail would ignore it.

## 1.2 Desktop entries (`internal/desktopentry`)

- **Date:** 2026-10-27
- **Branch:** feat/step-1.2-desktop-entries
- **Summary:** `internal/desktopentry` reads and writes `.desktop` files by the Desktop Entry Specification: groups and keys in file order with comments kept, localized lookup in the spec's order, string and list escapes, booleans, actions in `Actions=` order, and desktop file IDs. `SplitExec`, `QuoteExecArg` and `JoinExec` handle the `Exec` key's quoting and field codes. Every one of the 210 real entries in `/usr/share/applications` parses and its `Exec` lines split (checked once by the manager, not a test).
- **Main files:** `internal/desktopentry/entry.go`, `parse.go`, `write.go`, `value.go`, `exec.go`, `fileid.go`, their tests, and `testdata/` (VS Code, Firefox, quoted-Exec, comment and CRLF fixtures, 7 broken files, a golden built entry).
- **Tests:** parsing of every fixture and error case; localized lookup order; lists and booleans; `Exec` round trips over every reserved character, `%`, newline, non-ASCII and the empty argument; byte-identical write round trips; route tests for all 18 tags. 100% statement coverage.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** `JoinExec` returns `(string, error)` (an unlisted field code is refused). A repeated key in a group is an error (`DuplicateKey`), as the spec says; a file that fails to parse is skipped by step 1.3. A newline in an `Exec` argument is written inside quotes and escaped as `\n`. Deprecated field codes are dropped on reading.

## 1.3 Finding apps (`internal/apps`)

- **Date:** 2026-10-27
- **Branch:** feat/step-1.3-finding-apps
- **Summary:** `apps.Scan` lists every app of SPEC §4: desktop entries from the user and system `applications` folders (first folder wins for an ID, `Hidden`, `NoDisplay`, `TryExec`, Jailcat's own entries dropped), with the binary resolved through `env`, firejail wrappers, Flatpak, Snap and firecfg symlinks, and every executable on `$PATH` not already run by an entry. Dropped entries come back in `Skipped` with a reason. `Search` filters and orders the list (§4.3). On this machine (checked once by the manager) it finds 202 desktop apps and 3,329 binaries, and the 20 skipped entries are real faults (missing binaries, no `Exec`).
- **Main files:** `internal/apps/apps.go`, `desktop.go`, `command.go`, `lookup.go`, `binaries.go`, `search.go`, their tests, `testdata/fake/` (43 desktop fixtures).
- **Tests:** the whole fake system (shadowed and hidden IDs, every `Exec` form, firecfg symlink shadowing, `$PATH` order, actions, locales, unreadable folders); `LocaleFromEnv`; `Search` fields, ordering, filters; the `ScanCancelled` route test.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** an unknown `env` option (`-S`, `-C`) drops the entry instead of guessing; a Snap path is not checked for existence; an entry with no `Name` falls back to the binary name; the Flatpak and Snap launchers themselves stay in the binaries list.

## 1.4 Finding icons (`internal/icons`, lookup part)

- **Date:** 2026-10-27
- **Branch:** feat/step-1.4-icon-lookup
- **Summary:** `icons.Lookup` finds an app's icon by the Icon Theme Specification: absolute paths as they are, then the theme chain (`Inherits`, ending in `hicolor`), scalable SVG first, else the largest fixed size with SVG before PNG, then unthemed files and `pixmaps`; XPM only when nothing else exists. `CurrentTheme` reads the theme name from GTK 4, GTK 3 or KDE settings files. On this machine (checked once by the manager) Firefox gets hicolor's 256px PNG and breeze's own SVGs are found through its `index.theme`.
- **Main files:** `internal/icons/theme.go`, `lookup.go`, `current.go`, `ini.go`, their tests, `testdata/fake/` (themes `Test` → `Parent` → `hicolor`, a loop, settings files).
- **Tests:** absolute paths, theme chains and loops, size and format order, XPM and pixmaps fallbacks, `CurrentTheme` sources, I/O failures and cancellation; route tests for the 6 tags. 98.8% coverage.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** pixmaps are searched in `<each XDG data dir>/pixmaps` rather than only `/usr/share/pixmaps` (the same folder by default). The first theme in the chain that has the icon wins, even if a parent has a larger one. `LoadTheme`, `BaseDirs` and `PixmapDirs` are exported for the UI.

## 2.1 Reading profiles (`internal/firejail`, reader)

- **Date:** 2026-10-27
- **Branch:** feat/step-2.1-profile-reader
- **Summary:** `firejail.Read` reads a profile and its include chain the way firejail 0.9.80's `profile_read` and `profile_check_line` do: `fgets` chunking, comment and blank normalization (`line_remove_spaces`), include resolution and the depth limit, the ignore list with `is_in_ignore_list`'s matching and the 32-entry limit, every conditional with its real trigger, `quiet`, and the lines firejail makes itself (`whitelist-ro`, `noprinters`). Every rule names the C function it follows. `ReadConfig` reads `firejail.config`; `Commands` lists the 129 profile commands.
- **Main files:** `internal/firejail/profile_line.go`, `profile_read.go`, `profile_config.go`, `profile_commands.go`, their tests, `testdata/etc-firejail/` (16 files copied unchanged from `/etc/firejail`, GPL-2.0, with a README).
- **Tests:** `Normalize` and `Matches` tables; the real `code.profile` chain (effective lines, ignore entries, the ignored `dbus-user none` and `disable-xdg.inc`, missing `.local` files); user-folder precedence; every conditional and its ordering; 32 vs 33 ignores; include depth and cycles; `ReadConfig`; route tests for the 15 tags. 98.8% coverage.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** SPEC §8.1 corrected from firejail's source: only missing `.local` includes are skipped, there is a depth limit instead of a cycle check, `whitelist-ro` and `noprinters` make lines no `ignore` can remove, and the conditionals' triggers are listed. §9.1 says rows from `whitelist-ro` are locked; §7.2 allows four DNS servers. `keep-hostname` (a removed command) is not in the command list. A bare include name is not looked up in the working directory, which firejail does as a last resort.
