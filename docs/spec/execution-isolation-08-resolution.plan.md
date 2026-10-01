# Plan 执行需求与配置注册收口

任务 ID：61；任务名称：执行需求与配置注册收口；目标：`EXECUTION-ISOLATION`。状态：已合入 PR #174，ER4 远程 Service 成功场景仍待权限证据；见[验收记录](../validation/execution-isolation-08-resolution/README.md)。唯一执行依据：[Spec](execution-isolation-08-resolution.spec.md)。硬前置：#59、#60 合入，一个实现 PR 交付。

## 实施步骤

1. 接收完整权限及 publisher/provider/remote 快照和支持矩阵，列出项目、全局、实现字段及强制能力。核对两项已串行集成到一个当前格式快照，不把缺少的上游冻结留在本轮补做。
2. execution 定义纯需求/匹配，contracts 引用它并保存业务快照；保持 execution 不导入项目包。三路径的目标/工具/认证证据分别检查，remote 不用中心环境推定，不把能力查询或远程 mock 等同权限/双机认证。
3. 在主包集中内建注册键、构造、静态预检及组合快照；各适配器接收自己的 Config。核验完整摘要和领取版本，绑定后分别核验原资源。深复制可变字段。
4. 基于现有 worker_mode 和 workerflow 协调，把业务消费者切到公共操作/事件/能力，收敛注册选择和原生 PendingIdentity 解释，不重复造生命周期协调器。保留必要具体组装和诊断，不改 TaskDelivery 或存储事务。
5. 接入现有 query 四类读端口和中心 ProjectReader 投影，不重建 SQL 或直接把 sqlite.Reader 当完整 ProjectReader。检查 scope/cursor/同事务/部分错误与预算；测试 fake 后端无需改业务状态机。更新架构/配置和所有者，保留必要 facade 别名。

## 验证

```powershell
go test ./internal/steerline/execution ./internal/steerline/workerflow -count=1
go list -deps ./internal/steerline/contracts ./internal/steerline/execution ./internal/steerline/workerflow ./internal/steerline/query
go test ./internal/steerline/query -count=1 -run '^(TestReadyQueriesRejectMissingCapabilityAndProject|TestTaskQueriesRejectReaderScopeAndCursorDrift)$'
go test ./internal/steerline -count=1 -run '^(TestWorkerFlowContextsDoNotExchangeExecutionProfiles|TestWorkerFlowContextKeepsFixedRunAndCopiesConfig|TestWorkerFlowClaimsFreezeIndependentLocalProfiles|TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent|TestAppServerServiceTwoTurnsPersistNativeReceipts|TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestRemoteOrcaProtocolTwoTurnsKeepSessionAndSettleResources|TestRemoteOrcaProtocolLostStartReconcilesWithoutResend|TestRemoteOrcaServiceAdmissionRejectsMissingPermissionEvidence|TestProjectViewSharesTransactionAcrossWriterCommit)$'
go test ./internal/steerline/console -count=1 -run '^TestFileQuerySourceReadsCombinedProjectView$'
go run ./cmd/test-feedback -quick
```

需求/注册表驱动覆盖 ER1；底层包与读端口/调用审查对应 ER2；同事务、复制/领取/快照对应 ER3；三链路和 fake 接入对应 ER4。execution 与 contracts/query/sqlite 均纳入当前 quick，稳定结果跑一次，不重复全部包或远程矩阵；只有失败才扩大，不把 quick/mock 写成 full/双机认证。

## 交付

PR 附最终配置归属、注册键、业务/平台/事务所有者和删除清单；明确需求存在性检查与真实权限证据的区别。更新八轮剩余规划，不重复留下已交付清单。没有生产部署、中心任务创建或数据库迁移动作。

## 执行结果与验收限制

实现与实际命令结果见 [#61 验收记录](../validation/execution-isolation-08-resolution/README.md)。原九项中心回归真实执行；local 两轮与 native 终端通过，远程两项在 #59 权限缺证门槛拒绝，未替换/删除原成功测试或将它们计为通过。补跑真实 Claim 权限拒绝及远程协议包验证现行边界，不代替原 ER4 三链路成功条件。该缺证门槛沿用 #60 已记录的限制，本轮遵守 Spec 不补前置权限 producer。

补充定向验证包含 `TestExecutionRequirements.*`、`TestRequirementVersionParsers`、`TestEmptyExecutionRequirementsKeepMandatoryChecks`、`TestClaimRechecksAcceptedConfigFiles`、`TestExecutionRegistry.*`、`TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation` 及固定协议/事实断言。query/console fixture 保留同事务与部分错误断言；没有全量或真机重新认证。

PR174 用户要求的修改质量审计已补完整目标核验、Profile 组装复制、原资源根恢复、证据完整摘要及事务不可变保护；需求阻断与原 worker 生命周期分离，观察/对账保留，新增副作用意图复核当前需求。认证报告 producer/consumer 使用同一注册协议 key。审计复现与验证见同一验收记录；转 ready 不代表远程缺证成功场景得到认证。

远程原成功 fixture 已按当前准入契约拆为 Protocol 回归和真实 Claim 拒绝；上述命令不计作 ER4 远程 Service 成功，追加恢复修复见[清理记录](../validation/execution-isolation-cleanup/README.md)。
