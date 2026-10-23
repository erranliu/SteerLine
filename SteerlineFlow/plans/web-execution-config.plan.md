# Web 执行配置管理与能力探测 Plan

日期：2026-10-21。状态：实现覆盖 U1–U10，并按最新确认边界收口 U11；本文维护交付安排，实时验证及人工现场只登记在专属验收。执行范围以 [Spec](../spec/web-execution-config.spec.md) 为准，结果登记于[专属验收](../testing/acceptance/web-execution-config.md)。普通开发，不占 Task ID、不纳入 roadmap，不自动部署安装环境。

## 1. 基线、调查与整合

源码基线 `00eb8c6f89d4fe66bb4da840025094ce711d0422`。已读取 `docs/spec/SpecGuide.md`（同基线；本任务为用户直接授权，不采用其中 roadmap 转正/调度流程）、AGENTS.md、测试架构规范及本仓测试入口。主要调查如下。

| 位置 | 现有事实与处理方式 |
| --- | --- |
| `web/src/features/settings/settings.tsx`、`web/src/app/app.tsx` | 当前项目级只读设置；保留当前已加载方案展示，增加全局管理入口和独立页面，不复建项目管理 |
| `web/server/server.go`、`cmd/steerline-web/main.go` | Web 独立进程，已具认证/同源/CSRF和应用客户端；新增本地处理命名空间，保留产品 API 转发白名单 |
| `internal/app/runner_config.go`、`config_runtime.go` | 格式3、默认项、严格加载、参数摘要与文件冻结；提取必要纯值/校验供两侧消费，不让 Web 导入 app |
| `internal/app/application_catalog.go`、`application_directory.go` 及 applicationcontract | 启动固定全局文件快照；增加只读加载摘要，不增加配置写入/探测用例 |
| `internal/runnerclient/catalog.go`、`internal/runnercontract` | 已有公开自包含 schema 校验与参数摘要；复用纯校验，禁止 Web 调用 submit/service start 或读取 Runner 私有状态 |
| `internal/localservice/endpoint.go`、`web/server/home_config_*_test.go` | 私有文件原子替换与父目录权限保持已有基础；复用而不改变整个 home 权限 |
| `internal/testkit/boundary` | Web/HTTP 消费者有传递依赖守卫；为明确新增的纯配置/元数据职责补窄规则，不能用总开关放开 app、SQL、执行或存储依赖 |
| `docs/product-design/worker-lifecycle.md` | 已读取；本次不改变 W/E/V 状态机，现有换代/冻结/停止行为直接复用，不另写恢复规则 |

保留唯一 `execution.json`、profile 参数摘要和原 Run 冻结记录。新增模块是 Web 能力目录客户端及配置编辑 owner，不是新的执行链路。无旧格式兼容、迁移、双写、探测数据库或独立守护进程。

### 本机 Orca 增量调查与边界结论

2026-10-21 在同一源码基线与当前 dirty 工作树核对：Runner `C:/Project/SteerRunner/bin/steerrunner.exe` 为 `0.2.0-dev.3-r2`，SHA256 `4dbf25c74ecc5d360fbfacc88fe599744cdc80a10648962ddc86f73c7d4b467b`，resource digest `a48bfb1aa7250009b21b4170532d877be01c23375ac386cb39e269794263ef91`。仅调用既有 `capabilities --offline --json` 并读取发布材料，没有启动/连接业务服务。

公开 schema 已能表达 `orca-terminal + local`、`agent=codex|pi|dsh`、本机 runtime helper 路径/SHA、各 Agent 的 effort 分支及 workflow 条件；不是只凭 Agent 注册推定支持。当前构建的 development/development-audited 是版本2，审计的 Orca 分支要求本机 Codex及runtime字段，Pi/DSH不得照搬。后续选用构建仍重新读取其 schema，不把本次版本号或枚举写死。

Orca CLI 为 `C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`；已加载其版本匹配 `orca-cli` 指南并查公开 help。安装包 `out/cli/runtime/metadata.js`、`transport.js` 及 `shared/runtime-bootstrap` 表明本机走 `orca-runtime.json` 的 named pipe、authToken、换行 JSON envelope；Runner 现有本机连接规格和实现也采用这一原生通路。它们只作协议调查，正式 Web 不导入安装包 JS 或 Runner 私有包；该初步调查未向真实本机 Orca发送目录查询；后续实现的接线结果另见验收，不将静态调查当通过。

