# Spec：移除 TUI，保留 Web 交互前端

后继拓扑由[统一启动](unified-startup.spec.md)替换：项目改为同一后端进程内运行时，使用 start/serve/stop/status 与 application/v2；本文保留原任务范围和历史调查，当前启动方式见统一启动文档。

状态：2026-10-15 已按用户授权实施；所选验证中运行隔离场景超时待验，其余结果与限制见专属验收。普通开发，不分配 Task ID、不修改 roadmap 或 execution-map。配套 [Plan](tui-retirement.plan.md)及[专属验收](../acceptance/tui-retirement.md)。

## 1. 目标与范围

删除 SteerLine 自身的交互式终端控制台及其专属测试，让现有 Web 成为唯一交互前端。保留后台服务、运维/任务 CLI、原生 Codex/Orca 会话及业务 HTTP 能力。本任务是退役客户端，不重新设计执行链路，也不要求将每个 TUI 操作复制到 Web。

现有路径 `浏览器 → steerline-web → 应用服务 → 项目服务` 继续承担 Web 交互。业务事实、权限、原请求、数据库和 worker 仍归原所有者；不增加服务、适配框架、数据迁移或 TUI 回退入口。

## 2. 审计发现与方案修正

调查基线为 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`，调查时工作树干净。

| 现状与证据位置 | 审计结论及本次处理 |
| --- | --- |
| `cmd/steerline/main.go`、`internal/app/console.go`、`internal/consoleapp`、`internal/console` | 入口、组装、偏好/窗口锁、界面及 TUI 专属测试一起删除，不能只删除界面目录 |
| `internal/app/console_activity.go`、`app.go`、`execution_service_wiring.go` | 后台服务仍调用 TUI 目录内的纯活动格式化器；先迁出 formatter 与原测试，再删除 TUI 目录 |
| `internal/product/httpclient/console.go`、`request_journal.go`、`journal_lock*` | 可重连 Console 适配器及本机原请求日志的生产构造入口只有 consoleapp；随 TUI 删除。基础 `Client` 及 Web 原信封机制继续保留 |
| `httpclient/lifecycle_windows.go`、`lifecycle_other.go`、`internal/localservice/endpoint.go`、`app/application_directory.go` | `CurrentProcessIdentity`、`ProcessExited` 被应用/Web 服务复用；只删除 TUI 停服 observer，保留真实进程代际核验 |
| `scripts/new-debug-release.ps1`、`driver/debug_web.go` | 当前 Web 由非空 WebAddress 选择；默认包没有 Web。改成默认完整 Web 包，避免删除 TUI 后默认调试包无前端 |
| `driver/debug_reset.go`、`debug_release_test.go` | reset 读写 TUI 的 ui/views；移除该依赖，同时保留活动 Web/应用/项目/worker 的阻止重置条件 |
| `testenv/recovery.go`、`suites/recovery/disconnect_test.go` | 恢复场景夹带 TUI u 键；替换为公共 HTTP 断言，保留原失败和原执行只读检查。本机 Orca 工作区场景已由用户明确退役，删除专属驱动/登记；远端路径保留 |
| `internal/testkit/boundary`、`cmd/test-feedback/scoped.go`、旧源码调试脚本（本次删除） | 有硬编码 TUI 包、跨平台依赖闭包、隔离测试入口；同步清理，保留 HTTP/存储/执行域和安装隔离守卫 |

以上是代码审计结论，不表示已实施或已验证退役后的产品。

## 3. 删除与保留契约

### 3.1 删除 TUI 客户端

- 删除 `steerline console`、对应帮助/参数处理和只为它服务的 TTY 检查；该命令按现有未知命令规则失败。不提供别名、自动跳转或兼容 stub。
- 删除 TUI 模型、键盘/布局/阅读器、会话选择、偏好、ui/views 窗口锁与持久化代码，以及 consoleapp、PrepareConsole、RunConsole、LoadConsoleProjects。
- 删除安装 launcher 和调试模板的 `console.bat`、debug helper 的 `debug console`、旧隔离夹具脚本的 console 操作，清理生成器结果中的 console 字段，返回真实存在的 Web 入口。
- 删除 TUI 专属 HTTP Console、requestJournal、请求文件锁和停服 observer；新程序不再读取或创建 `ui/views`、`ui/http-requests`、`ui/http-request-locks`。不扫描、删除或迁移旧安装/调试包的数据。
- 按真实剩余导入清理 go.mod/go.sum；Bubble Tea、Lip Gloss 等仅服务 TUI 的依赖退出。不得为了追求依赖数量归零而替换服务仍使用的字符宽度等小型库。

### 3.2 继续保留的能力

| 能力 | 退役后的归属与要求 |
| --- | --- |
| 任务/DAG、执行方案、任务/候选启动、反馈/答案、final、候选停止、项目切换 | 使用现有 Web 页面与既有业务 API；不改变准入或原请求语义 |
| 项目/应用/Web 服务启动与正常停止、任务管理/诊断 | 保留现有 CLI、service-window 及 Web/app launchers；Web 浏览器不新增服务管理权限 |
| 服务日志 | 保留 `run -console pretty/summary/json/off` 及服务窗口。活动 formatter 迁往 `internal/product/activityrender`，保持原输出和输入值合同 |
| 原生会话与 Windows 控制台能力 | 保留 Web 的 open_native_session、Codex/Orca adapter、服务窗口及进程/Job 所有权；删除 SteerLine TUI 不涉及原生工具自身 TUI |
| HTTP 查询/命令、安全边界与进程身份 | 保留基础 Client、API、applicationclient、projectconnection、localservice；包括现名 console_routes.go 内的 recovery/decision-eligibility 公共路由，必要的代码更名不改协议 |
| 草稿与原命令 | 保留 Web IndexedDB 隔离、原 ID 查证、实例变化禁补交及服务端回执；不将 TUI 本机请求日志导入浏览器 |

TUI 键位、三窗布局、本机视图偏好、u 键恢复检查弹层及控制台停服交互退役。Web 当前已有恢复诊断和 Decision 回答，但没有等价的 u 键详情弹层；本批不新增。恢复检查的公共 API 保留，相关业务断言通过公共接口核验。Web 关闭仍不停止项目服务或 worker。

worker 流程、异常、重试和恢复规则遵循[统一生命周期](../product-design/worker-lifecycle.md)，本次规则差异为零。替换恢复测试入口不改变 W/E/V 矩阵或当前实现缺口；不新增另一套恢复判断。

## 4. 默认调试包与重置

`new-debug-release.ps1` 默认生成业务/Web/helper 三个 exe 和 Web 启动器，WebAddress 默认 `127.0.0.1:18742`，显式地址仍可选择另一个固定 loopback 端口；不再用空地址表示无 Web。保留 WebProjects 1/2，双项目各自拥有原仓库与执行身份。默认构建 Web 的 Node/npm 前置在生成前检查，缺依赖明确失败，不产出 ready 的缺前端包。

`start.bat [--simulate]` 继续启动本包项目、应用和 Web 服务，`web.bat` 只授权打开本包已启动的 Web。保持默认 live、AI 显式 simulate、首次方案冻结；不自动打开浏览器或修改执行方案。端口占用明确失败，不自动寻找端口。多包测试为各包配置不同固定端口并在收尾停止各自服务。

源码普通 `build.ps1` 仍保留无 Node 的业务构建；`build.ps1 -Web` 构建双产品。唯一调试包生成器默认调用 Web 构建，不另建生成路线，也不要求后台构建强制带前端。

reset 不再要求 ui/views 存在或处理其 view.json。保留本包所有活动进程/锁检查、检查模式不写入、任务定义/契约/依赖/Git/PR 事实保留和原状态重置范围。不能移除整个 ui 目录：应用/Web discovery 与锁仍有效。浏览器 IndexedDB 不由包内 reset 跨进程清除，沿现有实例和业务版本守卫处理；本批不扩展 reset 权限。绝不使用、修改或重置 `C:\SteerlineRuntime`。

## 5. 测试与文档退役规则

TUI 专属渲染、键盘、窗口偏好、TUI 客户端日志/observer 测试、TUI HTTP runtime 和脚本直接删除。混合测试逐项判断业务要求：保留任务/候选、原回执、权限、身份、恢复只读、安装隔离断言；移到当前 owner 或直接使用既有公共客户端，不保留测试专用 TUI 模拟器。

清理 runner 固定包、精确 subtest、权重及架构守卫中的 TUI 选择。保留现有非 TUI 反例和严格失败语义；不能以缺包跳过、空选择或删除全部边界测试使检查通过。只在无剩余消费者时删除 TUI 专用 ConPTY 按键驱动；服务可见窗口、Job 与原生终端测试继续保留。

当前 README、运行/调试、Web、架构和测试说明改用 Web 入口。TUI 专属 Spec/Plan/设计/验收保留历史事实并在当前入口标注退役；不把原未验项记为通过。共享 API/业务要求继续有效。实施时修复删除源码造成的文档失效链接，历史源码取证使用固定 Git 版本；不改写旧验收命令为本次证据。原规划阶段只新增导航；实施阶段同步当前入口与退役说明。

## 6. 自动验收

具体操作和实时结果归[专属验收](../acceptance/tui-retirement.md)，Plan 覆盖全部条目。

| ID | 通过条件 |
| --- | --- |
| TR-A1 | 当前生产/测试入口不再构造或启动 SteerLine TUI；TUI 专属代码/测试/脚本/依赖退出，普通业务构建及 Web 双产品构建通过 |
| TR-A2 | 活动格式化行为保留；基础 HTTP、原请求/认证、公共恢复路由、共享进程代际/正常停止和原生会话能力不因目录/代码删除失效 |
| TR-A3 | 默认生成完整 Web 包；显式端口和双项目仍可用；首次方案冻结、活动服务阻止 reset、静止后按原范围重置，独立包资源正常清理 |
| TR-A4 | 混合业务断言与非 TUI 边界保留，L1 和受影响定向检查实际命中；恢复失败场景用公共 HTTP 证明检查只读，其他登记 E2E 编译通过 |
| TR-A5 | 新独立 simulate 包真实 Edge web.smoke 通过：现有任务/反馈/final/交付/离线行为可用，浏览器关闭不停止业务，收尾本包进程/监听为零 |
| TR-A6 | 当前文档和测试导航无已删除的可执行入口/失效本地链接；历史结论与本批新验收分开，未执行内容不写成已通过 |

本批不新增阻塞人工验收项。既有 Web 人工体验仍归原任务，原 TUI 专属未验项在实际删除后标记退役、不再适用，不能改为通过；与后台能力有关的未验要求仍归原记录。


2026-10-15 用户确认的验证范围调整：旧综合 `runtime.installed-debug-isolation` 与旧源码调试 wrapper 退役；隔离保证由 `runtime.source-build-isolation`、`runtime.path-isolation`、`runtime.service-write-isolation`、`runtime.dual-home-isolation` 四项独立 controlled 场景及唯一新调试包测试承担。安全独立的四项最多并发二项，不共享可写构建产物；每项必须实际通过并清理。旧超时证据保留，不能仅删除旧场景即宣称验收完成。

本机 Orca 工作区路径按用户明确指示退役，不再要求本机删除/重启验收；远端 Orca environment 单终端实现与受控协议验证保持。
