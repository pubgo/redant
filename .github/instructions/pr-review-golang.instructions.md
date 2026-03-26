---
name: Go PR 审查补充规范
description: Use when reviewing Go code in pull requests. Apply together with pr-review.instructions.md.
---

# Go PR 审查补充规范

仅在 Go 代码审查时生效，需与通用 PR 审查规范一起使用。

## 重点检查项

- `[CONC]` 并发安全：goroutine 泄漏、竞态、channel 关闭与阻塞。
- `[RBST]` 错误处理：error 不可静默忽略，优先 `%w` 包装。
- `[PERF]` 性能：切片/map 预分配、避免不必要分配与重复计算。
- `[PRAC]` Go 惯例：命名、接口设计、defer 资源释放。

## 审查基线

- 禁止以 `_` 忽略关键 error。
- 长耗时/I/O 路径优先检查 `context.Context` 传递链。
- 共享可变状态必须有并发保护（锁、channel 或原子操作）。
- 库层避免 `panic` 作为常规错误处理。
