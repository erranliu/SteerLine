# 决策与反馈

SteerLine 当前使用 `worker-flow-v1`。SQLite 中的 Decision、Command、Operation 是问题、答案和发送进度的业务权威。控制台与 CLI 通过同一 `answer_decision` 命令提交；服务离线时只能读取和诊断。

## 问题与答案

一个未完成的 worker 轮次停下且没有可交付 PR 时，中心为原 WorkflowRun、Execution 和固定 worker attempt 创建一个 pending Decision。worker 明确提出的问题优先显示。PR 已核实并等待合并时，也可向原会话追加反馈。

每个 Decision 有独立 `decision_id` 和版本。请求保存完整问题、上下文、选项与详情，始终允许自由文字。答案可为纯文字，也可同时包含选项与备注。选项和备注会一起送达原会话；中心不解释“继续”为重派某个技术步骤。

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
| `reply_accepted` | 原会话已观察到本次反馈对应的工作轮次 |
| `superseded` | 新问题取代旧问题，历史答案保留 |

外部发送前，中心保存固定的 turn／Command ID 和 Operation 意图。收到平台回执后记录接受事实；以原生 Codex session ID、turn 开始事件和固定工作区身份确认续聊。回执不明时沿原请求和原终端查询，不用新 ID 盲发第二次。平台无法证明投递时保留原答案与不确定原因。

worker 处理反馈时，中心保持同一个 Execution、逻辑 session、worktree 和分支。轮次停止后如仍需输入，创建新的 Decision；真实 PR 合并或明确丢弃优先于未投递反馈。中心不要求 worker 回传 `applied_decision_id` 或答案摘要，也不把已保存／已接受误写成“已落实”。

本次交付的 local 路径使用固定 Codex session 的 `exec resume`。Orca 本机与受监管远端适配代码和试验结果保留，但两种目标在本次首次派发前拒绝，功能与进一步验收后续完成。终端静默不能作为轮次停止证明。

`retry_run`、`mark_stopped`、`resolve_command` 不属于当前用户反馈入口。当前格式重启保留原 Decision、Command、Operation 和外部身份；无法自动核实的副作用保持待决策及诊断事实。
