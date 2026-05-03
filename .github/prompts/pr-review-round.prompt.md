---
name: PR 分轮审查（按文档与指标）
description: "Run a round-by-round PR review using the project rubric and user metrics; enforce evidence-first output to reduce omissions."
argument-hint: "可留空；默认自动识别当前分支 PR 并执行完整 Round 0~4 全量审查"
agent: "PR 分轮审查代理"
---
请按以下审查文档执行 PR 审查：

- 轮次流程与门禁条件：[`docs/review/PR_REVIEW_RUBRIC.md`](../../docs/review/PR_REVIEW_RUBRIC.md)
- 评论模板与去重规则：[`docs/review/PR_COMMENT_TEMPLATE.md`](../../docs/review/PR_COMMENT_TEMPLATE.md)
- 问题分类与检查清单：[`docs/review/CODE_REVIEW_GUIDE_CN.md`](../../docs/review/CODE_REVIEW_GUIDE_CN.md)
- Go 语言专项规范：[`.github/instructions/pr-review-golang.instructions.md`](../instructions/pr-review-golang.instructions.md)

用户输入参数：
- PR：{{input}}（可为空）
- 当前轮次：可选
- 关注指标：可选

若用户未给任何输入：执行默认自动全量审查策略（Round 0~4 全流程）。
若用户给了轮次：按用户指定轮次执行。
若用户给了指标：按用户指定指标收敛范围。

若当前分支没有 PR：
- 先提示"可自动创建 Draft PR 后继续审查"。
- 用户同意时，创建 Draft PR 并继续本轮审查。
