package redant

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/pflag"
)

// Command format specification
// Application unified format: fastcommit command:sub-cmd [args...] [flags...]
// 1. fastcommit is the application name
// 2. command:sub-cmd uses colon separator to indicate command hierarchy
// 3. args support multiple formats:
//    - Query string format: name=value&a=b&name=123
//    - Form data format: name=value name2=value2
//    - JSON format: {"key":"value"} or ["value1","value2"]
//    - Single data format: direct data value
// 4. flags are always placed at the end, and subcommand flags cannot duplicate parent command flags
// 5. The entire organization format maintains consistency, all operations follow this specification

// Parameter format examples
// Query string format parameters:
// name=value&a=b&name=123
//
// Form data format parameters:
// name=value name2=value2
//
// JSON format parameters:
// {"id":123,"name":"test"} or ["value1","value2"]
//
// Single data format parameters:
// 123456
//
// Parameter help information display format:
// args:
// 	 name int, this is name , env=[$ABC] default=[123] required=[true] enums=[]
//
// Flag help information display format:
// flags(sub-command):
//   --name,-n int, short, env:[$ABC] default:[123] required:[true]
//   --name,-n int, short, env:[$ABC] default:[123] required:[true]
//
// Global flags:
// flags(global):
//   --name,-n int, short, env:[$ABC]
//   --name,-n int, short, env:[$ABC]
//
// Default global flags:
// --list-commands List all commands, including subcommands
// --list-flags List all flags
// --help,-h
// --version,-v
// --config-file,-c string, short, env:[$ABC]
// --debug
// --log-level string, short, env:[$ABC]
// --environment string, short, env:[$ABC]
// --env-file string, short, env:[$ABC]
// --env-files string, short, env:[$ABC]

// Parameter type description
// TextArg is a single argument to a command. 'user is hello and age is 18'
// QueryArg is a query argument to a command. user=hello&age=18
// FormArg is a form argument to a command. user=hello age=18
// JSONArg is a JSON argument to a command. {"user":"hello","age":18} or ["value1","value2"]

// ArgValidator is a function that validates an argument.

type ArgSet []Arg

type Arg struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// Required means this value must be set by some means.
	// If `Default` is set, then `Required` is ignored.
	Required bool `json:"required,omitempty"`

	// Default is the default value for this argument.
	Default string `json:"default,omitempty"`

	// Value includes the types listed in values.go.
	// Used for type determination and automatic parsing.
	Value pflag.Value `json:"value,omitempty"`
}

