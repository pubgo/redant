# [Unreleased]

> 推荐维护方式：`/changelog draft|release`

## 新增

- `llms-txt --format skill`：支持生成符合 [Agent Skills 规范](https://agentskills.io/specification) 的 SKILL.md 文件，含 YAML frontmatter 校验（`name`、`description`、`compatibility` 字段约束）。
- `Command.Metadata` 的 `skill.*` 前缀约定：可通过 Metadata 配置 SKILL.md 的标准字段（`name`、`description`、`license`、`compatibility`、`allowed-tools`）及自定义 `metadata:` 嵌套属性。
- MCP 工具排除机制：通过 `Metadata["agent.exclude"] = "true"` 将命令及其子命令从 MCP tools/resources/prompts 中排除；内置基础设施命令（`mcp`、`completion`、`doc`、`llms-txt`、`readline`、`richline`、`web`、`webtty`）已默认标记排除。
- `llms-txt` 输出同步排除 `agent.exclude` 命令：Markdown、JSON、Skill 三种格式均过滤标记了 `agent.exclude` 的命令，与 MCP 行为一致。

## 修复

- MCP 工具调用重复响应：修复 `ResponseHandler` / `ResponseStreamHandler` 的返回值同时出现在 `stdout`/`combined` 和 `response` 字段的问题。结构化响应现在仅出现在 `response` 字段，`content.text` 则改用序列化后的响应 JSON，避免输出冗余。

## 变更

- 简化 `Option` 继承配置：移除 `NoInherit`，统一使用 `Inherit bool` 控制是否向子命令继承（默认不继承）。
- 将 `--list-commands`、`--list-flags`、`--list-format` 调整为 `Inherit=false`，仅保留根命令可见/可用，避免扩散到子命令。
- 移除冗余 `DefaultOptionInherit` 常量；继承控制统一由 `Inherit` 字段显式声明。
- 移除未使用的 `Option.Category` 字段；flag 继承行为统一由 `Inherit` 控制。

## 文档

暂无
