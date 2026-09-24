# Spec 02：任务、执行模型、最小状态机与执行适配器

任务 ID：5；任务名称：中心执行模型。

状态：待实施。配套 [Plan](02-execution-model-and-adapter.plan.md)。日期：2026-09-24（双后端衔接修订）。

依据：[执行架构共同约束](execution-architecture.spec.md)。协议依赖：[Spec 01](01-orca-remote-protocol.md)。下游：[Spec 03](03-remote-execution-and-viewer.md)。

## 1. 目标与范围

解除当前 `Run` 对业务任务、agent、worktree、步骤尝试、决策和 PR 的混合表达。引入持久化领域模型与统一执行接口，使单 Execution 路径可以接入远端。

本阶段完成模型、最小流程状态机、项目步骤契约、事务存储、local/Orca 执行包装及客户端数据接口调整。每个 WorkflowRun 只允许一个活动 Execution；不实现同任务候选并行、比较、结果整合或任意 DAG/流程编辑器。

Spec 01 的逻辑接口可用于本阶段设计和替身测试。真实 remote adapter 的启用受 Spec 01 能力门槛和 Spec 03 约束。

local Codex 与 Orca 是长期正式后端。[控制台 4A](pr-04a-execution-backends.spec.md) 定义共同操作/身份/能力，[4B](pr-04b-conversation-events.spec.md) 定义会话和全文查询。本阶段复用先落地的这些契约与测试，负责领域模型和事务存储；若本阶段先落地，4A/4B 直接扩展本阶段权威接口。不得建立平行 adapter、另一会话模型或双重业务写者；不改变控制台 PR 3 的本机生命周期。

后续原生任务扩展分别由 [任务模型](task-core.spec.md)、[固定 DAG](task-dag.spec.md)、[Beads 退出](beads-exit.spec.md) 和 [关系视图](dag-view.spec.md) 四个独立任务承担，各有同名配套 Plan 和独立实现 PR，不增加本阶段的 DAG 交付范围。新原生项目以固定 DAG Spec D3 的中心任务/交付事务为权威；当前 Beads 路径仅用于本阶段开发验证，在原生接入时移除。已预分配的非负整数任务 ID 是同一任务后续原生 Task 的 ID，不再生成第二个对外任务号。后续扩展复用本阶段身份、数据库和命令服务。

## 2. 实体与身份

WorkflowRun、Execution、StepAttempt 等 ID 为不透明稳定标识，不由终端标题、远端路径或显示名称承担。Task 的对外 ID 使用预先分配的非负整数，以规范仓库身份为唯一性作用域；指向同一仓库的不同项目配置不能重复使用该数字。业务身份至少按 project 隔离；本阶段不实现多租户认证。

| 实体 | 必需信息与关系 |
| --- | --- |
| Task | 稳定 task_id、project、标题、领取者、最终任务状态；沿用规划阶段分配的数字 task_id，不另造对外 ID |
| WorkflowRun | Task、workflow_version、阶段、目标仓库/分支、固定配置、选用结果的关联 |
| Execution | WorkflowRun、agent、execution target、worktree 身份/分支/base、执行状态、current_step、候选 revision、修复轮数 |
| StepAttempt | Execution、step_id、尝试序号、operation_id、状态、输入 revision、契约摘要、前置结果 ID、结果、外部执行绑定 |
| Decision | 作用域、Execution/Attempt 或 WorkflowRun、问题 ID、请求、答案、回答者、状态及版本 |
| Event | 本地递增 event ID、业务身份、外部来源 ID、类型、接收时间、payload |
| Operation | 外部动作意图、幂等键、负载摘要、平台 request ID、回执及不确定原因 |

一个 Task 可保留多次 WorkflowRun，但只有一个权威活动 WorkflowRun。每次 WorkflowRun 当前只有一个活动 Execution；一个 Execution 包含顺序步骤及每步的历史尝试，同一步只有一个权威活动尝试。Orca Task/Dispatch 是外部绑定，不代替业务 Task 或 Execution 身份。

