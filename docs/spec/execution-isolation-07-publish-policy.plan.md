# Plan 项目发布责任契约

任务 ID：60；任务名称：项目发布责任契约；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-07-publish-policy.spec.md)。硬前置：#58 合入，一个实现 PR 交付。

## 实施步骤

1. 接收 #56 发布请求/provider、#58 执行边界，按 Spec 固定现有支持矩阵，分开 query 和 create 能力。不得根据未来可能支持的组合放行。
2. 在 execution/publication.go 定义纯值，publication 做能力匹配；contracts 记录引用底层值，中心接通 publisher，保持 contracts → execution、不引入反向导入或发布 I/O。支持矩阵包含当前远程 worker，不把缺少双机认证误写为代码不可启动。
3. prompt 从固定 publisher 生成责任；平台 preamble 单独构造。宿主模式接回原 marker 受理和发布意图，worker 模式仅收集候选并经相同 provider 核验。
4. 领取/恢复接通 publisher/provider/remote/工具/引用冻结、完整摘要与版本拒绝；冻结前拒绝并脱敏秘密 URL/参数，按原执行构造 provider。与 #59 串行扩展同一个 contracts 快照，不预填未实施字段。现有 query 读端口和 ProjectView 投影展示非秘密责任/目标。
5. 更新 examples/配置说明，删除旧布尔与隐式选择。与 #59 串行集成共享文件，保留 TaskDelivery 和当前恢复门槛。

## 验证

```powershell
go test ./internal/steerline/publication ./internal/steerline/workerflow -count=1
go list -deps ./internal/steerline/workerflow
go test ./internal/steerline -count=1 -run '^(TestLocalWorkerPublishUsesHostGitAndExactPR|TestLocalWorkerPublishMarkerMustBeFinalAndBound|TestNativeOrcaPRBindingUsesProviderAndRetainsSession|TestRemotePublicationClaimRequiresPermissionEvidence|TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged|TestNativeClaimPersistsChosenExecutionProfile)$'
go test ./internal/steerline/console -count=1 -run '^TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges$'
```

PP1 用最小 publisher/profile/provider 表驱动检查；PP2 复用提示、marker/native PR fixture 并检查传递依赖；PP3 在同一领取/恢复 fixture 修改当前 publisher/provider/remote 配置，确认原执行仍使用原目标及引用；PP4 复用错误 PR/合并门槛。同一 fixture 复用多个断言，不新增发布端到端矩阵或真实 provider 副作用。

串行集成 #59 后，默认远程目标缺少权限落实 producer，原 `TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery` 会在领取前拒绝；协调 session 在 #59 基线和本轮均已实际运行并复现同一失败，原测试及两轮流程保留，不计为通过。当前最小验收改核对发布能力仍支持、实际 Claim 因请求权限缺证拒绝且不探测工具/远端或写入执行状态；远程协议包验证原 session/terminal 和续聊边界。不放宽 #59 或增加本轮未授权的远程权限实现，具体记录见本轮验收文档。

## 交付

PR 附支持矩阵、固定请求与两条生产路径，说明两类项目策略的串行集成结果。向 #61 交接 publisher 能力与快照，不交付另一种发布后端或自动选择策略。
