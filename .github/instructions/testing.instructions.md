---
name: Go 测试开发约束
description: "Use when adding or modifying Go tests in redant, especially table-driven tests, subtests, command dispatch behavior, flags/options parsing, args parsing, env preload, and completion/help behavior."
applyTo: "**/*_test.go"
---

# Redant 测试开发规则（Go）

仅在修改 Go 测试文件时生效，目标是让测试覆盖真实行为边界并具备可维护性。

## 测试设计

- 优先使用表驱动测试，并通过子测试（`t.Run`）表达场景。
- 每个用例名称应描述“输入/前置条件 + 预期行为”。
- 新增行为必须新增测试；行为变更必须更新旧测试并解释预期变化。

## 覆盖重点（按项目语义）

- 命令分发：显式子命令、`argv0` 分发、根命令回退。
- 子命令路径：空格路径与冒号路径。
- 标志行为：继承、重名覆盖、默认值/env 回退、required 判定。
- 参数解析：位置参数、query、form、JSON 及边界输入。
- 特殊开关：`--list-commands` / `--list-flags` 的短路行为。

## 质量约束

- 避免只测 happy path，至少补一个失败或边界场景。
- 断言应稳定、聚焦语义，不依赖脆弱格式细节（除非格式就是需求）。
- 保持测试可重复执行，不依赖外部不稳定状态。

## 变更联动

- 如果测试变更源自行为变更，同步更新 `.version/changelog/Unreleased.md`。
- 如涉及架构/流程语义，联动更新 `docs/DESIGN.md`。
