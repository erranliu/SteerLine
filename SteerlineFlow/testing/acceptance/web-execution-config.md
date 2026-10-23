# Web 执行配置管理与能力探测验收

日期：2026-10-21。归属：仅对应 [Spec](../../spec/web-execution-config.spec.md) / [Plan](../../plans/web-execution-config.plan.md)，普通开发，无 Task ID。原本机Codex/远端Orca范围已按确认原型实施，定向自动测试、受控L3及真实目录专项通过；人工产品体验签收待安排。本机 Orca 增量已实现，A10–A14 的独立验证及新人工现场见末节，M02 待用户签收；不继承原通过记录。未部署或使用安装环境。

## 范围与记录规则

Web 前后端独立负责能力查询、配置生成和文件管理；Go 执行服务消费配置并投影原加载摘要；Runner 不新增探测接口。目录成功不代表模型调用权限、工作区就绪或任务可执行。

会话探索曾经通过本机 Codex `model/list` 和远端“台式机64G”的 Orca RPC 取得模型/effort目录；远端需要声明结构化查询客户端能力，首次可返回 listingInProgress。探索中使用的安装包内部 JS 只用于接口研究，正式实现禁止依赖；这些结果无本功能正式 Web 路径/固定源码/清理报告，不计 WEC-A03/A04/A09 通过。

每次执行记录源码 commit/dirty、Web版本、Runner EXE/version/SHA/resource、实际选择/退出码、环境与输出目录。原生项另记录 Codex/Orca版本、稳定目标身份、客户端/服务端协议能力、来源/时间/上下文确认范围、调用方法及owned资源结算；不存 token、配对码或认证正文。受控协议通过不冒充真实目录通过。

原始输出使用各驱动新建的独立临时目录，仓库只存最小摘要和必要精选证据。具体命令及分组映射见 Plan“测试计划”；以下小节是稳定验收链接，不因尚未执行而删除。

## 自动验收

### WEC-A01

已完成并通过下方登记的最小自动验证。依赖守卫验证 Web 子包及传递依赖、跨标签反例；执行服务不能导入 discovery，Web不能引入业务/SQL/Runner执行入口。正式 Web API 的 cookie/CSRF/Host/Origin/路径和方法白名单必须覆盖未授权零原生调用、零文件写入；Runner 无新增探测接口。

### WEC-A02

已完成并通过下方登记的原范围最小自动验证。用纯配置/生成夹具核对本机和远端 Codex、未支持 Agent、原范围本机 Orca拒绝、各模型独立effort、默认预填/明确值、手动未验证值、审计workflow及高级字段JSON值保留；产品范围和公开schema同时满足，不能用目录扩展产品能力。本机Orca拒绝属于当时的范围，不能作为新增映射的通过证据，后续由A11补正反例。

### WEC-A03

已完成并通过下方登记的最小自动验证。受控 Codex进程记录握手和所有方法；完整分页/游标环/超限/非法/超时/取消/断开均有断言，零 thread/start、turn/start 和配置写入；不杀兄弟进程，所有owned进程/句柄在预算内结算，Windows不闪控制台。

### WEC-A04

已完成并通过下方登记的最小自动验证。受控 Orca端点验证配对/客户端协商、严格参数门槛、agents/modelCatalog、in_progress→目录、seed/unknown来源及断网错误。缓存不跨目标/配对/Agent/账号上下文；晚到结果不污染新选择；零terminal/worktree/session创建及输入。未知原生上下文不能标记精确现场已核实。

### WEC-A05

已完成并通过下方登记的最小自动验证。组件和Web API消费者测试覆盖全局无项目入口、列表CRUD/复制/默认、字段联动、下游无效选择、离线保留/手动值、错误与缓存标识、冲突保留草稿、保存丢回执先GET及保存/加载/未知状态。项目页面不把磁盘新目录当服务已加载值。

### WEC-A06

