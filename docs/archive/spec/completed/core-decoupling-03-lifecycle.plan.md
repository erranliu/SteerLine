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
go run ./cmd/test-feedback -quick -- -p 1 -parallel 1 ./internal/steerline ./internal/steerlineintegration -count=1 -run '^(TestCD64IntentReceiptAndRecoveryGate|TestCD64OriginalSessionReconciliation)$'
go list -deps ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；`-p 1 -parallel 1` 串行运行 root 与 outer 包，固定本次最小验收的执行顺序。execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

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

## 实际交付与验证归属

生产生命周期通过 `execution.LifecyclePort` 接通原 actor/workerMessages 与事务：选步、单个已保存操作、只读对账、观察、恢复、工作区检查、停止和精确清理。正式生产注入构造 `NewServiceWithLifecycleResolver` 的唯一默认目录最终外移仍归 #66。

中心失败注入及准备/业务门槛归 root CD64 intent 入口；local 两轮与 native 原会话重开已通过公开 Command/Query/Service 迁至 outer CD64 原会话入口；native 入口同时保留丢 Dispatch/Task/reply/send/ack、替换 incarnation 拒绝与精确清理的原断言，同一入口承载四个 remote Protocol 回归。旧到新映射、修复、版本、失败证据和剩余所有者见[验收记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-03-lifecycle/README.md)。无可写私有状态导出，无虚构 remote 准入成功。

串行参数只限定包和用例执行并发，名称与断言保持。默认包并发实测在 app-server 等待中超时，串行组通过；尚不能仅据此确定根因。外层启动立即注册关闭回调，失败也关闭服务与 Reader。

SQLite schema 10、runtime/checkpoint schema 15；native execution-state codec 9、local binding 1、supervised 使用同一 native state codec 9。拒绝旧开发格式，不迁移、双写或 fallback。#65 接管来源/history/mapper、宿主证据、准备文件及 open-terminal；#66 接管唯一默认目录与产品入口；#67 完成剩余测试归位和隔离守卫。生命周期消费者和原操作桥本轮删除，不延期交给后继。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../validation/README.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
