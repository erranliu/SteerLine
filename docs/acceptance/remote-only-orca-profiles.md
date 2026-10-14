# 验收：本机 Codex、远端 Orca 执行方案

2026-10-14，普通配置调整，暂时停用本机 Orca 的部署方案。

- 仓库 `examples/execution.json` 删除 orca-local / orca-local-xhigh，保留 2 项本机 Codex 和 2 项远端 Orca，默认仍为 local-luna-medium。
- 调试模板原本就是本机 Codex，保持；生成器 `-Orca` 默认目标改为 name:台式机64G，其他远端通过 -OrcaEnvironment 指定。底层 Orca adapter、本机能力样本和隔离夹具保持，本次没有新建本机执行通道。
- 定向配置回归 `go test ./internal/app -run '^(TestCurrentExampleProfilesFreezeOpaqueOrcaCommands|TestGlobalExecutionProfilesSharedAcrossProjects)$' -count=1` 通过；PowerShell Parser.ParseFile 检查生成器无语法错误。未运行全量测试或真实 worker/PR 链路。
- 经用户授权同步维护 `C:\SteerlineRuntime\execution.json`，只删除同名两项方案；保留 3 项本机 Codex、2 项远端 Orca，默认仍为 project-default。程序未替换，任务与历史未重置；任务 18/19 原有冻结的本机 Orca 绑定保留，不能据删除目录方案宣称已转成 Codex。
- 通过正式 stop-service 入口结算并关闭服务后更新配置，重新打开服务窗口和控制台；启动前清除代理进程继承的 NO_COLOR / TERM / CODEX_CI，不修改用户全局变量。服务启动回执 started，投影 running/ready=true；15 个任务行（含 payload 摘要）与维护前完全相同，任务 17 仍 closed/version 3。

安装配置更新前副本：`C:\SteerlineRuntime\backup\remote-only-orca-20261014-143136\execution.json`。

本次证据目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-remote-only-orca-1791959355148`，包含 profiles-test.log、准备好的 execution.json、tasks-before.json、installation-result.json。service-start.log 记录无窗口 start-service 返回 window-required，不作为启动成功证据；成功启动记录为安装状态目录 `service/startups/5ceeb401538c433122c888a42cfdfd0c/startup.json`，最终就绪见 installation-result.json。