结论：按 Spec 的窄 IPC 客户端即可保持原边界，不需要 Runner 新探测接口或执行服务转发。当前所选 Runner 的 bin 只含 EXE及build元数据；发布文档 `docs/testing.md` 提供正式 helper 构建命令 `go build -o <安装路径>/steerrunner-agent.exe ./cmd/steerrunner-agent`。部署材料按 EXE 的 build.json 固定提交 `8261310d5f2d1edb6fa0c81ba05fa4119b561386` 隔离归档后构建，记录来源/归档SHA/命令/Go版本及helper SHA；不使用当前更高版本HEAD、不改Runner源码。此步骤由部署维护者在新debug包材料区完成，不交给Web或Go执行服务运行时。helper来源及缺失处理见U8/U10。若实施触发 Spec 的停止条件，停在该具体前置并向用户报告，不私自扩大 owner 权限、添加执行探针或降低验收。

## 2. 实施步骤

### U1：固定支持矩阵、公开配置契约与纯配置边界

1. 按实际选用构建记录 Runner EXE/version/SHA/resource digest 和公开配置 schema 来源；使用既有 `version` 与 `capabilities --offline --json`，不传 state-dir、不启动/连接常驻服务、不新增 Runner 接口。Web 直接编译返回的自包含 schema，验证前后 EXE SHA和元数据身份一致；缓存绑定该发布身份，不把测试 fixture 当生产契约。禁止普通 resident `capabilities`、外部schema读取或复制一套硬编码schema；固定EXE缺离线入口时明确不支持。
2. 原首版矩阵为local/codex-app-server与orca_environment/orca-terminal/Codex；本机Orca按后续U7–U10扩展。核对稳定 environment ID 可直接经当前 Runner 契约消费；若现有调用只接受名称，先修正调用方公开参数传递并定向验证，不把短命 runtime ID 当目标、不新增 Runner 接口。其他 Agent 按各目标的明确映射过滤。
3. 从 `internal/app/runner_config.go` 抽取双方需要的纯配置结构/严格解析/基本校验，例如 `internal/executionconfig`；该包不得做文件、进程、网络或业务存储操作。保留现有参数摘要语义，不把格式排版或名称并入参数摘要，不展开缺省值。现有 app 引用改为消费纯值，不留第二份配置定义。
4. Web 后端使用纯 schema 编译/校验能力；必要时从 runnerclient 提取纯部分，避免导入有执行端口的实现。前端根据目录及配置 DTO 编辑字段映射，Web 后端以支持矩阵和公开 schema 统一裁决生成配置；前端不复制 Runner schema。
5. 扩展依赖守卫：覆盖 `web/server` 所有子包、其他 OS/构建标签及传递依赖；执行服务不得导入 Web discovery，Web 管理包不得导入 app/taskrunner/runnerbridge/SQLite 或执行 adapter。允许明确的纯配置/公开契约和基础进程/传输设施。补反例夹具，不扩大所有 HTTP 消费者权限。

### U2：Web 直接目录探测

1. 新增 `web/server/discovery`，分别实现 Codex 与 Orca 的窄客户端，提供结构化 environments/agents/models 查询。复用已有无窗口进程设施及路径保护；所有进程归 Web owner，禁止借用业务 agent。
2. Codex 实现握手、分页、原始 ID/effort/default 解析、1 MiB 总预算、游标循环和 malformed 检查。全路径禁止 thread/start、turn/start 和配置写入；实现 30 秒总期限与 5 秒精确清理，子进程/句柄退出也须核实。
3. Orca 先只读 Orca 固定环境登记文件，按既有配对协议发现环境，再通过最小 RPC 传输进行认证/能力协商及目录读取；所需协议代码由 Web 维护，不能加载安装包内部 JS。用版本匹配契约固定报文，覆盖客户端 capability 缺失拒绝、strict 参数、savedOnly、listingInProgress 后 waitForListing、unsupported、断网及来源解析。
4. 查询范围不能表达 executable/provider/账号/配置覆盖时产生 context_unverified，绝不混用本机与远端目录。实现有界内存缓存、同上下文合并、最多两个并发、取消和晚到响应保护；不引入持久 job 系统。
5. 提供测试用公开协议子进程/远端 RPC 夹具，记录方法白名单及所有调用，验证零业务输入与资源清理。这些只测试 Web 新 owner，不在调用方重建 Runner 执行状态机。

### U3：Web 本地 API 与配置保存

