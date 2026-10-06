# Spec：本机 Codex app-server 执行迁移

适用说明（2026-09-30）：服务退出后 worker 必须独立存活、重开自动接续旧输入的规定已由[停服恢复 Spec](worker-service-recovery.spec.md)替代。下文 2026-09-29 的 A/B 验收保留为当时宿主的历史事实；本机受限 Job 的新闭环见[本次验证](../validation/worker-service-recovery/README.md)。

状态：实现及 A/B/C 验收已完成；仅定向 Go race detector 因当前工具链限制留待补验。配套 [Plan](local-codex-app-server.plan.md)。

## 1. 范围与依据

将 `worker-flow-v1` 的本机 Codex 执行从 `codex exec` / `exec resume` 替换为 app-server，支持原生轮次、运行中问答和结束后反馈。沿用 [worker-flow](../archive/spec/completed/worker-flow.spec.md)、[人工并行](../archive/spec/completed/manual-parallel-start.spec.md)及[会话事件](../archive/spec/completed/pr-04b-conversation-events.spec.md)的业务契约；本 Spec 优先规定本地执行、问答投递和恢复方式。

SQLite 继续是唯一业务权威，复用 Execution、Decision、Command、Operation 和会话存储。领取、固定工作区、PR 归属核验、真实合并与唯一 TaskDelivery 事务继续生效。Orca 接入、自动并行调度及新的审批界面不属于本次范围。

新本地执行协议为 `local_codex_app_server_v1`，业务协议仍为 `worker-flow-v1`。按 [AGENTS.md](../../AGENTS.md) 拒绝不支持的旧状态并保留现场，不建设旧数据迁移、双写或 `exec` 回退路径。

## 2. 并行与会话生命周期

每个 Execution 独立运行 app-server，绑定实际原生服务进程、原生 thread、固定 worktree、`CODEX_HOME` 和冻结配置。npm 等命令包装器须解析为同版本原生可执行文件，进程身份不能只绑定包装器。不同任务可以人工并行，同一任务只有一个权威活动 WorkflowRun；人工并行不设项目级数量上限，自动派发沿用单活动执行策略。

RPC、观察和恢复按 Execution 独立进行，结果携带运行、原生身份和宿主代次交给现有中心单写者提交。慢请求、待答或断线不能阻塞其他任务的观察、反馈及 PR 核验。异步代码不得依赖共享的“当前 worker / 当前配置”。

服务停止观察后，正在运行或等待回答的 Codex turn 及其请求须继续存活；重启能重接原执行。采用每 Execution 独立的 loopback WebSocket app-server 进程；真机已验证断开、重接并回复原请求后，原 turn 可完成。只有确认 turn 已结束且无待答请求，才可关闭 app-server 进程，后续用原 thread ID 恢复。

## 3. 原生身份与事件

首次 `thread/start` 前保存 Operation 意图；返回的 thread 身份和工作区经核验并落盘后才发送 `turn/start`。恢复使用固定 thread ID。每次输入先保存 Command、完整内容及目标，再执行外部发送；SteerLine 的 Command ID 与 Codex 返回的 turn ID 分开记录。

模型、推理强度、profile、sandbox 和 linked-worktree 的 Git 写入权限按冻结配置映射，并在首次派发前核验所需能力。本地非交互 worker 显式设置 `approvalPolicy=never`；仍收到的原生审批或不支持的宿主请求须明确拒绝，不自动批准，也不由普通任务反馈代替授权。

复用现有 app-server 历史读取和会话存储，合并实时通知与历史补读；事件、正文与游标按现有契约持久化、去重及有界读取。`item/completed` 只结束该 item；原生 `turn/completed` 的状态区分完成、中断和失败。进程退出、静默、问号或最后一条消息不能证明轮次结束或正在等待回答。

## 4. 统一人工决策

两类交互均通过现有 Decision 和 `answer_decision` 提交，沿用版本校验、first-wins、自由文字及“选项＋备注”。来源决定投递方式，界面提示其实际效果。

| 来源 | 提示 | 投递与状态 |
| --- | --- | --- |
| `in_turn_request`：收到 `steerline_ask` 的原生 `item/tool/call` | Codex 正在等待回答，提交后继续当前轮次 | 回复原请求；WorkflowRun 为 `waiting_decision`，Execution 和 turn 仍 active；回答被接受、原 turn 仍在运行且无其他待答请求时回到 `executing` |
| `post_turn_feedback`：原生轮次已结束 | 本轮已结束，提交后在原会话开始下一轮 | 在原 thread 上调用 `turn/start`；开放 PR 等待期间的反馈使用同一路径 |

