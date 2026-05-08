---
name: platform-watch
description: "监听资源变更事件流。"
---

# platform watch

## 使用场景

- 通过 SSE 或 WebSocket 连接实时监听资源变更。
- 响应类型: Stream `github.com/pubgo/redant/cmds/llmstxtcmd.WatchEvent`

## 用法

```sh
platform watch
```

### 选项

- `--filter string` — 事件类型过滤。

