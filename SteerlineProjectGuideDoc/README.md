# SteerLine 项目接入指南

在要接入的 Git 仓库中按下列步骤完成项目端初始化。准备结果是目标分支上一份可追溯的提交，其中包含固定指南、本项目的流程文档、任务测试入口和环境检测入口。

## 1. 确认项目和接入版本

- 确认项目 Git 根目录、当前提交、SteerLine 服务机器上的仓库路径、实际 remote、目标分支、PR 发布目标和拟用执行方案。空白仓库先按项目正常流程形成初始提交。
- 读取项目现有的 README、`AGENTS.md`、依赖和构建配置、启动方式、测试脚本及 CI。用实际命令和路径准备下列材料，保留已有修改，记录尚未实现或无法核实的能力。
- 选定与部署匹配的 SteerLine 源码版本，记录来源及完整提交。核实 SteerLine 与独立 Runner 支持测试入口 version 1、环境 manifest version 6 和结果信封 version 3。

## 2. 复制指南，建立项目流程目录

从选定版本完整复制 `SteerlineProjectGuideDoc/` 到项目 Git 根目录。在同一级创建 `SteerlineFlow/`。固定指南随 SteerLine 版本维护，项目的实际命令、路径和约定写入自己的流程文档。

```text
<项目 Git 根目录>/
├── SteerlineProjectGuideDoc/              # 完整复制固定指南
├── SteerlineFlow/
│   ├── paths.md                         # 项目流程及产物路径
│   ├── environment-project.json         # 固定的任务测试入口
│   ├── testing.md                       # 项目测试指南，默认路径
│   ├── testing/
│   │   ├── test-list.json               # 唯一正式测试清单，默认路径
│   │   └── runtime-progress.md          # 复制同版本的固定协议
│   ├── environment.md                   # 项目环境要求和机器声明
│   └── scripts/
│       └── verify-environment.ps1       # 项目实现，可选 .sh
└── tools/testing/RuntimeProgress.ps1    # 复制同版本的正式写入器
```

分别从同一 SteerLine 源码版本复制 `SteerlineFlow/testing/runtime-progress.md` 和 `tools/testing/RuntimeProgress.ps1`，在项目中保留原路径和正文。其余流程材料按下列步骤以本项目的实际情况编写。

1. 建立 `SteerlineFlow/paths.md`，列出 Git 中的流程文档、机器清单及其读取入口，记录测试报告和原始日志、临时工作区、应用状态的实际位置与所有权。区分默认输出和显式指定的路径，说明每次运行如何取得独立目录、怎样定位证据以及谁负责清理。
2. 在项目根 README 和 `AGENTS.md` 中链接本指南、`SteerlineFlow/paths.md` 及实际测试指南。已有架构、构建和协作说明可直接引用。
3. 在路径清单中登记后续材料的去处。需求用 `SteerlineFlow/spec/`，实现计划用 `SteerlineFlow/plans/`，任务验收用 `SteerlineFlow/testing/acceptance/`，过期流程材料用 `SteerlineFlow/archive/`。有实际任务时才创建对应的 Spec、Plan 和验收文档。有 roadmap 需求时，再建立 `SteerlineFlow/Roadmap/RoadmapGuide.md` 和 `execution-map.md`，使正式规划与确认的任务 DAG 一致。

## 3. 配置任务测试

在固定路径 `SteerlineFlow/environment-project.json` 写入以下内容。

```json
{
  "version": 1,
  "requirements": ["SteerlineFlow/testing.md"],
  "testing_catalogue": "SteerlineFlow/testing/test-list.json"
}
```

已有项目可将 `requirements` 和 `testing_catalogue` 指向现有的任务测试规范和唯一正式清单。`requirements` 必须是数组，可以为空，路径不得重复。两处引用均使用精确匹配大小写的仓库相对路径和 `/`，指向同一提交中的普通文件。JSON 使用 UTF-8。

在实际测试指南中列出各级的可读测试列表、运行方式、环境和前置条件、通过条件、报告位置及已知缺口。可以直接链接已有列表。

| 级别 | 登记内容 |
| --- | --- |
| L1 | 固定的最小代码检查 |
| L2 | 固定的核心流程冒烟测试 |
| L3 | 无顺序依赖、可写资源隔离的功能测试 |
| L4 | 临界条件、需要串行的功能测试及其他可复用测试集合 |

正式清单顶层为 `tests` 数组。每项填写唯一稳定的 `id`、`name`、`level`（L1–L4）、`environment`、`command`、`check`，可附 `note`。只登记实际可执行的入口，保持 ID、命令和通过条件与可读列表一致。尚无测试时提交 `{"tests":[]}`，在测试指南中标明各级缺口。