1. 新增 `web/server/executionconfig` owner，对固定 home 读/生成/校验/条件保存；不提供任意路径或命令能力。`/api/web/execution-config` 与现有应用转发路由分开处理，共用认证、Origin、Host、CSRF和请求预算。保留浏览器禁止整体停服及原生凭据隔离。
2. 实现 Spec API，validate/PUT 共用生成与校验；部署 pin 只读，未编辑高级字段 JSON 值保留，模型/effort 手动值明确未验证但 schema 合法，远端不接受本机 helper/runtime 字段。
3. `GET` 返回原字节摘要，`PUT` 必须 If-Match；锁内复核，序列化 Web 写入，使用同目录私有临时文件同步/原子替换/发布后确认。分别测试发布前失败、发布后回执丢失、冲突、无改动和权限保持；冲突不覆盖旧文件，不自动备份/迁移。
4. 识别启动冻结调试包并拒写，不能只在前端禁用按钮。缺失/坏文件、契约不匹配返回诊断，不重置或修复。明确本功能不支持与不遵守协调约定的外部工具同时编辑。

### U4：加载摘要投影与生效状态

1. 在 `applicationcontract.Catalog` 增加 loaded_execution_digest，初始化时从已固定的全局字节快照计算，读取失败时保持缺失及已有错误。只读目录查询原样投影，不读盘重算，不写文件，不发原生查询。
2. Web 按 home/当前宿主实例比较保存与加载摘要，显示已加载/待重启/未知；项目 ready 独立展示。保存、刷新、Web 重启均不调用应用 restart 或 Runner start。
3. 保留执行服务既有完整配置一致性门槛；在现有新候选、旧 Run 读取/停止测试中补参数快照断言。更名、改默认或改另一预设也会触发整文件待重启；不能用参数摘要相同跳过文件冻结。
4. 页面仅说明用既有正式 restart 或 stop/start 生效，不新增浏览器整体重启入口。更新项目新启动按钮的待重启提示；后端准入依旧是最终裁决，不能只靠前端禁用。

### U5：全局管理界面

1. 新建 `web/src/features/execution-configs` 及对应 API/Query keys（home + Web/宿主身份 + 查询上下文）；把全局路由接在项目选择之外，零项目及项目失败可进入。旧 settings 保留已加载方案和环境检查入口，并链接全局管理页。
2. 实现列表/编辑器、CRUD/复制/默认项约束、主字段联动、模型/effort 原始值、审计及高级值完整保留。已有 executable/args/认证引用只读，不新增任意命令输入。
3. 实现探测与缓存来源、手动未验证原始值、错误/忙碌/超时、取消和晚到响应保护。草稿切换/保存冲突不丢输入；保存成功定向刷新磁盘目录与当前加载投影，丢回执先 GET 对账。
4. 按已保存与已加载两种状态更新当前项目提示，不将磁盘新目录直接替换项目已加载列表。保持中文名称、键盘、窄屏和无项目状态可用。

### U6：最小验证能力、文档和交付

1. 新增下述定向 Go/Vitest/HTTP 用例与 Playwright 跨进程受控流程；将新 owner 的关键 Go 用例登记到固定 L1 的明确分组，保留全包/跨标签闭包核对。不得通过 run pattern 零匹配声明通过。
2. 新增 L3 `configuration.execution-profiles`（suite=configuration，env=controlled），映射 `tests/e2e/configuration/execution_profiles_test.go` 的 `TestExecutionProfiles`，包括真实 Web/应用进程、配置文件、原生 CLI、公开 Runner fixture 和浏览器。由其驱动 `web/tests/execution-configs.spec.ts`（`npm --prefix web run test:e2e -- execution-configs.spec.ts`），通过场景专属环境参数传入现场地址/证据位置，不硬编码安装地址。为它落实独立临时 home/repo/端口/输出目录及正常停启驱动，补 suites.json、README、SteerlineFlow/testing/test-list.json 和选择/证据校验；不消费其他场景产物，可并行。
3. 新增 `scripts/test-execution-profile-discovery.ps1` 原生目录联验驱动和 `web/tests/execution-configs-live.spec.ts`：先生成新 debug 包，启动正式 Web 查询路径，仅探测本机 Codex和已配对远端 Orca，输出结构化无凭据证据及资源结算。调试包只读，不在已启动包里保存/换方案，不发模型任务/真实 PR。该专项不登记为 L2 或 Orca 执行链。
4. 同步架构总览/源码索引、执行配置产品设计、Web 指南和测试入口，明确本次新增 Web 文件管理职责及已验/待验边界。依据现有生命周期文档复核没有修改 W/E/V 矩阵；若实施发现需要改变，先回到其唯一 owner 说明差异，不能在本任务偷偷加恢复机制。
5. 执行最小验证并在专属验收登记源码版本、实际选择、结果/失败、证据与清理；人工项单列。按下方测试计划执行功能验收，不部署安装环境。

