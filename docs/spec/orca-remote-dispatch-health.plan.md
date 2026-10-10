# 远端 Dispatch 健康检查修复 Plan

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](orca-terminal-transport.spec.md) / [Plan](orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

依据 [Spec](orca-remote-dispatch-health.spec.md)。普通直接开发，不改变执行 DAG。

1. 读取现场原终端与 Dispatch，核对第二个 Codex 的同 conversation 占用现象；保留现场，不重发 101。
2. 初始/反馈发送前追加已准备工作区身份，拒绝缺失路径、branch 或固定 commit 的输入。
3. 空 inbox/timeout 后使用原 Dispatch 做只读身份验证；失败结束 observer，通过既有状态机持久化阻塞。不得替代启动或伪造事件。
4. 补齐场景验证：真实远端双轮检查目标 worktree 的 Codex 终端唯一性，首轮后通过 terminal switch 显示原 worker，等待实际 renderer 挂载再核对；仅检查 worker，不删除 shell 或其他会话。renderer 未挂载产生的 orphaned 标记不代替 worker-show 的原生存活事实。
5. DH5 前置是可修改的 Orca 源码及默认 worktree 激活入口的验证能力；SteerLine 仓库不包含该源码，当前已向用户请求路径。取得后定位 renderer/恢复对原 terminal 的复用，补充回归并在同一目标环境验证；不得编辑安装包或关闭原 worker 作为修复。缺少前置时明确登记该项未完成。
6. 最小验证后记录结果及上游 Orca 限制，构建运行二进制；不把未启动的新构建称为已在运行服务生效。

## 测试计划

所有条目对应[任务专属验收](../acceptance/orca-remote-dispatch-health.md)。

| 验收项 | 级别/场景与命令 | 环境/时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| DH1 | 定向 Go：`TestWorkerWorkspaceInstructionUsesPreparedRemoteIdentity`、`TestWorkerFlowPromptBindsFixedTaskContract`、`TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace`；`go run ./cmd/test-feedback -- ./internal/execution -run 'TestWorkerWorkspaceInstructionUsesPreparedRemoteIdentity|TestWorkerFlowPromptBindsFixedTaskContract|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace' -count=1` | Windows/修改后 | 实际 path/branch/base 被派发，反馈沿用原身份，旧证据不作为 authority | 专属验收登记 runner summary 路径 |
| DH2、DH3 | 定向 Go：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcaremote -count=1`，含 `TestRemoteEmptyInboxDetectsLostDispatchWithoutRelaunch`、原 input acceptance 和反馈拒绝用例 | Windows/修改后 | 空和 timeout 的 attachment/terminal/incarnation/branch/runtime 失效均返回身份错误，无额外 mutation；原回归通过 | 同上 |
| DH1、DH2、DH3 | L1：`go run ./cmd/test-feedback -level=L1` | Windows/定向后 | 默认最小领取、SQLite、控制与依赖边界通过 | 同上 |
| DH4 | L3/live `delivery.orca-remote-codex`：提供绑定当前源码的无凭据 manifest，`go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-remote-codex'` | 台式机64g/独立 owned fixture，定向后 | 两轮唯一原 Codex terminal，PR 更新与真实合并唯一交付及清理通过 | runner scenarios/summary 路径及具体二进制 SHA-256 |
| DH5 | 取得 Orca 源码后，针对实际 worktree 默认激活/恢复入口补充可用回归命令；命令取决于 Orca 工程真实测试入口，源码取得前未落实，不算通过 | 明确前置：Orca 源码及其 renderer/原生 worktree 激活驱动；不以 terminal switch 冒充默认打开 | 已有终端被接回，无相同 conversation 的额外 resume 进程/占用提示 | 专属验收登记源码、命令、原生终端与会话占用结果 |

无需 Full/L2：本次使用最小回归与独立远端交付，不扩展其他场景或恢复 101。无人工项；若 Orca 公开身份仍失效或出现同 conversation 双 Codex，DH4 必须记失败，不能替换身份绕过。
