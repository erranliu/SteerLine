# Spec：统一 Worker 的 PR 成果流程

2026-10-12 用户确认采用按 PR 修改、审核、合并的流程，Codex 与 Orca 从共享 worker 到底层执行链路统一调整。本次按用户授权作为普通开发实施，不占 Task ID、不启动 roadmap 或安装操作。

[Plan](worker-pr-flow.plan.md) · [专属验收](../acceptance/worker-pr-flow.md)。实现以最新主干为基线，保留已合入的相关修复；本设计替代当前阶段成果快照比较和候选 SHA 锁定规则。

## 目标

102 的现场中，实现已提交新 HEAD，完成快照仍记录旧基线，报告发送前被 artifact_mismatch 阻断；缓存延迟尚未证实。本次取消阶段成果比较及 CandidateSHA 冻结，以本次执行的 PR 实际合并内容作为交付结果，不增加刷新等待、重复核验或另一套状态机。

本地 Codex、本机 Orca、Orca environment 共用同一业务流程。共享 worker 负责步骤、提示词、反馈和结果来源；adapter 负责原输入执行、原生身份及回复采集；现有发布/交付代码核对 PR 并提交 TaskDelivery。保持现有模块，不新增服务、agent、协议层或通用编排框架。

## 正常流程

`implement.execute → implement.report → publish → await_merge → delivered`

1. **领取。** 保留目标分支 Base 获取、前置实际合并提交包含性检查、独立工作分支及会话创建。
2. **实现。** 按 Spec/Plan 实现、最小验证、自查和提交。完成后保存原 final、失败及来源，直接进入报告；阶段 HEAD、dirty、ContentDigest 或 PR 快照不再是推进条件。
3. **报告。** 只整理原实现结果，不改代码或重跑检查。保留原 Command/Attempt/Turn、输入版本和 SourceCommandID/SourceTurnID；源失败不能被报告 done 覆盖。有效报告直接排队 publish，不采集成果快照或冻结 SHA。
4. **发布。** worker 在原分支完成必要提交/推送，创建 PR；已有 PR 时继续推送并复用它。结构化成功结果提供 `outputs.pr_url`，不提供 candidate_sha。代码反馈返回实现步骤，不在发布阶段扩展实现范围。
5. **等待与交付。** 中心通过现有 provider 查询核对 PR：open 等待合并，merged 且证据完整按既有事务交付，closed_unmerged 等待显式处理，查询失败或未知保留 reconciling。交付登记实际 MergeCommit/MergedAt、关闭任务并释放依赖。

保留已有一次 publish.report 格式补报：只删除候选 SHA/快照资格，仍绑定原发布回合及其结果，不能重做发布、覆盖失败或掩盖未知副作用。结果继续严格解析 done/needs_input/blocked。原生权限按真实能力保留；删除 `fact_comparison_only`、`revision_comparison_only` 等已不成立的报告策略说明，不把提示词宣称为强只读隔离。

## PR 与恢复规则

复用现有 provider 配置、PR URL/number、仓库、base/head 仓库与分支校验，不新增 PR 身份对象或 PR 版本表。head SHA 可以变化；HeadRevision 仅为可选观察事实，不要求其存在、格式有效或与本地/历史 HEAD 相等才能推进。

已有 PR 的反馈沿用原分支、同一 PR，并在共享输入中携带已有 URL。同一执行返回另一 URL 时拒绝替换绑定，不新增“更换 PR”审批或恢复流程；另开候选使用现有入口。既有 PR 历史、放弃后晚合并与多候选冲突规则保留。

交付只接受选定执行的正确 PR：provider 的 merged 状态、Merged 标志及实际 MergeCommit/MergedAt 必须完整、一致；MergeCommit 保留有效 Git 提交引用检查和后继领取时的包含性检查。不查询本地 HEAD，不比较 PR head 与旧候选。唯一交付、幂等、任务/执行/PR/资源身份及并发版本检查继续使用现有事务；动态 HEAD/dirty/采样时间不作为资源身份比较条件。

重启沿用现有原输入与已知 PR 的对账，不自动重发或重新发布；无 PR URL 且发布结果未知时等待显式处理。保留真实执行失败、问题/权限、未知副作用、错 PR 和前置阻断；删除仅由候选/阶段快照变化触发的阻断及 Continue 分支。任务契约、配置与输入摘要、原生回合来源校验保持现有规则。

## 完整删除范围

- 删除 CandidateSHA/candidate_sha 在运行记录、输入/结果、交付 gate、DTO、查询、控制台、事务指纹和测试材料中的正常业务用途。
- 删除 StageInput/StageResult.Snapshot、StageSnapshot、复制/序列化、inspectStage/sameStageSnapshot、固定候选发布校验封装和专属错误/恢复分支。PR 查询直接沿用现有发布及交付路径。
- 清理仅供成果比较使用的 ContentDigest 计算、端口、capability、接线和报告策略文案。`InspectLifecycleWorkspace` 仍被资源创建/恢复使用，保留其必要实现；不新增平行的身份 inspection 接口，也不保留无调用的摘要计算或空方法。
- 同步 API/console、testkit、E2E helper、test-feedback 和 debug-template；有独立用途的 HEAD 观察可保留，不能改名后重新成为成果锁。同步当前 worker-staged-flow、worker-prompt-convergence、worker-publish-report、Orca 传输说明及对应 Plan；历史契约/验收保留版本和事实。

业务 FlowRef 更新为 `development/v3`，使用已有 `ValidateStoredExecutions` 等准入入口拒绝旧 development/v2 执行，再进行执行接管或阶段调度。不新增兼容读取、迁移、双写或回退；不为本次业务变更另增 SQL schema、worker 通用协议或原生 codec 版本。

## 验收与边界

自动项 WPF-A1–A6 见[专属验收](../acceptance/worker-pr-flow.md)。复用既有受控测试，只补本次行为差异；不增加真实模型/真实 PR 矩阵或完整恢复系统验收。必要人工界面检查可后置。

本次不处理 C:/SteerlineRuntime 的旧 102 执行或外部会话。实现验证使用既有隔离夹具；临时原生调试按项目规则生成新 C:/SteerlineDebug 包。
