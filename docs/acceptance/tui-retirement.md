# TUI 退役与 Web 唯一交互前端验收

专属普通开发任务：[Spec](../spec/tui-retirement.spec.md) · [Plan / 测试计划](../spec/tui-retirement.plan.md)。2026-10-15 已按用户授权实施；运行隔离场景因固定期限超时待验，其余所选验证通过；无 roadmap Task ID。本记录只归本任务，既有 Web/TUI 结果不直接算本批通过。

## 验收项

| ID | 操作与通过条件 | 当前结果 / 证据 |
| --- | --- | --- |
| TR-A1 | 对照删除清单检查 CLI/包/模板/调试分派/runner/依赖；普通业务构建及 Web 双产品构建，无当前可执行 TUI 入口 | 通过；源码/依赖检查、无 Node 业务构建、双产品构建及新包三 exe/hash 通过 |
| TR-A2 | 运行迁移后的原 formatter 测试与基础 HTTP/localservice/app/Web owner 回归；确认日志、实例/认证/原信封、共享进程函数和公共 recovery 路由保留 | 通过；formatter 内容未变，基础 owners、CLI/候选接线、HTTP 恢复及 Web smoke 通过 |
| TR-A3 | 用生成器默认参数及显式不同端口创建新单/双项目包；核对默认 Web 产物、端口冲突拒绝、方案冻结、活动服务/worker 的 reset 拒绝、check 不写、静止后重置范围及全部退出 | 通过；最终 generation-test.json passed=true，默认包只验产物，19747 单项目与 19744 双项目实际运行、停止和 reset 通过 |
| TR-A4 | 固定 L1、定向混合测试、controlled recovery.failure-results / runtime.installed-debug-isolation 实际通过，所有 e2e-tag 场景编译通过；保留断言清单和查询前后状态，不能用删场景/缺包跳过换通过 | 部分通过；L1、编译、CLI/候选/数量 owner、recovery.failure-results 通过；runtime.installed-debug-isolation 仍固定 10 分钟超时，含实际 PR 提交复验，待验。原生 workspace 仅编译 |
| TR-A5 | 新独立 simulate 包真实 Edge web.smoke；在活动轮次关闭页面后核对服务与原执行继续，再打开完成既有反馈/final/交付/离线闭环，收尾进程/监听 0 | 通过；真实 Edge 1/1、0 retry，关闭页面分支及原闭环通过，cleanup 进程/监听均 0 |
| TR-A6 | 当前文档入口与实际一致；TUI 资料标注历史/退役并修复链接，原未验不变通过；文档链接与差异检查通过 | 通过；424 份 Markdown / 2693 条本地链接 / 0 错误，git diff --check 通过；roadmap 未修改 |

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
