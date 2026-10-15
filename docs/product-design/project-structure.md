# 项目结构与纳管核验（方案）

状态：拟议规则 `steerline-project/v1`，2026-10-15 形成，尚无产品核验器实现。用于[项目添加流程](project-management.md)，与[Codex 准备提示词](project-onboarding-prompt.md)配套。本文维护仓库结构规则；不是 SteerLine 自身仓库的一次初始化指令，也不要求本轮修改目标项目。

## 三种资格分别判定

| 判定 | 证明什么 | 不替代什么 |
| --- | --- | --- |
| 结构通过 | 固定 Git 版本内有可读取、符合角色和路径约定的准备文档 | 文档语义审查、编译/测试或执行环境就绪 |
| 可纳管 | 结构通过，且仓库身份、配置/发布目标、状态位置及重复登记检查通过 | 服务已启动、存在可执行任务 |
| 可执行 | 对具体 Task/执行方案的原任务契约、工作区、环境、权限等检查通过 | 不因项目已纳管而自动获得资格 |

核验是服务端只读操作。仅“确认添加”命令可以在复核通过后创建项目库。不得调用具有初始化副作用的任务/项目 open 路径来冒充结构检查；也不运行仓库脚本、依赖安装、Git hooks、测试或代理会话来判定目录结构。

## 最小仓库约定

原型的四类长期文档继续保留。默认布局如下；已有项目通过下面的角色清单显式采用现有文件，不要求为了匹配默认命名复制另一套正文。

```text
<repository>/
├─ .steerline/project.json              # 只含版本和文档角色/设计目录映射
├─ AGENTS.md                           # 项目协作与执行约束
├─ README.md                           # 项目入口
└─ docs/
   ├─ architecture.md                  # 架构事实
   ├─ architecture/parallel-development.md
   ├─ testing.md                       # 现有测试方法和最小入口
   ├─ spec/
   │  ├─ README.md                     # 后续任务文档约定
   │  └─ SpecGuide.md                  # Spec/Plan 编写规则
   ├─ acceptance/README.md             # 验收记录位置与约定
   └─ design/README.md                 # 长期设计目录入口，可显式映射现有目录
```

Git 不保存空目录，因此目录入口 README 是实际文件要求，不用空目录或占位 `.gitkeep` 代替。`docs/spec`、`docs/acceptance` 是本版固定约定；四类长期文档及设计目录由清单定位。不要求仓库具备 SteerLine 自己的全部目录、RoadmapGuide、execution-map、任务编号脚本、特定语言、测试分级实现或发布脚本。添加项目不生成示范 Task/Spec/Plan/验收成功记录，也不要求先规划 roadmap。

仓库中的 `.steerline/` 只存受 Git 管理的准备清单，不存数据库、服务发现、认证或运行状态。运行状态继续位于应用管理的仓库外目录。

### 角色清单

以下为完整默认内容；全部字段必需，未知字段、重复 JSON key、未知格式版本或缺失角色都失败，不从文件名猜测并放宽规则：

```json
{
  "format": "steerline-project/v1",
  "documents": {
    "architecture": "docs/architecture.md",
    "parallel": "docs/architecture/parallel-development.md",
    "testing": "docs/testing.md",
    "spec_guide": "docs/spec/SpecGuide.md"
  },
  "design_root": "docs/design"
}
```

这是拟议的新准备清单，不是现有 `ProjectConfigInput`。其中不接受 repository、remote、branch、run_dir、web_address、命令、hooks、执行方案或密钥；这些属于服务端纳管输入/应用配置。清单也不能声明自己已核验或已纳管。

角色文件各不相同，均为仓库相对 `.md` 路径。已有架构、测试文档可以原位采用；Codex 应整理必要内容并修改映射，而不是复制正文。设计目录同理：SteerLine 自身已有 `docs/product-design/`，若今后按本方案接入，应将 `design_root` 指向它，不再创建重复的 `docs/design/`。这只是未来映射示例，本轮不向当前仓库新增准备清单。

