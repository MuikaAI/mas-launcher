//go:build !windows

package main

import (
	"os"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, e := os.FindProcess(pid)
	return e == nil && p.Signal(syscall.Signal(0)) == nil
}
func killPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	p, e := os.FindProcess(pid)
	if e != nil {
		return e
	}
	return p.Kill()
}
