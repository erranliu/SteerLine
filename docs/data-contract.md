# 当前数据契约：worker-flow-v1

SteerLine 尚未发布。当前生产配置必须显式设置 `"task_authority":"native"` 和 `"workflow_protocol":"worker-flow-v1"`。旧 `minimal-v1` 配置及 `workflow_definition`、`project_steps`、`workspace_dirty_ignore` 不属于生产协议。配置加载与正式服务入口在启动外部 worker 前拒绝旧协议；不导入、迁移、双写或覆盖旧现场。

## 权威与身份

SQLite schema 5 是 Task、固定 TaskContract、依赖、WorkflowRun、Execution、Decision、Operation、会话事件与 TaskDelivery 的唯一业务权威。现有 v3/v4 库只通过只读元数据检查识别并拒绝，不自动升级，也不开放业务读取；拒绝时保留数据库文件与可能仍在运行的外部执行。运行时投影为 schema 6，可在当前格式内重启恢复。旧投影或旧 WorkflowRun 在工作区准备之前拒绝。

TaskContract 固定目标仓库、remote、base 分支以及 Spec、Plan 和其他引用的 commit/SHA-256。领取前再次核验 Task 版本、依赖就绪、项目 authority、目标仓库和前置合并已进入当前 base。领取事务预留一个 Task、WorkflowRun、Execution 与固定 `worker` attempt；不生成 implement／verify／review／publish_pr 链。

| WorkflowRun 主阶段 | 条件 |
| --- | --- |
| `queued` | 领取已落盘，准备工作区或首次会话 |
| `executing` | 原 worker 正处理任务或反馈轮次 |
| `waiting_decision` | 原轮次停下，需要输入或外部结果无法自动核实 |
| `waiting_pr_merge` | 有已核实的 PR，worker 当前轮次已停下 |
| `completed` | 真实合并已核验，TaskDelivery 和 Task closed 已同事务提交 |

Task 的 `discarded` 是独立终态，不伪装成 Decision 或完成。连接、进程、回执不明和清理待办保存在 Operation、Execution 与事件中，不增设第二套可写主阶段。

Execution 绑定后端目标、工作区、分支、base、原生 Codex `session_id`、当前／待开始／已停下 turn ID 及 turn 序号。反馈只推进 turn 和消息身份，不替换 Execution、worker attempt 或会话。本次仅 local Codex 路径开放派发；Orca 本机与受监管远端代码保留，均在首次派发前拒绝。

## 工作区与消息

初始工作区在外部创建前保存固定意图。当前 local 路径使用固定 Git worktree 和原生 Codex session；重启核对原身份，不能按同名标题或路径推断替代会话。工作开始后，中心不会因反馈、dirty 文件、项目检查结果或 PR 出现而 reset、清理或改写 worker 的提交。

每次反馈有固定 Command ID 和 `start_worker_turn` Operation。答案先在 SQLite 保存，外部输入回执和原 session 的 turn.started 分别记录。local worker 的结构化 Codex JSON 输出写入中心 journal 与会话事件；回执不明时不更换 ID 重发。Orca 的实验性分块转运代码保留，但不属于本次可派发协议。详见 [决策与反馈](decisions.md)。

人工模式允许不同 Task 分别领取；自动模式仍一次占用一个活动执行。暂停派发只限制新领取，不中断当前 worker、待决策反馈或 PR 核验。

## PR 与交付

worker 或用户提供的 URL 只是线索。中心经配置的 provider 查询真实 PR，并核对 URL、目标仓库、base、head 仓库和当前 Execution 的工作分支。查询失败保留线索并重试；关闭未合并返回待决策，仍可继续原会话。后续向同一 PR 推送不要求重跑中心 verify/review。

`CompleteNativeTask` 必须核实 provider 的 merged 状态、合并时间与有效 merge commit，然后在一笔 SQLite 事务中重核 Task 版本、选定 WorkflowRun/Execution、PR 归属及整任务发布绑定。事务唯一写入 TaskDelivery、关闭 Task、完成 WorkflowRun 并释放 DAG 后继。重复轮询和迟到事件重用同一交付，不回退终态。错误 PR、普通 closed、worker 自述完成或用户文字不能替代真实合并。

`task discard` 保留原有未合并核验与外部清理事实；状态不明时不静默关闭可能仍在运行的资源。已丢弃任务的 PR 若后来真实合并，只记录交付冲突，不生成正常 TaskDelivery。

## 控制入口

`steerline task list/show/graph/export` 只读中心权威；validate/create/discard/add-predecessors 通过固定版本与请求 ID 操作。控制台或 CLI 的 `answer_decision` 共享 CommandService 提交路径。正式服务入口只运行 `worker-flow-v1`；旧五步的 retry、mark-stopped、task reopen 和项目步骤验证器没有当前 CLI 入口。旧状态的人工核实边界见 [worker-flow Spec](spec/worker-flow.spec.md)。

本任务的 [逐项验收状态](validation/worker-flow/status.md) 区分已验证、推迟和发布前人工合并链。文档中的协议定义本身不代表所有 WF1–WF12 已通过。
