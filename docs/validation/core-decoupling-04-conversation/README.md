# CORE-IMPLEMENTATION-DECOUPLING #65 验收记录

任务：会话采集与宿主交互实现隔离。基线：`84097f3d0856d7b5d16036a53f6ed82016fb3593`（本地 `HEAD` 与 `origin/main` 相同）。执行候选已由协调端提交为 `c941c87`；协调端在原工作区、执行聊天停写后独立复审并修复。当前分支为 `codex/65-conversation-host-isolation`，最终组合验收和 PR 尚待完成。

## 所有者与删除

| 职责 | 交付位置 |
| --- | --- |
| 来源 DTO、mapper/source 端口、宿主观察/输出端口及持久动作值 | `internal/steerline/contracts`、`internal/steerline/execution` |
| Codex 原生 history、journal 发现/重放、映射、绑定和来源身份核验 | `internal/steerline/codexworker` |
| Orca terminal 事实、probe 文件解释及宿主动作执行/对账 | `internal/steerline/orcaworker` |
| 来源预算轮转、通用事件写入、cursor 随最终成功写批次提交、中心授权和人工门槛 | `internal/steerline` |
| 原生 history 到公共 Service 的接线验收 | `internal/steerlineintegration` |

删除 root 的 `codex_conversation.go`、`codex_history_port.go`、`codex_history_test.go` 和平台专属 `worker_environment_orca.go`。通用宿主桥接位于 `worker_environment_host.go`。Service 不再持有具体 mapper 或按 backend 选择映射；它使用实现提供的通用 mapper 和来源能力。来源 mapper 每批从已提交事件重建，失败时候选 cursor 与候选 mapper 状态一并丢弃。宿主文件/terminal 变更通过持久生命周期动作执行。

会话事件从版本 1 改为版本 2，新增必填通用来源角色 `canonical` / `fallback`；校验拒绝缺失/非法角色，查询拒绝旧事件格式。SQLite schema 11 / runtime 16；旧开发数据库和快照明确拒绝，无迁移、双写或 fallback。历史 checkpoint 使用实现拥有的 `codex.history-position` v1 / codec 1 / `source-position` 封套，固定 Project/Execution/Attempt/来源及完整绑定摘要，载荷和整体摘要共同校验，64 KiB 上限。产品默认构造仍由当前 Service 默认构造器提供，按路线图由 #66 外移。

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

## 协调独立复审修复（验收进行中）

- 资源建立前的宿主轮次观察返回暂无轮次；已绑定资源缺失/损坏仍拒绝。首轮 probe 文件操作绑定已提交的准备身份（包括原 nonce/profile/资源），不要求尚未创建的 Dispatch；对账同样核验原目标。
- 宿主读取仅接收独立的命令/nonce/工具证据值，回传候选工具证据；实现不能修改中心 Environment/Blocker。打开原 terminal 校验固定 Decision/版本/blocker 和原 terminal，可定位正在等待授权的原目标，仍不解除人工门槛。
- 历史及 journal 发现携带当前 Attempt；补扫原 workflow 的轮次目录，保留 prepare-session 阶段的历史绑定。来源读取复核完整所属字段，批次在进入事务前核对原执行归属。原探针期限没有增加额外两分钟。
- 原生生命周期状态通知不会被重新解释成 JSONL 或 terminal 会话。mapper 保留输入平台/协议和原 source event ID；候选/发现用的 Process 都是独立值。
- 恢复 8 个原生历史 fixture / 公共 Store+Query 的原断言到 `internal/steerlineintegration/conversation_history_fixture_test.go`，包括两平台两轮、工具交错、取消/失败、晚到 final、缺失/空正文、canonical 选择与 raw/旧 Attempt、绑定跨 Attempt 拒绝。它们收于 CD65 外层命名入口，不新增私有 actor 写接口。
- 公开协议 mock 的只读 stdio 端实现 thread/read、thread/turns/list，读取业务 WebSocket mock 实际保存的轮次；宿主故障包装器转发完整正式 host/mapper 端口。fixture 读写共用锁，不放宽原身份、正文、Dispatch/send/Task/ack 断言。
- 保留产品 launcher 的唯一默认装配入口在既有目录文件中，由 #66 随目录外移；删除原采集实现后产品仍可编译。

完全访问环境第一份组合报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261002T063652.289809400Z/summary.json` 真实失败（363.0s）。它不再受 sandbox Git 拒绝阻断：失败包含初始宿主状态、准备身份、公开 history 接线/夹具缺口，均需实修，无验收豁免。

后续定向诊断已通过中心全部 CD65（127.727s）、原生外层会话/恢复流程（该组局部成功）、公开历史单用例（17.391s），以及恢复的历史 fixture、来源 position 原归属/未知版本/摘要及 pending-anchor 拒绝、宿主动作与实现编译。诊断时源码仍在修复，不能将它们标作最终 SHA 完整验收。最终 head 的组合 quick/JSON 命中还需完成。

### 最终审核补充

来源发现返回的 sources/诊断 writes 也需在写入前匹配中心给出的 Project/Workflow/Execution/已知 Attempt，不能以仅符合 DTO 形状越过归属核验。新中性故障用例证明被拒绝的发现没有泄漏跨项目写入。

此前 root 的 `TestCodexHistoryRealFailedAcceptance`、`TestCodexHistoryNativeAcceptance`、`TestCodexHistoryRealSourceReplacement` 的原可选开关与正文/raw/能力/重开幂等断言保留在外层 `conversation_native_acceptance_test.go`，通过实现-owned public-history 端口及既有公共 Store/Query 接口读取精确的调用者提供的探针绑定。此次只编译它们，不启用开关、不调用真实产品 CLI或发现用户会话。

旧 `bindLocalCodexHistory` 没有当前产品消费者：当前唯一 native 产品任务流注册 local app-server；`SpawnerFor/LocalSpawner` 只有定义和直接测试使用。原始 JSONL 的当前格式 marker/映射/重放仍保留在实现侧；不添加旧 minimal/exec 产品任务或兼容读取。#66 接手唯一默认装配（含 launcher）和遗留装配/辅助定义的实际消费者审查，#67 接手中心业务规则中余下的协议 fixture 归位和最终零关键字守卫。原 source replacement 的实际 Attempt 提交和准备/业务资格断言保持当前 CD65 命中。

完整组合 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261002T075514.734700900Z/summary.json` 在 clean `f828ba94355d5522f029264703207bed22f590d5` 通过，244.35s：core 6/6、rules 90/90、targeted 28/28，未失败/跳过。后续增加来源发现归属拒绝及保留可选 native 入口后，再对最终提交执行同一组合验收；前一通过记录不冒称后续 SHA。
