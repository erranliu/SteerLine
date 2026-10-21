# 文档归档

最近整理：2026-10-21；此前归档自 2026-10-06 起。已完成规划退出活动 roadmap，已执行 Spec 与 Plan 成对保存；退役或被替代的资料单独标明，避免把未验收历史范围当作当前待办或支持声明。

| 内容 | 入口 | 使用边界 |
| --- | --- | --- |
| 已交付规格、计划及阶段约束 | [completed](spec/completed/README.md) | 追溯原交付；当前实现以源码与架构索引为准 |
| 退役执行流程和远端试验 | [retired](spec/retired/README.md) | 未完成历史门槛保留，不表示一期支持 |
| 已被替代的产品设计 | [design](design/README.md) | 保留 TUI、早期规划界面及迁出前 Orca 实现的独有历史；当前行为以现行 Spec 与验收为准 |
| 已完成三线重构 roadmap | [三线规划](roadmap/three-line-modularization.md) | #73–#82 全部合入；不再是执行入口 |
| 验收历史结果与旧方案 | [验收归档](../acceptance/archive/README.md) | 已结算与待重定分开，失败/限制及证据字节保持 |
| 历史配置 | [远程 Orca 配置](guides/orca-remote-configuration.md) | 原连接/协议记录，不表示当前支持或权限验收通过 |
| 历史规划推进边界 | [TUI/HTTP 阶段](roadmap/phase2-delivery.md) | #91 当时的交付范围，已被 TUI 退役与 Web 前端交付取代 |
| 历史操作指南 | [远端 MVP](guides/remote-execution.md) · [旧步骤契约](guides/skill-contract.md) · [Beads 项目接入](guides/merge-h-308-integration.md) · [旧发布步骤](guides/steps/steerline-publish-pr.md) | 只供历史核对 |

当前验收摘要及历史证据取证入口见 [验收入口](../acceptance/README.md)，任务身份仍在 [task-ids/](../roadmap/task-ids)。导航链接已适配新位置；历史命令、Canonical Spec 路径、SHA 和 JSON 回执不批量重写。固定契约仍读取其原 commit 的原路径。

[当前文档](../README.md) · [剩余规划](../roadmap/drafts/roadmap.md) · [当前规格](../spec/README.md)

## 2026-10-21 整理范围

基于本次工作树起点 `45d2e8d9` 的代码与专属验收，归档 33 份资料：21 份 Spec/Plan、11 份验收记录/结果索引及 1 份迁出前 Orca 架构说明。入口见[已交付规格](spec/completed/README.md#2026-10-21-补充归档)、[退役规格](spec/retired/README.md#2026-10-21-orca-历史方案归档)及[验收历史](../acceptance/archive/README.md)。当前仍有效的计划和待验记录保留。

本次只整理文档和导航，修正共享 Runner、兼容接管、格式来源与索引状态；不改变任务范围、任务身份、冻结契约、验收结果或外部状态。移动的 JSON 结果索引保持原字节；历史命令、SHA 和代码块不批量改写。

本次静态校验：`scripts/check_doc_links.py` 检查 520 份 Markdown、3377 个本地导航，0 错误；`roadmap_task_id.py check` 检查 97 个身份文件、3 个草稿行、3 个正式行通过；`git diff --check` 通过。逐份对比 33 份移动资料，除归档说明和导航外正文、命令代码块保持；JSON 与原工作树字节一致。表中七个实施/合并提交均可从核对基线追溯。纯文档整理未启动产品、模型或全量测试。