已完成并通过下方登记的最小自动验证。临时文件集成测试覆盖If-Match缺失/过期、多标签并发、发布前失败保留旧文件、发布后结果未核实、无改动零重写、默认删除约束、未编辑高级值、部署pin只读、文件/父目录权限与路径身份。冻结debug包由后端拒写；不调用任何应用停启/执行命令。

### WEC-A07

已完成并通过下方登记的最小自动验证。应用fixture固定原加载文件字节；修改磁盘后查询仍返回原digest，读取零探测。原门槛拒绝新候选，原Run/候选参数、名称、输入/实例绑定与原观察/停止目标不变；不自动重启或改投。重启Web/浏览器不重载执行配置；摘要缺失/后端失联显示未知。

### WEC-A08

已完成并通过下方登记的最小自动验证。新增L3 `configuration.execution-profiles` 自有controlled home、SteerlineTest仓库、端口、浏览器及公开Runner fixture。步骤：建立旧配置及原候选事实→浏览器创建/编辑/设默认并保存→确认待重启及旧宿主阻断新候选→正式stop/start→确认新宿主加载新digest→关闭Web并清其内存缓存→原生CLI提交新候选，核对公开提交使用新配置，原执行记录不变。最后正常停止并核对本场景owned资源，保存summary/scenario及trace。无真实模型/PR，不依赖其他场景。

### WEC-A09

已完成并通过下方登记的最小自动验证。先由新脚本生成独立 `C:/SteerlineDebug` 包，正式Web目录查询取得本机Codex及已有远端Orca的真实模型/effort和原始来源。包启动后不保存/切换方案，不执行模型业务；记录版本、目标、请求方法、上下文未核实限制和owned资源清理。缺远端或认证/协议条件记阻断，不能用本机结果或fixture替代。专项只证明目录接线，不计完整L2或Orca执行验收。

## 人工验收

新增本机Orca自动项见下方A10–A14，与原A01–A09记录分别追踪。

### WEC-M01

待安排，可后置。自动项完成后由用户在新隔离现场查看中文名称、列表/编辑器、键盘及窄屏布局，切换机器/模型，观察离线/缓存/手动值/待重启文案。人工窗口、参与人和实际结果待定；只阻塞本功能体验签收，不阻塞无关开发。若发现配置丢失、身份混用或保存/生效错误，转为必须修复的真实失败，不按体验项豁免。

## 结果登记

| 日期/源码 | 验收项/实际选择 | 环境与命令 | 结果及证据 | 清理/未验限制 |
| --- | --- | --- | --- | --- |
| 2026-10-21 / 基线00eb8c6 + 本次文档工作树 | Spec/Plan链接、验收覆盖检查 | `git diff --check`；Python检查三份新文档的本地链接/标题锚点、A01–A09与M01映射 | 通过，链接/锚点错误0，验收ID覆盖完整；仅文档检查，不计功能通过 | 未运行功能测试，WEC-A01–A09及M01均待验 |

## 页面原型评审（不计功能验收）

2026-10-21 按用户要求生成独立会话内交互原型，随后用户明确确认“这个原型可以，就按这个实现”。原型使用示例数据，仅在预览内编辑/保存，不连接真实 Codex/Orca、不写 execution.json、不启动 SteerLine 或 Runner。原型评审不替代后续正式产品自动验收及 M01 产品体验签收。

原型及预览证据在 `C:/Users/lmas1/.codex/visualizations/2026/10/21/01a15c19-bd56-7e41-9e29-56cada6f1cb4/`：`execution-config-manager.html` 为交互源，`execution-config-desktop.png`、`execution-config-mobile.png`、`execution-config-dark.png` 为本次浏览器检查证据。采用独立 headless Edge，已验证新建/复制/删除、编辑保存后的待重启提示、模型变更清空非法 effort、离线/刷新示例、未保存切换保护；1024/736/375/320 像素宽度无横向溢出，浏览器运行时错误0。上述仅证明原型交互/布局，不能替代正式产品/API/原生接线验收；用户已确认原型；正式产品人工体验仍待签收。

## 2026-10-21 实现与验证结果