重试生成新的 attempt ID；同一外部请求的恢复继续使用原 attempt/operation ID。消息先匹配外部 Dispatch，再校验业务 ID。旧尝试的迟到消息保留为历史，不推进新尝试。

## 3. 状态及不变量

状态字段分别描述业务、步骤与可观察性，不把所有情况压成一个 status。

| 字段 | 允许值 |
| --- | --- |
| Task.status | open、in_progress、discarded、closed；本阶段只建模状态，后续由固定 DAG Spec D3 的中心事务处理丢弃与交付；已合并 PR 不得转 discarded |
| WorkflowRun.phase | queued、executing、waiting_decision、waiting_pr_merge、completing、completed、blocked |
| Execution.status | pending、active、waiting_decision、succeeded、failed、blocked、stopped |
| StepAttempt.status | planned、dispatching、dispatched、running、waiting_decision、succeeded、failed、stopping、stopped、outcome_unknown |
| Observation.liveness | live、unverifiable、exited |
| Observation.connection | connected、degraded、unreachable、unknown |
| Decision.status | pending、answer_saved、reply_accepted、applied、failed、superseded |
| Operation.status | pending、in_flight、accepted、confirmed、rejected、outcome_unknown |

unknown 连接只降低事实新鲜度，不覆盖最后已知业务阶段。StepAttempt.outcome_unknown 用于外部动作是否生效不明，不因每次观察超时而写入。未获执行来源支持的状态迁移拒绝；原始平台状态与中心验证后的步骤状态分别保留，例如 Orca 成功结算但结果无效时，中心步骤仍可 failed。

agent 完成仅终止对应步骤；Execution 成功只表示流程步骤完成。中心核验唯一有效结果和真实交付后，后续固定 DAG 事务才能关闭 Task。

Execution 的问题暂停当前执行；流程级决策须显式声明作用域。WorkflowRun 如实展示等待决策，不把该状态当作执行完成。

不确定且可能仍在执行的尝试继续占用相应资源许可；不能因为暂时失联而释放许可并启动替代执行。停止必须取得目标退出/停止证据；中心服务退出与取消远端 worker 是不同操作。

### 3.1 本轮实现的最小流程

新增 `minimal-v1` 流程，流程定义和项目步骤契约在 WorkflowRun 创建时固定。每个 Execution 的步骤由中心推进；WorkflowRun.phase 继续作为聚合状态，current_step 明确展示当前细分步骤。

```text
prepare → implement → verify → review（按固定配置）→ publish_pr
                       ↑          │                      ↓
                       └─ implement（修复）         waiting_pr_merge → completing → completed
```

prepare 包括中心领取、确定 base 和通过执行适配器准备工作区；waiting_pr_merge、completing 为中心控制阶段，不伪装成远端 agent 步骤。问答可暂停任一活动步骤，回答后恢复原步骤，不自动进入下一阶段。

| 步骤 | 中心通过条件 | 允许的项目能力 |
| --- | --- | --- |
| implement | 有效候选 commit 与代码工作区检查回执，身份匹配 | 读取 Spec、规划、开发和自测、提交本地候选 commit；不推送/提 PR |
| verify | 必需检查全部实际执行且成功，检查前后代码 revision 相同 | 确定性测试/构建脚本；只产出检查结果，不顺带修改代码或提 PR |
| review | 固定配置要求的审查完成，verdict=passed，revision 与验证一致 | 标准审查提示或项目审查技能；报告问题，由中心安排修复 |
| publish_pr | 当前 revision 的验证及必需审查有效，精确 PR 和已发布 head 经平台核实 | 推送已通过门槛的 commit、标准或项目特殊 PR 流程；不合并/结项 |

默认要求 review；只有派发前显式固定 `review_required=false` 才可跳过，不能自动降低目标项目既有审查要求。运行中更改全局配置不改变该次流程的门槛。

首版 revision 使用候选 commit SHA；验证/审查/发布前均检查代码输入没有未提交变更。生成的构建日志等输出可按固定配置排除，未跟踪的代码输入不得被排除。代码变化使旧 verify/review 结果失效，必须重新验证；旧事件保留供审计。

