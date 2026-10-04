# #75 实际归属与消费者

固定依据：three-line-03-task-usecases Spec/Plan 与共同约束；main / #74 基线 `c4650056d701a158353836616668bfad25011552`。执行范围只有 #75。

| 原文件/声明 | 实际新所有者 | 消费者与权限 |
| --- | --- | --- |
| 主包 native_batch_command.go 的 Validate/Create、事务编排/回执重放 | `task/batch.go`、`commands.go`、`validation.go` | app 的 clientTaskCommands → task.Commands；真实产品/CLI 已接通；不 import execution/backend/model/SQL/product/app |
| 主包 native_task_predecessors.go 的 Add/规范请求/图资格与 CAS | `task/predecessors.go`、`task/model/predecessor_command.go` | 产品纯输入在 app 转为任务纯输入；只见 Task/依赖/图版本/命令审计窄事务 |
| 主包 native_contract_files.go 的摘要/验收内容规则 | `task/model/document_content.go` | task 文档能力与暂存执行启动核验均消费唯一纯规则；原规则/拒绝未改变 |
| 创建时固定文档的仓库/契约/引用核验与 Git IO | `task/documents.go` 的 Service.VerifyContractFiles；`steerlineapp/task_documents.go` 注入 RepositoryTarget/ReadCommittedRegularFile | 创建实际消费 Service.VerifyContractFiles；app 独立能力构造与 SHA/symlink 拒绝已实测；领取消费方切换归 #76 |
| 已由 #74 迁入的 Ready/Fixed/Claim/Declare/Intent/Discard | `task/work.go` 与 task/model | 不复制规则；原工作值 task/model.FixedWork、原结果/丢弃声明和执行消费方私有写能力保留 |
| 创建/追加前置原 persistence 回调 | app `task_command_store.go` → task.BatchTx / PredecessorTx | 只投影任务、契约、依赖、命令、事件；不暴露 Query、Workflow、Execution、通用 SQL 或 PutOperation；RecordPredecessorAudit 只映射已接受追加依赖命令，并在同事务写审计 |
| task_boundary_wiring / client_task_commands | 同名任务侧 app 文件 | NewTaskService 是唯一任务输入/文档构造；NewTaskCommands 复用其 Input/Documents；taskBoundary 返回实际 Service 值，保持现有共同调用形状 |
| CLI task_commands.go | 文件/产品 API 形状保持 | 原 CLI → product task adapter → 新 task.Commands；不需要重写 cmd/main 或产品 DTO |
| runner task_scope.go | task/model 与 task 固定包选择保持，更新实际覆盖说明 | L1 已执行迁入测试；shared scoped/coreboundary 未修改 |
| root digest/replay marker 的现存执行调用 | command_digest.go / command_replay.go | cleanup/discard/dispatch 等真实执行审计/回执消费者；不是旧 task 方法转发，#76 迁执行时归位 |
| task_configuration.go / task_record_lookup.go | 当前应用配置输入准入 / 执行只读查找辅助 | 前者只向 app 暴露原配置校验与目标解析，后者原执行消费者保持；不把 Config 传入 task，也不重复挑选规则 |

旧主包创建/追加依赖生产方法与文件已删除；生产主包没有 task 应用 import。原启动核验的剩余调用、精确端口签名及 #76 删除条件见 [interface-handoff](interface-handoff.md)，不由 #75 重写共同执行构造/类型。schema 14、Claims/Results/Discards 专用提交及唯一交付/结果关联不变；无迁移/双写/旧读/回退、新后端或第二写者。

## 测试归属

严格解码/已接受原回执、已交付/丢弃后继拒绝、中文/英文验收标题与固定值/结果断言已在 task；旧/新父子名称和断言所有者逐项见 [test-mapping.json](test-mapping.json)。

主包既有跨执行/SQL 用例测试仍保留在原处；task_commands_fixture_test.go / task_documents_fixture_test.go 仅提供测试端口投影及旧测试调用形状，真实业务调用 task.Commands，不重新实现状态机。主包不能 import app，故这些端口适配仅在 `_test.go` 编译；生产没有这些旧方法/桥。实际 app 适配另由外层 TestTM03TaskWiring 用隔离 Git/SQLite/真实 CLI 验证，包括 audit trigger 失败的原子回滚。#81 后继再做集成测试目录归组，不提前扩大本任务。

边界 source/negative fixtures 和 Windows/Linux 生产闭包原守卫保持，不新增白名单。`task` 的生产直接项目依赖只有 `task/model`；构造端口实际签名没有执行记录/backend/provider。当前全 OS/tag 文件、hash/import 见 [source-index.json](source-index.json)。
