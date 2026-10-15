# Plan：移除 TUI，保留 Web 交互前端

范围归 [Spec](tui-retirement.spec.md)，步骤及结果归[专属验收](../acceptance/tui-retirement.md)。2026-10-15 用户已授权实施；普通开发，无 Task ID，不改 execution-map，不创建执行聊天或独立 worktree。执行下列实施和验证安排；本任务不操作安装路径，不安装、替换或停止安装环境。

调查代码基线 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`。已读 `AGENTS.md`、`docs/spec/SpecGuide.md` 全文（SHA256 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`）、`tests/e2e/README.md`、`docs/testing.md`，核对现有 Web Spec/Plan/实现和 worker 生命周期的所有权及验证约束。本任务不改变 worker 节点/异常/恢复，统一规则继续链接[原文](../product-design/worker-lifecycle.md)，不复制矩阵。

## 1. 整合与前置

现有独立 Web、Go 应用目录、项目路由和模拟调试能力已经在本基线实现，直接消费它们；项目添加/纳管不作为本批前置。本次顺序为迁出共用代码、删除专属客户端、调整启动/测试、最小验证。不新建 bridge 或第二套状态。后续任何仍按旧路径实现的改动须采用本次删除后的 owner；不保留空包给并行开发兼容。

| 位置 | 处理安排 |
| --- | --- |
| `internal/console/activityrender`、`internal/app/console_activity.go` | formatter 和原 TestRenderActivity 迁至 `internal/product/activityrender`，更新服务组装引用；保留字符宽度处理和原输出 |
| `internal/product/httpclient` | 删除 Console/requestJournal/journal_lock/TUI observer 及专属测试；保留 client.go、timeout 与仍有消费者的公共测试，保留 lifecycle 平台文件内共享进程身份函数及其验证 |
| `internal/console`、`internal/consoleapp`、`internal/app/console.go`、`cmd/steerline/main.go` | 删除 TUI 及入口/只用辅助函数，清理引用和依赖 |
| `internal/app/application_directory.go`、`internal/projectconnection`、`internal/localservice`、`web/server` | 保留原发现/转接/实例核验；不把 consoleapp.Load 换名迁回应用目录，现有目录已独立实现 |
| launchers、`debug-template`、生成器、`driver/debug_release.go` / `debug_reset.go` / `debug_web.go` | 默认完整 Web 包、移除 TUI dispatch/状态处理、保留服务所有权与 reset 门槛 |
| runner/boundary、混合集成/E2E、`scripts/debug-runtime.ps1` | 移除 TUI 选择/动作，保留其他断言；旧脚本继续仅作隔离回归夹具，不变成日常调试入口 |

## 2. 实施步骤

