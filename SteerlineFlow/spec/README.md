# 当前 Spec 与 Plan

用量事实与计费职责划分（2026-11-05，已实施/隔离验证，未部署）：[Spec](billing-ownership.spec.md) · [Plan](../plans/billing-ownership.plan.md) · [验收](../testing/acceptance/billing-ownership.md)。Runner提供Token事实，SteerLine维护价表和本地估价；远端Codex授权闭环实测成功，原Task 101安装现场未修改。

SteerLine 与 SteerRunner 职责边界收口（2026-11-05，已实施）：[Spec](runner-boundary-consolidation.spec.md) · [Plan](../plans/runner-boundary-consolidation.plan.md) · [审计与验收](../testing/acceptance/runner-boundary-consolidation.md)。原生目录迁到 Runner 无服务 CLI，策略预检消费已有公开目录，内容生成共用纯绑定核验。实际正式测试、固定构建和真实目录页面证据分别登记于验收记录。
