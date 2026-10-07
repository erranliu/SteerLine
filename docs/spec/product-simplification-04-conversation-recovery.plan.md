# Plan：最终回复采集与原会话恢复

绑定[Spec](product-simplification-04-conversation-recovery.spec.md)及[验收](../acceptance/product-simplification-04-conversation-recovery.md)。

## 实施

1. 删除中心 JSONL 会话重放队列、文件位置/mapper 恢复接口及无消费者实现；保留执行 journal 和原请求对账。
2. Codex 原生历史只映射完成 final、turn 状态及缺口；Orca 终端只保留 wait/gap，无终端正文。简化正文存储为完整 immutable final，删除 raw 表与读取端口；提升 SQLite/会话版本并拒绝旧格式。
3. 恢复删除 new_session 与没有原 session 的人工重新启动，SQLite 写入约束恢复必须保留原 session；核验同 thread 恢复、未知结果和旧进程屏障。
4. 清理退役专属测试，保留共享身份、事务、反馈和交付验证；更新产品设计、接口归属及文档索引。统计同口径生产/测试净行数，不以移动或压缩排版计收益。

## 测试计划

全部原始产物放系统临时目录 `steerline-ps4-*`，最小摘要归专属验收文档。

| 级别 / 场景 | 环境与命令 | 时点 / 通过条件 / 证据 |
| --- | --- | --- |
| 定向 / PS4-01–03 | Windows 临时 SQLite、可控原生协议；`go run ./cmd/test-feedback -- ./internal/execution/adapters/codex ./internal/execution/adapters/orca ./internal/storage/sqlite ./internal/product/query ./internal/console -count=1` | 修改后；完整 final、拒绝过程、分页/摘要/隔离、原会话限制通过；临时 directed/summary.json |
| 定向 / PS4-03–04 | 原 execution/recovery 与 integration 最小关联用例，使用 test-feedback 的 `-run` 选择 | 接线后；同 thread、旧进程/未知动作屏障及原反馈通过；临时 recovery/summary.json |
| L1 / 既有固定组 | `go run ./cmd/test-feedback -level=L1` | 接线完成后一次；passed/complete，所有固定组通过；临时 l1/summary.json |
| L3 / recovery.waiting-merge-restart | controlled；`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart'` | 定向及 L1 通过后；原身份/唯一交付/后继与清理通过；临时 controlled/summary.json |
| 编译/静态 | `go test -tags=e2e ./tests/e2e/... -run '^$'`、`go build ./cmd/steerline`、文档链接、`git diff --check` | 最终；无编译/链接/格式错误，编译不充当运行证据 |

不跑 Full/完整 L2：没有新增交付能力或平台环境。原生读取 final 的接口和原 thread 恢复协议保持；如实际改变原生请求协议，补最小 native 验证并记录，controlled 不作为 native 证明。
