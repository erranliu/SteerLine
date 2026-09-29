# Plan：多候选会话与唯一PR交付

任务 ID：37；任务名称：多候选会话与唯一PR交付。唯一执行依据：[Spec](candidate-workers.spec.md)。硬前置：#36，一个独立实现 PR 交付。

1. 从 #36 的持续 worker、主阶段、反馈和真实 PR 交付接口盘点 Task 单运行假设；复用现有 WorkflowRun、Execution、会话、Query/Command 与 SQLite，不先建立通用候选调度器或步骤规则包。
2. 固定新格式的 single/multiple 策略、同一 claim 的共同 base 和精确候选 DTO；实现 start_candidate、版本竞争、启动意图和原请求对账，保留默认 single 路径。
3. 所有执行/反馈/正文/恢复按精确候选及原会话定位，列表按 Task 聚合；补齐多候选交错、分页和当前/历史领取区分。
4. 将 #36 的 PR 关联扩展到候选集合，接唯一 Delivery 事务及获胜/落选资格。候选启动与交付一并完成后启用 multiple 功能，不发布中心禁止 worker 提 PR 的半成品入口。
5. 处理已知多合并、查询不明、交付后在途输出与迟到 PR；落选仅限制中心新增推进，停止和清理沿平台实际能力。聚合全部候选核验明确丢弃，保留历史和后继阻塞。
6. 执行 MC1–MC7、共同约束回归和受影响 single/持久会话测试。记录请求、session、PR/Delivery 和残留资源证据，向 #38/#39 交接查询、命令与夹具。
