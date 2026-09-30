# 项目执行抽象与配置隔离规划

目标标识：`EXECUTION-ISOLATION`。

SteerLine 将项目执行规则与 Codex、Orca 等具体实现分开。项目定义权限边界、工作区要求、发布责任和必要能力；执行方案选择目标、agent、model 与实现选项；适配器落实固定请求并返回事实。更换执行方案时，项目规则不会随之隐式变化。

本规划划分 #54–#61 八项任务，前五项迁移职责并保持已有行为，后三项落实项目策略及配置隔离。每项各有独立 Spec、Plan 和实现 PR，执行依据见下表及[共同约束](../spec/execution-isolation-constraints.md)；本文只组织目标、归属和推进关系，变化不会自动改写任务契约。

## 抽象范围

| 抽象 | 项目或中心定义 | 执行实现负责 |
| --- | --- | --- |
| 执行权限 | 文件访问、网络及超出边界的授权规则 | 参数映射、能力拒绝、落实证据；Windows sandbox 等实现选项归具体后端 |
| 工作区 | 逻辑仓库身份、base、执行分支、隔离及保留规则 | 创建和核验 worktree、解析实际资源、执行明确清理请求 |
| 会话 | Execution、逻辑 Session、Turn、反馈和恢复语义 | 操作 thread、terminal、Dispatch 等原生资源，核对原身份 |
| 交互事件 | 输入接受、轮次开始、问题、结束、宿主阻断、观察缺口 | 解释原生协议并保留来源、原始证据和消费位置 |
| 发布交付 | worker 或宿主发布责任、PR 关联、真实合并结项 | Git 发布操作、请求传输、PR provider 查询 |
| 执行需求 | 项目必需能力、工具和认证引用 | 检查目标是否满足要求，说明不可用原因 |

可配置的项目策略与固定共同契约分开。输入接受、轮次开始、轮次结束和任务完成不能互相替代；观察断线、进程退出和结果未知也分别表达。恢复保留已有人工继续门槛，明确区分原会话重接和用户选择的新会话。

第一期权限模型保持最小范围，不增加通用 RBAC、命令或域名白名单。环境需求只检查必要条件，不建设自动安装器，不把认证秘密复制进任务契约、执行快照或提示词。共同事件只表达执行流程所需事实，详细正文与活动展示继续使用现有会话模型。

## 分包与文件归属

以下是目标文件规划，实际创建随对应职责迁移进行，不先提交空包或占位文件。文件均相对于 `internal/steerline/`。

| 包 | 目标文件 | 职责 |
| --- | --- | --- |
| `execution` | `backend.go`、`session.go`、`event.go`、`operation.go`、`capability.go`、`workspace.go`、`errors.go` | 公共请求、身份、事实、回执和必要接口 |
| `execution` | `policy.go`、`snapshot.go`、`requirements.go` | 项目执行权限、冻结策略和资源绑定、执行需求匹配 |
| `execution` | `publication.go`（#60 创建） | 发布责任纯值，workerflow 使用它而不导入发布 I/O |
| `workspace` | `git.go`、`prepare.go`、`inspect.go`、`resources.go`、`cleanup.go` | Git 与本机工作区操作，资源路径解析 |
| `publication` | `request.go`、`publish.go`、`pr.go`、`provider.go`、`gitea.go`、`command.go`、`policy.go` | 发布请求、Git 发布、PR 外部事实及项目发布责任 |
| `codexworker` | 增加 `backend.go`、`config.go`、`permissions.go`、`command.go`，复用已有文件 | Codex 执行操作、权限编译、命令构造和原生身份核验 |
| `codexapp` | 复用已有客户端、传输、参数及原生问题文件 | Codex app-server 协议，不决定项目策略 |
| `orcaworker` | 增加 `backend.go`、`config.go`、`events.go`，复用已有协议文件 | Orca 资源、终端、派发、消息、观察和对账 |
| `workerflow` | 保留 `state.go`、`recovery.go`，调整 `prompt.go` | 纯业务规则和固定任务指令；后端选择移出 `mode.go` |
| 主包 `steerline` | `project_config.go`、`execution_profiles.go`、`execution_registry.go`、`execution_snapshot.go`、`execution_service.go` | 项目配置、全局方案、实现组装、快照组合与执行事务接线 |
| 主包 `steerline` | 保留 `worker_start.go`、`worker_events.go`、`worker_completion.go`、`worker_feedback.go`、`worker_pr.go`、`worker_state.go` 等现有接线 | 业务推进、待决策和 TaskDelivery 事务，沿用已完成的 worker 文件重组 |

