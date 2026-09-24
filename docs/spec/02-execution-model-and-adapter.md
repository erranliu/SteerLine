# Spec 02：任务、执行模型、最小状态机与执行适配器

任务 ID：5；任务名称：中心执行模型。

状态：待实施。配套 [Plan](02-execution-model-and-adapter.plan.md)。日期：2026-09-24（双后端衔接修订）。

依据：[执行架构共同约束](execution-architecture.spec.md)。协议依赖：[Spec 01](01-orca-remote-protocol.md)。下游：[Spec 03](03-remote-execution-and-viewer.md)。

## 1. 目标与范围

解除当前 `Run` 对业务任务、agent、worktree、步骤尝试、决策和 PR 的混合表达。引入持久化领域模型与统一执行接口，使单 Execution 路径可以迁移到远程，同时允许未来一个任务有多个 agent 在独立 worktree 上并行执行。

本阶段完成模型、最小流程状态机、项目步骤契约、事务存储、旧记录迁移、local/现有 Orca 执行包装及客户端数据接口调整。生产调度仍只有一个活动 Execution；通过模型级测试证明能表达并行关系，不实现并行选择器、候选比较或结果整合，也不实现任意 DAG/流程编辑器。

Spec 01 的逻辑接口可用于本阶段设计和替身测试。真实 remote adapter 的启用受 Spec 01 能力门槛和 Spec 03 约束。

local Codex 与 Orca 是长期正式后端。[控制台 4A](pr-04a-execution-backends.spec.md) 定义共同操作/身份/能力，[4B](pr-04b-conversation-events.spec.md) 定义会话和全文查询。本阶段复用先落地的这些契约与测试，负责领域模型、事务和旧状态迁移；若本阶段先落地，4A/4B 直接扩展本阶段权威接口。不得建立平行 adapter、另一会话模型或双重业务写者；不改变控制台 PR 3 的本机生命周期。

后续原生任务扩展分别由 [任务模型](task-core.spec.md)、[固定 DAG](task-dag.spec.md)、[Beads 退出](beads-exit.spec.md) 和 [关系视图](dag-view.spec.md) 四个独立任务承担，各有同名配套 Plan 和独立实现 PR，不增加本阶段的 DAG 交付范围。未切换项目继续由 Beads 提供任务事实；已按 Beads 退出 Spec 切换的 native 项目以固定 DAG Spec D3 的中心任务/交付事务为权威。已预分配的非负整数任务 ID 是同一任务后续原生 Task 的 ID，不再生成第二个对外任务号；Beads ID 等外部来源身份另存。后续扩展复用本阶段身份、数据库和命令服务，不再迁移一套执行模型。

## 2. 实体与身份

WorkflowRun、Execution、StepAttempt 等 ID 为不透明稳定标识，不由终端标题、远端路径或显示名称承担。Task 的对外 ID 对已规划任务使用预先分配的非负整数，以规范仓库身份为唯一性作用域；指向同一仓库的不同项目配置不能重复使用该数字。未映射规划 ID 的旧 Beads 历史使用明确的非数字 legacy 身份，不能占用规划号。业务身份至少按 project 隔离；预留 namespace，单机固定为 `local`，本阶段不实现多租户认证。

| 实体 | 必需信息与关系 |
| --- | --- |
| Task | 稳定 task_id、project、外部来源 ID、标题、领取者、任务来源、最终任务状态；已有规划 ID 的任务沿用同一数字 task_id，不另造对外 ID |
| WorkflowRun | Task、workflow_version、阶段、目标仓库/分支、固定配置、选用结果的关联 |
| Execution | WorkflowRun、agent、execution target、worktree 身份/分支/base、执行状态、current_step、候选 revision、修复轮数 |
| StepAttempt | Execution、step_id、尝试序号、operation_id、状态、输入 revision、契约摘要、前置结果 ID、结果、外部执行绑定 |
| Decision | 作用域、Execution/Attempt 或 WorkflowRun、问题 ID、请求、答案、回答者、状态及版本 |
| Event | 本地递增 event ID、业务身份、外部来源 ID、类型、接收时间、payload |
| Operation | 外部动作意图、幂等键、负载摘要、平台 request ID、回执及不确定原因 |

一个 Task 可保留多个历史 WorkflowRun；首版只允许一个活动 WorkflowRun，这是调度策略。一个 WorkflowRun 可以拥有多个 Execution，不能增加 `UNIQUE(workflow_run_id)` 限制。一个 Execution 包含顺序步骤及每步的历史尝试，同一步只允许一个权威活动尝试。

