# Plan 通用执行事件与问题边界

任务 ID：62；任务名称：通用执行事件与问题边界；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-01-events.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#61 已交付的需求/注册/v4 快照基础；一个实现 PR。

## 实施顺序

1. 列出实时/重放/恢复的事件和问题消费者，明确当前原始输入到通用事实的唯一解码器与消息提交屏障。
2. 补必要通用值和路由封套，接通两类原生 codec；先迁 unit fixture，再更新生产观察和 WorkerEvent。
3. 切换 worker_events/Decision/回答回执消费者，保持回答资格与业务写集合，删除旧 DTO/解码桥。
4. 列旧测试到新文件/用例映射，检查生产消费者和 import，按本任务最小验收执行。

## 所有者与删除

`worker_types.go`、`worker_evidence.go`、`worker_events.go`、`decision.go`、`codex_worker_evidence.go`、`orca_worker_evidence.go`，以及 contracts 的原生问题字段。

删除 root WorkerEvent 的原生字段、OrcaInboxMessage 别名、workerFlowEventFact 的 CodexAskCall 字段以及仅用于原生解码的父包 helper；仍包含事务的函数改为消费通用问题/事实后保留。平台 DecodeEvent/Question 及相关 unit fixture 随实现归包。

## 验证与命中核对

只运行 execution/contracts 和两类解码实现的定向单元测试；中心选一组 pending ask replay/回答去重/消息持久化失败屏障。问题路由同时覆盖错误用途、版本和超过 64 KiB 的拒绝，配置/生命周期原生字段由后续阶段处理，不把本轮验收扩大为根包全清零。复用 TestAppServerRecoveryReplaysOnlyUnresolvedQuestion、TestNativeOrcaRejectsLateDispatchMessages 和相关 worker receipt 事务用例，迁包后按实际位置运行，不重复全协议矩阵。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerline ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1 -run '^(TestCD62NativeFactsAndQuestionRoute|TestCD62QuestionReceiptBarrier)$'
go list -deps ./internal/steerline/execution ./internal/steerline/contracts
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

这些命令在本次规划中未执行。Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查，实际命中数量与旧到新映射随实现记录。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD62-1 | TestCD62NativeFactsAndQuestionRoute | 已有两类解码/旧 Turn 拒绝断言迁入两个实现包；中心只看到公共事实。 |
| CD62-2 | TestCD62QuestionReceiptBarrier | 迁入现有 pending ask replay、答案去重和失败提交不 ack 的代表断言，中性 fixture 检查中心屏障。 |
| CD62-3 | 源码与依赖审查 | 搜索全部 WorkerEvent/Decision/回答消费者，确认原生字段、旧 decode/路由桥无生产调用。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

#63 消费已接通的通用事件、问题正文与严格路由封套；#64 消费明确的 started/accepted/settled 事实和消息消费 token。本任务只锁定自身路由数据，不预建完整实现状态机。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。
