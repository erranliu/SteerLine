# #76 消费方接口交接

基线 main `c4650056d701a158353836616668bfad25011552`（PR214）；本文件记录 #75/#76 的并行接口及唯一接线归属。接口完成实际实现后以本文件同步最终签名，不保留旧包 alias/转发。#75 文件由 #75 修改；共同执行构造段由 #76 顺序集成。

## 消费方端口

`internal/execution` 拥有 `TaskWorkPort` 和 `TaskResultPort`，按协调方 2026-10-04 最新精确交接，保留 #74 现有方法，并为 Work 增加 #75 已发布的文件核验能力：

```go
type TaskWorkPort interface {
    VerifyContractFiles(context.Context, taskmodel.TaskContract) error
    Ready(context.Context, string) ([]taskmodel.TaskRecord, error)
    Fixed(context.Context, string, string, uint64, bool) (taskmodel.FixedWork, error)
    Claim(taskmodel.FixedWork, string, time.Time) (taskmodel.TaskRecord, error)
}
type TaskResultPort interface {
    Declare(taskmodel.TaskRecord, taskmodel.TaskContract, taskmodel.Result, time.Time) (taskmodel.ResultDeclaration, error)
    Intent(taskmodel.DiscardIntent) (taskmodel.DiscardIntent, error)
    Discard(taskmodel.TaskRecord, taskmodel.DiscardIntent, string, []string, time.Time) (taskmodel.TaskRecord, error)
}
```

文件核验沿用 #75 已发布的 `task.Service.VerifyContractFiles(ctx, contract)`，由完整任务服务的 Input 固定 project identity。核验保持原 repository/remote/branch 绑定、完整 canonical commit、精确 regular blob、SHA-256、验收标题断言；执行在原领取前时点调用 `Work.VerifyContractFiles`，并继续实际工作区/启动资格检查。不导入 task 应用、不重复文件核验实现。此方法扩展来自协调方明确指令，替代早期独立 ContractPort 草案。

## 构造与 namespace

已读取 #75 PR215 发布接口；使用其 `NewTaskService(reader, config)` 实际值分别注入 Work/Result 两个消费方端口，文件核验归 WorkPort。执行命令归 `execution.CommandService`，构造实际签名为：

```go
execution.NewCommandService(store persistence.CommandStore, project string,
    work execution.TaskWorkPort, results execution.TaskResultPort) execution.CommandService
```

`execution.Service` 是唯一执行 actor；派发/反馈/Decision/恢复/交付/执行清理在该包。执行 Service 构造也显式注入相同 work/results 任务能力。`Service` 私有结果声明能力不交给产品/后端。

执行配置值与冻结/选择规则归 `execution.Config`；当前公开字段的语义和序列化保持。宿主所需状态目录、Persistence、Heartbeat 等残余组装字段先保留在此唯一值上，#80 再收口；本任务不会把启动/心跳/锁/控制文件传输搬入执行业务。#75 不应消费这个执行配置值，而由其 app 小适配器映射任务需要的固定 project identity、repository target、remote/branch、executable types；Git 文件读取通过任务自有小能力注入，不把执行 backend Command 或配置传入 task 应用。

## #75 专属接线

| 文件/消费者 | 所需任务侧接线 |
| --- | --- |
| `internal/steerlineapp/task_boundary_wiring.go` | 工厂返回具体 task 服务/能力值（例如 `(task.Service, task.Service)`），不要返回旧主包端口或要求 task import execution。提供可结构匹配 VerifyContractFiles 的任务文件核验能力。 |
| `internal/steerlineapp/client_task_commands.go` | 切到 #75 任务命令构造及最小任务输入；不再构造旧主包 NewCommandService。保留 Validate/Create/AddPredecessors 的产品行为。 |
| `cmd/steerline/task_commands.go` | 创建/依赖/文档操作只接 #75 任务入口及最小输入。 |
| `cmd/test-feedback/task_scope.go` | 注册 #75 迁入任务测试的真实包与父/子名称。 |
| `internal/steerline/native_{batch_command,task_predecessors,contract_files}.go`、`task_configuration.go`、`task_record_lookup.go` | 仍由 #75 迁移和删除；#76 不复制这些实现。 |

## #76 公共顺序集成

`execution_service_wiring.go`、`execution_command_wiring.go`、`client_discard_commands.go`、`client_command_wiring.go`、`directory.go`、`cmd/steerline/main.go`、`cmd/test-feedback/scoped.go`、核心守卫及共同存储端口由 #76 接 execution 的新 namespace。其余 app/CLI/集成测试中直接消费执行类型的调用者同次切换。任务核验通过上述 TaskWorkPort 注入。

独立执行迁移先推进；依赖 #75 已编译任务接口的最终构造与 MC 外层 app 验证，等协调安排 #75 主干交付后集成。中间构建缺口不能记为通过，也不以 alias/转发隐藏。最终定向 MC、TM04ExecutionWiring 和一次 L1 的范围/断言仍按原 Plan。

## 已发布 #75 接口的集成差异

读取 PR215 head `69867c5d9ee998aebf8fdbcb6e1c500dac183d09` 的 interface-handoff，并接受协调方明确接线要求后，采用 WorkPort.VerifyContractFiles；旧 Ready/Fixed/Claim/Declare/Intent/Discard 方法形状不变。早期 TaskContractPort 草案已从实现与当前文档删除，没有别名或转发。

共同执行构造改用 `tasks := NewTaskService(store,config)`，将 tasks 分别注入工作/结果两个窄端口。#75 的 app 构造/客户端方法及其 Git adapter 在主干集成时只需同步 `steerline.Config` → `execution.Config`，并给已迁到 execution 的 Runner 显式注入 config.ProcessHost；不改变 task 内部接口/规则。

`Config.ValidateTaskCommandConfiguration()` 与 `Config.TaskRepositoryTarget(ctx)` 的实际规则已随唯一配置/目标规则归 execution，不保留旧 Config alias。State 路径准入仍通过宿主 HostPort.ValidateStateLocation 注入，生产 NewService 仍执行原检查。Config.Resources、Config.ReadRuntime 是注入的 OS 事实/原子文件写与已绑定运行时只读能力，皆 json:"-"，不会改变冻结方案或持久格式。#80 后继收口宿主配置。

执行应用已独立构建；现有纯交付规则和新增全 OS/tag + Windows/Linux 禁止依赖反例通过（independent-core）。这不代表整仓可构建或 MC/TM04/L1 已通过；这些仍等待 #75 主干顺序集成、测试迁移和最终验证。没有修改 #75 专属当前文件，没有替代文件核验实现。
