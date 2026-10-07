# 已交付 Spec 与 Plan

本目录保存已经执行并合入的规格、配套计划与阶段共同约束。核对基线为三线最终清理 PR227 合入后的源码树；原文中的“待实施”“待评审”、旧模块路径和阶段前置属于交付时的历史记录，不是当前待办。移动仅修正文档导航，原固定 Git 引用仍由原 commit 提供。

当前剩余任务见[Spec 索引](../../../spec/README.md)，当前接口见[代码架构](../../../architecture/current-code-architecture.md)。本机 Orca 和 local 的未闭合定向验收仍留在活动 Spec 索引；归档不宣告远端流程、旧故障矩阵或未测平台已经通过。

## 交付依据

| 系列 | 已合入 PR / 当前证据 |
| --- | --- |
| 三线与目录整理 #73–#82 | PR210、214、215、219、221、220、223、225、226、227；[最终清理证据](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/three-line-10-independence-cleanup/README.md) |
| 主包解耦 #62–#72 | PR182、183、185、187、189、190、191、194、197、195、196；[边界验收](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-06-boundary/README.md) |
| 执行隔离 #54–#61 | PR164、169、168、170、171、172、173、174；[收口证据](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/execution-isolation-08-resolution/README.md)；未闭合远端证据仍是历史限制 |
| worker 与候选 #36–#39 | PR121、206、208、216；[本机候选验收](../../../acceptance/candidate-acceptance.md)，[环境就绪](../../../acceptance/worker-readiness.md)（PR160） |
| worker 原子提交/协议审计 | PR147、152、157 |
| 测试基础与流程简化 | [分级测试](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/tiered-e2e-testing/README.md)（PR180）、[依赖边界](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/e2e-dependency-boundaries/README.md)、[测试效率](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-implementation/results.md)（PR100）、[流程简化](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/simplified-product-flow/README.md)（PR202） |

## 文档索引

