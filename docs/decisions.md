# 决策与反馈

SteerLine 当前使用 `worker-flow-v1`。SQLite 中的 Decision、Command、Operation 是问题、答案和发送进度的业务权威。控制台与 CLI 通过同一 `answer_decision` 命令提交；服务离线时只能读取和诊断。

## 问题与答案

local Codex 在活动轮次内调用 `steerline_ask`，或本机 Orca 在当前 Dispatch 中发来原生 question 时，中心为原 WorkflowRun、Execution 和固定 worker attempt 创建 pending Decision，提示“提交后继续当前轮次”。轮次结束后若需要反馈，中心创建另一类 Decision，提示“提交后在原会话开始下一轮”。PR 已核实并等待合并时，也可向原会话追加反馈。

每个 Decision 有独立 `decision_id` 和版本。请求保存完整问题、上下文、选项与详情，始终允许自由文字。local 运行中请求还保存 app-server 代次、thread、turn、call 与 JSON-RPC 请求 ID；Orca 问题保存 Run、Task、Dispatch 和原 message ID。多题答案按问题 ID 保存。单题可提交纯文字或“选项＋备注”，多题在备注中提交按问题 ID 键控的 JSON。中心不解释“继续”为重派某个技术步骤。

```powershell
steerline answer -config .\service.json `
  -run-id <run-id|task-id> -decision-id <decision-id> `
  [-option <option-id>] [-note "完整反馈"] [-by "<name>"] [-wait]
```

同一问题的答案 first-wins。相同内容和请求 ID 的重交返回原记录；不同内容返回冲突。客户端提交时绑定服务实例、run revision、decision version 和原 attempt；SQLite 事务在保存答案前重新核验这些身份。旧草稿不会静默发给后续轮次或另一任务。

## 送达事实

| 记录 | 含义 |
| --- | --- |
| `pending` | 等待用户输入 |
| `answer_saved` | 答案和命令已保存，尚不能断言 worker 收到 |
| `reply_accepted` | local 运行中问题有匹配的原生工具结果，或 Orca 已接受原问题 reply／活动 send；结束后反馈有匹配的原生新轮次 |
| `superseded` | 新问题取代旧问题，历史答案保留 |

外部发送前，中心保存固定的 Command ID、原生请求绑定及 Operation 意图。回复 local 运行中问题时，核对同一 call 的工具结果状态和答案正文；结束后反馈以原生 thread、turn 开始事件和固定工作区身份确认。Orca 按原 message/Dispatch 核对 reply、send 或新 Task/Dispatch；注入接受不冒充 agent 已处理。回执不明时沿原请求与身份对账，不用新 ID 盲发第二次。平台无法证明投递时保留原答案与不确定原因。

worker 处理反馈时，中心保持同一个 Execution、逻辑 session、worktree 和分支。轮次停止后如仍需输入，创建新的 Decision；真实 PR 合并或明确丢弃优先于未投递反馈。中心不要求 worker 回传 `applied_decision_id` 或答案摘要，也不把已保存／已接受误写成“已落实”。

local 路径使用每 Execution 独立的 Codex app-server 和固定 thread；重启重新连接原请求，或在确认轮次已结束后于原 thread 开始下一轮。本机 Orca 路径重启核实原 agent terminal/incarnation、coordinator 和 Run；仅在当前 Dispatch 结算后向原 terminal 注入下一轮。受监管远端仍未接入。进程退出或终端静默不能作为轮次停止证明。

`retry_run`、`mark_stopped`、`resolve_command` 不属于当前用户反馈入口。当前格式重启保留原 Decision、Command、Operation 和外部身份；无法自动核实的副作用保持待决策及诊断事实。