U1 → U2/U3 → U4/U5 → U6 为实现依赖关系，不是 roadmap DAG，不创建执行聊天或独立 worktree。

### U7：Web 本机 Orca 目录客户端与显式通路选择

1. 在 `web/server/discovery` 增加窄本机 IPC transport（Windows named pipe；可使用已存在的基础依赖go-winio），与现有远端连接共享目录解析和固定方法白名单。选择方式由host+adapter决定，不导入Runner协议包，不增加通用可注册传输框架。其他平台明确unsupported。
2. 同步 `Request`、Web路由校验、前端API类型和所有调用/测试：models/agents显式提供adapter，local+codex-app-server、本机local+orca-terminal和远端orca_environment+orca-terminal分开；拒绝冲突environment/adapter和浏览器endpoint/path/token输入。不留缺adapter的歧义兼容分支，不增加Go执行服务路由或新的持久target种类。
3. 只读固定user-data的orca-runtime.json，限制路径、文件类型、metadata/响应字节及期限。按现有envelope带authToken和目录查询客户端能力，核对status、metadata和响应runtime身份；全部错误脱敏。keepalive不续总期限。连接关闭/取消归Web，禁止自动启动Orca、helper、worktree/terminal/agentSession或模型业务。
4. 将adapter、通道、user-data、runtime/认证身份摘要、Agent加入缓存/合并上下文；runtime变化使原查询及缓存不可复用。本机查询不使用远端配对或隐式环境变量，失败不改投Codex直连/远端。保留来源、部分目录/未知状态、30秒/1MiB/5秒清理及并发限制。

### U8：Web 支持矩阵、部署模板及配置映射

1. 扩展 `web/server/executionconfig` 的Supported、programsUnchanged、schema编译/投影及配置DTO，按Spec矩阵开放本机Codex/Pi/DSH。模型目录不可用与产品无映射分开；手动原始值仍通过完整run_configuration schema，远端范围不随本机扩容。
2. 从公开组合schema投影各Agent的effort/workflow约束，不再统一读取Codex effort枚举；无法可靠投影的组合标明原因并禁止生成，不能用通用agent字符串或workflow.supported_adapters列表绕过完整schema条件。增加本机Orca审计Codex正例、Pi/DSH审计反例，以及远端runtime字段（含空值）反例。
3. 本机Orca模板必须携带已有受信helper绝对路径和SHA；新增/改变执行参数时只从已保存本机模板继承，按固定摘要核对文件身份，不启动helper探测。未改动execution/workflow的既有profile可保留，仅改名称/默认及其他链路编辑不被材料暂缺阻断；缺部署材料是组合级诊断，不将整个管理页改为只读。新Agent的native executable仅能采用已适配默认程序或同Agent已有配置，既有args/认证引用/提示词等按JSON值保留。切到远端移除runtime字段，不能带路径/SHA空值占位；换回本机从受信模板重新选择，不从远端草稿推导helper。
4. 落实部署前置：部署维护者提供与所选Runner发布材料对应的正式helper；可以按上节固定发布提交和正式文档命令准备可追溯材料，不以任意工作树编译产物或SteerLine debug-helper冒充。沿用唯一生成器现有 `-Orca -OrcaEnvironment '' -OrcaRuntimeExecutable '<绝对路径>' -OrcaRuntimeSHA256 '<SHA>' -OrcaAgent codex|pi|dsh` 参数，在启动前生成含本机Orca模板的execution.json；生成器先校验文件与SHA，再复制到新包bin并固化路径；可用 `-OrcaRuntimePublication` 校验来源记录与Runner发布身份。debug prepare 同时按 adapter 和 target 区分本机 Codex 与本机 Orca，不再向后者注入顶层 Codex executable；必要的模板/材料存在性校验在该生成流程补齐，不能新建平行生成脚本、模板数据库或Web安装向导。普通部署同样在启动前供给模板；缺材料时Web返回可说明的部署阻断，不能猜邻接路径、下载或扩权。
5. 保持格式3、If-Match/原子替换、部署pin只读、无改动不重写、冻结debug包拒写、显式重启和旧候选冻结语义。执行服务只消费已有公开字段；不新增原生能力注册、探测回执门禁或worker状态。

