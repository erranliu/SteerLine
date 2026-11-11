# 原生执行配置边界收口 Spec

日期：2026-11-05，状态更新：2026-11-06。状态：代码实施及本轮规定的自动验收完成；固定 Runner 的远端正向资格已补验通过，分项证据以专属记录为准。进入用户授权的Runner→SteerLine提交/推送/PR交付，上游为[Runner PR #122](https://github.com/erranliu/SteerRunner/pull/122)。不启动roadmap、不分配Task ID、不合并或部署、不切换安装现场。对应 [Plan](../plans/native-execution-configuration-boundary.plan.md)和[专属验收规划](../testing/acceptance/native-execution-configuration-boundary.md)。

SteerLine 调查基线：`9b2e5d886b8f56eaf07bc697ca78c578c91eec00`。Runner 调查基线：`a66038c89208392c47531e640c84e97728c62424`，已在独立 worktree 分支 `codex/native-execution-config-docs` 核对并建立 `docs/spec/native-execution-configuration-boundary.spec.md`、同名 Plan 与专属验收草案。新增 CLI、capability 和字段已有实现；固定 EXE 与分项结果见专属验收，不把源码基线当作已验 EXE。

## 1. 问题与目标

既有[边界收口](runner-boundary-consolidation.spec.md)已把原生目录连接归 Runner。本轮处理执行配置的原生字段解释、转换及配置身份投影，不改 Task/DAG、Workflow 推进或恢复授权。

| 当前实现 | 问题与预期变化 |
| --- | --- |
| [environmentadaptation/service.go](../../internal/environmentadaptation/service.go) 的 useNativeOrcaExecutor | 环境检测自行识别 Orca/Codex，删除 agent_launch 的 executable/runtime/args。改由 Runner 按目标 execute/1 派生执行配置。 |
| [runnerclient/content.go](../../internal/runnerclient/content.go) 的 GenerationExecution | 调用方硬编码本机 Codex 字段复制及 read_only。测试生成、Roadmap 和 [project_materials.go](../../internal/app/project_materials.go) 的自动材料生成预检共用 Runner 的 generate/1 派生结果。 |
| [environmentadaptation/model/readiness.go](../../internal/environmentadaptation/model/readiness.go) 的 Context/Key | 资格纯模型理解原生 settings 布局并删除 model/effort。改为消费 Runner 提供的冻结身份投影，SteerLine 保留 key、basis 和资格规则。 |
| [Web schema.go](../../web/server/executionconfig/schema.go)、[store.go](../../web/server/executionconfig/store.go)及[配置页面](../../web/src/features/execution-configs/execution-configs.tsx) | 通过样例配置试探组合、按 adapter 改写模型字段、判断默认程序及 helper 结构。改用公开有限目录、逻辑字段和 Runner 配置构造；条件保存、部署授权和 UI 继续归 Web。 |

完成后，SteerLine 的运行请求构造、资格规则及页面编辑不再解释 agent_launch、transport 等原生 settings 路径，不自行选择原生默认 executable/args/helper，不用删除字段的方式切换执行用途。公开 schema 校验与原始配置只读展示保留。以下两类有明确消费者的读取/组装不在本轮迁移，不能用于替代已迁移的新请求路径：

- [RunnerProfileDisplay](../../internal/execution/model/runner_profile.go) 由 app/client、taskrunner/projection 和 product/query/runner_history 消费，继续只读解释候选原冻结请求，展示请求时的模型/effort/权限。它不构造 execution、不计算资格、不决定支持范围；历史读取不依赖当前 profile、describe 或新 CLI。当前配置的只读展示也不得反向成为构造或准入依据。
- [调试包生成器](../../scripts/new-debug-release.ps1)、debug-template 和 internal/testkit/remotetest 等可信部署/测试资产组装继续按选定构建的公开字段供给程序路径/参数，并接受同版完整 schema/gate 验证；部署来源、用户显式程序选择和文件复制/摘要校验仍由调用方负责。它们不放宽浏览器编辑权限；本轮不为迁移这些资产而给 compose 新增任意程序字段，也不把历史样例视为当前支持组合。

## 2. 两仓职能及保留边界

| 事项 | SteerLine | SteerRunner |
| --- | --- | --- |
| 产品配置 | profile ID/名称、默认选择、产品支持范围、命名方案 CRUD、execution.json 条件保存及显式重启 | 不读写 SteerLine 配置，不维护 profile 库或默认 profile |
| 原生配置 | 提交已选逻辑参数；冻结返回的完整 execution | adapter settings 布局、固定默认值、参数合法组合、原生配置构造/用途转换 |
| 工作流与权限 | 明确选择公开 workflow；开发请求明确要求 full_access，生成明确要求 read_only | 按公开合同解释/验证权限，不静默升权、不因失败换执行链 |
| 程序部署 | 可信配置来源、允许继承哪些已有程序、配置写权限；Runner EXE及部署资产自身的文件/摘要核验 | 解释可继承的原生设置和当前支持组合；拒绝退役程序/helper字段，不建立部署注册库 |
| 配置身份 | 保存投影，计算项目环境 key/basis，核验当前资格与失效 | 提供不含模型偏好的通用执行身份投影，不返回项目 ready/资格结论 |
| 运行期 | 固定材料、业务准入、原请求 outbox、TaskDelivery | 原生能力探测、资源身份、执行、结果与停止结算 |

Runner 既有 `capability_catalog`、`configuration_schemas`、`validate`、`native-catalog` 分别继续负责静态支持、输入校验和动态发现。新接口只补配置投影/构造；不增加另一份支持矩阵、动态账号库或自动安装程序。

PR 合入核验仍按[既有 Task 所有权](task-pr-merge-ownership.spec.md)。Workflow、异常、停止和重启继续遵循 [Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)，本轮无新增重试、自动输入、资源重建或停止范围要求。

## 3. Runner 新增的公开配置接口（提案）

### 3.1 入口、能力与无副作用要求

新增固定短命命令 `steerrunner execution-config --input - --json`，沿 `steerrunner-cli/v1` Response。stdin 为单个原始 JSON 对象，不是写信封；request_id=null。仅允许 --input、--json、--protocol，拒绝 state-dir、实例/完整绑定、Run/request 选择器及 --offline。在服务发现和 state-dir 规范化之前处理。

离线/在线 capabilities 同源发布 `execution_configuration`，包含 contract=`steerrunner-execution-configuration/v1`、command、operations、request_schema、response_schema、limits。operations 首版为 describe、derive、compose；schema 自包含。derive 的源/目标组合与可编辑逻辑字段从同一生产配置描述生成，支持条目引用既有 capability_catalog 的组合，不维护第二套手工笛卡尔积。

输入与完整输出分别最多 1 MiB，严格 UTF-8、唯一键、单对象、深度最多 64、未知字段与显式非法 null 拒绝；总调用预算 30 秒。操作只读输入与本构建内置资源，允许核验自身 EXE 身份；不读 profile/登录/原生配置，不检查输入中的程序文件，不连接原生端，不启动服务/Agent，不创建目录、Run、回执或 PR。调用方拥有该短命 CLI 的取消与回收。

成功 data 固定携带 contract、operation、input_digest、source；source 为实际 version、executable_sha256、resource_digest。input_digest 为完整输入对象按两仓一致的 CanonicalJSON 规则取 UTF-8 字节 SHA256。对应操作结果见下表，响应与退出码必须一致。

| operation | 请求的业务字段 | 结果与语义 |
| --- | --- | --- |
| describe | execution；workflow 为仅含 id/version 的公开选择器 | 返回 view、identity。只投影该 execution，不构造可提交的替代配置。 |
| derive | execution；workflow 为目标 id/version；access_mode 为调用方明确要求值 | 返回派生后的 execution 和其 view，以及原 source_identity。仅允许已公开的用途转换；不改原输入对象。 |
| compose | 可选 base_execution；完整目标 workflow；selection；unset | 返回完整 execution、view、identity。构造后校验完整 RunConfiguration，不修改输入 workflow。 |

view 只包含公开逻辑值 adapter、target、agent，以及原配置显式存在时的 model、reasoning_effort、access_mode；隐式 Agent 按公开确定语义投影。模型/effort 未固定时省略，不把静态或原生默认填成显式选择，不声称已核实实际执行模型。配置中的命令、参数、helper 路径及凭据引用不放入浏览器的逻辑编辑 DTO。

execution_configuration.reasoning_efforts 发布按 Agent 的有限逻辑 effort 选择，与生产原生配置 gate 复用同一取值定义；调用方不得把 Codex 取值应用于其它 Agent。

selection 是有限结构：adapter、target、agent、model、reasoning_effort、access_mode；前三项确定执行组合，后几项缺省时按 base 的可继承值及公开固定默认解释。unset 仅允许 model/reasoning_effort，不能与同名显式设置并用；null/空串不充当删除操作。程序路径、args、runtime、token、任意 JSON pointer/patch 均无输入字段。

权限是上述缺省规则的例外：compose 的 access_mode 必须显式提供并满足目标 workflow；derive 同样必须显式要求目标权限。不能从源方案、workflow 名称或旧缺省推导一次新的权限授权。describe 只报告已有配置，不补写权限。

compose 有 base 时仅继承匹配 adapter、完整 target（含 environment）、agent 的合法程序设置；跨组合必须由可信调用方选另一匹配 base，或明确从该组合公开默认构造。不能通过换 target.environment、Agent 或 workflow 偷渡不适用程序字段。参数缺项/不适用时返回错误，不能把远端换本机或选择其它模型。model/effort 等可继承性由 Runner 解释，所有未公开字段拒绝。

程序授权通过可信 saved base/default、重新 compose 和完整规范字节比较完成，不再增加 program_binding_digest、integrity_checks 或原生程序授权票据。Runner 调查基线的 contracts/run.go 与公开 configuration schema 已拒绝 Orca 的退役显式 launch/helper 字段，不为它们建设新的部署检查协议。Runner EXE pin 及可信部署资产自己的文件完整性核验继续归原 owner，不能与退役 helper 协议混为一体。

非法 JSON/参数/配置为 exit 2；未实现操作、组合或用途转换为 unsupported_capability/exit 3；输入读取及全部处理共享 30 秒期限，超时为 execution_configuration_timeout/exit 4；内部故障 exit 1。失败沿 v1 error、data=null，不返回部分可用 execution。CLI 无响应/超时由调用方记 unavailable，不改走旧字段转换。错误仅含安全字段路径/代码和固定说明，不回显整个 settings、命令、参数或凭据。

### 3.2 derive 的首版范围

| 消费方与目标 | Runner 规则 | SteerLine 继续负责 |
| --- | --- | --- |
| 环境检测 → execute/1 | 同一 adapter/target/Agent/模型偏好；当前合法 native Orca 源通常原样保留 execution，核对目标 workflow/权限适用性。退役显式 launch/helper/非空业务 args 源直接拒绝，不通过删除字段清洗。权限保持原 full_access 且须与显式请求一致。 | 项目脚本/材料、固定 commit、检测期限、执行正文、result-file、结果验收及资格 |
| 测试安排、Roadmap → generate/1 | 首版仅已声明的本机 Codex 源组合；复制合法 executable/模型/effort 偏好，按显式要求生成 read_only。开发 audit、publication、认证、任意开发 settings 不进入结果。 | 原生成目的、schema、材料/原文、内容验收、home 额度与记录 |

derive 校验源 execution 的当前公开结构、适用生产配置 gate 和已声明转换语义，不把旧构建退役字段洗成有效新配置。不能只用较宽的 ValidateExecution 绕过当前配置 gate 对退役原生字段的拒绝；从同一生产规则抽取所需纯判定。目标仅 id/version，不能因为 execute/1 正文尚未提供而错误要求 prompt_overrides；完整 Workflow、Run 及材料在调用方组装完成和 Runner submit 时分别按原 gate 再验。接口不得生成、删改或返回提示词正文。

describe 同样按 workflow 选择器核对 execution 适用性，不因缺少尚未组装的正文/审计参数而运行完整 Run gate；compose 接受完整 workflow，必须执行完整 RunConfiguration 校验。SteerLine 对派生目标的资格校验消费返回的 execution，不能先用目标 workflow 去拒绝尚未转换的源开发 execution。

### 3.3 通用身份投影及资格不变量

identity/source_identity 包含 contract=`steerrunner-execution-identity/v1`、source_execution_digest 和 context。digest 绑定原 execution 的规范 JSON；context 使用 ExecutionInput 形状承载 adapter、target 和不透明 settings，纯粹用于配置身份比较，不能用于构造 Run。

首版 context 排除模型/effort 偏好，保留 Agent、目标、权限及程序绑定等现有环境依据。对当前两仓共同接受的基线输入，除去除 model/effort 后无其它字段的空 agent_launch 偏好容器统一省略外，其规范字节与旧 Context 一致。此处只规范身份投影，不改原 execution；有 base 的 compose 保留隐式 Agent 的存在性与原 launch 容器，不能借模型编辑增删非偏好字段。旧活动不迁移，新格式使用新 home；不得借迁移把原本不同的执行环境合并。模型/effort 更换不使资格失效；目标、Agent、权限、程序绑定变化须改变身份或明确拒绝。缺省、显式空值及原生默认的等价关系按固定契约处理并做对照，不临时猜测。

identity 不是运行环境指纹或就绪证据：不包含机器在线状态、实际账号、安装探测、项目名或材料。环境 key 仍由 SteerLine 以冻结 context 的规范字节取 SHA256；项目材料 basis、current revision、ready 和失效继续由原项目规则决定。Runner resource_digest 仅固定投影算法来源，不直接拼入环境 key。

## 4. SteerLine 接入与冻结

### 4.1 可信调用与用途转换

runnercontract 增加纯协议值与严格校验；runnerclient 增加上述固定无服务动作的有界调用。Web 复用 nativequery 的元数据进程 owner，仍不导入后端 app、SQLite 或常驻 runnerclient。二者使用同一 runnercontract，不建立通用命令执行入口。

调用前后验证固定 EXE SHA/version/resource_digest，响应核对 operation、输入摘要、源 execution 摘要、返回 adapter/target/权限和所选 workflow。发布的 schema 和能力缺失/非法只令本次新配置操作不可用，不让可选能力解码失败阻断整个 Catalog、已有 Run 观察或停止。

环境检测在材料准备完成后、活动/原提交信封持久化前取得 derive 结果；完整冻结后才走原 outbox。原开发 Execution、原 identity 和实际派生 execution 分开保存，环境 key/basis 使用 source_identity；Run 请求使用派生结果。不得用 execute/1 的简化启动参数给原开发方案授予更宽资格。测试生成和 Roadmap 共用同一 derive 客户端，保留各自锁、CAS、额度、schema 与原文验证。

新请求一旦持久化，其观察、停止、同构建重启和未知提交只用原字节，不重新 derive/compose。请求持久化之前的纯配置查询失败可由显式新操作重新查询；不增加业务 submit 重试。保存失败零发送，响应未知不创建替代 Run。

### 4.2 环境身份从纯模型归位

配置加载/项目准备阶段按固定 pin 为现有 profile 建立只读 describe 快照；按 pin/resource_digest 与 source execution digest 键复用，避免每次状态刷新、每个项目或每个候选重复启动 CLI。Web 有自己的固定配置快照，后端不以 Web 返回值作为执行权威。配置变更和显式重启建立新快照，不采用晚到的旧结果。

环境活动及材料 Prepared 保存原 execution 与对应 identity 快照。model.Key/Context 的原生字段解析删除；模型只校验公开 envelope、源摘要、adapter/target 和规范字节并计算 key，Qualify 继续纯函数。app 的环境分组、basis、材料选择、状态及路由均消费同一快照/view，不再解析 agent_launch 取模型或 Agent。

SQLite 的 StartEnvironment、候选预留及资格复核在原事务内比较冻结 identity、key、basis、current/候选版本；不调用 Runner、不读配置、不启动进程，也不能只相信页面传来的 key。对活动 A 与当前开发候选 C，分别验证 `A.identity.source_execution_digest = H(A.Execution)`、`C.identity.source_execution_digest = H(C.Submit.Input.execution)`；跨对象只比较 context 的规范字节及由其得到的 key，不要求 A/C 的完整源摘要相等。H 为本 Spec 的规范 JSON SHA256。例如仅 model/effort 改变时，两边源摘要可以不同，context/key 与材料 basis 仍相同，原 ready 活动继续可用。实际目标/Agent/权限/程序变化仍拒绝沿旧资格准入。目标占用分组沿现有 adapter/target 公共值，不因模型偏好拆成新占用域。

identity 快照由受信后端在固定配置加载时取得，并随原活动持久化；摘要只证明绑定关系，不证明调用者提交的 context 正确。浏览器/HTTP 请求不能提供任意 identity 或 context，SQLite 不能只验证“摘要自洽”便接纳其资格。候选预留以当前后端冻结快照与已保存活动的项目权威相互复核，配置摘要变化沿原准入规则拒绝。

只增加资格所需的最小源绑定 context 快照，复用活动/Prepared 和候选预留证据，不建独立 identity 表、注册库或缓存服务。这是当前记录格式的必要调整。实施时升级对应持久格式版本并严格拒绝缺 identity 的旧活动；不加别名、双读、迁移或从旧 execution 临时重建 context。既有安装留在原构建处理，新构建用新隔离 home 验证，实际安装升级另行授权；不能用本计划清空旧数据库或跳过原停止义务。

### 4.3 Web 配置编辑

保留现有 execution-config 路由及 read/discover 语义，新增受浏览器认证及 CSRF 保护的逻辑构造入口 `POST /api/web/execution-config/compose`。请求为 saved_digest、可选已保存 base_profile_id、完整目标 workflow、selection、unset；浏览器不传 base_execution、Runner pin、程序字段或 identity。服务端从同一摘要的可信配置取 base；没有 base 时只允许公开且满足部署要求的默认构造。目标组合必须同时满足 SteerLine 明确产品范围与 Runner 当前有限能力目录，目录发现不扩张执行资格。

入口返回构造的 profile execution 与逻辑 view，不写 execution.json、不发起运行。前端草稿仅编辑逻辑值；原生 settings 视为不透明返回物，停止使用 changeModel 等 adapter 分支补字段。每次构造核对草稿序号/目标/原配置摘要，晚到结果不能覆盖新选择。审计模型建议仍属 SteerLine 的选择策略，保留当前切换审计 workflow 时以主模型填入初始草稿、用户可以修改并经保存确认的交互；完整 workflow 随请求明确交给 Runner。Runner 不擅自补齐缺失审计参数，不生成提示词或审计策略。本轮不顺带改变这些产品默认值政策。

validate/save 的请求体在同阶段统一改为 `{saved_digest,config,profile_edits}`，不兼容旧的仅完整 Config 请求体；配置文件本身仍保存原 Config 形状，不写 edits。profile_edits 是有限数组，每项 `{profile_id,base_profile_id?,selection,unset}`，目标 workflow 取同一 config 中的该 profile。新增 profile，或同 ID 的 execution/workflow 有变化时，恰好一条 edit；只改名称/默认选择、删除 profile、完全未变的 execution/workflow 不需要 edit。重复、缺失、多余 edit、未知 base 或旧 saved_digest 均拒绝。base 只能引用操作开始时已保存的可信配置，不引用尚未保存的新草稿；不带 base 表示显式从公开默认构造。复制/多次改草稿时始终保留其保存前 base 及累计逻辑选择/清除意图。

validate/save 在同一 saved_digest 的服务器配置上，按每条 edit 重新 compose 并与 config 中提议的 execution 逐规范字节比较，再校验完整配置。save 的 If-Match 继续必需且必须与请求体 saved_digest 相同，落盘前仍复读原文件；validate 也核对 saved_digest。不能只从输出 execution 猜来源或清除意图，不能因为它曾由 compose 返回就跳过重验。未变 profile 按现有严格 schema/权限规则处理，不为名称/默认选择操作重新派生 execution。这里没有签名票据、服务端草稿会话、待保存表或持久授权 token。

程序来源以可信 base/default 和完整 compose 输出比较为准。页面不能任意引入新 executable/args/helper 或其它不在逻辑编辑范围的 settings；退役 Orca 字段由 Runner 当前 schema/gate 明确拒绝。删除 Web 旧 helper 可用性及字段继承 gate，不以新的程序摘要/文件检查接口替换它；当前支持范围取固定 Runner 声明。Runner EXE/可信部署资产的原文件核验、其它顶层部署字段和不可编辑 profile 依原权限及严格配置验证保留，不产生全局/项目 ready。

目录查询继续必须沿用原 profile 的程序上下文。R2 为既有 native-catalog agents/models 请求新增可选 execution 字段；Web 从匹配 saved profile 的服务器配置供给，Runner 内部解析原生字段。agents 仅核对 adapter/target 与 execution 一致，继续拒绝请求中的 agent 字段，不按 execution 的所选 Agent 过滤原生 Agent 目录；models 另核对 agent 与 execution 的公开 view 一致。有 execution 时拒绝另传 executable。无 profile 的纯目标/Agent 目录继续使用原选择器，不借此回退一个失败的 profile 查询。扩展由固定 EXE 的公开 request_schema 声明并验证，缺字段能力只禁用该 profile 查询，不恢复 Web 的 settings 解析。Codex models 在 Runner 内取该 execution 对应的 executable，沿原 metadata 方式 initialize/model/list，不执行业务 args、helper 或 thread/turn。浏览器不能提供 execution/程序路径；Web 也不把原生程序路径加入逻辑 view。保持原 native-catalog 的限额、来源/目标核对、无服务与清理要求，不新增目录投影 CLI。

删除以虚构 model/helper 试探 schema 的组合生成、程序名映射和 helper 字段解码。选择项消费既有 capability_catalog 与新逻辑字段描述；不以 schema 接受任意 Agent 字符串推导支持。Runner 当前已退役 DSH，SteerLine 的旧 allowlist 不能令其重新可执行；产品可继续限制其它公开组合。完整 schema 和生产 gate 最终校验保留。

## 5. 分阶段交付及非目标

| 阶段 | Runner 必须交付 | SteerLine 必须交付 | 前置/退出条件 |
| --- | --- | --- | --- |
| R1 → S1 | describe 的最小 view/identity、derive、同源 capability/schema 与测试 | 环境检测/两类生成及自动材料预检迁移，冻结身份及事务接线，删除 useNativeOrcaExecutor、GenerationExecution、原 Context 字段裁剪 | 固定 EXE 与公开接口正反例通过，NEC-A01–A05/A08/A09 |
| R2 → S2 | compose、逻辑编辑描述、native-catalog 可信 execution 入参 | Web 逻辑 edits 与保存重验、前端草稿、目录交集和原程序上下文；删除编辑/准入原生映射、旧 helper gate 和样例试探 | R1/S1；NEC-A01/A06–A09 |
| 双仓最终联验 | 同一最终构建的接口与原生转换资格 | 真实 EXE 接线、正式浏览器、原请求重启和项目准入 | NEC-A10 按链路分别签收，不以 fixture 代真机 |

每阶段只发布已实现 operations；对应旧转换在切换后删除，缺新能力明确阻断该新操作，不回退旧代码。阶段间仍有实际消费者的代码可保留到对应切换，但不得成为已迁移路径的备用实现。

本轮不做 runnerbridge 生命周期抽取、通用语义客户端重构、app 整体拆包、配置格式大改、通用 JSON patch/DSL、共享 Go SDK、独立配置服务、权限模式扩展或新 Agent 支持。完整项目环境材料、提示词正文、result/progress、原恢复权限保持现行规则。若实施发现必须改正文/接线/对应测试资产，按[提示词治理](../../docs/prompt-governance.md)给出实际前后正文、UTF-8 字节数、业务原因与验证并经独立审核，更新 golden 不代替批准。

## 6. 通过条件

| ID | 自动通过条件 |
| --- | --- |
| NEC-A01 | Runner 无服务公开命令、同源能力/schema、严格输入/退出码/限额、source/摘要与零业务副作用均由固定 EXE 验证。 |
| NEC-A02 | derive 覆盖已声明 execute/1 和 generate/1 的正反例；目标/偏好保持，权限显式且不升权，退役/未支持组合拒绝；原生字段转换只在 Runner。 |
| NEC-A03 | 环境活动原配置/身份与实际执行分开冻结；key/basis 归原业务，封存结果和原 scope 停止后才授予资格，失败/未知不放行。 |
| NEC-A04 | 测试生成、Roadmap 和自动材料生成预检使用同一公开派生结果；read_only、原 schema/材料/正文/额度和结果验收完整；预检与实际请求不分叉、轮询不反复启动配置 CLI。 |
| NEC-A05 | 身份投影与旧接受输入按空偏好容器例外对照；每份源摘要独立校验，模型/effort 改变后源摘要不同但 context/basis 相同仍准入；执行环境差异不合并；并发/伪造身份拒绝；纯模型/事务零外部调用。 |
| NEC-A06 | Web 新建/复制/改模型/切换目标与 workflow 通过逻辑构造；原审计模型草稿建议保留，Runner 不代选；无程序注入，退役显式launch/helper源拒绝，无协议清洗或改投。 |
| NEC-A07 | 零项目可编辑，摘要冲突/草稿晚到隔离；逻辑 edits 完整且保存重建与提议字节相同；原顶层字段保持、显式重启生效；目录沿用自定义 Codex 程序且不执行业务 args。 |
| NEC-A08 | 已冻结请求重启/观察/停止不重新解析配置；缺新 capability 只阻断对应新操作；原 namespace、instance、请求字节、停止与升级排空边界保持。 |
| NEC-A09 | 旧原生字段转换和 schema 试探删除，owner/依赖守卫有效；受影响正式测试、提示词接线对照和独立审核满足治理；文档无坏链接。 |
| NEC-A10 | 同一最终 EXE 的本机 Codex、Orca 本机/远端按实际已声明组合独立联验；生成、环境检测、Web 正式入口及原 Run 收尾各有证据，缺链路不算整体完成。 |

验收具体命令、环境及状态独立保存在[专属规划](../testing/acceptance/native-execution-configuration-boundary.md)，不把本草案或接口夹具写成已具备生产能力。
