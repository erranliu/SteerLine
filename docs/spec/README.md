# Spec 与 Plan 索引

当前活动规划按下表执行，每项一份 Spec/Plan 和一个实现 PR。规格/计划不反向依赖 roadmap。新流程由 #36 固定，旧五步、项目验证和应用证明不作为新任务门槛。

## 当前执行规格

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 36 | 单 worker 持续执行与统一待决策 | 无 | [Spec](worker-flow.spec.md) | [Plan](worker-flow.plan.md) |
| 37 | 多候选会话与唯一PR交付 | `#36` | [Spec](candidate-workers.spec.md) | [Plan](candidate-workers.plan.md) |
| 38 | 候选观察界面 | `#37` | [Spec](candidate-console.spec.md) | [Plan](candidate-console.plan.md) |
| 39 | 多候选本机与远端真机验收 | `#38` | [Spec](candidate-native-acceptance.spec.md) | [Plan](candidate-native-acceptance.plan.md) |
| 40 | 固定版本与双入口 | 无 | [Spec](versioned-runtime.spec.md) | [Plan](versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | `#40`、`#36` | [Spec](runtime-isolation-promotion.spec.md) | [Plan](runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | `#39`、`#41` | [Spec](phase1-release-acceptance.spec.md) | [Plan](phase1-release-acceptance.plan.md) |
| 43 | Codex 规划主 agent 能力验证 | 无 | [Spec](phase2-agent-capabilities.spec.md) | [Plan](phase2-agent-capabilities.plan.md) |
| 44 | WinUI 基座与原生输入 | 无 | [Spec](phase2-winui-foundation.spec.md) | [Plan](phase2-winui-foundation.plan.md) |
| 45 | 本机客户端协议与历史查询 | `#36` | [Spec](phase2-local-api.spec.md) | [Plan](phase2-local-api.plan.md) |
| 46 | 规划提案与受控草稿 | `#45` | [Spec](phase2-planning-drafts.spec.md) | [Plan](phase2-planning-drafts.plan.md) |
| 47 | Codex 规划会话服务 | `#43`、`#46` | [Spec](phase2-planning-sessions.spec.md) | [Plan](phase2-planning-sessions.plan.md) |
| 48 | 正式文档发布与任务创建 | `#46` | [Spec](phase2-document-publish.spec.md) | [Plan](phase2-document-publish.plan.md) |
| 49 | Roadmap 与 DAG 原生浏览 | `#44`、`#48` | [Spec](phase2-roadmap-ui.spec.md) | [Plan](phase2-roadmap-ui.plan.md) |
| 50 | 执行与待处理原生界面 | `#44`、`#45` | [Spec](phase2-execution-ui.spec.md) | [Plan](phase2-execution-ui.plan.md) |
| 51 | 规划对话原生界面 | `#44`、`#47` | [Spec](phase2-conversation-ui.spec.md) | [Plan](phase2-conversation-ui.plan.md) |
| 52 | 文档审阅与确认原生界面 | `#44`、`#48` | [Spec](phase2-review-ui.spec.md) | [Plan](phase2-review-ui.plan.md) |
| 53 | 第二阶段完整闭环验收 | `#49`、`#50`、`#51`、`#52`、`#42` | [Spec](phase2-integration.spec.md) | [Plan](phase2-integration.plan.md) |

## 本机 Orca 接入补充

[本机 Orca 原生编排 Spec](worker-flow-orca-native.spec.md) / [Plan](worker-flow-orca-native.plan.md) 承接 #36 延期的本机 Orca 接入。状态：待实施，尚未分配 Task ID；Spec 内记录 2026-09-29 的平台实验、低层 Dispatch 选型及中心验收边界。当前功能支持状态以已交付实现为准。

## worker 环境就绪与宿主恢复

[worker 环境就绪与宿主阻断恢复 Spec](worker-environment-readiness.spec.md) / [Plan](worker-environment-readiness.plan.md)。状态：实现候选，真机服务闭环待验证；未分配 Task ID，一个实现 PR 交付。[验收记录](../validation/worker-environment-readiness/README.md)。补充本机 Orca + Codex 的冻结权限、原会话工具探针、独立宿主等待观察与原身份恢复，沿用 #36 的五主阶段及原生编排边界。

## 架构审计收口（3 个 PR）

状态：PR 1 实现候选，PR 2/3 待实施；未分配 Task ID。三份 Spec 按下表顺序各用一个实现 PR 交付。审计基线为 `main@4e786d7`；已合并的具体存储、typed transaction 和固定执行路由改动继续复用。验证遵循项目的最小必要范围。

| 顺序 | 交付范围 | 硬前置 | Spec |
| --- | --- | --- | --- |
| PR 1 | worker 协议事件与轮次终态边界 | 无未完成前置 | [worker 协议与终态边界](architecture-01-worker-protocol.spec.md) |
| PR 2 | worker 正常生命周期显式原子提交 | PR 1 合入 | [worker 生命周期显式原子提交](architecture-02-worker-commit.spec.md) |
| PR 3 | 剩余写者迁移、删除隐式写回、读写模型与后端接口收口 | PR 2 合入 | [移除隐式写回与读写模型收口](architecture-03-runtime-boundary.spec.md) |

## 共同约束

- [执行架构](execution-architecture.spec.md)与[原生任务](native-task-constraints.md)：身份、单写者、当前格式恢复和真实交付；被 #36 替代的旧执行语义不继续要求。
- [第一阶段](phase1-delivery-constraints.md)：持续会话、多候选、版本与验收交接。
- [第二阶段](phase2-constraints.md)：受控规划、正式文档、UI 和公共传输。

## 既有实现与历史基线

以下文档保留已交付能力及历史验收定位，不能作为重新引入旧步骤流程的入口：

- [原生任务模型](task-core.spec.md)、[固定 DAG](task-dag.spec.md)、[原生接入](beads-exit.spec.md)、[关系视图](dag-view.spec.md)、[追加前置](native-task-add-predecessors.spec.md)。
- [双执行后端](pr-04a-execution-backends.spec.md)、[会话事件](pr-04b-conversation-events.spec.md)、[服务生命周期](pr-03-service-lifecycle.spec.md)。
- [旧中心模型](02-execution-model-and-adapter.md)、[旧远端闭环](03-remote-execution-and-viewer.md)、[旧人工决策](pr-05-intervention.spec.md)、[旧控制台](pr-04-console.spec.md)。
- [Hosting](hosting-spec.md)与[边界说明](../hosting.md)。
