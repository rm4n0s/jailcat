# Jailcat: Implementation guide

This file breaks `docs/SPEC.md` into small steps, grouped into milestones. Each step is one unit of work. Work goes strictly top to bottom. Every finished step is recorded in `docs/IMPLEMENTED.md`.

**Who this is for**: coding agents (and humans) implementing Jailcat. You don't need to read the whole file; you need the Rules below and the one step you're doing.

---

## Rules

### Who does the work

The owner's choice: the main session is the **manager** (Opus 5.5, high effort) and **workers** write the code (Sonnet 5.5 agents, launched with the Agent tool, `model: "sonnet"`, `effort: "high"`).

- **The manager** does rules 1–3 (finds the step, reads it and the SPEC sections, creates the branch), then writes a brief for each worker in `.claude/briefs/step-<id>[-<part>].md` (git-ignored; never committed). A brief is the only thing the worker knows: the goal, the tasks it owns, the files and SPEC sections to read, the tests to write, the decisions already made and those it must bring back instead of making, and the rules it must follow (CLAUDE.md, the "Errors" rules below, doc comments, processes it starts are stopped, nothing written outside `t.TempDir()` and `.jailcat-test/`).
- **Workers run in the shared checkout, on the step branch, without worktree isolation**: an isolated worktree starts from `main`, not from the step branch. A worker never runs a git command that changes state (commit, merge, rebase, stash, push, tag, branch, switch, reset, checkout of files); the manager does all of them.
- **One worker per step**, or several at once only when their parts touch disjoint directories and do not depend on each other; at most three at a time. A part that needs another part's API waits for it.
- **The manager reviews the work, not the summary**: it reads the diff, runs the tests the worker says pass and the checks of rule 8 itself, and looks at every proof image. What is wrong, missing or outside the step goes back to the same worker (SendMessage, so it keeps its context) with the concrete failure. The manager writes only small fixes itself (a few lines, a doc comment, a test table row).
- **Questions.** A worker that hits a case of rule 7 stops and reports it; the manager decides what the plan and the SPEC already decide, and asks the user otherwise (rule 7). The manager never reports a worker's result before it arrived.
- **The manager finishes the step**: the `IMPLEMENTED.md` entry (rule 10; it may have a worker draft it, and checks every claim in it), the commit, the merge into the local `main` and the tag (rules 11–12), the push of a finished milestone (rule 12), the report (rule 13), and deletes the step's briefs.

### Before you start a step

1. **Find your step.** It's the first step in this file that has no entry in `docs/IMPLEMENTED.md`. Do only that step. Never work on two steps at once, and never skip ahead.
2. **Read, in this order:**
   1. `CLAUDE.md`, all of it.
   2. The whole step in this file.
   3. Every section of `docs/SPEC.md` listed under **Read**.
   4. The Glutter sources the step names, in the module cache (`$(go env GOMODCACHE)/github.com/rm4n0s/glutter@<version>`), and firejail's man pages or source when the step names them.
3. **Create the branch.**
   ```sh
   git switch main
   git switch -c feat/step-<id>-<slug>     # e.g. feat/step-1.4-profile-reader
   ```

### While you work

4. **Do exactly the step's Tasks.** Don't do work that belongs to a later step, even if it looks easy. If a later step will need a hook, leave only a comment: `// TODO(step 3.2): ...`.
5. **Follow CLAUDE.md**, in particular the error rules, the escaping rules of "Firejail and the files a build writes", and "Don't write to the real home".
6. **Follow firejail, don't guess.** When a step depends on how firejail reads a profile, read the man pages of the installed firejail (0.9.80) and, when they don't say, its source (`https://raw.githubusercontent.com/netblue30/firejail/0.9.80/src/firejail/...`). Name the file and function in a comment where the code follows it.
7. **Stop and ask the user** when any of these happens:
   - the step is ambiguous
   - the spec looks wrong or contradicts firejail
   - a tool is missing (give the exact install command, e.g. `sudo dnf install firejail`)
   - a new dependency seems necessary
   - something needs `sudo`
   - a check still fails after a real attempt to fix it

   Never weaken or delete a test, skip a check, or mark a step done just to get unblocked.

