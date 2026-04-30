---
name: add-response-handler
description: "Add typed response handlers (Unary or Stream) to a redant command. Use when: returning structured data, JSON response, streaming output, NDJSON, ResponseHandler, ResponseStreamHandler, callback pattern."
argument-hint: "unary 或 stream"
---

# 添加类型化响应处理器

## 使用场景

- 命令需要返回结构化 JSON 响应（而非纯文本打印）
- 需要流式输出多条结构化数据
- 希望同时支持 stdio 输出和编程回调消费

## Unary vs Stream 选择

| 场景 | 模型 | 字段 |
|---|---|---|
| 返回单个结构化结果 | Unary | `ResponseHandler` |
| 返回多条流式数据 | Stream | `ResponseStreamHandler` |

**互斥规则**：`Handler`、`ResponseHandler`、`ResponseStreamHandler` 三者只能选一。

---

## Unary（单响应）

### 1. 定义响应类型

```go
type DeployResult struct {
	Env     string `json:"env"`
	Version string `json:"version"`
	Status  string `json:"status"`
}
```

### 2. 创建命令

```go
deployCmd := &redant.Command{
	Use:   "deploy <env>",
	Short: "部署到指定环境",
	ResponseHandler: redant.Unary(func(ctx context.Context, inv *redant.Invocation) (DeployResult, error) {
		env := "staging"
		if len(inv.Args) > 0 {
			env = inv.Args[0]
		}
		return DeployResult{
			Env:     env,
			Version: "1.0.0",
			Status:  "success",
		}, nil
	}),
}
```

### 3. 消费方式

**stdio 模式**（自动 JSON 输出到 stdout）：

```go
inv := rootCmd.Invoke("deploy", "prod")
inv.Stdout = os.Stdout
inv.Stderr = os.Stderr
err := inv.Run()
```

输出：

```json
{"$":"resp","type":"DeployResult","data":{"env":"prod","version":"1.0.0","status":"success"}}
```

**回调模式**（编程消费）：

```go
inv := rootCmd.Invoke("deploy", "prod")
inv.Stdout = io.Discard
err := redant.RunCallback[DeployResult](inv, func(result DeployResult) error {
	fmt.Printf("deployed %s to %s\n", result.Version, result.Env)
	return nil
})
```

**直接取值**：

```go
inv := rootCmd.Invoke("deploy", "prod")
inv.Stdout = io.Discard
if err := inv.Run(); err != nil { ... }
result, err := redant.Response[DeployResult](inv)
```

---

## Stream（流式响应）

### 1. 定义消息类型

```go
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}
```

### 2. 创建命令

```go
logsCmd := &redant.Command{
	Use:   "logs",
	Short: "流式查看日志",
	ResponseStreamHandler: redant.Stream(func(ctx context.Context, inv *redant.Invocation, out *redant.TypedWriter[LogEntry]) error {
		entries := fetchLogs(ctx) // 你的数据源
		for _, e := range entries {
			if err := out.Send(e); err != nil {
				return err
			}
		}
		return nil
	}),
}
```

### 3. 消费方式

**stdio 模式**（NDJSON 输出）：

```go
inv := rootCmd.Invoke("logs")
inv.Stdout = os.Stdout
err := inv.Run()
```

每行输出一条：

```json
{"$":"resp","type":"LogEntry","data":{"time":"...","level":"info","message":"..."}}
```

**回调模式**（逐条消费）：

```go
inv := rootCmd.Invoke("logs")
inv.Stdout = io.Discard
err := redant.RunCallback[LogEntry](inv, func(entry LogEntry) error {
	fmt.Printf("[%s] %s: %s\n", entry.Time, entry.Level, entry.Message)
	return nil
})
```

**Channel 模式**（通过 ResponseStream 迭代）：

```go
inv := rootCmd.Invoke("logs")
inv.Stdout = io.Discard
go inv.Run()
stream := inv.ResponseStream()
for val := range stream {
	entry, err := redant.StreamValue[LogEntry](val)
	// ...
}
```

---

## NDJSON Envelope 规范

所有结构化输出都包裹在 envelope 中：

| 字段 | 说明 |
|---|---|
| `$` | `"resp"` 或 `"error"` |
| `type` | Go 类型名 |
| `data` | 序列化后的值 |

错误 envelope：

```json
{"$":"error","type":"string","data":"something went wrong"}
```

## 与 MCP 的关系

`ResponseHandler` / `ResponseStreamHandler` 的类型信息通过 `TypeInfo()` 暴露，`mcpcmd` 会自动将其映射为 MCP tool schema——无需额外配置。
