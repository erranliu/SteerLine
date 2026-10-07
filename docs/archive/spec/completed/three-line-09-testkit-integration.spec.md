# Spec：测试支持与集成目录整理

任务 ID：81；任务名称：测试支持与集成目录整理；目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。配套 [Plan](three-line-09-testkit-integration.plan.md)。一个独立实现 PR 交付。

## 范围与前置

硬前置：#80。将边界/反馈/进程/协议测试支持归 `internal/testkit`，两处跨模块 integration 归 `tests/integration`，同步 runner/固定选择/构建标签/fixture 路径并保持断言覆盖。

实际盘点起点：`internal/coreboundary`、`internal/testboundary`、`internal/testfeedback`、`internal/testprocess`、`internal/steerline/testprotocol` 已迁残留；`internal/steerline/integration`、`internal/steerlineintegration` 当前实际跨模块测试；`cmd/test-feedback` 与 `tests/e2e` runner/suites。开工时按实际消费者核对路径；前置迁移已发生的部分只承接其唯一实现，不重建。

## 实现要求

1. 包内单元测试随真实生产所有者保留，只把真实 app/跨模块断言归 tests/integration。迁出的两个旧 integration 路径删除，shared fixture 只用中性公开端口，不新增测试专用业务写接口。

2. 生产闭包不依赖 testkit/tests；E2E 场景只经现有 driver/证据适配层调用产品入口，Go internal 可见性保持。tests/integration 不直接 import tests/e2e/internal。

3. 迁守卫实现并持续扫描所有 OS/tag/生成源码与 Windows/Linux 生产依赖，禁止依赖反例仍失败；包改名不能减少边界范围或白名单放宽。

4. 固定 L1、Full 枚举、子进程 helper、fixture 和 suites.json 的 package/test 映射同步；全部旧到新 parent/subtest/关键断言用 test-mapping.json 记录，删除覆盖需有已替代断言证据。

5. 不借搬测试扩大 Full/live 默认选择或添加生产层测试能力；L2/L3 现有环境/场景 ID 保持，原生能力未交付继续报告缺口。

## 共同执行约束

遵守 [三线解耦共同实现约束](three-line-constraints.spec.md) 的最终依赖矩阵与跨域提交规则；本阶段负责实际启用自己迁移部分的守卫。

保持当前任务/DAG、持续 worker、人工并行/自动派发、原会话反馈、恢复授权、唯一交付和 UI 生命周期。失败、轮次结束、最终回复、交付、任务关闭与资源停止分别表达。具体实现无业务写权，状态仍由唯一业务写者经窄声明提交；同数据库查询保持同读快照。

同仓库、同 go.mod、同 SQLite 和服务部署方式；不引入新进程协议、后端、严格候选名额、评分/融合或规划实现。纯结构迁移保持数据格式；确需调整格式时由唯一所有者统一支持版本与拒绝边界，不新增旧版读取、迁移、双写或回退。各步切换真实消费者/构造/测试并当场删除迁出定义，保持可构建，不留永久兼容门面。

完整实施须硬前置整体交付；只读盘点可提前。共享定义、Service/Config/组装/runner 段先确定一个修改所有者并顺序集成，禁止多条线独立重写共同文件。与 #38/#39 并行使用隔离 worktree/构建/配置/测试资源；#39 记录固定构建，不在运行中替换现场。迁移后按实际接线/行为影响决定证据复用或定向重验，零命中、skip 或 mock 都不能代表本机原生通过；远端流程不纳入一期验收。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TM09.1 | 旧支持与集成路径无剩余消费者，生产不依赖 testkit，跨模块测试和 E2E internal 依赖边界成立。 |
| TM09.2 | L1、runner/fixture/标签和注册表指向唯一新路径；映射中迁移 parent/subtest 非零 run/pass，无丢失或跳过冒充通过。 |
| TM09.3 | 正/负向守卫与一组真实 app 集成测试通过，E2E registered 场景编译且覆盖范围保持。 |

验证只覆盖本次责任/接线变化，保留既有代表断言与禁止依赖反例，不重建全量故障矩阵。每次记录构建、环境、命令、实际 parent/subtest 命中和证据；证据保存 `docs/validation/three-line-09-testkit-integration/`。目录/依赖目标只在实际实现交付后更新当前架构页。

## 交接

#82 得到最终守卫、非零命中映射和独立组/场景入口；保存 #38/#39 已迁路径，后继直接消费唯一新入口。
