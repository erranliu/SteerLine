# Orca 原生编排接入单 worker 流程

本文为 2026-09-29 的调查与讨论记录。正式实施依据已整理为 [Spec](../../spec/worker-flow-orca-native.spec.md) 和 [Plan](../../spec/worker-flow-orca-native.plan.md)；后续实现以该 Spec/Plan 为准，本文保留方案形成过程。

## 已核实的边界

- Orca `ask/reply` 服务于活跃 Dispatch 的阻塞问题；活跃 Dispatch 的非阻塞补充指令走 `orchestration send --to dispatch:<id>`，成功回执只证明入队。
- `worker_done` 结算当前 Task/Dispatch。agent 回到空闲提示符，不再读取旧 Dispatch 的 mailbox。
- 后续任务沿用同一会话时，先从旧 Dispatch 的 `worker-show` 核实 `worker.agentTerminalHandle`，再以新 Task 调用 `worker-start --task <new> --terminal <same_handle> --worktree <exact_worktree>`。新 Dispatch 接管 terminal 的资源归属；旧 Task/Dispatch ID 只保留为历史。
- `worker-start --terminal` 不接受 `--model/--effort`。后续轮次沿用已启动 agent；切换 agent 只影响新 Execution。
- Orca 的启动模式由用户设置决定，部分 worker 没有可供复用的 terminal。单 worker 的“原会话继续”承诺必须在启动回执和 `worker-show` 中确认 terminal handle、agent 身份、runtime、incarnation、worktree；无法证明时停止后续派发并保留现场。
- 在 Orca supervised 路径，每次接受 `worker_done` 后都要决定 terminal 的归属：新 Dispatch、用户要求的 `worker-retain`、或 `worker-release`。当前低层路径由 SteerLine 自己持久化“等待反馈需保留 terminal”的工作意图，任务最终结束后关闭自己创建且身份仍匹配的 terminal。
- 本机 `worker-start` 的 Codex `agent_readiness` 判定会在可见空闲提示符上超时。Orca 文档另有 `task-create` + `dispatch --inject` 路径：它保留原生 Task/Dispatch、`ask/reply`、`worker_done`，但 worker 被标为 `unsupervised`，Orca 不接管终端的停止和释放。若采用这条路径，SteerLine 必须成为终端的明确资源所有者。

