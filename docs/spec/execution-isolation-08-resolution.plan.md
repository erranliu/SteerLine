# Plan 执行需求与配置注册收口

任务 ID：61；任务名称：执行需求与配置注册收口；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-08-resolution.spec.md)。硬前置：#59、#60 合入，一个实现 PR 交付。

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
go test ./internal/steerline -count=1 -run '^(TestWorkerFlowContextsDoNotExchangeExecutionProfiles|TestWorkerFlowContextKeepsFixedRunAndCopiesConfig|TestWorkerFlowClaimsFreezeIndependentLocalProfiles|TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent|TestAppServerServiceTwoTurnsPersistNativeReceipts|TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery|TestRemoteOrcaLostStartReconcilesAfterRestartWithoutResend|TestProjectViewSharesTransactionAcrossWriterCommit)$'
go test ./internal/steerline/console -count=1 -run '^TestFileQuerySourceReadsCombinedProjectView$'
go run ./cmd/test-feedback -quick
```

需求/注册表驱动覆盖 ER1；底层包与读端口/调用审查对应 ER2；同事务、复制/领取/快照对应 ER3；三链路和 fake 接入对应 ER4。execution 与 contracts/query/sqlite 均纳入当前 quick，稳定结果跑一次，不重复全部包或远程矩阵；只有失败才扩大，不把 quick/mock 写成 full/双机认证。

## 交付

PR 附最终配置归属、注册键、业务/平台/事务所有者和删除清单；明确需求存在性检查与真实权限证据的区别。更新八轮剩余规划，不重复留下已交付清单。没有生产部署、中心任务创建或数据库迁移动作。
