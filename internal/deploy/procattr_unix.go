//go:build unix

package deploy

import (
	"os/exec"
	"syscall"
)

// isolateFromTerminalSignals puts cmd in its own process group, so a
// terminal Ctrl+C reaches only proxyctl, which then kills cmd itself via
// its context. Otherwise the child could exit first and race proxyctl's
// own signal handling.
func isolateFromTerminalSignals(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
