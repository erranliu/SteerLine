# Worker 停机对账与恢复一致性修复验收

唯一归属：#87 Worker 停机对账与恢复一致性修复。[Spec](../../spec/worker-stop-recovery.spec.md) · [Plan](../../plans/worker-stop-recovery.plan.md) · [草稿](../../Roadmap/drafts/worker-stop-recovery.md)。用户于 2026-10-09 授权本聊天直接执行，不经 roadmap-runner；启动与实际结果见下文，仍未修改安装 101。

## 现场诊断基线（不作为修复验收）

2026-10-09 只读检查 `C:\SteerlineRuntime\state\SteerlineTest-b55e8cc42ae8\steerline.db` 及原 runs/service 日志。受影响产品 Task 为 101；它不是仓库修复 Task #87。

- run `1f20a4d6ff67c8511f3d682fe3f668b3`；execution `4c0789792168d8ea6237e94d3d47f847`；Attempt `63f8bca87ad42980a05468f3770d96da`；原 thread `01a11ecc-e1b6-7be1-b607-ccdfd602fa35`；原 turn `01a11ecc-f0bd-7a01-b139-3f05c5906b4c`。
- 北京时间 11:56:15，stop Operation `cc0deb06bbe14e5ecc3fbd540c5fcb9d`，kind `stop_worker_observation`，原 endpoint `ws://127.0.0.1:62956` 拒绝连接，状态 `outcome_unknown`。
- 11:58:39，Decision `5287b70aaea599f6256639177979fb9f` 的 continue 答案保存为 `answer_saved`；随后 Error 为 `An earlier external operation has an unknown outcome; no recovery input was sent`。只有一个 Attempt，原进程 PID 39520 在检查时不存在；原日志有 turn/started，没有 turn/completed。
- 新服务日志声明构建来源 `d8d66d6c5696f41d7df1ed3ff50d4b3ae77b9da3`；原停服服务来源 `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5`。这次跨构建现场只用于定位，不能作为同构建恢复验收。
- 源码确认停止记录未进入 lifecycleSnapshot 映射/过滤与恢复 stop 分支，hasUnknownWorkerOperation 则会阻塞它；正常 shutdown 的 interrupt 在 detach 后 defer 执行。后者是需复现验证的顺序风险，未凭现场日志断言 endpoint 退出的唯一原因。

没有修改数据库、发送恢复输入或重启安装服务，没有复制原始数据库/认证/完整日志入仓库。上述历史未知回执和失败不得改称成功。

## 自动验收（worker 生命周期边界）

2026-10-09 按用户明确要求补充 worker 停服行为标识和应用恢复边界：local 停止、Orca 继续必须分别证明。WSR-A1–A7 沿用并细化，新增 A8 的 Orca 真实活动停服/离线完成验收；启动时没有既有通过结果；当前逐项状态按实际测试更新。