当前 Codex Default 模式实测不提供内置 `request_user_input`，因此在 `thread/start.dynamicTools` 注册 `steerline_ask`，由 app-server 的原生工具请求/回复保持当前 turn。每条请求按 app-server 进程代次、thread、turn、call ID、JSON-RPC request ID 绑定 Decision；多个请求不得互相覆盖。一条请求内的多题按 question ID 保存完整答案。请求已失效或 turn 已结束时，旧答案不能改投新轮次；保留输入及原因，按原操作对账。等待人工输入不自动选择默认答案。

答案保存和平台接受分别记录。工具请求的清除或 `serverRequest/resolved` 不能单凭通知认定答案已送达；须结合原请求回复与匹配的工具结果对账。只有与本次发送匹配的原生证据才能推进送达状态；证据不足保持未知。真实合并或 Task 丢弃优先于尚未投递的反馈，迟到事件不能重开终态任务。

## 5. 恢复与幂等

外部操作先保存意图、后保存回执。重复 Command 返回已有事实；JSON-RPC ID 不视为外部幂等保证。发送后超时或崩溃，按原 Operation、传输回执及固定 thread 历史对账；无法唯一确认时保留未知，不重发、不换会话、不清理可能仍在运行的现场。

服务重启对所有未结算 Execution 完成一轮有界核验，再受理新人工领取。可核实的原执行继续观察；单个执行无法核实时记录未知，不无限等待或阻塞其他任务。待答请求只在原连接/请求身份仍有效时恢复投递；确认空闲的会话可用原 thread 恢复，活动或停止状态不明时禁止启动替代执行。

当前格式内的恢复和对账属于本次交付。发现不支持的旧格式时，应在派发、反馈等副作用之前明确拒绝并保留数据库、文件和外部执行。

## 6. 验收

交付按以下三组验证。同一用例可覆盖多个行为，复用既有测试，仅补此次改动影响的检查。

| 组 | 验证方式与通过条件 |
| --- | --- |
| A：双任务真机闭环 | 一次真实 app-server 验收覆盖首次执行、双任务并行、运行中问答、结束后反馈及服务重启。一个任务待答时另一个继续执行；重启后恢复各自原 thread/turn，答案只送达目标请求一次；结束后反馈在原 worktree/thread 开启新 turn。核对冻结配置、原生事件及统一 Decision 提示，本地执行不再调用 `codex exec`。 |
| B：针对性自动测试 | 覆盖多题/选项/备注、重复命令、失效答案、发送回执丢失、请求清除、宿主退出及事件重放。核对轮次终态与缺失正文，未知结果不盲重发；慢请求或单个恢复失败不阻塞其他任务。第三个合格任务可人工领取、自动模式保持单活动，用自动测试验证。故障注入针对本次新增的发送和持久化边界，不要求穷举所有组合。 |
| C：既有契约回归 | 复用 PR 归属、关闭未合并后续聊、真实合并唯一交付与 DAG 解锁的回归；补齐新协议的旧格式拒绝检查，证明拒绝发生在副作用前且现场保留。未修改交付逻辑时，无需为此次迁移重新执行人工合并；涉及交付逻辑改动时再针对实际风险补验。 |

记录版本、原生身份、操作及结果，区分真机、自动测试和未测范围。协议字段以目标 Codex 的生成 schema 和[官方文档](https://learn.chatgpt.com/docs/app-server)核对。

### 第一阶段残留（2026-09-29）

以下要求与结果同步至 [验收状态](../validation/local-codex-app-server/status.md)。A/B 已补验，保留完成记录以明确通过边界；当前证据见 [验收状态](../validation/local-codex-app-server/status.md)。

- **A：服务级双任务真机闭环，已通过。** 真实 SteerLine Service 中第一条原生问答待答时第二任务完成；重建服务后恢复原 call/thread/turn，答案只送达原请求，结束后原 thread/worktree 的新 turn 完成。冻结配置及并行隔离由现有定向测试和本次服务级证据共同核对。
- **B：独立进程生命周期及隔离回归，已通过。** 在 WMI 启动、确认不在 Job Object 的宿主上，服务反馈三种输入形式通过；承载服务的子进程于待答时退出后，独立 app-server 保持存活，新进程重接原请求并完成 turn。相关回执及进程身份见[补验记录](../validation/local-codex-app-server/acceptance-2026-09-29.md)。

Go race detector 因当前环境缺少 CGO/C 编译器未运行，作为残留验证记录；在具备工具链的环境对并行、恢复和回复路径补跑定向检测。
