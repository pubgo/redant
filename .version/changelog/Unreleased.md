# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 增加 `InvocationStream`，支持命令在执行期间进行响应流输出。
- 增加 `Invocation.ResponseStream()` 返回 `<-chan any`，由命令内部创建并暴露响应流供调用方消费。
- 增加 `ResponseHandler` / `ResponseStreamHandler` 接口化处理器，并提供 `Unary[T]` / `Stream[T]` 泛型适配器以携带运行时类型信息。
- 增加 `TypedWriter[T]` 泛型写入器，通过 `Send(v T)` 直接发送泛型数据到流通道。
- 增加 `RunCallback[T]` 泛型回调执行入口，统一支持 Unary 响应与 Stream 输出的类型化回调分发。
- 增加 `StreamError` 结构化错误类型，支持 `code/message/details`。
- `InvocationStream.Send` 自动镜像输出：`string`/`[]byte` → stdout，`StreamError` → stderr，struct → JSON 序列化到 stdout。

## 修复

- 修复 `InvocationStream.Send` 与 `closeResponseStream` 之间的并发竞态：channel 引用在创建时捕获，不再动态读取。

## 变更

- 流通道类型从 `chan map[string]any` 简化为 `chan any`，直接传递泛型数据，不再包装事件结构。
- 三类处理器互斥校验前移到 `init` 阶段：`Handler`、`ResponseHandler`、`ResponseStreamHandler` 同时配置时报错。
- 适配器函数 `adaptResponseHandler` / `adaptResponseStreamHandler` 改为包内私有。
- `handler.go` / `response_handlers.go` / `execution_typed.go` 合并为单一 `handler.go`。
- 对应测试文件 `execution_typed_test.go` / `stream_test.go` 合并为 `handler_test.go`。
- 移除 `Command.ResponseBuffer` 字段，流通道缓冲统一使用内部默认值。
- `webcmd/webui` 增加 `/api/run/stream/ws` 流式 WebSocket 执行通道。
- `webcmd/webui` 命令元数据 `/api/commands` 增加 `supportsStream` 字段。

## 文档

- 更新 `docs/DESIGN.md` 第 9 节，反映三类处理器互斥与泛型适配器架构，补充 Unary 执行路径与执行上下文兼容矩阵。
- 重写 `docs/INTERACTIVE_STREAMING.md`，更新为 `TypedWriter[T].Send` + `chan any` 模型；新增 Unary 执行路径、`ResponseTypeInfo` 说明、执行上下文兼容性表、MCP 集成章节。
- 更新 `docs/USAGE_AT_A_GLANCE.md`，新增第 8 节（Unary ResponseHandler）、第 9 节（Stream ResponseStreamHandler）、第 10 节（三类处理器对比表）。
- 更新 `docs/DOCS_CATALOG.md`，补充 `example/unary` 与 `example/stream-interactive` 示例引用。
- 新增 `example/unary` 示例及说明文档，展示 `Unary[T]` + `RunCallback[T]` 用法。
- 新增 `example/stream-interactive` 示例及说明文档。