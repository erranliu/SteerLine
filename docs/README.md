# 文档入口

当前第一阶段支持与验收范围是本地 Codex 和本机 Orca，运行协议为 `worker-flow-v1`。任务/执行/产品控制台三线、适配资源、SQLite、应用宿主与测试目录整理已交付；版本线与二期规划仍在推进。

| 阅读目的 | 入口 |
| --- | --- |
| 当前目标与剩余任务 | [Roadmap](roadmap/roadmap.md) · [执行依赖](roadmap/execution-map.md) · [产品愿景](roadmap/product-vision.md) |
| 未交付规格与待补验收 | [Spec / Plan 索引](spec/README.md) |
| 当前模块归属与源码 | [代码架构](architecture/current-code-architecture.md) |
| 配置、数据和工作区 | [执行配置](execution-profiles.md) · [数据契约](data-contract.md) · [工作区准备](worktree-preparation.md) |
| 反馈、会话与活动 | [决策与反馈](decisions.md) · [会话事件](conversation-events.md) · [Activity](activity.md) |
| 业务权威与状态边界 | [Hosting](hosting.md) · [状态模型](state-model.md) |
| 测试入口与场景 | [测试指南](testing.md) · [分级 E2E](../tests/e2e/README.md) |
| 二期产品设计与原型 | [第二阶段](phase-2/README.md) |
| 已交付规格和历史规划 | [归档索引](archive/README.md) |
| 原始验证证据 | [validation/](validation/) |

## 目录维护

- `roadmap/` 只保留剩余规划及仍适用的产品约束；`task-ids/` 保留全部已分配身份，号码不复用。
- `spec/` 保留未交付 Spec/Plan、明确未闭合的定向验收和当前共同约束。
- 已执行的 Spec 与配套 Plan 一起移入 `archive/spec/completed/`；被替代的流程、远端探针及旧操作说明放入 `archive/spec/retired/` 或 `archive/guides/`。
- `validation/` 保留原证据。历史命令、固定路径、commit 和摘要属于原验证时点；归档移动不改写它们，也不改中心固定任务契约。