### U9：运行机/链路独立的页面联动

1. 扩展 `ExecutionConfigs`：本机显示Codex直连/Orca+Agent选择，远端保持Orca固定。查询与字段选择使用Web结构化支持矩阵，Agent显示映射支持与目录状态，未注册/无目录不伪装成已探测；未知原始模型仍使用显式手动入口。
2. 切换机器/链路/Agent先取消旧订阅并清理不适用字段，重新约束模型/effort、权限、workflow及审计参数；不能因同为local覆盖或复用另一通路目录。保留现有名称、草稿切换保护、未知值、复制/默认/删除约束和丢回执GET对账。
3. 展示helper材料已就绪/缺失及只读来源，不允许浏览器填写任意helper/原生程序路径。新增本机配置缺部署模板时显示可处理的原因；配置读取、其他链路及既有配置保留不受阻。生效状态仍比较全文件保存/加载摘要，不增加自动重启入口。

### U10：最小验证与交付

1. 增加下表A10–A12的定向L1、IPC夹具、UI测试和依赖守卫反例。测试方法清单及零业务调用，覆盖本机token脱敏、认证/版本/runtime变化、超限/取消与连接清理、远端环境变量污染、三通路缓存及迟到响应隔离；不复制Runner执行/恢复矩阵。
2. 参数化/扩展既有 `configuration.execution-profiles` 的同一保存→正式停启→关闭Web→CLI消费流程，在自有controlled现场提供本机Orca受信helper模板，分别核对Codex/Pi/DSH的冻结提交字段。复用公开Runner fixture，不启动真正helper或Agent；新增参数映射的组合主要由L1覆盖，不另注册三份同构L3。实施时同步README、展示清单及场景断言说明和选择校验，不把登记文本当通过。
3. 扩展现有 `test-execution-profile-discovery.ps1` 和 `execution-configs-live.spec.ts`，新增明确的 `-LocalOrca` 选择。脚本先生成全新debug包、等待项目就绪，再经正式Web接口只查询本机Orca；与远端专项分支明确互斥，去除该分支对OrcaEnvironment的必填要求，不隐式改投。优先以Codex目录为真实正例，Pi/DSH目录可用性如实记录；未提供目录不得冒充真实模型查询成功。目录专项本身不需要/不启动执行helper，可用默认本机Codex执行模板承载Web宿主；它不证明本机Orca业务执行已验收。
4. 对A13的真实执行消费只核对调用方公开提交，原生业务执行资格仍由Runner原owner验证；引用其已有本机执行规格/证据，不把历史构建通过继承给本次选定发布物。若部署材料或原生IPC前置缺失，登记具体阻断；若必须改变既定边界才能补齐，则停止报告，不继续替代实现。
5. 在专属验收登记A10–A14的已验/未验/阻断及证据；既有A01–A09原记录保持其原范围。M02人工本机链路体验可后置；不修改当前仍运行的冻结验收包，实施后另建新现场。U7/U8 → U9 → U10为本机增量依赖，不是roadmap DAG。

### U11：外部 provider 配置所有权与统一 Orca 目录

1. 将 `discovery` 中的Orca目录查询与配置支持规则分开：统一RPC读取层删除local布尔策略和Codex硬编码，远端传递请求Agent；本机/远端使用相同注册核验、目录解析和Pi provider/model投影。只让连接/认证及目标身份留在各传输实现，不增加协议框架、独立provider发现或Runner探测接口。
2. 查询API按原生Agent身份及目标注册列表处理，返回Agent目录事实；生成配置继续由 `executionconfig.Supported` 与完整公开schema裁决。保持远端Pi/DSH配置拒绝，防止目录成功扩大执行能力。去除目录DTO中的产品supported/reason字段，由页面结合GET组合说明可编辑范围；未发布协议不保留旧DTO兼容。
3. 页面从Pi目录provider投影筛选模型，保持完整原始model selector；切provider清理模型/effort，手动原始值保持未验证。提示用户在目标Orca/Agent配置provider凭据，不提供API key/base URL/账号管理，不推断目录等于凭据或模型已验证。原生默认上下文继续标为未核实，不新增多账号身份选择。
4. 扩展既有 `TestOrcaCapabilityWire`、`TestLocalOrcaCatalog`、配置/路由测试和组件/API测试，覆盖两种传输同一Pi selector、远端注册Pi目录成功与生成配置拒绝、密钥字段不透出/不接受、Provider筛选原样保存。不增加同构L3场景；主干集成后定向复验原保存/重启/关闭Web消费场景及受影响应用读取边界。
5. 同步Spec、产品设计、架构/README及验收导航，修正“本机Orca待实施”的过期文字。实现合入最新主干后保留双方测试登记及owner守卫；交付PR，结果和未验范围只登记专属验收。当前冻结人工验收包不修改或重启。