并行 Execution 使用不同 worktree/分支。Orca Task/Dispatch 是外部绑定；业务 Task 与 Orca Task 不一一对应。未来并行 Execution 不能争用同一个只能保有一个权威 Dispatch 的 Orca Task。是否复用某个 worktree/agent 的新步骤，由显式操作和平台能力决定，不能隐式从路径推断。

重试生成新的 attempt ID；同一外部请求的恢复继续使用原 attempt/operation ID。消息先匹配外部 Dispatch，再校验业务 ID。旧尝试的迟到消息保留为历史，不推进新尝试。

## 3. 状态及不变量

状态字段分别描述业务、步骤与可观察性，不把所有情况压成一个 status。

| 字段 | 允许值 |
| --- | --- |
| Task.status | open、in_progress、discarded、closed；本阶段 Beads 路径由任务提供方回读核实，后续 native 路径由固定 DAG Spec D3 的中心事务处理丢弃与交付；已合并 PR 不得转 discarded |
| WorkflowRun.phase | queued、executing、waiting_decision、waiting_pr_merge、completing、completed、blocked |
| Execution.status | pending、active、waiting_decision、succeeded、failed、blocked、stopped |
| StepAttempt.status | planned、dispatching、dispatched、running、waiting_decision、succeeded、failed、stopping、stopped、outcome_unknown |
| Observation.liveness | live、unverifiable、exited |
| Observation.connection | connected、degraded、unreachable、unknown |
| Decision.status | pending、answer_saved、reply_accepted、applied、failed、superseded |
| Operation.status | pending、in_flight、accepted、confirmed、rejected、outcome_unknown |

unknown 连接只降低事实新鲜度，不覆盖最后已知业务阶段。StepAttempt.outcome_unknown 用于外部动作是否生效不明，不因每次观察超时而写入。未获执行来源支持的状态迁移拒绝；原始平台状态与中心验证后的步骤状态分别保留，例如 Orca 成功结算但结果无效时，中心步骤仍可 failed。

agent 完成仅终止对应步骤；Execution 成功只表示该执行的流程已完成。WorkflowRun 必须显式选用结果并核验交付，Task 关闭仍由中心执行：Beads 路径回读提供方，后续 native 路径同事务记录交付与关闭。首版单 Execution 可以自动选用其唯一有效结果；以后多个结果不能沿用“首个成功即完成”的隐式规则。

Execution 的问题只暂停自身。流程级决策必须显式声明作用域。WorkflowRun 的等待态是聚合结果：有其他可运行 Execution 时仍可 executing，并额外暴露待决策数量。

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

新配置使用 `workflow_definition`（id/version、review_required、max_repair_rounds）和 `project_steps`，与旧 `workflow` 自由文本及 start_skill/finish_skill 分开。不允许项目提供自由文本顺序来改写最小流程。

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

`local_process` 与 Orca 后端族都作为长期正式适配维护；`orca_terminal_v2` 保留现有协议，新增 `orca_supervised` 由 Spec 03 完成。原 `host=local|orca` 配置继续可读；新增远程目标必须显式选择受监管模式，不能改变现有 Orca 配置默认行为。backend 与 agent/target 分开，预检不能让未使用的本机 Codex 成为 Orca 的无条件依赖。能力使用 verified/unsupported/not_tested 及版本/目标/证据，不允许隐式 fallback。

沿用 4A 参数化契约套件，业务状态机不依赖具体平台类型；沿用 4B 的 session/turn/item/phase、来源 cursor、正文引用和缺口语义。轮次结束不驱动 Task 完成，后端接受 reply 不证明 Decision.applied。Windows 中心服务按 [PR 3](pr-03-service-lifecycle.spec.md) 由独立可见服务窗口承载：关闭 viewer 不停服，关窗使中心服务退出并清理本机 worker；Linux detached 路径按平台契约运行。当前本机停服策略与远端仅分离监督的策略显式区分，不藏在 Close/析构中；关窗或中心重启不证明远端 worker 已停止。

当前受监管远程目标不提供生产取消操作。状态模型和 StopExecution 接口保留给本机适配器及未来经验证的平台版本；远程命令入口应明确返回不支持并展示人工处理原因，不能把服务停机或断线当作远端停止。

## 5. 持久化与命令入口

本阶段采用本机 SQLite 事务存储，放在现有 run_dir 内；数据库驱动和具体表结构由实现选定。保留 Store 接口，为以后服务部署更换数据库提供边界，本阶段只允许单个调度服务持有仓库锁。

