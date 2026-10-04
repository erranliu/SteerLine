# Plan：多候选会话与唯一PR交付

任务 ID：37；任务名称：多候选会话与唯一PR交付。唯一执行依据：[Spec](candidate-workers.spec.md)。硬前置：#36，一个独立实现 PR 交付。

1. 按当前源码盘点Task单运行假设，复用WorkflowRun、Execution、Query/Command、SQLite及现有异步单步副作用通道；不另建候选Service/引擎或single/multiple模式。保持worker-only发布、独立环境blocker和产品只读规范数据库，不恢复已删旧路径。
2. 接首个领取和人工start_candidate，各自冻结执行配置、共同base、独立原会话与工作区。开始时读取参与数，已达5个拒绝；不增加数量预留/CAS/槽位/队列，保留普通业务版本与幂等。
3. 接人工abandon_candidate：核验已知PR未合并与前值，退出实验回执与停止Operation分开；复用原后台动作和actor结果交接。慢/未知停止不阻塞新实验，晚到启动/反馈结果保存原事实但不重新授权，重启不复活。
4. 查询/反馈/正文与独立blocker精确绑定候选，环境动作保持原版本/服务实例，新反馈阻断时拒绝且不补投。活动与正文只读规范数据库，不尾随/回填raw日志；提供实际配置、状态、耗时与结果定位。
5. 接包含放弃历史的PR集合核验和唯一Delivery；处理新增/放弃/交付竞争、多合并和迟到事实，区别单候选放弃与整体Task丢弃。
6. 迁移完整消费者与代表测试，删旧单运行假设和重复实现；向 #38/#39交接真实查询/命令与中性夹具。实现后同步实际路径和版本，不修改已创建任务的固定契约或运行时DAG。

## 测试计划

