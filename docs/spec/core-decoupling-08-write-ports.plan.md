# Plan 中心写入端口收窄

任务 ID：69；任务名称：中心写入端口收窄；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-08-write-ports.spec.md)及[共同约束](core-maintenance-constraints.md)。硬前置：#68 整体交付；一个实现 PR。

## 实施顺序

1. 对所有生产写调用建立消费者/当前事务/固定身份/失败处理清单，标出 SQL 嵌入、宽接口和连接所有者。按已提交代码确定声明类型及接口的唯一纯所有者。
2. 固定命令、worker、任务/DAG/交付、会话/元数据四组请求和结果；所有方法以原子操作表达。现有 SQLiteStore 在原位置实现端口，保留现有事务仲裁和 CAS。
3. 依次接通实际业务消费者，将其中 SQL/扫描归原存储实现；业务资格和候选计算留中心。连接资源由外层持有，删除无用宽接口/转发。中性 fake 仅用于端口消费断言，不替代 SQLite 事务用例。
4. 核对全部写集合及 checkpoint、附件、source cursor 的边界，确认 worker 失败回滚私有候选；更新受影响测试归属与模块说明，并向 #70 交付稳定端口。

## 测试计划

基线：main `184e1e06d217b243e5655f4f9b7c233e4c80f579`（#68 / PR191 已合并），Windows / Go 1.26.4。证据根目录：`C:/Users/lmas1/.codex/task-state/cm69-validation/`，不提交运行产物。

| 级别 / 组与场景 ID | 环境 / 命令 | 执行时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| L1 定向 / worker：`WorkerCommitRollsBackRecordsReceiptsEventsAndCheckpoint`、`WorkerCommitRollsBackOnStaleRecordAndCheckpointFailure`、`WorkerBindingRollsBackWithItsCheckpoint`、`WorkerRecoveryAttemptCommitsAtomically`、`WorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch` | 本地实际 SQLite；下述统一定向命令 | 全部生产消费者接通后 | 声明写集合、附件/checkpoint 原子回滚，过期前值/Attempt 拒绝，恢复和 intent 屏障保留；`targeted.jsonl` |
| L1 定向 / 命令：`CommandIdempotencyAndDecisionVersionArbitration`、`DecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional` | 同上 | 同上 | 重复/冲突请求、答案版本与 first-wins 原子性；同上 |
| L1 定向 / 任务：`CreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails`、`NativeClaimCompetitionCreatesOnlyOneWorkflowReservation`、`AddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim`、`TaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent`、`WorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor` | 同上，无真实模型请求 | 同上 | 批次全回滚、唯一领取、DAG 版本、未知 PR 原 intent 对账和唯一真实交付/后继资格；同上 |
| L1 定向 / 会话：`ConversationWriteFailureRollsBackContentAndCursor`、`ConversationLiveSourceFailureKeepsGapAndMapperPosition` | 同上 | 同上 | 内容/cursor 回滚及 mapper 提交屏障；同上 |
| L1 新增 / `CM69` 端口消费与边界 | 中性 fake + 实际 SQLite 清理/过期 Attempt + 源码/依赖检查；`go test ./internal/steerline ./internal/steerline/persistence -run '^TestCM69' -json -count=1` | 接通后 | 请求携带固定身份/前值、拒绝不发布成功视图；纯契约无中心/SQL/平台依赖，业务写消费者不访问具体事务；`ports.jsonl` |
| L1 / #67 守卫及第三实现 | `go test ./internal/steerlineintegration -run '^TestCD67(CoreBoundary\|AdditionalImplementation)$' -json -count=1` | 稳定后 | 两入口均非零命中且通过；`guard.jsonl` |
| L1 / 既有 quick | `go run ./cmd/test-feedback -quick -out C:/Users/lmas1/.codex/task-state/cm69-validation/quick` | 定向通过且自查稳定后仅一次 | 固定 quick 全部通过、无零命中；`quick/summary.json` |
| 静态检查 | `git diff --check` | 提交前 | 无输出且退出 0；`diff-check.txt` |

