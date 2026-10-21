# 部署中自动恢复中断宿主

普通开发，用户直接授权。规则归属 [worker lifecycle](../product-design/worker-lifecycle.md)，差异仅为部署自动完成原安装恢复和正式停服；不变更 Runner 原授权、输入、重试或原生停止规则。验收见[专属记录](../acceptance/deployment-interrupted-recovery.md)。

1. staged CLI 增加显式 `recover-deployment`。在应用启动协调锁下，仅对原宿主有精确已退出身份、原启动记录匹配、Web/Runner 已退出且当前停机回执缺失/陈旧的情况开放。安装记录中的三份 EXE 摘要、Runner pin/version/resource/state-dir、配置/checkpoint 的原进程和项目集合、原候选/检查/Generation 及候选兼容性须全部核实；匹配的当前失败回执、未知创建、活动服务/锁、变配置/文件或不兼容不触发恢复。
2. 使用已核实的原安装 EXE，沿既有启动入口等待所有项目 ready，再正常停服。全程持有协调锁，不创建业务 submit/reply/resume，不删除状态。原进程创建不明或恢复失败不循环重试、不强杀；失败继续阻断替换。
3. 复用原 verify-restart 全套关闭与接管核验；将中断宿主、旧回执、原文件摘要随新的重启证明保存。resources_settled=false 及旧诊断保留。部署只在允许重启且初次 verify-restart 失败时尝试一次，严格销毁路径和普通通过路径保持原行为。

## 测试计划

| 验收项 | 级别 / 场景与环境 | 命令 | 时点 / 通过条件 / 证据位置 |
| --- | --- | --- | --- | --- |
| DIR-A01/A02 | L1 / `TestDeploymentInterruptedRecovery`、`TestDeploymentRecoveryReadiness`，隔离 Git/SQLite/公开 Runner CLI fixture | `go test ./internal/app -run 'TestDeploymentInterruptedRecovery\|TestDeploymentRecoveryReadiness\|TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite\|TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost' -count=1 -timeout=180s` | 接线后；缺失/陈旧回执只恢复一次，等待项目 ready/online，核验新实例正式回执、原绑定和资源未知保留；活动/未知/错 hash/配置/成员/锁/候选及恢复失败反例均阻断；专属验收登记 |
| DIR-A03 | L1 / Windows PowerShell 部署 fixture | `./scripts/test-deployment-stop.ps1` | 接线后；stop→verify→恢复一次且验证证明，不在 strict/成功路径恢复；恢复失败/伪证明阻止替换；专属验收登记 |
| DIR-A04 | L1 / CLI 参数与现有身份接线，隔离 fixture | `go test ./cmd/steerline -run 'Test.*DeploymentRecovery\|TestRunnerTakeoverPreflight' -count=1 -timeout=120s`；`go test ./cmd/test-feedback -run '^TestQuickOwnerPartitionsRejectMissingAndReplayedGroups$' -count=1`；`git diff --check`；PowerShell Parser | 交付前；入口参数、编译、固定 L1 分区及语法通过；专属验收登记 |

本次不使用安装现场或模型调试，不跑 full/L2/L3：新增边界由既有生命周期入口、公开 CLI fixture 及部署路由组合验证。实际部署若另行执行，单独登记，不用它替代隔离测试。无人工阻塞项。
