---
name: add-command
description: "Add a new subcommand to a redant CLI app. Use when: adding subcommand, creating CLI command, new command with flags and options, extending command tree, adding middleware."
argument-hint: "命令名称，如 deploy"
---

# 添加子命令

## 使用场景

- 给现有 redant CLI 应用添加新的子命令
- 需要带选项、参数、中间件的完整命令

## 操作步骤

### 1. 确定命令位置

子命令通过 `Children` 挂载。确认挂载到哪个父命令下：

```go
// 一级子命令
rootCmd.Children = append(rootCmd.Children, newCmd)

// 二级子命令
parentCmd.Children = append(parentCmd.Children, newCmd)
```

### 2. 定义命令

#### 最小命令（仅 Handler）

```go
cmd := &redant.Command{
	Use:   "<name>",
	Short: "<描述>",
	Handler: func(ctx context.Context, inv *redant.Invocation) error {
		fmt.Fprintln(inv.Stdout, "done")
		return nil
	},
}
```

#### 带选项的命令

```go
var (
	output string
	force  bool
	count  int64
)

cmd := &redant.Command{
	Use:   "<name> [args...]",
	Short: "<描述>",
	Options: redant.OptionSet{
		{
			Flag:        "output",
			Shorthand:   "o",
			Description: "输出格式",
			Default:     "text",
			Value:       redant.EnumOf(&output, "text", "json", "yaml"),
		},
		{
			Flag:        "force",
			Shorthand:   "f",
			Description: "强制执行",
			Value:       redant.BoolOf(&force),
		},
		{
			Flag:        "count",
			Description: "数量",
			Value:       redant.Int64Of(&count),
			Default:     "10",
			Envs:        []string{"APP_COUNT"},  // 环境变量回退
			Required:    true,                    // 必填
		},
	},
	Handler: func(ctx context.Context, inv *redant.Invocation) error {
		fmt.Fprintf(inv.Stdout, "output=%s force=%v count=%d args=%v\n",
			output, force, count, inv.Args)
		return nil
	},
}
```

#### 带中间件的命令

```go
cmd := &redant.Command{
	Use:   "<name>",
	Short: "<描述>",
	Middleware: redant.Chain(
		func(next redant.HandlerFunc) redant.HandlerFunc {
			return func(ctx context.Context, inv *redant.Invocation) error {
				// 前置逻辑
				fmt.Fprintln(inv.Stderr, "middleware: before")
				err := next(ctx, inv)
				// 后置逻辑
				fmt.Fprintln(inv.Stderr, "middleware: after")
				return err
			}
		},
	),
	Handler: func(ctx context.Context, inv *redant.Invocation) error {
		return nil
	},
}
```

#### 带位置参数定义

```go
cmd := &redant.Command{
	Use:   "<name> <file> [message]",
	Short: "<描述>",
	Args: redant.ArgSet{
		{Name: "file", Description: "目标文件", Value: redant.StringOf(new(string))},
		{Name: "message", Description: "提交信息", Value: redant.StringOf(new(string))},
	},
	Handler: func(ctx context.Context, inv *redant.Invocation) error {
		// inv.Args[0] = file, inv.Args[1] = message
		return nil
	},
}
```

### 3. 可用值类型速查

| 类型 | 构造函数 | 示例 |
|---|---|---|
| 字符串 | `redant.StringOf(&s)` | `--name alice` |
| 布尔 | `redant.BoolOf(&b)` | `--verbose` |
| 整数 | `redant.Int64Of(&n)` | `--count 5` |
| 浮点 | `redant.Float64Of(&f)` | `--rate 0.95` |
| 时长 | `redant.DurationOf(&d)` | `--timeout 30s` |
| URL | `redant.URLOf(&u)` | `--endpoint https://...` |
| 枚举 | `redant.EnumOf(&s, "a","b","c")` | `--level info` |
| 枚举数组 | `redant.EnumArrayOf(&ss, "a","b")` | `--tags feat,fix` |
| 字符串数组 | `redant.StringArrayOf(&ss)` | `--files a.go,b.go` |
| 正则 | `&redant.Regexp{}` | `--pattern ^feat:` |
| Host:Port | `&redant.HostPort{}` | `--addr 127.0.0.1:8080` |
| 结构体 | `&redant.Struct[T]{Value: T{}}` | JSON/YAML 输入 |

### 4. 选项字段完整参考

```go
redant.Option{
	Flag:        "name",          // 长标志名（必填）
	Shorthand:   "n",             // 短标志名
	Description: "说明文字",       // 帮助文本
	Default:     "value",         // 默认值（字符串形式）
	Envs:        []string{"KEY"}, // 环境变量回退列表
	Required:    true,            // 是否必填
	Hidden:      true,            // 帮助中隐藏
	Deprecated:  "use --new",     // 弃用提示
	Value:       redant.StringOf(&s), // 值绑定
}
```

## 命名规约

- `Use` 首词即命令名：`Use: "deploy [env]"` → 命令名 `deploy`
- 支持别名：`Aliases: []string{"dp"}`
- 子命令可用空格或冒号调用：`app repo commit` = `app repo:commit`