把进度接到实际登记的测试入口。PowerShell 入口加载正式写入器，在开始或重跑时调用 `Set-RuntimeProgress -Name 'test.<实际ID>' -Completed $false -IfDeclared`，在实际选择非空、断言全部通过、无意外跳过且原报告和必要清理均核验后改写为 `$true`。其他语言入口按复制的协议实现相同语义。进度使用 Runner 注入的 `STEERLINE_PROGRESS_FILE`，原始报告继续由项目测试入口保存。

首个真实任务形成后，用 Spec 描述成果和验收条件，用 `*.plan.md` 写实现步骤与验证安排，在同名 `*.tests.json` 的 `tests` 数组中保存有序且唯一的正式测试 ID。按任务实际情况编写并审阅这些文件。

## 4. 实现项目环境检测

在 `SteerlineFlow/environment.md` 写明项目实际依赖的工具、SDK、权限和服务，就绪判据、失败诊断及清理方式。文档内保留一个 `steerline-environment` 代码块。

```steerline-environment
{
  "version": 6,
  "script": "SteerlineFlow/scripts/verify-environment.ps1",
  "interpreter": "powershell.exe",
  "dependencies": [],
  "requirements": {}
}
```

使用 Bash 时把 `script` 改成 `SteerlineFlow/scripts/verify-environment.sh`，`interpreter` 改成 `bash`。让脚本从机器段读取并执行 `requirements` 中的实际要求。键名以英文字母或数字开头，可包含点、下划线和连字符，如 `node.min_version`；值为非空、单行、非秘密字符串。`dependencies` 列出检测实际使用且会影响就绪结论的配置和辅助文件，使用唯一、大小写准确的仓库相对路径。环境文档和脚本入口已自动计入检测依据。

检测脚本应在环境尚未就绪时也能从固定提交的工作区根目录启动。平台以 `powershell.exe -NoProfile -NonInteractive -File <script>` 或 `bash <script>` 调用，无附加业务参数。脚本按统一标准检查各运行机上的项目依赖，完成诊断和自有清理。项目测试、PR 发布和 Runner 链路检查沿各自入口执行。

脚本从 `STEERLINE_RESULT_FILE` 读取平台预置的 version 3 JSON 信封，原样保留 `version`、`project_identity`、`environment_key`、`activity_id`、`round`、`call_context`、`source_commit` 和 `inputs_digest`。完成检测后原子写回同一文件，设置 `completed: true`、布尔值 `ready`、可读的 `message`，可附 `details`。依赖缺失、判据未确定或清理未完成时写入 `ready: false`，说明原因和下一步。成功完成一次负向检测也可正常退出。凭证由执行配置和目标运行机提供，Git 文件和检测输出只记录非秘密信息。

## 5. 自检并采用到目标分支

- 核对文档入口、JSON 和引用路径。确认测试清单指向真实命令，固定协议与写入器来自同一采用版本。运行已登记的最小检查，核对原始报告，以及失败或零匹配时进度仍为未完成。没有实际测试的项目记录缺口。
- 用独立的测试信封和结果文件验证环境脚本。至少核对缺依赖时的负向结果、身份字段保留和清理。对具备条件的目标验证正向结果，记录尚未验证的目标。
- 按项目正常 Git/PR 流程审阅并发布到约定的 remote 和目标分支。用实际发布的完整 commit 核对所需文件及引用都已进入同一提交。记录采用版本、材料路径、实测结果、原始证据位置及未验证项。运行状态、凭证和临时日志留在仓库外。

目标分支包含上述材料并通过自检后，项目端初始化完成。打开目标安装实例的 Web，从项目切换器或零项目页面选择「添加已初始化项目」。填写服务机器上的仓库路径、remote、目标分支和 PR 目标，核对目标提交并确认添加。服务机器缺少该提交时，先在该仓库执行 fetch。保存后使用现有安装入口重启宿主，再从项目切换器打开项目。

项目在线后，对实际运行机和执行方案分别发起环境检测，核对当前资格。有实际任务后，在「项目维护」核对任务材料与测试安排，再按项目需要启用派发。

后续维护可按需查阅 [项目初始化详解](project-initialization.md)、[项目材料规范](project-standards.md)、[测试架构规范](testing-architecture.md)、[环境适配指导](environment-adaptation.md)和 [Spec 与 Plan 编写规范](spec-plan-guidelines.md)。其中引用 SteerLine 平台源码或产品文档的位置，在所采用版本的 SteerLine 源码仓库中查看。
