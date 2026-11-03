# Spec：测试早期反馈与专项范围收敛

日期：2026-11-03。状态：已实施，自动验收与独立审核通过。用户已确认本文与 [Plan](../plans/test-early-feedback.plan.md)，要求实施、不放宽断言、独立审核通过后提 PR；普通开发，不启动 roadmap。遵循[测试架构](../../SteerlineProjectGuideDoc/testing-architecture.md)与[项目测试指南](../testing.md)；实际验收另见[专属记录](../testing/acceptance/test-early-feedback.md)。

## 背景与目标

以 `bff9e6133336fcb4e55154cb06f5915ee1034f4e` 为规划基线。上一轮已统一正式选择及报告、补 Web 和材料生成 L3，并修复 Go Full 失败。main 合入后再次出现 L1 通过、完整边界失败：环境纯模型依赖 executionconfig 违反原矩阵，说明真实仓库架构检查需要前移。原证据保留在 [Full 修复与冲突验收](../testing/acceptance/go-full-repair.md)。

基线 `go.http` 仅执行 web/server、product/httpapi、product/httpclient 三包，快照、Web 配置和探测 owner 没有直接纳入。`go.environment` 执行环境两个 owner、remotetest 及完整 app，包含部署、Roadmap 等无关回归。历史 Full 执行1321.463秒，其中 app 包1005.155秒；该历史耗时只用于识别瓶颈，不作为本轮性能对照或当前版本通过证据。

本轮保持 L1–L4 定义与现有稳定 ID，完成三个成果：L1 能提前发现真实仓库依赖违规；go.http 对相关基础 owner 有直接测试；go.environment 精确覆盖环境目的，不再为其执行完整 app。减少无关执行，不减少原测试和断言。

## 范围与排除项

允许修改测试执行器的固定选择/校验、必要的测试及公共测试辅助、测试清单/指南/脚本/报告和对应 CI 接线。生产业务、Runner 协议、数据库、固定提示词正文/组装/黄金样本不变。若实现发现生产缺陷，单列证据及处理范围，不在本 Spec 下自行改变业务语义。

L2 仍为单一 smoke.delivery；L3 场景、并发及隔离策略不变；本轮不做 L3 并行扩张、Linux 执行矩阵、Full 分片/并发提速、重型场景改级、安装维护、真实模型/原生 Runner/远端/测试 PR。Full 仍执行全部普通 Go 测试，不用专项通过替代 Full。

本次实施必须同步项目指南、L1/L4 列表、test-list.json、执行器清单校验及脚本相关说明；既有 JSON 只使用已支持字段。E2E manifest 没有新增场景，不为普通 Go 集伪造 E2E 条目；现有 manifest 与清单一致性检查保留。

## S1：真实仓库架构检查进入 L1

在现有 L1 的 boundary owner 分组中复用 `TestThreeLayerRepositoryBoundary`，与已有提示词源码守卫一起执行。该测试调用现有完整仓库 Check，保留 Windows/Linux、构建标签和非活动源码的实际检查范围；不复制一套缩小矩阵。其矩阵反例、其他完整 owner 回归仍归 L4 go.full。

保留 L1 原 Go 与两份 Vitest 文件，不删除或替换原提示词门禁。结果明确列出被选中的仓库架构根测试及原始 Go 终态；它缺失、零匹配、失败或跳过时，L1 的 summary、退出码、managed 和完成值都不能通过。

验收要证明“新增分组实际调用现有完整仓库检查”而非只测试一份人工 fixture：在独立导出的源码副本加入本次曾发生的 model→executionconfig 非活动源码依赖，以正式 L1 实际失败及相应原始输出证明；不得污染当前源码或安装现场。正常源码正式 L1 完整通过。允许复用同次测试进程内已有扫描缓存，不允许跨源码版本复用成功报告。

整体一分钟仍为设计目标。按相同机器、工具链与缓存条件比较原/新 L1，记录三次全新输出的准备+执行总时长、中位数与最大值；冷运行另外注明。若未达目标，明确性能项未达成并优化重复扫描/编排，不能缩小扫描、删原断言、增重试或放宽超时伪装达标。

## S2：go.http 固定覆盖七个 owner

