# Plan：移除 TUI，保留 Web 交互前端

范围归 [Spec](tui-retirement.spec.md)，步骤及结果归[专属验收](../acceptance/tui-retirement.md)。2026-10-15 用户已授权实施；普通开发，无 Task ID，不改 execution-map，不创建执行聊天或独立 worktree。执行下列实施和验证安排；本任务不操作安装路径，不安装、替换或停止安装环境。

调查代码基线 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`。已读 `AGENTS.md`、`docs/spec/SpecGuide.md` 全文（SHA256 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`）、`tests/e2e/README.md`、`docs/testing.md`，核对现有 Web Spec/Plan/实现和 worker 生命周期的所有权及验证约束。本任务不改变 worker 节点/异常/恢复，统一规则继续链接[原文](../product-design/worker-lifecycle.md)，不复制矩阵。

2026-10-15 审计交付接入主干 `317714e6`，保留已合入的双产品批处理构建、Web home 权限修复和查询复用。原基线证据继续绑定原版本；整合后补跑下文的最小检查，不把旧结果重新标为新基线通过。

## 1. 整合与前置

现有独立 Web、Go 应用目录、项目路由和模拟调试能力已经在本基线实现，直接消费它们；项目添加/纳管不作为本批前置。本次顺序为迁出共用代码、删除专属客户端、调整启动/测试、最小验证。不新建 bridge 或第二套状态。后续任何仍按旧路径实现的改动须采用本次删除后的 owner；不保留空包给并行开发兼容。

