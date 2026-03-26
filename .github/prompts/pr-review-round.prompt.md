---
name: PR 分轮审查（按文档与指标）
description: "Run a round-by-round PR review using the project rubric and user metrics; enforce evidence-first output to reduce omissions."
argument-hint: "输入 PR 链接/编号、当前轮次、关注指标（如正确性/覆盖率/兼容性）"
agent: "PR 分轮审查代理"
---
请按以下规则执行本轮 PR 审查：

- 审查基线文档：[`docs/PR_REVIEW_RUBRIC.md`](../../docs/PR_REVIEW_RUBRIC.md)
- 评论模板文档：[`docs/PR_COMMENT_TEMPLATE.md`](../../docs/PR_COMMENT_TEMPLATE.md)
- 输出必须使用“每轮输出模板”
- 本轮只做指定轮次，不跨轮
- 结论必须给证据（文件路径 + 关键片段）
- 如用户明确要求“发布到 PR comment”，需将本轮问题按文件行级评论写入 GitHub，并返回评论链接
- 发布到 GitHub 的评论默认使用中文，且每条必须使用统一模板：分类 / 模块 / 等级 / 问题 / 原因 / 修改意见
- 发布前先去重：按 `path + line + 分类 + 模块 + 等级 + 问题摘要` 检查已发布评论；若已存在则复用链接，不重复发同类评论
- 信息不足时，先列“所缺信息清单”，不要硬判

用户输入参数：
- PR：{{input}}（可为空；为空时自动按当前 git 分支定位对应 PR）
- 当前轮次：由用户在输入里明确（如 Round 1）
- 关注指标：由用户在输入里明确（可多个）

若当前分支没有 PR：
- 先提示“可自动创建 Draft PR 后继续审查”。
- 用户同意时，创建 Draft PR 并继续本轮审查。

如果用户没有给轮次：默认执行 Round 0。
如果用户没有给指标：默认检查“正确性 + 测试覆盖 + 兼容性”。
