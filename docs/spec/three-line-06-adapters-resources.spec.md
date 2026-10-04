# Spec：执行适配与资源目录归组

任务 ID：78；任务名称：执行适配与资源目录归组；目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。配套 [Plan](three-line-06-adapters-resources.plan.md)。一个独立实现 PR 交付。

## 范围与前置

硬前置：#76。将具体后端归 `internal/execution/adapters/{codex,orca,orcaremote}`，Git/PR/进程归 `internal/execution/{workspace,publication,process}`；核对共享协议并完成真实构造/资源路径切换。

实际盘点起点：`codexworker`、`codexapp`、`orcaworker`、`orca_remote_worker`、`remoteprotocol`、`workspace`、`publication`、`processhost`，app directory/factories、嵌入资源、测试子进程和 runner 路径。开工时按实际消费者核对路径；前置迁移已发生的部分只承接其唯一实现，不重建。

## 实现要求

1. 后端只消费 backend 纯契约与必要纯值，不导入 task/execution 应用用例、storage 写端口、具体 SQLite、app 或 console。PR 查询/Git/进程副作用各用既有独立包，业务核验仍由 execution 负责。

2. 盘点 codexapp 的 Codex/Orca 实际消费者。共享 JSON-RPC/WS 协议有多方调用就保留执行侧专用协议子包；不复制 codec、不创建通用 utils，不赋予未来 planning worker 权限。

3. 冻结配置与原封套归具体实现唯一 codec；旧格式拒绝、原 session/workspace/process/target/generation 和恢复对账保留。embed、OS build tags、launcher/资源模板及 remote 协议 fixture 同次修正。

4. 纯搬包不改变原生启动参数、权限、发布或停止能力；远端缺目标绑定权限仍领取前拒绝，取消 unsupported 不伪装成功。若为迁移必须改真实行为，明确改动并在本任务补对应最小原生证据。

5. 与 #39 可隔离构建和现场并行；不能在其运行中替换 executable、配置或平台宿主。共享 app 构造由本任务顺序集成，业务变化不借目录迁移扩大产品范围。

## 共同执行约束

遵守 [三线解耦共同实现约束](three-line-constraints.spec.md) 的最终依赖矩阵与跨域提交规则；本阶段负责实际启用自己迁移部分的守卫。

保持当前任务/DAG、持续 worker、人工并行/自动派发、原会话反馈、恢复授权、唯一交付和 UI 生命周期。失败、轮次结束、最终回复、交付、任务关闭与资源停止分别表达。具体实现无业务写权，状态仍由唯一业务写者经窄声明提交；同数据库查询保持同读快照。

同仓库、同 go.mod、同 SQLite 和服务部署方式；不引入新进程协议、后端、严格候选名额、评分/融合或规划实现。纯结构迁移保持数据格式；确需调整格式时由唯一所有者统一支持版本与拒绝边界，不新增旧版读取、迁移、双写或回退。各步切换真实消费者/构造/测试并当场删除迁出定义，保持可构建，不留永久兼容门面。

完整实施须硬前置整体交付；只读盘点可提前。共享定义、Service/Config/组装/runner 段先确定一个修改所有者并顺序集成，禁止多条线独立重写共同文件。与 #38/#39 并行使用隔离 worktree/构建/配置/测试资源；#39 记录固定构建，不在运行中替换现场。迁移后按实际接线/行为影响决定证据复用或定向重验，零命中、skip 或 mock 都不能代表本机原生通过；远端流程不纳入一期验收。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TM06.1 | 所有具体构造指向新包，旧 adapter/资源目录删除；共享协议唯一实现，资源路径和全 OS/tag 依赖守卫通过。 |
| TM06.2 | 本地 Codex/本机 Orca 代表协议、封套/配置拒绝、原身份恢复与真实资源接线保持；存量远端只复核协议 fixture、编译与拒绝保护，不要求新增远端实测，后端无业务写权。 |
| TM06.3 | 原生行为变更才定向补证据；纯搬包有证据复用和影响映射，不将 mock 或 unsupported 认证为新能力。 |

验证只覆盖本次责任/接线变化，保留既有代表断言与禁止依赖反例，不重建全量故障矩阵。每次记录构建、环境、命令、实际 parent/subtest 命中和证据；证据保存 `docs/validation/three-line-06-adapters-resources/`。目录/依赖目标只在实际实现交付后更新当前架构页。

## 交接

#80 接新工厂与资源小端口；#39 记录固定构建，迁移后只重验 native-impact.md 中发生实际变化的入口。