现有持久领域记录不因名称与公共契约接近就全部移入 `execution`。SQL、Query/Command、会话存储和业务提交保持已有所有者；公共包只接受实际调用所需的固定输入，不传入整个 Runtime 或 Service。

```mermaid
flowchart TD
    Center["steerline 配置组装与事务"] --> Flow["workerflow 纯业务规则"]
    Center --> Workspace["workspace Git 操作"]
    Center --> Publication["publication 发布与 PR"]
    Center --> Codex["codexworker"]
    Center --> Orca["orcaworker"]
    Flow --> Contract["execution 公共契约"]
    Workspace --> Contract
    Publication --> Contract
    Codex --> Contract
    Orca --> Contract
    Codex --> App["codexapp 原生协议"]
```

箭头表示允许的主要依赖方向。公共契约不依赖具体实现、数据库或中心 Service；后端包不导入主包，不持有业务写库权。中心在组装时将 Codex 生成的启动命令交给 Orca，Orca 保留 shell 包装及终端操作，不自行决定 Codex 权限。按真实消费者划分必要的小接口，不强迫所有后端实现没有用途的方法。

## 任务与独立交付

| 任务 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- |
| #54 | 公共执行契约归包 | 无本系列前置 | [Spec](../spec/execution-isolation-01-contracts.spec.md) · [Plan](../spec/execution-isolation-01-contracts.plan.md) |
| #55 | 工作区与 Git 资源迁移 | #54 | [Spec](../spec/execution-isolation-02-workspace.spec.md) · [Plan](../spec/execution-isolation-02-workspace.plan.md) |
| #56 | 发布与 PR 实现迁移 | #54 | [Spec](../spec/execution-isolation-03-publication.spec.md) · [Plan](../spec/execution-isolation-03-publication.plan.md) |
| #57 | Codex 执行适配器迁移 | #55、#56 | [Spec](../spec/execution-isolation-04-codex.spec.md) · [Plan](../spec/execution-isolation-04-codex.plan.md) |
| #58 | Orca 执行适配器迁移 | #57 | [Spec](../spec/execution-isolation-05-orca.spec.md) · [Plan](../spec/execution-isolation-05-orca.plan.md) |
| #59 | 项目执行权限策略 | #58 | [Spec](../spec/execution-isolation-06-policy.spec.md) · [Plan](../spec/execution-isolation-06-policy.plan.md) |
| #60 | 项目发布责任契约 | #58 | [Spec](../spec/execution-isolation-07-publish-policy.spec.md) · [Plan](../spec/execution-isolation-07-publish-policy.plan.md) |
| #61 | 执行需求与配置注册收口 | #59、#60 | [Spec](../spec/execution-isolation-08-resolution.spec.md) · [Plan](../spec/execution-isolation-08-resolution.plan.md) |

## 顺序与并行条件

```mermaid
flowchart LR
    P1["#54 公共契约"] --> P2["#55 工作区"]
    P1 --> P3["#56 发布实现"]
    P2 --> P4["#57 Codex"]
    P3 --> P4
    P4 --> P5["#58 Orca"]
    P5 --> P6["#59 权限策略"]
    P5 --> P7["#60 发布策略"]
    P6 --> P8["#61 配置与注册"]
    P7 --> P8
```

实线是本系列最终集成的前置关系，不表示所有准备工作必须等待。默认串行沿上述关系推进；采用并行开发时按下表限定范围。

| 组合 | 开工条件 | 并行范围和集成条件 |
| --- | --- | --- |
| #55 与 #56 | #54 合入，共同请求及资源类型已固定 | 两个独立包可完整并行；共享中心文件指定一个集成人，后合入方更新基线并验证共同接线 |
| #57 与 #58 | 公共契约固定后可准备 Orca 低层迁移 | Orca 协议与事实整理可提前；#58 最终基于已合入 #57，使用其命令构造并删除原 Codex 参数拼接 |
| #59 与 #60 | #58 合入 | 策略及各自适配逻辑可并行；`project_config.go`、快照、提示词和 UI 接线串行集成，合入分别完成本轮实际接通 |
| #61 | #59、#60 合入 | 基于两类完整策略进行最后组合和依赖检查 |

