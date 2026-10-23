# 添加项目：Codex 准备提示词（方案）

用途：在[添加项目流程](project-management.md)中，由 SteerLine 填入用户已确认的仓库信息后展示，用户复制到目标仓库的 Codex 会话执行。结构规则唯一正文见[项目结构与核验](project-structure.md)。本轮只交付模板，不运行它；产品尚无自动生成/核验/纳管入口。

模板版本：`project-prepare/v1`，对应拟议结构格式 `steerline-project/v1`。这不是 worker 开发/报告/发布阶段的提示词，也不触发 SteerLine 内部 agent 或 Task。

## 生成与使用

[统一维护方案](project-maintenance.md)提供共用材料规则和既有项目基线/局部维护；完整结构核验仍由添加批次补齐，不自动执行本模板。准备、生成、文件应用、Git 发布及正式纳管各自保留明确操作边界。模板输出文件的实际远端版本由服务重新核验，不能从 Codex 报告补造就绪。

输入来自用户填写的服务机器本地仓库位置、显示名、remote、目标分支、已知发布目标，以及已有长期文档位置。未填写的值明确标“待确认”，不推测 origin/main、平台身份、认证或测试命令。路径属于服务机器；用户应在能访问同一仓库的 Codex 会话执行。复制后用户可以先审阅和补充输入。

产品实现时生成完整可独立使用的提示词，不能只给另一台机器不可访问的产品源码链接。下面模板内已包含最小布局、清单和内容要求；详细规则版本随提示词记录。仓库内容由 Codex 自己调查，不能把显示名等自由文本当作可执行命令拼入 shell。

## 可复制模板

````text
请为下面的 Git 项目准备 SteerLine 接入所需的文档结构。本任务只做仓库准备，不在 SteerLine 中添加项目、建库、建任务或启动执行。

输入（未确定者必须保留“待确认”，先问清关键缺口）：
- 仓库绝对目录：{{repository_path}}
- 项目显示名：{{display_name}}
- Git remote 名：{{remote_name}}
- SteerLine 目标分支：{{target_branch}}
- 发布目标（平台/远端仓库，非认证信息）：{{publication_target}}
- 已知文档位置或特别约束：{{existing_document_notes}}
- 准备规则：steerline-project/v1；提示词：project-prepare/v1

先调查实际仓库：
1. 读取现有 AGENTS.md 和相关项目规则，确认 Git 工作树顶层、common-dir、当前分支、选定目标分支及 remote；查看工作区修改，区分我的现有工作和你本次新增内容。
2. 读取 README、现有架构、测试说明和文档目录，依据实际代码整理事实，不凭项目名称猜测技术栈、模块或测试命令。若目录不是已初始化且已有 commit 的 Git 工作树、关键目标不明确、或现有规则存在冲突，先说明并请求必要输入。
3. 若存在 .beads、.beads.db、beads.db 或其他已知任务状态冲突，只报告位置和原因；不要删除、迁移或伪装成空白项目。

准备最小结构，优先采用已有正文：
- 根 README.md：用途、入口、实际构建/启动方式；未实现就如实写明。
- 根 AGENTS.md：项目修改边界、最小检查、协作规则和提交/发布授权边界。
- 四类长期文档：架构事实、并行开发约束、测试方法、Spec/Plan 编写规范。
- SteerlineFlow/spec/README.md：任务 Spec/Plan 的目录及职责。
- SteerlineFlow/testing/acceptance/README.md：自动/人工验收、待验/失败/通过及证据记录约定。
- 一个长期设计目录及其中 README.md：优先采用已有设计目录，否则使用 docs/design。
- .steerline/project.json：明确映射四类文档及设计目录。

默认清单内容如下；若已有文档位于其他位置，修改映射并采用原文件，不复制另一套长期正文：
{
  "format": "steerline-project/v1",
  "documents": {
    "architecture": "docs/architecture.md",
    "parallel": "docs/architecture/parallel-development.md",
    "testing": "SteerlineFlow/testing.md",
    "spec_guide": "SteerlineFlow/spec/SpecGuide.md"
  },
  "design_root": "docs/design"
}

四个 documents 角色必须全部存在，分别指向不同的仓库相对 .md 普通文件。使用 / 分隔，不能含绝对路径、盘符、反斜杠或 .. 穿越；不使用 symlink、submodule 或目录代替文件。清单只接受上面这些字段，不加入命令、运行目录、认证或“已通过核验”等内容。根 README/AGENTS 和 SteerlineFlow/spec、SteerlineFlow/testing/acceptance 的 README 是固定入口。设计目录用 README 保存到 Git，不使用只有空目录或 .gitkeep 的假结构。

