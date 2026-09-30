# Plan 项目执行权限策略

任务 ID：59；任务名称：项目执行权限策略；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-06-policy.spec.md)。硬前置：#58 合入，一个实现 PR 交付。

## 实施步骤

1. 承接 #57 的 local Windows 和已有 Orca managed 参数、权限摘要、资源冻结，复用环境就绪原生材料。分开 local app-server 与 Orca TUI 的 Windows 支持：保留 local unelevated 分散根拒绝及已验证的 Orca unelevated 组合，先核对新策略映射等价与证据归属。
2. 创建 policy/snapshot 类型及项目解析，实现必填、scope/资源校验。删除顶层 sandbox/windows_sandbox 与原生 profile 授权来源，更新版本和 examples；Windows 设置归 Codex Config，按具体路径保留支持与拒绝规则，不统一误禁 unelevated。
3. 已有 CLI/thread/turn 编译和安全 args 改读冻结项目策略，Orca 有/无 model 继续同一受管命令，model/effort 独立可省略。保留 strict-config/排除隐式根和原准备接线，未知策略不能回落 opaque 创建。
4. 领取时核对受理版本和完整摘要，冻结策略/Profile；绑定时通过现有附件事务保存资源与实际设置。后续输入/恢复使用原快照，拒绝损坏或扩大根目录。与 #60 串行扩展同一快照，后合入方保留对方字段和摘要校验。
5. 从已有 ReadProjectView 同一快照接通 Query/控制台要求、冻结值和核验状态，保持部分只读错误及配置不一致启动拒绝，更新说明。同步环境就绪任务的配置归属，保留其探针和宿主恢复范围。
6. 在隔离 linked worktree、状态和会话各运行一次 local/Orca 的同类工具检查，核对工作区、Git 写根和明确权限设置，保存版本、身份、启动设置和原生工具结果；仅修改本轮临时文件/索引并精确清理，不重跑环境就绪收据流程。

## 验证

```powershell
go test ./internal/steerline/execution ./internal/steerline/codexworker -count=1
go test ./internal/steerline -count=1 -run '^(TestNativeClaimPersistsChosenExecutionProfile|TestWorkerFlowClaimsFreezeIndependentLocalProfiles|TestAppServerThreadUsesNoninteractiveApprovalPolicy|TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent|TestLocalWindowsSandboxProfileFreezesAndRestores|TestManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput)$'
go test ./internal/steerline/console -count=1 -run '^(TestExecutionSettingsOnlyAppearInStartChoiceAndConfigMismatchBlocksStart|TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges)$'
```

在已有参数/环境 fixture 补 PO1/PO2 的项目策略表驱动检查，分别覆盖 local/Orca 支持组合及冲突；领取、原资源与展示对应 PO3。PO4 保存新策略最小核验到 `docs/validation/execution-isolation-06-policy/`；已有就绪材料可作为映射/工具来源，不改写旧 ProfileDigest/收据或把它冒充新 Execution 证据，不重复整套准备实验。

## 交付

一个实现 PR 同时交付权限语义、参数、冻结、配置/展示和必要原生证据。格式变化明确初始化与保留旧现场的边界，不迁移旧执行，也不部署生产二进制或处理生产账户权限。
