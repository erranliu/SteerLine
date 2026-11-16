# 部署停止请求待核实回归（2026-11-16）

原部署在 `deploy.stop-and-verify` 等待约 15 分钟后失败。

用原安装 Runner 的公开 `inspect --request-id` 查询三条停止请求，其中两条已完成。剩下一条已受理，仍为 `pending`。该 Run 在 Git worktree 创建失败后，原 `prepare_workspace` 操作结果待核实。

使用正式 `service stop` 结束失败部署留下的原 Runner 写入服务。原 Runner 确认服务退出，`service verify-stopped` 仍报告该 Run 原生写入者缺少缺席证明。随后使用原部署包的同构建恢复入口。原应用启动时尝试提交新的维护停止请求，Runner 因原停止尚未结束而拒绝启动。恢复失败后，三份已安装 EXE 的 SHA-256 均与原部署记录一致，应用、Web 和 Runner 进程均已退出。此次未进行文件替换或运行历史归档。

修复让部署保留最近一次原停止回执中的 Run、请求和操作诊断。等待失败后，使用记录过的同一服务实例 ID 结束已核验的 Runner 写入服务。维护模块仍观察已受理的停止，启动及后台观察避免新建第二条停止请求。归档和替换仍要求原生资源核验通过。

## 自动验证

现场部署和首轮验证基于 `a6de6145` 加本次修复。

- L1 `go run ./cmd/test-feedback -level=L1 -out <独立目录>` 通过。
- L2 `go run ./cmd/test-feedback -level=L2 -out <独立目录>` 在独立目录复测通过。首轮沙箱调用未通过。
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` 通过。
- 定向 Go 回归通过。运行 `go run ./cmd/test-feedback -out <独立目录> -- ./internal/app ./internal/refactoring -run '^TestDeploymentDiscardRuns$/^stop-order-and-unknowns$|^TestRefactoringStartupReconcilesOriginalStop$|^TestMaintenanceUnknownSubmitStopAndFinish$' -count=1`。核实已受理的停止请求不重复提交，等待取消仍可核对原 Run、请求和操作，并收尾本轮启动的写入者。
- L4 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <独立目录>` 的 Windows 原生复测中，`TestDeploymentDiscardRuns` 全部通过。已有用例 `TestRunnerPresetFreeze/reject_mismatched_run_after_explicit_restart` 等待控制回执时超时，L4 整体未通过。
- 单独复测该超时用例通过。运行 `go run ./cmd/test-feedback -out <独立目录> -- ./internal/app -p=1 -parallel=1 -run '^TestRunnerPresetFreeze$/^reject_mismatched_run_after_explicit_restart$' -count=1`。

## 用户授权放弃旧运行后的实际部署

用户随后授权放弃旧运行并完成部署。现场再次核实原宿主、Web 和 Runner 写入服务已退出。原 Runner 清单包含 7 条 Run，6 条已结清。唯一未结清的仍为上述本机 worktree 创建，公开回执确认其没有工作目录和已启动回合，Git 路径、登记及分支均缺失，现场进程检查未发现该安装或该 Run 的进程。原 Runner 的待核实结果原样写入维护记录。

在部署锁、启动协调锁和项目仓库锁约束下，使用现有任务重建函数建立新库，逐项核对任务编号、标题、类型、优先级、目标、固定契约及全部依赖。旧项目库、应用与 Web 状态、三份原程序和原 Runner 状态完整归档。新库按显式放弃旧运行的语义恢复为 open、manual。

用 `scripts/build-deploy.ps1` 完成正式部署，耗时约 96 秒。后端、Web、Runner 与全部 4 个项目均 ready，Web 返回 HTTP 200，三份已安装二进制 SHA-256 与部署包相符。启动后用公开任务图接口核对 20 个 open 任务和 8 条依赖。原始日志和现场标识保存在本机验收记录。
