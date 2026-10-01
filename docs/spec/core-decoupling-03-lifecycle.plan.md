# Plan 通用执行生命周期与副作用接线

任务 ID：64；任务名称：通用执行生命周期与副作用接线；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-03-lifecycle.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#63 整体交付；一个实现 PR。

## 实施顺序

1. 逐个梳理外部动作的 intent 保存、变更、结果提交和下一步屏障，按实际消费者固定最小请求/结果端口。
2. 以一个已存在路径迁通一步操作/原绑定回执，再接通其余 native/remote 路径；每次迁移保留原单写者和同事务附件。
3. 把启动、反馈、观察、恢复和精确清理接到统一中心推进，取消生命周期注册中的 Service 回调与平台特例；异步结果验证原 Operation/请求摘要/目标/代次/输入绑定前值，失败提交不推进下一步。
4. 搬迁相应平台 unit fixture；中心失败注入改用中性实现，核对原多轮/恢复用例仍有实际消费者，删除平台协调旧入口。

## 所有者与删除

`codex_worker*.go`、`codex_feedback_hooks.go`、`orca_worker_prepare/dispatch/observation/terminal/workspace/relay.go`、`orca_remote_service.go`、`orca_execution_operations.go`、worker_execution/resume/recovery/feedback/events、service_workers 及 registry 的 Service 回调；`orca.go`/`orca_remote_adapter.go` 的原生 I/O 调用。

当场删除 root 的具体 lifecycle 方法、codex/orca feedback hooks 和 operation factories；保留的中心逻辑改为通用 worker_* 协调。orca_execution_operations 与 native/remote 分派不再留在主包；具体工厂最终外移之前只有一个明确的组装入口。root 平台命名文件里如含 history/宿主部分，按职责拆出并明确交 #65，不能复制或以平台 shim 留空文件。

## 验证与命中核对

选取现有 app-server 两轮/receipt barrier、worker_commit_boundary 的提交失败、native 完整恢复和一个 remote Protocol 丢失回执组。实际名称/路径在迁移时列明映射；对不同实现重复的纯业务矩阵只保留中心一份，remote 不走虚构 Claim 成功。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerline ./internal/steerlineintegration -count=1 -run '^(TestCD64IntentReceiptAndRecoveryGate|TestCD64OriginalSessionReconciliation)$'
go list -deps ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

这些命令在本次规划中未执行。Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查，实际命中数量与旧到新映射随实现记录。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD64-1 | TestCD64IntentReceiptAndRecoveryGate | 复用 worker receipt/绑定附件提交失败断言：未保存 intent 不发送，提交失败不越过下一步，旧 unknown 只读对账。 |
| CD64-2 | TestCD64OriginalSessionReconciliation | 在 #66 前外层测试经本轮正式端口注入构造服务，root 唯一默认目录仍调用同一构造，不提前建设最终应用默认组合。外层分别命中 app-server 两轮、native 原会话人工恢复与 remote Protocol 身份/回执断言，不重复中心纯规则矩阵。 |
| CD64-3 | 源码与端口审查 | 枚举生命周期所有生产入口及注册回调，确认无 Service 回调或变相可写中心端口，旧生命周期平台操作桥和 #63 临时 lifecycle 解码消费者删除；history/宿主消费者列明交 #65。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

#65 获得不需要平台类型的准备/观察/消费/宿主操作交接；#66 获得不接可写 Service 的实现构造接口。原生命周期不形成新引擎。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。
