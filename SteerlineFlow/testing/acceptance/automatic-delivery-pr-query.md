# 自动交付平台查询与项目 actor 解耦验收

对应[Spec](../../spec/automatic-delivery-pr-query.spec.md)及[Plan](../../plans/automatic-delivery-pr-query.plan.md)。

## 审计依据

- `TickPR` 在 scheduler 串行调用中进入 `Deliver`，平台读取发生在 `s.mu` 内。普通观察和派发要等同一调用返回。现有 stop 通知可取消当前 `RunWork`，异步结果入场仍需按原通知顺序处理，不能靠两个 channel 同时就绪时的选择顺序。[调度](../../../internal/execution/scheduler/loop.go)、[候选 actor](../../../internal/taskrunner/actor.go)、[交付入口](../../../internal/taskrunner/service.go)
- 普通同快照观察会随变化后的 Run 投影或事件提交推进候选 revision。原交付在读取证据和入事务时都要求候选 revision 相等。查询移出 actor 后，连续观察可能使每次平台结果失效。自动交付因此需要按原绑定及业务资格重核，图和任务版本及原提交事务保持。[候选观察](../../../internal/runnerbridge/bridge.go)、[证据验证](../../../internal/storage/ports/runner_pr.go)、[SQLite 交付](../../../internal/storage/sqlite/runner_result.go)
- 原用户 stop 是候选参与退出，是否另有 Runner 原生停止义务由原流程决定。后台查询结果既不取得控制授权，也不改变原回执。[候选撤回事务](../../../internal/storage/sqlite/runner_stop.go)、[生命周期](../../../docs/product-design/worker-lifecycle.md)

## 验收安排

以下均为实施后的自动验收。SteerLine 以 `e010373e` 加本工作树源码执行，输出根为 `C:\SteerlineDebug\automatic-delivery-pr-query-20261105`。Runner 与平台均使用隔离的公开协议夹具。

| ID | 环境与触发 | 通过条件及阻塞范围 | 状态与证据 |
| --- | --- | --- | --- |
| ADP-A01 | isolated，事务变更后 | 原绑定稳定时持续观察下完成交付。变更绑定、任务、图或停止状态时原子拒绝，幂等重放。失败阻塞异步切换。 | 已验收，`owner-final`、`storage-final` |
| ADP-A02 | isolated，调度变更后 | 慢查询期间观察、派发和 stop 完成，覆盖就绪竞争、查询数量、候选顺序和项目关停。失败阻塞调度切换。 | 已验收，`owner-final`、`storage-final` |
| ADP-A03 | controlled，交付前 | L1、L2、L3 重启通过，原任务/候选/PR、唯一交付、后继和清理均有证据。失败阻塞交付。 | 已验收，`L1`、`L2`、`L3` |
| ADP-A04 | isolated，交付前 | command/Gitea 取证和超时保持原行为，异常无误提交，废弃原规则通过。失败阻塞交付。 | 已验收，`provider-and-binding`、`storage-final` |

## 2026-11-05 实施验证

- `owner-final/summary.json` 共 38 项通过。`TestAutomaticDeliveryTransactionSurvivesObservation` 对比同步路径的严格 revision 拒绝与自动路径在原绑定稳定时的成功提交，并验证过期、任务/图版本冲突及会话变更拒绝。`storage-final/summary.json` 为固定 L4 `go.storage` 结果。
- `TestAutomaticDeliveryActorKeepsStopPriority` 用 channel 屏障让查询挂起，期间连续推进原候选 Run revision，并确认派发 tick 能到达环境准入。覆盖 stop 先应用、结果信号先被接收、交付先提交、首个 PR 未合入、平台超时及项目关停。复跑既有停止、同步交付、废弃和 provider 用例。首轮 `owner` 与 `storage` 的新增测试未编译，随后补齐测试代码与 stop 控制夹具并复跑。
- `provider-and-binding/summary.json` 共 15 项通过。包含 command 子进程和本机 Gitea HTTP 夹具的冻结配置与认证引用、自动交付的绑定变化反例和原废弃事务反例。
- 固定 L1 `l1.minimum`、L2 `smoke.delivery`、L3 `recovery.waiting-merge-restart` 均通过。L2 与 L3 的 `cleanup_complete=true`。命令、源码和原始证据随各自 `summary.json` 保存。
- 真实平台、真实模型和实际安装环境未验。
