---
name: new-cli-app
description: "Scaffold a new CLI application using the redant framework. Use when: creating new CLI app, starting new project with redant, initializing command-line tool, bootstrapping Go CLI."
argument-hint: "应用名称，如 myctl"
---

# 用 redant 创建 CLI 应用

## 使用场景

- 从零开始用 redant 构建 CLI 工具
- 需要一个带命令树、选项、帮助系统的 Go CLI 骨架

## 操作步骤

### 1. 初始化 Go 模块

```bash
mkdir <app-name> && cd <app-name>
go mod init <module-path>
go get github.com/pubgo/redant@latest
```

### 2. 创建 main.go

按以下模板生成入口文件，包含根命令 + 一个示例子命令：

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pubgo/redant"
)

func main() {
	rootCmd := &redant.Command{
		Use:   "<app-name>",
		Short: "<一句话描述>",
	}

	// 示例子命令
	var name string
	helloCmd := &redant.Command{
		Use:   "hello [name]",
		Short: "打招呼示例",
		Options: redant.OptionSet{
			{
				Flag:        "name",
				Shorthand:   "n",
				Description: "目标名称",
				Default:     "world",
				Value:       redant.StringOf(&name),
			},
		},
		Handler: func(ctx context.Context, inv *redant.Invocation) error {
			if len(inv.Args) > 0 {
				name = inv.Args[0]
			}
			fmt.Fprintf(inv.Stdout, "Hello, %s!\n", name)
			return nil
		},
	}

	rootCmd.Children = append(rootCmd.Children, helloCmd)

	inv := rootCmd.Invoke().WithOS()
	if err := inv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

### 3. 可选：集成扩展能力

按需添加以下模块（import 对应包并追加到 `Children`）：

| 能力 | 包 | 用法 |
|---|---|---|
| Shell 补全 | `cmds/completioncmd` | `completioncmd.New(rootCmd)` |
| MCP Server | `cmds/mcpcmd` | `mcpcmd.New(rootCmd)` |
| 交互式命令行 | `cmds/readlinecmd` | `readlinecmd.New(rootCmd)` |
| 富交互命令行 | `cmds/richlinecmd` | `richlinecmd.New(rootCmd)` |
| Web UI | `cmds/webcmd` | `webcmd.New(rootCmd)` |
| WebTTY | `cmds/webttycmd` | `webttycmd.New(rootCmd)` |

示例（集成补全 + MCP）：

```go
import (
	"github.com/pubgo/redant/cmds/completioncmd"
	"github.com/pubgo/redant/cmds/mcpcmd"
)

rootCmd.Children = append(rootCmd.Children,
	completioncmd.New(rootCmd),
	mcpcmd.New(rootCmd),
)
```

### 4. 验证

```bash
go run . --help
go run . hello --name redant
```

## 核心 API 概览

| 概念 | 类型 | 说明 |
|---|---|---|
| 命令 | `redant.Command` | Use / Short / Children / Options / Handler |
| 选项 | `redant.OptionSet` | Flag / Shorthand / Default / Envs / Value |
| 值类型 | `redant.StringOf` 等 | String / Bool / Int64 / Enum / Struct / Regexp ... |
| 参数 | `redant.ArgSet` | 位置参数定义 |
| 中间件 | `redant.Chain(...)` | Handler 拦截链 |
| 调用 | `cmd.Invoke(...).WithOS().Run()` | 构造 Invocation 并运行 |

## 参考

- 完整示例见 `example/` 目录
- API 用法速览见 `docs/USAGE_AT_A_GLANCE.md`