统一定向命令：`go test ./internal/steerline -run '^Test(CheckpointAndReadViewsCannotPublishWorkerCandidates|WorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor|WorkerCommitRollsBackRecordsReceiptsEventsAndCheckpoint|WorkerCommitRollsBackOnStaleRecordAndCheckpointFailure|WorkerBindingRollsBackWithItsCheckpoint|WorkerRecoveryAttemptCommitsAtomically|WorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch|CommandIdempotencyAndDecisionVersionArbitration|DecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional|CreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails|NativeClaimCompetitionCreatesOnlyOneWorkflowReservation|AddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim|TaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent|WorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor|ConversationWriteFailureRollsBackContentAndCursor|ConversationLiveSourceFailureKeepsGapAndMapperPosition)$' -json -count=1`。

实施前已用原始定向正则的 `-list` 核验上述 14 个既有顶层用例全部命中；自查补选 `CheckpointAndReadViewsCannotPublishWorkerCandidates` 和 `WorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor`，最终统一定向命令为 16 个既有用例；实施后从 JSON 的 run/pass 事件再次核验。既有测试不改名、不迁包；移除公开任意事务入口后，同包存储 fixture 改用已有私有 `transactSQLite`，断言保持原位置。复查发现完整清理回执无既有直接测试，新增 `TestCM69CleanupReceiptKeepsTaskAndReceiptAtomic` 验证 Task/pending item/graph/Operation/Command 同事务、回执失败全回滚及原请求重放。另复用外层 `TestFileQuerySourceReadsCombinedProjectView` 与 `TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding`：`go test ./internal/steerline/console ./internal/steerline/sqlite -run '^Test(FileQuerySourceReadsCombinedProjectView|ReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding)$' -json -count=1`，证据 `readers.jsonl`。仅新增端口消费/依赖边界所缺验证，不增加 e2e 环境能力。

不选 L2、L3 controlled/live 或 full：本任务仅替换持久提交边界，未改变原生启动、权限、provider 或产品协议；实际 SQLite 代表用例覆盖本次原子集合与失败屏障，既有 quick 覆盖最小产品闭环。无需新增模型/远端资源，也无需完整故障矩阵。

## 验证与交付

CM69-1 用全体写消费者清单、源码依赖与实际构造核对；CM69-2 从现有 worker 提交回滚/过期 Attempt、命令重复 ID、任务批次/真实唯一交付、会话 commit cursor 用例各选必要断言；CM69-3 用纯端口依赖检查、#67 既有守卫与一次 quick。

实施时记录所选入口及旧到新映射并核对实际命中；测试命名/路径按 #68 后基线确定，不在规划时虚构已存在入口。稳定后合并定向组与 quick 执行一次，不跑 full。运行 git diff --check。

PR 列明每组端口的真实消费者、原子写集合、移走 SQL 的唯一位置、删除清单及实际验证。若 #71 并行，写端口和中心 Store 段由本项修改；公共产品 DTO 不调整，应用组装与测试入口变更集中集成。规划阶段不改代码或数据库。


## 实际消费者与 #70 交接

纯契约唯一所有者为 `internal/steerline/persistence`。Service 分别持有中心只读接口、commandStore、conversationStore 和仅用于关闭的连接生命周期函数；串行 Store 仅持有 WorkerMetadata。CommandService 的组合接口只提供命令操作及具名原子任务提交，不提供共享事务或任意记录修改入口。每个任务回调只见该操作的小接口，仍由同一个私有 SQLite 事务执行。

