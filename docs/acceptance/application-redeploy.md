# 同 Runner 构建的重启与显示更新验收

归属 [Plan](../plans/application-redeploy.plan.md)，2026-10-19。

| 项 | 通过条件 | 状态 |
| --- | --- | --- |
| ARD-A01 | 服务退出证明与资源结算分别消费，原实例/请求/目录/EXE 围栏保持 | 通过 |
| ARD-A02 | 同构建重启核验保留原绑定和诊断，错配/未关闭拒绝；原重建零自动输入 | 通过（隔离公开 CLI fixture） |
| ARD-A03 | 普通部署复用固定 Runner；升级显式选择；核验路由和失败阻断正确 | 通过（Windows PowerShell 5.1） |
| ARD-A04 | CLI 编译、脚本解析和 diff 检查通过 | 通过；cmd 包编译，无匹配测试不计场景通过 |

用户明确要求本轮只修改代码，不停止远端终端、不重置任务、不部署。安装现场只读诊断不是验收现场；真实更新/远端重启未执行。

执行证据：`.evidence/application-redeploy/verification.json`；Go 结果分别保存在 `deployment-rules.log`、`runtime-rebinding.log`、`ownership-fences.log`，脚本解析为 `powershell-parser.log`。

- `go test ./internal/app ./internal/runnerclient ./internal/app/host ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestUnifiedStartupLaunchers' -count=1 -timeout 180s`：通过；app 61.383s，runnerclient 0.897s，host 0.926s，cmd 仅编译。
- `go test ./internal/app -run '^TestRunnerPresetFreeze$' -count=1 -timeout 180s`：通过，146.922s；保留既有旧运行时拒绝替代实例、显式重建原绑定/零输入、错 Run 和变构建拒绝断言。
- `go test ./internal/app -run '^TestSharedRunnerBuildPinBeforeService$|^TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost$' -count=1 -timeout 60s`：通过，3.974s。
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1`：通过，包括原严格资源核验、同构建重启证明、未知/不完整证明拒绝、默认复用、显式构建选择及已有 pin 不同提前拒绝。
- 四个修改的 PowerShell 脚本经 Windows PowerShell 5.1 Parser 解析通过；`git diff --check` 通过。

首次将多个既有 app 回归合为一组运行时达到累计 180s 上限（当时刚进入 TestSharedRunnerRejectsBuildBeforeStart）；不记该批通过。随后按上述定向组分开执行并通过，未运行全仓或 live。
