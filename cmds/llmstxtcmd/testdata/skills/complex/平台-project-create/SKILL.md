---
name: 平台-project-create
description: "创建新项目。"
argument-hint: "name, description"
tools: "create_file, run_in_terminal"
---

# 平台 project create

## 使用场景

- 创建一个新项目并初始化默认配置。支持指定模板和标签。

## 用法

```sh
平台 project create <name> [description]
```

### 参数

- `name`（必填） — 项目名称。
- `description` — 项目描述。

### 选项

- `--private` — 设为私有项目。
- `--template string` — 项目模板名称。（默认: default）
- `--tags string-array` — 项目标签（可多次指定）。

