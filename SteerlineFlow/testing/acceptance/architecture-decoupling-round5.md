# 架构解耦第五轮验收：宿主 checkpoint 与状态契约

日期：2026-10-17。归属：用户直接授权的第五轮实现与提 PR，无 roadmap Task ID。源码基线：主干 `64f918536a51c26a574e119e2419286f7f195725` 加本轮变更；前四轮已随 PR #350 合入。现状图见[代码架构](../../../docs/architecture/current-code-architecture.md)，关闭契约见[项目生命周期](../../spec/application-project-lifecycle.spec.md)，worker 规则仍归[生命周期矩阵](../../../docs/product-design/worker-lifecycle.md)。

## 实现与范围

- 新增纯 `internal/hostcontract`，统一拥有 `ServiceCheckpoint`、宿主 `ServiceState` / `ControlState` / `ControlOperation` / `ControlBlocker` / 仓库绑定、内部 `ControlReceipt`，以及 `ProcessIdentity` / `ProcessProbeFacts` / `ProcessFactsPort`。它只依赖标准库和公开 `runnercontract` 构建 pin，不持有存储、产品 DTO、资源句柄或关闭实现。
- 删除存储端口 checkpoint 定义、重建层重复的宿主状态、执行模型控制回执和 backend 进程观察定义；所有消费者直接使用新契约，无旧包别名、迁移、双读或回退。公开产品与 HTTP DTO 继续独立映射。
- SQLite 仍在原事务保存 checkpoint/心跳、递增版本并核验项目权威；应用组合读取仍在同一读事务采样 Task、候选、checkpoint。checkpoint schema 保持 21，数据库 schema 保持 23；14 项定义比对确认字段、JSON 标签或接口方法集保持。
- `reconstruction` 改为消费宿主契约和独立提供的 Task 派发权威，移除其 storage/ports 依赖。双向投影复制服务字段、进程/build pin、能力、原操作/回执与 blocker；补齐旧代码对预留任务及回执时间指针的浅拷贝，避免投影改动污染已采样事实。
- 进程事实实现仅替换契约类型，OS 取证、句柄关闭与 Job 语义未改。`ProjectRuntime` 仍最终持有并关闭 store/log/锁，actor/factory 借用资源；Read/Command/Task 排空、重复关闭、本地失败保锁及原 Runner 停服/资源结算证明保持。
- 守卫约束宿主契约纯依赖、旧定义/别名回流、checkpoint 存储文件的执行域 import，并禁止重建层重新依赖存储端口。Windows/Linux 依赖图显式包含新包，既有闭包反例夹具同步新根包；新投影测试进入现有 quick-rules，所有权守卫进入 quick-execution-boundary。

本轮没有修改 worker 节点、重试/恢复、产品命令、资源关闭算法或写事务边界。`execution/model` 的候选/活动/正文展示、配置值及 `execution/hostport` 控制传输仍按真实依赖保留，不声明全部执行域依赖已消失。

## 自动验收映射与结果

环境：Windows/amd64，隔离临时 SQLite、源码/内存夹具和受控进程；未使用安装环境、真实模型、真实 Runner 执行或产品真实 PR 现场。测试时点为实现及对应修正后。

| ID | 通过条件 | 测试 / 入口 | 结果与证据 |
| --- | --- | --- | --- |
| DCR5-A01 | checkpoint/运行视图往返保留原事实和独立 Task 权威；修改任一投影不污染原服务、进程、build pin、能力、操作、回执时间或预留任务；保留采样 schema | `TestHostCheckpointProjectionPreservesFactsAndOwnership`；C1 | 通过；E1 |
| DCR5-A02 | 宿主契约无执行/存储/产品实现依赖；旧定义/别名和未启用源文件不能绕过；重建层无存储端口；前四轮守卫保持 | `TestHostCheckpointOwnership`、`TestTM*`、`TestSharedFixtures*`、`TestTaskDeliveryOwnership`；C1；`go list -deps` | 通过；E1，Windows/Linux 生产图及全标签源文件 |
| DCR5-A03 | 并发写入不混入原应用快照：实例、进程创建身份、Runner build pin 与 Task/候选属于同次采样；下一次读取采用新事实 | 扩展 `TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit`，既有只读绑定/组合查询；C2 | 通过；E2 |
| DCR5-A04 | actor 沿宿主调度与心跳发布；控制回执保持原文件摘要绑定，不把变化后的原请求当成功 | `TestRunnerActorUsesHostScheduler`、`TestRunnerActorBindsReceiptToOriginalHostFile`；C2 | 通过；E2 |
| DCR5-A05 | 资源由唯一 owner 关闭；重复关闭不重复本地关闭；先排空公共入口；局部准备失败释放资源、本地失败保锁、Runner 未核实不伪报结算 | `TestProjectRuntime*`、`TestProjectLifecycleOwnershipBoundary`、`TestRunnerShutdownRetriesOnlyUnconfirmedOriginalStop`、`TestRunnerSettlementReadCanRecoverWithoutRepeatingStop`；C2 | 通过；E2 |
| DCR5-A06 | 原宿主/实例停止证明、离线重验不重启不改历史；进程身份/命令预算/取消及原子文件替换保持 | `TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost`、`TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite`、进程/宿主定向测试；C2 | 通过；E2 |
| DCR5-A07 | 迁移字段/JSON/方法集保持；Windows 生产入口及带标签消费者、Linux 宿主/进程实现可编译 | `TestExecutionRecordsKeepWireIdentity`；C1；定义静态比对与 C3 | 通过；E1 的 `moved-contract-shapes.txt`、E3；Linux 仅编译未执行 |
| DCR5-A08 | 文档链接、Go 格式、最终差异无错误 | 文档链接脚本、`gofmt -l`、`git diff --check` | 通过；478 份 Markdown、2930 个本地链接、0 错误；格式与差异检查通过；E1 |
| DCR5-A09 | 新增 L1 选择登记不替代固定覆盖，零匹配仍拒绝 | `TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage`、`TestQuickTargetedCoverageRejectsZeroMatches`；C4 | 通过；E4 |

