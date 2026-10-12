# Plan：统一 Worker 的 PR 成果流程

依据[Spec](worker-pr-flow.spec.md)及[专属验收](../acceptance/worker-pr-flow.md)。本次按用户授权在当前聊天实施，不占号或修改 execution-map。

## 实施步骤

1. **共享 worker 先改。** 在 internal/execution/staged_flow.go、staged_result.go、model/staged_flow.go/records.go 删除 CandidateSHA、阶段 Snapshot、成果采集/相等检查及专属反馈/恢复分支。报告有效后直接发布；一次发布补报保留来源与失败资格。FlowRef 改为 development/v3，复用 stored_execution_validation.go 的现有拒绝入口，不另设版本登记层。
2. **发布和交付跟随。** publication/pr.go 保留一个 PR 身份校验，删除冻结候选封装及 HeadRevision 必需检查。worker_state.go 的绑定及共享提示词明确同一执行复用原 PR、拒绝另一 URL。native_delivery_command.go、model/task_delivery_gate.go、storage/sqlite/task_result_commit.go 删除候选门槛，按实际合并证据交付；CandidateBindings、storage/sqlite/state_revision.go 等比较只移除候选字段及动态工作区观测，保留现有资源身份、任务版本和并发检查。无需新增比较抽象。
3. **底层与消费者一起删。** 从共享调用追到 Codex/Orca adapter 和 app 接线，删除成果比较专用实现、摘要计算及失效报告策略文案。现有 InspectLifecycleWorkspace 的创建/恢复用途保留。同步 DTO/API/console、internal/testkit、tests/e2e helper、cmd/test-feedback、debug-template 的字段和断言。E2E 从当前 PR/实际合并取证；驱动保护自己本次合并操作所用的当前 SHA 不成为产品冻结门槛。
4. **改现有测试和文档。** 重写 worker_stage_test.go、worker_stage_audit_test.go、worker_delivery_test.go、publication/pr_test.go 的旧成果锁断言；沿用 tests/integration/worker_stage_test.go 的三路受控夹具和 api_protocol_test.go 的投影测试。只扩展现有 provider fixture 的 PR head/merge 事实与反馈能力。调试模板复用 PR 时先推送当前分支，不再用旧 PR head 拒绝新提交；在现有 driver 包补两份模板的隔离 Git 推送回归，不新建测试体系。同步 Spec 指定的当前文档，执行下述验证并登记 acceptance。

测试差异必须在步骤 4 落实：阶段 inspection 不可用/旧 HEAD 仍推进；同一 PR head 更新或无 HeadRevision 可等待/交付；另一 URL 拒绝；原失败不被补报覆盖；已知 PR 查询失败后只查询恢复；新流程重启不重发；实际合并唯一交付并释放后继。三路正常链共用现有 helper，local 先完成反馈更新，再重启验证不重发，最后合并交付；完整场景只在 local 路补一次；不把所有故障与后继场景复制到三个后端。

在 SQLite 现有事务测试中补齐并统一由 `TestTaskDeliveryCommitPRFlow` 执行动态 HEAD/dirty/采样时间不影响资源身份比较、真实身份变化仍被拒绝和重复交付断言；复用已有夹具与断言，不复制交付实现。旧流程拒绝加入已有 `TestWorkerStageUnsupportedBeforeNativeProbe`；历史字段仅保留在明确的拒绝材料中。

## 测试计划

选定向 unit/存储、受控集成和全仓编译。风险集中在共享流程及消费者；不选择 full、完整 L2/L3、真实模型/真实 PR，原生 TUI 和权限机制没有新增行为。测试入口以现有名称为准，实施时更新断言，不只编译或登记未来测试通过。

