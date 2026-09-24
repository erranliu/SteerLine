# 规划依赖与并行关系

本表覆盖已经进入 [主 roadmap](roadmap.md) 的目标，包括尚未形成 Spec 的目标。产品计划中尚未选入的方向不在这里排执行顺序。条目与目标标识对应，Spec 链接提供契约来源；本表不能直接用于执行。

## 如何阅读

- **硬前置**：必须取得前置实现及相应验证结果，才能完成依赖它的工作。
- **接口前置**：输入输出、身份、错误与版本语义固定后，可规划基于契约的独立开发；真实集成仍需前置实现。
- **共享冲突**：双方修改同一业务权威、公共接口、schema 或平台资源，必须先指定修改职责与集成顺序。
- **规划假设**：并行机会或未确定的分工，不改变已有 Spec。没有明确接口/职责时，只能推进不依赖它的独立研究，不能据此启动冲突实现。

当前控制台剩余 Spec/Plan 的交付顺序为 4A → 4B → 4C → 5，前一阶段验证并合入后才开始下一阶段。下文的接口并行机会供后续重规划使用；采用前必须修订受影响的 Spec/Plan。

## 目标与契约入口

| 稳定标识 | 目标 | Spec / Plan |
| --- | --- | --- |
| EXEC-BACKENDS | 4A 双执行后端 | [Spec](../spec/pr-04a-execution-backends.spec.md) · [Plan](../spec/pr-04a-execution-backends.plan.md) |
| CONVERSATION | 4B 会话事件 | [Spec](../spec/pr-04b-conversation-events.spec.md) · [Plan](../spec/pr-04b-conversation-events.plan.md) |
| CONSOLE | 4C 执行控制台 | [Spec](../spec/pr-04-console.spec.md) · [Plan](../spec/pr-04-console.plan.md) |
| INTERVENTION | 5 人工介入 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) |
| REMOTE-PROTOCOL | 远程协议剩余门槛验收 | [Spec 01](../spec/01-orca-remote-protocol.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) |
| EXEC-MODEL | 中心执行模型 | [Spec 02](../spec/02-execution-model-and-adapter.md) |
| REMOTE-WORKFLOW | 分阶段远程闭环 | [Spec 03](../spec/03-remote-execution-and-viewer.md) |
| TASK-CORE | 原生任务模型 | [Spec](../spec/task-core.spec.md) · [Plan](../spec/task-core.plan.md) |
| TASK-DAG | 固定依赖调度 | [Spec](../spec/task-dag.spec.md) · [Plan](../spec/task-dag.plan.md) |
| BEADS-EXIT | 解除 Beads 依赖与项目迁移 | [Spec](../spec/beads-exit.spec.md) · [Plan](../spec/beads-exit.plan.md) |
| DAG-VIEW | 任务关系视图 | [Spec](../spec/dag-view.spec.md) · [Plan](../spec/dag-view.plan.md) |

## 依赖与并行总表

