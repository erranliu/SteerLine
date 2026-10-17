# 架构解耦第二轮验收：历史与候选读取

日期：2026-10-17。归属：用户直接授权的第二轮实现，无 roadmap Task ID。源码基线：`2fb07ba1e74dfe6b16326d7ed462da9c57a5dec0` 加当前工作区[第一轮](architecture-decoupling-round1.md)及本轮变更。现状与图见[代码架构](../architecture/current-code-architecture.md)。

## 实现与范围

- SQLite 在原读事务内返回 `ProjectView.Candidates`、`NativeTaskDetail.Candidates` 的 `runnerstate.Candidate` 事实；历史与任务详情不再使用产品 `HistorySummary` / `WorkflowSummary`。
- `query.ProjectPage`、`TaskDetail` 在事务外生成 DTO，沿用既有 `RunnerHistory` / `CandidateSummary`。公开身份、编号、参与关系、提交状态、停止意图、观察状态、阶段、profile、Attempt 与问题版本保持；私有 state-dir、原请求和执行 settings 不进入公开页。投影不重查数据库或 Runner，也不修改采样事实。
- 成功的空集合仍显示空历史；候选读取失败不把部分记录投影成成功历史，可用的任务页继续保留。
- 协议/游标在查询层补充；事件读取预算由 `storage/ports.MaxEventPageBytes` 统一定义。`TaskGraphSnapshot.ReadySnapshot` 只调用既有 Task 纯规则，字段与资格语义保持。
- SQLite 的六项 product/query 例外全部移除，新增生产传递依赖禁止规则；任务/历史读取源文件另禁止产品 DTO import，包含未启用标签与其他 OS 文件。会话/活动的产品 DTO 依赖保留为后续范围，没有声明整个 storage 包已独立于 product/api。
- 原 TaskDelivery 写事务、schema、候选/outbox 写入与资源所有权未改；同快照读取入口不拆分。E2E 的原始存储采样夹具同步从候选事实判断“后继未启动”，原断言目的保持。

## 自动验收映射与结果

全部运行在 Windows/amd64 的隔离临时数据库、内存/源码夹具；测试时点为本轮实现后。依赖守卫同时读取 Windows/Linux 生产依赖图。

| ID | 通过条件 | 场景 / 入口 | 结果与证据 |
| --- | --- | --- | --- |
| DCR2-A01 | 历史、任务详情从同一候选事实生成一致字段；公开页过滤私有材料；投影不修改来源；空集合/失败语义和任务页游标/bidi 表现保持 | `TestSampledHistoryAndCandidateProjection`、`TestSampledHistoryDistinguishesEmptyAndFailedReads`、`TestCandidateReadFactsAcrossViews`、`TestFileQuerySourceReadsCombinedProjectView`、既有纯页投影/任务游标测试；C1 | 通过；E1 |
| DCR2-A02 | 并发写者提交后，原事务的任务/候选/checkpoint/版本保持同一采样；事务结束后的历史/详情投影不偷读新值，下一次读取采用新事实 | 扩展的 `TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit`、既有 `TestProjectView*`、`TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding`；C2 | 通过；E2 |
| DCR2-A03 | 交付失败回滚全部状态；唯一交付、同请求重放、Task 关闭、停止义务和后继释放继续在一个事务成立 | `TestRunnerTaskDeliveryWithoutWorkflowReady`；C2 | 通过；E2；仅回归，未修改交付事务 |
| DCR2-A04 | 原 final/事件绑定、分页版本/身份、直接前置读取范围和事件字节预算保持 | `TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding`、`TestNativeAuditPaginationBoundsAndDatabaseIdentity`、`TestNativeAuditPageDecodesOnlyDirectPredecessors`、`TestQueryEventCursorAndLargeBodyPaging`；C2 | 通过；E2 |
| DCR2-A05 | SQLite 生产依赖不再包含 product/query；旧纯函数例外和 helper 无法绕过；选定读取源文件不能重新 import 产品 DTO | `TestTM07StorageBoundary`、`TestTM04ExecutionBoundary`、`TestTM01StoragePortDependencies`，`go list -deps ./internal/storage/sqlite`；C1 | 通过；E1 的 `sqlite-dependencies.txt`、`sqlite-imports.txt` |
| DCR2-A06 | 隔离 E2E 采样夹具继续核对数据库版本一致性；带标签的既有交付/恢复消费者可编译 | `TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions`；C3；交付/恢复包 `-tags=e2e -run=^$`，C4 | 首次夹具编译失败，修复读取字段后通过；E3 / E4，失败另存 EF |
| DCR2-A07 | 文档本地链接和最终差异无错误，修改的 Go 文件已格式化 | `python scripts/check_doc_links.py`、`git diff --check`、`gofmt -l` | 通过；464 份 Markdown、2830 个本地链接、0 错误；差异无空白错误，格式检查无输出；E1 |

