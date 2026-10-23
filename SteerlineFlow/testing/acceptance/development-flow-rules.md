# 验收：固定开发流程规则独立

唯一归属：任务 #94，固定开发流程规则独立。2026-10-14 草稿；仅规划，待验收，未纳入执行。

[Spec](../../spec/development-flow-rules.spec.md) · [Plan / 测试计划](../../plans/development-flow-rules.plan.md) · [草稿](../../Roadmap/drafts/roadmap.md)

2026-10-14仍留草稿、暂不安排验收；Workflow业务步骤/解析/提示词所有权与正在实施的Worker三层重构直接重叠；拟议development包不能与其workflow包作为两套owner同时实施。原自动项均未执行，未来须随重定范围更新，不将三层重构结果直接登记为本任务通过。见[冲突核对](../../Roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)。

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境/时点 | 状态 |
| --- | --- | --- | --- |
| DF94-A1 | 纯规则无需Service，输入值不可变、解析/构造只有一处；拒绝歧义且接受额外解释字段/完整围栏/CRLF/既有trim；all-tag/直接及传递闭包无I/O或业务能力 | 实施新增development包TestDevelopmentRules和TestDevelopmentBoundary；普通Go/Windows与Linux依赖结构，提取后/PR前 | 仅规划，待验收 |
| DF94-A2 | 枚举正常步骤、report重试上限、一次publish.report、源失败/问题/blocked/unknown、重复/冲突/错源及反馈/恢复继续资格；解析拒绝与接受正反例均保持 | 纯规则+原TestWorkerStage*隔离actor fixture，明确含TestWorkerStageStrictResult及TestWorkerStagePresentationDoesNotBlockResult；接线后/PR前，完整命令见Plan | 仅规划，待验收 |
| DF94-A3 | 注入提交失败后结果/步骤/下一输入均回滚；未知发布的原PR真实merged可交付，错误/替代PR拒绝；若基线已含#96，清理失败仍保持原交付 | 原原子失败入口+实施新增TestDevelopmentUnknownPublicationMergedDelivery始终运行；基线已含#96时加TestWorkerRuntimeDeliveredCleanup组合；SQLite/provider fixture，PR前最终候选 | 仅规划，待验收 |
| DF94-A4 | Codex、Orca本机及environment各三阶段；同原会话/Attempt，次数、输入/最终正文及步骤保持 | 既有三个tests/integration controlled入口；各独立fixture，无token，PR前/最终接线候选 | 仅规划，待验收 |

新增包/测试在Plan明确补齐，不是现有能力。全部命令、组/环境、时点和证据映射见Plan；真实测试数>0、所需子场景全部执行、无skip，错误/零匹配/清理失败不能通过。Linux只证明依赖结构，controlled不代替原生支持验收。

组合记录实际候选基线commit和已包含的#95/#96成果；本项未知合并/三个stage入口始终验。#96清理组合未触发时明确记基线不含该成果，不记通过、不等待兄弟；触发后入口缺失/零匹配失败。证据只登记本项验收，不替代兄弟验收。

## 人工与执行记录

无新增人工/native项。原#84/#91人工与原生缺口保持各自记录，不因本项结构验证改称已验。

尚未执行。按ID登记Spec/Plan固定commit、实际源码/dirty/构建、精确命令和执行数量、summary/原始日志绝对临时目录、结构diff、失败及修复结果。固定L1独立记回归。正式纳入后同步execution-map；本页只归属#94，#95不得以本页代替自己的用例验收。
