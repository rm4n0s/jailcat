# Firejail 0.9.80 profiles used as test data

The files in this folder, except this README, are copied unchanged from `/etc/firejail`
of firejail 0.9.80 (GPL-2.0, <https://github.com/netblue30/firejail>). They are test data
for the profile reader in `internal/firejail`: `code.profile`, `default.profile` and
the files they include, directly or through `electron-common.profile` and
`blink-common.profile`, plus `firejail.config`.

Tests point `xdg.Dirs.FirejailSystem` at this folder. Do not edit these files; hand-written
edge cases are written into a temporary folder by the tests.

`globals.local`, `code.local` and the other `.local` files are not here: the base
profiles include them, and firejail skips a missing `.local` file.