| 目标关系 | 类型与关键交接 | 并行判断及集成条件 |
| --- | --- | --- |
| EXEC-BACKENDS → CONVERSATION | 接口前置：ExecutionRef、能力、ReadOutput、取消与来源身份 | 规划机会：接口固定后可独立准备解码/重放样本；真实输出接入需适配器，当前阶段仍串行 |
| CONVERSATION → CONSOLE | 接口前置：会话 DTO、正文分页、游标、完整性与 gap | 规划机会：契约固定后 UI 可基于样本开发；精确折叠的集成验收必须使用两来源真实证据，当前阶段仍串行 |
| EXEC-BACKENDS + CONVERSATION + CONSOLE → INTERVENTION | 接口及集成前置：Reply/Continue、固定 Operation、应用证明与问题展示 | 业务恢复和 UI 不能各自解释“已应用”；当前按控制台完成后实施 |
| REMOTE-PROTOCOL 与本机控制台目标 | 输出映射/能力有共享边界；运行环境可以隔离 | 复用已有输出映射，剩余探测可与不依赖其结果的本机工作独立推进；远端分页/恢复证据不代替精确 turn/phase 验证，样本/映射变更需共同核对 |
| REMOTE-PROTOCOL → EXEC-MODEL | 协议输入：已有远端身份、回执、消息和恢复映射及明确能力限制 | 可依据实测映射开发模型与适配器；远程 StopExecution 返回 capability_unsupported，不发送停止变更；未验证能力不随模型上线 |
| EXEC-BACKENDS + CONVERSATION ↔ EXEC-MODEL | 共享冲突：执行契约、事件、Operation、权威存储与迁移 | 先确定公共契约的修改职责；先落地者提供权威接口，后者复用。不能各自实现 adapter、会话模型或竞争写者 |
| REMOTE-PROTOCOL + EXEC-MODEL → REMOTE-WORKFLOW | 开发输入：实测协议、能力限制及中心模型接口；生产硬前置：剩余协议门槛、模型交付和闭环验收 | 可先开发受限远程适配器；P13 由 Spec 03 接入已有校验器后完成端到端验证，不以纯函数测试代替，也不要求接入开发前先取得端到端证据 |
| CONVERSATION + INTERVENTION 与 REMOTE-WORKFLOW | 接口/共享冲突：输出来源、决策状态、应用证明与查询 DTO | 复用共同契约；尚未落地的一侧不得成为另一套实现，原生远端验证单独完成 |
| EXEC-MODEL → TASK-CORE | 硬前置：Task/WorkflowRun 身份、SQLite、中心 Query/Command 与迁移权威 | 可先研究原生身份与来源映射；实现和集成复用中心模型，不另建数据库。原生任务模型 Spec C4 和 Beads 退出 Spec B3/B4 明确身份交接与权威切换 |
| TASK-CORE → TASK-DAG | 硬前置：原生任务、固定契约、事务和项目身份 | 在同一存储中增加依赖、发布和领取；共享 Task/schema 修改顺序先固定，不按文件名推断可以并行 |
| TASK-DAG → BEADS-EXIT | 硬前置：原生 ready/claim/交付与重启恢复闭环 | 可先盘点技能/配置耦合及导入样本；真实项目切换须完成任务 DAG 集成并结清旧运行与回调，不双写任务权威 |
| TASK-DAG + CONSOLE → DAG-VIEW | 接口及集成前置：带版本的图查询、任务详情与控制台查询边界 | DTO 固定后可基于样本准备只读导出；接入与验收使用真实任务查询，按 DAG-VIEW 独立 Spec 在 BEADS-EXIT 后实施。首版 DOT/JSON 和可选 SVG，不扩大当前控制台 Spec 范围 |
| BEADS-EXIT → DAG-VIEW | 集成顺序及共享接口：任务状态、来源映射、阻塞原因和查询版本 | 各任务按独立 Spec/Plan 串行交付，每任务一个实现 PR；可先准备展示样本，不凭共享接口已固定绕过顺序。组合验收核对图/列表及迁移前后身份 |
| TASK-CORE + BEADS-EXIT 与 REMOTE-WORKFLOW | 共享冲突：派发上下文、Task 权威、技能版本、完成与恢复 | 按 Beads 退出 Spec B2/B4 和 Spec 03 的来源分流接入；原生任务无需等待整个远程闭环。远端复用中心上下文，不增加 Beads 或中心数据库依赖，不改变能力门槛 |

## 关键链路卡片

### EXEC-BACKENDS

- **范围与修改面**：Service、local/Orca 适配器、ExecutionRef、Operation、能力、观察/停止/恢复及版本化绑定。
- **输入 → 输出**：已交付的服务窗口生命周期与原执行路径 → 公共后端契约、固定身份、类型化事实/回执和能力报告。
- **关键约束**：唯一业务写者；历史目标不可随配置漂移；Detach 不取消；未知外部动作不盲目重发。依据 Spec B1～B5。
- **交接门槛**：向 CONVERSATION 固定 ReadOutput/ref/来源语义，向 INTERVENTION 固定 Reply/Continue 和 Operation；向 EXEC-MODEL 交接实际记录版本与迁移边界。

### CONVERSATION

- **范围与修改面**：来源解码、会话事件、正文存储、checkpoint/去重、Query DTO 和旧日志兼容。
- **输入 → 输出**：后端输出及真实样本 → session/turn/item/phase、正文分页、完整性、能力与 gap。
- **关键约束**：内容先存后推进 cursor；消息完成不等于轮次/业务完成；不猜 phase；来源与 attempt 隔离。依据 Spec E1～E4。
- **交接门槛**：给 CONSOLE 固定 DTO/分页/晚到 final 语义，给 EXEC-MODEL 固定 ID 与迁移规则；Orca 输出映射与 REMOTE-PROTOCOL 共用证据格式，各环境独立验证。

### CONSOLE

- **范围与修改面**：查询/控制客户端、多项目 UI、会话分组、折叠偏好、launcher；不写业务状态或解码平台协议。
- **输入 → 输出**：公共执行/会话/控制契约 → 可操作控制台与问题、回复、缺口的稳定展示。
- **关键约束**：异步消息携带目标身份；手动展开和阅读位置不被刷新覆盖；待处理问题在折叠区外。依据 Spec U1～U9。
- **交接门槛**：为 INTERVENTION 保留问题详情和精确对象导航接口。UI 开发与业务恢复可否并行取决于接口与文件职责划分，不能共用可变 Run 指针作为交接。

### INTERVENTION

