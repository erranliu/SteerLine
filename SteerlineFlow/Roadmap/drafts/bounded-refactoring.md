# 重构审计与范围收敛记录

2026-10-09 用户要求 rebase 至最新代码，不分配或派发任务编号，将后端组装与最终回复组件两项收紧后合并为一个 Spec/Plan。选定本文件保留决策摘要；本次采用普通开发方式，不作为 roadmap 草稿任务队列，不修改 execution-map。

当前唯一方案：[Spec](../../spec/bounded-structure-refactoring.spec.md) · [Plan](../../plans/bounded-structure-refactoring.plan.md) · [专属验收](../../testing/acceptance/bounded-structure-refactoring.md)。2026-10-10 基线更新为 `21d9735057f990f9fb83a678c5293eca8c8c520c`，一个实现交付范围，没有新增 Task ID。

审计结论：后端组装用同包私有函数收拢，不加 builder/resolver/registry；最终回复只收拢状态、目标失效和查询受理，不新增通用 UI 框架或字段转发层；原生命周期副作用执行器提取暂缓，已有无状态调用边界不足以支持再加异步执行层的收益判断。

原预留 #88–#90 只保留永久身份用于追溯、不复用、不派发。旧三项独立 Spec/Plan/验收草稿撤下，合并方案不继承这些实施编号；没有既有实现/验收证据需要转移。工作区 rebase 前的规划备份保留在 Git stash，可恢复旧讨论材料，不把旧草稿当作当前执行依据。

合并方案自行收口架构文档与验收，取消跨任务文档代管。#84 已随 PR #262 合入实际主干，正式清单待结算文字由原交付流程维护；#45 在途工作不由本次重构增加前置。实际实施/合入时按最新主干核对共同文件。此次只更新规划，不实施或提交 PR。

2026-10-10 再审：主干切断旧本机 Orca terminal，supervised 共用一个实现支持 local/environment；后端整理仍有归属收益，但只划分 local Codex/supervised 两个单元，不按目标加层。final 状态提取仍值得实施，新增报告阶段查询过滤、步骤展示和 Ctrl+N 的保持约束；真实 app 回归采用当前三轮 controlled 入口。未新增编号、执行聊天或前置关系。

随后用户授权“实施spec”，当前聊天按普通开发实施两项收敛；实现与验证状态归上述专属验收，不将本记录转成 roadmap 任务、派发队列或其他任务的前置。
