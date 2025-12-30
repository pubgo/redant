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
// --debug

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
			Flag:        "list-commands",
			Description: "List all commands, including subcommands.",
			Value:       BoolOf(new(bool)),
		},
		{
			Flag:        "list-flags",
			Description: "List all flags.",
			Value:       BoolOf(new(bool)),
		},
	}
}

// PrintCommands and PrintFlags have been moved to help.go for better formatting
