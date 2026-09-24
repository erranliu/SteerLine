# 已规划 Spec 的执行依赖

这里直接列出 [主 roadmap](roadmap.md) 中尚未交付的执行 Spec，以及它们的开工、集成和验收前置。规划 ID `#4` 已有部分真实探测，但剩余门槛尚未完成，因此仍在表中。已交付的 Spec 只在后文保留仍被使用的交接信息。执行范围和门槛以各 Spec/Plan 为准；本表用于安排顺序，不能放宽其中的要求。

下文的 `#数字` 均指表中规划 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成 Spec 与直接依赖

| 规划 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 0 | 双执行后端 | `EXEC-BACKENDS` · [Spec](../spec/pr-04a-execution-backends.spec.md) · [Plan](../spec/pr-04a-execution-backends.plan.md) | 无未完成任务前置；复用后文已交付的服务生命周期与控制协议 | 完成 local Codex 与 Orca 共同契约和原生验收；先于 `#1` 合入。Windows 验收沿用可见服务窗口，不把 viewer 退出当作关窗。 |
| 1 | 统一会话事件与完整回复 | `CONVERSATION` · [Spec](../spec/pr-04b-conversation-events.spec.md) · [Plan](../spec/pr-04b-conversation-events.plan.md) | `#0` 验收并合入 | 复用固定 ExecutionRef、ReadOutput 和能力报告；取得两来源真实 turn/phase/final 证据，缺口按 Spec 保留，不能用执行级展示宣告精确体验完成。 |
| 2 | 执行控制台与轮次折叠 | `CONSOLE` · [Spec](../spec/pr-04-console.spec.md) · [Plan](../spec/pr-04-console.plan.md) | `#1` 验收并合入 | 复用会话 DTO、分页、缺口和晚到 final 语义；真实 UI 集成后交付给 `#3`，Windows 离线时显示固定服务窗口入口。 |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) | `#2` 验收并合入；同时复用 `#0`/`#1` 契约 | 决策、续跑、恢复与 UI 共用固定身份和应用证明；若 `#5` 已先合入，复用其 SQLite/CommandService 权威。 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) | 无未完成任务前置；复用已有真实远端报告与字段映射，剩余探测使用隔离环境 | 补测 P10 消息大小/宿主权限边界；P13 必须在 `#6` 接入中心校验器后作端到端验收。P13 不构成 `#6` 开发前的循环前置。 |
| 5 | 中心执行模型 | `EXEC-MODEL` · [Spec](../spec/02-execution-model-and-adapter.md) | `#4` 已有的真实协议映射和明确能力限制，足以设计模型与受限适配器；剩余 P10/P13 不阻止独立模型开发 | 交付 WorkflowRun/Execution/StepAttempt、SQLite、迁移和共同接口后，供 `#6` 与 `#7` 使用。与 `#0`/`#1`/`#3` 重叠的接口或存储按先落地的权威实现扩展，不形成第二套适配器、会话模型或业务写者。 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) | `#5` 的模型、存储、迁移和执行接口；`#4` 已实测协议及限制 | 受限适配器可据已有证据开发；生产启用前须完成 `#4` 剩余必需门槛、P13 中心端到端验证及本任务验收。与 `#0`/`#1` 共用执行和会话契约、与 `#2`/`#3` 共用 UI 和决策边界；后落地者扩展先落地的权威实现。远程取消保持不可用。 |
| 7 | 原生任务模型 | `TASK-CORE` · [Spec](../spec/task-core.spec.md) · [Plan](../spec/task-core.plan.md) | `#5` 的 Task/WorkflowRun 身份、SQLite、单写者、Query/Command 与 Operation/outbox 已交付 | 在中心权威上扩展原生 Task/契约/来源映射；向 `#8` 交接身份、事务和查询版本。不要求 `#6` 或独立 `#0`/`#1` 先于 `#5` 落地。 |
| 8 | 固定依赖调度 | `TASK-DAG` · [Spec](../spec/task-dag.spec.md) · [Plan](../spec/task-dag.plan.md) | `#7` 完整验收并合入 | 交付发布、依赖、ready/claim、交付释放与图查询；生产无 Beads 项目接入仍待 `#9`。 |
| 9 | 解除 Beads 依赖与项目迁移 | `BEADS-EXIT` · [Spec](../spec/beads-exit.spec.md) · [Plan](../spec/beads-exit.plan.md) | `#8` 完整验收并合入；项目接入契约可用 | 完成新项目无 Beads 闭环与旧项目一次性迁移，结清待迁移项目的旧活动执行/回调。复用 `#5` 的中心模型和已支持执行后端；不等待 `#6` 整体完成。 |
| 10 | 任务关系视图 | `DAG-VIEW` · [Spec](../spec/dag-view.spec.md) · [Plan](../spec/dag-view.plan.md) | `#9` 验收并合入；`#8` 的图查询及 `#2` 的控制台查询边界已交付 | 用同一版本任务快照完成关系页和只读 DOT/JSON、可选 SVG；不增加任务修改入口。 |

控制台链按 **#0 → #1 → #2 → #3** 串行交付；原生任务链按 **#5 → #7 → #8 → #9 → #10** 串行交付。`#6` 在 `#5` 之后接入远程执行，但不是 `#7` 的前置。`#10` 还需要 `#2` 的查询边界。每个原生任务使用自己的 Spec、Plan 和实现 PR，不能将几项合在一个交付中。

## 跨链路的接口与并行条件

- **#0/#1/#3 与 #5**：它们共享执行身份、Operation、会话事件、决策及存储迁移，没有预设的单向硬前置。独立设计和测试可推进；进入重叠实现前须确定公共类型、版本、修改职责与集成顺序。先落地者提供权威接口，后落地者扩展它，不双写文件与 SQLite。
- **#4 与本机工作**：剩余远端探测可与不依赖结果的控制台和中心模型工作并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 Orca 的输出样本不代替远端能力证据；远端分页/恢复证据也不代替 `#1` 的精确 turn/phase 验证。
- **#6 与 #9**：两者共享派发上下文、Task 来源、技能版本、完成和恢复接口，但没有互为硬前置。Beads 来源继续走中心现有结项；native 来源在 `#9` 交付后复用固定 DAG 事务。后落地的一方接入已有权威实现，不扩大远端版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [统一观察契约](../spec/pr-01-observation.spec.md)：`#1`/`#2` 复用选择、游标、来源换代及 raw 读取。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：`#0`/`#2`/`#3` 复用请求及实例身份、回执、连接分类和停止确认。Windows 服务窗口默认 manual，关闭 viewer 不停服；关闭服务窗口结束服务。Linux detached 路径尚无原生会话生命周期实测。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#5`/`#6` 复用已实测派发、身份、问答和恢复语义；`#1` 的精确会话能力仍需独立验证。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：`#6` 接入后核验 worker_done、检查回执及执行端 HEAD；纯校验器测试不等于 P13 中心端到端通过。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。
