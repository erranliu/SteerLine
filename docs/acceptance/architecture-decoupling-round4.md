# 架构解耦第四轮验收：Task 交付类型归位

日期：2026-10-17。归属：用户直接授权的第四轮实现与提 PR，无 roadmap Task ID。前三轮和本轮修改共同提交于 `85927eb1`，随后集成最新主干 `11be2af5`（合并提交 `5e47df52`），本记录覆盖集成后的最终验证。当前图与归属见[代码架构](../architecture/current-code-architecture.md)，前三轮历史证据分别见[第一轮](architecture-decoupling-round1.md)、[第二轮](architecture-decoupling-round2.md)、[第三轮](architecture-decoupling-round3.md)。

## 实现与边界

- `TaskDeliveryRecord`、`TaskDeliveryEvidence`、`ProjectTaskAuthority`、`TaskDeliveryReceipt`、`ValidateDelivery` 从 `execution/model` 迁入 `task/model`。字段、JSON 标签、校验条件和错误码保持，复用 Task 现有的 `Error` 与相同提交摘要规则，不保留旧包别名、转发或双读。
- Task 服务与控制入口、图快照/任务详情存储事实、项目权威读取、初始化/派发和相关查询/应用消费者使用 Task 纯值。`task/model` 的生产依赖闭包只有标准库；尚未迁移的 checkpoint、宿主/配置/展示值及 `StartupBlockers` 继续保留原所有者，不声明所有包已与执行域完全解耦。
- `CommitRunnerTaskResult` 仅替换类型所有者，SQL 与事务顺序不变：交付、Task 关闭、图释放、结果/事件和全部候选停止义务仍原子提交；故障整体回滚。同候选与原 Task 版本重放返回原回执，不使用当前图版本改写，不重复生成结果或事件；更换版本或候选仍拒绝。
- 全标签源码守卫禁止其他生产包复制/别名转发这些定义，并禁止已收口的 Task 服务/控制入口、存储事实文件、任务图值文件和应用命令端口重新 import 执行域。Task 纯包既有依赖白名单继续禁止执行域及其他项目依赖。新守卫与服务重放测试已登记既有 L1 组。
- 集成主干时保留 Runner 预设/构建/实例冻结、workflow/audit/materials 展示、执行位置和旧运行记录集合清理。主干新增的公开 Runner 纯协议依赖同步进入执行展示模型与重建值的守卫白名单；不放宽 Task 纯包边界。
- 集成复测暴露第三轮活动流的取消时序竞争：驱动可能先交付缓存行，`Next` 现显式检查 context；资源关闭与同快照规则保持。首轮的准入测试同步主干既有 `runner_unavailable` 错误契约，没有改变准入实现。

本轮未新增 schema、数据库兼容、worker 状态机、重试或恢复流程；主干的 schema 23 是合入的既有改动。本次 PR 包含用户连续授权的四轮解耦。

## 自动验收映射与结果

环境：Windows/amd64，内存/源码夹具与隔离临时 SQLite；依赖守卫核对全部标签源文件和 Windows/Linux 生产图。以下均在主干集成及相关修正后执行。

| ID | 通过条件 | 场景 / 命令 | 结果与证据 |
| --- | --- | --- | --- |
| DCR4-A01 | Task 交付纯校验保留 PR 归属、目标分支、合并事实、提交摘要及旧 acceptance 拒绝错误码，且不修改证据 | `TestTaskDeliveryValidatesFixedMergeEvidence`；C1 | 通过；E1 |
| DCR4-A02 | 服务重放返回原保存版本/图版本/证据，无 Runner/写能力也能完成；改变候选或 Task 版本拒绝 | `TestRunnerDeliveryReplayKeepsOriginalTaskReceipt`；C1 | 通过；E1 |
| DCR4-A03 | 原子交付失败不泄漏交付/Task/图/停止义务；成功唯一交付、释放后继并保留停止义务；重放不新增结果/事件且拒绝身份变更 | 扩展 `TestRunnerTaskDeliveryWithoutWorkflowReady`；C2 | 通过；E2 |
| DCR4-A04 | 项目权威与派发回执同事务，失败回滚，原请求重放不再次变更权威，改变意图拒绝 | `TestRunnerDispatchReceiptAndAuthorityCommitTogether`；C2 | 通过；E2 |
| DCR4-A05 | 类型只能由 Task 纯模型定义；旧包别名、helper 转发、未启用源文件和执行域依赖回流被拒绝；存储依赖闭包继续无产品包 | `TestTaskDeliveryOwnership`、`TestTM04ExecutionBoundary`、`TestTM07StorageBoundary`、`TestTM01StoragePortDependencies`；C1；`go list -deps` | 通过；E1 的闭包文本；Task 闭包按 Go `Standard` 元数据核对 |
| DCR4-A06 | 类型替换与主干集成后，候选/历史、同快照读取、会话身份/游标/正文/预算、活动快照/取消释放、HTTP 路由及 workflow/执行位置投影保持 | C1 的查询/图/会话/活动用例；C2 的应用/SQLite/HTTP 定向用例 | 通过；E1/E2 |
| DCR4-A07 | CLI/Web 与受影响带标签的交付/恢复消费者可编译；前三轮的路由、候选缺失分类、原请求启动/重放及测试选择登记保持 | C3 编译；C4 定向消费者测试 | 通过；E3/E4；编译不代表真实 E2E 场景通过 |
| DCR4-A08 | 文档链接、Go 格式与最终差异无错误 | 文档链接脚本、`gofmt -l`、`git diff --check` | 通过；474 份 Markdown、2903 个本地链接、0 错误；格式和差异检查通过；E1 |

