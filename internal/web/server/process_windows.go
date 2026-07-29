//go:build windows

package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// configureDevelopmentCommand leaves Windows process creation unchanged because
// os.Process provides no portable graceful signal for a child process tree.
func configureDevelopmentCommand(*exec.Cmd) {}

// terminateDevelopmentCommand stops the Vite child immediately. Without a
// graceful tree signal, the first shutdown stage kills the direct child and
// then waits for commandChild's sole Wait call to reap it.
func terminateDevelopmentCommand(command *exec.Cmd) error {
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("stop Vite process: %w", err)
	}
	return nil
}

// killDevelopmentCommand repeats the direct-child kill if the shared shutdown
// deadline wins the race with process completion. An exited child is success in
// either shutdown stage.
func killDevelopmentCommand(command *exec.Cmd) error {
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill Vite process: %w", err)
	}
	return nil
}
