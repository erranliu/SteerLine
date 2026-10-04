# #77 当前实施证据

开工基线 main `621707d534b62e4b7df015463c53949715ba8be3`，分支 `codex/product-console-77`。
本目录当前是专属页面与端口准备记录，尚不声明 TM05.1–TM05.3 整体通过，也尚未提实现 PR。

已完成独立准备：product/api 的任务、候选、服务页面、历史、会话、图、命令回执与恢复摘要；product/query 的纯页面映射；product/client 的读/命令/连接注入端口。api.Error 只序列化 kind/message，Go cause 留在客户端内部包装并保持 errors.Is/errors.As。

第一轮 [DTO 准备报告](dto-preparation-01/summary.json) 是 Windows 无原生 opt-in 的定向入口：2 个 package，2 个 parent 与 5 个 subtest 共 7 个 test 实例通过，0 fail/skip。此后新增的字段隔离/唯一交付/会话投影断言需在真实集成后另存新运行证据，不覆盖该报告。

控制台、唯一客户端和图规则的迁入版本已经在独立临时模块中准备，CC1/CC2/CC4、原客户端资源/原请求回执代表与图读快照代表已编译并定向运行。临时模块不进入生产路径，不构成最终构造、SQL 同事务、原生输入或生命周期验收；正式消费者切换后按 Plan 重新取证。

共享段采用协调确认的顺序：#76 先独立复审并真实合入，随后本任务消费其实际 main namespace，再定向集成 [ownership.md](ownership.md) 的 app/cmd/runner/存储产品读取与守卫清单。原 execution/model/backend、Config/Service 与 actor 私有重建仍由 #76 唯一修改；无永久 alias、转发、第二 SQL 或业务写者替代计划。

`product/api.JournalCursor` 另已发现为执行日志扫描私有值，消费者限 conversation_position/replay/journal_reader 及其测试；该定义迁移需要协调 #76 或在其合入后指定执行路径，不能在收窄的公开 API 中保留 scanner Pending/Dropping/Anchor 状态。

#38 PR208/PR213 的既有输入/Unicode/草稿/锚点断言随控制台迁移，#39 PR216 的本机原生证据按实际产品接线影响判断复用或定向重验。当前不运行生产服务/任务库，不启动其它任务，不声称 fake 或临时模块证明原生能力。最终 L1、自查与 PR 仍待真实共享集成完成。
