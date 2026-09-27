# 已规划 Spec 的执行依赖

这里直接列出 [主 roadmap](roadmap.md) 中尚未完成的执行 Spec 与验收，以及它们的开工、集成和验收前置。`#3` 的实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入 `main`；早期 95008/95009/95010 的 Codex 应用均结算失败且未重放，后续 95012 已在精确 Git 沙箱授权下取得绑定应用证明及同 attempt 重启对账。`#4` 的 P13 中心正负向 verify 已有真机证据，P10 及完整协议仍有剩余门槛。执行范围和判定以各 Spec/Plan 及[最新验证记录](../validation/remaining-gates-2026-09-27.md)为准。

下文的 `#数字` 均指任务 ID。“直接开工前置”列出必须已有的实现或证据；“集成与交付条件”说明何时才能验收并交给后续任务。共享接口的并行准备不等于允许越过各 Spec 的硬前置。

## 未完成范围与直接依赖

| 任务 ID | 任务名称 | 目标 / 执行 Spec | 直接开工前置 | 集成与交付条件 |
| --- | --- | --- | --- | --- |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` · [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) · [剩余门槛](../validation/remaining-gates-2026-09-27.md) | 实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入；后续 Git 沙箱与不确定结果的修正及真机证据在当前验证记录中。95008/95009/95010 的失败应用未重放，95011 的失败决策未原地改写 | 用户报告系统剪贴板粘贴通过；stale-revision 与 live `answer -wait` 已验证；95012 的成功应用/同 attempt 重启及 95013 的 resuming 中终端死亡不重派已验证。精确折叠、可执行恢复确认、显式响应丢失和活跃 worker incarnation 替换仍待验收。发送回执不能代替应用证明。 |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` · [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) | 无未完成任务前置；复用已有真实远端报告与字段映射，命令权限提示的真机观察和人工处置已有证据 | 仍须验证固定 8 KiB 发送前拒绝及管理员/UAC 提示的完整捕获；P13 正负向受监管 verify 回执已有证据。 |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` · [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) · [验收记录](../validation/spec03-native-dag-2026-09-27.md) | 已交付的 `#3` 决策接口与 `#5` 模型、存储和执行接口；`#4` 已实测协议及限制 | 原生任务 103 已在远端五步发布、真实合并、中心结项并解锁 104；当前目标的六项必需受监管能力已有认证。生产启用仍须完成 `#4` 剩余消息/权限门槛、Windows 服务关窗及完整故障矩阵。远程取消保持不可用。 |

