# Spec 执行配置与原生绑定所有权

任务 ID：63；任务名称：执行配置与原生绑定所有权；目标：`CORE-IMPLEMENTATION-DECOUPLING`。配套 [Plan](core-decoupling-02-bindings.plan.md)。硬前置：#62 整体交付，一个实现 PR 完成。

## 目标与范围

把具体配置、原生持久绑定和 codec 的所有权移入实现包；主包冻结和恢复只使用通用策略、身份及实现数据封套，不读取具体配置字段。

遵守[共同约束](core-decoupling-constraints.md)，承接当前执行隔离及恢复修复，不回写历史 Spec。迁移输入：`config.go`、`execution_profiles.go`、`execution_snapshot.go`、`service_execution_profiles.go`、`execution_capability.go`、`execution_backend.go`、`codex_git_sandbox.go`、`orca.go`/`orca_remote_adapter.go` 的具体配置转换、contracts/execution.go、records.go、runtime.go 及对应适配器绑定。

## 接口交接与冻结边界

| 方向/所有者 | 输入或输出 | 固定语义 |
| --- | --- | --- |
| 外层/实现 → 中心 | 受理配置值 | 实现 key/契约版本、规范化配置封套、非秘密展示描述、有效策略/发布责任与支持结果；中心不解原生字段。 |
| 中心 ↔ codec | 持久数据封套 | schema/codec 版本、用途、摘要及最多 64 KiB 载荷；配置绑定 Project/Profile/受理版本，资源和问题绑定 Execution/代次，不能互换。 |
| 实现 → 中心 | 资源阶段事实 | 公共工作区/Session/Turn/目标与原生绑定封套；与领取阶段配置摘要绑定，实际路径及权限根不能在领取前伪填。 |
| 拒绝 | 新格式身份或版本失败 | 完整配置摘要、资源归属、codec key/版本、实际身份任一冲突拒绝；缺失资源不会被当前配置重建。 |

## 必须交付的契约与消费者

1. 项目 ExecutionPolicy/PublicationPolicy/ExecutionRequirements 及公共仓库/目标继续是显式值；Profile 的实现选项由选中实现严格解码、规范化和校验。保留项目 `publisher=execution_profile` 的明确授权与方案发布责任解析，冻结的是有效责任，不能强制还原为单一项目 publisher。纯命令的 executable/argv 不混入 WindowsSandbox 等实现选项，原生默认值及权限编译归实现。配置受理/冻结/展示消费者不声明或分支解释原生键；尚未迁移的具体工厂可在唯一组装处通过 codec 获取原生值，#66 将该组装整体外移。

2. 固定快照保存实现 key、实现契约版本、配置 codec 版本、有界配置载荷摘要、受理配置版本、通用策略和领取需求证据。资源阶段保存有身份/代次的绑定封套和通用工作区/会话/轮次事实，保留原两阶段冻结。两类封套都由固定目录选择唯一 codec；不能仅把旧 struct 改成 RawMessage 而保留 root JSON 字段访问。

3. 原生 thread/endpoint/generation、terminal/Dispatch/coordinator、来源 cursor 和待答原生路由由实现定义严格类型。contracts 的持久记录不暴露 CodexAskCall、OrcaNativeExecution/Turn 或具体原生配置类型；准备用途、业务计数、Decision、Operation 和 TaskDelivery 仍归中心。

4. 中心验证通用身份、摘要和受理版本，具体 codec 校验 key/版本/native 资源的完整一致性。静态参数冲突检查发生在任何外部 probe 前；未来恢复只使用原目录版本、固定配置和原绑定，不用当前选择补造证据。配置列表与查询展示使用实现提供的非秘密描述 DTO，不由 UI 或 root 重新编译参数。

5. 本任务更新所有绑定/快照的生产、持久读写、恢复、查询和深复制。尚未迁移的生命周期/采集方法只能通过唯一 codec 解码新的绑定封套得到实现拥有的临时类型视图；这种内存视图不是第二份持久权威，写回仍只产生新封套并由中心提交。逐项列出这些临时方法，由 #64/#65 迁移并删除；本轮不提前承担其业务协调改造。可在 root 临时保留最终组装调用，但不保留旧字段读取、旧格式快照、双写或 fallback。格式改变更新版本并拒绝旧格式。

## 删除与剩余所有者

删除 root 的具体配置别名、native Profile/Record 字段消费、平台权限/绑定解码 helper；原生类型和校验整体迁入实现，根包配置/持久/组合摘要与查询消费者只消费规范化封套、通用投影及公共策略；未迁移生命周期/采集的 codec 消费清单明确交 #64/#65。#62 的问题路由 codec 统一接入当前封套规则。

本任务不将中心多步协调或业务写者移入实现。外层最终工厂由 #66 迁移；#64 接收已能生产/恢复的固定通用快照与绑定，不负责补 codec。历史 #54 Spec 保留，新格式要求由本任务独立固定。

## 开工与集成

#62 整体交付后才完整开工。准备性阅读不构成提前领取资格。共享 execution/contracts、主包配置/Service/事务接线及原生 fixture 的变更串行处理；本任务当场删除已切换消费者的原实现，不交付双路或空接口。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CD63-1 | 缺失/未知/损坏 codec、摘要、配置版本、目标/代次或原资源身份在副作用前拒绝；配置变化不改变原执行，公开副本与私有对象不共享可变载荷。 |
| CD63-2 | 领取/资源两阶段冻结都命中真实生产消费者；工具/需求当前阻断不覆盖原生命周期事实；remote 缺证仍领取前拒绝。 |
| CD63-3 | contracts 不声明原生配置/绑定类型，不导入实现；root 配置/快照/查询消费者不解释原生字段；旧格式和旧校验路径删除。 |

受影响 codec 纯测试加一组中心冻结/恢复/事务接线。复用 TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority、TestClaimRechecksAcceptedConfigFiles、TestWorkerFlowContextsDoNotExchangeExecutionProfiles、TestResolutionAudit 系列中与本轮字段相关的断言及 remote Claim 拒绝，不重新认证全部平台。

## 交付

#64 获得可恢复的规范化配置、原绑定、通用会话/轮次和小型 codec 目录；具体构造仍留唯一组装入口供 #66 接管。PR 列明最终所有者、删除清单、测试路径映射、实际命中结果与剩余支持限制。规划时不启动实现，不创建中心任务或改数据库。
