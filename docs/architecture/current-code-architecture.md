# SteerLine 当前代码架构图

快照日期：2026-10-01。第 1–9 节及源码索引保留 PR #179 的纯投影架构快照，实施基线为 `9e13ac4a5a68682b38cd7189b29f860777316c39`；本次已 rebase 到 `41773e81f7db1a68341bff5dabd3c9baaafe5381`（fetch 后的 `origin/main`），第 10.5 节按最新源码更新已实现项和后续任务，包含 PR #182 的通用执行事件及 PR #183 的配置/原生绑定封套。本文起源于 [PR #153](https://github.com/erranliu/SteerLine/pull/153)，包含 #54–#61 执行隔离、PR #175 清理/恢复修复、PR #178 混合发布方案及 PR #176 会话查询。执行隔离的合入与验收边界见[验收记录](../validation/execution-isolation-cleanup/README.md)。

本文描述上述架构快照及第 10.5 节的最新增量核对：系统边界、实际 Go 包依赖、核心包内部职责、任务执行、人工决策、数据模型、日志与恢复。图中的职责分组不代表已拆成独立包或服务。箭头含义在各图前说明；未来方案与当前可启动路径分开列出，第 10 节给出后续解耦与复用建议。

## 1. 系统与进程总览

实线表示当前调用或数据通路，虚线表示实现保留但当前新执行入口不可用的远程通路。CLI 和控制台可以是不同进程；`run` 在独立服务进程中运行。查询经 Go 读取 SQLite，当前客户端控制协议主要使用文件请求/回执和数据库命令，而不是第二阶段计划中的 HTTP/SSE。

```mermaid
flowchart TB
    Human["用户"] --> CLI["cmd/steerline<br/>CLI / projects / task"]
    Human --> TUI["console 包<br/>Bubble Tea + Lip Gloss"]
    Config["项目配置：执行权限 / 发布责任 / 需求 / provider<br/>全局 execution.json：执行方案"] --> CLI
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
        Remote["orca_remote_worker + Orca CLI<br/>协议、原资源观察与对账实现保留<br/>当前权限 admission 拒绝新执行"]
        Git["Git repository / worktree / branch"]
        PR["publication.Client / HostPublisher<br/>固定 publisher、provider 与 Git 目标"]
        Provider["代码托管平台<br/>PR 与合并证据"]
    end
    Service <--> Local
    Service <--> Orca
    Service -.-> Remote
    Local --> Git
    Orca --> Git
    Remote --> RemoteGit["远端 Git worktree"]
    Service --> Workspace["workspace.Git<br/>本机准备、检查、资源与清理"]
    Workspace --> Git
    PR -->|按固定责任受控发布| Git
    Service --> PR
    PR --> Provider
    Human -->|审阅、合并 PR| Provider
```

**业务权威**是 SQLite 中的持久记录。服务负责运行中的业务推进；CLI 的任务创建、废弃、追加前置等命令也能通过 `CommandService` 写库，但须取得仓库身份锁。客户端提交命令不等于执行成功，agent 输出和日志不等于任务交付。

**生命周期**：Windows 通过可见的 `service-window.bat` 承载服务；控制台退出不会停止服务。服务重启先对账保存的身份、命令和外部状态，再允许新派发。当前代码有 Windows/Linux 平台实现，但可启动的权限组合以第 3.2 节为准，不能从平台文件存在推断生产支持。

源码入口：[main.go](../../cmd/steerline/main.go)、[Service](../../internal/steerline/service.go)、[console/source.go](../../internal/steerline/console/source.go)、[control_protocol.go](../../internal/steerline/control_protocol.go)、[native_project_open.go](../../internal/steerline/native_project_open.go)。

## 2. 实际 Go 包依赖

箭头仅表示非测试 `.go` 源文件中的项目内 `import`，覆盖所有平台源码，不代表运行时调用。包名均对应真实目录。

```mermaid
flowchart LR
    P0["cmd/orca-probe<br/>仅使用标准库"]
    P1["cmd/steerline"]
    P2["cmd/test-feedback"]
    P3["internal/steerline <br/>115 个非测试 Go 文件"]
    P4["codexapp"]
    P5["codexworker"]
    P6["console"]
    P7["contracts"]
    P8["execution"]
    P9["orca_remote_worker"]
    P10["orcaworker"]
    P11["projection"]
    P12["publication"]
    P13["query"]
    P14["sqlite"]
    P15["task"]
    P16["taskgraph"]
    P17["taskview"]
    P18["workerflow"]
    P19["workspace"]
    P20["internal/testfeedback"]

    P1 --> P3
    P1 --> P6
    P2 --> P3
    P2 --> P20
    P3 --> P4
    P3 --> P5
    P3 --> P7
    P3 --> P8
    P3 --> P9
    P3 --> P10
    P3 --> P11
    P3 --> P12
    P3 --> P13
    P3 --> P14
    P3 --> P15
    P3 --> P17
    P3 --> P18
    P3 --> P19
    P5 --> P4
    P5 --> P7
    P5 --> P8
    P6 --> P3
    P6 --> P8
    P7 --> P8
    P7 --> P15
    P7 --> P17
    P9 --> P8
    P9 --> P10
    P10 --> P7
    P10 --> P8
    P11 --> P7
    P11 --> P8
    P11 --> P18
    P12 --> P7
    P12 --> P8
    P13 --> P7
    P13 --> P8
    P13 --> P15
    P13 --> P17
    P14 --> P7
    P14 --> P8
    P14 --> P11
    P14 --> P13
    P14 --> P15
    P15 --> P16
    P17 --> P15
    P17 --> P16
    P18 --> P8
    P18 --> P15
    P19 --> P8
```

| 包 | 当前职责 | 非测试 Go 文件 |
| --- | --- | ---: |
| `cmd/orca-probe` | 只读 Orca 能力探测与脱敏证据采集 | 1 |
| `cmd/steerline` | 产品命令入口、服务启动、控制台和任务命令路由 | 1 |
| `cmd/test-feedback` | 最小/定向/全量测试调度、日志与反馈报告 | 4 |
| `internal/steerline` | 配置与实现组装、调度、业务事务、持久化、查询及平台进程接线 | 115 |
| `codexapp` | app-server WebSocket/JSON-RPC、原生问题与传输；参数与权限决定已移出 | 4 |
| `codexworker` | Codex 配置/命令/权限编译、启动/连接/观察/回答/恢复、活动/会话与原生历史解析 | 25 |
| `console` | TUI 状态、导航、任务列表、会话/正文、方案和决策；渲染与锚点按职责分文件 | 18 |
| `contracts` | 共享任务/执行记录、视图、活动/会话、决策内容及发布冻结 DTO、严格 JSON 解码 | 13 |
| `execution` | 公共身份、输入/回答、事实、回执、能力、工作区、错误、项目权限/发布责任与需求匹配纯值 | 10 |
| `orca_remote_worker` | 远端预检、启动/复用、观察、mailbox、工作区和清理协议；当前策略入口拒绝新执行 | 12 |
| `orcaworker` | Orca CLI/资源/终端/派发/消息/观察/对账、shell 包装、就绪与宿主事实 | 22 |
| `publication` | Git 发布、候选核验与对账、PR/provider、发布责任支持与非秘密目标校验 | 7 |
| `projection` | 从领域快照、checkpoint 和任务权威元数据构建 Runtime/Run；纯选择、状态、决策和 slot 转换，无 I/O | 3 |
| `query` | 按消费者分开的只读接口、领域/会话事件、正文/raw/能力读取、任务分页、DAG 与 Ready 查询规则 | 6 |
| `sqlite` | 领域/会话/能力/命令/任务/DAG/checkpoint 与组合快照的 SQLite 只读适配 | 7 |
| `task` | 任务契约、批次、状态、依赖资格、交付和命令规则 | 10 |
| `taskgraph` | DAG 校验、遍历、前后置关系和图算法 | 1 |
| `taskview` | 任务展示、分页和投影规则 | 3 |
| `workerflow` | 单 worker 的纯业务状态、恢复规则与按发布责任构造固定任务指令 | 3 |
| `workspace` | 本机 Git/worktree 准备、身份检查、精确资源解析与清理；不写业务状态 | 5 |
| `internal/testfeedback` | Go test JSON 结果汇总 | 1 |

合计 **21 个包目录、271 个非测试 Go 文件、50 条项目内 import 边**。统计覆盖 `cmd/` 与 `internal/`；不含 `_test.go`、测试数据、脚本和文档。适配器、工作区与发布实现已经归包，中心通过内置注册集中接线，并保留业务权威与事务。

## 3. 中心、独立实现与当前接线

图中 Service、业务转换、事务和配置组装仍在中心包；execution、workspace、publication 与各 worker 适配器是独立包。实线表示调用或数据流，虚线表示当前新执行入口被拒绝的远程路径。

