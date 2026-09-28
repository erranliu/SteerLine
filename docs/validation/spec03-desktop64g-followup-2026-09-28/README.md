# 台式机64g：后续原生闭环与可见 viewer

2026-09-28，本机 Orca 1.4.209 / 台式机64g Orca 1.4.215、远端 Codex CLI 0.156.1。此记录补充[首次新主机验收](../spec03-desktop64g-2026-09-28.md)，保留不同试次各自结果；后续负向协议试次改用 SteerlineTest 项目。

## 隔离试次

1. 第一次复用 PR #85 已合并的 canary base `9425f55452826cdc9d49eb1c632b04cb9d5daa27`。原 Task 91024 在 implement 提问并收到 `yes` 后，worker 发现要求新增的同一文件和 Git blob 已在固定 base，拒绝制造空提交，发送 escalation/失败结果；中心 blocked，无新 PR。原 Run `3fb06f0a310aed6b0ed9619bb9eee55d` 与失败 Dispatch 保留，已结算的远端终端按精确 Dispatch release。
2. 从原不含目标文件的提交 `995bdadebae849fd45d1a929cd6984b201dcea61` 建隔离基线 `codex/spec03-desktop64g-repeat-base-20260928`。第二个 Task `03caa0654a40f202de7e17ac6307e73d` 的 Decision `relay_80c1172663be` 由中心保存 `yes`；远端已创建精确单文件候选，但实现 worker 在发送前手抄两份身份摘要时失败，未发绑定应用证明/步骤结果。原 Decision 结算为 failed，原 Dispatch released/exited，无 PR。这暴露了冗余手写身份断言的可靠性问题。
3. 修正提示后，以同一隔离基线和新 SQLite 完成原生 Task 91024。WorkflowRun `3cd46319e60437e61789c3dddcee1020` 的原业务 Decision `relay_778a93228b77` 在可见 TTY 中显示完整问题，`a` 打开草稿，VT F2 只提交一次 `yes`；中心先记录 `answer_saved`，随后在原 Attempt 的独立回执后记录 `applied`。prepare、implement、verify、review、publish_pr 均成功；固定候选 `f29b329585413836954b6b93c1244d7d991c50ee` 的 [PR #89](https://github.com/erranliu/SteerLine/pull/89) 只向隔离 canary 基线合并，merge commit `7477158670589217f15df7cafa86bebae7172325`。原中心 Task 为 `closed`，五个原 Dispatch 均 `succeeded/completed` 且 `released/exited`；[中心原回执](closure-evidence.json)。这轮无远端人工补按 Enter；人工 `yes` 是预定业务决策。

## 可见画面时延

用户将当前台式机目标门槛改为 **4 秒内**；home 的既有 2 秒门槛不变。[原 Decision 的可见终端测量](viewer-decision-measurement.json)从中心 `created_at=2026-09-28T06:02:18.535871Z` 到首次读到同一 `decision_id` 的 Orca `source=screen` 画面，保守上界 **2.215 秒**，因此该新主机业务消息样本通过 4 秒门槛。测量包含 SQLite 观察、Orca CLI 与画面读取；不把模型内部渲染时间当作可见时间。一次后续 verify Attempt 的计时器在事件已创建 35 秒后才启动，只证明精确 Attempt 已出现在画面，不作为 4 秒步骤转换证据。

本轮通过新的完整闭环和上述 Decision 时延样本；新主机受影响故障路径及步骤转换的可见时延仍属 #6，长期无人值守矩阵属 #32。较早失败试次没有被改写为成功，也没有触及 `main`。
