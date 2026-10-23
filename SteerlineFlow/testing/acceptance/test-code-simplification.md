# 测试代码精简验收

普通开发请求：清理确认重复的测试代码并报告净减少行数。日期：2026-10-09；基准 Git commit：`e20c28eff3844d8ba74df7e0ad9c1672cd318257`，结果针对本次工作区改动。

## 范围与计数

共享 execution 与 app/host 的中立生命周期、固定 Git 文档读取、任务存储投影和 PR/进程 helper；包内保留配置、私有接口及生产 Runner 的接线。删除 46 处对独立顶层测试的重复引用，以及 9 处相同参数的重复场景调用。保留不同配置/恢复参数和所有独有业务断言；原有测试失败消息的静态集合未减少。

按全部 Go 源文件的物理行计数，包含空行、注释、各平台及 build tag：

| 口径 | 修改前 | 修改后 | 净减少 |
| --- | ---: | ---: | ---: |
| `*_test.go` | 43,620 | 42,453 | 1,167 |
| 测试文件及测试基础设施（testkit、E2E、runner） | 53,920 | 53,399 | 521 |
| 全部 Go 源码 | 112,634 | 112,113 | 521 |

新增公共 fixture 为 640 行，边界检查实现净增加 6 行；均已计入净数，没有把搬到普通 `.go` 文件的代码算作删除。新增边界回归包含在测试文件计数中。Markdown 文档未计入以上 Go 行数。

## 自动验收

环境：本机 Windows、Go、Git、隔离临时 home/SQLite/受控协议进程；不使用真实模型。执行时点：本次代码清理后。通过条件：runner 报告 passed/complete、命令退出 0、选择的场景均实际通过；静态检查退出 0。原始报告位于系统临时目录 `steerline-test-feedback`。

| ID | 检查与覆盖 | 状态 | 证据（目录下 summary.json） |
| --- | --- | --- | --- |
| TC-A1 | L1；生产依赖拒绝、两包测试限定引用、helper/适配器间接依赖拒绝、既有固定范围 | 已验收 | `20261009T044044.179685300Z`；12 组均通过，无 skip |
| TC-A2 | 两包任务创建/事务回滚、前置边原子性、固定文档/链接拒绝及同会话反馈 | 已验收 | `20261009T044139.511892100Z`；两包通过，无 skip |
| TC-A3 | TM04 双回合/原请求恢复、CD66 改配置原身份恢复、CD67 中立实现/远端协议拒绝、MC6 候选合并门槛 | 已验收（逐场景核验） | `20261009T044120.178169300Z`：组合命令整包超时、不完整；CD66/CD67/MC6 有完整逐项 pass，TM04 双回合独立报告 `20261009T045219.841778300Z` 已通过；原请求恢复独立报告 `20261009T045220.173494200Z` 已通过，两份拆分报告 passed/complete 均为 true，无 skip |
| TC-A4 | runner 选择/完整覆盖核对及 Orca 独立 codec/question 场景 | 已验收 | `20261009T044512.295671300Z`；两包通过，无 skip |
| TC-A5 | 最终恢复 seed 入口与重启不重发旧反馈 | 已验收 | `20261009T044734.416640200Z`；通过 |
| TC-A6 | 文档导航、差异格式及最终行数复核 | 已验收 | 275 个 Markdown 文件、1,741 个本地链接，0 错误；diff 格式检查通过；物理行复核见上表 |

首次组合集成回归受到 Go 默认整包 10 分钟期限影响，在 TM04 双回合期间被中断，`passed=false`、`complete=false`，不能作为整包通过证据。保留该失败报告；CD66、CD67、MC6 的已完成逐项结果分别为 pass，剩余 TM04 子场景改用独立进程验证并分别通过。原组合命令仍记录为失败，不改称整包通过。Go 命令期限覆盖既有场景期限和进程准备；未延长场景内部业务期限。