同一事务提交领域状态、规范事件和相应 Operation/outbox；收到外部消息后先按来源 ID 去重并保存，再应用状态转换。Ack 仅在平台要求的处理及本地持久化完成后发出。不能使用“内存已更新”代替落盘。

外部动作在事务外执行，调用结果另行入库；崩溃在两者之间时保留不确定性。平台未提供可恢复 request ID 时，不把本地 operation ID 当成远端去重保证。

中心服务是领域状态的单写者。CLI/viewer 通过 CommandService 提交命令，首版可向 SQLite 命令队列追加记录，不直接更新 Task/Execution/Decision。命令包含目标 ID、expected_version、idempotency_key、发起者和 payload。数据库约束保证同幂等键同内容复用结果，不同内容报冲突。

远程决策命令须能在服务暂停期间持久保存，展示为待处理；服务恢复后仍须校验版本和目标。历史 decision.answer.json 可由兼容导入器读入同一命令通道，避免产生第二个状态写者。

新增 QueryService 返回 WorkflowRun 与 executions 数组、决策列表，以及按 Execution/Attempt 筛选的事件分页。事件 cursor 是服务逻辑位置，不暴露文件字节偏移。兼容 watch 的原始日志入口仍可使用文件读取。

会话/正文查询直接复用 4B 的 DTO、分页上限、完整性和稳定 ID；迁移不得使已保存的 turn/item/正文修订变成全新消息，UI 私有折叠状态不迁入业务表。源 cursor 与 Query cursor 分开；如果存储迁移必须使旧 Query cursor 失效，返回明确 cursor_invalidated 并支持按稳定消息 ID 重定位，不静默跳过内容。

runtime.json 改为使用新 schema_version 的可重建读模型，标记 database revision、生成时间及导出错误。已合并控制协议占用 v4，不可复用；新投影版本至少为 5，实施时若已被其他不兼容结构占用则分配下一未使用版本，并提交精确版本/兼容矩阵。它不再参与业务恢复；导出失败不回滚已提交状态，客户端必须能显示投影过期并从 QueryService 读取。activity.jsonl 继续作为可重建展示日志，不能作为唯一 inbox 或恢复日志。

## 6. 既有版本与控制台扩展记录迁移

迁移持有仓库锁，拒绝与旧调度服务同时运行。先检查现有 repository/claimant/记录完整性，再保存原 runtime 的不可变备份和摘要；迁移在数据库事务中完成，并写入来源摘要与迁移版本。

迁移输入覆盖 runtime v1/v2/v3/v4，以及当时已落地的 4A/4B/5 版本化绑定、Operation、会话/正文/checkpoint 与决策应用记录。记录来源格式版本，不仅迁移 Run；控制请求/回执、dispatch mode/pause、实例身份及 indeterminate blockers 不得丢失或重放。已消费请求保留历史结果，旧实例未处理命令不在新实例自动执行，源 cursor 与去重事实成组迁移。源文件/正文保留且摘要校验，不从当前配置猜路由。

迁移前对已知有规划数字 ID 的旧任务必须提供显式、版本化的 `task-identity-map/v1` 输入，列出项目/仓库身份、无前导零的数字 task_id、以及已核实的旧来源三元组；输入摘要和核验结果随迁移事务保存。同一数字不能映射两个业务任务，同一旧来源不能映射两个数字，映射中的旧来源必须在实际旧状态中核对存在且身份一致。无法核实的对应关系不得按标题、Spec 文件名或相近文本猜测。未映射的旧任务取得非数字 legacy task_id，并保留外部来源 ID；后续发现它与规划数字其实是同一任务时阻止再发布新 Task，待独立身份迁移方案处理，不能自动创建第二个 Task 或改写已有运行的 task_id。迁移后的投影和命令只对同一 Task 返回一个稳定对外 ID。

- 每个旧 Run 映射为一个 WorkflowRun，其 ID 保留旧 run_id，workflow_version 标为 `legacy-v1`；同业务任务的历史记录关联同一 Task。不为旧的整段技能执行补造逐阶段验证/审查成功记录。
- 为旧 Run 建立一个 Execution；现有 Process、Host、worktree、PR、Decision、配置和 Beads 身份均保留来源。历史 v1/v2 的 local 解释保持原规则。
- 为每个可确证的 retry 历史及当前执行建立 StepAttempt，保留 attempts 目录 ID、previous-run.json 与原日志/结果路径。映射稳定且可重复；不移动或覆盖旧文件。
- 缺少历史字段的记录标注 legacy/incomplete，不猜测 agent、Dispatch 或已完成阶段。旧文件损坏或身份冲突使迁移失败，事务回滚，原目录不受影响。
- 旧 pending/running 或 resuming 的不确定恢复规则保持保守，不因迁移自动接管或续跑。旧 completed、等待 PR 和等待决策状态不得丢失。
- 数据库提交后若新版本投影写入失败，下次启动重新导出，不再次导入旧快照；来源摘要和迁移标记防止重复记录。
- 新二进制支持读取无数据库的旧快照作只读查看。完成迁移后的目录不得用旧二进制自动恢复运行，也不能让旧快照自动覆盖新数据库。系统不提供迁移完成后的回退流程；用户需要回退时自行分析执行现场、新外部动作和数据差异，并人工决定处置方式，系统不承诺自动修复。

