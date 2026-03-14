# Redant 命令行框架

Redant 是一个用于构建大型 Go 命令行程序的框架，提供命令树、选项系统、中间件链、帮助系统与多格式参数解析能力。

## 文档导航

```mermaid
flowchart TD
    A[README 总览] --> B[docs/INDEX.md 文档索引]
    B --> C[docs/DESIGN.md 架构与执行设计]
    B --> D[docs/EVALUATION.md 质量评估与改进]
    B --> E[docs/CHANGELOG.md 版本变更]
    B --> F[example/args-test/README.md 参数解析示例]
```

- 文档总索引：[`docs/INDEX.md`](docs/INDEX.md)
- 架构设计：[`docs/DESIGN.md`](docs/DESIGN.md)
- 评估报告：[`docs/EVALUATION.md`](docs/EVALUATION.md)
- 版本记录：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 参数示例：[`example/args-test/README.md`](example/args-test/README.md)

## 核心能力

- 命令树与子命令继承（支持嵌套）
- 选项多来源配置（命令行、环境变量、默认值）
- 中间件链式编排
- 自动帮助信息与全局标志
- 多格式参数解析（位置参数、查询串、表单、JSON）
- Busybox 风格 argv0 调度（软链接命令入口）

## 架构总览

```mermaid
flowchart LR
    U[用户输入] --> P[命令解析]
    P --> F[标志解析]
    F --> A[参数解析]
    A --> M[中间件链]
    M --> H[命令处理器]
    H --> O[输出与退出码]
```

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

## Busybox 风格命令入口

通过软链接将一个二进制映射为多个独立命令名，框架会根据 `argv0` 自动分发到对应子命令。

```mermaid
flowchart TD
    A[启动可执行文件] --> B{是否显式提供子命令}
    B -- 是 --> C[按参数分发]
    B -- 否 --> D[按 argv0 分发]
    C --> E[执行目标命令]
    D --> E
```

示例：

- 显式调用：`app echo hello`
- 软链接调用：`echo hello`

## 全局标志

- `--help, -h`：显示帮助
- `--list-commands`：列出命令树
- `--list-flags`：列出所有标志

## 示例目录

- `example/demo`：综合示例
- `example/echo`：最小命令示例
- `example/env-test`：环境变量示例
- `example/globalflags`：全局标志示例
- `example/args-test`：参数格式解析示例

## 许可证

本项目采用 MIT 许可证，详见 [`LICENSE`](LICENSE)。
