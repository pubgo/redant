# Redant

[![Go Reference](https://pkg.go.dev/badge/github.com/pubgo/redant.svg)](https://pkg.go.dev/github.com/pubgo/redant)
[![Go Report Card](https://goreportcard.com/badge/github.com/pubgo/redant)](https://goreportcard.com/report/github.com/pubgo/redant)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

[English](README.md) | 简体中文

Redant 是一个强大的 Go CLI 框架，专为构建大型命令行应用而设计。它提供灵活的选项配置、优秀的帮助输出和基于中间件的组合模式。

## 特性

- **命令树结构**：支持复杂的嵌套命令结构，子命令可继承父命令的标志
- **多源配置**：选项可从命令行标志和环境变量设置
- **中间件系统**：基于 Chi 路由器模式，便于功能扩展
- **优秀的帮助系统**：借鉴 Go 工具链的帮助输出风格
- **易于测试**：清晰分离 stdin/stdout/stderr，便于单元测试
- **灵活的参数格式**：支持查询字符串、表单数据和 JSON 格式
- **丰富的值类型**：String、Int64、Float64、Bool、Duration、Enum、URL、HostPort 等

## 安装

```bash
go get github.com/pubgo/redant
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/pubgo/redant"
)

func main() {
    cmd := redant.Command{
        Use:   "echo <text>",
        Short: "将给定文本打印到控制台",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            if len(inv.Args) == 0 {
                return fmt.Errorf("缺少文本参数")
            }
            fmt.Fprintln(inv.Stdout, inv.Args[0])
            return nil
        },
    }

    if err := cmd.Invoke().WithOS().Run(); err != nil {
        panic(err)
    }
}
```

### 带选项的命令

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/pubgo/redant"
)

func main() {
    var (
        port    int64
        host    string
        verbose bool
    )
    
    cmd := redant.Command{
        Use:   "server",
        Short: "启动 HTTP 服务器",
        Options: redant.OptionSet{
            {
                Flag:        "port",
                Shorthand:   "p",
                Description: "监听端口",
                Default:     "8080",
                Value:       redant.Int64Of(&port),
            },
            {
                Flag:        "host",
                Description: "绑定地址",
                Default:     "localhost",
                Envs:        []string{"SERVER_HOST"},
                Value:       redant.StringOf(&host),
            },
            {
                Flag:        "verbose",
                Shorthand:   "v",
                Description: "启用详细输出",
                Value:       redant.BoolOf(&verbose),
            },
        },
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintf(inv.Stdout, "在 %s:%d 启动服务器\n", host, port)
            if verbose {
                fmt.Fprintln(inv.Stdout, "详细模式已启用")
            }
            return nil
        },
    }

    if err := cmd.Invoke().WithOS().Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### 嵌套命令

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/pubgo/redant"
)

func main() {
    rootCmd := &redant.Command{
        Use:   "app",
        Short: "示例应用",
    }
    
    serverCmd := &redant.Command{
        Use:   "server",
        Short: "服务器命令",
    }
    
    startCmd := &redant.Command{
        Use:   "start",
        Short: "启动服务器",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "服务器已启动！")
            return nil
        },
    }
    
    stopCmd := &redant.Command{
        Use:   "stop",
        Short: "停止服务器",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "服务器已停止！")
            return nil
        },
    }
    
    serverCmd.Children = append(serverCmd.Children, startCmd, stopCmd)
    rootCmd.Children = append(rootCmd.Children, serverCmd)
    
    if err := rootCmd.Invoke().WithOS().Run(); err != nil {
        panic(err)
    }
}
```

### Busybox 风格的 argv0 调度

Redant 支持根据可执行文件名（\`argv[0]\`）分发子命令，可以通过软链接/硬链接把子命令暴露成独立命令，类似 busybox。

1. 像平常一样定义命令树；需要额外名字时，可以在命令上设置 \`Aliases\`。
2. 构建二进制后，为想暴露的子命令创建软链，例如：\`ln -sf /usr/local/bin/app /usr/local/bin/echo\`。
3. 通过软链执行 \`echo ...\` 会直接命中 \`echo\` 子命令；显式的 \`app echo ...\` 依旧可用，并在两者同时存在时优先使用显式参数。
4. 在测试或模拟场景，可使用 \`cmd.Invoke(args...).WithArgv0("echo").Run()\` 手动设置 \`argv0\`。

## 值类型

Redant 提供丰富的值类型：

| 类型 | 函数 | 描述 |
|------|------|------|
| `String` | `StringOf(&v)` | 字符串值 |
| `Int64` | `Int64Of(&v)` | 64位整数 |
| `Float64` | `Float64Of(&v)` | 64位浮点数 |
| `Bool` | `BoolOf(&v)` | 布尔值 |
| `Duration` | `DurationOf(&v)` | 时间间隔 |
| `StringArray` | `StringArrayOf(&v)` | 字符串切片 |
| `Enum` | `EnumOf(&v, choices...)` | 枚举（带验证） |
| `EnumArray` | `EnumArrayOf(&v, choices...)` | 枚举数组 |
| `URL` | `&URL{}` | URL 解析 |
| `HostPort` | `&HostPort{}` | Host:Port 解析 |

### 验证器

```go
var port int64

