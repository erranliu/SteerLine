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

## 基线失败与追加修复

清理初版的以下五项在分支（22.0s）及 `git archive 7350e84` 的独立主干副本（21.0s）复跑，失败位置和原因一致，确认是基线既有失败；不能计作成功：

- `TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement`：fixture 使用 opencode，领取报 unregistered execution combination；后续完整 Service 重启/回执流程未执行。
- `TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery`
- `TestRemoteOrcaLostStartReconcilesAfterRestartWithoutResend`
- `TestRemoteOrcaQuestionLostReplyAndAckAreReconciled`
- `TestRemoteOrcaRejectsReplacementSession`

追加修复后不再保留与准入契约冲突的失败 fixture：

- 本机原用例迁到 Codex/PowerShell，使用完整受管准备、有效工具/收据/结算，准备轮与业务轮分别计数；保留 worktree/coordinator/Run 创建回执丢失、五轮业务、回复/send/Task/Dispatch 丢失、Service 重启及替换身份拒绝。未知资源和派发使用公开 recheck 对账，不跳过人工门槛。
- 原四个 remote Service 成功用例改为四个名称明确的 Protocol 回归，继续覆盖两轮原 session/workspace、未结算 release 拒绝、精确清理、重建 adapter 后的丢失 start 只读对账、reply/ack 丢失及 replacement 拒绝。测试直接调用原适配器，没有 Claim 绕过、注册 gate 替换或权限证据伪造。合并交付的中心规则继续由 quick 的 TaskDelivery/provider 用例覆盖。
- 新增真实完整任务的 `TestRemoteOrcaServiceAdmissionRejectsMissingPermissionEvidence`：Claim 在任何外部 probe 前拒绝，整份 DomainSnapshot 保持不变。
- 实际暴露并修复两个生产问题：remote StartTask 复用前先 Show 并比较原 peer/coordinator/terminal/incarnation/Dispatch 完整身份及 liveness；managed native 的 resume 保留 RecoveryRequired，准备结算也不能自动派发业务，必须经原环境 recheck 确认。
- `TestWorkerEnvironmentRestartRequiresConfirmationBeforeBusinessInput` 证明重启后的原准备证据可以完成，业务 Dispatch 数在人工确认前保持 0；确认后继续同一 worker。

追加验证：本机完整恢复用例通过（90.5s）；环境准备 ER1 正向/故障矩阵、ER4 期限/丢失回执及新增重启门槛回归通过（139.2s）。remote 协议/Service 准入、注册操作及固定终端恢复组通过（54.0s），最小入口通过（execution=25.0s），构建、go vet 与 diff 检查均通过，未跑全量。相关中心命令：

```powershell
go test ./internal/steerline -count=1 -run '^TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement$'
go test ./internal/steerline -count=1 -run '^(TestWorkerEnvironmentRestartRequiresConfirmationBeforeBusinessInput|TestWorkerEnvironmentER1ProbeAndSettlementGate|TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt)$'
go test ./internal/steerline -count=1 -run '^(TestRemoteOrca(Protocol|ServiceAdmission).*|TestRegisteredOrca.*|TestRemotePublicationClaimRequiresPermissionEvidence|TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart)$'
```

远程 Service 的成功准入仍需要原生 worker-start 权限落实证据，ER4 的真实三链路成功条件仍未完成；协议模拟通过不能替代它。工具 PATH/target 认证的固定查询限制继续按 #61 返回 unknown 或拒绝。

## 主干整合

追加修复后整合 `49d71c9`（PR #176 会话只读查询拆包）。冲突仅在架构文档：保留新会话 Reader/端口/SQLite 接线与依赖图，叠加执行清理和恢复边界，重新生成完整源码索引。最终为 268 个生产文件、20 个包，中心 115 个文件；前文 263/114 是整合前的清理快照。

合并后最小入口通过（execution=24.2s），会话/history 确定性回归、构建及 go vet 通过；真实 native history 验收未启用。额外回归发现来源发现测试已构造私有 runs 却仍修改空的 State.Runs，修正为操作同一私有集合，保留错误 workspace 拒绝和原 session 保留断言。
