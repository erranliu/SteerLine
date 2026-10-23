# 退役流程与远端试验

这些资料描述旧 `minimal-v1`、中心步骤编排、早期 hosting 和远端协议实验，已经被当前本机持续 worker 范围替代。归档表示退出当前执行入口，不表示原列出的全部验收、远端权限、取消或故障门槛完成。

当前运行说明见[数据契约](../../../../docs/product-design/data-model.md)、[Hosting](../../../../docs/architecture.md)和[当前 Spec](../../../spec/README.md)。原版本、命令、状态和证据保持历史含义。

| Spec / 记录 | 配套 Plan |
| --- | --- |
| [Spec 01：Orca 远程能力验证与执行协议](01-orca-remote-protocol.md) | [Plan](01-orca-remote-protocol.plan.md) |
| [Spec 02：任务、执行模型、最小状态机与执行适配器](02-execution-model-and-adapter.md) | [Plan](02-execution-model-and-adapter.plan.md) |
| [Spec 03：最小状态机的分阶段远程闭环与 viewer 人工决策](03-remote-execution-and-viewer.md) | [Plan](03-remote-execution-and-viewer.plan.md) |
| [执行架构 Spec：共同约束](execution-architecture.spec.md) | — |
| [Native local and Orca execution](hosting-spec.md) | — |
| [原 #42 一期集成验收](phase1-release-acceptance.spec.md) | [原 Plan](phase1-release-acceptance.plan.md) |
| [原 #53 二期完整闭环验收](phase2-integration.spec.md) | [原 Plan](phase2-integration.plan.md) |

原 #42/#53 是人工组织为主的阶段验收，不再作为执行任务候选；ID 保留历史且不复用。现行自动/人工行为和状态分别由[一期](../../../testing/acceptance/phase1.md)和[二期](../../../testing/acceptance/phase2.md)验收文档维护，归档不取消外部 SteerLine 已有任务。

[Orca 协议探针与原始样本](orca-probe/README.md)

## 2026-10-13 清理的二阶段与延期规格

以下均已退出当前规划，不作为待重定队列或前置。编号不复用；[任务 #91](../../../spec/tui-http-client.spec.md)已交付并删除 #44 工程，TUI 随后退役。当前执行以[正式清单](../../../Roadmap/execution-map.md)为准，历史交付与原未验不改称通过。

| 原归属 / Spec | 原 Plan |
| --- | --- |
| [#40 固定版本与双入口](versioned-runtime.spec.md) | [Plan](versioned-runtime.plan.md) |
| [#41 dev 隔离与 stable 提升](runtime-isolation-promotion.spec.md) | [Plan](runtime-isolation-promotion.plan.md) |
| [#43 主 Agent 能力探测](phase2-agent-capabilities.spec.md) | [Plan](phase2-agent-capabilities.plan.md) |
| [#44 WinUI 基座](phase2-winui-foundation.spec.md) | [Plan](phase2-winui-foundation.plan.md) |
| [#46 规划草稿](phase2-planning-drafts.spec.md) | [Plan](phase2-planning-drafts.plan.md) |
| [#47 规划会话](phase2-planning-sessions.spec.md) | [Plan](phase2-planning-sessions.plan.md) |
| [#48 文档发布](phase2-document-publish.spec.md) | [Plan](phase2-document-publish.plan.md) |
| [#49 DAG 页面](phase2-roadmap-ui.spec.md) | [Plan](phase2-roadmap-ui.plan.md) |
| [#50 执行页面](phase2-execution-ui.spec.md) | [Plan](phase2-execution-ui.plan.md) |
| [#51 规划对话页面](phase2-conversation-ui.spec.md) | [Plan](phase2-conversation-ui.plan.md) |
| [#52 文档审阅页面](phase2-review-ui.spec.md) | [Plan](phase2-review-ui.plan.md) |

## 2026-10-21 Orca 历史方案归档

| 原方案 | 文档 | 退役原因与原证据 |
| --- | --- | --- |
| 本机原生通用 worker | [Spec](orca-native-worker.spec.md) · [Plan](orca-native-worker.plan.md) | 旧 worker-start/Dispatch 已删除；[原验收](../../../testing/acceptance/archive/superseded/orca-native-worker.md) |
| 本机原生编排 | [Spec](worker-flow-orca-native.spec.md) · [Plan](worker-flow-orca-native.plan.md) | 原 ON 场景随路线退役，未验保留；[原验收](../../../testing/acceptance/archive/superseded/worker-flow-orca-native.md) |
| supervised 分步结果 | [Spec](orca-supervised-results.spec.md) · [Plan](orca-supervised-results.plan.md) | payload/supervised 路线已替代；[原验收](../../../testing/acceptance/archive/superseded/orca-supervised-results.md) |
| supervised 双目标 | [Plan](orca-supervised-targets.plan.md) | 旧实现退役；[原验收](../../../testing/acceptance/archive/superseded/orca-supervised-targets.md) |
| 本机输入与就绪 | [Spec](orca-local-input-readiness.spec.md) · [Plan](orca-local-input-readiness.plan.md) | 本机 Orca 已放弃，有限通过样本仍有效于原版本；[原验收](../../../testing/acceptance/archive/superseded/orca-local-input-readiness.md) |

当前远端 Orca 经独立 Runner 接入，见[接入说明](../../../../docs/integration/steerrunner.md)。归档不宣告历史失败、取消或未验通过，也不要求恢复旧路线。
