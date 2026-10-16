# 文档入口

SteerLine 的整体职责、具体产品设计、后续规划、执行契约与验证证据分别维护。当前执行实现包括本地 Codex 和远端 Orca environment；本次只验本地 Codex，本机 Orca 路径已放弃；规划与原生 UI 的拟议设计不表示已经交付。

| 阅读目的 | 入口 |
| --- | --- |
| 供开发项目复用的工程规范 | [SteerlineProjectGuideDoc](../SteerlineProjectGuideDoc/README.md) · [测试架构规范](../SteerlineProjectGuideDoc/testing-architecture.md) |
| 系统整体职责与业务权威 | [架构总览](architecture.md) |
| 独立 Runner、固定 EXE 与 CLI 接入 | [Runner 接入](integration/steerrunner.md) |
| 当前代码、模块与接口归属 | [代码架构](architecture/current-code-architecture.md) |
| 规划任务前判断并行与冲突 | [并行开发指南](architecture/parallel-development.md) |
| 配置、数据、反馈、会话与 UI 设计 | [产品设计](product-design/README.md) |
| Worker 全流程异常、重试与恢复规则 | [Worker 流程异常处理与重试](product-design/worker-lifecycle.md) |
| 讨论目标与执行任务 | [RoadmapGuide](roadmap/RoadmapGuide.md) · [正式执行规划](roadmap/execution-map.md) · [产品愿景](roadmap/product-vision.md) |
| 未交付规格与待补验收 | [Spec / Plan](spec/README.md) |
| 全项目测试方法与入口 | [测试指南](testing.md) · [分级 E2E](../tests/e2e/README.md) |
| 历史规格、流程与配置 | [归档](archive/README.md) |
| 自动/人工验收流程、已验与待验 | [acceptance/](acceptance/README.md) |

## 目录边界

- 仓库根目录的 SteerlineProjectGuideDoc/ 维护供开发项目复用的通用工程规范，SteerLine 自身直接引用；docs/ 维护本产品的设计、实现和结果，不独立复制通用规则。
- docs 根目录只放对整个项目长期有效的整体文档：本入口、架构总览和测试指南。
- product-design/ 放长期维护的具体产品设计；按功能命名，明确当前契约、拟议方案和验证限制，不按阶段编号积累目录。
- architecture/ 放当前代码架构、源码索引与持续维护的并行开发指南；指南链接契约和测试，缺少独立说明时记录当前边界，不重复维护产品规则。
- roadmap/ 以 RoadmapGuide 为规划必读入口，drafts/ 保存可扩展的讨论草稿，execution-map.md列已确认执行规划并映射中心DAG状态；task-ids/ 保留所有已分配身份，号码不复用。指南不逐份索引草稿。
- spec/ 放独立任务 Spec/Plan 和共同执行约束，定义验收要求并链接 acceptance 明细。产品设计不替代任务固定契约。
- acceptance/ 统一维护自动/人工验收行为，根目录保留当前计划与未闭合项，archive/区分历史结果和旧方案，execution-map 同步执行与验收汇总并链接待验文档；原始产物放临时目录，历史证据经固定 Git 快照取证。validation/ 只保留跳转入口，不维护第二套验收状态。
- archive/ 保存有独有追溯价值的已交付或被替代资料。过期说明先吸收仍有效内容，再删除；没有独有价值的重复文档不另行归档。

新设计进入对应子目录并更新索引；整体文档只保留摘要和链接。规则有唯一维护位置，持久格式以[源码定位表](product-design/data-model.md#当前格式来源)为准。移动时修复当前导航，保留历史 Canonical Spec 路径和固定契约；不让导航调整改写中心 Task 或外部资源状态。

文档调整用 scripts/check_doc_links.py、roadmap_task_id.py check 和 git diff --check 验证；纯导航、文字及源码注释修改不启动产品或模型测试。
