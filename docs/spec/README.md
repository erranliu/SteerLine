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
