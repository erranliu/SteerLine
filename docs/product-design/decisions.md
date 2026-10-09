# 决策与反馈

当前一期反馈与恢复流程只覆盖本地 Codex/本机 Orca；远端协议资料保留，不作为一期运行入口或验收前置。

SteerLine 当前使用 `worker-flow-v2`。SQLite 中的 Decision、Command、Operation 是问题、答案和发送进度的业务权威。控制台与 CLI 通过同一 `answer_decision` 命令提交；服务离线时只能读取和诊断。

## 问题与答案

local Codex 在活动轮次内调用 `steerline_ask`，或本机 Orca 在当前 Dispatch 中发来原生 question 时，中心为原 WorkflowRun、Execution 和固定 worker attempt 创建 pending Decision，提示“提交后继续当前轮次”。轮次结束后若需要反馈，中心创建另一类 Decision，提示“提交后在原会话开始下一轮”。PR 已核实并等待合并时，默认显示待合并，无需继续执行；保留自由文字及“修改候选代码 / 修复发布或 PR 元数据”的可选反馈 Decision，不提供必答默认 Continue 选项，不计为待交互。用户按 a 主动追加修改，填写非空反馈并明确提交后，才在原会话开始下一轮；Enter 只转到运行与反馈区域。原生问题、恢复及环境阻断仍按各自门槛处理。

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

local 路径使用受服务管理的 Codex app-server 和固定 thread；服务重开先核验原进程、thread、turn 与问题。原请求仍有效时，新提交的答案可投递原请求；原进程退出后旧请求失效，需通过新的恢复 Decision 人工继续。核实旧进程树已停止后，新的 Attempt 仅可使用新进程恢复原 thread；原会话不可核实时需人工处理；恢复不能在原 Execution 中创建替代会话。本机 Orca 路径重启核实原 agent terminal/incarnation、coordinator 和 Run；仅在当前 Dispatch 结算后向原 terminal 注入下一轮。结果 relay 文件在重开后等待人工确认，不自动 `worker_done`。远端反馈/恢复协议只作保留资料，不是一期执行入口。进程退出或终端静默不能作为轮次成功停止的证明。详见[停服恢复 Spec](../spec/worker-service-recovery.spec.md)。

`retry_run`、`mark_stopped`、`resolve_command` 不属于当前用户反馈入口。当前格式重启保留原 Decision、Command、Operation 和外部身份；无法自动核实的副作用保持待决策及诊断事实。

错误、部分提交和连接未知以原操作的原因信息展示，不各自派生重派或恢复按钮。服务操作、明确丢弃和宿主登录/授权保留各自语义；本地 Codex 与本机 Orca 的恢复能力分别核验，不能通过隐式重启或跨后端替换补造同等能力。当前界面的目标选择、草稿和提交方式见[执行控制台](execution-console.md)。

Execution 的恢复门槛和版本每次换服务实例重新建立；第一次接管新领取任务只登记所属实例。先前已保存但未送达的答案保留为历史 Decision，新确认创建关联 Decision，不改写原 first-wins 答案。状态未核实、旧 reply/send 的结果不明或旧进程树仍可能写入时，答复入口不可执行。普通反馈复用仍存活的 worker；重建进程会创建新 Attempt，原 Attempt 的进程、会话和回执保持可查。

## 宿主阻断的中心动作

Windows Orca Codex 的环境阻断也通过 `answer_decision` 呈现。选项声明 `receiver=center` 和类型化 action，请求绑定 Execution、准备意图/turn、blocker ID；命令沿用 Decision version、WorkflowRun revision 与服务实例核验。`open_terminal` 仅定位原 UI，不代表批准或修复；`recheck_environment` 先核验原身份、等待及未知 Operation，不通过自由文字“重试”触发重新派发。

宿主等待期间的普通文字原样保存为待投递反馈，并提供新的中心动作 Decision；原 worker question/message ID、答案与发送事实保持独立。恢复后只向仍有效的原 Dispatch 投递一次；原目标过期则保留并提示，结果未知的旧发送只对账。中心动作附带备注保存在原接受记录中，不作为 worker 指令注入。准备问题使用原 message ID 回复，但不进入业务执行或计数。

受管环境重启恢复原准备期限、nonce、工具/收据事实和 Operation。原结果 relay 沿已保存操作对账；已核实原身份、尚未尝试转发的 worker 声明可走原 relay，未知回执不再次发送。其他 Orca agent 与 local 的原停服恢复入口不变。真实成功链路及原身份对账已通过，见[验收记录](../acceptance/archive/completed/worker-readiness.md)。

## 分步反馈与恢复

原生活动问题仍沿原请求回复。停止后的代码反馈明确回 implement.execute，发布修复留 publish；报告继续只整理同一源执行/快照，不能补做实现。report 活动期间的新反馈保存等待，不能改写已冻结报告输入或并发新执行。已知阻断保持当前步骤，unknown 只读核对原操作/产物。服务重开保留原人工恢复门槛，不自动补发 queued 的报告或下一阶段；原会话不可恢复时需人工处理。完整规则见 [分步 Spec](../spec/worker-staged-flow.spec.md)。
