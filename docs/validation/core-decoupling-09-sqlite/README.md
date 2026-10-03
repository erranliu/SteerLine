# #70 SQLite 写入实现归包验证

状态：实现已迁移，CM70-2 严格只读文件验收 `needs_user_decision`；不得按整体通过或可合并处理。工作区 `C:/Users/lmas1/.codex/worktrees/e8e1/SteerLine`，分支 `codex/cm70-sqlite-storage`。初始基线 `e81fadaf59d0411af1594be7c5cd6a832c71c29f`，已先集成 #71 PR195 的 `6dce3cf1abf293e18bd78f92677c72a3fa2f0210`，发布前再次集成 #72 PR196 的最新 main `0b5e8b342214e2d286b65b0494bfe5fad2c552d7`；执行模型按本次用户指定 `gpt-6.1-sol/high`。仅实施 #70；#72 只消费其已合并交付，不另行实施 UI/产品语义。

## 实际所有者与生产接线

- `internal/steerline/sqlite` 是连接、schema 11、SQL、声明提交、读写事务、附件格式及新库初始化失败清理的唯一实现。复用既有 `Reader`，没有复制读取路径或数据库模型。
- `persistence` 保留 #69 的各消费者窄端口，新增应用资源/工厂、类型化附件读取及纯错误分类。无 SQL/任意事务、原生客户端或测试写入口。SQL executor 私有；公共方法只消费既有声明和纯值。
- `steerlineapp/config.go` 选择唯一工厂；`app.go` 先调用中心 `PrepareServiceConfig` 完成原受理/恢复预检，再取得实际连接并注入 `NewService`。服务仅保存消费者所需端口及关闭回调，构造器不再开 writer。
- #71 `steerlineapp/client.go` 使用相同注入工厂和连接生命周期，内部句柄改为纯端口，错误消费纯 `ErrNotFound`。CLI、clientcontracts、steerlineclient、console 产品 DTO/语义不变；未留旧构造路径，也不要求 #71/#72 搬存储。
- 事务内继续重检 CAS 前值、版本、项目/目标/代次、工作流与执行/Attempt 关系及固定身份。SQL 条件/表定义/当前版本保持；驱动锁/IO 与缺记录/格式/身份/版本拒绝区分。

## 原子集合与删除

1. worker Workflow/Execution/Attempt（含退休代次）/Decision/Operation/Event、固定执行绑定/操作附件/receipt 和 checkpoint 同一事务；提交失败不发布、不 ack、不授权下一步。
2. 命令受理及同内容幂等/异内容冲突保留；批次任务/合同/依赖/回执、领取与固定执行记录、真实唯一 TaskDelivery 及后继资格各保留原事务和约束。
3. 会话 event/body/raw/source cursor 独立原子事务；heartbeat 只发布服务元数据。Runtime/ProjectView/任务页与 WAL 的同一快照保留。
4. 中心删除原 `sqlite_store*`、`sqlite_worker_commit`、`sqlite_write_ports`、任务 SQL、会话 SQL、schema 检查及附件 SQL。中心 business Store 的串行/故障屏障和恢复规则保留。执行绑定/操作的持久 JSON 及身份核验归存储，中心不传表名。
5. 不新增兼容读取、迁移、双写、回退或格式版本。SQL/测试写访问没有导出。

## 共享修改

应用连接构造涉及 `steerlineapp/app.go`、`config.go`、`client.go`，以及中心 `NewService` 的纯连接参数和准备接线。测试入口仅更新 #67 新的存储反向依赖/中心 SQL import 两个负向场景的 quick 命中清单。console/产品契约归客户端线，本文未替它实施 #72。

## 命令、命中与结论

精确顶层/子场景命中和失败清单见 `hits.json`，原始 Go JSON 日志均保留。没有使用零命中或 skip 代替验收。

