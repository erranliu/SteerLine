# 已规划 Spec 的执行依赖

这里直接列出 [主 roadmap](roadmap.md) 中尚未交付的执行 Spec，以及它们的开工、集成和验收前置。任务 `#4` 已有部分真实探测，但剩余门槛尚未完成，因此仍在表中。已交付的 Spec 只在后文保留仍被使用的交接信息。执行范围和门槛以各 Spec/Plan 为准；本表用于安排顺序，不能放宽其中的要求。

下文的 `#数字` 均指任务 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成 Spec 与直接依赖

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) | 已交付的 `#2` 控制台与 `#0`/`#1` 契约 | 决策、续跑、恢复与 UI 共用固定身份和应用证明；复用已交付的 `#5` SQLite/CommandService 权威。 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) | 无未完成任务前置；复用已有真实远端报告与字段映射，权限提示探测使用隔离环境 | 验证固定 8 KiB 上限的发送前拒绝和宿主权限提示人工处置；P13 必须在 `#6` 接入中心校验器后作端到端验收。P13 不构成 `#6` 开发前的循环前置。 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) | 已交付的 `#5` 模型、存储和执行接口；`#4` 已实测协议及限制 | 受限适配器可据已有证据开发；启用前须完成 `#4` 剩余必需门槛、P13 中心端到端验证及本任务验收。与 `#0`/`#1` 共用执行和会话契约、与 `#2`/`#3` 共用 UI 和决策边界；扩展已有 SQLite 中心权威。远程取消保持不可用。 |
| 9 | 解除 Beads 依赖与原生接入 | `BEADS-EXIT` · [Spec](../spec/beads-exit.spec.md) · [Plan](../spec/beads-exit.plan.md) | 已交付的 `#7` 固定契约与 `#8` 中心 DAG 权威、领取和交付接口 | 完成新项目无 Beads 闭环及真实 worker 启动/恢复、执行端契约核验、停止与 PR/工作区清理回执；旧项目状态拒绝自动导入。复用已支持执行后端，不等待 `#6` 整体完成。 |
| 10 | 任务关系视图 | `DAG-VIEW` · [Spec](../spec/dag-view.spec.md) · [Plan](../spec/dag-view.plan.md) | `#9` 验收并合入；`#8` 的图查询及 `#2` 的控制台查询边界已交付 | 用同一版本任务快照完成关系页和只读 JSON 导出；不增加任务修改入口。 |

控制台链只剩 `#3`；原生任务链的剩余任务按 **#9 → #10** 串行交付。`#6` 复用已交付的 `#5` 接入远程执行，但不是 `#9` 的前置。`#10` 复用已交付的 `#2` 查询边界。每个原生任务使用自己的 Spec、Plan 和实现 PR，不能将几项合在一个交付中。

## 跨链路的接口与并行条件

- **#3 对已交付底座的复用**：`#0` 固定执行身份、Operation 和双后端边界；`#1` 提供会话事件、完整正文及来源能力；`#2` 提供多项目控制台、请求对账、轮次视图和决策只读入口；`#5` 提供 SQLite 中心权威、Query/Command 和本地流程。`#3` 扩展同一执行、会话与决策边界，不另设业务写者或独立恢复来源。
- **#4 与本机工作**：剩余远端探测可与不依赖结果的控制台和原生任务调度工作并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 `#1` 的精确 turn/phase 验收不代替远端能力证据；远端分页/恢复证据也不代替远端精确会话验证。
- **#6 与 #9**：两者共享派发上下文、Task、技能版本、完成和恢复接口，但没有互为硬前置。原生路径在 `#9` 交付后复用固定 DAG 事务；后落地的一方接入已有权威实现，不扩大远端版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [执行控制台与轮次折叠](../spec/pr-04-console.spec.md)及[验收报告](../validation/pr-04c-console.md)：`#3` 复用项目、run/attempt、request_id、决策只读卡片和 UI 视图状态；`#6`/`#10` 复用同一查询与能力展示边界。
- [双执行后端](../spec/pr-04a-execution-backends.spec.md)与[Windows 验收报告](../validation/pr-04a-execution-backends.md)：`#3`/`#6` 复用固定 ExecutionRef、Operation、能力报告、ReadOutput 和双后端操作边界；会话 phase/turn、资源结算与 Linux 原生验收不在该后端 Spec 的交付范围内。
- [会话事件](../spec/pr-04b-conversation-events.spec.md)与[本机验收报告](../validation/pr-04b-conversation-events-2026-09-24.md)：`#3`/`#6` 复用统一会话 DTO、完整正文、分页、来源缺口与版本化能力报告；精确 turn/phase/final 仅在报告列明的本机 Codex/Orca 范围内通过。
- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#3`/`#6`/`#9` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和本地 minimal-v1 流程；远程生产执行仍待 `#6`。
- [原生任务模型](../spec/task-core.spec.md)与[验收记录](../validation/task-core/audit.md)：`#9`/`#10` 复用固定任务身份与契约、当前 SQLite 中心数据库的只读查询和交付证据存储。
- [固定依赖调度](../spec/task-dag.spec.md)与[验收记录](../validation/task-dag/README.md)：`#9` 复用任务入图、硬前置、claim 预留、精确 PR 交付与丢弃、重启对账和图查询；`#10` 复用同一关系读模型。生产 worker 与外部清理回执是 `#9` 的交付门槛。
- [统一观察契约](../spec/pr-01-observation.spec.md)：`#3` 继续复用选择、游标、来源换代及 raw 读取。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：`#3` 复用请求及实例身份、回执、连接分类和停止确认。Windows 服务窗口默认 manual，关闭控制台不停服；关闭服务窗口结束服务。Linux detached 路径尚无原生会话生命周期实测。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、问答和恢复语义；远端精确会话能力仍须独立验证，不能从本机 `#1` 的验收外推。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：`#6` 接入后核验 worker_done、检查回执及执行端 HEAD；纯校验器测试不等于 P13 中心端到端通过。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。
