---
name: Shell PR 审查补充规范
description: Use when reviewing shell scripts in pull requests. Apply together with pr-review.instructions.md.
---

# Shell PR 审查补充规范

仅在 Shell/Bash/Zsh 脚本审查时生效。

## 重点检查项

- `[RBST]` 健壮性：严格模式、变量引用、失败路径处理。
- `[SEC]` 安全：命令注入、临时文件安全、权限最小化。
- `[PRAC]` 最佳实践：shebang 明确、函数封装、可维护日志。

## 审查基线

- 建议使用 `set -euo pipefail`（视脚本语义可做例外说明）。
- 变量引用默认使用双引号，避免单词分裂与 glob 意外扩展。
- 避免 `eval`；如必须使用，应给出严格输入约束与注释说明。
