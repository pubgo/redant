---
name: PR 审查专项规范
description: Use when handling pull request review, code review feedback, or PR comment resolution tasks in redant. In review mode, provide analysis/comments only and do not modify repository files.
---

# Redant PR 审查专项规范

仅在"代码审查模式"生效（如：审查 PR、回复 review comment、整理审查结论）。

## 审查规则来源（单一事实来源）

所有轮次定义、门禁条件、分类映射以 `docs/review/PR_REVIEW_RUBRIC.md` 为准。
评论模板与去重规则以 `docs/review/PR_COMMENT_TEMPLATE.md` 为准。
问题分类与检查清单以 `docs/review/CODE_REVIEW_GUIDE_CN.md` 为准。

## 审查模式硬约束

- 审查模式下仅输出分析与建议，不直接修改仓库文件。
- 若用户明确切换为"请直接修复/落地修改"，先确认一次再进入实现模式。
- 结论必须证据优先：给出文件路径、符号、上下文片段（必要时附行号）。
- 每条问题必须带 `[分类]` 前缀（如 `[LOGI]`、`[SEC]`、`[PERF]`），分类集合以 `docs/review/CODE_REVIEW_GUIDE_CN.md` 为准。
- 一条评论只描述一个问题，避免把多个问题揉在一起。
- 优先提供"最小可执行修复建议"。

## 发布到 GitHub PR 评论时

- 默认发布到 GitHub PR 评论；仅当用户明确要求"不发布评论/仅聊天输出"时跳过发布。
- 行级评论优先；无法定位行号时再使用普通评论并说明原因。
- 评论格式与去重规则以 `docs/review/PR_COMMENT_TEMPLATE.md` 为准。
