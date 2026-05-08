package llmstxtcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pubgo/redant"
)

func TestWriteLLMSTxt_Golden(t *testing.T) {
	tests := []struct {
		name     string
		golden   string
		maxDepth int
		root     func() *redant.Command
	}{
		{
			name:   "basic_command_tree",
			golden: "basic_command_tree.golden",
			root:   newBasicRoot,
		},
		{
			name:     "depth_limit",
			golden:   "depth_limit.golden",
			maxDepth: 1,
			root:     newNestedRoot,
		},
		{
			name:   "hidden_excluded",
			golden: "hidden_excluded.golden",
			root:   newHiddenRoot,
		},
		{
			name:   "global_options",
			golden: "global_options.golden",
			root:   newGlobalOptsRoot,
		},
		{
			name:   "response_types",
			golden: "response_types.golden",
			root:   newResponseTypesRoot,
		},
		{
			name:   "comprehensive",
			golden: "comprehensive.golden",
			root:   newComprehensiveRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()

			var buf bytes.Buffer
			if err := WriteLLMSTxt(&buf, root, tt.maxDepth); err != nil {
				t.Fatalf("WriteLLMSTxt error: %v", err)
			}

			got := buf.String()
			wantPath := filepath.Join("testdata", tt.golden)

			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
					t.Fatalf("update golden %s: %v", wantPath, err)
				}
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", wantPath, err)
			}

			if got != string(want) {
				t.Fatalf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tt.golden, got, string(want))
			}
		})
	}
}