## 3. 测试计划

采用[统一测试架构](../../SteerlineProjectGuideDoc/testing-architecture.md)、[测试入口](../testing.md)和[E2E 扩展约定](../../tests/e2e/README.md)。原范围入口已提供；本机增量的测试/脚本选项随U7–U10提供；是否通过以专属验收的实际证据为准。实际执行结果登记于专属验收，入口或环境缺失不能跳过计通过。

原始日志写新建独立临时目录，最小结果/实际路径登记到[专属验收](../testing/acceptance/web-execution-config.md#结果登记)。命令均在仓库根运行，标注 web 工作目录的除外；各命令的实际源码及环境需一并记录。

| 验收项链接 | 级别与组/场景 | 环境及命令 | 时点、通过条件和证据 |
| --- | --- | --- | --- |
| [WEC-A01](../testing/acceptance/web-execution-config.md#wec-a01) | L1 / quick-execution-boundary，Web 本地 API 认证 | 隔离夹具；`go test ./internal/testkit/boundary -run '^(TestWebExecutionConfigOwnership|TestTM04ExecutionBoundary)$' -count=1`；`go test ./web/server -run '^TestWebExecutionConfigRoutes$' -count=1` | U1/U3后；包括新反例、子包/跨标签闭包及未授权零调用；记录 boundary/API 日志 |
| [WEC-A02](../testing/acceptance/web-execution-config.md#wec-a02)、[WEC-A06](../testing/acceptance/web-execution-config.md#wec-a06) | L1 / quick-rules 新纯包、quick-owner-web-config | 临时 home/文件；`go test ./internal/executionconfig ./web/server/executionconfig -count=1` | U1/U3后；合法/非法交集、往返不丢字段、手动值、条件写/失败窗口/权限/调试冻结全部断言通过；config 日志 |
| [WEC-A03](../testing/acceptance/web-execution-config.md#wec-a03)、[WEC-A04](../testing/acceptance/web-execution-config.md#wec-a04) | L1 / quick-owner-web-discovery | 受控原生子进程/RPC；`go test ./web/server/discovery -count=1` | U2后；方法计数零业务、分页/协商/来源/上下文/取消/清理；discovery 日志与方法清单 |
| [WEC-A05](../testing/acceptance/web-execution-config.md#wec-a05) | L1 / Web Vitest 定向组件与查询 | web 工作目录；`npm test -- src/features/execution-configs/execution-configs.test.tsx src/shared/api/execution-configs.test.ts`；`npm run build` | U5后；覆盖 CRUD、零项目、晚到结果、离线手动值、冲突草稿及待生效；UI 日志。build仅记编译，不替代行为 |
| [WEC-A07](../testing/acceptance/web-execution-config.md#wec-a07) | L1 / quick-owner-unified-startup + 新配置定向 | 原公开 Runner fixture；`go test ./internal/app -run '^(TestWebExecutionConfigLoadedSnapshot|TestRunnerPresetFreeze)$' -count=1` | U4后；读取零探测，保存未热加载，原新候选门槛/旧快照保持；loaded-snapshot 日志 |
| [WEC-A08](../testing/acceptance/web-execution-config.md#wec-a08) | L3 / configuration.execution-profiles | 自有隔离夹具；`go run ./cmd/test-feedback -level=L3 -suite=configuration -scenario=configuration.execution-profiles -env=controlled` | U6完成登记后；浏览器→真实保存→正式停启→关闭Web/清缓存→CLI消费新配置，原记录不变，清理完整；summary及scenario.json/浏览器trace |
| [WEC-A09](../testing/acceptance/web-execution-config.md#wec-a09) | 原生目录专项 / native.discovery（不计完整L2/L3） | `./scripts/test-execution-profile-discovery.ps1 -SteerRunnerExe '<固定EXE绝对路径>' -OrcaEnvironment '<已有environment ID>'` | U6驱动就绪后；在C:/SteerlineDebug新包经正式Web接口取得两类真实目录和来源，owned资源结算；discovery.json、版本/方法/清理及浏览器证据 |
| [WEC-M01](../testing/acceptance/web-execution-config.md#wec-m01) | 人工后置 | 新受控现场；具体参与窗口登记 acceptance | 自动项完成后；列表/编辑器、中文、键盘/窄屏、错误和待重启体验，无业务副作用 |

固定 L1 的新增分组必须在 U6 同步 `cmd/test-feedback/scoped.go` 的包/测试归属和覆盖校验，完整固定 L1 使用 `go run ./cmd/test-feedback -level=L1`。定向子集只报告实际分组，不声称整体 L1 通过。Web新子包在边界守卫纳入闭包，不能因分组枚举未收录漏测。

### 本机 Orca 增量测试计划

所有结果仍登记同一专属验收；下列命令在U7–U10实现后执行，禁止零匹配通过。新测试沿现有固定L1 owner分组登记，不运行整套Full。

| 验收项 | 级别/组或场景、环境 | 命令/能力补齐 | 时点、通过条件、证据位置 |
| --- | --- | --- | --- |
| [WEC-A10](../testing/acceptance/web-execution-config.md#wec-a10) | L1 / quick-owner-web-discovery、quick-execution-boundary；本机IPC受控夹具 | 新增 `TestLocalOrcaCatalog`，扩展共享缓存/ownership用例；`go test ./web/server/discovery ./internal/testkit/boundary -run '^(TestLocalOrcaCatalog|TestDiscoverySharingCancellationAndCache|TestWebExecutionConfigOwnership|TestTM04ExecutionBoundary)$' -count=1` | U7后；认证/协商/runtime/预算/取消、通路隔离、零业务方法及无执行owner反向依赖全部通过；新增测试须在scoped.go登记，日志放独立输出目录并链接A10 |
| [WEC-A11](../testing/acceptance/web-execution-config.md#wec-a11) | L1 / quick-owner-web-config、quick-owner-web；临时配置/公开schema与受信helper文件fixture | 新增 `TestLocalOrcaConfiguration`，扩展 `TestWebExecutionConfigRoutes`；`go test ./web/server/executionconfig ./web/server -run '^(TestLocalOrcaConfiguration|TestWebExecutionConfigRoutes)$' -count=1` | U8后；三Agent映射、审计/effort分支、缺helper/错SHA/远端字段拒绝、条件保存及高级值保留；登记配置/API日志及原子写断言，不执行fixture helper |
| [WEC-A12](../testing/acceptance/web-execution-config.md#wec-a12) | L1 / 定向Vitest；浏览器组件夹具 | 扩展既有配置组件/API测试；`npm --prefix web test -- src/features/execution-configs/execution-configs.test.tsx src/shared/api/execution-configs.test.ts`；`npm --prefix web run build` | U9后；链路选择、Agent/effort/workflow/审计清理、缺部署材料提示及迟到响应/草稿断言通过；UI日志与构建日志登记A12，构建不代替交互 |
| [WEC-A13](../testing/acceptance/web-execution-config.md#wec-a13) | L3 / 既有configuration.execution-profiles；独立controlled SteerlineTest现场 | 扩展当前Go场景/Playwright参数化及fixture模板；沿用 `go run ./cmd/test-feedback -level=L3 -suite=configuration -scenario=configuration.execution-profiles -env=controlled` | U10补齐后；Web保存→旧宿主门槛→正式stop/start→关闭Web→CLI提交，核对local/orca-terminal及Agent/model/effort/runtime路径/SHA，旧候选原字段保持、owned资源清理完整；summary/scenario/浏览器trace登记A13 |
| [WEC-A14](../testing/acceptance/web-execution-config.md#wec-a14) | 原生目录专项；新C:/SteerlineDebug包、已有本机Orca；不计完整L2/L3业务执行 | `./scripts/test-execution-profile-discovery.ps1 -SteerRunnerExe '<固定EXE>' -LocalOrca` | U10后；固定Orca版本/metadata通路及身份，真实Codex目录/effort/来源、token不落证据，Web连接/本包服务清理完整，共享Orca未被停止；discovery.json/cleanup/result及版本摘要登记A14。Orca不可用明确阻断，不自动启动它或借远端通过 |
| [WEC-M02](../testing/acceptance/web-execution-config.md#wec-m02) | 人工后置；新受控可写fixture或另建冻结debug只读现场 | 参与窗口由用户后续安排；冻结包不测试保存 | 自动项后体验本机双链路、三Agent、目录缺失/手动值/helper缺失、窄屏及待重启；不阻塞无关开发，真实配置损坏/身份混用转为必须修复 |

### 统一目录边界的最小测试计划

| 验收项 | 级别/组、环境和命令 | 时点、通过条件和证据 |
| --- | --- | --- |
| [WEC-A15](../testing/acceptance/web-execution-config.md#wec-a15) | L1 / quick-owner-web-discovery、quick-owner-web-config、quick-owner-web及quick-execution-boundary；`go test ./web/server/... ./internal/testkit/boundary -run 'TestOrcaCapabilityWire|TestLocalOrcaCatalog|TestLocalOrcaConfiguration|TestWebExecutionConfigRoutes|TestWebExecutionConfigOwnership|TestTM04ExecutionBoundary' -count=1` | U11后；受控named pipe/配对WebSocket均保留相同Pi selector/provider；目录无产品策略、未知注册不伪造；远端可查询但不生成不支持执行；凭据字段拒绝及不出结果，方法白名单与owner依赖成立。日志登记A15 |
| [WEC-A16](../testing/acceptance/web-execution-config.md#wec-a16) | L1 / 定向Vitest；`npm --prefix web test -- --run src/features/execution-configs/execution-configs.test.tsx src/shared/api/execution-configs.test.ts src/app/app.test.tsx`；`npm --prefix web run build` | U11后；Pi Provider筛选、切换清字段、完整selector保存、远端发现Pi仍禁生成、凭据说明及原组件用例通过；UI/构建日志登记A16 |
| [WEC-A08](../testing/acceptance/web-execution-config.md#wec-a08)、[WEC-A13](../testing/acceptance/web-execution-config.md#wec-a13) 主干集成复验 | 既有L3 / configuration.execution-profiles，controlled SteerlineTest；`./scripts/test-runtime-isolation.ps1 -Level L3 -Suite configuration -Scenario configuration.execution-profiles -OutDir '<新临时目录>'` | 合入当前主干并构建Web后；保存/旧宿主门槛/正式重启/关闭Web后CLI消费及清理不退化；仍使用自有离线契约和空Orca userData，报告登记专属验收 |

这次调整不执行真实远端Pi/DSH任务，也不以目录fixture证明它们执行就绪。统一协议/字段行为由两种传输的受控测试覆盖，先前真实目录证据只保留其当时版本和范围；不新增API key探针或重复真实模型/PR验收。人工Provider筛选体验沿M02后置。

本次不新增L2/L4，不重跑Runner本机模型/PR/原生停止矩阵；需要补的独有风险是Web的本机IPC与既有配置交接，分别由上述L1、参数化L3和真实目录专项覆盖。若发现新的真实执行缺陷，按其owner另行报告，不以扩大本机目录owner解决。

L2 不选：没有修改模型业务执行/PR/Delivery 核心闭环，原生调用只是能力目录，重复完整真实模型交付无必要。L3 只新增配置保存/重启/消费这一跨进程流程，不重复既有 service-lifecycle；它的独有风险是Web作为文件写者与独立消费者的交接。L4 不选：冲突、协议错误、清理和权限边界先由 L1 隔离验证，无需另建系统级矩阵。不跑 Full，不扩展 Runner 原生恢复/重试专项。

## 4. 环境前置及验收交付

原生专项要求已有可用 Codex、已登录/已配对 Orca 远端及固定 Runner 发布材料；只使用原生已有认证，不复制安装配置/数据库/凭据。缺任一前置明确阻断 WEC-A09，不切换模拟宣称通过。新脚本内部必须先调用唯一 `scripts/new-debug-release.ps1` 创建全新包；默认 live，仅查询目录，无 thread/turn、PR 或任务执行；结束核验 Web owned 进程、连接及本包服务退出，Orca 用户资源不受影响。

WEC-A08 使用其自身 controlled 隔离夹具，允许配置写入及正式停启；不得使用已经冻结的 debug 包来做配置保存闭环。测试项目统一 SteerlineTest，不接触 `C:/SteerlineInstall` 或 `C:/SteerlineRuntime`。若自动驱动或真实接口能力不足，作为实现缺口修复或明确前置，不能只登记“以后人工验”。

交付包括 Spec/Plan 对齐、实现及所选自动项的真实证据，未验项明确列出；M01 的人工体验可后置，不阻断无关后续开发。Plan 不维护实时结果，不因文档已经写好而把功能标记完成。
