# Plan：统一启动与多项目共进程宿主

范围归 [Spec](unified-startup.spec.md)，结果归[专属验收](../acceptance/unified-startup.md)。2026-10-15 用户只要求生成文档；本 Plan 不启动实现、进程、安装或验收，不纳入 roadmap。

初稿调查基线为 `db92f5c46bd90a45b1cfa2a6e94236113b77803a`。按用户要求于 2026-10-15 rebase 并重新审计，明确采用主干 `317714e6e42be0f902a97b1a5577e325970d2f75` 为当前调查基线，已包含 #321/#322 构建/ACL 修复及 #323 Web 查询优化；保留后者代码与验收边界。已完整读取 `docs/spec/SpecGuide.md`，该文件最后修改 commit 为 `94e704300485b82d54e44991148074841ed66019`，本次 SHA-256 为 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`；同时读取 [worker-lifecycle](../product-design/worker-lifecycle.md)、[停服恢复契约](worker-stop-recovery.spec.md)、[分级入口与扩展约定](../../tests/e2e/README.md)。普通开发依用户授权直接开展，不要求 roadmap 编号或执行聊天。

## 1. 当前实现调查与整合决定

| 现有位置 | 调查结论与本次处理 |
| --- | --- |
| `cmd/steerline/main.go`、`internal/app/application_service.go` | serve-app 只提供目录/HTTP 转发；run/start-service 为单项目进程。改为 start/serve/stop/status 和多项目应用宿主，去掉旧并行启动链 |
| `internal/app/application_directory.go`、`internal/projectconnection` | 当前按配置指纹异步只读解析、再借用项目 HTTP client。保留有界解析及身份/冲突规则；拆为启动配置快照和进程内运行时目录，去掉原连接缓存/回环转发 |
| `internal/app/app.go`、`execution_service_wiring.go` | 工厂已经组合 execution Service、存储和 host；当前 lifetime 还创建 per-project local API。抽出 ProjectRuntime，移除 listener/discovery 接线，保留原 actor/任务服务/存储提交边界 |
| `internal/app/host/service_client.go`、`service_host.go`、`service_loop.go` | 启动、日志、进程/锁、项目初始化混在单项目函数；停止观察依赖进程退出。将应用进程生命周期与项目运行时生命周期拆开；仓库/运行目录锁必须先于项目库初始化 |
| `internal/product/applicationcontract`、`applicationhttp`、`applicationclient`、`httpapi`、`httpcontract` | 应用 Directory.Connect 当前固定返回 HTTP client；项目 handler 已有查询/命令和原请求核验。抽取传输无关处理，改应用 handler 直接访问项目端口，不复制业务处理、不构造假 HTTP 请求 |
| `internal/product/query`、`internal/product/api`、`internal/execution` | 保留每项目任务/查询/命令/actor 规则及 worker 所有权；核对同一 PID 下实例核验、关闭、进程树归属，不将宿主 PID 当 worker 停止目标 |
| `cmd/steerline-web`、`web/server`、`web/src` | 更新应用协议与身份/目录消费；沿用现有单源认证、项目 URL、原信封、草稿和标签页隔离。保留 #323 的 queries.ts、缓存键、预取、可见查询及定向失效，不按宿主 PID 合并缓存、不重做页面架构 |
| `internal/localservice`、host launcher 模板、`build.bat`/`build.ps1` | 复用 home/精确进程身份/租约/原子私有文件；补短启动协调和可见错误，保留 bat 双产物与 home ACL 修复；删除明确拥有的旧模板 |
| `scripts/new-debug-release.ps1`、`debug-template/`、`tests/e2e/internal/driver`、`tests/e2e/internal/testenv`、`web/tests/helpers/package.ts` | 当前生成/驱动分别启动应用、Web、多个项目，testenv 仍调用 run/stop-service。统一采用宿主入口；每个 fixture home 可只有一个逻辑项目，但不保留旧单项目进程入口。接入原 project-switch、smoke 及新 query-cache 场景，统一冻结/启动/清理所有者 |

