# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 当前交付边界

先交付[有人值守的远程执行 MVP](../remote-execution.md)：已认证台式机64g、原生五步、人工启动/问答/合并、每任务单候选。执行契约见 [Spec 03](../spec/03-remote-execution-and-viewer.md#有人值守-mvp-交付边界)及其 Plan。下列 #3/#4/#6 继续保留完整验收，不因 MVP 发布关闭或解除后续硬前置；#26–#32 属于后续第一阶段完整版本，不阻塞本次最小交付。

## 第一阶段遗留：人工决策与恢复的剩余验收

`#3` 的实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入 `main`，包括 SQLite 答案仲裁、固定执行续跑、在线恢复与审计命令，以及控制台决策历史和恢复操作；自动化回归已通过。2026-09-26 的 Windows TTY stale-revision 和 Orca command-worker 路径已取得部分实时证据，用户已确认实际 Windows 系统剪贴板粘贴通过。Codex tasks 95008/95009/95010 的终端和保存结果均已核实并结算为失败：答案证明与原 decision/attempt/execution 绑定，但 `workspace-write` 下的 linked-worktree Git 管理目录写入失败。2026-09-27 已将精确 Git 对象及 worktree 管理目录授权接入代码，并修复原终端暂不可核实时过早判 Decision 失败的问题；新隔离任务 95012 取得真实 Codex 绑定应用证明、干净提交和同 attempt 重启对账，中心 Decision 为 `applied`。本机 95013 还证明 resuming 中精确终端死亡不会重派；下游 publish fixture 的结果仍为 unknown 且保留槽。2026-09-28 又以独立本机目标验证了 H-A15：Orca 已创建原续跑终端但回执丢失时，中心保存 unknown Operation、Decision.resuming 与执行槽，同 SQLite 重启不重派；无平台 request ID 时未伪称自动应用。另在活跃续跑 worker 的真实 Orca 观察回执中仅注入同句柄的新 incarnation，中心拒绝身份、保留原 unknown Attempt/执行槽，同库重启不重派；Orca 平台自身的同句柄换代仍未观察。[可执行恢复确认](../validation/pr-05-recovery-confirmation-2026-09-28.json)已通过；H-A16 精确折叠仍未完成。见[验证记录](../validation/pr-05-intervention.md)与[后续验证](../validation/remaining-gates-2026-09-27.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | 95012 已验证 Orca 承载 Codex 在 `workspace-write` 下提交、绑定应用证明和同 attempt 重启对账；95013 已验证 resuming 中终端死亡不重派。[H-A15 显式续跑创建回执丢失](../validation/pr-05-h15-response-loss-2026-09-28.json)与[H-A6 活跃 worker 同句柄 incarnation 注入](../validation/pr-05-h6-incarnation-2026-09-28.json)均验证原 Attempt/执行槽保留且同库重启不重派；Orca 自身真实同句柄换代与精确轮次折叠仍缺；[可执行恢复确认](../validation/pr-05-recovery-confirmation-2026-09-28.json)已通过。实时 stale-revision 和用户确认的系统剪贴板粘贴已通过 | 95008/95009/95010 的失败结果不重放；95011 的旧失败决策、95012 的 publish unknown、95013 的故障现场分别保留，不把完成的 implement 误报为任务交付 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) · [后续验证](../validation/remaining-gates-2026-09-27.md) |

只有发送或回复回执时，答案仍不能标记为已应用；直接本机 Codex 后端续跑仍是不支持能力，95012 的成功证据限定于本机 Orca 承载 Codex。验收结果须按真实平台证据记录，不把模拟后端测试外推到远端执行。

## 第一阶段遗留：远程协议与原生闭环的剩余验收

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | P10 宿主命令权限提示的观察与人工处置、活跃 home 问题上的 8 KiB 序列化回复发送前拒绝均已真机验证；台式机64g 当前目标的 UAC 安全桌面已有用户原图及受控拒绝/批准回执；旧主机早期设置提示仅保留为历史缺口。P13 的中心正负向 verify 回执已有真机证据，完整协议验收仍开放 | 复用真实远端证据；不测平台最大载荷；仅支持当前明确验收的版本族 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) · [消息门槛样本](../spec/orca-probe/samples/p10-home-live-reply-size-2026-09-27.json) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | [原生任务 103](../validation/spec03-native-dag-2026-09-27.md) 已完成五步、真实合并与 DAG 后继解锁；[原生业务问答任务 91024](../validation/spec03-native-question-2026-09-28.md) 又在同一 Attempt 获得答案应用证明、五步检查、固定 head 合并与中心结项。既有 home 1.4.209/1.4.210 目标六项必需受监管能力已认证，活跃 home 回复限额及[可见关窗后的原 prepare 恢复](../spec/orca-probe/samples/spec03-native-r07-visible-window-2026-09-27.json)已通过。[协调终端丢失](../spec/orca-probe/samples/spec03-native-review-coordinator-loss-2026-09-27.json)仍有开放结论；[本机可见 viewer 刷新](../spec/orca-probe/samples/spec03-viewer-follow-2026-09-28.json)为 1.118 秒，[home 业务消息到可见 viewer](../validation/spec03-viewer-latency-2026-09-28.md)测得 Decision 1.669 秒、两次步骤转换 1.414 / 1.798 秒，home 目标的 2 秒门槛通过；[SteerLine 仓库项目能力](../validation/spec03-production-project-capability-2026-09-28.md)已在独立只读工作树认证；[R01–R19 故障矩阵](../validation/spec03-fault-matrix-2026-09-28.md)已核对并补齐 implement 离线完成窗口；[台式机64g 原生五步 canary](../validation/spec03-desktop64g-2026-09-28.md) 已通过；固定 gh 命令发布已在当前主机复验；完整生产验收仍待持续无人值守可靠性及新主机故障/可见 viewer 验收；有人值守 MVP 按上述边界启用 | 复用 `#3` 决策接口、`#5` 中心模型及 `#4` 已测协议；远程取消不可用 | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [原生问答验收](../validation/spec03-native-question-2026-09-28.md) |

