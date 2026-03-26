---
name: PR 审查专项规范
description: Use when handling pull request review, code review feedback, or PR comment resolution tasks in redant. In review mode, provide analysis/comments only and do not modify repository files.
---

# Redant PR 审查专项规范

仅在“代码审查模式”生效（如：审查 PR、回复 review comment、整理审查结论）。

## 默认运行模式（零输入自动全量）

- 当用户未提供 PR、轮次或指标时，默认自动识别当前分支对应 PR。
- 默认自动执行完整轮次：Round 0 -> Round 1 -> Round 2 -> Round 3 -> Round 4。
- 默认覆盖 PR 变更涉及的所有模块与所有问题分类。
- 若用户提供了轮次/指标/模块等限制条件，再在默认全量基线上收敛范围。

## 审查模式硬约束

- 审查模式下仅输出分析与建议，不直接修改仓库文件。
- 若用户明确切换为“请直接修复/落地修改”，先确认一次再进入实现模式。
- 结论必须证据优先：给出文件路径、符号、上下文片段（必要时附行号）。

## 审查意见格式（必须）

- 每条问题必须带 `[分类]` 前缀（如 `[LOGI]`、`[SEC]`、`[PERF]`）。
- 若无法准确归类，优先使用最接近分类，或使用 `[PRAC]` / `[LOGI]` 兜底。
- 严禁输出不带分类标签的审查建议。
- 一条评论只描述一个问题，避免把多个问题揉在一起。
- 优先提供“最小可执行修复建议”。

## 问题分类（速查）

- 关键：`REQ` `LOGI` `SEC` `AUTH`
- 高：`DSN` `RBST` `TRANS` `CONC` `PERF`
- 中：`CPT` `IDE` `MAIN` `CPL`
- 普通：`READ` `SIMPL` `CONS` `DUP` `NAM` `DOCS`
- 低：`COMM` `LOGG` `ERR` `FOR` `GRAM` `PRAC` `PR`

> 分类完整定义与检查项来源：`docs/review/CODE_REVIEW_GUIDE_CN.md`

## 审查输出建议结构

1. 结论概览（先讲风险与结论）
2. 逐条问题（带 `[分类]` 前缀 + 证据 + 建议）
3. Review Conclusion（必须）
   - 统计：按严重等级统计问题数量
   - 风险：当前最大技术/业务风险
   - 决策：`✅ 批准 / ⚠️ 需要修改 / ❌ 拒绝`

> 推荐在统计中使用严重程度分桶（🔴/🟠/🟡/🟢/🔵），便于轮次横向比较与汇总。

## 发布到 GitHub PR 评论时

- 行级评论优先；无法定位行号时再使用普通评论并说明原因。
- 发布前去重，建议唯一键：`path + line + 分类 + 模块 + 等级 + 问题摘要`。
- 保持与 `docs/review/PR_COMMENT_TEMPLATE.md` 的字段一致：分类 / 模块 / 等级 / 问题 / 原因 / 修改意见。
