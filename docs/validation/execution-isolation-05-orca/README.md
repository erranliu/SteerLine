# #58 Orca 执行适配器迁移验收

基线：`6e728cfda1f2aeaf9973f145f550b406ca4bf96f`（PR170 合并提交）。开工前已 fetch origin/main，并确认 HEAD 与 origin/main 均包含此提交。交付依据：[Spec](../../spec/execution-isolation-05-orca.spec.md) / [Plan](../../spec/execution-isolation-05-orca.plan.md)。本轮范围止于 #58。

## 最终所有者

| 责任 | 唯一实现与中心消费者 |
| --- | --- |
| 本机 CLI envelope、原生 terminal 状态、观察串行门槛、版本探测及停止回执 | orcaworker client/backend/config；中心 orca.go 仅组装固定配置与受管进程 Runner，execution_backend 选择对应实现 |
| 原 worktree/agent/coordinator、terminal/incarnation、saved receipt、原 Task/Dispatch 对账、等待及精确清理核验 | orcaworker binding/workspace/mutation/inventory；中心 prepare/dispatch/terminal 文件保存每步意图和部分绑定后才进入下一外部动作 |
| Delivery 来源、消息/问题、工具/收据、输入资格、HostFact、结算、展示内容和 relay 安全文件 | orcaworker event/events/output/environment/readiness/relay；中心消费类型化事实，不解码 native envelope 或读取 terminal tail 判定结果 |
| Codex model/effort/sandbox、Git write roots、工作目录及完整 executable/argv | 复用 #57 codexworker.ManagedCommand 和 LinkedWorktreeWriteArgs；#55 workspace 核验实际 Git 资源；orcaworker 只对完整 argv 做 PowerShell 包装，删除包装后追加 Codex 参数入口 |
| 远程静态配置、版本族、backend、启动/续聊身份、消息/ack/结算/release | 已独立的 orca_remote_worker；中心 remote_adapter 只组装固定配置，remote_service 保留事务；远程不生成中心 Codex 命令，路径按远端原值交接 |
| 准备预算/nonce/deadline、Ready/Invalidated、准备与业务轮分流、HostTarget、DeferredDecision/Feedback、人工恢复门槛 | 中心 worker_environment/worker_start/worker_resume/worker_recovery；共享持久值仍由 contracts 唯一定义 |
| PR 查询/绑定、Operation、内容/消费持久化、TaskDelivery 和真实合并结项 | 中心原有事务与 #56 publication；ack 在消息及消费事实提交之后，交付不等待 terminal/release 清理成功 |

业务身份是 Task/WorkflowRun/Execution/逻辑 Turn 与代次；适配器接收固定 target、请求及原资源绑定；平台身份分别为 runtime、repository/worktree、terminal/incarnation、coordinator、Run、Task、Dispatch、Delivery。中心保存它们的关联，平台核验实际来源，不将平台 ID 互相替代或当作 Codex UUID。

未知创建/输入继续对账原意图，不能另建 terminal 或重复发送；新 Dispatch 回执独立保存，Attempt 初始 session 绑定保持不可变。准备不写业务 CurrentTurn/计数或 PR；业务派发仍要求工具、原子收据、原 Dispatch 结算、期限有效且无 blocker。宿主选项沿用可信 HostTarget，文字反馈保持原问题/worker 接收方；重查后再比较原 Decision 版本和 blocker，不能清除并发新故障。

远程仍使用原生 worker-start 和原 terminal/incarnation，中心 inbox/coordinator 与远端执行现场分别核验。远程 worktree 保留，取消/停止远端仍 unsupported；停止观察不等于远端退出。

## 已确认基线失败的修复

在未修改的 #57 合并基线上运行 `TestWorkerEnvironmentER1ProbeAndSettlementGate/success`，初次 poll 复现 `native Orca poll has no fixed execution`。在准备完成处检查原 blocker 后，发现资源准备更早已因 `git symbolic-ref --quiet --short HEAD` 失败而被保守阻断。

fixture 把 worktree 的分支报告为 `fake/sl-...`，实际却用 `git worktree add --detach` 创建。#55/#57 的真实 Git 资源核验拒绝这种身份不一致，因此尚未生成 native execution；poll 的错误只是后续表现。本轮改为 fixture 用 `-b state.Branch` 创建真实分支，并在 ER1 进入 poll 前断言资源绑定已完成。生产身份核验、Git 写入根检查、准备状态机及成功判定均未弱化。

