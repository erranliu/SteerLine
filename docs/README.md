# 文档入口

SteerLine 的整体职责、具体产品设计、后续规划、执行契约与验证证据分别维护。当前执行与验收范围是本地 Codex 和本机 Orca；规划与原生 UI 的拟议设计不表示已经交付。

| 阅读目的 | 入口 |
| --- | --- |
| 系统整体职责与业务权威 | [架构总览](architecture.md) |
| 当前代码、模块与接口归属 | [代码架构](architecture/current-code-architecture.md) |
| 规划任务前判断并行与冲突 | [并行开发指南](architecture/parallel-development.md) |
| 配置、数据、反馈、会话与 UI 设计 | [产品设计](product-design/README.md) |
| 讨论目标与执行任务 | [RoadmapGuide](roadmap/RoadmapGuide.md) · [已入 DAG 正式清单](roadmap/execution-map.md) · [产品愿景](roadmap/product-vision.md) |
| 未交付规格与待补验收 | [Spec / Plan](spec/README.md) |
| 全项目测试方法与入口 | [测试指南](testing.md) · [分级 E2E](../tests/e2e/README.md) |
| 历史规格、流程与配置 | [归档](archive/README.md) |
| 验收摘要、基线与待补项 | [validation/](validation/README.md) |

## 目录边界

- docs 根目录只放对整个项目长期有效的整体文档：本入口、架构总览和测试指南。
- product-design/ 放长期维护的具体产品设计；按功能命名，明确当前契约、拟议方案和验证限制，不按阶段编号积累目录。
- architecture/ 放当前代码架构、源码索引与持续维护的并行开发指南；指南链接契约和测试，缺少独立说明时记录当前边界，不重复维护产品规则。
- roadmap/ 以 RoadmapGuide 为规划必读入口，drafts/ 保存可扩展的讨论草稿，execution-map.md 映射中心 DAG 的正式任务；task-ids/ 保留所有已分配身份，号码不复用。指南不逐份索引草稿。
- spec/ 放独立任务 Spec/Plan、未闭合的定向验收和共同执行约束。产品设计不替代任务固定契约。
- validation/ 只维护最小验收摘要、最近基线和未闭合事项；原始运行产物写系统临时目录，历史证据通过固定 Git 快照取证。
- archive/ 保存有独有追溯价值的已交付或被替代资料。过期说明先吸收仍有效内容，再删除；没有独有价值的重复文档不另行归档。

新设计进入对应子目录并更新索引；整体文档只保留摘要和链接。规则有唯一维护位置，持久格式以[源码定位表](product-design/data-model.md#当前格式来源)为准。移动时修复当前导航，保留历史 Canonical Spec 路径和固定契约；不让导航调整改写中心 Task 或外部资源状态。

文档调整用 scripts/check_doc_links.py、roadmap_task_id.py check 和 git diff --check 验证；纯导航、文字及源码注释修改不启动产品或模型测试。