**Processes you start.** Every app, sandbox or compositor you start for a test, a proof or a check is stopped when you are done with it: run it under `timeout -k`, in its own process group, and stop it from `t.Cleanup` or `defer` in Go code. Before you finish a step, check with `pgrep -a` and `firejail --list` that nothing you started is still running. The user works on this desktop and sees every stray window.

**Firejail tests.** Inside Claude Code's Bash sandbox firejail sees "an existing sandbox" and runs programs unsandboxed. `jailcat_firejail` tests therefore run outside it (`dangerouslyDisableSandbox`), which needs the user's approval; they fail, never skip, when they see that warning.

**Errors.** `github.com/rm4n0s/errors` v0.2.0 is used to the fullest, as its `AI_GUIDE.md` says (`$(go env GOMODCACHE)/github.com/rm4n0s/errors@v0.2.0/AI_GUIDE.md`). Read all of it before you write or change code that makes, returns, logs or tests an error. CLAUDE.md "Error handling" has the rules; in short:
- **R1.** `errors.New(tag, message, metadata...)` always has a tag and a message.
- **R2.** Metadata is given in one call: to `errors.New`, or in exactly one `SetMetadata` after `NewErr`/`NewErrf`.
- **R3.** An error is tagged once, where it first appears. An error from a function of this module is returned as it is. An error from outside the module (standard library, GORM, the SQLite driver, Glutter, `exec`) is tagged with `errors.NewErr` where it first appears.
- **R4.** The constructor is called in the function that fails, never in a helper that builds and returns the error.
- **R5.** A tag is a constant next to the code that produces it (`const tagX = "X"`, or `TagX` with a doc comment when another package matches on it). A sentinel is `&errors.Error{Tag: TagX}`.
- **R6.** Import only `github.com/rm4n0s/errors`; the standard one, when needed, as `stdErrors "errors"`.
- **Logging** with `slog`, the error as the attribute `"err"`. **Tests**: every tag has a route test with `HasRoute("Outer->Inner.Tag")`.

Glutter's `glutter-gen errcheck` is tied to Glutter's module, so these rules are checked by the manager's review of every diff.

### Finishing a step

8. **The standard checks must all pass.** Run them from the repo root:
   ```sh
   test -z "$(gofmt -l .)"
   go vet ./...
   golangci-lint run
   go test -race ./...
   ```
   If the step lists **Firejail tests**, run them too, outside Claude Code's sandbox: `go test -tags jailcat_firejail <packages>`. If the step lists **Display tests**, run them under `scripts/headless-wayland.sh` (from step 4.1): `go test -race -tags jailcat_display <packages>`.
9. **Proof images.** Only when the step has a **Proof** item:
   - Produce exactly the images it lists, in `docs/implemented-proof-images/`, named `<id>-<name>.png`.
   - Use the method it names (see "Proof methods").
   - **Open every image with your image-viewing tool** and check that it shows what **Proof** describes. If it doesn't, fix the code. The step isn't done.
   - Never draw, edit, crop or generate a proof image any other way, and never reuse an older image.
