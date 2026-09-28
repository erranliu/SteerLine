# Spec 与 Plan 索引

项目规格与实施计划统一存放在 `docs/spec/`。每项文档中的状态和验收条件说明其实际完成程度。执行所需约束、范围、依赖和验收条件必须在 Spec 体系中明确，Spec/Plan 不得引用或依赖规划文档。

## 控制台升级

依赖、产品行为和跨项目集成约束见 [控制台升级总览](console-upgrade.md)。按以下顺序实施并审核合并：

| 顺序 | 交付 | Spec | Plan |
| --- | --- | --- | --- |
| 1 | 统一观察层与读取一致性 | [Spec](pr-01-observation.spec.md) | [Plan](pr-01-observation.plan.md) |
| 2 | 服务控制协议与定向执行 | [Spec](pr-02-control.spec.md) | [Plan](pr-02-control.plan.md) |
| 3 | 服务窗口生命周期与连接 | [Spec](pr-03-service-lifecycle.spec.md) | [Plan](pr-03-service-lifecycle.plan.md) |
| 4A | 长期双执行后端契约 | [Spec](pr-04a-execution-backends.spec.md) | [Plan](pr-04a-execution-backends.plan.md) |
| 4B | 统一会话事件与完整回复 | [Spec](pr-04b-conversation-events.spec.md) | [Plan](pr-04b-conversation-events.plan.md)；[验证报告](../validation/pr-04b-conversation-events-2026-09-24.md) |
| 4C | 执行控制台与轮次折叠 | [Spec](pr-04-console.spec.md) | [Plan](pr-04-console.plan.md) |
| 5 | 人工决策与恢复闭环 | [Spec](pr-05-intervention.spec.md) | [Plan](pr-05-intervention.plan.md) |

共七个串行交付阶段，阶段名不是 GitHub PR 编号。PR 3 已以 Windows 可见服务窗口方案交付；原 PR4 路径保留为 4C。架构衔接见 [共同约束 Spec](execution-architecture.spec.md) 和各阶段 Spec。后续阶段按 4A → 4B → 4C → 5 推进。

## 中心状态机与远程执行

共同执行约束见 [执行架构 Spec](execution-architecture.spec.md)。

1. [Orca 远程能力验证与执行协议](01-orca-remote-protocol.md) · [Plan](01-orca-remote-protocol.plan.md)
2. [任务、执行模型、最小状态机与执行适配器](02-execution-model-and-adapter.md) · [Plan](02-execution-model-and-adapter.plan.md)
3. [最小状态机的分阶段远程闭环与 viewer 人工决策](03-remote-execution-and-viewer.md) · [Plan](03-remote-execution-and-viewer.plan.md)

## 原生任务与固定 DAG

共同要求见 [原生任务共同约束](native-task-constraints.md)。每个任务对应一个独立 Spec、配套 Plan 和一个独立实现 PR，不把多个任务藏在同一总文件的章节中。

| 任务 | Spec | Plan |
| --- | --- | --- |
| TASK-CORE：原生任务模型 | [Spec](task-core.spec.md) | [Plan](task-core.plan.md) |
| TASK-DAG：固定依赖调度 | [Spec](task-dag.spec.md) | [Plan](task-dag.plan.md) |
| BEADS-EXIT：解除 Beads 依赖与原生接入 | [Spec](beads-exit.spec.md) | [Plan](beads-exit.plan.md) |
| DAG-VIEW：任务关系视图与 JSON 导出 | [Spec](dag-view.spec.md) | [Plan](dag-view.plan.md) |
| #25 TASK-DAG：已建任务追加前置与安全重置 | [Spec](native-task-add-predecessors.spec.md) | [Plan](native-task-add-predecessors.plan.md) |

按表中顺序分别交付，复用中心执行模型，不等待整个远程闭环。系统不提供任务拆分或回退流程，两者由用户人工处置；共同文档不代替各任务的执行范围与验收。


## 第一阶段多候选与版本交付

