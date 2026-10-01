# 执行隔离清理与验收

日期：2026-10-01。基线：`7350e84`，PR #174 已合入。此轮承接[总纲](../../roadmap/execution-isolation.md)，不重做 #54–#61 的配置、快照和业务事务。

## 所有者与删除清单

| 调整 | 最终所有者及理由 |
| --- | --- |
| 删除 `execution_recovery.go` | 仅包含无生产/测试调用的 `manuallyStopped` |
| 删除 `cloneRun`、`inspectRunWorkspace`、`showBoundOrcaTerminal` | 没有消费者，不保留包装入口 |
| 删除 `observeDurableWorkerSegment` | 旧 journal 观察循环无人调用；当前 app-server 观察和事件持久化入口保留 |
| 移出 `reattachAppServerWorker`、`appServerPendingAskEvents`、`requireBackendCapabilities`、`inspectBoundOrcaTerminal` | 仅测试/验收消费，集中到 `execution_acceptance_helpers_test.go`；继续调用现有适配器 |
| 移出 local_process `ReadOutput`、cursor 和 prefix 工具 | 仅 unit/live 验收消费，保留为测试工具；当前 app-server 生产输出/恢复链不变 |
| Orca 平台操作选择 | `orca_execution_operations.go` 绑定 native/remote 唯一适配器；共享消费者调用已注册的 delivery、settlement、核验、reply/find、ack/check 操作 |
| 本机结果转发/清理 | `orca_feedback_hooks.go` 的注册回调；恢复观察继续要求人工核对结果，不能自动重发 |
| 保留 `orca_worker_prepare/dispatch/observation`、`worker_environment`、`worker_commit` | 仍有生产消费者，承担 intent、receipt、Decision 和 TaskDelivery 协调与事务，不能按前缀批量删除 |

注册集缺失操作时在调用前明确拒绝。本机 CallError 与远端 ack 未知结果仍记作 outcome_unknown；原始 Cause 保留。回复重查、确认重查沿用原问题/Delivery 身份，不在注册层创建新会话或写中心状态。

中心生产文件仍为 114 个：删除一个死文件，新增一个注册操作组装文件。整个 cmd/internal 为 263 个生产源码、20 个包；文件数不是残留量。架构文档的完整索引、包数量和 42 条真实 import 边已同步。配置校验、固定身份映射和平台专属准备仍保留平台知识；本轮不声称所有中心平台知识已消除，也不新增通用插件系统。

## 实际验证

- 最小入口：`go run ./cmd/test-feedback -quick -out <临时目录>`，passed=true，61 项测试结果（含子用例），9 个包，execution=21.4s，无跳过。原始日志留在临时目录，不把批量日志加入仓库。
- 中心定向回归通过，89.6s：注册、固定终端及重启核验、PR provider 绑定、remote Claim 权限拒绝、app-server 两轮与恢复 replay、输出 cursor、替换 incarnation/connection、relay 请求身份、固定终端 stop 及失败确认、ResolutionAudit 系列。
- 注册操作回归：`go test ./internal/steerline -count=1 -run '^TestRegisteredOrca'`，通过。直接驱动既有 native/remote CLI fixture，先验证待确认 Delivery，再注入“已执行 ack、回执丢失”，要求 outcome_unknown，最后只读确认原操作已执行；不是远程权限认证。
- 受影响协议包：`go test ./internal/steerline/orcaworker ./internal/steerline/codexworker ./internal/steerline/codexapp -count=1`，三个包通过。remote 协议包已由 quick 覆盖。
- `go build ./cmd/steerline`、`go vet ./internal/steerline`、`git diff --check` 通过；最终构建输出写临时目录，仓库不保留生成的 exe。
- 370 条变更文档本地链接均存在；架构索引与 263 个实际生产源码集合一致；execution 生产源码无项目内 import。

中心定向命令：

```powershell
go test ./internal/steerline -count=1 -run '^(TestRegisteredOrca.*|TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart|TestNativeOrcaPRBindingUsesProviderAndRetainsSession|TestRemotePublicationClaimRequiresPermissionEvidence|TestAppServerServiceTwoTurnsPersistNativeReceipts|TestResolutionAudit.*|TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt|TestAppServerRecoveryReplaysOnlyUnresolvedQuestion|TestLocalReadOutputUsesBoundedExecutionCursor|TestOrcaObservationRejectsReplacedIncarnationAndConnection|TestNativeOrcaDoneRequestBindsCurrentTurn|TestOrcaStopExecutionConfirmsFixedTerminalExit|TestOrcaStopFailureDoesNotConfirmExit)$'
```

未运行全量测试、真实 agent 或双机认证。

## 尚未通过的验收

以下五项在当前分支（22.0s）及 `git archive 7350e84` 的独立主干副本（21.0s）复跑，失败位置和原因一致，确认是基线既有失败；不能计作成功：

- `TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement`：fixture 使用 opencode，领取报 unregistered execution combination；后续完整 Service 重启/回执流程未执行。
- `TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery`
- `TestRemoteOrcaLostStartReconcilesAfterRestartWithoutResend`
- `TestRemoteOrcaQuestionLostReplyAndAckAreReconciled`
- `TestRemoteOrcaRejectsReplacementSession`

四个 remote 用例均在 Claim 被 #59 权限证据门槛拒绝。本轮保留这些测试及准入拒绝，不通过 mock 认证、跳过测试或开放旧组合消除失败。旧 opencode fixture 需要改造为受支持方案并完成完整回归；远程成功验收需要原生 worker-start 权限落实证据。工具 PATH/target 认证缺乏固定查询入口的限制继续按 #61 返回 unknown 或拒绝。