| ID / 方法 | 行为与最小通过条件 | 入口 / 环境 | 状态 |
| --- | --- | --- | --- |
| WSR-A1 / 自动 | 两种 ServiceStopBehavior 由 worker 声明、随执行冻结；缺值/非法/能力变化及旧格式在副作用前拒绝。无 live observer 时仍可构造原 worker 入口；应用只消费规范事实，不解析实现身份或按 backend/protocol 恢复 | Plan owner/boundary 表驱动与冻结/版本负例 | 通过；精确候选/映射见结果登记 |
| WSR-A2 / 自动 | local 先门槛后精确 owned 树停止，零 interrupt/新 stop observation；Orca 只 detach，零平台关闭/停止，原 worker 能继续。准备晚到不漏资源，一个错误不漏兄弟，关闭 observer 不被当 worker 已停 | Plan owner +双 worker adapter 事实测试 | 通过；精确候选/映射见结果登记 |
| WSR-A3 / 自动 | 预期标识不覆盖实际事实；旧写者未排除/身份变化保持 unknown；支持格式原 stop 仅凭精确事实收尾，保留 RPC 未知，不伪造 completed；未知 start/input/reply/publication 不因进程退出消失 | Plan owner/adapter、既有 unknown publication 回归 | 通过；精确候选/映射见结果登记 |
| WSR-A4 / 自动 | 单次完整分类，展示/受理/投递资格一致；fresh answer 只消费一次，接管/实质改变保留旧答案关联新确认；未知发送只对账；相同 poll 不换状态/Decision/版本/活动，重复命令不多 Attempt | Plan owner +正式 Command 受控集成 | 通过；精确候选/映射见结果登记 |
| WSR-A5 / 自动 | 继续型 worker 的原运行/待答/停服期间 completed 均可接回；原结果/ack/业务结算不重复、不补发 worker_done，不把结果当 TaskDelivery；晚到不改新 Attempt/兄弟，unknown 不饿死无关执行；Query 反映中心离线与实际 worker 状态 | Plan owner/adapter 与双路径 Query 集成 | 通过；精确候选/映射见结果登记 |
| WSR-A6 / 自动 | 同构建受控双 worker：local 停止后重开不发送，正式继续同会话新 Attempt；Orca 原运行/待答/离线完成接回原 Attempt/session/Dispatch；重复命令/再次重开不重派、误停止或重复结算，清理完整 | TestWorkerServiceStopRecoveryControlled，复用 integration 的双 helper 和正式接口 | 通过；精确候选/映射见结果登记 |
| WSR-A7 / 自动 | Windows 真 local 正常停服实际结束旧树，重开不新 turn；正式 continue 复用原 thread/workspace、新进程/Attempt/turn 真实完成；host-exit 原分支和清理保持 | 现有 TestLocalCodexManualRecoveryLive 扩展正常停服，命令见 Plan | 通过；精确候选/映射见结果登记 |
| WSR-A8 / 自动 | 本机 Orca 真活动停服不停止原 worker，重开接回原 Attempt/terminal/Dispatch；再次停服让原轮次真实完成，重开接回输出及结算一次，零新增业务输入/替代执行，所有原生证据及清理通过 | 现有 L3 live recovery.orca-codex 扩展活动/离线完成，命令见 Plan | 通过；最终记录见下文 |

准备：用户确认纳入/固定文档；实现冻结标识及版本、worker 统一入口、各拟新增测试和现有测试分支；按 Plan 补 controlled 离线行为和 Orca 最小等待窗口。A7/A8 需要本机目标工具/认证/当前权限，独立 scratch/owned 资源，禁止操作安装 101 或现有 Orca 终端。缺场景、skip、零匹配或缺证据不算通过，自动失败阻塞对应交付。

A8 的原轮次离线继续允许 usage 增长；通过条件是身份/原轮次保持及没有新增业务发送，不能用旧 waiting-merge 场景的 usage 不增长断言替代。可信完成/原消息确认及业务交付分别核对。标识是新增冻结字段，旧构建格式拒绝；历史 101 不改称已恢复。

无新增人工活动或环境框架，不新增 L3 suite；现有 native/driver 最小扩展须实际可用。真实场景使用隔离任务和 loopback provider，不操作安装数据；原一期长期签收及安装替换另行安排。

## 结果登记

执行后逐项记录 actual commit/dirty/tree、exe/CLI/构建摘要、冻结 worker 标识及版本、精确命令/子场景匹配、日期/结果/清理、原/新通用身份、原生证据、start/send/stop/settlement 计数和临时产物目录。未知回执与后续核实分别留证，失败不覆盖，原始日志不入仓库。

纳入执行后同步 execution-map 的结果/待验链接，PR 合并与产品验收分开。本任务已实现完成，WSR-A1–A8 自动通过，实际合并事实见末尾交付结算；历史 101 证据保持原基线。

## 2026-10-09 直接执行与验证过程

启动：固定 Spec/Plan commit `a159d1ef7da02f38d610fa581b6f6f87ef5fb094`，路径为 `docs/spec/worker-stop-recovery.spec.md` 与 `.plan.md`；代码起点 `d8d66d6c5696f41d7df1ed3ff50d4b3ae77b9da3`，分支 `codex/worker-stop-recovery`。未操作 `C:\SteerlineRuntime` 安装服务/数据库或用户现有 Orca terminal。实现后于 `7b99b05f` 合入实际最新主干 `26f98994`，保留新并行启动与远端能力；共享测试 lifecycle 接线一起适配。

