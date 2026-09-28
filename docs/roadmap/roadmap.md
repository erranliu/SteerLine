# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 当前交付边界

先交付[有人值守的远程执行 MVP](../remote-execution.md)：已认证台式机64g、原生五步、人工启动/问答/合并、每任务单候选。#3 的决策安全与本机 TTY、#4 的当前范围协议验证已有回执；两项中心 Task 仍待正常 Delivery。#6 保留当前目标的受影响故障路径和 viewer 步骤转换门槛，长期无人值守由 #32 验收。#29/#24 的原有硬前置不变。

## 第一阶段：人工决策与恢复

`#3` 的实现已合入主线。[H-A6 活跃 worker 身份变化](../validation/pr-05-h6-incarnation-2026-09-28.json)、[H-A15 回执丢失](../validation/pr-05-h15-response-loss-2026-09-28.json)、[合格恢复确认](../validation/pr-05-recovery-confirmation-2026-09-28.json)与 [SteerlineTest Task 104 的 H-A16 可见 TTY](../validation/pr05-ha16-steerlinetest-2026-09-28/README.md)已有验收回执。H-A6 使用真实 worker 的入口单字段故障注入证明拒绝冒认和不重派；平台自然发生同句柄换代不作为关闭前置。早期失败试次与当时限制见[原验证记录](../validation/pr-05-intervention.md)，不回写为成功。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | H-A1–H-A16 按现有自动化与真机证据在受测范围完成；绑定答案应用、响应丢失不重派、恢复确认和精确折叠均有回执。当前剩余的是原中心 Task 的真实 Delivery，不重复已通过场景 | 直接本机 Codex 续跑仍 unsupported；发送回执不能代替答案应用证明 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) |


只有发送或回复回执时，答案仍不能标记为已应用；直接本机 Codex 后端续跑仍是不支持能力，95012 的成功证据限定于本机 Orca 承载 Codex。验收结果须按真实平台证据记录，不把模拟后端测试外推到远端执行。

## 第一阶段：远程协议与原生闭环

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | 当前 Orca 1.4 版本族与台式机64g 目标的 P01–P13 已通过；P10 人工处置和 P13 正负向中心门槛均有证据。原中心 Task 仍待真实 Delivery，不重跑未受变更影响的探测 | 远程取消 unsupported；新目标、版本族或调用路径变更时定向补证 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [P13 回执](../validation/spec01-p13-steerlinetest-2026-09-28/README.md) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | home 的 R01–R19、2 秒 viewer 和原生闭环已有证据；台式机64g 已完成原生五步、业务答案、固定 head 真实合并及释放，Decision 可见上界 2.215 秒，通过当前 4 秒门槛。当前主机步骤转换可见上界 1.334 秒通过 4 秒门槛；仍待受影响故障路径和原中心 Task Delivery | 长期无人值守矩阵归 #32；远程取消 unsupported | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [当前主机回执](../validation/spec03-desktop64g-followup-2026-09-28/README.md) · [Task 105 步骤可见](../validation/spec03-viewer-task105-2026-09-28/README.md) |

2026-09-28 远端目标改为 `台式机64g`。Smart App Control 的首次阻断、用户处置、UAC 人工拒绝与批准均见[目标验收记录](../validation/spec03-desktop64g-2026-09-28.md)；隔离原生 Task 的五步、答案、固定 head 合并和资源释放见[后续回执](../validation/spec03-desktop64g-followup-2026-09-28/README.md)。这些回执不触及 `main`，也不自动关闭中心 #6 Task。

2026-09-28 用户将当前 `台式机64g` 的可见 viewer 时延门槛从 2 秒调整为 **4 秒**；先前 `home` 的 2 秒验收不变。[当前主机后续回执](../validation/spec03-desktop64g-followup-2026-09-28/README.md)中，一次原生 Decision 从中心记录到可见画面的保守上界为 2.215 秒，按新门槛通过该样本；后续 [SteerlineTest Task 105](../validation/spec03-viewer-task105-2026-09-28/README.md) 的步骤转换可见上界为 1.334 秒，通过当前主机 4 秒门槛；首个因筛选丢失对象的样本不计通过。

