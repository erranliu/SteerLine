# Plan：多候选模型与准入

任务 ID：26；任务名称：多候选模型与准入。唯一执行依据：[Spec](task-candidates.spec.md)。原固定规格无未完成硬前置；中心已追加 #34 的开工约束，一个实现 PR 交付。

1. 审计 Task claim、workflow 唯一索引及全部命令/发布/交付入口，列出任务级与执行级目标。重点追踪 `refreshNativeSnapshot`、按 Task 查 Run 的 `Service.find`、停止/重试/输出/清理及启动后回读；保留已有按 WorkflowRun 管理的 worker 和单写者事件处理。
2. 在已集成的 #33 规则与 #34 显式上下文边界上接入多候选查找与投影：Task 查询返回候选集合，执行操作按 workflow_run_id 及 Execution/Attempt 从属关系定位，恢复枚举全部候选；CurrentTask/SlotOccupied 等摘要不参与选取候选。明确当前 claim 与历史候选，消除“最新运行”对兄弟候选的覆盖。
3. 实现 single/multiple 固定策略和 schema 5 的受控转换。固定源样本、目标 schema、停服/锁/备份及校验顺序；按 legacy-v1 保留原契约字节/摘要，按 candidates-v2 创建新契约。调整 Task 级 workflow 唯一约束，同时保留每 WorkflowRun 的活动 Execution 约束及每 Execution 的活动 Attempt 约束；不串联其他旧格式的自动升级或摘要猜测。
4. 实现首次 claim 的 base/前置证据与 start_candidate：事务外取得外部证据，事务内重新核版本、候选集合和资格并保存 Command/Operation/候选身份，提交后发起外部动作。原请求先按持久回执对账，stale_version 不自动换版本重发；追加候选复用首次 claim 的固定 base。
5. 交付候选列表/详情、分页与恢复，落实零候选补边/丢弃及全部未发布候选已结算后的任务级重置。在全部 multiple 发布、交付及后继释放入口落实 `candidate_delivery_unavailable`；向 #27/#28 提供精确 DTO、命令样例与意图入口清单，候选 TUI 改造归 #28。
6. 按 CM1–CM6 验证三候选及 single 回归，包含列表重排、同任务两个待回答问题、一个候选 unknown 时其他候选的独立路由、重启完整恢复和旧 claim 迟到结果。另覆盖请求重放、图变化、任务级重置竞争与转换中断；依共同约束运行回归，保留 docs/validation/task-candidates/ 证据。

本任务完整交付后 #27 可接仲裁；中心已有 #26 → #16 的开工约束，#16 仍按自身固定契约消费执行接口。文档细化不重绑定已创建任务，也不修改现有 DAG。
