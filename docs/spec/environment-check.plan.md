# Plan：执行权限与 Git 环境检查接入

日期：2026-10-16。SteerLine 实现安排与交付边界；按已合入 Runner `ea1bfd982c63818de02870c70fa2f7b1c9f258b2` 完成正式契约接入。[Spec](environment-check.spec.md) · [本仓专属验收](../acceptance/environment-check.md)。本 Plan 只安排 SteerLine；Runner 前置已拆到 `C:/Project/SteerRunner/docs/spec/environment-check.spec.md` / `.plan.md` 及独立 acceptance，无 Task ID、不改 execution-map。

## 1. 前置与复用

规范版本/调查基线见 Spec。实施时记录两仓实际 commit/dirty、Runner 固定 EXE version/SHA/resource_digest 和已公布 schema/支持组合；本地未提交规格不能冒充可获取的固定版本。本轮用户已授权 SteerLine 实现；未授权扩展 Runner 范围。

Runner 前置必须交接：Spec §4/§4.1 的三命令参数/CLI 信封/CheckReceipt/Inspection/错误码/报告 schema、同源 access_mode、实际支持目标、原生证据/认证及停止语义，以及其 Controlled 校验过的同版公开样例、对应验收和固定构建。真实集成时复用 SA 的最终提交必须合入并满足 SA 自身交付要求，当前有效凭据正例与 SA 所要求的 L2 smoke 未通过不能被本功能受控结果豁免。full_access 同权限预检由 Runner 本功能补齐，不反向要求 SA 新增全访问。SteerLine 不重写认证、不修复原生沙箱、不扩展 Orca 协议。

可以在固定契约的公开 fixture 上先写 caller/Web；实际集成必须消费已合入的 Runner 构建。Runner 某组合 unsupported 不阻断其他已支持组合或历史页，也不代替该组合真实通过。

复用 `runnerclient`、项目应用/SQLite、现有 HTTP 命令注册和 Query 缓存。`runnerbridge` 是 Candidate 用例，不直接套用；仅复用无 Candidate 语义的既有信封工具，不为这一次接入重构成通用 outbox/调度框架。

## 2. 实施步骤

### S1：公开契约接入

修改 `internal/runnercontract/`、`runnerclient/catalog.go`、`client.go`：按固定草稿消费 Check DTO，发布后核对 Runner 正式版本、start/inspect/stop 参数白名单、mutation 分类和 check_not_found 等退出码映射；遵循写/查询外层 request_id 规则。分别解析 start/stop 的命令回执、两种 inspect 选择和拒绝无 Check 的结果；受理后的检查失败作为正常报告解析，不能与命令/传输失败混淆。capabilities/schema 仍来自所选 EXE，不加载额外网络 schema。增加请求 ID/check ID/命令名/权威 input_digest/revision 错配及缺能力测试。

扩展 `internal/testkit/` 公开 CLI 子进程 fixture；交接前固定本仓拟议样例并明确其非原生/非 Controlled 来源，交接后核对并消费 Runner 同版 Controlled 已核验的请求/响应样例，覆盖检查快照与原 start/stop 请求；样例来源/契约/resource 摘要登记验收，变更时同步，不能两边各自手写不同信封。不模拟 Codex/Orca/权限状态机，不新增 events/cursor/cleanup 接口或私有 DB/日志读取。

### S2：应用与持久绑定

在 `internal/environmentcheck/` 增加小型 caller 用例，由 `internal/app/` 装配、`internal/runnerstate/` 保存纯记录、`internal/storage/sqlite/` 提供 CAS 关联/请求持久化。公开 API 投影在应用边界转换，不让 HTTP 消费方依赖进程客户端。检查 start/stop 写请求出站前保存用户原请求与冻结输入；原请求结果未知只查原 ID，重启不自动发送。保存固定 Runner 身份、check_id、公开输入和最近报告；原生事实仍由 Runner 单写，缓存不驱动执行。

复用项目 runtime 已有的原 Runner 实例/配置准入，只读查询不要求新输入资格。删除检查专用常驻服务启动，不允许检查重启 daemon 后使原实例停服/结算失配。原接口要求 Task/Candidate 的流程不作检查前置；复用项目身份/认证边界，不伪造 Task、Spec/Plan 或候选。设置/历史读取与运行就绪验证分开，不能经完整执行 preflight 才能打开页面。新增并发/迟到/本地提交失败用例，确认 Task/DAG/交付不变。

检查可预填方案或使用本次公开参数，但不写全局默认或运行中权限。普通执行继续使用 Runner 同一个 access_mode；本仓只透传公开字段，不生成原生权限 flags。stop 的新授权用于取消/原资源收尾，具体资格与实际清理由 Runner 判定。