保留 ID、级别 isolated/L4 及正式命令 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http`，固定范围改为：

| 包 | 直接验证职责 |
| --- | --- |
| `web/server` | 原 Web HTTP 入口与集成断言 |
| `internal/product/httpapi` | 原 HTTP 服务端 owner |
| `internal/product/httpclient` | 原 HTTP 客户端 owner |
| `internal/product/httpfs` | 快照并发发布、同句柄校验、权限/链接/排斥及跨进程断言 |
| `web/server/executionconfig` | 条件保存、取消/并发、公开 schema 与原字段保留 |
| `web/server/discovery` | 本机/远端目录发现、身份选择、共享/取消/缓存与元数据调用 |
| `web/server/nativequery` | 元数据子进程的 cwd/程序解析、取消与资源结算、输出上界 |

每包执行全部普通 Go 测试，所有包必须非零测试、完整终态、零实际测试 skip。nativequery 目前无测试文件，实施时须先补 owner 行为测试，之后才能宣称七包直接覆盖；不得把编译项或其他包的间接调用冒充其测试。

nativequery 使用本测试二进制的子进程角色与独立目录，不需要真实 Codex/Orca/Runner：验证固定 home 为 cwd、拒绝相对路径及 cwd/PATH 相对目录劫持、正常输出、超过 Limit 的失败、取消后原进程及其所拥有子进程结算。各项按当前平台公开能力验证，不用条件 skip 通过；子进程角色用 TestMain/init 分派，不添加只在子进程才执行的空测试。保留 discovery 现有协议/业务调用断言，不复制到新包。

scripts/test-web.ps1 仍委托同一 go.http 定义；其说明改为上述范围。失败、零包/零测试、意外额外包、缺包/缺终态均使专项整体失败，managed 仍为 go.http。

## S3：go.environment 使用 owner 全量加 App 专项

保留 ID 与环境验证目的，显式更新定义及受管摘要，不静默继承旧报告通过状态。固定结构为两组，均串行、互不重叠，不改变现有并发预算：

1. **owner 全量组**：environmentadaptation、environmentadaptation/model、testkit/remotetest，新增 internal/executionconfig 的全部普通 Go 测试，验证中性目标和分组。不把前一组的筛选参数用于这些包。
2. **App 环境集成组**：仅 internal/app 的下列固定根测试及其全部子测试，不运行部署/Roadmap/其他无关根测试。全部原函数与断言保留原地，不为选择集合迁移生产代码或复制集成测试。

| 基线 App 文件 | 必选根数 | 验证目的 |
| --- | --- | --- |
| `environment_adaptation_test.go` | 3 | 资格依据、持久撤销、预留竞争、跨项目目标占用 |
| `environment_decoupling_test.go` | 7 | 项目材料、稳定依据、项目测试/业务文档不进入资格依据、依赖变更/删除、读取失败与基线门禁 |
| `environment_lifecycle_test.go` | 2 | 同构建重启结算与外来 namespace 拒绝 |
| `environment_materials_test.go` | 2 | 敏感信息边界与历史/移除目标 |
| `environment_readiness_test.go` | 7 | 配置独立及不触碰项目、未准备查询与启动校验分离、刷新门禁、未知回执保留、模型分组与跨上下文占用 |
| `environment_target_test.go` | 1 | 中性全局目标与项目上下文的接受边界 |
| `environment_workflow_test.go` | 1 | 公开流程、项目入口单次调用与原执行结算 |
| `environment_script_windows_test.go` | 1 | Windows真实脚本的工具检测、原信封身份、失败诊断与原子结果 |
| `shared_environment_test.go` | 1 | 共享材料、独立资格、单链失败隔离、公共依据变更持久撤销与拒绝旧目录回退 |
| `developer_test.go` 的 Options/Load/ReadinessService 三根 | 3 | 配置返回冻结策略、加载及运行中冻结/重启、服务准入与占用边界 |
| `project_maintenance_test.go` 的 `TestPMReservationRejectsPendingEnvironmentObservation` | 1 | 环境观察未完成时不得预留旧资格 |

规划与首轮7d85a9e9为21个App顶层根；同步main fd844789后，readiness文件新增两根，原21根全部保留，共23根；继续同步82935861后新增资格忽略项目测试/业务文档一根，该轮共24根；同步02285e1c后经独审加入开发者配置/准入三根，scope04共27根；scope05继承共享材料主干后新增上述Windows脚本和shared各一根，当前Windows29根、其他平台28根。实施定义使用明确根名，不能用 `Test.*Environment` 宽泛正则，也不能只取名称含环境的几个快测试。测试选择守卫无条件解析上述八个environment文件的全部声明根、附加根及正式定义一致；新增/改名/删除根、出现未登记的 environment*_test.go 文件时必须提示维护选择，不能悄悄漏掉。其他文件新增环境相关用例须经审阅加入附加根。

只为该组合补足现有执行器的分组能力：复用原运行、汇总、managed 和进度机制；以 `(package, root)` 核对每组选择，不能靠另一包同名测试代替。owner 包全量与 App 精确根分别检查期望包、每个根和子测试的唯一开始/终态；任一组失败、缺失、重复、skip、零匹配或出现未分配根，整体不能通过。定义仍来自同一包内固定 Go 结构，不新增独立调度器或第二份运行时清单。

旧 go.environment 中不再入选的 App 用例只是退出环境专项，全部保留在 go.full。实施前冻结当前普通测试清单，实施后按 `(package, root)` 比对：全部旧根都仍在 Full，新增 nativequery 用例也在 Full；不能通过改标签、删测试或缩小 Full 达到专项提速。

## 验收要求

| ID | 通过条件 |
| --- | --- |
| TEF-A1 | 正常正式 L1 通过；隔离副本中的真实违规令正式 L1 失败，原提示词/前端/矩阵范围不减。 |
| TEF-A2 | 正式 go.http 七包均有实际非零测试、完整且零跳过；nativequery 行为与资源清理有原始证据。 |
| TEF-A3 | 正式 go.environment 两组精确执行，完整覆盖当前App环境根（Windows29/其他平台28，原27声明根全部保留）及全部子测试，owner 包全量；选择漂移、缺根/包、额外根、重复/skip等负例不能通过。 |
| TEF-A4 | 旧普通根在 Full 无缺失，最终 Full 实际完整通过、0实际测试skip，编译项按既定规则单列；旧报告不冒充新定义通过。 |
| TEF-A5 | 最终 L1/L2、web.unit、受影响两项环境 L3完整通过且清理完整；清单、脚本与文档一致。 |
| TEF-A6 | 同机同条件记录原/新真实耗时：L1三次样本目标中位数≤60秒；环境专项一次配对实测目标较同源码旧完整包集至少减少40%，不外推中位数或稳定性。不承诺Full提速，未达目标明确登记原因与状态。 |
| TEF-A7 | 提示词/黄金正文差异与字节数、CI/L1选择变化有独立审核；确认发现修正后再进入实施交付。无新增人工验收。 |

正式列表随本轮实现同步，所有实际通过、未验与性能结果只写[验收记录](../testing/acceptance/test-early-feedback.md)，不把要求本身当作通过证据。

## 实施期主干同步

验收期间main合入PR #426/#428，scope02集成基线为 `fd84478926bea32fbfa687ece56e931470924faf`。本轮不修改这些主干业务实现，只将新增的配置轻量查询及未准备依赖查询两根纳入固定环境集合；所有原根和断言保留。首轮7d源码的Full/性能证据作为历史保存，新基线重新冻结普通根清单、运行配对/正式专项/Full，不跨版本继承通过或性能比例。L2/L3/HTTP及相关Web按集成影响补验，独立审核确认后提PR。


main随后合入#429/#431至 `82935861a23db22147e43082bd1357c1579a9234`，这是scope03的历史基线。新增 `TestEnvironmentQualificationIgnoresProjectTestsAndBusinessDocuments` 显式纳入，decoupling七根，七文件23根加PM一根共24。旧fd的Full主动取消且不记通过；新基线重新冻结普通591根并验证各受影响范围、配对及最终Full。已有完整断言/生产源码只从main继承，本轮不改其语义；独立审核确认选择及证据后提PR。


## S4：验收阻塞下的最小测试辅助去重与耗时证据

scope03 shared L3在300秒原业务期限耗尽；此前双项目任务/材料、资格及a候选都已成功，b候选仅剩约23秒，原失败保留。允许仅去掉MainCandidate在BuildHostConfig之前的重复ResolveIdentity；BuildHostConfig内部原解析、SubmitControlTargeted、配置/原生归属/能力/CanControl预检和原WaitControlReceipt全部保留。answer分支的OpenNativeTaskReader仍会解析其配置；不改变任何调用方字段、原ID/payload/版本、资格、场景步骤/隔离/并发、重启不重发断言及5/10分钟、150/145秒原期限。

命令原证据追加真实started_at/finished_at/wall_seconds，并在既有真实CLI transport测试中验证，保留原DTO断言，不新增空通过角色或伪造阶段。曾提议的Inbox改写不实施：它会省去本地配置变更等预检，且第三个configuration调用方也依赖原门禁；本轮不得复制门禁或用后端检查代替原检查。

该微小去重是否足够仍未知，不能宣称独立解决全部超时。另已观察到本机其他验收负载，先前只能证明本聊天任务串行，不能写整机独占。正式性能需记录整机并行条件；优先等待其他长测试结束，原期限不变。先独审补充再实施，在新目录验证原场景，不重试碰运气；如仍失败继续依据新增真实耗时定位。生产/协议/提示词/矩阵及全部原断言不变。

## scope04：主干冲突集成

提PR前预检发现实际文档冲突，安全保存实现后快进main至 `02285e1cb9b62564d37ee8a03a0f2f839af4c482`。保留主干开发者配置边界与本轮安排；三个Developer App根经独审纳入附加根，七环境文件23根、PM一根、Developer三根共27。SQLite策略根仍由原go.storage/Full覆盖。本轮不改主干生产、提示词、矩阵与CI；其字节审计以新HEAD重新计算。scope03全部结果作为旧源码历史，scope04重新冻结普通根并按影响验证，不继承通过或性能比例。正式长集按用户选择等待其他测试结束，保持全部原断言与期限。

### scope04计时证据时钟一致性

首次owner中原DTO断言及执行器/boundary通过，新增计时等式失败：真实UTC起止差0.1888422s，monotonic Sub为0.1888574s，差15.2微秒。保留owners原报告；独审批准仅将两次捕获改为time.Now().UTC()，wall_seconds与序列化时间戳统一真实墙钟，不取整/钳制，不改变1微秒一致性、正值、先后、DTO或任何期限断言。重新冻结源码，在新目录复验owner及新L1，再继续既定长集。

## scope05：共享材料主干的直接选择漂移

2026-11-03集成 `e5cd4f2a58a713eb1c769604ffabe733b336aa1f`：共享材料manifest5、原独立资格及App TestMain角色全部从main继承，不新增旧目录兼容或回退。原27根无改名/删除，新增Windows脚本一根及shared一根；八个environment文件24声明根+PM1+Developer3+shared1=29。固定定义维护Windows源码的GOOS属性，执行明确根名仅按该属性选Windows29/其他28，不用宽泛根名正则或条件skip；源码守卫全平台解析全部八文件和附加根，元数据错误、Windows源新增/改名仍必须失败。纯选择对照及错平台额外/缺根证据经过原完整性、managed和完成值检查。原四owner、HTTP七包和Full范围不减。scope04功能全部通过只作旧基线历史，scope05重新冻结清单并按直接影响验证；提示词字节相对新HEAD重算，固定正文/黄金不由本轮修改。

### scope05 Full：PID夹具发布窗口的最小修正

原Full在TestWindowsOwnedWorkerRunsInJobAndStopsWithOwner失败，platform_windows_test.go:110严格strconv.Atoi收到空字符串，root0.22s。helpers_test.go中spawn-owned-child在child.Start成功后os.WriteFile直接创建目标，再写正PID；父循环只要ReadFile成功即严格解析，形成可见空文件窗口。原失败保留，整套继续取终态后再实施。

仅拟将测试helper的PID发布改为同包现有AtomicWrite；不改生产实现、child.Start顺序、父ReadFile/Atoi/正PID、真实身份、Job成员和Close后树消失任何原断言，不改两段3s/20ms或加条件skip/重试。方案先独审；修正后完整host owner及该原根固定次数压力诊断，再新目录完整Full，不能用局部通过覆盖原失败。原有效旧参考/环境源码不包含host内部_test.go，其复用须核对依赖与源码差异后独审，不预先宣称同一全树。


## 最终实施与验收归档

本轮实现及TEF-A1至A6自动项已完成，实际命令、原失败与补验、范围/源码身份、提示词字节及受控单次性能结果统一见[验收记录](../testing/acceptance/test-early-feedback.md)。最终Full为72包620根1572终态0fail/skip，原612根全保留；最终L1外层中位14.437秒，环境专项一次对照减少67.198%。不放宽原断言、期限或Full范围，独立审核通过后提PR。