func TestNewCommand_Integration(t *testing.T) {
	root := &redant.Command{Use: "app", Short: "Test app."}
	root.Children = append(root.Children, &redant.Command{
		Use:     "hello",
		Short:   "Say hello.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, New())

	var buf bytes.Buffer
	inv := root.Invoke("llms-txt")
	inv.Stdout = &buf
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("run llms-txt: %v", err)
	}

	got := buf.String()
	wantPath := filepath.Join("testdata", "integration.golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden %s: %v", wantPath, err)
		}
	}

	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", wantPath, err)
	}

	if got != string(want) {
		t.Fatalf("output mismatch for integration\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

// --- test fixtures ---

func newBasicRoot() *redant.Command {
	var msg string
	var upper bool

	root := &redant.Command{
		Use:   "myapp",
		Short: "My sample application.",
	}
	root.Children = append(root.Children, &redant.Command{
		Use:     "echo [message]",
		Short:   "Prints a message.",
		Aliases: []string{"ec"},
		Options: redant.OptionSet{
			{Flag: "upper", Shorthand: "u", Description: "Uppercase output.", Value: redant.BoolOf(&upper)},
		},
		Args: redant.ArgSet{
			{Name: "message", Description: "Text to echo.", Required: true, Value: redant.StringOf(&msg)},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	return root
}

func newNestedRoot() *redant.Command {
	root := &redant.Command{Use: "app"}
	level1 := &redant.Command{
		Use:     "l1",
		Short:   "Level 1.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	level2 := &redant.Command{
		Use:     "l2",
		Short:   "Level 2.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}
	level1.Children = append(level1.Children, level2)
	root.Children = append(root.Children, level1)
	return root
}

func newHiddenRoot() *redant.Command {
	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children,
		&redant.Command{
			Use:     "visible",
			Short:   "Visible cmd.",
			Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
		},
		&redant.Command{
			Use:     "secret",
			Short:   "Secret cmd.",
			Hidden:  true,
			Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
		},
	)
	return root
}

func newGlobalOptsRoot() *redant.Command {
	root := &redant.Command{
		Use:   "app",
		Short: "App with globals.",
		Options: redant.OptionSet{
			{Flag: "verbose", Shorthand: "v", Description: "Enable verbose.", Value: redant.BoolOf(new(bool))},
			{Flag: "config", Description: "Config file path.", Value: redant.StringOf(new(string)), Envs: []string{"APP_CONFIG"}},
		},
	}
	root.Children = append(root.Children, &redant.Command{
		Use:     "run",
		Short:   "Run something.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	return root
}

func newResponseTypesRoot() *redant.Command {
	type StatusResult struct {
		OK bool `json:"ok"`
	}

	root := &redant.Command{Use: "app"}
	root.Children = append(root.Children,
		&redant.Command{
			Use:   "status",
			Short: "Get status.",
			ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (StatusResult, error) {
				return StatusResult{OK: true}, nil
			}),
		},
		&redant.Command{
			Use:   "logs",
			Short: "Stream logs.",
			ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[string]) error {
				return out.Send("line")
			}),
		},
	)
	return root
}

func newComprehensiveRoot() *redant.Command {
	var (
		output   string
		force    bool
		count    int64
		endpoint string
	)

	root := &redant.Command{
		Use:   "myctl",
		Short: "My control tool.",
		Long:  "A comprehensive CLI tool for managing resources.",
		Options: redant.OptionSet{
			{Flag: "output", Shorthand: "o", Description: "Output format.", Value: redant.EnumOf(&output, "text", "json", "yaml"), Default: "text"},
		},
	}

	deployCmd := &redant.Command{
		Use:   "deploy [target]",
		Short: "Deploy to environment.",
		Long:  "Deploy the application to the specified target environment.",
		Options: redant.OptionSet{
			{Flag: "force", Shorthand: "f", Description: "Force deploy.", Value: redant.BoolOf(&force)},
			{Flag: "count", Description: "Instance count.", Value: redant.Int64Of(&count), Default: "1", Required: true},
			{Flag: "endpoint", Description: "Deploy endpoint.", Value: redant.StringOf(&endpoint), Envs: []string{"DEPLOY_ENDPOINT"}, Default: "https://deploy.example.com"},
		},
		Args: redant.ArgSet{
			{Name: "target", Description: "Target environment.", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	rollbackCmd := &redant.Command{
		Use:        "rollback",
		Short:      "Rollback deployment.",
		Deprecated: "Use 'deploy --rollback' instead.",
		Handler:    func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	deployCmd.Children = append(deployCmd.Children, rollbackCmd)
	root.Children = append(root.Children, deployCmd)
	return root
}

func TestWriteJSON_Golden(t *testing.T) {
	tests := []struct {
		name     string
		golden   string
		maxDepth int
		root     func() *redant.Command
	}{
		{"basic_json", "basic_json.golden", 0, newBasicRoot},
		{"response_types_json", "response_types_json.golden", 0, newResponseTypesRoot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()

			var buf bytes.Buffer
			if err := WriteJSON(&buf, root, tt.maxDepth); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}

			got := buf.String()

			// validate it's well-formed JSON
			var parsed jsonCommand
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("WriteJSON output is not valid JSON: %v\noutput:\n%s", err, got)
			}

			wantPath := filepath.Join("testdata", tt.golden)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(wantPath, []byte(got), 0o644); err != nil {
					t.Fatalf("update golden %s: %v", wantPath, err)
				}
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", wantPath, err)
			}
			if got != string(want) {
				t.Fatalf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tt.golden, got, string(want))
			}
		})
	}
}

func TestNewCommand_IntegrationJSON(t *testing.T) {
	root := &redant.Command{Use: "app", Short: "Test app."}
	root.Children = append(root.Children, &redant.Command{
		Use:     "hello",
		Short:   "Say hello.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, New())

	var buf bytes.Buffer
	inv := root.Invoke("llms-txt", "--format", "json")
	inv.Stdout = &buf
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("run llms-txt --format json: %v", err)
	}

	var doc jsonCommand
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if doc.Name != "app" {
		t.Fatalf("root name = %q, want %q", doc.Name, "app")
	}
	if len(doc.Children) != 2 {
		t.Fatalf("children count = %d, want 2", len(doc.Children))
	}
}

func TestWriteSkill_Golden(t *testing.T) {
	tests := []struct {
		name string
		root func() *redant.Command
	}{
		{"basic", newBasicRoot},
		{"comprehensive", newComprehensiveRoot},
		{"response_types", newResponseTypesRoot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()
			dir := t.TempDir()

			if err := WriteSkillDir(dir, root, 0); err != nil {
				t.Fatalf("WriteSkillDir error: %v", err)
			}

			// Walk all generated SKILL.md files, compare with golden dir
			goldenBase := filepath.Join("testdata", "skills", tt.name)
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || info.Name() != "SKILL.md" {
					return nil
				}
				rel, _ := filepath.Rel(dir, path)
				goldenPath := filepath.Join(goldenBase, rel)

				data, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatalf("read %s: %v", rel, readErr)
				}
				got := string(data)

				if os.Getenv("UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
						t.Fatalf("write golden %s: %v", goldenPath, err)
					}
				}

				want, readErr := os.ReadFile(goldenPath)
				if readErr != nil {
					t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", goldenPath, readErr)
				}
				if got != string(want) {
					t.Errorf("mismatch %s\n--- got ---\n%s\n--- want ---\n%s", rel, got, string(want))
				}
				return nil
			})
		})
	}
}