[第一阶段共同约束](phase1-delivery-constraints.md)定义交接、版本与证据边界。以下任务已中心创建，尚未实现或验收；执行前须先集成中心固定引用的规格提交。每项独立 Spec/Plan 对应一个实现 PR。

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | 无未完成硬前置 | [Spec](task-candidates.spec.md) | [Plan](task-candidates.plan.md) |
| 27 | PR 胜出与落选候选对账 | `#26` | [Spec](candidate-delivery.spec.md) | [Plan](candidate-delivery.plan.md) |
| 28 | 候选观察界面 | `#27` | [Spec](candidate-console.spec.md) | [Plan](candidate-console.plan.md) |
| 29 | 多候选本机与远端真机验收 | `#28`、`#3`、`#4`、`#6` | [Spec](candidate-native-acceptance.spec.md) | [Plan](candidate-native-acceptance.plan.md) |
| 30 | 固定版本与双入口 | 无未完成硬前置 | [Spec](versioned-runtime.spec.md) | [Plan](versioned-runtime.plan.md) |
| 31 | dev 隔离与 stable 提升 | `#30` | [Spec](runtime-isolation-promotion.spec.md) | [Plan](runtime-isolation-promotion.plan.md) |
| 32 | 第一阶段集成验收 | `#29`、`#31` | [Spec](phase1-release-acceptance.spec.md) | [Plan](phase1-release-acceptance.plan.md) |

## 第二阶段原生 UI 与受控规划

[共同接口与边界](phase2-constraints.md)只承载共同要求；下列每个任务各有独立 Spec、Plan 和实现 PR。未实现/未验收；硬前置均以真实任务交付为准。`#14–#24` 已进入中心 DAG，#16/#24 后续受控追加的前置只收紧开工资格，未改原 Spec/Plan 的固定 Git 引用。

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | 无未完成硬前置 | [Spec](phase2-agent-capabilities.spec.md) | [Plan](phase2-agent-capabilities.plan.md) |
| 15 | WinUI 基座与原生输入 | 无未完成硬前置 | [Spec](phase2-winui-foundation.spec.md) | [Plan](phase2-winui-foundation.plan.md) |
| 16 | 本机客户端协议与历史查询 | `#26`（中心追加） | [Spec](phase2-local-api.spec.md) | [Plan](phase2-local-api.plan.md) |
| 17 | 规划提案与受控草稿 | `#16` | [Spec](phase2-planning-drafts.spec.md) | [Plan](phase2-planning-drafts.plan.md) |
| 18 | Codex 规划会话服务 | `#14`、`#17` | [Spec](phase2-planning-sessions.spec.md) | [Plan](phase2-planning-sessions.plan.md) |
| 19 | 正式文档发布与任务创建 | `#17` | [Spec](phase2-document-publish.spec.md) | [Plan](phase2-document-publish.plan.md) |
| 20 | Roadmap 与 DAG 原生浏览 | `#15`、`#19` | [Spec](phase2-roadmap-ui.spec.md) | [Plan](phase2-roadmap-ui.plan.md) |
| 21 | 执行与待处理原生界面 | `#15`、`#16` | [Spec](phase2-execution-ui.spec.md) | [Plan](phase2-execution-ui.plan.md) |
| 22 | 规划对话原生界面 | `#15`、`#18` | [Spec](phase2-conversation-ui.spec.md) | [Plan](phase2-conversation-ui.plan.md) |
| 23 | 文档审阅与确认原生界面 | `#15`、`#19` | [Spec](phase2-review-ui.spec.md) | [Plan](phase2-review-ui.plan.md) |
| 24 | 第二阶段完整闭环验收 | `#20`、`#21`、`#22`、`#23`、`#3`、`#4`、`#6`，另有中心追加的 `#32` | [Spec](phase2-integration.spec.md) | [Plan](phase2-integration.plan.md) |

## 现有 Hosting 实现

- [Hosting 规范 v2：Orca 驱动执行](hosting-spec.md)
- [Hosting 边界与目标架构说明](../hosting.md)
