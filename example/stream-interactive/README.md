# stream-interactive 示例

这个示例展示 Redant 在“纯响应流”语义下的两种运行方式：

1. `stdio`：直接执行 `inv.Run()`，输出落到终端。
2. `callback`：通过 `RunCallback` 消费类型化输出数据，适合函数调用场景。

并且在 `callback` 模式下会实时打印类型化输出文本（`string`）。

> 注意：当前为新协议模型，不再使用旧的 `Kind/Payload` 字段。

## 调用流程

```mermaid
flowchart LR
    A[调用 stream-interactive] --> B{mode}
    B -- stdio --> C[inv.Run]
    B -- callback --> D[RunCallback]
    C --> E[ResponseStreamHandler]
    D --> E
    E --> F[控制/输出/退出事件]
    F -- stdio --> G[Stdout/Stderr]
    F -- callback --> H[typed callback]
```

## 运行方式

### 1) 终端交互模式（stdio）

```bash
go run ./example/stream-interactive stdio
```

运行后可直接看到控制信息与输出内容。

### 2) 回调模式（callback）

```bash
go run ./example/stream-interactive callback
```

该模式会通过 `RunCallback[string]` 实时接收并打印文本输出。

> 兼容说明：`channel` 仍作为别名可用，便于旧脚本平滑迁移。

## 关键代码点

- 命令定义：`ResponseStreamHandler: redant.Stream(...)`
- 回调执行：`RunCallback[T](inv, callback)`
- 轮次结束：`stream.Send(map[string]any{"event":"round_end", ...})`

## 阻塞语义说明

- `inv.Run()` 是阻塞调用。
- 在 `RunCallback` 模式中，仅 `output/output.chunk` 的类型化数据会分发到回调；`Run()` 结束后流自动关闭。
- 推荐始终通过 `context` 设置超时/取消，避免上游异常导致无限等待。
