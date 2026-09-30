# #60 发布责任验证

基线：`4202dbba7013ba248d74a47e3410bdeffe0b6e03`（PR172）；验证日期：2026-10-01。范围止于 #60，未启动 #61。以下为确定性回归，不包含真实外部 push/create/人工合并或双机权限认证。

## 所有者与集成

`execution.PublicationPolicy` 唯一拥有纯责任值。`publication` 拥有固定支持矩阵、非秘密校验、provider、宿主固定请求与只读 Reconcile；`contracts` 拥有非秘密 provider DTO 和同一个 FixedExecutionProfile/PolicyResources 组合。中心拥有领取、完整摘要、恢复组装与原唯一 TaskDelivery 事务。workerflow 仅使用纯 policy 生成共同责任指令，Orca 原生说明保留在平台接线。

Profile 格式升为 v3，既有 PolicyResources 格式不改，其 ProfileDigest 绑定完整 v3 摘要；宿主 Request 升为 v2，增加实际 remote URL 和完整 provider 配置摘要。旧 Profile/请求在副作用前拒绝，不回填、迁移、双写或降级。#59 两份原生报告的文件和原摘要保持不变；测试用原 v2 摘要算法独立核验历史证据，生产校验仍拒绝 v2。

| 路径/provider | worker | host_service |
| --- | --- | --- |
| local Codex/GitHub command | 拒绝 | 原精确宿主 GitHub draft PR 实现 |
| local/其他 provider | 拒绝 | 拒绝 |
| 本机 Orca/command 或 Gitea query | 原 worker 发布 | 拒绝 |
| 远程 Orca/已有可查询 provider | 保留原发布能力，组合权限缺证仍拒绝 | 拒绝 |

worker 成功或 URL 只提供候选。两种责任复用精确身份、closed-unmerged、真实 merged 和唯一交付规则。删除服务共享 PR client，恢复检查、轮次完成/轮询、原生候选、废弃查询/关闭及直接结项按原冻结 provider 组装。

## 已执行检查

- PP1：publication 包的 `TestPublicationPolicySupportMatrix`；中心 `TestProjectPublicationPolicyRequiredBeforeRepositoryProbe`、`TestConfigLoadsLocalAndRemoteOrcaProfilesWithoutProbingTargets`、`TestPublicationAndPermissionChecksRemainCombined`。缺失/非法策略先拒绝，不自动切换 publisher，保留远程具体权限请求门槛及 network deny/--search 拒绝。
- PP2：publication/workerflow 包全部测试；`TestWorkerFlowPromptIncludesFixedWorkflowRunID`、原 marker 用例及事务意图失败用例；本机 Orca `TestWorkerEnvironmentER1ProbeAndSettlementGate/success` 与 local `TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace/free_text`。均为受控协议/Service 回归，不能替代原生权限证据。
- PP3：`TestPublicationPolicyFrozenConfigurationAndCredentials`、`TestPublicationOriginalProviderAfterRestartAndRemoteDrift`；改当前 provider/query/close/认证引用后重启，原 query/close 仍有效且 create 计数不增加。真实 Git reader 与原 transport 不匹配时拒绝。`TestPublicationSecretsRejectedBeforeFreeze` 检查秘密 URL/参数诊断脱敏；宿主认证环境引用的值不进入 local worker。
- PP4：Plan 的本地发布、原生 PR、错误 PR/closed-unmerged、选定 Profile 用例通过。原宿主未知请求恢复仍只读对账；直接结项事务新增核对原 Profile/PolicyResources 未变化。
- #59 回归：`TestAppServerLaunchMessagePersistsPolicyResources`（thread_ready、失败 turn_result）及 `TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests` 通过。
- `go test ./internal/steerline/publication ./internal/steerline/workerflow ./internal/steerline/orca_remote_worker -count=1` 通过；`go list -deps ./internal/steerline/workerflow` 及 execution/contracts/publication 直接依赖检查完成，没有引入 workerflow → publication/平台/数据库/中心依赖。
- `go test ./internal/steerline/console -count=1 -run '^TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges$'` 通过；显示同一 ProjectView 中的 publisher、原 provider/remote/base、四类命令描述和认证引用。

## 未通过的原 Plan 回归

原命令已真实执行，六项均命中：

```powershell
go test ./internal/steerline -count=1 -run '^(TestLocalWorkerPublishUsesHostGitAndExactPR|TestLocalWorkerPublishMarkerMustBeFinalAndBound|TestNativeOrcaPRBindingUsesProviderAndRetainsSession|TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery|TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged|TestNativeClaimPersistsChosenExecutionProfile)$'
```

五项通过；`TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery` 在原 `ClaimNativeTask` 权限检查失败：

```text
unsupported: requested execution_policy (read=host write=workspace git=true network=allow escalation=deny) lacks remote worker-start enforcement evidence: environment=env-home connection=home-connection repository=id:repo-test Codex=unverified
```

这是 #59 保留的 `orca_remote_worker.CheckPolicy` 门槛，未将 mock 当作权限证据、未放宽它、未删除原两轮 Service 流程或远程协议链路。原测试本轮仅补显式 worker 发布策略和已有 query 工具引用。远程协议包回归通过；不能宣称原两轮 Service 场景已通过或已完成双机认证。独立复审须明确处理这项验证与当前缺证策略的冲突；本轮未新增远程权限落实能力。

## 自审修复

补齐直接 CompleteNativeTask/DiscardNativeTask 和只读 InspectRecovery 的原 provider 来源；发布前再次检查工作区 fetch/push URL 和完整 provider 请求摘要；对后续输入检查实际 worktree 的 remote。冻结命令参数拷贝避免当前配置的 slice 修改损坏原记录；host worker 环境排除 token、SSH/工具认证与 Git 配置环境引用值。保留原权限资源核验、来源/受理版本、人工恢复门槛与事务所有者。
