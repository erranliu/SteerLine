# Spec：第二阶段完整闭环验收

任务 ID：53；任务名称：第二阶段完整闭环验收；目标：`PHASE2`。状态：规格已编写，尚未实现或验收。配套 [Plan](phase2-integration.plan.md)。本任务由一个独立实现 PR 交付。

## 范围与前置

硬前置：#49、#50、#51、#52、#42，传递包含 #36。
依赖 [Roadmap/DAG UI](phase2-roadmap-ui.spec.md)、[执行 UI](phase2-execution-ui.spec.md)、[规划对话 UI](phase2-conversation-ui.spec.md)、[审阅 UI](phase2-review-ui.spec.md)、[持续 worker](../archive/spec/completed/worker-flow.spec.md)及[第一阶段组合验收](phase1-release-acceptance.spec.md)。这些任务完整交付后才执行本任务；不以条件分支跳过某后端。遵守[共同约束](phase2-constraints.md)。

本任务拥有第二阶段最终交付判定，交付可复现验收脚本/夹具、真实 Windows 操作证据、产品路径说明和必要的集成接线修复。原生 UI 成为本阶段完整日常入口；保留现有 CLI/TUI 诊断能力，不要求删除它们。新功能或上游契约变更超出此范围时另行规划，不能靠改动完成定义掩盖失败。

## 两条真实任务链

使用隔离、已注册的原生测试项目与测试托管仓库，记录实际二进制、CLI/Orca 版本、权限和资源。local Codex 与 Orca 远端分别运行一组至少两个有硬依赖的任务，起点为 WinUI 与主 agent 讨论目标：
讨论 → 同意编写规格/预留 ID → 生成独立 Spec/Plan → 审阅整套差异和拟定依赖 → 人确认发布 → 人将规划规格集成到执行目标分支 → 中心创建 → 原生 UI 观察与原 worker 持续反馈 → worker 实现/提交/创建 PR → 人合并任务 PR → 中心真实核验 TaskDelivery → 后继就绪并完成。

不得导入旧 Beads fixture 代替原生来源，不从历史 JSON 重建一个伪完成投影。每个任务独立 PR，未合并时后继不可执行。任一 worker 的自然语言成功或终端退出不能代替真实 PR 合并及 TaskDelivery。测试 PR 由用户明确合并，系统只核验。

完整链开始前从 UI 注册规划 worktree；规划规格集成采用保留原规划 commit 祖先关系的人工合并，不能用本机未提交文件充当目标分支。完成执行后，用户在 UI 同步目标分支的后继 commit，保留既有草稿；主 agent 读取实际交付与新代码，提出一次 documents_only 的产品方向/剩余规划更新，用户新建/确认该提案后只发布文档，不新占号或重建 DAG。这样证明“执行结果回到规划”的最后一段由真实 UI 完成。

## 故障与视图交叉验收

在同一受控环境至少覆盖：客户端保存信封后发送前退出及原信封补交；预留响应丢失；实际 index 替换与正式指针提交前后服务退出；Git 完成而中心创建回执丢失；发布拒绝后 needs_review 重审；等待取消与创建意图竞争；同内容 squash 集成拒绝；连续发布/同步；planning turn 活动时尝试发布；UI/服务窗口关闭与恢复；两个 session 争用；目标规格不一致；反馈已保存/送达但轮次尚未开始；PR 未合并关闭。复用上游注入入口，保存精确 owner/generation/request/commit/Task/Execution/session/turn 和原数据库，无重复 ID/turn/batch/worker/PR。

从规划提案、正式 Roadmap、DAG、运行、问题、PR 与交付互相导航，核对文档版本与中心快照分别一致。大正文、中文输入、窄窗口、滚动历史和持久待处理提示在真实原生窗口验证。截图证明展示，服务/平台回执证明业务事实，两者关联；截图不能代替后台门槛。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| E2E1 | 本机两任务真实完整链通过，独立 PR 人工合并、TaskDelivery 与后继释放可追踪 |
| E2E2 | Orca 远端同等原生链通过，版本/能力/取消限制正确，无生产门槛绕过 |
| E2E3 | 从交付事实发起 documents_only 更新并人工确认，零新任务/零新占号 |
| E2E4 | 上述故障矩阵在原身份/原中心恢复或明确阻塞，无重复副作用与错误释放 |
| E2E5 | 所有主要日常操作可由 WinUI 完成，离线/未知/审批与业务决策区分，无必需的隐式私有日志操作 |
| E2E6 | 验收入口、环境、证据清单、残留资源与可重复步骤齐全；全部前置与本任务未测项为零 |