func TestNewCommand_IntegrationSkill(t *testing.T) {
	root := &redant.Command{Use: "app", Short: "Test app."}
	root.Children = append(root.Children, &redant.Command{
		Use:     "hello",
		Short:   "Say hello.",
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	})
	root.Children = append(root.Children, New())

	var buf bytes.Buffer
	inv := root.Invoke("llms-txt", "--format", "skill")
	inv.Stdout = &buf
	inv.Stderr = &bytes.Buffer{}

	if err := inv.Run(); err != nil {
		t.Fatalf("run llms-txt --format skill: %v", err)
	}

	got := buf.String()
	// Verify it contains skill frontmatter markers
	if !strings.Contains(got, "name: app-hello") {
		t.Fatalf("skill output missing 'name: app-hello':\n%s", got)
	}
	if !strings.Contains(got, "## 用法") {
		t.Fatalf("skill output missing '## 用法':\n%s", got)
	}
}

func TestWriteSkillDir(t *testing.T) {
	root := newComprehensiveRoot()

	dir := t.TempDir()
	if err := WriteSkillDir(dir, root, 0); err != nil {
		t.Fatalf("WriteSkillDir error: %v", err)
	}

	// Should create flattened dirs by generated skill name
	expected := []struct {
		path    string
		nameVal string
	}{
		{filepath.Join("myctl-deploy", "SKILL.md"), "name: myctl-deploy"},
		{filepath.Join("myctl-deploy-rollback", "SKILL.md"), "name: myctl-deploy-rollback"},
	}
	for _, tt := range expected {
		path := filepath.Join(dir, tt.path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", tt.path, err)
		}
		content := string(data)
		if !strings.Contains(content, "---") {
			t.Fatalf("%s missing YAML frontmatter:\n%s", tt.path, content)
		}
		if !strings.Contains(content, tt.nameVal) {
			t.Fatalf("%s missing %q:\n%s", tt.path, tt.nameVal, content)
		}
	}
}

