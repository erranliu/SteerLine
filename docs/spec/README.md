# Spec 与 Plan 索引

项目规格与实施计划统一存放在 `docs/spec/`。每项文档中的状态和验收条件说明其实际完成程度。

## 控制台升级

依赖、产品行为和跨项目集成约束见 [控制台升级总览](console-upgrade.md)。按以下顺序实施并审核合并：

| 顺序 | 交付 | Spec | Plan |
| --- | --- | --- | --- |
| 1 | 统一观察层与读取一致性 | [Spec](pr-01-observation.spec.md) | [Plan](pr-01-observation.plan.md) |
| 2 | 服务控制协议与定向执行 | [Spec](pr-02-control.spec.md) | [Plan](pr-02-control.plan.md) |
| 3 | 服务窗口生命周期与连接 | [Spec](pr-03-service-lifecycle.spec.md) | [Plan](pr-03-service-lifecycle.plan.md) |
| 4 | 执行控制台 TUI | [Spec](pr-04-console.spec.md) | [Plan](pr-04-console.plan.md) |
| 5 | 人工决策与恢复闭环 | [Spec](pr-05-intervention.spec.md) | [Plan](pr-05-intervention.plan.md) |

## 中心状态机与远程执行

目标架构见 [架构决策](../architecture-direction.md)。

1. [Orca 远程能力验证与执行协议](01-orca-remote-protocol.md)
2. [任务、执行模型、最小状态机与执行适配器](02-execution-model-and-adapter.md)
3. [最小状态机的分阶段远程闭环与 viewer 人工决策](03-remote-execution-and-viewer.md)

## 现有 Hosting 实现

- [Hosting 规范 v2：Orca 驱动执行](hosting-spec.md)
- [Hosting 边界与目标架构说明](../hosting.md)
