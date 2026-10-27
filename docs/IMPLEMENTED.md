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

## 2.2 Settings and base values (`internal/firejail`, model)

- **Date:** 2026-10-27
- **Branch:** feat/step-2.2-settings-model
- **Summary:** `Settings` holds every setting of SPEC §7, and a field table (`Fields()`) gives each one its tab, getter, setter and equality, so diffing, applying, JSON and tests are built on it. `BaseValues` maps a read profile to settings, keeping the base lines behind each value (for step 2.3's `ignore` lines), the folder rows of the `.profile` and `.local` files, and the uncovered lines grouped by file. `Overrides` stores only what the user changed, as versioned JSON that keeps fields it doesn't know. `Validate` returns the problems the UI shows.
- **Main files:** `internal/firejail/settings.go`, `fields.go`, `base.go`, `overrides.go`, `validate.go`, their tests.
- **Tests:** the VS Code chain field by field (sound, camera, U2F, DVD, input, TV blocked; private `/dev`, `/tmp`, cache; firewall from `blink-common.profile`; five folder rows; no seccomp, nonewprivs or noroot, since Electron runs its own sandbox), `default.profile`, about 75 per-command cases, diff/apply round trips generated from the field table, folder overrides with locked rows, JSON with unknown fields, every validation rule; route tests for the 4 tags.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** `BaseValues` takes the home path as a second argument. Folder rows come from `.profile` and `.local` files; system folders and globs go to the read-only list (SPEC §7.1 updated). Base rows stay with Empty and Own private home, where `${HOME}` is the private home (SPEC §7.1). From firejail's source: `protocol` lines merge with `+`/`-`/`=`, `cpu` takes 0–31, `nice` is clamped to 0 for non-root, `rlimit-as` takes K/M/G (SPEC §7.7 updated). Launcher fields never come from a profile except `join-or-start`.

## 2.3 Writing profiles (`internal/firejail`, writer)

- **Date:** 2026-10-27
- **Branch:** feat/step-2.3-profile-writer
- **Summary:** `Generate` writes the profile of SPEC §9.1 from a base, overrides and Extra rules: for every field whose final value differs from what the `include` alone gives, it cancels the base lines behind it with `ignore` and writes the final value's lines; positive lines come before the ignores, base lines an `ignore` would hit by accident are written again above it, the 32-ignore limit is counted with the base chain, and Own private home adds `ignore mkdir`/`ignore mkfile`. Every value goes through one escaping function. `CheckExtraRules` checks the Extra rules rows with errors and warnings.
- **Main files:** `internal/firejail/header.go`, `writer.go`, `writer_lines.go`, `writer_value.go`, `extrarules.go`, `validate.go` (Severity, Row, two problems), their tests, `testdata/profiles/` (10 golden profiles); `docs/SPEC.md` §9.1.
- **Tests:** the round trip (generate → read with step 2.1's reader → base values → equal to the edited settings) for every field over 8 bases, every home mode with every folder-row kind, and seeded random combinations; the mutation check (removing the cancel, the rewrite or the shared-line handling makes it fail); golden profiles reviewed line by line by the manager; over-matching, `whitelist-ro`, `noprinters`, conditionals, the ignore limit, every escaping error, about 120 Extra-rules rows; route tests for all 14 tags.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none (step 3.4) · display tests none
- **Proof:** C: the golden profiles `internal/firejail/testdata/profiles/code-changes.profile`, `code-home-private.profile`, `code-over-matching.profile` and `none-defaults.profile`.
- **Manual checks:** none
- **Deviations and follow-ups:** the file order is now positive lines, then `ignore` lines, then Extra rules, then `include` (SPEC §9.1 updated). A base conditional line that Jailcat's changes would turn false is written again without its condition; a false one that becomes true is left to firejail. `private-cache` also acts on the real home under `private DIR` (man page), but only as a mount inside the sandbox, so nothing is written for it. Chosen home without any `whitelist` under the home is refused.

## 2.4 Launcher script (`internal/firejail`, launcher)

- **Date:** 2026-10-27
- **Branch:** feat/step-2.4-launcher
- **Summary:** `WriteLauncher` writes the launcher of SPEC §9.2 with the shared header: `exec` of firejail's absolute path, `--profile=…`, `--`, the binary, the extra arguments and `"$@"`, every word through `ShellQuote` (POSIX single quotes; NUL refused). `FindFirejail` finds firejail on `$PATH`, and `Version` reads `firejail --version` with a 5-second limit.
- **Main files:** `internal/firejail/shellquote.go`, `launcher.go`, `find.go`, their tests, `testdata/launchers/` (4 golden launchers).
- **Tests:** `ShellQuote` table; golden launchers byte for byte, each passing `sh -n`; a behaviour test where a fake firejail prints its arguments NUL-separated and every user argument (spaces, quotes, `$HOME`, `*`, newline, empty, `--`) arrives unchanged and in order; `FindFirejail` and `Version` cases with fake scripts; route tests for the 9 tags.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none (step 3.4) · display tests none
- **Proof:** C: `internal/firejail/testdata/launchers/special-args.sh`.
- **Manual checks:** none
- **Deviations and follow-ups:** `--` is written unquoted, as in SPEC §9.2. Only Jailcat's own 5-second limit counts as a version timeout; a cancelled caller context is `FirejailVersionFailed`.

## 3.1 Building icons (`internal/icons`, writer)

- **Date:** 2026-10-27
- **Branch:** feat/step-3.1-building-icons
- **Summary:** `BadgeIcon` writes a build's 128×128 SVG icon: the app's SVG (parsed, sanitized by an element allowlist, ids prefixed and references rewritten) or PNG (embedded as a data URI) as a nested `<svg>`, with the jail-bar badge at the bottom-right. KDE's colour-scheme `<style>` rules are applied as attributes, so breeze icons keep their colours. `RandomIcon` draws a seeded shape, gradient and initial with the badge. The badge and Jailcat's own icon are hand-written SVGs in `assets/`.
- **Main files:** `assets/badge.svg`, `assets/jailcat.svg`, `assets/embed.go`; `internal/icons/compose.go`, `sanitize.go`, `cssclass.go`, `svgnode.go`, their tests, `testdata/compose/`.
- **Tests:** byte-for-byte golden SVGs (simple, ids/gradients/`use`, hostile, PNG, VS Code-like, breeze, size-only, three random), each rendered by `rsvg-convert` without warnings; pixel checks of the colour-scheme rules and their precedence; 26 hostile `<style>` blocks dropped; refused sources (XPM, DTD entities, oversize, deep nesting, fake PNG); determinism; contrast of the random palette; route tests for the 3 tags.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:**
  ![3.1 badge on the VS Code-like icon](implemented-proof-images/3.1-badge-code.png)
  A blue rounded square with a white `</>` and the amber jail-bar shield at the bottom-right, viewed by the manager.
  ![3.1 random icons](implemented-proof-images/3.1-random.png)
  Three random icons, a blue circle with F, a teal rounded square with C and a purple hexagon with É, each with the badge, viewed by the manager.
- **Manual checks:** none
- **Deviations and follow-ups:** every icon starts with an XML declaration and Jailcat's header comment (SPEC §9.5's ownership check needs it). Of the system's icons, no hicolor app SVG uses `<style>`; 27% of breeze's do, all in the colour-scheme form now supported (SPEC §10.2 updated). Qt SVG rendering was not checked; only librsvg was available.

## 3.2 Database (`internal/store`)

- **Date:** 2026-10-27
- **Branch:** feat/step-3.2-database
- **Summary:** `internal/store` keeps builds and their installed files in SQLite through GORM, in english-tutor's pattern: the file is created with mode 0600, foreign keys are on for every connection, and the schema comes from numbered migrations with frozen structs (`001_init`). The API hands out plain `Build`, `Draft` and `InstalledFile` values; overrides are stored as firejail's versioned JSON and drafts as one JSON column. `RecordInstall` replaces a build's files in one transaction.
- **Main files:** `internal/store/store.go`, `migrations.go`, `build.go`, `builds.go`, `files.go`, their tests.
- **Tests:** every method on an in-memory and a file database; mode 0600; migrations applied twice; foreign keys on three pooled connections; unique sandbox name and installed path; cascade delete; an atomic `RecordInstall` that fails and keeps the old rows; overrides and drafts round trips; route tests for every tag, firejail's JSON errors passed back unwrapped.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none · display tests none
- **Proof:** none
- **Manual checks:** none
- **Deviations and follow-ups:** saving or clearing a draft does not change `UpdatedAt`. App kinds, icon modes and file kinds are constants but not checked on write.

## 3.3 Installing (`internal/install`)

- **Date:** 2026-10-27
- **Branch:** feat/step-3.3-installing
- **Summary:** `Prepare` turns a build into its files (profile, launcher, SVG icon with its fallback reason, the desktop entry of SPEC §9.3 with field codes, copied keys, `StartupWMClass` and same-binary actions, and the private home) without writing anything. `Check` classifies every target by SPEC §9.5's ownership rules, `Install` re-checks and writes atomically (temp file, fsync, chmod, rename) with the desktop entry last, and `Uninstall` removes only recorded files that are unchanged, keeping the private home unless asked.
- **Main files:** `internal/install/files.go`, `prepare.go`, `desktop.go`, `check.go`, `install.go`, `uninstall.go`, their tests, `testdata/` (apps, icons, golden texts).
- **Tests:** a whole VS Code-like install into a temp home compared with golden texts; every conflict kind, unconfirmed and confirmed overwrites, a stale plan; icon fallbacks; field codes, actions, localized action names, `StartupWMClass`, `MimeType`, menu and bin switches; the entry parses back; uninstall cases; route tests for all 21 tags. 94% coverage.
- **Checks:** gofmt ✅ · go vet ✅ · golangci-lint ✅ · go test -race ✅ · firejail tests none (step 3.4) · display tests none
- **Proof:** C: `internal/install/testdata/golden/vscode.desktop`, `vscode.sh` (the launcher), `vscode.profile`, reviewed by the manager.
- **Manual checks:** none
- **Deviations and follow-ups:** `Install` takes the owner lookup (it re-runs `Check`); `UninstallOpts` carries `Dirs`. An edited file is overwritable only if it still has Jailcat's header. The main `Exec` keeps only the app's first field code, not its other arguments. An empty menu name is refused when the entry is installed.
