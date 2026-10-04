# Plan：收敛执行与反馈产品流程

唯一行为依据：[Spec](simplified-product-flow.spec.md)。实施顺序：环境旁路与独立命令 → 规范会话/活动 → 统一单步副作用结果交接 → worker-only 与完整消费者切换 → 最小验证 → 新 PR。

复用现有 actor/workerMessages、query、persistence、codec；不新增通用框架。记录删除/消费者与旧断言到新语义映射；展示主包、全仓生产及含测试净行数。旧行为测试改为新语义，不删除原身份、未知结果、提交屏障及真实交付断言。

## 测试计划

环境：Windows amd64，临时 STEERLINE_HOME，隔离 Git/SQLite/协议 mock/loopback provider。证据归 `bin/simplified-product-flow/`，归档摘要至 `docs/validation/simplified-product-flow/`。

| 级别/组、场景 | 命令 | 时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 定向：环境旁路/反馈拒绝/命令归属 | `go run ./cmd/test-feedback -out bin/simplified-product-flow/environment -- ./internal/steerline ./internal/steerline/console ./internal/steerlineclient -run '^TestSimplifiedEnvironment' -count=1` | 环境消费者切换后 | 新入口实际命中；业务问题/草稿保留，拒绝/旧身份/恢复不补投，summary complete/pass |
| L1 定向：规范来源/副作用调度/发布责任 | `go run ./cmd/test-feedback -out bin/simplified-product-flow/focused -- ./internal/steerline ./internal/steerline/console ./internal/steerlineclient ./internal/steerlineapp ./internal/steerlineintegration -run '^TestSimplified' -count=1` | 源码稳定后 | 相关场景实际命中；数据库单一展示源、非阻塞、intent/receipt/ack、未知/过期/提交失败、worker-only 拒绝旧责任保持 |
| L1 quick | `go run ./cmd/test-feedback -level=L1 -out bin/simplified-product-flow/quick` | 最终一次 | 指定包和 parent/subtest 全部实际 run/pass，0 fail/skip；旧输入升级按断言映射更新，不放宽隔离守卫 |
| L3 controlled：delivery.basic、recovery.waiting-merge-restart | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out bin/simplified-product-flow/controlled` | 完整产品切换后 | helper 在 worker 环境真实 commit/push/create loopback PR；真合并后唯一交付，重启不重发、原身份/回执/资源保留；支持 fixture 的 worker 发布能力需同步实现 |
| 最小原生执行证据 | 按改动后的既有 native 入口选择本机与终端的单轮/恢复代表；精确命令与命中在实施时登记 | 原生启动调度变更后 | 保持原身份/权限、每步屏障及无重复输入；工具或环境不可用如实记限制，不用 mock 认证 |
| 构建/静态 | Windows/Linux amd64 产品构建；`git diff --check`；全 OS/tag 守卫与消费者清单 | 提交前 | 可构建、无第二调用路径、禁用依赖/悬空旧消费者为零 |

不跑 Full：范围以受影响代表、quick 和两项受控交付/恢复验证为主。不默认跑完整 L2/全部 live；原生验证只选调度改变的必要证据。没有新增模型或平台支持。