```mermaid
flowchart TB
    Config["项目配置 + 全局方案<br/>config / execution_profiles"] --> Policy["execution.ExecutionPolicy / PublicationPolicy<br/>codexworker.CompilePolicy / publication.CheckPolicy"]
    Policy --> Registry["execution_registry<br/>backend / agent / protocol 内置注册"]
    Registry --> Requirements["execution_requirements<br/>能力、目标工具和认证引用检查"]
    Requirements --> Service["Service 单中心 actor<br/>调度、决策、操作与恢复"]
    Service --> Native["native_* 业务命令<br/>领取、废弃、DAG 与交付"]
    Native --> Rules["task / taskgraph<br/>纯资格与交付规则"]
    Service --> Flow["worker_start / events / completion / feedback / pr<br/>业务转换与固定记录核验"]
    Flow --> FlowRules["workerflow<br/>业务状态、恢复与按 publisher 构造提示"]
    Flow --> Wiring["worker_execution / codex_worker* / orca_worker*<br/>注册回调与回执持久化"]
    Wiring --> Registry
    Wiring --> Codex["codexworker.Backend<br/>配置、命令、权限、原生操作与证据"]
    Codex --> CodexApp["codexapp<br/>WebSocket / JSON-RPC 客户端与传输"]
    Wiring --> Orca["orcaworker.Client / Backend<br/>CLI、资源、terminal、Dispatch 与消息事实"]
    Wiring -.-> Remote["orca_remote_worker<br/>远端协议与对账实现<br/>CheckPolicy 拒绝新执行"]
    Service --> Environment["worker_environment<br/>准备预算、Ready、blocker、DeferredDecision"]
    Environment --> Orca
    Flow --> Workspace["workspace.Git<br/>prepare / inspect / resources / cleanup"]
    Flow --> Publish["worker_publish_local / worker_pr<br/>发布受理与 PR 关联"]
    Publish --> Publication["publication.HostPublisher / Client<br/>候选核验、commit/push/create、PR/provider 与对账"]
    Publication -->|外部事实返回中心| Publish
    Service --> Recovery["worker_recovery<br/>人工恢复授权与业务代次"]
    Recovery --> Wiring
    Service --> Conversations["conversation_service / replay / codex_conversation<br/>来源绑定、采集协调与游标提交"]
    Conversations --> Mapper["codexworker.ConversationMapper / HistoryClient<br/>原生历史、正文与事件归一"]
    Flow --> Commit["service_commit / worker_commit<br/>声明前值、身份、记录与回执附件"]
    Environment --> Commit
    Native --> Store["SQLiteStore / DomainTx<br/>中心 SQL 写入、schema 与事务"]
    Commit --> Store
    Publish --> Commit
    Conversations --> Store
    Query["query.Service<br/>Workflow / Event / Task / Project Reader"] --> ReadSQL["sqlite.Reader<br/>四类 Reader 与同事务组合读取"]
    ReadSQL --> Projection["projection.Build<br/>领域快照 + checkpoint + 派发元数据<br/>独立 Runtime / Run"]
    Store --> Projection
    CQ["query.ConversationService<br/>Event / Body / Raw / Capability Reader"] --> ReadSQL
```

| 所有者 | 主要源码 | 已落实的边界 |
| --- | --- | --- |
| 公共执行值与小接口 | `execution/` | 不依赖中心、具体平台或 SQL；权限、发布责任与附加需求是项目纯值 |
| 配置、冻结与实现组装 | `config.go`、`execution_profiles.go`、`execution_snapshot.go`、`publication_snapshot.go`、`execution_registry.go`、`execution_requirements.go` | 内置注册集中接线；领取冻结策略、方案与需求证据，准备后固定实际资源 |
| 业务推进与原子提交 | `service.go`、`worker_*.go`、`native_*.go`、`service_commit.go`、`worker_commit.go` | 适配器不能写 Task、Decision、Operation 或 TaskDelivery；意图、回执和业务转换由中心提交 |
| 本机 Git/worktree | `workspace/`、中心 `workspace_config.go` / `workspace.go` | 本机准备、检查、Git 实际目录和清理已归包；中心构造固定意图与工具端口 |
| Git 发布与 PR/provider | `publication/`、`worker_publish_local.go`、`worker_pr.go`、`native_delivery_command.go` | 外部发布与对账归 publication；真实合并、版本重检及唯一 TaskDelivery 仍在中心 |
| Codex 平台实现 | `codexworker/`、`codexapp/`、中心 `codex_worker*.go` / `codex_history_port.go` | 统一 Codex 参数生产者；原生启动、回答、恢复、活动/会话/历史解析归 codexworker，中心注入受管进程端口 |
| 本机 Orca 平台实现 | `orcaworker/`、中心 `orca_worker*.go` / `orca.go` | 原生 CLI、JSON、资源和结算核验归适配器；Codex 命令由中心从唯一生产者传入，Orca 只包装 shell |
| 远程 Orca 平台实现 | `orca_remote_worker/`、`orca_remote_adapter.go`、`orca_remote_service.go` | 独立协议与原资源对账保留；当前缺少策略落实回执，虽已注册，新执行在权限 admission 拒绝 |
| 环境就绪与宿主处理 | `worker_environment.go`、`contracts/environment.go`、`orcaworker/readiness.go` / `environment.go` | 准备轮、Ready、blocker、重查及反馈暂存归中心；工具证据、收据与宿主事实归适配器 |
| 共享会话与决策数据 | `contracts/conversation.go`、`contracts/activity.go`、`contracts/decision_content.go` | 数据类型已归 contracts，原生解释与中心持久化分开 |
| 查询、投影与存储 | `query/`、`sqlite/`、`project_view.go`、`runtime_view.go`、`conversation_store.go` | 领域、会话和组合 ProjectView 读取归 sqlite；Runtime/Run 构建归 projection；写入/checkpoint 恢复仍在中心 |
| 控制台渲染 | `console/view_tasks.go`、`view_conversation.go`、`conversation_anchor.go`、`view_text.go` | 列表、正文、锚点和文本处理已同包拆文件；配置、提交资格和产品状态保留原入口 |

中心保留内置注册、Config/执行配置解析、平台进程承载与原生证据到业务记录的转换。`execution_backend.go` 保留能力证据/执行记录及注册接线；旧的 journal/output 观察已移除，`codex_conversation.go` 仍负责历史绑定与持久化协调。共享公共定义没有业务写入权。

### 3.1 已提交视图与原子写入

箭头表示数据流。写集合和事务执行器仍在中心；纯投影已在 projection 包，sqlite.Reader 组合读取后调用相同投影。工作副本不能作为已提交视图发布；数据库命令与 worker 转换共用数据库，但有各自的事务入口。

```mermaid
flowchart LR
    Actor["Service 私有 domain / runs<br/>worker 转换候选"] --> Declare["service_commit<br/>声明 Before / After、固定身份<br/>operation、decision、event 与附件"]
    Declare --> Writer["worker_commit + SQLiteStore<br/>核验前值 / revision / attempt<br/>同一事务写业务记录与 checkpoint"]
    Writer --> DB[("SQLite 业务权威")]
    Writer --> Committed["Service.committed<br/>成功后更新已提交副本"]
    Committed --> Copy["projection.Build<br/>复制记录与 metadata<br/>构建公开 Runtime / Run"]
    Copy --> Public["Service.State<br/>客户端只读视图"]
    Actor --> Private["projection.Runs → 中心 projectActor<br/>按 run ID 应用到原私有 Run 指针"]
    DB --> Reader["sqlite.Reader<br/>同一读事务：记录 + checkpoint"]
    Reader --> Project["sqlite.ProjectReader + projection.Build<br/>同一已采集快照组合 ProjectView"]
    Project --> Client["QueryService → CLI / TUI"]
    Heartbeat["heartbeat / publishCheckpoint"] -->|只写服务元数据| DB
```

`workerCommit` 附件包含执行绑定和底层操作回执；恢复时退休旧 Attempt、创建新 Attempt 与切换 Execution 当前代次也在同一事务内。提交失败后恢复私有候选，不能发布成功状态或消费成功回执。会话记录及其 source cursor 使用独立的 `CommitConversation` 事务，不能误解为所有日志和正文都与 worker 写集合一起提交。

源码：[service_commit.go](../../internal/steerline/service_commit.go)、[worker_commit.go](../../internal/steerline/worker_commit.go)、[worker_attachment_store.go](../../internal/steerline/worker_attachment_store.go)、[project_view.go](../../internal/steerline/project_view.go)、[conversation_store.go](../../internal/steerline/conversation_store.go)。

### 3.2 项目策略、执行方案与两阶段冻结

箭头表示配置和证据的生成通路。项目定义执行权限和发布责任。固定 host_service/worker 时方案不得覆盖；项目显式选择 execution_profile 时，全局每套方案必须声明自己的 publication_policy。领取仍冻结有效 publisher 和非秘密发布目标，不根据后续配置切换已开始的执行。

