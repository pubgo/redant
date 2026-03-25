---
name: Go 编码开发约束
description: "Use when modifying Go source code, command dispatch, flags/options parsing, args parsing, env preload, middleware chain, help rendering, completion integration, or related tests in redant."
applyTo: "**/*.go"
---

# Redant 编码开发规则（Go）

仅在修改 Go 代码时生效，目标是保证 CLI 行为与公开 API 兼容，避免无关重构。

## 兼容性与改动边界

- 默认做最小改动：仅修改与当前任务直接相关的代码路径。
- 不随意变更公开 API、命令语义、输出语义；若必须变更，需同步测试与文档说明。
- 避免顺手重命名、移动目录、跨模块重构（除非任务明确要求）。

## 关键语义保护（不得破坏）

- 子命令解析同时支持空格路径与冒号路径。
- 分发优先级保持：显式子命令 > `argv0` 分发 > 根命令。
- 子命令继承父标志；重名时深层标志覆盖浅层标志。
- `--list-commands` 与 `--list-flags` 在 Handler 前短路。
- Required 选项校验保持现有判定来源（显式 flag、默认值、env 列表）。

## 文件落点约定

- 命令分发/执行流程改动：优先落在 `command.go`，并补 `command_test.go`。
- flag/env/default 语义改动：优先改 `option.go` / `env_preload.go`，并补对应测试。
- 参数格式与解析改动：优先改 `args.go`，并验证示例或测试覆盖。
- 帮助与补全体验改动：改 `help.go`/`help.tpl` 或 `cmds/completioncmd/`，并验证输出。

## 测试与质量门槛

- 新增或修改行为时，优先补表驱动测试与子测试。
- 测试优先覆盖边界语义，不只覆盖 happy path。
- 变更后至少执行相关测试；条件允许时执行完整回归（`task test`）。
- 保持现有代码风格与命名习惯，不引入不必要的格式漂移。

## 文档与变更同步

- 行为变更需同步 `.version/changelog/Unreleased.md`。
- 涉及架构/流程变化时，先更新 `docs/DESIGN.md`，再补其他文档。

## 禁止项

- 不杜撰未实现能力或测试结果。
- 不为“看起来更整洁”而改变既有行为。
- 不以临时绕过方式（跳过测试、删除校验）替代根因修复。
