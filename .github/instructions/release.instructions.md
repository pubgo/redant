---
name: 发布前变更核对约束
description: "Use when preparing a release or completing behavior-impacting changes in redant, including changelog updates, docs synchronization, and regression checks."
---

# Redant 发布前核对规则

用于“准备发布”或“完成具备行为影响的改动”时的统一核对。

## 发布前检查清单

- 变更说明已写入 `.version/changelog/Unreleased.md`，分类正确（新增/修复/变更/文档）。
- 涉及架构或流程的改动，已同步更新 `docs/DESIGN.md`。
- 用户可见行为变化，已同步示例或说明文档（如 `README.md`、`docs/USAGE_AT_A_GLANCE.md`）。

## 质量门槛

- 至少完成相关范围测试；条件允许时执行完整测试回归。
- 不以“暂时跳过测试/校验”作为发布前状态。
- 仅基于真实改动与真实测试结果编写发布说明，不杜撰。

## 落版约束提示

- 若进入正式落版，版本号来源于 `.version/VERSION`。
- 落版后应重建 `Unreleased` 模板并更新 changelog 索引。
- changelog 结构与落版细节以 `.github/instructions/changelog.instructions.md` 为准。
