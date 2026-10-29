# SteerLine 项目开发流程

本目录维护 SteerLine 自身的开发流程材料，按[受管理项目规范](../SteerlineProjectGuideDoc/project-standards.md)组织。SteerLine 的介绍和工作方式见[认识 SteerLine](../SteerlineProjectGuideDoc/README.md)，产品设计与架构见[产品文档](../docs/README.md)。

| 内容 | 入口与职责 |
| --- | --- |
| 流程路径 | [paths.md](paths.md)：流程定义位置、L1–L4 固定报告结构、仓库外测试/并行调试目录、应用与调度状态。 |
| 远端环境 | [environment.md](environment.md)：项目必需工具、TTY及验收入口；[执行规则](environment-execution.md)由同版本SteerLine规范固定，未完成真实验收前保持未就绪。 |
| 任务需求 | [spec/](spec/README.md)：Spec、共同执行约束及任务资料索引。 |
| 实现计划 | [plans/](plans/)：对应 Spec 的实现步骤和最小必要测试计划。 |
| Roadmap | [RoadmapGuide](Roadmap/RoadmapGuide.md) 维护本项目规则；[execution-map](Roadmap/execution-map.md) 维护确认的范围、依赖和执行/验收汇总；草稿与永久任务身份分别在 `Roadmap/drafts/`、`Roadmap/task-ids/`。 |
| 测试入口 | [testing.md](testing.md)：日常执行与选择；[结构化清单](testing/test-list.json) 和 [L1](testing/l1.md)、[L2](testing/l2.md)、[L3](testing/l3.md)、[L4](testing/l4.md) 列表维护实际测试内容。 |
| 验收与历史报告 | [testing/acceptance/](testing/acceptance/README.md) 保存任务专属计划、已验/待验和历史结果；[报告约定](testing/reports.md) 说明证据及登记。原始大体积产物另存，文档保留摘要和定位。 |
| 过期流程文档 | [archive/](archive/README.md)：历史 Spec/Plan、旧规划与被替代设计，不作为当前执行入口。 |

普通开发由当前聊天直接实现；明确要求 roadmap 调度时按 RoadmapGuide 执行。安装和隔离调试说明见 `docs/` 中的[独立调试指南](../docs/runtime-install-debug.md)。迁移目录不改变既有任务、依赖、执行授权或历史验证结论。固定 Git 版本的历史路径仍按原 commit 读取，当前文档导航和程序默认读取使用本目录。