源码：`00eb8c6f89d4fe66bb4da840025094ce711d0422` + 本任务 dirty 工作树，Go `go1.26.4 windows/amd64`，Web `0.1.0`。未创建或改动 Runner 接口，未修改 W/E/V 生命周期规则。配置与原生目录 owner 在 Web，执行服务只增加原加载摘要投影并复用纯配置值。

| 验收项 | 实际选择和结果 | 证据 |
| --- | --- | --- |
| A01 | `TestWebExecutionConfigOwnership`、`TestTM04ExecutionBoundary`、`TestWebExecutionConfigRoutes` 通过；包括 Plan9 源码反例、Windows/Linux 生产闭包、Web 子包、认证/CSRF、条件写与不转发执行服务 | `wec-final-20261021/go-final.log`、`http-final.log`；后者同时核对原有认证/Host/Origin及HTTP文件读取原子替换 |
| A02、A06 | 纯配置及 store 全部用例通过；含 strict JSON、schema、手动值、pin/程序只读、冲突/并发、无改动不重写、取消发布保留旧文件及清临时文件、调试包后端拒写、父目录权限保持、高级字段保留 | `wec-final-20261021/go.log`、`store.log`、`go-final.log`；文件发布复用现有私有临时文件/原子替换设施 |
| A03、A04 | Codex 握手/分页、游标环、非法/超限/断连/取消/共享查询/缓存清理通过；配对加密 RPC fixture 的 probe、seed、缺能力和 runtime 变化通过，方法白名单零业务调用，环境结果不含配对凭据 | `wec-final-20261021/discovery-final.log`、`go-final.log`；测试源码记录实际方法，不运行模型轮次 |
| A05 | 定向 Vitest 共14项通过（配置组件7、API1、App6）；覆盖保存/复制/删除及默认约束、冲突保留、迟到结果、未知值、保留省略默认、丢回执先GET。TypeScript/Vite生产构建通过 | `wec-final-20261021/ui-final.log`、`build.log`；L3另含真实浏览器保存、刷新、窄屏无横向溢出和重启状态 |
| A07 | 新 `TestWebExecutionConfigLoadedSnapshot` 通过；原 `TestRunnerPresetFreeze` 定向回归通过（会话早期，约157秒）；L3再次核对旧文件门槛及旧输入不变 | `wec-final-20261021/snapshot.log`、会话测试输出及 L3 scenario |
| A08 | `configuration.execution-profiles` 最终复验完整通过，170.78秒；编辑保存→旧宿主阻断→正式stop/start→浏览器确认加载→正常停止Web→独立原生CLI/helper提交新候选，模型为fixture-updated、旧input_digest不变；cleanup_complete=true | `wec-e2e-20261021-final/L3/summary.json`、`scenarios/configuration.execution-profiles/scenario.json`、前后快照、两个浏览器阶段截图/trace |
| A09 | 正式Web路径取得本机及远端各7个模型/effort，origin=probe，context_unverified=true；原生目录专项通过，stop_exit=0、cleanup_exit=0，Runner resources_settled=true且runs/checks均空 | 下方独立debug包的discovery.json、截图/trace、cleanup.json、result.json |
| 测试登记 | suites/展示清单/README一致性及固定L1 owner分组校验通过；新配置owner和discovery owner已登记 | `wec-final-20261021/selection.log`。未运行完整固定L1、L2、L4或Full，不将定向子集称为全量通过 |

上述 `wec-*` 目录均位于 `C:/Users/lmas1/AppData/Local/Temp/`。受控 L3 使用其自身全新 SteerlineTest home/repo/端口，不使用生成调试包测试写入。

