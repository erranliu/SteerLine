# Plan：中心执行模型与适配器

任务 ID：5；任务名称：中心执行模型。执行依据：[Spec](02-execution-model-and-adapter.md)。前置使用 [远程协议 Spec](01-orca-remote-protocol.md) 已实测的字段和明确能力限制；剩余协议门槛不阻止独立模型开发。

## 实施顺序

1. 从最新主线确认已落地的 4A/4B 与人工介入阶段的执行、会话、决策和存储契约，列出权威写者和消费者。先落地者提供共同接口，本任务不得平行实现另一套 adapter、会话模型或业务状态写者。
2. 定义 Task、WorkflowRun、Execution、StepAttempt、Decision、Event 和 Operation 的稳定身份及状态转换。已分配规划数字 ID 的 Task 使用同一 task_id，不从标题或外部工具 ID 推断身份。
3. 在干净状态目录内实现 SQLite 事务 Store、命令/outbox、事件和读模型；数据库成为单一业务权威。旧 runtime、旧数据库或未知 schema 明确拒绝有副作用启动，不实现导入。
4. 接入共同 ExecutionBackend/会话接口，按 `minimal-v1` 逐步派发 prepare、implement、verify、review、publish_pr。远程停止明确不支持，中心停机不能推断远端已停。
5. 验证当前格式内事务提交、响应丢失、投影失败和重启后的对账；同一 Operation 不盲目重发。
6. 接入 QueryService/CommandService、决策和恢复对账；在提交、外部动作、回执、投影之间注入故障，按固定 Operation 恢复，不盲目重派。

## 验证与交接

- 逐项覆盖 Spec M01–M18：已规划数字 task_id 原值保存、重复数字拒绝、当前运行与 PR/消息关联在重启后保持一致；旧状态拒绝有副作用启动。
- 运行事务故障、后端契约与 local/Orca 当前路径回归；执行仓库要求的 Go 测试、静态检查及 `git diff --check`。真实远端上线验收由 [远程闭环 Spec](03-remote-execution-and-viewer.md) 负责。
- 向后续任务交接唯一 Task/Execution/Attempt 身份、SQLite/Query/Command 当前格式及受限远端能力，不能把尚未验收的协议能力标为可用。
