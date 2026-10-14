# 验收：本机 Codex、远端 Orca 执行方案

2026-10-14，普通配置调整，暂时停用本机 Orca 的部署方案。

- 仓库 `examples/execution.json` 删除 orca-local / orca-local-xhigh，保留 2 项本机 Codex 和 2 项远端 Orca，默认仍为 local-luna-medium。
- 调试模板原本就是本机 Codex，保持；生成器 `-Orca` 默认目标改为 name:台式机64G，其他远端通过 -OrcaEnvironment 指定。底层 Orca adapter、本机能力样本和隔离夹具保持，本次没有新建本机执行通道。
- 定向配置回归 `go test ./internal/app -run '^(TestCurrentExampleProfilesFreezeOpaqueOrcaCommands|TestGlobalExecutionProfilesSharedAcrossProjects)$' -count=1` 通过；PowerShell Parser.ParseFile 检查生成器无语法错误。未运行全量测试或真实 worker/PR 链路。
- 首次经用户授权同步维护 `C:\SteerlineRuntime\execution.json`，只删除同名两项方案；保留 3 项本机 Codex、2 项远端 Orca，默认仍为 project-default。程序未替换；首次配置维护时任务与历史保持，后续用户明确要求的局部状态清理见下节。
- 通过正式 stop-service 入口结算并关闭服务后更新配置，重新打开服务窗口和控制台；启动前清除代理进程继承的 NO_COLOR / TERM / CODEX_CI，不修改用户全局变量。服务启动回执 started，投影 running/ready=true；15 个任务行（含 payload 摘要）与维护前完全相同，任务 17 仍 closed/version 3。

安装配置更新前副本：`C:\SteerlineRuntime\backup\remote-only-orca-20261014-143136\execution.json`。

本次证据目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-remote-only-orca-1791959355148`，包含 profiles-test.log、准备好的 execution.json、tasks-before.json、installation-result.json。service-start.log 记录无窗口 start-service 返回 window-required，不作为启动成功证据；成功启动记录为安装状态目录 `service/startups/5ceeb401538c433122c888a42cfdfd0c/startup.json`，最终就绪见 installation-result.json。

## 用户授权清理旧本机 Orca 状态

随后用户要求丢弃安装版旧本机执行绑定并恢复任务 open。只读枚举安装版三个项目，旧 orca-local / orca-local-xhigh 绑定仅在 merge 项目任务 18/19；两项执行均未绑定实际 worker。通过正式 stop-service 结算关闭服务、关闭控制台，持有项目与控制台锁后执行一次性维护事务；该操作不新增产品迁移或重置接口。

任务 18/19 恢复 open，清除 claimant，版本 2→3；删除其 2 个 WorkflowRun、2 个 Execution、2 个 Attempt、6 个 Decision、4 个 Operation、2 个领取 Command、16 个运行 Event，以及 11 个相关控制文件；保留 task.created 事件、任务定义、契约、依赖及所有 Git 工作区。局部清理控制台记录，推进 graph_revision 2→3 并重新生成服务投影。没有备份旧运行记录。

事务内核对所有剩余业务记录逐行保持，任务 17 仍 closed/version 3，任务 20/22 的远端绑定保留；SQLite integrity_check=ok、foreign_key_check 无错误。正式 task list 复核任务 18/19 为 open/version 3。服务与控制台重新打开，保持 manual/unpaused；远端现有执行按原身份恢复观察，未启动新的业务领取。

维护脚本及证据：`C:\Users\lmas1\AppData\Local\Temp\steerline-local-orca-reset-1791959764110`，reset.py、reset-result.json、verification.json。这是用户明确授权的安装维护，不作为调试现场或真实执行链验收。
