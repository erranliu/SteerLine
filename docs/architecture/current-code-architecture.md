# SteerLine 当前代码架构图

快照日期：2026-09-30。源码基线：`bba4936`，生成前工作区无未提交修改。

本文描述当前检出的代码：系统边界、实际 Go 包依赖、核心包内部职责、任务执行、人工决策、数据模型、日志与恢复。图中的职责分组不代表已拆成独立包或服务。箭头含义在各图前说明；未来方案与当前可启动路径分开列出。

## 1. 系统与进程总览

实线表示当前调用或数据通路，虚线表示存在代码但当前不能启动的执行路径。CLI 和控制台可以是不同进程；`run` 在独立服务进程中运行。查询经 Go 读取 SQLite，当前客户端控制协议主要使用文件请求/回执和数据库命令，而不是第二阶段计划中的 HTTP/SSE。

```mermaid
flowchart TB
    Human["用户"] --> CLI["cmd/steerline<br/>CLI / projects / task"]
    Human --> TUI["console 包<br/>Bubble Tea + Lip Gloss"]
    Config["项目 etc/*.json<br/>全局 execution.json"] --> CLI
    Config --> TUI
    Config --> Service

    subgraph Center["SteerLine 本机中心"]
        Query["QueryService / ConversationQueryService<br/>ConnectService / 日志读取"]
        Command["CommandService<br/>校验版本、身份、幂等键"]
        Inbox["control/requests<br/>control/receipts"]
        Service["Service 常驻循环<br/>调度 / worker 事件 / 决策 / PR / 恢复"]
        DB[("steerline.db<br/>任务、执行、决策、交付、会话<br/>服务 checkpoint")]
        Journal["工作日志 / activity.jsonl<br/>进程回执 / 发布证据"]
    end

    CLI --> Query
    TUI --> Query
    Query --> DB
    Query --> Journal
    CLI --> Command
    TUI -->|回答决策| Command
    Command --> DB
    CLI -->|启动、派发模式、停止| Inbox
    TUI -->|启动、派发模式、停止| Inbox
    Inbox <--> Service
    Service <--> DB
    Service --> Journal

    subgraph Execution["外部执行与交付"]
        Local["codexworker → codexapp<br/>Codex app-server<br/>回环 WebSocket / JSON-RPC"]
        Orca["orcaworker + Orca CLI<br/>本机 runtime<br/>coordinator / task / dispatch / terminal"]
        Remote["orca_supervised<br/>配置、适配、核验代码存在<br/>worker-flow-v1 入口阻止启动"]
        Git["Git repository / worktree / branch"]
        PR["PRClient<br/>Gitea API / command provider"]
        Provider["代码托管平台<br/>PR 与合并证据"]
    end
    Service <--> Local
    Service <--> Orca
    Service -.-> Remote
    Local --> Git
    Orca --> Git
    Service -->|准备工作区 / 本机受控发布| Git
    Service --> PR
    PR --> Provider
    Human -->|审阅、合并 PR| Provider
```

**业务权威**是 SQLite 中的持久记录。服务负责运行中的业务推进；CLI 的任务创建、废弃、追加前置等命令也能通过 `CommandService` 写库，但须取得仓库身份锁。客户端提交命令不等于执行成功，agent 输出和日志不等于任务交付。

**生命周期**：Windows 通过可见的 `service-window.bat` 承载服务；控制台退出不会停止服务。服务重启先对账保存的身份、命令和外部状态，再允许新派发。当前代码有 Windows/Linux 平台实现，代码存在不表示各平台均完成原生验收。

源码入口：[main.go](../../cmd/steerline/main.go)、[Service](../../internal/steerline/service.go)、[console/source.go](../../internal/steerline/console/source.go)、[control_protocol.go](../../internal/steerline/control_protocol.go)、[native_project_open.go](../../internal/steerline/native_project_open.go)。

## 2. 实际 Go 包依赖

箭头仅表示非测试 `.go` 源文件中的项目内 `import`，不代表运行时调用。所有 `internal/steerline/` 子包在图中用短名表示。

```mermaid
flowchart LR
    CLI["cmd/steerline"] --> Core["internal/steerline<br/>103 个非测试 Go 文件"]
    CLI --> Console["console"]
    Console --> Core

    Core --> Task["task"]
    Core --> TaskView["taskview"]
    Core --> Flow["workerflow"]
    Core --> CW["codexworker"]
    Core --> CA["codexapp"]
    Core --> OW["orcaworker"]
    Task --> Graph["taskgraph"]
    TaskView --> Task
    TaskView --> Graph
    Flow --> Task
    CW --> CA

    Feedback["cmd/test-feedback"] --> Core
    Feedback --> Summary["internal/testfeedback"]
    Probe["cmd/orca-probe<br/>仅使用标准库"]
    Verify["remoteprotocol<br/>当前无生产包导入"]
```