verify 检查失败或 review 请求改动时，中心可以派发 implement 修复：固定输入包含失败结果，生成新 attempt，修复后重新走 verify 和 review。`max_repair_rounds` 随流程固定，默认 2；耗尽后 blocked，等待人工处理。基础设施故障、结果丢失及身份不明不触发自动代码修复。publish_pr 的不确定结果必须先查询同次发布副作用，不能借修复循环再建 PR。

单个 Execution 同时只允许一个步骤修改或检查工作区；下一步需等待前一步结算和资源所有权明确。每次成功只通过当前步骤门槛，中心在同一事务记录该门槛及下一步意图。进程在此处重启时从已记录步骤恢复，不从 implement 重新开始。

### 3.2 项目步骤契约

配置使用 `workflow_definition`（id/version、review_required、max_repair_rounds）和 `project_steps`。不允许项目提供自由文本顺序来改写最小流程。

每个步骤配置声明 provider（agent_skill 或 command）、版本化的仓库相对入口/内置入口、输入输出 schema、执行位置及超时。命令使用可执行文件/参数数组；远端适配器负责目标 shell 的传参，不将 JSON 字符串直接拼成 shell 代码。启动前验证入口、支持能力和契约摘要。

implement 可使用中心提供的通用开发指令并加载项目基础技能；verify 必须提供确定性检查命令及必需检查清单；review 可用标准入口或项目专用技能；publish_pr 可绑定特殊 PR 技能或脚本。项目不必提供包办全部流程的 start/finish 技能。当前平台若无内置发布实现，必须显式配置 publish_pr 入口，不能把查询 PR 的适配器当作发布器。

远程 command provider 首版允许由 Orca 受监管步骤包装执行，但需回传脚本自身的结构化回执；支持直接命令执行后可替换包装，中心状态机不变。自然语言“测试通过”不能替代检查回执。脚本/技能只能返回本步结果，不能写中心状态或自行派发下一步。

普通 agent 的操作约束依赖步骤协议和实际执行环境；中心校验不能保证具有全权限的 agent 绝不会越界产生副作用。发现提前发布 PR、跨步骤改动或不明副作用时进入 blocked 并核实，不把越界结果当作门槛通过。需要强制副作用隔离的项目应使用专用脚本及权限隔离，不能把提示词描述成系统级限制。

## 4. 执行适配器契约

定义平台无关的逻辑接口，具体 Go 签名由实现决定，但必须保留以下输入和结果语义。

| 操作 | 输入与输出 |
| --- | --- |
| Probe | target → 已验证/不支持/未知的能力及版本信息 |
| Preflight / Prepare | 按固定 backend/protocol/agent/target 检查依赖、准备或通过组合启动取得真实 workspace；缺能力在领取前尽可能拒绝 |
| Start | 固定的 AttemptSpec + Operation → accepted、rejected 或 outcome_unknown，以及外部身份/残留资源 |
| Observe | 已持久化的 ExecutionRef → 带时间和来源的观察事实 |
| InspectWorkspace | 固定 ExecutionRef → 目标仓库/worktree/base/HEAD/dirty 证据；由适配器按执行位置核验 |
| ReadOutput | ExecutionRef + opaque cursor → 有界内容、来源、下个 cursor、缺口 |
| Receive / Ack | 控制端 inbox 身份 → Delivery；确认遵循平台要求，与读取分离 |
| Reply | DecisionRef + 固定答案 + Operation → 平台接受/拒绝/不确定；不自行标记 applied |
| StopExecution | ExecutionRef + Operation → 停止回执，不把发出请求当成已退出；当前生产远程目标返回明确的 `capability_unsupported`，不发 Orca stop 变更；下游文档中的 Stop 指此操作 |
| DetachObservation | 仅结束中心监听，不取消 worker、不隐式删除工作区；服务生命周期按明确策略调用 |
| SettleResources | 已结算执行的 release/已明确选择的 retain → 回执或不确定状态 |
| ReconcileOperation | 已有平台 request ID 和原请求 → 只读查询或平台允许的同请求恢复 |