同日验收归属收口：#6 的关闭以 [Spec 03](../spec/03-remote-execution-and-viewer.md) 第 8–9 节的远程功能、故障安全、目标可见 viewer 和真实交付为准；已通过且未受变更影响的版本族回执不重跑，新主机特有和受影响路径补证。长期无人值守矩阵、固定 stable 24 小时及资源趋势属于 #32；#6 不因等待该矩阵阻塞 #29。此调整不跳过 #6 的真实 Task Delivery，也不自动关闭中心任务。

受限远程适配器已接入中心 verify 校验器，P13 正负向受监管回执见[剩余门槛](../spec/orca-probe/remaining-gates.md)。本轮新增的原生任务闭环与本机 `#3` 验收分别记录；远程闭环继续复用已有执行后端、会话读模型、决策与恢复命令及 SQLite 中心权威。

首版远程范围以已测 Windows Orca 1.4.209 中心/远端组合为基础；本轮验收也允许本机中心 1.4.209 与 home 远端 1.4.210 配对，小版本号不同本身不阻止远端流程或验收，不因此重跑已通过的 P01–P12。每次探测仍记录双方实际版本、能力和执行身份；其他版本组合须另行核验，不能自动纳入支持范围。结构化消息采用不高于 8 KiB 的序列化大小上限，超限发送前拒绝，宿主权限提示无法可靠分类时交人工核实。生产远程取消不可用，中心保留活跃执行身份与资源占用并要求人工处理，不自动发送 stop 或按终端句柄兜底关闭；已结算目标的正常 release 仍可用。

### 后续仍依赖的已有基础

- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用已验收的答案仲裁、应用证明门槛和在线恢复入口；`#3` 仍待中心 Task Delivery，远端能力由 `#6` 独立验证。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：供远程闭环复用派发、身份、输出、问答、同工作区跨步骤与故障恢复证据；远端执行目标仍须按版本独立验证精确 turn/phase 能力，不能借用本机 `#1` 的验收。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入受监管中心状态机，正负向真机回执及任务 103 的三项实际检查验证固定候选门槛；其他无效结果仍按各自验收范围核对。

## 第一阶段版本交付目标与推进顺序

[第一阶段版本交付方案](phase-1-delivery.md)汇合 #3/#4 的正常 Task Delivery、#6 的当前目标剩余验收，以及同任务多候选、stable/dev 双入口和最终集成验收。下列任务已有独立 Spec/Plan 并进入中心 DAG；执行遵守[第一阶段共同约束](../spec/phase1-delivery-constraints.md)，原 TaskContract 固定引用与硬前置不因文档状态更新而改写。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | `PHASE1-CANDIDATES` | 同 Task 独立候选、固定 base、精确命令/查询与恢复；仲裁交付前限制 multiple 发布 | 无未完成硬前置 | [Spec](../spec/task-candidates.spec.md) · [Plan](../spec/task-candidates.plan.md) |
| 27 | PR 胜出与落选候选对账 | `PHASE1-CANDIDATES` | 候选独立 PR、唯一 TaskDelivery、迟到合并与落选资源对账 | `#26` | [Spec](../spec/candidate-delivery.spec.md) · [Plan](../spec/candidate-delivery.plan.md) |
| 28 | 候选观察界面 | `PHASE1-CANDIDATES` | TUI 候选选择、独立会话/问题/PR 和未结算资源观察 | `#27` | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 29 | 多候选本机与远端真机验收 | `PHASE1-CANDIDATES` | 两后端双候选真实交付、后继链与故障恢复证据 | `#28`、`#3`、`#4`、`#6` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 30 | 固定版本与双入口 | `PHASE1-RUNTIME` | 不可变版本清单、stable/dev 固定入口及构建/home/channel 身份核验 | 无未完成硬前置 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 31 | dev 隔离与 stable 提升 | `PHASE1-RUNTIME` | 独立 home/仓库、受控选择与当前状态恢复；未知执行阻止提升 | `#30` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 32 | 第一阶段集成验收 | `PHASE1-DELIVERY` | 固定 stable 构建的两后端闭环、24 小时真实运行、dev 并行与受控提升 | `#29`、`#31` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

