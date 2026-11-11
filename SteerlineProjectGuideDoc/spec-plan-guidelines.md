# Spec 与 Plan 编写规范

适用范围：SteerLine 新编写或本次修改的 Spec、实现 Plan。本文是按主题读取规则的入口，普通开发不因此强制建立 Spec/Plan 或纳入 roadmap。

Spec 定义成果、边界与通过条件，Plan 安排实施与验证；具体自动/人工验收行为及实时结果另存任务专属验收文档。

任务测试选择在编写 Plan 时由语义分析确定，随同名 `.tests.json` 一起审阅、提交 Git。JSON 只保存有序唯一测试 ID；Plan 写验收映射、原因、时点和特殊准备，通用命令、环境及断言引用正式清单，避免重复维护。运行时读取并冻结，不另行生成安排。

执行时点、按实际行为选择测试、补测与结果复用统一引用[测试指南](../SteerlineFlow/testing.md#日常执行)及其“按行为选择测试”一节，不将每次小修、提交或更新记录分别写成一轮全量回归。证据范围与停止条件引用同文“环境与证据”，不默认增加逐文件摘要或全部产物的再次独立审计；任务确需额外取证时写明具体风险及范围。已固定的任务要求与安排不因通用规则变更自动删减，历史 Plan 和结果也不追改。

编写时按内容读取并遵循：

- 验证需求、Plan 的测试计划及执行入口：[测试指南](../SteerlineFlow/testing.md)。
- 验收规划、专属文档及结果登记：[验收流程](../SteerlineFlow/testing/acceptance/README.md)。
- roadmap 任务的草稿、身份与执行安排：[RoadmapGuide](../SteerlineFlow/Roadmap/RoadmapGuide.md) 及其必需文档。
- Runner 调用流程、异常、重试或恢复设计：[Runner 调用生命周期](../docs/product-design/runner-lifecycle.md)，链接统一规则并说明本次差异。