修复后的 ER1 五个子例（success/tool_failure/oral/missing_receipt/failed_settlement）均通过。success 仍要求两个 Dispatch、独立准备/业务轮、Ready 与业务计数门槛；失败场景不能派发业务。

## 自审与验证

- 自审补齐原 coordinator 身份在 Dispatch 等待重试中的检查；远程 ack/replay 改用远程 Client 的原 coordinator/Run 核验。创建、发送、ack 与关闭操作继续先保存意图，再提交回执。
- 自审修复 terminal show 的有效 JSON 掩盖传输失败问题。传输失败现在返回观察错误；`TestTerminalObservationRejectsFailedTransportWithValidPayload` 覆盖该边界，不把观察失败推定为退出。
- 观察门槛泄漏断言移至拥有该门槛的 orcaworker；`TestBoundTerminalGateReleasesCanceledWaiter` 覆盖等待取消和精确释放。原中心实际进程并发用例继续运行。
- `go test ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker ./internal/steerline/codexworker -count=1`：通过。另用完整 argv 顺序和 PowerShell 特殊字符测试核验命令交接。
- `go list -deps ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker`：通过；传递项目依赖仅 execution、contracts、task/taskgraph/taskview 和两个 Orca 包，无中心、SQL 或 codexworker。
- Plan 九项中心用例全部命中并通过，包含 ER1 五个子例、原资源复用、旧消息/替换 incarnation 拒绝、远程两轮/丢失回复/ack、替换 session 拒绝，以及原期限与重查并发门槛。
- 补充九项中心定向用例通过：RemoteOrcaLostStartReconcilesAfterRestartWithoutResend、SupervisedOrcaClientPinsLaunchAndReadsExactDispatch、SupervisedOrcaExecutionBindingRequiresExactDispatchIdentity、SettledSupervisedDispatchRetainsExactBindingWithoutLiveTerminal、NativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart、OrcaSameRefObservationsDoNotOverlapOrLeakGate、OrcaStopExecutionConfirmsFixedTerminalExit、OrcaStopFailureDoesNotConfirmExit、NativeOrcaDoneRequestBindsCurrentTurn（均加 Test 前缀）。与 Plan 合计 18 项，该批 202.653s。
- 最终代码再次运行 Plan 原始九项正则：全部通过（166.247s）；`-list` 核对九个名字全部命中。`go test ./... -run '^$' -count=1` 全仓编译/链接通过，无测试执行；`git diff --check` 与 staged 检查通过。提交前再次 fetch，最新 origin/main 仍为上述基线。

原 `TestOrcaVersionFamilyUsesMajorMinorOnly` 移至 orca_remote_worker；其余 Plan 指定测试名保留并调用迁入的新包。未用未命中正则、旧 journal 测试或跳过失败作为通过证据。

## 交接边界

#59/#60 接收已完整接线的两类 Orca 后端。本轮保留中心配置组装、业务意图/绑定/回执附件提交、WorkerEnvironment 与恢复授权；这些是其业务消费者的事务接线，并非剩余平台解析。后续项目权限/发布策略按各自 Spec 实施。

未增加旧格式迁移、双写或跨后端回退。完整启动命令的序列化和权限来源保持既有含义；本轮未做双机真机认证或全量测试，Service/协议 fixture 结果不代表双机认证。

## 协调 session 独立复审

Plan 九项中心回归由协调 session 独立运行全部通过（162.383s），包括 ER1 全部五个子例；三个适配器包及传递依赖检查通过。

复审发现 CLI 超时/取消附带的拒绝 JSON 仍可被 `NoAgentDetected` 或 `FeedbackDispatchInactive` 当作确定拒绝，导致未知动作被允许重试。现在 MetaCall 保留传输不确定事实，两个拒绝判断不再接受它；正常完成的 CLI 非零退出仍可保留有效平台拒绝。定向用例修复前失败、修复后通过，覆盖注入与反馈两个边界及正常平台拒绝；未知动作仍按原请求对账，不产生新的 Dispatch。
