# PR 4A 修改质量审计

审计起点：`713949b`，PR #21。范围为当前 Spec/Plan 的执行身份、回复副作用、观察结束、持久化恢复、能力隔离和既有 local retry；Linux 仍只检查架构边界。

以下问题均已通过复现测试确认并修复：

| 优先级 | 问题与影响 | 修复 |
| --- | --- | --- |
| P1 | Orca Observe 返回 exited/unverifiable 时，Reply 仍删除旧结果并发送输入；CLI 成功还会被直接当作输入已接受 | 发送前要求原 terminal live，逐项验证 runtime、handle、incarnation、accepted；不确定回执保留 unknown，TurnStarted 单独返回 |
| P1 | 监督器先接受结果文件，不核对 runtime/incarnation；失联、等待和超时被当作普通失败释放槽 | 有界读取前后核对原身份；身份不明和仍可能运行的观察结束交回 typed unknown，由 Service 保留槽 |
| P1 | 重启只检查 start，损坏或未决 stop/reply 可被旧 runtime 投影遮蔽 | 枚举当前 attempt 的 Operation，严格解码并核对 binding；未决动作保留资源，损坏状态拒绝启动 |
| P1 | Orca 返回的工作区分支/base 在任务派发前未实际核验 | 在发出正式提示前检查真实 Git worktree、分支、共享仓库和精确 base；不从路径猜分支、不任选 terminal |
| P1 | Orca 原生 agent 的 verified 报告可被本地 Codex CLI 覆盖启动模式复用 | 覆盖模式探测 Codex CLI 版本并加入能力报告键；证据必须匹配该版本，原生模式证据不能启用覆盖模式 |
| P2 | 回复摘要只固定 continuation 路径，同 ID 的答案内容变化无法检测 | 摘要固定 decision、指针和完整内容，同时保存原 ExecutionRef；同 ID 异内容拒绝 |
| P2 | local 准备后取消仍传 Background 给进程启动器 | 将原调用 context 传至 StartWorker 和 Spawner；准备后取消不启动进程 |
| P2 | local 工作区已准备但启动进程前失败时，没有 binding 导致受限 retry 永久失败 | 仅允许 rejected 且无进程绑定的 local start，按持久化 intent 重新核验工作区/HEAD/领取，再创建新 attempt |
| P2 | 人工 mark-stopped 清除的 unknown 在重启时再次占槽 | runtime 中保留绑定 attempt 的 manual_stop；不改写外部动作未知结果，新 retry 清除该确认 |

额外收紧：初始执行回执核对 target、agent 和真实进程句柄；本地拒绝启动保留工作区残留路径；Orca 自定义启动路径的初始 shell 关闭错误不再吞掉。移除了 Spec 内重复的 Linux 验收范围段落。

## 验证

- `execution_audit_test.go` 覆盖上述失败路径，包括错误/缺失发送回执、已退出/失联 terminal、换代结果文件、过大结果、观察超时、stop/reply 损坏恢复、取消、准备失败 retry、人工停止恢复和启动模式能力隔离。
- `TestOrcaLostReplyReceiptKeepsOriginalExecutionUnknown` 额外检查同路径不同答案必须改变 Operation 摘要。
- 审计后的真实 Orca Service 启动/输出/停止通过，证据见 [native audit](pr-04a-orca-service-audit-2026-09-24.json)。
- 完整回归、构建和静态检查结果见 [阶段验证报告](pr-04a-execution-backends.md)。

本次不把原生 Orca 证据扩展为自定义 Codex 覆盖模式的验证。race detector 的现有工具链限制和显式 unsupported 的资源释放能力仍见阶段报告。
