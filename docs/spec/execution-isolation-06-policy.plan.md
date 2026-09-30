# Plan 项目执行权限策略

任务 ID：59；任务名称：项目执行权限策略；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-06-policy.spec.md)。硬前置：#58 合入，一个实现 PR 交付。

## 实施步骤

1. 承接 #57 的本地 Windows 默认、冻结和冲突拒绝，核对实际 CLI/app-server 的权限、网络、Windows 原生契约与工具证据。先核实两条路径的最低组合及来源可得性再扩展配置接线，不在缺证时建一套只会拒绝的完整实现。
2. 创建 policy/snapshot 类型及项目配置解析，实现字段必填、scope 组合和资源授权校验。删除顶层 sandbox/windows_sandbox 与原生 profile 授权来源，更新版本和 examples；Windows 设置移进 Codex Config 并保留 local elevated 默认，统一分散根 + unelevated 拒绝。
3. 接通 Codex CLI/thread/turn 参数编译、安全 args 校验和 Orca 的有/无 model 统一受管启动。参数在任何外部副作用前完成验证，未知设置不能落回 opaque agent 创建。
4. 领取时核对受理版本和完整摘要，冻结策略/Profile；绑定时通过现有附件事务保存资源与实际设置。后续输入/恢复使用原快照，拒绝损坏或扩大根目录。与 #60 串行扩展同一快照，后合入方保留对方字段和摘要校验。
5. 从已有 ReadProjectView 同一快照接通 Query/控制台要求、冻结值和核验状态，保持部分只读错误及配置不一致启动拒绝，更新说明。同步环境就绪任务的配置归属，保留其探针和宿主恢复范围。
6. 在隔离 linked worktree、状态和会话各运行一次 local/Orca 的同类工具检查，核对工作区、Git 写根和明确权限设置，保存版本、身份、启动设置和原生工具结果；仅修改本轮临时文件/索引并精确清理，不重跑环境就绪收据流程。

## 验证

```powershell
go test ./internal/steerline/execution ./internal/steerline/codexworker -count=1
go test ./internal/steerline -count=1 -run '^(TestNativeClaimPersistsChosenExecutionProfile|TestWorkerFlowClaimsFreezeIndependentLocalProfiles|TestAppServerThreadUsesNoninteractiveApprovalPolicy|TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent|TestLocalWindowsSandboxProfileFreezesAndRestores)$'
go test ./internal/steerline/console -count=1 -run '^(TestExecutionSettingsOnlyAppearInStartChoiceAndConfigMismatchBlocksStart|TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges)$'
```

在迁移后的原参数 fixture 增补 PO1/PO2 表驱动检查，覆盖一组有效配置和必要冲突/不支持组合；领取/绑定与展示对应 PO3。PO4 使用实际原生入口保存到 `docs/validation/execution-isolation-06-policy/`，启动命令在实施能力核实后固定，不把占位命令当成运行结果。若该环境已完成同版本就绪探针，可以复用绑定同一策略和路径的证据，不重复整套实验。

## 交付

一个实现 PR 同时交付权限语义、参数、冻结、配置/展示和必要原生证据。格式变化明确初始化与保留旧现场的边界，不迁移旧执行，也不部署生产二进制或处理生产账户权限。
