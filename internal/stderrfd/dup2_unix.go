//go:build unix && !linux

package stderrfd

import "syscall"

// dup2 points newfd at whatever oldfd refers to. On the BSDs — darwin, the other
// platform kubecom releases for — dup2 is the syscall that exists; dup3 is Linux's.
func dup2(oldfd, newfd int) error {
	return syscall.Dup2(oldfd, newfd)
}
