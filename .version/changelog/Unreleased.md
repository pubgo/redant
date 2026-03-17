# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 新增隐藏全局标志 `--args`：支持以重复参数或 CSV 形式覆盖命令位置参数，用于在需要时通过 flag 直接注入/替代 `args`。
- 新增 MCP 能力：提供 `cmds/mcpcmd` 与 `internal/mcpserver`，支持将命令树映射为 MCP Tools，并通过 `mcp serve --transport stdio` 对外服务。
- 新增 Web 控制台能力：提供 `cmds/webcmd` 与 `internal/webui`，支持可视化选择命令、填写 flags/args、执行并查看调用过程与结果。

## 修复

- 修复根命令重复初始化场景下的全局标志重复注入问题，避免在 web 执行路径触发 `completion flag redefined: env` panic。
- 修复 Web 控制台 `enum-array` 交互异常（选择一个值时误全选）。
- 修复 Web 控制台中 args 映射丢失问题：当命名 `args` 缺失时回退使用 `rawArgs`，保证调用链路参数完整。
- 修复 `internal/webui/server_test.go` 中 `resp.Body.Close()` 返回值未检查导致的 `task lint` 失败。
- 修复 Web 控制台交互终端快捷键不生效问题：拦截并转发 `Ctrl+C/Ctrl+D/Ctrl+Z` 控制字符，避免浏览器默认快捷键吞掉输入，支持更稳定地中断 `top` 等交互程序。
- 修复 Web 控制台后端控制字符处理：收到 `Ctrl+C/Ctrl+Z` 时优先向 PTY 前台进程组发送 `SIGINT/SIGTSTP`，并在信号路径不可用时回退为原始字节写入，提升交互程序中断可靠性。
- 修复 Web 控制台 PTY 信号目标选择：控制字符信号路径改为基于 slave PTY (`pts`) 获取前台进程组，避免在 master PTY (`ptmx`) 场景下信号不生效。
- 增加 Web 控制台终端控制键后端诊断日志（环境变量 `REDANT_WEB_TTY_DEBUG=1`）：可观察控制键接收、信号路径结果与回退写入路径，便于排查 `Ctrl+C` 等问题。
- 修复 Web 控制台 `Ctrl+C` 信号兜底路径：当 `TIOCGPGRP` 返回 `inappropriate ioctl for device` 时，回退为向 shell 独立进程组发送信号，避免仅原始字节写入导致中断失效。

## 变更

- `internal/mcpserver` 的 MCP 协议处理切换为基于 `github.com/modelcontextprotocol/go-sdk`，移除自实现报文编解码，复用官方 Server/Transport 能力简化维护。
- MCP `serverInfo.name` 改为从根命令名动态推导（为空时回退 `redant-mcp`），使对外标识与 CLI 应用名一致。
- MCP `tools/call` 在保留文本 `Content` 的同时，新增 `StructuredContent`（`ok/stdout/stderr/error/combined`）并声明 `OutputSchema`，便于上层程序化消费。
- Web 运行接口返回扩展为 `program + argv + invocation`，前端据此渲染反斜杠续行的多行 CLI 调用过程，提升长命令可读性。
- Web 控制台交互终端增强：连接后返回并展示 `shell/cwd` 上下文，前端终端随容器自动 fit，并在尺寸变化时同步 PTY `resize`，提升“本地 shell”一致性体验。
- Web 控制台左侧 `Command` 列表改为树形可缩进导航，支持节点展开/折叠与搜索联动，减少多级命令场景下的选择混乱。
- Web 控制台左侧菜单支持收起/展开（窄栏模式），便于在小屏或参数编辑时为主内容区释放更多空间。
- Web 控制台交互终端支持一键全屏放大与 ESC 退出，并在切换时自动同步终端尺寸，提升长输出与交互调试体验。
- 优化 Web 控制台交互终端全屏样式：增加暗色遮罩、沉浸式面板布局与页面滚动锁定，提升全屏观感与操作一致性。

## 文档

- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md` 与 `docs/DESIGN.md`：新增隐藏内部标志 `--args` 的用途、示例与 `RawArgs` 交互说明。
- 补充 `README.md`、`docs/DESIGN.md` 与 `docs/INDEX.md`：新增 MCP 集成入口、模块职责与阅读路径。
- 补充 `README.md`、`docs/USAGE_AT_A_GLANCE.md`、`docs/DESIGN.md` 与 `docs/INDEX.md`：新增 Web 控制台说明、调用过程展示约定与参数顺序语义。
- 新增 `docs/MCP.md`：补全 MCP 子命令、工具映射规则、`tools/call` 输入输出协议、类型映射与常见问题排查。
- 重排 `README.md` 为总览型入口（精简章节与篇幅），将 Busybox/MCP/Web 等细节流程下沉到 `docs/*` 专项文档。

