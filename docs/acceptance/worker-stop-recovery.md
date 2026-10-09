# Worker 停机对账与恢复一致性修复验收

唯一归属：#87 Worker 停机对账与恢复一致性修复。[Spec](../spec/worker-stop-recovery.spec.md) · [Plan](../spec/worker-stop-recovery.plan.md) · [草稿](../roadmap/drafts/worker-stop-recovery.md)。用户于 2026-10-09 授权本聊天直接执行，不经 roadmap-runner；启动与实际结果见下文，仍未修改安装 101。

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
| WSR-A1 / 自动 | 两种 ServiceStopBehavior 由 worker 声明、随执行冻结；缺值/非法/能力变化及旧格式在副作用前拒绝。无 live observer 时仍可构造原 worker 入口；应用只消费规范事实，不解析实现身份或按 backend/protocol 恢复 | Plan owner/boundary 表驱动与冻结/版本负例 | 待验收 |
| WSR-A2 / 自动 | local 先门槛后精确 owned 树停止，零 interrupt/新 stop observation；Orca 只 detach，零平台关闭/停止，原 worker 能继续。准备晚到不漏资源，一个错误不漏兄弟，关闭 observer 不被当 worker 已停 | Plan owner +双 worker adapter 事实测试 | 待验收 |
| WSR-A3 / 自动 | 预期标识不覆盖实际事实；旧写者未排除/身份变化保持 unknown；支持格式原 stop 仅凭精确事实收尾，保留 RPC 未知，不伪造 completed；未知 start/input/reply/publication 不因进程退出消失 | Plan owner/adapter、既有 unknown publication 回归 | 待验收 |
| WSR-A4 / 自动 | 单次完整分类，展示/受理/投递资格一致；fresh answer 只消费一次，接管/实质改变保留旧答案关联新确认；未知发送只对账；相同 poll 不换状态/Decision/版本/活动，重复命令不多 Attempt | Plan owner +正式 Command 受控集成 | 待验收 |
| WSR-A5 / 自动 | 继续型 worker 的原运行/待答/停服期间 completed 均可接回；原结果/ack/业务结算不重复、不补发 worker_done，不把结果当 TaskDelivery；晚到不改新 Attempt/兄弟，unknown 不饿死无关执行；Query 反映中心离线与实际 worker 状态 | Plan owner/adapter 与双路径 Query 集成 | 待验收 |
| WSR-A6 / 自动 | 同构建受控双 worker：local 停止后重开不发送，正式继续同会话新 Attempt；Orca 原运行/待答/离线完成接回原 Attempt/session/Dispatch；重复命令/再次重开不重派、误停止或重复结算，清理完整 | TestWorkerServiceStopRecoveryControlled，复用 integration 的双 helper 和正式接口 | 待验收 |
| WSR-A7 / 自动 | Windows 真 local 正常停服实际结束旧树，重开不新 turn；正式 continue 复用原 thread/workspace、新进程/Attempt/turn 真实完成；host-exit 原分支和清理保持 | 现有 TestLocalCodexManualRecoveryLive 扩展正常停服，命令见 Plan | 待验收 |
| WSR-A8 / 自动 | 本机 Orca 真活动停服不停止原 worker，重开接回原 Attempt/terminal/Dispatch；再次停服让原轮次真实完成，重开接回输出及结算一次，零新增业务输入/替代执行，所有原生证据及清理通过 | 现有 L3 live recovery.orca-codex 扩展活动/离线完成，命令见 Plan | 待验收 |

准备：用户确认纳入/固定文档；实现冻结标识及版本、worker 统一入口、各拟新增测试和现有测试分支；按 Plan 补 controlled 离线行为和 Orca 最小等待窗口。A7/A8 需要本机目标工具/认证/当前权限，独立 scratch/owned 资源，禁止操作安装 101 或现有 Orca 终端。缺场景、skip、零匹配或缺证据不算通过，自动失败阻塞对应交付。

A8 的原轮次离线继续允许 usage 增长；通过条件是身份/原轮次保持及没有新增业务发送，不能用旧 waiting-merge 场景的 usage 不增长断言替代。可信完成/原消息确认及业务交付分别核对。标识是新增冻结字段，旧构建格式拒绝；历史 101 不改称已恢复。

无新增人工活动或环境框架，不新增 L3 suite；现有 native/driver 最小扩展须实际可用。真实场景使用隔离任务和 loopback provider，不操作安装数据；原一期长期签收及安装替换另行安排。

## 结果登记

执行后逐项记录 actual commit/dirty/tree、exe/CLI/构建摘要、冻结 worker 标识及版本、精确命令/子场景匹配、日期/结果/清理、原/新通用身份、原生证据、start/send/stop/settlement 计数和临时产物目录。未知回执与后续核实分别留证，失败不覆盖，原始日志不入仓库。

纳入执行后同步 execution-map 的结果/待验链接，PR 合并与产品验收分开。本任务已纳入并正在执行；历史 101 证据保持原基线。

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