1. **固定删除清单和测试归属。** 搜索全部 tags/平台源文件、脚本、模板和测试，记录 consoleapp/Console/journal/observer 的实际消费者。重点区分 `ProcessExited` / `CurrentProcessIdentity`、公共 recovery/decision-eligibility 路由、`run -console`、原生会话及服务可见窗口。逐个混合测试登记“删除 TUI 断言/保留并改接公共接口”的归属；不以全仓 console/TUI 字符串零匹配作为目标。
2. **先迁移共用，再删除客户端。** 原样移动纯 formatter 及测试，接回两处服务组装。删除 TUI 模型、app/console 组装和 CLI 入口；删除 HTTP Console 的本机 journal/锁及 observer 工厂。平台文件仅删除 observer 专属逻辑，保留共享进程函数及 import。基础 httpclient 与公共路由保持，确需更名只改内部文件/标识符。清理 TUI 专属单测、集成子项、go.mod/go.sum；`go mod tidy` 后核对剩余依赖的真实消费者。
3. **收敛安装/调试入口。** 删除两套 console.bat 和 helper 的 console dispatch；生成器 WebAddress 默认固定 18742，检查 Node/npm 前置，始终构建 Web 并返回 web 路径/hash。缺 Web 产物不得进入 ready，start 的完整包链不悄悄降为无前端；启动项目之前核对 Web 原生 ready，端口冲突或子进程退出立即失败并清理本次资源。保留 `build.ps1` / `-Web` 分工及 start/web 启动语义。更新所有生成器消费者，避免依赖旧 JSON console 字段、空 WebAddress 或可选 Web exe。仅验证项目服务的既有沙箱驱动显式使用原有 run-project；生成包仍完整包含 Web，不新增生成路线。退役 helper 操作在准备/冻结方案前拒绝。
4. **调整 reset 与受控包测试。** 删除 ui/views 枚举、锁、view.json 改写及模板空目录依赖；ui/web、application-api、项目/worker/runtime-launcher 的检查及业务重置范围保留。删去只为旧 TUI view 写入服务的 journal helper。扩展现有 `scripts/test-debug-release.ps1` / `TestDebugReleaseRuntime` 检查默认双产品、无 TUI、活动 Web/app/项目和 worker 门槛、检查模式无写、原任务/Git/PR 保留；覆盖单/双项目包。默认地址包仅核对生成产物，不启动可能属于其他环境的 18742；另建 19747 单项目与 19744 双项目运行包，核对端口占用拒绝；显式 -Live 校验另建未冻结单项目包，不能复用已选 simulate 的单/双项目包；正常停止所有本包服务和 launcher 后才能 reset，不把后台尚存当作超时后可忽略。
5. **整理混合测试及基础设施。** 按下表保留业务覆盖，清理 runner 硬编码、TUI 依赖扫描和无消费者 helper；Windows/Linux 生产依赖扫描继续覆盖非 TUI owners。同步新 formatter owner 的边界与定向选择，不引入宽泛白名单。为 held-turn 与 WebFeedback 组合补首次发布/后续复用原 PR 的最小夹具反例，按已有 PR 事实核对，不用不同调用路径中的 turns 长度判断首次发布。扩展原 Web smoke 时复用 controlled 的 `HoldStep: implement.execute` / `HoldStream`，等原生轮次已活动后关闭页面；用同一浏览器 context 的新页面/请求上下文核对原服务实例和 execution/attempt/session/turn，不依赖已关闭 page 的 helper。重开原目标后释放 hold，再继续原闭环，断言零重复启动/输入。所有 e2e-tag 场景仍能编译/枚举，零匹配和 skip 不算验收。
6. **更新文档并验证。** 修改当前启动/架构/测试入口；在 TUI 专属资料增加实际退役说明，修复源码链接，保留固定版本证据。运行下述测试计划并将实际源码、命令、证据、未验项写入专属 acceptance；Plan 不登记实时结果。安装相关回归仅使用临时夹具 home，绝不操作实际安装路径。

### 混合测试处理

