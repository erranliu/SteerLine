# Spec：Codex 规划会话服务

任务 ID：18；任务名称：Codex 规划会话服务；目标：`PHASE2`。状态：规格已编写，尚未实现或验收。配套 [Plan](phase2-planning-sessions.plan.md)。本任务由一个独立实现 PR 交付。

## 范围与前置

硬前置：#14、#17。依赖 [能力验证](phase2-agent-capabilities.spec.md)与[规划提案权威](phase2-planning-drafts.spec.md)，后者已包含 #16。本任务遵守[共同约束](phase2-constraints.md)，交付真实 app-server 会话服务，不修改任务 worker 后端。

## 会话与输入

提供 /planning/sessions、/planning/sessions/{id}、/planning/sessions/{id}/events 及正文查询。通过 planning.session.create/resume、planning.turn.start/steer/interrupt、planning.approval.answer 命令接公共信封。PlanningSession 存项目/workspace/thread_id、CLI/配置/能力摘要；PlanningTurn 存输入摘要、原 request、真实 turn ID、状态与 write generation。新 session 绑定同一固定目录，但必须新建 thread，不继承旧消息、摘要或 fork；同一 session 的 resume 按原 thread。

开始轮次前取得 #17 的 turn guard，核对所选 Codex 版本和权限能力报告。轮次不确定、等待工具或审批时仍持有 owner；另一个 session 可建立和读历史，但不能开始冲突轮次。草稿工具把原 session/turn/generation 绑定到 #17，模型只持 file_id 不持产品认证凭证；手工发布/编辑与 turn 互斥。结束或成功中断只有经确定的终态与无在途草稿调用后释放；不根据输出沉默释放。

明确区分输入保存、外部接受、轮次开始、轮次结束。steer 必须绑定当前 turn；不支持的能力拒绝，不暗中开下一轮或将排队显示为已读。interrupt 回执不等于已终止，查询原 turn 结算。初版不提供“编辑已发送输入并重跑”，不配置自动 goal continuation，不由主 agent 自主新建规划或任务。

## 协议隔离、事件和恢复

只使用 #14 验证的一种受控工具通道和固定 schema；发现未知版本、配置注入、重新启用 exec/多 agent 或新增有副作用工具，启动拒绝。审批仅限允许能力，UI 即使同意也不能放宽禁止执行/写正式文件的产品边界。Codex 权限审批与 SteerLine reserve/publish/execute 确认分别存储和展示。

复用规范会话事件与正文模型，增加规划 session 的来源作用域，不伪造 Task/Execution ID。commentary、final_answer、工具事件、等待审批和 turn_completed 独立存储；原始来源无法给出的字段标缺口。事件与正文先持久化再推进游标，重复事件去重，丢失历史显示 gap；有界内存流不当成唯一存储。

服务持有 app-server 进程身份，UI 关闭不终止会话。服务退出保留 owner 与外部动作意图；重启先核原进程/thread/turn 与在途工具，再允许新轮次。对 thread/start 或 turn/start 已发送但未保存回执的窗口，只在 #14 已验证的明确关联可恢复时绑定原对象；否则 outcome_unknown 并阻止替代创建。不能用会话标题或时间近似搜索后重发。历史读取通过公开协议与自有存储，不解析 Codex 私有日志。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| PS1 | 真机新 session 无旧上下文且同 worktree；恢复只打开原 thread |
| PS2 | 两个真实 session、人工编辑和按 #17 契约建立的 publish owner 争用统一 guard；迟到工具和旧 generation 拒绝。发布消费者可用夹具，真实 #19 交叉验收归 #24 |
| PS3 | 真实流式正文、最终回复、审批/拒绝、steer、中断逐项有明确状态与回执 |
| PS4 | 服务在 start/工具保存/轮次结束窗口退出后按原身份恢复或阻塞，没有第二 thread/turn 或重复写 |
| PS5 | exec/子 agent/产品命令绕过被代码拒绝，凭证及宿主私有信息不进入模型正文 |
| PS6 | UI 关闭仍可继续原会话；重开补读完整，缺口不伪造，恢复后不自动开始新目标 |
