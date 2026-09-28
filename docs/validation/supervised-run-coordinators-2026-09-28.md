# 原生远端并行 Run 的协调终端与受控重开（2026-09-28）

`C:\SteerlineRuntime` 中 #30、#26、#14、#15 曾共用项目配置的一个 Orca 协调终端。Orca 每次 `run-create` 将该终端改绑到新 Run；之前三个 Run 的 `status` / `worker_done` 虽已送达原 Run 收件箱，中心以共享终端读取时收到 `consumer_fenced`。#15 的 verify worker 当时仍活跃，不能切换原终端去抢读其他 Run。

用户决定放弃这四次执行，由自己以后重新启动。正式服务通过原 `stop-service` 入口停止；它没有发送不支持的远程取消。操作者以原 Dispatch 精确停止 #15（Orca 回执 `stopped/closed_agent_terminal/ptyKilled=true`，后续 `worker-show` 为 `stopped/process_stopped/exited`），并对其余三个已经 `worker_done/succeeded/completed` 的 Dispatch 执行精确 `worker-release`，逐个取得 `released`、精确归档输出和 `exited`。四个旧工作树保持原样，供用户人工清理。

保留的远端工作树分别为 `steerline-a189b2ee950b`（#30）、`steerline-82e5d3178ae7`（#26）、`steerline-752d3210250b`（#14）和 `steerline-bb8ffcc3bd0c`（#15），均属于台式机64g 的 Orca SteerLine 仓库。它们不是本机配置的项目根目录；对应原 worker 终端已退出，不需要再按终端标题猜测清理对象。

新的中心 `task reopen` 远端分支在当前 SQLite 上按每个旧 Attempt 的原 binding 读取 Orca `worker-show`，只接受精确匹配且 terminal 不再连接的 `released/exited` 或 `process_stopped/exited`。它还拒绝已发布步骤、已交付任务、未决 Decision 和任务/投影版本竞争。一个事务将本次 Attempt 记 `stopped`、Execution 记 `stopped`、WorkflowRun 记 `abandoned`，Task CAS 回到 `open`，保留原证据与工作树。并行的其他任务不会因为其中一个未知运行而被重写。相同 request ID 回读原回执，不二次修改。

先在正式状态的隔离副本上运行四次重开并回读幂等结果，然后在原中心提交四个相同身份的请求。正式回执：#30 `4d4d3b8ff67f4d469a3676c224fc7412`，#26 `b1b6cf24e3784949939ddd9ac8dd6e10`，#14 `12b8a8cfb8c24ec7abb8ec2d329e8e23`，#15 `c4d36d9028494f4ca73d4f8fa758f0bb`。四项回读均为 `open`、`dependency_ready=true`、无启动阻塞；旧 WorkflowRun 均为 `abandoned`。原状态和 SQLite 备份位于 `C:\SteerlineRuntime\backup\reopen-four-20260928-1530`。

后续中心又为 #26 追加了 #34 硬前置，#34 依赖 #33。最终安装后的回读中，四项仍为 `open`；#30/#14/#15 可启动，#26 因 #34 尚未交付而 `dependency_ready=false`。本次修复未删除或绕过新边，用户后续重新运行时须遵守当前 DAG。

后续新远端 Run 在本机 Orca 为各自 WorkflowRun 创建独立协调终端，并将句柄固定到 ExecutionTarget。调用收件、确认和回复前查询 Orca Run 的当前绑定，绑定不符则拒绝，不从另一个 Run 的终端读取。终端创建或 Run-create 回执不明时，保留原 Operation/身份且禁止自动重派。现有格式中的旧 Run 仅在 Orca 回报其仍绑定原配置终端时可继续读取，不接管已换绑的外部执行。

本次只修复未来运行的协调终端与四项状态，不把这次已放弃的工作记为交付，也没有重新启动四项任务。完整远端验收仍按 #6 保留。
