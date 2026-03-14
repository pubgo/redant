# Redant 设计文档

## 1. 设计目标

Redant 的目标是提供一套可组合、可测试、可扩展的命令行框架，重点解决以下问题：

- 复杂命令树的统一管理
- 多来源参数（标志、环境变量、默认值）的一致解析
- 命令执行链路的可观测与可扩展
- 帮助信息、补全、测试支持的一体化

> 关联文档：[`README`](../README.md) · [`评估报告`](EVALUATION.md) · [`变更日志`](CHANGELOG.md)

## 2. 总体架构

```mermaid
flowchart LR
    I[用户输入 argv] --> R[命令路由层]
    R --> O[选项与参数解析层]
    O --> M[中间件编排层]
    M --> H[处理器执行层]
    H --> X[输出/错误/退出码]

    R --> C1[命令树]
    O --> C2[Option 与 Arg]
    M --> C3[Middleware]
    H --> C4[Handler]
```

## 3. 命令解析流程

```mermaid
flowchart TD
    A[接收输入参数] --> B[初始化命令树]
    B --> C[构建命令映射]
    C --> D{是否命中显式子命令}
    D -- 是 --> E[消费命令参数]
    D -- 否 --> F{是否命中 argv0}
    F -- 是 --> G[按 argv0 选择子命令]
    F -- 否 --> H[保留根命令]
    E --> I[合并全局与局部标志]
    G --> I
    H --> I
    I --> J[解析标志与参数]
    J --> K[执行中间件链]
    K --> L[执行 Handler]
```

### 3.1 参数解析细分流程

```mermaid
flowchart TD
    A[进入参数解析阶段] --> B{RawArgs 是否开启}
    B -- 是 --> C[跳过标志解析并保留原始参数]
    B -- 否 --> D[使用 pflag 解析标志]
    D --> E[得到 parsedArgs]
    C --> F[参数格式识别]
    E --> F
    F --> G{参数是否包含 '='}
    G -- 否 --> H[位置参数处理]
    G -- 是 --> I{是否包含 '&'}
    I -- 是 --> J[按查询串解析]
    I -- 否 --> K{是否包含空格}
    K -- 是 --> L[按表单解析]
    K -- 否 --> M{是否 JSON 对象/数组}
    M -- 是 --> N[按 JSON 解析]
    M -- 否 --> O[作为普通键值参数]
    H --> P[写入 inv.Args / ArgSet]
    J --> P
    L --> P
    N --> P
    O --> P
    P --> Q[执行必填项与类型校验]
    Q --> R[交给中间件链与 Handler]
```

关键点：

- 参数解析发生在命令定位与标志合并之后。
- `RawArgs=true` 时，命令自行处理参数；框架不做常规标志解析。
- 对于复杂参数场景，建议在处理器中显式调用 `ParseQueryArgs`、`ParseFormArgs`、`ParseJSONArgs`。

### 3.2 解析优先级规则

```mermaid
flowchart TD
    A[命令输入] --> B{显式子命令是否匹配}
    B -- 是 --> C[选择显式子命令]
    B -- 否 --> D{argv0 是否匹配命令/别名}
    D -- 是 --> E[选择 argv0 对应子命令]
    D -- 否 --> F[使用根命令]
    C --> G[合并并解析标志]
    E --> G
    F --> G
    G --> H[参数形态识别与解析]
    H --> I[校验与执行]
```

规则摘要：

1. 显式子命令优先于 `argv0`。
2. `argv0` 仅在未显式指定子命令时生效。
3. 命令最终确定后，才进入标志解析与参数解析阶段。

## 4. 执行状态机

```mermaid
stateDiagram-v2
    [*] --> 初始化
    初始化 --> 解析命令
    解析命令 --> 解析标志
    解析标志 --> 校验参数
    校验参数 --> 执行中间件
    执行中间件 --> 执行处理器
    执行处理器 --> 成功
    执行处理器 --> 失败
    成功 --> [*]
    失败 --> [*]
```

## 5. 模块职责

| 模块           | 主要文件               | 说明                               |
| -------------- | ---------------------- | ---------------------------------- |
| 命令系统       | `command.go`           | 命令树、命令查找、执行流程         |
| 选项系统       | `option.go`            | 标志定义、FlagSet 构建             |
| 参数系统       | `args.go`              | 多格式参数解析（查询串/表单/JSON） |
| 值类型系统     | `flags.go`             | 自定义 `pflag.Value` 类型集合      |
| 帮助系统       | `help.go` / `help.tpl` | 帮助渲染、命令与标志展示           |
| 中间件与处理器 | `handler.go`           | 执行链组装与业务回调               |

## 6. Busybox 风格 argv0 分发

```mermaid
flowchart TD
    A[程序启动] --> B{参数中是否明确子命令}
    B -- 是 --> C[按参数子命令执行]
    B -- 否 --> D[读取 argv0]
    D --> E{argv0 是否映射到命令/别名}
    E -- 是 --> F[按 argv0 命令执行]
    E -- 否 --> G[执行根命令或帮助]
    C --> H[统一执行链]
    F --> H
    G --> H
```

说明：

- 显式子命令优先于 argv0。
- argv0 支持命令名与别名。
- 行为用于软链接入口场景，便于将子命令暴露为独立命令。

### 6.1 部署与运行时序（构建 + 软连接）

```mermaid
sequenceDiagram
    participant Dev as 开发者
    participant FS as 文件系统
    participant Bin as app 二进制
    participant RT as 运行时分发器

    Dev->>Bin: 构建生成 app
    Dev->>FS: ln -sf app echo
    Dev->>FS: 执行 ./echo hello
    FS->>Bin: 启动 app（argv0=echo）
    Bin->>RT: 传入 argv0 与参数
    RT->>RT: 检查 argv0 是否匹配命令/别名
    alt 匹配成功
        RT->>RT: 选择对应子命令
        RT-->>Dev: 执行子命令结果
    else 未匹配
        RT->>RT: 回退到显式参数解析或根命令
        RT-->>Dev: 帮助信息或默认执行结果
    end
```

## 7. 扩展点

- 自定义值类型：实现 `pflag.Value`。
- 自定义中间件：包装 `HandlerFunc` 实现统一鉴权、日志、超时控制。
- 自定义帮助模板：修改 `help.tpl`。
- 新增子命令：扩展 `Command.Children`。

## 8. 文档关联

- 上游：[`README`](../README.md) 提供入口与使用视图。
- 同级：[`EVALUATION.md`](EVALUATION.md) 提供质量视图。
- 下游：[`../example/args-test/README.md`](../example/args-test/README.md) 提供参数解析落地示例。
