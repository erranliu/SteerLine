# Spec：正式文档发布与任务创建

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[三节点推进边界](../../../roadmap/drafts/phase2-delivery.md)为调整入口。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：48；任务名称：正式文档发布与任务创建；目标：`PHASE2`。旧方案状态：曾编写候选规格，未宣称实现或验收；当前待重定。配套 [Plan](phase2-document-publish.plan.md)。本任务由一个独立实现 PR 交付。

## 范围与前置

硬前置：#46。依赖[规划提案与受控草稿](phase2-planning-drafts.spec.md)，遵守[共同约束](../../../spec/phase2-constraints.md)、[任务契约](../completed/task-core.spec.md)和[固定 DAG](../completed/task-dag.spec.md)。
交付 documents_only 与 task_batch 两种发布、固定 Git 文档版本、中心创建和全部部分失败恢复。不自动开始任务，不自动合并任何 PR。

## 确认和准备

planning.proposal.publish 命令固定 proposal/version、base_document_version、完整 candidate manifest 摘要、验证回执和人工确认 actor；planning.publication.reconcile 只能核实原 publication。planning.publication.cancel 仅用于下面规定的中心创建尚未开始的人工取消，不撤回已发布文档。查询 /planning/publications/{id} 返回原请求、阶段、Git/中心各自结果、can_cancel 和下一步原因。agent 不可调用以上命令。

取得 #46 publishing guard，拒绝未结算 turn/编辑/预留或其他发布。再次检查 worktree HEAD/index/文件、正式快照及候选摘要、任务定义和依赖资格。重用已创建任务的编号、修改已创建契约、外部差异或失效基线均拒绝。限定候选 Markdown 与服务生成的身份 JSON，不把 untracked 代码或用户暂存文件纳入提交。

## 完整版本发布算法

1. 在 SQLite 持久化 publication 意图、基线 commit、候选全部路径/blob 摘要、预期提交信息/时间和原命令；固定输入后再写 Git 对象。使用隔离 index 构造候选 tree，只更改候选白名单，保留非文档树。
2. 从固定 tree、parent 和保存的提交元数据生成确定 commit，保存/恢复其 hash；按旧值 CAS 更新专用规划分支。分支或文件被外部改动时保留 conflict，不 force/reset 或重新生成另一个提交。原进程崩溃后先核 branch/object 与日志确定动作是否完成。
3. 在 guard 内按日志物化正式路径，逐文件记录前后摘要；同时结算该 worktree 的实际 Git index，不能只更新构树用的隔离 index。准备阶段保存实际 index 的旧逻辑 tree 摘要；取得实际 index.lock 后再次核验旧值，构造与候选 tree 一致的新 index 并原子替换、持久化。不得覆盖与已记录旧/新 tree 都不符的 index。恢复时当前 index 为旧 tree 则继续原更新，为新 tree 则确认已应用，其他值为 conflict。文件或 index 中断都只能沿原 manifest 对账；查询仍返回旧正式 commit，禁止新 agent 轮次读取中间目录。
4. HEAD、实际 index tree、受管正式文件和候选 commit 全部一致，且除受管草稿投影外没有意外暂存/未暂存修改后，才在同一 SQLite 事务调用 ActivateDocumentVersion，写入 manifest、GoalRef/关联和 publication 阶段。查询从此返回新 commit；任意时点读旧或新完整版本。离线查询同样使用已提交指针，不读半套文件。
5. documents_only 在此 applied，无 ID 预留或 DAG 创建。task_batch 继续下面的规格交接与中心创建；文档发布已完成不等于任务已创建。

原始工作树不是外部无锁读者的原子目录；保证对象是服务查询及受 guard 约束的 agent。故障恢复不能删除既有证据、静默回退正式指针或跳过失效校验。

## 规格进入执行目标及创建

执行任务的 base 将从项目绑定的目标分支取得，其 Spec/Plan 和共同引用必须与固定契约内容一致。规划分支 commit 不自动满足该条件。

