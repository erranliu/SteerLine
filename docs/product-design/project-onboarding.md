# 项目接入与管理

本文维护当前项目选择，以及拟议添加、结构核验和登记规则。仓库实际准备方法见[初始化指南](../../SteerlineProjectGuideDoc/project-initialization.md)，独立复制正文见[准备提示词](project-onboarding-prompt.md)，已配置项目的同步/材料/新候选就绪归[项目维护](project-maintenance.md)。

| 范围 | 当前状态 |
| --- | --- |
| 统一启动、独立 Web、配置项目切换 | 已实现；各项目在共享后端内独立运行，范围见[切换验收](../../SteerlineFlow/testing/acceptance/web-project-switch.md)和[统一启动验收](../../SteerlineFlow/testing/acceptance/unified-startup.md) |
| 复制准备 → PS 核验 → 确认纳管 | 后置设计，完整 UI、核验器与正式登记未实现；现有切换不依赖拟议清单，配置存在不证明结构通过 |

下文“拟议”内容不作为当前项目或任务的新门禁，不创建清单、数据库或执行状态。

## 当前项目与切换

界面围绕当前项目的 DAG、任务、候选、问题、文档和设置；切换器可搜索仓库位置、在线/离线及配置错误，零项目显示只读说明。Web 使用稳定本机地址，URL/请求绑定稳定项目 ID，各标签页独立选择，最近选择不抢改已有页面。

重名按仓库位置区分，同 common-dir 重复配置显示冲突并拒绝路由，不按目录名认身份。目标无对应任务/候选返回其列表，不用同号 Task 自动定位。缓存、草稿、选择和原请求按项目/实例/执行身份隔离；切换取消旧查询、资格检查及未提交回调，已提交/未知命令留在原项目按原号核实。

目录与连接分别限时，坏配置或慢项目不拖住切换器。Go 应用服务管理项目目录与业务资格，Web 不接受页面指定任意后端 URL。共享后端、独立 Web 和各项目资源见[代码架构](../architecture/current-code-architecture.md)；Web 重启不停止项目，浏览器断线不证明停服。认证、缓存和命令归[Web README](../../web/README.md)。

## 拟议添加流程

| 步骤 | 行为与副作用边界 |
| --- | --- |
| 1. 选择目标 | 填服务机器仓库路径、显示名、remote、目标分支及发布目标，服务解析规范身份；只读，不写仓库/配置/库，同仓已管理时给原入口 |
| 2. 准备文档 | 用户在自己的 Codex 会话使用完整提示词；SteerLine 不启动 Agent 或写仓库，已准备项目可直接核验 |
| 3. 返回核验 | 从目标分支固定 commit 读取结构和参数，逐项展示；可存应用级记录，不建项目库/登记，不采信 Agent 报告或浏览器 passed |
| 4. 确认纳管 | 用户确认目标、版本和状态位置；服务重核原依据，核对/初始化原生权威后发布唯一登记，仅此步允许新建项目状态 |
| 5. 返回项目 | 原请求返回结果和在线状态；新库为空 DAG，合法现有库明确复用，不生成任务/自动派发，启动另须显式操作 |

正式登记前的候选只属于添加面板。准备文本须自足，未知输入保留待确认，不猜认证、分支或测试命令；工作区文件、准备分支或未合入 PR 不代替目标版本。仓库准备不授权 push、合并或纳管。

## 拟议结构与核验

| 结论 | 能证明什么 |
| --- | --- |
| 结构通过 | 固定 Git 版本的必需文件满足角色、路径与最低格式 |
| 可纳管 | 结构通过，身份/发布目标/状态位置及重复登记检查通过 |
| 可执行 | 具体任务和方案满足原契约、工作区、环境及权限 |

三者独立。结构不证明语义真实、测试通过或 Agent 可运行，可纳管也不等于已登记/启动。核验只读 Git 对象，不运行脚本、hooks、安装、测试、模型或项目初始化；只有最终添加可创建库。

### 最小仓库约定与角色清单

拟议格式 steerline-project/v1，固定入口是根 README.md、AGENTS.md、SteerlineFlow/spec/README.md、SteerlineFlow/testing/acceptance/README.md 与 .steerline/project.json。四类长期文档及设计目录显式定位，采用已有正文，不复制新的一套；目录以真实 README 保存到 Git，不用空目录或 .gitkeep 代替。

```json
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
```

全部字段及四角色必需，未知字段、重复 JSON key、未知版本或缺角色均失败，不猜路径。清单只描述角色和设计目录，不接受仓库/分支、运行目录、命令/hooks、预设、密钥或 passed。它不是当前应用配置，.steerline/ 不存运行状态、认证或发现记录。

角色指向互不相同的仓库相对 .md 普通文件，设计目录有 README。可映射已有 docs/product-design/，不复制 docs/design；testing 与维护设计衔接时用 SteerlineFlow/testing.md，清单依项目测试入口定位。不要求预先有具体任务、Spec/Plan 或 Roadmap。

| 文件/角色 | 正文应解释 |
| --- | --- |
| README / AGENTS | 用途与入口、真实启动/检查、修改协作及发布授权 |
| architecture | 模块、调用/数据路径、权威、当前能力与缺口 |
| parallel | 独立成果、共享修改、依赖、集成与资源隔离 |
| testing | 真实最小入口、环境、通过条件和证据；无测试如实说明 |
| spec_guide | 调查、Spec/Plan 分工、最小验证与自动/人工验收；普通开发不强制 roadmap |
| spec / acceptance README | 任务文档职责、待验/失败/通过及证据维护 |
| design_root README | 用途、现有设计与维护约定；空白如实说明 |

