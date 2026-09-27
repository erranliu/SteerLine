# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 第一阶段遗留：人工决策与恢复的剩余验收

`#3` 的决策与恢复实现已交付，原生验收仍未完成。用户报告 Windows TTY 实际系统剪贴板粘贴通过；这项结果未由验证会话独立观察。答案被服务接受不等于已由 Codex 应用，剩余证据见[验证记录](../validation/pr-05-intervention.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | 验收精确轮次折叠与导航、符合当前步骤资格的恢复确认及指定 Orca 故障注入；解决 Windows `workspace-write` 对 linked-worktree Git 管理目录的写入限制，取得 Codex 成功应用及应用后重启恢复的原生证明 | 已失败的 Codex attempts 保留原身份与结果，不重放；本机证据不替代远端验收 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) |

## 第一阶段遗留：远程协议与原生闭环的剩余验收

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | 完成 P10 宿主权限提示与人工处置的受控验收、固定 8 KiB 消息上限的发送前拒绝验证；保留已取得的 P13 中心正反向结果证据 | 与 `#6` 共用已验证的中心结果校验路径；未完成的能力仍阻止生产启用 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | 完成原生任务来源的远端发布、真实 PR 合并与 TaskDelivery 结项闭环；验收 Windows 可见服务窗口关窗恢复、可见 viewer 展示及资源释放异常；使生产能力报告通过本任务的全部门槛 | 复用已交付的中心模型、决策和原生任务权威；生产启用前完成 `#4` 的剩余必需门槛及本任务验收 | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) |

`#6` 已有受监管的分阶段远端执行、问答、验证、审查、测试 PR 发布与合并、重启和回执丢失恢复证据；P13 的正反向 verify 门槛也已有真实中心回执。这些证据不代替原生任务来源的完整闭环或生产能力认证，当前远端生产领取仍受阻。`#3` 的本机验收、`#4` 的协议门槛和 `#6` 的原生远端验收各自独立。

首版远端按已验证的 Windows Orca 1.4 `major.minor` 版本族预检，保存中心与远端的完整版本；其他版本族拒绝。结构化消息限不高于 8 KiB，超限须在发送前拒绝。生产远程取消不可用；结果或资源状态不明时保留原身份并等待核实，不自动 stop、重派或释放可能仍活跃的资源。

### 后续仍依赖的已有基础

- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和本地 minimal-v1 流程；远程生产执行仍由 `#6` 交付。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用已实现的答案仲裁、应用证明门槛和在线恢复入口；`#3` 的本机 TTY/Orca 原生验收仍单独完成，远端能力由 `#6` 独立验证。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、输出、问答和恢复语义；远端精确会话能力仍须按目标独立验证。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：`#6` 已接入固定 worker_done 身份、检查回执和执行端 HEAD 的校验；P13 的实际证据见[剩余门槛](../spec/orca-probe/remaining-gates.md)。

## 第二阶段：Windows 原生 UI 与受控规划

第二阶段交付 Windows 原生 UI 和人与 Codex 主 agent 的受控规划闭环，沿用第一阶段业务权威与[共同接口边界](../spec/phase2-constraints.md)。各任务以独立 Spec/Plan 为实施依据，范围与硬前置如下；阶段区别见[第二阶段说明](../phase-2/parallel-roadmap.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | `PHASE2` | 固定 Codex/app-server 能力、权限和受控工具的 Windows 真机证据 | 无未完成硬前置 | [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) |
| 15 | WinUI 基座与原生输入 | `PHASE2` | WinUI 原生窗口、导航、IME/多行输入及本地视图状态 | 无未完成硬前置 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) |
| 16 | 本机客户端协议与历史查询 | `PHASE2` | 回环认证、执行 Query/Command、SSE 补读和离线只读查询 | 无未完成硬前置 | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) |
| 17 | 规划提案与受控草稿 | `PHASE2` | 固定规划 worktree、提案/ID 预留、共享写入 guard、草稿与文档读模型 | `#16` | [Spec](../spec/phase2-planning-drafts.spec.md) · [Plan](../spec/phase2-planning-drafts.plan.md) |
| 18 | Codex 规划会话服务 | `PHASE2` | 真实 thread/turn、规范事件、审批和恢复；复用统一写入 guard | `#14`、`#17` | [Spec](../spec/phase2-planning-sessions.spec.md) · [Plan](../spec/phase2-planning-sessions.plan.md) |
| 19 | 正式文档发布与任务创建 | `PHASE2` | 纯文档/任务批次发布、完整版本可见性、规格进入目标分支及 Git/中心对账 | `#17` | [Spec](../spec/phase2-document-publish.spec.md) · [Plan](../spec/phase2-document-publish.plan.md) |
| 20 | Roadmap 与 DAG 原生浏览 | `PHASE2` | 正式 Roadmap/目标/版本和 DAG 关系的真实原生浏览 | `#15`、`#19` | [Spec](../spec/phase2-roadmap-ui.spec.md) · [Plan](../spec/phase2-roadmap-ui.plan.md) |
| 21 | 执行与待处理原生界面 | `PHASE2` | 任务/运行/决策/PR 的原生观察、操作与原请求恢复 | `#15`、`#16` | [Spec](../spec/phase2-execution-ui.spec.md) · [Plan](../spec/phase2-execution-ui.plan.md) |
| 22 | 规划对话原生界面 | `PHASE2` | 真实规划对话、新 session、流式消息、追加/中断及受控草稿工具 | `#15`、`#18` | [Spec](../spec/phase2-conversation-ui.spec.md) · [Plan](../spec/phase2-conversation-ui.plan.md) |
| 23 | 文档审阅与确认原生界面 | `PHASE2` | 预留、整套差异、人工审阅、固定版本确认与发布回执 | `#15`、`#19` | [Spec](../spec/phase2-review-ui.spec.md) · [Plan](../spec/phase2-review-ui.plan.md) |
| 24 | 第二阶段完整闭环验收 | `PHASE2` | 两条后端的真实依赖链、人工合并、交付回到规划及跨组件故障验收 | `#20`、`#21`、`#22`、`#23`、`#3`、`#4`、`#6` | [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) |

`#14/#15/#16` 可独立启动；`#16` 交付后可推进 `#17`，并在 `#15` 已交付时推进 `#21`。`#17` 后的发布 `#19` 与能力已通过时的会话 `#18` 可分头实现；再分为 `#20/#23` 文档页面和 `#22` 对话页面。最终 `#24` 汇合各页面与第一阶段 `#3/#4/#6`。所有箭头都是完整交付后的硬前置；并行研发不新增运行时执行槽或部分完成放行。共享文件/平台资源的集成约束见执行依赖图。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