| 包 | 当前职责 | 非测试 Go 文件 |
| --- | --- | ---: |
| `cmd/steerline` | 产品命令入口、服务启动、控制台和任务命令路由 | 1 |
| `internal/steerline` | 服务编排、业务持久化、平台适配接线、查询和控制 | 103 |
| `console` | TUI 状态、导航、任务列表、活动/会话/正文、执行方案和决策编辑 | 14 |
| `task` | 任务契约、批次、状态、依赖资格、交付和命令规则 | 10 |
| `taskgraph` | DAG 校验、遍历、前后置关系和图算法 | 1 |
| `taskview` | 任务展示、分页和投影规则 | 3 |
| `workerflow` | 单个可续聊 worker 的纯状态规则、模式选择和提示构造 | 3 |
| `codexapp` | Codex app-server 参数、客户端、原生问题与 WebSocket 协议 | 6 |
| `codexworker` | app-server 启动、连接、能力探测、轮次观察、journal 与回答 | 10 |
| `orcaworker` | Orca task/dispatch、terminal、消息、pull/relay、输出与工作区协议 | 14 |
| `remoteprotocol` | 结构化远端 verify result 与固定执行证据核验；当前未接入生产包依赖 | 1 |
| `cmd/orca-probe` | 只读能力探测与脱敏证据采集 | 1 |
| `cmd/test-feedback` | 最小/定向/全量测试调度、日志与反馈报告 | 4 |
| `internal/testfeedback` | Go test JSON 结果汇总 | 1 |

合计 **14 个包目录、172 个非测试 Go 文件**。统计覆盖 `cmd/` 与 `internal/`；不含 `_test.go`、测试数据、脚本和文档。核心包仍集中承载多项职责，不能将上述逻辑边界理解为微服务或已经完成的全面拆包。

## 3. 核心包内部职责与接线

图中节点是同一 `steerline` 包内的逻辑职责。实线表示主要协作关系，不是完整函数调用图。

```mermaid
flowchart TB
    Config["配置与项目身份<br/>config / projects / execution_profiles"] --> Service["Service<br/>service / service_workers"]
    Lifecycle["服务承载与连接<br/>service_client / platform_* / launchers"] --> Service
    Control["控制协议与处理<br/>control_client / control_service"] --> Service
    Commands["数据库命令<br/>query_command / command_processor"] --> Service
    Service --> Native["native 调度与任务命令<br/>claim / dispatch / batch / discard / cleanup"]
    Native --> Rules["task / taskgraph<br/>契约、资格、依赖、交付规则"]
    Service --> Flow["worker_flow_service / state<br/>续聊、事件、PR 与人工反馈"]
    Flow --> FlowRules["workerflow<br/>平台无关状态转换"]
    Flow --> Binding["worker_flow_adapter<br/>固定执行身份与模式选择"]
    Binding --> Local["worker_flow_app_server_*<br/>codexworker / codexapp"]
    Binding --> Orca["worker_flow_orca_*<br/>orcaworker / orca / orcaexec"]
    Binding -.-> Supervised["orca_supervised<br/>当前启动门槛未开放"]
    Flow --> Workspace["workspace / workspace_dirty<br/>worktree 与 Git 检查"]
    Flow --> Publish["worker_flow_publish_local<br/>受控 commit / push / draft PR"]
    Publish --> PR["pr / native_delivery_command<br/>PR 身份、合并证据和交付"]
    Flow --> PR
    Service --> Recovery["recovery / execution_recovery<br/>app-server 与 worker 恢复对账"]
    Recovery --> Binding
    Service --> Conversation["conversation_* / codex_history<br/>事件归一、正文、游标和补读"]
    Service --> Activity["activity / journal_reader<br/>活动与原始日志"]
    Native --> Store["SQLiteStore / DomainStore / DomainTx<br/>sqlite_store_* / execution_store"]
    Flow --> Store
    PR --> Store
    Conversation --> Store
    Commands --> Store
    Store --> Projection["runtime_view / native_graph_projection<br/>Runtime、Run 与任务展示视图"]
    Projection --> Query["QueryService / ConversationQueryService<br/>native_task_page / native_graph_query"]
    Query --> ViewRules["taskview"]
```