运行摘要：C1 为 4 包、58 项（含子用例）、0 跳过，20.598 秒；C2 为 1 包、11 项、0 跳过，8.028 秒；C3 为 1 包、1 项、0 跳过，3.931 秒。新增查询测试由既有 L1 quick-rules 覆盖，读取矩阵和守卫沿原 L1 组执行，不新增组/场景 ID。

C4 只证明两个带 `e2e` 标签的包编译通过，零业务场景执行。此次没有执行完整 L1/Full 或 L2/L3/L4，没有启动真实模型、调试包、安装环境或真实 PR。用户要求的读取/交付语义由本轮最小隔离回归覆盖，不以编译代替真实 E2E 验收。

## 证据

各 test-feedback 目录包含 `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`：

- E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round2-core-394d22c692fd4a3da7fbd1c57c9b61b5/`。
- E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round2-storage-c19eafb5773e43ebb4876ddf7afdf3b8/`。
- E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round2-consumer-73561bfab0594fbb8f36b5423a03ae34/`。
- E4：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round2-e2e-compile-75a7697d0d29439ba41c12d2fef58b5a/compile.txt`。
- EF：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round2-consumer-70a68a39a35b4a7bae79f58e8e77ac99/`；`live.go` / `recovery.go` 仍引用已移除的原始读取字段 `WorkflowRuns`，导致编译失败；同步为 `Candidates` 后 C3/C4 通过。失败证据保留。

编译验证曾误将 test-feedback 的 `-build-only` 与定向 Go 参数组合，入口在运行前拒绝；随后使用 C4，未改成全仓编译。

## 复现入口

每次使用独立新目录；C4 不执行 E2E 流程。

```powershell
# C1：产品纯投影、端口、依赖守卫与应用读取
$coreOutput = Join-Path $env:TEMP ('steerline-round2-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/product/query ./internal/storage/ports ./internal/testkit/boundary ./internal/app '-run=^(TestSampledHistory.*|TestTM05PageProjection|TestTask.*|TestReady.*|TestTM01StoragePortDependencies|TestTM07StorageBoundary|TestTM04ExecutionBoundary|TestCandidateReadFactsAcrossViews|TestFileQuerySourceReadsCombinedProjectView)$' -count=1

# C2：原事务采样、交付与读取预算
$storageOutput = Join-Path $env:TEMP ('steerline-round2-storage-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $storageOutput -- ./internal/storage/sqlite '-run=^(TestProjectView.*|TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit|TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestRunnerTaskDeliveryWithoutWorkflowReady|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestNativeAuditPaginationBoundsAndDatabaseIdentity|TestNativeAuditPageDecodesOnlyDirectPredecessors|TestQueryEventCursorAndLargeBodyPaging)$' -count=1

# C3：既有 E2E 夹具的一项内存快照测试
$consumerOutput = Join-Path $env:TEMP ('steerline-round2-consumer-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $consumerOutput -- ./tests/e2e/internal/testenv '-run=^TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions$' -count=1

# C4：仅编译受影响的带标签消费者
go test -tags=e2e '-run=^$' ./tests/e2e/smoke ./tests/e2e/suites/recovery -count=1
```
