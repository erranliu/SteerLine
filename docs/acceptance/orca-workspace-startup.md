# Orca 工作区启动时序修复验收

归属：本次普通修复；[实现及测试计划](../plans/orca-workspace-startup.plan.md)。

| ID | 自动验收项 | 状态与证据 |
| --- | --- | --- |
| OWS-A1 | 终端短暂未就绪可等待成功；列表与详情共用期限；其他错误和未知创建不重试 | 通过；workspace-targeted.log / TestWorkspaceTerminalSurfaceReadiness |
| OWS-A2 | 创建回执保存、立即核对及序列化后恢复；失败后保留原回执；恢复零资源写操作 | 通过；workspace-targeted.log / TestWorkspaceCreationReceiptRecovery 与 workspace-effect.log |
| OWS-A3 | 无回执、runtime/工作区实例/path/base/branch/head/终端身份改变、多终端时拒绝 | 通过；workspace-targeted.log / TestWorkspaceCreationRecoveryRejectsChangedIdentity |
| OWS-A4 | 独立 SteerlineTest 包真实 Orca 正常启动及原创建回执恢复，唯一原终端/main-agent 就绪并清理 | 部分核实；工作区/原终端绑定和原回执恢复通过，Codex 就绪被环境阻塞，整体未通过 |

## 范围和限制

真实探针只初始化 Codex 并请求 READY，不创建 PR、不证明完整任务交付。
目录信任仅通过本次探针的启动参数配置精确新工作区，不修改全局配置。
当前安装环境 101 缺少创建回执，未修改、重置或接管。

## 执行记录

2026-10-13 定向回归通过：完整 orcatransport/protocol 两个小包；立即恢复回执传递；
TestTaskStartControlRecoversReservationsWithoutReplay、TestTaskStartControlKeepsUncertainEffectsScopedToOriginalTask；
internal/testkit/boundary/... 架构边界。未运行全量。

最终证据包：[evidence](C:/SteerlineDebug/dev-20261013-145812-bd3a3537-dirty/evidence)。
真实探针最初被新目录信任提示阻塞；精确 Windows 路径的会话级信任补齐后，正常启动已完成
原工作区/终端绑定并发送启动命令，但 Codex 报 account/read 的 workspace routing discovery
timed out（-32603），未证明 main-agent 就绪。见 codex-bootstrap-block.json、workspace-startup-live.log。
这不是 OWS-A4 通过；不得以终端命令已接受代替 agent 就绪。

前两次诊断包分别为 dev-20261013-145033-bd3a3537-dirty、dev-20261013-145520-bd3a3537-dirty，
保留环境失败及探针修正记录（目录信任路径格式、测试 request ID 唯一性）；两包各自的探针工作区/终端
已清理，orca-startup-*.json 中 cleaned_up=true。保留生成包及 Orca 项目注册作为诊断现场，未操作安装项目。

原回执真实恢复的最终核验结果与清理另见 workspace-recovery-live.log、orca-startup-saved-receipt-recovery.json。
该记录 workspace_binding_verified=true、agent_start_commands=1、cleaned_up=true；
恢复过程只执行 worktree show、terminal list/show，通过 adapter 的原工作区实例和终端 incarnation 核验。
随后等待原生代理身份时 CLI 在期限内退出并无完整 JSON，真实 main-agent 就绪未通过。
正常启动记录同样 cleaned_up=true。未补 Enter、未重发原启动命令、未切换代理权限或修改账号配置。
