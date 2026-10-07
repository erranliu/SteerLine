# Plan：执行与待处理原生界面

任务 ID：50；任务名称：执行与待处理原生界面。唯一执行依据：[Spec](phase2-execution-ui.spec.md)。硬前置：#44、#45，一个独立实现 PR 交付。

1. 对齐 #36/#45 的五阶段、原会话/轮次、PR 和反馈 DTO，列出真实能力及不可用原因，复用 #44 的输入/导航组件。
2. 实现任务列表、详情、正文和折叠，接统一反馈输入与原请求持久化；保留服务控制和明确丢弃，不增加专用恢复路由。
3. 注入重复提交、回执丢失、旧轮次、换项目和离线恢复，验证反馈完整且目标固定，状态只消费中心事实。
4. 完成 EX1–EX6 的 Windows 真机证据、受影响测试与构建；向 [二期验收活动](../acceptance/phase2.md) 交接真实页面入口。

验收行为与状态记录于 docs/acceptance/phase2-execution-ui.md，原始证据放本次临时目录；自动与人工分别记录，少量人工项按允许范围集中后置。

## 本次文档修订测试计划

本次只调整验收归属与导航，不实施上述能力。正式准入前按实际源码补齐 EX1–EX6 的最小实现测试命令与环境；不把本次静态结果记为产品验收。

| 级别 / 场景 | 环境 / 命令 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| 文档静态 / doc-links、task-identities、whitespace | Windows、Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次修订交付前；导航存在、身份一致、无差异格式错误 | [规划文档验收](../acceptance/roadmap-planning.md)与临时检查输出 |

未选产品 L1/L2/L3/Full：此次没有实现、测试环境或场景改动。
