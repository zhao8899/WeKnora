//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LocalSandbox implements the Sandbox interface using local process isolation.
// The Windows build uses the same whitelist + timeout guardrails as the Unix
// implementation but skips the process-group setup (Setpgid/Kill(-pid)) which
// has no Windows analogue. Callers that need OS-level sandboxing on Windows
// should prefer the Docker sandbox.
type LocalSandbox struct {
	config *Config
}

// NewLocalSandbox creates a new local process-based sandbox.
func NewLocalSandbox(config *Config) *LocalSandbox {
	if config == nil {
		config = DefaultConfig()
	}
	return &LocalSandbox{config: config}
}

// Type returns the sandbox type.
func (s *LocalSandbox) Type() SandboxType { return SandboxTypeLocal }

// IsAvailable reports whether the local sandbox can be used.
func (s *LocalSandbox) IsAvailable(ctx context.Context) bool { return true }

// Execute runs a script locally with basic isolation.
func (s *LocalSandbox) Execute(ctx context.Context, config *ExecuteConfig) (*ExecuteResult, error) {
	if config == nil {
		return nil, ErrInvalidScript
	}
	if err := s.validateScript(config.Script); err != nil {
		return nil, err
	}

	interpreter := s.getInterpreter(config.Script)
	if !s.isAllowedCommand(interpreter) {
		return nil, fmt.Errorf("interpreter not allowed: %s", interpreter)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = s.config.DefaultTimeout
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append([]string{config.Script}, config.Args...)
	cmd := exec.CommandContext(execCtx, interpreter, args...)
	if config.WorkDir != "" {
		cmd.Dir = config.WorkDir
	} else {
		cmd.Dir = filepath.Dir(config.Script)
	}
	cmd.Env = s.buildEnvironment(config.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if config.Stdin != "" {
		cmd.Stdin = strings.NewReader(config.Stdin)
	}

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	result := &ExecuteResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			// CommandContext already sent os.Kill to the process — there's
			// no process-group kill on Windows, so child grandchildren may
			// linger. Docker sandbox is the right answer for production.
			result.Killed = true
			result.Error = ErrTimeout.Error()
			result.ExitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
			result.ExitCode = -1
		}
	}
	return result, nil
}

// validateScript checks that the script path is absolute, exists, is a file,
// and falls inside the configured allow-list.
func (s *LocalSandbox) validateScript(scriptPath string) error {
	return validateScriptCommon(s.config, scriptPath)
}

func (s *LocalSandbox) getInterpreter(scriptPath string) string {
	return getInterpreterCommon(scriptPath)
}

func (s *LocalSandbox) isAllowedCommand(cmd string) bool {
	return isAllowedCommandCommon(s.config, cmd)
}

func (s *LocalSandbox) buildEnvironment(extra map[string]string) []string {
	return buildEnvironmentCommon(extra)
}

// Cleanup releases any resources held by the sandbox.
func (s *LocalSandbox) Cleanup(ctx context.Context) error { return nil }
