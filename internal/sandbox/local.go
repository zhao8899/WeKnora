package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// LocalSandbox implements the Sandbox interface using local process isolation
// This is a fallback option when Docker is not available
// It provides basic isolation through:
// - Command whitelist validation
// - Working directory restriction
// - Timeout enforcement
// - Environment variable filtering
type LocalSandbox struct {
	config *Config
}

// NewLocalSandbox creates a new local process-based sandbox
func NewLocalSandbox(config *Config) *LocalSandbox {
	if config == nil {
		config = DefaultConfig()
	}
	return &LocalSandbox{
		config: config,
	}
}

// Type returns the sandbox type
func (s *LocalSandbox) Type() SandboxType {
	return SandboxTypeLocal
}

// IsAvailable checks if local sandbox is available
func (s *LocalSandbox) IsAvailable(ctx context.Context) bool {
	// Local sandbox is always available
	return true
}

// Execute runs a script locally with basic isolation
func (s *LocalSandbox) Execute(ctx context.Context, config *ExecuteConfig) (*ExecuteResult, error) {
	if config == nil {
		return nil, ErrInvalidScript
	}

	// Validate the script path
	if err := s.validateScript(config.Script); err != nil {
		return nil, err
	}

	// Determine interpreter
	interpreter := s.getInterpreter(config.Script)
	if !s.isAllowedCommand(interpreter) {
		return nil, fmt.Errorf("interpreter not allowed: %s", interpreter)
	}

	// Set default timeout
	timeout := config.Timeout
	if timeout == 0 {
		timeout = s.config.DefaultTimeout
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	args := s.buildCommandArgs(interpreter, config.Script, config.Args)
	cmd := exec.CommandContext(execCtx, interpreter, args...)

	// Set working directory
	if config.WorkDir != "" {
		cmd.Dir = config.WorkDir
	} else {
		cmd.Dir = filepath.Dir(config.Script)
	}

	// Setup minimal environment
	cmd.Env = s.buildEnvironment(config.Env)

	// Configure process attributes so timeout cleanup can terminate the spawned
	// process tree on each supported platform.
	applySandboxSysProcAttr(cmd)

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
			// Best-effort timeout cleanup. Platform-specific helpers handle
			// process-group termination where available and fall back to killing
			// the direct child process otherwise.
			if killErr := killSandboxProcess(cmd); killErr != nil {
				result.Stderr += fmt.Sprintf("\n[sandbox cleanup] %v", killErr)
			}
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

func (s *LocalSandbox) buildCommandArgs(interpreter, script string, scriptArgs []string) []string {
	ext := strings.ToLower(filepath.Ext(script))
	if ext == ".cmd" || ext == ".bat" {
		args := []string{"/c", script}
		return append(args, scriptArgs...)
	}
	return append([]string{script}, scriptArgs...)
}

// validateScript checks if the script path is valid and safe
func (s *LocalSandbox) validateScript(scriptPath string) error {
	// Check if script exists
	info, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrScriptNotFound
		}
		return fmt.Errorf("failed to access script: %w", err)
	}

	if info.IsDir() {
		return ErrInvalidScript
	}

	// Check path is absolute
	if !filepath.IsAbs(scriptPath) {
		return fmt.Errorf("script path must be absolute: %s", scriptPath)
	}

	// Validate against allowed paths if configured
	if len(s.config.AllowedPaths) > 0 {
		allowed := false
		absPath, _ := filepath.Abs(scriptPath)
		for _, allowedPath := range s.config.AllowedPaths {
			absAllowed, _ := filepath.Abs(allowedPath)
			if strings.HasPrefix(absPath, absAllowed) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("script path not in allowed paths: %s", scriptPath)
		}
	}

	return nil
}

// getInterpreter returns the appropriate interpreter for a script
func (s *LocalSandbox) getInterpreter(scriptPath string) string {
	ext := strings.ToLower(filepath.Ext(scriptPath))
	switch ext {
	case ".py":
		return s.firstAvailableCommand("python3", "py", "python")
	case ".sh", ".bash":
		return s.firstAvailableCommand(
			"bash",
			"C:\\Program Files\\Git\\bin\\bash.exe",
			"C:\\Program Files\\Git\\usr\\bin\\bash.exe",
			"C:\\Program Files\\Git\\bin\\sh.exe",
			"C:\\Program Files\\Git\\usr\\bin\\sh.exe",
			"sh",
		)
	case ".js":
		return s.firstAvailableCommand("node")
	case ".rb":
		return s.firstAvailableCommand("ruby")
	case ".pl":
		return s.firstAvailableCommand("perl")
	case ".php":
		return s.firstAvailableCommand("php")
	case ".cmd", ".bat":
		return s.firstAvailableCommand("cmd")
	default:
		return s.firstAvailableCommand("sh", "bash")
	}
}

func (s *LocalSandbox) firstAvailableCommand(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			if strings.Contains(strings.ToLower(resolved), `\windowsapps\`) {
				continue
			}
			return resolved
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// isAllowedCommand checks if a command is in the allowed list
func (s *LocalSandbox) isAllowedCommand(cmd string) bool {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(cmd), filepath.Ext(cmd)))
	if len(s.config.AllowedCommands) == 0 {
		// Use default allowed commands
		defaults := defaultAllowedCommands()
		for _, allowed := range defaults {
			if base == strings.ToLower(allowed) {
				return true
			}
		}
		return false
	}

	for _, allowed := range s.config.AllowedCommands {
		if base == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}

// buildEnvironment creates a safe environment for script execution
func (s *LocalSandbox) buildEnvironment(extra map[string]string) []string {
	if runtime.GOOS == "windows" {
		env := []string{
			"PATH=" + os.Getenv("PATH"),
			"SystemRoot=" + os.Getenv("SystemRoot"),
			"ComSpec=" + os.Getenv("ComSpec"),
			"TEMP=" + os.Getenv("TEMP"),
			"TMP=" + os.Getenv("TMP"),
			"LANG=en_US.UTF-8",
			"LC_ALL=en_US.UTF-8",
		}
		home := os.Getenv("USERPROFILE")
		if home == "" {
			home = os.Getenv("HOME")
		}
		if home != "" {
			env = append(env, "HOME="+home, "USERPROFILE="+home)
		}

		dangerous := map[string]bool{
			"PYTHONPATH":   true,
			"NODE_OPTIONS": true,
		}
		for key, value := range extra {
			if dangerous[strings.ToUpper(key)] {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		return env
	}

	// Start with minimal environment
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}

	// Dangerous environment variables to exclude
	dangerous := map[string]bool{
		"LD_PRELOAD":      true,
		"LD_LIBRARY_PATH": true,
		"PYTHONPATH":      true,
		"NODE_OPTIONS":    true,
		"BASH_ENV":        true,
		"ENV":             true,
		"SHELL":           true,
	}

	// Add extra environment variables (filtered)
	for key, value := range extra {
		upperKey := strings.ToUpper(key)
		if dangerous[upperKey] {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// Cleanup releases any resources
func (s *LocalSandbox) Cleanup(ctx context.Context) error {
	// Local sandbox doesn't need cleanup
	return nil
}
