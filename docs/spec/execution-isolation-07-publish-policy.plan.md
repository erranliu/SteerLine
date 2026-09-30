# Plan 项目发布责任契约

任务 ID：60；任务名称：项目发布责任契约；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-07-publish-policy.spec.md)。硬前置：#58 合入，一个实现 PR 交付。

## 实施步骤

1. 接收 #56 发布请求/provider、#58 执行边界，按 Spec 固定现有支持矩阵，分开 query 和 create 能力。不得根据未来可能支持的组合放行。
2. 在 execution/publication.go 定义纯值，在 publication/policy.go 实施能力匹配；接通必填 publisher 解析，替换 localPublish/host 责任推导。中心启动时匹配策略与实际路径，明确记录拒绝原因。
3. prompt 从固定 publisher 生成责任；平台 preamble 单独构造。宿主模式接回原 marker 受理和发布意图，worker 模式仅收集候选并经相同 provider 核验。
4. 领取与恢复接通 publisher/provider/remote/工具及认证引用冻结、完整摘要与版本拒绝；冻结前拒绝并脱敏带认证秘密的 URL/参数。provider/发布操作从当前 Execution 的固定上下文构造。扩展同一个执行配置快照；#59 尚未合入时不预填未知权限字段，后合入方保留前者全部字段和摘要校验。Query/控制台展示非秘密责任及目标。
5. 更新 examples/配置说明，删除旧布尔与隐式选择。与 #59 串行集成共享文件，保留 TaskDelivery 和当前恢复门槛。

## 验证

```powershell
go test ./internal/steerline/publication ./internal/steerline/workerflow -count=1
go list -deps ./internal/steerline/workerflow
go test ./internal/steerline -count=1 -run '^(TestLocalWorkerPublishUsesHostGitAndExactPR|TestLocalWorkerPublishMarkerMustBeFinalAndBound|TestNativeOrcaPRBindingUsesProviderAndRetainsSession|TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged|TestNativeClaimPersistsChosenExecutionProfile)$'
go test ./internal/steerline/console -count=1 -run '^TestRunDetailUsesItsFixedProfileAfterProjectConfigChanges$'
```

PP1 用最小 publisher/profile/provider 表驱动检查；PP2 复用提示、marker/native PR fixture 并检查传递依赖；PP3 在同一领取/恢复 fixture 修改当前 publisher/provider/remote 配置，确认原执行仍使用原目标及引用；PP4 复用错误 PR/合并门槛。同一 fixture 复用多个断言，不新增发布端到端矩阵或真实 provider 副作用。

## 交付

PR 附支持矩阵、固定请求与两条生产路径，说明两类项目策略的串行集成结果。向 #61 交接 publisher 能力与快照，不交付另一种发布后端或自动选择策略。
