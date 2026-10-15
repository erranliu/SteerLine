# 统一启动与多项目共进程宿主验收

专属普通开发任务：[Spec](../spec/unified-startup.spec.md) · [Plan](../spec/unified-startup.plan.md)。2026-10-15 用户要求规划并排除 TUI；仅文档已生成，产品未实施、未执行自动/人工验收，无 roadmap Task ID。此记录只归属本任务，不复用前序 Web 项目切换的通过状态。

## 自动验收

| ID | 最小检查与通过条件 | 实施后步骤 / 证据 | 当前状态 |
| --- | --- | --- | --- |
| UST-A1 | 单 bat 自动启动双进程并授权打开网页，重复/并发与半启动只补缺项，失败窗口及退出码可见 | host 定向反例 + 新 Edge 包；start/status、精确进程/就绪/打开结果，见 Plan 测试表 | 未实施 / 未验 |
| UST-A2 | 两真实项目同 PID、独立实例/DB/actor，零/坏/慢/冲突/跨 home 隔离，身份解析总期限、锁先于初始化、manual 和配置快照正确 | host fixture + 新包状态/锁/数据库归属事实；失败项目无初始化副作用 | 未实施 / 未验 |
| UST-A3 | 应用 API 直达原项目用例，原生/浏览器认证、固定 CLI 操作及批次/依赖原回执 GET、大整数、旧实例/同号任务隔离；旧 stop_service 不变成整体停止 | api/CLI/前端定向反例 + 实际写入、丢回执只读查证零重放；保留 #323 查询回归 | 未实施 / 未验 |
| UST-A4 | Web 重启不影响活动轮次；后端停启覆盖全部项目，逐项停机结果与精确退出/租约共同核实；冻结策略及重开零自动输入/答案正确 | lifecycle 两种策略 + 原宿主 shutdown.json + 新包活动轮次/后端重开，记录原身份/授权/副作用 | 未实施 / 未验 |
| UST-A5 | 项目错误不误停兄弟；本地 actor 未关闭与 worker 未知分别处理；停机结果缺失/旧实例/部分失败不假报成功；已退出 stale 可恢复、未知进程不接管，端口/超时/部分启动诊断正确 | host/api/lifecycle 反例；正常与失败停机结果、原回执、实际进程/租约/项目锁分别取证 | 未实施 / 未验 |
| UST-A6 | bat 双产物、资源和安装守卫；旧启动器清理仅限 owned 文件，路径/cwd/ACL/日志正确，调试端口/冻结正确，无新增 TUI 支持 | 构建 + 隔离安装/ACL 反例 + 新生成包 manifest；两 exe SHA-256、路径和权限对照 | 未实施 / 未验 |
| UST-A7 | 统一入口 Edge 双项目选择/两标签、命令与丢回执、Web 活动轮次重启、后端停启/授权门槛；另包 query-cache 保留查询优化，分别清理完整 | Plan 中 web.unified-startup 与 web.query-cache 两场景、不同新包；截图/trace、原身份、CLI/请求计数、各自 cleanup JSON | 未实施 / 未验 |

Plan 中的新 Go 场景、wrapper 和生成器扩展必须先实现。不存在的入口、零匹配、仅编译、模拟 provider 的自述或历史通过均不算验收。L1 是定向反例的长期回归入口，不能替代真实产品接线场景。

## 环境与执行记录要求

1. 开始前记录实际源码 commit/dirty、Go/Node/Edge 版本、命令和起止时间；沿用 Plan 的源码调查版本不代表已经验证当前源码。
2. 实际产品调试只能先运行唯一生成器，在 `C:\SteerlineDebug` 为两个场景分别生成全新双项目包；公共 Node helper 先 fresh 校验，再通过包级 `--simulate` 冻结一次并调用统一启动，wrapper 不提前冻结。两个独立 SteerlineTest clone/provider，manifest 固定双 exe/hash/端口。禁止使用安装目录或复制其数据库、认证、配置。
3. `evidence/` 保存宿主/项目实例、精确 OS PID/创建时间/路径、原请求/执行/会话/turn、CLI 状态与日志、Edge screenshot/trace。当前未生成验收包，路径在实际执行时登记，不能预填不存在的现场。
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
