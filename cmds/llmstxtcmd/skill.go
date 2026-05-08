package llmstxtcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pubgo/redant"
)

// WriteSkill writes the command tree as SKILL.md documents following the
// standard format: YAML frontmatter (---) + # title + ## 使用场景 + usage example.
// Each leaf command becomes a separate skill entry.
func WriteSkill(w io.Writer, root *redant.Command, maxDepth int) error {
	p := &printer{w: w}

	// Collect all leaf/handler commands
	var commands []*skillEntry
	collectSkillEntries(root, root.Name(), 0, maxDepth, &commands)

	if len(commands) == 0 {
		return nil
	}

	for i, entry := range commands {
		if err := validateSkillEntry(entry); err != nil {
			return err
		}
		if i > 0 {
			p.line("")
		}
		writeSkillEntry(p, entry)
	}

	return p.err
}

type skillEntry struct {
	name         string   // skill name, includes full command path in hyphen style (e.g. "平台-project-create")
	pathSegments []string // command path segments from root (e.g. ["平台", "project", "create"])
	fullPath     string   // space-joined command path (e.g. "平台 project create")
	description  string
	long         string
	aliases      []string
	args         []redant.Arg
	options      []redant.Option
	response     string            // e.g. "Unary StatusResult" or "Stream string"
	metadata     map[string]string // skill.* metadata from Command.Metadata
}

// skillMetadataPrefix is the prefix for Metadata keys recognised by the skill
// generator. Keys matching this prefix are stripped and mapped to SKILL.md
// frontmatter according to the Agent Skills specification:
//
//   - Standard top-level fields: name, description, license, compatibility,
//     allowed-tools — emitted directly.
//   - Everything else is placed in the nested "metadata:" YAML map.
//
// Reference: https://agentskills.io/specification
const skillMetadataPrefix = "skill."

// skillStandardFields lists frontmatter keys defined in the Agent Skills spec
// that may appear as top-level YAML fields. Any skill.* metadata key NOT in
// this set is written under the nested "metadata:" map.
var skillStandardFields = map[string]bool{
	"name":          true,
	"description":   true,
	"license":       true,
	"compatibility": true,
	"allowed-tools": true,
}

// skillNamePattern matches valid skill names per the Agent Skills specification:
// 1-64 chars, lowercase a-z / 0-9 / hyphens, no leading/trailing hyphen,
// no consecutive hyphens.
// Reference: https://agentskills.io/specification
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// SkillValidationError collects one or more validation problems for a skill entry.
type SkillValidationError struct {
	Name   string   // skill name (may be invalid itself)
	Errors []string // human-readable error descriptions
}

func (e *SkillValidationError) Error() string {
	return fmt.Sprintf("skill %q: %s", e.Name, strings.Join(e.Errors, "; "))
}

// validateSkillEntry checks a skillEntry against the Agent Skills specification.
// Returns nil when all fields are valid.
func validateSkillEntry(e *skillEntry) error {
	name := e.metadata["name"]
	if name == "" {
		name = e.name
	}

	desc := e.metadata["description"]
	if desc == "" {
		desc = e.description
	}

	var errs []string

	// --- name ---
	if name == "" {
		errs = append(errs, "name is required")
	} else {
		if n := utf8.RuneCountInString(name); n > 64 {
			errs = append(errs, fmt.Sprintf("name exceeds 64 characters (%d)", n))
		}
		if !skillNamePattern.MatchString(name) {
			errs = append(errs, fmt.Sprintf("name %q must contain only lowercase a-z, 0-9, hyphens; must not start/end with hyphen or contain consecutive hyphens", name))
		}
	}

	// --- description ---
	if desc == "" {
		errs = append(errs, "description is required")
	} else if n := utf8.RuneCountInString(desc); n > 1024 {
		errs = append(errs, fmt.Sprintf("description exceeds 1024 characters (%d)", n))
	}

	// --- compatibility (optional, max 500 chars) ---
	if v, ok := e.metadata["compatibility"]; ok && v != "" {
		if n := utf8.RuneCountInString(v); n > 500 {
			errs = append(errs, fmt.Sprintf("compatibility exceeds 500 characters (%d)", n))
		}
	}

	if len(errs) > 0 {
		return &SkillValidationError{Name: name, Errors: errs}
	}
	return nil
}

func normalizeSkillName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func skillNameFromPathSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	norm := make([]string, 0, len(segments))
	for _, seg := range segments {
		norm = append(norm, normalizeSkillName(seg))
	}
	return strings.Join(norm, "-")
}

func collectSkillEntries(cmd *redant.Command, path string, depth, maxDepth int, out *[]*skillEntry) {
	if cmd.Hidden {
		return
	}

	// Build path segments from the space-separated path
	segments := strings.Fields(path)

	// A command is a skill entry if it has a handler (leaf) or explicitly has
	// ResponseHandler/ResponseStreamHandler
	isLeaf := cmd.Handler != nil || cmd.ResponseHandler != nil || cmd.ResponseStreamHandler != nil

	if isLeaf {
		// Collect skill.* metadata
		skillMeta := make(map[string]string)
		for k, v := range cmd.Metadata {
			if strings.HasPrefix(k, skillMetadataPrefix) {
				skillMeta[strings.TrimPrefix(k, skillMetadataPrefix)] = v
			}
		}

		entry := &skillEntry{
			name:         skillNameFromPathSegments(segments),
			pathSegments: segments,
			fullPath:     path,
			description:  cmd.Short,
			long:         cmd.Long,
			aliases:      cmd.Aliases,
			metadata:     skillMeta,
		}

		// Collect non-hidden options (skip global flags)
		for _, opt := range cmd.Options {
			if opt.Flag != "" && !opt.Hidden {
				entry.options = append(entry.options, opt)
			}
		}

		// Collect args
		for _, arg := range cmd.Args {
			entry.args = append(entry.args, arg)
		}

		// Response type
		if cmd.ResponseHandler != nil {
			ti := cmd.ResponseHandler.TypeInfo()
			entry.response = fmt.Sprintf("Unary `%s`", ti.TypeName)
		} else if cmd.ResponseStreamHandler != nil {
			ti := cmd.ResponseStreamHandler.TypeInfo()
			entry.response = fmt.Sprintf("Stream `%s`", ti.TypeName)
		}

		*out = append(*out, entry)
	}

	// Recurse into children
	if maxDepth > 0 && depth >= maxDepth {
		return
	}
	for _, child := range cmd.Children {
		childPath := path + " " + child.Name()
		collectSkillEntries(child, childPath, depth+1, maxDepth, out)
	}
}

