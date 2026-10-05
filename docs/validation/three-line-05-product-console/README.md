# #77 当前实施证据

实现基于合入 #76 的 main（PR #219，merge `4288c43b`），位于 `codex/product-console-77`。共享读取端口、产品 DTO/客户端/query 和 console 已切到实际消费者；全部迁移路径、接口及测试归属见 [ownership.md](ownership.md) 与 [test-mapping.json](test-mapping.json)。

最终接口迁移后的定向结果：

- [product-02](product-02/summary.json)：5 个 packages、33 个 test instances 全通过，覆盖 product API/client/query、图投影与 storage ports。
- [console-02](console-02/summary.json)：完整 `internal/console`，1 个 package、78 个 test instances 全通过。
- [app-query-02](app-query-02/summary.json)：真实 app 项目快照和 profile digest 绑定，2 个 test instances 全通过。
- [sqlite-snapshot-01](sqlite-snapshot-01/summary.json)：SQLite pinned transaction 与 partial read，3 个 test instances 全通过。
- [boundary-01](boundary-01/summary.json)：36 个边界测试实例全通过，包括全 tag 源扫描和 Windows/Linux 依赖图。
- [wiring-02](wiring-02/summary.json)：真实 app/client/SQLite/受控后端候选流程，2 个测试通过。
- [l1-02](l1-02/summary.json)：9 个固定组、356 个 test instances 全通过，0 fail/skip。

早期 [DTO 准备报告](dto-preparation-01/summary.json) 保留为历史证据，不替代真实接线。`console-01` 的一次失败是 console 目录迁移后仍使用旧的三层相对路径读取仓库 fixture；修正路径后，`console-02` 的全包测试通过。首次 `l1/` 失败是 runner 固定 package 清单仍包含迁出目录，且 app/client 边界 fixture 仍读取旧目录；更新 `cmd/test-feedback/scoped.go` 和边界 fixture 后，`l1-02/` 全组通过。早期临时模块不作为最终交付证据。

## 自查结论

真实接线与 L1 均通过。产品请求、执行/会话身份、revision/profile digest 由相应 DTO/接线断言验证；原实现已提交 PR221，独立复审修复/集成记录见下文。

## 协调独立复审

共享集成、确认问题/修复、首轮失败及最终证据见 [coordinator-review](coordinator-review/README.md)。原 worker 报告保留真实历史候选和结果。