| 职责 | 主要源码 | 关键边界 |
| --- | --- | --- |
| 配置与固定执行方案 | `config.go`、`execution_profiles.go`、`service_execution_profiles.go`、`projects.go` | 项目身份、PR 和 DAG 属于项目；每次领取冻结 worker 执行方案和摘要 |
| 服务调度与事件循环 | `service.go`、`service_workers.go`、`native_service.go` | worker 观察器按 workflow run ID 管理；启动、回答和恢复不依赖一个共享的“当前 worker” |
| 控制与命令 | `control_*.go`、`query_command.go`、`command_processor.go` | 控制文件绑定服务实例；数据库命令有版本和幂等身份 |
| 任务管理 | `native_batch_command.go`、`native_claim_command.go`、`native_dispatch_command.go`、`native_task_predecessors.go`、`native_discard_command.go` | 验证固定契约、DAG 与版本；废弃后的外部清理单独对账 |
| 图与候选读取 | `native_graph_*.go`、`native_readiness*.go`、`native_task_page*.go` | 使用事务快照和游标；展示 Ready 缓存不是领取许可 |
| 单 worker 生命周期 | `worker_flow_service.go`、`worker_flow_state.go`、`worker_flow_adapter.go`、`worker_flow_resume_adapter.go` | Session、Turn、Execution、Attempt 和 Operation 必须对应持久身份 |
| 本机 Codex | `worker_flow_app_server*.go`、`codex_history.go`、`codexworker/`、`codexapp/` | 实际 worker 经回环 WebSocket 连接 app-server；不是第二阶段规划会话的 stdio 方案 |
| Orca | `worker_flow_orca*.go`、`orca.go`、`orcaexec.go`、`orcaworker/` | coordinator、runtime、task、dispatch、terminal 与消息回执分别核验 |
| 工作区与发布 | `workspace*.go`、`worker_flow_publish_local.go`、`codex_git_sandbox.go` | 本机 worker 发出发布就绪标记后，中心校验并办理发布 |
| PR 与交付 | `pr.go`、`native_delivery_command.go`、`task/delivery*.go` | 合并事实先外部查询，再事务内重检并写交付与 closed 状态 |
| 决策与干预 | `decision.go`、`worker_feedback_phase.go`、`operation.go` | 保存答案、送达回执、原会话新轮次开始是不同事实 |
| 日志与会话 | `activity.go`、`journal_*.go`、`conversation*.go`、`codex_conversation.go` | 原始日志、归一事件、正文版本与最终回复分别处理 |
| 持久化与投影 | `sqlite_store*.go`、`execution_store.go`、`store.go`、`runtime_view.go` | SQLite 业务实体是权威；checkpoint 不保存第二套业务状态 |
| 恢复与平台 | `recovery.go`、`execution_recovery.go`、`platform_*.go`、`spawner.go` | 对账原身份；不因超时、观察断线或重启直接创建替代执行 |

## 4. 任务从创建到交付

这是业务流程图。每次“已请求”“已接受”“已确认”均需对应持久记录或外部证据。当前 worker 可以多轮续聊，不应将历史的 prepare/implement/verify/review 分阶段方案当作生产必经流水线。

```mermaid
flowchart TB
    Batch["CLI task create<br/>固定 Spec/Plan 引用与批次"] --> Validate["校验契约、文档、任务 ID、DAG<br/>CommandService + task/taskgraph"]
    Validate --> Create["事务创建 Task / Contract / Dependency<br/>保存批次与命令回执"]
    Create --> Ready["QueryService 计算 Ready<br/>前置任务具有一致交付证据"]
    Ready --> Start["人工选择执行方案并确认<br/>或自动派发默认方案"]
    Start --> Admit["服务重新校验资格与配置版本<br/>领取任务并冻结执行方案"]
    Admit --> Records["Task in_progress<br/>WorkflowRun / Execution / Attempt / Operation"]
    Records --> Prepare["能力探测、工作区准备<br/>保存外部操作意图与身份"]
    Prepare --> Worker["固定 Codex session / Orca worker<br/>开始并确认原生轮次"]
    Worker --> Events["持久化事件、会话、活动<br/>核验轮次及执行证据"]
    Events --> Question{"需要人工决策？"}
    Question -->|是| Decision["Decision → answer_saved<br/>回答送达原执行 / 原会话"]
    Decision --> Worker
    Question -->|否| Publication["本机：发布就绪标记 → 中心受控发布<br/>Orca：核验 worker 提供的 PR"]
    Publication --> Wait["绑定准确 PR 身份<br/>waiting_pr_merge"]
    Wait --> Merge["用户审阅并合并<br/>服务轮询 PR provider"]
    Merge --> Delivery["事务重检所选执行与任务版本<br/>保存 TaskDelivery / Task closed<br/>推进 graph revision"]
    Delivery --> Complete["WorkflowRun completed<br/>重新计算下游 Ready"]
    Complete --> Ready
    Prepare -->|结果未知| Blocked["blocked / outcome_unknown<br/>恢复对账或人工处理"]
    Events -->|身份或证据不明确| Blocked
    Publication -->|发布结果待核实| Blocked
    Blocked -->|核验原身份后续接| Prepare
```

人工派发允许多个不同 Ready 任务并行，每个任务冻结独立执行方案与 worker 身份；同一任务不能重复领取。自动模式一次保持一个活动执行，切换自动模式不会停止已经人工启动的任务。领取、进程开始、轮次结束、PR 合并、任务交付不是同一层状态。

源码：[native_service.go](../../internal/steerline/native_service.go)、[worker_flow_service.go](../../internal/steerline/worker_flow_service.go)、[workerflow/state.go](../../internal/steerline/workerflow/state.go)、[worker_flow_publish_local.go](../../internal/steerline/worker_flow_publish_local.go)、[native_delivery_command.go](../../internal/steerline/native_delivery_command.go)。

