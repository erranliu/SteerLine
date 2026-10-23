# TUI 退役与 Web 唯一交互前端验收

专属普通开发任务：[Spec](../../spec/tui-retirement.spec.md) · [Plan / 测试计划](../../plans/tui-retirement.plan.md)。2026-10-15 已按用户授权实施；随后按用户确认拆除旧综合场景，四项独立隔离回归均实际通过。旧超时证据保留；本机 Orca 路径明确退役，远端路径保持。无 roadmap Task ID。本记录只归本任务，既有 Web/TUI 结果不直接算本批通过。

## 验收项

| ID | 操作与通过条件 | 当前结果 / 证据 |
| --- | --- | --- |
| TR-A1 | 对照删除清单检查 CLI/包/模板/调试分派/runner/依赖；普通业务构建及 Web 双产品构建，无当前可执行 TUI 入口 | 通过；源码/依赖检查、无 Node 业务构建、双产品构建及新包三 exe/hash 通过 |
| TR-A2 | 运行迁移后的原 formatter 测试与基础 HTTP/localservice/app/Web owner 回归；确认日志、实例/认证/原信封、共享进程函数和公共 recovery 路由保留 | 通过；formatter 内容未变，基础 owners、CLI/候选接线、HTTP 恢复及 Web smoke 通过 |
| TR-A3 | 用生成器默认参数及显式不同端口创建新单/双项目包；核对默认 Web 产物、端口冲突拒绝、方案冻结、活动服务/worker 的 reset 拒绝、check 不写、静止后重置范围及全部退出 | 通过；最新独立Runner新包2364dc55，generation-test.json passed=true，19747单项目81.57秒、19744双项目77.94秒，运行/停止/reset通过，进程与监听均0 |
| TR-A4 | 固定 L1、定向混合测试、controlled recovery.failure-results 和用户确认替换的四项 runtime 独立隔离回归实际通过，所有剩余 e2e-tag 场景编译通过；保留原断言覆盖归属、查询前后状态与旧失败证据 | 通过；原CLI/候选/数量/recovery结果保留原版本；最新Runner基线L1通过，四项隔离4/4、零skip、cleanup全通过，执行261.17秒，实际最多两项并发；本机workspace退役不再待验 |
| TR-A5 | 新独立 simulate 包真实 Edge web.smoke；在活动轮次关闭页面后核对服务与原执行继续，再打开完成既有反馈/final/交付/离线闭环，收尾进程/监听 0 | 通过；真实 Edge 1/1、0 retry，关闭页面分支及原闭环通过，cleanup 进程/监听均 0 |
| TR-A6 | 当前文档入口与实际一致；TUI 资料标注历史/退役并修复链接，原未验不变通过；文档链接与差异检查通过 | 通过；最新431份 Markdown / 2607条本地链接 / 0错误，git diff --check 通过；roadmap 未修改 |

## 执行与证据登记

实施前后分别记录代码 commit/dirty、实际命令、命中用例/场景、时间、结果、原始日志/summary 路径、构建 hash 和限制。删除清单附每个混合测试的业务断言保留位置；重复已覆盖用例注明现有 owner，独有业务要求不能只填“删除”。

源码检查允许历史文档、原生工具 TUI、run -console 日志和仍有效的服务控制台概念；检查目标是没有退役客户端的生产/测试接线，不是抹除全部 console 字样。

所有实际调试从 `scripts/new-debug-release.ps1` 新建 SteerlineTest 包；AI 显式 `--simulate`，不用安装目录、旧包数据库/认证/配置准备现场。测试清理由独立驱动显式停止本包项目/应用/Web/launcher，保存退出与监听事实；浏览器关闭与测试最终收尾分别记录。

外部模型/PR 使用 controlled provider，Go、Git、SQLite、HTTP、Edge 与进程/锁为真实实现。未计划 Full、真实模型/PR、远端矩阵及原生工作区重跑，不能据此声称对应验收完成。没有新增人工阻塞项；既有 Web 人工体验归原任务。

## 2026-10-15 规划文档检查

本轮仅创建 Spec/Plan/本记录并增加导航；未删除产品代码、未生成调试包、未运行产品测试。静态检查实际结果在执行后登记，不计为 TR-A1～TR-A5 产品验收。

- `python scripts/check_doc_links.py`：421 份 Markdown、2677 条本地导航链接，0 错误。
- `git diff --check`：通过；仅修改两份目录导航，并新增本任务三份文档。产品验收仍全部待实施、待验。


## 验证记录与失败修正