func TestWriteSkillDir_Complex(t *testing.T) {
	root := newComplexSkillRoot()

	dir := t.TempDir()
	if err := WriteSkillDir(dir, root, 0); err != nil {
		t.Fatalf("WriteSkillDir error: %v", err)
	}

	// Verify expected skill files were created (flattened name-based structure)
	expectedSkills := []struct {
		relPath      string // relative path from dir to SKILL.md
		expectedName string // name field in frontmatter
		mustHave     []string
		mustNotHave  []string
	}{
		{
			relPath:      filepath.Join("平台-search", "SKILL.md"),
			expectedName: "平台-search",
			mustHave: []string{
				"name: 平台-search",
				"description: \"Search platform resources by keyword with scope filtering.\"",
				"argument-hint: \"搜索关键词，如 deployment nginx\"",
				"applyTo:",
				"condition:",
				"`query`（必填）",
				"`scope`",
				"-n, --limit",
				"环境变量: SEARCH_ENDPOINT",
				"Unary",
				"## 使用场景",
				"## 用法",
				"平台 search <query> [scope]",
				"别名: s, find",
			},
		},
		{
			relPath:      filepath.Join("平台-watch", "SKILL.md"),
			expectedName: "平台-watch",
			mustHave: []string{
				"name: 平台-watch",
				"Stream",
				"## 用法",
			},
		},
		{
			relPath:      filepath.Join("平台-project-create", "SKILL.md"),
			expectedName: "平台-project-create",
			mustHave: []string{
				"name: 平台-project-create",
				"`name`（必填）",
				"`description`",
				"--private",
				"--template",
				"平台 project create <name> [description]",
				"tools: \"create_file, run_in_terminal\"",
			},
		},
		{
			relPath:      filepath.Join("平台-project-delete", "SKILL.md"),
			expectedName: "平台-project-delete",
			mustHave: []string{
				"name: 平台-project-delete",
				"`project-id`（必填）",
				"--force",
				"--dry-run",
			},
		},
		{
			relPath:      filepath.Join("平台-project-import", "SKILL.md"),
			expectedName: "平台-project-import",
			mustHave: []string{
				"name: 平台-project-import",
				"别名: im",
			},
		},
	}

	for _, tt := range expectedSkills {
		t.Run(tt.expectedName, func(t *testing.T) {
			path := filepath.Join(dir, tt.relPath)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("expected %s to exist: %v", tt.relPath, err)
			}
			content := string(data)

			for _, s := range tt.mustHave {
				if !strings.Contains(content, s) {
					t.Errorf("skill %s missing %q in:\n%s", tt.expectedName, s, content)
				}
			}
			for _, s := range tt.mustNotHave {
				if strings.Contains(content, s) {
					t.Errorf("skill %s should not contain %q", tt.expectedName, s)
				}
			}
		})
	}

	// hidden command should NOT produce a skill file
	hiddenPath := filepath.Join(dir, "平台-internal-gc", "SKILL.md")
	if _, err := os.Stat(hiddenPath); err == nil {
		t.Fatal("hidden command 'internal-gc' should not have a skill file")
	}

	// Verify each generated SKILL.md matches golden files in nested dir structure
	goldenBase := filepath.Join("testdata", "skills", "complex")
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		goldenPath := filepath.Join(goldenBase, rel)

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", rel, readErr)
		}
		got := string(data)

		t.Run("golden/"+rel, func(t *testing.T) {
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
			}

			want, readErr := os.ReadFile(goldenPath)
			if readErr != nil {
				t.Fatalf("read golden %s: %v\nHint: run with UPDATE_GOLDEN=1 to create", goldenPath, readErr)
			}
			if got != string(want) {
				t.Fatalf("mismatch %s\n--- got ---\n%s\n--- want ---\n%s", rel, got, string(want))
			}
		})
		return nil
	})
}

