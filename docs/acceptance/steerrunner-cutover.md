# SteerRunner 接入验收

归属：[接入 Plan](../spec/steerrunner-cutover.plan.md)。普通开发，不纳入 roadmap。Runner 内部及真机验收由其独立仓库 `docs/acceptance/steerrunner.md` 管理。

最新状态：本轮按用户要求完成自查、直接修复并分别交付 PR，不宣称独立复审，不自动合并或发布。此前实际产品受控闭环、本机独立 Runner 真机及三个 R1 的通过只归属各自版本；最新产品真机补验失败、最终固定版本 R1 尚未通过，不能声明完整交付。最新细节见末尾审计段，Orca 两路径均已取消验收。

| ID | 自动验收项 | 当前结果 |
| --- | --- | --- |
| SLC-A01 | 公共 CLI 严格解码、丢响应原请求、终态不重发、显式新授权 | 通过；最终定向证据 final-directed.log |
| SLC-A02 | 候选原子领取和固定请求、绑定/事件/游标 CAS、终态停止义务 | 通过；最终 SQLite/Task 定向证据 final-directed.log |
| SLC-A03 | 正确合并候选唯一交付、无 pr_ready 门槛、兄弟未知不阻塞、原子回滚与幂等 | 通过；最终 SQLite/Task 定向证据 final-directed.log |
| SLC-A04 | 实际产品命令→Runner CLI 的完整闭环、双库、重启和停止 | 独立 EXE 受控产品闭环及 R1 重启/停止已通过；新产品真机包另行补验 |
| SLC-A05 | 最小既有 L3 场景切换到新 CLI 后通过 | 通过；用户保留三个本机场景，精确证据聚合见 regression/local-r1-summary.json |
| SLC-A06 | 单一执行 owner、旧 actor/worker/适配器/写表删除、固定可获取 Runner 版本 | 通过；旧 owner/写表/闲置端口删除，公开 Runner 源版本/SHA 已固定，边界守卫及构建通过 |

人工项 SLC-M01：两个独立项目的安装/升级、服务与日志定位体验；窗口待双方自动链路可用后安排，可后置，不阻塞无关开发。

所有运行证据归本 worktree `.evidence/steerrunner-cutover/`，登记实际源 commit/dirty 状态、命令、退出码及非零场景数。代码和文档尚未提交时的通过只说明当前定向边界，不代表最终版本验收。

2026-10-15：`client-release.log` 的 runnerclient/runnerbridge 定向通过，新增每次调用前 EXE 摘要校验、公开 schema 校验及外部 $ref 零读取。`control.log` 的 `go test ./internal/storage/sqlite -run '^TestRunner' -count=1 -timeout=2m` 通过：包含候选/交付、Task service、真实 host scheduler、WAL 快照、调度事务回滚和已领取但外层回执丢失后的原 ID 恢复。测试保留了夹具缺少 profile digest、契约目标和验收标题时的失败；修正夹具后运行成功，未放宽生产规则。

实际 app service 已使用 taskrunner actor，版本 3 配置固定独立 Runner EXE/版本/SHA；答案、resume、作废、候选控制和 TaskDelivery 均经过公开 caller 边界。新 SQLite schema 21 删除旧执行/原生会话写表，旧根执行 actor/worker 和其专属 in-process 测试已物理删除。剩余原生 owner 和测试辅助依赖现已删除；五个 L3 的最终结果见下，尚不能声明完成跨仓交付。


2026-10-15 迭代证据（当前 source dirty，不作为最终固定版本）：

- `all-test-compile-2.log`：全部 Go 包只编译、不运行完整测试，通过。
- `pure-contract-tests.log`：app、runnerclient、runnerbridge、taskrunner、SQLite 定向通过；storage ports/SQLite 只引用 `runnercontract`/`runnerstate` 纯值，不反向引用 CLI 应用实现。
- `caller-boundary-input.log`：taskrunner、边界 checker、storage ports 与产品 CLI 依赖闭包通过；含多原生问题完整键集/选项/输入不可变及单题展示。
- `caller-integration/result-8.json`：实际启动命令已接受，Runner 缓存 clone 失败，1 场景退出 1；未创建 Run/模型输入。
- `caller-integration/result-9.json`：1 场景退出 1，补充的脱敏诊断证明 Windows 深目录加专属分支名超过 Git 默认路径限制。Runner `5f664a9` 增加进程内 scoped longpaths，真实长分支 clone 测试通过，未修改全局配置。
- `caller-integration/result-10.json`：1 场景退出 1，调试命令扫描到原子凭据发布的瞬时双链接；修正只读扫描边界，重置在停服证明后校验删除树。需要新包复验，不能将此记录为通过。

