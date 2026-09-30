# Spec 公共执行契约归包

任务 ID：54；任务名称：公共执行契约归包；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-01-contracts.plan.md)。无本系列未完成任务硬前置，一个实现 PR 交付。

## 目标与范围

将现有共同请求、身份、事实、回执、能力和错误移入 `internal/steerline/execution`，使后端包能使用同一边界而不导入父包。遵守[共同约束](execution-isolation-constraints.md)，承接当前已有 worker 事实及显式提交，不重写协议或事务框架。

`execution_backend.go` 已拆成 ExecutionProbe、ExecutionBackend 和 ExecutionStopper，但部分回执/操作仍含 `Run`、`Worker`、`ProcessIdentity` 等中心类型。`worker_evidence.go` 是中心事实桥，原生解析已有 `codexworker/event_fact.go`、`orcaworker/event.go` 等唯一实现。本任务迁移共同值和必要接口，不把平台原生 payload 一起提升为通用模型，也不重建已拆好的接口。

## 契约与所有者

| 文件 | 必须交付的共同契约 |
| --- | --- |
| `backend.go` | 身份、能力查询及当前消费者所需的小接口；资源和观察句柄不暴露中心 Worker |
| `session.go` | 固定执行上下文、逻辑 Session/Turn、输入和回答；逻辑 ID 与 native ID 分开 |
| `event.go` | 输入接受、开始、问题、回答回执、终态及观察缺口；来源、原始证据引用与通用 PR 线索提取 |
| `operation.go` | 操作身份、请求摘要、accepted/rejected/confirmed/outcome_unknown 回执和固定启动命令 |
| `workspace.go` | 逻辑仓库、base、分支、目标位置、实际绑定和资源解析结果 |
| `capability.go`、`errors.go` | 当前能力状态和错误分类，保留 unsupported/not_tested/verified 与错误链语义 |

固定上下文包含项目、任务、WorkflowRun、Execution、当前代次/轮次及目标所需身份，但不携带整份业务记录。native thread/Dispatch 等关联由平台核验后作为有来源的绑定事实提供；共同问题结构表达正文、选项、自由文本许可和回答路由引用，不要求中心解码 Codex 原生问题。

工作区契约只描述固定输入及核验结果，不实施 Git 操作。发布关联只固定执行、分支、轮次与证据引用；PR/provider 类型留给 #56。启动命令为固定 executable、argv 和工作目录，shell 包装归执行实现。后继 #55/#56 使用这些类型，#57/#58 接通生命周期操作。

## 迁移边界

更新所有生产与测试引用，删除原共同定义，保留必要的中心记录到公共请求的转换。数据库记录、业务 OperationRecord 和 TaskDelivery 不因名字相似整体搬包，现有串行 mode 选择暂留当前所有者，#61 完成最终注册。

当前层仍可调用唯一原适配入口并转换成新契约；这是尚未迁移的当前接线，不允许增加并行旧/新实现。迁移 `codexworker.PRClues` 的通用部分到公共纯 helper，两类平台保留自己的消息/终态提取，避免 #58 为了 PR 线索导入 Codex 包。权限值、配置格式、发布责任、恢复门槛、平台能力和消息确认顺序保持不变。不能只定义无消费者的新接口。

## 前置与交接

本任务接收主干已有事实/事务边界，无新增全局事务整改前置。中心的 `codex_worker_evidence.go`、`orca_worker_evidence.go` 只做固定身份与业务事实转换；本轮保留当前必要接线供 #57/#58 完成包内迁移。迁移公共身份时保留现有 execution_bindings/execution_operations 的格式和 `worker_attachment_store.go` 原子附件提交，不退回文件写绑定。

#55 与 #56 在本任务合入后可并行；公共契约变更由本任务的固定定义统一扩展，不能各自复制。#57/#58 依照此处启动命令、事件与回执契约迁移，不建立第二套会话接口。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| EC1 | 公共包没有中心、SQL、具体后端或 I/O 依赖；共同类型只有一份且生产消费者已更新 |
| EC2 | 现有固定目标、能力与操作记录用例通过；错误身份、记录损坏和 unsupported 仍拒绝 |
| EC3 | 一个 Codex 与一个 Orca 事实 fixture 保留开始/问题/终态来源，输入接受不误判为开始或成功 |

复用现有能力、绑定和 worker 事实测试，不要求真机或完整故障矩阵。PR 附类型所有者及剩余接线清单，说明后继消费者。