10. **Record the step.** On the step branch, add an entry to `docs/IMPLEMENTED.md`, using the format at the top of that file. Embed the proof images.
11. **Commit on the step branch.** One commit per step, with the message `Step <id>: <imperative summary>`.
12. **Merge every finished step into the local `main` right away; publish to GitHub only a finished milestone** (the owner's rule of 2026-10-27).
    ```sh
    git switch main
    git merge --ff-only feat/step-1.4-profile-reader
    git tag -a step-1.4 -m "Step 1.4: Read firejail profiles"
    git branch -d feat/step-1.4-profile-reader
    ```
    - A step counts as finished when its commit is on the local `main` with its tag. Merge only after rules 8–11 are complete.
    - If a tag already exists, stop and tell the user; never move or delete a tag.
    - **Publish the milestone** when its last step is merged and tagged: tag that commit `milestone-<n>` (annotated), then `git push --follow-tags origin main`, then check with `git ls-remote --tags origin` that every tag of the milestone is there. The milestone is finished only then. If the push fails (authentication, network), retry once; if it still fails, stop and tell the user.
    - A milestone that finishes a version also gets its annotated release tag (`v1.0.0`), made without asking and pushed with the milestone.
    If `--ff-only` fails, rebase the step branch onto `main`, run the standard checks again, and retry.
13. **Report to the user.** Give the step id, a two-line summary, the check results, the proof image paths, and the commit hash now on `main`. Then start the next step in the same turn.

**Default "Done when".** A step without a **Done when** line is done when its Tasks are complete, its **Tests** pass, its **Firejail tests** and **Display tests** pass, its **Proof** images are verified, and the standard checks pass.

### Proof methods

| Method | Use it for | How |
|---|---|---|
| **A. Widget golden** | a screen or widget | A `gluttertest` test pumps the widget with fixture data and saves `Tester.Screenshot()` to the proof path when `JAILCAT_PROOF_DIR` is set (the same test also checks it with `MatchesGolden`) |
| **B. App self-capture** | the running app | `XDG_DATA_HOME=$PWD/.jailcat-test/data XDG_CONFIG_HOME=$PWD/.jailcat-test/config XDG_CACHE_HOME=$PWD/.jailcat-test/cache XDG_BIN_HOME=$PWD/.jailcat-test/bin GLUTTER_SCREENSHOT=docs/implemented-proof-images/<file>.png GLUTTER_SCREENSHOT_AFTER=3s timeout -k 5 60 scripts/headless-wayland.sh -- go run ./cmd/jailcat` (the script is `scripts/headless-wayland.sh`, from step 4.1). The scratch home keeps everything Jailcat writes under `.jailcat-test/`; the system's apps are only read. The app saves its own window and exits |
| **C. Generated files** | profile, launcher, desktop entry, icon | The file itself is the proof: the step lists which golden text files under `testdata/` to show in the `IMPLEMENTED.md` entry; an icon is shown by rendering it with `rsvg-convert -w 128 -o docs/implemented-proof-images/<file>.png <icon.svg>` |
| **D. User desktop** | what only the real menu and taskbar show | Ask the user to run the command given in the step and save a screenshot at the given path. Then view and verify the image yourself |

---

## Milestone overview

| Milestone | Steps | What works at the end |
|---|---|---|
| M0 Skeleton | 0.1 | The module builds; the docs are committed |
| M1 Reading the system | 1.1–1.4 | Folders, desktop entries, apps on the menu and `$PATH`, icon lookup |
| M2 Firejail model | 2.1–2.4 | Reading profiles, the settings and their base values, writing profile and launcher |
| M3 Icons, storage, installing | 3.1–3.4 | SVG icons, the database, installing and uninstalling a build, running firejail |
| M4 The app | 4.1–4.7 | The whole GUI |
| M5 1.0 | 5.1 | Checked on the real desktop, released as `v1.0.0` |

---

## M0: Skeleton

### 0.1 Module, layout and docs
**Goal:** a module that builds and passes the checks, with the docs committed.
**Read:** CLAUDE.md; SPEC §3.
**Tasks:**
1. `go mod init github.com/rm4n0s/jailcat`, `go 1.27`. Require `github.com/rm4n0s/glutter` v1.9.0, `github.com/rm4n0s/errors` v0.2.0, `gorm.io/gorm`, `gorm.io/driver/sqlite` (the versions english-tutor uses, or newer).
2. Create the packages of CLAUDE.md "Layout", each with a `doc.go` whose package comment states its purpose and the SPEC sections it implements: `cmd/jailcat` (an empty `main`), `internal/xdg`, `internal/apps`, `internal/desktopentry`, `internal/icons`, `internal/firejail`, `internal/store`, `internal/install`, `internal/ui`. Add `assets/` with a `README.md` saying what goes there.
3. Add `docs/IMPLEMENTED.md` (already written) and `.claude/briefs/` to `.gitignore`.
4. Commit CLAUDE.md, `.golangci.yml`, `.gitignore`, `docs/SPEC.md`, this file and `docs/IMPLEMENTED.md` with the step.
**Done when:** the standard checks pass (`go mod tidy` keeps the four dependencies, held by a blank import in `cmd/jailcat` if no code uses them yet).

---

## M1: Reading the system

### 1.1 Folders (`internal/xdg`)
**Goal:** every folder of SPEC §3.1 from a given environment.
**Read:** SPEC §3.1; the XDG Base Directory Specification.
**Tasks:**
1. `type Env` holding the environment as a lookup function and the home folder; `FromOS()` builds it in `main`, tests build it from a map.
2. `type Dirs` with every folder of §3.1 (data, database, profiles, imported, homes, cache, bin, applications, icons, firejail system and user config) plus `DataDirs` (`$XDG_DATA_DIRS`) and `PATH` folders. Relative or empty values fall back to the defaults, as the specification says.
3. `UserDirs`: the user folders of `user-dirs.dirs` (`DOWNLOADS`, `DESKTOP`, …) for firejail's macros (§8.1).
4. `EnsureDirs(ctx)` creates Jailcat's folders with the modes of §3.1.
5. `OnPath(dir)` says whether the bin folder is on `$PATH` (§3.1).
**Tests:** table tests for every variable set, unset, relative and empty; `user-dirs.dirs` with quotes and `$HOME`; `EnsureDirs` modes in `t.TempDir()`.

### 1.2 Desktop entries (`internal/desktopentry`)
**Goal:** read and write `.desktop` files by the Desktop Entry Specification.
**Read:** SPEC §4.1, §9.3; the Desktop Entry Specification (keys, localized keys, value escapes, the `Exec` key's quoting and field codes); english-tutor's `internal/menuentry` (module cache) for the writing side.
**Tasks:**
1. `Parse(r)`: groups in order, keys, localized keys (`Name[de_DE@euro]`), with the specification's locale matching (`Localized(key, locale)`), value escapes, lists (`;`), booleans; errors with line numbers.
2. `SplitExec(value)`: the `Exec` value into arguments by the specification's quoting rules, keeping field codes as tokens.
3. `QuoteExecArg(arg)`, `EscapeValue(s)`, and `Write(w, entry)` that writes groups and keys in a stable order.
4. Desktop file IDs from a path relative to an `applications` folder (`kde/foo.desktop` → `kde-foo.desktop`).
**Tests:** a fixture set in `testdata/` (real-world shapes: VS Code, Firefox with actions, a quoted `Exec`, a broken file); round trips; quoting with spaces, quotes, `$`, backticks, `%`, newlines, non-ASCII.

### 1.3 Finding apps (`internal/apps`)
**Goal:** the app list of SPEC §4.
**Read:** SPEC §4; step 1.2's API.
**Tasks:**
1. `Scan(ctx, dirs)` returns `[]App` from desktop entries (§4.1: folder order, `Hidden`, `NoDisplay`, `TryExec`, the binary from `Exec` with the `env`, firejail-wrapper, Flatpak and Snap cases, actions, Jailcat's own entries skipped) and from `$PATH` (§4.2: first folder wins, firecfg symlinks skipped, binaries of desktop entries not listed twice).
2. `App` has: ID, kind (desktop or binary), names (localized), comment, keywords, icon name, binary path, flags (`UsesFirejail`, `AlreadySandboxed`, `NoDisplay`), the entry's actions.
3. `Search(apps, query, filter)` (§4.3).
**Tests:** a fake system in `testdata/` (two `applications` folders with a shadowed ID, a fake `$PATH` with a firecfg symlink and a duplicate name, a Flatpak entry, a Jailcat entry); search ordering.

### 1.4 Finding icons (`internal/icons`, lookup part)
**Goal:** the icon file of an app (SPEC §10.2, first bullet).
**Read:** SPEC §10.2; the Icon Theme Specification (`index.theme`, `Inherits`, directories, sizes, `hicolor`, `/usr/share/pixmaps`).
**Tasks:**
1. `Lookup(name, theme, dirs)`: absolute paths as is; the theme chain to `hicolor`; scalable and the largest size first, SVG before PNG; then `pixmaps`. Returns the path and the format.
2. The current theme's name: from `gsettings`-free sources only (`$XDG_CONFIG_HOME/gtk-3.0/settings.ini`, `kdeglobals`), else `hicolor`.
**Tests:** a small fake icon theme with inheritance in `testdata/`.

---

## M2: Firejail model

### 2.1 Reading profiles (`internal/firejail`, reader)
**Goal:** read a profile and its include chain the way firejail 0.9.80 does (SPEC §8.1).
**Read:** SPEC §8; `man firejail-profile`; firejail's `src/firejail/profile.c` (`profile_read`, `profile_check_line`, `is_in_ignore_list`, `profile_check_conditional`) and `util.c` (`line_remove_spaces`) at tag 0.9.80.
**Tasks:**
1. `Read(ctx, path, env)` returns the ordered lines that take effect, each with its file and line number, and the lines skipped by `ignore` (with the `ignore` that skipped them).
2. Line normalization, `include` resolution (`${HOME}`, `${CFG}`, absolute, bare names: user folder then `/etc/firejail`; missing files skipped; cycles an error), `ignore` with firejail's exact matching rule, `?CONDITION:` lines (§8.1), the count of `ignore` entries.
3. The list of profile commands of 0.9.80 (from `man firejail-profile`) as data, used to say whether a command exists.
**Tests:** fixture profiles in `testdata/` copied from `/etc/firejail` (`code.profile`, `electron-common.profile`, `default.profile` and what they include; they are GPL-2 text used as test data, with a `testdata/firejail/README.md` saying where they come from); ignore-matching cases (`Code` vs `Code - OSS`, macros not expanded, `ignore include`); a cycle; a missing `.local`.

### 2.2 Settings and base values (`internal/firejail`, model)
**Goal:** the settings of SPEC §7 as Go types, and their base values from a read profile (§8.2).
**Read:** SPEC §7, §8.2.
**Tasks:**
1. `type Settings` with one field per setting of §7.1–§7.8 (home choice, folders, toggles, network, devices, D-Bus, X11, security, programs, limits, launcher), and its defaults for base **None** (§7, "Default" values, and §7.5's Jailcat defaults).
2. `BaseValues(read)`: the settings a read profile sets, and the lines no field covers (the read-only list of §7.9), grouped by file.
3. `type Overrides`: the fields the user changed, with `Apply(base) Settings` and `Diff(base, edited) Overrides`; JSON with a `version` field (§12).
4. Validation of a `Settings` (§7: names, paths, the private-home folder rule of §7.1, numbers, lists).
**Tests:** base values of the fixture profiles (VS Code: sound off, camera off, U2F off, session D-Bus full, network full, …, as SPEC §6.1 lists them); overrides round trip; validation errors with routes.

### 2.3 Writing profiles (`internal/firejail`, writer)
**Goal:** the generated profile of SPEC §9.1.
**Read:** SPEC §9.1, §7.9.
**Tasks:**
1. `WriteProfile(w, build)`: header, `quiet`, settings lines from overrides by the table of §9.1, Extra rules, `include` of the base profile; `${HOME}` for paths under the home.
2. The `ignore` rules of §9.1: exact base text, lines re-written before an `ignore` that would skip too much, the 32-entry limit counted with the base chain.
3. The one profile-line escaping function, with the errors of §9.1.
4. Extra-rules validation (§7.9): unknown command, malformed arguments, `include` outside the allowed folders, rows a field covers (warnings).
**Tests:** golden text profiles in `testdata/` for VS Code with each home choice and a set of overrides, and for base **None**; every escaping error; the ignore limit; Extra-rules diagnostics.

### 2.4 Launcher script (`internal/firejail`, launcher)
**Goal:** the launcher of SPEC §9.2.
**Read:** SPEC §9.2.
**Tasks:**
1. `WriteLauncher(w, build)`: header, `exec` with firejail's absolute path, `--profile=`, `--`, the binary, extra arguments, `"$@"`; the one POSIX single-quote function.
2. `FindFirejail(dirs)` and `Version(ctx, path)` (`firejail --version`, first line).
**Tests:** golden launchers; quoting with `'`, spaces, `$`, backticks, newlines; `sh -n` on every golden launcher; a test that runs a generated launcher with a fake `firejail` script (in `t.TempDir()`) that prints its arguments, to prove they arrive unchanged.

---

## M3: Icons, storage, installing

### 3.1 Building icons (`internal/icons`, writer)
**Goal:** the build icon of SPEC §10.1–§10.3.
**Read:** SPEC §10.
**Tasks:**
1. `assets/badge.svg`: Jailcat's own badge (a shield with jail bars; drawn by hand as SVG, not firejail's logo), and `assets/jailcat.svg`, the app's own icon.
2. `BadgeIcon(src, format)`: SVG sources parsed with `encoding/xml`, sanitized (§10.2), ids prefixed, inlined as a nested `<svg>`; PNG embedded as a `data:` URI; the badge over the corner.
3. `RandomIcon(seed, name)` (§10.3).
4. Fallbacks of §10.2.
**Tests:** golden SVG text; sanitizing (`<script>`, `onload`, external `href`, `<foreignObject>`); id clashes; `rsvg-convert` renders every golden SVG without error (skipped if missing).
**Proof:** method C: `3.1-badge-code.png` (VS Code's fixture icon with the badge) and `3.1-random.png` (three random icons side by side, one `rsvg-convert` each, joined by the test helper into one image).

### 3.2 Database (`internal/store`)
**Goal:** the database of SPEC §12.
**Read:** SPEC §12; english-tutor's `internal/store/store.go` and `migrations.go` (module cache) for the pattern.
**Tasks:**
1. `Open(ctx, path)` (file mode `0600`, busy timeout) and `OpenMemory(ctx)` for tests; numbered migrations with frozen structs and `schema_migrations`.
2. `Build` and `InstalledFile` models; methods to create, get, list, update, delete builds, save and clear drafts, and record installed files; GORM types stay inside the package (it returns `firejail` types and plain structs).
3. Not-found and unique-name errors as their own tags.
**Tests:** every method on a fresh database; migrations applied twice are a no-op; a unique sandbox name clash; route tests for every tag.

### 3.3 Installing (`internal/install`)
**Goal:** writing and removing a build's files (SPEC §9.3, §9.5, §11.3 steps 2–4, §13.2).
**Read:** SPEC §9, §11.3, §13.
**Tasks:**
1. The desktop entry of §9.3 from an `App` and the build (field codes, copied keys, actions, `StartupWMClass`).
2. `Plan(build)`: the targets and their state (missing, ours, conflict) by the rules of §9.5.
3. `Install(ctx, build, plan)`: writes profile, launcher, icon, then desktop entry (temporary file and rename, modes), creates the private home, returns the files with their SHA-256 to record.
4. `Uninstall(ctx, files, opts)` (§13.2).
**Tests:** a whole install into a `t.TempDir()` home and its golden files; each conflict case; uninstall leaves edited files; the private home is kept by default.
**Proof:** method C: the golden desktop entry, profile and launcher of a VS Code build, listed in the entry.

### 3.4 Running firejail (Try and the build check)
**Goal:** SPEC §11.2 and §11.3 step 5.
**Read:** SPEC §11; `man firejail` (`--name`, `--shutdown`, `--list`).
**Tasks:**
1. `Try(ctx, build)`: the temporary profile, the command, the output stream, `Stop`; stops on context cancel.
2. `Check(ctx, launcherCommand)` with `true`.
3. The "existing sandbox" warning detected and returned as its own tag.
**Tests:** unit tests with a fake `firejail` script.
**Firejail tests:** `go test -tags jailcat_firejail ./internal/install ./internal/firejail`, outside Claude Code's sandbox: install a build into a `t.TempDir()` home and run its launcher: a hidden folder is not visible, a read-only folder cannot be written, a folder outside the list is missing with "Only the folders I choose", `net none` leaves only `lo`, `ignore` before `include` undoes the base line, Own private home does not show the real home, `Try` starts and `Stop` ends the sandbox.

---

## M4: The app

### 4.1 App shell and app list
**Goal:** `cmd/jailcat` runs the window of SPEC §5 with the app list.
**Read:** SPEC §5, §4.3, §10.4, §14; Glutter's `examples/counter`, `examples/svg` and its SPEC §13 (UI thread) in the module cache.
**Tasks:**
1. `main`: `xdg.FromOS`, `EnsureDirs`, the store, the logger (`JAILCAT_LOG`), firejail lookup, then `m.RunApp` with app ID `io.github.rm4n0s.jailcat`. There is no dev-home variable: manual runs and proofs point `XDG_DATA_HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME` and `XDG_BIN_HOME` at folders under `.jailcat-test/` (CLAUDE.md "Commands"). The flags are parsed with the standard `flag` package; `-setup-desktop` is task 4.
2. `internal/ui`: the app widget, the split view, the app list with icons (`svg` package, `svg.NewSvgCacheScope`), search and filters, built marks, the empty state, the firejail-missing banner. Scanning off the UI thread.
3. `scripts/headless-wayland.sh` (from Glutter's script of the same name, adapted).
4. `jailcat -setup-desktop` (SPEC §3.2): Jailcat's own menu entry and icon, written atomically under `$XDG_DATA_HOME`, then exit.
**Tests:** widget tests with a fake app list and goldens.
**Proof:** method A `4.1-app-list.png`; method B `4.1-app.png` (the app on a scratch home with fixture apps).

### 4.2 Build editor shell and profile selector
**Goal:** SPEC §6 without the tabs' contents.
**Read:** SPEC §6, §6.1, §8.
**Tasks:** the header, the profile selector (§6.1 groups, search, Import, Keep/Start over), the tab row with change dots, the button bar, Reset to profile, drafts saved on leaving (§5).
**Tests:** widget tests: default profile for an app with and without its own profile; switching profile with overrides asks; drafts.
**Proof:** method A `4.2-editor.png`, `4.2-profile-selector.png`.

### 4.3 Files tab
**Goal:** SPEC §7.1.
**Tasks:** the home choice at the top, the folder list (chooser with `desktop.OpenFiles` `Directory: true`, typed paths, access levels, "from profile" rows, the private-home rule), the quick toggles.
**Tests:** widget tests with `Tester.FakeDesktop()` answering the chooser; the real-home refusal in private modes.
**Proof:** method A `4.3-files-tab.png`.

### 4.4 Network, Devices and Desktop tabs
**Goal:** SPEC §7.2–§7.4.
**Proof:** method A `4.4-network.png`, `4.4-devices.png`, `4.4-desktop.png`.

### 4.5 Security, Programs and Limits tabs
**Goal:** SPEC §7.5–§7.7.
**Proof:** method A `4.5-security.png`, `4.5-programs.png`, `4.5-limits.png`.

### 4.6 Launcher and Extra rules tabs
**Goal:** SPEC §7.8, §7.9.
**Tasks:** the launcher fields with the icon preview (`svg.NewSvgPictureMemory`), the bin-folder-not-on-`$PATH` note; the Extra rules editor with diagnostics per row; the read-only base lines grouped by file.
**Proof:** method A `4.6-launcher.png`, `4.6-extra-rules.png`.

### 4.7 Preview, Try, Build and Uninstall
**Goal:** SPEC §11, §13.
**Tasks:** the Preview dialog; the Try panel with output and Stop; Build with the conflicts dialog, the result, and the check warning; Uninstall with its options; the build chooser for apps with several builds; "firejail updated" and "base profile missing" marks (§13.3).
**Tests:** widget tests with fake install and firejail runners.
**Firejail tests:** an end-to-end test that builds VS Code's fixture app into a scratch home through the UI's controller and runs it.
**Proof:** method A `4.7-preview.png`, `4.7-conflict.png`; method B `4.7-built.png` (the app after a build, on a scratch home).

---

## M5: 1.0

### 5.1 Release 1.0
**Goal:** Jailcat checked on the real desktop and released.
**Tasks:**
1. `README.md`: what Jailcat does, install (`go install github.com/rm4n0s/jailcat/cmd/jailcat@latest`, needs gcc and firejail), a screenshot (`docs/implemented-proof-images/4.7-built.png`), how a build works (the four files).
2. Manual checks with the user (method D): build VS Code with Own private home; `5.1-menu.png` (the menu shows "Visual Studio Code (sandboxed)" with the badge icon); `5.1-running.png` (the sandboxed VS Code running, with the taskbar); `firejail-code` from bash; uninstall removes the menu entry.
3. Tag `v1.0.0` with the milestone.
**Proof:** method D `5.1-menu.png`, `5.1-running.png`.