| 生产消费者 | 提交端口 / 原子写集合 | 事务内身份、前值与失败规则 |
| --- | --- | --- |
| query_command、command_processor、worker_events/feedback | Commands；命令受理、first-wins 答案、处理结果 | 重复请求/内容、答案版本和当前 run/服务身份重检；失败不确认消费 |
| service_commit、worker 转换及绑定/回执附件 | Worker.CommitWorker；Before/After、Decision/Operation/Event、绑定与附件、checkpoint | authority/control revision、当前 Attempt、固定执行身份和原生操作摘要均在原事务重检；拒绝恢复私有候选，intent/receipt 屏障及人工恢复门槛保留 |
| native_batch_command | TransactTaskBatch；Batch/Task/Contract/Dependency/Event/Command receipt | 固定 project、authority、graph 和 request/content 重检；任一插入失败全部回滚 |
| native_claim_command、native_service | TransactClaim；Task CAS、固定 Workflow/Execution/Attempt、Operation intent、Event/Command receipt | task/authority/graph/Profile/模式/slot/前置交付身份重检；无外部 worker 启动藏在存储中 |
| native_dispatch_command、control_service | TransactDispatch；authority CAS、Operation、Event/Command receipt | 原请求与 authority version 重检 |
| native_task_predecessors | TransactPredecessors；依赖、open Task 版本 CAS、graph revision、Operation/Event/Command receipt | 原 graph/task、未领取且无交付/活动 workflow 重检 |
| native_discard_command | TransactDiscardIntent / UpdateDiscardReceipt / TransactDiscardPending / TransactDiscard | intent+Command+Event，未知回执+Command+Event，最终 Task CAS+graph+Operation+Command+Event 分别保持原屏障；未知 PR 只按原 intent 对账，不改派或撤销人工门槛 |
| native_delivery_command、worker_pr | TransactDelivery；唯一 TaskDelivery、Task CAS、graph revision、Event；晚到 merge 仅审计事件 | 原 authority/task/Workflow/Execution/contract/PR/provider 与后继资格同事务；失败不产生成功交付 |
| native_cleanup / native_service | TransactCleanupProgress / TransactCleanup | 精确资源 Operation 回执；确认清理与 Task pending-item CAS、必要停止 Workflow/Execution/Attempt、graph、Command/Event 同事务，清理不撤销交付 |
| conversation_service/replay/sources/position | Conversations；事件、正文版本、原始证据、source cursor 独立事务；能力报告独立记录 | 固定来源身份和 cursor/mapper 屏障；失败不推进已提交状态 |
| Store.Publish / Heartbeat、项目连接构造 | Metadata.SaveCheckpoint / Heartbeat、Projects.InitializeProject | checkpoint/heartbeat 只写元数据；初始化 authority 保持原独立事务与空库检查 |

SQL/扫描由原唯一实现接收：`sqlite_write_ports.go`、`sqlite_task_ports.go`、`sqlite_native_authority.go`、`sqlite_task_graph.go`、`sqlite_runtime_snapshot.go`、`sqlite_worker_commit.go` 及原 `conversation_store.go`。#70 整体搬迁时消费这些已接通的纯契约，不补迁生产消费者。

删除：DomainTx、DomainStore、公开 SQLiteStore.Transact、重复私有 taskGraphSnapshot、taskGraphFromStore、readNativeCommandByKey、无人消费 scanTaskDelivery；原 nativeContractForTask/Tx、lookupOperation 与 taskGraphSnapshotTx 转为类型化存储方法。测试既有断言不改名、不迁包；同包任意写 fixture 仅使用私有 SQLite executor，外层 reader/console fixture 使用正式初始化及 BatchTx，损坏 checkpoint 由测试本地 SQL 制造，未暴露测试专用生产写 API。

并行集成：不修改 contracts 的共享字段、CLI、console 默认源、应用组装、quick 或 #67 守卫入口。console 的既有 `project_source_test.go` 仅更新旧事务 fixture，若 #71 移走该测试，协调聊天应保留其断言与本次新 fixture 接线。


## 实际验证结果

最终命中：16 个既有定向顶层用例、6 个新增端口/边界/清理/过期 Attempt 顶层用例、2 个 #67 顶层用例、2 个外层 reader/console 顶层用例；run/pass 集合逐项匹配，无 fail/skip。一次既有 quick 的 summary 为 complete=true、passed=true、errors=[]，全部 7 组非零命中且无 skip/fail，执行约 334 秒。git diff --check 与 staged diff check 通过。精确命令及 JSON 证据位于上述证据目录的 commands.json、selection-results.json 和各组日志。

自查核对完整实际 diff、Spec/Plan、生产调用路径、原子写集合和事务内重检；worker SQL executor 的原前置和提交代码保持不变，29 个机械接线的既有测试文件断言保持不变。补齐原完整清理回执测试缺口，并验证 checkpoint/heartbeat 不写候选、过期控制版本和已被替换 Attempt 均拒绝。无需 L2/L3/full 或新增原生权限证据。提交前再次获取 origin/main，仍为上述基线；无未解决问题。
