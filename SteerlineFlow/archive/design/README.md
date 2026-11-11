# 设计文档归档

本目录保存已被当前产品方向取代、但仍有追溯价值的设计文档。内容描述其成文时的产品状态，不作为当前实现契约、执行计划或验收通过声明。

| 文档 | 归档原因 | 当前依据 |
| --- | --- | --- |
| [执行控制台设计与操作](execution-console.md) | SteerLine TUI 于 2026-10-15 退役；保留旧键位、布局和操作背景 | [TUI 退役验收](../../testing/acceptance/tui-retirement.md) · [Web 前端验收](../../testing/acceptance/web-frontend.md) |
| [规划界面原型参考](planning-ui.md) | 早期 Web 规划工作台及产品流程设想未成为当前界面契约 | [Spec 索引](../../spec/README.md) · [Web 前端验收](../../testing/acceptance/web-frontend.md) |
| [单文件规划界面原型](prototypes/planning-ui/README.md) | HTML 原样迁入，演示数据、流程文案及内嵌文档均为历史快照 | [Web 工程](../../../web/README.md) · [Roadmap 当前设计](../../../docs/product-design/roadmap-promotion.md) |
| [迁出前 Orca 单终端 worker](orca-native-worker.md) | 2026-10-14 的本仓 adapter/阶段实现已迁至 Runner，本机 Orca 路径已放弃 | [Runner 接入](../../../docs/integration/steerrunner.md) · [Runner 调用生命周期](../../../docs/product-design/runner-lifecycle.md) |
| [可靠性交付简化分析](reliability-simplification-analysis.md) | 保留原审计方案、未实施设计和 SIM 验证约束，历史接管前提不适用于当前 v8 | [可靠性交付链路](../../../docs/architecture/reliable-delivery-chain.md) · [实施验收](../../testing/acceptance/reliability-simplification.md) |
| [Runner 与可靠性历史证据入口](runner-reliability-history.md) | 历次精简和[首轮三组合并](runner-reliability-history.md#首轮文档合并)的旧路径、契约、验收、Runner 来源及固定 Git 原文 | [Runner 调用生命周期](../../../docs/product-design/runner-lifecycle.md) · [执行观察](../../../docs/product-design/execution-observability.md) · [执行配置](../../../docs/product-design/execution-configuration.md) · [项目接入](../../../docs/product-design/project-onboarding.md) |
| [架构与 Web 历史设计入口](architecture-web-history.md) | 汇集历轮解耦、旧任务交接、Web 首期选型和未实施建议，不作为当前任务安排 | [架构总览](../../../docs/architecture.md) · [代码架构](../../../docs/architecture/current-code-architecture.md) · [Web 工程](../../../web/README.md) |
| [第六批精简与提示词审阅](docs-sixth-batch-review.md) | 三份文档原文入口、复制提示词实际差异、字节/SHA 及独立审核；不改变原验收结论 | [准备提示词](../../../docs/product-design/project-onboarding-prompt.md) · [Activity](../../../docs/product-design/execution-observability.md#流程与状态活动) · [Roadmap](../../../docs/product-design/roadmap-promotion.md) |

## 第三批整理来源

2026-11-06 精简项目接入材料与原型说明；原型正文只迁移，不继续承载产品规则。整理前原文在本仓提交 `5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7` 中按下表原路径读取：`git show <提交>:<原路径>`。

| 原路径 | 当前入口 |
| --- | --- |
| `docs/product-design/project-management.md` | [当前切换与拟议添加流程](../../../docs/product-design/project-onboarding.md) |
| `docs/product-design/project-structure.md` | [拟议结构及 PS 规则](../../../docs/product-design/project-onboarding.md#拟议结构与核验) |
| `docs/product-design/project-onboarding-prompt.md` | [原复制正文与精简说明](../../../docs/product-design/project-onboarding-prompt.md) |
| `docs/product-design/prototypes/planning-ui/README.md` | [简洁演示入口](prototypes/planning-ui/README.md)；完整旧演示路线按上述 Git 原文追溯 |
| `docs/product-design/prototypes/planning-ui/index.html` | [原字节 HTML](prototypes/planning-ui/index.html) |

当前项目添加仍为未实现方案，范围与历史调查见[Spec](../../spec/project-onboarding.spec.md)、[Plan](../../plans/project-onboarding.plan.md)和[验收安排](../../testing/acceptance/project-onboarding.md)；当前切换、宿主与材料维护分别按[切换验收](../../testing/acceptance/web-project-switch.md)、[统一启动验收](../../testing/acceptance/unified-startup.md)和[维护验收](../../testing/acceptance/project-maintenance-simplification.md)判断，不把文档整理记为产品验收通过。

第三批整理时提示词正文差异为零：当时复制正文保持 7,542 字节（含原 CRLF），SHA-256 为 `ef8eb1db4182556b82e4707fb1a8cc6639e74c591f409fbdd7485da6eaa13b12`；外围说明收缩后文件由 9,911 变为 9,107 字节。业务原因是让流程、结构规则和复制模板分别维护，保留模板独立使用所需正文。HTML 迁移前后均为 402,865 字节，SHA-256 为 `f07eb24d47ab9c543c40b4867631ecd2660858d821f93a8757dc1ce3799d59fe`。字节核对、导航检查及独立审阅只验证该次文档变更，不扩大运行能力；后续提示词变更见[第六批记录](docs-sixth-batch-review.md)。
