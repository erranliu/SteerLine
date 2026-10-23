# 统一启动与多项目共进程宿主验收

专属普通开发任务：[Spec](../../spec/unified-startup.spec.md) · [Plan](../../plans/unified-startup.plan.md)。2026-10-15 用户已授权实施，且已 rebase 到含 #324 TUI 退役的主干 `60dae583`；按普通开发交付，无 roadmap Task ID。此记录只归属本任务，不复用前序 Web 项目切换的通过状态。

## 自动验收

| ID | 最小检查与通过条件 | 实施后步骤 / 证据 | 当前状态 |
| --- | --- | --- | --- |
| UST-A1 | 单 bat 自动启动双进程并授权打开网页，重复/并发与半启动只补缺项，失败窗口及退出码可见 | host 定向反例 + 新 Edge 包；start/status、精确进程/就绪/打开结果，见 Plan 测试表 | 通过（定向 + 综合） |
| UST-A2 | 两真实项目同 PID、独立实例/DB/actor，零/坏/慢/冲突/跨 home 隔离，身份解析总期限、锁先于初始化、manual 和配置快照正确 | host fixture + 新包状态/锁/数据库归属事实；失败项目无初始化副作用 | 通过（host/fixture） |
| UST-A3 | 应用 API 直达原项目用例，原生/浏览器认证、固定 CLI 操作及批次/依赖原回执 GET、大整数、旧实例/同号任务隔离；旧 stop_service 不变成整体停止 | api/CLI/前端定向反例 + 实际写入、丢回执只读查证零重放；保留 #323 查询回归 | 通过（API/CLI/前端及独立缓存回归） |
| UST-A4 | Web 重启不影响活动轮次；后端停启覆盖全部项目，逐项停机结果与精确退出/租约共同核实；冻结策略及重开零自动输入/答案正确 | lifecycle 两种策略 + 原宿主 shutdown.json + 新包活动轮次/后端重开，记录原身份/授权/副作用 | 通过（策略 fixture + 综合停启） |
| UST-A5 | 项目错误不误停兄弟；本地 actor 未关闭与 worker 未知分别处理；停机结果缺失/旧实例/部分失败不假报成功；已退出 stale 可恢复、未知进程不接管，端口/超时/部分启动诊断正确 | host/api/lifecycle 反例；正常与失败停机结果、原回执、实际进程/租约/项目锁分别取证 | 通过（定向反例） |
| UST-A6 | bat 双产物、资源和安装守卫；旧启动器清理仅限 owned 文件，路径/cwd/ACL/日志正确，调试端口/冻结正确，无新增 TUI 支持 | 构建 + 隔离安装/ACL 反例 + 新生成包 manifest；两 exe SHA-256、路径和权限对照 | 通过（最终双 exe 已重建） |
| UST-A7 | 统一入口 Edge 双项目选择/两标签、命令与丢回执、Web 活动轮次重启、后端停启/授权门槛；另包 query-cache 保留查询优化，分别清理完整 | Plan 中 web.unified-startup 与 web.query-cache 两场景、不同新包；截图/trace、原身份、CLI/请求计数、各自 cleanup JSON | 通过（两个独立新包） |

Plan 中的新 Go 场景、wrapper 和生成器扩展已交付。不存在的入口、零匹配、仅编译、模拟 provider 的自述或历史通过均不算验收。L1 是定向反例的长期回归入口，不能替代真实产品接线场景。

## 环境与执行记录要求

1. 开始前记录实际源码 commit/dirty、Go/Node/Edge 版本、命令和起止时间；沿用 Plan 的源码调查版本不代表已经验证当前源码。
2. 实际产品调试只能先运行唯一生成器，在 `C:\SteerlineDebug` 为两个场景分别生成全新双项目包；公共 Node helper 先 fresh 校验，再通过包级 `--simulate` 冻结一次并调用统一启动，wrapper 不提前冻结。两个独立 SteerlineTest clone/provider，manifest 固定双 exe/hash/端口。禁止使用安装目录或复制其数据库、认证、配置。
3. `evidence/` 保存宿主/项目实例、精确 OS PID/创建时间/路径、原请求/执行/会话/turn、CLI 状态与日志、Edge screenshot/trace。本次新包与实际证据在后文逐项登记。
4. `cleanup.json` 分别记录 Web/后端退出、每项目运行时关闭/资源结算/锁释放、本包监听和 owned 进程；对照原宿主 shutdown.json，不能只看 PID 不在即标清理通过。外部 continue_execution 资源按实际策略另列。清理失败仍为失败，不因业务断言通过而省略。
5. 默认打开由协调器定向测试覆盖：实际 start 默认分支调用本 home 的 Web CLI open 一次，验证 exe 路径/参数/home/退出结果和失败传播；--no-open 场景不能单独证明该项。原 Web launcher 用例核验一次性 URL 归属，Edge 验证其授权及真实页面；系统默认浏览器关联的实际弹出和故障窗口暂停由 UST-M1 检查。业务 exe 不通过依赖 web/server 获得 opener，自动项不宣称完成该人工体验。

