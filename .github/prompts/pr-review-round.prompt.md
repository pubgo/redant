---
name: PR 分轮审查（按文档与指标）
description: "Run a round-by-round PR review using the project rubric and user metrics; enforce evidence-first output to reduce omissions."
argument-hint: "可留空；默认自动识别当前分支 PR 并执行完整 Round 0~4 全量审查"
agent: "PR 分轮审查代理"
---
请按以下规则执行 PR 审查：

- 审查基线文档：[`docs/review/PR_REVIEW_RUBRIC.md`](../../docs/review/PR_REVIEW_RUBRIC.md)
- 评论模板文档：[`docs/review/PR_COMMENT_TEMPLATE.md`](../../docs/review/PR_COMMENT_TEMPLATE.md)
- 审查指令文档：
	- [`.github/instructions/pr-review.instructions.md`](../instructions/pr-review.instructions.md)
	- [`.github/instructions/pr-review-golang.instructions.md`](../instructions/pr-review-golang.instructions.md)
	- [`.github/instructions/pr-review-javascript.instructions.md`](../instructions/pr-review-javascript.instructions.md)
	- [`.github/instructions/pr-review-shell.instructions.md`](../instructions/pr-review-shell.instructions.md)
- 输出必须使用“每轮输出模板”
- 本轮只做指定轮次，不跨轮
- 结论必须给证据（文件路径 + 关键片段）
- 问题建议必须使用 `[分类]` 前缀（如 `[LOGI]`、`[SEC]`、`[PERF]`）
- Round 4 最终结论必须包含“全分类勾选清单（26 类）”，并标注每类 `已检查 / N/A`
- 默认将问题发布到 PR 行级评论，并返回评论链接；若用户明确要求“不发布评论/仅聊天输出”，则仅在聊天中输出
- 发布到 GitHub 的评论默认使用中文，且每条必须使用统一模板：分类 / 模块 / 等级 / 问题 / 原因 / 修改意见
- 评论中的“分类”字段必须使用代码标签格式：`[分类代码] 分类名称`（如 `[LOGI] 逻辑问题`）
- 发布前先去重：按 `path + line + 分类 + 模块 + 等级 + 问题摘要` 检查已发布评论；若已存在则复用链接，不重复发同类评论
- 信息不足时，先列“所缺信息清单”，不要硬判

默认运行策略（零输入）：
- 无需用户输入 PR、轮次、指标。
- 自动识别当前分支对应 PR。
- 默认执行完整 Round 0 -> Round 1 -> Round 2 -> Round 3 -> Round 4。
- 默认覆盖 PR 变更涉及的所有模块与所有问题分类。
- 默认审查指标：正确性 + 安全 + 性能 + 可维护性 + 兼容性 + 测试覆盖 + 文档一致性。

用户输入参数：
- PR：{{input}}（可为空）
- 当前轮次：可选
- 关注指标：可选

若当前分支没有 PR：
- 先提示“可自动创建 Draft PR 后继续审查”。
- 用户同意时，创建 Draft PR 并继续本轮审查。

如果用户给了轮次：按用户指定轮次执行。
如果用户给了指标：按用户指定指标收敛范围。
如果用户未给任何输入：执行默认自动全量审查策略。
