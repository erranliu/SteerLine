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

## 协调复审进度（2026-10-02，尚未交付）

PR185 的执行 session 已停写。协调 session 已 rebase 到 `7e79a9b3151290397bfdff4ef000476c384f7c1b`，正在独立审核与修复；活动 roadmap 的 #64 尚未移除，#65 尚未启动。本段不是完成或合并认证。

已确认并修复：

- 生命周期 Operation 持久保存原 Execution/Attempt、目标、配置/资源代次与输入绑定摘要；恢复不再用当前记录重造旧操作身份，异步结果核对当前绑定前值。回执先在私有候选中完整验证，错误不能污染后续诊断提交。
- ack 恢复只处理当前 Execution；消息提交失败不生成 ack intent。资源回执身份拒绝不再被等待决定的成功返回吞掉。
- native/remote 实现拥有原生生命周期解释；主包三个 worker 文件拆为通用 lifecycle 协调，删除无人调用的 remote reply/client 桥及宿主反馈原生路由桥。启动命令由实现作为通用事实返回，中心不再为此解码原生状态。
- remote release 通过 Dispatch ID 查原 turn；native cleanup/reconcile 使用完整原资源身份，pending paste 对账核验原 terminal/incarnation 和实际 screen 来源。
- 工作轮用途固定为 business/readiness。宿主动作保留中心接收路径，延迟文字使用 status 发送端口，不套用恢复出来的问题路由；旧 send/reply 仅对账原生命周期 Operation。
- 准备回执丢失后的只读确认保留人工 recheck；重开原生观察先完成恢复身份分类；worker 开始事实不得覆写宿主等待 phase 或接受中心 HostTarget 答案。

持久格式变为 SQLite schema 10、runtime/checkpoint schema 15；拒绝此前开发格式，没有迁移、双写或 fallback。执行 session 的先前记录中的 schema 9/runtime 14 已被本轮修复取代。

### 实际测试迁移映射

| 原断言 | 当前承载与所有者 |
| --- | --- |
| root app-server 两轮、失败 session/turn 回执与 worker_commit_boundary | root CD64 聚合入口调用原断言；中心提交屏障只有一份 |
| root native 原会话恢复/问题/丢 ack/丢 reply | root CD64 原会话入口；接入通用生命周期及原恢复人工门槛 |
| 删除的 remote service 文件中的四个 Protocol 回归（两轮/settlement、丢 start、丢 reply+ack、替换 session） | `internal/steerlineintegration/remote_protocol_test.go`，归 outer CD64 入口；唯一 remote 假进程随消费者移动到 `remote_fake_test.go`，不虚构 remote Claim 成功 |
| 删除的 native relay 原 turn/Dispatch capability 断言 | `internal/steerline/orcaworker/relay_binding_test.go`；直接调用实现的 `ReadDoneRequest` |
| 删除的消息提交失败不得 ack | `core_decoupling_64_review_test.go` 的中性中心失败注入，归 CD64 intent/receipt 入口 |
| 删除的 provider PR 分支身份/closed-unmerged 保留会话断言 | `worker_pr_binding_test.go`；共享业务所有者，真实 HTTP provider fixture |
| 宿主等待保留问题/文字、准备 deadline/迟到成功/重开不重发 | 保留原 `worker_environment_test.go` 断言，迁移异步 observer fixture 的接线与完成等待，纳入 CD64 聚合入口 |

### 验证与未完成项

诊断已实际通过：中心新增回执/恢复隔离回归；外层四个 remote Protocol 回归；remote released Dispatch 只读对账；native 原会话用例；provider PR 绑定与 relay 原身份用例；宿主 ER2 及 ER4 的失败定位修复。没有跑全量测试。

第一次协调 quick 报告 `20261001T204916.682627800Z/summary.json` 为失败：quick-core 6/6、quick-rules 87/87 通过，targeted 29/31，native 资源准备受初始计划分支与真实资源分支的边界影响；已修复，仅在资源实际绑定后固定其分支。该报告期间又有后续代码修复，不能作为最终候选认证。正在以稳定后的源码重新运行原 Plan 命令，最终 JSON/命中及 head 另记。

曾用宽泛 `TestLifecycle.*` 诊断误匹配服务启动旧用例，它们在 config admission 被 publication policy 拒绝；`41773e8` 的协调工作区上单个相同用例也失败。本轮无基线豁免；这些结果不代替必需验收。

合并前仍须完成剩余完整 diff/生产入口审核、外层 local/native 实际流程覆盖归属核对、所有者/删除清单与 #65/#66 精确交接、最终命中记录与预期 head 合并门禁。外层当前构造/选步用例不能单独认证原会话恢复。#65 接管 history/source/mapper 与宿主证据、准备文件/open-terminal；#66 接管唯一内置目录/default 组合。这里只明确剩余职责，不把未完成迁移记为完成。

第二次协调组合报告 `20261001T211702.780936400Z/summary.json` 仍为失败：quick-core 6/6、quick-rules 87/87、targeted 31/33；native 原会话准备在丢资源回执后保留了 PendingCommand，但 recheck 拒绝继续同一准备 Command。该问题已修复，随后该原会话单例通过（33.025s）。宿主 ER2 单例及 ER4 deadline/迟到结果单例也已通过（ER4 26.476s）；修复 worker 开始事件覆盖 HostTarget waiting phase，保留完整结算事件后才确认测试的观察完成。该失败组合报告不作为最终 head 的通过证据。

复审中的结构缺口仍需核对：外层构造用例目前仅构造空 Service 并选步，实际 local/native 流程断言仍由 root 聚合入口承载；remote 四个完整协议断言已由外层入口承载。合并之前完成 Spec 规定的外层流程覆盖归属及剩余完整 diff 审核，不以目录/接口存在代替流程命中。活动 roadmap 的 #64 继续保留。
