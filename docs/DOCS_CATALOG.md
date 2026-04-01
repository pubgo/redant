# Redant 文档分类目录

本页用于按主题聚合文档，降低检索成本。默认从上到下阅读。

## 1) 快速上手

- [`USAGE_AT_A_GLANCE.md`](USAGE_AT_A_GLANCE.md)：命令命名、参数形态、Flag 规则速览。
- [`MCP.md`](MCP.md)：MCP 子命令、工具映射、输入输出约定。
- [`WEBTTY.md`](WEBTTY.md)：WebTTY 能力、接口约定、排查建议。

## 2) 架构与质量

- [`DESIGN.md`](DESIGN.md)：核心模型、解析流程、扩展点。
- [`INTERACTIVE_STREAMING.md`](INTERACTIVE_STREAMING.md)：交互式命令与结构化响应流方案。
- [`EVALUATION.md`](EVALUATION.md)：质量评估、风险与优化建议。

## 3) PR 审查体系（聚合）

- [`review/PR_REVIEW_RUBRIC.md`](review/PR_REVIEW_RUBRIC.md)：分轮审查基线（含零输入自动全量模式）。
- [`review/PR_COMMENT_TEMPLATE.md`](review/PR_COMMENT_TEMPLATE.md)：PR 行级评论统一模板。
- [`review/CODE_REVIEW_GUIDE_CN.md`](review/CODE_REVIEW_GUIDE_CN.md)：问题分类字典与详细检查清单。

## 4) 发布与变更维护

- [`CHANGELOG_LLM_PROMPT.md`](CHANGELOG_LLM_PROMPT.md)：changelog 自动维护提示词。
- 版本日志目录：`.version/changelog/`（Unreleased 与版本落版记录）。

## 5) 仓库外延入口（按需）

- 项目总览：`README.md`
- 示例：`example/args-test/README.md`（参数解析示例）
- 示例：`example/unary/README.md`（Unary 响应处理器示例）
- 示例：`example/stream-interactive/README.md`（流式响应处理器示例）
- 内部样式维护：`internal/pretty/README.md`

---

## 建议阅读路径

1. 先看“快速上手”了解能力边界。
2. 再看“架构与质量”建立实现心智模型。
3. 涉及 PR 审查时进入“PR 审查体系（聚合）”。
4. 发布前进入“发布与变更维护”。