## 5. 人工决策与原会话续聊

这是时序图。原生问题响应与普通反馈续聊分别处理；“答案被保存”不能展示为“agent 已应用”。

```mermaid
sequenceDiagram
    participant W as 固定 worker / 原会话
    participant S as Service
    participant DB as SQLite
    participant UI as CLI / TUI
    participant C as CommandService

    W->>S: 原生问题或需要人工反馈的证据
    S->>DB: 保存 Decision 与执行绑定
    UI->>DB: 经 QueryService 读取问题、选项、版本
    UI->>C: 答案 + decision ID + 预期版本 + 幂等身份
    C->>DB: 事务校验并保存数据库命令
    S->>DB: 读取 pending command
    S->>C: 接受答案，校验持久 control revision
    C->>DB: 保存 answer_saved 与命令处理结果
    alt Codex 原生 request_user_input
        S->>W: 向原 app-server 请求响应
        W-->>S: 响应送达回执
        S->>DB: 保存送达事实，原轮次继续
    else 普通执行反馈
        S->>DB: 保存续聊 Operation 和输入意图
        S->>W: 向固定 session / worker 发送新轮次输入
        W-->>S: 接受回执与原生 turn-start 证据
        S->>DB: 保存 reply_accepted、turn ID 和运行状态
    end
    UI->>DB: 读取答案保存、送达与轮次开始状态
```

配置、session、run、execution、turn 身份不匹配时拒绝发送；结果未知时保留原命令和操作身份等待对账。心跳和日志更新不会改变 WorkflowRun 的持久控制版本。

源码：[query_command.go](../../internal/steerline/query_command.go)、[command_processor.go](../../internal/steerline/command_processor.go)、[worker_flow_adapter.go](../../internal/steerline/worker_flow_adapter.go)、[worker_flow_app_server_reply.go](../../internal/steerline/worker_flow_app_server_reply.go)。

## 6. 核心数据模型

图表示主要领域关系，省略所有关系共有的 `project_identity`。部分关系保存在 payload 中，并不全部是 SQL 外键；完整表定义见 `sqlite_store_schema.go`。一个任务可有历史 workflow，但数据库约束同一任务只有一个非终态 workflow，同一 workflow 只有一个活动 execution。

```mermaid
erDiagram
    ProjectAuthority ||--o{ Task : owns
    Task ||--|| TaskContract : fixes
    TaskBatch ||--o{ Task : creates
    Task ||--o{ TaskDependency : successor
    Task ||--o{ TaskDependency : predecessor
    Task ||--o{ WorkflowRun : history
    WorkflowRun ||--o{ Execution : executes
    Execution ||--o{ StepAttempt : attempts
    WorkflowRun o|--o{ Decision : scope
    Execution o|--o{ Decision : scope
    StepAttempt o|--o{ Decision : scope
    Decision ||--o{ DecisionAnswer : answers
    WorkflowRun ||--o| TaskDelivery : selected_run
    Task ||--o| TaskDelivery : delivery
    Execution ||--o{ DomainEvent : evidence
    Execution ||--o{ ConversationEvent : messages
    StepAttempt ||--o{ ConversationEvent : source

    Task {
        string task_id PK
        string status
        uint64 version
        string contract_id
    }
    TaskContract {
        string contract_id PK
        string digest
        blob fixed_file_references
    }
    WorkflowRun {
        string workflow_run_id PK
        string task_id FK
        string phase
        uint64 control_revision
        blob fixed_execution_profile
        string pr_url
    }
    Execution {
        string execution_id PK
        string workflow_run_id FK
        string protocol
        string session_id
        string current_turn_id
        string status
        blob workspace_and_target
    }
    StepAttempt {
        string attempt_id PK
        string execution_id FK
        string step_id
        string operation_id
        string status
    }
    Decision {
        string decision_id PK
        string question_id
        uint64 version
        string status
        blob request_and_answer
    }
    TaskDelivery {
        string task_id PK
        string selected_workflow_run_id FK
        string pr_url
        string merge_commit
        string merged_at
    }
```

