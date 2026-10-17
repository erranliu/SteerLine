# PR #350 四轮解耦复审

日期：2026-10-17。用户直接授权复审、修复、最小验证后合并 [PR #350](https://github.com/erranliu/SteerLine/pull/350)。复审原 head 为 `a8158317`；已集成新主干 `082e48bc`（候选安全停止），合并提交 `1c6e3558`。本记录对应本次复审，不改写[第四轮](architecture-decoupling-round4.md)及前三轮的历史结果。

## 发现与修复

1. **公开 CLI 驱动仍使用存储 DTO（P2）**：`Client.Tasks` 把 `task list` 的公开 JSON 解码成 `storage/ports.NativeTaskPage`。第二轮将存储中的 `workflow_runs` 改为原始 `Candidates` 后，公开候选会被静默丢弃；公开交付的 `workflow_run_id` / `execution_id` 也不是存储交付记录的字段。改为 `product/api.TaskPage`。新增隔离子进程测试，实际经 `Client.Tasks` 的 CLI 传输核对候选编号、候选/执行身份与交付身份；不创建项目、服务或 Runner。
2. **闭包守卫反例夹具漏同步新根包（P2）**：第一轮将 `web/server` 与 `cmd/steerline-web` 加入 Windows/Linux 依赖采样，但 `TestTM06ContractValueClosure` 的模拟仓库没有这些包，四个反例在 `go list` 失败，尚未执行依赖断言。补上两个根包及正确的 `main` 包声明，保留原反例和断言，完整守卫集合通过。

另解决主干测试 import 冲突，保留新的候选退出身份、最后参与者保护、原请求重放、人工退出不触发原生 stop 及 Task 终态停止义务；复测停止与交付竞态。架构文档同步主干安全停止行为，worker 文档仅更新 Task 交付规则的所有者，节点/异常/重试矩阵未改变。

## 其余语义与依赖核对

- 第一轮的共享读取路由正则及前后端使用范围保持，候选不存在错误映射保留可识别的原错误与存储端口分类。
- 第二轮继续在原读事务内采样候选、任务与 checkpoint，查询投影不额外查库；排序、成功空集合/失败区别、公共字段过滤与任务页协议/游标保持。
- 第三轮继续按原 final/Run/Attempt 身份生成正文引用和 v3 游标；正文摘要/大小/UTF-8 分页及会话字节预算保持；活动仍以同事务记录流按公开 JSON 长度计量，预算溢出不前进游标，失败/取消关闭记录流。
- 第四轮四个迁移结构的字段/类型/JSON 标签与主干一致，交付校验复用同样的提交摘要规则与错误码；SQL 写事务和顺序没有拆分，回滚、唯一交付、图释放和原回执重放保持。
- 全标签源码和 Windows/Linux 依赖图守卫通过。Task 纯模型生产闭包仅标准库，`storage/ports` 与 `storage/sqlite` 的生产闭包无产品包。未发现其他阻止合并的问题；仍使用的宿主/展示执行纯值按现有架构明确保留。

## 自动验证

环境：Windows/amd64，源码夹具、内存夹具与临时隔离 SQLite；无安装环境、真实模型或产品真实 PR 操作。以下均在主干集成和对应修复后完成，不是全量 L1/Full 或真实 L2/L3/L4。

| 项 | 场景与通过条件 | 结果 / 证据 |
| --- | --- | --- |
| P350-A01 | 全部 `TestTM*`、共享测试夹具边界及 Task 类型归属；覆盖全标签和 Windows/Linux、闭包反例实际执行 | C1 通过；6 包、150 项（含子用例）、0 跳过，12.395 秒；E1 |
| P350-A02 | CLI 任务页保留公开候选与交付身份；同快照/会话/活动预算和取消、原子交付/重放、权威回执、候选停止及交付竞态 | C1/C2 通过；C2 为 6 包、34 项、0 跳过，27.844 秒；E1/E2 |
| P350-A03 | 命令行、Web、交付/恢复/runtime 与 testenv 带标签消费者可编译 | C3 通过；六包编译，无业务场景执行；E3 |
| P350-A04 | Task 与存储生产闭包、迁移类型结构一致，文档链接/格式/差异无错误 | 通过；四个结构与主干一致，Task 闭包仅标准库、存储闭包无产品包；477 份 Markdown、2923 个本地链接、0 错误，格式及差异检查通过；E1 |

E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr350-review-core-f8b5f86346af4df98a77540d1e43a025/`。E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr350-review-storage-cf9b9966cfab4151a8b9ae3504fa5143/`。两者含 `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`；E1 另存依赖闭包、`moved-value-shapes.txt`、`doc-links.txt`、`gofmt-check.txt`、`diff-check.txt`。

E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr350-review-compile-4fce1dc751bb4b979b3e9fd2646d7a54/compile.txt`。

失败证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr350-review-core-f21c0a997f5c4a6e8d2663e19b793cf5/`，四个闭包反例在 `go list` 提前失败；夹具补齐后 C1 全部通过，未弱化依赖断言。

## 复现

```powershell
$coreOutput = Join-Path $env:TEMP ('steerline-pr350-review-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/product/query ./internal/product/query/taskgraph ./internal/task/model ./internal/taskrunner ./internal/storage/ports ./internal/testkit/boundary '-run=^(TestTaskDelivery.*|TestRunnerDeliveryReplay.*|TestTM.*|TestSharedFixtures.*|TestSampledHistory.*|TestTask.*|TestReady.*|Test.*Graph.*|TestConversation.*|TestActivityProjection.*|TestStateNamingActivity.*|TestRunnerStartClassifiesCandidateLookupErrors|TestRunnerCandidateStopProjection)$' -count=1

$storageOutput = Join-Path $env:TEMP ('steerline-pr350-review-storage-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $storageOutput -- ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./internal/runnerbridge ./tests/e2e/internal/driver ./tests/e2e/internal/testenv '-run=^(TestRunnerTaskDeliveryWithoutWorkflowReady|TestRunnerDispatchReceiptAndAuthorityCommitTogether|TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestCandidateReadFactsAcrossViews|TestFileQuerySourceReadsCombinedProjectView|TestClientHTTPRoutes|TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions|TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestQueryEventCursorAndLargeBodyPaging|TestRunnerCandidateReadErrors|TestRunnerCandidateStop.*|TestRunnerControlStop.*|TestCandidateWithdrawal|TestTaskListDriverRetainsPublicCandidateAndDeliveryIdentity)$' -count=1

go test -tags=e2e '-run=^$' ./cmd/steerline ./cmd/steerline-web ./tests/e2e/smoke ./tests/e2e/suites/recovery ./tests/e2e/suites/runtime ./tests/e2e/internal/testenv -count=1
```
