# SteerRunner 拆分删除审计

归属：[接入验收](steerrunner-cutover.md)。当前为实施检查点，P6 未全部完成。

已删除 SteerLine 根 `internal/execution/*.go` 的 actor、worker、Directory、原生输入/恢复/交付推进；app 的旧 Directory 构造、原生查看器与直接会话/日志读取；SQLite 的旧执行、操作、轮次、问题/回答、conversation 写者和对应 schema。Task/DAG、候选绑定、事件副本、唯一交付和服务 checkpoint 留在 SteerLine。

相应旧 in-process worker 集成、app Directory/codec 和 SQLite 原生执行写入专属测试退休。原生组件及其测试已迁入 Runner；SteerLine 新增公开 CLI 子进程、caller outbox、Task 事务/事件读取、输入校验与实际产品 CLI 依赖守卫。退休清单及原文件大小保存在本次 `.evidence/steerrunner-cutover/retired-*.json`。新测试并不自动代表旧全部覆盖等价通过。

后续已删除剩余 adapters/workflow/reliable/conversation 实现及其旧辅助依赖、闲置执行事务端口/重建逻辑。workspace 留调用方 Git 身份解析，publication 留配置校验；二者旧工作区生命周期与 PR 查询 owner 也已删除。独立 EXE 受控产品闭环 result-13 已通过。仍待完成：固定可获取 Runner 构建；R1 五项最终回归、真机验收及两仓 PR。禁止以仅编译或依赖闭包通过替代这些行为验收。
