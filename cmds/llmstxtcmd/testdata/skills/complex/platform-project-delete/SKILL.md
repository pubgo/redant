---
name: platform-project-delete
description: "删除项目。"
---

# platform project delete

## 使用场景

- 永久删除指定项目及其所有资源。此操作不可逆。

## 用法

```sh
platform project delete <project-id>
```

### 参数

- `project-id`（必填） — 要删除的项目 ID。

### 选项

- `-f, --force` — 跳过确认提示。
- `--dry-run` — 仅模拟删除，不实际执行。

