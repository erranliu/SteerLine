先读取本次固定文件材料 roadmap-inputs/source.md 中的任务卡和 roadmap-inputs/confirmed-planning.json，以及固定提交 {{.Commit}} 的 SteerlineFlow/Roadmap/document-rules.md、SteerlineFlow/spec/SpecGuide.md、SteerlineFlow/environment-project.json 及其声明的项目材料、SteerlineProjectGuideDoc/spec-plan-guidelines.md。文件材料使用 Runner 给出的 file 路径。调查同一代码快照，只为目标内部代号 {{.Key}} 生成完整 Spec/Plan、测试选择和验收材料。只返回原 schema 结果。

以任务卡的 Markdown 快照作为原文，以已确认的单任务与正式前置为准。沿用 SpecGuide 的文档生成规范。本次文档 PR 仅包含 Spec、Plan、测试选择和验收材料。
