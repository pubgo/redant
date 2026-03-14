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
- 使用规范速览：[`docs/USAGE_AT_A_GLANCE.md`](docs/USAGE_AT_A_GLANCE.md)
- 架构设计：[`docs/DESIGN.md`](docs/DESIGN.md)
- 评估报告：[`docs/EVALUATION.md`](docs/EVALUATION.md)
- 版本记录：[`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- 参数示例：[`example/args-test/README.md`](example/args-test/README.md)

术语使用请参考：[`docs/INDEX.md`](docs/INDEX.md) 的“术语约定”章节。

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

### 完整调用流程（构建到分发）

```mermaid
flowchart TD
    A[编译生成二进制: app] --> B[创建软连接: ln -sf app echo]
    B --> C[调用软连接: ./echo hello]
    C --> D[程序读取 argv0]
    D --> E{argv0 名称是否与命令/别名一致}
    E -- 是 --> F[选择对应子命令并执行]
    E -- 否 --> G[回退到显式参数解析或根命令]
```

流程说明：

1. 先构建主二进制（例如 `app`）。
2. 通过 `ln -sf` 创建软连接（例如 `echo -> app`）。
3. 用户调用软连接名（例如 `echo hello`）。
4. 框架读取 `argv0`（此时通常为 `echo`）。
5. 若 `argv0` 与命令名或别名匹配，则直接调用该子命令；否则按常规参数路径继续解析。

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

## 参数解析流程

框架在命令分发完成后，会进入统一参数解析阶段，支持位置参数、查询串、表单与 JSON。

```mermaid
flowchart TD
    A[接收命令行输入] --> B[完成命令分发]
    B --> C[解析全局/局部标志]
    C --> D[提取剩余参数]
    D --> E{参数形态判断}
    E -- 普通 token --> F[位置参数]
    E -- 包含 '=' 且含 '&' --> G[查询串参数]
    E -- 包含 '=' 且含空格 --> H[表单参数]
    E -- 以 '{' 或 '[' 开头 --> I[JSON 参数]
    F --> J[写入 ArgSet / inv.Args]
    G --> J
    H --> J
    I --> J
    J --> K[必填与类型校验]
    K --> L[进入中间件与 Handler]
```

参数解析落地示例见：[`example/args-test/README.md`](example/args-test/README.md)。

### 参数解析优先级

```mermaid
flowchart TD
    A[输入命令行] --> B{是否命中显式子命令}
    B -- 是 --> C[按显式子命令执行]
    B -- 否 --> D{argv0 是否命中命令/别名}
    D -- 是 --> E[按 argv0 分发子命令]
    D -- 否 --> F[保留根命令路径]
    C --> G[解析标志]
    E --> G
    F --> G
    G --> H[解析剩余参数]
    H --> I[进入中间件与 Handler]
```

优先级顺序：

1. 显式子命令（最高）
2. `argv0` 命令/别名分发
3. 根命令默认路径
4. 标志解析与参数格式解析

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
