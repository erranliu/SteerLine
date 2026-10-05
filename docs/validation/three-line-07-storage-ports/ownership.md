# #79 存储目录与消费者端口归属

本清单记录 #79 在当前工作树的实际迁移。旧路径仅用于说明迁移来源；实现没有保留旧路径读取、迁移或转发门面。

## 文件与类型迁移

| 旧归属 | 新归属 | 迁移内容与所有者 |
| --- | --- | --- |
| `internal/steerline/sqlite/*.go` | `internal/storage/sqlite/*.go` | 唯一 SQLite Store、Reader、schema、SQL、事务/CAS、只读行为、会话存储、执行和任务提交及对应 SQLite 测试；数据库存储层拥有。旧目录已删除。 |
| `internal/steerline/persistence/ports.go` | `internal/storage/ports/transactions.go` | 仅保留跨记录声明事务接口、WorkerMetadata 和 ProjectInitializer；宽 `Connection`、`Factory`、旧 `Projects`/`CenterReader` 聚合均删除。 |
| `internal/steerline/contracts/query.go`、`snapshot.go`、`internal/steerline/domain_snapshot.go` | `internal/storage/ports/query.go`、`native_task.go`、`execution_snapshot.go` | 产品只读能力、NativeTaskPage/Detail/Ready 值、ProjectView、ExecutionSnapshot 和 ExecutionStateSnapshot 由存储端口持有。ExecutionSnapshot 不含任务/交付；TaskGraphSnapshot 拥有任务/交付记录。 |
| `internal/steerline/contracts/runtime.go` 中的 checkpoint 类型 | `internal/storage/ports/checkpoint.go` | ServiceCheckpoint、checkpoint 内嵌的服务/仓库/控制值和 schema 常量属于存储值；Runtime 重建由 `execution/reconstruction` 完成。 |
| `internal/execution/stored_execution_validation.go` 的持久记录拒绝规则 | `execution.Config.ValidateStoredExecutions` + app 注入的只读 execution state reader | execution 在真实 worker-flow preflight 入口校验冻结 profile、binding、preparation intent、receipt/tool facts、Ready/session/generation 与 write-root；app/SQLite 只提供同快照值。 |
| Runtime / command / product query 的 project-wide execution 查询 | `ExecutionStateSnapshot`, `TaskCandidateSnapshot`, `WorkflowSnapshot`, `ActivityBinding` | Runtime 恢复保留整体 DAG 和完整执行事实；task command 只读取目标 task 的全量候选及完整 TaskGraph；Workflow/Activity 只读取选定身份所需事实。 |
| app ProjectView 与连接观察的分次读取 | `storage/ports.ApplicationProjectSnapshot` / `ApplicationProjectSnapshotReader` | SQLite 一次只读事务组合纯 `ProjectView`、同快照的 `ExecutionStateSnapshot` 与其读取错误；steerlineapp 在 app 边界重建并分类 Runtime。product/query 和 product/api 不携带 Runtime/Domain。 |
| `internal/product/query/conversation_ports.go` | `internal/storage/ports/conversation_read.go` | SQLite 与 product/query 共用的 conversation 只读能力移入 storage ports；页面和过滤 API 值使用 product/api。 |
| `internal/steerline/native_project_open.go` | `internal/steerlineapp/native_project_open.go` | 数据库路径、SQLite 打开、native authority 绑定、项目初始化、格式校验和 Runtime 读取归唯一应用组装。核心层仅保留由 app 注入的验证/初始化/读取回调。 |
| `internal/steerline/persistence/connections.go`、SQLite `Factory` | 删除；构造入口在 `steerlineapp/native_project_open.go`、`steerlineapp/client.go` | app 是唯一生产构造及关闭责任持有者；业务构造器只接收声明端口。 |

## 消费端口与构造

| 消费者 | 接收的能力 | 组合者/具体实现 |
| --- | --- | --- |
| `task` | 任务快照、固定契约、任务命令与任务专属批次/前驱声明 | `steerlineapp.taskCommandStore` 把 SQLite TaskGraphSnapshot 投影成 task-owned Reader；窄写接口由 `storage/ports` 声明。 |
| `execution` | 原子 `ExecutionStateReader`、任务候选前值 reader、`CommandStore` 写入声明、`WorkerMetadata`、`ConversationStore`、`ExecutionRecordReader` | `execution.ServiceStorage` 用独立接口字段接收这些能力和 `Close` 回调；app 单独打开一份 SQLiteStore 并在 app 边界关闭。CommandStore 不暴露 project-wide Query；candidate/task 前值与 TaskGraph 在同一事务读取。 |
| `product/query` | Workflow(id)、Event、Task、Project、Activity 各自只读端口 | `product/query.NewService(storageports.ProductReader, project)` 只把实现投影到分开的读取字段；Workflow 与 Activity 在 SQLite 事务内按 workflow/execution/attempt identity 取值，Activity 与事件共享同事务。 |
| core host/control | 由 app 配置的验证、native authority/Runtime 读取、task 和 command 端口 | core 不打开数据库，不导入 SQLite。 |

SQLite 保留对 `product/query` 的纯调用仅限 `History`、`ReadySnapshot`、`SetTaskPageCursor` 及数据常量；没有调用 Query/Activity/Task/Project/Command 查询用例。`coreboundary` 的 `TestTM07StorageBoundary` 对 adapter/app 依赖和读取用例调用设负向夹具。

## 迁入测试及断言所有者

以下测试名是最终 package 的父测试名；实际子测试命中和顺序由同目录 `summary.json` 及 `*-01/test.jsonl` 记录。

