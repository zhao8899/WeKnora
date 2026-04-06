package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateScriptCommon implements the cross-platform portion of script
// validation: existence, file type, absolute-path check, and allow-list match.
func validateScriptCommon(cfg *Config, scriptPath string) error {
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
	if !filepath.IsAbs(scriptPath) {
		return fmt.Errorf("script path must be absolute: %s", scriptPath)
	}
	if len(cfg.AllowedPaths) == 0 {
		return nil
	}
	absPath, _ := filepath.Abs(scriptPath)
	for _, allowedPath := range cfg.AllowedPaths {
		absAllowed, _ := filepath.Abs(allowedPath)
		if strings.HasPrefix(absPath, absAllowed) {
			return nil
		}
	}
	return fmt.Errorf("script path not in allowed paths: %s", scriptPath)
}

// getInterpreterCommon returns the interpreter executable for a script file
// based on its extension. Falls back to "sh" for unknown extensions.
func getInterpreterCommon(scriptPath string) string {
	ext := strings.ToLower(filepath.Ext(scriptPath))
	switch ext {
	case ".py":
		return "python3"
	case ".sh", ".bash":
		return "bash"
	case ".js":
		return "node"
	case ".rb":
		return "ruby"
	case ".pl":
		return "perl"
	case ".php":
		return "php"
	default:
		return "sh"
	}
}

// isAllowedCommandCommon checks the interpreter against the configured or
// default allow-list.
func isAllowedCommandCommon(cfg *Config, cmd string) bool {
	allowed := cfg.AllowedCommands
	if len(allowed) == 0 {
		allowed = defaultAllowedCommands()
	}
	for _, a := range allowed {
		if cmd == a {
			return true
		}
	}
	return false
}

// buildEnvironmentCommon constructs a minimal, filtered environment for
// script execution. Dangerous variables (LD_PRELOAD, PYTHONPATH, etc.) are
// dropped from the caller-supplied extra map.
func buildEnvironmentCommon(extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	}
	dangerous := map[string]bool{
		"LD_PRELOAD":      true,
		"LD_LIBRARY_PATH": true,
		"PYTHONPATH":      true,
		"NODE_OPTIONS":    true,
		"BASH_ENV":        true,
		"ENV":             true,
		"SHELL":           true,
	}
	for key, value := range extra {
		if dangerous[strings.ToUpper(key)] {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}
