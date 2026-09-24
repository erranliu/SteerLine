# 控制台后续设计决策：长期双后端与轮次折叠

日期：2026-09-24。状态：已落实为待实施的 Spec/Plan；本文件保留设计动机与导航，不重复定义行为契约。

## 目标

local Codex 与 Orca 均长期正式支持。两条执行路径使用一个业务状态机、一套执行操作契约、一种会话读模型和共同契约测试；具体平台与 agent 输出差异留在适配器/解码器。进度消息 commentary 与最终回复 final_answer 分开，一轮 turn 明确结束后自动折叠过程。UI 关闭不停止服务，消息折叠不修改执行或业务状态。

## 交付顺序

| 阶段 | 内容 | 正式文档 |
| --- | --- | --- |
| PR3 | 既有后台服务生命周期，内容与验收不变 | [Spec](pr-03-service-lifecycle.spec.md) · [Plan](pr-03-service-lifecycle.plan.md) |
| PR4A | 长期双执行后端契约、固定身份、能力、动作对账与生命周期分离 | [Spec](pr-04a-execution-backends.spec.md) · [Plan](pr-04a-execution-backends.plan.md) |
| PR4B | 统一会话事件、真实轮次/阶段、全文、游标、去重与恢复 | [Spec](pr-04b-conversation-events.spec.md) · [Plan](pr-04b-conversation-events.plan.md) |
| PR4C | 原 PR4 控制台与逐轮折叠、阅读位置、手动展开和能力降级 | [Spec](pr-04-console.spec.md) · [Plan](pr-04-console.plan.md) |
| PR5 | 决策与恢复复用共同接口，送达/轮次开始/应用证明分开 | [Spec](pr-05-intervention.spec.md) · [Plan](pr-05-intervention.plan.md) |

阶段名不是 GitHub PR 编号。包括已合并的阶段 1/2，共七个串行交付；各阶段审核修复、必需验收并确认合并后才开始下一阶段。原 PR4 文件路径保留为 4C，详细规则以相应 Spec 为准，执行与验证以相应 Plan 为准；总流程见 [总览](console-upgrade.md)。本次只更新文档，定时协调保持暂停。

## 关键选择及原因

- **抽象先于 UI。** 原有 Spawner 只覆盖 local，Orca 另有 Service 启动分支；共同操作、固定 ref、能力与类型化回执先落地，避免 UI 再包一层宿主条件。
- **观察与取消分开。** 本机停服继续遵循 PR3，未来远端可以只停止中心监督。DetachObservation 不能暗中取消执行，StopExecution 必须核验原身份；释放句柄也不等于业务成功。
- **以真实协议识别消息。** Orca 的结构化读取声明不保证包含 phase/turn。两边各自验证；只有原始文本时使用执行过程/执行摘要，不能通过“最后一句”或静默时间猜 final。精确双链路能力缺口保持未验收，不能靠降级宣告目标完成。
- **持久消息与 UI 视图分开。** 内容先存、cursor 后推进；正文完整可分页。折叠状态只存 UI 偏好，历史手动展开不被刷新覆盖，待处理问题和缺口不藏在过程区。
- **答案送达与应用分开。** send accepted 和 turn started 都不是 Decision.applied。必须绑定原问题、attempt、外部身份和固定答案的实际应用证据；结果未知时不盲目重发。
- **长期结构共用。** backend 与 agent/target/protocol 正交；local continuation、Orca retry 仍按真实能力限制。新能力要独立实现和验证，不借重构扩大恢复资格。

## 与中心状态机计划衔接

[Spec 02](02-execution-model-and-adapter.md) 复用 4A/4B 接口及契约测试，负责 WorkflowRun/Execution/StepAttempt、SQLite 和历史迁移；[Spec 03](03-remote-execution-and-viewer.md) 复用相同输出/UI，负责原生远端验证。谁先落地，后者就在其权威接口上扩展，不建立竞争写者。

Spec 02 的新 SQLite 投影不能复用已合并控制协议使用的 runtime v4，必须选择未占用版本，迁移既有 v1–v4 和本批已落地的绑定、Operation、会话/checkpoint、决策应用证据。PR3 文档与实现不为此改动。