| 组 | 最终 package | 父测试 | 断言所有者 |
| --- | --- | --- | --- |
| `sqlite-01` | `internal/storage/sqlite` | `TestSQLiteTransactionRollsBackAllDomainWrites`; `TestProjectViewSharesTransactionAcrossWriterCommit`; `TestTM05ActivitySharesTransactionAcrossWriterCommit`; `TestCM70ReadOnlyOpenPreservesFiles`; `TestCM70WindowsCloseReleasesHandlesAndMissingReaderCannotCreate`; `TestConversationWriteFailureRollsBackContentAndCursor` | SQLite 原子回滚、项目/活动读取事务、只读不建库和句柄释放、conversation 写屏障。 |
| `execution-01` | `internal/execution` | `TestMC1`; `TestMC7`; `TestTM02ResultCAS` | Execution 候选领取/身份与历史，结果唯一提交、原回执幂等、CAS 冲突与交付/丢弃仲裁；真实临时 SQLite fixture。 |
| `product-01` | `internal/storage/ports`, `internal/product/query` | `TestTM01StoragePortDependencies`; `TestTM05PageProjection`; `TestNativeCandidateSnapshotKeepsDispatchOrderBeyondDetailPage` | storage ports 依赖边界、公开页面投影、501 候选超详情页排序。 |
| `storage-boundary-01` | `internal/coreboundary` | `TestTM07StorageBoundary` | SQLite 禁止 adapter/app 与 product/query I/O 反向依赖。 |
| `product-boundary-01` | `internal/steerlineintegration` | `TestCD67CoreBoundary` | #77/#78 既有 core/query/console 依赖反例和 Windows/Linux 生产图；reverse storage fixture 使用新 storage/sqlite 路径。 |
| `wiring-01` | `internal/steerlineintegration` | `TestTM07StorageWiring` | app 同库构造、task/execution/product 端口字段、app 持有 Close 与重开读取。 |
| `wiring-regression-01` | `internal/steerlineintegration` | `TestCD67AdditionalImplementation` | #78 保留原生 app-server/terminal lifecycle、原输入和重启后人工答复；CommandReader/Writer 由 app 注入。 |
| `preflight-01` | `internal/steerlineintegration` | `TestTM07PersistedExecutionPreflight` | 真实 app 服务构造前拒绝当前 schema execution 缺失 Preparation；确认 unsupported_schema 且无数据库写入。 |
| `runtime-snapshot-01` | `internal/steerlineintegration`, `internal/execution` | `TestTM07RuntimeUsesSampledAuthorityAndCheckpoint`; `TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor` | 真实 app Runtime 及 execution fixture 都从同一 checkpoint/graph/records snapshot 构造；fixture reader 有非零 L1 `quick-execution` owner 选择。 |
| `bounded-read-01` | `internal/steerlineintegration` | `TestTM07ScopedActivityAndWorkflowReaders` | 真实 product Activity 与 Workflow(id) 读取只采样目标候选；损坏的另一候选 operation 不影响成功读取，事件仍按固定 identity 同事务采样。 |
| `bounded-command-01` | `internal/steerlineintegration` | `TestTM07CandidateCommandUsesTaskScopedSnapshot` | 真实 candidate command 由按 task 的全部候选事实和 TaskGraph 前值驱动；无关 operation 损坏不影响读取，CommandStore 不再提供 project-wide Query。 |
| `mainline-runner-01` | `cmd/test-feedback` | `TestPrepareFullCoreEnumeratesExecutionBusinessTests`; `TestFullCoverageRequiresExecutionPartitionsAndCompleteOtherOwners` | #224 Full 分区覆盖 execution 业务 owner，coverage 校验拒绝遗漏、重复、旧 owner 和错误 owner；不启动 Full suite。 |
| `mainline-fixtures-01` | `internal/execution`, `internal/steerline` | capability/resource、WorkerFlow preflight、project view、committed task document 定向父测试 | #224 file resource 注入、execution profile/旧 checkpoint 拒绝、host SQLite fixture 与 managed Git task-document 验证均归真实 owner；每包指定父测试非零命中。 |
| `mainline-e2e-01` | `tests/e2e/internal/testenv` | `TestBusinessDeadlineIsFrozenBeforeIndependentCleanup`; `TestCandidatePublicationFailureRequiresCompletedOriginalFeedback`; `TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions` | #224 business deadline、独立 cleanup 与候选 feedback 边界；重复 cleanup 保留冻结结果；E2E snapshot composer 拒绝 page 与 execution 来自不同 database revision。 |
| `project-snapshot-01` | `internal/storage/sqlite`, `internal/steerlineapp` | `TestSQLiteApplicationProjectSnapshotSharesTransactionAcrossWriterCommit`; `TestFileQuerySourceReadsCombinedProjectView` | SQLite 唯一 owner 的 product/runtime WAL 交错断言，以及真实 app client 复用同一 combined read 的当前数据库/缺 checkpoint/error 行为。 |
| `compile-only-01` | 全仓 | `go test -run '^$' ./...` | 保存 stdout、stderr 与实际退出码，仅证明编译；不作为行为回归证据。 |

现有 L1 保留 SQLite quick-rule package、`TestCD67CoreBoundary`、#77 的活动快照和产品视图测试，以及 #78 的执行/adapter/产品边界测试；本次新增 `TestTM07StorageBoundary` 与 `TestTM07StorageWiring`。完整 L1 命中见[验证摘要](README.md)。
