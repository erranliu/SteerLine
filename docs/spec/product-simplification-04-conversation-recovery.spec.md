# Spec：只保存最终回复与原会话恢复

状态：工作区实现、最小自动验收完成；已有平台故障限制见专属验收。依据：2026-10-07 用户要求实施精简建议 1、2。直接授权工作区修改；未分配外部 Task ID，不声明 DAG 准入。选定[草稿](../roadmap/drafts/conversation-recovery-simplification.md)，实施见[Plan](product-simplification-04-conversation-recovery.plan.md)，行为与证据见[验收](../acceptance/product-simplification-04-conversation-recovery.md)。

## 固定行为

1. 中心保留 Decision 的完整问题、上下文、选项、答案与原请求投递事实；不改变问答格式。保留活动摘要、原 session/turn/call、停止、投递、PR 与交付核验。多后端、多候选和不同任务人工并行保持。
2. 会话采集只保存可核实的完整最终回复、必要 session/turn/host wait 事实及明确缺口。删除原始会话字节持久化/查询、终端正文、工具过程、普通消息/增量映射和独立 JSONL 会话重放。原生日志和用于投递/恢复的 app-server 操作 journal 保留为原生诊断/协议证据，不复制到中心会话表。
3. 最终正文仅在原生明确 final_answer 且 turn 有真实完成事实时保存。准备轮不显示为业务 final；缺失正文、未知 phase 与读取错误明确显示，不能采用最后一条消息冒充 final。最终正文仍有大小上限、UTF-8 分页、摘要核验、历史读取和原候选身份隔离。中心不提供完整过程副本。
4. 会话读写删除无消费者的 raw/replay 和增量正文机制。必要原生历史读取继续用于读取 final 与核验 turn；不可因删过程采集删除工具回复对账。持久格式变化提升版本，旧开发格式拒绝，不迁移或双写。
5. 运行恢复只继续原 Execution 的原 session。删除 new_session 选项、分支和隐式替代会话。原会话不可核实时保留工作区、PR、历史和原因，要求人工处理；在确认原执行停止且满足候选资格后，用户可另开候选。原输入结果未知只核对原请求。
6. 允许核实旧进程树停止后以新进程/Attempt 恢复同一个原 thread；不能清空 session ID 来触发 thread/start。原执行尚未取得会话的未完成准备只对账原已保存意图，不能由恢复 Decision 新建替代 worker。

## 最小验收

| ID | 通过条件 |
| --- | --- |
| PS4-01 | 混合工具、commentary、未知事件、部分 final、完整 final 的原生样本只保存完整 final 与必要 turn/缺口，不保存 raw/过程正文；重读幂等 |
| PS4-02 | 中文完整 final 与历史 final 可读，准备轮排除，错误/缺正文不冒充 final，分页/摘要与候选隔离保持 |
| PS4-03 | 只有 continue 原会话；new_session 明确拒绝；原会话缺失/旧执行未停止/未知动作不新建 session；同 thread 恢复仍可用 |
| PS4-04 | 当前格式重启与反馈对账、一次 Delivery/后继释放保持；退役原始表/接口/重放确实删除；测试自身诊断与清理保持 |
