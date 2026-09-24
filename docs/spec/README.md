# Spec 与 Plan 索引

项目规格与实施计划统一存放在 `docs/spec/`。每项文档中的状态和验收条件说明其实际完成程度。执行所需约束、范围、依赖和验收条件必须在 Spec 体系中明确，Spec/Plan 不得引用或依赖规划文档。

## 控制台升级

依赖、产品行为和跨项目集成约束见 [控制台升级总览](console-upgrade.md)。按以下顺序实施并审核合并：

| 顺序 | 交付 | Spec | Plan |
| --- | --- | --- | --- |
| 1 | 统一观察层与读取一致性 | [Spec](pr-01-observation.spec.md) | [Plan](pr-01-observation.plan.md) |
| 2 | 服务控制协议与定向执行 | [Spec](pr-02-control.spec.md) | [Plan](pr-02-control.plan.md) |
| 3 | 后台服务生命周期 | [Spec](pr-03-service-lifecycle.spec.md) | [Plan](pr-03-service-lifecycle.plan.md) |
| 4A | 长期双执行后端契约 | [Spec](pr-04a-execution-backends.spec.md) | [Plan](pr-04a-execution-backends.plan.md) |
| 4B | 统一会话事件与完整回复 | [Spec](pr-04b-conversation-events.spec.md) | [Plan](pr-04b-conversation-events.plan.md) |
| 4C | 执行控制台与轮次折叠 | [Spec](pr-04-console.spec.md) | [Plan](pr-04-console.plan.md) |
| 5 | 人工决策与恢复闭环 | [Spec](pr-05-intervention.spec.md) | [Plan](pr-05-intervention.plan.md) |

共七个串行交付阶段，阶段名不是 GitHub PR 编号。PR 3 内容与验收不变；原 PR4 路径保留为 4C。架构衔接见 [共同约束 Spec](execution-architecture.spec.md) 和各阶段 Spec。本次文档更新不启动实现，定时协调保持暂停。

## 中心状态机与远程执行

共同执行约束见 [执行架构 Spec](execution-architecture.spec.md)。

1. [Orca 远程能力验证与执行协议](01-orca-remote-protocol.md)
2. [任务、执行模型、最小状态机与执行适配器](02-execution-model-and-adapter.md)
3. [最小状态机的分阶段远程闭环与 viewer 人工决策](03-remote-execution-and-viewer.md)

## 原生任务与固定 DAG

- [Spec：原生任务、固定 DAG 与 Beads 退出](native-task-dag.spec.md)
- [Plan：分阶段实现与验证](native-task-dag.plan.md)

按 TASK-CORE → TASK-DAG → BEADS-EXIT → DAG-VIEW 交付，复用中心执行模型的事务与身份，不等待整个远程闭环完成。系统不提供拆分或回退流程，两者由用户人工处置；具体范围、来源权威接替、接口与验收以该 Spec 为准。

## 现有 Hosting 实现

- [Hosting 规范 v2：Orca 驱动执行](hosting-spec.md)
- [Hosting 边界与目标架构说明](../hosting.md)