CLI 现状另有明确传输缺口：`task_commands.go` 已通过 product.Client 办理批次操作，但 httpapi.command_registry 对 create_task/add_predecessors 明确返回 unsupported；`clientReader.Command` 的投影也不能完整还原批次回执。不得用“切换应用客户端”省略下列工作：

| 消费者 | 本次固定迁移安排 |
| --- | --- |
| task show/list/graph、discard、control、answer | 应用 v2 客户端选择项目，复用现有查询/commands、资格、request ID 与回执；退役公开 stop_service，不改成全局停止 |
| task validate/create/add-predecessors | 增加 Spec 指定的三个窄传输入口与两个原请求 GET 入口；调用现有 TaskCommands/事务。抽出原持久化 Command 结果的只读回执投影，不用 POST 重放查询，不新增领域流程/状态库 |
| check、home/install/launchers 与纯本机工具 | 保留其本机职责，不因本次重构改为业务远程调用；写项目业务库仍只有原项目应用用例拥有 |
| debug/helper、E2E testenv 及 Web 公共包 helper | 产品调用统一换为新命令/项目选择；模拟冻结仍由包级能力拥有，不保留 run/stop-service 或 v1 的测试备用通道；TUI 场景按原移除任务退役而非适配 |

用户明确 TUI 正在移除。调查快照中的 console/consoleapp 残留不作为本任务消费者；不为其增加适配、兼容、测试或新的依赖。实施同步当时主干的移除成果后处理本任务入口交叉，不重新引入 TUI，也不把其他聊天的移除工作列为本任务人工前置。Orca agent 终端与产品 TUI 区分。

本次无需新增通用服务容器、第二套业务注册表、跨项目任务总库、独立恢复引擎或应用到项目的网络桥。ProjectRuntime 为已有工厂/lifetime 的必要资源 owner；宿主目录只保存配置快照、句柄和生命周期投影。两者消费者分别是应用 host 与应用 handler。

## 2. 实施步骤与依赖

