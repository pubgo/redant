# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 增加 `StreamHandlerFunc` 与 `InvocationStream`，支持命令在执行期间进行结构化响应流输出。
- 增加 `Invocation.ResponseStream()`，由命令内部创建并暴露响应流供调用方消费。
- 增加 `StreamMessage` 的 JSON-RPC 风格协议字段（`jsonrpc/id/method/type/data/error/meta`）与归一化逻辑。
- 增加 `InvocationStream` 便捷方法：`Output/Outputf/Control/Error/Exit/EndRound`。
- 增加流事件 method 约定常量（如 `stream.output.chunk`、`stream.round.end`、`stream.exit`）。
- 增加自动消息 ID 生命周期：未显式设置 `id` 时自动生成 `<stream-id>-<seq>`。
- 增加轮次结束事件 `stream.round.end`，用于表达阶段输出完成。

## 修复

- 暂无

## 变更

- 调整命令执行优先级：当命令定义 `StreamHandler` 时，运行时优先使用流式处理器（通过适配层复用现有中间件链）。
- 明确并验证 `inv.Run()` 在流式通道上的阻塞语义：发送端在无消费者时阻塞，直至消费或上下文取消。
- Stream 消息协议升级为非兼容新模型：统一采用 `jsonrpc/id/method/type/data/error/meta`，移除 `Kind/Payload` 兼容字段。
- 流模型进一步收敛：移除 `NextInput` 及相关输入能力，当前仅保留纯响应流输出语义。

## 文档

- 新增 `docs/INTERACTIVE_STREAMING.md`，补充流式交互模型、执行路径与开发任务同步。
- 更新 `docs/DESIGN.md`、`docs/USAGE_AT_A_GLANCE.md`、`docs/INDEX.md`、`docs/DOCS_CATALOG.md` 以反映交互流改造。
- 新增 `example/stream-interactive` 示例及说明文档，覆盖 stdio 回退模式与 channel 响应流消费模式。
- 更新 `example/stream-interactive` 为“纯响应流输出 + 内建响应流消费”模式。

