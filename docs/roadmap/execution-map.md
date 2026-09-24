# 规划依赖与并行关系

本表覆盖已经进入 [主 roadmap](roadmap.md) 的目标，包括尚未形成 Spec 的目标。产品计划中尚未选入的方向不在这里排执行顺序。条目与目标标识对应，Spec 链接提供契约来源；本表不能直接用于执行。

## 如何阅读

- **硬前置**：必须取得前置实现及相应验证结果，才能完成依赖它的工作。
- **接口前置**：输入输出、身份、错误与版本语义固定后，可规划基于契约的独立开发；真实集成仍需前置实现。
- **共享冲突**：双方修改同一业务权威、公共接口、schema 或平台资源，必须先指定修改职责与集成顺序。
- **规划假设**：并行机会或未确定的分工，不改变已有 Spec。没有明确接口/职责时，只能推进不依赖它的独立研究，不能据此启动冲突实现。

当前控制台 Spec/Plan 的交付顺序仍为生命周期 → 4A → 4B → 4C → 5，前一阶段验证并合入后才开始下一阶段。下文的接口并行机会供后续重规划使用；采用前必须修订受影响的 Spec/Plan。

## 目标与契约入口

| 稳定标识 | 目标 | Spec / Plan |
| --- | --- | --- |
| SERVICE-LIFECYCLE | 后台服务生命周期 | [Spec](../spec/pr-03-service-lifecycle.spec.md) · [Plan](../spec/pr-03-service-lifecycle.plan.md) |
| EXEC-BACKENDS | 4A 双执行后端 | [Spec](../spec/pr-04a-execution-backends.spec.md) · [Plan](../spec/pr-04a-execution-backends.plan.md) |
| CONVERSATION | 4B 会话事件 | [Spec](../spec/pr-04b-conversation-events.spec.md) · [Plan](../spec/pr-04b-conversation-events.plan.md) |
| CONSOLE | 4C 执行控制台 | [Spec](../spec/pr-04-console.spec.md) · [Plan](../spec/pr-04-console.plan.md) |
| INTERVENTION | 5 人工介入 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) |
| REMOTE-PROTOCOL | 远程协议验证 | [Spec 01](../spec/01-orca-remote-protocol.md) |
| EXEC-MODEL | 中心执行模型 | [Spec 02](../spec/02-execution-model-and-adapter.md) |
| REMOTE-WORKFLOW | 分阶段远程闭环 | [Spec 03](../spec/03-remote-execution-and-viewer.md) |

## 依赖与并行总表

| 目标关系 | 类型与关键交接 | 并行判断及集成条件 |
| --- | --- | --- |
| SERVICE-LIFECYCLE → EXEC-BACKENDS | 硬前置：实例身份、启动/停止行为及原生平台验证 | 按现有顺序完成；抽象重构不能同时改写未经验证的生命周期语义 |
| EXEC-BACKENDS → CONVERSATION | 接口前置：ExecutionRef、能力、ReadOutput、取消与来源身份 | 规划机会：接口固定后可独立准备解码/重放样本；真实输出接入需适配器，当前阶段仍串行 |
| CONVERSATION → CONSOLE | 接口前置：会话 DTO、正文分页、游标、完整性与 gap | 规划机会：契约固定后 UI 可基于样本开发；精确折叠的集成验收必须使用两来源真实证据，当前阶段仍串行 |
| EXEC-BACKENDS + CONVERSATION + CONSOLE → INTERVENTION | 接口及集成前置：Reply/Continue、固定 Operation、应用证明与问题展示 | 业务恢复和 UI 不能各自解释“已应用”；当前按控制台完成后实施 |
| REMOTE-PROTOCOL 与本机控制台目标 | 输出映射/能力有共享边界；运行环境可以隔离 | 远端探测与不依赖探测结果的本机工作可独立推进；本机证据不能代替远端，样本/映射变更需共同核对 |
| REMOTE-PROTOCOL → EXEC-MODEL | 协议前置：远端身份、回执、消息和恢复语义 | 领域模型与本地存储的独立部分可先行；外部映射依据真实探测，不将未验证远端能力上线 |
| EXEC-BACKENDS + CONVERSATION ↔ EXEC-MODEL | 共享冲突：执行契约、事件、Operation、权威存储与迁移 | 先确定公共契约的修改职责；先落地者提供权威接口，后者复用。不能各自实现 adapter、会话模型或竞争写者 |
| REMOTE-PROTOCOL + EXEC-MODEL → REMOTE-WORKFLOW | 硬前置：远端必需能力、事务模型和原尝试恢复 | 生产接入须同时满足两个门槛；可先做不宣称真实能力的独立设计 |
| CONVERSATION + INTERVENTION 与 REMOTE-WORKFLOW | 接口/共享冲突：输出来源、决策状态、应用证明与查询 DTO | 复用共同契约；尚未落地的一侧不得成为另一套实现，原生远端验证单独完成 |

