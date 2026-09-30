# Plan 执行需求与配置注册收口

任务 ID：61；任务名称：执行需求与配置注册收口；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-08-resolution.spec.md)。硬前置：#59、#60 合入，一个实现 PR 交付。

## 实施步骤

1. 接收完整权限及 publisher/provider/remote 快照和支持矩阵，列出项目、全局、实现字段及强制能力。核对两项已串行集成到一个当前格式快照，不把缺少的上游冻结留在本轮补做。
2. 实现 requirements 纯模型与认证报告匹配；注册固定工具解析器和 id/site 认证引用检查。git 使用原冻结 executable，其他工具按固定映射/目标 PATH，能力查询与 sandbox 就绪分开。未知目标、版本解析或认证 site 不满足要求，日志/快照不含秘密。
3. 在主包集中内建注册键、构造、静态预检及组合快照；各适配器接收自己的 Config。核验完整摘要和领取版本，绑定后分别核验原资源。深复制可变字段。
4. 基于现有 worker_mode 和 workerflow 协调，把业务消费者切到公共操作/事件/能力，收敛注册选择和原生 PendingIdentity 解释，不重复造生命周期协调器。保留必要具体组装和诊断，不改 TaskDelivery 或存储事务。
5. 用测试内 fake 实现证明公共契约可接入，不增加 production 模式。更新架构与配置说明，审查全部类型、调用链和副作用写入点。

## 验证

```powershell
go test ./internal/steerline/execution ./internal/steerline/workerflow -count=1
go list -deps ./internal/steerline/execution ./internal/steerline/workerflow
go test ./internal/steerline -count=1 -run '^(TestWorkerFlowContextsDoNotExchangeExecutionProfiles|TestWorkerFlowContextKeepsFixedRunAndCopiesConfig|TestWorkerFlowClaimsFreezeIndependentLocalProfiles|TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent|TestAppServerServiceTwoTurnsPersistNativeReceipts|TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestUnsupportedOrcaWorkerFlowConfigurationCannotClaim|TestProjectViewSharesTransactionAcrossWriterCommit)$'
go test ./internal/steerline/console -count=1 -run '^TestFileQuerySourceReadsCombinedProjectView$'
go run ./cmd/test-feedback -quick
```

新增一组需求/注册表驱动用例覆盖 ER1，包括 service/target 不可互代、版本解析失败和 mock 不能冒充认证；传递依赖与原生调用审查对应 ER2；复制/领取/完整快照及两执行断言对应 ER3；两类路径和 fake 接入对应 ER4。快速入口只运行一次，使用现有隔离/非真机开关，不把 quick 写成 full。只有跨模块失败才扩大范围，必要全量使用 `-workers=3`。

## 交付

PR 附最终配置归属、注册键、业务/平台/事务所有者和删除清单；明确需求存在性检查与真实权限证据的区别。更新八轮剩余规划，不重复留下已交付清单。没有生产部署、中心任务创建或数据库迁移动作。
