//go:build !windows

package localipc

import "syscall"

func running(pid int) bool {
	var status syscall.WaitStatus
	if wpid, _ := syscall.Wait4(pid, &status, syscall.WNOHANG, nil); wpid == pid {
		return false
	}
	return syscall.Kill(pid, 0) == nil && !zombie(pid)
}
