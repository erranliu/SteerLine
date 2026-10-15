# SteerRunner 拆分删除审计

归属：[接入验收](steerrunner-cutover.md)。当前为实施检查点，P6 未全部完成。

已删除 SteerLine 根 `internal/execution/*.go` 的 actor、worker、Directory、原生输入/恢复/交付推进；app 的旧 Directory 构造、原生查看器与直接会话/日志读取；SQLite 的旧执行、操作、轮次、问题/回答、conversation 写者和对应 schema。Task/DAG、候选绑定、事件副本、唯一交付和服务 checkpoint 留在 SteerLine。

相应旧 in-process worker 集成、app Directory/codec 和 SQLite 原生执行写入专属测试退休。原生组件及其测试已迁入 Runner；SteerLine 新增公开 CLI 子进程、caller outbox、Task 事务/事件读取、输入校验与实际产品 CLI 依赖守卫。退休清单及原文件大小保存在本次 `.evidence/steerrunner-cutover/retired-*.json`。新测试并不自动代表旧全部覆盖等价通过。

仍待完成：移除测试辅助代码对旧 adapters/workflow/reliable 的依赖，物理删除剩余旧实现；清理无使用者的旧 execution storage ports；固定可获取 Runner 构建；实际独立 EXE 闭环与 R1 五项受控回归、真机验收及两仓 PR。禁止以仅编译或依赖闭包通过替代这些行为验收。
