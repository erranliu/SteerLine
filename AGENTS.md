# 项目默认规则

SteerLine 尚未发布，以快速开发、跑通流程和简洁设计为默认目标。
除非用户或当前 Spec 明确要求，不兼容先前开发版本的数据、数据库、配置或自身协议，不新增旧版读取、迁移、双写或回退路径。

普通开发由当前聊天直接实现，仅用户明确要求按 roadmap 调度时使用 roadmap-runner。

固定模型指令只在 `internal/prompts/*.md` 维护，通过同包组装函数使用；业务代码只提供原正文与事实，不额外拼接行为规则。提示词、接线、资产或对应测试/CI 变更须给出实际正文差异、字节数前后值、业务原因及验证并经独立审核，更新黄金样本不能代替批准。维护及远端保护边界见 [提示词维护](docs/prompt-governance.md)。

开始相关工作前，按需读取并遵循：

- 流程目录、测试输出或并行调试路径：[路径清单](SteerlineFlow/paths.md)。
- 编写或修改 Spec / Plan：[编写规范](SteerlineProjectGuideDoc/spec-plan-guidelines.md)。
- 测试、验收或临时/AI/真实 PR 调试：[测试指南](SteerlineFlow/testing.md)。
- 规划、修改或执行 roadmap：[RoadmapGuide](SteerlineFlow/Roadmap/RoadmapGuide.md) 及其必需文档。
