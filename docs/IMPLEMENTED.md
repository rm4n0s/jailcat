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
