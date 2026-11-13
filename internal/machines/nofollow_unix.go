//go:build !windows

package machines

import "syscall"

// noFollow refuses to open a file through a symlink at its last element.
const noFollow = syscall.O_NOFOLLOW