公共契约修改集中处理，不能在两个并行 PR 中各自发展同一类型。明确接口生产者、消费者和共享文件所有者；后合入方核验双方的契约与持久化顺序。未知依赖或文件不重叠不构成可并行证据，也不能改变已经固定到任务 Spec 的串行要求。

## 每轮迁移与验证

每轮先明确类型、函数、消费者、依赖及副作用所有者，再建包移入完整职责，更新生产调用和必要测试，删除原实现。每次合入都应形成可编译、可独立工作的结果。当前轮范围内不得保留双实现、旧版兼容或提交失败后回退到旧路径；不把必要的接线与删除留给最后一轮。

前五轮以行为保持为目标；后三轮分别交付明确的配置行为变化。格式变化按[项目规则](../../AGENTS.md)拒绝不支持的旧配置或状态并保留现场，不提供迁移、双写或跨后端回退。当前格式内的重启、身份校验和未知操作对账仍须成立。

验证默认使用已有最小入口、受影响包的定向用例和必要的 import 检查，复用现有 fixture。搬迁不增加完整故障矩阵或重新认证全部平台；实际启动和权限行为改变时补必要的原生证据，模拟测试不能代替权限落实证明。只有具体失败或未解决风险才扩大测试，确需全量时使用现有并行入口。

每轮 Spec 固定范围、接口交接、前置与最小验收；Plan 固定该轮搬迁顺序、接线、删除项和验证命令。实现 PR 描述列明最终所有者、行为变化、实际验证及有消费者的剩余项。

## 与已有规划的关系

承接主干已落地的[协议与终态边界](../spec/architecture-01-worker-protocol.spec.md)、[显式原子提交](../spec/architecture-02-worker-commit.spec.md)、[Runtime 与后端边界](../spec/architecture-03-runtime-boundary.spec.md)及 worker 文件重组。本系列迁移共同契约和实现所有者，保留现有协调器、绑定/回执附件事务和只读 Runtime，不重复建设它们。

权限迁移保留现有 local Windows elevated 默认、冻结与 unelevated 分散根拒绝；新的项目策略由 #59 交付。#60 冻结 publisher 和完整非秘密发布目标，#59/#60 串行扩展同一个执行快照，不等 #61 再合并两份结构。

Orca 已交付的显式 Windows 实现选择、无 model 受管命令、安全 args、隐式临时根排除、准备/业务轮分离和宿主恢复由后续迁移保持。已证实的 Orca unelevated 组合不因套用 local 限制而被删除。#54 的已实施契约保持固定；#57 迁移唯一 Codex 参数生产者，#58 基于现成契约承接准备/阻断与可信接收方，保留中心 WorkerEnvironment 和平台工具证据边界。

[worker 环境就绪原范围](../spec/worker-environment-readiness.spec.md)已交付，后续沿用其工具/原子收据/结算与重查并发证据。#59 将已有权限编译和两阶段冻结改为项目策略来源，新快照/授权接线须独立验证；不把已交付能力反向设为等待 #59，也不重复完整就绪工程。

[停服恢复](../spec/worker-service-recovery.spec.md)的人工门槛、代次与原身份核验继续成立；本系列不借搬迁重新自动发送已保存输入或静默替换 worker。[持续 worker 流程](../spec/worker-flow.spec.md)及其后续明确补充仍决定业务状态，测试、修复、审查来自固定任务契约。

## 系列完成目标

项目权限、工作区要求、发布责任和执行需求有明确归属；共同会话、事件和操作事实不携带业务代码必须解释的原生协议。所有执行路径使用固定项目策略和方案，能力不足时明确拒绝。

新增执行配置主要修改对应实现、配置校验与注册接线，Task 状态机、反馈流程、PR 交付判定和 UI 主状态无需增加后端分支。已有执行在配置变化后继续使用原快照及原资源身份，当前格式恢复与唯一交付规则保持成立。