| 级别/组或场景 | 自动项 | 环境、命令、时点 | 通过条件、证据 |
| --- | --- | --- | --- |
| unit：既有 WorkerStage、WorkerFlowDelivery、DeliveryGate、WorkerPRIdentity | [WPF-A1/A2/A3/A4/A6](../acceptance/worker-pr-flow.md) | 独立 worker/provider/Git/SQLite 夹具；步骤 1–2 后：`go test ./internal/execution ./internal/execution/model ./internal/execution/publication -run '^(TestWorkerStage\|TestWorkerFlowDelivery\|TestWorkerFlowProviderDelivery\|TestDeliveryGate\|TestWorkerPRIdentity\|TestWorkerPRBinding)' -count=1 -v` | 无阶段成果检查；PR 更新/缺 HEAD 合法，错 PR/未合并拒绝；源失败、查询恢复和旧流程拒绝正确；unit.log |
| 存储事务：TestTaskDeliveryCommitPRFlow | [WPF-A3](../acceptance/worker-pr-flow.md) | 现有独立 SQLite 夹具；事务比较修改后：`go test ./internal/storage/sqlite -run '^TestTaskDeliveryCommitPRFlow$' -count=1 -v` | 动态观测不阻断；真实身份变化拒绝；重复交付不新增 Delivery；storage.log |
| 受控共享链：既有三路 RoundTrip 及 local PublishReport | [WPF-A1/A2/A3/A4](../acceptance/worker-pr-flow.md) | 既有隔离进程/Git/provider/数据库；步骤 3–4 后：`go test ./tests/integration -run '^TestWorkerStage(LocalRoundTrip\|OrcaTerminalRoundTrip\|OrcaEnvironmentRoundTrip\|LocalPublishReport)$' -count=1 -timeout=10m -v` | 三路原输入/来源与正常推进相同；local 反馈修改推送到原 PR，更新后合并唯一交付并解锁后继；等待合并重启不重发/重发布；flow.log |
| 既有 API/console 投影 | [WPF-A5](../acceptance/worker-pr-flow.md) | 现有规范快照/隔离服务；消费者修改后：`go test ./internal/console ./tests/integration -run '^(TestWorkerStagePresentation\|TestAPIWorkerStageQueryProjection)$' -count=1 -timeout=5m -v` | 查询/显示无需旧字段，真实阻断与等待合并可用；projection.log |
| 模板及消费者回归 | [WPF-A2/A5](../acceptance/worker-pr-flow.md) | 现有 Go/PowerShell、隔离 Git 与 stub PR 事实；模板修改后：`go test ./tests/e2e/internal/driver ./cmd/test-feedback ./internal/execution/adapters/codex -run '^(TestDebugPublicationReusesPR\|TestSandboxScenarioRequiresBothOriginalMergedTasks\|TestWorkerRuntime)' -count=1 -v` | 两份模板真实推送新提交并复用已有 URL，不比较旧 PR head；反馈证据用实际合并，原生权限设置保留；template-consumers.log |
| 编译与删除审计 | [WPF-A5/A6](../acceptance/worker-pr-flow.md) | 最终修改后：`go test ./... -run '^$'`；`rg -n 'CandidateSHA\|candidate_sha\|StageSnapshot\|sameStageSnapshot\|inspectStage\|ValidateWorkerPublication\|artifact_mismatch\|ContentDigest\|fact_comparison_only\|revision_comparison_only' internal cmd tests debug-template`；`git diff --check` | 所有消费者编译；正常链无旧字段/门槛、无死代码或改名成果锁；拒绝材料和仍有独立用途的命中说明用途；当前文档与新规则一致；compile.log、cleanup-audit.md |

CandidateBindings 和 PR 换绑规则涉及既有多候选代码，补选 `TestMC5`、`TestMC6/abandoned_merged`、`TestMC6/late_after_delivery`、`TestMC7/discard_full_history_and_exited_set`，在相关修改后运行 `go test ./internal/execution -run '^(TestMC5|TestMC6|TestMC7)$/(abandoned_merged|late_after_delivery|discard_full_history_and_exited_set)$' -count=1 -timeout=5m -v`，沿既有隔离夹具核对事务回滚/唯一交付、放弃后晚合并和历史 PR 全集；历史 PR 由审计夹具预置，不调用已删除的换绑流程。证据 unit-boundaries.log，映射 [WPF-A3](../acceptance/worker-pr-flow.md)。

各命令须匹配非零测试，skip 不算通过。每项登记实际命令、场景结果及证据，统一留在本次隔离测试反馈目录；Plan 不维护实时结果。存储场景入口须在实施时补齐后执行，不可用零匹配替代。若需临时原生调试，先运行 scripts/new-debug-release.ps1，AI 模拟方案显式 --simulate。

人工 WPF-M1 由用户在新环境查看步骤与 PR 反馈，窗口待定，可后置，不阻塞自动验收。不修改安装环境或旧 102。

## PR #295 质量审计修复

沿现有流程修正关闭 PR 的位置与结算：已绑定的 closed_unmerged PR 进入 await_merge 下的人工等待，继续查询原 PR；空闲且没有待投递输入/真实问题/未知原生操作时，实际合并可直接唯一交付。对账中的另一 PR URL 按原归属规则阻断，不抛出业务冲突导致候选对账失败。调试模板查询 all 状态，遇到关闭/已合并 PR 不另建；验收证据的合并时间须可解析且非零，不新增 SHA 门槛或状态字段。

测试计划补选（不重跑全量或三路矩阵）：

- [WPF-A2/A3/A4](../acceptance/worker-pr-flow.md)：既有隔离 worker/Git/SQLite/provider，修复后执行 `go test ./internal/execution -run '^(TestWorkerStageClosedPublicationKeepsPRPolling|TestCandidateClosedPRPollDeliversLaterMerge|TestCandidateOpenPRPollPreservesOriginalPendingInput|TestWorkerStageReconcileRejectsReplacementPR|TestWorkerStagePublicationReadRetrySettlesOpenPR|TestWorkerStagePublicationReportReadRetrySettlesOpenPR)$' -count=1 -timeout=4m -v`。要求关闭 PR 保持原绑定和轮询、后续合并不新增输入且唯一交付、原 pending 输入保留、换 PR 阻断、合法只读对账恢复；证据 worker-final.log。
- [WPF-A2/A5](../acceptance/worker-pr-flow.md)：现有隔离 Git/PowerShell + stub PR，执行 `go test ./tests/e2e/internal/driver ./cmd/test-feedback -run '^(TestDebugPublicationReusesPR|TestSandboxScenarioRequiresBothOriginalMergedTasks)$' -count=1 -timeout=2m -v`。要求两份模板 OPEN 正常复用、CLOSED/MERGED 不另建，非法/零合并时间拒绝；证据 template-evidence.log。
- [WPF-A5/A6](../acceptance/worker-pr-flow.md)：最终修改后 `go test ./... -run '^$'`、`git diff --check`，要求消费者编译和格式检查通过；证据 compile.log、审计记录。

证据为本次临时测试反馈目录 worker-pr-flow-audit-20261012；实际结果登记 acceptance。沿既有夹具，无安装环境或真实外部 PR 操作。