C1：4 包、118 项（含子用例）、0 跳过，17.141 秒。C2：5 包、41 项，其中 40 通过、1 个辅助入口按预期跳过，50.744 秒。跳过的是 `TestProcessHostCommandHelper`：它只在父测试设置 helper 环境时工作，相关命令/预算父测试实际通过；不作为业务验收项。C3：Windows 六个消费者包与 Linux 两个宿主/进程测试包编译通过，不计业务用例。没有运行完整 L1/Full 或 L2/L3/L4，不以编译替代真实流程验收。

## 证据

- E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-round5-core-4df1c98951e64c5a9608e736359ea1b0/`；包含 test-feedback 三件套，以及 `hostcontract-dependencies.txt`、`ports-dependencies.txt`、`sqlite-dependencies.txt`、`moved-contract-shapes.txt`、`doc-links.txt`、`gofmt-check.txt`、`diff-check.txt`。
- E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-round5-lifecycle-478e6801cef3428aa5655b16612a7859/`，包含 `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`。
- E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-round5-compile-0e982270a0824351be8e3614a99e727d/`，包含 `windows-consumers.txt`、`linux-compile.txt` 与 Linux 测试二进制。
- E4：`C:/Users/lmas1/AppData/Local/Temp/steerline-round5-selection-6c228d2b020b4577beb5e3c61ad61d44/`，测试选择登记回归通过，2.436 秒。

失败证据保留：首次 C1 的 `steerline-round5-core-16c574c96f6d4c0e945c4aecb18e0fe6` 中，旧“重建层允许存储端口”正例与新边界冲突；正例改为宿主契约，新增反例继续拒绝存储端口。首次 C2 的 `steerline-round5-lifecycle-bc255a8ac934495ab9b014e3c4e9e55c` 中，新增快照夹具误用不存在的 `runnerclient.BuildPin`，导致 SQLite 测试编译失败；修正为公开 `runnercontract.BuildPin` 后通过。以上目录均位于系统临时目录，不计入通过结果。

## 复现入口

```powershell
# C1：投影隔离、保持 wire identity、全部既有及新增守卫
$coreOutput = Join-Path $env:TEMP ('steerline-round5-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/execution/reconstruction ./internal/execution/backend ./internal/storage/ports ./internal/testkit/boundary '-run=^(TestHostCheckpoint.*|TestExecutionRecordsKeepWireIdentity|TestTM.*|TestSharedFixtures.*|TestTaskDeliveryOwnership)$' -count=1

# C2：同快照、原回执、资源关闭与证明
$storageOutput = Join-Path $env:TEMP ('steerline-round5-lifecycle-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $storageOutput -- ./internal/storage/sqlite ./internal/app ./internal/app/host ./internal/execution/process ./internal/product/query '-run=^(TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit|TestRunnerActor.*|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestProjectRuntime.*|TestProjectLifecycleOwnershipBoundary|TestRunnerShutdownRetriesOnlyUnconfirmedOriginalStop|TestRunnerSettlementReadCanRecoverWithoutRepeatingStop|TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost|TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite|TestFileQuerySourceReadsCombinedProjectView|TestTM05PageProjection|Test.*Process.*|TestWindowsReplaceFailurePreservesSnapshot|TestAtomicWriteENOSPCPreservesPreviousSnapshot)$' -count=1

# C3：Windows 消费者只编译；Linux 使用独立 shell 设置 GOOS，不执行二进制
go test -tags=e2e '-run=^$' ./cmd/steerline ./cmd/steerline-web ./tests/e2e/smoke ./tests/e2e/suites/recovery ./tests/e2e/suites/runtime ./tests/e2e/internal/testenv -count=1
$linuxOutput = Join-Path $env:TEMP ('steerline-round5-linux-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $linuxOutput | Out-Null
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go test -c -o $linuxOutput ./internal/execution/process ./internal/app/host

# C4：在 Windows shell 执行测试选择登记回归
go run ./cmd/test-feedback -- ./cmd/test-feedback '-run=^(TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage|TestQuickTargetedCoverageRejectsZeroMatches)$' -count=1
```