2026-09-28 生产远端已改为 `台式机64g`（本机 Orca 1.4.209/远端 1.4.215）。[新主机完整记录](../validation/spec03-desktop64g-2026-09-28.md)保留首次 Smart App Control 拒绝未签名 `orca.exe` 的失败 Dispatch；用户批准关闭该设置后，独立只读 Run 通过新主机六项受监管能力，随后 SteerLine 项目原生 Task 91024 在同一新主机完成五步、原业务答案应用证明、固定 head 的 [PR #78](https://github.com/erranliu/SteerLine/pull/78) 合并、原中心结项及五个 Dispatch release/exited。测试 PR 只合入隔离 canary 基线，未修改 `main`。首次完整 Run 不需人工补按 Enter，但当时远端 `gh` 无会话，publish worker 通过已连接 GitHub 能力创建 PR。用户完成远端 `gh` 登录后，新的隔离 Task 按固定 `gh pr create` 命令创建并合并 [PR #85](https://github.com/erranliu/SteerLine/pull/85)，原业务答案应用、五步、Task 关闭及资源释放再次通过；[命令与闭环回执](../validation/spec03-desktop64g-2026-09-28.md)已归档。远端 UAC 原为关闭；经用户授权启用并重启后，受控提示的人工拒绝、批准/程序启动，以及用户拍摄的安全桌面原图已分别归档，当前目标 P10 的 UAC 可见分支通过。先前 home 的 R01–R19 故障矩阵和 2 秒 viewer 证据仍限定于旧主机；新主机长期无人值守及 #3 精确轮次/平台同句柄换代继续开放，完整生产派发仍受剩余门槛约束；有人值守 MVP 依当前独立边界启用。

受限远程适配器已接入中心 verify 校验器，P13 正负向受监管回执见[剩余门槛](../spec/orca-probe/remaining-gates.md)。本轮新增的原生任务闭环与本机 `#3` 验收分别记录；远程闭环继续复用已有执行后端、会话读模型、决策与恢复命令及 SQLite 中心权威。

首版远程范围以已测 Windows Orca 1.4.209 中心/远端组合为基础；本轮验收也允许本机中心 1.4.209 与 home 远端 1.4.210 配对，小版本号不同本身不阻止远端流程或验收，不因此重跑已通过的 P01–P12。每次探测仍记录双方实际版本、能力和执行身份；其他版本组合须另行核验，不能自动纳入支持范围。结构化消息采用不高于 8 KiB 的序列化大小上限，超限发送前拒绝，宿主权限提示无法可靠分类时交人工核实。生产远程取消不可用，中心保留活跃执行身份与资源占用并要求人工处理，不自动发送 stop 或按终端句柄兜底关闭；已结算目标的正常 release 仍可用。

### 后续仍依赖的已有基础

- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用已实现的答案仲裁、应用证明门槛和在线恢复入口；`#3` 的本机 TTY/Orca 原生验收仍单独完成，远端能力由 `#6` 独立验证。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：供远程闭环复用派发、身份、输出、问答、同工作区跨步骤与故障恢复证据；远端执行目标仍须按版本独立验证精确 turn/phase 能力，不能借用本机 `#1` 的验收。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入受监管中心状态机，正负向真机回执及任务 103 的三项实际检查验证固定候选门槛；其他无效结果仍按各自验收范围核对。

## 第一阶段：模块边界与结构重构

按[模块重构共同约束](../spec/modular-refactoring-constraints.md)提取纯执行规则、封装显式执行上下文，再在多候选交付模型稳定后封装中心与 SQLite。每项一个独立实现 PR；固定版本模块化继续归 #30/#31，本机/Orca 全面拆包保留为后续候选。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 33 | 执行规则包提取 | `PHASE1-MODULARITY` | 独立 workflow 纯规则包、生产调用接线、行为与依赖检查 | 无未完成硬前置 | [Spec](../spec/workflow-rules.spec.md) · [Plan](../spec/workflow-rules.plan.md) |
| 34 | 执行上下文封装与服务职责收缩 | `PHASE1-MODULARITY` | 固定上下文、显式事件/操作路由、Service 职责与状态所有权 | `#33` | [Spec](../spec/execution-context.spec.md) · [Plan](../spec/execution-context.plan.md) |
| 35 | 中心命令查询与SQLite边界封装 | `PHASE1-MODULARITY` | center 包、业务事务与只读查询、迁移全部 SQL 旁路及消费者 | `#27` | [Spec](../spec/center-boundary.spec.md) · [Plan](../spec/center-boundary.plan.md) |

执行顺序为 #33 → #34 → #26 → #27 → #35，再由 #28/#16 消费。三项任务和全部前置已进入中心 DAG；原有依赖及任务固定契约保留。[任务创建记录](../validation/modular-refactoring-task-creation/README.md)保存创建、补边与完整回读证据。

## 第一阶段版本交付目标与推进顺序

[第一阶段版本交付方案](phase-1-delivery.md)将现有 `#3/#4/#6` 的真实剩余验收，与同任务多候选、stable/dev 双入口及最终集成验收组成一个交付目标。下列任务已有独立 Spec/Plan 并已进入中心 DAG；具体执行遵守[第一阶段共同约束](../spec/phase1-delivery-constraints.md)，执行前须将中心固定引用的规格提交集成到目标分支。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 26 | 多候选模型与准入 | `PHASE1-CANDIDATES` | 同 Task 独立候选、固定 base、精确命令/查询与恢复；仲裁交付前限制 multiple 发布 | `#34`（中心追加的开工约束） | [Spec](../spec/task-candidates.spec.md) · [Plan](../spec/task-candidates.plan.md) |
| 27 | PR 胜出与落选候选对账 | `PHASE1-CANDIDATES` | 候选独立 PR、唯一 TaskDelivery、迟到合并与落选资源对账 | `#26` | [Spec](../spec/candidate-delivery.spec.md) · [Plan](../spec/candidate-delivery.plan.md) |
| 28 | 候选观察界面 | `PHASE1-CANDIDATES` | TUI 候选选择、独立会话/问题/PR 和未结算资源观察 | `#27`、`#35`（后者为中心追加） | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 29 | 多候选本机与远端真机验收 | `PHASE1-CANDIDATES` | 两后端双候选真实交付、后继链与故障恢复证据 | `#28`、`#3`、`#4`、`#6` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 30 | 固定版本与双入口 | `PHASE1-RUNTIME` | 不可变版本清单、stable/dev 固定入口及构建/home/channel 身份核验 | 无未完成硬前置 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 31 | dev 隔离与 stable 提升 | `PHASE1-RUNTIME` | 独立 home/仓库、受控选择与当前状态恢复；未知执行阻止提升 | `#30` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 32 | 第一阶段集成验收 | `PHASE1-DELIVERY` | 固定 stable 构建的两后端闭环、24 小时真实运行、dev 并行与受控提升 | `#29`、`#31` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

项目以人工派发优先完成第一阶段，重构线的 `#33`、版本线的 `#30` 与 `#3/#4/#6` 的独立验收可使用隔离资源推进；`#26` 等待 `#34` 完整交付。`#28` 不等待远端真机门槛，远端证明单列到 `#29`。已交付的[追加前置能力](../spec/native-task-add-predecessors.spec.md)已用于建立 `#26 → #16` 与 `#32 → #24`；`#14/#15` 保持可并行准备。补边只增加开工资格，已有任务的固定执行内容没有被重绑定。[原第一阶段中心创建与回读证据](../validation/phase1-task-creation/README.md)独立记录已生效命令事实。

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
| 24 | 第二阶段完整闭环验收 | `PHASE2` | 两条后端的真实依赖链、人工合并、交付回到规划及跨组件故障验收 | `#20`、`#21`、`#22`、`#23`、`#3`、`#4`、`#6`，另有中心追加的 `#32` | [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) |

`#14/#15` 可在隔离资源上准备；`#16` 领取前必须等待 `#26/#35` 完整交付。`#16` 交付后可推进 `#17`，并在 `#15` 已交付时推进 `#21`。`#17` 后的发布 `#19` 与能力已通过时的会话 `#18` 可分头实现；再分为 `#20/#23` 文档页面和 `#22` 对话页面。最终 `#24` 汇合各页面、原 `#3/#4/#6` 和新增 `#32` 第一阶段整体交付门槛。所有箭头都是完整交付后的硬前置；共享文件/平台资源的集成约束见执行依赖图。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