执行命令：

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -- ./internal/execution ./internal/app/host -run '^(TestCreateTaskBatchAtomicIdempotentAndQueryable|TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails|TestTaskBatchRejectsDuplicateAndCrossProjectDependencies|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace|TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim|TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences|TestTaskContractRejectsCommittedSymlinkReferences)$' -count=1
# 首次组合命令（整包超时；保留失败证据）
go run ./cmd/test-feedback -- ./tests/integration -run '^(TestTM04ExecutionWiring|TestCD66FixedProfileRecovery|TestCD67AdditionalImplementation|TestMC6)$' -count=1
# 剩余场景分别运行
go run ./cmd/test-feedback -- ./tests/integration -run '^TestTM04ExecutionWiring$/^intent_binding_and_two_original_turns$' -count=1 -timeout=5m
go run ./cmd/test-feedback -- ./tests/integration -run '^TestTM04ExecutionWiring$/^original_request_ack_and_manual_recovery$' -count=1 -timeout=12m
go run ./cmd/test-feedback -- ./cmd/test-feedback ./internal/execution/adapters/orca -run '^(TestQuick.*|TestScoped.*|TestFull.*|TestCD6[23].*)$' -count=1
go run ./cmd/test-feedback -- ./internal/execution -run '^(TestCD63FrozenConfigAndBinding|TestRestartDoesNotSendPreviouslyAcceptedFeedback)$' -count=1
python scripts/check_doc_links.py
git diff --check
```

未选择 Full：本次修改集中于共享测试适配器和重复入口，固定 L1 加相关定向回归覆盖受影响行为。未选择 L2/L3/live：未修改真实模型、产品交付链路或 E2E 环境能力，无需额外付费原生验收。无新增人工验收项。

## 2026-10-09 一期范围说明同步

用户明确一期包含远端路径交付；本轮删除现行 README、产品/架构说明、一期共同约束、测试指南及既有 `SteerlineFlow/Roadmap/drafts/roadmap.md` 中将远端延后的描述，链接远端独立 Spec 与已登记验收。未改变后端实际能力、场景选择、历史证据或任务/DAG 状态。此范围说明不执行新增产品或模型验收。

静态检查通过：275 个 Markdown 文件、1,751 个本地导航链接，0 错误；身份检查为 86 个身份文件、10 条草稿及 4 条正式行；差异格式检查通过。结果属于文档一致性检查。

## 2026-10-09 提 PR 前主干同步与复验

分支无冲突同步到主干 `90e4ca96`（含 PR #256 远端 Dispatch 绑定修复），复验源码 commit 为 `3c2e77c1`。此前报告保持原基准；本段是新的复验范围。

- `go run ./cmd/test-feedback -level=L1 -- ./internal/execution -run '^(TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace|TestRemoteWorker.*)$' -count=1`：L1 与同会话反馈定向追加通过，passed/complete 均为 true，无 skip；报告 `20261009T054738.886917600Z/summary.json`。命中的追加场景为 `TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace`。
- 远端相关定向追加：`go run ./cmd/test-feedback -- ./internal/execution ./internal/execution/adapters/orcaremote ./tests/e2e/internal/native -run '^(TestWorkerWorkspaceInstructionUsesPreparedRemoteIdentity|TestWorkerFlowPromptIncludesFixedWorkflowRunID|TestRemoteEmptyInboxDetectsLostDispatchWithoutRelaunch|TestRemoteInventoryDistinguishesWorkerShellAndResumedDuplicate)$' -count=1`：通过，passed/complete 均为 true，无 skip；报告 `20261009T054815.919101400Z/summary.json`。
- 文档检查：278 个 Markdown 文件、1,757 个本地链接，0 错误；身份检查仍为 86 个身份文件、10 条草稿/4 条正式行；差异格式通过。
- 对最新主干再次核对 Go diff：新增 780 行、删除 1,301 行，净减少 521 行；测试文件新增 131 行、删除 1,298 行，净减少 1,167 行。Markdown 修订另计。

本轮未新增真实模型运行，远端产品验收事实继续引用远端专属记录。
