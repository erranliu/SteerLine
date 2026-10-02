# 通用执行生命周期验收记录

任务 #64；依据 [Spec](../../spec/core-decoupling-03-lifecycle.spec.md)、[Plan](../../spec/core-decoupling-03-lifecycle.plan.md)。实现 [PR185](https://github.com/erranliu/SteerLine/pull/185) 的最终审核评论绑定候选 head/base、实际报告与合并回执；完整运行状态在仓库外。

## 所有者与消费者

`execution/lifecycle.go` 拥有固定上下文、闭集动作、原 Operation/请求/目标/Execution/Attempt/代次/输入绑定身份、纯值回执和观察/恢复/工作区端口。中心 `worker_lifecycle*.go` 与既有 worker/commit 路径拥有资格、intent、私有候选验证、原子附件提交、推进、消息消费后 ack、人工恢复门槛及唯一 TaskDelivery。三种实现拥有原生操作/观察/结算/对账/清理，不依赖中心写者。local/native 共用 workspace 原绑定 Git 检查；remote 保留远端路径及原 dirty 事实，不按中心 OS 改写。

正式 `NewServiceWithLifecycleResolver` 接收纯值目录，唯一默认目录最终外移归 #66。保留 thread 绑定提交后才首轮、资源回执提交后才派发、消息内容与消费提交后才 ack。未知 start/reply/send/ack 只读对账原操作，不另造 mutation ID。process exit、detach、stop、settlement、cleanup 分开；远端取消仍 unsupported，清理失败不撤销 TaskDelivery。

## 协调复审修复

`6a48136`、`ec6b89e` 修复原操作身份/输入绑定前值、错误候选泄漏、跨执行 ack、消息提交失败不得 ack、Dispatch release 定位、完整 terminal/incarnation、pending paste screen 来源、准备/业务用途、宿主等待 phase、未知准备人工 recheck、重开先核验身份。launch command 从实现返回，中心不解码原生状态。

执行 session 补齐外层完整流程并修复正式构造缺少 ManagedProcessHost、mutable/fixed 跨域摘要比较、投影刷新后的 pending-operation 写集合、丢 ack 立即只读对账。协调随后保留同一固定摘要域的 Claim 原子复核，修复测试共享配置 payload slice，取消按 host 预填计划资源身份。资源只在回执提交后附加。

协调最终把恢复预览与工作区检查接入同一生命周期端口，删除协议 switch、旧 remote resource bridge、旧回复键恢复和旧操作门禁类别，增加中性原进程/工作区回归。外层启动立即注册关闭；提前失败或已读退出事件也关闭服务/Reader。

## 删除与剩余交接

删除 root `codex_worker.go`、`codex_worker_reply.go`、`codex_worker_settlement.go`、`codex_feedback_hooks.go`；三个平台协调文件改为通用 `worker_lifecycle_async/feedback/recovery.go`。删除 `orca_worker_dispatch/observation/terminal/workspace/relay/settlement.go`、`orca_remote_service.go`、`orca_remote_support.go`、`orca_execution_operations.go`、`orca_feedback_hooks.go` 及旧宿主反馈操作桥。相应旧回调/工厂/原生 lifecycle 解码消费者与重复 remote fixture 随消费者删除。

`orca_worker_prepare.go` 只余固定 agent 命令编译；`worker_environment_orca.go`、`implementation_bindings.go` 的视图仅供宿主证据/history。`worker_completion.go` 准备输出采集属于宿主证据消费者。这些与 conversation/history/source/mapper、准备文件/open-terminal 由 #65 完整迁移。唯一 registry/配置工厂与产品构造由 #66 移走。#67 迁剩余平台测试并加 Windows/Linux 与源码依赖守卫。本轮不宣称主包关键字/平台依赖清零。

## 旧到新断言映射

| 原入口/断言 | 当前入口/所有者 |
| --- | --- |
| root `TestAppServerServiceTwoTurnsPersistNativeReceipts` | outer `TestCD64OriginalSessionReconciliation/app_server_two_turns`：公开配置、Claim、RunWithDispatch、查询/回答；原 Session/Process，一个 prepare、两个 start 和完成 Turn，无 TaskDelivery。原 root 用例删除。 |
| root `TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement` | outer `native_original_session_manual_gate`：readiness + business、原 terminal/incarnation/session/workspace/Run/Task/Dispatch；消费后 ack，丢 ack 一次写入后对账；重开无新 Dispatch/reply/send；公开回答后原问题结算，无 TaskDelivery。原私有 actor 用例删除。 |
| remote 两轮/结算、丢 start、丢 reply+ack、替换 session | outer `remote_protocol_test.go` 四个 Protocol 子用例，唯一假进程在 `remote_fake_test.go`；不伪造准入成功。 |
| 消息提交失败不得 ack、资源提交失败不派发 | root CD64 intent 入口及 `core_decoupling_64_review_test.go` 中性失败注入。 |
| 宿主等待、deadline/迟到成功、原准备重开不重发 | root CD64 intent 入口复用 `worker_environment_test.go` 既有断言；来源/解释归 #65。 |
| 原 turn/Dispatch relay capability | `orcaworker/relay_binding_test.go`、`relay_lifecycle_test.go` 实现端口。 |
| provider PR 分支身份、closed-unmerged 保留会话 | `worker_pr_binding_test.go`，共享业务规则和 HTTP provider fixture。 |

共享原生测试进程 fixture 在 stdlib-only `testprotocol`；外层使用正式 Command/Query，不导出任意中心写入口，不复制纯业务矩阵。

## 格式与实测边界

SQLite schema 10、runtime/checkpoint schema 15；native execution-state codec 9、local binding 1、supervised 使用同一 native state codec 9。拒绝旧开发格式，无迁移/双写/fallback。

历史报告位于仓库外 `%LOCALAPPDATA%/Temp/steerline-test-feedback`：

- `20261001T213835.717165900Z`：`ec6b89e` quick 完整通过，core 6/rules 87/targeted 33；当时外层完整流程尚缺，未据此合并。
- `20261001T235533.314495800Z`：`507a7f0` 默认并发失败，core 6/rules 87 通过，targeted 29/31；app-server 上下文超时，失败时数据库句柄清理也失败。root/native/remote 通过，无豁免。
- `20261002T000328.055843300Z`：同一 `507a7f0` 串行最小命令 passed/complete，core 6/rules 87/targeted 31，186.32s，无失败/跳过/未命中。不能仅凭串行通过宣称已定位并发超时根因。

最终协调修复后的 source SHA、报告/命中/依赖/diff 检查记录在 PR 最终审核评论。历史结果不代替最终 head 验证。仅运行最小入口和问题定向诊断，无全量测试。local/native 是隔离协议 mock 与临时 Git/SQLite，不调用产品 CLI，不证明真实平台权限/双机认证；remote admission 拒绝断言保留。