本机 Orca 1.4.210 的版本匹配文档：`orca skills get orchestration`、`references/coordinator-loop.md`、`references/recovery-and-cleanup.md`、`references/low-level-topology.md`。公开源码对应 [orchestration guide](https://github.com/stablyai/orca/blob/main/skill-guides/orchestration.md) 和 [dispatch preamble](https://github.com/stablyai/orca/blob/main/src/main/runtime/orchestration/preamble.ts)。[Orca #22825](https://github.com/stablyai/orca/issues/22825) 报告了相同形态的 Codex 空闲提示符与 `agent_readiness` 超时；本机症状相符，具体内部原因尚未在本机证明。

## 当前可执行的接入选择

建议先用 **Orca 原生 Task/Dispatch + SteerLine 托管终端** 完成当前接入：

1. 首轮通过 Orca `worktree create --agent <Profile.Agent>` 启动选定 agent，并核实返回的唯一 agent terminal、agent 身份、worktree 和进程 incarnation；每种 agent 的实际启动命令由 Orca 决定。低层路径不提供 `worker-start` 的按次 `--model/--effort`，只能使用 Orca 对该 agent 的默认设置；需要按次指定模型时应先补 Orca 能力或修复 readiness，不能把 Codex 参数写死进 SteerLine。
2. 创建 Run/Task，再用 `dispatch --task <id> --to <已核实 handle> --inject` 派发。问题用 Orca `ask/reply`，进行中的补充用 `send --to dispatch:<id>`，完成只认精确 Dispatch 的 `worker_done`。
3. 结算后的反馈创建新 Task，重新 `dispatch --inject` 到同一 handle，并验证 incarnation/worktree 未变。旧 Dispatch mailbox 失效。终结时由 SteerLine 对**自己创建且仍匹配身份**的 terminal 执行 `terminal close`；`worker-release` 对这种 `unsupervised` worker 返回 `no_owned_resource`，不能代替关闭进程。
4. 持久化每次 Orca mutation request ID、Run/Task/Dispatch、Delivery、terminal/worktree/incarnation。重启先以 Orca 当前状态对账；连接中断或身份不明时阻止新派发和关闭，等待可证明的状态。PR 的真实合并核验仍由现有 provider 决定。

这保持了 Orca 原生问答与编排协议，且更换新 Execution 的 agent 只需改 Profile。代价是 SteerLine 要承担终端生命周期和恢复对账。待 Orca 修复 `worker-start` 的 Codex readiness 后，可改用其 supervised 资源管理；两种方式都使用相同的 Run/Task/Dispatch 消息语义，但资源归属不同，不应在同一 Execution 中默默切换。

## 单 worker 执行路径

1. **领取前**：沿用 SteerLine native Task、固定 Spec/Plan、Execution Profile 和 PR provider。校验 Orca runtime、版本/能力、仓库注册和所选 `agent`；不依赖本机 Codex CLI，也不在 Orca 故障时回退本地执行。
2. **创建工作区与 Run**：按已冻结的仓库、基线 commit 和确定性名称，用 Orca 创建或核实 worktree，保存 Orca worktree ID、实际 path/branch/base。为本次 workflow 创建 coordinator terminal 与 `orchestration run-create`，持久化 center runtime、coordinator handle、Run ID 和原始 request ID。任何不明回执先按精确身份对账。
3. **首轮**：当前本机使用 `worktree create --agent <Profile.Agent>` + `task-create` + `dispatch --inject`。保存 Orca Task/Dispatch、原始 request ID、agent terminal handle、incarnation 和 execution host；核实实际 agent/worktree 与 Profile/Execution 一致。`worker-start --agent` 是 readiness 修复后的 supervised 目标路径。
4. **活跃交互**：coordinator 用 `check` 消费 Delivery；先持久化原始消息与 Decision，再 ack。worker 的 `ask` 映射为当前 Decision，用户答案对同一 message ID 调 `reply`。用户主动补充指令发往当前 `dispatch:<id>`，并以 worker 后续 `check`/结果确认是否被处理。`worker-read --source auto` 用于会话展示，`worker-show`/fleet 用于身份与存活判定。
5. **结算与反馈**：只接受当前精确 Task/Dispatch 且 Orca 已结算的 `worker_done`。中心照旧查询 PR provider，核实仓库、base、worker branch 和真实 merge commit。若用户接下来给反馈，创建新 Orca Task，以 `dispatch --inject` 投递到经重新核实的同一 terminal；旧 ID 不再作为发送目标。等待用户反馈期间持久化保留 terminal 的工作意图；任务交付或明确放弃后，按 SteerLine 持有的资源身份关闭 terminal。Supervised 路径恢复可用后，复用则改为 `worker-start --terminal`，终结改为 `worker-release`。
6. **重启恢复**：从持久化的 Run/Task/Dispatch、request ID、terminal/incarnation、worktree 重新读取 Orca `run-show`/`worker-show`/mailbox。先对账再继续观察、回复、复用或释放。请求超时和 `unverifiable` 均不意味着进程已退出；不生成替代 session，不重复投递反馈。

## 代码改动范围

| 位置 | 所需改动 |
| --- | --- |
| `config.go`、`execution_profiles.go` | 开放 `worker-flow-v1` 的本地 `host=orca`；移除 Orca 对 Codex 的硬编码要求。当前低层路径按 agent 使用 Orca 默认模型，明确拒绝不能兑现的按次 model/effort/profile 覆盖；已冻结 Profile 继续支配在途 Execution。 |
| `worker_flow_local.go`、`worker_flow_orca_terminal.go` | 保留本地 Codex 路径；Orca worker-flow 改用原生 Run/Task/Dispatch API，停止以 shell terminal 包装 `codex exec resume`。Orca 工作区和 agent terminal 由 Orca 创建、SteerLine 以精确身份托管。 |
| `worker_flow_service.go`、`worker_flow_state.go`、`model.go` | 稳定会话身份改为后端相关的已核实绑定；Orca 用 terminal handle + runtime + incarnation + worktree，当前 Dispatch 每轮更新。`Begin/StopWorkerTurn` 与重启恢复按 Orca `worker_done` 和新 Dispatch 转换，不要求 Codex UUID/JSONL。 |
| `decision.go`、会话读取 | 活跃问题调用 Orca `reply`；已结算反馈创建新 Task/Dispatch 并复用 terminal。Decision 的幂等命令 ID 映射到 Orca 原始 request ID；展示通过 `worker-read`，保留来源和缺口标记。 |
| `orca_supervised.go` | 复用已有 Run、Task/Dispatch、`worker-show/read`、Delivery 入库/ack 的严格回执解析和身份校验；新增低层派发与 SteerLine 持有的终端关闭对账，不把 `worker-release` 当作 unsupervised 进程清理。远端五步契约不直接套入单 worker。 |
| 文档与测试 | 更新执行 Profile 文档和 UI 能力提示；加入 Orca 回执契约测试、重启/幂等测试和真实同 terminal 双 Dispatch 验收。 |

## 必须通过的验收

1. 首轮真实 agent 产生 `worker_done`；`worker-show` 证明 settled Dispatch 的 terminal handle、runtime、incarnation 和 worktree。
2. 同一 handle 上新建**不同的 Task/Dispatch**，第二轮真实 agent 能看到上一轮上下文、处理反馈并再次 `worker_done`；旧 Dispatch 接收补充消息应返回 inactive。当前以 `dispatch --inject` 验收；`worker-start --terminal` 另列为 Orca readiness 修复后的 supervised 验收。
3. 活跃 `ask/reply`、主动 `send`、结算后反馈分别走各自路径；问题及答案在中心重启后可按原 Delivery/请求 ID 对账。
4. 在 worktree/terminal 创建、Run/Task 创建、Dispatch 注入已接受、首轮结算、复用注入、Delivery ack 和 terminal close 的响应丢失点注入重启，不重复启动 agent、不覆盖可能仍活跃的外部执行。
5. agent terminal 缺失、runtime/incarnation/worktree 改变、Orca 断联、`turn_start_unobserved`、PR 错配或未合并均阻止自动结项与替代 session；仅 provider 的真实 merged 状态完成 native Task。
6. 在有第二种 agent 可用的环境中验证新 Execution 的切换只改 Profile；在途 Execution 保持原 agent 和会话。当前低层路径使用 Orca 默认模型；若恢复 supervised `worker-start`，新终端的 model/effort 才以 `launch.effective` 核验，复用时不传这两个参数。

## 当前真机结果

见 [边界测试记录](orca-local-dispatch-boundary-2026-09-29.json)。先前 `worker-start --terminal` 曾返回 `ready/input_accepted/turn_started`，但 Codex 请求超时，未结算。Codex 0.159.0 的后续重试中，`worker-start` 对可见空闲提示符反复在 `agent_readiness/timeout` 失败；180 秒等待、原始输出加 ready marker、`--no-alt-screen` 均未消除超时。

本次改用 Orca 文档的低层 `dispatch --inject`，在隔离工作树里完成了真实 Codex 双轮实验：第一轮 Codex `ask`，协调端按同一 message ID `reply`，Codex 发送 `worker_done succeeded`；第二轮对**同一 terminal handle 和 incarnation**创建新 Task/Dispatch，Codex 凭同一会话记住第一轮回答并再次发送 `worker_done succeeded`。旧 mailbox 返回 `dispatch_inactive`。两轮的 Orca worker 状态都为 `unsupervised`，因此证明了原生问答、Dispatch 结算和会话延续，却没有证明 `worker-start` 的 supervised 资源托管可用。实验 terminal 已关闭，隔离工作树已从 Orca/Git 移除。

另用 Orca `worktree create --agent codex` 而非手动 Codex 命令验证正式启动方式：返回 `agentTerminalHandle`，启动初期一次 `dispatch --inject` 得到 `inject_rejected/no_agent_detected`；继续观察后**同一 handle**成功接受注入，Codex 无命令审批阻断并发送真实 `worker_done succeeded`。因此实现应观察 agent 识别/启动状态，对明确的启动中状态作有界等待；不可把 `inject_rejected` 当成已投递，也不可凭可见提示符认定 readiness。