旧测试退休范围见[删除审计](steerrunner-deletion-audit.md)。原生执行专属用例迁由 Runner 已迁出的测试覆盖；这不表示旧全部集成覆盖已等价替换，新 CLI 跨进程和 R1 场景仍为必验。


检查点 `849f8737` 已提交实际切换及旧根 worker 删除（441 文件，删除约五万行）。随后 `caller-integration/result-11.json` 的 1 场景仍失败：已接受原 Run `eda670184228da1b881ea8dc3e07f667` 并创建原会话，受控 helper 在新缓存中提交时缺 Git 作者配置。修正夹具为每条 Git 命令显式固定测试作者、禁签名与长路径，不让 Runner 复制源仓库或用户配置；`fixture-git-identity.log` 的三阶段 Git/PR 夹具测试通过。原 Run 经公开 CLI 确认 stopped、服务 not_running，证据 `caller-integration/cleanup-original-11.json`。

`regression/r1-first/summary.json` 的 delivery/basic 与 waiting-merge/restart 两场景失败且 cleanup_complete=false；不计通过。旧清理入口只终止父进程并提前删除目录，暴露常驻 Runner 的清理缺口。已增加仅使用固定 EXE/公开 CLI 的清理：先围栏停止原服务，必要时显式启动仅清理的 epoch，查询原 request/Run、停止原 Run、确认服务退出；不发业务 resume，未知则保留现场。旧失败现场的 discovery 已被旧清理误删，按原启动日志 PID/实例、创建时间、保留句柄、映像 SHA、命令目录与 SID 核对后结束其两个受控测试 daemon，单列 `verified-forced-cleanup.json`，不当作原生停止回执。

`current-start-graph.log` 的 taskrunner、SQLite Runner 场景和测试入口定向通过。启动请求的图版本同时进入原始 Start 摘要并与 FixedWork 图版本比较，最终由领取事务的同图 CAS 保护；旧图在获取 base/派发前拒绝，既有原 ID 回执仍先返回。独立 EXE 与五项定向场景的业务窗口包含额外服务/CLI 边界，调整为 10 分钟，单命令时限不变，父测试另留清理窗口。


`caller-integration/result-12.json`：ef780c60 + Runner ede6dc5，1 场景退出 1（603 秒）。原会话实际实现和报告通过，发布受 Git `$GIT_DIR too big` 阻塞；新清理成功围栏原服务、创建仅清理 epoch、停止原 Run 后关闭服务，当前状态 not_running。Runner 6b18f3e 收短私有缓存布局并通过真实 linked-worktree push 回归，待新包。

R1 余下三场景已改用公开候选/Run/Attempt/session、原生夹具次数和产品 resume 控制；Orca 夹具补公开 terminal list 及独立 agent-start 接受，agent 启动不计业务输入。`r1-public-recovery-compile.log` 编译通过，`public-terminal-fixture.log` 定向通过；尚未计五场景行为通过。`web-answer.log` 的 6 个回答交互测试通过（含多题 JSON 不重复为 note），`web-build.log` TypeScript/Vite 构建通过。

审计并移除旧 `test-debug-release.ps1 -Live` 的包内 token 读取/导出和宿主代 worker 发布路径，改为实际原生工作流入口；固定 EXE 选择贯通生成及拒绝路径验证，PowerShell 语法检查无错误。该入口 live 尚未执行，不作通过记录。


后续 P6 检查点：

