//go:build !windows

package server

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// configureDevelopmentCommand places Vite and the descendants launched through
// pnpm in a process group owned by this command lifecycle.
func configureDevelopmentCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateDevelopmentCommand begins graceful shutdown for the entire Vite
// process group. commandChild.Stop waits for the group through its deadline
// before escalating.
func terminateDevelopmentCommand(command *exec.Cmd) error {
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM); err != nil &&
		!errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("terminate Vite process group: %w", err)
	}
	return nil
}

// killDevelopmentCommand ends the process group after the graceful shutdown
// deadline. A missing group means the child tree exited during escalation.
func killDevelopmentCommand(command *exec.Cmd) error {
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil &&
		!errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill Vite process group: %w", err)
	}
	return nil
}
