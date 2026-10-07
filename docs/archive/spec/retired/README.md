# 退役流程与远端试验

这些资料描述旧 `minimal-v1`、中心步骤编排、早期 hosting 和远端协议实验，已经被当前本机持续 worker 范围替代。归档表示退出当前执行入口，不表示原列出的全部验收、远端权限、取消或故障门槛完成。

当前运行说明见[数据契约](../../../product-design/data-model.md)、[Hosting](../../../architecture.md)和[当前 Spec](../../../spec/README.md)。原版本、命令、状态和证据保持历史含义。

| Spec / 记录 | 配套 Plan |
| --- | --- |
| [Spec 01：Orca 远程能力验证与执行协议](01-orca-remote-protocol.md) | [Plan](01-orca-remote-protocol.plan.md) |
| [Spec 02：任务、执行模型、最小状态机与执行适配器](02-execution-model-and-adapter.md) | [Plan](02-execution-model-and-adapter.plan.md) |
| [Spec 03：最小状态机的分阶段远程闭环与 viewer 人工决策](03-remote-execution-and-viewer.md) | [Plan](03-remote-execution-and-viewer.plan.md) |
| [执行架构 Spec：共同约束](execution-architecture.spec.md) | — |
| [Native local and Orca execution](hosting-spec.md) | — |
| [原 #42 一期集成验收](phase1-release-acceptance.spec.md) | [原 Plan](phase1-release-acceptance.plan.md) |
| [原 #53 二期完整闭环验收](phase2-integration.spec.md) | [原 Plan](phase2-integration.plan.md) |

原 #42/#53 是人工组织为主的阶段验收，不再作为执行任务候选；ID 保留历史且不复用。现行自动/人工行为和状态分别由[一期](../../../acceptance/phase1.md)和[二期](../../../acceptance/phase2.md)验收文档维护，归档不取消外部 SteerLine 已有任务。

[Orca 协议探针与原始样本](orca-probe/README.md)