| Spec / 记录 | 配套 Plan |
| --- | --- |
| [Spec：数据库活动与操作回执收口](activity-db-receipt-simplification.spec.md) | [Plan](activity-db-receipt-simplification.plan.md) |
| [Spec：架构收口 PR 1 — worker 协议与终态边界](architecture-01-worker-protocol.spec.md) | — |
| [Spec：架构收口 PR 2 — worker 生命周期显式原子提交](architecture-02-worker-commit.spec.md) | — |
| [Spec：架构收口 PR 3 — 移除隐式写回与读写模型收口](architecture-03-runtime-boundary.spec.md) | — |
| [Spec：解除 Beads 依赖与原生接入](beads-exit.spec.md) | [Plan](beads-exit.plan.md) |
| [Spec：移除 Beads 旧执行路径](beads-path-removal.spec.md) | [Plan](beads-path-removal.plan.md) |
| [Spec：候选观察界面](candidate-console.spec.md) | [Plan](candidate-console.plan.md) |
| [Spec：多候选本机验收](candidate-native-acceptance.spec.md) | [Plan](candidate-native-acceptance.plan.md) |
| [Spec：多候选会话与唯一PR交付](candidate-workers.spec.md) | [Plan](candidate-workers.plan.md) |
| [控制台升级：Spec + Plan 总览](console-upgrade.md) | — |
| [Spec 通用执行事件与问题边界](core-decoupling-01-events.spec.md) | [Plan](core-decoupling-01-events.plan.md) |
| [Spec 执行配置与原生绑定所有权](core-decoupling-02-bindings.spec.md) | [Plan](core-decoupling-02-bindings.plan.md) |
| [Spec 通用执行生命周期与副作用接线](core-decoupling-03-lifecycle.spec.md) | [Plan](core-decoupling-03-lifecycle.plan.md) |
| [Spec 会话采集与宿主交互实现隔离](core-decoupling-04-conversation.spec.md) | [Plan](core-decoupling-04-conversation.plan.md) |
| [Spec 内置实现组装与主包依赖反转](core-decoupling-05-composition.spec.md) | [Plan](core-decoupling-05-composition.plan.md) |
| [Spec 主包隔离守卫与测试归位](core-decoupling-06-boundary.spec.md) | [Plan](core-decoupling-06-boundary.plan.md) |
| [Spec 主包解耦重构后代码清理](core-decoupling-07-cleanup.spec.md) | [Plan](core-decoupling-07-cleanup.plan.md) |
| [Spec 中心写入端口收窄](core-decoupling-08-write-ports.spec.md) | [Plan](core-decoupling-08-write-ports.plan.md) |
| [Spec SQLite写入实现归包](core-decoupling-09-sqlite.spec.md) | [Plan](core-decoupling-09-sqlite.plan.md) |
| [Spec 产品客户端适配层与CLI接入](core-decoupling-10-client.spec.md) | [Plan](core-decoupling-10-client.plan.md) |
| [Spec TUI与中心实现解耦](core-decoupling-11-tui.spec.md) | [Plan](core-decoupling-11-tui.plan.md) |
| [主包与执行实现解耦共同约束](core-decoupling-constraints.md) | — |
| [主包解耦后续维护共同约束](core-maintenance-constraints.md) | — |
| [Spec：任务关系视图与 JSON 导出](dag-view.spec.md) | [Plan](dag-view.plan.md) |
| [L2/L3 测试依赖边界](e2e-dependency-boundaries.spec.md) | [Plan](e2e-dependency-boundaries.plan.md) |
| [Spec 公共执行契约归包](execution-isolation-01-contracts.spec.md) | [Plan](execution-isolation-01-contracts.plan.md) |
| [Spec 工作区与 Git 资源迁移](execution-isolation-02-workspace.spec.md) | [Plan](execution-isolation-02-workspace.plan.md) |
| [Spec 发布与 PR 实现迁移](execution-isolation-03-publication.spec.md) | [Plan](execution-isolation-03-publication.plan.md) |
| [Spec Codex 执行适配器迁移](execution-isolation-04-codex.spec.md) | [Plan](execution-isolation-04-codex.plan.md) |
| [Spec Orca 执行适配器迁移](execution-isolation-05-orca.spec.md) | [Plan](execution-isolation-05-orca.plan.md) |
| [Spec 项目执行权限策略](execution-isolation-06-policy.spec.md) | [Plan](execution-isolation-06-policy.plan.md) |
| [Spec 项目发布责任契约](execution-isolation-07-publish-policy.spec.md) | [Plan](execution-isolation-07-publish-policy.plan.md) |
| [Spec 执行需求与配置注册收口](execution-isolation-08-resolution.spec.md) | [Plan](execution-isolation-08-resolution.plan.md) |
| [执行配置隔离共同约束](execution-isolation-constraints.md) | — |
| [L2/L3 真实执行环境](live-tiered-e2e.spec.md) | [Plan](live-tiered-e2e.plan.md) |
| [人工并行启动任务 Spec](manual-parallel-start.spec.md) | [Plan](manual-parallel-start.plan.md) |
| [Spec：已建任务追加前置与安全重置](native-task-add-predecessors.spec.md) | [Plan](native-task-add-predecessors.plan.md) |
| [PR 1 Spec：统一观察层与读取一致性](pr-01-observation.spec.md) | [Plan](pr-01-observation.plan.md) |
| [PR 2 Spec：服务控制协议与定向执行](pr-02-control.spec.md) | [Plan](pr-02-control.plan.md) |
| [PR 3 Spec：服务窗口生命周期与连接](pr-03-service-lifecycle.spec.md) | [Plan](pr-03-service-lifecycle.plan.md) |
| [PR 4C Spec：执行控制台与轮次折叠](pr-04-console.spec.md) | [Plan](pr-04-console.plan.md) |
| [PR 4A Spec：长期双执行后端契约](pr-04a-execution-backends.spec.md) | [Plan](pr-04a-execution-backends.plan.md) |
| [PR 4B Spec：统一会话事件与完整回复](pr-04b-conversation-events.spec.md) | [Plan](pr-04b-conversation-events.plan.md) |
| [PR 5 Spec：人工决策与恢复闭环](pr-05-intervention.spec.md) | [Plan](pr-05-intervention.plan.md) |
| [Spec：清理退役日志解析与发布元数据](retired-consumer-pruning.spec.md) | [Plan](retired-consumer-pruning.plan.md) |
| [Spec：收敛执行与反馈产品流程](simplified-product-flow.spec.md) | [Plan](simplified-product-flow.plan.md) |
| [Spec：原生任务模型](task-core.spec.md) | [Plan](task-core.plan.md) |
| [Spec：固定依赖 DAG 调度](task-dag.spec.md) | [Plan](task-dag.plan.md) |
| [Spec：任务图规则拆包与测试分层](taskgraph-modules.spec.md) | [Plan](taskgraph-modules.plan.md) |
| [Spec：测试执行提速](test-execution-efficiency.spec.md) | [Plan](test-execution-efficiency.plan.md) |
| [Spec：测试耗时基线与夹具精简](test-feedback.spec.md) | [Plan](test-feedback.plan.md) |
| [Spec：纯类型与契约归属整理](three-line-01-types.spec.md) | [Plan](three-line-01-types.plan.md) |
| [Spec：任务选择与执行结果边界](three-line-02-task-execution-boundary.spec.md) | [Plan](three-line-02-task-execution-boundary.plan.md) |
| [Spec：任务应用用例归包](three-line-03-task-usecases.spec.md) | [Plan](three-line-03-task-usecases.plan.md) |
| [Spec：执行应用用例归包](three-line-04-execution-usecases.spec.md) | [Plan](three-line-04-execution-usecases.plan.md) |
| [Spec：产品查询与控制台契约收窄](three-line-05-product-console.spec.md) | [Plan](three-line-05-product-console.plan.md) |
| [Spec：执行适配与资源目录归组](three-line-06-adapters-resources.spec.md) | [Plan](three-line-06-adapters-resources.plan.md) |
| [Spec：存储目录与消费者端口收口](three-line-07-storage-ports.spec.md) | [Plan](three-line-07-storage-ports.plan.md) |
| [Spec：应用组装与服务宿主收口](three-line-08-app-host.spec.md) | [Plan](three-line-08-app-host.plan.md) |
| [Spec：测试支持与集成目录整理](three-line-09-testkit-integration.spec.md) | [Plan](three-line-09-testkit-integration.plan.md) |
| [Spec：独立开发证明与最终清理](three-line-10-independence-cleanup.spec.md) | [Plan](three-line-10-independence-cleanup.plan.md) |
| [三线解耦共同实现约束](three-line-constraints.spec.md) | — |
| [Spec：分级测试与共享 E2E 环境的最小实现](tiered-e2e-testing.spec.md) | [Plan](tiered-e2e-testing.plan.md) |
| [Spec：worker 环境就绪与宿主阻断恢复](worker-environment-readiness.spec.md) | [Plan](worker-environment-readiness.plan.md) |
| [Spec：单 worker 持续执行与统一待决策](worker-flow.spec.md) | [Plan](worker-flow.plan.md) |
| [Plan：解耦后的代码精简](post-decoupling-simplification.plan.md) | 独立修复/验证计划 |
| [189 后基线修复](rebase-189-baseline-repair.plan.md) | 独立修复/验证计划 |
| [Plan：测试设计审计与基线修复](test-baseline-repair.plan.md) | 独立修复/验证计划 |
