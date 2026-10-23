# Spec：规划提案与受控草稿

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../Roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[三节点推进边界](../../roadmap/phase2-delivery.md)为调整入口。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：46；任务名称：规划提案与受控草稿；目标：`PHASE2`。旧方案状态：曾编写候选规格，未宣称实现或验收；当前待重定。配套 [Plan](phase2-planning-drafts.plan.md)。本任务由一个独立实现 PR 交付。

## 范围与前置

硬前置：#45。依赖 [本机客户端协议](../../../spec/phase2-local-api.spec.md)，遵守[共同约束](../../../spec/phase2-constraints.md)。
本任务是规划领域权威：固定 worktree、提案与 ID 预留、草稿/校验、正式文档只读快照和共享写入仲裁均在此实现。SQLite 仍是业务单写存储，Git shared ref 仍是任务数字分配权威。会话进程和正式发布由后继实现。

## 固定规划工作区及统一写入 guard

人工 RegisterPlanningWorkspace 命令固定项目规范仓库、受管绝对目录、专用分支与初始基线 commit。仅在空目标创建 worktree；已存在时核验归属与分支，不接管其他执行 workspace，不清理外部文件。一个项目只绑定一个规划 worktree。初始文档快照来自已提交 docs 下的受管文件，排除草稿；已有目标没有显式映射时显示未关联，不从自然语言生成 goal_id。

人工 planning.workspace.sync 在 idle 时核对项目绑定目标分支的固定 commit；只允许目标 commit 是当前规划 HEAD 后继的干净 fast-forward，受管草稿保持原版本。同步使用同一 guard、持久 Git 意图和旧 ref CAS，恢复规则与身份校验不变；分叉、外部改动或无法确认结果时保留原状态并要求人工核实，不自动 merge/reset。同步后从已校验的新 commit 建立完整文档快照，令基于旧正式版本的提案重新校验；已有 task contract 不随同步变化。该入口用于执行成果返回后继续规划，不开放给 agent。

SQLite 保存 workspace_id、generation、owner_kind/owner_id、状态 idle/turn/edit/reserving/publishing/reconciling/conflict、固定基线。提供 AcquirePlanningWrite、ValidatePlanningWrite、ReleasePlanningWrite 和 QueryPlanningWorkspace，作为服务内部受限接口；owner 由服务根据已认证命令或 session 绑定生成，不接受 agent 自报 owner。begin-turn 与 publish 获取同一排他 guard。每次文件、Git 或草稿副作用前再核 generation；旧 token 和迟到请求拒绝。

未完成的 turn/publish/reserve/edit 先由各原 owner 的恢复逻辑结算再开放写入，不能因超时清锁。未知 owner 类型或缺恢复证据保持 blocked，服务不能静默置 idle。所有共享路径操作都经 guard，领域 HTTP handler 不能各建第二把内存锁。正式历史读固定 commit，不等候活动草稿；新 Codex 轮次在 publishing/reconciling 时拒绝。

## 提案、人工确认及数字预留

路由为 /planning/workspace、/planning/proposals、/planning/proposals/{id}、/planning/documents 和 /planning/goals。命令 action 为 planning.workspace.register/sync、planning.proposal.create/revise、planning.proposal.reserve、planning.proposal.abandon、planning.draft.save、planning.proposal.validate。均用公共请求信封；生产 agent 只可调用服务授予的 draft.save/受限只读工具。人的 register/sync/revise/reserve/abandon 不因 agent 文本自动提交。提案必须由人的 create 命令先建立，后续 agent 只能在被授权的该提案内保存草稿。

Proposal 固定 documents_only/task_batch 模式、稳定 goal_refs、base_document_version 和自增 version；状态为 discussing/reserving/drafting/invalid/reviewable/needs_review/abandoned，发布阶段由发布任务扩展为 publishing/awaiting_spec_integration/awaiting_task_create/applied/outcome_unknown/conflict。Publication 有独立终态 rejected/cancelled_before_create/applied，不能与提案版本混为一项。任何正文、目标映射、任务依赖或基线改变都增加 proposal.version 并清除原校验/确认资格。

人工 revise 允许未发布提案，以及上一 publication 已确定 rejected/cancelled_before_create 后的 needs_review 提案；必须核 expected_version、没有未知副作用、原预留 ID 尚未成为中心任务。服务在共享 guard 下按当前正式基线增加新 proposal.version，修改同一交付范围的目标映射、候选依赖、文档槽或验收定义，保留既有发布记录且清空旧确认。预留后不改任务数字、稳定名称或将原 ID 指向另一交付；改变任务数量/拆分或替换范围时须按资格放弃旧提案并重新预留，旧号保留。提案模式不可在原 ID 下切换。修改 docs_only 文件范围同样需人确认，不由 agent 任意追加目标路径。