| 位置 | 必须保留的断言及最小改法 |
| --- | --- |
| `cmd/steerline/native_task_test.go` 的 CM72、`internal/app/console_projects_test.go`、`projects_test.go`、`core_decoupling_66_test.go` | 删除 actual TUI/q/PrepareConsole 断言；CLI 任务与原回执、配置发现、冲突、运行身份和 launchers 由现有 CLI、目录、HTTP owner 测试覆盖。重复已覆盖部分可删除，独有要求须先转移，不能保留 LoadConsoleProjects 空壳 |
| `tests/integration/inline_console_test.go`、`candidate_console_test.go`、`core_decoupling_66_test.go` | 删除键位/渲染/界面组装；任务/候选/反馈/身份/原请求断言转到现有业务客户端/HTTP 接线测试。按最终 owner 选测试，不增加整套 TUI 替代夹具；新客户端接线只保留两个真实候选的唯一交互断言，五候选/第六次拒绝与原请求重放复用 execution 的 TestMC9 和 reservation 并发边界，不再额外启动五个原生 fixture |
| `internal/app/tui_http_boundary_test.go`、`client_boundary_test.go`、`internal/product/httpclient/capability_boundary_test.go`、`internal/testkit/boundary`、CD67/runner 反例 | 删除 Console/consoleapp 的规则与负例；HTTP consumer、application/localservice/projectconnection 的无 backend/SQLite 依赖等约束保留。移除单独 go list 已删除包的 Windows/Linux 闭包扫描，保留其余跨平台检查 |
| `tests/e2e/internal/driver/tui_http_test.go`、`scripts/test-tui-http.ps1` | 删除整个 TUI runtime 入口；原请求/交付/final 等仍有效要求由当前 HTTP/Web 与业务场景承担，在验收记录列出覆盖归属 |
| `testenv/recovery.go` 的 AssertRecoveryConsole、`suites/recovery/disconnect_test.go` | 删除 u 键/弹层和截图；driver 增加窄公共 HTTP recovery 查询，带原 run/execution/attempt，testenv 只消费 DTO。用 WaitRecovery 已核验的原快照作查询前证据，查询后经 driver 读取同一业务域快照，避免重复 CLI 枚举；保留两次检查前后业务不变、缺 thread 不给继续资格及原失败事实；复用 recovery.failure-results 场景；暴露既有 FinishBody 期限边界，在最后业务断言后冻结，正常停服作为独立收尾，不放宽业务期限 |
| `suites/nativeworkspace/removal_restart_test.go` | 删除重开 TUI；在删除后/重启后用现有基础 HTTP 查询补齐客户端可读及项目/数据库/原执行身份断言，保留真实工作区删除与无关任务启动的场景 ID |
| `suites/runtime/installed_debug_isolation_test.go`、`scripts/test-debug-runtime-isolation.ps1`、`scripts/debug-runtime.ps1` | 删除 TUI 批处理/ConPTY 和 ui/views 专属断言；保留源码/模拟安装 home 隔离、服务/CLI、链接越界、过期实例与跨 home 锁反例。launcher 文件反例改用仍有效的 service-window/Web 文件；场景内“安装版”仍用隔离夹具 |
| `testenv/console_windows*`、`console_other.go`、ConPTY helper 测试 | 消费者替换后复核：仅服务 TUI 的按键/退出驱动删除；通用可见服务窗口、Job/process 及原生工具能力保留，不按文件名批量删除 |

## 3. 测试计划