## 人工验收

| ID | 行为 | 参与窗口 / 阻塞范围 | 当前状态 |
| --- | --- | --- | --- |
| UST-M1 | 新隔离包双击 start，看到浏览器并选择项目；再次双击复用；查看真实失败窗口与可理解的诊断；stop 的结果可见 | 自动项通过后，窗口待安排；仅阻塞日常体验签收，可后置 | 未验 |

人工体验使用独立生成包，安装目录更新须另按用户明确授权安排；不因编写本规格自动安装。发生失败时记录原样错误和精确环境，不能把人工未执行写成已通过。

## 本次文档静态检查

2026-10-15：文档生成阶段，`python scripts/check_doc_links.py` 通过（421 文件、2697 本地导航链接、0 错误）；`python scripts/roadmap_task_id.py check` 通过（97 身份文件，未改任务编号/执行清单）；`git diff --check` 通过。只验证文档静态一致性，未启动/安装产品或运行产品测试；上述 UST-A1～UST-A7 / UST-M1 仍全部未验。

## 2026-10-15 rebase 后规格审计

已保存初稿并在独立文档分支 `codex/unified-startup-spec` rebase 到主干 `317714e6e42be0f902a97b1a5577e325970d2f75`，保留已合并 #321/#322 及 #323 查询优化；只在两份文档导航发生冲突，已合并保留两边条目。本轮仅审计/修文档，未开始产品实现或验收。

结论：适合一次开发、一个 PR 原子交付。实施内部设运行时/契约、宿主/消费者、入口/驱动、最终验收四阶段门；不拆出需要旧服务兼容链才能工作的半成品，不扩展 TUI、项目添加、SSE 或任务业务。原 Spec/Plan 中以下问题已直接修正：

| 问题 | 修正 | 涉及要求 |
| --- | --- | --- |
| 将“CLI 改用应用客户端”写成笼统步骤，但当前 HTTP 明确拒绝 create_task/add_predecessors，现有回执投影也不完整 | 固定现有 CLI 迁移清单，补批次校验/创建、依赖和原请求只读 GET；复用原事务/幂等，不重放 POST 查证、不扩展领域功能 | UST-A3 |
| 保留原 stop_service action 可能让单项目旧命令误触整体停止 | 删除公开 action/capability，不转译为宿主停机；内部停止仍按项目办理、旧事实可读 | UST-A3/A5 |
| 原进程退出观察不能报告各项目失败，受理或 PID 消失可能假报整体成功 | 定义绑定原宿主的私有停机结果，stop 同时核验逐项目事实、精确退出及租约；结果缺失/旧实例/部分失败均非零 | UST-A4/A5 |
| 把本地 actor 未退出与外部 worker 未知都解释成禁止重开，可能阻断必要取证 | 前者仍持库/锁、禁止替代运行时；后者本地已关后允许重开原库取证，禁止替代 worker/新输入；停止先关闭全部派发资格 | UST-A4/A5 |
| 旧 discovery 一律不可复用，崩溃后可能永久无法重启；协调锁等待未计总预算 | 精确进程已退出且租约可取时才清理旧发现并恢复；未知/持锁继续观察，不接管；锁等待计入整次期限 | UST-A1/A5 |
| 全候选身份检查等待与慢项目隔离关系不明 | 固定只读 census 总期限/并发/既有大小上限；冲突检查后启动已确定集合，慢 runtime preflight 独立，不初始化超时项 | UST-A2 |
| 初稿 wrapper 提前冻结与新主干 freshDualPackage 的 scheme.json 检查矛盾；通用 30/45 秒父超时短于产品启动/停止期限 | 公共 helper 唯一负责 fresh→freeze→ready→cleanup，wrapper 不提前准备；同步所有驱动及父子期限，单独记录阶段条件 | UST-A1/A6/A7 |
| 未纳入 #323 的 query-cache/公共 helper 和既有 testenv，删旧 CLI 后会留下失效消费者 | 维护有效调用清单，统一启动适配；纯 Go fixture 用低层 serve，不强引入 Node；补 driver/testenv 回归、带 tag 编译和另一个全新包的 query-cache Edge 回归 | UST-A3/A6/A7 |
| Markdown 表内带转义的正则复制到终端可能零匹配 | 新增场景统一 TestUnifiedStartup 前缀，命令无表内 alternation 转义，仍要求实际非零命中 | 全部 Go 定向项 |
| 将 Web 的 OpeningURL/opener 直接接入业务协调器可能引入 web/server/页面依赖 | 统一 start 委托本 home Web CLI 的短 open 命令，保留两常驻进程和业务构建无 Web 依赖；定向验证命令目标及原 Web 授权机制 | UST-A1/A6 |

