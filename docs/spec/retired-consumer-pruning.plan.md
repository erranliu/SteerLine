# Plan：清理退役消费者

行为依据：[Spec](retired-consumer-pruning.spec.md)。实施：全仓消费者核对 → 删除解析与专属测试 → 删除元数据/版本及展示消费者 → 同步夹具与 quick 选择 → 最小验证 → 统计净行数/提交/新 PR。

## 测试计划

环境为 Windows amd64，临时目录/STEERLINE_HOME、隔离 Git/SQLite、native 协议 fixture；Linux 仅产品编译。证据位于 bin/retired-consumer-pruning/，最终摘要归 docs/validation/retired-consumer-pruning/。

| 级别/组或场景 | 命令 | 时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 定向：文件会话采集、日志 framing、活动镜像 | `go run ./cmd/test-feedback -out bin/retired-consumer-pruning/journal -- ./internal/steerline -run '^TestJournal|^TestConversationJournalReplay|^TestActivityWriter|^TestActivityJournal|^TestOpenActivity' -count=1` | 删除 parser 后 | 原文件字节/UTF-8/CRLF、offset/轮转/预算、实际 conversation mapper 入库与幂等、镜像 seq 保持，命中且 complete/pass |
| L1 定向：固定发布、配置/CLI/TUI | `go run ./cmd/test-feedback -out bin/retired-consumer-pruning/publication -- ./internal/steerline ./internal/steerline/publication ./internal/steerlineapp ./internal/steerline/console ./cmd/steerline ./internal/steerline/integration -run '^TestPublication|^TestRetired|^TestFiveGlobal|^TestLoadProjects|^TestCD66Explicit|^TestTaskCLIProcess|^TestNativeTask|^TestExecutionSettings|^TestOrcaExecutionProfiles' -count=1` | 字段/夹具切换后 | worker-only、无 create 元数据/展示、v5/旧字段拒绝、原 provider/remote/认证不变与例子/发现/CLI 正例保持；actual run/pass |
| L1 quick | `go run ./cmd/test-feedback -level=L1 -out bin/retired-consumer-pruning/quick` | 最终源码稳定后 | 新 owner 选择实际命中、所有组/parent/subtest complete/pass，0 fail/skip；all tags 和依赖边界未放宽 |
| L3 controlled：delivery.basic、recovery.waiting-merge-restart | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out bin/retired-consumer-pruning/controlled` | v6 切换后 | worker 真实 Git/loopback PR 交付；v6 原身份重启/核验/唯一交付/清理完成，旧快照无回退 |
| 编译/静态 | `go run ./cmd/test-feedback -build-only -out bin/retired-consumer-pruning/build`；Windows/Linux amd64 CLI build；`git diff --check` 和退役消费者核对 | 提交前 | 所有包可编译、两平台构建、无旧消费者或悬空 quick 选择 |

不选择 Full/完整 L2/live：这次删除无生产调用代码与派生元数据，执行、原生协议和权限不变；最小受控交付/恢复覆盖快照变化，原生会话与权限的既有确定性 fixture/quick 保留。无需新模型、环境或权限能力。