验收项统一链接 [TR-A1～TR-A6](../acceptance/tui-retirement.md#验收项)，以下是实施后必须执行的安排。普通 Go 用例使用临时 home/隔离夹具；产品调试仅由唯一生成器创建 `C:\SteerlineDebug` 新包，SteerlineTest、AI 显式 simulate。原始日志写 runner 默认临时证据目录或本次新空目录，acceptance 仅登记摘要与路径。

| 验收项 | 级别 / 组或场景 | 环境、命令与时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| TR-A1、TR-A6 | 静态/编译，退役清单 | 删除完成后：`go list -deps ./cmd/steerline ./cmd/steerline-web`；`go list -test -deps ./...`；审查 TUI 导入/命令/模板/runner 的 rg 结果；`./build.ps1`；`./build.ps1 -Web` | 当前入口无 TUI 依赖/失效包；普通构建仍无 Node 必需，双产品成功。保存命令/产物 hash、删除清单；历史与原生工具用语单列允许理由 |
| TR-A2、TR-A4 | L1 固定集与 formatter/HTTP owners | owner 调整后：`go run ./cmd/test-feedback -level=L1`；`go run ./cmd/test-feedback -- ./internal/product/activityrender ./internal/product/httpclient ./internal/localservice ./internal/product/applicationclient ./internal/product/applicationhttp ./internal/product/httpapi ./web/server -count=1` | 实际命中并全部通过；formatter 原测试、HTTP/实例/认证/原信封/共享进程守卫保留。runner summary.json 及原日志 |
| TR-A2、TR-A4 | 定向 CLI/app/混合接线 | 用步骤 1 的保留清单冻结精确 `-run`；现有前缀入口：`go run ./cmd/test-feedback -- ./cmd/steerline ./internal/app ./tests/integration -run '^Test(NativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs|PS1|Candidate|CD66|WebSwitch|InstallLaunchers|ListProjects)' -count=1`，改接后执行；已改名的独有断言须追加实际名称 | 精确命中保留/转移用例；保存用例清单、实际命令和报告，不能仅靠前缀命中其他测试算覆盖。范围不扩至整个 execution/integration 包 |
| TR-A4 | 候选数量/原请求 owner 回归 | `go run ./cmd/test-feedback -- ./internal/execution -run '^Test(MC9|CandidateStartReservationsRetainFailuresAndBoundParticipation|CandidateReservationConcurrentLimit)$' -count=1`；客户端接线改为两候选后执行 | 原五候选上限、串行/并发第六次拒绝及原请求重放由现有 owner 用例实际核对，不用删除覆盖替代精简。runner summary 与用例清单 |
| TR-A3 | 定向调试包生成/重置 | 步骤 4 先补入口能力，再运行 `./scripts/test-debug-release.ps1`（不传 Live）；扩展该现有脚本保留默认地址生成检查，并为运行中的各包分配明确不同端口，覆盖单/双项目 | 默认包三 exe/Web 入口存在、无 TUI 入口；冲突失败、冻结/锁门槛/reset 范围和清理通过。包内 generation-test.json、service-test.log 及新增最小 reset/退出证据 |
| TR-A4 | L3 controlled，recovery.failure-results | 公共 driver 改接后：`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.failure-results'` | 原失败/缺 thread/只读检查均通过；原执行及业务不变，零补发。scenario.json、查询响应及检查前后快照 |
| TR-A4 | L3 controlled，runtime.installed-debug-isolation；E2E 编译 | 隔离夹具调整后：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.installed-debug-isolation'`；`go test -tags=e2e ./tests/e2e/... -run '^$'` | 非 TUI 的安装/调试隔离反例实际通过；其余 E2E 编译不引用删除入口。运行证据与编译日志分开，不把编译声明为场景通过 |
| TR-A5、TR-A2 | Web 单元/构建、真实 Edge web.smoke | `./scripts/test-web.ps1`；最终代码生成新包并运行 `./scripts/test-web-smoke.ps1`。扩展原 smoke 的浏览器关闭检查：在交付前已有活动轮次时关闭页面，经公共查询核对原项目服务/执行仍活动，再打开原目标继续 | Web 单元/构建通过；任务/反馈/final/交付/离线及关闭页面不停止业务实际通过。smoke.log、browser trace/截图、原身份快照、smoke-cleanup.json，进程/监听为 0 |
| TR-A6 | 文档静态 | 文档完成后：`python scripts/check_doc_links.py`；`git diff --check` | 导航无失效本地链接，差异格式通过；历史未验不改为通过。结果写 acceptance |

`internal/product/activityrender` 为步骤 2 迁入的路径，相关命令在迁入后执行。其他既有脚本须按步骤补齐默认 Web/停止/reset 能力后再作为本批证据；不把旧脚本目前的能力写成已满足。

不选择 Full、完整 L2、live/真实 PR、全部 L3：本任务不改 worker/PR/资源行为。原生 `workspace.orca-removal-restart` 保留并编译，其实际原生删除链不因本次查询驱动替换要求再跑；替换后的公共查询由定向 HTTP/driver 与上述 controlled 场景核对。Web 多项目、原信封、应用实例重连由保留的 TestWebSwitch 与固定 Web owner 用例覆盖，本批不改目录/切换规则，不重复整个双项目浏览器综合场景；若实施发现必须改变这些规则，先更新本计划及对应验收范围。

## 4. 交付与当前文档检查

实施交付时逐项登记 TR-A1～TR-A6，列出保留测试的迁入位置、删除理由及未执行边界。只对实际删除的 TUI 专属验收注明退役，保留历史通过/失败事实。规划时仅建立 Spec、Plan、专属验收及导航；实施时同步当前 README/架构并执行上述验收，文档静态检查不代替产品验收。
