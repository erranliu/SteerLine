# 已规划 Spec 的执行依赖

这里列出 [主 roadmap](roadmap.md) 中尚未完成的中心 Task、执行 Spec 与硬前置。#3 的 H-A6/H-A16 和 #4 的当前范围 P01–P13 已有验收回执，仍需各自原 Task 的真实 Delivery；#6 保留当前目标受影响故障路径和 viewer 步骤转换门槛，长期无人值守归 #32。具体证据见各行引用。

下文的 `#数字` 均指任务 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成范围与直接依赖

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) | 实现及 H-A1–H-A16 受测范围已验收；活跃真实 worker 的同句柄新 incarnation 入口注入验证 fail-closed，平台自然发生该现象不作为关闭前置 | 原中心 Task 的真实 Delivery；直接本机 Codex 续跑仍 unsupported，发送回执不能代替答案应用证明 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [P13 回执](../validation/spec01-p13-steerlinetest-2026-09-28/README.md) | 当前版本族与目标的 P01–P13 已验收；未受变更影响的探测不重跑 | 原中心 Task 的真实 Delivery；远程取消仍 unsupported，新目标或版本族定向认证 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [验收记录](../validation/spec03-native-dag-2026-09-27.md) | 已交付的 `#3` 决策接口与 `#5` 模型、存储和执行接口；`#4` 已实测协议及限制 | 原生任务 103 已在远端五步发布、真实合并、中心结项并解锁 104；当前目标的六项必需受监管能力已有认证。台式机64g 已完成原生五步及固定 gh 发布闭环，允许按 [Spec 03 MVP 边界](../spec/03-remote-execution-and-viewer.md#有人值守-mvp-交付边界) 人工值守使用；新主机受影响故障路径及 4 秒步骤转换 viewer 样本仍为 #6 条件；长期无人值守由 #32 验收。远程取消保持不可用。 |

控制台与原生任务关系视图、#3 的决策与恢复实现均已交付代码；#3/#4 的验收证据已收齐，但中心 Task 仍需真实 Delivery。#6 复用单一原生中心权威接入远程执行，按当前目标剩余门槛验收。

## 跨链路的接口与并行条件

- **#6 与当前执行接口的共享修改**：Service、配置、worker 上下文、Query/Command、Task DTO 和测试夹具存在共同修改面。独立协议样例可并行；涉及共同接口的修改串行集成，后集成方校验接口并运行受影响回归。`#6` 只接 native，不增加既有运行时任务的可变依赖。
- **#3/#4 与 #6 的交付资源**：#3/#4 的正常 Task Delivery 和 #6 的定向验收共用 Service、SQLite、CLI 构建或 Orca 宿主时串行集成；#6 已测调用路径变更时只重跑受影响项。
- **#3 的原生验收**：绑定答案应用、同 attempt 恢复、回执丢失、活跃 worker 身份变化拒绝、可执行恢复确认和 SteerlineTest Task 104 精确 turn/final 折叠均有回执，见[验证记录](../validation/pr-05-intervention.md)。早期失败任务保持原历史；95012 的下游 publish unknown 不算 Task Delivery。
- **#4 与本机工作**：新目标或受影响路径的定向远端探测可与 `#3` 的本机工作并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 `#1` 的精确 turn/phase 及 `#3` 的应用证明验收不代替远端能力证据；远端分页/恢复证据也不代替远端精确会话验证。
- **#6 复用已交付的原生接入**：远程闭环沿用 `#9` 的中心任务权威、固定执行上下文和恢复接口；远端生产能力仍须按 `#4`/`#6` 的独立门槛验收，不扩大版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [执行控制台与轮次折叠](../spec/pr-04-console.spec.md)及[验收报告](../validation/pr-04c-console.md)：#3 的决策卡片、UI 状态与精确折叠已有受测证据；#6 复用相同的查询与能力展示边界。
- [双执行后端](../spec/pr-04a-execution-backends.spec.md)与[Windows 验收报告](../validation/pr-04a-execution-backends.md)：`#3` 的本机 Orca 验收和 `#6` 复用固定 ExecutionRef、Operation、能力报告、ReadOutput 及双后端操作边界；会话 phase/turn、资源结算与 Linux 原生验收不在该后端 Spec 的交付范围内。
- [会话事件](../spec/pr-04b-conversation-events.spec.md)与[本机验收报告](../validation/pr-04b-conversation-events-2026-09-24.md)：`#3`/`#6` 复用统一会话 DTO、完整正文、分页、来源缺口与版本化能力报告；精确 turn/phase/final 仅在报告列明的本机 Codex/Orca 范围内通过。
- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#3`/`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待 `#6` 剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：#6 复用已验收的 SQLite CommandService、固定答案和应用证明；#3 仍待原中心 Task Delivery。
- [原生项目接入](../spec/beads-exit.spec.md)与[验收记录](../validation/beads-exit/README.md)：`#6` 沿用固定 worker 上下文，仍按独立远端门槛验收。
- [统一观察契约](../spec/pr-01-observation.spec.md)：#3 沿用已测选择、游标、来源换代及 raw 读取；未来受影响平台版本按能力重新核验。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：#3 沿用请求/实例身份、回执、连接分类和停止确认；关闭控制台不停服，关闭服务窗口结束服务。Linux detached 原生会话生命周期仍未实测，不属于当前 Windows #3 关闭前置。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、问答和恢复语义；远端精确会话能力仍须独立验证，不能从本机 `#1` 的验收外推。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入中心；P13 正负向真机 verify 回执及任务 103 的三项实际检查证明固定候选门槛，未测无效结果仍按各自条件核对。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。

## 第一阶段新增范围与第二阶段的交接

[第一阶段版本交付方案](phase-1-delivery.md)的新增范围已落实为独立规格和中心任务；下表依赖由中心创建及补边回读核验。#3/#4 的正常交付与 #6 的目标定向验收可在独立资源推进，共用 Service/SQLite、构建产物或 Orca 宿主的工作串行协调。固定 TaskContract 的 Git 引用不因本图状态更新而改变。

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | `PHASE1-CANDIDATES` · [Spec](../spec/task-candidates.spec.md) · [Plan](../spec/task-candidates.plan.md) | 无未完成硬前置 | 交付精确候选身份/Query/Command；multiple 发布在 #27 仲裁交付前受控拒绝 |
| 27 | PR 胜出与落选候选对账 | `PHASE1-CANDIDATES` · [Spec](../spec/candidate-delivery.spec.md) · [Plan](../spec/candidate-delivery.plan.md) | `#26` | 唯一交付、在途冻结、迟到冲突和 closed 后资源对账通过 |
| 28 | 候选观察界面 | `PHASE1-CANDIDATES` · [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) | `#27` | 真实 Windows TUI 展示/输入和中心命令身份通过；跨主机 agent 证据不在本任务代验 |
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

图中关系均已存在于中心 DAG；各任务的真实交付仍由中心 PR 合并核验决定。

## 第二阶段硬前置与并行集成

本组公共契约随独立规格固定，C 等草案字母不再承担依赖身份。表中列出直接硬前置，传递前置由图继承；已交付基础见各 Spec。没有依赖边的任务可独立开发，但服务的实际执行并发仍受现有槽策略约束。

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | `PHASE2` · [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) | 无未完成硬前置 | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 15 | WinUI 基座与原生输入 | `PHASE2` · [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 无未完成硬前置 | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
| 16 | 本机客户端协议与历史查询 | `PHASE2` · [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | `#26`（中心追加） | 本 Spec 全部验收通过；共享接口有变更时同步消费者和受影响回归。 |
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