冻结版本：profile 9、checkpoint schema 20、builtin-execution-v4；缺失/不匹配停服能力或旧格式拒绝。local 的 stop_execution 使用精确 owned Job/进程组退出证明，不再为服务停机发 interrupt；Orca continue_execution 只退出观察。恢复只调用 worker 的 Reconcile/Observe/VerifyManualRecovery，待答和真正 completed 分别核实；session 是否可继续另存规范事实。未知 publication/input/reply 保留门槛，原消息确认成功或可信结果不能被取消 probe 覆盖。

原始产物均在 `C:\Users\lmas1\AppData\Local\Temp`，仓库仅记录摘要。以下候选与结果不自动套用于后续源码：

| 范围 / 候选 | 实际结果 / 最小证据 |
| --- | --- |
| A1–A5 定向 owner/adapter、未知/兄弟隔离；`60f20aeb` | `steerline-test-feedback/20261009T050801.634252600Z/summary.json` 通过，100.06s；后续增量重新验证 |
| 同范围 + preparation、晚答重试、CD64、remote lost Dispatch；`7b99b05f` | `steerline-test-feedback/20261009T061033.558821900Z/summary.json` 通过，39 tests/0 skip，243.34s；准备单测证实原 owner 在 resolver 故障后释放，回执不创建 unsent business intent |
| L1；`f199472a` / `7b99b05f` | `steerline-test-feedback/20261009T060327.750947400Z/summary.json` / `20261009T061032.184653400Z/summary.json` 均通过；后者已包含最新主干集成，不作为产品 A7/A8 证据 |
| A7 normal_service_stop；`f199472a` | `steerline-87-local-final-20261009.jsonl`，1 subtest PASS，223.32s/0 skip；old PID 43228 → new 43776，同 thread `01a11f44-20f8-7cf0-8ffa-4ddd307f6035`，old Attempt `7610c25aaa830d0090aa4b44645f426e` → new `63772c6a655d13b561b2d156a00d505a`，原/新真实 turn 完成；重开前无新输入，正常清理 |
| A7 host_exit；修复夹具环境后的 `60f20aeb` 工作区候选 | `steerline-87-local-host-exit-20261009-131431.jsonl` PASS，106.51s subtest；old PID 42560 → new 44568，同 thread `01a11f16-28b4-7cb0-8343-a2efc20b46f3`；新 Attempt 仅在正式 continue 后生成。先前 codex.cmd shim 与 Git fixture 环境缺失失败保留，未算产品通过 |

失败与后续核实分别登记：

| 证据目录 / 范围 | 原失败 / 处理 |
| --- | --- |
| `steerline-test-feedback/20261009T051740.922459100Z`、`20261009T052953.690122500Z` / A8 | 活动原执行及离线真实 PR 已发生，等待合并再次重开却落 waiting_decision；取消中的读取生成未知决策，已完成 PR 未及时恢复合并 gate。修复取消 probe 与 completed PR 路径，二次验证不以该局部成功替代完整 A8。两次 cleanup_complete=true |
| `20261009T051616.952957900Z`、`20261009T052839.277881000Z`、`20261009T054648.310915600Z`、`20261009T060326.710986600Z` / A6 | 分别暴露 exited terminal 误用 live 验证、原 active Delivery ack 取消后未知；最后一次达 Go 总超时，pending-question 分支未完成。修复可信退出结果观察、独立有界 ack 与取消后原回执 drain；不得覆盖历史失败，需全分支复验 |
| `20261009T055323.964548000Z`、`20261009T060327.752585600Z` / preparation 单测 | 新测试缺有效工作区绑定、即时读取 Job close 后异步退出。补完整 WorkspaceEvidence 与 5s 有界退出观察；后者没有改生产停止语义，`7b99b05f` 定向通过 |
| `20261009T055536.855746500Z` / A8，`8e6fb839` | readiness Dispatch 身份拒绝，未进入活动窗口；cleanup_complete=true。当前 Orca worker-show 对手动注入返回 unsupervised/context_only；原隔离 Dispatch `ctx_98bbcbecc4cb` 的 exactWorker=true、runtime/terminal/incarnation/worktree 匹配。`df21916a` 改为要求 exactWorker 与完整原绑定，缺失/false及替换身份均拒绝；仍保留独立 input/turn 提交证明，未伪造业务开始 |

