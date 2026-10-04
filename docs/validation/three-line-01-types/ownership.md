# #73 唯一归属与后继清单

起点：origin/main `95237e6abc76fbee743ebc347aa1ef599a76efab`（含 #37/PR206、文档 PR209）。本工作树只实施 #73；无 #38 分支变更。数据格式、任务领取、固定 base、候选/执行/session 身份、SQLite 事务和未知结果屏障保持。

## 已迁定义和真实消费者

逐声明的旧文件、符号与新文件见 [migration-inventory.json](migration-inventory.json)；当前定义与字段见 [definition-inventory.json](definition-inventory.json)。迁出定义、相关旧类型别名及纯函数转发已删除；旧 execution/taskgraph/workerflow 没有生产定义。backend 的 QuestionRoute 用途别名统一使用唯一 ImplementationDataEnvelope，原用途校验和 JSON 保持。

| 旧所有者 | 唯一定义所有者 | 实际生产消费/构造 |
| --- | --- | --- |
| steerline/execution 全部纯定义；contracts 的宿主实现证据/Command | execution/backend | 三个执行实现、workspace/publication/processhost、目录注册、app.NewService、SQLite 原操作与封套 |
| task 的固定任务/契约、批次/版本/资格；taskgraph | task/model | 原 CommandService 创建/领取/依赖，query/taskview，sqlite，worker prompt，CLI/app |
| contracts 的 Workflow/Execution/Attempt/Decision/Operation、冻结 profile/资源/PR；workerflow state/recovery | execution/model | 原 worker/恢复/交付用例、同快照 projection/query/sqlite、adapter 的纯事实与产品现有视图 |
| workerflow.Prompt | execution/prompt.go | worker_instruction.go 的生产提示构造；接收 task/model 输入，不污染 execution/model |
| clientcontracts 的独立状态标量/输入/错误/游标；ControlReceipt/HostActionTarget | product/api | CLI、console、steerlineclient、app 的局部构造、控制请求/回执与原会话目标 |
| persistence.RecordWrite/Commands/错误；contracts.CommandRecord/DomainEvent | storage/ports | WorkerCommit 的前值声明、CommandService 命令读写、SQLite、客户端命令回执；无 query/Runtime/连接依赖 |

[consumers.json](consumers.json) 枚举全生产与测试 import 的实际路径；app/directory.go、app/client.go、app/app.go（当前目录 steerlineapp）和 cmd 已同次切换，没有默认注册表或新写者。root/sqlite 的迁出类型 alias 和纯转发函数也删除，直接消费唯一所有者。

## 唯一保留和后继

[residual-ownership.json](residual-ownership.json) 覆盖旧 task/contracts/clientcontracts/persistence 每个现存类型、接口方法/聚合字段及无 receiver 规则函数；每个字段明确一个后继 ID。表中定义后继负责删除旧定义，field_successors 区分先行拆分段；没有将宽快照搬入 api/ports。

| 当前唯一保留 | 唯一后继 | 边界/交接 |
| --- | --- | --- |
| task.ProjectTaskAuthority（含 DispatchMode/DispatchPaused）、Snapshot（执行占槽和 PR 交付明细） | #74 | task/model.GraphAuthority/Snapshot 仅取同一读快照的任务身份、图版本和 DeliveryFact。TaskGraph 是旧混合快照到纯事实的有损投影，不核验 PR、不授权写入、不读第二次数据库。 |
| task.StartupBlockers、DeliveryGate/DeliveryPR、TaskDeliveryRecord/Evidence/ValidateDelivery/CompleteDelivery、领取/交付/追加前置复合回执输入 | #74 | 拆调度资格/执行核验与任务结果；真实关闭仍经过原 PR 核验及专用原子交付事务，不依据图事实关闭。 |
| contracts.DomainSnapshot、persistence 全连接/Runtime/WorkerCommit 与事务组合 | #79 | 拆各消费者读/提交端口，保留同快照及前值；WorkerCommit.Checkpoint 字段的宿主解耦由 #80 消除，不能塞进新 ports 聚合。 |
| contracts.NativeTask*/ProjectView、Run、TaskSnapshot、Activity 和 clientcontracts 的宽读取/Resources/Factory/资格展示 | #77 | 保持当前候选 UI/产品语义；收窄为公开摘要后切换 console，另等 #38。Run 的 Completion/BeadsContext 拒绝标记由 #80 清除。 |
| contracts.Runtime 执行记录字段、ConversationEvent 规则 | #76 | actor/执行重建；Runtime 的调度状态字段由 #74 拆，宿主字段由 #80 拆；不在 #73 增建第二状态机。 |
| contracts.ConversationWrite/Checkpoint/MapperSnapshot 与 Conversation*Record | #79 | 持久读取/事件正文游标同事务声明。 |
| contracts 会话来源/历史/mapper/capability 与 ConversationJournalPosition | #78 | 对应执行来源/协议资源小包；不在本轮实现 planning。 |
| contracts.ServiceCheckpoint/ServiceState/ControlOperation/ControlBlocker/Repository、旧拒绝标记和通用 DecodeJSON | #80 | 仅拆宿主/组装剩余，不新增旧格式读取。 |

#74 开始时应据本清单盘点并拆 Service/CommandService/Config 字段、方法和文件：当前共用段包括 service.go、service_commit.go、command.go、native_claim_command.go、native_delivery_command.go、native_task_predecessors.go、native_discard_command.go、worker_state.go、write_ports.go、config.go，以及 steerlineapp/{app,client,directory}.go 和 cmd/test-feedback/scoped.go。#74 指定单一顺序集成所有者；#75/#76/#77 各自所有者文件再分线，不能让 #80 替前置补调用者或编译。

## 守卫与验证

coreboundary 扫描全部 OS/tag 的生产/测试 Go 文件，严格启用五个纯包的直接允许矩阵；Windows/Linux 实际 go list 图使用 Standard 标记检查传递闭包。CD67 保留原负向断言并增加纯包对旧聚合、SQL、adapter/app、helper、外部模块及跨域纯包的禁止反例。新 ports 无旧 query 例外；api 仅标准库。旧 CM69/CM71 按唯一新 owner 接线，不新增实现白名单。

最小执行命令、原/新测试父子命中和断言所有者见 [test-mapping.json](test-mapping.json) 及 [README](README.md)。尚未迁的 console 内部记录/客户端聚合由 #77 消除，混合事务端口由 #79 消除；这些中间职责不被误记为最终矩阵已经完成。

L1 暴露既有 Windows 协议 fixture 在受控子进程取消时遗留 O_EXCL 文件锁。native_lock_windows.go 使用独占/delete-on-close 句柄，native_lock_other.go 保持原非 Windows 行为；TestTM01FixtureLockProcessDeath 保证活进程互斥和终止释放。只修复 testprotocol 的原停止/清理能力，没有改生产调用或原恢复断言；首次失败和修复后最小验证均保留。测试支持目录最终归组仍由 #81 承接。
