# Spec 与 Plan 索引

当前活动规划按下表执行，每项一份 Spec/Plan 和一个实现 PR。规格/计划不反向依赖 roadmap。新流程由 #36 固定，旧五步、项目验证和应用证明不作为新任务门槛。

## 当前执行规格

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 36 | 单 worker 持续执行与统一待决策 | 无 | [Spec](worker-flow.spec.md) | [Plan](worker-flow.plan.md) |
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

## 任务执行控制台解耦与目录整理

目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。每项一份独立 Spec/Plan 和实现 PR；共同边界见 [三线解耦共同实现约束](three-line-constraints.spec.md)。#73 可与 #38 并行；#77 等 #38；#39 按固定构建与实际影响定向复核，不增加重构总前置。

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 73 | 纯类型与契约归属整理 | `#37` | [Spec](three-line-01-types.spec.md) | [Plan](three-line-01-types.plan.md) |
| 74 | 任务选择与执行结果边界 | `#73` | [Spec](three-line-02-task-execution-boundary.spec.md) | [Plan](three-line-02-task-execution-boundary.plan.md) |
| 75 | 任务应用用例归包 | `#74` | [Spec](three-line-03-task-usecases.spec.md) | [Plan](three-line-03-task-usecases.plan.md) |
| 76 | 执行应用用例归包 | `#74` | [Spec](three-line-04-execution-usecases.spec.md) | [Plan](three-line-04-execution-usecases.plan.md) |
| 77 | 产品查询与控制台契约收窄 | `#74`、`#38` | [Spec](three-line-05-product-console.spec.md) | [Plan](three-line-05-product-console.plan.md) |
| 78 | 执行适配与资源目录归组 | `#76` | [Spec](three-line-06-adapters-resources.spec.md) | [Plan](three-line-06-adapters-resources.plan.md) |
| 79 | 存储目录与消费者端口收口 | `#75`、`#76`、`#77` | [Spec](three-line-07-storage-ports.spec.md) | [Plan](three-line-07-storage-ports.plan.md) |
| 80 | 应用组装与服务宿主收口 | `#75`、`#76`、`#77`、`#78`、`#79` | [Spec](three-line-08-app-host.spec.md) | [Plan](three-line-08-app-host.plan.md) |
| 81 | 测试支持与集成目录整理 | `#80` | [Spec](three-line-09-testkit-integration.spec.md) | [Plan](three-line-09-testkit-integration.plan.md) |
| 82 | 独立开发证明与最终清理 | `#81` | [Spec](three-line-10-independence-cleanup.spec.md) | [Plan](three-line-10-independence-cleanup.plan.md) |

## 本机 Orca 接入补充

[本机 Orca 原生编排 Spec](worker-flow-orca-native.spec.md) / [Plan](worker-flow-orca-native.plan.md) 承接 #36 延期的本机 Orca 接入。状态：待实施，尚未分配 Task ID；Spec 内记录 2026-09-29 的平台实验、低层 Dispatch 选型及中心验收边界。当前功能支持状态以已交付实现为准。

## worker 环境就绪与宿主恢复

[worker 环境就绪与宿主阻断恢复 Spec](worker-environment-readiness.spec.md) / [Plan](worker-environment-readiness.plan.md) 原范围已交付，[验收记录](../validation/worker-environment-readiness/README.md)供后续迁移复用。#57/#58 迁移既有受管启动、平台事实及中心准备边界，#59 改为项目策略来源，不重新建设或反向阻塞已交付的就绪能力。

## 架构审计收口（3 个 PR）

状态：PR 1（#147）与 PR 2（#152）已合入；PR 3 承接 #156 的基础实现完成收口，待评审。未分配 Task ID。三份 Spec 按下表执行，验证遵循项目的最小必要范围。

| 顺序 | 交付范围 | 硬前置 | Spec |
| --- | --- | --- | --- |
| PR 1 | worker 协议事件与轮次终态边界 | 无未完成前置 | [worker 协议与终态边界](architecture-01-worker-protocol.spec.md) |
| PR 2 | worker 正常生命周期显式原子提交 | PR 1 合入 | [worker 生命周期显式原子提交](architecture-02-worker-commit.spec.md) |
| PR 3 | 剩余写者迁移、删除隐式写回、读写模型与后端接口收口 | PR 2 合入 | [移除隐式写回与读写模型收口](architecture-03-runtime-boundary.spec.md) |

## 执行配置隔离

目标：`EXECUTION-ISOLATION`。#54 公共契约已交付，作为后续硬前置复用，见[验收记录](../validation/execution-isolation-01-contracts/README.md)。剩余七项各交付独立 PR；承接 contracts → execution、只读 query/sqlite 和独立远程协议，不重复已落地机制。范围与并行条件见各自契约及[共同约束](execution-isolation-constraints.md)。

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 55 | 工作区与 Git 资源迁移 | #54 | [Spec](execution-isolation-02-workspace.spec.md) | [Plan](execution-isolation-02-workspace.plan.md) |
| 56 | 发布与 PR 实现迁移 | #54 | [Spec](execution-isolation-03-publication.spec.md) | [Plan](execution-isolation-03-publication.plan.md) |
| 57 | Codex 执行适配器迁移 | #55、#56 | [Spec](execution-isolation-04-codex.spec.md) | [Plan](execution-isolation-04-codex.plan.md) |
| 58 | Orca 执行适配器迁移 | #57 | [Spec](execution-isolation-05-orca.spec.md) | [Plan](execution-isolation-05-orca.plan.md) |
| 59 | 项目执行权限策略 | #58 | [Spec](execution-isolation-06-policy.spec.md) | [Plan](execution-isolation-06-policy.plan.md) |
| 60 | 项目发布责任契约 | #58 | [Spec](execution-isolation-07-publish-policy.spec.md) | [Plan](execution-isolation-07-publish-policy.plan.md) |
| 61 | 执行需求与配置注册收口 | #59、#60 | [Spec](execution-isolation-08-resolution.spec.md) | [Plan](execution-isolation-08-resolution.plan.md) |