真实目录现场：`C:/SteerlineDebug/dev-20261021-103213-00eb8c6f-dirty`，由唯一生成器新建，未复制安装认证/数据库。实际所选 Runner 源 EXE 为 `C:/Project/SteerRunner/bin/steerrunner.exe`，版本 `0.2.0-dev.3-r2`，SHA256 `4dbf25c74ecc5d360fbfacc88fe599744cdc80a10648962ddc86f73c7d4b467b`，resource digest `a48bfb1aa7250009b21b4170532d877be01c23375ac386cb39e269794263ef91`。本机Codex握手版本 `0.159.2`；远端Orca `1.4.223`，environment `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`（台式机64G）。仅使用既有配对与账号，范围始终标为原生默认上下文未核实。

目录原始证据：`<包>/evidence/native-discovery/browser/execution-configs-live-nat-b193b--and-paired-remote-catalogs/discovery.json`，同目录 native-local.png/trace.zip；包级 `evidence/native-discovery/result.json` 和 `cleanup.json`。允许的方法固定于 Web 窄客户端：Codex initialize/initialized/model/list；Orca status.get、agentSession.agents、agentSession.modelCatalog（首次获取中时再次带 waitForListing）。未创建 thread/turn、终端/worktree/session，未执行模型或真实PR。真实联验不证明全部Orca版本/Agent、模型权限或远端业务执行已验收。

首次受控运行错误等待不存在的 awaiting_merge fixture状态，已改为既有 waiting_pr_merge；第二轮驱动错误使用仅支持start_task的control helper，已改用既有Candidate CLI driver。v1/v2报告保留失败且清理完整，v3通过，收紧原配置门槛的identity-error断言后final轮再次通过。真实专项首轮浏览器使用相对请求URL失败，第二轮查询通过但过早停初始化项目导致closure证明不完整；第三轮先等待项目ready后通过全部清理。失败包102541/102925证据保留，后者不计清理证明通过。开发期间生成器npm ci与一次单元命令重叠造成vitest缺失，依赖就绪后重跑通过；新增测试误用Playwright的exact选项导致一次TS编译失败，修正后最终构建通过。

首轮L3和原生联验后补充格式化、浅色标签对比度、Linux路径大小写判断、Orca正文及Web响应1MiB预算；最终L3再次通过，相关最终单元/构建通过（另见http-final.log）。目录缓存TTL另按Web插入时间计算，避免远端时钟延长缓存；其过期反例及最终discovery测试通过。没有重复真实目录专项。WEC-M01正式产品人工签收待用户安排，不把原型认可当产品人工验收通过。

## 2026-10-21 人工验收现场已打开

按用户要求新建并启动 `C:/SteerlineDebug/dev-20261021-104714-00eb8c6f-dirty`，由唯一生成器构建，`debug prepare` 固定live方案后使用正式start启动；未使用旧安装/调试包。SteerlineTest已online，后端PID44196、Web PID53852，状态记录在该包 `evidence/manual-config-acceptance/ready-status.json`。验收页面已在当前聊天浏览器打开并保留，地址 `http://127.0.0.1:61774/settings/execution-configs`；已从页面确认配置编辑器、7个真实本机模型、已登记远端机器、thinking effort以及当前服务已加载提示。

环境保持运行供用户验收，WEC-M01仍待用户反馈，不记人工通过。冻结调试包的新增/保存受限制，页面显示明确原因；本现场用于布局、字段联动、目录查询及预览验收，保存/重启闭环已有受控L3证据。结束时应在此包home范围使用正式stop，不能停止其他环境。

## 本机 Orca 增量设计阶段记录（2026-10-21）

本轮仅补规格/计划。静态调查确认Orca本机named pipe/authToken/envelope已有入口，所选Runner既有schema能表达local/orca-terminal及Codex/Pi/DSH，不需要Runner新探测接口或Go执行服务转发。未运行本机Orca目录探针、helper、模型或新增功能测试，未重启/修改当前人工验收包。调查路径、固定EXE身份及依赖前置见Plan“本机Orca增量调查与边界结论”；不将Runner仓库历史验收当成本任务通过。

### WEC-A10

