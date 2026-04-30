# [Unreleased]

> 推荐维护方式：
>
> - 使用 LLM 提示词自动更新：[`docs/CHANGELOG_LLM_PROMPT.md`](../../docs/CHANGELOG_LLM_PROMPT.md)
> - 建议通过 agent 提示词执行：`/changelog-maintenance draft|release`

## 新增

- 新增 `cmds/vizcmd/` 命令组：支持生成命令树结构图（`viz tree`）、命令分发流程图（`viz dispatch`）、MCP 调用时序图（`viz mcp-sequence`），输出 Mermaid 格式。
- 新增 `cmds/doccmd/` 交互式文档站命令（`doc`）：从命令树自动生成类 Swagger UI 的浏览界面，集成 Mermaid 图渲染、命令搜索、参数/选项表格。
- `WriteTree` / `WriteDispatch` / `WriteMCPSequence` 导出函数，可供外部集成复用。

## 修复

暂无

## 优化

- `OptionSet.FlagSet()` 对默认值和环境变量值 `Set` 失败时输出 warning 到 stderr，不再静默忽略。

## 变更

- 移除全局 `--env` / `-e` / `--env-file` 标志及 `env_preload.go` 环境预加载模块，避免与业务命令自定义同名标志冲突。选项级 `Envs` 环境变量回退机制不受影响。

## 文档

- 更新 `docs/DOCS_CATALOG.md`，新增"可视化与文档生成"分类。
- fastcommit 示例集成 `vizcmd` 和 `doccmd` 命令。- README 改为“三步式黄金路径”快速开始（最小命令 → 加标志 → 加子命令）。
- 示例目录新增 [example/README.md](../../example/README.md) 分层索引（入门 → 基础 → 进阶 → 综合）。
- 弱化 `--args` 内部标志在文档中的可见度，明确标注为框架内部保留标志。
- `ParseJSONArgs` 错误提示改为包含格式示例的可操作性信息。
- 示例 `echo`、`unary`、`stream-interactive` 补充 `Long` 详细描述与用法示例。