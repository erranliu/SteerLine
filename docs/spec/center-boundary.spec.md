# Spec：中心命令查询与SQLite边界封装

任务 ID：35；任务名称：中心命令查询与SQLite边界封装；目标：`PHASE1-MODULARITY`。配套 [Plan](center-boundary.plan.md)。状态：待实现与验收，一个独立实现 PR 交付。

## 范围与前置

硬前置：#27，传递消费 #26 及按拟定 DAG 交付的 #33/#34。复用[多候选模型](task-candidates.spec.md)、[交付仲裁](candidate-delivery.spec.md)及[模块重构共同约束](modular-refactoring-constraints.md)。交付真实的 center Go 包边界，为 #28 与 #16 提供中心命令/查询；不改变其固定交付范围或增加 API 传输协议。

## 包与状态所有权

建立 `internal/steerline/center`，接收现有 Task/DAG、WorkflowRun/Execution/Attempt、Decision、Command/Operation、TaskDelivery、会话持久化及投影相关的中心事务和存储职责。迁移同一个 SQLite 实现及所有当前表的访问者，不新建数据库或第二个 store。SQL、连接和事务对象保持包内，外部只能提交业务命令、执行事实或读取快照。

center 对根 steerline、Service、console、本机/Orca 具体实现没有 import 依赖。持久记录和公开 DTO 采用必要的值类型；跨边界共用类型由明确拥有者定义，保持原序列化字段及摘要。外部 PR、Git 与执行证据通过小范围接口或值对象输入，具体客户端在启动组装处注入；不把完整 Config/Service 作为依赖。

业务命令与底层存储可以在 center 内先共存；本任务不要求多数据库插件或每表一个 Repository。公开接口不得返回 sql.DB、sql.Tx、SQLiteStore、DomainTx 或可执行任意 SQL 的回调，也不得通过 downcast 获取这些对象。已有 DomainStore/DomainTx 若仍有内部用途可保留为私有实现细节，不能作为调用方绕过事务规则的入口。

## 命令、事务与查询

对外覆盖既有任务创建/补边/重置/丢弃、claim/start_candidate、步骤与执行事实接收、决策回答/恢复、候选发布/交付/结算，以及对应查询。沿用原命令信封、错误码、幂等键和返回内容；新增 Go 封装不是新线协议或兼容层。所有当前 CLI、控制台、Service、会话、适配器回执和离线命令调用点须迁移，不能只封装一个门面后保留原 SQL 旁路。

事务内部仍拥有最终版本/CAS/集合/资格复核。Command/Operation/事件、唯一交付和候选冻结继续按原原子边界提交；网络、进程和文件平台动作仍在事务外。预检取得外部证据后，提交时复核该证据绑定的 Task/候选/发布集合版本，不能拆成多个独立 CRUD 调用。重复请求及已持久化意图对账不因接口迁移重派。

读取任务/候选/决策/事件和会话的结果来自中心一致快照，保留 database_revision、cursor 失效、分页和完整正文读取语义。返回值不引用可变中心对象；投影失败仍只影响读模型，不回滚已提交事实。read-only 打开和离线命令继续遵守当前格式、项目身份、仓库锁及维护 owner，若 #31 已交付须保留全部维护准入。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| CB1 | center 无根 Service/UI/具体后端反向依赖；包外无业务 SQL、SQLite downcast 或裸事务入口，全部原访问者迁移并通过依赖检查 |
| CB2 | 原 CLI/TUI/服务命令与错误码一致；single/multiple、精确候选、决策 applied、PR 唯一交付及历史冲突查询回归通过 |
| CB3 | 真实临时 SQLite 验证领取/候选/回答/交付竞争、事务中断、提交后丢回执与同 request 重放；没有部分写入、重复执行或提前释放后继 |
| CB4 | 原当前格式样本可读且同身份恢复，schema/契约摘要/已有记录不因代码搬迁改变；unknown、closed 后落选待办、历史 PR 观察均保留 |
| CB5 | 查询快照/游标/正文与修改前等价，调用方修改返回值不影响中心；投影失败及重启不丢权威业务事实 |
| CB6 | 已有锁、离线命令与已集成的维护屏障保持；本机与 Orca 的受影响回执/恢复接线在隔离环境复验，向 #28/#16 提供可调用接口和样例 |

证据保存于 `docs/validation/center-boundary/`，执行共同约束规定的回归和原生验证。真实事务测试必须使用 SQLite，mock store 不能代替 CB3/CB4。