## 关键链路卡片

### SERVICE-LIFECYCLE

- **范围与修改面**：后台服务客户端、进程启动、startup/instance 身份和平台生命周期；不引入新 UI。
- **输入 → 输出**：既有控制协议与锁 → 可复用的连接、启动、停止结果及验证证据。
- **关键约束**：关闭 UI 不停止 worker；显式停服核验原实例；未知存活不能启动替代服务。依据 Spec 的 L1 及生命周期验收。
- **交接门槛**：Windows Job/真实终端与其他支持平台行为可核验；EXEC-BACKENDS 接入时保持这些语义。与进程所有权、Stop/Close 改动视为共享冲突。

### EXEC-BACKENDS

- **范围与修改面**：Service、local/Orca 适配器、ExecutionRef、Operation、能力、观察/停止/恢复及版本化绑定。
- **输入 → 输出**：生命周期与原执行路径 → 公共后端契约、固定身份、类型化事实/回执和能力报告。
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

- **范围与修改面**：隔离环境中的能力探测、脱敏样本、协议映射；不实现业务调度器。
- **输入 → 输出**：精确远端/agent/版本 → verified/unsupported/not_tested 能力、Dispatch/消息/请求恢复与输出映射。
- **关键约束**：无共享磁盘假设；CLI help/替身不代验；按原 home runtime/Dispatch 寻址。依据 Spec 01 的前置门槛、寻址及能力矩阵。
- **交接门槛**：EXEC-MODEL 使用实际字段语义，REMOTE-WORKFLOW 的必需能力必须真实验证。探测与开发可以分离，但不能同时操作同一真实测试执行资源。

### EXEC-MODEL

- **范围与修改面**：领域模型、最小状态机、SQLite、迁移、Query/Command 和 outbox；不提前扩大生产并发。
- **输入 → 输出**：共同执行/会话契约与远端映射 → 事务状态机、稳定业务身份和可恢复命令/事件。
- **关键约束**：Task/Execution/Attempt 分离；旧事件不推进新尝试；新投影不复用 runtime v4；迁移不产生双写权威。依据 Spec 02 的实体、存储与迁移章节。
- **交接门槛**：接口接入前需明确公共类型、记录版本、迁移负责范围和消费者。它与 EXEC-BACKENDS/CONVERSATION/INTERVENTION 不是按文件拆开就能安全并行的工作。

### REMOTE-WORKFLOW

- **范围与修改面**：Orca supervised 接入、分步骤派发、远端问答/输出/对账及交付核验；复用中心状态机和 UI。
- **输入 → 输出**：远端必需能力与中心模型 → 可验证的远程最小流程。
- **关键约束**：固定 base 与执行身份；逐步骤门槛；中心停机与取消远端分开；结果/PR 由中心核验。依据 Spec 03 的派发、转换和故障验收章节。
- **交接门槛**：真实远端与本机回归分别通过；为后续并行执行留下隔离身份和工作区，不能将 Task 与唯一 Dispatch 固化为公共约束。

## 共享修改职责与待确定项

以下是跨目标规划需要明确的事项，不代表已决定并行实施：

1. **公共接口演进**：在 EXEC-BACKENDS 与 EXEC-MODEL 重叠前，指定本轮负责修改公共接口的 Spec，固定契约版本/字段/错误语义及消费者，另一方不得另建同义接口。
2. **存储切换**：在 CONVERSATION、INTERVENTION 接入期间确定是否已有 SQLite 权威、扩展记录版本和迁移负责方；不得同时引入两种恢复来源。
3. **Orca 输出能力**：确认 terminal v2 与 supervised 各自可提供的 turn/phase/replay 证据；缺口影响 CONVERSATION、CONSOLE 和 REMOTE-WORKFLOW，不能通过放宽其中一个界面验收掩盖。
4. **合并与集成顺序**：即使后来允许基于接口并行开发，也要为每次组合确定真实集成测试和合并前置。接口变化须重新评估全部消费者，不能仅更新摘要继续执行。

## 后续仍依赖的已有交接信息

- [统一观察契约](../spec/pr-01-observation.spec.md)：CONVERSATION、CONSOLE 使用选择/cursor/generation 与 raw 兼容语义；两个消费者不再依赖该阶段交接信息后移除此项。
- [服务控制协议](../spec/pr-02-control.spec.md)：SERVICE-LIFECYCLE、EXEC-BACKENDS、CONSOLE、INTERVENTION 使用请求/实例身份、准入、回执和 blocker 语义；相关交接依赖结束后移除此项。

目标完成后移出本表；仍有后续消费者时只保留必要交接，直到依赖结束。这里不维护执行进度、文档形成过程或 PR 状态。
