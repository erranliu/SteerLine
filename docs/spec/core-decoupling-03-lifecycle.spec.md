# Spec 通用执行生命周期与副作用接线

任务 ID：64；任务名称：通用执行生命周期与副作用接线；目标：`CORE-IMPLEMENTATION-DECOUPLING`。配套 [Plan](core-decoupling-03-lifecycle.plan.md)。硬前置：#63 整体交付，一个实现 PR 完成。

## 目标与范围

把平台专属生命周期协调转为中心统一的分步执行/对账流程。原生步骤、命令与结果解释归实现；业务资格、提交屏障和唯一事务留中心。

遵守[共同约束](core-decoupling-constraints.md)，承接当前执行隔离及恢复修复，不回写历史 Spec。迁移输入：`codex_worker*.go`、`codex_feedback_hooks.go`、`orca_worker_prepare/dispatch/observation/terminal/workspace/relay.go`、`orca_remote_service.go`、`orca_execution_operations.go`、worker_execution/resume/recovery/feedback/events、service_workers 及 registry 的 Service 回调；`orca.go`/`orca_remote_adapter.go` 的原生 I/O 调用。

## 接口交接与提交屏障

| 方向/所有者 | 输入或输出 | 固定语义 |
| --- | --- | --- |
| 中心 → 实现 | 只读选步/观察输入 | 不可变通用上下文、原配置/绑定及已提交操作状态；没有可写 Service/Run/Store，不执行隐藏变更。 |
| 实现 → 中心 | 下一步候选 | 明确操作类别、固定目标、请求摘要与输入值；中心按当前业务/恢复资格决定是否持久化 intent。 |
| 中心 → 实现 | 单步变更请求 | 已持久 Operation 身份、固定请求、目标和代次；只允许这一项变更，不把下一项副作用包进一次调用。 |
| 实现 → 中心 | 结果或只读对账事实 | accepted/confirmed/rejected/outcome_unknown、原 request ID/证据、绑定变化；中心在原事务提交结果，成功后才授权下一步。 |
| 中心 → 实现 | 精确清理 | 已交付或明确停止后的原资源清单和对应授权，settlement 与清理分开；不把 release 当取消或交付条件。 |

## 必须交付的契约与消费者

1. 复用现有 execution 小接口、Operation 和 workerMessages/actor，为真实消费者固定准备/启动/输入/回答/观察/对账/停止/清理的必要请求与结果。请求是公共上下文、固定绑定及单个中心持久 intent；结果是明确通用事实、回执、部分绑定/观察 token 和有界实现状态变化。不能采用通用 Execute(any)、平台字符串 switch 或含全域写方法的 Engine。

2. 实现可确定下一个具体资源操作和读取原现场，中心核验当前准备/业务/恢复资格并先保存 Operation。每次变更只执行已保存的一步；结果原子附加到原 Execution/Attempt 后再选下一步。没有 receipt 的不确定动作只进入 Reconcile，不能靠新 request ID 重发。候选步骤按配置/输入绑定摘要确定并绑定用途；重复选步不能另造 mutation 身份。异步结果匹配原操作、请求摘要、目标、代次和输入绑定摘要，中心用现有前值事务提交候选绑定；冲突/过期结果拒绝，提交失败不推进下一步。

3. 不暴露 Service/Runtime/Run/Store/整份 Domain 或可写业务记录给实现。状态结果以不可变值交给中心现有写集合处理；异步观察器仅回报通用事件，不直接改 Decision/Operation。注册中的 Prepare/Start/Resume/DeliverFeedback 等 Service 回调由端口替代，OrcaOperations 特例取消。

4. 本机进程/远端 Dispatch 的生命周期差异通过能力和明确事实表达；输入接受不等于 turn 开始，process exit 不等于完整结算。复用的原 session 必须在变更前核验完整身份。关闭观察、stop、settlement、release/cleanup 分开，远程取消继续 unsupported。

5. 准备/业务轮资格、人工继续门槛、已保存答案新实例确认、消息内容先持久再 ack，以及 TaskDelivery 的唯一事务均保留。thread/start 绑定提交后才 turn/start，原资源创建回执提交后才派发。具体权限证据不因迁移获得自动认证。

## 删除与剩余所有者

当场删除 root 的具体 lifecycle 方法、codex/orca feedback hooks 和 operation factories；保留的中心逻辑改为通用 worker_* 协调。orca_execution_operations 与 native/remote 分派不再留在主包；具体工厂最终外移之前只有一个明确的组装入口。root 平台命名文件里如含 history/宿主部分，按职责拆出并明确交 #65，不能复制或以平台 shim 留空文件。

具体 history 来源采集和宿主工具证据解释由 #65 迁移；本轮提供其准备/观察/确认端口。#63 列明的生命周期临时解码视图消费者在本轮全部迁走；采集/宿主剩余视图明确交 #65，CD64-3 的无原生解释约束只针对生命周期。应用组合位置与默认目录的最终移出由 #66 完成，本轮不引入第二个事务或模型服务。

## 阶段构造与外层测试

本轮可按真实消费者创建 `steerlineintegration` 测试包；尚无 #66 的应用组装时，由外层测试通过显式通用依赖构造核心服务并连接现有实现。#64 为生产生命周期所需的端口注入提供正式构造参数，root 的当前唯一内置目录仍可暂时调用同一构造；不能为测试新增私有 actor/Domain 写入口。#66 只把内置默认组合移至 steerlineapp 并删除 root 默认构造，测试随真实产品入口切换。两者是同一注入构造的不同调用者，不是两套生命周期。

## 开工与集成

#63 整体交付后才完整开工。准备性阅读不构成提前领取资格。共享 execution/contracts、主包配置/Service/事务接线及原生 fixture 的变更串行处理；本任务当场删除已切换消费者的原实现，不交付双路或空接口。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CD64-1 | 代表性 local 两轮与 native 原会话恢复通过，thread/资源绑定提交失败不执行下一步；消息提交失败不 ack；未知 start/reply/send/ack 对账不重复变更。 |
| CD64-2 | 准备结算/迟到成功不跨越人工恢复门槛，替换 session/incarnation 拒绝；TaskDelivery 不因清理失败改变，remote 协议的固定身份和 Claim 拒绝保留。 |
| CD64-3 | 所有生命周期消费者使用同一通用端口（采集/宿主解释交 #65），平台实现无中心写者依赖，root 不再按 backend/协议名派发或解释 native 操作结果。 |

选取现有 app-server 两轮/receipt barrier、worker_commit_boundary 的提交失败、native 完整恢复和一个 remote Protocol 丢失回执组。实际名称/路径在迁移时列明映射；对不同实现重复的纯业务矩阵只保留中心一份，remote 不走虚构 Claim 成功。

## 交付

#65 获得不需要平台类型的准备/观察/消费/宿主操作交接；#66 获得不接可写 Service 的实现构造接口。原生命周期不形成新引擎。PR 列明最终所有者、删除清单、测试路径映射、实际命中结果与剩余支持限制。规划时不启动实现，不创建中心任务或改数据库。