独立 reviewer `/root/review87` 对 `8e6fb839` 与主干集成/Orca 身份候选 `df21916a` 均未发现新确认问题；此前确认的问题已逐项修复。`5bc3f7fa` 定向 `TestWorker(ShutdownDrainsCommittedDeliveryAck|PreparationStopDoesNotJournalUnsentBusinessTurn|ServiceStopRecovery|ServiceStopBoundary|LateReplyReleasesOnlyItsOriginalUncertainty)` 在 `steerline-test-feedback/20261009T061433.356999900Z/summary.json` 通过（107.45s）；独立 reviewer 亦确认此有界 ack 增量 clean。Linux/amd64 `go build ./cmd/steerline` 通过，隔离临时产物 SHA256 `F727727D15020C3C43C2D93F3CA441E4ACF307FC05EC8F482D7FD148E43C4D78`。新完整受控与真实 Orca 验证仍在进行；未记实现完成或 A8 通过，PR 与实际合并核验待完成。原安装 101 及一期长期验收不在本次通过范围。

A8 补充失败：`steerline-test-feedback/20261009T061152.020964900Z/summary.json`（`df21916a`、265.46s、cleanup_complete=true）在正式 `control start_task` 的驱动期限内超时；最终快照仅 readiness start/submit accepted，尚未进入活动停服窗口。不据此宣称 A8 通过，也未将超时断言为已定位的环境或产品原因；待结束并行受控验证后串行复验当前完整生产候选。

## 最终生产增量与受控闭环

生产候选 `5bc3f7fa`（后续至 `932919e3` 只改文档/测试）：13 个定向测试 0 skip 通过，原消息 ack 取消仍有界完成并消费原回执，尚未知 ack 只 Reconcile 不重新 Execute，提交失败不 ack；独立复审 clean。A1–A5 的基线与增量证据共同覆盖冻结拒绝、worker 架构边界、精确 owned writers、原停止对账、未知 start/publication/answer、晚答核实重试、取消/准备 owner、同事实 poll 幂等及兄弟隔离。Orca 当前 exactWorker 负例为 `TestRecoveredDispatchRequiresExactOriginalIdentity`，其独立 Go 定向通过；标识与分类不替代实际身份或业务提交证明。

A6 完整命令 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerServiceStopRecoveryControlled$' -count=1 -timeout=20m`，`20261009T061423.147599900Z/summary.json`，`5bc3f7fa`，507.86s。local（88.74s）、completed live 原会话下一轮（133.96s）、原待问题/丢 ack 和 reply 回执（137.70s）PASS；Orca 活动重接→离线完成→再次重开全部行为断言通过，但 TempDir 删除 SQLite 文件时报告占用，子项 FAIL，不能把本次总结果登记为通过。没有改称清理成功或覆写原结果。

`a4d7e263096233e68bce25a8bf745c6c857bdb78` clean source，仅去掉该子项末尾重复打开数据库的独立 Query：使用已验证同一公开最终 snapshot 断言无新增 Attempt/Delivery并显式 stop；不删行为或身份/计数断言。`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerServiceStopRecoveryControlled$/^orca_continues_and_recovers_completion_after_terminal_exit$' -count=1 -timeout=6m`，`20261009T062319.664347200Z/summary.json` PASS，101.63s/0 skip，临时目录清理成功。A6 各子项由上述精确同生产版本结果共同验收通过；不宣称重新跑了另一遍完整 suite。至此受控 local 1→2 Attempts、Orca 原 Attempt/session/Dispatch、二次完成重启零额外 Dispatch/reply/send/ack、同 live session下一轮只新增1 Dispatch，原问题只回复1次，均有正式接口与计数断言。

最终 L1：`a4d7e263` clean，`20261009T062130.133828700Z/summary.json` PASS，preparation 0.3s/execution 96.3s。未跑 full/L2，未扩大原生 suite。

A8 驱动增量 `932919e3`：同一正式 control 请求的原产品准入期限仍为2m。驱动等待真正最终 applied 回执最多6m（父 helper390s，仍受场景 context 约束），让已经 claim_reserved 的原生准备可在准入期限后完成；未重新发送请求、未把 applying 当已执行/已交付，未延长产品准入。当前串行复验，结果待登记。

## 原生 worker 主干整合与新的真实故障定位