task_batch 发布后回读固定目标分支的实际 commit，同时核验原规划发布 commit 是该目标 commit 的祖先，以及全部契约 blob 摘要完全一致。首版只接受保留原规划提交的 merge/fast-forward 集成；只复制文件、squash 或 cherry-pick 即使内容相同也不能放行，返回 integration_ancestry_mismatch 并保留等待状态。确认前及等待页都要解释这一要求，确保后续 #46 fast-forward 同步可行。不自动合并、推送或改写主线，也不先创建可能自动派发的任务。

未满足集成条件时为 awaiting_spec_integration，显示原规划 commit、目标分支和差异；原 publication 持有 guard、正式历史可读。人可继续集成，也可按下述受控 cancel 结束等待；不能在没有结束路径的状态永久占用规划写入。

目标分支通过祖先和内容两项校验后，以该固定 commit 生成 task-batch/v1 输入并持久化完整 payload、摘要和原中心 create request，再运行要求已提交 blob 的最终 task validate、调用既有 CreateTaskBatch。状态/依赖仍由中心事务重新核验；重启不能从更新的目标 HEAD 重新生成同 request 的不同 payload。创建响应丢失先按 request_id 查同一回执，禁止第二请求或占号。只有文档发布、目标规格核验和中心创建全部确认才 applied。

已确定无中心创建副作用的最终拒绝，先结算所有 Git/文件/index 动作并核实原预留 ID 未创建为中心任务，原 publication 终态为 rejected。提案转为 needs_review，保留已发布 commit、旧确认和拒绝原因，再按原 owner/generation 释放 guard。#46 明确允许人修订这种提案：基于当前正式版本增加 proposal.version、清空旧校验/确认，保留同一交付的原数字并建立新 publication；旧 publication 不重开，已生效文档历史不倒退。部分副作用或中心结果未知时仍不能解锁。applied 也仅在全部动作结算后释放原 guard。

cancel 只允许 awaiting_spec_integration，或尚未登记中心创建意图的 awaiting_task_create。服务事务核 expected_version、原 owner、Git/index 已结算且没有中心创建 intent/在途请求；与准备中心创建的事务竞争至多一方成功，并核实原任务 ID 尚未创建。成功时 publication 为 cancelled_before_create，提案为 needs_review，保留 Git 提交和占号，再释放原 guard。迟到的 reconcile 看到终态不得继续创建。创建意图已经登记或结果未知时返回 cancel_not_eligible，须先按原请求对账，不能用取消代替任务丢弃。

不改变已有派发模式：若项目为 auto，界面确认前和成功回执必须说明任务可能在下一轮调度启动；manual/paused 保持原值。任务状态与交付查询从中心取得，不由 publication 自行写 closed/ready。后续目标分支漂移继续由现有领取前规格核验阻塞。

## 恢复和验收

| 编号 | 通过条件 |
| --- | --- |
| DP1 | documents_only 完整发布不占号；task_batch 全文件固定且不改变既有契约 |
| DP2 | tree/commit、ref CAS、文件物化、实际 index 替换与正式指针前后注入崩溃；恢复后 HEAD/index/文件一致，连续发布和同步不出现意外 dirty 状态 |
| DP3 | 发布与按 #46 契约建立的 turn/edit owner 竞争、外部改动和旧 generation 全部拒绝；未知 owner 不释放。此处用服务消费者夹具，不依赖 #47；真实 Codex 交叉验收归 [二期验收活动](../../../acceptance/phase2.md) |
| DP4 | 未集成或同内容 squash/cherry-pick 不创建；保留规划提交的人工 merge/fast-forward 且 blob 一致后才按原 ID 创建，并可再次同步 |
| DP5 | Git 已完成/中心未完成、中心已完成/回执丢失均按原操作结算，只有一个 commit 身份与一个 batch |
| DP6 | UI/离线查询同时读新旧版本与结果待核实状态一致；草稿、正式、已创建三个事实不混同 |
| DP7 | auto/manual/paused 模式均不被发布命令重置，回执说明真实派发后果 |
| DP8 | 文档已发布、中心确定拒绝后实际执行 needs_review→revise→validate→新 publication，沿原 ID 成功；旧确认失效、旧 publication 不重开 |
| DP9 | 等待时取消释放原 guard并保留文档/占号；取消与创建意图竞争只有一个胜出，迟到恢复不创建；未知创建拒绝取消 |
