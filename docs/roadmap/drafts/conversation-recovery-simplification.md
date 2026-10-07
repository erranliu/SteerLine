# 会话采集与恢复精简

本轮选定草稿。依据：2026-10-07 用户直接要求实施精简建议 1、2；本轮在当前工作区修改，不派发外部 SteerLine worker，不从草稿启动 DAG 任务。未分配 Task ID，不声称正式准入或工具侧交付。

成果：中心只保存最终回复、问题及必要执行事实；恢复只能继续原执行的原会话。多后端、多候选、人工并行、反馈、真实 PR 核验和唯一交付保留。

两项共用会话身份、采集、恢复及存储边界，在一份 [Spec](../../spec/product-simplification-04-conversation-recovery.spec.md) 与 [Plan](../../spec/product-simplification-04-conversation-recovery.plan.md) 固定；验收属于本项，见[专属记录](../../acceptance/product-simplification-04-conversation-recovery.md)。自动验收随实现执行，无独立人工活动或新增平台验收要求；真实模型验证范围依实际变化决定，不以 controlled 结果替代 native。