代码基线 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`，本任务为 dirty 工作树。实际安装路径未操作；安装隔离场景使用临时夹具，临时产品现场均从生成器建立在 C:\SteerlineDebug。以下结果绑定各自构建，不能将早期通过项冒充后续所有改动的复验。

- 固定 L1 通过：`.../steerline-test-feedback/20261015T041032.942158000Z/summary.json`（277.9 秒）；迁移后的 formatter、HTTP/localservice/application/Web 与非 opt-in driver 定向通过：`.../20261015T041233.564812500Z/summary.json`（71.847 秒）。
- CLI 原请求测试修正 wire 的 outcome_unknown / 原 ControlReceipt.indeterminate 区别后通过，证据在 `.../20261015T041713.334912600Z/`；该次整体仍因候选测试失败，不能记为整组通过。app/CD66/公共客户端边界在首次定向组实际通过；候选启动/反馈/停止的最终结果另记。
- Web 单元与构建、Web Go owners 通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-web-check-20261015-121613-748/`；源码双 exe 构建通过。无 Node 的业务构建、全部产品/测试依赖枚举及 e2e-tag 编译通过，原始日志在 `C:/Users/lmas1/AppData/Local/Temp/steerline-tui-retirement-20261015-122347-208/`。
- 新增受控首次发布/原 PR 复用反例通过（普通与 recovery 两种路径，真实本地 Git）：`.../20261015T043900.740061000Z/summary.json`（23.705 秒）。修复仅在测试夹具：首次发布不再按两种路径中不同的 turns 长度误判为反馈。

保留的失败样本及修正：

| 样本 | 发现与后续处理 |
| --- | --- |
| `20261015T041233.543954600Z`、`041713.334912600Z`、`042226.504529400Z`、`042757.262677200Z` 定向报告 | 迁移测试误用 resume_dispatch、wire 状态和 start_task/start_candidate 字段，已按原契约调整；最后一次多候选原生启动超时，待串行复验，不删除断言换通过 |
| `20261015T041731.897502100Z` recovery.failure-results | 领取阶段超时，未到达新的 HTTP 只读检查；已清理，待单独复验 |
| `20261015T042253.895692300Z` runtime.installed-debug-isolation | 构建与源码文件迁移并发，读取已移动的 console_activity.go 失败；源码稳定后复验 |
| `C:/SteerlineDebug/中文 verification-302013292d0c4551b5589789df27ba46/` | 两个生成包成功；脚本仍引用旧初始化证据文件名，已修正。直接 runtime 验证还发现测试 HTTP 驱动继承错误环境及 held-turn/WebFeedback 首次发布夹具问题，均已修正；原样本不标通过，后续使用新包 |
| `C:/SteerlineDebug/dev-20261015-122757-47fafc12-dirty/` | 新 smoke 包在 task create 的既有期限内未完成，manifest=failed，未执行 Edge；保留 init.log，待串行新包验证 |

本任务保留恢复、任务/DAG、CLI、原回执、共享进程身份、服务日志与工作区场景；TUI 键盘/布局/本机偏好/原请求 journal/observer 和专属 ConPTY 按键驱动退出。单/双项目调试测试覆盖独立 Web、端口冲突、退役命令不得冻结方案、正常停止及 reset；专门的沙箱驱动继续用原有 run-project 私有入口，只拥有项目服务，生成包仍完整包含 Web。

