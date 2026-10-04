# #74 唯一所有者与生产路径

起点 origin/main `39059035152a76a8a0d8f0036eb3fa2f6155ac90`（PR210）。只承接 #73 的唯一纯定义；原 `internal/steerline/task` 的全部生产定义及原测试文件已删除，无别名/转发或旧格式读取。#73 的 Project/Paginate 每次快照只转换一次 TaskGraph 的修复保持。

| 旧定义/职责 | 唯一实际所有者/替代 | 真实消费者 |
| --- | --- | --- |
| task.ProjectTaskAuthority、TaskDeliveryRecord/Evidence、TaskClaimReceipt/TaskDeliveryReceipt | execution/model `task_delivery.go` / `task_receipts.go` | SQLite、执行 claim/delivery、同快照查询/产品旧宽视图；迁出 root/contracts 的相关 alias 同次删除。 |
| task.DeliveryGate/DeliveryPR/ValidateDelivery | execution/model `task_delivery_gate.go` / `task_delivery_validation.go` | 执行在外部查询前核原 session/选中身份；SQLite 在同一提交中重检绑定和 proof。执行模型不依赖 task/model。 |
| task.CompleteDelivery（理解 PR） | task/model.Result、ResultDeclaration、ResultReceipt / DeclareResult | 执行核验后调用 app 注入的 TaskResultPort；SQLite 将任务声明与唯一执行 Delivery 组合原子提交。 |
| task.StartupBlockers 的模式/暂停/slot | execution/model `task_admission.go` | taskview/sqlite 查询资格展示；任务只判断纯 status/DeliveryFact/前置交付。 |
| task.Snapshot / TaskGraph | taskview.Snapshot / TaskGraph，只读纯投影 | SQLite/query/taskview 维持同一读快照；任务经 SQLite.TaskSnapshot 只得 task/model.Snapshot。没有宽快照搬入 storage/ports。 |
| task.TaskPredecessorAddInput/Receipt | product/api `task_predecessors.go` | CLI/client/当前任务命令；旧 root alias 删除。 |
| root candidate set/base/discard facts | execution/model CandidateBindings/CandidatePRBindings/CandidateCommonBase/CandidateDiscardFacts/VerifiedCandidatePR | 外部核验、原子 claim/result/discard；Result/Discard 版本包含候选执行的固定身份，start 的 PR-only 检查保持宽松参与数语义。 |
| persistence.ClaimTx / TransactClaim | storage/ports.ClaimCommit / SQLite.CommitTaskClaim | 固定工作与任务前后值 + 执行声明；重检后一次保存 Task（仅首次）、Workflow、Execution、Attempt、Command、Operation、event。 |
| 交付宽写回调 | storage/ports.ResultCommit / SQLite.CommitTaskResult | 唯一任务结果、执行 Delivery、closed、DAG revision 与 event 同事务；重放返回原回执并不写版本。剩余 DeliveryAuditTx 仅读绑定/写冲突事件。 |
| persistence.DiscardTx / TransactDiscard | task.DiscardIntent / TaskResultPort.Intent+Discard、storage/ports.DiscardCommit | 任务记录人意图；执行核验原 PR 集合、停止及资源清理；存储核原 Operation/request、任务/集合版本。 |

实际调用链：app `task_boundary_wiring.go` → NewService/NewCommandService 注入 task 的 Work/ResultPort → 首次 claim / later candidate / worker PR 交付 / task discard 协调 → storage/ports 的闭集声明 → 唯一 SQLite executor。产品 task-only adapter 的服务接口仅 Validate/Create/AddPredecessors；产品的 Discard 适配只提供用户意图，后端没有结果写能力。

SQLite schema 由 13 升为 14，增加唯一 task_results（任务/结果/原请求 ID 唯一），仅接受当前完整 schema；不创建旧版读取、迁移、双写或回退。ResultID 为固定选中 WorkflowRun 的结果关联，内部交付请求 ID 固定为 `delivery:<ResultID>`，重试不产生替代结果；task 只比较其标量身份，不解释 Workflow/PR/session。执行 PR 证据仍唯一保存在 task_deliveries 的执行记录，并在查询组合。

任务包守卫现在包括 `internal/task`：只允许标准库和 task/model，不能经 storage/ports 取得跨域执行提交能力。taskview 的 Snapshot 作为纯投影由全 OS/tag 矩阵限定只依赖标准库与 task/execution 纯模型；CM69 用该唯一定义替换已删的旧 task 值例外，不引入 query/usecase/SQL 依赖。既有全 OS/tag 扫描和 Windows/Linux 生产依赖检查保留；负向注入同步加入 task 对 execution/model、storage/ports 和实现的反例。最终全部用例迁包/产品 DTO 收窄、存储读取拆分仍分别归 #75/#76/#77/#79，本轮没有提前声称完成。

共享结构/方法与 app/cmd/runner 的真实拆分、后继唯一修改者见 [integration-ownership.md](integration-ownership.md)。旧 #73 盘点与历史报告保持不变，不能用旧快照的证据声称本次新格式或构造已验收。