ExecutionRef 保存 backend/protocol/agent、固定 target/连接 ID、worktree 身份及协议专属绑定：local 的 PID/创建身份，Orca terminal v2 的 runtime/terminal/incarnation，supervised 的 home runtime/Run/Task/Dispatch/execution host。不能要求 local 或 terminal v2 虚构 Dispatch。历史尝试使用派发时绑定，配置默认目标改变不能把旧动作路由到新机器。Reply/Continue 共用固定 Operation；Receive/Ack 为支持 inbox 的协议的可选接口，不支持须显式报告。

适配器不能写业务状态、领取任务或自行决定重试。所有返回由中心状态机在事务内校验并应用。错误区分能力不支持、明确拒绝、暂时不可达、身份不符、协议无效和动作结果不确定；禁止用单个 error 字符串决定是否重发。

`local_process` 与 Orca 后端族都作为长期正式适配维护；受监管远端 `orca_supervised` 由 Spec 03 完成。配置显式选择 backend/protocol/agent/target，预检不能让未使用的本机 Codex 成为 Orca 的无条件依赖。能力使用 verified/unsupported/not_tested 及当前目标/版本证据，不允许隐式 fallback。

沿用 4A 参数化契约套件，业务状态机不依赖具体平台类型；沿用 4B 的 session/turn/item/phase、来源 cursor、正文引用和缺口语义。轮次结束不驱动 Task 完成，后端接受 reply 不证明 Decision.applied。Windows 中心服务按 [PR 3](pr-03-service-lifecycle.spec.md) 由独立可见服务窗口承载：关闭 viewer 不停服，关窗使中心服务退出并清理本机 worker；Linux detached 路径按平台契约运行。当前本机停服策略与远端仅分离监督的策略显式区分，不藏在 Close/析构中；关窗或中心重启不证明远端 worker 已停止。

当前受监管远程目标不提供生产取消操作。状态模型和 StopExecution 接口保留给本机适配器及未来经验证的平台版本；远程命令入口应明确返回不支持并展示人工处理原因，不能把服务停机或断线当作远端停止。

## 5. 持久化与命令入口

本阶段采用本机 SQLite 事务存储，放在现有 run_dir 内；数据库驱动和具体表结构由实现选定。保留 Store 接口，为以后服务部署更换数据库提供边界，本阶段只允许单个调度服务持有仓库锁。

同一事务提交领域状态、规范事件和相应 Operation/outbox；收到外部消息后先按来源 ID 去重并保存，再应用状态转换。Ack 仅在平台要求的处理及本地持久化完成后发出。不能使用“内存已更新”代替落盘。

外部动作在事务外执行，调用结果另行入库；崩溃在两者之间时保留不确定性。平台未提供可恢复 request ID 时，不把本地 operation ID 当成远端去重保证。

中心服务是领域状态的单写者。CLI/viewer 通过 CommandService 提交命令，首版可向 SQLite 命令队列追加记录，不直接更新 Task/Execution/Decision。命令包含目标 ID、expected_version、idempotency_key、发起者和 payload。数据库约束保证同幂等键同内容复用结果，不同内容报冲突。

远程决策命令须能在服务暂停期间持久保存，展示为待处理；服务恢复后仍须校验版本和目标。答案通过同一命令通道提交，不建立第二个状态写者。

新增 QueryService 返回 WorkflowRun 与 executions 数组、决策列表，以及按 Execution/Attempt 筛选的事件分页。事件 cursor 是服务逻辑位置，不暴露文件字节偏移；原始输出可按固定执行身份读取。

会话/正文查询直接复用 4B 的 DTO、分页上限、完整性和稳定 ID；UI 私有折叠状态不进入业务表。源 cursor 与 Query cursor 分开；当前格式内查询版本失效时返回 cursor_invalidated，不静默跳过内容。