装配项目实际 Repository/Publication，不能漏传发布要求或把项目业务 base 当测试 PR 目标。保存 Runner 首次确认的 Check input_digest，不重算其归一化算法；输入匹配校验按公开请求上下文。快照以单调 revision 更新；低版、同版冲突、缺字段、摘要与冻结必需清单漂移拒绝覆盖原可信值；JSON 整数校验不经 float64 舍入。原 start 尚无可信绑定时仅提供查询，不发猜测 stop，不因 request_not_found 换 ID 重发未知操作。

### S3：HTTP/Web

修改 `internal/product/api/`、`httpapi/`、`applicationhttp/server.go` 和既有客户端：项目范围命令 start/stop、检查索引/详情读取；沿现有认证/请求 ID/revision/项目路由守卫。原 request 查询只核验该 start/stop，不先查历史 start；detail 只查未知原请求或已绑定 check，不逐轮重查全部历史命令。detail 读取可更新本仓公开缓存，不触发检查/清理；同 revision 保留源时间，错误不把旧结果刷成当前成功。

新增 `web/src/features/environment-checks/`，在设置和项目上下文挂同一入口；使用既有 Query options/可见轮询/定向失效。实现最少字段、双模式显式选择、操作预览、进度/报告、历史适用性与资源待处理。取消和再次收尾使用原 stop 语义；关闭页面不发 stop，重复点击复用当前未完成授权。无通用 schema 编辑器、增量事件、自动重试或分项复查框架。

### S4：集成与文档

固定 Runner 已合入版本和真实 EXE 后核对 capability/schema/模式；缺能力只阻断对应检查。更新必要示例、debug-template 和新包参数传递，不能修改冻结包切换模式。同步执行配置与生命周期中的实际缺口/验证映射；本仓只记录 caller 与展示项，Runner 原生结果留其仓库验收。

## 3. 测试计划