已实施，定向验证通过，证据见末节。Web自有本机IPC夹具验证metadata与端点边界、authToken/客户端能力协商、status/agents/modelCatalog方法白名单、响应ID/runtime及版本核对、获取中/空目录/不支持、断开/超限/超时/取消与owned连接清理。证明本机Codex、本机Orca、远端Orca及各Agent的缓存/晚到响应隔离；本机不使用远端配对/隐式environment，不启动Orca或任何业务资源。依赖守卫仍禁止执行服务探测、Web导入Runner私有包/业务库，无Runner新增接口。

### WEC-A11

已实施，定向验证通过，证据见末节。本机Codex/Pi/DSH公开配置字段映射及完整schema交集成立；各Agent effort、workflow/审计条件正确。受信本机helper路径/SHA继承、新增/变更缺材料阻断、错SHA/任意路径拒绝，未改动既有执行值及其他链路可保留；远端拒绝携带runtime键（含空值）。未知目录允许显式手动schema合法值但不伪造已验证，原条件保存、部署pin/程序只读与高级字段保留不退化。实际部署需要正式 helper，本次按固定发布提交及正式构建方式准备，来源与摘要见末节；目录不作为执行资格证明。

### WEC-A12

已实施，定向验证通过，证据见末节。本机运行机下可选Codex直连或Orca+Agent，后者显示Codex/Pi/DSH的独立目录状态；切换链路/Agent正确重置或确认模型、effort、workflow/审计及runtime不适用字段。三通路旧响应不能覆盖新选择。保留草稿、显式手动入口、原生默认省略、默认/删除约束、材料缺失提示、只读程序来源及待重启状态；不新增自动重启。

### WEC-A13

已实施，受控 L3 通过，证据见末节。沿既有 `configuration.execution-profiles` 参数化本机Orca输入，在自己的controlled SteerlineTest home/端口/仓库和公开Runner fixture中完成Web保存、旧宿主门槛、正式stop/start、关闭Web后CLI消费；核对adapter/target/agent/model/effort/runtime路径/SHA与保存值一致，旧候选完整输入/摘要不变，资源正常清理。三Agent字段组合可复用流程和L1，不另造同构场景；fixture通过不代表真实helper/模型业务执行通过。

### WEC-A14

已实施，真实目录专项通过，证据见末节。新生成C:/SteerlineDebug包，经正式Web API取得真实本机Orca的Codex目录/effort，记录版本、named-pipe通路、来源、runtime身份和上下文未核实范围；Pi/DSH目录是否存在如实记录，缺目录不伪造探测结果。无配对/终端/worktree/agentSession创建、无helper或模型业务输入、无认证正文落证据；仅释放Web连接和本包服务，不关闭共享Orca。此专项可使用默认本机Codex执行配置承载Web，不据此证明本机Orca执行部署材料或业务链路就绪。缺IPC/认证为明确阻断，不能借远端结果通过。

### WEC-M02

人工后置，窗口及参与人待定。实施及自动项完成后，在新受控可写fixture或另建冻结debug只读现场体验本机双链路、三Agent、各目录/手动值/helper缺失提示、字段联动及窄屏。冻结包不改方案或保存；保存/正式重启用独立controlled现场。只阻塞本机增量体验签收，真实数据损坏或身份混用必须修复。

### 本机增量结果登记

| 日期/源码 | 实际选择 | 结果与证据 | 未验/阻断 |
| --- | --- | --- | --- |
| 2026-10-21 / 00eb8c6 + 当前dirty文档修订 | 读取Orca版本匹配CLI指南/help及本机metadata/transport源码；固定Runner既有离线schema、发布build.json及EXE SHA | 设计可保持既定边界；正式Web将自行维护窄IPC，不加载安装JS或Runner私有实现。Spec/Plan覆盖A10–A14/M02 | 仅调查/文档检查；A10–A14/M02均未验。正式helper材料未提供；原验收环境保持运行且未改配置 |

本轮文档检查：`git diff --check` 通过，本地链接及新增A10–A14/M02的Spec/Plan/验收映射与锚点检查通过，错误0。仅文档检查，不计新增功能验收。

## 2026-10-21 本机 Orca 实现与验证

