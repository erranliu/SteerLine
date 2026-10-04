# #76 消费方接口交接

基线 `c4650056d701a158353836616668bfad25011552`（#74）。本阶段把执行 actor、配置、命令与执行侧纯重建实现归入 `internal/execution`；app/CLI/runner 的唯一构造路径也已切到新所有者。执行实际接口以源码为准，旧 `internal/steerline` 不保留执行定义、别名或转发。

## 任务能力与构造

`execution.TaskWorkPort` 和 `execution.TaskResultPort` 消费 #74/#75 的任务能力。WorkPort 包含 `VerifyContractFiles(context.Context, taskmodel.TaskContract) error`，沿用 #75 的 `task.Service.VerifyContractFiles`；核验仍在领取前执行，并校验固定 repository/remote/branch、完整 commit、regular blob、SHA-256 与验收标题。ResultPort 持有声明、丢弃 intent 与丢弃应用。执行包不导入 task app、SQLite 或具体 adapter。

`steerlineapp.NewCommandService(store, execution.Config)` 构造完整 task 服务并注入执行命令。`steerlineapp.NewService` / `NewServiceWithLifecycleResolver` 组装唯一 `execution.Service`、目录、backend、ProcessHost、任务能力与宿主端口。`execution.NewService` 接收显式能力端口；service 写域状态仍集中在 actor 和原声明事务。`execution.Config` 持有执行冻结/选择规则；宿主残留字段由 #80 继续收口。

根 `steerline` 保留应用宿主、启动/锁/控制文件传输、状态位置检查和通用调度循环。循环只认识 `host.Scheduler[T]`，execution 提供调度步骤；app 通过 HostPort 注入控制传输、诊断、就绪发布和 OS/文件事实。执行端口不携带业务写入权限。

## 归属边界

| 定义/行为 | 当前唯一所有者 | 消费者与证据 |
| --- | --- | --- |
| `execution.Config`、profile、候选、Conversation JournalCursor、worker、Decision、恢复、交付核验与清理 | `internal/execution` | CLI/app/runner 和公共查询；`moved-symbols.json` 与源码映射 |
| 由任务、workflow、execution、attempt、decision、delivery 记录重建 runtime/run/selectors 的纯逻辑 | `internal/execution/reconstruction` | execution actor、SQLite 同快照查询、metadata 查询；只依赖纯 model/backend/contracts，不依赖产品 query、SQL 或 adapter |
| 产品 API DTO 与产品查询 | `internal/product/api` 与 #77 查询所有者 | #76 将原执行 journal 游标移至 execution；没有复制产品投影 |
| 宿主循环与文件/进程现场事实 | `internal/steerline/host`、`internal/execution/process` | 宿主和 execution 的窄端口；coreboundary 扫描所有 tag 和 Windows/Linux 图 |
| `ClaimCommit`、`ResultCommit`、`DiscardCommit` 事务和 schema | `internal/storage/ports` + 唯一 SQLite 实现 | actor 只经窄声明端口写入；app 服务 wiring 验证事务失败回滚 |

旧 `internal/steerline/projection` 已删除。`execution/reconstruction` 接管的是执行记录重建这一组旧纯函数，输入/输出仍为既有内部 domain contracts；它不对外生成产品 DTO。#77 继续拥有产品 DTO 与面向产品的查询收窄。

## 测试归属与接线

原执行包内的白盒断言随实际私有状态所有者迁入 `internal/execution`，包括 MC1–MC9；全量 parent/subtest 映射在 [test-mapping.json](test-mapping.json)，源文件与 SHA 清单在 [file-mapping.json](file-mapping.json)。被 #75 提前迁出的任务 batch parent 单独标为 #75。

`internal/steerlineintegration` 新增真实 app MC 外层断言：`TestMC1/3/5/6/7/8/9` 经 `steerlineapp.NewCommandService`、真实 app actor、公开 runtime/task query 和隔离 SQLite/Git/provider 验证候选身份与宽松五个检查、精确反馈、唯一 provider delivery、放弃/替补、原 intent 与晚到回执。原包内父子断言仍留在 execution。`TestTM04ExecutionWiring` 使用真实 app 构造覆盖 intent/receipt、故障 trigger 回滚、原请求只读重放、app-server 两轮、原生手动恢复和回执 ack；原生细节复用 Windows 受控 fixture，不启用用户原生接受开关。

```
execution -> execution/model, execution/backend, task/model, storage/ports,
             persistence/query contracts, execution/reconstruction
execution -X-> task app, root steerline, product query/projection, SQL, adapters,
              app, console, client, integration, test runner
host -> generic scheduler contract; app binds execution.Service
```

当前映射、测试分组、命令和运行证据见 [ownership.md](ownership.md) 与 [README.md](README.md)。