- 候选数量/并发与原请求 owner 回归通过：`20261015T045619.124231900Z/summary.json`，实际命中 TestMC9、TestCandidateStartReservationsRetainFailuresAndBoundParticipation、TestCandidateReservationConcurrentLimit，3/3 通过、0 skip（86.22 秒）。客户端接线保留两个真实候选，数量上限不再重复启动五个原生 fixture。
- 最新 E2E 全部场景编译通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-tui-retirement-e2e-final-compile.log`。
- `20261015T043930.213847900Z` 恢复业务断言与两个 HTTP 分支完成，但最终停服使业务计时超限，runner 正确拒绝；已将既有 FinishBody 边界暴露并用于最后断言后、独立停服前，未放宽期限。`20261015T044730.199335200Z` 仍在冗余 CLI 快照阶段超时；改为复用 WaitRecovery 原快照并经 driver 读取查询后业务域，再串行复验。
- `20261015T045820.352203500Z` 两候选接线已完成启动、原请求重放、反馈推进，在 stop_candidate 处因迁移测试漏传 task_id 被契约拒绝；补齐原 task_id，待复验，不改产品校验。

- 修正后 controlled `recovery.failure-results` 实际通过：`20261015T050132.888655900Z/summary.json`；Go 场景 253.13 秒，runner passed=true。两个 HTTP 分支、原失败事实、缺 thread 不可继续及独立清理均已核验。

## 最终删除与覆盖归属

| 退役部分 | 保留能力的实际位置 |
| --- | --- |
| TUI UI/组装、CLI console、两套 console.bat、debug console | 现有 Web/app/项目服务链、运维 CLI 与 service-window；launcher 负例在 internal/app/projects_test.go |
| TUI request journal/锁/停服 observer 及专属 HTTP 测试 | 基础 httpclient、localservice、applicationclient/http、Web owner；CurrentProcessIdentity/ProcessExited 保留 |
| TUI 目录内 formatter | internal/product/activityrender/render.go 及原 render_test.go；逐字节规范换行对比一致，服务两个组装点已改接 |
| candidate_console / inline_console 混合集成 | tests/integration/candidate_client_test.go 保留两个原候选、原请求重放、作用域 activity、原 Decision 中文反馈、原候选手动停止；数量/并发/第六次拒绝归 execution 三个 owner 测试；CLI 原请求/回执归 cmd/steerline/native_task_test.go |
| recovery 的 u 键/弹层、原生工作区重开 TUI | recovery.failure-results 改公共 HTTP，保留两次查询前后业务域与身份；workspace.orca-removal-restart 改 ReadHTTPProject 并保留工作区/无关任务断言，本批只编译 |
| TUI boundary/runner/ConPTY 按键驱动 | HTTP consumer 和 execution/storage/adapters/Windows/Linux 约束保留；TUI 独占驱动删除，通用原生进程/Job 与服务窗口驱动保留 |
| ui/views reset、旧隔离脚本 console 入口 | Web/application/project/worker/launcher 锁、原 SQL 重置、定义/契约/依赖/Git/PR 事实保留；通用 ui 越界负例保留 |

未保留 TUI 键盘、布局、视图偏好、窗口锁、请求 journal 或停服 observer 的替代实现；原生 Codex/Orca 自身终端功能和后台 run -console 日志不在删除范围。

- 新单/双项目包 `C:/SteerlineDebug/中文 verification-9ac7e5daa3294ac4b690d8df40538693/` 均 ready，双项目 TestDebugReleaseWebProjects 实际通过（81.50 秒），包括主动占用端口后的失败清理与重新启动、双身份、正常停止与 reset。单项目在启动前发现默认 18742 已占用，Web 正确 bind 拒绝，没有停止或修改占用进程。调整脚本：默认地址包只验生成产物，运行用另建 19747 单项目包和 19744 双项目包，旧包不改方案、不重用。

- `20261015T051211.280435500Z` 停止请求在反馈尚在推进时被 stale_revision 拒绝；测试改为等原候选下一次稳定待决后取当前版本，没有放宽产品版本守卫。最终 `TestCandidateClientWiring` 实际通过：`20261015T051510.336353000Z/summary.json`（129.09 秒，1/1、0 skip）。

- `./scripts/test-debug-release.ps1` 最终通过（Live=false）：`C:/SteerlineDebug/中文 verification-2357a665370e49ffa463338047162d65/dev-20261015-131905-47fafc12-dirty/evidence/generation-test.json`。默认产物包 131749、19747 单项目运行包 131905、19744 双项目包 132018。runtime-test.log 命中 TestDebugReleaseWebProjects（69.29 秒）及 TestDebugReleaseRuntime（121.38 秒），2/2 通过，包含活动 worker/reset、交付、原任务/Git/PR 保留、正常停止，以及安装路径参数在任何路径探测前拒绝、junction 与重名输出拒绝；仅传递拒绝字符串，实际安装路径未读取或写入。

- 最终真实 Edge `web.smoke` 通过（1/1、0 retry，6.4 分钟）：`C:/SteerlineDebug/dev-20261015-132527-47fafc12-dirty/evidence/smoke.log`；同目录 `browser/` 保存 trace、截图与 browser-close.json。活动轮次关闭页面后原服务/执行/attempt/session/turn 保持、原轮次继续，重开完成原请求查证（零重复启动）、反馈第 4 轮复用原 PR、草稿、final、交付/后继与离线。`smoke-cleanup.json` passed=true、browser_passed=true、processes=0、listeners=0、stop_errors=[]。

- 通用 HTTP/driver 定向报告 113 个测试中 110 个通过、3 个显式 opt-in debug 测试 skip；不将 skip 记为验收。Runtime/WebProjects 随后已由新包命中通过，CodexSandbox 未选。最终文档链接检查 421 / 2673 / 0 错误及差异检查通过。删除后的空 TUI 目录也已清理，仅限工作树内空目录。

- 新包 CLI 负例实际执行：steerline console 返回 unknown command，未进入配置读取；PowerShell 四份修改脚本的原 BOM 状态保持，最后文档检查更新为 421 / 2674 / 0 错误。

- `20261015T053353.711031300Z` 运行隔离场景完成两套交付、重启、越界与 SQLite 链接反例，但末尾普通源码构建撞到原 10 分钟期限；passed=false、cleanup_complete=true。保留全部断言，调整测试领取顺序：两套已有隔离服务均先领取，再分别等原候选发布，利用各自 provider 并行推进以减少串行空等；不改产品或期限，使用全新夹具复验。

## 最终交付限制

TUI 专属实现、入口、launcher、依赖和测试已删除；共享日志/HTTP/CLI/worker/原生工具能力保留，Web 为唯一交互前端。实际安装目录未操作，源码工作树中实施，没有执行安装或发布。

`runtime.installed-debug-isolation` 最终仍未通过，TR-A4 保持待验：`20261015T053353.711031300Z` 在末尾普通源码构建处撞到固定 10 分钟期限；调度优化试验 `20261015T054631.661511900Z` 在第二次服务窗口就绪等待时撞到同一期限，两次 cleanup_complete=true。未观察到这两处提前返回产品断言失败；超时本身仍是验证失败，不能据此证明全部隔离断言通过。调度试验没有改善结果，已撤回，原有顺序、期限和全部非 TUI 断言保留。本次没有为此重构隔离测试体系。

其余 TR-A1/A2/A3/A5/A6 和 TR-A4 的 L1、定向 CLI/app/候选/数量 owner、controlled recovery、全部 E2E 编译均有上述通过证据。未执行 Full、真实模型/真实 PR、全部 L3 或原生 workspace 重跑；对应既有未验项保持未验。

## 2026-10-15 PR 前审计

审计本次完整差异，并接入主干 `317714e6`（原实现提交重放为 `11f0842f`）。主干双产品 build.bat、Web home ACL 修复、查询复用均保留；唯一架构文档冲突已合并双方最终语义。

| 发现 | 修正与验证安排 |
| --- | --- |
| 原生工作区查询迁移在“无关任务启动”循环中重复 HTTP 查询，瞬时错误直接 Fatal；删除/重启后的查询只要求非空实例，没有与当前服务比对 | 两个必需查询等待 HTTP ready，并与已有当前快照的服务实例比对；保留原 run/execution/attempt 与项目/数据库检查；删除轮询中的重复查询。按 Plan 编译全部 E2E，真实原生场景仍未重跑 |
| CM72 从 TUI 改为 HTTP 后漏掉 profile digest 非空断言，两端均空也可能通过 | 恢复非空检查；用原精确 CLI 测试复验请求/回执与服务保留 |
| 原先通过证据基于旧主干，不能直接证明 Web 查询优化与 ACL 修复整合后的结果 | 新跑 L1、定向 owners、Web 单元/构建、全部 E2E 编译及新包真实 Edge；结果另行登记 |

runtime 综合隔离的两个超时仍是未完成验证，保持 TR-A4 待验，PR 以草稿提交；没有把超时归因成已证实的产品回归，也没有借审计放宽期限或删除剩余断言。安装路径未操作。

审计整合后的已完成复验（基于 `317714e6` + `11f0842f` + 审计修正）：

- L1：`20261015T060421.786769900Z/summary.json`，passed=true / complete=true，271.1 秒执行时间。
- Web 单元/构建：`C:/Users/lmas1/AppData/Local/Temp/steerline-web-check-20261015-140412-934/`，70 个前端测试全部通过；Web/HTTP owner 定向 runner `20261015T060526.274111400Z/summary.json` 通过。
- CLI 原请求/回执、非空摘要与关闭连接保留服务：`20261015T060924.171584900Z/summary.json` 通过（98.53 秒）。
- formatter/localservice/applicationclient/applicationhttp：`20261015T060924.226100300Z/summary.json` 通过（6.21 秒）。controlled 普通/recovery 首次发布与原 PR 复用、独立清理计时边界：`20261015T060924.226632400Z/summary.json` 通过，6 个测试、0 skip（19.14 秒）。
- 全部 E2E 场景编译：`C:/Users/lmas1/AppData/Local/Temp/steerline-tui-retirement-review-e2e-compile.log` 通过。产品/测试完整依赖图：同临时目录 `steerline-tui-retirement-review-product-deps.log`、`steerline-tui-retirement-review-test-deps.log`，无退役 TUI 包及专属库。
- 当前文档链接 424 份 / 2692 条 / 0 错误；整合差异检查通过。Web 真实 Edge 新包复验结果完成后补在下方，原运行隔离超时继续保留。

- 整合后真实 Edge `web.smoke` 通过（9.1 分钟，1/1、0 retry）：`C:/SteerlineDebug/dev-20261015-141159-11f0842f-dirty/evidence/smoke.log`；browser-close.json 保留活动轮次关闭/重开原身份，反馈第 4 轮、原 PR 复用、交付与离线均通过。`smoke-cleanup.json` passed=true、browser_passed=true、processes=0、listeners=0、stop_errors=[]。

补充验证安排：审计核对源码构建子检查通过 Git clone 绑定 HEAD，前次 dirty 工作树的该子检查仍取旧提交。先提交审计修正和此安排，再保持工作树干净，对实际 PR 提交执行一次原 runtime 综合隔离（期限与断言均不变）；结果随后补记，旧超时不改写为通过。是否将草稿转为可评审由该次实际结果决定。

### 实际 PR 提交验证结论

对干净提交 `8c166ac6b4a2915abca34abf8f29638918b8674c` 执行的 runtime 综合隔离仍未通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T062348.721529400Z/summary.json`，passed=false / cleanup_complete=true。场景在第 2 次可见服务窗口就绪等待达到原 10 分钟期限；正常清理也报告 context deadline exceeded，最终资源/进程核验完成。旧 HEAD 的源码克隆不能解释这次超时，前述核对只解决证据版本绑定，没有解决耗时问题。本次保持原期限与全部非 TUI 断言，TR-A4 继续待验。