- `center-current.jsonl`：#69、worker 回滚/声明边界、会话、CAS/唯一交付、未知 Stop 结果/原绑定、恢复代次与附件的代表断言全部通过。
- `storage-current.jsonl`：迁入 SQL/schema/CAS/回滚/幂等/cursor/分页/WAL 断言与 Windows 关闭、锁分类通过；唯一失败为 `TestCM70ReadOnlyOpenPreservesFiles`。`owner-recheck.jsonl` 验证后续错误分类/所有者自查修改。
- `wiring.jsonl`：实际应用与 #67 第三实现/双 OS 图。新增工厂检查一度改变缺目录错误优先级，保留失败记录；修正后 `app-current.jsonl` 的原断言和实际客户端 ProjectView 通过。`guard-final.jsonl` 全标签、Windows/Linux 守卫通过。
- `initialization-current.jsonl`：新库一次初始化、旧任务数据库拒绝、退休 projection 拒绝通过；失败清理已归存储，现有文件不删除。暂时误删仍有中心消费者的通用 session getter 导致编译失败，已恢复并定向验证；诊断保留在 `initialization-build-failure.jsonl`。
- `command-current.jsonl`：最后的数据库故障分类修正后，原命令幂等/Decision 仲裁与 first-wins 断言通过。附件最终类型化读取另见 `attachment-final.jsonl`。
- 稳定接线的 `go run ./cmd/test-feedback -quick -out docs/validation/core-decoupling-09-sqlite/quick-reviewed`：其余六组通过，quick-rules 仅因严格只读文件断言失败，完整入口未通过。前两次诊断 quick 日志保留：首次暴露迁入的子进程 helper skip；helper 已改为 TestMain 调度，原五个退出边界断言不变，后两次无 skip。后续初始化/错误分类只做受影响定向复验。
- `git diff --check` 通过。未跑 full、L2/L3 或真实模型/平台认证；本项未改变启动、权限及原生支持矩阵。

## 严格只读失败及待决边界

合法 WAL 主库在最后一个 writer 关闭后没有辅助文件，既有 `mode=ro` + `query_only(1)` 读取会新增 0B WAL 与 32KiB SHM。主库及业务版本未变，变更能力拒绝；但原 CM70-2 要求“只读前后无文件/业务写入”，严格文件集合断言失败。首次/最终失败保留，Spec 和该断言没有修改，不声明例外已获授权。