// ParseQueryArgs parses query string formatted arguments into a map
func ParseQueryArgs(query string) (map[string][]string, error) {
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// ParseFormArgs parses form formatted arguments into a map
// Format: key1=value1 key2=value2 key3="value with spaces"
// Values containing spaces should be quoted with single or double quotes
func ParseFormArgs(form string) (map[string][]string, error) {
	values := make(map[string][]string)

	// Parse key=value pairs, respecting quoted strings
	start := 0
	inQuotes := false
	quoteChar := byte(0)
	seenEquals := false // Track if we've seen '=' in current pair

	for i := 0; i <= len(form); i++ {
		if i >= len(form) {
			// End of string - process remaining part
			if i > start {
				part := strings.TrimSpace(form[start:i])
				if part != "" {
					if strings.Contains(part, "=") {
						kv := strings.SplitN(part, "=", 2)
						key := strings.TrimSpace(kv[0])
						value := strings.TrimSpace(kv[1])

						// Remove quotes if present
						value = trimQuotes(value)

						if key != "" {
							values[key] = append(values[key], value)
						}
					} else {
						values[""] = append(values[""], part)
					}
				}
			}
			break
		}

		char := form[i]

		// Handle quote characters
		if char == '"' || char == '\'' {
			if !inQuotes {
				// Starting a quoted section
				inQuotes = true
				quoteChar = char
			} else if char == quoteChar {
				// Ending a quoted section
				inQuotes = false
				quoteChar = 0
			}
			continue
		}

		// Check for '=' to mark the start of value
		if char == '=' && !inQuotes {
			seenEquals = true
			continue
		}

		// Split on space if not in quotes and we've seen '=' for this pair
		if char == ' ' && !inQuotes && seenEquals {
			if i > start {
				part := form[start:i]
				if strings.Contains(part, "=") {
					kv := strings.SplitN(part, "=", 2)
					key := strings.TrimSpace(kv[0])
					value := strings.TrimSpace(kv[1])

					// Remove quotes if present
					value = trimQuotes(value)

					if key != "" {
						values[key] = append(values[key], value)
					}
				}
			}
			start = i + 1
			seenEquals = false // Reset for next pair
		}
	}

	return values, nil
}

// trimQuotes removes surrounding quotes from a string if present
func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ParseJSONArgs parses JSON formatted arguments into a map
// JSON can be either an object like {"name":"value","age":18} or an array like ["value1","value2"]
func ParseJSONArgs(jsonStr string) (map[string][]string, error) {
	values := make(map[string][]string)

	// Try to parse as JSON object
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &obj); err == nil {
		// Successfully parsed as object
		for key, val := range obj {
			// Convert value to string
			var strVal string
			switch v := val.(type) {
			case string:
				strVal = v
			case float64:
				strVal = fmt.Sprintf("%g", v)
			case bool:
				strVal = fmt.Sprintf("%t", v)
			case nil:
				strVal = ""
			default:
				// For complex types, marshal back to JSON string
				if jsonBytes, err := json.Marshal(v); err == nil {
					strVal = string(jsonBytes)
				} else {
					strVal = fmt.Sprintf("%v", v)
				}
			}
			values[key] = append(values[key], strVal)
		}
		return values, nil
	}

	// Try to parse as JSON array
	var arr []any
	if err := json.Unmarshal([]byte(jsonStr), &arr); err == nil {
		// Successfully parsed as array - use empty key for positional args
		for _, val := range arr {
			var strVal string
			switch v := val.(type) {
			case string:
				strVal = v
			case float64:
				strVal = fmt.Sprintf("%g", v)
			case bool:
				strVal = fmt.Sprintf("%t", v)
			case nil:
				strVal = ""
			default:
				if jsonBytes, err := json.Marshal(v); err == nil {
					strVal = string(jsonBytes)
				} else {
					strVal = fmt.Sprintf("%v", v)
				}
			}
			values[""] = append(values[""], strVal)
		}
		return values, nil
	}

	return nil, fmt.Errorf("invalid JSON format")
}

// GlobalFlags returns the default global flags that should be added to every command
func GlobalFlags() OptionSet {
	return OptionSet{
		{
			Flag:        "help",
			Shorthand:   "h",
			Description: "Show help for command.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "version",
			Shorthand:   "v",
			Description: "Show version information.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "list-commands",
			Description: "List all commands, including subcommands.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "list-flags",
			Description: "List all flags.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "config-file",
			Shorthand:   "c",
			Description: "Path to the configuration file.",
			Value:       StringOf(new(string)),
		},
		{
			Flag:        "debug",
			Description: "Enable debug mode.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "log-level",
			Description: "Set the logging level.",
			Value:       StringOf(new(string)),
			Default:     "info",
		},
		{
			Flag:        "environment",
			Shorthand:   "e",
			Description: "Set the environment.",
			Value:       StringOf(new(string)),
		},
		{
			Flag:        "env-file",
			Description: "Path to the environment file.",
			Value:       StringOf(new(string)),
		},
		{
			Flag:        "env-files",
			Description: "Paths to the environment files.",
			Value:       StringArrayOf(new([]string)),
		},
	}
}

// PrintCommands prints all commands in a formatted list with full paths
func PrintCommands(cmd *Command) {
	// Collect all commands with their full paths
	var commands []struct {
		path string
		desc string
	}

	// Recursive function to collect commands
	var collectCommands func(*Command, string)
	collectCommands = func(c *Command, prefix string) {
		// Build the full path for this command
		var fullPath string
		if prefix == "" {
			fullPath = c.Name()
		} else {
			fullPath = prefix + ":" + c.Name()
		}

		// Add this command to the list
		commands = append(commands, struct {
			path string
			desc string
		}{
			path: fullPath,
			desc: c.Short,
		})

		// Recursively collect child commands
		for _, child := range c.Children {
			collectCommands(child, fullPath)
		}
	}

	// Start collecting from the root command's children
	for _, child := range cmd.Children {
		collectCommands(child, cmd.Name())
	}

	if len(commands) == 0 {
		fmt.Println("No commands available.")
		return
	}

	// Find the maximum path length for alignment
	maxPathLen := 0
	for _, command := range commands {
		if len(command.path) > maxPathLen {
			maxPathLen = len(command.path)
		}
	}

	// Print header
	fmt.Println("Available Commands:")
	fmt.Println()

	// Print all commands with aligned formatting
	for _, command := range commands {
		padding := strings.Repeat(" ", maxPathLen-len(command.path)+2)
		fmt.Printf("  %s%s%s\n", command.path, padding, command.desc)
	}
}