C1：6 包、114 项（含子用例）、0 跳过，5.050 秒。C2：4 包、31 项、0 跳过，17.595 秒。C4：6 包、13 项、0 跳过，8.756 秒。C3 为四个包的编译检查，不计业务用例。跨命令可能涉及同一包，以上不是独立场景总数。

未运行完整 L1/Full、L2/L3/L4、真实模型、调试安装或产品真实 PR 流程；本轮为类型归属与读取/投影边界改动，最小隔离回归覆盖全部本轮自动项。用户要求的源码 PR 单独创建，不作为产品真实 PR 验收。

## 证据与修正记录

test-feedback 目录含 `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`：

- E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round4-core-4357f6110da0439da44183a9d72e71eb/`；附 `task-model-dependencies.txt`、`ports-dependencies.txt`、`sqlite-dependencies.txt`、`moved-value-shapes.txt`、`doc-links.txt`、`gofmt-check.txt`、`diff-check.txt`。
- E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round4-storage-ea2d0f9cc5284e16a620ac6af3e8a8a5/`。
- E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round4-compile-36295f08a9a240a8a6ace04e2c2f8c5e/compile.txt`。
- E4：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round4-consumers-076f62ecbe6b48d1a90b43dc30f38fc2/`。

失败证据保留：

- `steerline-decoupling-round4-core-4a40eeec628146e3a7780f52ce943f50`：最初把仍消费 `StartupBlockers` 的 `sqlite/tasks.go` 纳入完全禁止执行域的文件集合；修正守卫范围，保留目标类型所有权限制。
- `steerline-decoupling-round4-core-ed4a34e3c7c440e1a4ede93d777ae984`：主干新增公开 Runner 协议值依赖未同步到守卫；按其真实值依赖更新并加入正例，Task 纯模型白名单保持。
- `steerline-decoupling-round4-storage-d64845622c3d4945aeacd568a74dac6f`：活动流取消后偶发返回缓存行；补显式 context 检查后 C2 通过。
- `steerline-decoupling-round4-consumers-9479cecf0e1f4f7a88fb080999292539`：原候选缺失测试仍期待准入原始错误，主干已返回 `runner_unavailable`；断言改为核对稳定错误码与原消息后 C4 通过。

以上目录均位于 `C:/Users/lmas1/AppData/Local/Temp/`。未通过的运行不计入验收通过结果。

## 复现入口

```powershell
# C1：Task 纯值、服务重放、查询及所有权守卫
$coreOutput = Join-Path $env:TEMP ('steerline-round4-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/task/model ./internal/taskrunner ./internal/product/query ./internal/product/query/taskgraph ./internal/storage/ports ./internal/testkit/boundary '-run=^(TestTaskDelivery.*|TestRunnerDeliveryReplay.*|TestTM01StoragePortDependencies|TestTM04ExecutionBoundary|TestTM07StorageBoundary|TestTM05.*|TestSampledHistory.*|TestTask.*|TestReady.*|Test.*Graph.*|TestConversation.*|TestActivityProjection.*|TestStateNamingActivity.*)$' -count=1

# C2：原子交付/权威回执、同快照与集成读取
$storageOutput = Join-Path $env:TEMP ('steerline-round4-storage-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $storageOutput -- ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./tests/e2e/internal/testenv '-run=^(TestRunnerTaskDeliveryWithoutWorkflowReady|TestRunnerDispatchReceiptAndAuthorityCommitTogether|TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestCandidateReadFactsAcrossViews|TestFileQuerySourceReadsCombinedProjectView|TestClientHTTPRoutes|Test.*Reconciliation.*|TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions|TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestQueryEventCursorAndLargeBodyPaging|Test.*Runner.*Workflow.*|Test.*ExecutionLocation.*)$' -count=1

# C3：仅编译受影响消费者
go test -tags=e2e '-run=^$' ./cmd/steerline ./cmd/steerline-web ./tests/e2e/smoke ./tests/e2e/suites/recovery -count=1

# C4：既有路由/候选/原请求语义及测试选择
$httpOutput = Join-Path $env:TEMP ('steerline-round4-consumers-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $httpOutput -- ./internal/product/applicationhttp ./web/server ./internal/app ./internal/storage/sqlite ./internal/taskrunner ./cmd/test-feedback '-run=^(TestEnvironmentCheckReadRoutes|TestUnifiedStartupAuthenticationRoutesAndUnavailable|TestWebAuthenticationAndOriginalForwarding|TestHTTPConsumerBoundary|TestRunnerCandidateReadErrors|TestRunnerStartClassifiesCandidateLookupErrors|TestRunnerTaskServiceFreezesOriginalStart|TestRunnerControlRecoversCommittedStartAfterReceiptLoss|TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage|TestQuickTargetedCoverageRejectsZeroMatches)$' -count=1
```
