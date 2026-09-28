# Plan：PR 胜出与落选候选对账

任务 ID：27；任务名称：PR 胜出与落选候选对账。唯一执行依据：[Spec](candidate-delivery.spec.md)。硬前置：#26，一个实现 PR 交付。

1. 基于 #26 的运行身份接通逐候选发布和 PR 唯一绑定，移除 multiple 的临时交付拒绝。
2. 复用当前 PR provider 与步骤证据核验；实现全候选发布快照、唯一交付事务及迟到冲突记录。
3. 将候选推进资格检查接到启动、步骤、回答、重试与发布入口；用持久待办接原本机 Stop/PR 关闭及远端不支持分支。
4. 让 Task closed 后仍加载未结算候选；扩展全部 PR 确认 closed_unmerged 后的安全任务变更，持续核对 abandoned/discarded 的历史 PR，完善 delivery_role、待办与冲突信息。
5. 按 CD1–CD7 注入提交前后、回执丢失、并发双合并、在途发布及重置后的历史 PR 重开/迟到合并故障，运行共同约束回归并保存记录。