// PrintFlags prints all flags for all commands, separating global and command-specific flags
func PrintFlags(rootCmd *Command) {
	globalFlags := rootCmd.GetGlobalFlags()

	// Collect all commands with their full paths
	var commands []struct {
		cmd  *Command
		path string
	}

	// Recursive function to collect commands
	var collectCommands func(*Command, string)
	collectCommands = func(c *Command, prefix string) {
		// Build the full path for this command
		var fullPath string
		if prefix == "" {
			fullPath = c.Name()
		} else {
			fullPath = prefix + ":" + c.Name()
		}

		// Add this command to the list
		commands = append(commands, struct {
			cmd  *Command
			path string
		}{
			cmd:  c,
			path: fullPath,
		})

		// Recursively collect child commands
		for _, child := range c.Children {
			collectCommands(child, fullPath)
		}
	}

	// Start collecting from the root command's children
	for _, child := range rootCmd.Children {
		collectCommands(child, rootCmd.Name())
	}

	// Helper function to format flag name
	formatFlagName := func(opt Option) string {
		if opt.Flag == "" {
			return ""
		}
		if opt.Shorthand != "" {
			return "-" + opt.Shorthand + ", --" + opt.Flag
		}
		return "--" + opt.Flag
	}

	// Helper function to get flag type
	getFlagType := func(opt Option) string {
		if opt.Value == nil {
			return "bool"
		}
		return opt.Value.Type()
	}

	// Helper function to format flag info
	formatFlagInfo := func(opt Option) string {
		var parts []string
		if opt.Default != "" {
			parts = append(parts, "default: "+opt.Default)
		}
		if opt.Required {
			parts = append(parts, "required")
		}
		if len(opt.Envs) > 0 {
			// Show all environment variables
			envNames := make([]string, len(opt.Envs))
			for i, env := range opt.Envs {
				envNames[i] = "$" + env
			}
			parts = append(parts, "env: "+strings.Join(envNames, ", "))
		}
		if len(parts) > 0 {
			return " (" + strings.Join(parts, ", ") + ")"
		}
		return ""
	}

	// Print global flags
	if len(globalFlags) > 0 {
		fmt.Println("Global Flags:")
		fmt.Println()
		for _, opt := range globalFlags {
			if opt.Flag != "" {
				flagName := formatFlagName(opt)
				flagType := getFlagType(opt)
				flagInfo := formatFlagInfo(opt)
				fmt.Printf("  %s %s%s\n", flagName, flagType, flagInfo)
				if opt.Description != "" {
					fmt.Printf("      %s\n", opt.Description)
				}
			}
		}
		fmt.Println()
	}

	// Print flags for each command
	hasCommandFlags := false
	for _, command := range commands {
		if len(command.cmd.Options) > 0 {
			// Filter out global flags from command options
			var commandSpecificFlags OptionSet
			for _, opt := range command.cmd.Options {
				isGlobal := false
				for _, globalOpt := range globalFlags {
					if opt.Flag == globalOpt.Flag {
						isGlobal = true
						break
					}
				}
				if !isGlobal && opt.Flag != "" {
					commandSpecificFlags = append(commandSpecificFlags, opt)
				}
			}

			if len(commandSpecificFlags) > 0 {
				if !hasCommandFlags {
					fmt.Println("Command-Specific Flags:")
					fmt.Println()
					hasCommandFlags = true
				}
				fmt.Printf("  %s:\n", command.path)
				for _, opt := range commandSpecificFlags {
					flagName := formatFlagName(opt)
					flagType := getFlagType(opt)
					flagInfo := formatFlagInfo(opt)
					fmt.Printf("    %s %s%s\n", flagName, flagType, flagInfo)
					if opt.Description != "" {
						fmt.Printf("        %s\n", opt.Description)
					}
				}
				fmt.Println()
			}
		}
	}

	if !hasCommandFlags && len(globalFlags) == 0 {
		fmt.Println("No flags available.")
	}
}
