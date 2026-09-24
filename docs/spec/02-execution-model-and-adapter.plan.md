# Plan：中心执行模型与适配器

任务 ID：5；任务名称：中心执行模型。执行依据：[Spec](02-execution-model-and-adapter.md)。前置使用 [远程协议 Spec](01-orca-remote-protocol.md) 已实测的字段和明确能力限制；剩余协议门槛不阻止独立模型开发。

## 实施顺序

1. 从最新主线确认已落地的 4A/4B/5 执行、会话、决策与存储契约，列出权威写者、版本和消费者。先落地者提供共同接口，本任务不得平行实现另一套 adapter、会话模型或业务状态写者。
2. 定义 Task、WorkflowRun、Execution、StepAttempt、Decision、Event 和 Operation 的稳定身份及状态转换；保留旧运行身份。已分配规划数字 ID 的 Task 使用同一 task_id，Beads 等外部来源 ID 单独保存；未映射旧任务使用非数字 legacy 身份。
3. 固定 `task-identity-map/v1` 的项目/仓库、数字 task_id、旧来源三元组与摘要格式。对已知规划任务要求事先核实映射；一对多、多对一、重复 ID、来源不存在或身份不符均拒绝，不按标题、Spec 文件名或相似文本猜测，也不产生第二个 Task。
4. 在现有 run_dir 内实现 SQLite 事务 Store、命令/outbox、事件和版本化读模型；迁移 v1–v4 及当时已落地的扩展记录。数据库成为单一业务权威，旧文件保留可读但不形成双写恢复来源。
5. 接入共同 ExecutionBackend/会话接口，按 `minimal-v1` 逐步派发 prepare、implement、verify、review、publish_pr；保留 `legacy-v1` 兼容路径。远程停止明确不支持，中心停机不能推断远端已停。
6. 接入 QueryService/CommandService、决策和恢复对账；在提交、外部动作、回执、投影之间注入故障，按固定 Operation 恢复，不盲目重派。

## 验证与交接

- 逐项覆盖 Spec M01–M18：已规划数字 task_id 原值保留、旧来源映射幂等、legacy 号不占规划号、历史 Run/PR/消息关联不丢。
- 运行相关迁移、事务故障、后端契约与既有 local/Orca 回归；执行仓库要求的 Go 测试、静态检查及 `git diff --check`。真实远端上线验收由 [远程闭环 Spec](03-remote-execution-and-viewer.md) 负责。
- 向后续任务交接唯一 Task/Execution/Attempt 身份、SQLite/Query/Command 版本、迁移规则及受限远端能力，不能把尚未验收的协议能力标为可用。
