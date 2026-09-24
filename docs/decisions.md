# 人工决策通道

SQLite 是请求、答案和决策状态的权威来源。CLI 与 viewer 通过 `CommandService` 追加答案命令，服务校验并应用；`runtime.json` 和决策文件仅用于展示或诊断。手工写入 `decision.answer.json` 不会授权续跑。

## 提问与回答

`minimal-v1` 的步骤结果可携带 `result.decision`：必须有 `question`，以及最多 12 个选项或 `allow_free_text`。结果须匹配本次 WorkflowRun、Execution、Attempt 和前置结果身份。服务保存问题与工作区证据，将流程置为 `waiting_decision`，保留任务领取和工作区。

```powershell
steerline answer -config .\service.json `
  -run-id <run-id|task-id> -option <option-id> [-note "..."] [-decision-id <id>] [-by "<name>"]
```

回答入口从当前数据库读取问题，提交包含目标 ID、expected_version、idempotency_key、回答者与答案的命令。服务暂停时仍可排队。无效选项或身份不匹配会在占用版本前拒绝；同版本只有一个答案命令，同幂等键同内容重试复用原命令。viewer 携带用户看到的问题版本，版本变化后必须刷新。

## 状态含义

| 状态 | 证据 |
| --- | --- |
| `pending` | 问题已持久化，尚无已保存答案 |
| `answer_saved` | 服务已通过命令校验并将答案写入数据库 |
| `reply_accepted` | 后端已接受续跑输入；尚无答案应用证明 |
| `applied` | 当前尝试返回匹配的 `result.applied_decision_id`，证明已应用该答案 |
| `failed` / `superseded` | 续跑失败或问题已被取代 |

本地 minimal-v1 回答后为同一步建立新的尝试，固定输入包含原问题、数据库中的答案和前置结果正文；原尝试保留为历史，不推进到下一步。返回结果必须携带 `applied_decision_id`，且值等于输入的 `decision.answer.decision_id`。worker 启动和正常退出都不能单独证明决策已应用。

现有 Orca terminal v2 续跑继续核验原终端与 incarnation；terminal send 成功只记录 `reply_accepted`。协议没有提供应用证明时不升级为 `applied`。

## 恢复与交付

已保存但未派发的步骤意图在重启后从原步骤继续；已开始但结果不明的外部动作进入阻塞，不盲目重发。续跑重新核验任务领取、工作区身份和执行槽。implement 可以继续决策暂停时保存的代码现场；verify/review/publish_pr 必须保持固定的干净 revision。

步骤完成不关闭 Task。发布与合并核验先检查精确 PR、目标分支、head 和对应验证/审查证据，通过后才能执行外部任务结项。

请求文件和 `continue.txt` 可保留供诊断或作为执行输入，但不能替代数据库中的版本、答案与应用状态。事件与领域状态在同一事务提交，展示日志不参与恢复。
