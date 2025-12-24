package redant

import (
	"bufio"
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
				// Root command: filter to only global flags
				globalFlags := c.GetGlobalFlags()
				opts = globalFlags
			} else {
				// Non-root command: filter out global flags
				globalFlags := c.GetGlobalFlags()
				globalFlagMap := make(map[string]bool)
				for _, gf := range globalFlags {
					globalFlagMap[gf.Flag] = true
				}
				for _, opt := range c.Options {
					if !globalFlagMap[opt.Flag] {
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

// DefaultHelpFn returns a function that generates usage (help)
// output for a given command.
func DefaultHelpFn() HandlerFunc {
	return func(inv *Invocation) error {
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
