# Spec：恢复 Worker 三轮流程与统一提示词

> 2026-10-12：本文保留为原实现的历史契约。当前有效业务规则和实施/测试安排已由 [统一 PR 流程 Spec](worker-pr-flow.spec.md) 与 [Plan](../plans/worker-pr-flow.plan.md) 替代；下文的候选 SHA/阶段快照比较不得继续用于当前执行，历史验收结果不改写。

当前提示词以本文末尾的 2026-10-10 精简补充为准，发布补报流程见 [Spec](worker-publish-report.spec.md)。

用户直接授权恢复实现；普通开发，不新增 roadmap 任务。配套 [Plan](../plans/worker-prompt-convergence.plan.md)、[验收](../testing/acceptance/worker-prompt-convergence.md)。沿用 [#84 流程](worker-staged-flow.spec.md)的 implement.execute → implement.report → publish；publish 不增加报告轮。

三个步骤各有一个固定短模板。首次输入、反馈、恢复和重试使用同一渲染入口，后端不得装饰提示词。任务引用单独冻结为事实；feedback/resume/blocker、已有候选与 PR 仅作为数据附加。报告只带源执行终态、正文可用性/缺口、必要快照和固定任务引用，不重放原 prompt、内部状态记录或后端权限说明。权限策略继续作为原生控制元数据保存和核对。

原实现轮已经核实结束且符合接续资格时，可以进入报告轮；没有完整执行正文应如实表达为未知，不是阻止 report 的理由。报告不能覆盖原执行失败，也不能补做检查。报告/发布正文须绑定本步骤输入并通过严格 JSON 与现有 Git/PR/工作区事实核验，模型声明不直接成为业务状态权威。

Orca 原生 worker_done 只结算生命周期，摘要不替代业务正文。不要求 steerline_result、模型回填中心身份、写结果文件或手工封装传输。supervised adapter 用公开 worker-read transcript，核对原 runtime/Dispatch/source，按 Orca 原 Task preamble 的 user 消息定位输入段，取该段最后工具调用之后的 assistant 文本回复。后续 Task、错来源、工具输出和自身被裁剪的回复不可冒充结果；其他历史/工具裁剪不否定完整的本次回复。业务 JSON 由中心解析，adapter 不选择下一步。

原输入段出现没有当前来源绑定的后续 user 输入时，保留采集缺口，不将其回复归为原 Task。原生活动消息/工具问答仍走原 Dispatch/call，不通过自由文字推断新 user 输入的归属。

原完成消息先持久化再 ack；迟到回复在原输入段只读重采。回复内容及来源通过原有 envelope 和 StageResult 保存，推进前提交；恢复和释放后复用已冻结结果。不得重发任务或解析 agent 私有日志。流程版本为 development/v2，Orca 状态 codec 为 13；不迁移旧格式，不修改安装环境或已有运行。

最小自动验收 WPC-A1–A4 见专属验收。这次不另设可行性探针或真实模型验收门槛；使用既有隔离夹具验证实现。

## 2026-10-10 当前提示词精简

用户直接授权按“当前动作、不可推导参数、机器消费格式”精简，覆盖上文的事实包要求。沿用最新主干的 [发布补报](worker-publish-report.spec.md)，不改变进入条件、结果核验和恢复资格。

| 阶段 | 模板与附加输入 |
| --- | --- |
| implement.execute | `Implement the following Spec and Plan.`；Spec/Plan 路径和去重的其他契约文档路径 |
| implement.report | `Format the preceding implementation result as JSON.`；实现报告格式 |
| publish | `Push and create or reuse a PR.`；配置指定的 remote 和 PR base，要求严格 JSON；done 必须含 `outputs.pr_url` |
| publish.report | `Format the preceding publication result as JSON.`；发布报告格式 |

正常报告利用原会话，不重复正文、文档引用、任务身份、工作区、当前分支、base、候选 SHA、摘要校验值或快照。内部冻结引用与核验保持。实现报告不要求模型抄写候选 SHA，程序从核实快照绑定；发布及发布报告要求结构化实际 PR URL。结果格式列必要字段及 done/needs_input/blocked。源执行失败或正文证据缺口才补诊断；报告重试时上一回复是失败报告，补绑定的原执行正文（最多 16 KiB，截断标缺口）。反馈仅提供原问题、相关上下文、选中答案文字、用户文字/答案和具体阻断，不发送内部恢复状态、全部选项或重复 Git/PR 事实。最小验收为 [WPC-A5](../testing/acceptance/worker-prompt-convergence.md)。