最低检查为 UTF-8、非空、有非空一级标题和实质非标题正文，仅排除空壳，不用关键词或模型自述证明质量，不一律拒绝真实 TODO。清单最多 64 KiB，必需 Markdown 单份 1 MiB、合计 8 MiB。

### 固定版本与路径

解析用户目录的工作树顶层、规范 common-dir、remote/目标分支完整 commit，从该版本读取清单/blob。linked worktree 共用仓库身份，无初始提交、目标不可解析或文件只在工作区时要求准备。无关代码 dirty 不使结构失败，清单/必需文档未提交差异则提示并阻断。

清单路径用 /，拒绝绝对路径、盘符、反斜杠、./.. 穿越、符号链接或别名逃逸。文件为精确普通 blob，不用 symlink、submodule/gitlink 或目录代替。正文链接允许 ../，解析后须在同一 commit 仓库内，文件要求 blob、目录要求 tree，不沿链接读宿主；外链、仅锚点和代码示例不作存在检查，不联网。

记录规则版本、身份、目标 commit、清单/文件与纳管参数摘要、逐项结果及时间；最终提交重核，变化要求重新核验。remote/发布目标另核，远端暂不可达、账号或环境未就绪不伪装成缺文件，实际任务启动仍查资源可取得。

### PS-01–PS-07

以下是拟议规则标识，不是 Task ID 或已登记测试；只检查本清单及必需集合，不递归全仓/任务/历史。前置不可解析时依赖项标“未检查”。

| 规则 | 必须核验 | 失败反馈 |
| --- | --- | --- |
| PS-01 仓库 | 可访问的非 bare 工作树、规范顶层/common-dir、目标分支及完整 commit | repository_unavailable：路径、环节及 Git 诊断 |
| PS-02 清单 | 固定版本 .steerline/project.json，严格字段、版本及角色 | manifest_invalid：字段/原因，不猜路径 |
| PS-03 路径与对象 | 必需文件/目录入口为本次 commit 普通 blob，精确匹配且无歧义 | required_file_missing / unsafe_document_path：角色/路径/类型 |
| PS-04 正文与限额 | 清单 64 KiB、单份 Markdown 1 MiB、合计 8 MiB，UTF-8/标题/正文有效 | document_invalid：缺口或实际大小 |
| PS-05 文档链接 | 必需文档相对链接在同一 commit 存在且不越界，不作全仓/锚点扫描 | local_link_missing：源文档/链接/目标 |
| PS-06 一致性 | 最终提交的仓库、commit、摘要、规则及参数与核验一致 | validation_stale：变化项，要求重核 |
| PS-07 纳管参数 | remote/发布目标一致、配置可表达；状态在仓库外且位于本应用受管边界；无同仓重复、错误归属、旧格式或 .beads 等冲突 | registration_blocked：具体参数/状态，不迁移、删除或接管 |

结构通过但 PS-07 失败仍保留两个结论。当前格式库仅身份/目标匹配时明确复用。缺预设、Orca 未登记或登录不可用属于执行就绪问题。纳管资格依据结构和登记核验，核验过程不启动执行 Agent 或创建真实 PR。单仓失败不影响已管理项目。

## 拟议登记与恢复

应用服务是 home 内项目管理唯一写者，零项目或全离线也应可添加/核验，不构造伪项目数据库。候选解析和结构检查不依赖执行方案、登录或 Orca，不调用会初始化的完整 open/preflight；仅复用窄 Git/路径/材料检查，不伪造 TaskContract。

原请求固定 ID、home、common-dir、分支/commit、规则、清单/文件及纳管参数摘要，实例为受理事实；最终重核变化则重验。显示名不改身份，同仓登记串行，目录/配置由应用服务发布，未来所有新初始化入口统一准入，不只限制按钮。

| 情况 | 处理 |
| --- | --- |
| 核验失败/取消未提交面板 | 保留输入/问题，不建库、改仓库或删状态 |
| 重复请求/并发同仓 | 返回原结果或已有项目，不建第二库 |
| 初始化失败 | 不发布登记，保留发生的状态及原因 |
| 初始化成功但登记/回执失败 | 查原号、原库权威和发布记录补齐原操作，不换号重建/删半成品 |
| 已有库 | 仅当前格式、仓库/remote/branch 与归属匹配时明确复用；旧格式、别的 home 写者或身份错误阻断，不迁移/清空/接管 |

数据库和配置不伪装为跨介质事务：最小请求/完成事实存应用目录，核验前不建项目库，初始化成功后原子发布唯一登记。新库沿原权威保持 manual + paused；现有任务/DAG、候选冻结、已受理执行、反馈/交付和资源操作不因未来登记状态被改写或停止。

重启后查询原请求只读、不自动补交；继续使用同 ID/正文，重核当前实例与原依据，完成则返回原结果。本方案不含项目删除、迁移、克隆、工具安装、Web 调度 Codex 或自动发布。

## 维护与历史

实施与自动/人工验证归[项目添加 Spec](../../SteerlineFlow/spec/project-onboarding.spec.md)、[Plan](../../SteerlineFlow/plans/project-onboarding.plan.md)和[专属验收](../../SteerlineFlow/testing/acceptance/project-onboarding.md)。早期调查见[第三批来源](../../SteerlineFlow/archive/design/README.md#第三批整理来源)，两份旧文档及当前合并入口见[合并来源](../../SteerlineFlow/archive/design/runner-reliability-history.md#首轮文档合并)。结构/登记文档存在不证明能力已交付，也不改变当前任务执行资格。