## 分级测试基础

[分级测试与共享 E2E 环境 Spec](tiered-e2e-testing.spec.md) / [Plan](tiered-e2e-testing.plan.md)：已实现并通过[最小验证](../validation/tiered-e2e-testing/README.md)，待 PR 评审，未分配 Task ID。首期沿用 L1，新增独立 L2 交付 smoke、L3 等待合并后服务重启场景，共用 controlled 测试项目/任务环境管理；Windows 串行最小实现，不新增真实平台或全量测试门槛。

## 共同约束

- [执行架构](execution-architecture.spec.md)与[原生任务](native-task-constraints.md)：身份、单写者、当前格式恢复和真实交付；被 #36 替代的旧执行语义不继续要求。
- [第一阶段](phase1-delivery-constraints.md)：持续会话、多候选、版本与验收交接。
- [第二阶段](phase2-constraints.md)：受控规划、正式文档、UI 和公共传输。
- [主包隔离约束](core-decoupling-constraints.md)与[后续维护约束](core-maintenance-constraints.md)：执行实现、存储、产品客户端和单写者的持续边界。

## 既有实现与历史基线

以下文档保留已交付能力及历史验收定位，不能作为重新引入旧步骤流程的入口：

- [原生任务模型](task-core.spec.md)、[固定 DAG](task-dag.spec.md)、[原生接入](beads-exit.spec.md)、[关系视图](dag-view.spec.md)、[追加前置](native-task-add-predecessors.spec.md)。
- [双执行后端](pr-04a-execution-backends.spec.md)、[会话事件](pr-04b-conversation-events.spec.md)、[服务生命周期](pr-03-service-lifecycle.spec.md)。
- [旧中心模型](02-execution-model-and-adapter.md)、[旧远端闭环](03-remote-execution-and-viewer.md)、[旧人工决策](pr-05-intervention.spec.md)、[旧控制台](pr-04-console.spec.md)。
- [Hosting](hosting-spec.md)与[边界说明](../hosting.md)。

### 主包解耦、存储与客户端

目标：`CORE-IMPLEMENTATION-DECOUPLING`。#68–#72 均已交付，以下保留规格、原前置及交付证据，供维护和后续任务复用。既有隔离守卫及支持限制见 [#67 验收记录](../validation/core-decoupling-06-boundary/README.md)，当前模块归属见[代码架构索引](../architecture/current-code-architecture.md)。

| 任务 ID | 任务名称 | 原硬前置 | Spec | Plan | 交付证据 |
| --- | --- | --- | --- | --- | --- |
| 68 | 主包解耦重构后代码清理 | #67 | [Spec](core-decoupling-07-cleanup.spec.md) | [Plan](core-decoupling-07-cleanup.plan.md) | [PR191](https://github.com/erranliu/SteerLine/pull/191) · [验收记录](../validation/core-decoupling-07-cleanup/README.md) |
| 69 | 中心写入端口收窄 | #68 | [Spec](core-decoupling-08-write-ports.spec.md) | [Plan](core-decoupling-08-write-ports.plan.md) | [PR194](https://github.com/erranliu/SteerLine/pull/194) |
| 70 | SQLite写入实现归包 | #69 | [Spec](core-decoupling-09-sqlite.spec.md) | [Plan](core-decoupling-09-sqlite.plan.md) | [PR197](https://github.com/erranliu/SteerLine/pull/197) · [验收记录](../validation/core-decoupling-09-sqlite/README.md) |
| 71 | 产品客户端适配层与CLI接入 | #68 | [Spec](core-decoupling-10-client.spec.md) | [Plan](core-decoupling-10-client.plan.md) | [PR195](https://github.com/erranliu/SteerLine/pull/195) · [验收记录](../validation/core-decoupling-10-client/README.md) |
| 72 | TUI与中心实现解耦 | #71 | [Spec](core-decoupling-11-tui.spec.md) | [Plan](core-decoupling-11-tui.plan.md) | [PR196](https://github.com/erranliu/SteerLine/pull/196) · [验收记录](../validation/core-decoupling-11-tui/README.md) |

[L2/L3 真实执行 Spec](live-tiered-e2e.spec.md) / [Plan](live-tiered-e2e.plan.md)：L2 受控门槛后执行本地 Codex 与 Orca+Codex，使用真实 SteerlineTest GitHub PR；L3 delivery/recovery 按环境和场景独立选择，固定 gpt-6-luna/medium，校验原生 token usage。
