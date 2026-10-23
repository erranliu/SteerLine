# 控制台操作即时反馈

> 2026-10-15 TUI 退役：本文保留原交付/验收事实，TUI 专属操作和未验项不再适用，不记为通过。共享后台/API 要求继续归原记录；当前范围见 [TUI 退役](../../spec/tui-retirement.spec.md)。

2026-10-09，按用户明确要求直接实现，不经 roadmap-runner，不修改 execution-map。范围为控制台显示、原请求回读及短暂加快轮询；服务协议和业务持久化时序保持原契约。

| 项 | 验证方式 | 结果 |
| --- | --- | --- |
| COF-A1 | `TestOperationProgressFromSubmissionThroughFailure`：提交即显示、立即回读同一请求、首次 applying 即刷新、失败刷新并清除进度，不重复提交 | 已通过 |
| COF-A2 | `TestOperationProgressKeepsSiblingAndServiceIdentity`：新增候选不覆盖运行中兄弟候选，旧实例/停止跟踪请求不覆盖当前状态 | 已通过 |
| COF-A3 | `TestOperationReceiptPollingIsBriefAndDoesNotStack`：前五秒使用 100ms 轮询调度，之后恢复 500ms，不叠加在途回读 | 已通过 |
| COF-A4 | 控制台包现有原请求、决策、候选、恢复、布局回归 | 已通过，147 tests / 0 failed / 0 skipped |
| COF-A5 | Windows `build.ps1` 构建及安装版只读 `task list` | 已通过，新二进制可读取当前 SteerlineTest 数据 |
| COF-M1 | 重新打开安装控制台，现场感受提交、启动及反馈过程；不声明已测得端到端延迟 | 待用户实际使用，不阻塞本次安装 |

自动验证命令：`go run ./cmd/test-feedback -- ./internal/console -count=1`。不涉及服务状态机或后端变更，未选择 L2/L3、Full 或真实模型交付。

原始证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T032626.461773200Z/summary.json`。源基线 `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5` 加本次未提交控制台修改，构建标记 source_dirty=true。

本机验证曾安装 `C:/SteerlineRuntime/bin/steerline-20261009-console-feedback.exe`；启动器原文备份在 `C:/SteerlineRuntime/backup/console-feedback-20261009-112816/`。随后按用户要求，将 `console.bat`、`service-window.bat` 恢复为指向 `bin/steerline.exe`，当前标准启动器不启用本次候选二进制。原程序和运行中的服务保留，本次安装不提交任务操作，不修改 execution.json。
