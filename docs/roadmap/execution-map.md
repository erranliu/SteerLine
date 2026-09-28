# 已规划 Spec 的执行依赖

这里直接列出 [主 roadmap](roadmap.md) 中尚未完成的执行 Spec 与验收，以及它们的开工、集成和验收前置。`#3` 的实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入 `main`；早期 95008/95009/95010 的 Codex 应用均结算失败且未重放，后续 95012 已在精确 Git 沙箱授权下取得绑定应用证明及同 attempt 重启对账。`#4` 的 P13 中心正负向 verify 已有真机证据，P10 及完整协议仍有剩余门槛。执行范围和判定以各 Spec/Plan 及[最新验证记录](../validation/remaining-gates-2026-09-27.md)为准。

下文的 `#数字` 均指任务 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成范围与直接依赖

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [剩余门槛](../validation/remaining-gates-2026-09-27.md) | 实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入；后续 Git 沙箱与不确定结果的修正及真机证据在当前验证记录中。95008/95009/95010 的失败应用未重放，95011 的失败决策未原地改写 | 用户报告系统剪贴板粘贴通过；stale-revision 与 live `answer -wait` 已验证；95012 的成功应用/同 attempt 重启及 95013 的 resuming 中终端死亡不重派已验证。可执行恢复确认、显式响应丢失和活跃 worker incarnation 注入已通过；精确折叠与平台真实同句柄换代仍待验收。发送回执不能代替应用证明。 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) | 无未完成任务前置；复用已有真实远端报告与字段映射，命令权限提示的真机观察和人工处置已有证据 | home 活跃问题的 8 KiB 发送前拒绝及台式机64g 受控 UAC 原图/人工处置已通过；其余完整协议验收保留；P13 正负向受监管 verify 回执已有证据。 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [验收记录](../validation/spec03-native-dag-2026-09-27.md) | 已交付的 `#3` 决策接口与 `#5` 模型、存储和执行接口；`#4` 已实测协议及限制 | 原生任务 103 已在远端五步发布、真实合并、中心结项并解锁 104；当前目标的六项必需受监管能力已有认证。台式机64g 已完成原生五步及固定 gh 发布闭环，允许按 [Spec 03 MVP 边界](../spec/03-remote-execution-and-viewer.md#有人值守-mvp-交付边界) 人工值守使用；新主机持续无人值守、故障/viewer 复验仍为完整交付条件。远程取消保持不可用。 |

控制台与原生任务关系视图的实现已交付；`#3` 的决策与恢复实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入，只剩原生验收。`#6` 复用当前单一原生中心权威接入远程执行，远端生产能力仍按自身门槛验收。`#4`、`#6` 各自沿用独立实现范围；本机决策验收不替代远端能力验收。

## 跨链路的接口与并行条件

- **#6 与当前执行接口的共享修改**：Service、配置、worker 上下文、Query/Command、Task DTO 和测试夹具存在共同修改面。独立协议样例可并行；涉及共同接口的修改串行集成，后集成方校验接口并运行受影响回归。`#6` 只接 native，不增加既有运行时任务的可变依赖。
- **#3/#4 与 #6 的验收资源**：本机 TTY/Orca 验收与远端探测可在固定版本和独立资源上继续；共享仓库、CLI 构建产物或平台执行不能被同时改写。`#6` 改变已测调用路径时重跑受影响项，不把此前版本证据直接标为新实现通过。
- **#3 的原生验收**：已实现的决策、续跑和恢复复用 `#0` 固定执行身份、`#1` 会话事件、`#2` 控制台与 `#5` SQLite 中心权威。2026-09-26 的实时 TTY stale-revision/Orca command-worker 样例只证明已记录范围；95008/95009/95010 的 Codex continuation 均结算失败且未重放。2026-09-27 新任务 95012 使用精确 Git 目录授权，原终端暂不可核实时保留 `reply_accepted`，确认退出后重启接回同一 attempt 的绑定证明、中心记 Decision.applied；后续 publish unknown 不回退该事实，也不等于任务交付。用户报告 TTY 实际系统剪贴板粘贴已通过；精确轮次折叠、可执行恢复确认和明确故障注入仍须独立验收。95012 会话来源只有 Orca observation/step-result，不能证明精确 turn/final。
- **#4 与本机工作**：剩余远端探测可与 `#3` 的本机验收并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 `#1` 的精确 turn/phase 及 `#3` 的应用证明验收不代替远端能力证据；远端分页/恢复证据也不代替远端精确会话验证。
- **#6 复用已交付的原生接入**：远程闭环沿用 `#9` 的中心任务权威、固定执行上下文和恢复接口；远端生产能力仍须按 `#4`/`#6` 的独立门槛验收，不扩大版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [执行控制台与轮次折叠](../spec/pr-04-console.spec.md)及[验收报告](../validation/pr-04c-console.md)：`#3` 的剩余 TTY 验收沿用决策卡片和 UI 视图状态；`#6` 复用同一查询与能力展示边界。
- [双执行后端](../spec/pr-04a-execution-backends.spec.md)与[Windows 验收报告](../validation/pr-04a-execution-backends.md)：`#3` 的本机 Orca 验收和 `#6` 复用固定 ExecutionRef、Operation、能力报告、ReadOutput 及双后端操作边界；会话 phase/turn、资源结算与 Linux 原生验收不在该后端 Spec 的交付范围内。
- [会话事件](../spec/pr-04b-conversation-events.spec.md)与[本机验收报告](../validation/pr-04b-conversation-events-2026-09-24.md)：`#3`/`#6` 复用统一会话 DTO、完整正文、分页、来源缺口与版本化能力报告；精确 turn/phase/final 仅在报告列明的本机 Codex/Orca 范围内通过。
- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#3`/`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待 `#6` 剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用同一 SQLite CommandService、固定答案与应用证明；`#3` 的真实 TTY/Orca 门槛仍待完成。
- [原生项目接入](../spec/beads-exit.spec.md)与[验收记录](../validation/beads-exit/README.md)：`#6` 沿用固定 worker 上下文，仍按独立远端门槛验收。
- [统一观察契约](../spec/pr-01-observation.spec.md)：`#3` 的剩余验收沿用选择、游标、来源换代及 raw 读取。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：`#3` 的剩余验收沿用请求及实例身份、回执、连接分类和停止确认。Windows 服务窗口默认 manual，关闭控制台不停服；关闭服务窗口结束服务。Linux detached 路径尚无原生会话生命周期实测。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、问答和恢复语义；远端精确会话能力仍须独立验证，不能从本机 `#1` 的验收外推。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入中心；P13 正负向真机 verify 回执及任务 103 的三项实际检查证明固定候选门槛，未测无效结果仍按各自条件核对。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。

## 第一阶段模块重构与交接

本组按[共同约束](../spec/modular-refactoring-constraints.md)划分职责、包依赖和状态所有者。下列为已选定规划；中心创建与追加边的执行状态单列于[创建记录](../validation/modular-refactoring-task-creation/README.md)。当前 #34 → #26、#35 → #28、#35 → #16 尚无中心回执，不能由本图代替生效。

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 33 | 执行规则包提取 | `PHASE1-MODULARITY` · [Spec](../spec/workflow-rules.spec.md) · [Plan](../spec/workflow-rules.plan.md) | 无未完成硬前置 | workflow 纯规则、全部生产接线、错误语义及包依赖检查通过；供 #34 消费 |
| 34 | 执行上下文封装与服务职责收缩 | `PHASE1-MODULARITY` · [Spec](../spec/execution-context.spec.md) · [Plan](../spec/execution-context.plan.md) | `#33` | 显式上下文、单写者、不同任务交错与当前格式恢复通过；拟作为 #26 的追加前置 |
| 35 | 中心命令查询与SQLite边界封装 | `PHASE1-MODULARITY` · [Spec](../spec/center-boundary.spec.md) · [Plan](../spec/center-boundary.plan.md) | `#27` | center 包无 SQL 旁路/反向依赖，事务、查询及恢复回归；向 #28/#16 交接 |

原生创建先在同一批次写入 #33 → #34 和 #27 → #35，再通过追加前置命令提交 #34 → #26、#35 → #28、#35 → #16。#26 已领取时必须满足既有安全重置能力；活动、unknown 或不支持的后端不能补边，不能把原运行改成“已停止”。无法完成的边保持待核实，不自动调整到相反顺序。

代码集成顺序为 #33 → #34 → #26 → #27 → #35 → #28/#16。#30/#31 独立推进；与本组共用 Service/SQLite/启动恢复入口时串行集成并回归此前已交付的边界。#3/#4/#6 在固定版本与隔离资源上继续原验收。

## 第一阶段新增范围与第二阶段的交接

[第一阶段版本交付方案](phase-1-delivery.md)的 #26–#32 已有独立规格和中心任务；原有依赖已回读核验，结构重构新增前置在表中单独标注。`#3/#4/#6` 的剩余原生验收与 `#33/#30` 可使用独立资源推进，#26 遵守新增重构顺序；共用 Service/SQLite、构建产物或真实 Orca 宿主的集成和故障注入串行协调。规格必须先集成到执行目标分支才能按原固定契约开始实现。

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | `PHASE1-CANDIDATES` · [Spec](../spec/task-candidates.spec.md) · [Plan](../spec/task-candidates.plan.md) | 拟追加 `#34`，须先安全重置原活动运行并取得中心回执 | 交付精确候选身份/Query/Command；multiple 发布在 #27 仲裁交付前受控拒绝 |
| 27 | PR 胜出与落选候选对账 | `PHASE1-CANDIDATES` · [Spec](../spec/candidate-delivery.spec.md) · [Plan](../spec/candidate-delivery.plan.md) | `#26` | 唯一交付、在途冻结、迟到冲突和 closed 后资源对账通过 |
| 28 | 候选观察界面 | `PHASE1-CANDIDATES` · [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) | `#27`；拟追加 `#35`，待中心回执 | 真实 Windows TUI 展示/输入和中心命令身份通过；跨主机 agent 证据不在本任务代验 |
| 29 | 多候选本机与远端真机验收 | `PHASE1-CANDIDATES` · [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) | `#28`、`#3`、`#4`、`#6` | 当前认证目标上两后端双候选/后继真实合并及故障恢复均通过 |
| 30 | 固定版本与双入口 | `PHASE1-RUNTIME` · [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) | 无未完成硬前置 | 不可变版本、固定入口与实例身份通过；切换资格由 #31 接入 |
| 31 | dev 隔离与 stable 提升 | `PHASE1-RUNTIME` · [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) | `#30` | home/仓库隔离、受控提升、失败继续和当前格式恢复通过 |
| 32 | 第一阶段集成验收 | `PHASE1-DELIVERY` · [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) | `#29`、`#31` | 固定组合构建真实两链、24 小时运行、dev 并行及提升验收通过 |

第二阶段原有边保留，并已通过[追加前置能力](../spec/native-task-add-predecessors.spec.md)加入 `#26 → #16` 与 `#32 → #24`。`#14/#15` 可同步准备；#16 消费 #26 的身份接口，超出其冻结规格的新 API/WinUI 范围须另有执行契约。中心新边收紧执行资格，原 TaskContract 的 Git 引用保持不变。[创建与回读证据](../validation/phase1-task-creation/README.md)列出原请求和精确快照。

```mermaid
flowchart LR
    T26["#26 多候选模型"] --> T27["#27 PR 仲裁"] --> T28["#28 候选界面"] --> T29["#29 多候选真机验收"]
    T3["#3 决策验收"] --> T29
    T4["#4 协议验收"] --> T29
    T6["#6 远端闭环"] --> T29
    T30["#30 固定版本/双入口"] --> T31["#31 隔离/提升"] --> T32["#32 一阶段集成验收"]
    T29 --> T32
    T26 --> T16["#16 本机协议"]
    T32 --> T24["#24 二阶段完整验收"]
```

上图展示原 #26–#32 已存在的中心关系。模块重构新增关系另见前节；拟追加边须取得回执，各任务的真实交付仍由中心 PR 合并核验决定。

```mermaid
flowchart LR
    R33["#33 执行规则"] --> R34["#34 执行上下文"]
    R34 -. "待安全重置与补边" .-> T26["#26 多候选"]
    T26 --> T27["#27 交付仲裁"] --> R35["#35 中心封装"]
    R35 -. "待补边" .-> T28["#28 候选界面"]
    R35 -. "待补边" .-> T16["#16 本机 API"]
```

本图为选定顺序；#33/#34/#35 的创建边也须以创建回执确认。虚线明确表示尚未提交的追加边。

## 第二阶段硬前置与并行集成

本组公共契约随独立规格固定，C 等草案字母不再承担依赖身份。表中列出直接硬前置，传递前置由图继承；已交付基础见各 Spec。没有依赖边的任务可独立开发，但服务的实际执行并发仍受现有槽策略约束。

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | `PHASE2` · [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) | 无未完成硬前置 | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 15 | WinUI 基座与原生输入 | `PHASE2` · [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 无未完成硬前置 | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 16 | 本机客户端协议与历史查询 | `PHASE2` · [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | `#26`（已追加）；拟追加 `#35`，待中心回执 | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 17 | 规划提案与受控草稿 | `PHASE2` · [Spec](../spec/phase2-planning-drafts.spec.md) · [Plan](../spec/phase2-planning-drafts.plan.md) | `#16` | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 18 | Codex 规划会话服务 | `PHASE2` · [Spec](../spec/phase2-planning-sessions.spec.md) · [Plan](../spec/phase2-planning-sessions.plan.md) | `#14`、`#17` | #14 必须有通过证据，复用 #17 guard；与 #19 的 Service/存储接线串行集成。 |
| 19 | 正式文档发布与任务创建 | `PHASE2` · [Spec](../spec/phase2-document-publish.spec.md) · [Plan](../spec/phase2-document-publish.plan.md) | `#17` | 复用 #17 guard 与正式读模型；与 #18 独立开发，共享接线后合入方回归双方。 |
| 20 | Roadmap 与 DAG 原生浏览 | `PHASE2` · [Spec](../spec/phase2-roadmap-ui.spec.md) · [Plan](../spec/phase2-roadmap-ui.plan.md) | `#15`、`#19` | 使用 #19/#17 已交付的真实版本查询；与另一文档页面可分头开发，共享 WinUI 导航串行集成。 |
| 21 | 执行与待处理原生界面 | `PHASE2` · [Spec](../spec/phase2-execution-ui.spec.md) · [Plan](../spec/phase2-execution-ui.plan.md) | `#15`、`#16` | 未通过的第一阶段能力保持不可用；本任务交付不代替 #24 完整后端验收。 |
| 22 | 规划对话原生界面 | `PHASE2` · [Spec](../spec/phase2-conversation-ui.spec.md) · [Plan](../spec/phase2-conversation-ui.plan.md) | `#15`、`#18` | 真实会话/工具经 #18；不等 #19，但无审阅发布副作用入口。 |
| 23 | 文档审阅与确认原生界面 | `PHASE2` · [Spec](../spec/phase2-review-ui.spec.md) · [Plan](../spec/phase2-review-ui.plan.md) | `#15`、`#19` | 使用 #19/#17 已交付的真实版本查询；与另一文档页面可分头开发，共享 WinUI 导航串行集成。 |
| 24 | 第二阶段完整闭环验收 | `PHASE2` · [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) | `#20`、`#21`、`#22`、`#23`、`#3`、`#4`、`#6`，另有中心追加的 `#32` | 全部前置真实交付；本任务逐项完成两后端和跨组件故障验收，不以样例代替。 |

```mermaid
flowchart LR
    T14["#14 能力验证"] --> T18["#18 规划会话"]
    T26["#26 多候选模型"] --> T16["#16 本机协议"]
    T32["#32 一阶段集成验收"] --> T24["#24 完整闭环验收"]
    T16["#16 本机协议"] --> T17["#17 提案与草稿"]
    T17 --> T18
    T17 --> T19["#19 正式发布"]
    T15["#15 WinUI 基座"] --> T21["#21 执行页面"]
    T16 --> T21
    T15 --> T22["#22 规划对话"]
    T18 --> T22
    T15 --> T20["#20 Roadmap/DAG"]
    T19 --> T20
    T15 --> T23["#23 文档审阅"]
    T19 --> T23
    T20 --> T24["#24 完整闭环验收"]
    T21 --> T24
    T22 --> T24
    T23 --> T24
    T3["#3 决策恢复"] --> T24
    T4["#4 远端协议"] --> T24
    T6["#6 远端闭环"] --> T24
```

- 规划 worktree、提案、ID 预留、正式/草稿读模型与写入 owner/generation 由 #17 单独实现；#18/#19 共同消费，不各建状态或锁。#19 推进正式版本，#20/#23 的真实交付等待它完成。
- #16 只拥有传输、认证和既有执行映射；#17/#18/#19 分别拥有自身业务 handler、DTO 和恢复。共同信封已在 [Spec 共同约束](../spec/phase2-constraints.md)中固定，不能将 #16 的内部里程碑当作放行。
- #18/#19 在 Service、Query/Command、SQLite 接线处串行集成；#20/#21/#22/#23 在 WinUI 导航、连接与输入公共组件处串行集成。独立业务文件可并行开发，后合入方重跑受影响消费者。
- #3/#4/#6 与第二阶段共用测试宿主、二进制、远端资源时分时验收；隔离工作区不隔离全局 CLI 配置、进程或真实 Orca runtime。不得在另一项验收途中替换它使用的构建或重启共享宿主。
- #24 是最终验收的唯一任务归属，不承担各上游任务缺失的实现。各组件交付与完整产品交付分别记录；任务创建时保留上述固定 AND 依赖，不新增部分完成放行机制。