源码基线仍为 `00eb8c6f89d4fe66bb4da840025094ce711d0422` + 本任务 dirty 工作树；Web 0.1.0，Go go1.26.4 windows/amd64。增量生产代码位于 Web 前后端及唯一调试包生成器，另修正调试工具 prepare 的 adapter 判断。Go 执行服务沿用配置消费/冻结及原加载摘要，无本机目录接口或 Web 依赖；Runner 仓库未修改，没有新增 Runner 探测接口。

正式 helper 来源已核对：Runner `docs/testing.md` 支持 `go build -o <安装路径>/steerrunner-agent.exe ./cmd/steerrunner-agent`；所选 EXE 的 build.json 固定干净发布提交 `8261310d5f2d1edb6fa0c81ba05fa4119b561386`。在新准备包 `C:/SteerlineDebug/dev-20261021-110118-00eb8c6f-dirty/materials/runner-8261310/` 隔离归档该提交后按此命令构建，不使用较新 Runner 工作树 HEAD，也未修改 Runner。`publication.json` 记录归档SHA `6cab635689c9f5b7108911b31b3b453038a75115405b840f67b04d06ada3a897`、helper SHA `49363b3cf8eea218c8babb33611fb928b9d6060d5526c8b3daf6570d5ed3f415`、命令及 Go 版本。Runner EXE/version/SHA/resource 身份与前文相同。Web/执行服务不构建或运行 helper 获取目录；这份程序只作为显式部署材料，由生成器校验、复制并固定路径/SHA。

| 验收项 | 实际选择与结果 | 证据位置 |
| --- | --- | --- |
| A10、A01回归 | `TestLocalOrcaCatalog`、原 Codex/远端 RPC/共享缓存用例和 Web/执行依赖守卫通过；覆盖本机方法白名单、认证、协议版本/metadata及响应runtime、错误ID、未注册/空目录、获取中、断开/超限/取消与连接清理、Agent和身份变化缓存隔离。新增 Runner 私有导入反例，原路由认证/CSRF及显式adapter/任意endpoint拒绝通过 | `C:/Users/lmas1/AppData/Local/Temp/wec-local-orca-20261021/go-final.log`、`discovery-final.log`；用例登记 quick-owner-web-discovery/quick-execution-boundary |
| A11、A02/A06回归 | 三Agent及完整公开schema验证通过；Codex本机审计正例、Pi/DSH审计与effort分支、helper继承/错SHA/任意路径拒绝、远端runtime键（含空值）、helper暂缺仍保留名称/默认和其他链路，原条件写/冻结/程序只读通过。测试使用带所选发布身份的独立 schema 快照，生产读取固定 EXE 的实时离线 schema | 同上 `go-final.log`；新增 `web/server/executionconfig/testdata/local-orca-capabilities.json` 仅为测试契约，不作为生产能力源 |
| A12、A05回归 | 定向 Vitest 16项通过（组件9、API1、App6）；验证本机双链路、Pi/DSH独立effort、迟到结果、helper缺失仍可查询、三Agent映射/程序继承、切远端剥离runtime、提示词高级值保留。TypeScript/Vite生产构建通过 | `wec-local-orca-20261021/ui-final.log`、`build-final.log` |
| A13、A08回归 | 既有 `configuration.execution-profiles` 扩展后完整通过，236.03秒；浏览器保存Codex/Pi/DSH三配置→旧宿主配置门槛→正式停启→加载摘要一致→停止Web→CLI分别提交，核对adapter、local target、Agent、模型、effort、helper路径/SHA及native executable，旧候选input_digest不变；cleanup_complete=true | `wec-local-orca-20261021/l3-v2/L3/summary.json`、`scenarios/configuration.execution-profiles/scenario.json`、before/after快照与浏览器两个阶段trace/截图 |
| A14 | 正式 Web API 的本机 Orca named pipe 目录专项通过；Orca 1.4.224、协议3，Codex 7模型，Pi 414模型，均origin=probe；DSH未注册，返回unsupported及手动未验证说明，零模型，不借用Codex结果。本机Codex直连另取得7模型。共享Orca未关闭，专项stop_exit=0、cleanup_exit=0，Runner resources_settled=true、runs/checks为空 | `C:/SteerlineDebug/dev-20261021-111754-00eb8c6f-dirty/evidence/native-discovery/result.json`、`cleanup.json`、`browser/.../discovery.json`、`native-local-orca.png`/trace |
| 测试登记 | 分级展示清单与实际场景、固定owner归属与零匹配检查通过；没有新增三份同构L3，没有运行Full/L2/L4或真实helper/模型/PR任务 | `wec-local-orca-20261021/selection.log`；测试场景源码及README同步 |

