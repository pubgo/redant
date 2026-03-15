package redant

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type envSnapshot struct {
	value   string
	existed bool
}

// preloadEnvFromArgs scans global env-related flags from raw args, applies them
// to the process environment before normal flag parsing, and returns a restore
// function to avoid leaking state between invocations.
func preloadEnvFromArgs(args []string) (restore func() error, err error) {
	snapshots := make(map[string]envSnapshot)

	defer func() {
		if err != nil && len(snapshots) > 0 {
			_ = restoreEnvSnapshots(snapshots)
		}
	}()

	setEnv := func(key, value string) error {
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("environment variable name cannot be empty")
		}
		if _, ok := snapshots[key]; !ok {
			prev, existed := os.LookupEnv(key)
			snapshots[key] = envSnapshot{value: prev, existed: existed}
		}
		return os.Setenv(key, value)
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--" {
			break
		}

		if flagName, value, ok, parseErr := parseEnvFlagFromArgs(args, i); parseErr != nil {
			return nil, parseErr
		} else if ok {
			if consumesNextArg(arg, flagName) {
				i++
			}

			switch flagName {
			case "env":
				if err := applyEnvAssignmentsCSV(value, setEnv); err != nil {
					return nil, fmt.Errorf("invalid --env value %q: %w", value, err)
				}
			case "env-file":
				paths, err := readAsCSV(value)
				if err != nil {
					return nil, fmt.Errorf("parsing --env-file value %q: %w", value, err)
				}
				for _, path := range paths {
					path = strings.TrimSpace(path)
					if path == "" {
						continue
					}
					if err := loadEnvFile(path, setEnv); err != nil {
						return nil, fmt.Errorf("loading --env-file entry %q: %w", path, err)
					}
				}
			}
		}
	}

	if len(snapshots) == 0 {
		return nil, nil
	}

	restore = func() error {
		return restoreEnvSnapshots(snapshots)
	}

	return restore, nil
}

func restoreEnvSnapshots(snapshots map[string]envSnapshot) error {
	var merr error
	for key, snap := range snapshots {
		var err error
		if snap.existed {
			err = os.Setenv(key, snap.value)
		} else {
			err = os.Unsetenv(key)
		}
		merr = errors.Join(merr, err)
	}
	return merr
}

func parseLongFlag(arg string) (name, value string, hasInlineValue, ok bool) {
	if !strings.HasPrefix(arg, "--") {
		return "", "", false, false
	}
	token := strings.TrimPrefix(arg, "--")
	if token == "" {
		return "", "", false, false
	}
	parts := strings.SplitN(token, "=", 2)
	name = parts[0]
	if len(parts) == 2 {
		value = parts[1]
		hasInlineValue = true
	}
	return name, value, hasInlineValue, true
}

func parseShortEFlag(arg string) (value string, hasInlineValue, ok bool) {
	if strings.HasPrefix(arg, "--") || !strings.HasPrefix(arg, "-e") {
		return "", false, false
	}
	if arg == "-e" {
		return "", false, true
	}
	if strings.HasPrefix(arg, "-e=") {
		return strings.TrimPrefix(arg, "-e="), true, true
	}
	return strings.TrimPrefix(arg, "-e"), true, true
}

func parseEnvFlagFromArgs(args []string, i int) (name, value string, ok bool, err error) {
	arg := strings.TrimSpace(args[i])

	if flagName, flagValue, hasInlineValue, parsed := parseLongFlag(arg); parsed {
		switch flagName {
		case "env", "env-file":
			if !hasInlineValue {
				if i+1 >= len(args) {
					return "", "", false, fmt.Errorf("flag --%s requires a value", flagName)
				}
				flagValue = args[i+1]
			}
			return flagName, flagValue, true, nil
		default:
			return "", "", false, nil
		}
	}

	if flagValue, hasInlineValue, parsed := parseShortEFlag(arg); parsed {
		if !hasInlineValue {
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("flag -e requires a value")
			}
			flagValue = args[i+1]
		}
		return "env", flagValue, true, nil
	}

	return "", "", false, nil
}

func consumesNextArg(currentArg, flagName string) bool {
	if strings.HasPrefix(currentArg, "--") {
		return currentArg == "--"+flagName
	}
	if flagName == "env" {
		return currentArg == "-e"
	}
	return false
}

func parseEnvAssignment(raw string) (key, value string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("empty environment assignment")
	}
	idx := strings.Index(raw, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("expected KEY=VALUE")
	}
	key = strings.TrimSpace(raw[:idx])
	value = strings.TrimSpace(raw[idx+1:])
	if key == "" {
		return "", "", fmt.Errorf("environment variable name cannot be empty")
	}
	return key, value, nil
}

func applyEnvAssignmentsCSV(raw string, setEnv func(key, value string) error) error {
	entries, err := readAsCSV(raw)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, value, err := parseEnvAssignment(entry)
		if err != nil {
			return err
		}
		if err := setEnv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string, setEnv func(key, value string) error) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("env file path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, err := parseEnvAssignment(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		value = normalizeEnvValue(value)
		if err := setEnv(key, value); err != nil {
			return fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
	}
	return nil
}

func normalizeEnvValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if unquoted, err := strconv.Unquote(v); err == nil {
			return unquoted
		}
	}
	return v
}
