# Spec 通用执行事件与问题边界

任务 ID：62；任务名称：通用执行事件与问题边界；目标：`CORE-IMPLEMENTATION-DECOUPLING`。配套 [Plan](core-decoupling-01-events.plan.md)。硬前置：#61 已交付的需求/注册/v4 快照基础，一个实现 PR 完成。

## 目标与范围

把生产 WorkerEvent、Decision 输入和回答确认转换为通用事实；原生消息解码、问题路由和回答回执解释归对应执行实现。中心业务消费者不接收原生 envelope 或消息类型。

遵守[共同约束](core-decoupling-constraints.md)，承接当前执行隔离及恢复修复，不回写历史 Spec。迁移输入：`worker_types.go`、`worker_evidence.go`、`worker_events.go`、`decision.go`、`codex_worker_evidence.go`、`orca_worker_evidence.go`，以及 contracts 的原生问题字段。

## 接口交接与拒绝条件

| 方向/所有者 | 输入或输出 | 固定语义 |
| --- | --- | --- |
| 实现 → 中心 | 通用事件批次 | 固定 Execution/代次/Session/Turn、输入/开始/停止/问题/缺口事实、来源 cursor、原始证据引用；原生 envelope 不出实现。 |
| 中心 ↔ 实现 | 问题及答案路由 | 中心保存公共问题/选项与有身份的路由封套；实现严格解释路由并核对原 native 接收方，答案只使用该路由。 |
| 中心 → 实现 | 消费确认 | 已提交的消息/来源消费 token；内容或消费事实提交失败时不调用 ack，token 不构成任意执行权限。 |
| 拒绝 | 来源或路由不匹配 | 不同代次/Session/Turn、重复冲突问题、未知 codec 或损坏摘要拒绝；gap 与当前不确定性不等于结束。 |

## 必须交付的契约与消费者

1. 复用 execution.EventFact、Question、Answer、ToolReplyFact 和 SessionContext，按真实消费者补充问题/输入接收/开始/结束/缺口的明确事实，不另定义第二套相同事件。WorkerEvent 只携带通用事实、内容、来源证据、原始证据引用及持久化反馈通道。原生 codexapp.Envelope 与 OrcaInboxMessage 不出实现边界。

2. 问题中心值包含问题身份、正文/选项/回答约束和固定 Execution/代次/Turn；原生 call/message/dispatch/server 路由由实现拥有的类型及严格 codec 表达，以有界、版本化且绑定所有者的路由封套交给中心保存。中心不自行构造原生路由，不使用 worker 自报接收方。路由封套的 codec 接线在本轮交付，不等 #63 补齐；配置工厂/原生执行记录尚未迁移，不要求本轮删除其唯一生命周期实现。根包业务问题只保存公共内容和路由封套，后续原生回答方法通过 codec 接收解码视图，#64 再移走这些方法。

3. 回答资格、Decision 版本、来源 Attempt、恢复版本、去重与原子提交仍归中心。实现返回接受/拒绝/结果未知和经核验的回答事实，不能因普通工具成功或输入已接收推定回答完成。实时、重放与恢复调用唯一解码/校验实现。

4. 保留内容持久化后才 ack 的现有屏障：实现观察得到事实与消费 token，中心提交后才调用确认端口。处理结果不能通过把 Service 或可写 Run 放入回调来返回。

## 删除与剩余所有者

删除 root WorkerEvent 的原生字段、OrcaInboxMessage 别名、workerFlowEventFact 的 CodexAskCall 字段以及仅用于原生解码的父包 helper；仍包含事务的函数改为消费通用问题/事实后保留。平台 DecodeEvent/Question 及相关 unit fixture 随实现归包。

配置与全执行绑定冻结、资源创建、多步启动和 history 采集不在本任务迁移；后续 #63/#64/#65 接收本轮通用事件及路由封套。root 中尚有具体工厂不阻止本轮交付，但不能通过它把原生事件交回业务消费者。

## 开工与集成

#61 的已交付代码基础可直接消费；ER4 缺少的 remote 成功准入证据不是本任务开工前置。准备性阅读不构成提前领取资格。共享 execution/contracts、主包配置/Service/事务接线及原生 fixture 的变更串行处理；本任务当场删除已切换消费者的原实现，不交付双路或空接口。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CD62-1 | 用同一中心中性 fixture 消费两种实现返回的 question/started/completed/gap，中心事件与决策消费者不按 native method、消息 type 或平台名称分支。 |
| CD62-2 | 复用问题选项/答案验证和回执丢失/replay 用例：原身份/旧 Turn 拒绝，回答不会重复发送，Decision 与消费事实提交失败时不确认消息。 |
| CD62-3 | 主包事件/问题端口无平台 DTO，execution/contracts 不反向导入具体实现；所有生产消费者已切换，旧 decode 入口删除。 |

只运行 execution/contracts 和两类解码实现的定向单元测试；中心选一组 pending ask replay/回答去重/消息持久化失败屏障。复用 TestAppServerRecoveryReplaysOnlyUnresolvedQuestion、TestNativeOrcaRejectsLateDispatchMessages 和相关 worker receipt 事务用例，迁包后按实际位置运行，不重复全协议矩阵。

## 交付

#63 消费已接通的通用事件、问题正文与严格路由封套；#64 消费明确的 started/accepted/settled 事实和消息消费 token。本任务只锁定自身路由数据，不预建完整实现状态机。PR 列明最终所有者、删除清单、测试路径映射、实际命中结果与剩余支持限制。规划时不启动实现，不创建中心任务或改数据库。