真实本机 runtime 只在证据记录摘要 `ba0c735e610e556683c39c8cf8d4065242419468f6cd8c10ceabb449f80a579b`，不落 authToken、pipe认证正文或远端配对凭据。目录来源时间由 Orca 原样返回，仍为原生默认上下文，context_unverified=true；目录成功不表示模型权限/工作区/原生业务执行已验证。专项没有执行 helper、模型轮次或真实 PR；本次 helper 发布材料及受控字段消费不能替代 Runner 本机执行资格验收。

首轮 L3 因原 caller fixture 只发布无约束 object、没有 effort 契约而无法保存；已为该场景补显式最小离线契约及自有空 Orca userData，避免受控测试读取真实用户Orca。首轮在只读目录方法内曾访问本机目录，没有业务输入；其失败与清理证据保留于 `wec-local-orca-20261021/l3/L3/`，不计通过。v2受控与真实专项分开验证，未用fixture冒充真实目录。开发中新增测试两次因预期分支/TS类型失败，修复后定向复跑通过。

人工包首轮 `dev-20261021-111945-00eb8c6f-dirty` 的启动被既有 kill-on-close Job 守卫拒绝，未到API ready；准备工具还暴露了将所有local误判为Codex的旧假设，已改为adapter+target判断。失败包保留证据且不复用；其原宿主未建立可核验resident，stop/verify-stop的未核验结果保留，不冒称正式清理通过。最终包采用仓库既有独立用户上下文启动方式，未放宽服务Job守卫、修改Runner或复用安装状态。

### 新本机 Orca 人工验收现场

按用户要求新建 `C:/SteerlineDebug/dev-20261021-112240-00eb8c6f-dirty`，唯一生成器在启动前校验并复制正式 helper 到本包 `bin/steerrunner-agent.exe`，部署来源保存在 `evidence/helper-publication.json`；live方案冻结后通过正式start启动。准备脚本自动确认本机Orca未被写入顶层Codex executable。后端PID12500、Web PID55796，SteerlineTest已online、runtime_state=ready，状态记录于 `evidence/manual-local-orca/ready-status.json`。

