# 人工决策与应用证明

SQLite 是当前问题、答案、命令和决策状态的唯一业务权威。控制台与 CLI 都向当前 `CommandService` 提交 `answer_decision`；服务离线时只提供读取与诊断，不写答案文件或维护路径。写入 `decision.answer.json` 不会授权续跑，旧 sidecar 决策投影会被服务明确拒绝且不会导入。

## 问题与回答

每个问题有新的 `decision_id`，绑定原 workflow run、execution、attempt 和问题版本。问题正文、上下文、最多 12 个选项、选项详情和推荐项完整保存在 `DecisionRecord`。同一问题的答案 first-wins：相同 option/note 重交返回原命令结果；内容不同返回 `answer_conflict`。答案不允许覆盖，重新提问必须使用新的 `decision_id`。

```powershell
steerline answer -config .\service.json `
  -run-id <run-id|task-id> -decision-id <decision-id> `
  [-option <option-id>] [-note "多行补充说明"] [-by "<name>"] [-wait]
```

有声明选项且 `allow_free_text=false` 时必须选择一个选项；`note` 可补充说明。有选项且 `allow_free_text=true` 时还可提交自定义答复，此时 `option_id` 为空且 `note` 非空。没有选项的问题也要求非空说明。问题文本与答案说明按协议长度校验；超限会拒绝，绝不静默截短。

命令目标绑定服务实例、`decision_id`、显示的 decision version 和 `run.control_revision`。服务在 SQLite 事务中重新核对 pending 状态、问题/attempt/execution 身份、实例与运行 revision，再同时保存 first-wins 答案和命令接受事实。旧草稿冲突时保留给操作者检查，不把答案应用到后续问题。

local `local_process` continuation 当前不受支持。服务会以相同能力规则拒绝它；不能通过重新启动另一执行伪装成续跑。Orca terminal v2 只对固定 terminal/incarnation 核验 `reply_continue`；发送前提示应用状态仍需证明，服务不会因终端接受输入就显示答案已应用。

## 持久状态与证明

| 状态 | 可核验含义 |
| --- | --- |
| `pending` | 问题已保存在 SQLite，尚无被服务接受的答案 |
| `answer_saved` | 当前 `CommandService` 已接受固定答案；不代表后端已收到 |
| `resuming` | 服务已保存续跑/worker 准备意图；重启后不自动重发 |
| `reply_accepted` | 后端接受了输入；决策仍待应用证明 |
| `applied` | 验证了绑定原 decision、原 decision attempt、execution 和固定答案摘要的证明 |
| `failed` | 原执行结束但无有效应用证明，或续跑明确失败；原问题、答案和 Operation 留档 |
| `superseded` | 历史问题由新的 decision_id 取代；旧答案不变 |

单次结果的 `decision_application` 要与 `decision_id`、原 `attempt_id`、`execution_id`、固定 `ExecutionRef` SHA-256 和答案摘要 SHA-256 相同。`turn.started`、进程存活、final 消息、PR 结果或单独的 reply receipt 都不足以证明应用。当前结果未提供匹配证明时，已结束的 run 进入 `blocked`，decision 进入 `failed`，不能补写 `applied`，也不能盲目重发 continuation。

minimal-v1 步骤结果要求 `applied_decision_id`、`applied_decision_attempt_id`、`applied_decision_execution_id` 和 `applied_answer_sha256` 同时匹配。步骤 envelope 自身还必须绑定本次 workflow/execution/resumed step attempt。旧 attempt 的迟到结果不会解锁新的决策。

受监管 Orca 的 `question` message ID 是 Decision ID，原 Attempt 和 Dispatch 在等待期间保持活跃。回答命令经服务保存后，用独立的 `reply_orca_question` Operation 调用 Orca `reply`；回执必须匹配原 question、Run、Dispatch、答案及平台 request ID。worker 以 `ask --json` 保存原问题 ID，收到答案后在同一 Attempt 发送 `decision_applied` status。回执丢失时，服务只读检查 Orca inbox 中原 question thread 的唯一答案消息；无法唯一确认就保持 `outcome_unknown`，不另开 worker 或重发答案。

远端 `decision_applied` 还必须满足本页固定答案摘要和原 Attempt/Execution 的应用证明门槛；只含 question ID 的旧探测回执不构成当前完整证明，生产远端能力保持未认证。

## 历史与恢复

历史页按 `decision_id` 展示本轮所有问题和已接受答案；折叠、展开、session/turn 结束只改变视图，不消费问题或修改决策。当前问题卡片留在折叠区外，UI 还分别呈现答案已保存、后端已接受、轮次已开始、已应用、失败与待核实。

retry、mark-stopped、accept-result 和 resolve-command 也通过同一 SQLite `CommandService` 在线提交。执行结果不确定时保留原 command/request id、回执和 blocker；重启不重放 worker start 或 terminal send。见 [PR 5 Spec](spec/pr-05-intervention.spec.md) 与 [验证报告](validation/pr-05-intervention.md)。
