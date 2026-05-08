---
name: 平台-search
description: "Search platform resources by keyword with scope filtering."
argument-hint: "搜索关键词，如 deployment nginx"
applyTo: "**/*.go"
condition: "当用户需要在平台中搜索资源时使用"
---

# 平台 search

## 使用场景

- 在平台中全文搜索资源，支持按范围过滤和结果限制。
- 别名: s, find
- 响应类型: Unary `[]llmstxtcmd.SearchResult`

## 用法

```sh
平台 search <query> [scope]
```

### 参数

- `query`（必填） — 搜索关键词。
- `scope` — 搜索范围（project/org/global）。（默认: project）

### 选项

- `-n, --limit int64` — 最大返回条数。（默认: 20）
- `--endpoint string` — 搜索服务地址。（默认: https://search.example.com; 环境变量: SEARCH_ENDPOINT）
- `-f, --format enum[table\|json\|csv]` — 输出格式。（默认: table）

