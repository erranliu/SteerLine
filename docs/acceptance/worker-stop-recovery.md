# Worker 停机对账与恢复一致性修复验收

唯一归属：#87 Worker 停机对账与恢复一致性修复。[Spec](../spec/worker-stop-recovery.spec.md) · [Plan](../spec/worker-stop-recovery.plan.md) · [草稿](../roadmap/drafts/worker-stop-recovery.md)。当前仅规划、待验收；没有执行授权或产品通过结果。

## 现场诊断基线（不作为修复验收）

2026-10-09 只读检查 `C:\SteerlineRuntime\state\SteerlineTest-b55e8cc42ae8\steerline.db` 及原 runs/service 日志。受影响产品 Task 为 101；它不是仓库修复 Task #87。

- run `1f20a4d6ff67c8511f3d682fe3f668b3`；execution `4c0789792168d8ea6237e94d3d47f847`；Attempt `63f8bca87ad42980a05468f3770d96da`；原 thread `01a11ecc-e1b6-7be1-b607-ccdfd602fa35`；原 turn `01a11ecc-f0bd-7a01-b139-3f05c5906b4c`。
- 北京时间 11:56:15，stop Operation `cc0deb06bbe14e5ecc3fbd540c5fcb9d`，kind `stop_worker_observation`，原 endpoint `ws://127.0.0.1:62956` 拒绝连接，状态 `outcome_unknown`。
- 11:58:39，Decision `5287b70aaea599f6256639177979fb9f` 的 continue 答案保存为 `answer_saved`；随后 Error 为 `An earlier external operation has an unknown outcome; no recovery input was sent`。只有一个 Attempt，原进程 PID 39520 在检查时不存在；原日志有 turn/started，没有 turn/completed。
- 新服务日志声明构建来源 `d8d66d6c5696f41d7df1ed3ff50d4b3ae77b9da3`；原停服服务来源 `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5`。这次跨构建现场只用于定位，不能作为同构建恢复验收。
- 源码确认停止记录未进入 lifecycleSnapshot 映射/过滤与恢复 stop 分支，hasUnknownWorkerOperation 则会阻塞它；正常 shutdown 的 interrupt 在 detach 后 defer 执行。后者是需复现验证的顺序风险，未凭现场日志断言 endpoint 退出的唯一原因。

没有修改数据库、发送恢复输入或重启安装服务，没有复制原始数据库/认证/完整日志入仓库。上述历史未知回执和失败不得改称成功。

## 自动验收（worker 生命周期边界）

2026-10-09 按用户明确要求补充 worker 停服行为标识和应用恢复边界：local 停止、Orca 继续必须分别证明。WSR-A1–A7 沿用并细化，新增 A8 的 Orca 真实活动停服/离线完成验收；没有既有通过结果，当前全部仅规划、待验收。

| ID / 方法 | 行为与最小通过条件 | 入口 / 环境 | 状态 |
| --- | --- | --- | --- |
| WSR-A1 / 自动 | 两种 ServiceStopBehavior 由 worker 声明、随执行冻结；缺值/非法/能力变化及旧格式在副作用前拒绝。无 live observer 时仍可构造原 worker 入口；应用只消费规范事实，不解析实现身份或按 backend/protocol 恢复 | Plan owner/boundary 表驱动与冻结/版本负例 | 待验收 |
| WSR-A2 / 自动 | local 先门槛后精确 owned 树停止，零 interrupt/新 stop observation；Orca 只 detach，零平台关闭/停止，原 worker 能继续。准备晚到不漏资源，一个错误不漏兄弟，关闭 observer 不被当 worker 已停 | Plan owner +双 worker adapter 事实测试 | 待验收 |
| WSR-A3 / 自动 | 预期标识不覆盖实际事实；旧写者未排除/身份变化保持 unknown；支持格式原 stop 仅凭精确事实收尾，保留 RPC 未知，不伪造 completed；未知 start/input/reply/publication 不因进程退出消失 | Plan owner/adapter、既有 unknown publication 回归 | 待验收 |
| WSR-A4 / 自动 | 单次完整分类，展示/受理/投递资格一致；fresh answer 只消费一次，接管/实质改变保留旧答案关联新确认；未知发送只对账；相同 poll 不换状态/Decision/版本/活动，重复命令不多 Attempt | Plan owner +正式 Command 受控集成 | 待验收 |
| WSR-A5 / 自动 | 继续型 worker 的原运行/待答/停服期间 completed 均可接回；原结果/ack/业务结算不重复、不补发 worker_done，不把结果当 TaskDelivery；晚到不改新 Attempt/兄弟，unknown 不饿死无关执行；Query 反映中心离线与实际 worker 状态 | Plan owner/adapter 与双路径 Query 集成 | 待验收 |
| WSR-A6 / 自动 | 同构建受控双 worker：local 停止后重开不发送，正式继续同会话新 Attempt；Orca 原运行/待答/离线完成接回原 Attempt/session/Dispatch；重复命令/再次重开不重派、误停止或重复结算，清理完整 | 拟新增 TestWorkerServiceStopRecoveryControlled，复用 integration 的双 helper 和正式接口 | 待验收 |
| WSR-A7 / 自动 | Windows 真 local 正常停服实际结束旧树，重开不新 turn；正式 continue 复用原 thread/workspace、新进程/Attempt/turn 真实完成；host-exit 原分支和清理保持 | 现有 TestLocalCodexManualRecoveryLive 扩展正常停服，命令见 Plan | 待验收 |
| WSR-A8 / 自动 | 本机 Orca 真活动停服不停止原 worker，重开接回原 Attempt/terminal/Dispatch；再次停服让原轮次真实完成，重开接回输出及结算一次，零新增业务输入/替代执行，所有原生证据及清理通过 | 现有 L3 live recovery.orca-codex 扩展活动/离线完成，命令见 Plan | 待验收 |

准备：用户确认纳入/固定文档；实现冻结标识及版本、worker 统一入口、各拟新增测试和现有测试分支；按 Plan 补 controlled 离线行为和 Orca 最小等待窗口。A7/A8 需要本机目标工具/认证/当前权限，独立 scratch/owned 资源，禁止操作安装 101 或现有 Orca 终端。缺场景、skip、零匹配或缺证据不算通过，自动失败阻塞对应交付。

A8 的原轮次离线继续允许 usage 增长；通过条件是身份/原轮次保持及没有新增业务发送，不能用旧 waiting-merge 场景的 usage 不增长断言替代。可信完成/原消息确认及业务交付分别核对。标识是新增冻结字段，旧构建格式拒绝；历史 101 不改称已恢复。

无新增人工活动或环境框架，不新增 L3 suite；现有 native/driver 最小扩展须实际可用。真实场景使用隔离任务和 loopback provider，不操作安装数据；原一期长期签收及安装替换另行安排。

## 结果登记

执行后逐项记录 actual commit/dirty/tree、exe/CLI/构建摘要、冻结 worker 标识及版本、精确命令/子场景匹配、日期/结果/清理、原/新通用身份、原生证据、start/send/stop/settlement 计数和临时产物目录。未知回执与后续核实分别留证，失败不覆盖，原始日志不入仓库。

纳入执行后同步 execution-map 的结果/待验链接，PR 合并与产品验收分开。当前未纳入、未实现、未跑产品测试，历史证据保持原基线。
