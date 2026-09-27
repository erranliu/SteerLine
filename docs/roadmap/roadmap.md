# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 第一阶段遗留：人工决策与恢复的剩余验收

`#3` 的实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入 `main`，包括 SQLite 答案仲裁、固定执行续跑、在线恢复与审计命令，以及控制台决策历史和恢复操作；自动化回归已通过。2026-09-26 的 Windows TTY stale-revision 和 Orca command-worker 路径已取得部分实时证据，用户已确认实际 Windows 系统剪贴板粘贴通过。Codex tasks 95008/95009/95010 的终端和保存结果均已核实并结算为失败：答案证明与原 decision/attempt/execution 绑定，但 `workspace-write` 下的 linked-worktree Git 管理目录写入失败。2026-09-27 已将精确 Git 对象及 worktree 管理目录授权接入代码，并修复原终端暂不可核实时过早判 Decision 失败的问题；新隔离任务 95012 取得真实 Codex 绑定应用证明、干净提交和同 attempt 重启对账，中心 Decision 为 `applied`。本机 95013 还证明 resuming 中精确终端死亡不会重派；下游 publish fixture 的结果仍为 unknown 且保留槽。H-A6 活跃 worker incarnation 替换、H-A15 显式响应丢失、H-A16 精确折叠和可执行恢复确认尚未完成。见[验证记录](../validation/pr-05-intervention.md)与[后续验证](../validation/remaining-gates-2026-09-27.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | 95012 已验证 Orca 承载 Codex 在 `workspace-write` 下提交、绑定应用证明和同 attempt 重启对账；95013 已验证 resuming 中终端死亡不重派。仍缺活跃 worker incarnation 替换、显式响应丢失、精确轮次折叠与可执行恢复确认。实时 stale-revision 和用户确认的系统剪贴板粘贴已通过 | 95008/95009/95010 的失败结果不重放；95011 的旧失败决策、95012 的 publish unknown、95013 的故障现场分别保留，不把完成的 implement 误报为任务交付 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) · [后续验证](../validation/remaining-gates-2026-09-27.md) |

只有发送或回复回执时，答案仍不能标记为已应用；直接本机 Codex 后端续跑仍是不支持能力，95012 的成功证据限定于本机 Orca 承载 Codex。验收结果须按真实平台证据记录，不把模拟后端测试外推到远端执行。

## 第一阶段遗留：远程协议与原生闭环的剩余验收

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | P10 宿主命令权限提示的观察与人工处置已在 home 真机验证；仍需实际发送入口的 8 KiB 发送前拒绝及早期管理员/UAC 提示的完整捕获。P13 的中心正负向 verify 回执已有真机证据，完整协议验收仍开放 | 复用真实远端证据；不测平台最大载荷；仅支持当前明确验收的版本族 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) · [真机样本](../spec/orca-probe/samples/p10-host-permission-2026-09-27.json) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | [远端原生任务 103](../validation/spec03-native-dag-2026-09-27.md) 已完成五步、真实合并并由中心关闭，后继 104 解锁；当前 1.4.209/1.4.210 目标的六项必需受监管能力已有真机认证。生产启用仍待剩余权限、消息限额、Windows 服务关窗和完整故障矩阵验收 | 复用 `#3` 决策接口、`#5` 中心模型及 `#4` 已测协议；远程取消不可用 | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [验收记录](../validation/spec03-native-dag-2026-09-27.md) |

受限远程适配器已接入中心 verify 校验器，P13 正负向受监管回执见[剩余门槛](../spec/orca-probe/remaining-gates.md)。本轮新增的原生任务闭环与本机 `#3` 验收分别记录；远程闭环继续复用已有执行后端、会话读模型、决策与恢复命令及 SQLite 中心权威。

首版远程范围以已测 Windows Orca 1.4.209 中心/远端组合为基础；本轮验收也允许本机中心 1.4.209 与 home 远端 1.4.210 配对，小版本号不同本身不阻止远端流程或验收，不因此重跑已通过的 P01–P12。每次探测仍记录双方实际版本、能力和执行身份；其他版本组合须另行核验，不能自动纳入支持范围。结构化消息采用不高于 8 KiB 的序列化大小上限，超限发送前拒绝，宿主权限提示无法可靠分类时交人工核实。生产远程取消不可用，中心保留活跃执行身份与资源占用并要求人工处理，不自动发送 stop 或按终端句柄兜底关闭；已结算目标的正常 release 仍可用。

### 后续仍依赖的已有基础

- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用已实现的答案仲裁、应用证明门槛和在线恢复入口；`#3` 的本机 TTY/Orca 原生验收仍单独完成，远端能力由 `#6` 独立验证。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：供远程闭环复用派发、身份、输出、问答、同工作区跨步骤与故障恢复证据；远端执行目标仍须按版本独立验证精确 turn/phase 能力，不能借用本机 `#1` 的验收。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入受监管中心状态机，正负向真机回执及任务 103 的三项实际检查验证固定候选门槛；其他无效结果仍按各自验收范围核对。

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