验收页已在当前聊天浏览器新标签打开并保留：[执行配置](http://127.0.0.1:51327/settings/execution-configs)。已从真实页面确认本机Orca链路、Codex/Pi/DSH选择器、helper材料核对通过、7个Codex模型/effort及“当前服务已加载保存配置”。该包保持运行，M01/M02仍待用户正式反馈，不将打开页面或原型认可记作人工通过。冻结包可预览/查询字段，新增保存由后端拒绝；保存与正式重启已有独立受控L3证据。原61774人工现场未改配置或停止，安装目录未使用。结束本现场时只在本包home范围正式stop。

## 统一 Orca 目录与 provider 凭据边界增量

按用户最新确认，SteerLine只查询和选择目标已配置能力，模型provider/API key由Orca所启动的Agent管理。新增A15/A16，不把原目录通过扩展为远端Pi执行通过；当前51327人工包仍为先前冻结版本，本轮不修改/重启。

### WEC-A15

已实施，定向自动验证通过。Web查询层统一本机/远端Orca RPC与Agent、模型、provider、effort投影；两传输身份/认证独立。远端注册Pi可以查询，原始多级provider/model保留，目录不附带Web执行策略、不虚构注册，目录成功不开放远端Pi/DSH执行。返回中不透出provider凭据；API拒绝凭据输入，Web不增加provider/Agent独立配置发现或Runner接口。

### WEC-A16

已实施，17项定向前端测试及生产构建通过。Pi Provider选项只取Orca目录，切换清理模型/effort，保存完整selector而不保存provider/key配置字段；未支持远端Agent显示目录事实和禁用原因，目录/凭据/调用及上下文状态分开。现有CRUD、未知值保留、冲突及生效提示不退化。

本轮已同步主干 `ef6449a7`，生产代码固定在 `530631b8`；其后仅修正组件测试中已移除的目录DTO夹具，并登记结果。以下证据根目录为 `C:/Users/lmas1/AppData/Local/Temp/wec-unified-orca-20261021/`。测试范围、入口、环境和通过条件见Plan U11及对应测试表。

| 验收项/范围 | 实际验证与结果 | 证据 |
| --- | --- | --- |
| A15、A01/A03/A04/A10/A11回归 | 本机named pipe/远端配对加密WebSocket受控测试通过；相同Pi完整selector和provider投影、远端按请求Agent查询、注册目录与执行支持分离、凭据不透出/不接收、原生方法白名单及依赖边界成立。固定owner/展示清单/零匹配守卫通过 | `go-final.log`，实际命令包含Plan A15及Codex目录、环境目录、缓存取消和测试登记定向项；没有调用模型业务方法 |
| A16、A05/A12回归 | 配置组件10、API1、App6，共17项通过；Provider筛选及原样保存、切换字段清理、远端已发现Pi仍禁用，原组件回归通过。TypeScript/Vite生产构建通过 | `ui-final.log`、`build.log`；构建为同一生产源码，夹具修正后重跑17项 |
| A07回归 | `TestWebExecutionConfigLoadedSnapshot` 通过，执行服务只提供已加载配置摘要，不查询能力 | `snapshot.log` |
| A08/A13主干集成复验 | `configuration.execution-profiles` 通过，163.11秒；报告记录源码 `530631b844202ad3126e69eb2ec3a42a6d99e18b`、启动时工作树clean。浏览器保存本机Orca三Agent配置、旧宿主门槛、正式stop/start、加载摘要及关闭Web后CLI消费均通过，旧候选摘要不变，cleanup_complete=true | `l3-final/L3/summary.json`、`scenarios/configuration.execution-profiles/scenario.json`、before/after快照、两个browser阶段日志/截图/trace；使用自有controlled SteerlineTest home/repo/端口、离线契约与空Orca userData |

文档本地链接目标和 `git diff --check` 通过。未重复真实目录专项或真实helper/模型/PR业务任务，不把受控远端Pi目录查询计为远端Pi执行验收；M01/M02人工项仍后置。

首轮独立进程包装启动后没有形成可观测测试子进程或scenario报告，未计测试通过，已停止该轮owned等待进程。最终使用同一WMI独立用户上下文，通过临时脚本文件调用既有 `test-runtime-isolation.ps1 -IndependentProcess`，报告与退出码均通过；未修改宿主Job守卫或产品代码以绕过限制。

## PR #375 主干冲突处理（2026-10-21）

按用户要求仅同步主干 `b9e5002d38da4722dc8457338e063ce56817fb48`，保留文档归档和现行契约修订；唯一内容冲突为Spec导航，保留执行配置入口并采用主干最新测试管理说明。相对原PR head `a683a730` 没有生产/测试代码变化，文档链接检查通过（523份Markdown、3424个本地导航链接、0错误），无冲突标记及diff空白错误；不重复运行功能测试、不将旧验证称为本轮复跑。原边界保持。

执行配置与机器/项目环境验证的整合设计/原型已按用户意见停止继续推进。该轮未提交设计单独保存在具名Git stash中，不进入本PR；当前规格与产品实现维持整合前的独立功能范围，不实施U12–U15或相应界面改动。现有人工调试环境未改动。
