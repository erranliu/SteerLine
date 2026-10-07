# Plan：dev 隔离与 stable 提升

任务 ID：41；任务名称：dev 隔离与 stable 提升。配套 [Spec](runtime-isolation-promotion.spec.md)。状态：延后，不作为当前执行入口。原草案硬前置 #40、#36，重新启用时复核 #40/#83 能力及维护需求，重新固定范围与前置。

## 待重定的实施草案

1. 消费 #40 的选择/版本接口及 #36 的工作轮次、消息和会话恢复事实，列出 home、项目及全部中心写入口，固定空闲可恢复条件。
2. 接入原事务和锁实现维护屏障，只控制新增启动/反馈投递/配置选择；在实际外部轮次仍活动时拒绝切换，不给 worker 内部步骤增加锁。
3. 实现持久提升请求、一致备份、原子选择和带原 owner 的新服务恢复；对齐 dev select 的同一准入条件。
4. 用真实 SQLite 和可控后端注入 RP3/RP5/RP6/RP7，涵盖部分屏障、原请求恢复及已保存反馈不重复发送。
5. 完成 Windows 双 channel 和空闲原会话提升真机证明 RP1/RP2/RP4，运行共同约束回归。提升组合的后继消费者届时另行固定，当前 [一期验收活动](../acceptance/phase1.md) 不消费这些证据。

## 测试计划（延后实施与自动验收）

任务专属记录：[验收文档](../acceptance/runtime-isolation-promotion.md#自动验收)。覆盖 RP1-A、RP2-A、RP3-A、RP4-A、RP5-A、RP6-A、RP7-A，保留原范围等待重定，不启用本任务。原静态检查只用于规划修订，不证明这些自动项通过。

重新启用时先固定实际范围、前置、构建/测试名称及环境，再在既有 driver/native/evidence/testenv 内补齐最小版本/维护驱动，登记 runtime 场景与精确验收项断言。以下入口为规划新增，尚未实现/登记；不得按表提前执行或占用生产目录。

| 验收项 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- | --- |
| [RP1-A、RP2-A、RP3-A、RP4-A、RP5-A、RP6-A、RP7-A](../acceptance/runtime-isolation-promotion.md) | L3 / runtime / runtime.promotion（待新增） | Windows、两独立 clone/home、固定产物、当前格式、受控后端；`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.promotion' '-out=<本次空临时目录>'` | 重新规划准入、驱动/场景实现/登记及能力准备后 | 每项原身份、选择/恢复与拒绝断言通过；报告完整，清理成功；不以安装/调试隔离替代版本/提升证据 | 临时 summary.json、scenarios/runtime.promotion/；本任务专属验收文档逐项结果 |
| 必要回归（非全部产品验收） | 实际受影响 owner + L1 | 重定时选择相关入口的精确定向命令；Go 接线变化时 `go run ./cmd/test-feedback -level=L1` | 改动完成后一次；新影响才补跑 | 实际命中、无受影响消费者回退 | 各自临时目录与本任务记录 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 当前草案交付前 | 导航/身份/格式一致，延后状态明确 | 本次静态输出 |

需要真实原会话/后端能力时在重定阶段明确是否复用原证据或增加最小 live 子场景，并固定 chain/权限/命令与清理；controlled 不证明原生权限。默认不选 Full、完整 L2/全部 live 或无关平台组合。当前只是改 Plan，不执行上列产品入口。

执行自动验收及在任务专属文档登记实际源码/摘要、命令、逐项结果/证据，并同步 execution-map 属于重新启用后的交付步骤。Plan 不存实时通过状态；人工项如需新增按允许范围集中后置，关键先验和真实失败仍阻塞。
