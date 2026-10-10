# Spec：Orca supervised 分步结果接线

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](orca-terminal-transport.spec.md) / [Plan](orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

当前规则由用户直接授权的[提示词收敛 Spec](worker-prompt-convergence.spec.md)、[Plan](worker-prompt-convergence.plan.md)恢复；不占号、不进入 roadmap。原 payload 方案的 [Plan](orca-supervised-results.plan.md)、[原生探针](../acceptance/orca-supervised-result-probe-2026-10-10.md)与[验收](../acceptance/orca-supervised-results.md)保留历史结果，不能作为当前传输协议。

本机和远端共用 supervised lifecycle。每轮 worker_done 保持 Orca 原生摘要和身份/结果字段，仅结算生命周期；不要求模型包装 steerline_result。中心通过公开 worker-read 采集本次输入段的回复：implement 普通说明，report/publish 为业务结果 JSON。身份由程序绑定，业务模板与后端无关。

adapter 原样发送中心冻结的输入。保存输入版本和原 ProcessIdentity，核对完成消息原 Run/Task/Dispatch、发送者、目标 mailbox 及实际结算；完成消息先提交再 ack。按原 Task preamble 的 user 消息、稳定来源和消息 ID 定位本轮工具调用之后的 assistant 回复，拒绝跨 Task、来源变化、缺失或自身裁剪的正文；正文和绑定通过原有 envelope 提交。迟到结果只读重采，不重发原输入。

规范来源为 worker_declared，表示应用绑定的步骤回复，不伪造 provider final phase。只有原 worker/terminal/incarnation、工作区与 runtime 核验一致、原 Dispatch 实际结算及本轮回复完整可用，才能返回 FinalVerified。缺执行正文不阻止报告轮；缺报告/发布正文不推进。已冻结回复在释放和重启后复用，不解析私有日志；不同回合保留各自 Command/Dispatch 与正文。

将已核验结果写入现有规范会话正文存储，重复恢复幂等、冲突不可覆盖；缺证据不存完整 final。沿用原 execute/report/publish、原会话续轮、工作区与 PR 事实核验和唯一 TaskDelivery，不新增另一套流程或兼容旧状态。

当前最小自动验收为[WPC-A1–A4](../acceptance/worker-prompt-convergence.md)。原 OSR-A1–A6、调试初始化/ACL 修复及安装记录保留为对应历史构建事实，不推定当前真实模型交付通过。