本仓自动项 [SLEC-A01–A04](../acceptance/environment-check.md#自动项) 全部映射如下。新 `TestEnvironmentCheck*` 和 Web 文件为计划新增，实施步骤须落实并核实非零匹配；原始结果用入口新临时目录，acceptance 登记源码/dirty、Runner 版本/fixture、命令/用例数/证据位置。Plan 不维护实时结果。

| 级别 / 场景 | 自动项 | 环境、命令、时点 | 通过条件与登记位置 |
| --- | --- | --- | --- |
| 文档静态 SLEC-DOC | 文档整理 | 当前 Windows/Python/Git，交付前 `python scripts/check_doc_links.py`、`git diff --check`；新文档尾空白/冲突标记检查 | 无新增缺链/空白，登记本仓文档记录，不替代产品项 |
| 定向 caller SLEC-CALLER | [A01/A02](../acceptance/environment-check.md#自动项) | S1/S2 后；`go run ./cmd/test-feedback -- ./internal/environmentcheck ./internal/runnerclient ./internal/app ./internal/taskrunner ./internal/storage/sqlite ./tests/e2e/internal/driver -run '^TestEnvironmentCheck' -count=1`；固定草稿样例（正式交接前明确待核对）、CLI fixture、临时 SQLite；含 reset 未核实资源保护 | start/stop/两种 inspect/无 Check 拒绝、检查失败正常报告与退出码核验；完整 Publication、不把项目 base 用于检查 PR；权威摘要/单调 revision、缺字段/同版冲突拒绝；未知/重启/迟到正确，Task 不变；A01/A02 |
| 定向 HTTP SLEC-HTTP | [A01/A03](../acceptance/environment-check.md#自动项) | S3 后；`go run ./cmd/test-feedback -- ./internal/product/... ./web/server -run '^TestEnvironmentCheck' -count=1`；真实本地 HTTP、浏览器 proxy/深链与 CSRF、公开 fixture | 跨项目/认证/revision 拒绝；GET 零启动/检查/清理，缺 capability/运行离线历史仍可读；A01/A03 |
| Web SLEC-WEB | [A02/A04](../acceptance/environment-check.md#自动项) | S3 后；`npm --prefix web test -- src/features/environment-checks/environment-checks.test.tsx src/shared/api/client.test.ts src/shared/api/queries.test.ts`；`npm --prefix web run build` | 显式完全访问、预览、现场缺凭据与机制 unsupported 区分，源时间/离线缓存/未核实/清理/未测范围正确；未知无绑定/权威不存在均只查询、关闭页面不取消、重复点击不重派，选中详情的最新资源状态优先；A02/A04 |
| 实际 EXE 契约 SLEC-INTEGRATION | [A01](../acceptance/environment-check.md#自动项) | Runner 固定构建交接后；新增 `TestEnvironmentCheckPublishedContract`，通过 `go run ./cmd/test-feedback -- ./internal/runnerclient -run '^TestEnvironmentCheckPublishedContract$' -count=1` 运行，测试配置同时固定 `STEERRUNNER_EXE` / `STEERRUNNER_VERSION` / `STEERRUNNER_SHA256`；独立临时 state-dir | 实际 version/capabilities/schema 与 caller 及同版受控样例一致；必要时显式启动/停止空闲 Runner 服务取得 capabilities，不启动 Agent/Git。成功受理与原生流程归 Runner Controlled/native；本项只验 A01，不冒充 A02 的持久请求行为 |

实测模型、权限/认证、Git/PR 及原生停止由 Runner SREC-A01–A06 负责，本仓不复制四组合矩阵。现有完整 SteerLine L2/L3 与 Full 不默认选：未改 development 流程、TaskDelivery 和既有恢复；若实施触及这些共同规则，再据具体缺口选择对应既有组，不能用全量代替新增 caller 用例。

需要产品调试/人工体验时先由 `scripts/new-debug-release.ps1 -SteerRunnerExe <固定EXE>` 创建 `C:/SteerlineDebug` 新包；AI 模拟显式 `--simulate`，换模式新包。不使用/修改 `C:/SteerlineInstall`、`C:/SteerlineRuntime`，不复制其数据库/认证/配置。分级测试沿自身隔离夹具；真实 PR 验证只由 Runner 对 SteerlineTest 本次专属分支执行。

[SLEC-M01](../acceptance/environment-check.md#人工项) 在自动项通过后的维护者可用窗口体验设置入口、范围和错误可读性，可后置不阻塞无关工作；接口/权限失败不能转成人工待验豁免。

## 4. 完成条件

本仓 caller/HTTP/Web 验收、固定 Runner 契约核对、必要评审/合入及文档同步完成，才记接入完成；Runner 未支持的组合继续显示不可用并引用其前置。两仓各自登记结果，不共用一份验收。本轮实现 caller/HTTP/Web，但真实 Runner 契约核对与人工体验仍按前置安排，不得将 fixture 通过记为真实集成完成。


共享入口变更的最小回归：交付前通过 test-feedback 运行 `internal/product/httpapi` 的 TestHTTPCommandBoundary*、`internal/product/applicationhttp` 的 TestUnifiedStartup*、`internal/app` 的 TestUnifiedStartup*、SQLite 的 TestNativeDatabaseInitializesCurrentSchema / TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation、`internal/testkit/boundary` 的 TestThreeLayerRepositoryBoundary、driver 的 TestDebugResetBeforeInitialization*、taskrunner 的 TestRunnerShutdown*/TestRunnerSettlement*；Web 同跑 shared/api/queries.test.ts 和 client.test.ts，并做 TypeScript/Vite build。映射 SLEC-A01/A02/A03/A04，Windows 隔离临时目录；通过条件为旧请求仲裁、认证/路由、schema 严格拒绝和数据/进程依赖边界无回归，原始日志登记在 acceptance。本次不新增测试层级或全量执行要求。

## 4. 安装报错的契约对齐

根因是 caller 仍要求草稿 `version/supported`，正式 Runner 用 `contract/targets` 声明能力。同步替换 capability、schema 引用、报告、停止权限及 reset 资源完成条件；只改 caller/Web，不改 Runner 原生状态机。统一生命周期的节点/异常/验证规则不变。

测试计划补充（覆盖原 SLEC-A01–A04，证据仍登记本任务专属 acceptance）：

| 级别/组 | 验收项 | 环境、命令与时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 SLEC-CALLER/HTTP/INTEGRATION | A01/A02/A03 | 实现后运行原定向 Go 命令，包含 `TestEnvironmentCheckPublishedSamples` 与 `TestEnvironmentCheckPublishedContract`；先用 `scripts/new-debug-release.ps1 -SteerRunnerExe C:/Project/SteerRunner/bin/steerrunner.exe` 生成独立包，再固定该包 Runner 的三个环境变量。集成测试只启动临时 state-dir 的空闲服务，cleanup 停止原 instance | 实际能力可用；Orca unsupported 拒绝；六份原 inspection 完整往返/schema 通过；capabilities 中公开样例与固定交接完全一致；无 skip、服务清理成功。test-feedback summary/go-test.jsonl 登记 acceptance |
| L1 SLEC-WEB | A02/A04 | 原定 Web 三文件测试与 build，最终源码执行 | 读取原生 result/actual/cleanup_status/allowed_actions/证据 facts，原未知与重复操作保护继续通过；日志登记 acceptance |

不选择 L2/L3/L4 或真实 Agent/Git/PR：变更属于已发布协议消费与展示，Runner 原生执行规则未修改；本仓定向回归与实际 EXE 空闲服务足以验证本次差异。人工项仍为 SLEC-M01，不把后置体验记录为通过。不修改安装目录或冻结已有调试包。
