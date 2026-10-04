# #75 → #76 精确接口交接

基线为 main `c4650056d701a158353836616668bfad25011552`（#74 / PR214）。#75 只修改任务所有者文件；本分支在共同执行接线保持该基线时可构建。以下是已实现、真实测试消费的接口，不是接口草案或旧包 alias。

## 任务能力

`internal/task.Service` 的字段为 `Reader task.Reader`、`Input task.Input`、`Documents task.Documents`。没有完整 Config/Runtime、执行模型、后端、provider 或存储写句柄。

```go
func (s task.Service) Ready(context.Context, string) ([]taskmodel.TaskRecord, error)
func (s task.Service) Fixed(context.Context, string, string, uint64, bool) (taskmodel.FixedWork, error)
func (s task.Service) Claim(taskmodel.FixedWork, string, time.Time) (taskmodel.TaskRecord, error)
func (s task.Service) Declare(taskmodel.TaskRecord, taskmodel.TaskContract, taskmodel.Result, time.Time) (taskmodel.ResultDeclaration, error)
func (s task.Service) Intent(taskmodel.DiscardIntent) (taskmodel.DiscardIntent, error)
func (s task.Service) Discard(taskmodel.TaskRecord, taskmodel.DiscardIntent, string, []string, time.Time) (taskmodel.TaskRecord, error)
func (s task.Service) VerifyContractFiles(context.Context, taskmodel.TaskContract) error
```

新增最后一个方法只验证固定文档：规范完整 commit、精确 path、唯一 regular blob（100644/100755）、SHA-256、Spec/Plan/共享引用、验收标题及固定 repository/remote/branch。验证器不领取、不查询候选、不访问 PR、不冻结执行配置，也不替代执行工作区/启动资格检查。文档端口缺失明确失败；没有不执行验证的默认实现。

`task.Documents` 精确签名：

```go
RepositoryTarget(context.Context) (string, error)
ReadCommittedRegularFile(context.Context, taskmodel.FileReference) ([]byte, error)
```

`task.Input` 只含 Project/Authority/Repository/Remote/Branch、ExecutableTypes 和 DefaultRequestedBy；Repository 为读取时解析的目标。摘要与验收内容纯规则唯一归 `task/model/document_content.go`。任务声明仍由 #74 专用原子提交消费，接口没有给任务执行写权。

## 已发布 app 构造

```go
func steerlineapp.NewTaskService(task.Reader, steerline.Config) task.Service
func steerlineapp.NewTaskCommands(persistence.CommandStore, steerline.Config) task.Commands
```

当前 `taskBoundary(reader task.Reader) (task.Service, task.Service)` 返回实际值，不再引用消费方 TaskWorkPort/TaskResultPort 的包路径；两个既有 main 调用形状不变。这两个既有消费者目前只使用 Ready/Fixed/Claim 和声明能力。新的完整构造 `NewTaskService` 在 `TestTM03TaskWiring` 中被实际消费，并通过固定文件成功、坏 digest、regular-file 拒绝验证。

`NewTaskService` / `NewTaskCommands` 仅在 app 层读取当前主干 Config，注入 task.Input 与文档适配器；任务包不见该 Config。配置转化集中在任务侧独立文件，没有猜测 #76 尚未集成的新 Config 类型。创建/追加依赖的应用配置准入仍在 app 的命令适配器；读取/重放时点保持原行为。

## #76 单点集成的最小差异

1. 在新的执行消费方工作端口增加 `VerifyContractFiles(context.Context, taskmodel.TaskContract) error`；执行只引用 task/model 与自己定义的接口，不 import task 应用。领取中原 `VerifyTaskContractFiles(ctx, config, contract)` 调用改成注入工作端口的 `VerifyContractFiles(ctx, contract)`，保持原调用时点与随后实际工作区/启动检查。
2. 在拥有的 `execution_command_wiring.go` / `execution_service_wiring.go` 的两个构造处，以 `tasks := NewTaskService(storeOrConnection, config)` 替换 `work, results := taskBoundary(...)`，分别将 tasks 注入工作/结果端口。结果能力只注入执行，`clientTaskCommands` 仅使用 task.Commands。
3. 两个消费者已切换后删除无消费者的 `taskBoundary`；同时删除 `internal/steerline/native_contract_files.go` 内旧启动文档核验函数与 Git reader。当前仅领取及对应旧启动测试仍消费它，#75 没有用转发/alias 掩盖共同调用者尚未切换。任务创建/验证已经完全不走该主包函数。固定文档内容规则已共享唯一 model 实现。
4. #76 迁 Config/应用配置准入时，同步任务侧构造所需最小读取：Identity、TaskAuthority、Repository/Remote/Branch、Git、CommandTimeout、ExecutableTypes；以及本次在 #75 拥有的 task_configuration.go 提供的 `Config.TaskRepositoryTarget(context.Context) (string,error)` 与 `Config.ValidateTaskCommandConfiguration() error`。前者沿用现有 publication target/非秘密/PR配置一致性检查，后者沿用原 validateWorkflowConfig。可以把这些提取到 app 的新唯一配置所有者，但不能丢原准入检查或把完整执行 Config 传入 task。
5. `command_digest.go` 与 `command_replay.go` 只保留当前执行 cleanup/discard/dispatch 等真实调用者所需的摘要/abort replay marker；#76 迁执行调用者后同次归位/删除。它们不转发 task 用例，replay marker 不提供任务写能力。
6. 一并同步执行测试的实际任务能力注入。当前旧 `task.Service{Reader: store}` 只满足已有读/声明消费；新领取消费 VerifyContractFiles 后，真实 app/外层测试用 `NewTaskService(store, config)`。主包同包测试不能反向 import app，现有 task_commands_fixture_test.go 的测试专用 `NewTaskCommands(store, config)` 已提供 Input/Documents；测试工作服务应显式使用这些字段加原 Reader，更新相关测试构造的 Config 输入。若 #76 迁 CommandService/Config 的定义与执行测试包，同步这些 `_test.go` 接收者和输入到其真实定义所有者，不保留旧生产类型 alias、不给缺文档的 Service 默认放行、也不删原固定文件断言。task.Documents 的 regular-blob IO 适配仍由本任务发布的独立任务侧文件承接。

`client_task_commands.go`、`task_boundary_wiring.go` 与任务文档/存储适配文件由 #75 发布；#76 只为实际新公共类型/消费者顺序集成上面的必要差异。主 cmd/main、runner scoped、共同存储端口和 Service/CommandService 定义在本分支未改。没有遗留当前编译修复，也不将构造修复留到 #80。

## 验证与后继

`TestTM03TaskWiring` 消费真实 app/product/CLI、固定 Git blob、SQLite 和新文件核验能力；包括创建重放、追加依赖审计失败回滚、同一原请求 CLI 重试/重放、SHA-256 拒绝与 committed symlink 拒绝。完整证据由本目录 README 定位。#76 集成公共调用者后应按本目录 Plan 重跑任务/wiring 定向；若其执行 actor/恢复行为确有变化，按 #76 自己 Plan 选定恢复场景。

#79 继续迁当前 persistence/SQL 目录并保持 task-only 事务适配；#80 接剩余 app/配置宿主。#77/#79/#80/#81/#82 仍依用户指令暂停。本任务不启动其它聊天、不合并 PR、不使用生产业务服务/中心任务库。
