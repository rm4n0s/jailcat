# CLAUDE.md

Guidance for Claude Code when working in this repository (Go, hosted on GitHub).

## Project
Jailcat is a desktop GUI for [firejail](https://firejail.wordpress.com/), written in Go with [Glutter](https://github.com/rm4n0s/glutter) (Flutter's framework API for Go) for the UI and [GORM](https://gorm.io) on SQLite for storage. It lists every application the user can start: the desktop entries of the application menu, and every executable on `$PATH`, the way bash completes a command. The user picks one (say VS Code), chooses what the sandbox allows (which folders it can see, read-only or read-write; network; sound; devices; and the other firejail options), and presses **Build**. Jailcat then writes the sandbox for that app and installs it under a new name, `firejail-<app>` (e.g. `firejail-code`):
- a launcher script in the user's bin folder (`~/.local/bin/firejail-code`), so it runs from bash
- a desktop entry in the user's application menu (`~/.local/share/applications/firejail-code.desktop`)
- an SVG icon, so the sandboxed app is easy to find in the menu: by default the app's own icon with a firejail badge over its corner, or, if the user asks, a random one Jailcat makes

Every build is saved in Jailcat's own data folder, in a SQLite database, so it can be opened, changed, rebuilt or uninstalled later. Linux only.

**`docs/SPEC.md` is the source of truth.** Read the sections that cover a task before starting it. If the code and the spec disagree, or a task needs a decision the spec doesn't make, ask instead of guessing. When a decision changes, update the spec on the same branch.

## Commands
- Build: `go build ./...`. It needs cgo and a C compiler (`gcc`): GORM's SQLite driver is built on `mattn/go-sqlite3`
- Test all: `go test -race ./...`
- Test one package: `go test ./internal/firejail -run TestName -v`
- Tests that run the real `firejail`: `go test -tags jailcat_firejail ./...`
- Tests that need a real display/GPU: `go test -race -tags jailcat_display ./...`
- Update golden images: `go test ./... -run Golden -update-goldens`, then review every changed PNG
- Run the app: `go run ./cmd/jailcat`
- Run the app against a scratch home (always, for manual checks and proofs): point `XDG_DATA_HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME` and `XDG_BIN_HOME` at folders under `.jailcat-test/` in the module root, so nothing lands in the real `~/.local`
- Vet: `go vet ./...`
- Lint: `golangci-lint run`
- Format: `gofmt -w .` (or `goimports -w .`)
- After changing dependencies: `go mod tidy`

## Definition of done
Before reporting a task as finished, run `gofmt`, `go vet ./...`, `golangci-lint run` and `go test -race ./...`, and fix anything they report. Say so explicitly if any step was skipped or failed. Also:
- A change to what a build writes (profile, launcher script, desktop entry, icon) also passes `go test -tags jailcat_firejail ./...` when `firejail` is installed; say so if it isn't
- Every new exported identifier has a doc comment
- Every changed golden image has been looked at, not just regenerated

## Layout
- `cmd/jailcat/` is the app's `main`: it reads the environment once, opens the database and runs the UI
- `internal/xdg/` resolves the folders Jailcat reads and writes (XDG base directories, the bin folder, Jailcat's data folder) from an environment it is given. No other package calls `os.Getenv`, `os.UserHomeDir` or hard-codes `~/.local`
- `internal/apps/` finds the applications: desktop entries from `$XDG_DATA_HOME/applications` and every `$XDG_DATA_DIRS/*/applications`, and the executables on `$PATH`
- `internal/desktopentry/` reads and writes `.desktop` files (the freedesktop Desktop Entry Specification: groups, localized keys, `Exec` field codes and quoting)
- `internal/icons/` finds an app's icon (the Icon Theme Specification: the current theme, `hicolor`, `/usr/share/pixmaps`) and writes the SVG icon of a build: the app's icon with the firejail badge, or a random one
- `internal/firejail/` is the sandbox model: the options a user can choose, turned into a firejail profile and a launcher script; it also finds `firejail` and reads its version
- `internal/store/` is the database: the GORM models, opening and migrating the SQLite file. GORM types never leave this package; it hands out the plain types of `internal/firejail` and `internal/apps`
- `internal/install/` writes a build's files into the user's folders and removes them on uninstall, and only those files
- `internal/ui/` holds the Glutter widgets and screens
- `assets/` holds the firejail badge (SVG) and Jailcat's own icon (embedded with `embed`)
- `docs/SPEC.md` is the specification; `docs/IMPLEMENTATION.md` is the step-by-step plan; `docs/IMPLEMENTED.md` logs finished steps; `docs/implemented-proof-images/` holds each step's proof screenshots
- `.jailcat-test/` (git-ignored) holds the scratch home of manual runs and proofs
- Tests live next to the code (`foo_test.go`); golden images live in `testdata/goldens/` of their package; sample desktop entries, icon themes and `$PATH` folders for tests live in `testdata/` of their package

## Go conventions
- Never ignore a returned error; handle it or return it
- Pass `context.Context` as the first parameter of anything that does I/O (the database, the file system, running `firejail`, file dialogs). Name it `ctx`. Name a Glutter `BuildContext` `bc`
- Tests are table-driven
- No global mutable state: no package-level database handle, logger, folder paths or caches. `main` builds them and passes them down through constructors. Package-level read-only values are fine and are never reassigned
- Prefer the standard library. Allowed dependencies: `github.com/rm4n0s/glutter` (with its `svg` package), `github.com/rm4n0s/errors`, `gorm.io/gorm` and `gorm.io/driver/sqlite` (GORM's SQLite driver), with what they pull in; `golang.org/x/image` (already pulled in by Glutter) only for the Go Mono font of the widget tests. Ask before adding any other
- **cgo only for SQLite**: the one cgo in the app is `mattn/go-sqlite3`, under GORM's driver. Jailcat's own code never writes `import "C"`, and no other dependency may need cgo
- Run external programs with `exec.CommandContext` and an argument list, never through `sh -c`, and only `firejail` itself (and, in `jailcat_firejail` tests, the launchers a build wrote)
- The database, the file system and `firejail` are used off the UI thread. Only the UI thread touches the widgets; other goroutines hand results back through `SetState` or the binding's `Post` (Glutter SPEC §13)

## Using Glutter
- Glutter v1.9.0 or newer. Apps import one package: `m "github.com/rm4n0s/glutter/material"`; SVG images are drawn with `github.com/rm4n0s/glutter/svg` (`svg.NewSvgPictureFile`, `svg.NewSvgPictureMemory`). Desktop services (the folder chooser, notifications) are in `github.com/rm4n0s/glutter/desktop`; choose folders with `desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{Directory: true, Multiple: true})`
- Write Jailcat's own widgets the way Glutter's are: `NewX(required..., opts ...XOpts) *X`, required parameters positional, optional ones in a trailing options struct, `child`/`children` last
- Glutter's source is in the module cache (`$(go env GOMODCACHE)/github.com/rm4n0s/glutter@<version>`); read its `docs/SPEC.md` and examples there before using a part of it for the first time
- Glutter belongs to the same owner. If it lacks something Jailcat needs or has a bug, don't work around it quietly: tell the user what is missing, with a minimal example, and ask how to go on

## Firejail and the files a build writes
- Jailcat never runs as root, never calls `sudo`, and never writes to `/etc`, `/usr` or any folder outside the user's own (`internal/xdg`). It does not run `firecfg` and does not change firejail's own profiles
- One function turns the sandbox model into profile text, and one into the launcher script; firejail option names appear nowhere else. Check every option against the man pages of the installed firejail (`man firejail`, `man firejail-profile`) before adding it
- Every value that goes into a generated file is escaped by the one function for that format: POSIX `sh` single quotes for the launcher, the Desktop Entry Specification's rules for `.desktop` keys and `Exec`, firejail's rules for profile lines. Each of these has tests with spaces, quotes, `$`, backticks, newlines, `%` field codes and non-ASCII names. A value that can't be written safely is an error, never dropped or trimmed
- The `firejail-<app>` name is made only of `[a-z0-9-]`. A build never overwrites a file it didn't make: every generated file carries a "generated by Jailcat" header and is recorded in the database, and an existing file without both is a conflict the user resolves in the UI. Uninstall removes only recorded files
- A rebuild writes each file to a temporary file in the same folder and renames it over the old one
- The launcher `exec`s firejail with the absolute path of the app's binary and passes the user's arguments through (`"$@"`)

## Icons
- A build's icon is always an SVG, installed as `$XDG_DATA_HOME/icons/hicolor/scalable/apps/firejail-<app>.svg` and named in the desktop entry by that name (`Icon=firejail-<app>`)
- **Default:** the app's own icon with the firejail badge from `assets/` over its bottom-right corner. An SVG source is inlined as a nested `<svg>` (its `id`s and the references to them get a prefix, so they can't clash with the badge's); a PNG source is embedded as a base64 `data:` URI in an `<image>`. Other formats (XPM) and apps with no icon fall back to a random icon
- **Random:** made from a seed stored with the build, so a rebuild gives the same icon; the user can ask for a new one. It still carries the badge
- The SVG is written as text by Jailcat, with `encoding/xml` for reading the source SVG. Nothing is rasterized to make it, and a source SVG keeps no `<script>`, event attributes or external references (only `data:` URIs and `#` references stay)
- The icon must show in GTK/GNOME (librsvg) and KDE (Qt SVG): stay inside SVG 1.1 features both render. Tests compare the written SVG with golden text in `testdata/`
- Jailcat's own window draws SVG icons with Glutter's `svg` package (SPEC §10.4). Never add another SVG rasterizer

## Database
- One SQLite file in Jailcat's data folder, opened in `cmd/jailcat` and passed to `internal/store`. Every query uses `db.WithContext(ctx)`
- Schema changes go through the migration code in `internal/store`, never by hand. A released database must open in every later version
- Tests open a fresh database in `t.TempDir()`, never a shared one

## Error handling
Use `github.com/rm4n0s/errors` (v0.2.0) for all errors, the way its `AI_GUIDE.md` says. This is a requirement of the project owner: read the whole guide (it is in the module cache, `$(go env GOMODCACHE)/github.com/rm4n0s/errors@v0.2.0/AI_GUIDE.md`) before you write or change code that makes, returns, logs or tests an error, and again when the dependency version changes. Never use `fmt.Errorf`, and never import the standard library `errors` package under the name `errors`: this package re-exports `errors.Is`, `errors.As` and `errors.Unwrap`, so a single import covers everything (if `Join` is needed, import the standard one as `stdErrors "errors"`).

**Creating errors**
- New failure with a fixed message: `errors.New("Tag", "message", "key", value, ...)`. It always has a tag and a message
  - Tag: short PascalCase name of the thing the function cannot do, e.g. `FirejailNotFound`. Declare it as a constant next to the code that produces it (`const tagFirejailNotFound = "FirejailNotFound"` at the end of the file; exported as `TagX`, with a doc comment, when another package matches on it)
  - Trailing arguments are key/value metadata pairs with string keys, e.g. `"path", path`. Give all of them in this one call; never chain `SetMetadata` after `errors.New`
- New failure with a formatted message: `errors.NewErrf("Tag", "format %s", arg)`
- Wrapping an error that comes from outside this module (stdlib, GORM, the SQLite driver, Glutter, `exec`): `errors.NewErr("Tag", err)`, once, where it first appears. `NewErr` and `NewErrf` take no metadata: add it with one `.SetMetadata("key", value, "key2", value2)` call. `SetMetadata` merges, so a later call in another function adds keys; `RemoveMetadata` and `SetMessage` change an error that is already made. `NewErr(tag, nil)` panics, so call it only inside `if err != nil`
- Call the constructor in the function that fails, never in a helper that builds and returns the error: the route is the call stack at the constructor, and a helper adds a frame that is not the failing function (an exported function or method is not a helper)
- Every `New`, `NewErr` and `NewErrf` call captures the stack at that point
- A sentinel for `errors.Is` is a struct literal, `&errors.Error{Tag: TagX}`. Never write `var ErrX = errors.New(...)` at package level: its route is `init`

**Propagating errors**
- Every error is tagged exactly once. If the error came from a function of this module, return it untouched: `return err`. Re-wrapping it (`NewErr`, `NewErrf` with `%w`, `fmt.Errorf`) would replace the Tag and the stack and break route tests; the route already shows who called whom, so no context is added by wrapping
- Functions return the plain `error` interface, never `*errors.Error` (avoids the non-nil-interface-holding-nil-pointer bug)
- `errors.FromError(err)` is a plain type assertion. It fails if something else wrapped the error, so don't wrap these errors with anything

**Inspecting errors**
- `appErr, ok := errors.FromError(err)`, then use `appErr.Tag`, `appErr.Metadata`, `appErr.Message`
- Branch on the tag constant (`appErr.Tag == pkg.TagX`) or on `errors.Is(err, sentinel)` (it matches by Tag), never on a string literal, `Message` or `Error()`
- Use `errors.Is` / `errors.As` / `errors.Unwrap` for the underlying cause stored in `OriginalErr` (e.g. `errors.Is(err, gorm.ErrRecordNotFound)` inside `internal/store`, which turns it into its own tag)

**Logging errors**
- Log with `slog` and pass the error as the attribute `"err"`: `logger.Error("what failed", "err", err)`. `*errors.Error` is an `slog.LogValuer`: the entry has `Tag`, `Message`, `Route`, `OriginalErr`, `Metadata` and `StackFrames`, and a test replays it with `json.Unmarshal` into `errors.ErrorJson` and `errors.FromJson`. Do not call `ToJson` or `json.Marshal` on an error to log it
- Metadata values must be JSON-serializable (strings, numbers, bools, simple structs); after a round trip numbers are `float64`
- An error the user should see is shown in the UI with a message written for them; the tag, route and metadata go to the log

**Testing errors**
- Every new failure case gets its own unique Tag and a test that asserts the route, not just that an error occurred:
  ```go
  appErr, ok := errors.FromError(err)
  if !ok {
      t.Fatalf("expected *errors.Error, got %T", err)
  }
  if !appErr.HasRoute("Installer.Install->writeLauncher.LauncherWriteFailed") {
      t.Errorf("unexpected failure path: %s", appErr.Route())
  }
  ```
- A route lists the call chain from outermost to innermost (`Type.Method` or `Func`, joined by `->`, no package names) and ends with the Tag. `Route` and `HasRoute` stop at the function that called them, so in a test the route starts just below the test function. `HasRoute` matches by substring: give the path from the entry function you call down to `.Tag`, so the tag is checked too. Print `appErr.Route()` when an assertion fails

## Git and GitHub
- Never commit or push directly to `main`; work on a branch named `feat/...` or `fix/...`
- **Exception:** if `docs/IMPLEMENTATION.md` exists, follow its Rules section for the steps it lists. Each step is done on its own branch, one at a time, and a finished step (all Definition-of-done checks passing) is merged into the local `main` by a fast-forward merge, without a PR, after its entry is added to `docs/IMPLEMENTED.md` on that branch. Never commit work for a step directly on `main`. Everything not covered by that file (other changes, fixes outside the listed steps) still follows the rule above
- **Publish milestones, not steps** (the owner's rule, 2026-10-27): nothing is pushed to GitHub after a single step. `docs/IMPLEMENTATION.md` groups the steps into milestones; when the last step of a milestone is merged into the local `main`, push `main` and every tag made since the last push, at once, without asking. A step's branch is deleted locally after its merge and is never pushed
- **Tags:** after a finished step is merged into the local `main`, tag its commit with an annotated tag `step-<id>` (e.g. `step-3.2`); the step counts as finished when the tag is made. The last step of a milestone also gets an annotated tag `milestone-<id>` (e.g. `milestone-2`), and the milestone counts as finished when that tag is on `origin`. **When a milestone finishes a version**, its commit also gets the annotated release tag (`v1.0.0` and the like), pushed with the milestone. Never move or delete a tag
- Commit messages: imperative mood, e.g. "Add retry to client"
- PRs: short description, link the issue (`Closes #123`), keep them small
- No `.github/` directory and no GitHub Actions workflows: never add them. The Definition-of-done checks are run locally

## Working through the steps
- Never stop to ask for confirmation, and never end a turn with a question such as "Should I start the next step?", until every step in `docs/IMPLEMENTATION.md` is finished (through the last one). After a step is merged, report it briefly and start the next step in the same turn; after a milestone is pushed, report the milestone the same way
- A request such as "publish to main" that is already satisfied gets a one-line answer, then work continues with the next step. A request to push in the middle of a milestone is the owner overriding the milestone rule for that once: push `main` and the tags made so far
- **Manager and workers:** the main session (Opus 5.5, high effort) manages and Sonnet 5.5 agents at high effort write the code (Agent tool with `model: "sonnet"`, `effort: "high"`). The manager reads the step and the SPEC, writes the brief, launches and steers the workers, reviews their diffs, runs the checks, and does the git work; it writes only small fixes itself. The details are in `docs/IMPLEMENTATION.md`, Rules, "Who does the work"

## Don't
- Don't edit generated files (`*_gen.go`); regenerate them with `go generate ./...`
- Don't commit secrets, `.env` files, `.jailcat-test/` or a database file
- Don't let a GORM type, a Glutter type or a file path of the real home appear in `internal/firejail`, `internal/apps` or `internal/desktopentry`
- Don't write to the real home. Tests write only into `t.TempDir()`; manual runs and proofs use the scratch home in `.jailcat-test/` (see Commands). Never run a launcher that was installed into the real `~/.local`
- Don't leave processes running: every app started for a test, a proof or a check (Jailcat, a sandboxed app, `firejail`) is stopped when it is no longer needed (run it under `timeout -k`, in its own process group), and `firejail --list` and `pgrep -a` show nothing left before a task is reported done
- Never look for, list, open or read files outside this project folder (no home-directory databases, configs, logs or other projects). If something outside it is needed, ask the user to provide it. Exceptions, read-only: dependency source in the Go module cache; the system folders Jailcat itself reads, to learn their real shape (`/usr/share/applications`, `/usr/local/share/applications`, `/usr/share/icons`, `/usr/share/pixmaps`, the folders on `$PATH`); firejail's own files (`/etc/firejail`, its man pages) and `firejail --version`

## Testing notes
- Discovery, profile, script, desktop-entry and icon tests run on fixtures in `testdata/` (a fake `$PATH`, fake `applications` folders, a small icon theme) through an `internal/xdg` environment that points there, never at the real system
- Generated files are compared with expected files in `testdata/` (golden text). A change to one is reviewed line by line like a golden image
- Widget tests use `gluttertest` (`PumpWidget`, `Pump`, `Tap`, finders) with a fake clock and the fake platform; no display needed. The folder chooser is answered by `Tester.FakeDesktop()`. Screens get golden tests with `Tester.MatchesGolden` (software renderer, deterministic, no GPU)
- Tests that run the real `firejail` carry the `jailcat_firejail` build tag and skip when `firejail` is missing. Inside another sandbox, Claude Code's Bash sandbox included, firejail prints "an existing sandbox was detected" and runs the program unsandboxed: these tests fail when they see that warning, and they are run outside Claude Code's sandbox (ask the user the first time in a session). They build into a `t.TempDir()` home and check what the sandbox does (a folder that wasn't allowed is not visible, `net none` has no network), not just that it starts
- Tests that open real windows carry the `jailcat_display` build tag and are skipped by the default test run
- Every error Tag has a route test