1. **固定契约及生命周期边界。** 定义 application/v2 路由/DTO、应用 ready 与项目状态、宿主/运行时实例归属、错误和 CLI start/status/stop 结果，包含 Spec 固定的 CLI 批次/依赖写入和只读原请求入口。将现有 Directory.Connect 替换为窄项目查询/命令端口；浏览器信封继续使用项目实例，不增加另一套业务回执。定义绑定原宿主的私有 shutdown.json 及逐项目关闭/资源结算结果，避免只看进程退出就假报成功。先用契约/handler 反例锁定错项目、旧实例、原请求结果未知和失败停机语义。
2. **抽取 ProjectRuntime。** 从 app 工厂和 host 单项目启动函数提取可独立建立、运行、正常停止、关闭及报告终态的 owner。每项目持有独立 context、actor、数据库、日志和锁；逐项登记失败前已创建资源并关闭。核对进程级单例、signal、cwd/环境、输出及 Windows Job 的所有者；进程级资源由宿主统一管理，项目资源显式注入，不能通过修改全局环境/目录或结束进程完成单项目操作。将身份/配置只读核验前置，取锁后才能初始化库，所有退出路径聚合清理结果；项目停止不依赖共享进程退出。保留冻结执行停止策略和原 worker 规则。
3. **实现多项目宿主。** 建立 home 单实例后固定 etc 配置快照，以共同 30 秒期限/最多 8 并发完成有界身份解析及冲突检查，未解析项失败，不逐项累加等待；随后有界并发启动无冲突项目。应用 API 先可用，目录展示启动/失败投影；坏项目、慢 preflight、缺执行环境分别隔离。默认 manual，禁止启动/重连自动产生新业务输入。停服先关闭全部项目新业务与新输入派发资格，再并行办理原请求/停止并聚合事实，原子写入绑定实例的 shutdown.json 后关闭端点。区分本地 actor/store/锁未关闭（仍持有且禁止替代运行时）与 worker 未知但本地已关闭（可重开原库取证，禁止替代 worker/新输入），某项目失败不跳过其他项目。
4. **收口项目 API 与 CLI。** 从原 httpapi 提取并复用查询/命令处理，接入宿主运行时端口，拆除单项目 HTTP 发布及应用回环客户端。CLI 用 `--project <project-id>` 选定项目，按上表完成固定操作的传输与只读回执投影；不再用“所缺能力后补”留下范围。CLI 专有路由独立匹配，不扩大 Web 共用 AllowedRead 或浏览器代理白名单；补浏览器尝试新路由的拒绝反例。原产品事务/幂等保持，CLI 不开第二写者，不用浮点对象重编码 payload。测试驱动在对应项目真实 ready 后正式校验/创建任务、添加依赖，不能由夹具直接写业务表。
5. **实现统一启动与可观测性。** start 为短协调命令，与 stop 使用同一协调锁且与常驻租约分离；按精确身份复用实例、建立进程创建事实、等待真实 API 就绪，不固定 sleep。落实 start 180 秒、stop 120 秒的整次协调上限（含锁等待）和 ready/degraded/未核实结果。区分“原精确进程已退出且租约可取”的 stale 记录与“创建/探测未知或仍持锁”的原尝试，前者可按归属清理并恢复、后者不得接管。Web/浏览器失败保留已就绪服务，重试仅补缺项。status 复用观察代码；stop 对 Web/宿主均尝试，并核验原宿主的逐项目停机结果、精确进程退出与租约释放，缺失/错误/旧实例结果返回失败。默认打开委托本 home 的 Web CLI open 短命令，定向测试记录实际命令路径/参数/home/退出结果，原 URL 与票据归属沿用 Web launcher 测试；业务 exe 不导入 web/server，不新增生产测试开关。提供脱离短启动器后仍存活的 Windows 句柄/Job 策略，保持 worker 的原资源归属。
6. **更新 Web 与启动器。** Web 自动连接 application/v2 并核验宿主换代/项目实例；目录能显示零项目、全失败、部分启动与重连状态。start.bat/stop.bat 固定安装 home、正确转义中文/空格/cwd、保留失败窗口和退出码；不要求用户分步启动。低层 Web CLI 保留独立重启。安装清理仅删除归属明确的旧启动器，未知用户文件保留；不现场迁移/重置数据，不为安装 home 设置私有目录 ACL。
7. **先补齐唯一生成器与驱动能力。** 调试生成器默认输出双 exe 及统一宿主拓扑，WebProjects 仅表示项目数量；未指定地址时分配空闲端口并固定，产品启动不漂移。包级 --simulate 保留唯一冻结入口，产品 start 不承担方案选择。复用正式 install/launchers 和 start --no-open/status/stop，核验两个运行中安装目标分别拒绝覆盖；按 exe 路径区分产品、helper 与 worker，不再为每项目派生产品进程。同步 test-web-smoke、test-debug-release、debug driver、testenv、Web 的 project-switch/smoke/query-cache 及公共 helper，禁止 run/stop-service 备用链。纯 Go 的既有隔离 testenv 可直接使用低层 serve 宿主及同一停止能力，不为组件验证强行构建 Web/引入 Node。

   新增稳定 controlled wrapper `scripts/test-unified-startup.ps1 -Package <新包> -Simulate [-Scenario unified-startup|query-cache]`，默认 unified-startup；未显式给 Simulate 时拒绝，不隐式选择模式，包本体的默认 live 保留。参数只映射既定文件。wrapper 只检查包边界、设置环境并调用对应 Edge 文件，不提前冻结/启动。扩展 `web/tests/helpers/package.ts` 统一承担 fresh 检查 → debug prepare --simulate 冻结 → debug run 调用统一启动/就绪 → finally 正常清理；project-switch 和 query-cache 复用同一所有者，不在各文件重复创建服务。freshDualPackage 要求无 scheme.json，必须在冻结前调用；ready 检查则验证同一已冻结 manifest/方案，不能再套用 fresh 条件。Go/Node/PowerShell 父调用期限分别覆盖产品 start 180 秒、stop 120 秒及必要通信余量，替换现有 30/45 秒通用超时，不能提前截断合法启动/收尾。此 wrapper 和新 helper 行为当前尚未实现，必须先补齐再验收。
