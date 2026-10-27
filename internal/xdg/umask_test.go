package xdg

import "syscall"

// umaskForTest sets the process umask and returns the old one.
func umaskForTest(m int) int {
	return syscall.Umask(m)
}