| 存储类别 | 当前表 | 用途 |
| --- | --- | --- |
| 版本与项目权威 | `schema_migrations`、`database_state`、`project_task_authorities` | schema、数据库身份与版本、项目绑定、派发模式、图版本 |
| 任务与 DAG | `tasks`、`task_contracts`、`task_batches`、`task_dependencies`、`task_deliveries` | 固定契约、不可随意修改的依赖、任务创建批次和合并交付证据 |
| 执行 | `workflow_runs`、`executions`、`step_attempts` | workflow、外部执行绑定、workspace、session/turn 和 attempt |
| 命令、操作与决策 | `commands`、`operations`、`decisions`、`decision_answers` | 命令幂等、外部副作用意图、决策版本与回答回执 |
| 底层执行记录 | `execution_bindings`、`execution_operations` | 后端执行绑定与操作记录 |
| 领域事件 | `events` | 业务事实与可分页读取的证据 |
| 会话 | `conversation_events`、`conversation_bodies`、`conversation_current_messages`、`conversation_raw` | 归一事件、完整正文版本、当前消息和原始来源 |
| 会话读取位置与能力 | `conversation_source_cursors`、`conversation_capability_reports` | 恢复读取游标、来源能力及版本证据 |
| 服务 checkpoint | `projection` | 服务实例、心跳、未完成控制操作、阻塞信息与认领前预留；不持久化第二套 Runtime/Run 业务权威 |

当前 SQLite schema 为 **6**，服务 checkpoint schema 为 **8**。数据库使用 WAL、外键、busy timeout 和事务；旧格式直接拒绝，不增加迁移、双写或回退路径。

源码：[domain.go](../../internal/steerline/domain.go)、[task/model.go](../../internal/steerline/task/model.go)、[sqlite_store_schema.go](../../internal/steerline/sqlite_store_schema.go)、[runtime_view.go](../../internal/steerline/runtime_view.go)。

## 7. 输出、会话与恢复读取

箭头表示采集和读取通路。会话补读与业务 worker 事件处理并存；日志只能提供相应来源确认的事实。

```mermaid
flowchart LR
    Codex["Codex app-server 事件"] --> Worker["codexworker journal / observe"]
    Orca["Orca 消息与 terminal 输出"] --> OW["orcaworker pull / output / relay"]
    Worker --> Service["Service workerMessages<br/>worker_flow_evidence"]
    OW --> Service
    Service --> Domain[("领域 events<br/>执行 / 决策 / operation")]
    Service --> Activity["ActivityWriter<br/>activity.jsonl"]
    Worker --> Raw["原始 journal"]
    OW --> Raw
    History["Codex 原生历史<br/>codex_history"] --> Mapper["ConversationMapper<br/>conversation_service / replay"]
    Raw --> Mapper
    Service --> Mapper
    Mapper --> Conversation[("conversation_*<br/>事件 / 正文 / raw / source cursor")]
    Domain --> View["QueryService / Runtime 投影"]
    Conversation --> CQ["ConversationQueryService<br/>分页事件 / 正文 / final"]
    Activity --> JR["journal_reader<br/>来源身份与游标检查"]
    View --> TUI["console<br/>任务 / 活动 / 会话 / 最终回复"]
    CQ --> TUI
    JR --> TUI
    Conversation --> Replay["服务重启<br/>恢复 mapper 与 source cursor"]
    Replay --> Mapper
```

读取视图在同一事务中组合业务记录和 checkpoint。会话事件及来源游标持久化后可以补读，正文按身份与版本定位；来源没有明确 turn 终态或 final 时，界面显示未识别或能力原因。重启恢复还需核验进程身份、app-server generation、固定工作区、Orca dispatch 和未完成 Operation，不能只从日志推测是否完成。

## 8. 工具、依赖与尚未交付的边界

| 项目 | 当前定位 |
| --- | --- |
| Go 1.24.2 | `go.mod` 声明的 Go 版本 |
| Bubble Tea / Lip Gloss / ANSI / runewidth | 当前终端控制台与文本渲染依赖 |
| `modernc.org/sqlite` | 当前 SQLite 驱动 |
| `golang.org/x/sys` | 平台进程和系统能力 |
| `build.ps1` / `build.bat` | Windows 本地构建 `cmd/steerline` 到 `bin/steerline.exe` |
| `internal/steerline/launcher/` | 内嵌 console/service-window 启动入口与说明 |
| `scripts/steerline-pr-provider.py` | 调用 `gh`，提供准确 GitHub PR 查询与关闭事实；属于 command provider 适配 |
| `scripts/roadmap_task_id.py` | 通过 Git ref 占用与校验 roadmap task ID；不是当前运行调度器 |
| `examples/` | 项目配置、全局执行方案、任务批次与控制协议示例 |
| `docs/spec/`、`docs/roadmap/`、`docs/validation/` | 执行契约、规划依赖与验收证据；不是运行时服务模块 |
| `docs/phase-2/prototype/index.html` | 五页交互原型，演示数据与按钮没有接真实服务 |
| WinUI 3、规划主 agent、规划 worktree、受控文档发布、HTTP/JSON/SSE | 第二阶段 Spec/方案，当前 `cmd/`、`internal/` 中没有对应产品实现 |
| stable/dev 固定版本选择与发布 | 当前只有本地构建、安装相关代码；固定版本运行交付仍为 Spec |
| `orca_supervised` | 配置和多个适配/核验函数存在；`StaticUnavailableReason` 和 native preflight 阻止当前单 worker 启动 |
| `remoteprotocol` | 独立结果核验包；生产源码 import 图中尚未被使用 |
| Pi 输出归一代码 | `activity.go` 中存在归一器，不代表当前本机 worker-flow 支持 Pi 执行 |