- **范围与修改面**：决策发布/留档、Reply Operation、恢复守卫、命令与决策 UI；保留现有恢复资格。
- **输入 → 输出**：后端能力、固定身份和会话模型 → 同一业务答案/恢复入口、应用证据及可追溯结果。
- **关键约束**：first-wins；接受输入/轮次开始不证明 applied；只解除目标 blocker；不释放其他 unknown 的槽。依据 Spec H1～H6。
- **交接门槛**：与 EXEC-MODEL 统一文件/SQLite 权威切换，与 REMOTE-WORKFLOW 统一 Decision/应用证明。仅平台回执不足时保留待核实，不能用 UI 文案弥补证据。

### REMOTE-PROTOCOL

- **范围与修改面**：P10 消息大小与宿主权限提示的未测边界、能力报告收口，以及配合 Spec 03 完成 P13 端到端验收；旧版兼容暂缓。
- **输入 → 输出**：已有真实远端报告、字段映射和校验器 → 剩余能力的证据或明确限制，以及中心拒绝无效结果的集成证据。
- **关键约束**：首版限定已测 Windows Orca 1.4.209 组合；消息实测到 30 KiB 不代表 Orca 上限，Spec 03 仍采用不高于 8 KiB 的保守上限；生产远程取消不可用，人工清理不能证明自动取消受支持。依据 Spec 01 和 Spec 03 的范围限制。
- **交接门槛**：EXEC-MODEL 复用实测字段语义；REMOTE-WORKFLOW 在生产启用前完成剩余必需验收。P13 接入由 Spec 03 负责，探测与开发不能同时操作同一真实测试执行资源。

### EXEC-MODEL

- **范围与修改面**：领域模型、最小状态机、SQLite、迁移、Query/Command 和 outbox；不提前扩大生产并发。
- **输入 → 输出**：共同执行/会话契约与远端映射 → 事务状态机、稳定业务身份和可恢复命令/事件。
- **关键约束**：Task/Execution/Attempt 分离；旧事件不推进新尝试；新投影不复用 runtime v4；迁移不产生双写权威；远程 StopExecution 明确不支持，服务停机或断线不证明远端停止。依据 Spec 02 的执行接口、实体、存储与迁移章节。
- **交接门槛**：接口接入前需明确公共类型、记录版本、迁移负责范围和消费者。它与 EXEC-BACKENDS/CONVERSATION/INTERVENTION 不是按文件拆开就能安全并行的工作。

### REMOTE-WORKFLOW

- **范围与修改面**：Orca supervised 接入、分步骤派发、远端问答/输出/对账及交付核验；接入已有 verify 校验器并验证 P13；复用中心状态机和 UI。
- **输入 → 输出**：实测协议、明确限制、中心模型和独立校验器 → 满足生产门槛的受限远程最小流程。
- **关键约束**：固定 base、执行身份与已测版本；发送前限制消息大小；预检和 viewer 明示远程取消不可用，不发送 stop 或按终端句柄兜底，保留活跃资源占用并要求人工处理；正常结算后可 release。依据 Spec 03 的范围、派发、转换和故障验收章节。
- **交接门槛**：中心将已接受的 worker_done 身份及执行端 HEAD 交给校验器，拒绝无效结果推进审查/发布，并核验结果工作区；若使用无 agent 脚本步骤，需实现已测的远端文件回执与重启后新终端取证路径。真实远端与本机回归分别通过；保留隔离身份和工作区，不将 Task 与唯一 Dispatch 固化为公共约束。

### TASK-CORE

- **范围与修改面**：原生 Task/TaskContract/来源映射、SQLite 扩展、查询与旧历史关联。
- **输入 → 输出**：EXEC-MODEL 的稳定身份和事务 → 无外部任务 ID 也可成立的原生任务模型。
- **关键约束**：Beads 为兼容来源，Task/WorkflowRun/Execution 身份继续分离；不实现父子聚合和拆分状态。依据 TASK-CORE 独立 Spec C1–C4 及共同约束。
- **交接门槛**：按 TASK-CORE 独立 Plan 完成 NT01–NT03、NT21-CORE；向 TASK-DAG 固定 ID、契约和事务边界，向 BEADS-EXIT 固定外部映射与旧记录语义。

### TASK-DAG

- **范围与修改面**：批次草稿校验/发布、硬依赖、ready 查询、事务领取、交付释放和恢复。
- **输入 → 输出**：原生任务契约与中心执行事实 → 同项目固定 DAG 及可解释的执行资格。
- **关键约束**：所有前置交付后才能领取；发布后不改依赖；无 split/rollback/reopen、动态重规划或聚合验收。拆分和回退均由用户人工处理。依据 TASK-DAG 独立 Spec D1–D5。
- **交接门槛**：按 TASK-DAG 独立 Plan 完成 NT04–NT11、NT21-DAG；向 BEADS-EXIT 和 DAG-VIEW 交接 Query/Command 与错误语义；原生项目生产启用仍等待 BEADS-EXIT。

