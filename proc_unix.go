//go:build !windows

package main

import "syscall"

// detachedProcAttr détache le script d'installation du process parent, afin
// qu'il survive au quit de l'app pendant l'auto-update.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
