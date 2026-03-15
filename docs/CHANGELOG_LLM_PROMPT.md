# Changelog LLM 提示词模板

本文档提供“修改功能后自动生成 changelog”的提示词模板，适用于 Copilot/ChatGPT 等 LLM。

## 使用目标

- 基于当前代码改动自动更新 `.version/changelog/Unreleased.md`。
- 发布前将 `Unreleased.md` 落版为 `.version/VERSION` 对应版本文件。

## 模板 A：开发阶段（自动更新 Unreleased）

将以下提示词完整复制给 LLM：

```text
你是本仓库的 Changelog 维护助手。请根据当前工作区改动，自动更新 .version/changelog/Unreleased.md。

请严格执行：
1) 读取并理解以下文件：
   - .version/VERSION
   - .version/changelog/Unreleased.md
   - 本次改动涉及的文件 diff（若可用）
2) 仅更新 .version/changelog/Unreleased.md，不修改已发布版本文件。
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
- 直接给出对 .version/changelog/Unreleased.md 的修改结果（或补丁）。
- 若某小节没有内容，写“暂无”。
```

## 模板 B：发布阶段（Unreleased 落版）

将以下提示词完整复制给 LLM：

```text
你是本仓库的 Release Changelog 助手。请把 .version/changelog/Unreleased.md 落版为 .version/VERSION 对应版本文件。

请严格执行：
1) 读取：
   - .version/VERSION（例如 v0.0.6）
   - .version/changelog/Unreleased.md
2) 在 changelog 中执行：
   - 创建新版本文件：`.version/changelog/<VERSION>.md`，格式为：
     # [<VERSION>] - <YYYY-MM-DD>
   - 将 `Unreleased.md` 的内容迁移到该版本文件（分类保持：新增/修复/变更/文档）。
   - 将 `Unreleased.md` 重建为空模板（四个分类，内容写“暂无”）。
3) 不改写历史版本文件语义，不重排已发布版本顺序。
4) 保持现有 Markdown 风格与中文术语风格一致。

输出要求：
- 直接给出 `.version/changelog/<VERSION>.md` 与 `.version/changelog/Unreleased.md` 的修改结果（或补丁）。
```

## 推荐工作流

1. 功能开发完成后，使用“模板 A”更新 `Unreleased`。
2. 提交前复核条目是否准确、可读、可追溯。
3. 发布前使用“模板 B”执行落版。