- `caller-integration/result-13.json`：SteerLine 干净 `025de7fc` + Runner 干净 `6b18f3e`，独立实际产品包 `C:/SteerlineDebug/caller/dev-20261015-130023-025de7fc`，1 场景 exit 0（137.44 秒）。公开 CLI 完成三阶段、真实 Git push、受控 PR 合并、唯一 TaskDelivery/后继释放、原 Run 停止、服务停止及重置隔离。PR provider 为受控环境，不是真实 GitHub PR。
- 物理删除 `execution/{adapters,workflow,reliable,conversation}`；删除闲置 worker/claim/result/discard 事务端口及旧 native 重建函数。workspace 只保留调用方仓库身份解析；publication 只保留调用方配置校验，不再持有 PR 查询或 native worktree 操作。原生专属测试在 Runner，SteerLine 测试只使用公开 CLI/投影和外部工具自身证据。
- `retired-owner-directed-2.log`：跨 OS 源码/依赖守卫、e2e 导入负例、Task/SQLite/公开查询定向通过。`public-native-directed-3.log`：原生工具自身 usage/inventory、精确自有子进程故障、外部认证引用零复制、公开诊断过滤通过。故障测试保留首次 CIM 微秒精度与 FILETIME 精度不一致失败，统一精度且保留句柄后通过。`e2e-cutover-compile-final.log` 为 e2e 全包只编译通过，不计行为覆盖。
- R1 `r1-public`：冻结 `7b42cd7f` 和 Runner `6b18f3e`；基础交付通过 155.51 秒。本机 terminal 因夹具错误携带空 environment 被严格拒绝；codex resume 因刷新前提交过期版本失败；environment unknown-send 因夹具只识别裸 codex 命令、未识别真实初始化命令失败，且清理遇已明确拒绝的 stale_revision。修正夹具、原请求未持久化时的有界版本刷新和同一停止目标的有界刷新；所有失败保留，待新冻结版本复验。
- Native helper 不再复制认证、配置、hooks、token 或 sandbox 凭据；既有原生登录留在原外部 home。诊断不遍历 Runner 私有库/日志或 native auth/config，只导出公开身份与由原生 rollout 筛选的本 Run usage。真实发布仍须通过正常原生工具，不能以宿主代发绕过失败。


范围与 rebase 补充：最新主干 `317714e6` 的独立 Web/项目切换和查询优化已合入；切换分支 rebase 完成，`rebase-compile-2.log` 全包只编译通过，`rebase-app-runner.log` 的 TaskRunner/App 定向通过。一次 Web 定向命令误选不存在的 internal/webhost 包，整体 exit 1；同次 App/WebSwitch、applicationclient/applicationhttp/projectconnection 各包通过，不能把错误入口的整个命令计作通过。

`r1-user-host` 的 TestCodexResume 通过（316.41 秒，干净 cf0b818f），原 Run/thread/workspace 不变、新 Attempt 显式恢复、四次业务输入完成交付。其本机 Orca 历史失败因用户明确取消该路径不再重跑。用户已明确远端原生环境修好，恢复 environment 真机验收。

Runner 第五轮远端已完成实施，在报告阶段暴露应用冻结策略错误；使用原 worker 的实际 PolicyCapabilities 代替本机默认，Runner 新构建 c85f4e7 单元通过，需新包复验。修正不绕过原策略资格检查。之前受控 unknown-send 清理遗留的原 Run，已通过同用户前台 serve 的显式清理 epoch 确认 stop_status=stopped，服务 not_running，见 `r1-public/unknown-original-cleanup.json`。没有按猜测 PID 清理或发送业务恢复输入。


最新用户调整为仅完成本机 Codex 验收，两个 Orca 路径均不再作为本次待完成门槛；第六轮远端在通知时已退出，原 Run 66c944425084d05d2401eeea5961cf82 的 stop_status=stopped，服务发现不可用，无 PR。不能记为远端通过。rebase 后冻结 ed44a08b 的 TestCodexResume 再次通过（261.02 秒）；余下本地等待合并重启场景正在运行。