控制台与原生任务关系视图的实现已交付；`#3` 的决策与恢复实现已随 [PR #39](https://github.com/erranliu/SteerLine/pull/39) 合入，只剩原生验收。`#6` 复用当前单一原生中心权威接入远程执行，远端生产能力仍按自身门槛验收。`#4`、`#6` 各自沿用独立实现范围；本机决策验收不替代远端能力验收。

## 跨链路的接口与并行条件

- **#6 与当前执行接口的共享修改**：Service、配置、worker 上下文、Query/Command、Task DTO 和测试夹具存在共同修改面。独立协议样例可并行；涉及共同接口的修改串行集成，后集成方校验接口并运行受影响回归。`#6` 只接 native，不增加既有运行时任务的可变依赖。
- **#3/#4 与 #6 的验收资源**：本机 TTY/Orca 验收与远端探测可在固定版本和独立资源上继续；共享仓库、CLI 构建产物或平台执行不能被同时改写。`#6` 改变已测调用路径时重跑受影响项，不把此前版本证据直接标为新实现通过。
- **#3 的原生验收**：已实现的决策、续跑和恢复复用 `#0` 固定执行身份、`#1` 会话事件、`#2` 控制台与 `#5` SQLite 中心权威。2026-09-26 的实时 TTY stale-revision/Orca command-worker 样例只证明已记录范围；95008/95009/95010 的 Codex continuation 均结算失败且未重放。2026-09-27 新任务 95012 使用精确 Git 目录授权，原终端暂不可核实时保留 `reply_accepted`，确认退出后重启接回同一 attempt 的绑定证明、中心记 Decision.applied；后续 publish unknown 不回退该事实，也不等于任务交付。用户报告 TTY 实际系统剪贴板粘贴已通过；精确轮次折叠、可执行恢复确认和明确故障注入仍须独立验收。95012 会话来源只有 Orca observation/step-result，不能证明精确 turn/final。
- **#4 与本机工作**：剩余远端探测可与 `#3` 的本机验收并行，探测与适配器集成不能同时占用同一真实远端测试执行资源。本机 `#1` 的精确 turn/phase 及 `#3` 的应用证明验收不代替远端能力证据；远端分页/恢复证据也不代替远端精确会话验证。
- **#6 复用已交付的原生接入**：远程闭环沿用 `#9` 的中心任务权威、固定执行上下文和恢复接口；远端生产能力仍须按 `#4`/`#6` 的独立门槛验收，不扩大版本、消息大小或取消能力。
- **接口前置与硬前置**：接口固定后可以准备独立样本、只读展示或测试；需要真实平台、数据库或 UI 的集成验收仍须等待对应前置实现。并行建议不改变各 Spec/Plan 明定的串行交付要求；调整顺序时先修订受影响的 Spec/Plan。

## 后续仍依赖的已交付基础

- [执行控制台与轮次折叠](../spec/pr-04-console.spec.md)及[验收报告](../validation/pr-04c-console.md)：`#3` 的剩余 TTY 验收沿用决策卡片和 UI 视图状态；`#6` 复用同一查询与能力展示边界。
- [双执行后端](../spec/pr-04a-execution-backends.spec.md)与[Windows 验收报告](../validation/pr-04a-execution-backends.md)：`#3` 的本机 Orca 验收和 `#6` 复用固定 ExecutionRef、Operation、能力报告、ReadOutput 及双后端操作边界；会话 phase/turn、资源结算与 Linux 原生验收不在该后端 Spec 的交付范围内。
- [会话事件](../spec/pr-04b-conversation-events.spec.md)与[本机验收报告](../validation/pr-04b-conversation-events-2026-09-24.md)：`#3`/`#6` 复用统一会话 DTO、完整正文、分页、来源缺口与版本化能力报告；精确 turn/phase/final 仅在报告列明的本机 Codex/Orca 范围内通过。
- [中心执行模型](../spec/02-execution-model-and-adapter.md)：`#3`/`#6` 复用 WorkflowRun/Execution/StepAttempt、SQLite 单写者、Query/Command、Operation 和 minimal-v1 流程；远端原生闭环已有真机回执，生产启用仍待 `#6` 剩余门槛。
- [人工决策与恢复](../spec/pr-05-intervention.spec.md)与[验证记录](../validation/pr-05-intervention.md)：`#6` 复用同一 SQLite CommandService、固定答案与应用证明；`#3` 的真实 TTY/Orca 门槛仍待完成。
- [原生项目接入](../spec/beads-exit.spec.md)与[验收记录](../validation/beads-exit/README.md)：`#6` 沿用固定 worker 上下文，仍按独立远端门槛验收。
- [统一观察契约](../spec/pr-01-observation.spec.md)：`#3` 的剩余验收沿用选择、游标、来源换代及 raw 读取。
- [服务控制协议](../spec/pr-02-control.spec.md)与[服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：`#3` 的剩余验收沿用请求及实例身份、回执、连接分类和停止确认。Windows 服务窗口默认 manual，关闭控制台不停服；关闭服务窗口结束服务。Linux detached 路径尚无原生会话生命周期实测。
- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：`#4`/`#6` 复用已实测派发、身份、问答和恢复语义；远端精确会话能力仍须独立验证，不能从本机 `#1` 的验收外推。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：已接入中心；P13 正负向真机 verify 回执及任务 103 的三项实际检查证明固定候选门槛，未测无效结果仍按各自条件核对。

Spec 完成交付后从未完成表移出；仍有消费者时只保留必要交接。本图不记录 PR、提交或文档维护历史。
