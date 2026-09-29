# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 当前推进目标

当前优先推进 #36 的持续 worker 与统一待决策。其余 21 项已登记任务的裁减、合并和目标依赖见 [#36 后的 Roadmap 精简审查](worker-flow-review.md)：建议取消 #33、吸收 #34 必要部分、合并 #26/#27、后移 #35，并使本机 API 的后继规格直接消费 #36。以下活动表继续保留待处置的旧任务；表内已登记任务的硬前置是当前中心事实，不代表新推荐路线已经生效。

有人值守[远程执行 MVP](../remote-execution.md)已在受测范围交付。#3/#4/#6 的结项证据分别见[人工决策与协议](../validation/phase1-task-closure-2026-09-28/README.md)、[远程闭环](../validation/spec03-task6-closure-2026-09-28/README.md)。固定 stable 24 小时及受控提升目标保留；旧任务必须取得正式处置回执后才能移出活动规划。

## 执行流程精简

worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。中心围绕原会话处理启动、反馈、待决策和真实 PR 交付，移除项目逻辑验证与五步编排。本文登记规划，不表示新流程已实现或该任务已在运行时中心创建。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 36 | 单 worker 持续执行与统一待决策 | `WORKER-FLOW` | 五个主阶段、原会话持续反馈、移除中心项目验证、PR 观察与真实交付、当前格式恢复和双后端验收 | 无未完成硬前置；共享规格及集成冲突见执行图 | [Spec](../spec/worker-flow.spec.md) · [Plan](../spec/worker-flow.plan.md) |

#36 与 #33/#34 保持旧五步行为的规格存在冲突，需要先协调再串行集成；#26/#27/#35 与 #28/#16/#21 分别涉及交付和查询/UI 接口。已有中心 TaskContract 和依赖继续保持，不通过编辑 roadmap 自动重绑定或补边。#36 合入规划后，按固定 Spec/Plan commit 另行完成中心任务登记；旧运行的自动迁移不在该任务范围内。

## 已登记的模块重构任务：待规划处置

下表保留当前固定的[模块重构规格](../spec/modular-refactoring-constraints.md)。#33 的主要规则将由 #36 删除，#34 的必要上下文工作由新流程承担，#35 的全面搬包建议后移；不再把先完成全部旧步骤重构作为新的推进建议。任务尚未被中心丢弃，不移除身份或伪造完成。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 33 | 执行规则包提取 | `PHASE1-MODULARITY` | 独立 workflow 纯规则包、生产调用接线、行为与依赖检查 | 无未完成硬前置 | [Spec](../spec/workflow-rules.spec.md) · [Plan](../spec/workflow-rules.plan.md) |
| 34 | 执行上下文封装与服务职责收缩 | `PHASE1-MODULARITY` | 固定上下文、显式事件/操作路由、Service 职责与状态所有权 | `#33` | [Spec](../spec/execution-context.spec.md) · [Plan](../spec/execution-context.plan.md) |
| 35 | 中心命令查询与SQLite边界封装 | `PHASE1-MODULARITY` | center 包、业务事务与只读查询、迁移全部 SQL 旁路及消费者 | `#27` | [Spec](../spec/center-boundary.spec.md) · [Plan](../spec/center-boundary.plan.md) |

本组现有中心执行顺序为 #33 → #34 → #26 → #27 → #35，再由 #28/#16 消费；与 #36 的新流程语义需按上文协调，不把该顺序解释为 #36 必须先实现旧五步门槛。三项任务和全部前置已进入中心 DAG；原有依赖及任务固定契约保留。[任务创建记录](../validation/modular-refactoring-task-creation/README.md)保存创建、补边与完整回读证据。

## 第一阶段版本交付目标与推进顺序

[第一阶段版本交付方案](phase-1-delivery.md)汇合已完成的 #3/#4/#6 Task Delivery，以及同任务多候选、stable/dev 双入口和最终集成验收。下列任务已有独立 Spec/Plan 并进入中心 DAG；执行遵守[第一阶段共同约束](../spec/phase1-delivery-constraints.md)，原 TaskContract 固定引用与硬前置不因文档状态更新而改写。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | `PHASE1-CANDIDATES` | 同 Task 独立候选、固定 base、精确命令/查询与恢复；仲裁交付前限制 multiple 发布 | `#34`（中心追加的开工约束） | [Spec](../spec/task-candidates.spec.md) · [Plan](../spec/task-candidates.plan.md) |
| 27 | PR 胜出与落选候选对账 | `PHASE1-CANDIDATES` | 候选独立 PR、唯一 TaskDelivery、迟到合并与落选资源对账 | `#26` | [Spec](../spec/candidate-delivery.spec.md) · [Plan](../spec/candidate-delivery.plan.md) |
| 28 | 候选观察界面 | `PHASE1-CANDIDATES` | TUI 候选选择、独立会话/问题/PR 和未结算资源观察 | `#27`、`#35`（后者为中心追加） | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 29 | 多候选本机与远端真机验收 | `PHASE1-CANDIDATES` | 两后端双候选真实交付、后继链与故障恢复证据 | `#28`；`#3`、`#4`、`#6` 已交付 | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 30 | 固定版本与双入口 | `PHASE1-RUNTIME` | 不可变版本清单、stable/dev 固定入口及构建/home/channel 身份核验 | 无未完成硬前置 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 31 | dev 隔离与 stable 提升 | `PHASE1-RUNTIME` | 独立 home/仓库、受控选择与当前状态恢复；未知执行阻止提升 | `#30` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 32 | 第一阶段集成验收 | `PHASE1-DELIVERY` | 固定 stable 构建的两后端闭环、24 小时真实运行、dev 并行与受控提升 | `#29`、`#31` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

