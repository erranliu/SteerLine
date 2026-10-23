# Spec 会话采集与宿主交互实现隔离

任务 ID：65；任务名称：会话采集与宿主交互实现隔离；目标：`CORE-IMPLEMENTATION-DECOUPLING`。配套 [Plan](core-decoupling-04-conversation.plan.md)。硬前置：#64 整体交付，一个实现 PR 完成。

## 目标与范围

把具体会话来源发现、history client、mapper、重放解析和宿主事实解释移入实现；主包只调度通用来源、保存内容/checkpoint 并按可信通用动作处理准备和宿主阻断。

遵守[共同约束](core-decoupling-constraints.md)，承接当前执行隔离及恢复修复，不回写历史 Spec。迁移输入：`codex_conversation.go`、`codex_history_port.go`、conversation_service/replay/position、Service.conversationMappers 与具体通道、`worker_environment.go` 的平台调用及 workerflow 就绪消费；保留 PR176 的 query/sqlite 会话只读分层。

## 接口交接与消费边界

| 方向/所有者 | 输入或输出 | 固定语义 |
| --- | --- | --- |
| 中心 → 来源实现 | 原绑定及读取预算 | 固定项目/Execution/Attempt/资源、来源位置、页/字节预算；发现不能跨项目或擅自改 source 身份。 |
| 实现 → 中心 | 读取及映射批次 | 共享 contracts 中的通用 ConversationWrite/来源 DTO、raw/body 引用、完整度/缺口和候选 checkpoint；不包含具体 mapper/client。 |
| 中心 → 来源实现 | 已提交位置 | CommitConversation 成功后发布 cursor/checkpoint；失败时保留原位置。实现的暂存 mapper 状态也不得提前成为消费权威。 |
| 实现 ↔ 中心 | 准备/宿主事实与动作 | 原目标工具/收据/settlement、HostFact 与可信动作值；中心持有 nonce/deadline/资格，打开目标及 probe 变更仍经过 #64 保存 intent。 |

## 必须交付的契约与消费者

1. 实现来源端口返回绑定项目/Execution/Attempt/原资源的来源身份、分页 cursor、完整度/缺口与共享 contracts 中的中性 ConversationWrite 值（现有唯一写入 DTO 如在主包则连同必要纯值归 contracts）。历史发现、命令/客户端构造、native 来源优先级解释、mapper 和原始游标核验由对应实现拥有，不导入主包或自行写数据库。

2. 中心仍拥有采集调度、并发/页/字节预算、成功提交后的 checkpoint/cursor、旧 Attempt 隔离和公开视图。来源 mapper 的候选状态须可丢弃/重建：读取不得提前推进已提交状态；commit 失败重新使用原 checkpoint 得到同一批次，不能只回滚 cursor 而留下已消费的 mapper。采集事务复用 CommitConversation，不并入 workerCommit，也不在 mapper 中推进业务轮次或交付。重放/实时/历史使用同一来源与证据核验。

3. 宿主观察、输入资格、原 terminal 打开、探针文件准备/只读结果核验、工具证据和安全文件读取归对应实现。纯探针内容生成与文件写入分开；写 probe/ignore 文件、打开或操作 terminal、清理 probe 等变更都通过 #64 单步持久 intent，Observe/读取/映射端口不得包含隐藏写入。中心保存通用准备用途、nonce/deadline、Ready/Invalidated/Blocker、DeferredDecision/Feedback，可信动作需校验 Decision/原目标/代次。宿主动作接收方与 worker 文字分别表达，不注入菜单。

4. 准备结果可能来自任意实现；root 不读 terminal tail、app-server tool wrapper 或 Dispatch settlement。原步骤通过 #64 单步副作用/对账端口执行，准备成功仍需工具+收据+结算和无 blocker；重启/期限/并发重查语义保持；原终端 agent 尚未识别时仅在原 deadline 内观察，不新建 terminal/Dispatch；真实字符串化工具 wrapper 与普通 stdout 仍按既有证据等级区分，不把输出文本冒充 native exit/tool 收据。

5. 来源和能力展示使用通用 DTO 与现有 query Reader。同一读快照、正文/raw 损坏拒绝、Unicode 分页、缺口完整度和旧 Attempt 边界不变。Service 不持有具体 HistoryBinding/Client/Mapper，只持有通用采集端口及每来源的不可变状态。

## 删除与剩余所有者

删除 root 的 codex_conversation/history_port、具体 conversation mapper 字段、采集原生 callback 及 worker_environment 平台 helper；保留通用采集协调/准备业务状态。平台 fixture 随采集/宿主实现迁包，生产无重复 mapper 或并行查询读取实现。

不再次迁移会话只读 SQL、不改 UI 排版或增添新会话来源支持。#66 外移具体构造；本任务使 Service 和业务运行不需要知道它们的类型；#63 暂存的来源/宿主原生解码视图消费者在本轮全部删除。

## 开工与集成

#64 整体交付后才完整开工。准备性阅读不构成提前领取资格。共享 execution/contracts、主包配置/Service/事务接线及原生 fixture 的变更串行处理；本任务当场删除已切换消费者的原实现，不交付双路或空接口。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CD65-1 | 会话 raw/body/缺口、晚到 final、Unicode 分页和旧 Attempt fixture 保持实际命中，写入失败不推进源 cursor；native 来源替换不丢原始证据。 |
| CD65-2 | 复用准备成功/工具失败与重启期限代表用例，host 动作不会投递成 worker 反馈，准备/业务用途与计数隔离，重查竞争不清除新 blocker。 |
| CD65-3 | Service/采集/准备消费者不引用具体 client/mapper/binding 或解释原生输出，采集实现不依赖主包/SQL写端口；query/sqlite 原有只读分层保留。 |

会话只读 pure 包及中心来源 commit/cursor 代表用例；复用错误 workspace 拒绝、来源替换/晚到 final、raw/body 损坏和 Unicode 页面。准备侧选 ER1 正向/工具失败、ER3 重查竞争、ER4 固定期限和重启确认，按本轮变化挑必要子用例而不重复全矩阵。

## 交付

#66 获得具体类型已隐藏的 Service/采集/宿主端口，可在外层构造全部依赖；#67 接收实际迁移后的来源 fixture 和测试入口清单。PR 列明最终所有者、删除清单、测试路径映射、实际命中结果与剩余支持限制。规划时不启动实现，不创建中心任务或改数据库。