项目以人工派发优先完成第一阶段；`#26/#30` 与 `#3/#4` 的正常交付、`#6` 的剩余验收可在独立资源上推进。`#28` 不等待远端真机门槛，远端证明单列到 `#29`。已交付的[追加前置能力](../spec/native-task-add-predecessors.spec.md)建立 `#26 → #16` 与 `#32 → #24`；`#14/#15` 可在隔离资源准备。中心固定 TaskContract 和 AND 依赖保持不变。[中心创建与回读证据](../validation/phase1-task-creation/README.md)记录命令事实。

## 第二阶段：Windows 原生 UI 与受控规划

第二阶段交付 Windows 原生 UI 和人与 Codex 主 agent 的受控规划闭环，沿用第一阶段业务权威与[共同接口边界](../spec/phase2-constraints.md)。各任务以独立 Spec/Plan 为实施依据，范围与硬前置如下；阶段区别见[第二阶段说明](../phase-2/parallel-roadmap.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 14 | Codex 规划主 agent 能力验证 | `PHASE2` | 固定 Codex/app-server 能力、权限和受控工具的 Windows 真机证据 | 无未完成硬前置 | [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) |
| 15 | WinUI 基座与原生输入 | `PHASE2` | WinUI 原生窗口、导航、IME/多行输入及本地视图状态 | 无未完成硬前置 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) |
| 16 | 本机客户端协议与历史查询 | `PHASE2` | 回环认证、执行 Query/Command、SSE 补读和离线只读查询 | `#26`（中心追加的开工约束） | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) |
| 17 | 规划提案与受控草稿 | `PHASE2` | 固定规划 worktree、提案/ID 预留、共享写入 guard、草稿与文档读模型 | `#16` | [Spec](../spec/phase2-planning-drafts.spec.md) · [Plan](../spec/phase2-planning-drafts.plan.md) |
| 18 | Codex 规划会话服务 | `PHASE2` | 真实 thread/turn、规范事件、审批和恢复；复用统一写入 guard | `#14`、`#17` | [Spec](../spec/phase2-planning-sessions.spec.md) · [Plan](../spec/phase2-planning-sessions.plan.md) |
| 19 | 正式文档发布与任务创建 | `PHASE2` | 纯文档/任务批次发布、完整版本可见性、规格进入目标分支及 Git/中心对账 | `#17` | [Spec](../spec/phase2-document-publish.spec.md) · [Plan](../spec/phase2-document-publish.plan.md) |
| 20 | Roadmap 与 DAG 原生浏览 | `PHASE2` | 正式 Roadmap/目标/版本和 DAG 关系的真实原生浏览 | `#15`、`#19` | [Spec](../spec/phase2-roadmap-ui.spec.md) · [Plan](../spec/phase2-roadmap-ui.plan.md) |
| 21 | 执行与待处理原生界面 | `PHASE2` | 任务/运行/决策/PR 的原生观察、操作与原请求恢复 | `#15`、`#16` | [Spec](../spec/phase2-execution-ui.spec.md) · [Plan](../spec/phase2-execution-ui.plan.md) |
| 22 | 规划对话原生界面 | `PHASE2` | 真实规划对话、新 session、流式消息、追加/中断及受控草稿工具 | `#15`、`#18` | [Spec](../spec/phase2-conversation-ui.spec.md) · [Plan](../spec/phase2-conversation-ui.plan.md) |
| 23 | 文档审阅与确认原生界面 | `PHASE2` | 预留、整套差异、人工审阅、固定版本确认与发布回执 | `#15`、`#19` | [Spec](../spec/phase2-review-ui.spec.md) · [Plan](../spec/phase2-review-ui.plan.md) |
| 24 | 第二阶段完整闭环验收 | `PHASE2` | 两条后端的真实依赖链、人工合并、交付回到规划及跨组件故障验收 | `#20`、`#21`、`#22`、`#23`、`#3`、`#4`、`#6`，另有中心追加的 `#32` | [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) |

`#14/#15` 可在隔离资源上准备；`#16` 领取前必须等待 `#26` 完整交付。`#16` 交付后可推进 `#17`，并在 `#15` 已交付时推进 `#21`。`#17` 后的发布 `#19` 与能力已通过时的会话 `#18` 可分头实现；再分为 `#20/#23` 文档页面和 `#22` 对话页面。最终 `#24` 汇合各页面、原 `#3/#4/#6` 和新增 `#32` 第一阶段整体交付门槛。所有箭头都是完整交付后的硬前置；共享文件/平台资源的集成约束见执行依赖图。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
