# 已规划 Spec 的执行依赖

这里直接列出 [主 roadmap](roadmap.md) 中尚未完成的执行 Spec 与验收，以及它们的开工、集成和验收前置。`#3` 已实现但原生验收未完成；`#4` 已有部分真实探测，但剩余门槛尚未完成。已交付的 Spec 只在后文保留仍被使用的交接信息。执行范围和门槛以各 Spec/Plan 为准；本表用于安排顺序，不能放宽其中的要求。

下文的 `#数字` 均指任务 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成范围与直接依赖

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 12 | 移除 Beads 旧执行路径 | `REFACTOR` · [Spec](../spec/beads-path-removal.spec.md) · [Plan](../spec/beads-path-removal.plan.md) | 已交付的 `#11` 测量与行为映射 | 旧执行与专属测试删除，当前行为转到原生测试，旧状态仍安全拒绝；配置、Service 和 worker 上下文与 `#6` 串行集成。向 `#13` 交接单一原生路径。 |
| 13 | 任务图规则拆包与测试分层 | `REFACTOR` · [Spec](../spec/taskgraph-modules.spec.md) · [Plan](../spec/taskgraph-modules.plan.md) | 硬前置 `#12` 合入；使用 `#11` 测量工具 | taskgraph 无父包/I/O 依赖，CLI 集成独立，默认完整入口仍覆盖所有确定性行为；Query/Command 和图协议保持，跨层及受影响消费者回归通过。 |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [剩余门槛](../validation/pr-05-intervention.md) | 决策与恢复实现已交付；剩余验收需要 Windows TTY 样例及隔离注册的本机 Orca 测试仓库 | 完成长问题、12 选项、中文多行答案、旧版本冲突、折叠导航和恢复确认的 TTY 验证；同会话 Orca 路径须提供固定原执行的续跑、原操作恢复和答案应用证明。发送回执不能代替应用证明。 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) | 无未完成任务前置；复用已有真实远端报告与字段映射，权限提示探测使用隔离环境 | 验证固定 8 KiB 上限的发送前拒绝和宿主权限提示人工处置；P13 必须在 `#6` 接入中心校验器后作端到端验收。P13 不构成 `#6` 开发前的循环前置。 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) | 已交付的 `#3` 决策接口与 `#5` 模型、存储和执行接口；`#4` 已实测协议及限制 | 受限适配器可据已有证据开发；启用前须完成 `#4` 剩余必需门槛、P13 中心端到端验证及本任务验收。与 `#0`/`#1` 共用执行和会话契约、与 `#2`/`#3` 共用 UI 和决策边界；扩展已有 SQLite 中心权威。远程取消保持不可用。 |

`#11` 已交付测量工具和夹具精简；后续重构顺序为 `#12 → #13`，分别使用独立 Spec、Plan 和实现 PR。控制台与原生任务关系视图的实现已交付；`#3` 只剩原生验收。`#6` 复用单一原生中心权威接入远程执行，远端生产能力仍按自身门槛验收。`#4`、`#6` 各自沿用独立实现范围；本机决策验收不替代远端能力验收。

## 跨链路的接口与并行条件

- **重构链的硬前置与接口交接**：`#12` 复用已交付的 `#11` 基线、工具和用例行为映射来清理旧执行路径；`#13` 在 `#12` 合入后抽出纯图算法和 CLI 集成测试。独立样例可提前准备，共享实现须等待前置合入。各任务边界以自己的 Spec 为准，测量不授权扩大为整个存储/Service 重写。
- **重构与 #6 的共享冲突**：Service、配置、worker 上下文、Query/Command、Task DTO 和测试夹具存在共同修改面。独立协议样例可并行；共同修改串行集成，后集成方校验新接口并运行受影响双方回归。`#6` 只接 native，不能为过渡 Beads 路径保留或重建分支；此规则同步在其 Spec/Plan，不增加既有运行时任务的可变依赖。
- **重构与 #3/#4 的验收资源**：本机 TTY/Orca 验收与远端探测可在固定版本和独立资源上继续；共享仓库、CLI 构建产物或平台执行不能被同时改写。重构改变已测调用路径时重跑受影响项，不把此前版本证据直接标为新实现通过。
- **#3 的原生验收**：已实现的决策、续跑和恢复复用 `#0` 固定执行身份、`#1` 会话事件、`#2` 控制台与 `#5` SQLite 中心权威。剩余本机 TTY/Orca 验收须取得真实交互和应用证明；缺少证明时保留应用待核实状态。
- **#4 与本机工作**：剩余远端探测可与 `#3` 的本机验收并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 `#1` 的精确 turn/phase 及 `#3` 的应用证明验收不代替远端能力证据；远端分页/恢复证据也不代替远端精确会话验证。
- **#6 复用已交付的原生接入**：远程闭环沿用 `#9` 的中心任务权威、固定执行上下文和恢复接口；远端生产能力仍须按 `#4`/`#6` 的独立门槛验收，不扩大版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [执行控制台与轮次折叠](../spec/pr-04-console.spec.md)及[验收报告](../validation/pr-04c-console.md)：`#3` 的剩余 TTY 验收沿用决策卡片和 UI 视图状态；`#6` 复用同一查询与能力展示边界。
- [双执行后端](../spec/pr-04a-execution-backends.spec.md)与[Windows 验收报告](../validation/pr-04a-execution-backends.md)：`#3` 的本机 Orca 验收和 `#6` 复用固定 ExecutionRef、Operation、能力报告、ReadOutput 及双后端操作边界；会话 phase/turn、资源结算与 Linux 原生验收不在该后端 Spec 的交付范围内。
- [会话事件](../spec/pr-04b-conversation-events.spec.md)与[本机验收报告](../validation/pr-04b-conversation-events-2026-09-24.md)：`#3`/`#6` 复用统一会话 DTO、完整正文、分页、来源缺口与版本化能力报告；精确 turn/phase/final 仅在报告列明的本机 Codex/Orca 范围内通过。
- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#3`/`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和本地 minimal-v1 流程；远程生产执行仍待 `#6`。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用同一 SQLite CommandService、固定答案与应用证明；`#3` 的真实 TTY/Orca 门槛仍待完成。
- [原生项目接入](../spec/beads-exit.spec.md)与[验收记录](../validation/beads-exit/README.md)：`#6` 沿用固定 worker 上下文，仍按独立远端门槛验收。
- [测试耗时基线与夹具精简](../spec/test-feedback.spec.md)与[验证记录](../validation/test-feedback/README.md)：`#12`/`#13` 复用逐用例报告、覆盖映射和完整无缓存测试入口；原生平台验收仍按各自门槛执行。
- [统一观察契约](../spec/pr-01-observation.spec.md)：`#3` 的剩余验收沿用选择、游标、来源换代及 raw 读取。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：`#3` 的剩余验收沿用请求及实例身份、回执、连接分类和停止确认。Windows 服务窗口默认 manual，关闭控制台不停服；关闭服务窗口结束服务。Linux detached 路径尚无原生会话生命周期实测。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、问答和恢复语义；远端精确会话能力仍须独立验证，不能从本机 `#1` 的验收外推。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：`#6` 接入后核验 worker_done、检查回执及执行端 HEAD；纯校验器测试不等于 P13 中心端到端通过。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。