```mermaid
flowchart LR
    Project["项目 policy / publication_policy / requirements<br/>repository、Git remote、PR provider"] --> Compile["策略与方案与附加需求匹配<br/>注册核验 / CompilePolicy / CheckPolicy / Match"]
    Profiles["全局执行方案<br/>host、agent、model、effort<br/>显式委托时的发布责任 / 实现选项"] --> Compile
    Compile -->|拒绝不支持的组合| Reject["副作用前拒绝<br/>无自动降级或 publisher 切换"]
    Compile -->|领取事务| Fixed["WorkflowRun.ExecutionProfile v4<br/>策略、publisher、provider、Git 目标<br/>需求及领取证据、实现版本、配置版本和 digest"]
    Fixed --> Prepare["工作区及 native 资源准备"]
    Prepare --> Resources["Execution.PolicyResources v2<br/>实际 workspace / 精确写根 / native 版本<br/>启动命令与设置、资源摘要"]
    Resources --> Worker["原 worker / 原会话<br/>使用冻结策略和资源"]
    Fixed --> Publication["按冻结 publisher 发布及核验<br/>host_service / worker"]
    Worker --> Publication
    Restart["后续轮次 / 重启恢复"] -->|重用原快照与原资源，不从当前配置补造| Fixed
```

当前可验证的权限映射限定在 Codex 适配器：Windows local / Orca 的 workspace 写、精确 Git metadata 写、network=allow、escalation=deny；另有 Windows local 的 read-only + network=deny 分支。其他组合明确拒绝。原生映射固定 `codex-cli 0.159.2`，使用 approval=never、明确写根并排除隐式临时根；network=deny 同时禁止 hosted live web search。local 受限模式缺省 elevated；local workspace-write 的 unelevated 分散写根仍拒绝，Orca 必须显式选择 Windows 实现。版本/help 能力检查不等于实际 worker 工具权限证明。

发布责任覆盖 commit、push 与建 PR 一整组动作。当前 host_service 支持 local Codex + GitHub command provider；worker 支持 Orca + 有 query 能力的 provider，远程配置还须通过独立权限 admission。provider 有 query 不表示可 create，能力不足不改变 publisher。完整非秘密目标随 v4 快照冻结，包括 provider、Git 工具、remote 名与实际 URL、base branch 和认证引用；认证值不进快照。恢复、PR 查询与交付按原配置构造 client，不共享当前服务的可变 PR 配置。

源码：[execution/policy.go](../../internal/steerline/execution/policy.go)、[execution/publication.go](../../internal/steerline/execution/publication.go)、[execution_profiles.go](../../internal/steerline/execution_profiles.go)、[execution_snapshot.go](../../internal/steerline/execution_snapshot.go)、[publication_snapshot.go](../../internal/steerline/publication_snapshot.go)、[codexworker/policy.go](../../internal/steerline/codexworker/policy.go)、[publication/policy.go](../../internal/steerline/publication/policy.go)。

### 3.3 内置注册、附加执行需求与当前阻断

这是解析和执行门槛图。注册表位于中心组装层，回调仍由同一个业务 writer 执行；不是动态插件协议，也没有将 Service 或业务事务移入 execution 公共包。

```mermaid
flowchart TB
    Select["选中执行方案 + 项目策略/附加需求"] --> Registry["builtinExecutionRegistry<br/>backend / agent / protocol 唯一组合<br/>builtin-execution-v1"]
    Registry --> Probe["evaluateExecutionRequirements<br/>注册的能力 probe、目标工具查询与服务认证引用"]
    Probe --> Match["execution.ExecutionRequirements.Match<br/>精确目标、版本、唯一证据<br/>能力须 verified；工具/引用须 present"]
    Match -->|missing / unknown / mismatch| Reject["拒绝新领取 / 新业务操作<br/>不创建替代执行或自动降级"]
    Match -->|通过| Claim["领取事务重读配置文件字节与摘要<br/>保存 v4 原方案及领取时需求证据"]
    Claim --> Binding["实际资源准备<br/>PolicyResources v2 固定原生设置"]
    Binding --> Input["启动、反馈及需检查的新副作用意图"]
    Input --> Recheck["按原需求与原目标重新检查<br/>CurrentRequirementsEvaluation / Error"]
    Recheck -->|满足| Intent["原意图持久化 → 注册操作 → 精确回执提交"]
    Recheck -->|不满足| Block["只更新当前需求阻断<br/>不覆盖领取证据或推断 worker 死亡"]
    Block --> Observe["继续原资源观察与未知结果对账<br/>释放、关闭、ack 等结算不被新增门槛阻断"]
    Observe -->|再次请求新业务输入| Input
```

当前注册三条 Codex 路由：local / local_codex_app_server_v1、Orca / orca_terminal_v2、远程 Orca / orca_supervised。实现 key 与实际 target 分开核验，重复 key、重复 host/agent 路由或缺少必要操作在组装时拒绝；未注册组合明确报 unsupported。原 workerflow/mode.go、SelectMode 和 ByMode 路由已移除，启动、事件、轮次、等待/反馈、恢复、清理经注册接线，PendingIdentity 分别由对应适配器解释。远程路由已注册不代表可启动，原 CheckPolicy 权限门槛仍会拒绝新执行。

execution_requirements 只增加必要条件，不授予权限。能力要求使用精确 backend/protocol/target/agent 及平台/agent 版本匹配的已认证报告；版本 probe 不能制造 verified 能力。当前工具 ID 只有 git、go、python，支持存在或精确版本要求；local 在固定环境中只读查询，Go 使用 GOTOOLCHAIN=local 防止版本查询触发下载。本机 Orca 仅能证明固定宿主 Git，terminal 的 Go/Python 返回 unknown；远程工具查询也返回 unknown，不能用服务 PATH 假定目标工具存在。

当前认证要求仅支持 pr.token_env、site=service，检查原 provider 指定的环境变量是否非空，证据只记录引用与 present/missing/unknown；不证明远端登录或 provider 请求一定成功。权限、发布责任、现有 worker preflight 和 readiness 仍独立成立，空附加需求不会跳过它们。

v4 快照固定 ExecutionRequirements、领取时 RequirementsEvaluation 及摘要、ImplementationVersion，后续使用原需求/目标而不是新配置。Execution 的 CurrentRequirementsEvaluation / Error 表示当前检查及阻断；需求不满足只阻止新输入和相应新副作用，不重写领取证据、原绑定或原生存活事实。已经发出的操作保持原身份只读对账，资源结算和消息 ack 不被新的业务需求门槛卡住。控制台从同一次 ProjectView/Domain 快照展示领取时证据和当前阻断。

源码：[execution_registry.go](../../internal/steerline/execution_registry.go)、[execution_requirements.go](../../internal/steerline/execution_requirements.go)、[execution/requirements.go](../../internal/steerline/execution/requirements.go)、[worker_execution.go](../../internal/steerline/worker_execution.go)、[service_commit.go](../../internal/steerline/service_commit.go)、[console/execution_profile.go](../../internal/steerline/console/execution_profile.go)。

## 4. 任务从创建到交付

这是业务流程图。每次“已请求”“已接受”“已确认”均需对应持久记录或外部证据。当前 worker 可以多轮续聊，不应将历史的 prepare/implement/verify/review 分阶段方案当作生产必经流水线。

```mermaid
flowchart TB
    Batch["CLI task create<br/>固定 Spec/Plan 引用与批次"] --> Validate["校验契约、文档、任务 ID、DAG<br/>CommandService + task/taskgraph"]
    Validate --> Create["事务创建 Task / Contract / Dependency<br/>保存批次与命令回执"]
    Create --> Ready["QueryService 计算 Ready<br/>前置任务具有一致交付证据"]
    Ready --> Start["人工选择执行方案并确认<br/>或自动派发默认方案"]
    Start --> PolicyCheck["注册、权限、发布与附加需求检查<br/>能力不足在副作用前拒绝"]
    PolicyCheck --> Admit["服务重检资格与配置版本<br/>领取事务重检配置文件与快照摘要<br/>冻结 v4 策略、方案、需求证据和发布目标"]
    Admit --> Records["Task in_progress<br/>WorkflowRun / Execution / Attempt / Operation"]
    Records --> Prepare["能力探测、工作区准备<br/>保存意图、身份与 PolicyResources v2"]
    Prepare --> Gate{"Windows 受管 Orca？"}
    Gate -->|是| Preparation["固定准备轮、nonce 与 deadline<br/>探针工具证据 + 收据 + 原生结算"]
    Preparation -->|就绪且无宿主阻断| Worker["固定 Codex session / Orca worker<br/>开始并确认业务轮次"]
    Gate -->|否| Worker
    Worker --> Events["持久化事件、会话、活动<br/>核验轮次及执行证据"]
    Events --> Question{"需要人工决策？"}
    Question -->|是| Decision["Decision → answer_saved<br/>回答送达原执行 / 原会话"]
    Decision --> Worker
    Question -->|否| Publication["按冻结 publication_policy.publisher<br/>host_service：标记 → 中心调用 HostPublisher<br/>worker：核验原 worker 的 PR"]
    Publication --> Wait["绑定准确 PR 身份<br/>waiting_pr_merge"]
    Wait --> Merge["用户审阅并合并<br/>服务轮询 PR provider"]
    Merge --> Delivery["事务重检所选执行与任务版本<br/>保存 TaskDelivery / Task closed<br/>推进 graph revision"]
    Delivery --> Complete["WorkflowRun completed<br/>重新计算下游 Ready"]
    Complete --> Ready
    Prepare -->|结果未知| Blocked["blocked / outcome_unknown<br/>恢复对账或人工处理"]
    Preparation -->|宿主等待或证据不明| HostWait["宿主 Decision<br/>打开原 terminal / 重新检查"]
    HostWait -->|原资源核验| Preparation
    Events -->|身份或证据不明确| Blocked
    Publication -->|发布结果待核实| Blocked
    Blocked -->|核验原身份后续接| Prepare
```

