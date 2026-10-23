# 文档入口

SteerLine 的整体职责、具体产品设计、后续规划、执行契约与验证证据分别维护。当前通过独立 SteerRunner 接入本地 Codex 和远端 Orca environment，本机 Orca 路径已放弃。实际验证按专属验收中的固定构建与范围判断；规划与原型的拟议设计不表示已经交付。

| 阅读目的 | 入口 |
| --- | --- |
| SteerLine 与受管理项目的关系、工作方式 | [认识 SteerLine](../SteerlineProjectGuideDoc/README.md) |
| 本项目的规划、任务与验证流程 | [SteerlineFlow](../SteerlineFlow/README.md) |
| 系统整体职责与业务权威 | [架构总览](architecture.md) |
| 独立 Runner、固定 EXE 与 CLI 接入 | [Runner 接入](integration/steerrunner.md) |
| 本机安装、隔离调试包与真实 PR 调试 | [安装与独立调试](runtime-install-debug.md) |
| 当前代码、模块与接口归属 | [代码架构](architecture/current-code-architecture.md) |
| 规划任务前判断并行与冲突 | [并行开发指南](architecture/parallel-development.md) |
| 配置、数据、反馈、会话与 UI 设计 | [产品设计](product-design/README.md) |
| Worker 全流程异常、重试与恢复规则 | [Worker 流程异常处理与重试](product-design/worker-lifecycle.md) |
| 讨论目标与执行任务 | [RoadmapGuide](../SteerlineFlow/Roadmap/RoadmapGuide.md) · [正式执行规划](../SteerlineFlow/Roadmap/execution-map.md) · [产品愿景](../SteerlineFlow/Roadmap/product-vision.md) |
| 未交付规格与待补验收 | [Spec / Plan](../SteerlineFlow/spec/README.md) |
| 全项目测试方法与入口 | [测试指南](../SteerlineFlow/testing.md) · [分级 E2E](../tests/e2e/README.md) |
| 历史规格、流程与配置 | [归档](../SteerlineFlow/archive/README.md) |
| 自动/人工验收流程、已验与待验 | [acceptance/](../SteerlineFlow/testing/acceptance/README.md) |

## 目录边界

- 仓库根目录的 `SteerlineProjectGuideDoc/` 维护 SteerLine 提供的固定通用规范；`SteerlineFlow/` 维护本项目的开发流程材料；`docs/` 维护产品设计、架构和接入说明。
- docs 根目录保留本入口和架构总览，测试总入口位于 `SteerlineFlow/testing.md`。
- product-design/ 放长期维护的具体产品设计；按功能命名，明确当前契约、拟议方案和验证限制，不按阶段编号积累目录。
- architecture/ 放当前代码架构、源码索引与持续维护的并行开发指南；指南链接契约和测试，缺少独立说明时记录当前边界，不重复维护产品规则。
- roadmap、Spec、Plan、测试清单、验收计划与历史报告、过期流程文档的维护入口统一见 [SteerlineFlow](../SteerlineFlow/README.md)。产品设计通过链接引用这些材料，不替代任务固定契约。

新设计进入对应子目录并更新索引；整体文档只保留摘要和链接。规则有唯一维护位置，持久格式以[源码定位表](product-design/data-model.md#当前格式来源)为准。移动时修复当前导航，保留历史 Canonical Spec 路径和固定契约；不让导航调整改写中心 Task 或外部资源状态。

文档调整用 scripts/check_doc_links.py、roadmap_task_id.py check 和 git diff --check 验证；纯导航、文字及源码注释修改不启动产品或模型测试。