task_batch 模式的人确认“同意编写规格”后，按固定任务名称与本次 reserve request 原子预留一批数字，并生成每任务唯一 Spec/Plan 与身份记录的槽。扩展现有 scripts/roadmap_task_id.py 使用同一 refs/steerline/roadmap-task-ids：CAS 同时保存请求摘要、批次映射和计数器；响应丢失可按同请求取回原数字，不用单次 reserve 的盲重试。共享 Git ref 更新和 SQLite 之间持久化 Operation，重启核对原映射；两者冲突停在 reconciling，不再占号。CLI 分配与服务分配必须共用同一算法及 ref。

仅讨论和 documents_only 不占号。放弃提案后保留数字映射和审计；发布仍在进行或结果未知时拒绝 abandon，应先由发布服务判断是否可取消。未发布或上一 publication 已结算且 ID 未入图的 needs_review 提案可放弃，但不删除已生效文档、占号或历史。已创建任务不能通过 abandon 修改。服务生成的 task-ids JSON 随最终候选清单保存，agent 不能自选数字或直接写共享 ref。缺失工作区文件不能成为重置计数器的理由。

## 草稿校验和规划读模型

agent 只接 file_id、expected_version、正文；正式目标路径来自已登记槽，只允许 docs 下 Markdown，拒绝 .git、脚本、测试、设备路径、路径遍历、链接/目录联接及重解析逃逸。验证实际打开目标，不只验证字符串。初版固定每文件 1 MiB、每提案 64 个文件、总正文 16 MiB；超限发送前/写入前拒绝，错误定位到文件。服务生成的身份 JSON 不受 agent Markdown 白名单支配，且内容由数字分配事实构造。

文件正文与摘要先保存为受管不可变版本，再更新 proposal manifest；docs/.steerline-drafts 下文件是工作区投影。写入中断恢复按日志和摘要补全原版本或阻止继续，查询不能返回 manifest 与正文不符的版本。agent 读取受控草稿时标明 proposal/version；外部投影修改引发冲突并使旧校验失效，不自动覆盖。

校验覆盖每任务独立 Spec/Plan、预留身份/稳定名称、所有本地链接、验收引用、无环依赖、目标项目和固定任务契约；复用已有校验器的纯规则、图只读快照与数字检查规则，不从零复制 DAG 算法。草稿引用以候选路径/摘要预检；尚未提交的草稿不冒充已有 Git blob，要求固定 commit 的最终 task validate 由正式发布任务完成。机器通过只保证可判定结构和一致性，任务合理性仍待人确认。任务批次输入从显式结构化 task definitions 构造，不让模型文本成为任务状态。

DocumentSnapshot/GoalRef 查询由此拥有：正式内容只从已提交 commit/blob 读取，草稿按 proposal/version 读取；返回精确来源、正文摘要、路径/锚点、关联 Spec/Plan/task_ids、未解析关联及差异。正式映射由确认过的 manifest 提供，任务运行/交付状态仍从中心查询关联，不能复制到 Markdown 当作业务权威。发布任务通过受限的 ActivateDocumentVersion 推进正式快照；本任务负责注册/人工同步时的基线校验，不开放客户端直接调用该接口。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| PD1 | 重复注册同项目回读同一 worktree；错仓库/已有目录/任务 workspace 被拒绝，无覆盖 |
| PD2 | turn/edit/reserve/publish 争抢同一 guard 至多一个生效；旧 generation、重启未知 owner 均不能写 |
| PD3 | reserve 在 ref CAS 前后及 SQLite 回执前后故障仍取回原 ID；与 CLI 竞争无重复，放弃后不复用 |
| PD4 | 负向路径/类型/大小和外部改动全部拒绝；草稿部分落盘与正文查询重启一致 |
| PD5 | 缺 Spec/Plan、错身份、悬空/循环依赖及错误链接定位；修复后新版本校验，旧确认不能沿用 |
| PD6 | 正式/草稿/目标映射查询来源可追溯，未关联不伪造；离线查询复用同一只读版本 |
| PD7 | documents_only 不占 ID；放弃和未知发布不能修改既有任务或丢失证据 |
| PD8 | 人工 fast-forward 同步保留受管草稿并使旧基线确认失效；分叉/响应丢失不 reset、不重复动作 |
| PD9 | 已发布但中心未创建、上一 publication 已终结的 needs_review 可修订/放弃；旧确认失效，数字不变；未知或已入图情况拒绝 |
