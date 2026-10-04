# Spec：应用组装与服务宿主收口

任务 ID：80；任务名称：应用组装与服务宿主收口；目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。配套 [Plan](three-line-08-app-host.plan.md)。一个独立实现 PR 交付。

## 范围与前置

硬前置：#75、#76、#77、#78、#79。将配置读取/构造注入归 `internal/app`，服务循环、控制传输/路由、心跳、锁和进程承载归 `internal/app/host`，切换 cmd 并删除旧 steerline 主包及 contracts 聚合。

实际盘点起点：`internal/steerlineapp` 与主包剩余 `service.go`、`control_service.go`、`control_protocol.go`、`control_client.go`、`config.go`、安装/launcher/平台入口、`contracts` 未迁项；`cmd/steerline` 和相关构建/安装脚本。开工时按实际消费者核对路径；前置迁移已发生的部分只承接其唯一实现，不重建。

## 实现要求

1. app 构造具体 adapter、sqlite、任务/执行/产品客户端和小端口；不决定任务资格/交付/恢复。host 只依赖注入小接口，不直接 import 具体后端/SQL 或把业务状态机搬到服务循环。

2. 服务循环驱动 execution 已拥有的 actor/单步调度端口，Task/候选状态转换仍归业务所有者；控制传输只解码、路由精确目标/原请求并返回实际回执。心跳只写元数据，停止中心不等于停止 worker。

3. Config 按 app 输入、task 最小输入、execution 冻结输入和 host 运行输入分配；实现专有配置由 app/adapter 唯一拥有。固定服务/进程/版本/实例身份与恢复授权不因搬宿主放松。

4. 未迁 contracts 字段逐个归 task/model、execution/model/backend、product/api、storage/ports 或 host 自有类型；全消费者切换后删除原主包/contracts，不留下永久别名门面或通用全能 Service。

5. 保留受控规划领域与产品 transport 的显式注入位置，不创建尚无消费者的 planning/HTTP/SSE 空实现。#40/#41 或 #45/#47 并行时只在实际接线段协调，不给其添加全面重构硬前置。

## 共同执行约束

遵守 [三线解耦共同实现约束](three-line-constraints.spec.md) 的最终依赖矩阵与跨域提交规则；本阶段负责实际启用自己迁移部分的守卫。

保持当前任务/DAG、持续 worker、人工并行/自动派发、原会话反馈、恢复授权、唯一交付和 UI 生命周期。失败、轮次结束、最终回复、交付、任务关闭与资源停止分别表达。具体实现无业务写权，状态仍由唯一业务写者经窄声明提交；同数据库查询保持同读快照。

同仓库、同 go.mod、同 SQLite 和服务部署方式；不引入新进程协议、后端、严格候选名额、评分/融合或规划实现。纯结构迁移保持数据格式；确需调整格式时由唯一所有者统一支持版本与拒绝边界，不新增旧版读取、迁移、双写或回退。各步切换真实消费者/构造/测试并当场删除迁出定义，保持可构建，不留永久兼容门面。

完整实施须硬前置整体交付；只读盘点可提前。共享定义、Service/Config/组装/runner 段先确定一个修改所有者并顺序集成，禁止多条线独立重写共同文件。与 #38/#39 并行使用隔离 worktree/构建/配置/测试资源；#39 记录固定构建，不在运行中替换现场。迁移后按实际接线/行为影响决定证据复用或定向重验，零命中、skip 或 mock 都不能代表本机原生通过；远端流程不纳入一期验收。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TM08.1 | cmd、默认 app、host 和具体构造全部使用新边界，旧主包/contracts/app 路径无消费者并删除；host 无业务状态机/具体 SQL/adapter。 |
| TM08.2 | 实际服务启动/退出、控制幂等/实例版本、心跳与连接关闭保持；worker 存活及原请求恢复不误推断。 |
| TM08.3 | 配置和纯类型有唯一所有者，生产闭包/OS tags/安装资源守卫通过，未来领域可显式注入且未交付能力 unsupported。 |

验证只覆盖本次责任/接线变化，保留既有代表断言与禁止依赖反例，不重建全量故障矩阵。每次记录构建、环境、命令、实际 parent/subtest 命中和证据；证据保存 `docs/validation/three-line-08-app-host/`。目录/依赖目标只在实际实现交付后更新当前架构页。

## 交接

#81 得到最终生产路径与测试边界；尚未归组的测试迁到当前实际所有者或外层，不能依赖被删主包。