## 9. 快照验证与维护

包依赖图由非测试源码的项目内 import 提取并人工对照入口。系统、执行与存储图对照当前关键代码和 schema，不依据未来 Spec 推断已交付能力。本文仅新增说明文档，不修改业务代码，不运行全量测试。

维护时优先核对：入口命令、包 import、可启动执行模式、schema、checkpoint、控制协议、外部进程通信和交付事务。新增生产源码后应同步下面的全量文件索引；生产支持范围有变化时同步图 1、图 4 和第 8 节。

## 附录：全部非测试 Go 源文件索引

下列索引覆盖 `cmd/` 与 `internal/` 中全部非测试 `.go` 文件，按真实包目录分组。每个包的文件名可定位到上面对应职责；核心包内部职责划分见第 3 节。

### cmd/orca-probe

| 源文件 |
| --- |
| [main.go](../../cmd/orca-probe/main.go) |

### cmd/steerline

| 源文件 |
| --- |
| [main.go](../../cmd/steerline/main.go) |

### cmd/test-feedback

| 源文件 |
| --- |
| [main.go](../../cmd/test-feedback/main.go) |
| [process_unix.go](../../cmd/test-feedback/process_unix.go) |
| [process_windows.go](../../cmd/test-feedback/process_windows.go) |
| [scoped.go](../../cmd/test-feedback/scoped.go) |

### internal/steerline

