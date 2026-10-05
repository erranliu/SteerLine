# Plan：存储目录与消费者端口收口

任务 ID：79；任务名称：存储目录与消费者端口收口。唯一执行依据：[Spec](three-line-07-storage-ports.spec.md)。硬前置：#75、#76、#77。一个独立实现 PR 交付。

Spec 路径：`docs/spec/three-line-07-storage-ports.spec.md`。Spec SHA-256：`A26308A7919B822AEED91382D542660EE92D9BD90A674CD3E8CD2ADABEA6D606`。集成后的代码基线：`origin/main` `d532c5f13c1d3269c019a8cdee6df008580c0446`；#79 原始测试基线：`b49dec2cc27455fbf8044651a9975d28293aa372`。

## 实施步骤

1. 按三线实际需求收窄端口/声明，移动 query 依赖的读取契约到 ports，保持生产闭包无反向依赖。

2. 迁唯一 sqlite 和所有 app 工厂；任务/执行/产品改为各自端口，移除全 Domain 读取。

3. 迁已有 SQLite 原子/只读/快照代表，更新 L1、schema 版本说明和消费者所有权。

实施中维护 `docs/validation/three-line-07-storage-ports/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下测试按当前真实所有者和调用路径选择；测试重命名、搬迁时在同次实施中更新到最终 package、parent/subtest，并记录旧/新位置。SQLite 层保留原回滚、只读、会话写屏障、项目快照与 #77 活动快照断言；业务提交用 execution 的真实 SQLite fixture 覆盖候选领取、结果 CAS、唯一交付与丢弃竞争；产品 501 候选排序断言仍归 product/query。`TestMC*` 的实际来源是 `internal/execution/candidate_workers_test.go`，不再标成旧 `internal/steerline`。

环境：Windows；每个定向命令使用独立 STEERLINE_HOME、Git/SQLite/工作目录和受控后端，不设置原生 opt-in；涉及深层 Git worktree 的 wiring 回归和最终 L1 使用短 SystemDrive 临时根。L1 的每个 quick 组由 runner 再分配独立 home/temp。所有 `-out` 均指向本次新建的空运行目录；保留此前报告，不覆盖。保留原始 Go JSON、runner summary、测试身份/版本/原请求回执和必要清理结果。下表中的 `$run` 是每次运行新建且为空的 `docs/validation/three-line-07-storage-ports/<group>-<run-id>` 目录；实际命令和证据绝对/相对路径写入对应 summary。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM07.2–TM07.3 `sqlite-01` | Windows；临时当前格式 SQLite、无模型 | `go run ./cmd/test-feedback -out $run -- ./internal/storage/sqlite -run '^(TestSQLiteTransactionRollsBackAllDomainWrites|TestProjectViewSharesTransactionAcrossWriterCommit|TestTM05ActivitySharesTransactionAcrossWriterCommit|TestCM70ReadOnlyOpenPreservesFiles|TestCM70WindowsCloseReleasesHandlesAndMissingReaderCannotCreate|TestConversationWriteFailureRollsBackContentAndCursor)$' -count=1` | 唯一 SQL/端口与读取调用者迁入后 | 原子回滚、项目与活动同快照、只读不建库/关闭释放句柄、会话写屏障各有非零命中并通过，无 skip | `$run/summary.json` 及 `sqlite-01/` 原始 JSON |
| 定向 Go：TM07.2 `execution-01` | Windows；execution 临时 SQLite/Git/受控 provider fixture | `go run ./cmd/test-feedback -out $run -- ./internal/execution -run '^(TestMC1|TestMC7|TestTM02ResultCAS)$' -count=1` | execution 声明端口接通后 | `TestMC1`/`TestMC7` 原有领取及候选身份/历史断言通过；`TestTM02ResultCAS` 的唯一交付、原回执幂等、CAS 拒绝/不部分提交和交付/丢弃仲裁子例全部通过 | `$run/summary.json` 及 `execution-01/` 原始 JSON |
| 定向 Go：TM07.1 `product-01` | Windows；纯内存读取值和临时固定图 | `go run ./cmd/test-feedback -out $run -- ./internal/storage/ports ./internal/product/query -run '^(TestTM01StoragePortDependencies|TestTM05PageProjection|TestNativeCandidateSnapshotKeepsDispatchOrderBeyondDetailPage)$' -count=1` | 端口边界与消费者切换后 | 纯端口依赖规则、公开页面投影、501 候选/超详情页排序断言均有非零命中并通过 | `$run/summary.json` 及 `product-01/` 原始 JSON |
| 定向 Go：TM07.1 `storage-boundary-01` | Windows；源码扫描负向夹具，不开数据库 | `go run ./cmd/test-feedback -out $run -- ./internal/coreboundary -run '^(TestTM04ExecutionBoundary|TestTM07StorageBoundary)$' -count=1` | 存储目录迁移及重建端口依赖加入既有守卫后 | SQLite 拒绝 adapter/app 和 product/query 读取用例；execution/reconstruction 接受 storage/ports 纯值但拒绝 SQLite；#76 直接依赖反例通过 | `$run/summary.json` 及 `storage-boundary-01/` 原始 JSON |
| 定向 Go：#77/#78 guard `product-boundary-01` | Windows；源码扫描负向夹具，不开数据库 | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 新目录路径登记和 storage ports 纯依赖许可加入后 | #77 产品/API/只读边界、#78 core 反向依赖反例、全 OS/tag 扫描及 Windows/Linux 生产依赖图通过 | `$run/summary.json` 及 `product-boundary-01/` 原始 JSON |
| 定向 Go：#78 continuation `wiring-regression-01` | Windows；短临时根，隔离 Git/SQLite/受控后端 | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestCD67AdditionalImplementation$' -count=1` | app decision 读取/写入端口注入完成后 | 原生 app-server/terminal 和固定输入/恢复子例全部通过，decision 提交由应用注入 command read/write ports | `$run/summary.json` 及 `wiring-regression-01/` 原始 JSON |
| 定向 Go：TM07.1、TM07.3 `wiring-01` | Windows；隔离 app home、临时 SQLite 与受控后端 | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestTM07StorageWiring$' -count=1` | app 唯一构造与窄端口注入接通后 | 真实 app 工厂读写同一数据库；task、execution、product 获得各自消费者端口；关闭由应用唯一持有；无完整连接/Runtime 写端口泄漏 | `$run/summary.json` 及 `wiring-01/` 原始 JSON |
| 定向 Go：TM07.2 startup admission `preflight-01` | Windows；隔离 app home、临时 SQLite/Git/受控后端；当前 schema 记录 | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestTM07PersistedExecutionPreflight$' -count=1` | execution 的持久记录预检端口接回后 | 真实 app `NewServiceWithLifecycleResolver` 在写库/副作用前拒绝缺失 Preparation 的当前格式 execution；断言 `unsupported_schema` 且数据库版本未变 | `$run/summary.json` 及 `preflight-01/` 原始 JSON |
| 定向 Go：TM07.2 atomic runtime `runtime-snapshot-01` | Windows；临时 WAL SQLite 与真实 WorkerCommit writer | `go run ./cmd/test-feedback -out $run -- ./internal/storage/sqlite ./internal/steerlineintegration -run '^(TestSQLiteExecutionStateSnapshotSharesTransactionAcrossWorkerCommit|TestTM07RuntimeUsesSampledAuthorityAndCheckpoint)$' -count=1` | checkpoint/graph/execution 统一到一个读事务、app/runtime 消费该端口后 | writer 在 checkpoint 采样后提交 WorkerCommit；第一次读取完整看到旧 revision，下一次读取/真实 app Runtime 完整看到新 checkpoint、authority 与 execution，不接受混合事实 | `$run/summary.json` 及 `runtime-snapshot-01/` 原始 JSON |
| 定向 Go：TM07.1 bounded product reads `bounded-read-01` | Windows；真实 app SQLite；两个完整候选及一条损坏的无关 operation | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestTM07ScopedActivityAndWorkflowReaders$' -count=1` | Workflow/Activity reader 改为按 identity 查询后 | 候选 B operation payload 损坏时，候选 A 的真实 Activity 与 `Workflow(id)` 仍成功；所选 execution/attempt 归属、事件身份和同事务语义仍通过 | `$run/summary.json` 及 `bounded-read-01/` 原始 JSON |
| 定向 Go：TM07.1 bounded command reads `bounded-command-01` | Windows；真实 app command store；多候选及损坏的无关 operation | `go run ./cmd/test-feedback -out $run -- ./internal/steerlineintegration -run '^TestTM07CandidateCommandUsesTaskScopedSnapshot$' -count=1` | CommandStore 改用任务候选前值端口后 | 新候选命令只解码目标 task 的全部候选 workflow/execution 与必要 TaskGraph；无关 operation 损坏不影响命令，历史候选集合仍完整；CommandStore 不暴露 project-wide Query | `$run/summary.json` 及 `bounded-command-01/` 原始 JSON |
| 集成定向 Go：mainline runner `mainline-runner-01` | Windows；测试编排包，隔离临时目录 | `go run ./cmd/test-feedback -out $run -- ./cmd/test-feedback -run '^(TestPrepareFullCoreEnumeratesExecutionBusinessTests|TestFullCoverageRequiresExecutionPartitionsAndCompleteOtherOwners)$' -count=1` | 合入 #224 runner/Full owner 变更后 | Full 的 execution 分区确实枚举迁移后的非零业务测试；coverage 检查保留全部其他 owner、禁止漏测/重复和错误 owner | `$run/summary.json` 及 `mainline-runner-01/` 原始 JSON |
| 集成定向 Go：mainline fixtures `mainline-fixtures-01` | Windows；execution + host/task fixtures，无原生 opt-in | `go run ./cmd/test-feedback -out $run -- ./internal/execution ./internal/steerline -run '^(TestCapabilityReportRejectsRetiredWholeRunActions|TestNativeCapabilityCheckKeepsFixedAgent|TestExecutionRequirementsDoNotUpgradeProbeToVerified|TestFreshWorkerFlowRuntimeUsesDistinctFormat|TestWorkerFlowRejectsOldRuntimeBeforeWorkspacePreparation|TestWorkerFlowRejectsOldLocalExecutionProtocol|TestProjectViewDoesNotOwnServiceRuntime|TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences|TestTaskContractRejectsCommittedSymlinkReferences)$' -count=1` | 合入 #224 File resources、SQLite fixture、managed Git 与 heartbeat fixture 修正后 | capability 证据使用 File resource port；checkpoint/service fixture 使用当前 storage owner；task committed-file 校验经 managed Git 通过；所有者内每个包有真实非零命中 | `$run/summary.json` 及 `mainline-fixtures-01/` 原始 JSON |
| 集成定向 Go：mainline E2E helper `mainline-e2e-01` | Windows；E2E testenv 本地 fixture，无 live provider | `go run ./cmd/test-feedback -out $run -- ./tests/e2e/internal/testenv -run '^(TestBusinessDeadlineIsFrozenBeforeIndependentCleanup|TestCandidatePublicationFailureRequiresCompletedOriginalFeedback)$' -count=1` | 合入 #224 deadline、独立 cleanup 与候选样本复用后 | deadline 在 cleanup 前冻结，cleanup 失败不覆盖业务结果；候选失败仍要求完成原始 feedback/body；测试不启动真实模型/PR 流程 | `$run/summary.json` 及 `mainline-e2e-01/` 原始 JSON |
| 诊断编译 `compile-only-01` | Windows；本机 Go toolchain；不建业务现场 | `go test -run '^$' ./...`，将 stdout、stderr、退出码分别保存到新证据目录 | 所有实现与定向回归完成后、最终 L1 前 | Go 命令退出码为 0；仅作为编译证据，不记为行为测试通过 | `$run/stdout.log`、`$run/stderr.log`、`$run/exit-code.txt` |
| L1 固定最小入口 `L1` | Windows；全新隔离 STEERLINE_HOME/临时测试资源，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out $run` | 自查和定向修复完成后，最终只运行一次 | 全部固定 quick 组及当前 L1 非零条目真实命中并通过，无 skip、零匹配或意外 live；保留产品纯 view、TM05 活动事务、TM06 守卫与 #77/#78 所有既有选择 | `$run/summary.json`、各组 Go JSON/stderr |

所有者切换必须同步修订 `cmd/test-feedback/scoped.go` 的 package、parent/subtest 与边界归属，并核对 L1 固定项仍实际非零命中。完整性只运行上表定向组、诊断编译和一次最终 L1；不跑 Full、L2、L3 或 live：本次修复恢复当前格式拒绝与同快照/有界读取，不改变协议或记录形状。独立复审发现的问题由新增定向组覆盖，旧 431 个 L1 事件不替代这些断言。#224 的 Full 分区和 fixture/E2E 边界由对应最小定向组验证，不重跑 Full、L2、L3 或 live。若记录格式或恢复事实实际改变，依原 Spec 追加 `recovery.waiting-merge-restart` 受控验证，不扩大旧格式兼容。

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：不改变协议或原生启动。L3 controlled 最终组合归 #82；若记录格式/恢复实际改变，追加 recovery.waiting-merge-restart，不扩大旧格式兼容。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-07-storage-ports/README.md`，注明复用证据、定向重验依据及未具备能力。

#80 接唯一连接与窄工厂/查询端口；#75/#76/#77 已整体交付才完整实施，接口草案不解除硬前置。