本轮静态复核：`python scripts/check_doc_links.py` 通过（424 文件、2717 本地导航链接、0 错误）；`python scripts/roadmap_task_id.py check` 通过（97 身份文件，无编号/执行清单变更）；`git diff --check` 通过。已重新核对 Spec/Plan/逐项验收映射及新主干消费者，本轮未发现剩余文档阻断项；产品自动项与人工项仍全部未验，审计结论不冒充实现结果。

## 2026-10-15 实施验证记录

### 源码、环境与范围

实现已 rebase 到 `60dae583`（#324），保留 #323 缓存。运行环境：Windows、Go 1.26.4、Node v24.13.0、Edge 155.0.4283.45。包 manifest 固定实际 `source_commit=62157c28`、`source_dirty=true` 及三份二进制 SHA-256；不是以调查 commit 冒充已测版本。包内 helper 是验证所有者，常驻产品仍只有同一 home 的后端与 Web 两进程。模型/PR 为 controlled 模拟，Git、SQLite、原生 worker 进程、浏览器与产品均真实运行；没有执行 live/真实 GitHub PR 验收。

| 验证 | 命令与结果 | 证据 |
| --- | --- | --- |
| UST-A1/A2/A5/A6 host | `go run ./cmd/test-feedback -- ./internal/app ./internal/app/host ./internal/localservice ./cmd/steerline ./cmd/steerline-web -run '^TestUnifiedStartup' -count=1`；通过，40.165 秒。含默认 opener 的实际 exe/参数/home/cwd、并发复用/缺 Web、配置快照/冲突/重叠、零项目、共享 PID 和独立锁、端口归属、原宿主停机结果反例 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T072222.397122900Z/summary.json` |
| UST-A3 原生 CLI | `go run ./cmd/test-feedback -- ./cmd/steerline -run '^TestNativeTaskBatch' -count=1`；通过，52.974 秒。真实宿主下校验/创建/依赖/丢弃/图/控制与原回执，关闭客户端不停止宿主 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T072944.836895100Z/summary.json` |
| UST-A3/A5 API | `go test ./internal/product/httpapi ./internal/product/applicationhttp ./internal/product/applicationclient ./internal/product/httpfs ./web/server -count=1`；全部通过，保留原请求/错身份/认证/JSON 数字与拒绝反例 | 本次终端测试输出；应用路由不扩展浏览器 AllowedRead |
| UST-A4/A5 worker V06/V07 | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestWorker(ServiceStopRecovery\|ServiceStopBoundary\|ShutdownDrainsCommittedDeliveryAck)$' -count=1`（终端执行正则使用正常 alternation）；通过，55.783 秒，覆盖原冻结停止策略/服务换代/提交后停止边界 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T073200.261563000Z/summary.json` |
| UST-A1～A6 L1 | `go run ./cmd/test-feedback -level=L1`；通过，执行 194.3 秒，保留原架构/事务守卫并登记共享宿主 owner | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T072451.637322300Z/summary.json` |
| UST-A3/A6 前端 | `npm --prefix web test`；10 文件、70 测试通过。双 exe 构建通过；业务 `go list -deps ./cmd/steerline` 无 Web 包 | 本次 Vitest/构建输出；最终 SHA-256 随收尾登记 |
| UST-A2/A6 受影响驱动 | `go test ./tests/e2e/internal/driver ./tests/e2e/internal/testenv -count=1` 通过；`go test -tags=e2e ./tests/e2e/... -run '^$'` 全部编译通过。tag 检查只算编译，不算业务运行 | 本次终端输出；live/全量 L2/L3 未选择 |
| UST-A6 源码隔离 | `scripts/test-debug-runtime-isolation.ps1 -Out <新临时目录>`，10 个隔离/路径别名/启动器守卫反例通过 | `C:/Users/lmas1/AppData/Local/Temp/steerline-unified-isolation-380160411ed548e3bd3f0a726ac5fc85` |

### 独立浏览器包

- 缓存回归新包：`C:/SteerlineDebug/查询缓存 独立验证/dev-20261015-152438-62157c28-dirty`。运行 `scripts/test-unified-startup.ps1 -Package <该包> -Simulate -Scenario query-cache`，真实 Edge 通过（2.1 分钟）；原 bootstrap 合并、缓存内容、隐藏轮询、两项目切换反例保留，页面错误 0。`evidence/web-query-cache.json` 记录请求计数、项目身份与 cleanup：产品/owned 进程 0，监听 0，stop_errors 为空；截图/trace 位于 `evidence/browser-query-cache`。
- 综合最终新包：`C:/SteerlineDebug/统一启动 交付验证/dev-20261015-153103-62157c28-dirty`。入口 `scripts/test-unified-startup.ps1 -Package <该包> -Simulate`；真实 Edge 完整通过（5.8 分钟）：两项目同 PID/独立实例、重复启动、两标签页、两个运行中安装目标拒绝覆盖、丢回执原 ID 查证仅一次 POST、Web 重启保持原 native 进程/session/turn、原执行发布/交付、CLI 创建两任务和新增依赖及原回执只读查询、浏览器拒绝 CLI 专有路由、活动执行时整体 clean 停机、全部项目换代、恢复门槛及零新派发、旧原请求保持原实例。`evidence/web-project-switch.json`、`evidence/browser-unified-startup` 保存事实与截图/trace；最终 cleanup 为进程/监听 0、stop_errors 为空，五个 `native-*.json` 退出码均为 0。

### 失败与修复记录

早期调试包保持原日志，不改成通过记录。私有 runtime-launcher 在创建锁之后收紧目录 ACL 导致锁无访问项，已改为先建立私有目录再取锁；进程内路由重复查询完整项目身份，已区分固定路由绑定与公开 readiness 查询，并借用原 runtime 的 reader/store。退出探测的短暂权限错误改为在原期限内保留“未核实”并重试，不能据此假报退出。

`C:/SteerlineDebug/统一启动 最终验证/dev-20261015-151957-62157c28` 在活动轮次处失败于 PowerShell 中文路径输出编码；修复 UTF-8 输出。`C:/SteerlineDebug/查询缓存 最终验证/dev-20261015-152206-62157c28-dirty` 已完成 Web 原轮次保持、CLI 写入/原回执、整体 clean 停机，在同页 fragment 未重新装载授权而返回 401 的恢复检查处失败；驱动改为新文档兑换票据。二者的原 `evidence/web-project-switch.json` 都保留 business_error，最终 cleanup 均为进程/监听 0、stop_errors 为空，均不算综合验收通过。更早的所有者/Job/ACL 探索失败也不算通过。

人工 UST-M1 仍未执行，仅后置体验签收；不把自动 --no-open、假 opener 或模拟 provider 描述为系统默认浏览器的人工体验结果。

### 最终收尾

最终 `build.bat --no-pause` 成功；本地 `bin/steerline.exe` SHA-256 为 `5c81d72f9b4bcf94a4dcb92741e2f1e8c07e570a603e157b0c3f9c5de26ed80b`，`bin/steerline-web.exe` 为 `49f8c47c60bcb1082a5893f6602b0f8305f2f4ecbf6035c73c07de3c129bb2c4`。构建源码为 `c0bcea40`；仅生成本地双 exe，不部署安装目录。

在原成功场景的 JSON、私有最终 shutdown.json 与全部原生启动器退出记录上再次执行一致性核对，两包都为两个项目 local_closed/locks_released/resources_settled=true、clean=true，全部原生启动器退出 0。`evidence/cleanup.json` 是从这些已保存原始事实导出的清理索引，标记 derived_from，不修改原场景记录。驱动收尾增加相同退出码核对并稳定输出 cleanup.json，防止仅因结果文件存在就掩盖非零退出。

补入“坏配置与两个健康 runtime 并存”反例后，`go test ./internal/app -run '^TestUnifiedStartup' -count=1` 再次通过（55.119 秒）；前端再次 10 文件/70 测试通过。独立执行三项原 worker 停服策略测试通过；没有扩大为 Full。最终自动 UST-A1～UST-A7 已通过；UST-M1 保持未验。

文档链接检查：427 文件、2734 本地链接、0 错误；Task ID 检查通过（97 身份文件），未改 roadmap 执行清单；git diff --check 通过。后续文档纯结果登记再次执行静态检查。