已创建草稿 [PR #324](https://github.com/erranliu/SteerLine/pull/324)，接入主干、两处审计修正和其他通过的验证均已提交；该 PR 尚不具备全部计划验收通过的结论。原生 Orca 工作区实际删除链依旧仅编译。实际安装路径没有操作，Web smoke 的进程/监听清理均为 0。


## 独立隔离场景与退役本机 Orca 补充（2026-10-15）

用户授权删除旧综合场景，精简成长期必要的独立回归，并明确本机 Orca 路径已退役、远端路径保留。本批不改变产品 worker、恢复或执行行为。旧综合场景与两份专属 `debug-runtime` 脚本删除；源码构建只写本次独立 clone，不再写/重命名工作树 bin。所有场景 home/仓库/DB/provider/端口/进程和证据独立，共享 runner 构建产物只读，每项仍为 5 分钟业务期限。只对已审查的 runtime controlled 四项开放 Go `-parallel=2`；L2、live 和其余包顺序保持。

### 原断言的长期归属

| 原综合内容 | 当前长期测试 / 处置 |
| --- | --- |
| 普通源码构建/重建、bin junction/输出硬链接拒绝、另一环境 exe/hash/DB/业务事实不变 | runtime.source-build-isolation，运行自己的源码 clone；另一 home 无活动 writer，DB 字节摘要检查无心跳干扰 |
| state junction、run_dir/worktree_root 越界、SQLite DB/WAL/SHM 硬链接、拒绝前不写及解除链接后原生校验可用 | runtime.path-isolation，无业务 worker；保留逐文件 size/hash 和 SQLite 文件集合证据 |
| 原生 service-window 入口/可见子进程身份、launcher 幂等、同构建重启/新实例、过期实例/改配置不发请求、跨 home 仓库锁 | runtime.service-write-isolation，不再为启停重复跑完整交付；重启后另核对真正的上一实例拒绝 |
| 双home写入与原请求不串用 | runtime.dual-home-isolation；当前Runner基线用相同任务ID/请求ID的独立作废及原回执重放，双向任务/版本/provider不串用、无候选；完整执行/交付归已有delivery/生成包，不再重复两条流程。前述原执行双流程结果保留其版本范围 |
| 正式新调试包生成/完整 Web、不同包/项目身份、锁/reset、方案冻结、端口冲突、停止清理 | 已有 scripts/test-debug-release.ps1，始终新生成，不扩张为第二套产品/驱动 |
| 旧 wrapper 专属 UI alias、源码 exe fallback/missing、wrapper 的 build/service 操作 | 随 wrapper 退役；对应当前生成根重叠/链接拒绝及缺 Web 等由现有生成包入口覆盖 |
| 本机 Orca workspace 删除/重启 | 用户明确退役，删除场景、专属驱动、孤立解码器、native 环境/证据规则和正向测试；增加旧入口拒绝检查。远端实现与公共输出/usage 守卫保留 |

### 本次执行结果

四项场景在干净源码提交 `55a34457cabcd695c3722ec26b5af7d3c4c1f44c` 上执行：

```powershell
go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled
```

证据根 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T072823.695593000Z`，`summary.json` passed=true / cleanup_complete=true，4 tests、0 skip、无失败；准备 14.54 秒、执行 415.61 秒。各 scene 的 context_error 为 nil、cleanup_complete=true，全部 scratch 由各自 owned cleanup 删除。

| 场景 | Go 实际耗时（含收尾） | 结果 |
| --- | --- | --- |
| runtime.source-build-isolation | 132.22 秒 | 通过 |
| runtime.path-isolation | 82.91 秒 | 通过 |
| runtime.service-write-isolation | 212.20 秒 | 通过 |
| runtime.dual-home-isolation | 279.47 秒 | 通过 |

`parallel-proof.json` 从原 `e2e-1.jsonl` 的 cont/pass 事件重建：max_active=2，结束 active 为空。首次为 source-build 与 service-write 同时运行，source-build 完成后 dual-home 进入，service-write 完成后 path 进入。不是仅配置并发参数；原始执行区间确有重叠。旧综合场景的超时记录不改为通过，新结果只证明上表的当前必要隔离保证。

其他定向证据根统一为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`：

| 命令 / 范围 | 证据 / 结果 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1` | `20261015T072847.435265200Z/summary.json`，passed=true/complete=true；执行151.3秒，源码55a34457 |
| runner 全包 `go run ./cmd/test-feedback -- ./cmd/test-feedback -count=1` | `20261015T073805.941755600Z/summary.json`，90个用例/子项、零skip，通过；包含旧综合/本机 native 入口拒绝、四场景选择/安全并行、remote public output/usage 与其他原门槛 |
| `go run ./cmd/test-feedback -- ./internal/app ./internal/execution/adapters/orcatransport -run '^Test(TerminalRegistrationRejectsRetiredRoutesAndAgentPrivateFields\|RemoteRepositoryDiscoveryRejectsBeforeCreation\|RemoteRepositoryCreationRetainsReceiptOnMismatch)$' -count=1` | `20261015T073637.032113700Z/summary.json`，9个用例/子项、零skip，通过；使用 fake，不调用本机/远端 Orca，不当作远端真机验收 |
| `go run ./cmd/test-feedback -- ./tests/e2e/internal/native ./internal/testkit/boundary/... -count=1` | `20261015T073836.572089400Z/summary.json`，96通过，1个原有沙箱账号预检 opt-in 未选择而skip；该项不属于本批验收。adapter/边界实际选项通过，无模型 |
| `go test -tags=e2e ./tests/e2e/... -run '^$'` | `%TEMP%/steerline-independent-isolation-final-compile.log`，删除本机 native 场景后全部剩余 E2E 编译通过 |

L3 四场景之后清理的代码只涉及已退役本机 native 环境/专属字段与解码器；四场景本体、普通期限、产品源码和业务夹具未改变。其后重新编译全 E2E，并执行 runner 全包及 decoder/边界测试；不把四场景旧结果标成另一提交的新运行。

### 取消的本机 Orca 补验及清理

开始时按旧计划补跑本机 native 场景：`20261015T072705.926514600Z` 因 Orca 未启动，在环境准备阶段失败。启动已有客户端后创建的新运行 `20261015T072944.383407400Z` 只登记了本次临时 repo，尚未创建工作区、启动模型、执行真实删除链。用户明确该路径已退役后中止，绝不算通过，也不再要求补验本机路径。远端 Orca environment 路径和既有未验事实不受影响，没有执行远端真实模型或 PR。

取消运行的拥有资源已核对：没有测试子进程、没有工作区/终端；唯一临时 setup `4ca3bf33-6547-4c73-bf97-97f7afe98e59`（路径在 `steerline-e2e-732815938/workspace.orca-removal-restart`）通过已核对的 Orca CLI setup-delete 移除。自动审批审查拒绝递归删除该临时 scratch 目录，未继续尝试删除；该目录与取消运行的日志保留，不冒充全清理完成。独立四场景的成功清理不包含这个已取消现场。没有使用、修改、安装或重置实际安装路径。

原 PR #324 已在主干 `60dae58306ed2dfe6daec3cd3ecf89bb0e80a7db` 合并；本次增量另提后续 PR。原先未完成的 runtime 隔离验收已由用户确认的新场景全部完成；本机 native 项按明确退役退出当前门槛。


### 当前生成包入口复验

`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-debug-release.ps1` 通过，无 `-Live`。新根 `C:/SteerlineDebug/中文 verification-56ada7a4d6454b8c8aa75cfadb6be048`，三个新包：

- 默认产物 `dev-20261015-153555-55a34457-dirty`，只验证18742配置/三exe/Web/hash，不启动默认端口。
- 单项目运行 `dev-20261015-153712-55a34457-dirty`，19747；TestDebugReleaseRuntime 75.46秒通过。
- 双项目运行 `dev-20261015-153842-55a34457-dirty`，19744；TestDebugReleaseWebProjects 46.99秒通过。

单项目包 `evidence/generation-test.json` passed=true / live=false；`runtime-test.log` 2/2通过。包含端口占用拒绝、活动 Web/app/项目/worker 的 reset 门槛、方案冻结、独立项目身份、静止 reset/保留业务定义与 Git/PR 及停止。`evidence/audit-cleanup.json` 最终 processes=0/listeners=0/passed=true。外层原始日志 `%TEMP%/steerline-independent-isolation-debug-release.log`。本批源码基于55a34457加已记录的本机 native 退役/文档增量（dirty），产品源码未变；未把源码提交号不同当成产品行为变化。

最新文档检查：424份 Markdown / 2701条本地链接 / 0错误，`git diff --check`通过。TR-A1～TR-A6 当前选定自动验收均已完成；原超时、取消的本机 native 现场以及各自历史结论保留。没有重跑 Full、真实模型、远端真机或真实 PR 交付，也不把它们计入本次通过范围。


### 接入 #326 的验证边界

PR #327 创建后主干合入统一多项目启动（0228e51d）。前述55a34457及生成包结果均属于旧宿主版本，不作为新宿主通过证据。已解决修改/删除冲突并保留旧脚本/综合场景/本机Orca场景删除；四项独立测试改接serve/应用API与原宿主停机结果。新基线的执行结果另行登记，旧结果保持原范围。


统一宿主首轮 `20261015T074903.398462100Z` 未通过：构建163.42秒、双home201.34秒通过；服务测试把停机结果的目录项目ID与数据库身份比较，路径测试把DefaultState.json写入etc造成重复配置。服务失败现场另报owned exe短暂Access denied，保留原清理失败；不计为全部隔离通过。修正为正确身份映射、反例配置置于项目root，服务测试只读共享runner程序，不再重复安装自己的exe。

同轮新包根 `C:/SteerlineDebug/中文 verification-000c5400108a4799ae9f308bd5be0a30` 失败：旧Go测试Job包装使detached产品仍在kill-on-close Job，产品在打开资源前正确拒绝；端口冲突后的fresh包尚无DB，reset误将缺失状态目录当成锁冲突。修正测试脚本采用已有浏览器驱动相同WMI独立现场，保留产品Job拒绝规则；fresh reset不建库且仍验证活动锁。新fresh reset定向回归 `20261015T075649.027992200Z/summary.json` 通过。失败包不改方案、不复用，以下重新生成并执行。

第二轮新包根 `中文 verification-7c010ba5b0cf48c8bca97854d61b2c4a`：WebProjects通过92.55秒，Runtime在宿主API已ready但种子尚未写入时读到0任务而失败。旧就绪判断过早，已改为匹配包owner/ready token，并为失败路径补正式stop后再关闭Job的收尾。失败包的单/双项目已通过各自helper debug stop正常停止，不复用；重新生成验证。


### 统一宿主下独立隔离回归最终结果

固定实现 `e54dcb83`（基于主干0228e51d），执行 `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled`：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T075743.254973400Z/summary.json`，passed=true / cleanup_complete=true，4/4、0skip；执行391.06秒。`parallel-proof.json` 基于原始cont/pass事件，max_active=2，结束active为空。四项scene的passed/cleanup_complete均true、context_error均nil。

| 场景 | 新宿主实测耗时（含收尾） | 结果 |
| --- | --- | --- |
| runtime.source-build-isolation | 140.33秒 | 通过：本次clone构建/替换与链接拒绝，静止另一home的exe/DB/业务事实不变 |
| runtime.path-isolation | 166.01秒 | 通过：check入口实际拒绝state/配置/SQLite别名，链接解除后普通check成功 |
| runtime.service-write-isolation | 139.41秒 | 通过：原宿主PID/目录项目ID/项目实例，拒绝时无请求、正向控制计数+1，跨home锁不建库，两代宿主的逐项目clean shutdown |
| runtime.dual-home-isolation | 244.96秒 | 通过：两原执行/请求/交付不串用，依赖释放及两home实例保持，各自正常停止 |

补充新主干证据：L1 `20261015T074900.796223900Z/summary.json` passed/complete=true、执行159秒；runner全包 `20261015T075022.124833700Z/summary.json`通过；远端入口/仓库fake定向 `20261015T075834.357054300Z/summary.json`通过。这三项在2b5b1b18基线上运行，后续仅修正debug夹具/就绪/收尾、删无消费者的服务窗口测试分支，未改这些所有者。

当前提交5ec30fad的全部剩余E2E编译日志 `%TEMP%/steerline-isolation-unified-seeded-compile.log` 成功；fresh reset回归 `20261015T080733.375105100Z/summary.json`再次通过。四场景之后只删未调用的旧窗口分支/字段、未消费的stop输出并补编译，没有改变所测普通StartHost/StopService路径；不把旧场景失败改为通过。


### 统一宿主新调试包最终结果

固定源码 `5ec30fad`，重新从唯一生成器创建全新根 `C:/SteerlineDebug/中文 verification-5fab91c5280549709395671fa827971b`：

- 默认产物 `dev-20261015-160626-5ec30fad`，按当前统一启动契约自动选择空闲端口后固定，不启动默认包。
- 单项目 `dev-20261015-160706-5ec30fad`（19747），TestDebugReleaseRuntime通过94.41秒，包括实际受控PR合并/交付、活动worker与宿主锁、检查不写、静止reset保留任务/Git/PR事实、冻结方案拒绝切换。
- 双项目 `dev-20261015-160740-5ec30fad`（19744），TestDebugReleaseWebProjects通过57.27秒，包括端口冲突后收尾、独立项目身份/数据库、活动锁/reset与整体停止。

单项目 `evidence/generation-test.json` passed=true/live=false，`runtime-test.log` 2/2实际通过（总153.32秒）。`evidence/audit-cleanup.json` processes=0/listeners=0/passed=true。两轮统一宿主失败包的精确exe前缀进程核验也为0，失败原日志保留。外层日志 `%TEMP%/steerline-isolation-unified-seeded-debug-release.log`，WMI独立进程现场 `%TEMP%/steerline-debug-acceptance-4d5949bc1e104282beb46630bc6f8cdf/{test.log,result.json}`，exit_code=0。产品Job守卫、统一协调入口、方案冻结与原业务期限均未削弱。

最终文档427份/2743条本地链接/0错误、差异检查通过。当前增量的TR-A3/TR-A4/TR-A6自动项完成；前序TUI/Web及统一启动验收按各自源码和范围保留，没有把旧Edge、recovery或远端结果移标成新版本。未跑Full/真实模型/远端真机；本机Orca场景明确退役，不再待验。取消的本机测试scratch仍按前文保留，实际安装环境未操作。

后续 [PR #327](https://github.com/erranliu/SteerLine/pull/327) 基于统一启动主干0228e51d，包含独立隔离回归、必要调试夹具修正和本机Orca测试退役。


### 合入 SteerRunner 主干的边界

最终推送检查时主干再合入#325（f3e35373），执行所有者迁出。067a3cfc及5ec30fad一轮通过仍仅归旧执行所有者；当前继续整合和补验。保留当前owner的Runner投影/清理与预编译E2E入口，移除隔离测试中旧执行record断言。双home按长期最小范围验证同号任务/原请求回执不串用，完整交付由已有delivery/生成包单项目负责，避免每次隔离都重复两条原生流程。

本次只读固定外部现有程序到 `%TEMP%/steerline-isolation-runner-c23f908af1bc4e91a85dff443d6bfef3/steerrunner.exe`：version=0.1.0-dev.1、source_commit=af545563c58171c584bd35d098166cf39012e699、SHA256=7ff7941cb625cec63bf32180058d5113c6f54799c567289135b6420014da312e；来源C:/Project/SteerRunner/bin，未修改源程序或另一仓库。后续运行均选择这份固定副本，版本不以文档旧41908e6冒充。实际安装环境仍未操作。


### Runner补验首轮失败及修正

`20261015T082931.819474200Z` 四项均在受限工具启动上下文被Runner凭据一致性守卫拒绝，未作运行通过；守卫不修改。`20261015T082931.804507100Z` L1的实际139条rules用例通过，但reconstruction只剩无测试纯DTO而被计为空包skip，导致固定集失败；仅移出该空包，静态/编译依赖覆盖保留。

新包根 `C:/SteerlineDebug/中文 test-d0c21c5319f1` 在独立上下文中WebProjects通过53.36秒，Runtime启动任务收到identity_conflict。定位为原控制请求冻结了非零图版本，而原HTTP回执比对仍要求start_task的映射图版本为零；请求已发布，不能换ID补发。失败收尾保留原请求事实并按已有公开CLI围栏/停止原Runner，旧包不复用。新增映射图绑定/替换反例，并同步formal consumer夹具的非零图版本。

HTTP全包 `20261015T084158.715032000Z` 首轮保留两组过期fixture的失败（图版本未设置）；修正夹具后 `20261015T085057.611686800Z/summary.json` 全包通过。Runnerclient/bridge契约 `20261015T083239.290430700Z/summary.json`通过。后续使用固定同一Runner副本，原生四项入口共享已有WMI原生进程方式，不以更换Runner版本/权限或跳过守卫换通过。

### 独立Runner基线最终验收（2026-10-15）

固定源码2364dc55的四项原生场景 `20261015T085407.268898500Z` 实际4/4通过、零skip、cleanup_complete=true，执行268.04秒；独立入口却因Windows PowerShell把成功stderr摘要转为异常而返回1，未把该入口标为通过。414b390b仅修正捕获后的字符串输出/原退出码判断，保留真正非零退出失败。

以干净源码 `414b390b69e5db15dbce066587bc2303cf7ffa8e`、上述固定af545563 Runner执行：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-runtime-isolation.ps1 -SteerRunnerExe C:/Users/lmas1/AppData/Local/Temp/steerline-isolation-runner-c23f908af1bc4e91a85dff443d6bfef3/steerrunner.exe
```

原始证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T090006.293618000Z/summary.json`：passed=true/cleanup_complete=true，4/4、0skip；准备15.09秒、执行261.17秒。冻结二进制清单 `build/frozen-binaries.json` 绑定源码与全部SHA。四个scenario均passed/cleanup_complete=true、context_error=nil；`parallel-proof.json`由原始test2json cont/pass事件重建，max_active=2、结束active为空。

| 场景 | 实际耗时（含收尾） |
| --- | --- |
| runtime.source-build-isolation | 123.01秒 |
| runtime.path-isolation | 136.92秒 |
| runtime.service-write-isolation | 117.66秒 |
| runtime.dual-home-isolation | 125.77秒 |

入口本身最终退出0；外层日志 `%TEMP%/steerline-isolation-runner-native-final.log`，独立原生现场 `%TEMP%/steerline-native-test-32ae76e30d124e1aa347092b7b708863/{test.log,result.json}` exit_code=0。场景本体与普通5分钟期限未改变，等待并行名额不计入场景业务期限。

新调试包以源码2364dc55、相同Runner副本生成，全新根 `C:/SteerlineDebug/中文 test-fd2afb45a050`：默认产物 `dev-20261015-165408-2364dc55` 只验产物，单项目 `dev-20261015-165500-2364dc55`（19747）Runtime通过81.57秒，双项目 `dev-20261015-165559-2364dc55`（19744）WebProjects通过77.94秒，总161.55秒。单项目 `evidence/generation-test.json` passed=true/live=false、`runtime-test.log` 2/2通过；覆盖实际受控交付、冻结、活动锁/reset、静止reset保留及正常停止。`audit-cleanup.json` processes=0/listeners=0/passed=true，检查排除审计进程自身。失败根d0c21c5319f1的精确路径进程/监听同为0。外层 `%TEMP%/steerline-isolation-runner-fixed-debug.log`，独立现场 `%TEMP%/steerline-native-test-134c764746224ba3b143efbce49bec1c/result.json` exit_code=0。后续414b390b只修改另一隔离入口的stderr处理，不改变这些生成器/产品/调试测试代码。

同一Runner基线其他最小检查：

- L1 `20261015T085401.729078000Z/summary.json` passed/complete=true，执行108.76秒，所有组零skip/空包；源码2364dc55。
- runner全包 `20261015T090228.693209600Z/summary.json` 90/90、零skip，包含远端public output/usage守卫、退役入口拒绝及安全并行选择；仅受控证据，不声称远端真机。
- runner/新fresh reset定向 `20261015T085923.580536400Z/summary.json`通过；HTTP全包与Runnerclient/bridge证据见上一节。
- 全部剩余E2E编译 `%TEMP%/steerline-isolation-runner-final-compile.log`通过；三个修改的PowerShell脚本语法检查通过。
- 最终431份Markdown/2607条本地导航/0错误，git diff --check通过。Spec/Plan仅同步当前主干所有者及最终证据，未改roadmap。

本次漏验的独立隔离/新调试包项完成。既有Edge/recovery等记录仍绑定各自源码，不称本次重跑。没有执行Full、真实模型、远端真机或真实PR；本机Orca明确退役，远端能力保留。实际安装环境未操作。取消的本机场景scratch仍按前文自动审批拒绝删除的事实保留。
