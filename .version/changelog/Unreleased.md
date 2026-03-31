# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 增加 `InvocationStream`，支持命令在执行期间进行结构化响应流输出。
- 增加 `Invocation.ResponseStream()`，由命令内部创建并暴露响应流供调用方消费。
- 增加流事件轻量载荷（`event/data/error`）并通过 `ResponseStream()` 通道传递。
- 增加 `InvocationStream.Send` 统一发送接口（便捷方法已移除）。
- 增加流事件标签约定（如 `output_chunk`、`round_end`、`exit`）。
- 增加轮次结束事件 `round_end`，用于表达阶段输出完成。
- 增加 `Invocation.Execute(ctx, ExecutionOptions)` 通用执行入口，统一支持请求-响应（RR）与请求-响应流（RRS）两种模型。
- 增加 `ResponseHandler` / `ResponseStreamHandler` 接口化处理器，并提供 `Unary[T]` / `Stream[T]` 泛型适配器以携带运行时类型信息。

## 修复

- 暂无

## 变更

- 调整命令执行语义：流式能力统一由 `ResponseStreamHandler` 承载，并继续复用现有中间件链。
- 明确并验证 `inv.Run()` 在流式通道上的阻塞语义：发送端在无消费者时阻塞，直至消费或上下文取消。
- Stream 消息协议进一步收敛：统一采用 `event/data/error`，移除 `jsonrpc/id/type/round/meta` 冗余字段。
- 流模型进一步收敛：移除 `NextInput` 及相关输入能力，当前仅保留纯响应流输出语义。
- 移除 `StreamMessage` 类型，函数调用路径不再暴露通信结构；流事件通道改为原始载荷（`map[string]any`）。
- `webcmd/webui` 增加请求-响应流执行通道：新增 `/api/run/stream/ws`，可实时接收结构化 `stream` 事件与最终 `result`。
- `webcmd/webui` 命令元数据增强：`/api/commands` 增加 `supportsStream`，并纳入 `ResponseStreamHandler` 命令展示。
- `webcmd/webui` 执行路径改为复用 command 核心运行能力（无响应场景走 `Run`，有响应场景走 `RunCallback`）。
- `command` 执行分发支持同级三类处理器：`Handler`（无响应）、`ResponseHandler`（单响应）、`ResponseStreamHandler`（响应流）。
- 移除 `ExecutionModel` / `Command.Model` 显式模型声明，改为以处理器类型驱动运行路径选择。
- `command` 运行时统一处理器解析入口并前移校验到 `init` 阶段：多处理器冲突等配置错误可在初始化阶段提前暴露。
- 流式通道改为按需订阅：仅在调用 `ResponseStream()` 时写入内部通道，`inv.Run()` 在无通道消费者场景不再因通道背压阻塞。
- 新增并收敛泛型运行入口 `RunCallback[T]`：运行上下文由 `inv` 承载（无需额外传 `ctx`），回调仅接收类型化数据；内部直接调用原始 `Run`，统一支持 unary 响应与 stream 输出/分块输出的类型化回调分发，并在类型不匹配时显式报错。
- 收缩对外执行 API：移除 `Execute` / `RunWithCallbacks` 暴露；有响应调用统一走 `RunCallback`，无响应调用继续走原始 `Run`。

## 文档

- 新增 `docs/INTERACTIVE_STREAMING.md`，补充流式交互模型、执行路径与开发任务同步。
- 更新 `docs/DESIGN.md`、`docs/USAGE_AT_A_GLANCE.md`、`docs/INDEX.md`、`docs/DOCS_CATALOG.md` 以反映交互流改造。
- 新增 `example/stream-interactive` 示例及说明文档，覆盖 stdio 回退模式与 channel 响应流消费模式。
- 更新 `example/stream-interactive` 为“纯响应流输出 + 内建响应流消费”模式。