| 源文件 |
| --- |
| [activity.go](../../internal/steerline/activity.go) |
| [adapter_error.go](../../internal/steerline/adapter_error.go) |
| [codex_conversation.go](../../internal/steerline/codex_conversation.go) |
| [codex_git_sandbox.go](../../internal/steerline/codex_git_sandbox.go) |
| [codex_history.go](../../internal/steerline/codex_history.go) |
| [codex.go](../../internal/steerline/codex.go) |
| [command_processor.go](../../internal/steerline/command_processor.go) |
| [command.go](../../internal/steerline/command.go) |
| [config.go](../../internal/steerline/config.go) |
| [control_client.go](../../internal/steerline/control_client.go) |
| [control_protocol.go](../../internal/steerline/control_protocol.go) |
| [control_service.go](../../internal/steerline/control_service.go) |
| [conversation_position.go](../../internal/steerline/conversation_position.go) |
| [conversation_replay.go](../../internal/steerline/conversation_replay.go) |
| [conversation_service.go](../../internal/steerline/conversation_service.go) |
| [conversation_store.go](../../internal/steerline/conversation_store.go) |
| [conversation.go](../../internal/steerline/conversation.go) |
| [decision.go](../../internal/steerline/decision.go) |
| [domain.go](../../internal/steerline/domain.go) |
| [execution_backend.go](../../internal/steerline/execution_backend.go) |
| [execution_capability.go](../../internal/steerline/execution_capability.go) |
| [execution_profiles.go](../../internal/steerline/execution_profiles.go) |
| [execution_recovery.go](../../internal/steerline/execution_recovery.go) |
| [execution_store.go](../../internal/steerline/execution_store.go) |
| [journal_platform_linux.go](../../internal/steerline/journal_platform_linux.go) |
| [journal_platform_other.go](../../internal/steerline/journal_platform_other.go) |
| [journal_platform_windows.go](../../internal/steerline/journal_platform_windows.go) |
| [journal_reader.go](../../internal/steerline/journal_reader.go) |
| [launchers.go](../../internal/steerline/launchers.go) |
| [model.go](../../internal/steerline/model.go) |
| [native_authority.go](../../internal/steerline/native_authority.go) |
| [native_batch_command.go](../../internal/steerline/native_batch_command.go) |
| [native_claim_command.go](../../internal/steerline/native_claim_command.go) |
| [native_cleanup_stop.go](../../internal/steerline/native_cleanup_stop.go) |
| [native_cleanup.go](../../internal/steerline/native_cleanup.go) |
| [native_contract_files.go](../../internal/steerline/native_contract_files.go) |
| [native_delivery_command.go](../../internal/steerline/native_delivery_command.go) |
| [native_discard_command.go](../../internal/steerline/native_discard_command.go) |
| [native_dispatch_command.go](../../internal/steerline/native_dispatch_command.go) |
| [native_graph_export.go](../../internal/steerline/native_graph_export.go) |
| [native_graph_projection.go](../../internal/steerline/native_graph_projection.go) |
| [native_graph_query.go](../../internal/steerline/native_graph_query.go) |
| [native_graph_snapshot.go](../../internal/steerline/native_graph_snapshot.go) |
| [native_graph_types.go](../../internal/steerline/native_graph_types.go) |
| [native_project_open.go](../../internal/steerline/native_project_open.go) |
| [native_readiness_query.go](../../internal/steerline/native_readiness_query.go) |
| [native_readiness.go](../../internal/steerline/native_readiness.go) |
| [native_repository_target.go](../../internal/steerline/native_repository_target.go) |
| [native_service.go](../../internal/steerline/native_service.go) |
| [native_state_preflight.go](../../internal/steerline/native_state_preflight.go) |
| [native_task_page_queries.go](../../internal/steerline/native_task_page_queries.go) |
| [native_task_page.go](../../internal/steerline/native_task_page.go) |
| [native_task_predecessors.go](../../internal/steerline/native_task_predecessors.go) |
| [observation.go](../../internal/steerline/observation.go) |
| [operation.go](../../internal/steerline/operation.go) |
| [orca_supervised.go](../../internal/steerline/orca_supervised.go) |
| [orca_version.go](../../internal/steerline/orca_version.go) |
| [orca.go](../../internal/steerline/orca.go) |
| [orcaexec.go](../../internal/steerline/orcaexec.go) |
| [platform_linux.go](../../internal/steerline/platform_linux.go) |
| [platform_windows.go](../../internal/steerline/platform_windows.go) |
| [pr.go](../../internal/steerline/pr.go) |
| [projects.go](../../internal/steerline/projects.go) |
| [query_command.go](../../internal/steerline/query_command.go) |
| [recovery.go](../../internal/steerline/recovery.go) |
| [render.go](../../internal/steerline/render.go) |
| [runtime_view.go](../../internal/steerline/runtime_view.go) |
| [service_client.go](../../internal/steerline/service_client.go) |
| [service_execution_profiles.go](../../internal/steerline/service_execution_profiles.go) |
| [service_workers.go](../../internal/steerline/service_workers.go) |
| [service.go](../../internal/steerline/service.go) |
| [spawner.go](../../internal/steerline/spawner.go) |
| [sqlite_store_read.go](../../internal/steerline/sqlite_store_read.go) |
| [sqlite_store_runtime.go](../../internal/steerline/sqlite_store_runtime.go) |
| [sqlite_store_schema.go](../../internal/steerline/sqlite_store_schema.go) |
| [sqlite_store_write_commands.go](../../internal/steerline/sqlite_store_write_commands.go) |
| [sqlite_store_write_execution.go](../../internal/steerline/sqlite_store_write_execution.go) |
| [sqlite_store_write_native.go](../../internal/steerline/sqlite_store_write_native.go) |
| [sqlite_store_write.go](../../internal/steerline/sqlite_store_write.go) |
| [sqlite_store.go](../../internal/steerline/sqlite_store.go) |
| [state_preflight.go](../../internal/steerline/state_preflight.go) |
| [state_revision.go](../../internal/steerline/state_revision.go) |
| [store.go](../../internal/steerline/store.go) |
| [worker_feedback_phase.go](../../internal/steerline/worker_feedback_phase.go) |
| [worker_flow_adapter.go](../../internal/steerline/worker_flow_adapter.go) |
| [worker_flow_app_server_async.go](../../internal/steerline/worker_flow_app_server_async.go) |
| [worker_flow_app_server_recovery.go](../../internal/steerline/worker_flow_app_server_recovery.go) |
| [worker_flow_app_server_reply.go](../../internal/steerline/worker_flow_app_server_reply.go) |
| [worker_flow_app_server.go](../../internal/steerline/worker_flow_app_server.go) |
| [worker_flow_evidence.go](../../internal/steerline/worker_flow_evidence.go) |
| [worker_flow_orca_native_model.go](../../internal/steerline/worker_flow_orca_native_model.go) |
| [worker_flow_orca_native.go](../../internal/steerline/worker_flow_orca_native.go) |
| [worker_flow_orca_relay.go](../../internal/steerline/worker_flow_orca_relay.go) |
| [worker_flow_orca_terminal.go](../../internal/steerline/worker_flow_orca_terminal.go) |
| [worker_flow_orca_workspace.go](../../internal/steerline/worker_flow_orca_workspace.go) |
| [worker_flow_process.go](../../internal/steerline/worker_flow_process.go) |
| [worker_flow_publish_local.go](../../internal/steerline/worker_flow_publish_local.go) |
| [worker_flow_resume_adapter.go](../../internal/steerline/worker_flow_resume_adapter.go) |
| [worker_flow_service.go](../../internal/steerline/worker_flow_service.go) |
| [worker_flow_state.go](../../internal/steerline/worker_flow_state.go) |
| [workflow_runtime_helpers.go](../../internal/steerline/workflow_runtime_helpers.go) |
| [workspace_dirty.go](../../internal/steerline/workspace_dirty.go) |
| [workspace.go](../../internal/steerline/workspace.go) |

### internal/steerline/codexapp

| 源文件 |
| --- |
| [ask.go](../../internal/steerline/codexapp/ask.go) |
| [client.go](../../internal/steerline/codexapp/client.go) |
| [doc.go](../../internal/steerline/codexapp/doc.go) |
| [executable.go](../../internal/steerline/codexapp/executable.go) |
| [params.go](../../internal/steerline/codexapp/params.go) |
| [transport.go](../../internal/steerline/codexapp/transport.go) |