8. **运行最小验证并统一交付。** 执行下表定向测试、受影响 fixture 回归、L1、双 exe 构建，以及不同新包中的统一启动和既有 query-cache 两个 Edge 场景；更新本任务 acceptance，不复用前序通过状态。同步运行指南、架构、应用/项目协议、project-management、前序 Spec 后继说明及 worker-lifecycle 的 W18/V06/V07/实现缺口。后续添加仍只消费宿主，不在本批实现其准入/登记。旧进程断言替换为新边界，保留事务、身份及 #323 查询反例。

步骤 1→2→3 为主依赖；步骤 4/5 在运行时所有权固定后接入，步骤 6 消费固定协议；步骤 7 必须在真实调试/自动验收之前完成。实现时只复用当前主干正式规则，不按此调查 commit 恢复已被移除的 TUI 或旧链路。

与后续项目添加的交接条件：应用 host、协议、目录和项目装载/停止端口已实现并验收，更新后的当前 commit 固定可读。它将接管成员准入；本任务不抢先实现登记或结构核验，也不为其新增热加载 API。

## 3. 一次实施的边界与阶段门

适合一次开发、一个 PR 原子交付。多项目所有权、API/CLI 和启动器是同一拓扑变更，拆成独立上线会产生半套消费者或被迫维护旧链；实施内部按下列四阶段逐段验证，不要求一次提交全部代码：

| 阶段 | 成果与继续条件 |
| --- | --- |
| A：运行时与契约 | 固定 v2/CLI 缺口及停机 DTO，抽出 ProjectRuntime；独立锁/实例/失败清理、冻结配置、两种资源停止策略定向反例通过 |
| B：宿主与消费者 | 多项目同进程、进程内路由、既有 CLI 全部固定操作接通；真实 CLI 创建/依赖及丢回执只读查证、旧 stop_service 拒绝、跨项目隔离通过 |
| C：统一入口与驱动 | start/status/stop、停机结果、launcher/生成器及全部有效测试消费者接好；审阅产品 exe 的旧 CLI 调用命中清单并逐项替换/退役（helper debug run 仍是有效包命令）；并发/部分启动、stale/未知分流、中文/空格、ACL、父子期限与隔离 fixture 回归通过 |
| D：交付验证 | L1、双 exe 构建、两个各用新包的 Edge 定向场景、文档静态检查通过，逐项登记 acceptance，人工体验允许后置 |

阶段不增加 Task ID、独立发布、兼容模式或另一套启动器。范围上限为 Spec 明列的现有 CLI 传输及生命周期整合；不扩展任务业务、TUI、项目添加、热加载、SSE、全局并发调度或 Worker 重试规则。必要的原请求只读投影和停机结果属于完成本次行为的最小能力，不能延期成“后续补验/补接口”。

## 4. 测试计划

所有自动要求链接[本任务专属验收](../acceptance/unified-startup.md)。新增 Go 场景统一使用 `TestUnifiedStartup` 前缀，生命周期子集使用 `TestUnifiedStartupLifecycle`，避免 Markdown 表内正则转义使复制命令零匹配。下表各组选定用例必须存在且命中非零；明确标注的编译检查不算场景通过。原始报告用 runner 新临时目录或新包 evidence，Plan 不记录实时结果。

