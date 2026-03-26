---
name: JS/TS PR 审查补充规范
description: Use when reviewing JavaScript or TypeScript code in pull requests. Apply together with pr-review.instructions.md.
---

# JS/TS PR 审查补充规范

仅在 JavaScript/TypeScript 审查时生效。

## 重点检查项

- `[SEC]` 安全：XSS/注入、`eval`/`Function`/危险 `innerHTML` 使用。
- `[RBST]` 健壮性：空值保护、异步错误传播、输入校验。
- `[PERF]` 性能：重复渲染/重复计算、$O(n^2)$ 循环、深拷贝开销。
- `[PRAC]` 最佳实践：TypeScript 类型约束、模块化、风格一致。

## 审查基线

- 外部输入（API/表单/环境变量）必须有类型与范围校验。
- Promise/async 路径必须有清晰错误处理。
- 尽量避免 `any`，公开接口参数/返回类型应可推断且稳定。