### internal/steerline/codexworker

| 源文件 |
| --- |
| [ask_journal.go](../../internal/steerline/codexworker/ask_journal.go) |
| [ask.go](../../internal/steerline/codexworker/ask.go) |
| [capability.go](../../internal/steerline/codexworker/capability.go) |
| [connection.go](../../internal/steerline/codexworker/connection.go) |
| [doc.go](../../internal/steerline/codexworker/doc.go) |
| [journal.go](../../internal/steerline/codexworker/journal.go) |
| [launch.go](../../internal/steerline/codexworker/launch.go) |
| [observe.go](../../internal/steerline/codexworker/observe.go) |
| [outcome.go](../../internal/steerline/codexworker/outcome.go) |
| [params.go](../../internal/steerline/codexworker/params.go) |

### internal/steerline/console

| 源文件 |
| --- |
| [activity_source.go](../../internal/steerline/console/activity_source.go) |
| [conversation.go](../../internal/steerline/console/conversation.go) |
| [execution_profile.go](../../internal/steerline/console/execution_profile.go) |
| [model_control.go](../../internal/steerline/console/model_control.go) |
| [model_keys.go](../../internal/steerline/console/model_keys.go) |
| [model_navigation.go](../../internal/steerline/console/model_navigation.go) |
| [model_update_actions.go](../../internal/steerline/console/model_update_actions.go) |
| [model_update_conversation.go](../../internal/steerline/console/model_update_conversation.go) |
| [model_update_project.go](../../internal/steerline/console/model_update_project.go) |
| [model.go](../../internal/steerline/console/model.go) |
| [preferences.go](../../internal/steerline/console/preferences.go) |
| [profile_choice.go](../../internal/steerline/console/profile_choice.go) |
| [source.go](../../internal/steerline/console/source.go) |
| [view.go](../../internal/steerline/console/view.go) |

### internal/steerline/orcaworker

| 源文件 |
| --- |
| [command.go](../../internal/steerline/orcaworker/command.go) |
| [dispatch_submit.go](../../internal/steerline/orcaworker/dispatch_submit.go) |
| [inventory.go](../../internal/steerline/orcaworker/inventory.go) |
| [journal.go](../../internal/steerline/orcaworker/journal.go) |
| [message.go](../../internal/steerline/orcaworker/message.go) |
| [mutation.go](../../internal/steerline/orcaworker/mutation.go) |
| [orchestration.go](../../internal/steerline/orcaworker/orchestration.go) |
| [output.go](../../internal/steerline/orcaworker/output.go) |
| [prompt.go](../../internal/steerline/orcaworker/prompt.go) |
| [pull.go](../../internal/steerline/orcaworker/pull.go) |
| [relay.go](../../internal/steerline/orcaworker/relay.go) |
| [terminal_protocol.go](../../internal/steerline/orcaworker/terminal_protocol.go) |
| [terminal.go](../../internal/steerline/orcaworker/terminal.go) |
| [workspace.go](../../internal/steerline/orcaworker/workspace.go) |

### internal/steerline/remoteprotocol

| 源文件 |
| --- |
| [verify_result.go](../../internal/steerline/remoteprotocol/verify_result.go) |

### internal/steerline/task

| 源文件 |
| --- |
| [batch.go](../../internal/steerline/task/batch.go) |
| [command_plan.go](../../internal/steerline/task/command_plan.go) |
| [contract.go](../../internal/steerline/task/contract.go) |
| [delivery_gate.go](../../internal/steerline/task/delivery_gate.go) |
| [delivery.go](../../internal/steerline/task/delivery.go) |
| [model.go](../../internal/steerline/task/model.go) |
| [predecessors.go](../../internal/steerline/task/predecessors.go) |
| [readiness.go](../../internal/steerline/task/readiness.go) |
| [receipts.go](../../internal/steerline/task/receipts.go) |
| [status.go](../../internal/steerline/task/status.go) |

### internal/steerline/taskgraph

| 源文件 |
| --- |
| [taskgraph.go](../../internal/steerline/taskgraph/taskgraph.go) |

### internal/steerline/taskview

| 源文件 |
| --- |
| [model.go](../../internal/steerline/taskview/model.go) |
| [page.go](../../internal/steerline/taskview/page.go) |
| [project.go](../../internal/steerline/taskview/project.go) |

### internal/steerline/workerflow

| 源文件 |
| --- |
| [mode.go](../../internal/steerline/workerflow/mode.go) |
| [prompt.go](../../internal/steerline/workerflow/prompt.go) |
| [state.go](../../internal/steerline/workerflow/state.go) |

### internal/testfeedback

| 源文件 |
| --- |
| [summary.go](../../internal/testfeedback/summary.go) |
