# 决策与反馈

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../architecture/current-code-architecture.md)。

反馈与恢复按各后端的实际原生能力核验。

SteerLine 当前使用 `worker-flow-v2`。SQLite 中的 Decision、Command、Operation 是问题、答案和发送进度的业务权威。TUI 经 HTTP 映射到服务端 `answer_decision`，CLI 仍可使用内部 Go 产品入口；领域核验和提交规则共用。TUI 离线仅保留本窗口已观察内容与草稿，禁止新业务查询/变更；重开无观察时显示不可查询原因，不直接读库。

## 问题与答案

local Codex 在活动轮次内调用 `steerline_ask` 时，中心为原 WorkflowRun、Execution 和固定 worker attempt 创建 pending Decision，提示“提交后继续当前轮次”；异步问题沿[异步提问契约](../spec/native-async-questions.spec.md)核对资格及迟到答案。Orca 单终端当前不提供原生提问路由，不能沿用旧 Dispatch/question 协议。轮次结束后若需要反馈，中心创建另一类 Decision，提示“提交后在原会话开始下一轮”。PR 已核实并等待合并时，默认显示待合并，无需继续执行；保留自由文字及“修改候选代码 / 修复发布或 PR 元数据”的可选反馈 Decision，不提供必答默认 Continue 选项，不计为待交互。用户按 a 主动追加修改，填写非空反馈并明确提交后，才在原会话开始下一轮；Enter 只转到运行与反馈区域。原生问题、恢复及环境阻断仍按各自门槛处理。

每个 Decision 有独立 `decision_id` 和版本。请求保存完整问题、上下文、选项与详情，始终允许自由文字。local 运行中请求还保存 app-server 代次、thread、turn、call 与 JSON-RPC 请求 ID；停止后反馈绑定原 WorkflowRun、Execution、Attempt、会话与恢复版本。多题答案按问题 ID 保存。单题可提交纯文字或“选项＋备注”，多题在备注中提交按问题 ID 键控的 JSON。中心不解释“继续”为重派某个技术步骤。

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
| `reply_accepted` | local 运行中问题有匹配的原生工具结果；结束后反馈有匹配的原生新轮次/原终端输入接受与开始事实 |
| `superseded` | 新问题取代旧问题，历史答案保留 |

外部发送前，中心保存固定的 Command ID、原生请求绑定及 Operation 意图。回复 local 运行中问题时，核对同一 call 的工具结果状态和答案正文；结束后反馈以原生 thread、turn 开始事件和固定工作区身份确认。Orca 结束后反馈使用同一原终端的普通输入，核对原请求、runtime/terminal/incarnation 及接受/开始事实，不创建 Task/Dispatch。输入接受不冒充 agent 已处理。回执不明时沿原请求与身份对账，不用新 ID 盲发第二次。平台无法证明投递时保留原答案与不确定原因。

worker 处理反馈时，中心保持同一个 Execution、逻辑 session、worktree 和分支。轮次停止后如仍需输入，创建新的 Decision；真实 PR 合并或明确丢弃优先于未投递反馈。中心不要求 worker 回传 `applied_decision_id` 或答案摘要，也不把已保存／已接受误写成“已落实”。

local 路径使用受服务管理的 Codex app-server 和固定 thread；服务重开先核验原进程、thread、turn 与问题。原请求仍有效时，新提交的答案可投递原请求；原进程退出后旧请求失效，需通过新的恢复 Decision 人工继续。核实旧进程树已停止后，新的 Attempt 仅可使用新进程恢复原 thread；原会话不可核实时需人工处理；恢复不能在原 Execution 中创建替代会话。Orca 重启核实原 runtime、terminal/incarnation、worktree、pane/provider、请求与基线，先恢复只读观察；离线完成可补查询结算一次，但下一输入仍须取得恢复资格与显式确认。进程退出、屏幕变化或终端静默不能作为轮次成功停止的证明。详见[停服恢复 Spec](../spec/worker-service-recovery.spec.md)与[当前 Orca 完成规则](../spec/orca-completion-query.spec.md)。

`retry_run`、`mark_stopped`、`resolve_command` 不属于当前用户反馈入口。当前格式重启保留原 Decision、Command、Operation 和外部身份；无法自动核实的副作用保持待决策及诊断事实。

错误、部分提交和连接未知以原操作的原因信息展示，不各自派生重派或恢复按钮。服务操作、明确丢弃和宿主登录/授权保留各自语义；后端恢复能力按其专属验收分别核验。旧界面的目标选择、草稿和提交方式见[已归档的执行控制台设计](../archive/design/execution-console.md)。

Execution 的恢复门槛和版本每次换服务实例重新建立；第一次接管新领取任务只登记所属实例。先前已保存但未送达的答案保留为历史 Decision，新确认创建关联 Decision，不改写原 first-wins 答案。状态未核实、原输入结果不明或旧进程树仍可能写入时，答复入口不可执行。普通反馈复用仍存活的 worker；重建进程会创建新 Attempt，原 Attempt 的进程、会话和回执保持可查。

## 宿主阻断的中心动作

宿主环境阻断与 worker 业务问题分别处理。现有中心动作声明 `receiver=center` 和类型化 action，请求绑定 Execution、原 turn 与 blocker ID；命令沿用 Decision version、WorkflowRun revision 与服务实例核验。`open_terminal` 仅定位原 UI，不代表批准或修复；`recheck_environment` 先核验原身份、等待及未知 Operation，不通过自由文字“重试”触发重新派发。动作是否可用由当前后端真实能力决定，公开 HTTP 路由存在不代表所有执行路径都支持它。

宿主等待期间的普通文字与中心动作备注不混成新的 worker 指令。业务反馈的原问题、答案与发送事实保持独立，恢复后仍须核验原目标与投递资格；目标过期则保留并提示，未知发送只对账。原生会话查看是独立宿主副作用，精确绑定原会话，不创建业务轮次。

当前单终端 Orca 没有旧 supervised 的 Dispatch/message/结果 relay 或准备 nonce 路径；历史[环境验收](../acceptance/archive/completed/worker-readiness.md)保留原结果，不作为现行能力声明。当前能力、完成核验及未重跑范围见[Orca 架构](../archive/design/orca-native-worker.md)与[补查询验收](../acceptance/orca-completion-query.md)。

## 分步反馈与恢复

支持的原生活动问题仍沿原请求回复。停止后的代码反馈明确回 implement.execute，发布修复留 publish；正常报告只整理绑定的源执行，不补做实现。当前 development/v3 不保存独立 artifact 快照，已提交报告的恢复必要时先回原会话重新核实实现。report 活动期间的新反馈保存等待，不能改写已冻结报告输入或并发新执行。已知阻断保持当前步骤，unknown 只读核对原操作/产物。服务重开保留原人工恢复门槛，不自动补发 queued 的报告或下一阶段；原会话不可恢复时需人工处理。当前规则见[staged_flow.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/staged_flow.go)，历史分步契约见[Spec](../spec/worker-staged-flow.spec.md)。
