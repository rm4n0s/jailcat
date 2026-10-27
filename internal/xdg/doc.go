// Package xdg resolves the folders Jailcat reads and writes (SPEC §3.1): the XDG base
// directories, the bin folder and Jailcat's own data folder, from an environment it is
// given. No other package reads the environment or hard-codes a folder of the home.
package xdg
