# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 近期：人工决策与恢复的剩余验收

`#3` 的实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入 `main`，包括 SQLite 答案仲裁、固定执行续跑、在线恢复与审计命令，以及控制台决策历史和恢复操作；自动化回归已通过。2026-09-26 的 Windows TTY stale-revision 和 Orca command-worker 路径已取得部分实时证据；TTY bracketed-paste 事件也已输入，但实际 Windows 剪贴板仍未验证。Codex tasks 95008/95009 的终端和保存结果均已核实并结算为失败：答案证明与原 decision/attempt/execution 绑定，但 `workspace-write` 仍无法写 linked worktree 的 `.git/worktrees/<run>` 管理目录；临时 `--add-dir` common-dir 授权无效，未保留在源码中。没有遗留 `unknown` 终端；完整 H-A1～H-A16 尚未通过，见[验证记录](../validation/pr-05-intervention.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | TTY 仍缺实际 Windows 剪贴板粘贴、精确轮次折叠和可执行的恢复确认；实时 stale-revision 已通过。需解决 Codex `workspace-write` 对 linked worktree admin files 的写入限制，再取得成功的 Codex 应用证明和成功响应丢失恢复验收 | 2026-09-26 的实时终端均已结算；95008/95009 的失败结果和 proof 均留档，不重放这些 attempts | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [验证记录](../validation/pr-05-intervention.md) |

只有发送或回复回执时，答案仍不能标记为已应用；本机 Codex 续跑仍是不支持能力。验收结果须按真实平台证据记录，不把模拟后端测试外推到本机 Orca，也不外推到远端执行。

## 后续：中心状态机与远程执行

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | 验证固定消息上限和宿主权限提示的人工处置；结合 `#6` 完成 P13 中心拒绝无效结果的端到端验收 | 复用真实远端证据；不测平台最大载荷；仅支持已验证版本组合 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | 中心逐步派发实现、验证、审查与发布；接入已有 verify 校验器，完成远程观察、人工决策、断线对账及结项 | 开发使用 `#4` 已实测协议、已交付的 `#3` 决策接口与 `#5` 中心模型；生产启用前通过 `#4` 剩余门槛及本任务验收 | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) |

可依据实测证据开发受限远程适配器，协议目标保留剩余验收；P13 需要在中心接入后验证，不能将纯校验器测试视为端到端通过。剩余远端探测可与 `#3` 的本机验收并行，但各自保留独立的真实平台证据。远程闭环复用已交付的执行后端、会话读模型、决策与恢复命令以及 SQLite 中心权威，扩展同一执行/会话契约，避免形成两套适配器或业务写者。

首版远程范围仅支持已测 Windows Orca 1.4.209 中心/远端组合，其他版本预检拒绝；结构化消息采用不高于 8 KiB 的序列化大小上限，超限发送前拒绝，宿主权限提示无法可靠分类时交人工核实。生产远程取消不可用，中心保留活跃执行身份与资源占用并要求人工处理，不自动发送 stop 或按终端句柄兜底关闭；已结算目标的正常 release 仍可用。

### 后续仍依赖的已有基础

- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和本地 minimal-v1 流程；远程生产执行仍由 `#6` 交付。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用已实现的答案仲裁、应用证明门槛和在线恢复入口；`#3` 的本机 TTY/Orca 原生验收仍单独完成，远端能力由 `#6` 独立验证。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：供远程闭环复用派发、身份、输出、问答、同工作区跨步骤与故障恢复证据；远端执行目标仍须按版本独立验证精确 turn/phase 能力，不能借用本机 `#1` 的验收。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：供远程闭环接入已接受的 worker_done 身份、预期检查清单和执行端 HEAD，阻止缺回执、非零退出、身份或候选 commit 不符的结果推进审查/发布。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