runtime.json 作为当前数据库的可重建读模型，标记 database revision、生成时间及导出错误；它不参与业务恢复。导出失败不回滚已提交状态，客户端显示投影过期并从 QueryService 读取。activity.jsonl 可作为展示日志，不能作为唯一 inbox 或恢复日志。只支持本轮确定的格式；不提供旧投影读者。

## 6. 上线前状态边界与当前路径

本任务只初始化和读取本轮 SQLite/Query/Command 格式，不导入 runtime v1–v4、旧 Beads 数据或其他旧状态。首次启动需干净的状态目录；检测到旧状态、未知 schema 或另一服务仍持锁时，明确拒绝有副作用动作。人工先核实可能仍活跃的 worker、PR 和资源，再决定旧目录处置；服务不自动删除或接管。当前格式内事务提交、投影失败、重启与响应丢失仍按第 5 节恢复，不得重复外部动作。

读模型顶层至少包括 project/service、revision、tasks、workflow_runs、executions、attempts 和 decisions；各集合使用 ID 关联。local 与已支持的 Orca 后端在新格式上运行，远程能力未启用时不要求远端环境。领域层不把远端路径交给本机文件或 Git 操作；路径属于固定 target，由相应适配器核验。

新增运行的 `minimal-v1` 由中心逐步派发，不能包装回单个 `execute_workflow`。新流程先通过平台替身和本地步骤测试，远程验证由 Spec 03 完成。配置版本和实际输入摘要随尝试固定，不能因修改全局配置而改变活动尝试的恢复方式。

## 8. 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| M01 | Task、WorkflowRun、Execution、StepAttempt 身份互不混用；同一 WorkflowRun 不启动第二个活动 Execution |
| M02 | 较早尝试的成功或回答迟到，只更新原尝试记录，不能推进当前尝试或流程 |
| M03 | 单个 Execution 完成、失败或等待决策，均不错误关闭整个业务任务 |
| M04 | 两个客户端并发回答同一版本的问题，仅一个不同答案有效；同内容重试返回同一命令结果 |
| M05 | 状态、事件、outbox 任一写入失败，事务全部回滚，不发外部动作 |
| M06 | 事务提交后进程退出，重启仍找到待发送动作；响应丢失时进入可核实的不确定状态 |
| M07 | 旧状态、未知 schema 或另一服务持锁时拒绝有副作用启动，保留现场并给出明确诊断 |
| M08 | 当前数据库事务已提交而投影未导出时重启，从数据库重建投影，不重复外部动作 |
| M09 | local 与已支持 Orca 后端在新格式下运行；不接管旧格式的 unknown/resuming |
| M10 | 查询接口按任务聚合、按执行过滤；新增远端路径不会进入本机文件/Git操作 |
| M11 | implement 成功仅生成 verify 意图；verify 失败或必需 review 未通过时没有 publish_pr 派发 |
| M12 | 验证后 revision 改变，旧证据失效；按修复回路重新验证，超出固定次数后停止自动修复 |
| M13 | 在每一步结果提交及下一步派发之间重启，只恢复正确步骤；已通过且 revision 未变化的步骤不重复执行 |
| M14 | 特殊 PR provider 能被替换而不改状态机；越步结果/未匹配前置结果不能推进流程 |
| M15 | 新流程缺必需步骤入口时在领取前拒绝，不把整段技能执行伪装成分阶段结果 |
| M16 | 同一双后端/会话契约套件在新 Store 上通过；当前运行的路由、正文、源 checkpoint、去重和应用证明重启后保持一致 |
| M17 | 投影只由当前数据库生成，未知格式拒绝；失效 Query cursor 明确报错，无双写权威 |
| M18 | 预分配数字 task_id 原值保存；同一仓库重复数字拒绝，不从标题或文件名推断第二个对外身份 |

实施需运行存储故障、身份隔离、决策竞争、当前格式重启及现有执行路径回归测试，再执行仓库要求的 Go 测试和静态检查。此 Spec 完成不等于远程执行已上线。
