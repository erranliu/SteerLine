# SteerAgent 分类收敛 Spec

状态：2026-11-10 完成审计并按用户指令实施；实际测试结果以专属验收为准。依据为 [SteerAgent 长期设计](../../docs/product-design/steer-agent.md)，配套 [Plan](../plans/steer-agent-classification.plan.md)、[测试选择](../plans/steer-agent-classification.tests.json)及[审计与专属验收](../testing/acceptance/steer-agent-classification.md)。本次按普通开发实施分类收敛，不启动 roadmap，不扩展新能力。

## 1. 目标与基线

将现有“主 Agent”收敛为 SteerAgent 对话续作中的建议生成，建立四类业务能力与具体操作的初始结构。调用方能准确表达正在使用什么能力，业务状态和 Runner 执行仍由原所有者管理。本期不增加新的 LLM 业务能力、自动执行授权或后端。

调查基线为 SteerLine `a1451a16b3e7f4f025eff5c923cf7cbe13f63079`，长期文档包含本工作树的审计修订。当前主 Agent 核心只返回提示词，宿主经 generate/1 本地只读执行；冲突处理由 taskrunner 经原开发 Run 的 message 执行；Roadmap 解析和文档分别有独立记录。当前分类散落在模块名、配置、路由和活动投影中。

## 2. 分类与操作

分类由 SteerLine 的共享纯值定义维护，只提供四类名称及下表既有用途的类型/操作标识，必要时提供显示名称；用途和效果由本 Spec 说明。不建立运行时目录、组合校验器、枚举/查询 API，不维护可调用 handler、执行条件或资源状态，不进行 I/O、调度或授权。业务类型不成为 Runner workflow，不设置覆盖整类的“已支持”开关。

| 类型 | 本期操作 | 实际含义与原所有者 |
| --- | --- | --- |
| material | generate_documents | 现有 Roadmap Spec/Plan/验收文档生成；记录、审阅及发布仍归 Roadmap |
| continuation | suggest | 原主 Agent 的提示词建议；核心与宿主分别承接原职责 |
| continuation | resolve_conflict | 现有解决合并冲突入口；沿 taskrunner 原候选命令与 Run |
| planning | parse | 现有 Roadmap 草稿解析与依赖建议；确认及入图仍归 Roadmap |
| maintenance | 无已实现操作 | 仅声明长期分类，没有配置、入口、执行服务或后台轮次 |

上述四个用途仍从原业务入口调用，不接受用户提供 type/operation 来选择执行。只有产品查询层在需要展示现有建议、Roadmap 解析及文档活动时读取共享定义；冲突处理仅明确业务归属，不为凑齐四个代码消费者给 taskrunner、runnerbridge 或 Runner 请求添加分类依赖。不得从模型回复、标题或诊断文案推测分类。查询层无法识别实际来源时不附加分类，不默认转 suggest 或普通开发；业务非法输入仍由原 owner 拒绝，分类缺失不新增停止、取消或全局阻断。原问题回答、resume、取消、停止、观察、文档发布和入图属于原业务控制，不成为新增智能操作。

## 3. 核心、宿主与业务解耦

现有 `internal/mainagent/contract` 和 `internal/mainagent` 分别移入 `internal/steeragent/suggestion/contract` 与 `internal/steeragent/suggestion`，只承接提示词建议的值契约、输入组装和唯一 prompt 校验。模块按实际产物命名，不把整个 continuation 类型的其他执行能力纳入建议契约。契约仅依赖标准库；核心不依赖 Runner、存储、Git、网络、进程或业务命令。共享分类定义位于 `internal/steeragent`，保持中性，不反向依赖具体能力、业务所有者或 Runner。

`internal/mainagentbridge` 移入 `internal/steeragentbridge/suggestion`，保留来源读取接线、异步记录、冻结请求、共享队列、观察和结算。app 的组装、启动恢复、关闭及离线停止核验同步更新，不能遗漏辅助执行。两处 suggestion 模块沿用既有核心/宿主边界，不为四种产品类型各建模块、端口或服务。

