# Redant 项目指引

## 适用范围

- 本文件是仓库级 always-on 指引，适用于整个 `redant` 工作区。
- 本文件为唯一工作区指引入口。

## 代码审查模式入口

- 当任务是 PR 审查 / review comment 处理 / 审查结论整理时，优先进入审查模式并遵循：
	- `.github/instructions/pr-review.instructions.md`
	- `.github/instructions/pr-review-golang.instructions.md`
	- `.github/instructions/pr-review-javascript.instructions.md`
	- `.github/instructions/pr-review-shell.instructions.md`
- 审查模式下输出要求以审查规则文件为准（分类标签、证据链、Review Conclusion、评论模板与去重规则）。

## 技术栈与目标

- 语言：Go（见 `go.mod`，当前 `go 1.25`）。
- 项目类型：CLI 框架，核心能力包括命令树、标志解析、参数解析、中间件链、帮助系统。
- 在修改实现时，优先保持现有公开 API 与行为兼容，避免无关重构。

## 首选开发命令

- 查看任务：`task -a`
- 测试（默认推荐）：`task test`
- 静态检查：`task vet`
- Lint：`task lint`

如需绕过 task，可使用等价命令：

- `go test -short -race -v ./... -cover`
- `go vet ./...`
- `golangci-lint run --verbose ./...`

## 架构边界（修改前先定位）

### 核心模块

- `command.go`：命令分发与执行主流程（`Invocation.Run` → `inv.run`），含子命令解析、运行入口。
- `option.go`：`OptionSet` 到 flag 的映射；`FlagSet()` 先注册 flag，再按 `Envs` 做环境回退，最后由 CLI 输入覆盖。
- `args.go`：位置参数 / query / form / JSON 参数解析与全局标志（`GlobalFlags()`）。
- `flags.go`：自定义 `pflag.Value` 类型（Enum/Regexp/Struct/CSV 等）+ YAML/JSON 编解码。
- `handler.go`：`HandlerFunc` / `ResponseHandler` / `ResponseStreamHandler`，Unary 与 Stream 两类响应模型。
- `help.go` + `help.tpl`：模板驱动帮助渲染，终端宽度自适应、彩色样式。

### 扩展命令模块（`cmds/`）

- `completioncmd/`：Shell 补全命令集成（bash/zsh/fish）。
- `mcpcmd/`：MCP stdio server 集成。
- `readlinecmd/` / `richlinecmd/`：交互式 readline 命令驱动。
- `webcmd/` / `webttycmd/`：Web UI 与 WebTTY 远程终端能力。

### 内部包（`internal/`）

- `gitshell`：git 命令执行与工作区状态判断。
- `mcpserver`：命令树 → MCP tools 映射。
- `pretty`：内部化文本样式与格式化（替代外部 `coder/pretty`）。
- `webui`：Web UI 命令元数据、PTY 信号适配、静态资源嵌入。

## 关键运行规则（不要破坏）

- 子命令解析同时支持空格路径（`app repo commit`）与冒号路径（`app repo:commit`）——见 `getExecCommand`。
- 分发优先级：显式子命令 > `argv0` busybox 分发 > 根命令（`getExecCommand` + `resolveArgv0Command`）。
- 子命令继承父标志；同名时深层命令标志覆盖浅层（`copyFlagSetWithout`）。
- `--list-commands` / `--list-flags` 在 Handler 前短路执行。
- Required 选项判定认可三类来源：显式 flag、默认值、配置了 env 键列表。

## 项目特有约定

- 参数形态支持：位置参数、query（`&`）、form（空格分隔 `k=v`）、JSON。
- 测试优先使用表驱动与子测试，覆盖 flag 继承、argv0 分发、参数解析边界。
- Unary/Stream 响应输出采用 NDJSON envelope：`{"$":"resp|error","type":"...","data":...}`。

## 集成点与依赖

- CLI 标志引擎：`github.com/spf13/pflag`（自定义值类型见 `flags.go`）。
- 帮助输出格式：`github.com/muesli/termenv` + `github.com/mitchellh/go-wordwrap` + `internal/pretty`。
- MCP：`github.com/modelcontextprotocol/go-sdk`。
- YAML/JSON 值包装：`flags.go` + `gopkg.in/yaml.v3`。

## 变更落点清单

- 命令分发/执行 → 改 `command.go`，补 `command_test.go`。
- flag/env/default 语义 → 改 `option.go`，补对应测试。
- 参数格式行为 → 改 `args.go`，同步 `example/args-test/`。
- 帮助/补全体验 → 改 `help.go`/`help.tpl` 或 `cmds/completioncmd/`。
- 值类型 → 改 `flags.go`，补 `flags_test.go`。

## 文档与变更同步

- 文档入口：`docs/INDEX.md`（目录索引见 `docs/DOCS_CATALOG.md`）。
- 涉及架构或流程变化时，先更新 `docs/DESIGN.md`，再补示例/说明文档。
- 行为变更需同步 `.version/changelog/Unreleased.md`，必要时更新 `docs/EVALUATION.md`。
- 文档默认使用中文，流程图优先 Mermaid。

## 实施原则（对 AI 代理）

- 仅做与任务直接相关的最小改动，避免顺手大改。
- 保持现有命名风格与文件组织，不随意移动目录结构。
- 涉及依赖迁移到 `internal/` 时：先提供最小兼容 API，再替换引用，最后执行 `go mod tidy` 与 `go test ./...` 验证。
- 修改 CLI 输出样式相关代码时，优先保持 API 兼容，不改变业务语义。

## 常见坑点

- `task test` 含 `-race` 与覆盖率，执行耗时可能较高。
- `Taskfile.yml` 声明了 `dotenv: [".env"]`；若任务依赖环境变量，请显式在 `.env` 中补齐占位键。
- changelog 维护采用 agent/LLM 流程，优先使用 `/changelog-maintenance draft|release`。

## 参考文件

- `README.md`、`Taskfile.yml`
- `docs/DESIGN.md`、`docs/USAGE_AT_A_GLANCE.md`、`docs/CHANGELOG_LLM_PROMPT.md`
- 核心源码：`command.go`、`option.go`、`args.go`、`flags.go`、`handler.go`