TestMC1–TestMC9 的共享业务所有者是 `internal/steerline/candidate_workers_test.go`，不是原规划的适配器集成包。使用真实临时 SQLite、Git、单步 Lifecycle port、worker 子进程和 PR provider 子进程；不请求模型。原生混合方案和 TUI 继续分别归 #39/#38。生产路径为 `native_claim_command.go`（首个领取及新增）、`candidate_control.go`（独立控制回执）、`candidate_workers.go`（退出及完整 PR 集合）、`candidate_resources.go`（原回执与资源观察）、`native_delivery_command.go` / `native_discard_command.go`（事务仲裁）和 `projection`（全部候选）。SQLite 格式 13、runtime 格式 18；拒绝旧开发格式，保留原文件，无迁移。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向Go：MC1、MC2、MC7、MC9 | Windows；临时SQLite、Git和中性后端；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/identity-count-01 -- ./internal/steerline -run '^TestMC[1279]$' -count=1` | 身份、开始检查和格式路径接通后 | 实际命中各ID及子测试；领取一次、串行第6次拒绝、放弃后再增；不要求数量并发严格不超额 | 对应输出目录的summary/Go JSON与固定身份记录 |
| 定向Go：MC3–MC6、MC8 | Windows；隔离Service/SQLite，可控来源与PR provider；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/flow-abandon-01 -- ./internal/steerline -run '^TestMC[34568]$' -count=1` | 反馈、放弃与唯一交付接通后 | 原身份/请求、退出实验与真实停止区分、合并后拒绝、回滚及迟到事实正确；无fail/skip或零匹配 | 原始事件、PR/Delivery/停止回执与测试JSON |
| 定向Go：当前数据库、旧格式拒绝、投影 | Windows；隔离 SQLite；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/storage-02 -- ./internal/steerline/sqlite ./internal/steerline/projection -run 'TestSQLiteV(3\|4\|5\|12)\|TestNativeDatabaseInitializesCurrentSchema\|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding\|TestBuildOwnsRecords\|TestProjection' -count=1` | 格式与完整投影接通后 | 当前 reader/writer 与非迁移拒绝通过；每个选择包实际非零命中 | storage-02/summary.json 与 Go JSON |
| 定向Go：最终停止/迟到回执增量 | Windows；原 Service/SQLite、原 Operation；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/receipt-final-02 -- ./internal/steerline -run '^TestMC[58]$\|^TestSimplifiedCleanup\|^TestAppServerLateReply' -count=1` | 自查修复资源事实后 | 唯一交付不被真实停止覆盖；原回执事务屏障、退出后重启/不重发、无 fail/skip/零匹配 | receipt-final-02/summary.json、Go JSON |
| 定向Go：完整丢弃与原交付消费者 | Windows；原隔离 Git/SQLite/provider；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/settlement-final-02 -- ./internal/steerline -run '^TestMC7$\|^TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor$\|^TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent$' -count=1` | 自查修复历史 PR 消费者后 | 原断言保留；完整已知集合、未结算动作、迟到合并时间/归属、唯一交付通过 | settlement-final-02/summary.json、Go JSON |
| L1固定最小入口 | 本次测试 runner/fixture 隔离STEERLINE_HOME，无原生开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-workers/l1-03` | 定向稳定、自查完成后一次 | 守卫/原单候选代表命中且整体通过，记录源码及构建身份 | l1-03/summary.json与原始日志 |
| 跨 OS 编译代表 | Windows 主机、GOOS=linux；无运行、无模型 | `$taskOriginalGoOS=$env:GOOS; $taskLinuxOutput=Join-Path $env:TEMP ('candidate-workers-37-'+[guid]::NewGuid().ToString('N')+'.test'); try { $env:GOOS='linux'; go test -c -o $taskLinuxOutput ./internal/steerline } finally { if ($null -eq $taskOriginalGoOS) { Remove-Item Env:GOOS } else { $env:GOOS=$taskOriginalGoOS } }` | 生产调用链自查后 | Linux 代表源码及依赖可编译；不将其称为原生平台通过 | `docs/validation/candidate-workers/cross-os-rebase.txt` |

新用例复用当前隔离环境与worker侧发布fixture，不使用已退役host_service发布或旧DeferredDecision/Feedback。MC3覆盖一个blocker下拒绝/解除不补投与兄弟继续；MC4核对worker发布和raw日志不参与产品读取；MC8用受控慢/unsupported/unknown停止及晚到启动/反馈结果验证原回执屏障。复用既有断言，只补候选归属缺口，不增加全套单worker故障矩阵。证据目录每次使用新的空目录，重跑不得覆盖旧证据。数量边界不启动五个真实模型；不选择Full/L2/L3或新的权限认证矩阵，真实后端增量归 #39。

### 旧到新断言映射

| 原断言入口 | 新入口与保留/新增范围 |
| --- | --- |
| `TestWorkerFlowClaimNeedsNoProjectSteps` / `TestWorkerFlowContextsDoNotExchangeExecutionProfiles` | MC7 / MC2 迁入调用；MC1/MC9 新增五个不同冻结方案、独立 worktree/分支/session、领取一次、先重放再检查、退出后再增 |
| `TestSimplifiedLifecycleActorDoesNotWaitForEffect` | MC2 迁入原回执/错误 Attempt 屏障；MC8 新增退出期间的晚到启动/反馈、未知停止、原请求与兄弟新增 |
| `TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR` / `TestSimplifiedEnvironmentSavedInputRequiresFreshAnswer` | MC3 迁入独立 blocker/无补投断言；新增同 Task 两候选交错反馈与精确选择 |
| `TestPublicationPolicyFrozenConfigurationAndCredentials` / `TestConversationLiveSourceFailureKeepsGapAndMapperPosition` | MC4 迁入 worker-only 权限与规范数据库缺口断言；新增两个 worker 子进程真实 commit/push/create 与 PR 归属核验 |
| `TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged` / `TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor` | 原入口保留；MC5 新增多候选完整 PR 集合、事务回滚、唯一 winner/解锁与 replay；MC6 新增多合并/放弃合并/未知兄弟/交付后迟到冲突 |
| SQLite V3/V4/V5/V12 拒绝和当前 reader/writer、projection 断言 | storage 定向入口保留；MC7 新增全候选退出后的原 claim/base 重启与整体丢弃 |


实测回归还保留 `TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent` 的未结算 PR/迟到冲突字段断言；夹具只补真实 head repository/branch 身份。未削弱断言。此前失败产物保留在 validation 目录，最终通过位置以本表为准。

主线在本次执行期间合并 #205（`886a520e570c00f4a4e63fbbaaa22814604a4ee3`）。已 rebase；对受影响冻结发布配置/规范来源消费者增验 `go run ./cmd/test-feedback -out docs/validation/candidate-workers/rebase-consumers-01 -- ./internal/steerline -run '^TestMC[14]$' -count=1`，然后用 l1-03 在同步后的源码上执行固定最小入口。旧到新断言映射不变。

### 协调复审与主线集成验证

协调复审集成主线的原 intent/receipt 帮助方法，保留候选退出后的业务授权检查。以下在 Windows、隔离 Git/SQLite/中性生命周期环境执行，不请求模型。证据根为本 run 的 `C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-candidates-20261004-01a10247/`；每次使用新的空目录。

- MC1–MC9：`go run ./cmd/test-feedback -out <证据根>/coordinator37-mc-01 -- ./internal/steerline -run '^TestMC[1-9]$' -count=1`。主线集成后执行，要求全部场景非零命中、无 fail/skip；证据为 summary.json 和 Go JSON。
- 原回执及消费者：`go run ./cmd/test-feedback -out <证据根>/coordinator37-consumers-01 -- ./internal/steerline ./internal/steerline/sqlite ./internal/steerline/projection -run '^TestSimplifiedLifecycle|^TestSimplifiedCleanup|^TestAppServerLateReply|^TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor$|^TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent$|^TestSQLiteV(3|4|5|12)|^TestNativeDatabaseInitializesCurrentSchema$|^TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding$|^TestBuildOwnsRecords$|^TestProjection' -count=1`。集成后核对原请求、事务屏障、精确停止、唯一交付及当前格式；每包非零命中且无 fail/skip；同目录保存报告与原始事件。
- 最终固定 L1：`go run ./cmd/test-feedback -level=L1 -out <证据根>/coordinator37-l1-01`。最终内容稳定后执行一次，要求七个固定组通过、无 fail/skip/零匹配；summary.json 记录源码与构建身份。不增加 Full/L2/L3 或原生验收矩阵。