func writeSkillEntry(p *printer, e *skillEntry) {
	// --- resolve standard fields ---

	// name: metadata override > generated name
	name := e.metadata["name"]
	if name == "" {
		name = e.name
	}

	// description: metadata override > Short
	desc := e.metadata["description"]
	if desc == "" {
		desc = e.description
	}
	if desc == "" {
		desc = fmt.Sprintf("Run %s command.", name)
	}

	// --- YAML frontmatter (spec fields) ---
	p.line("---")
	p.line("name: %s", name)
	p.line("description: \"%s\"", escapeYAMLString(desc))

	// Optional spec top-level fields: license, compatibility, allowed-tools
	for _, key := range []string{"license", "compatibility", "allowed-tools"} {
		if v, ok := e.metadata[key]; ok && v != "" {
			p.line("%s: \"%s\"", key, escapeYAMLString(v))
		}
	}

	// Build nested metadata map from non-standard skill.* keys
	var metaKeys []string
	for k := range e.metadata {
		if skillStandardFields[k] {
			continue
		}
		metaKeys = append(metaKeys, k)
	}
	if len(metaKeys) > 0 {
		sort.Strings(metaKeys)
		p.line("metadata:")
		for _, k := range metaKeys {
			p.line("  %s: \"%s\"", k, escapeYAMLString(e.metadata[k]))
		}
	}
	p.line("---")
	p.line("")

	// # Title
	p.line("# %s", e.fullPath)
	p.line("")

	// ## 使用场景
	p.line("## 使用场景")
	p.line("")
	if e.long != "" && e.long != e.description {
		p.line("- %s", e.long)
	} else {
		p.line("- %s", desc)
	}
	if len(e.aliases) > 0 {
		p.line("- 别名: %s", strings.Join(e.aliases, ", "))
	}
	if e.response != "" {
		p.line("- 响应类型: %s", e.response)
	}
	p.line("")

	// ## 用法
	p.line("## 用法")
	p.line("")

	// Usage line
	usage := e.fullPath
	for _, arg := range e.args {
		if arg.Required {
			usage += " <" + arg.Name + ">"
		} else {
			usage += " [" + arg.Name + "]"
		}
	}
	p.line("```sh")
	p.line("%s", usage)
	p.line("```")
	p.line("")

	// Arguments
	if len(e.args) > 0 {
		p.line("### 参数")
		p.line("")
		for _, arg := range e.args {
			req := ""
			if arg.Required {
				req = "（必填）"
			}
			desc := arg.Description
			if arg.Default != "" {
				desc += fmt.Sprintf("（默认: %s）", arg.Default)
			}
			if desc == "" {
				desc = "位置参数"
			}
			p.line("- `%s`%s — %s", arg.Name, req, desc)
		}
		p.line("")
	}

	// Options
	if len(e.options) > 0 {
		p.line("### 选项")
		p.line("")
		for _, opt := range e.options {
			flag := "--" + opt.Flag
			if opt.Shorthand != "" {
				flag = "-" + opt.Shorthand + ", " + flag
			}
			typeStr := ""
			if opt.Value != nil {
				typeStr = opt.Value.Type()
			}
			if typeStr != "" && typeStr != "bool" {
				flag += " " + typeStr
			}

			desc := opt.Description
			var extras []string
			if opt.Default != "" {
				extras = append(extras, "默认: "+opt.Default)
			}
			if opt.Required {
				extras = append(extras, "必填")
			}
			if len(opt.Envs) > 0 {
				extras = append(extras, "环境变量: "+strings.Join(opt.Envs, ", "))
			}
			if len(extras) > 0 {
				desc += "（" + strings.Join(extras, "; ") + "）"
			}

			p.line("- `%s` — %s", flag, desc)
		}
		p.line("")
	}
}

func escapeYAMLString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// WriteSkillDir writes each leaf command as a separate SKILL.md file under dir.
// Directory structure follows generated skill names:
//
//	<dir>/<skill-name>/SKILL.md
//
// The SKILL.md name field uses full command path joined by hyphens.
// The directory name matches the name field per the Agent Skills specification.
func WriteSkillDir(dir string, root *redant.Command, maxDepth int) error {
	var commands []*skillEntry
	collectSkillEntries(root, root.Name(), 0, maxDepth, &commands)

	for _, entry := range commands {
		if err := validateSkillEntry(entry); err != nil {
			return err
		}

		// Resolve effective name (metadata override > generated)
		// The directory name must match the name field per the spec.
		effectiveName := entry.metadata["name"]
		if effectiveName == "" {
			effectiveName = entry.name
		}

		skillDir := filepath.Join(dir, effectiveName)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("creating skill dir %s: %w", skillDir, err)
		}

		var buf bytes.Buffer
		p := &printer{w: &buf}
		writeSkillEntry(p, entry)
		if p.err != nil {
			return fmt.Errorf("writing skill %s: %w", effectiveName, p.err)
		}

		path := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	return nil
}
