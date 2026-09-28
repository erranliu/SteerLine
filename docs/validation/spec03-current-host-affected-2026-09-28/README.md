# 台式机64g：受影响故障路径的定向回执

[机器回执](receipt.json)比较原中心在 2026-09-28 受控停服前保存的 SQLite 副本和恢复监督后的原 SQLite，并对同一 Orca Run/Dispatch 作只读核验。本机中心为 Orca 1.4.209，执行主机 `台式机64g` 为 1.4.215。原中心服务最终以源码 merge commit `02b126d261ad33653b81a30eeb7b8349c01a49a8` 的版本化二进制恢复 `running/ready`。

| Spec 03 路径 | 当前目标观察 | 结论边界 |
| --- | --- | --- |
| R07 停服后重接原执行 | #14/#15/#30 在停服前与服务恢复后，Run ID、Execution ID、原 Dispatch/进程身份及各自两条 Attempt 身份和数量逐项相同；Orca 对三个固定 Dispatch 均返回原目标、`exactWorker=true`。中心没有新建替代 Attempt/Dispatch，原项目占槽仍为 true | 证明本次当前主机活跃 worker 重接与不重派。#15 的 Orca Dispatch 此后显示 completed，但该采样时中心仍为原 Attempt running；不把它计为“中心离线期间完成并成功推进”的新样本。各步骤完成窗口此前的 [home 真机 R07 回执](../spec03-fault-matrix-2026-09-28.md)按未受变更影响的版本族范围复用。 |
| R08 身份变化与 fail-closed | 原 #4 verify 的 `ctx_06a3ae2ebfff` 在 Orca 只读观察中为 `identity_changed`、`exactWorker=false`，中心保留原 unknown Attempt/占槽，没有重发。Dispatch 精确 abandoned 且无残留资源、原远端 terminal 同 runtime/句柄/incarnation 以 `operator_close` 退出后，才依[原 Task 结项](../phase1-task-closure-2026-09-28/README.md)记 stopped | 这是当前目标实际身份不可核实和人工结算现场，不声称远端 Orca 自发生成了同句柄新 incarnation，也不声称 unknown 自动应用。home 的真实 runtime 更替和旧身份拒绝证据仍按[原矩阵](../spec03-fault-matrix-2026-09-28.md)复用。 |
| R14 远程取消不可用 | 中心受控停服后，三个远端 Dispatch 仍可按原身份观察；服务恢复只重接，不按句柄发送 stop。另一个中断会话终端在明确核实无继续执行后按精确句柄人工关闭，没有归入任何原 Dispatch 的自动取消 | 本次是受控 `stop-service`，不是再次关闭可见窗口；窗口关闭与 viewer 退出的旧目标回执仍保留原范围。当前生产目标的 remote cancel 继续标为 unsupported。 |

[SteerlineTest Task 105](../spec03-viewer-task105-2026-09-28/README.md)已经通过当前主机 4 秒的步骤转换可见门槛；[此前原生问答和固定 head 合并](../spec03-desktop64g-followup-2026-09-28/README.md)及 [P13 两项负向入口故障](../spec01-p13-steerlinetest-2026-09-28/README.md)也在 SteerlineTest 验收。这里复用自然发生于原 SteerLine 中心的身份/停服现场，不重复这些已通过试次。#6 仍须原中心 Task Delivery；长期无人值守和固定 stable 24 小时属于 #32。