`f6fcd757` 整合最新默认主干 `ade7042b`，包含 #86 原生 Orca worker-start、宿主管理权限、显式完整输出材料与释放后 canonical evidence，以及控制台 PR links。继续型停服标识、退出 terminal 的原 inbox 接回、规范 session/live/writers 事实保留。`40fd0235` 只调整原生测试 profile：Orca 使用独立 codec 与 native model/effort，不再继承客户端 Codex sandbox 配置。

`20261009T063140.957081500Z/summary.json` 的完整 A6 四个受控分支均 PASS，261.95s；两个 adapter facts 子项因新 main 校验冻结 Agent 后测试构造缺 setup 而 FAIL，不算总通过。修正测试的原冻结 setup 后，adapter 全包（明确不选择 opt-in smoke）在 `41897854` 的 `20261009T064010.773284000Z/summary.json` PASS，5.829s；其一个独立 CLI fixture skip 不算真实验收。L1 `40fd0235` clean，`20261009T063411.524734700Z/summary.json` PASS，106.1s。随后只读原 startup 回执增量的 native mutation/丢回执服务回归 `TestNativeOrcaMutationRecoveryPreservesOriginalSession` 在 `20261009T064944.872519500Z/summary.json` PASS，126.25s。

独立 reviewer 确认新 main 的 reconcileStart 会再次调用 worker-start；原 UUID 幂等不能证明重开零输入。`41897854` 改为只读 request-show：原 UUID/runtime/method=orchestration.workerStart/state=completed/receipt 全部吻合才绑定旧 native 回执。absent/pending 保持 unknown，不发 worker-start；partial 的 Reconcile 也只观察，不 stop/release。新增未到平台的原意图负例；reviewer 已复核闭环。曾报告 completed-exited validator 必须 live 的问题已核对真实代码并撤回，不列为未修复问题。

真实 A8 新增失败仍完整保留：

- `20261009T062543.292280000Z` / `932919e3`：仍在旧 readiness 管线等待原 control 最终回执超时（429.99s，清理成功）；延长等待没有解决该旧路径，未算通过。
- `20261009T064018.421443700Z` / `41897854`：真实 native worker-start 在 agent_readiness 返回 `Agent startup blocked: agent-update-prompt`，140.63s/清理成功，未进入业务停服窗口。
- 原生观察只创建本任务新的 launcher fixture repo `1eb389a7-b18a-4e64-863f-b02228f9f676`。观察终端证实 Codex0.159.2提示0.162.0更新；普通Skip只消除当前启动。2026-10-09 用户明确批准 Skip until next version；已核实 `dismissed_version=0.162.0`，没有升级CLI或变更Agent Permissions。通过 computer-use/Orca CLI操作新建终端；两个自有worktree与terminal及注册setup已精确清理，临时Git根保留。用户中断期间Orca runtime发生换代，旧native local日志 `steerline-87-local-native-main-final-20261009.jsonl` 无terminal PASS/FAIL，明确不算通过。
- `20261009T070140.212275000Z` / `41897854`：不再被更新提示挡住；native result明确原输入accepted，但state=outcome_unknown/turn_start_unobserved。当前实现把未observed当失败并stop/release，137.74s/清理成功，不能算活动恢复通过。
- `dabae8f3` 对这条精确原UUID、accepted effect/原terminal及incarnation、原runtime/Run/Task/Dispatch/worktree/agent/model完整证据，分别承认 InputAccepted 而保持 TurnStarted=false；保留原diagnostic/state/observer，不生成ready/completed。transport/cancel/缺证据各负例拒绝；此stage即使身份校验失败也保留partial未知、不误停可能已接输入者。driver只完成自有原稳定paste的一次bare Enter，原JSON/exit保持，不重发prompt。独立复审认可边界；实际业务是否开始仍需原消息/started证据。
- `20261009T072852.169782400Z` / `dabae8f3`：模型真实提交并执行，在原hold等release；真实 heartbeat `msg_372c9c90eca0` 的全部原Task/Dispatch/terminal吻合，但未进入普通coordinator check流，中心仍pending，control最终超时；470.36s/清理成功。fixture改为模型向原Run发status实际进度，不注入状态。

