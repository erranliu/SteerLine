# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 近期：模块解耦与测试反馈提速

测试耗时基线、报告入口和夹具精简已交付。后续按“删除旧执行路径 → 拆分规则包与集成测试”的顺序推进，每项由独立实现 PR 交付。优先缩短日常局部修改的反馈，同时保留合入前的完整无缓存回归、当前格式恢复和真实交付核验。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 12 | 移除 Beads 旧执行路径 | `REFACTOR` | 实现和自动化验收完成；[#44](https://github.com/erranliu/SteerLine/pull/44) 待合入。live Codex/Orca/PR-provider/可见窗口仍按各自门槛验收 | 已交付的 `#11` 耗时和覆盖基线 | [Spec](../spec/beads-path-removal.spec.md) · [Plan](../spec/beads-path-removal.plan.md) · [验证记录](../validation/beads-path-removal/results.md) |
| 13 | 任务图规则拆包与测试分层 | `REFACTOR` | 独立 taskgraph 规则包、CLI 黑盒集成包和按模块测试入口；按隔离条件评估并行 | `#12` 收敛后的原生代码；沿用 `#11` 测量工具 | [Spec](../spec/taskgraph-modules.spec.md) · [Plan](../spec/taskgraph-modules.plan.md) |

[测试反馈验证记录](../validation/test-feedback/README.md)与[行为映射](../validation/test-feedback/coverage-map.md)供 `#12`/`#13` 复用。`#3` 的剩余原生验收、`#4` 的远端探测可在固定版本和隔离资源上继续。`#6` 可准备独立协议适配，涉及 Service、Query/Command、配置和同一夹具时与 `#12`/`#13` 串行集成；后集成方核对接口并补受影响回归。具体交接见 [execution-map](execution-map.md)。

## 近期：人工决策与恢复的剩余验收

`#3` 已实现 SQLite 中的答案仲裁、固定执行续跑、在线恢复与审计命令，以及控制台决策历史和恢复操作；自动化回归已通过。完整 H-A1～H-A16 验收仍缺真实交互与应用证明，范围见[验证记录](../validation/pr-05-intervention.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 前置条件 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | Windows TTY 验证长问题、12 个选项、中文多行答案、旧版本冲突、折叠导航与恢复确认；在隔离注册仓库验证本机 Orca 同会话续跑、原操作恢复及答案应用证明 | 可复现的 Windows TTY 样例与隔离注册的 Orca 测试仓库 | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [剩余门槛](../validation/pr-05-intervention.md) |

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