### 内容编写要求

下表供 Codex 根据实际仓库整理内容，也供用户检查质量；程序不能仅用几个关键词或模型自述证明内容真实。

| 文档 | 至少应解释的内容 |
| --- | --- |
| 根 README | 项目用途、代码/文档入口、现有启动或构建方式；没有可运行功能时如实说明 |
| 根 AGENTS | 修改边界、常用检查、仓库协作约束、提交/发布授权边界；不得要求直接改中心状态 |
| architecture | 实际模块及职责、主要数据/调用路径、状态与权威、当前能力与已知缺口 |
| parallel | 哪些修改可独立交付，哪些共享文件/接口需协调，依赖及合入条件，工作区/数据隔离 |
| testing | 已有最小测试命令、环境、通过条件与证据位置；尚无测试时明确现状，不虚构命令或通过结果 |
| spec_guide | 先调查代码再整合、Spec 与 Plan 分工、范围及非目标、最小必要测试、自动/人工验收分别安排、结果进入验收记录；普通开发不强制 roadmap |
| spec README | 任务 Spec/Plan 的组织和独立职责；具体任务创建时再固定 Git 引用及验收要求 |
| acceptance README | 自动/人工项和实际证据如何维护，待验/失败/通过分开，不以文件存在宣称通过 |
| design_root README | 设计目录用途、现有资料入口与维护约定；空白新项目明确尚无专题设计 |

本版机器内容门槛仅为：合法 UTF-8、非空、具有非空 Markdown 一级标题，以及至少一段非标题/非纯空白正文。它只能排除空壳文件，不能判断架构、命令或约束的语义质量。允许正文如实记录尚无实现或测试；不以 `TODO` 关键词一律拒绝，也不把占位说明包装成已交付能力。语义矛盾由用户/Codex继续处理，不在本版引入自动模型审核。

## 核验输入与固定版本

服务取得用户选择的仓库目录、remote、目标分支和发布目标，自己解析工作树顶层及规范 Git common-dir；项目身份复用当前 common-dir 派生规则，不能相信清单中的身份声明，也不能只看目录名称。支持 linked worktree，但同一 common-dir 不能登记多个独立项目。

核验精确解析选定目标分支为完整 commit，读取该版本的清单和所引用 blob。HEAD 恰好处于另一分支、工作区未提交文件或未合入的准备 PR，均不能替代选定分支版本。没有初始 commit、目标分支不能解析、文件只存在于工作区时，返回具体准备要求。无关代码 dirty 不构成结构失败；准备清单和必需文档若在当前工作区有未提交的不同内容，提示并阻断，避免用户以为核验了那些修改。

核验结果记录规则版本、规范仓库身份、remote/目标分支、解析 commit、清单摘要、角色路径及文档摘要、纳管参数摘要、逐项结果和时间。最终添加重新解析并比对；发生变化则重新核验。结果由服务生成，不接受浏览器或 Codex 提供的通过布尔值。

远端同步和凭证可用不是文件结构检查。服务另外检查 remote 已配置、URL 与发布目标一致、参数格式受支持；远端暂不可达或执行环境未就绪按对应资格显示，不伪装成文件缺失。具体任务启动仍需确认其固定 commit 与所需资源实际可取得。

## 程序检查顺序与失败项

以下 PS 编号是拟议核验规则标识，不是 Task ID 或已经存在的测试场景。