`dabae8f3` owner最小回归 `20261009T073242.807876000Z/summary.json` PASS，68.55s。独立reviewer另确认真实started会把原accepted receipt覆盖为只有session/turn的摘要，导致native launch证据丢失；`9c984748` 在通用确认中保留原receipt与原PlatformRequestID，空时才生成摘要；原started/重复started/保存答案真实transition回归通过。E2E model/launch/业务证据与#86原生路径一致，真实usage先只匹配host-owned home下根session metadata的唯一自有worktree，再读匹配session正文，不读取其他session正文，不把缺usage当通过。

当时 PR #260 仍为草稿；完整真实 A8 在 `9c984748` 执行中，当时未登记通过或实现完成。无 installed101 修复、安装替换或长期组合签收结论。

## 最终自动验收闭环（2026-10-09）

最终生产候选 `399c448508371c4e08e85f82688929ccde71531d`，tree `b1d35b51c3b97a1a8d6babf9f77880b56d8c63eb`，Windows/amd64 clean source；含最新主干 `8001fc94`（#263 启动预算/失败回执与原生资源对账），保留本任务更严格的原UUID、effect、terminal/incarnation与全部绑定门槛。独立 reviewer `/root/review87` 对此集成及Started回执保留修复复核 clean，无未修复确认问题。以下自动验收结果在合并前登记，当时尚待 PR #260 真实合并核验；最终结算另记于末尾。

| 范围 / 精确验证 | 结果及证据 |
| --- | --- |
| WSR-A1–A5 / owner最终增量 | `20261009T080938.368260600Z/summary.json` PASS，14 tests/0 skip，133.73s；包括原stop对账、树退出不能消除未知输入/发布、同poll幂等、晚答核实重试、停机准备owner、有界原ack、Started/重复Started保留原回执和原请求ID。此前完整owner/兄弟隔离结果按原范围共同覆盖 |
| WSR-A2/A3/A5 / native adapter及证据driver | `20261009T080706.940823000Z/summary.json` PASS，7.964s；来源 `241288c0` 加两处测试修改（随后完整提交于399），159 tests/4 skip。原消息/接回、请求只读、unobserved接受正负例、完整原model/launch与业务结算、UI原稳定输入提交和模型计数通过。四个skip为两个CLI subprocess helper、TestOrcaNativeHostWait、TestSandboxReadinessExistingAccounts，不作为产品原生通过；显式不选择opt-in adapter smoke，A8由后续真实完整场景证明 |
| WSR-A6 / 双受控产品分支 | `f6fcd757` 的 `20261009T063140.957081500Z` 全部四个产品子项 PASS（总报告包含旧adapter fixture失败）；明确同主干native worker路径、local fresh Continue同session新Attempt、Orca active/offline/第二次完成重开及live next round、原问题/丢reply/ack。在 `41897854` 只读原startup结果增量后的 native mutation/原session回归 `20261009T064944.872519500Z` PASS，126.25s。之后更严格unobserved接受与原回执保留由定向正负例和真机A7/A8覆盖，不宣称再次跑了完整A6 |
| WSR-A7 / 真 local正常停服 | `399c4485` 的 `steerline-87-local-delivery-final-20261009.jsonl` PASS，220.83s subtest/0 skip；旧PID33620退出，正式continue后新PID50688；同thread `01a11fc2-d417-7a83-baaa-d7036cdb71c2`，原Attempt `ce5a159b4ff257a4dff00215738e9aad` → 新 `c775918cae20bf705caf9efd32c5cfd8`，原turn `01a11fc2-dec5-7f41-89fe-c0e6d7e4ac38` → 新真实完成turn `01a11fc4-b368-7b60-bdef-54ed731491d3`；同工作区，重开不自动新输入，正式continue只换代一次，测试清理完成。host-exit原分支的原已测基线保留，不把被用户中断的复验算通过 |
| WSR-A8 / 真 Orca活动停服与离线完成 | `399c4485` 的 `20261009T080915.402600000Z/summary.json` PASS，1 test/0 skip，447.36s；`scenario.json` passed=true/usage_verified=true/cleanup_complete=true/cleanup_errors=null。必须真实started、原运行、原产物与原Dispatch结算，不注入假worker_done |
| 必要回归 / L1 | `399c4485` clean，`20261009T080926.046879900Z/summary.json` PASS，preparation0.3s/execution141.7s |
| Linux/amd64编译 | `399c4485` 的 `go build ./cmd/steerline` PASS；隔离临时产物 `steerline-87-linux-delivery-final`，SHA256 `4B677D95C5F638B86D595B2779CED51C98DD98E907F5D480C5E8A601F1F4A915` |

