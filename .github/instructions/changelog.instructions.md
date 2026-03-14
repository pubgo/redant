---
name: Changelog 专项规范
description: 仅用于维护 docs/CHANGELOG.md，保证 Unreleased 与版本落版结构稳定、分类一致、条目可追溯
applyTo: "docs/CHANGELOG.md"
---

# Redant Changelog 维护规范

本规则仅适用于 `docs/CHANGELOG.md`。

## 结构约束

- 保持顶层结构稳定：`[Unreleased]` 在前，历史版本按既有顺序保留。
- `Unreleased` 推荐分类：`新增` / `修复` / `变更` / `文档`。
- 若某分类暂无内容，写“暂无”。

## 内容约束

- 仅基于可见改动编写条目，不杜撰能力或影响。
- 单条应简洁、可读、可追溯，尽量以动词开头。
- 重复事项需合并去重，避免同义重复。
- 不改写历史版本块语义，不重排已发布版本。

## 落版约束（release）

- 版本号来源于 `.version/VERSION`。
- 落版格式：`## [<VERSION>] - <YYYY-MM-DD>`。
- 落版后需在顶部重建新的 `[Unreleased]` 模板（四个分类）。

## 协同建议

- 优先参考：`docs/CHANGELOG_LLM_PROMPT.md`。
- 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`。
