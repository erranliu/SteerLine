# Plan：多候选模型与准入

任务 ID：26；任务名称：多候选模型与准入。唯一执行依据：[Spec](task-candidates.spec.md)。无未完成硬前置，一个实现 PR 交付。

1. 审计 Task claim、workflow 唯一索引、按任务查最新运行的 Service/投影路径，列出全部命令目标和发布/交付入口。
2. 实现 single/multiple 固定策略、schema 5 的受控转换与首次 claim 的 base/前置证据；用固定旧样本验证 legacy-v1 契约字节/摘要不变、candidates-v2 策略参与摘要，拒绝未知模式和不合法回退。
3. 实现 start_candidate 的事务准入、版本竞争与原 Operation 对账；将 worker、步骤和人工输入路由收敛到精确运行身份。
4. 交付候选列表/详情、分页与恢复，落实零候选补边/丢弃及全部未发布候选已结算后的任务级重置；为 #27/#28 提供 DTO/命令样例，在 multiple 发布及结项入口落实能力门槛。
5. 按 CM1–CM6 验证三候选、错误路由、重复请求、未知启动、图变化和原状态转换；依共同约束运行回归，保留 docs/validation/task-candidates/ 证据。

本任务合入后 #27 可接仲裁，#16 拟增加的开工边须另由中心命令受理；本文不修改 #16 的已冻结契约或现有 DAG。
