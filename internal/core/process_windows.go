//go:build windows

package core

import "syscall"

const processQueryLimitedInformation = 0x1000

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, e := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if e != nil {
		return false
	}
	_ = syscall.CloseHandle(h)
	return true
}
func killPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	h, e := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
	if e != nil {
		return e
	}
	defer syscall.CloseHandle(h)
	return syscall.TerminateProcess(h, 1)
}