| 位置 | 处理安排 |
| --- | --- |
| `internal/console/activityrender`、`internal/app/console_activity.go` | formatter 和原 TestRenderActivity 迁至 `internal/product/activityrender`，更新服务组装引用；保留字符宽度处理和原输出 |
| `internal/product/httpclient` | 删除 Console/requestJournal/journal_lock/TUI observer 及专属测试；保留 client.go、timeout 与仍有消费者的公共测试，保留 lifecycle 平台文件内共享进程身份函数及其验证 |
| `internal/console`、`internal/consoleapp`、`internal/app/console.go`、`cmd/steerline/main.go` | 删除 TUI 及入口/只用辅助函数，清理引用和依赖 |
| `internal/app/application_directory.go`、`internal/projectconnection`、`internal/localservice`、`web/server` | 保留原发现/转接/实例核验；不把 consoleapp.Load 换名迁回应用目录，现有目录已独立实现 |
| launchers、`debug-template`、生成器、`driver/debug_release.go` / `debug_reset.go` / `debug_web.go` | 默认完整 Web 包、移除 TUI dispatch/状态处理、保留服务所有权与 reset 门槛 |
| runner/boundary、混合集成/E2E、`scripts/debug-runtime.ps1` | 移除 TUI 选择/动作，保留其他断言；删除无剩余消费者的旧脚本；隔离回归改由独立场景和唯一生成器承担 |

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
| 原 `suites/nativeworkspace/removal_restart_test.go` | 用户明确本机 Orca 路径已退役，删除整个本机专属场景/驱动/登记与 native 环境选项；远端 Orca environment 路径、受控协议测试和证据规则保留 |
| `suites/runtime/isolation*_test.go`、旧 debug-runtime 两份脚本 | 删除旧综合场景与两份专属脚本。保留构建输出、路径/SQLite、服务写保护、双 home 的四个独立场景；新调试包由唯一生成器测试覆盖。旧 UI/wrapper/源码 exe 回退等已删除入口断言随入口退役 |
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
| TR-A4 | L3 controlled，runtime 四个独立隔离场景；E2E 编译 | 隔离夹具调整后：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled`；`go test -tags=e2e ./tests/e2e/... -run '^$'` | 四个独立隔离场景实际通过，零 skip、每项资源清理成功；核对原始 Go JSON 至少两项执行区间重叠；其余 E2E 编译不引用删除入口。运行证据与编译日志分开，不把编译声明为场景通过 |
| TR-A5、TR-A2 | Web 单元/构建、真实 Edge web.smoke | `./scripts/test-web.ps1`；最终代码生成新包并运行 `./scripts/test-web-smoke.ps1`。扩展原 smoke 的浏览器关闭检查：在交付前已有活动轮次时关闭页面，经公共查询核对原项目服务/执行仍活动，再打开原目标继续 | Web 单元/构建通过；任务/反馈/final/交付/离线及关闭页面不停止业务实际通过。smoke.log、browser trace/截图、原身份快照、smoke-cleanup.json，进程/监听为 0 |
| TR-A6 | 文档静态 | 文档完成后：`python scripts/check_doc_links.py`；`git diff --check` | 导航无失效本地链接，差异格式通过；历史未验不改为通过。结果写 acceptance |

`internal/product/activityrender` 为步骤 2 迁入的路径，相关命令在迁入后执行。其他既有脚本须按步骤补齐默认 Web/停止/reset 能力后再作为本批证据；不把旧脚本目前的能力写成已满足。

不选择 Full、完整 L2、live/真实 PR、全部 L3：本任务不改 worker/PR/资源行为。原生 `workspace.orca-removal-restart` 保留并编译，其实际原生删除链不因本次查询驱动替换要求再跑；替换后的公共查询由定向 HTTP/driver 与上述 controlled 场景核对。Web 多项目、原信封、应用实例重连由保留的 TestWebSwitch 与固定 Web owner 用例覆盖，本批不改目录/切换规则，不重复整个双项目浏览器综合场景；若实施发现必须改变这些规则，先更新本计划及对应验收范围。

## 4. 交付与当前文档检查

实施交付时逐项登记 TR-A1～TR-A6，列出保留测试的迁入位置、删除理由及未执行边界。只对实际删除的 TUI 专属验收注明退役，保留历史通过/失败事实。规划时仅建立 Spec、Plan、专属验收及导航；实施时同步当前 README/架构并执行上述验收，文档静态检查不代替产品验收。

## 5. PR 前审计复验安排

对应 [TR-A1/A2/A4/A5/A6](../acceptance/tui-retirement.md#验收项)，接入 `317714e6` 并修复审计发现后执行：

| 范围 | 命令、环境与时点 | 通过条件与证据位置 |
| --- | --- | --- |
| TR-A1/A2/A4，固定 L1 | `go run ./cmd/test-feedback -level=L1`；当前审计代码、隔离临时夹具 | runner passed/complete，边界与 owner 实际命中；summary 写 acceptance |
| TR-A2/A4，CLI 原回执与共享 formatter/HTTP 进程实例 | `go run ./cmd/test-feedback -- ./cmd/steerline -run '^TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs$' -count=1`；以及 formatter/localservice/applicationclient/applicationhttp、controlled helper、deadline 的原有最小 owner 用例 | 恢复非空摘要断言后 CLI/HTTP 原请求与连接关闭不停止服务通过；原 owner 通过，记录精确命令和 runner summary |
| TR-A4，原生 workspace 仅编译 | `go test -tags=e2e ./tests/e2e/... -run '^$'`；HTTP 就绪/实例核验调整后 | 全部 E2E 场景编译，保留“未重跑真实 Orca 工作区”限制；编译日志与场景通过分开登记 |
| TR-A2/A5，Web | `./scripts/test-web.ps1`；随后 `./scripts/test-web-smoke.ps1 -WebAddress 127.0.0.1:19748` 创建新 simulate 包，串行真实 Edge | 最新主干整合后的 Web 单元/构建与 smoke 通过、关闭浏览器后原执行保持、收尾进程/监听 0；新包日志/trace/cleanup 写 acceptance |
| TR-A6，文档与差异 | `python scripts/check_doc_links.py`、`git diff --check`；最终文档完成后 | 0 错误，记录范围；旧失败与待验不改为通过 |

runtime 综合隔离包含从 Git HEAD 克隆源码进行构建的检查。前次仅有 dirty 工作树时，该子检查仍绑定旧 HEAD；本轮先提交已审计代码与安排，保持工作树干净，再用 `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled` 对实际 PR 提交执行一次验证，所有夹具仍为临时 home，保留原 10 分钟期限和全部非 TUI 断言。新 runner summary/scenario/cleanup 写入专属验收，旧失败保留。该项仍未通过时以草稿 PR 明确列为待验，不凭其他检查代替通过。


## 4. 独立隔离回归与补验安排（2026-10-15）

用户授权删除旧综合场景，保留长期必要隔离回归并安全并行。删除 `runtime.installed-debug-isolation` 及其仅用旧脚本的冗余检查，不用新结果覆盖旧超时记录。四个新场景每项仍为普通 controlled 5 分钟业务期限，Go 父期限保留每项 6 分钟余量；并发排队不计入场景期限。只有已审查的 runtime controlled 包允许 `-parallel=2`，其他包、live 和 L2 顺序不变。各项自有 home/仓库/进程/端口/provider/证据，runner 构建产物只读；构建场景只写自己的源码 clone，不再改工作树 bin。所有业务 I/O 完成后冻结 FinishBody，再由独立 cleanup 核实资源清理。

| 验收项 | 场景 / 环境 | 最小实现与通过条件 | 命令 / 时点 / 证据位置 |
| --- | --- | --- | --- |
| [TR-A4](../acceptance/tui-retirement.md#验收项) | runtime.source-build-isolation / L3 controlled | 独立源码 clone 的 bin junction、输出硬链接拒绝；普通构建/替换保持另一临时 home 的程序/hash、DB、业务事实不变；源码/构建身份正确 | 完成改造且提交当前源码后，`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled`；runner summary 与各场景 runtime-identities.json，结果登记专属 acceptance |
| TR-A4 | runtime.path-isolation / L3 controlled | state junction、run_dir/worktree_root 越界、SQLite DB/WAL/SHM 硬链接实际拒绝，无越界写；解除链接后原生校验恢复；无业务 worker | 同上；state/SQLite 反例日志、SQLite 字节摘要与 scenario.json |
| TR-A4 | runtime.service-write-isolation / L3 controlled | 原生 service-window 启停/同构建重启、子进程身份；旧实例/改配置未发布请求；跨 home 仓库锁有效；不跑完整交付 | 同上；窗口日志、控制回执/计数、前后快照与 scenario.json |
| TR-A4 | runtime.dual-home-isolation / L3 controlled | 唯一双环境业务检查，两独立执行/原请求/交付，A 合并时 B 保持原状态、B 交付不改变 A；完整通用交付规则复用已有 delivery.basic | 同上；两侧快照、请求身份/摘要和 scenario.json |
| TR-A3、TR-A4 | 当前新调试包生成/运行 / controlled | 继续使用已有 test-debug-release 唯一流程，验证已生成包互不串用、固定端口、Web、锁/reset、冻结方案；生成阶段串行，因为共享 npm ci/dist，运行各包沿现有所有权清理 | `./scripts/test-debug-release.ps1`；四场景通过后执行；新包 generation-test.json/runtime-test.log，登记 acceptance |

| TR-A4、TR-A6 | runner/编译/文档静态 | 新登记精确选择，旧 ID 拒绝，安全并发白名单/期限不放宽；所有 E2E 编译、依赖边界、链接无失效 | `go run ./cmd/test-feedback -- ./cmd/test-feedback -run '^Test(E2E|Level)' -count=1`；`go test -tags=e2e ./tests/e2e/... -run '^$'`；L1、文档检查按实际修改范围执行；日志路径登记 acceptance |

不重跑 Full、全部 L3/live 或真实模型；本次未改变产品 worker 行为。原 Web Edge 闭环与 recovery.failure-results 已在最终产品实现上通过，本次仅重构测试组织，不重复它们；若补验发现产品修改，再选择受影响回归。无新增人工项。

用户随后明确：本机 Orca 路径已经退役，远端 Orca environment 路径仍保留。因此删除 workspace.orca-removal-restart 的登记、专属驱动/证据字段与 native 环境选项，当前任务不再补跑本机原生删除链；既有远端实现、单终端受控协议测试和原始证据保留，不以本机测试替代远端验收。此前混合测试表中的工作区迁移安排随退役取消；旧记录只保留历史。

远端保留的必要定向回归映射 [TR-A4](../acceptance/tui-retirement.md#验收项)：改造后运行 `go run ./cmd/test-feedback -- ./internal/app ./internal/execution/adapters/orcatransport -run '^Test(TerminalRegistrationRejectsRetiredRoutesAndAgentPrivateFields|RemoteRepositoryDiscoveryRejectsBeforeCreation|RemoteRepositoryCreationRetainsReceiptOnMismatch)$' -count=1`；受控 fake 验证退役目标拒绝、远端仓库前置拒绝/原回执保留，无本机 Orca 运行或真实模型。runner summary 及原输出登记专属 acceptance，不当作远端真机验收。
