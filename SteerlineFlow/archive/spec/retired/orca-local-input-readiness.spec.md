# Spec：Orca 本机输入与就绪

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../../docs/integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

普通开发，2026-10-13。[Plan](orca-local-input-readiness.plan.md) · [验收](../../../testing/acceptance/archive/superseded/orca-local-input-readiness.md)。

本机现场曾在发送前被 CLI 可达性预检拒绝，另有 tui-idle 超时；第三个候选完成实现但未完成发布。修正本机普通终端启动/投递的连接和失败处理，保持现有原生状态结束判定。

本机普通终端操作使用已有进程环境中的配对认证，连接必须先匹配本机 Orca bootstrap 的 runtime；命名 environment 继续使用其自身认证。认证不进入参数、配置、执行封套和证据，不回退管道或更换终端。

就绪等待超时或明确尚未就绪后，对原终端最多复查一次，第二次不超过一分钟；明确交互阻塞立即停止。CLI 外层预算必须覆盖原生等待及响应余量。最终失败保留原身份、错误原因和有界当前屏幕摘要，任务输入数为零；屏幕只作诊断。

绑定原 runtime/terminal/incarnation/request 的明确拒绝回执保存为 rejected；已接受但未启动、投递未知与拒绝不能混同。无法确认未投递时保持未知，不自动重发，也不能单按 runtime_unavailable 错误码判定拒绝。无监督、worker_done、模型结束标记、结构化会话或 agent 私有日志。

最小验证：必要正反例、两次普通只读回合、一个本机 SteerlineTest 真实 PR 合并交付。仅用新生成独立包，不触碰安装环境，不恢复已取消的第二轮或远程流程。