Roadmap 与 taskrunner 保持现有业务实现，不因展示分类新增包装调用或迁入统一 Agent 调度器。文档记录仍归 Roadmap，冲突处理不新建作业、Run 或会话，分类不复制 Task/DAG/Question。不同能力不共用任意 action/result 对象，不新增统一执行总线、动态注册框架或未来类型空服务。产品分类不强制决定包层级；今后只有出现真实职责和消费者时才增加相应模块。

## 4. 配置与调用方命名

本期调整代码与产品命名、重归专用配置，范围固定如下：

| 范围 | 本期名称与要求 |
| --- | --- |
| 建议模型设置 | `continuation_suggestion = {model, reasoning_effort}`，替换 `main_agent`；是现有执行配置中的一个具体能力设置，不建立类型/操作配置树 |
| 建议业务协议 | 保留 `steerline-main-agent/advisory-v1`、advisory capability 及原输入输出；它是当前有效的协议身份，不增加新协议或别名 |
| 建议查询与取消 | 保留 `/main-agent` 及其详情、candidate_id、`main_agent_cancel` 和原命令/回执身份；仅内部端口与代码命名按职责收敛 |
| 建议活动与存储 | 保留 `main_agent_advisory`、prompt_suggestion、表结构、payload、来源去重键、correlation 及停止信封；分类不改变持久身份 |
| 产品文案 | 总称 SteerAgent，具体设置和详情使用“对话续作 · 提示词建议”，不再称为主 Agent |

专用设置仍所有项目共享，默认保持 `gpt-6.1-sol` / `high`，仍只使用已有本地 Codex 执行配置与原生目录校验。仅替换配置键与类型名称，沿用原设置值和校验语义：缺省或 null 使用默认值，提供对象时须有两个有效字段，非法值和未知字段按原规则拒绝。不新增配置继承、能力列表或其他类型的空配置。保存时保留无关 profile、并发和用量设置及原条件保存要求。

保存及重启生效规则不变：已加载宿主继续使用其快照，重启加载新设置，准备新建议请求时固定；已准备、排队或执行的原建议请求不因设置变化重建。原 Run 的冲突处理不读取此设置，也不切换模型或权限。Roadmap 继续选择原 profile，不增加材料、规划或维护的虚构专用设置。

仅专用配置键一次性替换，当前消费者同步更新；旧 `main_agent` 配置键按未知字段拒绝，不读取、迁移、双写或兜底，未带该专用键的合法配置仍按原缺省规则处理。保留执行配置 version 3，不为改名递增外层版本；不更改数据库格式或建立旧 home 升级流程。保留当前有效 wire/存储身份不属于新增兼容路径。本期不删除现场、不执行安装升级或部署，验收使用自有隔离 home。

固定历史 Spec/Plan、验收证据、受测 commit 和稳定测试 ID 不批量改名；现行文档说明哪些标识保持。保留原记录解码和同构建重启恢复，不因模块移动改请求、去重键、回执或停止责任；这不增加跨构建接管能力。

## 5. Runner 对接与行为保持

Runner CLI 协议、DTO、workflow、公开能力和已支持的后端不变。本期不修改 Runner 仓库或要求升级构建，不新增运行前查询、现场探针或执行门禁。已有能力校验继续由原入口消费原 catalog/schema；共享分类只表达用途，不能另写一份执行组合白名单或准入状态机。

suggest 仍使用 generate/1、原完整文本通道和私有空目录；不提供仓库、业务目录、文件材料或输出 schema。Roadmap parse/generate_documents 仍使用原固定仓库、文件材料和 schema，来源及结果校验保持。resolve_conflict 仍使用原开发 Run 的 message 与原短指令，不改投 reply、resume、execute 或 conversation。

能力依赖按操作隔离：suggest 不因缺仓库/file 材料能力而受阻；Roadmap 两个生成操作仍检查各自实际需要的固定材料能力；原 message 不因缺 generate 能力受阻。一个操作不可用不撤销原问题回答、观察、停止或无关操作的资格；已有全局构建/实例及原业务规则仍有效。

原建议自动来源、去重、完整上下文、未知零重投、失败不自动重开、取消、源解除、共享配额、停止信封及重启恢复全部保持；具体规则引用[现有建议 Spec](main-agent-advisory.spec.md)和[Worker 生命周期](../../docs/product-design/worker-lifecycle.md)。本期仅改调用方命名和归属，不改变这些规则。