正文按实际情况补齐：

与后续统一维护首版衔接时，testing 角色固定为 SteerlineFlow/testing.md，不另建同内容测试规范。维护测试管理所需的 SteerlineFlow/testing/test-list.json；有实际 Plan 时在测试计划里说明最小必要入口，SteerLine在远端Plan更新后自动生成系统测试安排；不要求创建、应用或提交旁置.tests.json。没有测试时如实记录、清单可为空；没有实际任务时不创建示范 Plan/安排。服务采用配置远端目标分支的已发布 commit，本地领先提交或未合入准备 PR 仍须报告待发布。

- 架构：模块及职责、主要调用/数据路径、状态权威、当前能力和缺口。
- 并行开发：独立交付边界、共享文件/接口如何协调、依赖与合入条件、工作区和数据隔离。
- 测试：已有最小命令、环境、通过条件及证据位置。没有可用测试时明确写“当前尚无”，不得虚构通过结果；不为接入临时跑全量或真实模型/真实 PR 验收。
- Spec/Plan 规范：先调查代码和整合已有实现；Spec 写目标、范围、契约与验收要求，Plan 写实施及最小必要测试安排；自动/人工项区分，结果进入 SteerlineFlow/testing/acceptance；普通开发不强制 roadmap，也不默认新增兼容/迁移。
- 目录 README：解释用途、已有文档入口及维护规则；没有专题设计时如实说明。

每份必需 Markdown 都要合法 UTF-8、非空，有一级标题和实质正文；单份不超过 1 MiB，总和不超过 8 MiB。清单不超过 64 KiB。核对必需文档里的相对 Markdown 链接能在同一 Git 版本解析到实际文件/目录且不越出仓库；正文链接允许正常的 ../ 相对引用，不把代码示例里的路径当成实际文件要求。不要用空标题、伪造实现事实或复制 SteerLine 自身的 Go/worker/roadmap 规则填满文档。

权限与完成边界：
- 本提示词只授权调查以及最小必要的文档/清单修改，不改业务代码、仓库地址、Git remote、账户或权限，不安装工具，不生成数据库、任务或运行状态。
- 不修改 STEERLINE_HOME、C:\SteerlineRuntime 或任何已运行的 SteerLine 状态；不调用任务创建、派发、服务启动或纳管命令。此准备清单不是现有 SteerLine CLI 的已实现配置格式。
- 不创建示范 Spec/Plan、Task ID、execution-map 或“验收已通过”记录来凑结构；不自动引入 roadmap 或多 agent 流程。
- 保留我的现有修改，禁止 reset/clean/清理旧状态；不删除或覆盖已有规范。必须改变现有约束才能继续时，先指出矛盾。
- 可以按仓库规则把本次文档变更提交为独立本地 commit，只暂存本次文件。若仓库规则要求先审阅或走 PR，则遵守规则并报告尚未合入；不自动 push、创建/合并 PR、直接更新受保护分支或改写历史。
- SteerLine 后续核验的是选定目标分支的已提交版本。只有工作区文件、准备分支 commit 或未合入 PR，不代表目标分支已满足要求；不能为通过核验偷偷切换目标分支。

最后报告：
1. 实际仓库顶层/common-dir、remote、目标分支及能解析到的完整 commit。
2. 清单内容、文档角色对应路径，本次新增/修改文件和采用原文件的理由。
3. 已运行的最小检查及实际结果；未执行的检查明确列出。
4. 提交情况；文档是否已进入目标分支，仍需审阅/提交/推送/合入的事项。
5. 未解决的事实、冲突和人工事项。只能说“仓库准备完成/部分完成”，不能宣称 SteerLine 已核验、已添加、已建库或已可执行。

完成后停止，用户回到 SteerLine 点击“核验项目”。最终准入由 SteerLine Go 服务根据实际文件、Git 版本及纳管参数决定，不采信本报告作为通过凭证。
````

## 返回 SteerLine 的处理

Codex 报告仅帮助用户发现尚缺输入或合入步骤。Web 不解析自然语言报告替代检查，也不让用户粘贴一份 JSON passed 结果直接放行。用户完成准备后，应用服务按结构规则重新读取仓库；失败项由服务返回，提示词可带上这些具体问题让用户继续在 Codex 修订。

执行方案、全局认证和真实 worker 可运行性属于后续就绪检查，不放进这份仓库准备提示词。项目纳管、任务创建、Roadmap 转正和 PR 发布是不同动作；本模板没有启动这些动作的授权。
