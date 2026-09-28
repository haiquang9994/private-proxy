//go:build windows

package deploy

import (
	"os/exec"
	"syscall"
)

// isolateFromTerminalSignals starts cmd in a new process group, so a
// console Ctrl+C reaches only proxyctl, which then kills cmd itself via
// its context.
func isolateFromTerminalSignals(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
