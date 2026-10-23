# Spec：文档审阅与确认原生界面

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../Roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[历史三节点推进边界](../../roadmap/phase2-delivery.md)追溯当时安排。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：52；任务名称：文档审阅与确认原生界面；目标：`PHASE2`。旧方案状态：曾编写候选规格，未宣称实现或验收；当前待重定。配套 [Plan](phase2-review-ui.plan.md)。本任务由一个独立实现 PR 交付。

## 范围与前置

硬前置：#44、#48。依赖 [WinUI 基座](phase2-winui-foundation.spec.md)和[正式文档发布](phase2-document-publish.spec.md)，通过 #48 获得 #46 的提案与 ID 预留能力。遵守[共同约束](../../../spec/phase2-constraints.md)。
交付人工审阅与确认页面，独立于规划对话实现；可从受控提案列表或 proposal 深链接进入。页面只提交已有领域命令，不解析数据库或自行生成任务 ID。

本任务同时提供规划初始化与“同步目标分支”入口，调用 #46 的人工 register/sync，显示固定目录/分支和现有 owner；分叉或外部修改保留诊断，不在 UI 中 reset。用户可从讨论建议或手工表单新建 task_batch/documents_only 提案，确认目标路径、拟定任务/依赖或产品文档范围后调用 proposal.create；模型建议不自动建提案。审阅 UI 与对话 UI 通过 proposal_id 深链接连接，单独验收可用手工提案输入。

## 受控流程

1. 讨论中的 task_batch 显示拟定任务/依赖及其范围。用户“同意编写规格”提交固定 proposal/version 的 reserve 请求；返回原 task_ids 后展示，不由客户端自增。documents_only 显示无任务创建的范围。
2. drafting/invalid/needs_review 显示每份 Spec/Plan、正式目标路径、完整差异、版本、机器检查及错误位置。机器通过与人工判断分栏；后者明确任务拆分、依赖、验收与影响，由人审阅。“返回修改”可由人提交服务允许的 proposal.revise；发布后重审须显示原 publication 的 rejected/cancelled_before_create 终态与已生效文档，不能撤回历史。预留后变更任务数量或替换范围须按服务规则放弃重建，旧号保留。
3. reviewable 展示完整文件集和拟创建 DAG 预览；正式 DAG 和草稿预览明确隔离。确认固定 proposal/version、base_document_version、manifest 摘要和当前校验结果。任一正文、依赖、目标或基线变化后确认资格清空。
4. 确认前与发布后分别显示正式文档版本、Git commit、目标分支规格集成和中心 batch 回执。明确要求以保留原规划 commit 祖先关系的 merge/fast-forward 集成；同内容 squash/cherry-pick 不满足本阶段支持条件。awaiting_spec_integration 显示目标分支、规划提交和待核验项，不自动合并/推送或宣告已创建。
5. 服务 can_cancel 为真时提供“结束本次创建等待”，绑定原 publication/version，解释保留已发布文档和预留数字；取消成功进入 needs_review。取消与中心创建竞争由服务仲裁，结果未知不能本地清锁。outcome_unknown 保留原请求、最后证据与恢复入口，只查原结果。applied 后可跳正式 DAG；abandon 只对服务允许的未发布/needs_review 提案开放，不能作用于已创建任务。

确认前显示真实 auto/manual/paused 模式及可能自动派发的后果，不改变派发模式。产品方向的 documents_only 发布使用同样差异和确认，不预留 ID、创建空 DAG 或虚构执行任务。

## 可恢复交互

未完成命令的完整信封、摘要与当前审阅版本持久保存；退出 UI 不取消后端操作。重开后先读原 command/publication/reserve；同一项目权威明确 command_not_found 时可补交原信封，不换 ID 或重复占号。已有记录只查询/对账，已有未知副作用不重放。冲突保留阅读位置并展示新旧差异，必须重新审阅新版本。未知状态/权限不足/服务离线不给变更按钮。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| DR1 | 真实预留/放弃过程只占一次号，documents_only 无占号；错误项准确定位 |
| DR2 | 缺 Spec/Plan、版本变化、外部文件修改和同提案竞争不能绕过确认 |
| DR3 | 发布中、等待规格集成、中心结果未知与已创建分别显示并可追溯，不多建批次 |
| DR4 | UI 关闭/重开/切项目后原 request 与审阅版本保持，晚到回执不串项目 |
| DR5 | 真实完整文件差异和 DAG 预览可键盘审阅，人工与机器通过不混同，模式提示准确 |
| DR6 | 人工规划初始化、新建两类提案与同步目标分支均可从 UI 完成；未授权模型建议不占号/写正式路径 |
| DR7 | 发布后确定拒绝/取消可重审同一提案；取消竞争、同内容 squash 拒绝及未送达信封补交均与服务一致，无永久空请求等待 |