推荐 #36 与不冲突的 #30、#14/#15 使用隔离资源准备。候选创建和交付拟合并，#28/#29 收窄为候选增量，具体裁减见[精简审查](worker-flow-review.md)。当前中心仍保留 #34 → #26、#26/#35 → #16、#35 → #28、#32 → #24 等既有边；[追加前置能力](../spec/native-task-add-predecessors.spec.md)不能删除它们或替换任务内容。取得重新规划和处置回执前，不绕过旧资格启动受影响任务。[中心创建与回读证据](../validation/phase1-task-creation/README.md)记录既有命令事实。

## 第二阶段：Windows 原生 UI 与受控规划

第二阶段交付 Windows 原生 UI 和人与 Codex 主 agent 的受控规划闭环，沿用第一阶段业务权威与[共同接口边界](../spec/phase2-constraints.md)。各任务以独立 Spec/Plan 为实施依据，范围与硬前置如下；阶段区别见[第二阶段说明](../phase-2/parallel-roadmap.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | `PHASE2` | 固定 Codex/app-server 能力、权限和受控工具的 Windows 真机证据 | 无未完成硬前置 | [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) |
| 15 | WinUI 基座与原生输入 | `PHASE2` | WinUI 原生窗口、导航、IME/多行输入及本地视图状态 | 无未完成硬前置 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) |
| 16 | 本机客户端协议与历史查询 | `PHASE2` | 回环认证、执行 Query/Command、SSE 补读和离线只读查询 | `#26`、`#35`（中心追加的开工约束） | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) |
| 17 | 规划提案与受控草稿 | `PHASE2` | 固定规划 worktree、提案/ID 预留、共享写入 guard、草稿与文档读模型 | `#16` | [Spec](../spec/phase2-planning-drafts.spec.md) · [Plan](../spec/phase2-planning-drafts.plan.md) |
| 18 | Codex 规划会话服务 | `PHASE2` | 真实 thread/turn、规范事件、审批和恢复；复用统一写入 guard | `#14`、`#17` | [Spec](../spec/phase2-planning-sessions.spec.md) · [Plan](../spec/phase2-planning-sessions.plan.md) |
| 19 | 正式文档发布与任务创建 | `PHASE2` | 纯文档/任务批次发布、完整版本可见性、规格进入目标分支及 Git/中心对账 | `#17` | [Spec](../spec/phase2-document-publish.spec.md) · [Plan](../spec/phase2-document-publish.plan.md) |
| 20 | Roadmap 与 DAG 原生浏览 | `PHASE2` | 正式 Roadmap/目标/版本和 DAG 关系的真实原生浏览 | `#15`、`#19` | [Spec](../spec/phase2-roadmap-ui.spec.md) · [Plan](../spec/phase2-roadmap-ui.plan.md) |
| 21 | 执行与待处理原生界面 | `PHASE2` | 任务/运行/决策/PR 的原生观察、操作与原请求恢复 | `#15`、`#16` | [Spec](../spec/phase2-execution-ui.spec.md) · [Plan](../spec/phase2-execution-ui.plan.md) |
| 22 | 规划对话原生界面 | `PHASE2` | 真实规划对话、新 session、流式消息、追加/中断及受控草稿工具 | `#15`、`#18` | [Spec](../spec/phase2-conversation-ui.spec.md) · [Plan](../spec/phase2-conversation-ui.plan.md) |
| 23 | 文档审阅与确认原生界面 | `PHASE2` | 预留、整套差异、人工审阅、固定版本确认与发布回执 | `#15`、`#19` | [Spec](../spec/phase2-review-ui.spec.md) · [Plan](../spec/phase2-review-ui.plan.md) |
| 24 | 第二阶段完整闭环验收 | `PHASE2` | 两条后端的真实依赖链、人工合并、交付回到规划及跨组件故障验收 | `#20`、`#21`、`#22`、`#23`、中心追加的 `#32`；`#3`、`#4`、`#6` 已交付 | [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) |

目标规划让本机 API 的后继规格在 #36 后承载单会话执行查询/反馈，不等待多候选或全面 SQL 搬包。规划草稿、主 agent 会话、文档发布及页面的业务边界保留；#21 删除旧步骤恢复与应用证明 UI，#24 改验持续 worker 产品链。替换旧 #16 会影响 #17 等传递后继，须整体核对固定依赖后重新登记；当前表中旧边仍有效。#14/#15 可独立准备；共享资源及现有图见执行依赖图。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