固定提示词、规则、golden 的路径及正文保持；核心移动后仍调用 prompts 同包现有组装函数，不创建第二份资产。固定事实和身份相同的建议、冲突及 Roadmap 模型正文、材料、schema 和 digest 应与基线一致，分类元数据不进入模型上下文或 Runner 请求。按[提示词治理](../../docs/prompt-governance.md)对实际输入给出完整差异及字节对照，接线和对应测试须独立审核；不能因预期 0 B 变化免审，也不能以更新 golden 消除不符合预期的变化。

## 6. 查询、活动与界面

只在已有建议和 Roadmap 活动项及相同的 request_links 项中增加可选 `steer_agent: {type, operation}`，由产品查询层按来源和 Purpose 派生，供现有活动卡片显示能力名称，不增加重复详情字段或数据库分类权威。Roadmap 当前 Purpose 为空表示 parse，documents 表示 generate_documents；未知值不猜测分类。非 SteerAgent 来源不带该字段，客户端不显示能力标签。活动及 request links 复用原记录，不增加重复条目、分组状态或新筛选工作台；冲突入口与原命令保持，不另建分类接线或活动来源。

分类只说明工作用途。发布、合并、入图等阶段继续展示原 phase、action 与实际 execution_ref，不因存在材料分类就显示模型仍在运行；原候选的实施活动不整体标成冲突续作。本期分类投影及建议 GET 只读已保存事实，不触发模型、Git、Runner 对账或业务写入；其他现有读取入口沿原契约，不增加新全局门禁。

设置与建议详情保留当前操作面：读取模型目录、保存原覆盖、查看、复制、取消。不新增生成按钮、答案采用、自动续作、材料发布捷径、多轮规划或维护入口。

## 7. 非目标与通过条件

本期不实现通用材料服务、建议自动回答、任意消息编辑器、conversation 接入、多轮规划、维护调度、跨轮记忆、新 LLM/后端、权限扩展、严格隔离、跨构建接管或新的交付机制；不调整 roadmap 执行清单。

| ID | 验收要求 |
| --- | --- |
| SAC-A01 | 四类与四个既有用途归属清楚，三种内容活动按原来源映射；未知来源不附分类，maintenance 无操作，控制通道不误归类；无运行时目录、组合校验器或通用执行/准入接口 |
| SAC-A02 | 共享定义和建议契约/核心依赖符合边界；业务记录不复制，冲突沿原 Run，Roadmap 材料与解析仍由原 owner 管理 |
| SAC-A03 | 新配置缺省/完整/非法及旧键拒绝、条件保存和冻结时点完整验证；当前协议/路由/命令/来源/存储身份保持，原记录同构建恢复且无重建/迁移 |
| SAC-A04 | 原 Runner 参数与按操作隔离的能力准入保持，缺必要能力明确不可用、缺无关能力不新增阻断；零新增 CLI/原生/模型旁路、现场查询、workflow 或后端 |
| SAC-A05 | 建议自动触发、上下文、去重与失败语义不变，建议成功不投递答案、不修改材料或原 Task |
| SAC-A06 | 未知零重投、取消跨进度版本、源解除、重启停止恢复和未结算共享槽等原可靠性要求保持 |
| SAC-A07 | 活动及 request links 分类相同，已知/未知 Purpose 与无关来源映射准确；计数、深链、设置/复制/取消及隔离保持，阶段和执行身份不被分类覆盖，GET 零推进 |
| SAC-A08 | 资产路径/原文保持，同输入模型正文、材料/schema/digest 逐字一致，完整差异和字节数可审阅；独立审核覆盖接线与测试，不以 golden 更新替代 |
| SAC-A09 | 同名测试安排及必要定向覆盖完整执行，正式报告非零、无 skip、原始失败保留、资源清理成立，不继承旧版本通过 |
| SAC-A10 | 长期设计、当前功能说明、接口/配置导航和缺口清单与实现一致；无未来能力空壳或未经实现的可用声明 |

具体自动行为、证据和实时状态归[专属验收](../testing/acceptance/steer-agent-classification.md)，本 Spec 不记录运行结果。
