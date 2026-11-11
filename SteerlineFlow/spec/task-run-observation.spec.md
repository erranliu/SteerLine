# 任务候选 Run 同快照观察 Spec

SteerLine 使用 Runner 的 `observe` 同步已绑定任务候选的 Run 投影和事件。Runner 提供本次持久快照，SteerLine 核对它是否属于原候选并在同一事务提交投影、事件和游标。这个改动只涉及 SteerLine 的任务候选观察链路。

## 观察边界

- 已绑定且无待处理输入或停止义务的候选，每轮直接从已保存游标调用一次 `observe --run-id`，按现有页大小消费事件。有待处理控制的候选先完成本轮控制对账，再用一次 `observe` 读取最新持久投影与事件页。
- 任务候选正常运行要求 Runner 发布 `steerrunner-run-observation/v1`，声明 `same_snapshot: true` 且支持至少 100 条事件每页。在已固定并验证实例身份的在线 catalog 上检查此要求。能力缺失或不匹配时，带明确诊断阻断正常项目运行，原服务停机和资源结算仍走既有路径。能力检查位于启动前对账及 `pending_stops` 交接之后，项目任务运行时启动之前。
- 删除 SteerLine 候选同步的 `inspect → events → inspect` 后备路径。`observe` 失败或数据无效时保留原投影、事件和游标，下一轮从已持久化游标继续。原输入摘要、候选/Run 绑定、分支、已知 PR、revision 单调性、事件归属与事件 revision 上界继续校验。提交冲突时整页失效。分页游标仅越过本次消费的事件，空页后的新增事件仍可读到。

## 原请求、启动和停止

| 路径 | 保留的取证与动作 |
| --- | --- |
| 原 `request_id` 对账 | 继续用 `inspect --request-id` 获取原 submit、reply、resume 或 stop 的回执，核对原命令、Run 绑定及适用的命令 ID。保留既有首发、同 ID 重试及只读对账分支，晚到受理绑定原 Run。结果未知或已经撤回的请求继续按原规则处理，事件与 Run 快照不能证明某个原请求已受理或从未受理。运行时 `VerifyBinding` 保留原只读绑定检查。 |
| 启动前离线读取 | 保留 `InspectBeforeStart`。服务 ready 时核对原服务绑定后在线 inspect，`not_running` 时由 Runner 持原 writer lease 执行 `inspect --offline`。仅合格的离线 `request_not_found` 可以证明原提交未受理。身份、lease 或启动归属未知时维持原阻断。已有停止义务继续先持久化原信封，再经 `pending_stops` 在 Runner worker 恢复前受理。 |
| stop 前后 revision | stop 前继续用 `inspect --run-id` 获取新读取的 revision，首次构造信封时将其固定为 `expected_revision`。未知结果先按原请求查询并按既有规则处理原信封。只有明确拒绝后才允许按现有策略用重新读取的 revision 构造后续信封。控制成功后的观察单独发起，保存 stop 后当时可读的新 revision 和同快照事件。受理回执继续先独立持久化，快照读取失败不撤销回执、不推进游标，也不把受理视为停止完成或资源已结算。 |

适用的启动、恢复和结算规则见[Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)。测试生成、roadmap、环境准备、原生会话和部署预检继续使用各自的查询。Runner 公共命令、发布、恢复、文件通道和 SteerLine 平台交付查询沿用现有行为。

## 通过条件

1. 常规候选观察只发一次 `observe`。能力缺失、快照失败、绑定不一致或提交竞争均不会改写本次快照对应的候选事实。多页、空页后新增事件和重启后继续读取均无漏读。
2. 原提交、输入、stop 的丢失回执仍能按原 ID 对账。晚到受理、在线缺失、撤回后的只读分支及结果未知时的原信封处理均有回归。
3. 启动前在线与离线读各走原入口，只有离线缺失可作缺失证明。启动前已知停止义务仍在 worker 前交接。在线能力不满足时项目诊断清楚，服务停机和结算仍有原身份和回执。
4. stop 后读到更新的 revision 和对应事件。覆盖 stop 前后的并发 revision 变化、明确 `stale_revision` 后重新采样、原回执未知，以及控制成功后 `observe` 失败再恢复。停止受理、停止完成与资源结算分别证明。

实施顺序和证据入口见[Plan](../plans/task-run-observation.plan.md)和[验收记录](../testing/acceptance/task-run-observation.md)。
