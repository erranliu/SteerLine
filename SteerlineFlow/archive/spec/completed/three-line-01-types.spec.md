# Spec：纯类型与契约归属整理

任务 ID：73；任务名称：纯类型与契约归属整理；目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。配套 [Plan](three-line-01-types.plan.md)。一个独立实现 PR 交付。

## 范围与前置

硬前置：#37（已交付候选基础）。把纯类型按真实所有者归到 `internal/task/model`、`internal/execution/model`、`internal/execution/backend`、`internal/product/api` 和 `internal/storage/ports`；合并任务纯 DAG 规则，切换本阶段迁移类型的全部真实调用者。

实际盘点起点：`internal/steerline/task`、`taskgraph`、`workerflow`、`execution`、`contracts`、`clientcontracts`、`persistence`；调用者包括 sqlite、query、三个后端、steerlineapp/client、cmd 与 console。开工时按实际消费者核对路径；前置迁移已发生的部分只承接其唯一实现，不重建。

## 实现要求

1. 任务值、固定契约、DAG 与任务资格归 task/model。现有 StartupBlockers 的 mode/paused/slotBusy、DeliveryGate 以及 TaskDeliveryRecord/Evidence 中的 Workflow/session/PR 明细与核验暂保留唯一旧实现并列为 #74 迁移项，不把执行语义直接搬进任务纯模型。图资格在纯模型中只消费任务级规范交付事实，详细 PR 证据仍由执行所有者承接。

2. 当前 execution 包的后端能力/意图/事实/回执移到 execution/backend，仅依赖标准库；Workflow/Execution/Attempt/Decision 等业务记录与 workerflow 纯状态转换归 execution/model。worker 提示组装及含任务输入的跨领域规则留待执行用例，不令 execution/model 反向依赖 task/model。

3. 先迁移产品独立标量、目标、错误和已有可独立消费的小契约；product/api 不导入内部记录。包含 Domain/Runtime 的现有聚合仅留原所有者供 #77 收窄，不把宽快照改名后塞进 api。storage/ports 不依赖 product/query 或未迁 contracts 聚合；能独立消费的声明/只读纯契约先迁，实际调用者随改。仍含完整连接/Runtime 的组合留唯一旧 persistence 所有者，明确 #79 的拆分责任，不为提前搬完引入别名或全能新接口。

4. 建立文件/类型到唯一所有者的清单，余下 contracts 聚合字段明确由 #74–#80 哪项消除。迁出定义立即删除，不以永久 alias、转发包或新增空接口维持旧路径；未迁定义可在唯一旧包暂存。

5. 候选 Task/领取/WorkflowRun/Execution 关系、精确来源/游标及多候选语义保持；不实现 planning 来源、HTTP/SSE 或新远端准入。未来来源由实际领域扩展，未知来源明确 unsupported。

## 共同执行约束

遵守 [三线解耦共同实现约束](three-line-constraints.spec.md) 的最终依赖矩阵与跨域提交规则；本阶段负责实际启用自己迁移部分的守卫。

保持当前任务/DAG、持续 worker、人工并行/自动派发、原会话反馈、恢复授权、唯一交付和 UI 生命周期。失败、轮次结束、最终回复、交付、任务关闭与资源停止分别表达。具体实现无业务写权，状态仍由唯一业务写者经窄声明提交；同数据库查询保持同读快照。

同仓库、同 go.mod、同 SQLite 和服务部署方式；不引入新进程协议、后端、严格候选名额、评分/融合或规划实现。纯结构迁移保持数据格式；确需调整格式时由唯一所有者统一支持版本与拒绝边界，不新增旧版读取、迁移、双写或回退。各步切换真实消费者/构造/测试并当场删除迁出定义，保持可构建，不留永久兼容门面。

完整实施须硬前置整体交付；只读盘点可提前。共享定义、Service/Config/组装/runner 段先确定一个修改所有者并顺序集成，禁止多条线独立重写共同文件。与 #38/#39 并行使用隔离 worktree/构建/配置/测试资源；#39 记录固定构建，不在运行中替换现场。迁移后按实际接线/行为影响决定证据复用或定向重验，零命中、skip 或 mock 都不能代表本机原生通过；远端流程不纳入一期验收。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TM01.1 | 迁移类型由实际生产入口消费，定义唯一；纯包无主包、SQL、产品实现和 adapter 反向依赖。 |
| TM01.2 | 任务/DAG 与执行身份规则保持；后端接口只依赖标准库，product/api 无 Domain/Runtime 或完整持久记录。 |
| TM01.3 | 全 OS/tag 源码和 Windows/Linux 依赖守卫包含新旧包及禁止依赖反例；迁移清单覆盖未迁 contracts 字段，构建/原最小入口不中断。 |

验证只覆盖本次责任/接线变化，保留既有代表断言与禁止依赖反例，不重建全量故障矩阵。每次记录构建、环境、命令、实际 parent/subtest 命中和证据；证据保存 `docs/validation/three-line-01-types/`。目录/依赖目标只在实际实现交付后更新当前架构页。

## 交接

#74 得到唯一类型与端口及明确残留；#38 可继续做交互，仅共享定义、import 与构造段由本任务顺序集成。
