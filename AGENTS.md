# AGENTS.md

## 适用范围
- 本指南面向在 `redant` 仓库内工作的 AI 编码代理。
- 优先保证行为与 API 兼容性，避免无关重构；该项目是 CLI 框架核心。

## 架构速览
- 命令运行主线在 `command.go`（`Invocation.Run` -> `inv.run`）：命令定位、标志装配、参数解析、中间件链与处理器执行。
- 标志模型在 `option.go`：`OptionSet.FlagSet()` 先应用默认值，再按 `Envs` 首个非空值做环境回退，最后由 CLI 输入覆盖。
- 参数形态在 `args.go`：位置参数、query（`a=1&b=2`）、form（`a=1 b=2`）、JSON 对象/数组。
- 帮助渲染由 `help.go` + `help.tpl` 模板驱动，样式层位于 `internal/pretty`。
- Shell 补全作为命令模块集成在 `cmds/completioncmd/completion.go`。

## 关键运行规则（不要破坏）
- 子命令解析同时支持 `app repo commit` 与 `app repo:commit`（`command.go` 的 `getExecCommand`）。
- 分发优先级：显式子命令 > `argv0` busybox 分发 > 根命令（见 `getExecCommand` + `resolveArgv0Command`）。
- 根全局标志来自 `args.go` 的 `GlobalFlags()`，在命令初始化时注入。
- 子命令继承父标志；出现重名时，深层命令标志覆盖浅层标志（`command.go` 的 `copyFlagSetWithout` 逻辑）。
- `--list-commands` / `--list-flags` 会在 Handler 前短路执行（`command.go`）。
- 环境预加载（`--env`、`-e`、`--env-file`）先从原始参数读取，再在运行结束后恢复（`env_preload.go`）。
- Required 选项判定认可三类来源：显式改动 flag、默认值、配置了 env 键列表（`command.go` 必填校验逻辑）。

## 开发工作流
- 任务入口（`taskfile.yml`）：
  - `task test`（内部使用 `go test -short -race -v ./... -cover`）
  - `task vet`
  - `task lint`
- `Taskfile` 会加载 `.env`（`dotenv: [".env"]`），测试行为可能受环境占位键影响。

## 项目约定（来自现有代码）
- 测试以表驱动 + 子测试为主（见 `command_test.go`、`env_preload_test.go`）。
- 补测试优先覆盖边界语义，而非只测 happy path：argv0 分发、flag 继承、env-file 的 CSV/重复输入、`--` 停止符。
- 文档默认中文；涉及流程文档变更时保留 Mermaid 图（`README.md`、`docs/DESIGN.md`、`docs/USAGE_AT_A_GLANCE.md`）。

## 集成点与依赖
- CLI 标志引擎：`github.com/spf13/pflag`（自定义值类型见 `flags.go`）。
- 帮助输出格式：`github.com/muesli/termenv`、`github.com/mitchellh/go-wordwrap`、`internal/pretty`。
- YAML/JSON 值包装能力在 `flags.go` 中实现，用于类型化选项。

## 变更落点清单
- 调整命令分发/执行：改 `command.go`，并在 `command_test.go` 增加针对性测试。
- 调整 flag/env/default 语义：改 `option.go` / `env_preload.go`，并更新 `env_preload_test.go`。
- 调整参数格式行为：改 `args.go`，并同步 `example/args-test/` 示例。
- 调整帮助/补全体验：改 `help.go`/`help.tpl` 或 `cmds/completioncmd/`，并验证相关输出路径。

