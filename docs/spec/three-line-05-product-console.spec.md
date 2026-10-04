# Spec：产品查询与控制台契约收窄

任务 ID：77；任务名称：产品查询与控制台契约收窄；目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。配套 [Plan](three-line-05-product-console.plan.md)。一个独立实现 PR 交付。

## 范围与前置

硬前置：#74、#38。将 query/projection/taskview/activityview 收拢到 `internal/product/query`，唯一客户端归 `internal/product/client`，TUI 归 `internal/console`；以实际页面/命令 DTO 替换公开 Domain/Runtime 和完整持久记录。

实际盘点起点：`internal/steerline/clientcontracts`、`query`、`projection`、`taskview`、`activityview`、`console`，`internal/steerlineclient`，app 产品工厂及 cmd 客户端入口；包含 #38 新增的候选界面与测试。开工时按实际消费者核对路径；前置迁移已发生的部分只承接其唯一实现，不重建。

## 实现要求

1. 等待 #38 交付后迁完整消费者，避免同时重写候选界面。product/api 只定义稳定、可序列化的项目/任务/候选/会话摘要、目标/版本、命令/回执、错误/能力/游标；不含 Config、Go 句柄、Runtime/Domain 或持久记录。

2. 投影基于同一数据库读事务，复用任务/model 的唯一资格规则，不自行计算业务授权；产品不成为写者。Git 文档与 SQL 不伪装全局原子快照。Go client 是 CLI/TUI 共用客户端，未来 C# 经协议消费语义。

3. 候选操作、会话正文/raw、草稿与阅读锚点绑定项目/WorkflowRun/Execution/session；歧义拒绝。blocker/Decision 分开、解除不自动补投，规范来源缺口不从原生日志回填。

4. 保留原 request_id、版本及实例回执，不将 accepted 显示为已送达/已停止；Close/退出只关 UI/client 资源。console 只依赖 product/api 和独立视图状态，不构造 backend 或 SQLite。

5. 不实现 HTTP/SSE、planning 或 WinUI；协议 #45 若并行，其 transport 只消费顺序集成后的 api，迁移对接段由本任务负责；未装配命令/source/domain 返回 unsupported，不创建空未来域。

## 共同执行约束

遵守 [三线解耦共同实现约束](three-line-constraints.spec.md) 的最终依赖矩阵与跨域提交规则；本阶段负责实际启用自己迁移部分的守卫。

保持当前任务/DAG、持续 worker、人工并行/自动派发、原会话反馈、恢复授权、唯一交付和 UI 生命周期。失败、轮次结束、最终回复、交付、任务关闭与资源停止分别表达。具体实现无业务写权，状态仍由唯一业务写者经窄声明提交；同数据库查询保持同读快照。

同仓库、同 go.mod、同 SQLite 和服务部署方式；不引入新进程协议、后端、严格候选名额、评分/融合或规划实现。纯结构迁移保持数据格式；确需调整格式时由唯一所有者统一支持版本与拒绝边界，不新增旧版读取、迁移、双写或回退。各步切换真实消费者/构造/测试并当场删除迁出定义，保持可构建，不留永久兼容门面。

完整实施须硬前置整体交付；只读盘点可提前。共享定义、Service/Config/组装/runner 段先确定一个修改所有者并顺序集成，禁止多条线独立重写共同文件。与 #38/#39 并行使用隔离 worktree/构建/配置/测试资源；#39 记录固定构建，不在运行中替换现场。迁移后按实际接线/行为影响决定证据复用或定向重验，远端权限缺口、零命中、skip 或 mock 都不能代表原生通过。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TM05.1 | 公开产品契约不再含 Domain/Runtime/完整内部记录，CLI/TUI 使用唯一新 client/query；console 全源码与测试闭包符合边界。 |
| TM05.2 | #38 候选选择/反馈/放弃、身份草稿/锚点、blocker 与规范来源保持；同快照、Unicode/游标及原回执保持。 |
| TM05.3 | 实际 app/CLI 构造消费新路径；序列化代表 DTO 保留目标/版本/回执/错误，未知能力明确 unsupported。 |

验证只覆盖本次责任/接线变化，保留既有代表断言与禁止依赖反例，不重建全量故障矩阵。每次记录构建、环境、命令、实际 parent/subtest 命中和证据；证据保存 `docs/validation/three-line-05-product-console/`。目录/依赖目标只在实际实现交付后更新当前架构页。

## 交接

#79 接产品读取需求，#80 接产品工厂；可与 #39 固定构建验收并行，默认构建换入本任务后定向重验改动的产品入口。
