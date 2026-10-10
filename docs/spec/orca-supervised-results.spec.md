# Spec：Orca supervised 分步结果接线

用户直接开发授权；不占号、不进入 roadmap。前置接口已由[原生探针](../acceptance/orca-supervised-result-probe-2026-10-10.md)验证。配套 [Plan](orca-supervised-results.plan.md)、[验收](../acceptance/orca-supervised-results.md)。

本机和远端共用 supervised lifecycle。每轮 worker_done 的 body 保留原生摘要，payload 在 taskId/dispatchId/outcome 外携带 steerline_result={version:1,command_id,input_version,body}。implement 的 body 是完整普通回复；report/publish 的 body 是中心要求的完整结构化报告。CLI 只使用完整 --payload，不混用便捷 payload 参数；其他授权参数复制原生 preamble。完整消息限 8 KiB，超限明确拒绝，不截断或分块。

领取能力开放之前，结果路径须接入。创建原生 Task 前将结果指令加入冻结输入；保存输入版本和原 ProcessIdentity。只消费原 Run/Task/Dispatch、发送者及目标 mailbox 一致的完成消息；拒绝外层重复字段，steerline_result 字段名精确匹配，严格解码结果，核对中心 Command/版本。完整正文、原消息 ID、摘要和结果诊断先随现有状态 envelope 提交，再 ack。

规范来源为 worker_declared，不伪造 provider final phase。只有原 worker/terminal/incarnation、工作区与 runtime 核验一致，worker 记录的终端句柄与同快照 terminal 一致，且原 Dispatch 实际结算后，才能返回 FinalVerified。缺结果、非法/超限/错绑定与冲突完成保持可诊断的未核实结果，不用摘要替代。已验证并冻结的结果可在资源释放和服务重启后读取，不重新发送任务、读取私有日志或要求远端存活。不同回合保留各自 Command/Dispatch 与正文。

将已核验结果写入现有规范会话正文存储，重复恢复幂等、冲突不可覆盖；缺证据不存完整 final。沿用原 execute/report/publish、原会话续轮、工作区与 PR 事实核验和唯一 TaskDelivery，不新增另一套流程或兼容旧状态。

最小自动验收 OSR-A1–A6 见专属验收。rebase 后在独立调试包验证共同交付链，包初始化支持 LF/CRLF 标题，Windows 私有 ACL 设置不对已有当前用户 owner 重复请求 WRITE_OWNER；保留严格 ACL 核验。首次输入草稿提交和默认模型账号拒绝仍属独立原生前置，不靠本变更伪造通过。
