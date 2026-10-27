package firejail

import "slices"

// Commands returns the profile commands of firejail 0.9.80, sorted, as a fresh slice.
//
// The list is what profile_check_line and profile_read in firejail's profile.c
// recognize, checked against "man firejail-profile". It leaves out what Jailcat must
// not offer:
//   - "tab" (a command line option kept in the code) and "shell" (the code answers it
//     with "the shell command has been removed" and can never reach it)
//   - "keep-hostname", removed ("ignoring removed command", replaced by
//     hostname-randomize)
//   - "xephyr-extra-params", which the man page lists but profile.c does not handle
//
// "apparmor", "apparmor-replace", "apparmor-stack" and "private-lib" are in the list
// although the man page of a build without them leaves them out: profile.c reads them.
func Commands() []string {
	list := []string{
		"allow-bwrap", "allow-debuggers", "allusers", "apparmor", "apparmor-replace",
		"apparmor-stack", "bind", "blacklist", "blacklist-nolog", "caps", "caps.drop",
		"caps.keep", "cpu", "dbus-system", "dbus-system.broadcast", "dbus-system.call",
		"dbus-system.own", "dbus-system.see", "dbus-system.talk", "dbus-user",
		"dbus-user.broadcast", "dbus-user.call", "dbus-user.own", "dbus-user.see",
		"dbus-user.talk", "defaultgw", "deterministic-exit-code", "deterministic-shutdown",
		"disable-mnt", "dns", "env", "hostname", "hostname-randomize", "hosts-file",
		"ignore", "include", "ip", "ip6", "ipc-namespace", "iprange", "join-or-start",
		"keep-config-pulse", "keep-dev-ntsync", "keep-dev-shm", "keep-dev-tpm", "keep-fd",
		"keep-shell-rc", "keep-var-tmp", "landlock.enforce", "landlock.fs.execute",
		"landlock.fs.makedev", "landlock.fs.makeipc", "landlock.fs.read",
		"landlock.fs.write", "mac", "machine-id", "memory-deny-write-execute", "mkdir",
		"mkfile", "mtu", "name", "net", "netfilter", "netfilter6", "netlock", "netmask",
		"netns", "nice", "no3d", "noautopulse", "noblacklist", "nodbus", "nodvd", "noexec",
		"nogroups", "noinput", "nonewprivs", "noprinters", "noroot", "nosound", "notpm",
		"notv", "nou2f", "novideo", "nowhitelist", "private", "private-bin",
		"private-cache", "private-cwd", "private-dev", "private-etc", "private-home",
		"private-lib", "private-opt", "private-srv", "private-tmp", "protocol", "quiet",
		"read-only", "read-write", "restrict-namespaces", "rlimit-as", "rlimit-cpu",
		"rlimit-fsize", "rlimit-nofile", "rlimit-nproc", "rlimit-sigpending", "rmenv",
		"seccomp", "seccomp.32", "seccomp.32.drop", "seccomp.32.keep",
		"seccomp.block-secondary", "seccomp.drop", "seccomp-error-action", "seccomp.keep",
		"timeout", "tmpfs", "tracelog", "veth-name", "warn", "whitelist", "whitelist-ro",
		"writable-etc", "writable-run-user", "writable-var", "writable-var-log", "x11",
		"xephyr-screen",
	}
	slices.Sort(list)
	return list
}

// IsCommand reports whether name is one of the profile commands of [Commands].
func IsCommand(name string) bool {
	return slices.Contains(Commands(), name)
}
