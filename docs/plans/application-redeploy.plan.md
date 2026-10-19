# 同 Runner 构建的重启与显示更新

普通开发，2026-10-19。规则归属 [worker lifecycle](../product-design/worker-lifecycle.md) 的 W18/E04–E05/V06–V08；本次仅修复调用方把服务退出和原生资源销毁混为同一部署门槛的问题，不改变 Runner 恢复、输入授权或停止策略。

1. 普通 build-deploy 复用安装配置固定的 Runner EXE，构建并更新 SteerLine/Web。首次安装或显式 `-UpdateRunner` 才从 RunnerRepo 构建 Runner。
2. 增加只读 `verify-restart`：原宿主/Web 退出、项目本地关闭/锁释放、checkpoint/配置/实例一致；原 Runner 服务的最终停止记录绑定原实例/request/state-dir/EXE，待用 EXE 与原 build pin 完全一致。逐候选和检查保持同 build/state-dir；未知请求保留原 ID。允许原生资源未结算，但输出单独的 restartable 证明，不伪造 resources_settled。
3. 部署停服失败时，同构建走重启核验；换构建仍要求资源结算及新环境，已有 pin 不同在停服前拒绝。新增显式 `restart`/`restart.bat` 使用相同核验和原启动入口；重建 ProjectRuntime 后沿原规则恢复观察，零自动业务输入。
4. 原失败回执、Run、PR、任务及原生终端保持。新 Runner 构建仍不能接管旧状态；当前格式不增加迁移或回退。

## 测试计划

专属自动验收 [ARD-A01–A04](../acceptance/application-redeploy.md)，无人工阻塞项。

| 验收项 | 级别 / 组 | 环境与命令 | 时点、通过条件与证据 |
| --- | --- | --- | --- |
| ARD-A01 | L1 / Runner service 消费 | 隔离 Go fixture；`go test ./internal/runnerclient -run 'TestServiceSettlement' -count=1` | 实现后；同原停止身份可重启，native unknown 不晋升资源结算；错实例/请求/目录/EXE/活动写者拒绝；本验收记录命令结果 |
| ARD-A02 | L1 / deployment、shared Runner | 隔离 Git/SQLite/公开 CLI fixture；分组运行 `go test ./internal/app -run '^TestDeployment' -count=1 -timeout 180s`、`go test ./internal/app -run '^TestRunnerPresetFreeze$' -count=1 -timeout 180s`、`go test ./internal/app -run '^TestSharedRunnerBuildPinBeforeService$\|^TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost$' -count=1 -timeout 60s` | 实现后；same build 只读核验通过并保留原 Run/PR/未知状态，变构建/本地未关/错配置/错 pin 拒绝，既有重建零输入测试保持；本验收记录 |
| ARD-A03 | L1 / PowerShell 部署路由 | Windows PS 5.1；`powershell -NoProfile -File scripts/test-deployment-stop.ps1`、部署选择 fixture | 实现后；默认复用 pin、显式升级才构建；重启证明不走资源销毁核验，失败仍拒绝文件替换；本验收记录 |
| ARD-A04 | 静态与编译 | `git diff --check`、PowerShell Parser、新 CLI 编译 | 交付前；语法及格式通过，未操作安装/远端执行；本验收记录 |

不跑 L2/L3/full：本次不新增原生执行语义，调用方公开协议和已有生命周期 fixture 覆盖变更边界。用户明确本轮不停止执行、不部署；实际安装更新及真实远端跨重启验证记为未执行。
