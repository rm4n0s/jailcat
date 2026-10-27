# Jailcat specification

This is the source of truth for what Jailcat does (see CLAUDE.md). Sections are cited as §N. A decision that changes is changed here, on the same branch as the code.

## §1 Goal

Jailcat is a desktop GUI that makes firejail sandboxes for the applications on a Linux system. The user picks an application or a binary, chooses what its sandbox allows, and presses **Build**. Jailcat installs a sandboxed copy of the app under a new name, `firejail-<app>`, that starts from the application menu and from bash. Every build is kept in a database, so it can be opened, changed, rebuilt and uninstalled later.

A sandbox that Jailcat built needs only firejail to run: it keeps working when Jailcat is closed or removed.

### §1.1 Out of scope for 1.0
- Running as root, `sudo`, or any change outside the user's own folders: `/etc/firejail`, `firecfg`, system menus
- Flatpak, Snap and AppImage apps (Flatpak and Snap apps are listed but marked "already sandboxed"; §4.1)
- Network namespaces that need root setup (`net <interface>`, `veth`, bridges)

## §2 Terms

| Term | Meaning |
|---|---|
| App | Something the user can sandbox: a desktop entry or an executable on `$PATH` (§4) |
| Build | One sandbox of one app: its settings, its name and the files it installed. An app can have several builds (`firejail-code`, `firejail-code-work`) |
| Sandbox name | The build's name, `firejail-<app>` by default. Only `[a-z0-9-]`, unique among builds |
| Base profile | The firejail profile a build starts from: a profile file, or **none** (§6.1) |
| Base value | What the base profile, with everything it includes, sets for a setting (§8) |
| Override | A setting the user changed from its base value. A build stores only its overrides (§12) |
| Build files | What a build installs: profile, launcher, desktop entry, icon, and the private home folder if it has one (§9) |

## §3 Platform, dependencies and folders

- Linux only. Go 1.27. UI: Glutter (`github.com/rm4n0s/glutter`, v1.9.0 or newer: the first version with the `svg` package). Storage: GORM (`gorm.io/gorm`) with `gorm.io/driver/sqlite` (needs cgo). Errors: `github.com/rm4n0s/errors`. The rules for dependencies are in CLAUDE.md
- Firejail 0.9.80 is the version Jailcat is developed against. Jailcat reads `firejail --version` at start; without firejail it still opens, shows a banner, and disables Try and Build
- Glutter app ID: `io.github.rm4n0s.jailcat`

### §3.1 Folders
All folders come from `internal/xdg`, resolved once at start from the environment:

