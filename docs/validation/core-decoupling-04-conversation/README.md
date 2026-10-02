# CORE-IMPLEMENTATION-DECOUPLING #65 验收记录

任务：会话采集与宿主交互实现隔离。基线：`84097f3d0856d7b5d16036a53f6ed82016fb3593`（本地 `HEAD` 与 `origin/main` 相同）。本工作树保留未提交候选改动；没有创建分支、提交或 PR。

## 所有者与删除

| 职责 | 交付位置 |
| --- | --- |
| 来源 DTO、mapper/source 端口、宿主观察/输出端口及持久动作值 | `internal/steerline/contracts`、`internal/steerline/execution` |
| Codex 原生 history、journal 发现/重放、映射、绑定和来源身份核验 | `internal/steerline/codexworker` |
| Orca terminal 事实、probe 文件解释及宿主动作执行/对账 | `internal/steerline/orcaworker` |
| 来源预算轮转、通用事件写入、cursor 随最终成功写批次提交、中心授权和人工门槛 | `internal/steerline` |
| 原生 history 到公共 Service 的接线验收 | `internal/steerlineintegration` |

删除 root 的 `codex_conversation.go`、`codex_history_port.go`、`codex_history_test.go` 和平台专属 `worker_environment_orca.go`。通用宿主桥接位于 `worker_environment_host.go`。Service 不再持有具体 mapper 或按 backend 选择映射；它使用实现提供的通用 mapper 和来源能力。来源 mapper 每批从已提交事件重建，失败时候选 cursor 与候选 mapper 状态一并丢弃。宿主文件/terminal 变更通过持久生命周期动作执行。

会话事件从版本 1 改为版本 2，新增必填通用来源角色 `canonical` / `fallback`；校验拒绝缺失/非法角色，查询拒绝旧事件格式。SQLite schema 仍为 7，未增加旧格式读取或迁移路径。产品默认构造仍由当前 Service 默认构造器提供，按路线图由 #66 外移。

## 旧到新测试归属

| 原断言 | 当前入口 | 本地结果 |
| --- | --- | --- |
| Codex mapper 的 session/turn/phase/final、原生 JSONL 两轮及失败/取消映射 | `codexworker/conversation_test.go` | 通过 |
| history cursor 重试、晚到 final、错误 workspace/替换来源、host version capability 请求、cursor 大小限制 | `codexworker/conversation_adapter_test.go` | 通过 |
| 失败写入不推进 mapper/cursor、来源预算轮转、history batch/checkpoint 同事务 | `core_decoupling_65_test.go` | 对应安全子用例通过 |
| query 的旧版本/缺失来源角色拒绝、分离 Reader 与有界分页；SQLite 的只读来源范围 | `query/conversation_test.go`、`sqlite/conversation_test.go` | 通过 |
| terminal wait 保持为宿主事实；probe 内容纯生成、durable 写入与只读核验；固定原目标动作 | `orcaworker`、`execution` 测试 | 通过 |
| 公开 Service 的 native history 绑定与原决策门槛 | `internal/steerlineintegration/core_decoupling_65_test.go` | 编译通过，运行受 Git 子进程限制 |
| ER1/ER3/ER4 和 native source replacement 原断言 | 保留原中心业务断言，收于 `TestCD65HistoryAndPreparation` / `TestCD65SourceCommitAndHostGate` | 本机运行未到达断言，见验证限制 |

## 已执行验证

以下命令均使用工作树内的临时 Go 构建缓存 `.tmp-cd65-go-cache`：

```powershell
$env:GOCACHE = Join-Path $PWD '.tmp-cd65-go-cache'
go test ./internal/steerline/codexworker -run '^(TestConversation|TestHistory)' -count=1
go test ./internal/steerline -run '^TestConversation' -count=1
go test ./internal/steerline ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/execution ./internal/steerline/sqlite ./internal/steerlineintegration -run '^$' -count=1
go test -json ./internal/steerline -run '^(TestCD65SourceCommitAndHostGate|TestCD65HistoryAndPreparation)$/(failed_live_source_does_not_advance_committed_mapper_or_cursor|host_fact_becomes_a_center_manual_gate|active_and_historical_source_budgets_rotate|history_cursor_commits_with_the_conversation_batch)$' -count=1
go test -json ./internal/steerline/orcaworker ./internal/steerline/execution -run '^(TestTerminalWaitMapsAsHostFactWithoutBusinessInput|TestProbeMutationRunsAsOneDurableLifecycleAction|TestProbeContentBuildIsPureAndWritesAreExplicit|TestHostMutationRequiresOneFixedOriginalTarget)$' -count=1
go test -json ./internal/steerline/query ./internal/steerline/sqlite -run 'Conversation|conversation' -count=1
git diff --check
```

All listed commands passed. The center conversation store/replay tests covered transactional rollback, exact Unicode paging, attempt isolation, bounded replay, and source-byte checks. The named CD65 subtests in the JSON run reported `run` and `pass`; history cursor failure/retry, active/historical rotation, source commit failure, and host manual gate were exercised. The host run hit terminal wait mapping, durable probe mutation, pure probe preparation, and fixed original-target validation. Query and SQLite conversation tests passed, including explicit rejection of v1 events and missing source roles.

## 未完成验证与环境限制

按 Plan 的 quick 入口、ER1/ER3/ER4 native Git fixture 和 native source replacement 测试没有通过本地完整验收。Windows Git fixtures 在测试断言前启动 Git 子进程时失败：

```text
C:\Program Files\Git\usr\bin\sh.exe: *** fatal error - couldn't create signal pipe, Win32 error 5
fatal: Could not read from remote repository.
```

这是当前工作树运行环境对 Git 子进程的限制；依据协调要求未继续重试。编译和安全的包内测试通过不能替代 quick/native fixture 验收。协调者需在可运行 Git fixtures 的完整权限环境执行 Plan quick，并复核未运行的原生断言后再发布。
