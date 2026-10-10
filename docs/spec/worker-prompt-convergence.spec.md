# Spec：恢复 Worker 三轮流程与统一提示词

用户直接授权恢复实现；普通开发，不新增 roadmap 任务。配套 [Plan](worker-prompt-convergence.plan.md)、[验收](../acceptance/worker-prompt-convergence.md)。沿用 [#84 流程](worker-staged-flow.spec.md)的 implement.execute → implement.report → publish；publish 不增加报告轮。

三个步骤各有一个固定短模板。首次输入、反馈、恢复和重试使用同一渲染入口，后端不得装饰提示词。任务引用单独冻结为事实；feedback/resume/blocker、已有候选与 PR 仅作为数据附加。报告只带源执行终态、正文可用性/缺口、必要快照和固定任务引用，不重放原 prompt、内部状态记录或后端权限说明。权限策略继续作为原生控制元数据保存和核对。

原实现轮已经核实结束且符合接续资格时，可以进入报告轮；没有完整执行正文应如实表达为未知，不是阻止 report 的理由。报告不能覆盖原执行失败，也不能补做检查。报告/发布正文须绑定本步骤输入并通过严格 JSON 与现有 Git/PR/工作区事实核验，模型声明不直接成为业务状态权威。

Orca 原生 worker_done 只结算生命周期，摘要不替代业务正文。不要求 steerline_result、模型回填中心身份、写结果文件或手工封装传输。supervised adapter 用公开 worker-read transcript，核对原 runtime/Dispatch/source，按 Orca 原 Task preamble 的 user 消息定位输入段，取该段最后工具调用之后的 assistant 文本回复。后续 Task、错来源、工具输出和自身被裁剪的回复不可冒充结果；其他历史/工具裁剪不否定完整的本次回复。业务 JSON 由中心解析，adapter 不选择下一步。

原输入段出现没有当前来源绑定的后续 user 输入时，保留采集缺口，不将其回复归为原 Task。原生活动消息/工具问答仍走原 Dispatch/call，不通过自由文字推断新 user 输入的归属。

原完成消息先持久化再 ack；迟到回复在原输入段只读重采。回复内容及来源通过原有 envelope 和 StageResult 保存，推进前提交；恢复和释放后复用已冻结结果。不得重发任务或解析 agent 私有日志。流程版本为 development/v2，Orca 状态 codec 为 13；不迁移旧格式，不修改安装环境或已有运行。

最小自动验收 WPC-A1–A4 见专属验收。这次不另设可行性探针或真实模型验收门槛；使用既有隔离夹具验证实现。
