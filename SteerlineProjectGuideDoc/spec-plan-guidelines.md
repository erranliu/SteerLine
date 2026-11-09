# Spec 与 Plan 编写规范

适用范围：SteerLine 新编写或本次修改的 Spec、实现 Plan。本文是按主题读取规则的入口，普通开发不因此强制建立 Spec/Plan 或纳入 roadmap。

Spec 定义成果、边界与通过条件，Plan 安排实施与验证；具体自动/人工验收行为及实时结果另存任务专属验收文档。

任务测试选择在编写 Plan 时由语义分析确定，随同名 `.tests.json` 一起审阅、提交 Git。JSON 只保存有序唯一测试 ID；Plan 写验收映射、原因、时点和特殊准备，通用命令、环境及断言引用正式清单，避免重复维护。运行时读取并冻结，不另行生成安排。

编写时按内容读取并遵循：

- 验证需求、Plan 的测试计划及执行入口：[测试指南](../SteerlineFlow/testing.md)。
- 验收规划、专属文档及结果登记：[验收流程](../SteerlineFlow/testing/acceptance/README.md)。
- roadmap 任务的草稿、身份与执行安排：[RoadmapGuide](../SteerlineFlow/Roadmap/RoadmapGuide.md) 及其必需文档。
- worker 流程、异常、重试或恢复设计：[worker 生命周期](../docs/product-design/worker-lifecycle.md)，链接统一规则并说明本次差异。
