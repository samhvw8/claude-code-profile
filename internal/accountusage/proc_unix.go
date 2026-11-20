//go:build unix

package accountusage

import (
	"os/exec"
	"syscall"
)

// configureAgyCmd puts agy in its own process group so timeout cancellation
// terminates any spawned child processes along with the parent.
func configureAgyCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}
