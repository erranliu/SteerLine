# #57 Codex 执行适配器迁移验收

基线：`9a3fe7beb88f11d9a21d8827018c5d86f641c16c`（origin/main），包含 #55 的 `fc5142922fb3e959c4637f5f394be9dc41101ecf` 和 #56。交付依据：[Spec](../../archive/spec/completed/execution-isolation-04-codex.spec.md) / [Plan](../../archive/spec/completed/execution-isolation-04-codex.plan.md)。未实施 #58–#61。

## 最终所有者与交接

| 责任 | 唯一实现 / 中心消费者 |
| --- | --- |
| CLI、原生 executable、profile、model/effort、Windows 权限、Git 附加目录 argv、受管 args 与 help 解释 | codexworker 的 config/command/permissions/native_params/executable/capability；中心组装冻结输入；Orca command 只做 PowerShell 包装 |
| 进程与 thread 回执、启动/连接、turn/start、interrupt、原请求回复 | codexworker.Backend、SessionHandle、connection；OS 受管进程/Job Object 仍通过中心注入的 ProcessPort 承载 |
| 原进程/endpoint/generation/thread/turn 检查、历史日志、答案内容、待处理问题重放、观察取消 | codexworker recovery/ask/journal/outcome/observe；Observation 不认识 Service/Run/Worker，可独立取消 |
| 原生历史读取、绑定检查、session launcher、JSONL 会话映射和活动分类 | codexworker history/history_binding/session_launcher/conversation/activity；原格式保持不变 |
| 资格、用户继续/新会话授权、准备/输入意图、绑定/回执附件、Decision 与 Operation、内容及游标提交 | 中心 worker_start/worker_mode/codex_worker/worker_recovery/worker_events/conversation_service/store 等原有事务 |
| 共享记录和展示内容规则 | contracts 保留 Command/CodexAskCall/固定 Profile；会话/活动 DTO 移入 contracts 单一定义，中心 facade 引用；无 contracts → codexworker 依赖 |
| Git 资源、准备/检查/清理与发布/PR 外部对账 | 复用 #55 workspace、#56 publication，包括 HostPublisher.Reconcile；未引入第二套实现 |

中心依序执行 PrepareSession → 保存部分 process/thread 绑定 → StartTurn → 保存 turn 回执 → 登记观察。适配器没有业务事务回调或第二个生命周期引擎；失败保留原请求和可得的原生身份，取消未登记观察。观察通知先写原生日志，再等待中心内容/业务提交确认。

恢复保留原生 thread/resume 对新连接的订阅和 thread/read 身份检查；该请求不含模型输入，不创建业务轮次。停服重开只接回观察及读取/对账，不 turn/start 或补发答案；继续输入/新会话仍经过中心人工门槛。远程 Orca 的 worker-start、agent/model/effort 和原 terminal 续聊协议未改。

## 实际验证

- `go test ./internal/steerline/codexworker ./internal/steerline/codexapp ./internal/steerline/orcaworker -count=1`：通过。
- `go list -deps ./internal/steerline/codexworker`：传递项目依赖为 codexapp、execution、contracts、task/taskgraph/taskview；无父包、SQL、query、workerflow、Orca 包。
- Plan 指定七项中心回归全部命中并通过：两轮同步/异步、匹配答案证据、迟到结果不改终态、重启不补发保存反馈、意图失败阻止派发、回执失败取消未登记观察、Windows 快照冻结/恢复。
- 补充定向中心用例全部通过：CodexAskCallPreservesNativeBindingAndAllQuestions、AppServerPendingFeedbackRecoveryUsesNewTurnReceipt、AppServerRecoveryReplaysOnlyUnresolvedQuestion、ConversationMapperKeepsTurnAndPhaseEvidenceExplicit、ConversationMapperNativeCodexTwoTurnFixture、ConversationMissingTextPreservesSnapshotAndEmptyFinal、ConversationLiveSourceFailureKeepsGapAndMapperPosition、CodexNormalizer、CodexHistoryReconcilesLateFinalInEarlierTurn、CodexHistoryReconcilesFinalTextArrivingAfterTerminalStatus、CodexHistoryDisconnectKeepsVisibleGapAndCheckpoint、ManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput（用例名均加 Test 前缀）。与上述七项合计 19 项，最终该批用时 49.197s。
- LocalSandboxArgs 原测试由 codexapp/windows_sandbox_test.go 移到 codexworker/windows_sandbox_test.go；受管命令原测试由 orcaworker/readiness_test.go 移到 codexworker/command_test.go。AppServerLaunch/TurnStartParams 和 NativeExecutable fixture 同步迁到 codexworker。补充精确 argv 顺序、无 model 的独立 effort、静态设置拒绝不产生现场、进程收据写失败保留部分身份。
- `go test ./... -run '^$' -count=1`：全仓编译/链接通过，无测试执行；未跑全量测试或新增真机认证。`git diff --check` 通过。

## 已确认的基线剩余项

额外尝试 `TestWorkerEnvironmentER1ProbeAndSettlementGate` 时，其五个 fixture 在初次 poll 报 `native Orca poll has no fixed execution`。在未修改的上述 origin/main 源码归档上独立运行 `go test ./internal/steerline -run '^TestWorkerEnvironmentER1ProbeAndSettlementGate$/^success$' -count=1 -v`，同样失败（9.475s）。因此不把该 fixture 计为本轮通过证据，也不借 #57 修改 Orca 准备/就绪状态机。该基线问题应在后续 Orca 迁移/复审中单独核查；本轮的受管 argv fixture 和冻结资源篡改拒绝回归已通过。

## 协调 session 独立复审

发现异步 `turn_result` 失败处理只保存部分进程身份，丢弃适配器已返回的原生 SessionID。线程回执写入失败等场景会因此失去原会话身份。现已保存完整可得绑定，并拒绝覆盖已有不一致的进程、endpoint、generation 或 session；失败仍保持原操作 outcome_unknown，不登记观察或推进业务轮次。

`TestAppServerFailedLaunchKeepsPartialSessionBinding` 修复前失败、修复后通过。协调 session 运行三个受影响包和 Plan 七项中心回归通过；修复后再次运行部分绑定、观察回执失败、两轮反馈和两个恢复定向用例通过。另在 #56 完整交付分支与本 PR 独立运行 ER1 success 子例，两者均报同一固定执行缺失错误，确认其为已有基线问题并向 #58 交接。