opt := redant.Option{
    Flag:  "port",
    Value: redant.Validate(redant.Int64Of(&port), func(v *redant.Int64) error {
        if v.Value() < 1 || v.Value() > 65535 {
            return fmt.Errorf("端口必须在 1-65535 之间")
        }
        return nil
    }),
}
```

## 中间件

Redant 支持中间件模式处理横切关注点：

```go
cmd := redant.Command{
    Use:   "example",
    Short: "示例命令",
    Middleware: redant.Chain(
        // 要求恰好 1 个参数
        redant.RequireNArgs(1),
        // 自定义日志中间件
        func(next redant.HandlerFunc) redant.HandlerFunc {
            return func(ctx context.Context, inv *redant.Invocation) error {
                fmt.Printf("执行: %s\n", inv.Command.Name())
                err := next(ctx, inv)
                fmt.Printf("完成: %s\n", inv.Command.Name())
                return err
            }
        },
    ),
    Handler: func(ctx context.Context, inv *redant.Invocation) error {
        // 处理逻辑
        return nil
    },
}
```

## 参数格式

Redant 支持多种参数格式：

```bash
# 位置参数
app arg1 arg2 arg3

# 查询字符串格式
app "name=value&age=30"

# 表单数据格式
app "name=value age=30"

# JSON 格式
app '{"name":"value","age":30}'
```

## 全局标志

所有命令都可使用的内置全局标志：

| 标志 | 描述 |
|------|------|
| `--help, -h` | 显示帮助信息 |
| `--list-commands` | 列出所有可用命令 |
| `--list-flags` | 列出所有标志 |

## 测试

Redant 通过分离 I/O 使测试变得简单：

```go
func TestCommand(t *testing.T) {
    var stdout, stderr bytes.Buffer
    
    cmd := &redant.Command{
        Use: "test",
        Handler: func(ctx context.Context, inv *redant.Invocation) error {
            fmt.Fprintln(inv.Stdout, "Hello, World!")
            return nil
        },
    }
    
    inv := cmd.Invoke()
    inv.Stdout = &stdout
    inv.Stderr = &stderr
    
    err := inv.Run()
    if err != nil {
        t.Fatal(err)
    }
    
    if got := stdout.String(); got != "Hello, World!\n" {
        t.Errorf("got %q, want %q", got, "Hello, World!\n")
    }
}
```

## 文档

- [设计文档](docs/DESIGN.md) - 详细的架构和设计决策
- [评估报告](docs/EVALUATION.md) - 框架评估和改进建议
- [更新日志](docs/CHANGELOG.md) - 版本历史和变更记录
- [示例](example/) - 示例应用

## 示例

更多示例请查看 [example](example/) 目录：

- [echo](example/echo/) - 简单的 echo 命令
- [demo](example/demo/) - 功能演示
- [args-test](example/args-test/) - 参数格式测试
- [env-test](example/env-test/) - 环境变量测试
- [globalflags](example/globalflags/) - 全局标志用法

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。
