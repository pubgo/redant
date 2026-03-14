# Redant 项目指引

## 适用范围

- 本文件是仓库级 always-on 指引，适用于整个 `redant` 工作区。
- 不再额外创建 `AGENTS.md`，避免两套同类指引并存。

## 技术栈与目标

- 语言：Go（见 `go.mod`，当前 `go 1.23`）。
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

- `command.go`：命令分发与执行主流程（含子命令解析、运行入口）。
- `option.go`：`OptionSet` 到 flag 的映射，默认值与环境变量回退逻辑。
- `args.go`：位置参数 / query / form / JSON 参数解析与全局标志。
- `help.go` + `help.tpl`：帮助信息渲染与模板。
- `cmds/completioncmd/`：补全命令集成。

## 项目特有约定

- 子命令调用支持两种形式：空格路径（`a b c`）与冒号路径（`a b:c`）。
- 参数形态支持：位置参数、query（`&`）、form（空格分隔 `k=v`）、JSON。
- 解析优先级：显式子命令 > `argv0` 分发 > 根命令 > 标志与参数解析。
- 测试优先使用表驱动与子测试，覆盖 flag 继承、argv0 分发、参数解析边界。

## 文档与变更同步

- 文档入口：`docs/INDEX.md`。
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

- `README.md`
- `Taskfile.yml`
- `docs/USAGE_AT_A_GLANCE.md`
- `docs/DESIGN.md`
- `command.go`
- `option.go`
- `args.go`
- `docs/CHANGELOG_LLM_PROMPT.md`