人工派发允许多个不同 Ready 任务并行，每个任务冻结独立执行方案与 worker 身份；同一任务不能重复领取。自动模式一次保持一个活动执行，切换自动模式不会停止已经人工启动的任务。领取、进程开始、轮次结束、PR 合并、任务交付不是同一层状态。

源码：[native_service.go](../../internal/steerline/native_service.go)、[worker_start.go](../../internal/steerline/worker_start.go)、[workerflow/state.go](../../internal/steerline/workerflow/state.go)、[worker_publish_local.go](../../internal/steerline/worker_publish_local.go)、[native_delivery_command.go](../../internal/steerline/native_delivery_command.go)。

### 4.1 环境就绪与停服恢复的边界

Windows 受管 Orca 在领取事务中保存 `Execution.Environment`：固定执行方案摘要、准备轮 ID、nonce 和跨重启的截止时间；资源准备后再固定实际 runtime / terminal / incarnation / worktree 和启动命令。业务输入须同时满足 readiness 已核验、没有宿主 blocker、无需人工恢复。准备轮不计作业务轮，不能用模型的“环境可用”回复代替工具调用证据、原子探针收据与原生成功结算。

宿主阻断使用中心接收的 `HostTarget` 决策；打开 terminal 不等于放行，重查也不能覆盖观察期间出现的新故障。原生问题、普通反馈可以被暂存，保留自己的发送身份与回执。已经绑定 PR 的执行不会因为迟到的 terminal 诊断被退回业务阶段。

停服恢复另有人工门槛：服务重启可以对账原进程与原操作，恢复发起新业务输入须取得当前服务实例下的新决策；旧实例已保存的答案不自动重发。本机原进程及后代确认停止、原 worktree 核验且没有未知操作后，用户可继续原 Codex session 或明确选择新 session；新 Attempt 与旧 Attempt 的退休一并提交。Orca 保留原 runtime / terminal / dispatch 进行核验。

源码：[native_claim_command.go](../../internal/steerline/native_claim_command.go)、[worker_environment.go](../../internal/steerline/worker_environment.go)、[contracts/environment.go](../../internal/steerline/contracts/environment.go)、[orcaworker/readiness.go](../../internal/steerline/orcaworker/readiness.go)、[worker_recovery.go](../../internal/steerline/worker_recovery.go)。

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
    alt 宿主 HostTarget 决策
        S->>W: 打开原 terminal 或核验原环境
        W-->>S: 资源、准备轮与宿主事实
        S->>DB: 保存动作回执及 blocker，满足门槛后才继续
    else Codex 原生 request_user_input
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

源码：[query_command.go](../../internal/steerline/query_command.go)、[command_processor.go](../../internal/steerline/command_processor.go)、[worker_execution.go](../../internal/steerline/worker_execution.go)、[codex_worker_reply.go](../../internal/steerline/codex_worker_reply.go)。

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
        blob fixed_execution_profile_v4
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
        blob current_requirements_evaluation_and_error
        blob policy_resources_v2
        blob environment_preparation_and_blocker
        bool recovery_required
        string current_attempt_id
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

当前 SQLite schema 为 **8**（`sqlite.SchemaVersion`），Runtime / 服务 checkpoint schema 为 **11**（`contracts.WorkerFlowRuntimeSchema`）。v4 执行方案、权限和发布目标保存在 WorkflowRun payload，PolicyResources v2、环境准备、阻断及恢复身份保存在 Execution payload，底层表仍按固定项目和执行身份绑定。数据库使用 WAL、外键、busy timeout 和事务；旧格式直接拒绝，不增加迁移、双写或回退路径。

源码：[domain.go](../../internal/steerline/domain.go)、[task/model.go](../../internal/steerline/task/model.go)、[sqlite_store_schema.go](../../internal/steerline/sqlite_store_schema.go)、[runtime_view.go](../../internal/steerline/runtime_view.go)。

## 7. 输出、会话与恢复读取

箭头表示采集和读取通路。会话补读与业务 worker 事件处理并存；日志只能提供相应来源确认的事实。

```mermaid
flowchart LR
    Codex["Codex app-server 事件"] --> Worker["codexworker journal / observe"]
    Orca["Orca 消息与 terminal 输出"] --> OW["orcaworker orchestration / output / relay"]
    Worker --> Service["Service workerMessages<br/>worker_evidence"]
    OW --> Service
    Service --> Domain[("领域 events<br/>执行 / 决策 / operation")]
    Service --> Activity["ActivityWriter<br/>activity.jsonl"]
    Worker --> Raw["原始 journal"]
    OW --> Raw
    History["codexworker.HistoryClient<br/>原生历史与来源核验"] --> Mapper["codexworker.ConversationMapper<br/>中心 conversation_service / replay 协调"]
    Raw --> Mapper
    Service --> Mapper
    Mapper --> Conversation[("conversation_*<br/>事件 / 正文 / raw / source cursor")]
    Domain --> View["QueryService / Runtime 投影"]
    Conversation --> CQ["query.ConversationService + sqlite.Reader<br/>分页事件 / 正文 / raw / 能力"]
    Activity --> JR["journal_reader<br/>来源身份与游标检查"]
    View --> TUI["console<br/>任务 / 活动 / 会话 / 最终回复"]
    CQ --> TUI
    JR --> TUI
    Conversation --> Replay["服务重启<br/>恢复 mapper 与 source cursor"]
    Replay --> Mapper
```

Service 将私有领域工作副本、已提交快照和私有工作 Run 分开；公开 Runtime/Run 由已提交记录深复制构建，不复用工作 Run 的指针。worker 转换通过 `service_commit.go` / `worker_commit.go` 声明记录前值、操作和事件；业务记录与 checkpoint 在同一事务提交。执行绑定与停止意图/回执也作为明确的附属写集合在该事务中提交。`publishCheckpoint` 与 heartbeat 只写服务快照，不扫描业务字段。手动本地恢复在一笔提交中退休旧 Attempt、创建新 Attempt 并切换 Execution 当前代次。

读取视图在同一事务中组合业务记录和 checkpoint。会话事件及来源游标持久化后可以补读，正文按身份与版本定位；来源没有明确 turn 终态或 final 时，界面显示未识别或能力原因。重启恢复还需核验进程身份、app-server generation、固定工作区、Orca dispatch 和未完成 Operation，不能只从日志推测是否完成。

`QueryService.ReadProjectView` 在一笔 SQLite 读事务内组合任务页、完整 Ready 候选、选中任务（含页外选择）、领域记录和服务 checkpoint，复用现有任务、DAG 与 Runtime 投影规则。TUI 直接消费组合结果，不再分别读取列表与 Ready 后重试版本匹配；服务连接依据同一 Runtime 样本执行独立的进程与配置核验，并按核验时刻重新计算心跳新鲜度，保留原数据库采样时间。分区读取错误或连接身份核验失败仍可保留可用的只读数据，控制和领取继续自行校验。领域事件分页每批最多读取 32 条；SQL 先读取有界元数据，只物化本页剩余字节预算内的正文，超出预算的预读事件和条数探测只返回正文引用。每页保留 1 MiB 正文上限、超大正文引用和按最后实际返回事件推进的游标。

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
| `orca_supervised` | 远程 worker 协议及业务桥接已实现；当前 CheckPolicy 缺少权限落实回执，配置/方案选择和领取均拒绝新执行，远端取消仍不支持 |
| Pi 输出归一代码 | `activity.go` 中存在归一器，不代表当前本机 worker-flow 支持 Pi 执行 |
| `execution` / `workspace` / `publication`、平台适配器、项目策略与需求/注册 | #54–#61 均已落地；注册表是内置中心组装，目标能力/权限 admission 独立核验 |

当前依赖边界：execution 的策略、需求与 Match 只依赖标准库，workspace 只依赖 execution；publication 使用 contracts / execution，workerflow 只使用 task / execution。Codex、Orca 与远程适配器不导入中心或 SQL；中心提供受管进程、命令执行和事务接线。contracts 使用 execution / task / taskview，包含已归包的会话、活动和发布记录；它不是无依赖的纯类型包。Codex app-server 原生参数、可执行文件与 Windows 权限解释已移到 codexworker，codexapp 保留传输与原生问题客户端。