### BEADS-EXIT

- **范围与修改面**：配置/check、worker 环境、worktree 核验、prompt/Schema、技能与 completion、viewer 来源查询，以及项目导入切换。
- **输入 → 输出**：原生 DAG 和已审阅项目契约 → 无 bd/bv 的完整开发交付路径与一次性迁移。
- **关键约束**：旧活动执行和回调先结清；未知历史完成事实及特殊聚合/动态后继不自动映射；导入事务只切换一次权威，不长期同步，不提供切换后自动回退。依据 BEADS-EXIT 独立 Spec B1–B4。
- **交接门槛**：按 BEADS-EXIT 独立 Plan 完成 NT12–NT17、NT21-EXIT；真实 PR 合并核验到后继就绪闭环及 local/Orca 各自能力回归；向 DAG-VIEW 提供稳定来源映射及原生任务态。

### DAG-VIEW

- **范围与修改面**：图查询、TUI 关系详情、DOT/JSON 和可选 Graphviz SVG 导出。
- **输入 → 输出**：固定版本的任务/边/阻塞原因快照 → 可读的前置后继关系与交付追踪。
- **关键约束**：只读，展示时间和版本，过滤不隐藏依赖缺口；不增加拆分、回退、拖线编辑或实时 Web 控制台。依据 DAG-VIEW 独立 Spec V1–V3。
- **交接门槛**：按 DAG-VIEW 独立 Plan 完成 NT18–NT20、NT21-VIEW；真实图与列表同一版本一致，布局故障不影响调度。

## 共享修改职责与待确定项

以下是跨目标规划需要明确的事项，不代表已决定并行实施：

1. **公共接口演进**：在 EXEC-BACKENDS 与 EXEC-MODEL 重叠前，指定本轮负责修改公共接口的 Spec，固定契约版本/字段/错误语义及消费者，另一方不得另建同义接口。
2. **存储切换**：在 CONVERSATION、INTERVENTION 接入期间确定是否已有 SQLite 权威、扩展记录版本和迁移负责方；不得同时引入两种恢复来源。
3. **Orca 输出能力**：确认 terminal v2 与 supervised 各自可提供的 turn/phase/replay 证据；缺口影响 CONVERSATION、CONSOLE 和 REMOTE-WORKFLOW，不能通过放宽其中一个界面验收掩盖。
4. **合并与集成顺序**：即使后来允许基于接口并行开发，也要为每次组合确定真实集成测试和合并前置。接口变化须重新评估全部消费者，不能仅更新摘要继续执行。
5. **原生任务接替**：按 TASK-CORE 和 BEADS-EXIT 各自的 Spec/Plan 核对实际接口与版本，TASK-CORE/BEADS-EXIT 接替权威、技能和结项；复用 EXEC-MODEL 单写者，不产生第二套事务或恢复路径。
6. **任务图展示**：TASK-DAG 定义图查询，DAG-VIEW 负责布局；规模和失败预算按 DAG-VIEW Spec V2，算法库/渲染版本分别在 TASK-DAG 与 DAG-VIEW 的独立 Plan 中固定并留证，Web 交互图另行选入规划。

## 后续仍依赖的已有交接信息

- [统一观察契约](../spec/pr-01-observation.spec.md)：CONVERSATION、CONSOLE 使用选择/cursor/generation 与 raw 兼容语义；两个消费者不再依赖该阶段交接信息后移除此项。
- [服务控制协议](../spec/pr-02-control.spec.md)：EXEC-BACKENDS、CONSOLE、INTERVENTION 使用请求/实例身份、准入、回执和 blocker 语义；相关交接依赖结束后移除此项。
- [服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：EXEC-BACKENDS 与 CONSOLE 复用连接分类、startup/instance 身份、Windows `window-required` 离线结果和固定启动入口；Windows 服务窗口默认 manual，关闭 viewer 不停服，控制 stop、Ctrl+C 与关窗各有不同的结果证明。Linux detached 路径尚无原生会话生命周期实测；涉及进程所有权、Stop/Close 的改动需保持已验收的 Windows 行为。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：EXEC-MODEL、REMOTE-WORKFLOW 复用固定身份、请求对账、同工作区跨步骤和重启恢复语义；CONVERSATION 复用 source_changed 与历史缺口映射，精确会话能力另行验证。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：REMOTE-WORKFLOW 复用身份、检查回执、退出码和候选 commit 的校验规则；P13 仍需中心接入后的端到端证据。

目标完成后移出本表；仍有后续消费者时只保留必要交接，直到依赖结束。这里不维护执行进度、文档形成过程或 PR 状态。
