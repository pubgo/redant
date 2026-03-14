# Changelog LLM 提示词模板

本文档提供“修改功能后自动生成 changelog”的提示词模板，适用于 Copilot/ChatGPT 等 LLM。

## 使用目标

- 基于当前代码改动自动更新 `docs/CHANGELOG.md` 的 `Unreleased` 区域。
- 发布前将 `Unreleased` 落版为 `.version/VERSION` 对应版本。

## 模板 A：开发阶段（自动更新 Unreleased）

将以下提示词完整复制给 LLM：

```text
你是本仓库的 Changelog 维护助手。请根据当前工作区改动，自动更新 docs/CHANGELOG.md 的 [Unreleased] 区域。

请严格执行：
1) 读取并理解以下文件：
   - .version/VERSION
   - docs/CHANGELOG.md
   - 本次改动涉及的文件 diff（若可用）
2) 仅更新 docs/CHANGELOG.md 的 [Unreleased] 区域，不修改已发布版本历史。
3) 将变更归类到以下小节（不存在则创建）：
   - 新增
   - 修复
   - 变更
   - 文档
4) 归类规则：
   - feat/新增能力 -> 新增
   - fix/bug 修正 -> 修复
   - 重构、依赖迁移、行为调整 -> 变更
   - README、docs、注释更新 -> 文档
5) 去重并合并同类项，语言使用中文技术文风，单条以动词开头，避免口语化。
6) 忽略噪音提交（例如仅时间戳 quick update、无实质改动的 chore）。
7) 不杜撰内容，只基于可见改动生成。

输出要求：
- 直接给出对 docs/CHANGELOG.md 的修改结果（或补丁）。
- 若某小节没有内容，写“暂无”。
```

## 模板 B：发布阶段（Unreleased 落版）

将以下提示词完整复制给 LLM：

```text
你是本仓库的 Release Changelog 助手。请把 docs/CHANGELOG.md 中的 [Unreleased] 落版为 .version/VERSION 对应版本。

请严格执行：
1) 读取：
   - .version/VERSION（例如 v0.0.6）
   - docs/CHANGELOG.md
2) 在 changelog 中执行：
   - 将 [Unreleased] 的现有内容迁移到新版本块：
     ## [<VERSION>] - <YYYY-MM-DD>
   - 在顶部重建新的 [Unreleased] 模板，包含四个小节：新增/修复/变更/文档，内容先写“暂无”。
3) 不修改历史发布块内容顺序，不改写历史语义。
4) 保持现有 Markdown 风格与中文术语风格一致。

输出要求：
- 直接给出 docs/CHANGELOG.md 的修改结果（或补丁）。
```

## 推荐工作流

1. 功能开发完成后，使用“模板 A”更新 `Unreleased`。
2. 提交前复核条目是否准确、可读、可追溯。
3. 发布前使用“模板 B”执行落版。
