# Spec：本机 Orca 原生编排接入单 worker

目标：`WORKER-FLOW-ORCA-NATIVE`。配套 [Plan](worker-flow-orca-native.plan.md)。状态：实施中；已完成平台连续对话、隔离 SQLite 回归和 SteerLine 中心真机双轮问答／重启探针，其他验收项仍待完成。日期：2026-09-29。当前未分配生产原生 Task ID。

## 1. 目标与范围

将本机 `host=orca` 接入 `worker-flow-v1`，使用 Orca 原生 Run、Task、Dispatch、问答和结算协议。一个 SteerLine Execution 保持一个 agent 会话和工作区；用户可在统一待决策入口回答问题或在本轮结算后继续原会话。新 Execution 更换 Orca 支持且本机可用的 agent 时，只改 Execution Profile 的 agent ID，不修改 SteerLine 代码。

本任务承接 [单 worker Spec](worker-flow.spec.md) §4、WF12 中延期的本机 Orca 接入，保留其五个主状态、固定任务契约、唯一业务权威和真实 PR 合并交付。实现交付后仅解除本机 Orca 的首次派发拒绝；受监管远端、同任务多候选、旧五步执行、Orca 自身修复和替换已经丢失的 agent 会话不在本期范围。

沿用 [项目默认规则](../../AGENTS.md)、[原生任务共同约束](native-task-constraints.md)及[第一阶段共同约束](phase1-delivery-constraints.md)。不导入或迁移旧 shell 包装 Codex 的 Orca Execution，不新增旧版兼容、运行中换后端或自动回退分支。发现不支持的已有状态时保留现场并阻止外部副作用。

## 2. 接入决策与资源归属

本期固定使用以下路径，作为本机 Orca 的唯一新接入方式：

```text
worktree create --agent <冻结的 Profile.Agent>
  → run-create / task-create
  → dispatch --task <task_id> --to <原 agent terminal> --inject
  → check / reply / send / worker-read
  → worker_done 结算当前 Dispatch
  → 新 task-create / dispatch --inject 继续同一 agent terminal
```

Orca 负责启动所选 agent、生成并注入带权威身份的 preamble、维护 Task/Dispatch 和消息关系。SteerLine 不自制 preamble、Dispatch capability 或 worker_done，不把 `codex exec resume`、Codex UUID、Codex JSONL 作为 Orca 的协议。

Windows 的 Codex `workspace-write` 进程可能无法连接本机 Orca 命名管道。此时 worker 可在其固定 worktree 内原子写入一份有界的 `worker_done` 请求，包含本轮 SteerLine Run/Turn、Orca Task/Dispatch 身份及原注入 preamble 中的 Dispatch capability。中心只在核对原 agent terminal 和上述全部身份后，代该 worker 调用 Orca `orchestration send`；仍由 Orca 产生并结算 `worker_done`，中心继续从原 Run inbox 核验回执和 Dispatch 状态。中心不把 capability 记入数据库或日志，并在 Orca 接受后删除请求文件。请求写入本身不算交付成功，发送结果不明时不得盲目重发。该通道仅覆盖最终结果，不替代 Orca 原生 question/reply。

Orca 版本匹配文档 `references/low-level-topology.md` 明确：`dispatch --inject` 的 worker 为 `unsupervised`，不创建 supervised terminal resource；`worker-stop`、`worker-abandon`、`worker-retain/release` 不替该路径管理进程。SteerLine 因而承担自己创建的 agent/coordinator terminal 的身份保存、观察、保留、关闭和重启对账。`worker-release` 返回 `retained/no_owned_resource` 不能作为已关闭证明。

这是对通常优先 `worker-start` 的明确选型：本机已复现其 readiness 阻断，而低层原生协议已完成双轮实验。本期不同时实现第二条 supervised 路径；未来修复可用后另行评估资源归属切换，不在现有 Execution 内静默更换。

## 3. 实验情况与证据边界

### 3.1 环境、资料和记录

