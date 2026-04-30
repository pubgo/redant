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

## 变更

- 移除全局 `--env` / `-e` / `--env-file` 标志及 `env_preload.go` 环境预加载模块，避免与业务命令自定义同名标志冲突。选项级 `Envs` 环境变量回退机制不受影响。

## 文档

- 更新 `docs/DOCS_CATALOG.md`，新增"可视化与文档生成"分类。
- fastcommit 示例集成 `vizcmd` 和 `doccmd` 命令。