| 规则 | 必须核验 | 失败反馈 |
| --- | --- | --- |
| PS-01 仓库 | 可访问的非 bare Git 工作树；规范顶层/common-dir；分支及完整 commit 可解析 | `repository_unavailable`：实际路径、失败环节与原 Git 诊断 |
| PS-02 准备清单 | 固定版本包含 `.steerline/project.json`；本版 JSON 字段/格式/角色完整且不冲突 | `manifest_invalid`：文件、字段及原因；不能静默猜路径 |
| PS-03 路径与对象 | 所有必需文档及目录入口都是本次 commit 的普通 blob，文件路径精确匹配、无歧义 | `required_file_missing` / `unsafe_document_path`：具体角色、路径、对象类型 |
| PS-04 最小正文 | 清单不超过 64 KiB；每份必需 Markdown 不超过 1 MiB、合计不超过 8 MiB；UTF-8/标题/正文有效 | `document_invalid`：缺失内容或实际大小；不执行文件验证其真实性 |
| PS-05 必需文档链接 | 必需文档正文中的相对 Markdown 链接可在同一 commit 解析到文件/目录且不越出仓库；外链不联网探测，代码示例不当成真实链接；本版不要求全仓文档/锚点扫描 | `local_link_missing`：源文档、链接与解析后的目标 |
| PS-06 观察一致 | 最终添加时，仓库身份、目标 commit、清单/文件摘要、规则和纳管参数仍与核验一致 | `validation_stale`：变化字段，要求重新核验 |
| PS-07 纳管参数 | remote/发布目标一致，应用配置可表达；状态在仓库外且位于本应用受管边界；拒绝同仓重复、错误归属或不兼容原生状态，以及既有 `.beads` 等冲突 | `registration_blocked`：指出配置、重复项目或状态冲突；不迁移/删除/接管 |

清单路径沿用原任务文件引用的规范：相对路径、统一 `/`，不允许绝对路径、盘符、反斜杠、`.`/`..` 穿越或经符号链接/目录别名逃逸。不能把 symlink、submodule/gitlink 或目录当正文。文档内的相对链接允许 `../`，从源文档位置规范解析后必须仍在仓库内；锚点-only 链接及外部 URL 不进入本版文件存在检查，目录链接要求 Git tree 存在，文件链接要求目标 blob 存在，不顺着 symlink 打开宿主文件。服务使用 Git blob 与明确允许的只读 Git 操作检查；不通过资源管理器展开后执行“自检脚本”。清单不是命令配置。

只检查本清单与必需文件集合，不递归扫描全部源码、所有 Task 或完整历史。错误尽量逐项返回；仓库/清单无法解析时，其依赖检查标为“未检查”，不能将未检查当通过。一个项目核验错误不能影响已管理项目。

PS-07 中已有 current-format 数据只允许身份/目标匹配的明确复用；旧版格式不是本方案的读取对象。结构通过但纳管参数失败时，界面应保留两者结果，不能将“结构通过”显示为“项目已添加”。执行方案尚未安装、Orca 仓库未登记、账户未登录等显示为执行就绪事项，不运行 worker 或真实 PR 来探测项目是否可以添加。

## 与现有能力的整合

- 复用 [workspace](../../internal/execution/workspace) 的仓库身份解析及 [httpcontract](../../internal/product/httpcontract/contracts.go) 的项目 ID；不另造按名称识别的项目编号。
- [task_documents.go](../../internal/app/task_documents.go) 已有固定 commit 普通 blob 读取，[task/model/contract.go](../../internal/task/model/contract.go) 有规范路径验证；实施时提取实际共用读取/路径能力，新增本文的角色与大小/正文检查，不能因此构造假 TaskContract。
- [Preflight](../../internal/app/service_preflight.go) 与 [原生状态前置](../../internal/app/host/native_state_preflight.go)可提供配置/状态边界检查；须分离具有环境探测或初始化职责的部分。查结构不能调用 [OpenNativeTaskProject](../../internal/app/native_project_open.go) 提前建库。
- 只有最终纳管提交可复用原 `InitializeProject` 及初始手动/暂停状态。添加项目不改变任务的 Spec/Plan/AcceptanceRefs 契约、DAG 准入、执行方案冻结或交付规则。

上述清单格式、检查项与错误码均待实现；当前版本不会因本文存在而检查这些要求。实施测试及自动/人工验收安排由后续本任务专属 Spec/Plan/acceptance 固定，本轮不宣称核验器已经可用。
