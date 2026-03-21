# Redant 命令行框架

Redant 是一个用于构建大型 Go 命令行程序的框架，提供命令树、选项系统、中间件链、帮助系统与多格式参数解析能力。

## 文档导航

README 仅保留“快速上手 + 能力入口”。详细设计与流程请跳转：

- 总索引：[`docs/INDEX.md`](docs/INDEX.md)
- 使用速览：[`docs/USAGE_AT_A_GLANCE.md`](docs/USAGE_AT_A_GLANCE.md)
- 架构设计：[`docs/DESIGN.md`](docs/DESIGN.md)
- MCP 指南：[`docs/MCP.md`](docs/MCP.md)
- WebTTY 指南：[`docs/WEBTTY.md`](docs/WEBTTY.md)
- 评估报告：[`docs/EVALUATION.md`](docs/EVALUATION.md)
- 变更记录：[`.version/changelog/README.md`](.version/changelog/README.md)

## 核心能力

- 命令树与子命令继承（支持嵌套）
- 选项多来源配置（命令行、环境变量、默认值）
- 中间件链式编排
- 自动帮助信息与全局标志
- 多格式参数解析（位置参数、查询串、表单、JSON）
- Busybox 风格 argv0 调度（软链接命令入口）
- MCP 工具暴露（将命令树映射为 Model Context Protocol Tools）
- Web 控制台（`web` 子命令）：可视化选择命令、填写 Flags/Args、查看调用过程与执行结果

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/pubgo/redant"
)

func main() {
    cmd := redant.Command{
        Use:   "echo <text>",
        Short: "输出传入文本",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            if len(inv.Args) == 0 {
                return fmt.Errorf("缺少文本参数")
            }
            fmt.Fprintln(inv.Stdout, inv.Args[0])
            return nil
        },
    }

    if err := cmd.Invoke().WithOS().Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

## 常用能力速览

### 参数与标志

- 子命令支持空格路径与冒号路径（如 `app repo commit` / `app repo:commit`）。
- 参数支持位置参数、query、form、JSON 四种形态。
- 推荐写法：`app <command> [flags...] [args...]`。

常用全局标志：

- `--help, -h`
- `--list-commands`
- `--list-flags`
- `--env, -e KEY=VALUE`
- `--env-file FILE`
- `--args VALUE`（内部隐藏，用于覆盖位置参数）

详细解析规则见：[`docs/USAGE_AT_A_GLANCE.md`](docs/USAGE_AT_A_GLANCE.md)。

### Web 调试界面

```text
app web
app web --addr 127.0.0.1:18080 --open=false
```

Web 控制台支持可视化填写 flags/args，并展示 `curl` 与多行 CLI 调用过程。更多说明见：[`docs/USAGE_AT_A_GLANCE.md`](docs/USAGE_AT_A_GLANCE.md)。

### WebTTY 本地终端

```text
app webtty
app webtty --addr 127.0.0.1:18081 --open=false
```

`webtty` 提供最简本地 Web 终端能力（`WebSocket + PTY`），并支持文件上传/下载。详细接口与迭代路线见：[`docs/WEBTTY.md`](docs/WEBTTY.md)。

### MCP 集成

```text
app mcp list
app mcp list --format text
app mcp serve --transport stdio
```

MCP 输入/输出协议、Schema 规则与排查建议见：[`docs/MCP.md`](docs/MCP.md)。

## 示例目录

- `example/demo`：综合示例
- `example/echo`：最小命令示例
- `example/env-test`：环境变量示例
- `example/globalflags`：全局标志示例
- `example/args-test`：参数解析示例

## 开发与维护

- 文档入口：[`docs/INDEX.md`](docs/INDEX.md)
- 变更记录：[`.version/changelog/README.md`](.version/changelog/README.md)
- 文档/变更维护提示：[`docs/CHANGELOG_LLM_PROMPT.md`](docs/CHANGELOG_LLM_PROMPT.md)

## 许可证

本项目采用 MIT 许可证，详见 [`LICENSE`](LICENSE)。