[SQLite 官方 WAL Read-Only Databases](https://sqlite.org/wal.html#read_only_databases) 说明辅助文件条件；immutable 会改变锁/变更检测，不能用来代替本项的并发 WAL 读取。协调聊天已独立复现并向用户提出有限辅助文件边界草案，当前等待明确批准。此 PR 只能为 draft，批准并更新相应 Spec/断言、验证后才可作为整体交付。其他事务断言不因此豁免。

## 测试迁移映射

以下同名测试迁包保留原业务/SQL断言；数据来源名称继续使用原中性 fixture 值。

| 原测试名 | 原文件 | 当前存储文件 |
| --- | --- | --- |
| `TestConversationSQLiteDedupesBodiesAndPagesUnicodeByRevision` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationCommitDoesNotAdvanceCheckpointOnInvalidBody` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationBodyAndRawCorruptionCannotReadAsComplete` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationLongUnicodeFinalIsExactAcrossPages` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationWriteFailureRollsBackContentAndCursor` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationSQLiteFullRollsBackContentAndCursor` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationLateOldAttemptCannotCrossNewAttempt` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationProcessExitBeforeCommitRecoversAllBoundaries` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationCommitRejectsChangedRawAtSameSourceIdentity` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestConversationCheckpointPreflightRejectsUnanchoredState` | `internal/steerline/conversation_test.go` | `internal/steerline/sqlite/migrated_conversation_test.go` |
| `TestNativeAuditPaginationBoundsAndDatabaseIdentity` | `internal/steerline/native_task_audit_test.go` | `internal/steerline/sqlite/migrated_native_task_audit_test.go` |
| `TestNativeAuditPageDecodesOnlyDirectPredecessors` | `internal/steerline/native_task_audit_test.go` | `internal/steerline/sqlite/migrated_native_task_audit_test.go` |
| `TestNativeAuditDependencyCycleRollsBackAndEdgesAreImmutable` | `internal/steerline/native_task_audit_test.go` | `internal/steerline/sqlite/migrated_native_task_audit_test.go` |
| `TestNativeAuditTaskCASComparesInstantsNotTimeRepresentation` | `internal/steerline/native_task_audit_test.go` | `internal/steerline/sqlite/migrated_native_task_audit_test.go` |
| `TestNativeAuditQueryRejectsPayloadIdentityDrift` | `internal/steerline/native_task_audit_test.go` | `internal/steerline/sqlite/migrated_native_task_audit_test.go` |
| `TestProjectViewRejectsNilReader` | `internal/steerline/project_view_test.go` | `internal/steerline/sqlite/migrated_project_view_test.go` |
| `TestProjectViewSharesTransactionAcrossWriterCommit` | `internal/steerline/project_view_test.go` | `internal/steerline/sqlite/migrated_project_view_test.go` |
| `TestProjectViewKeepsReadOnlySectionsWithoutCheckpointOrSelection` | `internal/steerline/project_view_test.go` | `internal/steerline/sqlite/migrated_project_view_test.go` |
| `TestSQLiteV3IsRejectedWithoutUpgrade` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestSQLiteV4IsRejectedWithoutUpgrade` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestSQLiteV5IsRejectedWithoutUpgrade` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestSQLiteRejectsParallelExecutionsInOneWorkflow` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestSQLiteTransactionRollsBackAllDomainWrites` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestPreflightRejectsRetiredWholeRunResultCommandWithoutMutation` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestQueryEventCursorAndLargeBodyPaging` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestOperationIdempotencyKeepsOneDurableIntent` | `internal/steerline/sqlite_store_test.go` | `internal/steerline/sqlite/migrated_sqlite_store_test.go` |
| `TestNativeDatabaseInitializesCurrentSchema` | `internal/steerline/native_tasks_test.go` | `internal/steerline/sqlite/schema_ownership_test.go` |
| `TestSchemaOneCenterDatabaseIsRejectedWithoutMutation` | `internal/steerline/native_tasks_test.go` | `internal/steerline/sqlite/schema_ownership_test.go` |

`sqlite_store_test.go` 中依赖中心 CommandService/actor 的 first-wins、仲裁、checkpoint 控制断言归 `command_persistence_test.go`；平台/真实应用集成断言继续在外层。核心中性 fixture 仅在 `_test.go` 中以既有窄声明端口准备现场，必要损坏注入用测试自己开的 SQL 连接；无生产测试写接口。`TestConversationProcessExitMidTransactionHelper` 是子进程调度器，现归 TestMain；父 `TestConversationProcessExitBeforeCommitRecoversAllBoundaries/{raw,body,event,cursor,committed}` 的退出码和恢复断言保持。

自查已阅读实际完整 diff、Spec/Plan、应用/客户端唯一构造、全部搬迁事务和消费者。未解决项仅严格只读辅助文件要求；等待用户决定及协调聊天独立复审，不自行合并、启动任务或创建 automation。

## 发布前主干集成

最新 #72 主干在发布前已合并进本分支。共享冲突顺序集成在 architecture、coreboundary、app/client.go 及客户端 ProjectView 测试：保留 #72 的纯 TUI/资源工厂/配置视图，唯一实际连接使用 #70 的纯端口。相对最新 origin/main，console、clientcontracts 和 steerlineclient 没有本任务的产品实现差异；不保留旧 Store 构造，也不要求客户端线补搬迁。

`post-main-guard.jsonl` 再验全标签及 Windows/Linux 的中心、console 图和两个新增存储负向场景；`post-main-wiring.jsonl` 再验实际 CLI 批次校验/创建/废弃、ProjectView、零/部分/显式项目配置及 CM72 注入/退出资源语义。`post-main-quick/summary.json` 记录集成后的最后一次既有 quick，原严格只读断言仍未获授权变更，必须如实保留失败。主干整合后重新阅读共同 Spec/Plan、实际生产构造与冲突 diff；未发现第二接线或公开产品 DTO/语义变更。

集成后最小接线全通过，`post-main-quick` 七组均非零命中：core 11、rules 128、remote 15、codec 5、identity 3、app 6、boundary 35，均无 skip；仅 rules 中严格只读文件断言失败，整体仍失败。最后自查保留了 `NewService` 内不可绕开的中心受理校验（与 PrepareServiceConfig 共用），不让应用承担业务准入；`admission-current.jsonl` 复验非 native 拒绝、实际提交及应用构造。原始完整数据和结论见更新后的 `hits.json`。
