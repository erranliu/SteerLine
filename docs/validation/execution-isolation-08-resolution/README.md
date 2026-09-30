# #61 执行需求与注册验证

基线：`ef52f94`（#59/#60 已合入）；日期：2026-10-01。实现对应 [Spec](../../spec/execution-isolation-08-resolution.spec.md) 和 [Plan](../../spec/execution-isolation-08-resolution.plan.md)。验证是确定性契约、真实 Git fixture 和原协议/Service 回归；没有新启动行为、生产部署、中心建任务、数据库迁移或真机/双机重新认证。

## 配置、注册与所有者

| 项目规则/共同操作 | 原生配置与证据所有者 | 业务写入与投影所有者 |
| --- | --- | --- |
| ExecutionPolicy、PublicationPolicy、ExecutionRequirements 的纯值/校验 | execution；后端权限解释由 codexworker / orca_remote_worker，发布支持矩阵由 publication | 中心组合策略及 Profile，contracts 持有同一冻结记录 |
| 必需能力与附加需求匹配 | 既有认证报告、内容绑定证据和精确目标/agent/协议/版本；注册项只做版本查询，不能升级 verified | 中心在领取前评估，contracts 保存非秘密评估，领取事务复核候选 |
| Git / go / python 版本与引用存在性 | 固定注册检查器；Git 使用冻结命令，local 使用与 worker 相同的认证引用剔除环境及 PATH；native 终端 PATH/remote 缺查询入口则 unknown；pr.token_env 只查冻结 provider 的 service 引用 | 中心保存领取证据与独立证据摘要，准备/反馈/业务轮/宿主发布与恢复的后续阻断事实写入 Execution；不保存秘密/秘密摘要 |
| 工作区创建、核验与资源绑定 | workspace 的 Git/资源实现；remote 保留独立协议目标/资源 | 中心保存意图、原资源与回执附件，沿用唯一 worker commit 事务 |
| 准备、启动、原问题/反馈、事件与结算 | codexworker / orcaworker / orca_remote_worker 的参数、身份、原生报告；PendingIdentity 在对应适配器 | 注册回调接通原 Service 协调器；workerflow 只保留纯状态/提示词；等待与结算根据类型化事实推进 |
| 观察、停止、恢复对账、交付后清理 | 注册项构造对应 backend；Stopper 按小接口能力拒绝；thread/terminal/Dispatch 原身份归实现 | 中心保留人工恢复门槛、代次、原 Operation、原回执和唯一 TaskDelivery |
| commit / push / create / query / close 与 merged 证据 | publication 的原 provider/固定目标，workspace 的冻结 Git | 中心冻结 publisher/provider/remote，保持原交付与并发核验 |
| Workflow/Event/Task/Project 查询及控制台展示 | query 四个只读端口；sqlite.Reader 同事务原始 ProjectRecords | 中心 NewQueryService/projectReadRecords 水合 Domain/checkpoint；UI 只消费同一 ProjectView，写动作仍经原事务 |

内建键：`local/codex/local_codex_app_server_v1`、`orca/codex/orca_terminal_v2`、`orca/codex/orca_supervised`。具体 local 平台/主机、Orca executable/args 或 remote connection/environment 单独绑定。重复键、歧义 host/agent、未知组合和缺操作均拒绝；没有用户插件或动态代码加载。测试 fake 仅替换测试注册项的操作，不增加生产注册或改变状态机。

快照升至 v4，增加需求、评估/独立证据摘要和 `builtin-execution-v1` 实现配置版本。完整配置摘要包括原项目权限/发布/需求、Profile、实现选项及受理配置文件版本。所有新增 slice/指针均独立复制，同时补齐 Profile 列表的发布命令/认证引用复制。实际 PolicyResources/原生身份仍独立核验，格式不变。checkpoint schema 11 不变，v3 或更早 Profile 明确拒绝，不迁移、双写或补造原值。

## 删除与生产调用审查

删除 `workerflow/mode.go`、其仅实现选择的测试和准备/启动/恢复 ByMode switch。原 `worker_mode.go` 改为 `worker_execution.go`，只保留执行接线与启动协调；实现键和固定身份复核在 `execution_registry.go`。三个 PendingIdentity 生产者移至对应实现包。原纯 SelectMode 测试由中心固定协议/身份及注册重复/未知检查覆盖。

