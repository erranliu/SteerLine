# 环境检测自动收尾

`merge-h-308-code` 的原检测 `f98884685a9a4f81bb6ba1feefee2a24` 在 Orca 状态连接及 Runner 对账超时后进入 `blocked / manual_intervention_required`，仍无停止请求。该现场用于只读诊断。实现采用[环境检测流程](../../spec/project-environment-workflow.spec.md#5-生命周期与界面)中的检测中、收尾中及最终结果。

首次观察到原 Run 明确需要人工恢复时，测试验证了期限前发送一次原停止请求并保留 Orca 故障。停止回执未知、原 Run 后续状态变化或经过总期限时继续核实原请求。原 Run 已停止而结果仍待封存时，收尾状态和操作指引继续显示。结果封存后显示检测失败，页面恢复重新检测入口。

受测基线为 `62440dc5d2805f3b2055b7b7009c6ccbcdd73e4c` 加本次工作树改动。L1、L2、环境页与任务派发的 39 项前端测试、生产构建、两项模拟 API 的 Edge 浏览器检查，以及 SQLite 活动中心定向测试均通过。初次沙箱内 App 测试受临时 Git 目录写入及 PowerShell 权限阻止，正式验证使用隔离临时目录及沙箱外进程。测试使用受控 Runner，安装现场未部署此次变更。

环境专项 `go.environment` 的四个 owner 包及 32 个 App 测试根中的 31 个通过。唯一失败是新增详情和历史测试的收尾指引前缀断言。修正断言后，失败子项在 `environment-cleanup-app-recheck` 中定向复测通过。

分级报告和浏览器 trace 位于 `C:/Users/lmas1/.codex/visualizations/2026/11/15/01a1de3f-2527-7c90-945e-c7351d039ea0` 下的 `environment-cleanup-*` 目录。