query.Service 的 WorkflowReader、EventReader、TaskReader、ProjectReader 均由 sqlite.Reader 实现；同事务采样后使用 projection.Build 构建组合 Runtime，中心只保留开库与薄接线。产品会话查询由 query.ConversationService 使用四类小 Reader，sqlite.Reader 实现事件、正文、raw 和能力读取；中心 NewConversationQueryService 只负责连接与 Reader 接线。写事务和 checkpoint 恢复仍在中心；纯 Runtime/Run 构建已归 projection。存在类型别名或工厂桥接不代表另一条实现路径。

## 9. 快照验证与维护

本轮基于 `9e13ac4` 提取 projection 纯投影包，核对 271 个生产 Go 文件、21 个真实包目录、50 条项目内 import、schema 与关键路径。10 张 Mermaid 图语法、全量源码索引、真实依赖边与相对链接及 `git diff --check` 通过。

验证：项目 -quick 最小入口通过（rebase 后执行 28.1 秒），已将 projection 加入固定最小包入口；projection/contracts/sqlite 包测试、Runtime 私有/公开隔离、actor 指针复用、固定 WAL 快照、缺失 checkpoint/选择、原生答案校验与提交冲突等定向用例，以及受影响包 go vet 与测试入口自身测试通过。projection 的直接和传递项目依赖限于 contracts、execution、workerflow、task/taskview/taskgraph，无 SQL、平台适配、Service 或配置 I/O。未运行项目全量测试、平台认证或完整视觉渲染验收。

TestProjectViewConnectionUsesSampledRuntime 与 TestProjectViewConnectionRechecksHeartbeatAtUse 在本轮及未修改主干的 Go overlay 验证中均因 PublicationPolicy fixture 的支持组合失败，尚未进入连接断言。本轮保留该基线限制，不修改无关 fixture；固定读事务、元数据版本和数据独立性使用通过的现有及新用例验证。

维护时同步核对配置与策略来源、可启动组合、adapter 与中心的写入边界、两阶段冻结版本、schema、checkpoint、控制协议、外部身份和交付事务。新增生产源码更新全量索引，生产支持范围变化同步图 1、图 4 与第 8 节。

## 10. 后续重构建议与已完成项

本轮源码核对确认 #54–#61 已合入，不能继续将工作区、发布或平台适配迁移列为未实施建议。中心有 115 个生产 Go 文件，剩余集中职责是配置组装、业务状态、事务、查询组装和会话持久化；已归包的实现继续通过固定输入及回执与中心协作。

### 10.1 已完成的执行隔离与控制台拆分

| 范围 | 当前实现 | 主干交付 |
| --- | --- | --- |
| #54 公共执行契约 | execution 身份、事实、操作、资源和小接口 | PR #164 |
| #55 本机工作区 | workspace Git、准备/核验、精确资源与清理 | PR #169 |
| #56 发布及 PR | publication 的 HostPublisher、Client 与 query/create/close 边界 | PR #168 |
| #57 Codex 适配器 | codexworker 配置、统一命令、权限、原生操作、历史与事件 | PR #170 |
| #58 Orca 适配器 | orcaworker / orca_remote_worker 的 CLI、协议与资源对账 | PR #171 |
| #59 项目执行权限 | execution.ExecutionPolicy、CompilePolicy、两阶段冻结与原生设置证据 | PR #172 |
| #60 项目发布责任 | PublicationPolicy、固定 publisher/provider/Git 目标与认证引用 | PR #173 |
| 控制台渲染拆分 | view_tasks、view_conversation、conversation_anchor、view_text 同包分工 | PR #166 |
| #61 执行需求和配置注册 | ExecutionRequirements、内置注册、v4 快照与当前需求阻断 | PR #174 |

