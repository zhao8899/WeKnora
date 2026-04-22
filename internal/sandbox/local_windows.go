//go:build windows

package sandbox

import "os/exec"

func applySandboxSysProcAttr(cmd *exec.Cmd) {
	// No special process-group configuration for Windows in the local sandbox.
}

func killSandboxProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
