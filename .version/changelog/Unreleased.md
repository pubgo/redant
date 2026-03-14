# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 新增隐藏全局标志 `--args`：支持以重复参数或 CSV 形式覆盖命令位置参数，用于在需要时通过 flag 直接注入/替代 `args`。

## 修复

暂无

## 变更

暂无

## 文档

- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md` 与 `docs/DESIGN.md`：新增隐藏内部标志 `--args` 的用途、示例与 `RawArgs` 交互说明。

