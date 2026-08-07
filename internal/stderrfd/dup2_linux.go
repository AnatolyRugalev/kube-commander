//go:build linux

package stderrfd

import "syscall"

// dup2 points newfd at whatever oldfd refers to. Linux has no dup2 syscall on every
// architecture kubecom builds for — arm64 dropped it — so the portable spelling here
// is dup3 with no flags, which is defined to behave exactly as dup2 does (and, unlike
// dup2, is present on every linux GOARCH in package syscall).
func dup2(oldfd, newfd int) error {
	return syscall.Dup3(oldfd, newfd, 0)
}