工作区准备通过注册请求/准备回调，删除原 remote 布尔选择；准备/业务轮、事件解码、等待/反馈、轮次结算、恢复轮询与已完成资源清理通过注册操作选择。移除业务事件/反馈/结算中的后端枚举和 LocalCodex 标记；命令关联使用原 CommandID，等待/结算由注册项返回事实或接通原事务协调。原生恢复代次/收据核验仍在对应中心协调函数，原生 ID 校验和现场解释委托现有适配器；没有新增 host/backend 业务分支。

完整传递依赖与直接 import 核对：execution 仅标准库；contracts → execution/纯任务 DTO；workerflow → execution/纯 task/taskgraph；query → contracts/纯任务规则；sqlite → contracts/query/纯任务规则。平台包不导入中心。没有 query 写事务、SQLiteStore 下转或另读 SQL；四读端口、同事务水合、scope/cursor、部分错误和正文预算均保持原实现。

## 已通过

- ER1：execution requirements 表驱动覆盖目标/agent/协议/版本错配、未核验/unsupported、工具版本/missing/unknown、引用 site/missing/unknown、未知/重复 ID。中心验证三个工具版本归一、缺解析器/错误输出、无 token_env 不冒用宿主登录、证据无秘密；空需求保留 publisher/CLI/remote 权限门槛；剔除 worker PATH 的引用不能再用服务 PATH 证明 go/python 存在。
- ER2：注册重复/未知与测试操作替换；`go list -deps` 及直接 import 检查；两个 query 端口用例、console `TestFileQuerySourceReadsCombinedProjectView` 通过。控制台使用同一 ProjectView 展示原/当前需求证据、时间和来源，权限就绪另列。
- ER3：Profile 上下文两用例、两执行独立 Profile 冻结/恢复、漂移先拒绝、同事务项目视图、快照复制/原值恢复、真实 Claim 竞争通过。新增用例在工具检查期间修改项目配置文件，领取事务以 stale_profile 拒绝且 Task 仍 open；重新核验后可领取，重启后 token 消失新增持久 blocked 事实，原快照字节不变。
- ER4 已通过部分：`TestAppServerServiceTwoTurnsPersistNativeReceipts` 的同步/service_messages 两种路径；`TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal`；固定原协议/事件事实与 `TestRemotePublicationClaimRequiresPermissionEvidence`。remote 协议包在 quick 中通过，远程取消保持 unsupported。
- `go test ./internal/steerline/execution ./internal/steerline/workerflow -count=1` 通过。
- 最终 `go run ./cmd/test-feedback -quick` 通过（执行 21.2s），execution 已纳入 quick，contracts/query/sqlite/远程协议包及两项核心闭环均覆盖；未运行全量或并行全量。

原 context/mode 的单元 fixture 缺少当前冻结配置或使用已拒绝 args，本轮补当前完整值与允许的 args。local 两轮 fixture 改为使用领取时冻结配置绑定资源和真实 Git worktree，保留同步/异步两轮、回执与原会话断言。console 只读 fixture 补显式 publisher/provider，删除已禁用的全局 sandbox 字段，Git/PR 仓库身份一致；查询断言未放宽。

## 原 ER4 未通过项

Plan 原九项中心命令真实执行并命中所有目标，远程两项仍失败：

- `TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery`
- `TestRemoteOrcaLostStartReconcilesAfterRestartWithoutResend`

两项均在 ClaimNativeTask 静态权限检查返回 `requested execution_policy ... lacks remote worker-start enforcement evidence`。这与 [#60 验收记录](../execution-isolation-07-publish-policy/README.md) 已记录的 #59 门槛相同；本轮没有放松它，也没有修改/删除原两轮和丢失回执成功测试。真实 Claim 拒绝行为与纯远程协议测试通过，不能代称这两个 Service 成功场景通过，原 ER4 的三链路全成功条件尚未满足。

Spec 明确本轮不替前置补权限参数、冻结或重新认证全部原生能力，因此后续需要单独补齐原生权限证据并处理这一验收冲突。本轮没有把 mock 视为双机、原生权限、登录或远端授权认证。