| 验收项 | 级别 / 组 / 场景 | 环境及命令 | 时点 / 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| [UST-A1、UST-A2、UST-A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.host` | Windows 隔离 home/Git/SQLite 夹具；`go run ./cmd/test-feedback -- ./internal/app ./internal/app/host ./internal/localservice ./cmd/steerline ./cmd/steerline-web -run '^TestUnifiedStartup' -count=1` | A/C 阶段；双项目同 PID、独立 actor/store/context、身份解析总期限/配置快照/冲突、锁先于初始化、manual、单项目关闭、并发/半启动；已退出 stale 可恢复、未知不可接管、父子调用期限和失败诊断通过 | 对应 UST ID；summary、项目实例与锁/写入断言；Windows真实子进程只在隔离夹具中 |
| [UST-A3、UST-A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.api` | 隔离运行时和 HTTP；`go run ./cmd/test-feedback -- ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^TestUnifiedStartup' -count=1` | B 阶段；身份/认证/原命令/大整数；批次创建/依赖的版本和原 ID、丢回执 GET 查证零重放；旧 stop_service 拒绝且零全局停止；无项目 HTTP 回环、旧实例/不可用语义通过 | UST-A3/A5；runner JSONL/summary、调用/提交计数 |
| [UST-A4、UST-A5](../acceptance/unified-startup.md) | Go 定向 `unified-startup.lifecycle`，关联 worker V06/V07 | 隔离持久化 fixture 和 controlled 生命周期端口；`go run ./cmd/test-feedback -- ./internal/app -run '^TestUnifiedStartupLifecycle' -count=1` | A/C 阶段；两种冻结策略、正常/非正常换代零新输入/答案；本地 actor 未关闭保留锁、worker 未知但本地已关可只读重开；停机结果缺失/损坏/旧宿主/部分失败和超时返回非零，旧回执不串兄弟 | UST-A4/A5；原 Operation/Attempt、停机结果、进程/锁和副作用计数 |
| [UST-A3、UST-A6](../acceptance/unified-startup.md) | Web 定向、构建与依赖边界 | `npm --prefix web test`；`./build.bat --no-pause`；`go list -deps ./cmd/steerline`；隔离 home 的 launcher/ACL/依赖断言纳入 TestUnifiedStartup | C/D 阶段；保留项目/标签/原信封及 #323 查询反例；双 exe 刷新、业务依赖不含 web/server/嵌入资源、运行目标拒绝覆盖、旧 bat 删除范围/ACL/路径正确，不新增 TUI 支持 | UST-A3/A6；Vitest、双 SHA-256、依赖清单/边界断言与安装证据 |
| [UST-A1～UST-A7](../acceptance/unified-startup.md) | 真实 Edge / controlled `web.unified-startup`，改造原 project-switch 场景 | `./scripts/new-debug-release.ps1 -WebProjects 2`；`./scripts/test-unified-startup.ps1 -Package <本次新包 A> -Simulate` | D 阶段；包级 --simulate 冻结一次，统一入口、双项目同 PID、授权/选项目/两标签、CLI 正式创建/依赖/命令与原 ID 查证、Web 活动轮次重启、后端正常停启/授权门槛、原实例停机结果及清理完整 | UST-A1～UST-A7；manifest/hash、原身份/计数、停机结果、Edge screenshot/trace、cleanup JSON |
| [UST-A3、UST-A6、UST-A7](../acceptance/unified-startup.md) | 受影响真实 Edge 回归 `web.query-cache` | 同一生成器另建包 B；`./scripts/test-unified-startup.ps1 -Package <另一个新包 B> -Simulate -Scenario query-cache` | D 阶段；原 query-cache.spec.ts 经公共 helper 使用统一宿主，保留 #323 同键共享/有界预取/隐藏查询/定向失效和不同身份隔离，运行及清理成功；不复用包 A 状态 | UST-A3/A6/A7；原场景 trace/请求计数及独立 cleanup |
| [UST-A1、UST-A2、UST-A4、UST-A6](../acceptance/unified-startup.md) | 受影响 Go fixture 回归与 E2E 编译检查 | `go run ./cmd/test-feedback -- ./tests/e2e/internal/driver ./tests/e2e/internal/testenv -count=1`；`go test -tags=e2e ./tests/e2e/... -run '^$'` | C 阶段；适用的非 TUI fixture 实测通过，模拟冻结/所有权/启停正确；所有仍有效带 tag 场景编译通过，不留下删 CLI 后无法运行的入口；后者只登记编译，不宣称业务验收 | UST 对应项；runner summary、编译日志、有效/退役场景清单 |
| [UST-A1～A6](../acceptance/unified-startup.md) | L1 最小回归，新增小型 owner `quick-owner-unified-startup` | `go run ./cmd/test-feedback -level=L1` | 最终候选；在 scoped.go 登记零/坏项目、共享 PID 下实例停止、重复启动及跨项目命令隔离的少量关键反例；更新因旧进程模型失效的 owner，保留架构/事务守卫，剔除已退役 TUI 选择，不借机扩大全量 | 对应 ID 的回归摘要与 coverage，不能代替 Edge 综合验收 |
| 文档 | 静态 | `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | 规划及实现文档收尾；本地链接/Task ID/差异无错误 | acceptance 静态检查记录，与产品验收状态分开 |

两个浏览器场景分别使用全新生成包，沿用真实 Go/React/Edge、两个独立 SteerlineTest clone 和独立 controlled provider；禁止 linked worktree 假造两个项目，模型/PR 行为属于模拟，不能记为 live 或真实 PR 验收。已退役 TUI 测试/选择在其原移除范围内清理，不为使 fixture 回归通过而恢复旧入口。

### 必须补齐的综合驱动及取证

- 更新生成器为默认双 exe/统一宿主，WebProjects 1/2 均不再依赖显式 WebAddress 来启用 Web；维护模板与 manifest，不另写临时安装器。显式端口冲突在隔离测试中用本次 owned listener 制造，不占用/停止别人的服务。
- wrapper 只接受生成器 ready manifest 对应的新包；公共 Node helper 在 fresh 校验后调用唯一冻结入口，随后经 debug driver 使用本 home 的统一产品入口，wrapper 不提前写 scheme.json。先启动宿主后走正式 CLI 创建任务；活动轮次用原 controlled HoldStep/HoldStream 保持。新日志不能泄露认证材料。query-cache 使用同一 helper，不能保留独立旧服务准备/清理链。
- helper 将 manifest ready（静态包完整）、scheme frozen（执行方案固定）、产品 API ready 和测试目标项目 ready 分别核验。start 成功不代表两个项目已完成装载；两个场景均在共同 180 秒观察预算内等待所需项目按 config ID/真实 project ID 绑定且 ready，再创建任务/发命令，不用文件出现或固定 sleep 代替。该观察预算属于场景，不延长产品 start 的定义；失败时保留具体项目诊断并清理。普通项目 RPC 的父调用也沿用其现有期限并留通信余量，不能继续套通用 45 秒上限。
- 同一个新场景依次记录两项目共享后端 PID/创建时间、不同 project/runtime instance、独立 DB/Execution/Attempt，以及 worker 原进程/session/turn；正常重启 Web 后全部原项目与轮次不变，再释放并按原轮次结算。
- 在明确可核验的 controlled 活动窗口正常停止整个宿主，分别保存两项目的运行时和资源收尾；重开后验证新项目实例及零新派发，按正式继续授权检查适用的原会话恢复路径。stop_execution/continue_execution 的全部分支由上表定向 fixture 覆盖，不为穷举后端再生成多套浏览器场景。
- 场景通过条件在业务断言结束时冻结；正常停止本包 Web/宿主，聚合停止失败并单独记录 cleanup。核对本包产品进程及监听为 0，owned helper/worker 由驱动逐项结算；允许继续运行的外部资源按冻结策略另列，不混称全部已退出。场景上限 15 分钟，清理独立 2 分钟；预算和原失败在 evidence 保留，禁止失败后换包结果冒充同一次通过。

不选择 Full、全部 L2/L3、真实模型/PR 或远端矩阵：本批重构服务宿主与入口，采用真实 Windows 进程/ACL、隔离生命周期/受影响 fixture、一个真实 Edge 综合场景和一个实际改变启动驱动的既有 query-cache 回归；不重复独立 smoke 或无关浏览器矩阵。Worker 重试/业务流程规则不变。若实现改变其规则，须显式更新 Spec、统一生命周期文档和最小验证范围，不能默认为已覆盖。

UST-M1：自动通过后由用户体验双击启动/重复打开、故障提示、选择项目和停止反馈；参与窗口待安排，仅阻塞体验签收，不阻塞已通过自动项的交付。实施交付时同步 acceptance 的逐项已验/待验、源码版本/环境/命令和证据；本次生成文档仅登记静态检查，不执行产品验收或部署安装目录。