执行隔离的配置与注册实现已合入，远程成功准入仍待权限证据；后续重构复用已有策略、需求、内置注册和事务，不重建 backend、readiness 或 publication 引擎。注册回调仍接通原中心协调函数，不拥有第二套业务状态。类型所有者、删除项与原始验证证据见 [#61 验收记录](../validation/execution-isolation-08-resolution/README.md)；该历史验收不是本次重新运行。远端能否 admission 放行取决于具体权限落实证据，不能以完成注册代替能力认证。

PR #175 通过 `orca_execution_operations.go` 将 inbox、结算、原 Dispatch 核验、回复/重查和确认/重查绑定到注册项；本机结果转发/清理使用注册回调。中心仍持有 intent、receipt、Decision、恢复门槛和 TaskDelivery 事务，配置校验与固定身份映射也仍保留平台知识。无人调用的旧恢复/journal 观察和包装入口已删除，仅验收消费的辅助入口移入测试文件。受管 native 重启不再自动清除人工确认，准备完成后也需确认才派发业务；remote 续聊在发送前核对原 incarnation。删除依据与最小验证见[清理记录](../validation/execution-isolation-cleanup/README.md)。

### 10.2 会话只读查询：PR #176 已落地

后端无关查询 DTO、游标筛选与分页返回类型在 contracts/conversation_query.go；query.ConversationService 分别消费 ConversationEventReader、ConversationBodyReader、ConversationRawReader、ConversationCapabilityReader，无 SQLite 具体类型、连接所有权或写接口。事件字节预算、cursor、正文摘要/版本/UTF-8 与 raw 完整性保持原规则，SQL 和默认/显式来源选择归 sqlite/conversation.go。

中心 conversation_query.go 仅保留公开类型别名、cursor 入口和 Reader 组装；控制台与已有采集测试改用构造入口。conversation_store.go 的 7 个写入/checkpoint/mapper 恢复函数保持原实现，CommitConversation 仍原子提交事件、正文版本及 source cursor。没有改 schema、JSON/cursor 格式、原生采集或 worker 状态机，也没有增加第二条查询路径。

最小入口已自动覆盖 query/sqlite 新测试；只读 SQLite 用例验证四类读取、项目/来源隔离与 canonical/raw 选择，独立 Reader 用例覆盖无数据库分页、cursor 和正文/raw 规则。纯 Runtime 投影已在 PR #179 落地；最新批次按第 10.5 节及主包解耦 roadmap 推进。

### 10.3 纯 Runtime 投影已落地，写入接口待后续收窄

projection.Build 从 DomainSnapshot、ServiceCheckpoint 与独立任务权威的派发模式/暂停/版本构建全新 Runtime；它复制嵌套记录和元数据，保留 JSON payload 外的 ControlRevision。projection.Run/Runs 统一处理记录选择、execution 状态、工作区展示、决策绑定、交付与 slot 规则。接受答案和投影读取共用 contracts.ValidateDecisionAnswer 的纯内容校验，授权、版本和持久化仍由中心负责。

Service.projectState、数据库 Runtime 加载、sqlite.Reader.ReadProjectView 使用同一投影器；ProjectReader 不再依赖中心 projectReadRecords。SQL 仍在同一读事务采集记录、任务和 checkpoint，投影不再次开库或读取当前配置，也不从旧客户端 Run 补造缺失记录。中心 projectActor 单独将新的私有 Run 值按 run ID 应用到原指针，保留在途 worker 持有的对象身份；公开 Runtime 不复用这些指针。写集合、业务权威、schema 和序列化格式保持原职责。

CommandService 与 Service 仍依赖具体 SQLiteStore，DomainTx 的任务、决策、操作和 graph 接口较宽。后续按命令提交/接受答案、worker 转换、领取/交付的真实消费者收窄接口，再迁移 SQL 写实现。沿用 workerCommit 的前值和固定身份，业务记录、执行绑定、操作回执与 checkpoint 仍由原事务提交；不为分包增加第二个提交入口或拆开原子写集合。

### 10.4 控制台后续接线与并行边界

任务列表、会话正文、身份 token/锚点和文本处理的同包拆分已经完成。后续可继续拆分 model 和主 view 的活动、决策等局部职责；配置选择、源读取及提交资格应复用已落地的注册与冻结接线，不提前复制一套 UI 框架或状态规则。

会话只读查询和纯投影均已完成；当前已有 #63–#67 的串行主包解耦链路，客户端构造接线由 #66 统一处理。控制台模型的局部整理只有在不修改共享 Config、contracts、Service 或组装文件时才适合独立并行；写接口与 SQL 搬迁不纳入该链路，后续按最新消费者另行规划。

每项实现迁移完整职责、接通真实消费者并删除原实现。最小验收使用既有受影响用例、依赖检查及必要中心接线，不新增旧版兼容、自动回退或全量故障矩阵。本节只更新建议和状态，不创建新任务或改变已有任务契约。

### 10.5 最新主干核对与后续批次（2026-10-01）

本次已 rebase 到 `41773e8`，保存并恢复了原工作区的架构文档修改，按源码与合入记录核对上轮建议。当前源码包含 282 个生产 Go 文件、21 个 Go 包，其中中心直属为 117 个生产文件；第 1–9 节及附录保留 PR #179 的历史快照统计，本节为增量核对。后续五项已按用户确认形成 #68–#72 的独立 Spec/Plan，不改变 #64–#67 的原契约，也不启动实现或创建中心任务。

| 上轮建议或基础能力 | 最新状态 | 实际实现与剩余边界 |
| --- | --- | --- |
| 执行、工作区、发布与平台实现归包 | 已实现 | #54–#61 与 #175 已合入，复用已有实现，不重复搬包 |
| 会话只读查询与小 Reader 接口 | 已实现，PR #176 | query/sqlite 已独立；会话写入和采集仍不在该交付范围 |
| TUI 列表、正文、锚点与文本拆分 | 已实现，PR #166 | 同包分文件已完成；客户端与中心依赖仍待收窄 |
| 纯 Runtime/Run 投影独立包 | 已实现，PR #179 | projection.Build/Run 被服务发布、私有 actor 投影及 sqlite.Reader 组合查询实际消费；中心 runtime_view/project_view 保留薄接线与业务修改 helper，不表示投影规则仍在中心 |
| 通用执行事件与问题边界 | 已实现，#62 / PR #182 | WorkerEvent 使用 execution.EventFact，原生解码和问题路由 codec 归实现；通用问题/答案 DTO、固定身份/版本/摘要封套与提交后确认已接通。不是完整的生命周期或主包隔离 |
| 执行注册与生命周期解耦 | 部分实现 | #175 的操作路由及 #62 的通用事实已交付；executionRegistration 仍有多个 *Service/*Run 回调、平台类型和内置 init 注册，后续由 #64/#66 收口 |
| 配置、冻结和原生绑定所有权 | 已实现，#63 / PR #183 | 配置/资源/问题统一用 ImplementationDataEnvelope，具体配置和绑定 codec 归实现；冻结/持久/查询消费者使用通用封套，生命周期/采集的唯一 codec 临时视图明确交 #64/#65 删除，最终构造交 #66 |
| 客户端依赖收窄 | 部分基础已有，目标未完成 | console 有 QuerySource/ControlClient/DecisionCommandClient，但仍导入中心并开库、解析配置和探测服务；#66 迁移实现构造，完整产品客户端适配边界需后续单独核对 |
| 写入端口与 SQL 实现分离 | 尚未实现 | CommandService/Service 仍依赖具体 SQLiteStore，DomainTx 仍宽；#62–#67 明确不搬迁 SQLite 写实现 |
| 会话采集协调与存储解耦 | 尚未实现，已纳入 #65 | Service 仍持有具体 mapper，RestoreConversationMapper 仍在 SQLiteStore 构造 codexworker.ConversationMapper；query/sqlite 的只读分层不能替代采集隔离 |
| 服务协调与进程承载分离 | 尚未完成，#64/#66 覆盖部分目标 | Service 仍混合业务工作副本、观察器、进程句柄和控制请求；执行副作用/构造外移后，服务自身承载的剩余职责另行评估 |
| 模块维护契约与自动依赖检查 | 规划已形成，守卫尚未实现 | PR #181 已固定共同约束、所有者、迁移归属和 #62–#67 Spec/Plan；#67 的自动守卫、测试归位及第三实现证明仍待交付，主要模块的简短维护说明可逐步补充 |

PR #178 的混合发布责任已由 #63 保留。PR #182 保留 #179 的唯一投影器并修复通用 start 和按 Turn/代次隔离的问题重放；PR #183 更新当前开发格式并明确拒绝旧格式，实施及质量修复记录见[#63 Plan](../spec/core-decoupling-02-bindings.plan.md)。这些实现进展不代表远端权限认证已完成，remote 缺证准入拒绝继续保留。PR #177 的 roadmap-runner 是开发协调技能，不是产品内部规划服务。

最新主干已有[主包与执行实现解耦 roadmap](../roadmap/core-implementation-decoupling.md)及[共同约束](../spec/core-decoupling-constraints.md)，#63 已交付，剩余先按 **#64 → #65 → #66 → #67 → #68** 串行，再分为 **#69 → #70** 和 **#71 → #72** 两线。以下保留执行隔离批次说明；#68–#72 的独立任务范围见[后续维护约束](../spec/core-maintenance-constraints.md)。每项仍一个独立实现 PR，硬前置不因归为同批而解除。

| 批次 | 工作与交付边界 | 依赖与并行安排 |
| --- | --- | --- |
| 第一批：配置与绑定所有权（#63，已交付） | 具体配置/绑定 codec 归实现；中心冻结公共策略、身份和有界实现封套；复用 #62 问题路由，保留两阶段冻结和混合发布责任 | 后继直接消费已交付契约，原临时生命周期/采集视图按 #64/#65 删除 |
| 第二批：通用执行生命周期（#64） | 将准备、启动、观察、反馈、恢复和清理接到值输入及单步 intent/receipt 端口；替换生命周期对可写 Service/Run 和原生结果解释的依赖 | #63 已交付，可作为下一实施项。共享 Service/Operation/事务与恢复路径串行；各实现内部准备与测试仅在明确接口和修改归属后并行，不能提前完整实施后继 |
| 第三批：会话采集与宿主交互（#65） | 来源发现、history/mapper、原生重放及宿主事实归实现；中心保留采集预算、授权、准备状态及内容/checkpoint 原子提交；存储恢复数据与平台 mapper 构造分开 | 等 #64 完整交付。Service、conversation_store、source checkpoint 和 worker_environment 由本项统一集成；不重复搬只读查询，也不同时大搬 SQL 写层 |
| 第四批：外层组装与边界守卫（#66，随后 #67） | #66 将目录/工厂/默认值移至 steerlineapp，CLI/服务/Claim/查询/控制台显式注入同一能力；#67 完成测试归位、自动依赖守卫和第三实现证明 | #66 等 #65；#67 等 #66，二者仍顺序 PR。生产平台依赖由 #66 当场清除，#67 不接收遗留生产搬迁；同包测试随构造变更及时处理 |

当前最合适的起点是 #64。#64–#68 完整实现按既定链路串行，允许提前只读盘点、准备不触及共享契约的 fixture 和审阅接口；不能仅凭文件不重叠判断可并行。contracts、配置快照、注册、Service、schema 和最终组装各自指定修改归属，已交付的 pure 包与只读 Reader 可作为独立维护边界。

用户确认的后续任务已固定为：#68 重构后代码清理；存储线 #69 中心写入端口收窄 → #70 SQLite写入实现归包；客户端线 #71 产品客户端适配层与CLI接入 → #72 TUI与中心实现解耦。两线均硬依赖 #68，按写端口/存储与产品 DTO/客户端的修改归属并行，共享类型形状和组装/验证入口顺序集成。#66 构造外移不自动等于 TUI 已摆脱中心，#72 完成该消费迁移。HTTP/JSON/SSE 和第二阶段规划模块继续按原 Spec 接入，服务自身承载的剩余整理未新增为本次任务。

每项迁移继续接通真实消费者并删除原实现，保留固定身份、人工恢复门槛、公开/私有对象隔离和唯一 TaskDelivery。副作用按 intent 提交后调用、回执提交后下一步、消息内容提交后确认；分包不拆开原子写集合。最小验证复用各 Plan 的定向用例和既有 quick，不增加旧版兼容、第二套执行引擎或全量故障矩阵。

本次只核对源码、合入记录及规划身份/依赖/链接和 diff；PR #179/#182/#183 的代码测试结果是已有验收记录，未在本次重跑代码测试或原生认证。

## 附录：全部非测试 Go 源文件索引

下列索引覆盖 `cmd/` 与 `internal/` 中全部非测试 `.go` 文件，按真实包目录分组。

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
| [codex_conversation.go](../../internal/steerline/codex_conversation.go) |
| [codex_feedback_hooks.go](../../internal/steerline/codex_feedback_hooks.go) |
| [codex_git_sandbox.go](../../internal/steerline/codex_git_sandbox.go) |
| [codex_history_port.go](../../internal/steerline/codex_history_port.go) |
| [codex_worker.go](../../internal/steerline/codex_worker.go) |
| [codex_worker_evidence.go](../../internal/steerline/codex_worker_evidence.go) |
| [codex_worker_recovery.go](../../internal/steerline/codex_worker_recovery.go) |
| [codex_worker_reply.go](../../internal/steerline/codex_worker_reply.go) |
| [command.go](../../internal/steerline/command.go) |
| [command_processor.go](../../internal/steerline/command_processor.go) |
| [config.go](../../internal/steerline/config.go) |
| [control_client.go](../../internal/steerline/control_client.go) |
| [control_protocol.go](../../internal/steerline/control_protocol.go) |
| [control_service.go](../../internal/steerline/control_service.go) |
| [conversation.go](../../internal/steerline/conversation.go) |
| [conversation_position.go](../../internal/steerline/conversation_position.go) |
| [conversation_query.go](../../internal/steerline/conversation_query.go) |
| [conversation_replay.go](../../internal/steerline/conversation_replay.go) |
| [conversation_service.go](../../internal/steerline/conversation_service.go) |
| [conversation_store.go](../../internal/steerline/conversation_store.go) |
| [decision.go](../../internal/steerline/decision.go) |
| [domain.go](../../internal/steerline/domain.go) |
| [execution_backend.go](../../internal/steerline/execution_backend.go) |
| [execution_capability.go](../../internal/steerline/execution_capability.go) |
| [execution_helpers.go](../../internal/steerline/execution_helpers.go) |
| [execution_profiles.go](../../internal/steerline/execution_profiles.go) |
| [execution_registry.go](../../internal/steerline/execution_registry.go) |
| [execution_requirements.go](../../internal/steerline/execution_requirements.go) |
| [execution_snapshot.go](../../internal/steerline/execution_snapshot.go) |
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
| [native_cleanup.go](../../internal/steerline/native_cleanup.go) |
| [native_cleanup_stop.go](../../internal/steerline/native_cleanup_stop.go) |
| [native_contract_files.go](../../internal/steerline/native_contract_files.go) |
| [native_delivery_command.go](../../internal/steerline/native_delivery_command.go) |
| [native_discard_command.go](../../internal/steerline/native_discard_command.go) |
| [native_dispatch_command.go](../../internal/steerline/native_dispatch_command.go) |
| [native_graph_export.go](../../internal/steerline/native_graph_export.go) |
| [native_graph_snapshot.go](../../internal/steerline/native_graph_snapshot.go) |
| [native_graph_types.go](../../internal/steerline/native_graph_types.go) |
| [native_project_open.go](../../internal/steerline/native_project_open.go) |
| [native_readiness.go](../../internal/steerline/native_readiness.go) |
| [native_readiness_query.go](../../internal/steerline/native_readiness_query.go) |
| [native_repository_target.go](../../internal/steerline/native_repository_target.go) |
| [native_service.go](../../internal/steerline/native_service.go) |
| [native_state_preflight.go](../../internal/steerline/native_state_preflight.go) |
| [native_task_page.go](../../internal/steerline/native_task_page.go) |
| [native_task_predecessors.go](../../internal/steerline/native_task_predecessors.go) |
| [observation.go](../../internal/steerline/observation.go) |
| [operation.go](../../internal/steerline/operation.go) |
| [orca.go](../../internal/steerline/orca.go) |
| [orca_execution_operations.go](../../internal/steerline/orca_execution_operations.go) |
| [orca_feedback_hooks.go](../../internal/steerline/orca_feedback_hooks.go) |
| [orca_remote_adapter.go](../../internal/steerline/orca_remote_adapter.go) |
| [orca_remote_service.go](../../internal/steerline/orca_remote_service.go) |
| [orca_worker_dispatch.go](../../internal/steerline/orca_worker_dispatch.go) |
| [orca_worker_evidence.go](../../internal/steerline/orca_worker_evidence.go) |
| [orca_worker_observation.go](../../internal/steerline/orca_worker_observation.go) |
| [orca_worker_prepare.go](../../internal/steerline/orca_worker_prepare.go) |
| [orca_worker_relay.go](../../internal/steerline/orca_worker_relay.go) |
| [orca_worker_terminal.go](../../internal/steerline/orca_worker_terminal.go) |
| [orca_worker_workspace.go](../../internal/steerline/orca_worker_workspace.go) |
| [platform_linux.go](../../internal/steerline/platform_linux.go) |
| [platform_windows.go](../../internal/steerline/platform_windows.go) |
| [pr.go](../../internal/steerline/pr.go) |
| [project_view.go](../../internal/steerline/project_view.go) |
| [projects.go](../../internal/steerline/projects.go) |
| [publication_snapshot.go](../../internal/steerline/publication_snapshot.go) |
| [query_command.go](../../internal/steerline/query_command.go) |
| [recovery.go](../../internal/steerline/recovery.go) |
| [render.go](../../internal/steerline/render.go) |
| [runtime_view.go](../../internal/steerline/runtime_view.go) |
| [service.go](../../internal/steerline/service.go) |
| [service_client.go](../../internal/steerline/service_client.go) |
| [service_commit.go](../../internal/steerline/service_commit.go) |
| [service_execution_profiles.go](../../internal/steerline/service_execution_profiles.go) |
| [service_workers.go](../../internal/steerline/service_workers.go) |
| [spawner.go](../../internal/steerline/spawner.go) |
| [sqlite_store.go](../../internal/steerline/sqlite_store.go) |
| [sqlite_store_checkpoint.go](../../internal/steerline/sqlite_store_checkpoint.go) |
| [sqlite_store_read.go](../../internal/steerline/sqlite_store_read.go) |
| [sqlite_store_schema.go](../../internal/steerline/sqlite_store_schema.go) |
| [sqlite_store_write.go](../../internal/steerline/sqlite_store_write.go) |
| [sqlite_store_write_commands.go](../../internal/steerline/sqlite_store_write_commands.go) |
| [sqlite_store_write_execution.go](../../internal/steerline/sqlite_store_write_execution.go) |
| [sqlite_store_write_native.go](../../internal/steerline/sqlite_store_write_native.go) |
| [state_preflight.go](../../internal/steerline/state_preflight.go) |
| [state_revision.go](../../internal/steerline/state_revision.go) |
| [store.go](../../internal/steerline/store.go) |
| [worker_attachment_store.go](../../internal/steerline/worker_attachment_store.go) |
| [worker_commit.go](../../internal/steerline/worker_commit.go) |
| [worker_completion.go](../../internal/steerline/worker_completion.go) |
| [worker_environment.go](../../internal/steerline/worker_environment.go) |
| [worker_events.go](../../internal/steerline/worker_events.go) |
| [worker_evidence.go](../../internal/steerline/worker_evidence.go) |
| [worker_execution.go](../../internal/steerline/worker_execution.go) |
| [worker_feedback.go](../../internal/steerline/worker_feedback.go) |
| [worker_pr.go](../../internal/steerline/worker_pr.go) |
| [worker_process.go](../../internal/steerline/worker_process.go) |
| [worker_publish_local.go](../../internal/steerline/worker_publish_local.go) |
| [worker_recovery.go](../../internal/steerline/worker_recovery.go) |
| [worker_resume.go](../../internal/steerline/worker_resume.go) |
| [worker_start.go](../../internal/steerline/worker_start.go) |
| [worker_state.go](../../internal/steerline/worker_state.go) |
| [worker_types.go](../../internal/steerline/worker_types.go) |
| [workspace_config.go](../../internal/steerline/workspace_config.go) |

### internal/steerline/codexapp

| 源文件 |
| --- |
| [ask.go](../../internal/steerline/codexapp/ask.go) |
| [client.go](../../internal/steerline/codexapp/client.go) |
| [doc.go](../../internal/steerline/codexapp/doc.go) |
| [transport.go](../../internal/steerline/codexapp/transport.go) |

### internal/steerline/codexworker

| 源文件 |
| --- |
| [activity.go](../../internal/steerline/codexworker/activity.go) |
| [ask.go](../../internal/steerline/codexworker/ask.go) |
| [ask_journal.go](../../internal/steerline/codexworker/ask_journal.go) |
| [backend.go](../../internal/steerline/codexworker/backend.go) |
| [capability.go](../../internal/steerline/codexworker/capability.go) |
| [command.go](../../internal/steerline/codexworker/command.go) |
| [config.go](../../internal/steerline/codexworker/config.go) |
| [connection.go](../../internal/steerline/codexworker/connection.go) |
| [conversation.go](../../internal/steerline/codexworker/conversation.go) |
| [doc.go](../../internal/steerline/codexworker/doc.go) |
| [event_fact.go](../../internal/steerline/codexworker/event_fact.go) |
| [executable.go](../../internal/steerline/codexworker/executable.go) |
| [history.go](../../internal/steerline/codexworker/history.go) |
| [history_binding.go](../../internal/steerline/codexworker/history_binding.go) |
| [intent.go](../../internal/steerline/codexworker/intent.go) |
| [journal.go](../../internal/steerline/codexworker/journal.go) |
| [launch.go](../../internal/steerline/codexworker/launch.go) |
| [native_params.go](../../internal/steerline/codexworker/native_params.go) |
| [observe.go](../../internal/steerline/codexworker/observe.go) |
| [outcome.go](../../internal/steerline/codexworker/outcome.go) |
| [params.go](../../internal/steerline/codexworker/params.go) |
| [permissions.go](../../internal/steerline/codexworker/permissions.go) |
| [policy.go](../../internal/steerline/codexworker/policy.go) |
| [recovery.go](../../internal/steerline/codexworker/recovery.go) |
| [session_launcher.go](../../internal/steerline/codexworker/session_launcher.go) |

### internal/steerline/console

| 源文件 |
| --- |
| [activity_source.go](../../internal/steerline/console/activity_source.go) |
| [conversation.go](../../internal/steerline/console/conversation.go) |
| [conversation_anchor.go](../../internal/steerline/console/conversation_anchor.go) |
| [execution_profile.go](../../internal/steerline/console/execution_profile.go) |
| [model.go](../../internal/steerline/console/model.go) |
| [model_control.go](../../internal/steerline/console/model_control.go) |
| [model_keys.go](../../internal/steerline/console/model_keys.go) |
| [model_navigation.go](../../internal/steerline/console/model_navigation.go) |
| [model_update_actions.go](../../internal/steerline/console/model_update_actions.go) |
| [model_update_conversation.go](../../internal/steerline/console/model_update_conversation.go) |
| [model_update_project.go](../../internal/steerline/console/model_update_project.go) |
| [preferences.go](../../internal/steerline/console/preferences.go) |
| [profile_choice.go](../../internal/steerline/console/profile_choice.go) |
| [source.go](../../internal/steerline/console/source.go) |
| [view.go](../../internal/steerline/console/view.go) |
| [view_conversation.go](../../internal/steerline/console/view_conversation.go) |
| [view_tasks.go](../../internal/steerline/console/view_tasks.go) |
| [view_text.go](../../internal/steerline/console/view_text.go) |

### internal/steerline/contracts

| 源文件 |
| --- |
| [activity.go](../../internal/steerline/contracts/activity.go) |
| [conversation.go](../../internal/steerline/contracts/conversation.go) |
| [conversation_query.go](../../internal/steerline/contracts/conversation_query.go) |
| [decision_content.go](../../internal/steerline/contracts/decision_content.go) |
| [environment.go](../../internal/steerline/contracts/environment.go) |
| [execution.go](../../internal/steerline/contracts/execution.go) |
| [json.go](../../internal/steerline/contracts/json.go) |
| [pr.go](../../internal/steerline/contracts/pr.go) |
| [publication.go](../../internal/steerline/contracts/publication.go) |
| [query.go](../../internal/steerline/contracts/query.go) |
| [records.go](../../internal/steerline/contracts/records.go) |
| [runtime.go](../../internal/steerline/contracts/runtime.go) |
| [snapshot.go](../../internal/steerline/contracts/snapshot.go) |

### internal/steerline/execution

| 源文件 |
| --- |
| [backend.go](../../internal/steerline/execution/backend.go) |
| [capability.go](../../internal/steerline/execution/capability.go) |
| [errors.go](../../internal/steerline/execution/errors.go) |
| [event.go](../../internal/steerline/execution/event.go) |
| [operation.go](../../internal/steerline/execution/operation.go) |
| [policy.go](../../internal/steerline/execution/policy.go) |
| [publication.go](../../internal/steerline/execution/publication.go) |
| [requirements.go](../../internal/steerline/execution/requirements.go) |
| [session.go](../../internal/steerline/execution/session.go) |
| [workspace.go](../../internal/steerline/execution/workspace.go) |

### internal/steerline/orca_remote_worker

| 源文件 |
| --- |
| [backend.go](../../internal/steerline/orca_remote_worker/backend.go) |
| [config.go](../../internal/steerline/orca_remote_worker/config.go) |
| [intent.go](../../internal/steerline/orca_remote_worker/intent.go) |
| [messaging.go](../../internal/steerline/orca_remote_worker/messaging.go) |
| [observation.go](../../internal/steerline/orca_remote_worker/observation.go) |
| [policy.go](../../internal/steerline/orca_remote_worker/policy.go) |
| [preflight.go](../../internal/steerline/orca_remote_worker/preflight.go) |
| [run.go](../../internal/steerline/orca_remote_worker/run.go) |
| [start.go](../../internal/steerline/orca_remote_worker/start.go) |
| [types.go](../../internal/steerline/orca_remote_worker/types.go) |
| [version.go](../../internal/steerline/orca_remote_worker/version.go) |
| [worker.go](../../internal/steerline/orca_remote_worker/worker.go) |

### internal/steerline/orcaworker

| 源文件 |
| --- |
| [backend.go](../../internal/steerline/orcaworker/backend.go) |
| [binding.go](../../internal/steerline/orcaworker/binding.go) |
| [client.go](../../internal/steerline/orcaworker/client.go) |
| [command.go](../../internal/steerline/orcaworker/command.go) |
| [config.go](../../internal/steerline/orcaworker/config.go) |
| [dispatch_submit.go](../../internal/steerline/orcaworker/dispatch_submit.go) |
| [environment.go](../../internal/steerline/orcaworker/environment.go) |
| [event.go](../../internal/steerline/orcaworker/event.go) |
| [events.go](../../internal/steerline/orcaworker/events.go) |
| [intent.go](../../internal/steerline/orcaworker/intent.go) |
| [inventory.go](../../internal/steerline/orcaworker/inventory.go) |
| [message.go](../../internal/steerline/orcaworker/message.go) |
| [message_identity.go](../../internal/steerline/orcaworker/message_identity.go) |
| [mutation.go](../../internal/steerline/orcaworker/mutation.go) |
| [orchestration.go](../../internal/steerline/orcaworker/orchestration.go) |
| [output.go](../../internal/steerline/orcaworker/output.go) |
| [readiness.go](../../internal/steerline/orcaworker/readiness.go) |
| [relay.go](../../internal/steerline/orcaworker/relay.go) |
| [saved_receipt.go](../../internal/steerline/orcaworker/saved_receipt.go) |
| [terminal.go](../../internal/steerline/orcaworker/terminal.go) |
| [terminal_protocol.go](../../internal/steerline/orcaworker/terminal_protocol.go) |
| [workspace.go](../../internal/steerline/orcaworker/workspace.go) |

### internal/steerline/projection

| 源文件 |
| --- |
| [run.go](../../internal/steerline/projection/run.go) |
| [runtime.go](../../internal/steerline/projection/runtime.go) |
| [selectors.go](../../internal/steerline/projection/selectors.go) |

### internal/steerline/publication

| 源文件 |
| --- |
| [command.go](../../internal/steerline/publication/command.go) |
| [gitea.go](../../internal/steerline/publication/gitea.go) |
| [policy.go](../../internal/steerline/publication/policy.go) |
| [pr.go](../../internal/steerline/publication/pr.go) |
| [provider.go](../../internal/steerline/publication/provider.go) |
| [publish.go](../../internal/steerline/publication/publish.go) |
| [request.go](../../internal/steerline/publication/request.go) |

### internal/steerline/query

| 源文件 |
| --- |
| [conversation.go](../../internal/steerline/query/conversation.go) |
| [conversation_ports.go](../../internal/steerline/query/conversation_ports.go) |
| [events.go](../../internal/steerline/query/events.go) |
| [ports.go](../../internal/steerline/query/ports.go) |
| [project.go](../../internal/steerline/query/project.go) |
| [tasks.go](../../internal/steerline/query/tasks.go) |

### internal/steerline/sqlite

| 源文件 |
| --- |
| [checkpoint.go](../../internal/steerline/sqlite/checkpoint.go) |
| [conversation.go](../../internal/steerline/sqlite/conversation.go) |
| [graph.go](../../internal/steerline/sqlite/graph.go) |
| [project.go](../../internal/steerline/sqlite/project.go) |
| [reader.go](../../internal/steerline/sqlite/reader.go) |
| [records.go](../../internal/steerline/sqlite/records.go) |
| [tasks.go](../../internal/steerline/sqlite/tasks.go) |

### internal/steerline/task

| 源文件 |
| --- |
| [batch.go](../../internal/steerline/task/batch.go) |
| [command_plan.go](../../internal/steerline/task/command_plan.go) |
| [contract.go](../../internal/steerline/task/contract.go) |
| [delivery.go](../../internal/steerline/task/delivery.go) |
| [delivery_gate.go](../../internal/steerline/task/delivery_gate.go) |
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
| [prompt.go](../../internal/steerline/workerflow/prompt.go) |
| [recovery.go](../../internal/steerline/workerflow/recovery.go) |
| [state.go](../../internal/steerline/workerflow/state.go) |

### internal/steerline/workspace

| 源文件 |
| --- |
| [cleanup.go](../../internal/steerline/workspace/cleanup.go) |
| [git.go](../../internal/steerline/workspace/git.go) |
| [inspect.go](../../internal/steerline/workspace/inspect.go) |
| [prepare.go](../../internal/steerline/workspace/prepare.go) |
| [resources.go](../../internal/steerline/workspace/resources.go) |

### internal/testfeedback

| 源文件 |
| --- |
| [summary.go](../../internal/testfeedback/summary.go) |


### #67 执行边界与测试所有者

主包直属生产和测试只依赖通用执行、contracts、query/sqlite、workspace/publication 等中心契约；主包没有具体平台名称、配置编译或默认实现工厂。中心测试通过单一中性目录和有界事实端口验证原输入、事务、人工恢复及交付。原生 codec/命令/身份与真实应用接线分别由实现包和 `steerlineapp`/`steerlineintegration` 所有。

`internal/coreboundary` 扫描全部构建标签及直属测试/文件名，并遍历完整源码 import 图与 Windows/Linux 生产依赖；除 Git 元数据外不以目录名排除源码。quick 的中心最小组合、实现代表和外层 CD67 入口均要求真实 JSON run/terminal 命中，拒绝零命中、跳过和重复执行。

未识别的外部宿主身份通过 `LifecycleReceipt.OriginalHost` 固定，仍与合格执行 process 区分；检测 agent 前不授权输入、不重置原准备预算。实现只读检测原宿主，检测成功后提交资格回执，再允许下一步。第三中性实现只证明目录接线、原输入对账与中心唯一交付，不能认证原生权限；remote 缺失权限落实证据时 Claim 仍拒绝。
