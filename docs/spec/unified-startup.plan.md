# Plan：统一启动与多项目共进程宿主

范围归 [Spec](unified-startup.spec.md)，结果归[专属验收](../acceptance/unified-startup.md)。2026-10-15 用户只要求生成文档；本 Plan 不启动实现、进程、安装或验收，不纳入 roadmap。

代码调查基线 `db92f5c46bd90a45b1cfa2a6e94236113b77803a`，含双 exe bat 与 Web 配置 ACL 修复的本地整合。已完整读取 `docs/spec/SpecGuide.md`，该文件最后修改 commit 为 `94e704300485b82d54e44991148074841ed66019`，本次 SHA-256 为 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`；同时读取 [worker-lifecycle](../product-design/worker-lifecycle.md)、[停服恢复契约](worker-stop-recovery.spec.md)、[分级入口与扩展约定](../../tests/e2e/README.md)。普通开发依用户授权直接开展，不要求 roadmap 编号或执行聊天。

## 1. 当前实现调查与整合决定

| 现有位置 | 调查结论与本次处理 |
| --- | --- |
| `cmd/steerline/main.go`、`internal/app/application_service.go` | serve-app 只提供目录/HTTP 转发；run/start-service 为单项目进程。改为 start/serve/stop/status 和多项目应用宿主，去掉旧并行启动链 |
| `internal/app/application_directory.go`、`internal/projectconnection` | 当前按配置指纹异步只读解析、再借用项目 HTTP client。保留有界解析及身份/冲突规则；拆为启动配置快照和进程内运行时目录，去掉原连接缓存/回环转发 |
| `internal/app/app.go`、`execution_service_wiring.go` | 工厂已经组合 execution Service、存储和 host；当前 lifetime 还创建 per-project local API。抽出 ProjectRuntime，移除 listener/discovery 接线，保留原 actor/任务服务/存储提交边界 |
| `internal/app/host/service_client.go`、`service_host.go`、`service_loop.go` | 启动、日志、进程/锁、项目初始化混在单项目函数；停止观察依赖进程退出。将应用进程生命周期与项目运行时生命周期拆开；仓库/运行目录锁必须先于项目库初始化 |
| `internal/product/applicationcontract`、`applicationhttp`、`applicationclient`、`httpapi`、`httpcontract` | 应用 Directory.Connect 当前固定返回 HTTP client；项目 handler 已有查询/命令和原请求核验。抽取传输无关处理，改应用 handler 直接访问项目端口，不复制业务处理、不构造假 HTTP 请求 |
| `internal/product/query`、`internal/product/api`、`internal/execution` | 保留每项目任务/查询/命令/actor 规则及 worker 所有权；核对同一 PID 下实例核验、关闭、进程树归属，不将宿主 PID 当 worker 停止目标 |
| `cmd/steerline-web`、`web/server`、`web/src` | 更新应用协议与身份/目录消费；沿用现有单源认证、项目 URL、原信封、草稿和标签页隔离，不重做页面架构 |
| `internal/localservice`、host launcher 模板、`build.bat`/`build.ps1` | 复用 home/精确进程身份/租约/原子私有文件；补短启动协调和可见错误，保留 bat 双产物与 home ACL 修复；删除明确拥有的旧模板 |
| `scripts/new-debug-release.ps1`、`debug-template/`、`tests/e2e/internal/driver`、`web/tests/project-switch.spec.ts` | 当前生成/驱动分别启动应用、Web、多个项目。统一采用双进程入口与共享宿主，保留包级模拟冻结、两个独立 clone/provider 和精确清理 |

用户明确 TUI 正在移除。调查快照中的 console/consoleapp 残留不作为本任务消费者；不为其增加适配、兼容、测试或新的依赖。实施同步当时主干的移除成果后处理本任务入口交叉，不重新引入 TUI，也不把其他聊天的移除工作列为本任务人工前置。Orca agent 终端与产品 TUI 区分。

本次无需新增通用服务容器、第二套业务注册表、跨项目任务总库、独立恢复引擎或应用到项目的网络桥。ProjectRuntime 为已有工厂/lifetime 的必要资源 owner；宿主目录只保存配置快照、句柄和生命周期投影。两者消费者分别是应用 host 与应用 handler。

## 2. 实施步骤与依赖

1. **固定契约及生命周期边界。** 定义 application/v2 路由/DTO、应用 ready 与项目状态、宿主/运行时实例归属、错误和 CLI start/status/stop 结果。将现有 Directory.Connect 替换为窄项目查询/命令端口；浏览器信封继续使用项目实例，不增加另一套业务回执。先用契约/handler 反例锁定错项目、旧实例及原请求结果未知的语义。
2. **抽取 ProjectRuntime。** 从 app 工厂和 host 单项目启动函数提取可独立建立、运行、正常停止、关闭及报告终态的 owner。每项目持有独立 context、actor、数据库、日志和锁；逐项登记失败前已创建资源并关闭。核对进程级单例、signal、cwd/环境、输出及 Windows Job 的所有者；进程级资源由宿主统一管理，项目资源显式注入，不能通过修改全局环境/目录或结束进程完成单项目操作。将身份/配置只读核验前置，取锁后才能初始化库，所有退出路径聚合清理结果；项目停止不依赖共享进程退出。保留冻结执行停止策略和原 worker 规则。
3. **实现多项目宿主。** 建立 home 单实例后固定 etc 配置快照，分阶段解析全部候选及冲突，再有界启动无冲突项目。应用 API 先可用，目录展示启动/失败投影；坏项目、慢项目、缺执行环境分别隔离。默认 manual，禁止启动/重连自动产生新业务输入。正常停服先拒绝新业务，再并行停止全部项目并聚合事实，最后关闭端点；某项目停止失败不跳过其他项目。未核实的资源阻止替代写者。
4. **收口项目 API 与 CLI。** 从原 httpapi 提取并复用查询/命令处理，接入宿主运行时端口，拆除单项目 HTTP 发布及应用回环客户端。CLI 用 `--project <project-id>` 选定宿主内项目；梳理 task/control/answer 的实际读写消费者，所缺服务器能力补到原项目应用用例，写入不能退回本地 SQLite。纯文档/Git 只读校验可继续作为本机工具。测试驱动的任务准备改为宿主就绪后走正式创建入口，不由夹具写业务表。
5. **实现统一启动与可观测性。** start 为短协调命令，与 stop 使用同一协调锁且与常驻租约分离；按精确身份复用实例、建立进程创建事实、等待真实 API 就绪，不固定 sleep。落实 start 180 秒、stop 120 秒的协调上限和 ready/degraded/未核实结果。处理并发双击、已有一个、创建未知、端口占用及 stale discovery；Web 失败保留后端，浏览器失败保留两个服务，重试仅补缺项。status 复用观察代码且只读；stop 对 Web/宿主均尝试、汇总结果，项目停止不 kill 共享 PID。复用实际系统打开浏览器的窄适配能力，协调器定向测试可注入记录器，验证默认分支调用一次与授权 URL 归属，不增加生产测试专用开关。提供脱离短启动器后仍正常存活的 Windows 进程句柄/Job 策略；worker 资源的原 Job 所有权保持可核实。
6. **更新 Web 与启动器。** Web 自动连接 application/v2 并核验宿主换代/项目实例；目录能显示零项目、全失败、部分启动与重连状态。start.bat/stop.bat 固定安装 home、正确转义中文/空格/cwd、保留失败窗口和退出码；不要求用户分步启动。低层 Web CLI 保留独立重启。安装清理仅删除归属明确的旧启动器，未知用户文件保留；不现场迁移/重置数据，不为安装 home 设置私有目录 ACL。
7. **先补齐唯一生成器与驱动能力。** 调试生成器默认输出双 exe 及统一宿主拓扑，WebProjects 仅表示项目数量；不再按 WebAddress 是否提供选择旧单项目拓扑。默认自动选择空闲端口并写入 manifest，显式 WebAddress 仍可固定，启动不漂移。包级 start.bat --simulate 先冻结执行方案，再调用统一入口；产品 start 不承担 simulate 选择。新驱动复用正式 install/launchers 在本包 home 安装双 exe 和入口，调用 start --no-open/status/stop，并核验正在运行的两个安装目标分别拒绝覆盖；按产品 exe 路径区分两个常驻进程与测试 helper/worker，不再为每项目派生产品进程。同步 `test-web-smoke.ps1`、`test-debug-release.ps1` 和共用 driver 的启动接线，不留旧 CLI 备用链。扩展原浏览器 project-switch 场景，新增 `scripts/test-unified-startup.ps1 -Package <新包> -Simulate` 作为稳定单场景入口，负责正确冻结、Edge 调用、证据和正常清理。此入口及新测试在当前文档阶段尚不存在，必须实现后才执行验收。
8. **运行最小验证并统一交付。** 执行下表的定向测试、L1、双 exe 构建及一个新双项目 Edge 综合场景；更新本任务 acceptance，不借用前序结果宣称本次通过。同步运行指南、架构、应用/项目协议、project-management、前序 Spec 的后继说明、worker-lifecycle 的 W18/V06/V07 与实现缺口，并明确后端停止已覆盖全部项目。后续项目添加规格改为消费本宿主的准入/装载端口，保留登记不自动运行/派发的边界。移除旧单项目流程对应的测试断言，以新边界替换，保留仍适用的事务与身份反例。

步骤 1→2→3 为主依赖；步骤 4/5 在运行时所有权固定后接入，步骤 6 消费固定协议；步骤 7 必须在真实调试/自动验收之前完成。实现时只复用当前主干正式规则，不按此调查 commit 恢复已被移除的 TUI 或旧链路。

与后续项目添加的交接条件：应用 host、协议、目录和项目装载/停止端口已实现并验收，更新后的当前 commit 固定可读。它将接管成员准入；本任务不抢先实现登记或结构核验，也不为其新增热加载 API。

## 3. 测试计划

所有自动要求链接[本任务专属验收](../acceptance/unified-startup.md)。新增 Go 场景统一使用 `TestUnifiedStartup` 或 `TestMultiProject` 前缀，下表各组选定的用例必须实际存在且命中非零；仅编译包不能记作场景通过。原始报告用 runner 新临时目录或新包 evidence，Plan 不记录实时结果。

| 验收项 | 级别 / 组 / 场景 | 环境及命令 | 时点 / 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| [UST-A1、A2、A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.host` | Windows 隔离 home/Git/SQLite 夹具；`go run ./cmd/test-feedback -- ./internal/app ./internal/app/host ./internal/localservice -run '^(TestUnifiedStartup\|TestMultiProject)' -count=1` | 步骤 2/3/5 后；覆盖双项目同 PID、独立 actor/store/context、慢/坏/重复/状态冲突/跨 home、锁先于初始化、manual、单项目关闭、聚合停止、并发/部分启动和失败诊断 | 对应 UST ID；summary、项目实例与锁/写入断言；Windows真实子进程只在隔离夹具中 |
| [UST-A3、A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.api` | 隔离运行时和 HTTP；`go run ./cmd/test-feedback -- ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^(TestUnifiedStartup\|TestMultiProject)' -count=1` | 步骤 4/6 后；身份/路由/认证、原命令/回执/大整数、旧实例与不可用语义、无项目 HTTP 回环、断线与实例刷新通过 | UST-A3/A5；runner JSONL/summary |
| [UST-A4、A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.lifecycle`，关联 worker V06/V07 | 隔离持久化 fixture 和 controlled 生命周期端口；`go run ./cmd/test-feedback -- ./internal/app -run '^TestMultiProjectLifecycle' -count=1` | 步骤 3/4 后；stop_execution 与 continue_execution 分别核验、正常/未正常停止后重开零新输入/答案、旧写者未知阻止替代、迟到事实不串兄弟，项目 stopped 不要求 PID 退出 | UST-A4/A5；原资源/Operation/Attempt、停止事实与副作用计数 |
| [UST-A3、A6](../acceptance/unified-startup.md) | Web 定向与构建 | `npm --prefix web test`；`./build.bat --no-pause`；隔离 home 的 Go launcher/ACL 用例纳入 `TestUnifiedStartup` | 步骤 6 后；保留仍适用的项目/标签/草稿回执测试、故障提示正确；两个 exe 均刷新，业务 exe 无 Web 资源，运行目标拒绝覆盖，旧 bat 删除范围/ACL/路径正确，无新增 TUI 支持 | UST-A3/A6；Vitest、构建日志与双 SHA-256、隔离安装断言 |
| [UST-A1～A7](../acceptance/unified-startup.md) | 真实 Edge / controlled `web.unified-startup`，改造原 project-switch 场景 | `./scripts/new-debug-release.ps1 -WebProjects 2`（步骤 7 默认双 exe、自动固定空闲端口）；`./scripts/test-unified-startup.ps1 -Package <本次新包> -Simulate` | 全接线后；使用包级 --simulate 冻结和产品统一入口，真实双项目同 PID、自动连接、授权/选项目/两标签、CLI 正式写入、丢回执原 ID、Web 活动轮次重启、后端正常停启及授权门槛通过；收尾事实完整 | UST-A1～A7；manifest/hash、CLI 状态/原身份、提交次数、Edge screenshot/trace 与 cleanup JSON |
| [UST-A1～A6](../acceptance/unified-startup.md) | L1 最小回归，新增小型 owner `quick-owner-unified-startup` | `go run ./cmd/test-feedback -level=L1` | 最终候选；在 scoped.go 登记零/坏项目、共享 PID 下实例停止、重复启动及跨项目命令隔离的少量关键反例；更新因旧进程模型失效的 owner，保留架构/事务守卫，剔除已退役 TUI 选择，不借机扩大全量 | 对应 ID 的回归摘要与 coverage，不能代替 Edge 综合验收 |
| 文档 | 静态 | `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | 规划及实现文档收尾；本地链接/Task ID/差异无错误 | acceptance 静态检查记录，与产品验收状态分开 |

表中正则的竖线是 Go 的 alternation；Markdown 表转义不属于命令文本。综合场景沿用真实 Go/React/Edge、两个独立 SteerlineTest clone 和独立 controlled provider；禁止 linked worktree 假造两个项目，模型/PR 行为属于模拟，不能记为 live 或真实 PR 验收。

### 必须补齐的综合驱动及取证

- 更新生成器为默认双 exe/统一宿主，WebProjects 1/2 均不再依赖显式 WebAddress 来启用 Web；维护模板与 manifest，不另写临时安装器。显式端口冲突在隔离测试中用本次 owned listener 制造，不占用/停止别人的服务。
- wrapper 只接受生成器 ready manifest 对应的新包，先显式冻结 simulate，再使用该包 home 的统一入口。先启动宿主后走正式 CLI 创建任务；活动轮次用原 controlled HoldStep/HoldStream 保持。新日志不能泄露认证材料。
- 同一个新场景依次记录两项目共享后端 PID/创建时间、不同 project/runtime instance、独立 DB/Execution/Attempt，以及 worker 原进程/session/turn；正常重启 Web 后全部原项目与轮次不变，再释放并按原轮次结算。
- 在明确可核验的 controlled 活动窗口正常停止整个宿主，分别保存两项目的运行时和资源收尾；重开后验证新项目实例及零新派发，按正式继续授权检查适用的原会话恢复路径。stop_execution/continue_execution 的全部分支由上表定向 fixture 覆盖，不为穷举后端再生成多套浏览器场景。
- 场景通过条件在业务断言结束时冻结；正常停止本包 Web/宿主，聚合停止失败并单独记录 cleanup。核对本包产品进程及监听为 0，owned helper/worker 由驱动逐项结算；允许继续运行的外部资源按冻结策略另列，不混称全部已退出。场景上限 15 分钟，清理独立 2 分钟；预算和原失败在 evidence 保留，禁止失败后换包结果冒充同一次通过。

不选择 Full、全部 L2/L3、真实模型/PR 或远端矩阵：本批重构服务宿主与入口，采用真实 Windows 进程/ACL、隔离存储/生命周期反例和一个真实 Edge simulate 综合场景；Worker 重试/业务流程规则不变。若实现实际改变其规则，须显式更新 Spec、统一生命周期文档和最小验证范围，不能默认为已覆盖。

UST-M1：自动通过后由用户体验双击启动/重复打开、故障提示、选择项目和停止反馈；参与窗口待安排，仅阻塞体验签收，不阻塞已通过自动项的交付。实施交付时同步 acceptance 的逐项已验/待验、源码版本/环境/命令和证据；本次生成文档仅登记静态检查，不执行产品验收或部署安装目录。