// newComplexSkillRoot creates a complex command tree for thorough skill testing:
// - Multiple nested levels
// - Multiple args (required + optional)
// - Mixed response types (Unary, Stream, plain Handler)
// - Aliases, env vars, deprecated commands, hidden commands
// - Various value types (enum, bool, int64, string, string-array)
func newComplexSkillRoot() *redant.Command {
	type SearchResult struct {
		ID    string  `json:"id"`
		Title string  `json:"title"`
		Score float64 `json:"score"`
	}

	type WatchEvent struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}

	var (
		limit    int64
		endpoint string
		format   string
		tags     []string
		private  bool
		template string
		force    bool
		dryRun   bool
	)

	root := &redant.Command{
		Use:   "平台",
		Short: "统一资源管理平台。",
		Long:  "支持搜索、监控、项目管理等多种操作的 CLI 工具。",
		Options: redant.OptionSet{
			{Flag: "verbose", Shorthand: "v", Description: "启用详细日志。", Value: redant.BoolOf(new(bool))},
			{Flag: "config", Description: "配置文件路径。", Value: redant.StringOf(new(string)), Envs: []string{"PLATFORM_CONFIG"}},
		},
	}

	// Leaf command with Unary response + multiple args + aliases + env var + metadata
	searchCmd := &redant.Command{
		Use:     "search <query> [scope]",
		Short:   "搜索资源。",
		Long:    "在平台中全文搜索资源，支持按范围过滤和结果限制。",
		Aliases: []string{"s", "find"},
		Metadata: map[string]string{
			"skill.description":   "Search platform resources by keyword with scope filtering.",
			"skill.argument-hint": "搜索关键词，如 deployment nginx",
			"skill.applyTo":       "**/*.go",
			"skill.condition":     "当用户需要在平台中搜索资源时使用",
		},
		Options: redant.OptionSet{
			{Flag: "limit", Shorthand: "n", Description: "最大返回条数。", Default: "20", Value: redant.Int64Of(&limit)},
			{Flag: "endpoint", Description: "搜索服务地址。", Value: redant.StringOf(&endpoint), Envs: []string{"SEARCH_ENDPOINT"}, Default: "https://search.example.com"},
			{Flag: "format", Shorthand: "f", Description: "输出格式。", Value: redant.EnumOf(&format, "table", "json", "csv"), Default: "table"},
		},
		Args: redant.ArgSet{
			{Name: "query", Description: "搜索关键词。", Required: true, Value: redant.StringOf(new(string))},
			{Name: "scope", Description: "搜索范围（project/org/global）。", Default: "project", Value: redant.StringOf(new(string))},
		},
		ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) ([]SearchResult, error) {
			return []SearchResult{{ID: "1", Title: "test", Score: 0.95}}, nil
		}),
	}

	// Leaf command with Stream response
	watchCmd := &redant.Command{
		Use:   "watch",
		Short: "监听资源变更事件流。",
		Long:  "通过 SSE 或 WebSocket 连接实时监听资源变更。",
		Options: redant.OptionSet{
			{Flag: "filter", Description: "事件类型过滤。", Value: redant.StringOf(new(string))},
		},
		ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[WatchEvent]) error {
			return out.Send(WatchEvent{Type: "update", Payload: "test"})
		}),
	}

	// Nested commands: project > create / delete / import
	projectCmd := &redant.Command{
		Use:   "project",
		Short: "项目管理。",
	}

	createCmd := &redant.Command{
		Use:   "create <name> [description]",
		Short: "创建新项目。",
		Long:  "创建一个新项目并初始化默认配置。支持指定模板和标签。",
		Metadata: map[string]string{
			"skill.tools": "create_file, run_in_terminal",
		},
		Options: redant.OptionSet{
			{Flag: "private", Description: "设为私有项目。", Value: redant.BoolOf(&private)},
			{Flag: "template", Description: "项目模板名称。", Value: redant.StringOf(&template), Default: "default"},
			{Flag: "tags", Description: "项目标签（可多次指定）。", Value: redant.StringArrayOf(&tags)},
		},
		Args: redant.ArgSet{
			{Name: "name", Description: "项目名称。", Required: true, Value: redant.StringOf(new(string))},
			{Name: "description", Description: "项目描述。", Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	deleteCmd := &redant.Command{
		Use:   "delete <project-id>",
		Short: "删除项目。",
		Long:  "永久删除指定项目及其所有资源。此操作不可逆。",
		Options: redant.OptionSet{
			{Flag: "force", Shorthand: "f", Description: "跳过确认提示。", Value: redant.BoolOf(&force)},
			{Flag: "dry-run", Description: "仅模拟删除，不实际执行。", Value: redant.BoolOf(&dryRun)},
		},
		Args: redant.ArgSet{
			{Name: "project-id", Description: "要删除的项目 ID。", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	importCmd := &redant.Command{
		Use:     "import <url>",
		Short:   "从 URL 导入项目。",
		Aliases: []string{"im"},
		Args: redant.ArgSet{
			{Name: "url", Description: "Git 仓库 URL。", Required: true, Value: redant.StringOf(new(string))},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	// Hidden command — should NOT appear in skill output
	hiddenCmd := &redant.Command{
		Use:     "internal-gc",
		Short:   "Internal garbage collection.",
		Hidden:  true,
		Handler: func(ctx context.Context, inv *redant.Invocation) error { return nil },
	}

	projectCmd.Children = append(projectCmd.Children, createCmd, deleteCmd, importCmd)
	root.Children = append(root.Children, searchCmd, watchCmd, projectCmd, hiddenCmd)
	return root
}