实验日期为 2026-09-29，环境为 Windows、本机 Orca 1.4.210、后续 Codex CLI 0.159.0，隔离仓库为 `C:/project/SteerlineTest`。完整汇总及各轮身份见 [实验记录 JSON](../validation/worker-flow/orca-local-dispatch-boundary-2026-09-29.json)。该文件为实验观察汇总，不是完整原始命令日志。

已读取安装版本自带的 `orca skills get orchestration` 及 `references/coordinator-loop.md`、`references/worker-contract.md`、`references/messaging-and-gates.md`、`references/recovery-and-cleanup.md`、`references/low-level-topology.md`，并对照 CLI help。公开定位资料为 [Orca orchestration guide](https://github.com/stablyai/orca/blob/main/skill-guides/orchestration.md) 和 [Orca #22825](https://github.com/stablyai/orca/issues/22825)。公开 main 与本机安装版本可能不同，接入命令以实际运行版本能力为准。

### 3.2 已进行的实验

| 实验 | 实际观察 | 能证明和不能证明的内容 |
| --- | --- | --- |
| 早期 supervised Codex 启动 | 曾返回 `outcome_unknown/turn_start_unobserved`；另一次预启动 terminal 的 `worker-start --terminal` 返回 `ready/input_accepted/turn_started`，模型请求随后超时，未产生 worker_done | 曾接受任务或观察到回合启动，未完成 supervised 双轮闭环 |
| shell Dispatch 结算 | 实验 shell 发送成功 worker_done，旧 mailbox 返回 `dispatch_inactive` | 验证 Orca mailbox 生命周期，不作为真实 agent 同会话证据 |
| Codex 网络复测 | 短提示在约 1 分 42 秒重连后返回 `CONNECTED` | 模型连接当时恢复，不证明 worker-start readiness 正常 |
| Codex readiness | 新建及已有 Codex terminal 均出现 `agent_readiness/timeout`；180 秒等待、raw ready marker、`--no-alt-screen` 未消除超时；第一轮真实结算后重试 worker-start 仍失败 | 超时与模型网络故障是独立观察。#22825 报告相似症状，本机内部根因未独立证明 |
| 交互审批 | 手动 `codex` 启动后，运行 Orca 命令出现 Codex 审批；`terminal send` 回答被拒绝为 `agent_prompt_blocked` | Orca ask/reply 不等于宿主命令审批；不能用普通反馈冒充批准 |
| 真实 Codex 原生问答 | 低层 dispatch 注入 preamble；Codex 发出 ask；协调端 reply 原 message ID；Codex 真实发送成功 worker_done，Task/Dispatch completed | 已证明当前活跃 Dispatch 的原生问答及结算 |
| 真实 Codex 同会话续聊 | 同一 terminal handle、同一 incarnation 上新建 Task/Dispatch；第二轮未再次提供第一轮 token，Codex 仍复述 token 并再次发送成功 worker_done；旧 mailbox 拒绝发送 | 已证明低层路径的结算后上下文延续；没有证明 supervised 资源接管 |
| Orca agent 启动器 | `worktree create --agent codex` 返回 agent handle；首次注入为 `inject_rejected/no_agent_detected`，观察后同一 terminal 注入成功；Codex 发出包含 `AGENT_FIRST_OK` 的真实 worker_done，无命令审批阻断 | 已证明可由 Orca 选择 Codex 启动方式；创建回执与屏幕提示符不等于 agent 已可注入 |
| agent 启动器同 worker 双轮 | 原生启动 Codex；首轮注入未识别后在同一 terminal 重试成功，worker_done 结算；第二轮新 Task/Dispatch 不再提供私有 token，Codex 从原会话复述并再次 worker_done | [连续对话记录](../validation/worker-flow/orca-native-continuity-2026-09-29.json)证明所选 agent 启动路径可跨结算复用同一 terminal/incarnation；网络波动和首次未识别仍需处理 |
| 清理回执 | unsupervised release 返回 `no_owned_resource`；显式关闭实验 terminal 成功。worktree rm 曾丢失响应，后续 Orca selector_not_found 且 Git 清单已移除 | 清理必须按实际资源事实对账，不能凭连接异常重做破坏性动作 |

用户明确本机未安装可用 Claude 后，后续实验均只使用 Codex。早先尝试 Claude 的失败记录保留为历史，不属于第二 agent 验收。

### 3.3 成功双轮的可核对身份

共同 Orca Run 为 `run_0b94c448bc6e`；terminal 为 `term_cb3036f4-b110-42d1-a15e-268f20f6fad2`；incarnation 为 `292b3213-61f8-42b3-a776-23d6e4c17189`。两轮始终为 `unsupervised`。

| 事实 | 第一轮 | 第二轮 |
| --- | --- | --- |
| Task | `task_09c6bc554bd8` | `task_b5049678d445` |
| Dispatch | `ctx_c852e8ae0be1` | `ctx_9d338ddec47a` |
| 问题 | `msg_b5dde19fb75a` | 无新问题 |
| 回答 | `msg_9a6fc6baaf5e`，内容 `REPLY_BOUNDARY_OK` | 从上一轮上下文复述同一 token |
| worker_done | `msg_7d89f4cbc8f3`，`FIRST_SETTLED` | `msg_581723e12aad`，`SECOND_SETTLED` |
| Orca 结算 | completed / succeeded | completed / succeeded |

两轮之间的 supervised 复用尝试 `ctx_b955326355f2` 在 `agent_readiness` 失败，没有注入任务，不能计为成功第二轮。实际成功第二轮使用表中另一 Task/Dispatch。

上述双轮用手动启动的 `codex -a never -s danger-full-access` 完成无文件改动的协议实验。另一次 Orca 原生启动器实验为 Run `run_5fec6e0f43af`、Task `task_8338b5c40eb8`、Dispatch `ctx_2b18d674da48`、worker_done `msg_b24c029de152`，确认原生启动路径也能真实结算。实验启动参数不是要求产品自动关闭审批的授权。

实验终端已关闭，测试工作树已从 Orca 和 Git 注销；最新低层实验当时仍留有一个被其他进程占用的空目录 `sl-ready-marker-20260929`，历史 Run/Task/Dispatch 保留。该清理观察不等同于所有历史资源已审计。

### 3.4 尚未验证

SteerLine 中心已有隔离 SQLite／假 Orca 的 ask、重启 reply、worker_done、再建 Task/Dispatch 回归。随后[真实中心探针](../validation/worker-flow/orca-native-center-live-2026-09-29.json)在同一 Execution 中完成原生 ask、中心重启、原问题 reply、第一轮 worker_done 和第二轮记忆验证；探针通过 Service 内部答案路径，没有覆盖公共 UI/CLI。真实 PR 交付、消息与操作回执丢失后的恢复、第二种真实 agent、完整 transcript 可用性、Orca runtime 重启后原会话恢复仍未验证，也未证明首次 `input_accepted` 本身可推出 `turn_started`。这些差异必须出现在实施验收结果中。

## 4. 配置、准备与身份

复用 `host=orca`、Execution Profile 及任务固定配置，不新增通用 agent 插件框架。Profile 的 `agent` 是传给 Orca 的 opaque agent ID；本机是否安装、能否识别和启动由 Orca 的实际回执核实。Local Codex 的专属校验只用于 local 后端，Orca 不调用中心配置的 Codex CLI 来证明能力。

当前 `worktree create --agent` 不暴露按次 model/effort/profile 覆盖。Orca 路径使用 Orca 对该 agent 的启动设置；配置和 UI 明示该来源。显式请求本路径不能兑现的模型、推理强度、Codex profile 或启动权限覆盖时，在启动前明确拒绝，不能默默丢弃或声称已经生效。不得为 Orca 分支硬编码 Codex 启动参数或自动修改 Orca 全局设置。

领取前核对 runtime 可达、当前接口能力、精确已注册仓库和选定 agent 的已知支持事实；最终 agent 身份仍以启动结果核验。保存固定的仓库、base、Spec/Plan 和 Profile 快照，以确定性名称及精确 repo/base 参数创建 Orca worktree 和首个 agent terminal。核对返回的 workspace 身份、路径、分支与初始 base；后续反馈不 reset 工作区，也不要求 HEAD 保持初始值。

复用 Execution/ProcessIdentity/Operation 保存以下 Orca 绑定，具体字段名由实现选择，不能复制一套业务状态：

| 绑定 | 生命周期 |
| --- | --- |
| host、runtime、Orca executable、repo/worktree ID、path/branch、Profile agent | 固定于 Execution；变化需停止副作用并对账 |
| agent terminal handle、process incarnation、启动归属证据 | 原 worker 资源身份；不得按标题或目录猜测替代 terminal |
| coordinator terminal、Orca Run ID | 本 Execution 的 mailbox 和消费身份；中心服务重启时重用已证明的原绑定 |
| 每轮 Orca Task/Dispatch、中心 turn ID、输入 operation/request ID | 每轮追加；新 Dispatch 原子成为当前目标，旧映射保留历史 |
| question/message/Delivery ID、回答 operation、输出 cursor/source | 绑定所属 Dispatch，按已持久化事实去重和恢复 |

Orca Run/Task 与 SteerLine WorkflowRun/Task 是不同命名空间。一次原生任务对应一个中心 Execution，多轮 Orca Task 不产生新的中心任务、DAG 节点或候选。

可用的 provider session identity 作为平台提供的附加核验事实保存，缺少时不伪造 Codex session ID。终端相同不构成任意进程重启后同会话恢复的承诺；本期只自动延续经核实仍存活的原 agent。若发现 agent/session 被替换，或 Orca runtime/handle/incarnation 变更且缺少平台可验证的原会话关联，保留反馈并进入待决策，不启动替代会话。

## 5. 首轮派发与启动观察

为本 Execution 创建最小 coordinator shell terminal 作为 Orca Run 消费者，保存精确身份；它不是第二个智能 worker。通过 `run-create`、`task-create` 和 `dispatch --inject` 注入包含固定任务契约的自包含 Task spec，由 Orca 生成 preamble。

启动状态必须区分：资源已创建、agent 可识别、输入已接受、观察到 agent 处理当前 Dispatch、Dispatch 已结算。`input_accepted` 不直接推进成“本轮已开始”；使用对应平台启动回执或当前 Dispatch 的可信 worker 活动/问题/完成证明推进。快速完成可一次收敛开始与结束事实。

`inject_rejected/no_agent_detected` 且确认本次没有创建 Dispatch/接受输入时，在原 terminal 上按已有超时配置有界观察后重试；不创建另一 terminal。未知回执走 §8，不按明确 preflight 拒绝处理。到期仍不可注入则进入统一待决策并保留资源身份。不得用固定睡眠、伪造 ready marker、可见提示符或忽略所有错误来宣告成功。

Task 文本必须保留中文、换行、引号、任意反馈和完整 Spec/Plan 引用。若 Windows 命令行长度或 Orca payload 限制不足，必须明确拒绝或采用已证实的 Orca 原生传输能力；不得退回 shell 分块拼接 Codex exec，也不得静默截断。

## 6. 交互、结算和业务状态

| 输入或事件 | 原生路径及中心处理 |
| --- | --- |
| 当前 Dispatch 的 question | `check` 后持久化原始问题、选项及来源；进入 waiting_decision |
| 回答这个 question | 对原 message ID `reply`；仍为原 Dispatch、原中心工作轮次，不创建新 Task。按回复被接受与后续活动的真实事实恢复观察 |
| 当前 Dispatch 的主动补充 | `send --to dispatch:<当前 ID>`；入队成功仅显示已接受，不声称 worker 已读或已落实；遵循原生 preamble 的检查点读取机制 |
| 当前 Dispatch 的 worker_done | 同时核对来源、Run/Task/Dispatch 和 Orca 实际结算；成功/失败都结束当前轮。无有效 PR 时 waiting_decision，有已核实未合并 PR 时 waiting_pr_merge |
| 已结算后的用户反馈 | 新建 Orca Task/Dispatch，注入同一核实 terminal；中心 Execution、会话和 worktree 不变。新轮次绑定后才允许后续消息投向新 Dispatch |
| 宿主命令审批/登录 | 展示平台入口和实际阻断；`agent_prompt_blocked` 不能转换成 Orca question 的已回答或代理点击批准 |
| 超时、断联或缺少输出 | 保存未核实事实并继续可行对账；不认定 agent 停止或 Dispatch 完成 |

收到待消费 Delivery 后，先将全部消息、Decision、操作意图和结算事实持久化，再按原 Delivery ID ack；重放不重复创建 Decision、发回答或推进轮次。需等待用户回答的问题可在持久化后 ack，回答由原 message ID 后续提交，不能长期阻塞同一批其他消息。

worker 的 `ask` 超时或连接中断不取消原问题；继续等待使用 Orca 的原 message ID `ask --resume`，中心仍回复这个问题，不创建重复问题或合成答案。该恢复约定沿用 Orca preamble，不增加 SteerLine 专属 worker 问答格式。

agent 异常结束但没有 worker_done 时，保存平台证明的退出/失败事实并对账原 Dispatch；不能把 PTY 存活当作 agent 仍在工作，也不能因静默主动结算。旧 Dispatch 仍活跃或结果不明时不得创建下一轮。原 agent 已退出且无法证明恢复原会话时，保留反馈并显示不可投递。

结算与补充输入发生竞争时，根据固定 Dispatch 和 Orca 实际回执对账。明确 `dispatch_inactive` 且证实未投递的反馈可在旧轮已结算后用同一中心输入操作创建一次新 Task/Dispatch；若旧 send 已接受或结果未知，不自动复制到新轮，保留真实投递事实供观察或决策。回答过期问题不得转发到另一个 question。

旧/重复 worker_done 不回退新轮次；worker_done 只代表本轮结算，不关闭中心 Task。PR 线索仍由已有 provider 核验仓库、base、执行分支及真实 merged 状态，复用唯一 TaskDelivery 事务。worker 的成功报告、用户说“完成”和普通 PR closed 均不能替代合并证明。

## 7. 输出、资源保留与关闭

通过 `worker-read --source auto` 读取会话输出，保存 source/cursor、clipping、fallbackReason 与完整性事实；`source_changed` 时按 Orca 指引重新读取并去重。只得到 terminal tail 时明确展示有界输出，不宣称完整 transcript，不要求中心读取 Codex 私有 JSONL。

正常 worker_done 后保留原 agent terminal 和 worktree，用于等待反馈或合并。用户选择持续 worker 流程即表达本流程期间保留会话的意图；每轮不运行 unsupervised `worker-release` 来伪造资源转移。

中心服务停止观察不等于关闭 worker。最终交付或明确丢弃时，保存清理意图并核实资源仍由本 Execution 创建/持有、身份一致且无新工作或用户接管，再对精确 terminal 执行 `terminal close`。活跃或未知 worker 不因 PR 已合并就被描述为已停止；保留资源事实，按已有明确停止授权和实际回执处理。中心 Task 的唯一交付不依赖清理已经完成。

coordinator 在没有待处理 inbox/回答或清理操作后才关闭。不得批量关闭工作区内配置产生的其他 terminal，不删除含有工作成果的 worktree。关闭超时先对账，无法证明身份或停止时保留清理未核实记录，不扩大关闭范围。

## 8. 幂等与重启恢复

沿用中心 SQLite 单写和 Operation。外部调用前保存固定意图，记录原始 Orca request ID 和回执；支持原请求恢复的 orchestration mutation 使用 `request-show` 与对应 `--retry-request`。必须按安装版本确认 ID 生成/返回语义；不能假设 worktree/terminal create 也支持 orchestration 的重试参数。

| 响应丢失位置 | 恢复要求 |
| --- | --- |
| worktree/agent/coordinator create | 查精确创建身份、repo/base 和资源事实；证明唯一归属后接回。名称相同不是充分证据；无法证明则停止新建，保留未知意图 |
| run/task create、dispatch 注入 | 原 request receipt 与 Run/Task/Dispatch 对账；已接受不重复注入。没有 request ID 也不能假定没有生效 |
| check / Delivery ack | 已入库消息按 message/Delivery 去重，恢复原消费进度；不漏批中其他消息 |
| reply / send | 原问题、答案和 request receipt 对账；平台接受与 worker 消费分别记录，不要求业务答案应用证明 |
| worker_done 后、反馈启动中 | 重建当前轮及历史 Dispatch 映射，一次反馈最多创建一个有效新轮；旧完成消息不能覆盖新轮 |
| terminal close | 核实原 terminal/incarnation 的关闭或存活事实，不按新句柄关闭另一个进程 |

中心服务在当前格式下重启，必须接回仍存活的 coordinator、Run、agent 和未完成操作；不得重新领取中心 Task。Orca runtime 自身重启后的会话恢复不在本期自动承诺内，身份不可证明时必须保留现场并阻止发送/关闭。有限次无输出不是恢复失败或进程退出证明。

## 9. 验收与交付边界

| 编号 | 必须验证的结果 |
| --- | --- |
| ON1 | 本机 Orca Profile 可进入 worker-flow；非 Codex agent ID 原样交给 Orca；不支持的覆盖配置在副作用前拒绝；冻结配置不受后续 Profile 修改影响 |
| ON2 | 正式中心路径用 Orca agent 启动器和原生 Dispatch；无 Codex exec/resume、UUID/JSONL 硬依赖；输入仅接受不被误记为开始 |
| ON3 | 当前 Dispatch 的 ask 在统一入口展示；纯文字、选项＋备注完整 reply 同一 message ID，仍为原轮次；重复提交不重复发送 |
| ON4 | 中心接入下完成两轮真实 Codex 任务：第一轮 ask/reply/done，结算后反馈建新 Task/Dispatch，同一会话记得上一轮信息并再次 done；中间工作区及提交不被重置 |
| ON5 | 活跃 send、过期 question、旧 Dispatch inactive、send/done 竞争按 §6 处理；迟到事件不串线、不重复注入 |
| ON6 | 当前格式中心重启及 §8 的回执丢失点以针对性故障测试对账；至少一次真实中心重启保留原会话和待回答问题，恢复后完成 reply 与续聊 |
| ON7 | 身份变化、Orca 断联、agent 未识别、审批阻断、旧状态和首次创建结果不明时保留反馈与现场，不创建替代 worker，不误关闭其他 terminal |
| ON8 | reader 的有界输出、source_changed、重复读取及恢复正确展示；没有完整 transcript 时不声称完整 |
| ON9 | 已结算资源保留至反馈/交付；unsupervised cleanup 用精确 terminal close 并对账；服务停止观察不误杀 agent，worktree 内容保留 |
| ON10 | 隔离 SQLite/provider 回归验证 PR 归属、关闭未合并后续聊、真实 merge 唯一交付和 DAG 解锁；另做一条隔离仓库真实 PR 链，合并由用户操作 |
| ON11 | 用不同 agent ID 的契约测试证明无需改代码；第二种真实 agent 安装可用后再做同路径验收。目前仅 Codex 可用，跨 agent 真机项标为未验证，不宣称全 agent 已验证 |

实现 PR 至少通过 ON1–ON9、ON10 的自动化部分、ON11 的 agent 参数契约测试及 Codex 中心真机闭环。ON10 的人工真实合并和 ON11 第二 agent 真机验证单列为发布前/能力认证证据；缺少对应环境或用户合并不能伪造通过，也不要求安装 Claude 来完成本期 Codex 集成。

按当前项目规则只做最小必要验证：复用已覆盖的身份、交付和恢复测试，补充本次 Orca 接入改变的边界，运行现有 `test-feedback -quick` 入口及受影响用例；不将全量 Go 回归、全仓 vet 和双平台构建设为本期默认门槛。只有具体失败或未解决的跨模块风险才扩大验证，确需全量时使用现有三进程入口。新增证据写入 `docs/validation/worker-flow/`，逐项记录 ON 编号、构建/平台版本、Execution/Orca 身份、真实结果及未验证项。本 Spec 的平台实验不能替代正式中心实现验收。