新版本导出顶层至少包括 project/service、revision、tasks、workflow_runs、executions、attempts 和 decisions；各集合使用 ID 关联。旧 `current_task/slot_occupied` 仅可作为兼容展示摘要，不再是身份或并行能力的权威。

## 7. 现有路径回归要求

local、现有本机 Orca、watch、answer、retry、accept-result 和合并后 completion 的行为在对应兼容适配下可继续使用；远程能力未启用时不要求远端环境。

领域层不再接受远端路径给 `os.ReadFile`、本地 Git workspace 校验或本地 completion。路径属于明确的 target；兼容本地路径保留原校验，远程验证由 Spec 01/03 的协议承担。

新增运行的 `minimal-v1` 必须由中心逐步派发，不能包装回单个 `execute_workflow`。旧 local/本机 Orca 记录与未迁移项目继续使用显式 `legacy-v1` 兼容路径，不宣称具有逐步骤保证。新流程先通过平台替身和本地步骤测试，远程验证由 Spec 03 完成。配置版本和实际输入摘要随尝试固定，不能因修改全局配置而改变历史尝试的恢复方式。

## 8. 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| M01 | 同一 Task/WorkflowRun 存入两个 agent Execution，各自 worktree/Dispatch/日志/问题独立，无唯一键冲突 |
| M02 | 旧尝试的成功或回答迟到，只更新历史，不能推进当前尝试或其他 Execution |
| M03 | 单个 Execution 完成、失败或等待决策，均不错误关闭整个业务任务 |
| M04 | 两个客户端并发回答同一版本的问题，仅一个不同答案有效；同内容重试返回同一命令结果 |
| M05 | 状态、事件、outbox 任一写入失败，事务全部回滚，不发外部动作 |
| M06 | 事务提交后进程退出，重启仍找到待发送动作；响应丢失时进入可核实的不确定状态 |
| M07 | v1–v4 各状态、retry、控制 blocker/回执及已落地 4A/4B/5 扩展记录迁移后关联完整，重复启动不重复导入，损坏输入不部分迁移 |
| M08 | 数据库已迁移而投影未导出时重启，使用数据库恢复，原历史文件保持可读 |
| M09 | local 与现有 Orca 路径的相关测试通过；新模型不自动续跑旧 unknown/resuming |
| M10 | 查询接口按任务聚合、按执行过滤；新增远端路径不会进入本机文件/Git操作 |
| M11 | implement 成功仅生成 verify 意图；verify 失败或必需 review 未通过时没有 publish_pr 派发 |
| M12 | 验证后 revision 改变，旧证据失效；按修复回路重新验证，超出固定次数后停止自动修复 |
| M13 | 在每一步结果提交及下一步派发之间重启，只恢复正确步骤；已通过且 revision 未变化的步骤不重复执行 |
| M14 | 特殊 PR provider 能被替换而不改状态机；越步结果/未匹配前置结果不能推进流程 |
| M15 | 旧 Run 标为 legacy-v1，无虚构细分阶段；新流程缺必需步骤入口时在领取前拒绝 |
| M16 | 同一双后端/会话契约套件在新 Store 上通过；迁移不变更原执行路由，不丢正文、源 checkpoint、去重或应用证明 |
| M17 | 新投影不复用已有 v4；旧只读客户端明确拒绝不兼容格式，失效 Query cursor 可按稳定消息重定位，无双写权威 |
| M18 | 预分配数字 task_id 与已知旧来源在迁移后指向同一 Task；重复数字、冲突映射或不明来源被拒绝；未映射 legacy 任务不占数字号，不能为同一业务任务再建第二个对外 ID |

实施需运行相关迁移、存储故障、身份隔离、决策竞争及现有执行回归测试，再执行仓库要求的 Go 测试和静态检查。此 Spec 完成不等于远程执行已上线。
