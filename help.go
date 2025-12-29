package redant

import (
	"bufio"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"text/tabwriter"
	"text/template"

	"github.com/coder/pretty"
	"github.com/mitchellh/go-wordwrap"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

//go:embed help.tpl
var helpTemplateRaw string

type optionGroup struct {
	Name        string
	Description string
	Options     OptionSet
}

// getOptionGroupsByCommand returns option groups organized by command hierarchy
func getOptionGroupsByCommand(cmd *Command) []optionGroup {
	var groups []optionGroup

	// Collect commands in hierarchy order (root to current)
	var commands []*Command
	current := cmd
	for current != nil {
		commands = append([]*Command{current}, commands...)
		current = current.parent
	}

	// Create a group for each command that has options
	for _, c := range commands {
		if len(c.Options) > 0 {
			// Filter out global flags for non-root commands
			var opts OptionSet
			if c.parent == nil {
				// Root command: show all options as global options
				for _, opt := range c.Options {
					if opt.Flag != "" && !opt.Hidden {
						opts = append(opts, opt)
					}
				}
			} else {
				// Non-root command: filter out global flags
				globalFlags := c.GetGlobalFlags()
				globalFlagMap := make(map[string]bool)
				for _, gf := range globalFlags {
					globalFlagMap[gf.Flag] = true
				}
				for _, opt := range c.Options {
					if !globalFlagMap[opt.Flag] && opt.Flag != "" && !opt.Hidden {
						opts = append(opts, opt)
					}
				}
			}

			if len(opts) > 0 {
				var groupName string
				if c.parent == nil {
					// Root command: show as "Global Options"
					groupName = "Global"
				} else {
					// For subcommands, show just the command name (not full path)
					groupName = c.Name()
				}
				groups = append(groups, optionGroup{
					Name:    groupName,
					Options: opts,
				})
			}
		}
	}

	return groups
}

func ttyWidth() int {
	width, _, err := term.GetSize(0)
	if err != nil {
		return 80
	}
	return width
}

// wrapTTY wraps a string to the width of the terminal, or 80 no terminal
// is detected.
func wrapTTY(s string) string {
	return wordwrap.WrapString(s, uint(ttyWidth()))
}

// indent indents a string with the given number of spaces and wraps it to terminal width
func indent(body string, spaces int) string {
	twidth := ttyWidth()
	spacing := strings.Repeat(" ", spaces)
	wrapLim := twidth - len(spacing)
	body = wordwrap.WrapString(body, uint(wrapLim))
	sc := bufio.NewScanner(strings.NewReader(body))
	var sb strings.Builder
	for sc.Scan() {
		_, _ = sb.WriteString(spacing)
		_, _ = sb.Write(sc.Bytes())
		_, _ = sb.WriteString("\n")
	}
	return sb.String()
}

var (
	helpColorProfile termenv.Profile
	helpColorOnce    sync.Once
)

// Color returns a color for the given string.
func helpColor(s string) termenv.Color {
	helpColorOnce.Do(func() {
		helpColorProfile = termenv.NewOutput(os.Stdout).ColorProfile()
		if flag.Lookup("test.v") != nil {
			// Use a consistent colorless profile in tests so that results
			// are deterministic.
			helpColorProfile = termenv.Ascii
		}
	})
	return helpColorProfile.Color(s)
}

// prettyHeader formats a header string with consistent styling.
// It uppercases the text, adds a colon, and applies the header color.
func prettyHeader(s string) string {
	headerFg := pretty.FgColor(helpColor("#337CA0"))
	s = strings.ToUpper(s)
	txt := pretty.String(s, ":")
	headerFg.Format(txt)
	return txt.String()
}

var defaultHelpTemplate = func() *template.Template {
	optionFg := pretty.FgColor(
		helpColor("#04A777"),
	)
	return template.Must(
		template.New("usage").Funcs(
			template.FuncMap{
				"wrapTTY": func(s string) string {
					return wrapTTY(s)
				},
				"trimNewline": func(s string) string {
					return strings.TrimSuffix(s, "\n")
				},
				"keyword": func(s string) string {
					txt := pretty.String(s)
					optionFg.Format(txt)
					return txt.String()
				},
				"prettyHeader": prettyHeader,
				"typeHelper": func(opt *Option) string {
					switch v := opt.Value.(type) {
					case *Enum:
						return strings.Join(v.Choices, "|")
					case *EnumArray:
						return fmt.Sprintf("[%s]", strings.Join(v.Choices, "|"))
					default:
						return v.Type()
					}
				},
				"joinStrings": func(s []string) string {
					return strings.Join(s, ", ")
				},
				"indent": func(body string, spaces int) string {
					twidth := ttyWidth()

					spacing := strings.Repeat(" ", spaces)

					wrapLim := twidth - len(spacing)
					body = wordwrap.WrapString(body, uint(wrapLim))

					sc := bufio.NewScanner(strings.NewReader(body))

					var sb strings.Builder
					for sc.Scan() {
						// Remove existing indent, if any.
						// line = strings.TrimSpace(line)
						// Use spaces so we can easily calculate wrapping.
						_, _ = sb.WriteString(spacing)
						_, _ = sb.Write(sc.Bytes())
						_, _ = sb.WriteString("\n")
					}
					return sb.String()
				},
				"rootCommandName": func(cmd *Command) string {
					return strings.Split(cmd.FullName(), " ")[0]
				},
				"formatSubcommand": func(cmd *Command) string {
					// Minimize padding by finding the longest neighboring name.
					maxNameLength := len(cmd.Name())
					if parent := cmd.parent; parent != nil {
						for _, c := range parent.Children {
							if len(c.Name()) > maxNameLength {
								maxNameLength = len(c.Name())
							}
						}
					}

					var sb strings.Builder
					_, _ = fmt.Fprintf(
						&sb, "%s%s%s",
						strings.Repeat(" ", 4), cmd.Name(), strings.Repeat(" ", maxNameLength-len(cmd.Name())+4),
					)

					// This is the point at which indentation begins if there's a
					// next line.
					descStart := sb.Len()

					twidth := ttyWidth()

					for i, line := range strings.Split(
						wordwrap.WrapString(cmd.Short, uint(twidth-descStart)), "\n",
					) {
						if i > 0 {
							_, _ = sb.WriteString(strings.Repeat(" ", descStart))
						}
						_, _ = sb.WriteString(line)
						_, _ = sb.WriteString("\n")
					}

					return sb.String()
				},
				"flagName": func(opt Option) string {
					return opt.Flag
				},

				"formatGroupDescription": func(s string) string {
					s = strings.ReplaceAll(s, "\n", "")
					s = s + "\n"
					s = wrapTTY(s)
					return s
				},
				"visibleChildren": func(cmd *Command) []*Command {
					return filterSlice(cmd.Children, func(c *Command) bool {
						return !c.Hidden
					})
				},
				"optionGroups": func(cmd *Command) []optionGroup {
					return getOptionGroupsByCommand(cmd)
				},
				"envName": func(opt Option) string {
					if len(opt.Envs) > 0 {
						// Return all env names joined with ", "
						envNames := make([]string, len(opt.Envs))
						for i, env := range opt.Envs {
							envNames[i] = "$" + env
						}
						return strings.Join(envNames, ", ")
					}
					return ""
				},
				"isDeprecated": func(opt Option) bool {
					return opt.Deprecated != ""
				},
				"useInstead": func(opt Option) string {
					// useInstead is not currently implemented
					return ""
				},
				"hasParent": func(cmd *Command) bool {
					return cmd.parent != nil
				},
			},
		).Parse(helpTemplateRaw),
	)
}()

func filterSlice[T any](s []T, f func(T) bool) []T {
	var r []T
	for _, v := range s {
		if f(v) {
			r = append(r, v)
		}
	}
	return r
}

// newLineLimiter makes working with Go templates more bearable. Without this,
// modifying the template is a slow toil of counting newlines and constantly
// checking that a change to one command's help doesn't break another.
type newlineLimiter struct {
	// w is not an interface since we call WriteRune byte-wise,
	// and the devirtualization overhead is significant.
	w     *bufio.Writer
	limit int

	newLineCounter int
}

// isSpace is a based on unicode.IsSpace, but only checks ASCII characters.
func isSpace(b byte) bool {
	switch b {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0:
		return true
	}
	return false
}

func (lm *newlineLimiter) Write(p []byte) (int, error) {
	for _, b := range p {
		switch {
		case b == '\r':
			// Carriage returns can sneak into `help.tpl` when `git clone`
			// is configured to automatically convert line endings.
			continue
		case b == '\n':
			lm.newLineCounter++
			if lm.newLineCounter > lm.limit {
				continue
			}
		case !isSpace(b):
			lm.newLineCounter = 0
		}
		err := lm.w.WriteByte(b)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

var usageWantsArgRe = regexp.MustCompile(`<.*>`)

type UnknownSubcommandError struct {
	Args []string
}

func (e *UnknownSubcommandError) Error() string {
	return fmt.Sprintf("unknown subcommand %q", strings.Join(e.Args, " "))
}

// formatCommandName formats a command name with keyword color
func formatCommandName(name string) string {
	optionFg := pretty.FgColor(helpColor("#04A777"))
	txt := pretty.String(name)
	optionFg.Format(txt)
	return txt.String()
}

// formatFlagName formats a flag name with keyword color, returns colored shorthand and flag separately
func formatFlagName(opt Option) (shorthandColored, flagColored string) {
	optionFg := pretty.FgColor(helpColor("#04A777"))
	if opt.Shorthand != "" {
		shorthandTxt := pretty.String("-" + opt.Shorthand)
		optionFg.Format(shorthandTxt)
		shorthandColored = shorthandTxt.String()
	}
	flagTxt := pretty.String("--" + opt.Flag)
	optionFg.Format(flagTxt)
	flagColored = flagTxt.String()
	return shorthandColored, flagColored
}

// formatFlagType returns the type string for a flag
func formatFlagType(opt Option) string {
	if opt.Value == nil {
		return "bool"
	}
	switch v := opt.Value.(type) {
	case *Enum:
		return strings.Join(v.Choices, "|")
	case *EnumArray:
		return fmt.Sprintf("[%s]", strings.Join(v.Choices, "|"))
	default:
		return v.Type()
	}
}

// formatFlagEnvNames formats environment variable names
func formatFlagEnvNames(opt Option) string {
	if len(opt.Envs) == 0 {
		return ""
	}
	envNames := make([]string, len(opt.Envs))
	for i, env := range opt.Envs {
		envNames[i] = "$" + env
	}
	optionFg := pretty.FgColor(helpColor("#04A777"))
	envStr := strings.Join(envNames, ", ")
	txt := pretty.String(envStr)
	optionFg.Format(txt)
	return txt.String()
}

// formatArgType returns the type string for an arg
func formatArgType(arg Arg) string {
	if arg.Value == nil {
		return "string"
	}
	switch v := arg.Value.(type) {
	case *Enum:
		return strings.Join(v.Choices, "|")
	case *EnumArray:
		return fmt.Sprintf("[%s]", strings.Join(v.Choices, "|"))
	default:
		return v.Type()
	}
}

// PrintCommands prints all commands in a formatted list with full paths, using help formatting style
func PrintCommands(cmd *Command) {
	// Collect all commands with their full paths
	type cmdInfo struct {
		path string
		cmd  *Command
	}
	var commands []cmdInfo

	// Recursive function to collect commands
	var collectCommands func(*Command, string)
	collectCommands = func(c *Command, prefix string) {
		if c.Hidden {
			return
		}
		// Build the full path for this command
		var fullPath string
		if prefix == "" {
			fullPath = c.Name()
		} else {
			fullPath = prefix + ":" + c.Name()
		}

		// Add this command to the list
		commands = append(commands, cmdInfo{
			path: fullPath,
			cmd:  c,
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
		return
	}

	// Find the maximum path length for alignment
	maxPathLen := 0
	for _, info := range commands {
		if len(info.path) > maxPathLen {
			maxPathLen = len(info.path)
		}
	}

	// Print all commands with aligned formatting similar to help
	for _, info := range commands {
		var sb strings.Builder

		// Format command name with color
		coloredPath := formatCommandName(info.path)
		_, _ = fmt.Fprintf(&sb, "%s%s\n",
			strings.Repeat(" ", 2), coloredPath,
		)

		// Print description below the command name
		if info.cmd.Short != "" {
			desc := indent(info.cmd.Short, 4)
			_, _ = sb.WriteString(desc)
		}

		// Print args if defined
		if len(info.cmd.Args) > 0 {
			if info.cmd.Short != "" {
				_, _ = sb.WriteString("\n")
			}
			argsIndent := strings.Repeat(" ", 4)
			for i, arg := range info.cmd.Args {
				argName := arg.Name
				if argName == "" {
					argName = fmt.Sprintf("arg%d", i+1)
				}

				argType := formatArgType(arg)
				_, _ = fmt.Fprintf(&sb, "%s%s %s", argsIndent, argName, argType)

				if arg.Default != "" || arg.Required {
					_, _ = sb.WriteString(" (")
					if arg.Default != "" {
						_, _ = fmt.Fprintf(&sb, "default: %s", arg.Default)
					}
					if arg.Default != "" && arg.Required {
						_, _ = sb.WriteString(", ")
					}
					if arg.Required {
						_, _ = sb.WriteString("required")
					}
					_, _ = sb.WriteString(")")
				}

				if arg.Description != "" {
					_, _ = sb.WriteString("\n")
					desc := indent(arg.Description, 6)
					_, _ = sb.WriteString(desc)
				} else {
					_, _ = sb.WriteString("\n")
				}
			}
		}

		fmt.Print(sb.String())
	}
}

// PrintFlags prints all flags for all commands, using help formatting style
func PrintFlags(rootCmd *Command) {
	// Get all root command options as global flags (not just predefined ones)
	var globalFlags OptionSet
	for _, opt := range rootCmd.Options {
		if opt.Flag != "" && !opt.Hidden {
			globalFlags = append(globalFlags, opt)
		}
	}

	// Collect all commands with their full paths
	type cmdInfo struct {
		cmd  *Command
		path string
	}
	var commands []cmdInfo

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
		commands = append(commands, cmdInfo{
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
		collectCommands(child, "")
	}

	// Print global flags
	if len(globalFlags) > 0 {
		fmt.Println(prettyHeader("Global Options"))
		for _, opt := range globalFlags {
			if opt.Flag == "" || opt.Hidden {
				continue
			}

			var sb strings.Builder
			shorthandColored, flagColored := formatFlagName(opt)
			if opt.Shorthand != "" {
				_, _ = fmt.Fprintf(&sb, "\n ")
				_, _ = sb.WriteString(shorthandColored)
				_, _ = sb.WriteString(", ")
				_, _ = sb.WriteString(flagColored)
			} else {
				_, _ = sb.WriteString("\n      ")
				_, _ = sb.WriteString(flagColored)
			}

			flagType := formatFlagType(opt)
			if flagType != "" {
				_, _ = fmt.Fprintf(&sb, " %s", flagType)
			}

			if len(opt.Envs) > 0 {
				envStr := formatFlagEnvNames(opt)
				_, _ = fmt.Fprintf(&sb, ", %s", envStr)
			}

			if opt.Default != "" || opt.Required {
				_, _ = sb.WriteString(" (")
				if opt.Default != "" {
					_, _ = fmt.Fprintf(&sb, "default: %s", opt.Default)
				}
				if opt.Default != "" && opt.Required {
					_, _ = sb.WriteString(", ")
				}
				if opt.Required {
					_, _ = sb.WriteString("required")
				}
				_, _ = sb.WriteString(")")
			}

			if opt.Description != "" {
				desc := indent(opt.Description, 10)
				_, _ = sb.WriteString("\n")
				_, _ = sb.WriteString(desc)
			}

			if opt.Deprecated != "" {
				deprecatedMsg := fmt.Sprintf("DEPRECATED: %s", opt.Deprecated)
				deprecatedIndented := indent(deprecatedMsg, 10)
				_, _ = sb.WriteString("\n")
				_, _ = sb.WriteString(deprecatedIndented)
			}

			fmt.Print(sb.String())
		}
		fmt.Println()
	}

	// Print flags for each command
	hasCommandFlags := false
	for _, info := range commands {
		if len(info.cmd.Options) == 0 {
			continue
		}

		// Filter out global flags from command options
		var commandSpecificFlags OptionSet
		for _, opt := range info.cmd.Options {
			isGlobal := false
			for _, globalOpt := range globalFlags {
				if opt.Flag == globalOpt.Flag {
					isGlobal = true
					break
				}
			}
			if !isGlobal && opt.Flag != "" && !opt.Hidden {
				commandSpecificFlags = append(commandSpecificFlags, opt)
			}
		}

		if len(commandSpecificFlags) > 0 {
			if !hasCommandFlags {
				fmt.Println(prettyHeader("Command-Specific Options"))
				hasCommandFlags = true
			}

			fmt.Printf("\n  %s\n", info.path)

			for _, opt := range commandSpecificFlags {
				var sb strings.Builder
				shorthandColored, flagColored := formatFlagName(opt)
				if opt.Shorthand != "" {
					_, _ = fmt.Fprintf(&sb, "    ")
					_, _ = sb.WriteString(shorthandColored)
					_, _ = sb.WriteString(", ")
					_, _ = sb.WriteString(flagColored)
				} else {
					_, _ = sb.WriteString("      ")
					_, _ = sb.WriteString(flagColored)
				}

				flagType := formatFlagType(opt)
				if flagType != "" {
					_, _ = fmt.Fprintf(&sb, " %s", flagType)
				}

				if len(opt.Envs) > 0 {
					envStr := formatFlagEnvNames(opt)
					_, _ = fmt.Fprintf(&sb, ", %s", envStr)
				}

				if opt.Default != "" || opt.Required {
					_, _ = sb.WriteString(" (")
					if opt.Default != "" {
						_, _ = fmt.Fprintf(&sb, "default: %s", opt.Default)
					}
					if opt.Default != "" && opt.Required {
						_, _ = sb.WriteString(", ")
					}
					if opt.Required {
						_, _ = sb.WriteString("required")
					}
					_, _ = sb.WriteString(")")
				}

				if opt.Description != "" {
					desc := indent(opt.Description, 10)
					_, _ = sb.WriteString("\n")
					_, _ = sb.WriteString(desc)
				}

				if opt.Deprecated != "" {
					deprecatedMsg := fmt.Sprintf("DEPRECATED: %s", opt.Deprecated)
					deprecatedIndented := indent(deprecatedMsg, 10)
					_, _ = sb.WriteString("\n")
					_, _ = sb.WriteString(deprecatedIndented)
				}

				fmt.Print(sb.String())
				fmt.Println()
			}
		}
	}

	if !hasCommandFlags && len(globalFlags) == 0 {
		fmt.Println("No flags available.")
	}
}

// DefaultHelpFn returns a function that generates usage (help)
// output for a given command.
func DefaultHelpFn() HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		// We use stdout for help and not stderr since there's no straightforward
		// way to distinguish between a user error and a help request.
		//
		// We buffer writes to stdout because the newlineLimiter writes one
		// rune at a time.
		outBuf := bufio.NewWriter(inv.Stdout)
		out := newlineLimiter{w: outBuf, limit: 2}
		newWriter := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
		err := defaultHelpTemplate.Execute(newWriter, inv.Command)
		if err != nil {
			return fmt.Errorf("execute template: %w", err)
		}
		err = newWriter.Flush()
		if err != nil {
			return err
		}
		err = outBuf.Flush()
		if err != nil {
			return err
		}
		if len(inv.Args) > 0 && !usageWantsArgRe.MatchString(inv.Command.Use) {
			_, _ = fmt.Fprintf(inv.Stderr, "---\nerror: unknown subcommand %q\n", inv.Args[0])
		}
		if len(inv.Args) > 0 {
			// Return an error so that exit status is non-zero when
			// a subcommand is not found.
			return &UnknownSubcommandError{Args: inv.Args}
		}
		return nil
	}
}