A8 精确原身份：execution `1b4864482da1d46d8e98761d95da91bc`、Attempt `39cce8602d529eddeab602d87768bf68`、native command/turn `6777a96aca9615a46386e61874bc3ce1`；runtime `e11d4024-70e8-46c6-b86a-e17338831ba9`、terminal `term_f795fe50-9da2-4b70-8d40-0208e19d9e2b`、incarnation `c24fc9f1-2bbc-4124-8628-6c4a0c8ecc57`。active-before-stop → active-after-restart → completed-after-offline-window → completed各快照均保持同Attempt/session/command、TurnNumber=1；离线发布/接回后仍零Delivery，实际fixture PR合并核验后只有一个Delivery，后继ready且未启动。

同一原生模型根session `01a11fb8-4917-7993-b80e-1573d7a14ec0`、原物理turn `01a11fb8-f2d0-7210-9e2f-b6384d6ef7a9`，actual model=gpt-6-luna/effort=medium。完成后baseline、again-restart、final usage均 Input387843/Cached344576/Output2862/Reasoning1155/Total390705，严格相等；活动/offline期间允许原轮usage增长，零重派依据原身份/原命令与TurnNumber，未使用“不增长”错误替代活动窗口证明。采集文件只保存该自有根session的最小usage/lifecycle记录。

此前 `20261009T075238.678968200Z`（9c984748，613.46s）已走通活动/离线完成/原launch全部行为，但usage元数据发现遇到他人的空/写入中header而失败，cleanup_complete=true。`399c4485` 只在不能证明自有归属时跳过该header，绝不读取其正文；匹配自有session后仍须完整合法JSON、唯一root、完整原turn、正tokens与实际model/effort，不能用原失败局部成功代替新的PASS。主干新增soft-gap fixture缺mutation/inc/effect的失败 `20261009T080225.905483300Z` 亦保留；已按真实native样本补fixture，生产门槛不降。

WSR-A1–A8自动项通过，无新增人工项；未运行Full或扩大为完整L2/L3。历史101未知停止/跨构建失败未修改，安装版及现有用户Orca终端未处理。#84后续新分步流程、一期单安装/24小时组合签收仍是独立未验范围。本任务只包含用户明确批准的0.162.0更新提示偏好，未升级Codex/Orca、未变更Agent Permissions。PR #260 合并与仓库状态汇总另记真实回执。

最终证据复审：reviewer对final summary/scenario、原五快照、三份native usage和local日志逐项只读核对；确认生产399 clean、A8 1 test/0 skip及完整清理、A7 220.83s、owner14 tests/0 skip、L1精确141.673s。四包定向的241加测试dirty与四个skip已明确，历史“当时在跑”不作为当前未验状态。没有新增产品问题，所有自动项按上述真实证据闭合。

## 2026-10-09 真实合并与交付结算

GitHub 确认 [PR #260](https://github.com/erranliu/SteerLine/pull/260) 为 MERGED，mergedAt=`2026-10-09T08:39:52Z`（北京时间16:39:52），审核 head `187dd098b9c2c2b11912770fdc70fdfce6f06255`，merge commit `d951ca8729d5854b8429b7790fef8ff75feaa750`。已 fetch 默认主干并以 `git merge-base --is-ancestor` 核验该 head 已进入主干。最终证据精度修订 `187dd098` 的独立复审 clean；从真实验收候选 `399c4485` 到审核 head 仅修改结果文档，没有生产/测试代码变化。

#87 实现完成；WSR-A1–A8 自动项通过，无任务人工项。执行清单、Spec/验收导航已同步，已完成草稿任务条目移除但永久身份与能力摘要保留。本次合并后结算仅修改文档并执行文档链接、任务身份和 diff 静态检查，不宣称重新运行产品验收。历史失败、skip、各自测试基线及未重跑范围均保留；安装101、安装替换、#84新流程及一期长期签收不因此变为通过。