本机 SR-A11 已通过：Runner c85f4e7 的 `C:/SteerlineDebug/srlocal/dev-20261015-141218-560ce11e/evidence/native-20261015-061410/result.json` exit 0、1 场景、235.62 秒，原生 AppContainer/TEMP/项目检查/相邻写拒绝通过，原 session 三阶段完成 result.txt 提交 `5d74b109c9f6035029ac8152d360d8fb36039b1b`，Runner 核验真实 [SteerlineTest PR #253](https://github.com/erranliu/SteerlineTest/pull/253)，原 Run stopped。普通发布先 401，原生自动审批批准同一命令后成功；Runner 自有只读 provider 核验 PR，不以模型后续 401 查询结果代替事实。

更深 R1 目录随后暴露 Windows Git 的管理路径长度限制，外部夹具已将原始错误写到自有回执，无需读取 Runner 私有日志；`local-waiting-2` 失败保留。Runner dc24205 将私有缓存与工作区叶目录编码为 128-bit base32，完整 Run/分支/输入摘要不变。更深真实 Git push 回归通过，实际 EXE workflow/PR identity 两组通过。SteerLine 已固定可获取版本及 SHA；新本机产品包 `C:/SteerlineDebug/slcodex/dev-20261015-143226-0f86647b` 正在执行实际产品入口→原生→真实 PR/TaskDelivery。

外部 rollout 绑定/指定轮次命令/沙箱证据检查 `external-rollout-3.log` 通过；live driver 不再读取 Runner 私有 journal，原生自动审批由 Runner 自身验收负责。rebase 后 Web 回答交互 6 项通过，TypeScript/Vite 构建通过（`web-answer-rebased.log`、`web-build-rebased.log`）。


`local-waiting-3` 在干净 ca5d8563 + Runner dc24205 下，TestWaitingMergeRestart 通过（319.75 秒）：原 Run/Attempt/session/workspace/branch 保持，重启零重复投递，原 PR 合并后唯一交付和后继释放，清理通过。本次 R1 的三个本机场景均已有通过证据，未扩大为全部历史恢复矩阵通过。

最新产品真机包第一次测试启动参数被 PowerShell 拆分（exit 2，零场景）；修正仅外部 harness 参数后，实际 1 场景失败（14.75 秒），调试 helper 在启动 SteerLine 前推送本次专属 base 时显式清空 credential helper，未引用本机已有 gh 登录。没有 Run/模型派发/PR；失败包冻结保留。修正 live 模板只显式引用 `gh auth git-credential`，不读取/复制/导出凭据；须新生成包复验。


产品包 `dev-20261015-143940-151259b5` 在初始化时被严格无内嵌认证校验拒绝：Git 参数里的标准 helper 引用也被其保守规则拒绝。没有启动业务服务/Run/模型/PR。配置契约不扩宽；live 准备阶段只在本包仓库 Git config 写 `!gh auth git-credential` 的非秘密引用，实际认证仍由外部 gh 所有。生成失败包保持 failed，继续用新包验证。


`dev-20261015-144115-879aa39a`：104.56 秒失败，原生会话创建前版本核验超时，零模型输入，清理通过；相同 EXE 单独版本核验 0.427 秒。新包 `dev-20261015-144556-879aa39a` 同源复验已通过权限资格、原生探针、实施和报告；219.33 秒失败于发布 final 的 permission blocker：普通发布/匹配 PR 查询均 401，代理没有调用原生自动审核。原生审核未被请求，不能写成被拒绝；未创建 PR，清理通过。对齐已通过的 Runner 独立 L1 提示：本机 live 准备先核实外部 gh 登录，冻结 Spec/Plan/AGENTS 明确同一发布命令只申请一次原生自动审核，任何拒绝保持阻断；须新包复验。


最终边界与文档核验：`final-owner-boundary.log` 的生产/测试源码及依赖守卫通过；`doc-links-final.log` 425 份 Markdown、2534 个本地导航链接、0 错误。`regression/local-r1-summary.json` 只聚合本次用户保留的三个场景的原始通过和清理事实，明确记录各批源版本及整个旧批次是否失败，不伪装同版本重新执行。两仓 PR 已互相引用：[SteerLine #325](https://github.com/erranliu/SteerLine/pull/325)、[SteerRunner #1](https://github.com/erranliu/SteerRunner/pull/1)。

## PR 审计修复与主干同步

本轮自查确认并修复：

- SteerLine 的多题回答文本原用 json.Unmarshal，重复键会静默覆盖；现先使用已有 CanonicalJSON 拒绝重复键、尾随值和含混 JSON。合法题目/选项及调用者原始输入仍保留。
- runnerbridge 只在新投影 PR 非空时检查原 URL，允许已绑定 PR 消失；现拒绝消失或替换，并在校验失败时不改变整个原候选。
- Runner 自身同步补齐原题集、答案数量/内容及严格 JSON 验证，拒绝发生在原生意图前。SteerLine 固定其可获取实现提交 `41908e641b34a890448158d2f21a9edc597d08cb`，版本 `0.1.0-dev.1`，SHA-256 `5eb9c3a9162d13449d55d363dd136e0ee50dea0d1e06ac5afaa8b439b73e5a71`。实际独立 EXE 包检查通过，详见 Runner 专属验收 §9。
- 接入最新主干 `60dae58306ed2dfe6daec3cd3ecf89bb0e80a7db`，保留 #324 的 TUI 退役、Web-only 和双项目调试检查；原 live 验收仍使用全新包及真实 Runner，不恢复读取 token/宿主代发布路径。恢复 HTTP 检查改为比较公开 caller RunnerSnapshot；候选服务使用主干的 serviceActivityFormatter。
- 新定向验证发现 CLI 原回执 fixture 的 checkpoint 漏写 claimant，按真实配置补齐后，生产身份门槛不放宽；CLI→原请求重放→HTTP 原回执→关闭客户端不停止服务的嵌套测试通过。

最小验证：`audit-input-binding.log` 及 `audit-after-main.log` 中 taskrunner/runnerbridge/app/test-feedback/受控三阶段均通过；后者原 CLI fixture 失败保留，修正后的 `audit-cli-http.log` TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs 通过（115.14 秒），含 CM71/CM72。`audit-boundary-main.log` 的生产/测试全标签依赖守卫和 Runner 公开边界通过；`audit-driver-main.log` 的环境隔离/业务期限/原生命令阶段检查通过；`audit-retirement.log` 的退役命令和 launcher 检查通过。`audit-main-compile-final.log` 全包只编译及 `audit-tagged-compile.log` 的 e2e 标签编译通过，不是全量测试。`audit-doc-links.log` 为 428 份 Markdown、2553 个本地导航链接、0 错误。PowerShell 调试脚本语法检查通过；未重新启动整套生成/Web/live 测试。

最新失败保持：`C:/SteerlineDebug/slcodex/dev-20261015-150152-bba00436` 为干净 SteerLine bba00436 / Runner 7bede6e，服务退出，原清理 Git 子进程报 `0xc000026b`。候选提交为 unknown，未绑定 Run 不能证明零接受。随后使用原冻结 helper/公开 CLI，在独立外部宿主核清原请求并停止，`evidence/original-public-cleanup-external-result.json` exit 0、日志 confirmed=true；没有业务恢复或新发布。工具 Job 内曾被 `detach_unsupported` 安全拒绝，保留该失败，不放宽服务 token 检查。`local-final/e2e-1.jsonl` 的 TestBasicDelivery 同样报 Git `0xc000026b`，批次没有完成汇总，不计最终版本 R1 通过。此前真实 Codex PR #253 和三个 R1 原通过记录仍有效，但不能覆盖这次失败。

## PR #325 统一启动主干冲突解决

接入主干 `0228e51d7b5cfe3a3a4c0e9d068df8f732a9badd`（包含 #326）。保留统一 start/serve/stop/status、应用 v2、共享宿主及独立 ProjectRuntime/store/实例/锁；不恢复被迁出的 execution actor、native codec 或旧格式。冻结配置读取改为 Runner v3 快照，项目使用 Task actor 和固定 Runner CLI，各项目读端借用所属 store，任务写入串行提交并发布 caller checkpoint。

原主干 ResourceSettlementError 接口改由 Task actor 提供，停服与资源事实分开：unknown 提交、缺投影及原 Run 未证明 stopped 时不能报告 settled；构造失败可能已有独立 Runner 时也保留未知。五种资源状态反例与原停止幂等/Task actor scheduler/命令文件身份检查通过，证据 `conflict-unified-actor.log`。

`conflict-unified-tests-2.log` 的八个统一启动定向测试通过，覆盖共享 PID/不同实例、配置冲突与冻结、零项目、组件重用、未知创建、停机证据与安装 Web opener；app 合计 27.409 秒。CLI Task 批次/依赖/作废及原回执与关闭客户端不换实例，在 `conflict-unified-tests.log` 单包通过（43.263 秒）。快速测试清单合并 app 包的登记后，scoped/full coverage 校验通过；该日志中的首轮登记重复和 opener helper 遗漏失败保留。合并时补回了仅供测试的 opener 分支，未恢复原生 codec；首轮递归产生的测试 opener 已按精确临时路径、命令和创建时间核实并清理。

共享模拟宿主通过生成时的测试私有路径路由，将 Runner 工作区匹配到原 provider，不读取 Runner 私有内容。两份同公共 Git 仓库契约共用模拟 Git transport，保持各自 base/head、clone/common-dir、provider、caller DB 和 Runner state-dir；继续使用真实 Git push。`conflict-unified-routing-2.log` 的原三阶段/反馈复用及新路径路由反例通过，第一次测试遗漏 seed 目录的失败单列保留。该定向验证不等于完整双项目 Edge/模型/PR 矩阵已复验。

`conflict-unified-boundary-final.log` 为源码/跨平台生产闭包、测试依赖与 Runner 公开边界检查；`conflict-unified-compile-final-2.log` 与 `conflict-unified-e2e-compile-final.log` 记录全包及 e2e 标签只编译。Web TypeScript/Vite 构建通过（`conflict-unified-web-build.log`），生成与测试 PowerShell 脚本语法通过，文档检查 431 份/2596 个本地链接/0 错误（`conflict-unified-doc-links.log`）。没有运行全量测试，没有新 live/Orca 验收，最新产品真机与最终版本 R1 的已有未通过状态不变。