| Name | Value |
|---|---|
| Data | `$XDG_DATA_HOME/jailcat` (default `~/.local/share/jailcat`) |
| Database | `<data>/jailcat.db` |
| Profiles | `<data>/profiles/` (generated), `<data>/profiles/imported/` (imported, §6.1) |
| Private homes | `<data>/homes/<sandbox name>/` |
| Cache | `$XDG_CACHE_HOME/jailcat` (default `~/.cache/jailcat`); Try's temporary profiles (§11.2) |
| Bin | `$XDG_BIN_HOME` if set, else `~/.local/bin` |
| Applications | `$XDG_DATA_HOME/applications` |
| Icons | `$XDG_DATA_HOME/icons/hicolor/scalable/apps` |
| Firejail system config | `/etc/firejail` (firejail's `${CFG}`) |
| Firejail user config | `$HOME/.config/firejail`, always: firejail ignores `$XDG_CONFIG_HOME` (`profile.c`) |

Jailcat creates its folders with mode `0700` (data, homes) or `0755` (the rest) when they are missing. If the bin folder is not on `$PATH`, the Launcher tab says so.

### §3.2 Jailcat in the application menu (`-setup-desktop`)
`jailcat -setup-desktop` adds Jailcat itself to the desktop's application menu and exits, without opening the window, the log or the database (the owner's request, 2026-10-27, like english-tutor's flag of the same name). It writes, under `$XDG_DATA_HOME` (`~/.local/share` when unset or not absolute):
- `icons/hicolor/scalable/apps/io.github.rm4n0s.jailcat.svg`: Jailcat's own icon (`assets/jailcat.svg`)
- `applications/io.github.rm4n0s.jailcat.desktop`: a desktop entry named after the app ID, so the window's Wayland `app_id` and X11 `WM_CLASS` match it (`StartupWMClass=io.github.rm4n0s.jailcat`). `Name=Jailcat`, `Comment=Build firejail sandboxes for your applications`, `Exec` is the absolute path of the running program with symbolic links resolved (quoted and escaped as the Desktop Entry Specification requires), `Icon=io.github.rm4n0s.jailcat`, `Terminal=false`, `Categories=System;Security;Utility;`

Both files are written through a temporary file renamed over the target, the icon first, so running the flag again (after moving or rebuilding the program) replaces them. They carry no `X-Jailcat-Build` key: they are not a build and are not recorded in the database. The program prints where it wrote the files, or, on failure, the reason, and exits with 1. The entry points at the program that ran the flag, so it should be run from the installed binary (`go install`), not from `go run`.

## §4 Finding apps

### §4.1 Desktop entries
- Read every `*.desktop` file under `$XDG_DATA_HOME/applications` and each `<dir>/applications` of `$XDG_DATA_DIRS` (default `/usr/local/share:/usr/share`), subfolders included. The desktop file ID follows the Desktop Entry Specification (`kde-foo.desktop` for `kde/foo.desktop`); the first folder in that order wins for an ID
- Keep `Type=Application` entries. `Hidden=true` removes the entry. `NoDisplay=true` entries are listed only when "Show hidden apps" is on. A `TryExec` that is not found removes the entry
- Name, GenericName, Comment and Keywords are read in the current locale with the specification's fallback
- The app's binary is the first word of `Exec`, unquoted by the specification's rules, then:
  - `env VAR=… prog …` → `prog`
  - a firejail wrapper (`firejail [options] prog …`, as `firecfg` and other tools write) → `prog`, and the app is marked "already uses firejail"
  - `flatpak run …` or a `/snap/bin/…` binary → marked "already sandboxed": shown, but cannot be built
  - a bare name → looked up on `$PATH` (§4.2), skipping firecfg's symlinks to firejail
- Desktop actions (`[Desktop Action …]`) are read for §9.3
- Jailcat's own entries (with `X-Jailcat-Build`) are not listed as apps; they appear as builds

### §4.2 Binaries on `$PATH`
- Every regular file or symlink with an execute bit, in each folder of `$PATH`, first folder wins for a name (as the shell does). A symlink to `firejail` (made by `firecfg`) is skipped, and the name resolves to the next folder that has it
- A binary that a desktop entry already runs is not listed twice: it belongs to that app
- Binaries are listed with a terminal icon and their name

### §4.3 Search
One search box matches name, generic name, keywords, binary name and binary path, case-insensitive, by substring. Results put matches at the start of the name first. With no search text, **All** lists the menu apps before the binaries. Filters: **All**, **Menu apps**, **Binaries**, **Built**.

Discovery runs off the UI thread at start and again on "Refresh". It does not watch folders.

## §5 Main window

```
┌────────────────────────┬───────────────────────────────────────────────────┐
│ [Search apps…        ] │ (build editor, §6, or an empty state)             │
│ All Menu Binaries Built│                                                   │
│ ────────────────────── │                                                   │
│ [ic] Visual Studio Code│                                                   │
│      code      ● built │                                                   │
│ [ic] Firefox           │                                                   │
│ [>_] curl              │                                                   │
│ …                      │                                                   │
└────────────────────────┴───────────────────────────────────────────────────┘
```

- Left: the app list with search and filters. A built app shows "built" and the number of builds
- Selecting an app with no build opens a new build in the editor. Selecting a built app opens its build, or a chooser if it has several, with "New build"
- Unsaved changes in the editor are kept in the database as a draft when another app is selected or the window closes (§12); a draft is not installed until Build. A new build with changes is stored (never built, not installed) so its draft has a row; a build that was never built shows a "draft" chip in the list instead of "built", and only built builds pass the **Built** filter

## §6 Build editor

```
┌──────────────────────────────────────────────────────────────────────┐
│ [icon] Visual Studio Code → firejail-code                            │
│ Profile: [ code (firejail)                                    ▾ ]    │
├──────────────────────────────────────────────────────────────────────┤
│ Files │ Network │ Devices │ Desktop │ Security │ Programs │ Limits │ Launcher │ Extra rules │
├──────────────────────────────────────────────────────────────────────┤
│ (the selected tab's form, §7)                                        │
├──────────────────────────────────────────────────────────────────────┤
│                  [Uninstall]          [Preview]  [Try]  [Build]      │
└──────────────────────────────────────────────────────────────────────┘
```

- One screen: header, profile selector, a row of tabs (one per category, §7), the tab's form, and the buttons, which stay visible on every tab. No setting is collapsed or hidden
- A tab with at least one override shows a dot on its name. Each overridden field shows a small "changed" mark with **Reset to profile**, which removes the override
- The header shows the build's icon (§10.4), the app's name and the sandbox name
- Uninstall is shown only for an installed build (§13.2)

### §6.1 Profile selector
- A searchable list in groups:
  1. **For this app**: the profile firejail itself would pick for the binary: `<binary name>.profile` in the firejail user config folder, then in `/etc/firejail`
  2. **None**: no base profile; the sandbox is only what the form says
  3. **Default**: `default.profile` (user config folder first, then `/etc/firejail`)
  4. **Your profiles**: `*.profile` in the firejail user config folder
  5. **Imported**: `*.profile` in Jailcat's `profiles/imported/`
  6. **Firejail profiles**: every `*.profile` in `/etc/firejail`
- A new build starts on group 1 if it exists, else on **Default**, which is what firejail does with no `--profile`
- **Import profile…** picks a `.profile` file with the file chooser and copies it into `profiles/imported/` (a name that exists gets a `-2`, `-3`… suffix). Its includes are not copied; they resolve the way firejail resolves them (§8.1)
- The build stores the selected profile's path, not its content (§13.3)
- Selecting another profile recomputes every base value (§8). If the build has overrides, Jailcat asks: **Keep my changes** (overrides stay; ones that now equal the base value are dropped), **Start over** (all overrides are removed except the sandbox name and the icon seed; Extra rules stay) or **Cancel**

## §7 Settings

Every field shows its base value until the user changes it. "Default" below is the value of a field when the base profile is **None**. The firejail lines each setting writes are in §9.1; the names here are the profile commands of `man firejail-profile`.

### §7.1 Files tab
**Home folder** (at the top; the most important setting of a build). One of:

| Choice | What the app sees as its home |
|---|---|
| Full home | The real home, minus what the base profile hides |
| Only the folders I choose | Only the folders of the folder list (and the app's own folders, below); the rest of the home is an empty temporary folder |
| Empty home | An empty home that is thrown away when the sandbox closes (`private`) |
| Own private home | `<data>/homes/<sandbox name>/`, kept between runs (`private <dir>`). Jailcat creates it at Build |

Default with **None**: Full home. With a base profile: what it sets (a `private` line → Empty or Own private home; `whitelist` lines under `${HOME}` → Only the folders I choose; otherwise Full home).

**Folder list**: rows of a path (from the folder chooser, `desktop.OpenFiles` with `Directory: true`, or typed) and an access level: **Read-write**, **Read-only** or **Hidden**. Rows that come from the base profile are shown with a "from profile" mark; changing or removing one is an override. They are the `whitelist`, `read-only`, `read-write`, `blacklist` and `noblacklist` lines (after `ignore`) of the base chain's `.profile` files only: the shared `.inc` files (`disable-common.inc` alone has hundreds of `blacklist` lines) go to the read-only list of §7.9, so the list shows what is particular to this app. Lines for system folders (`/dev`, `/proc`, `/sys`, `/run`, `/tmp`, `/etc`, `/usr`, `/var`, `/boot`, `/bin`, `/sbin`, `/lib*`, `/root`), globs and the home itself are not rows either; they go to the read-only list.
- With **Only the folders I choose**, the folders the base profile allows under the home (its `noblacklist`/`whitelist` lines, e.g. `~/.config/Code` and `~/.vscode` for VS Code) are listed as Read-write rows from the profile, so the app keeps its own settings
- With **Own private home** and **Empty home**, the real home stays out of the sandbox completely (the owner's decision, 2026-10-27): the folder list accepts only folders outside the home (`/mnt/data`, `/opt/projects`), and the chooser's answer inside the real home is refused with a message that says so. Rows from the base profile stay: inside a private home `${HOME}` is the private home, so `${HOME}/.config/Code` names the app's own folder there, not the real one

**Quick toggles**: Downloads read-write (`${DOWNLOADS}`), Hide removable drives (`disable-mnt`), No programs from home (`noexec ${HOME}`), Private `/tmp` (`private-tmp`), Private `/dev` (`private-dev`), Private cache (`private-cache`).

### §7.2 Network tab
- **Network**: Full / None (`net none`). Default: Full
- **DNS servers**: up to four addresses (`dns`; firejail ignores a fifth, `profile.c`). Default: system
- **Hosts file**: a file to use as `/etc/hosts` (`hosts-file`). Default: system
- **Firewall**: firejail's default client filter (`netfilter`). Default: off
- DNS, hosts file and firewall are disabled while Network is None

### §7.3 Devices tab
Checkboxes, checked = allowed. Default with **None**: all allowed.

| Checkbox | Blocked by |
|---|---|
| Sound and microphone (firejail cannot separate them) | `nosound` |
| Camera | `novideo` |
| GPU / 3D acceleration | `no3d` |
| Printers | `noprinters` |
| U2F security keys | `nou2f` |
| Input devices (game controllers, raw input) | `noinput` |
| DVD drives | `nodvd` |
| TV and radio tuners | `notv` |
| TPM | `notpm` |

### §7.4 Desktop tab
- **Session D-Bus**: Full / Filtered / None (`dbus-user filter`, `dbus-user none`). When Filtered, checkboxes:

  | Checkbox | Line |
  |---|---|
  | Notifications | `dbus-user.talk org.freedesktop.Notifications` |
  | File chooser and other portals | `dbus-user.talk org.freedesktop.portal.*` |
  | Keyring (Secret Service) | `dbus-user.talk org.freedesktop.secrets` |
  | Tray icon | `dbus-user.talk org.kde.StatusNotifierWatcher` |
  | Media keys (MPRIS) | `dbus-user.own org.mpris.MediaPlayer2.<binary name>.*` |

  plus a list of further names to talk to or own. Default: Full
- **System D-Bus**: Full / None (`dbus-system none`). Default: Full
- **X11 isolation**: Off / Xpra / Xephyr / Xvfb (`x11 xpra` …). Only the ones whose program is installed can be picked. Default: Off

### §7.5 Security tab
Default with **None** in parentheses.
- Seccomp filter (`seccomp`) (on)
- Drop all capabilities (`caps.drop all`) (on)
- No new privileges (`nonewprivs`) (on)
- No root user in the sandbox (`noroot`) (on)
- Restrict namespaces (`restrict-namespaces`) (on)
- Socket types: checkboxes unix, inet, inet6, netlink, packet, bluetooth (`protocol …`) (unix, inet, inet6)
- Block writable-and-executable memory (`memory-deny-write-execute`) (off). The field notes that it breaks JIT engines: Electron, browsers, Java, VS Code
- Landlock (`landlock.enforce` with firejail's `landlock-common.inc`) (off)

These defaults are Jailcat's: with **None** they are written as lines, because a sandbox with no profile would otherwise have none of them.

### §7.6 Programs tab
- **Only these programs** (`private-bin`): off, or a list of program names
- **Only these `/etc` files** (`private-etc`): off, or a list of names relative to `/etc` (no `/`, as firejail requires)
- **Environment variables** (`env NAME=value`): a list of name/value rows
- **Extra arguments**: words added to the app's command line before the user's own arguments (§9.2)

### §7.7 Limits tab
All off by default.
- Memory (`rlimit-as`), entered with a unit (MB, GB), written in bytes (firejail also reads `K`/`M`/`G` suffixes in a base profile)
- CPU cores (`cpu`): checkboxes, one per core (firejail accepts cores 0–31)
- Priority (`nice`): −20 to 19; firejail turns a negative value into 0 for a user who is not root, and the field says so
- Time limit (`timeout hh:mm:ss`)
- Maximum open files (`rlimit-nofile`), maximum processes (`rlimit-nproc`)

### §7.8 Launcher tab
- **Sandbox name**: default `firejail-<binary name>`, sanitized to `[a-z0-9-]`; for a second build of the same app, `-2`, `-3`…
- **Menu name**: default "<app name> (sandboxed)"; **Comment**: default the app's comment
- **Icon**: App icon with badge / Random (with **New random icon**) (§10)
- **Install in the menu** (on), **Install in the bin folder** (on); at least one must be on
- **Open file types** (`MimeType`): off by default, so the sandboxed copy does not take over the app's file types
- **Single instance** (`join-or-start <sandbox name>`): off
- **Run in a terminal** (`Terminal=true`): on for binaries without a desktop entry, else the entry's value
- **Hide firejail's messages** (`quiet`): on

### §7.9 Extra rules tab
- **Extra rules**: a text box, one firejail profile line per row, written after the settings (§9.1). Each row is checked as the user types:
  - the command exists in firejail's command list (`internal/firejail` keeps the list of 0.9.80)
  - its arguments are well formed (paths, lists, numbers)
  - `include` names only a file in `/etc/firejail`, the firejail user config folder or Jailcat's profiles folder
  - a row that a field covers (e.g. `net none`, `nosound`) is a warning that names the field: the field is the one place for that setting
  - rows with errors block Build and Try
- **From the base profile**: read-only, the lines of the base profile and its includes that no field covers (e.g. the `blacklist` lines of `disable-common.inc`), grouped by file, with the file's path. They change only through firejail updates, the user's own `.local` files, or `ignore`/`noblacklist` rows in Extra rules

## §8 Reading a profile

`internal/firejail` reads a base profile the way firejail does, to compute base values and the read-only list of §7.9.

### §8.1 Resolution
- `include <name>`: `${HOME}` and `${CFG}` are expanded (`$HOME` without braces is an error); a path with a `/` is used as is; a bare name is looked up in the firejail user config folder, then in `/etc/firejail`. Only a missing file whose name ends in `.local` is skipped; any other missing or unreadable include stops firejail, so it is an error for Jailcat too. Firejail has no cycle check, only a depth limit of 16 includes (`MAX_INCLUDE_LEVEL`); deeper is an error. After an `allow-debuggers` line, an include of `disable-devel.inc` is skipped (`profile_read`)
- Lines firejail makes before any `ignore` or conditional is applied, so no `ignore` can remove them: `whitelist-ro X` becomes `whitelist X` and `read-only X` (`profile_read`), and `noprinters` adds `blacklist /dev/lp*` and `blacklist /run/cups/cups.sock` (`profile_check_line`)
- The user's `.local` files (`code.local`, `globals.local`) are read like any include, so their lines count in the base values
- Macros in paths (`${HOME}`, `${DOWNLOADS}`, `${DESKTOP}`, `${DOCUMENTS}`, `${MUSIC}`, `${PICTURES}`, `${VIDEOS}`, `${PATH}`, `${RUNUSER}`) are kept as written in profile text and expanded only to show paths in the form. `${DOWNLOADS}` and the other user folders resolve as firejail's `macros.c` does: the `XDG_<NAME>_DIR="$HOME/<dir>"` line of `$HOME/.config/user-dirs.dirs` (only values relative to `$HOME`, and only when the folder exists), else the first of firejail's fixed translations that exists in the home (`Downloads`, `Загрузки`, `Téléchargement`, …), else unresolved
- Each line is first normalized as firejail does (`profile.c`, `line_remove_spaces` in `util.c`): everything from the first `#` is a comment, leading blanks are dropped, and every run of spaces and tabs becomes one space
- `ignore <text>` adds `<text>` to the ignore list; every line read after it that starts with `<text>` followed by the end of the line or a space is skipped (`is_in_ignore_list`). The match is on the text as written, before macros are expanded: `ignore whitelist ${HOME}/x` does not match `whitelist /home/me/x`. So `ignore nosound` skips `nosound`, `ignore whitelist` skips every `whitelist` line, and `ignore whitelist ${HOME}/.config/Code` also skips `whitelist ${HOME}/.config/Code - OSS`. An `include` line can be ignored too (`ignore include disable-xdg.inc`)
- Firejail accepts at most 32 `ignore` entries in one sandbox, counting the ones in every included file (`MAX_PROFILE_IGNORE`); one more stops firejail with an error
- `?CONDITION: line` (`profile_check_conditional`): `HAS_NET` is true once a `net` line took effect; `HAS_NODBUS` once `nodbus`, or a `dbus-user`/`dbus-system` line with `filter` or `none`; `HAS_NOSOUND` once `nosound`; `HAS_PRIVATE` once `private` (any form) or `private-home`; `HAS_X11` once `x11 none` or `x11 xorg`; `HAS_APPIMAGE` is false; `BROWSER_DISABLE_U2F` (default yes), `BROWSER_ALLOW_DRM` and `ALLOW_TRAY` (default no) come from `/etc/firejail/firejail.config`. A false conditional line goes to the read-only list as "conditional"; an unknown condition, a missing colon, or `quiet`/`include` after a true condition is an error, as firejail exits on each

### §8.2 Base values
- A field's base value comes from the lines that remain after `ignore`; a field with no line gets the firejail default (e.g. sound allowed, network full)
- Lines no field covers go to the read-only list of §7.9

## §9 Build files

A build writes four files and, for **Own private home**, one folder. Every generated file starts with the header below and is recorded in the database with its SHA-256 (§12).

```
# Generated by Jailcat. Do not edit: change it in Jailcat and rebuild.
# jailcat-build: <build id>
```

### §9.1 Profile: `<data>/profiles/<sandbox name>.profile`
```
# Generated by Jailcat. Do not edit: change it in Jailcat and rebuild.
# jailcat-build: 7
quiet

# Settings
net none
noblacklist ${HOME}/Projects
read-only ${HOME}/Projects

# Undo base profile lines
ignore nosound

# Extra rules
dbus-user.talk org.freedesktop.Notifications

# Base profile
include /etc/firejail/code.profile
```
- `quiet`, when on, is the first command (firejail requires it). When it is off and the base chain has a `quiet`, `ignore quiet` undoes it
- All Jailcat lines come before the `include`, because `ignore` acts only on lines read after it. With base **None** there is no `include`
- The order is: header, `quiet`, `# Settings` (the lines of the settings, then the base lines written again, below), `# Undo base profile lines` (every `ignore`), `# Extra rules`, `# Base profile` (the `include`). The settings come before the ignores so that no `ignore` of the profile can skip a line Jailcat wrote; a section with no line is left out. An Extra rule that one of the ignores would skip is also written in the Settings part, so it keeps its effect
- What the settings are compared with is what firejail would do with only the `include`: the base values of the base profile, and, with **None**, firejail's own defaults. So with **None** Jailcat's security defaults (§7.5) are written as lines
- The profile is in Jailcat's folder, not in the firejail user config folder: a `code.profile` there would change every `firejail code`, and `firecfg`'s symlinks
- How each override is written:

  | Change | Lines |
  |---|---|
  | A restriction turned on that the base lacks | the command (`nosound`, `net none`, `seccomp`) |
  | A restriction the base sets turned off | `ignore <the base line>` (`ignore nosound`, `ignore net none`) |
  | A value the base sets changed | `ignore <the base line>`, then the new line (`ignore dbus-user none`, `dbus-user filter`, then the `dbus-user.talk` lines) |
  | A list the base sets changed (`protocol`, `private-bin`, `private-etc`, `dns`, `cpu`, `env`, the D-Bus names) | firejail merges the lines of a list, so `ignore <line>` for every base line of the field, then the whole new list. The same for a value where firejail keeps the last line (`nice`, `timeout`, `rlimit-*`, `hosts-file`, `x11`) |
  | Folder Read-write | `noblacklist <path>`, and `whitelist <path>` with Only the folders I choose |
  | Folder Read-only | as Read-write, plus `read-only <path>` |
  | Folder Hidden | `blacklist <path>` |
  | A base folder row removed | `ignore <the base line>`. Rows made from `whitelist-ro` cannot be removed or loosened this way (§8.1): the editor shows them locked, with the reason |
  | Home: Only the folders I choose | the `whitelist` lines of the folder rows and of Downloads (a whitelist under `${HOME}` hides the rest of the home). With no folder allowed under the home the build is refused: firejail would not restrict the home at all |
  | Home: Empty home | `private`. Downloads read-write is refused with an empty or private home: the Downloads folder is in the real home |
  | Home: Own private home | `private ${HOME}/.local/share/jailcat/homes/<sandbox name>` (the real path, written with `${HOME}` when under the home), `ignore mkdir` and `ignore mkfile`: with a private home the `mkdir` and `mkfile` lines of the base profile would still make folders and files in the real home (`man firejail-profile`, under `private directory`) |
  | Home: Full home, with a base that restricts the home | `ignore` for each `private` and home `whitelist` line of the base |

- How `ignore` lines are written (the matching rule is in §8.1):
  - an `ignore` repeats the base line's text exactly as firejail normalizes it, macros as written in the base profile
  - Jailcat applies the same rule to the base chain to find every line an `ignore` it writes would skip. When it would also skip a line the user did not mean to remove (the `Code - OSS` case above), that line is written again *before* the `ignore`, where the `ignore` does not reach it
  - the build counts the `ignore` lines of its profile plus those of the whole base chain; more than 32 blocks Build and Try, and the editor says how many there are and which overrides add them
- Paths under the home are written with `${HOME}`. Values are escaped by the one profile-line function (CLAUDE.md). Because of the normalization in §8.1, these are errors, not escaped: a newline, NUL or `#` in a value; a value that starts or ends with a blank, or holds two blanks in a row or a tab; a comma in a list item (`private-bin`, `private-etc`, `protocol`)

### §9.2 Launcher: `<bin>/<sandbox name>`, mode `0755`
```sh
#!/bin/sh
# Generated by Jailcat. Do not edit: change it in Jailcat and rebuild.
# jailcat-build: 7
exec '/usr/bin/firejail' '--profile=/home/rmanos/.local/share/jailcat/profiles/firejail-code.profile' -- '/usr/bin/code' "$@"
```
- Absolute paths of firejail and of the app's binary, resolved at Build. Every word is in POSIX single quotes
- `--` ends firejail's options, so the app's arguments are never read as firejail options
- Extra arguments (§7.6) go between the binary and `"$@"`

### §9.3 Desktop entry: `<applications>/<sandbox name>.desktop`
```ini
[Desktop Entry]
# Generated by Jailcat. Do not edit: change it in Jailcat and rebuild.
# jailcat-build: 7
Type=Application
Name=Visual Studio Code (sandboxed)
Comment=Code Editing. Redefined.
Exec=/home/rmanos/.local/bin/firejail-code %F
Icon=firejail-code
Categories=Development;IDE;
Keywords=vscode;
Terminal=false
StartupNotify=true
X-Jailcat-Build=7
```
- Written only when "Install in the menu" is on. `Exec` runs the launcher, with the field codes of the app's own `Exec` (`%f`, `%F`, `%u`, `%U`). Without "Install in the bin folder", the launcher is still written, inside `<data>/launchers/`, and `Exec` points there
- Copied from the app's entry: Comment (unless changed), Categories, Keywords, GenericName, StartupNotify, Terminal (unless changed), MimeType (only with "Open file types")
- A desktop action whose `Exec` runs the same binary is copied with its `Exec` rewritten to the launcher plus its arguments (VS Code's "New Empty Window" → `Exec=<launcher> --new-window %F`); other actions are dropped
- Binaries without an entry get `Categories=Utility;`
- `StartupWMClass` is the class the app's window really reports, so the desktop ties the window to this entry, the way english-tutor's `-setup-desktop` does (`internal/menuentry`: one ID for the file name, `Icon` and `StartupWMClass`). Firejail does not change a window's class: it is the original entry's `StartupWMClass` when it has one, else the original desktop file ID without `.desktop`, else the binary's name. The original entry can claim the same window, so with both installed the desktop may group the window under either one
- Values and `Exec` arguments are escaped by the Desktop Entry Specification's rules (CLAUDE.md)

### §9.4 Icon: `<icons>/<sandbox name>.svg`, see §10.

### §9.5 Ownership, conflicts and writing
- Before writing, each target path is checked: missing → write; recorded for this build with the same SHA-256 → overwrite; anything else (made by someone else, or edited since Jailcat wrote it) → a conflict. Conflicts are listed in a dialog with **Overwrite** (for files that carry Jailcat's header only) or **Choose another sandbox name**; Build writes nothing until all are resolved
- Each file is written to a temporary file in the same folder and renamed over the target; the database is updated after all files are written
- The icon is written before the desktop entry, so the entry shows its icon as soon as it appears. Jailcat does not run `update-desktop-database` or `gtk-update-icon-cache`: the menu picks up new and changed files in `~/.local/share` on its own (checked by the owner, 2026-10-27)

## §10 Icons

### §10.1 Format and place
A build's icon is always an SVG, at `<icons>/<sandbox name>.svg`, named in the desktop entry by `Icon=<sandbox name>`. Jailcat writes it as text and never rasterizes. It must render in librsvg (GTK, GNOME) and Qt SVG (KDE): only SVG 1.1 features that both support.

### §10.2 App icon with badge (default)
- The app's icon is found by the Icon Theme Specification: an absolute `Icon=` path as is; a name in the current theme, its parents, `hicolor`, then `/usr/share/pixmaps`; the largest size, SVG first
- An SVG source is parsed with `encoding/xml` and inlined as a nested `<svg>` with its `viewBox`. Its `id`s, and every reference to them, get a unique prefix. elements outside SVG 1.1 drawing (an allowlist: `<script>`, `<foreignObject>`, animation, metadata and foreign namespaces go), `on*` attributes and references outside the file (other than `data:` image URIs and `#` ids) are removed. A `<style>` is kept only in KDE's colour-scheme form, rules `.<class> { <colour or opacity property>: <plain value>; }`, which is applied to the matching elements as attributes (class rules beat presentation attributes and lose to `style=""`); any other `<style>` is dropped whole. A document with an internal DTD subset, over 4 MiB, nested deeper than 128, or a PNG larger than 4096 px is refused and falls back to a random icon
- A PNG source is embedded as a base64 `data:image/png` URI in an `<image>`
- The badge from `assets/badge.svg` sits over the bottom-right corner, at 40% of the icon's width
- Other formats (XPM), a missing icon or a source that cannot be parsed fall back to a random icon, and the Launcher tab says why

### §10.3 Random icon
Made from a 64-bit seed stored in the build, so a rebuild gives the same icon; **New random icon** draws a new seed. A random icon is a background shape (circle, rounded square or hexagon) with a two-colour gradient from a fixed palette, the first letter of the menu name, and the badge.

### §10.4 Icons inside Jailcat
Jailcat's window draws icons with Glutter's `svg` package (v1.9.0): `svg.NewSvgPictureFile` for an SVG icon file (an app's icon, a build's installed icon) and `svg.NewSvgPictureMemory` for an icon not yet written (the editor's header while the build is being edited). PNG icons are drawn with Glutter's image widgets. An icon that cannot be found or drawn shows a generic app icon (or a terminal icon for a binary); the error goes to the log. One `svg.Cache` (`svg.NewSvgCacheScope` at the root of the app) keeps the app list's icons from being parsed again while scrolling.

## §11 Preview, Try and Build

### §11.1 Preview
A dialog with read-only tabs: Profile, Launcher, Desktop entry. Lines that come from overrides and Extra rules are highlighted.

### §11.2 Try
- Writes the profile to `<cache>/try/<sandbox name>.profile` and runs `firejail --profile=<that file> --name=jailcat-try-<sandbox name> -- <binary> <extra arguments>` off the UI thread. Nothing is installed and the database is not changed (a private home folder is created if the build needs one)
- A panel shows firejail's output as it comes, and a **Stop** button (`firejail --shutdown=jailcat-try-<sandbox name>`). Closing Jailcat stops a running Try

### §11.3 Build
1. Validate the settings and Extra rules; resolve firejail's and the binary's paths
2. Check the targets (§9.5)
3. Write the profile, the launcher, the icon, the desktop entry; create the private home folder if needed
4. Record the files, the firejail version and the time in the database
5. Check: run the installed launcher's command with `true` in place of the app (`firejail --profile=… -- true`) and show a warning with firejail's output if it fails. There is no `--quiet` on this command line: with it firejail does not print its warning "an existing sandbox was detected", which the check must see (the profile's own `quiet` line already silences firejail's banner). For a build with "Only these programs" (`private-bin`), where `true` might not be in the sandbox, the command is `firejail --ignore=private-bin --profile=… -- true`. That command-line ignore counts toward firejail's limit of 32 `ignore` entries (§9.1): if the build already has 32, the check is skipped and the reason is shown in place of the result

## §12 Database

One SQLite file (§3.1), opened in `cmd/jailcat`, used only through `internal/store`. Models:

**Build**
| Field | Type | Notes |
|---|---|---|
| ID | uint | primary key |
| SandboxName | string | unique |
| AppKind | string | `desktop` or `binary` |
| DesktopFileID | string | for `desktop` apps |
| BinaryPath | string | as resolved at the last Build |
| BaseProfile | string | absolute path, or empty for **None** |
| Overrides | JSON | the overridden fields only, with a schema version |
| ExtraRules | text | |
| IconMode | string | `badge` or `random` |
| IconSeed | int64 | |
| Draft | JSON | unsaved overrides (§5); empty when none |
| FirejailVersion | string | at the last Build |
| CreatedAt, UpdatedAt, BuiltAt | time | `BuiltAt` is null until the first Build |

**InstalledFile**
| Field | Type | Notes |
|---|---|---|
| ID | uint | |
| BuildID | uint | foreign key |
| Kind | string | `profile`, `launcher`, `desktop`, `icon`, `home` |
| Path | string | unique |
| SHA256 | string | empty for `home` |

- Overrides are stored as JSON, not as tables: the set of fields grows with firejail, and the store never queries inside it. The JSON has a `version` field; reading an older version upgrades it in `internal/store`
- The schema is made only by numbered migrations, applied in order, each in its own transaction, and recorded by ID in a `schema_migrations` table. Each migration uses its own frozen copy of the table structs, so a later change to a model cannot rewrite history (english-tutor's `internal/store/migrations.go` does this). A migration is never edited or reordered once released, and a released database must open in every later version
- The database file is created with mode `0600`

## §13 Lifetime of a build

### §13.1 Rebuild
Opening a build computes base values from the base profile as it is now, then applies the stored overrides. Build regenerates every file from the database (§11.3).

### §13.2 Uninstall
Removes every recorded file of the build whose SHA-256 still matches; a file that was edited since is listed and left in place unless the user confirms. The private home folder is removed only if the user ticks "Also delete its private home" (off by default). Then the build is removed from the database, or kept as "not installed" if the user chooses **Keep settings**.

### §13.3 Firejail updates
A build includes its base profile by path, so firejail's fixes reach the sandbox at its next start without a rebuild. If the base profile file is gone, the build is marked "base profile missing" and its editor asks for another one. When the firejail version differs from the build's `FirejailVersion`, the build list shows "firejail updated" and Extra rules are checked again.

## §14 Errors and logs
- Errors follow CLAUDE.md (tags, routes). An error the user should see is shown in the UI in plain words with what to do; the details go to the log
- The log is `slog` text on standard error; `JAILCAT_LOG=debug` turns on debug entries

## §15 Testing
- Unit tests run on fixtures in `testdata/` through an `internal/xdg` environment that points there: desktop entries, a small icon theme, a fake `$PATH`, a fake `/etc/firejail` with profiles and includes
- Generated files are compared with golden text; screens with golden images (`gluttertest`, `Tester.MatchesGolden`)
- `jailcat_firejail` tests run the real firejail on builds in a `t.TempDir()` home and check what the sandbox does: a hidden folder is not visible, a read-only folder cannot be written, `net none` has only `lo`, `nosound` has no sound devices, `ignore` before `include` undoes the included line. Firejail does not sandbox inside another sandbox (it prints "an existing sandbox was detected" and runs the program unsandboxed), so these tests fail when they see that warning instead of passing
