# SteerRunner 拆分删除审计

归属：[接入验收](steerrunner-cutover.md)。执行 owner 切换及物理删除已实施，最新产品真机和最终固定版本回归尚未通过；本轮交付两仓草稿 PR，不代表跨仓全部验收完成。

已删除 SteerLine 根 `internal/execution/*.go` 的 actor、worker、Directory、原生输入/恢复/交付推进；app 的旧 Directory 构造、原生查看器与直接会话/日志读取；SQLite 的旧执行、操作、轮次、问题/回答、conversation 写者和对应 schema。Task/DAG、候选绑定、事件副本、唯一交付和服务 checkpoint 留在 SteerLine。

相应旧 in-process worker 集成、app Directory/codec 和 SQLite 原生执行写入专属测试退休。原生组件及其测试已迁入 Runner；SteerLine 新增公开 CLI 子进程、caller outbox、Task 事务/事件读取、输入校验与实际产品 CLI 依赖守卫。退休清单及原文件大小保存在本次 `.evidence/steerrunner-cutover/retired-*.json`。新测试并不自动代表旧全部覆盖等价通过。

后续已删除剩余 adapters/workflow/reliable/conversation 实现及其旧辅助依赖、闲置执行事务端口/重建逻辑。workspace 留调用方 Git 身份解析，publication 留配置校验；二者旧工作区生命周期与 PR 查询 owner 也已删除。独立 EXE 受控产品闭环 result-13 已通过。仍待完成：本次 R1 三个本机场景的最终汇总、最新修正后的产品真机验收及两仓 PR。禁止以仅编译或依赖闭包通过替代这些行为验收。

接入主干 #324 后继续保留 TUI 删除，不恢复其依赖、入口或 journal。主干新增 `candidate_client_test.go` 是原候选 TUI 场景改为 client 的版本，仍依赖已迁出的 Codex codec、旧版配置和本仓已删除的 lifecycle/service fixture，因此随原 execution 集成测试退休；不为此恢复旧执行 owner。当前调用方的候选准入、原请求和 TaskDelivery 由 taskrunner/runnerbridge/SQLite 的真实事务反例覆盖，CLI/HTTP 原回执由保留的 CM71/CM72 验证；不宣称旧多候选原生场景已完整等价迁移。`audit-boundary-main.log` 验证当前 Runner 公开依赖与源码负例，完整实现/未验范围以接入验收最新段为准。
