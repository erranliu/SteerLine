# worker 环境就绪实现候选验收

日期：2026-09-30。依据：PR154 原范围的 [Spec](../../spec/worker-environment-readiness.spec.md)、[Plan](../../spec/worker-environment-readiness.plan.md)。状态：审计修复及真实服务闭环通过，原实现 PR160 已合入。下述证据可供后续分包/策略迁移复用，不认证尚未实施的 #59 项目策略接线。

## 已取得的证据

- [平台门槛](platform-gate-2026-09-30.json)：Windows 本机 Orca 1.4.209、Codex 0.159.2，在同一真实 agent terminal/incarnation、workspace-write、unelevated、approval=never 下，由命令工具执行读 Git、读输入、写/读/删除临时文件及原子收据。保存了确切 transcript 的 tool-call 和 tool-result、exit 0、nonce/脚本摘要；复用 worker 声明的 relay 后，原 Dispatch 完成原生成功结算。中心没有代执行探针或代写成功收据。
- 最终定向运行通过 39 个顶层用例（含 ER1–ER4、相关原生身份/重启、反馈与写者回归）；CLI 构建及最终 quick 入口通过。未运行全量测试。

ER1–ER4 使用同一 Orca fake 补充故障、重复操作和重启断言；只把它们作为受控回归。包括准备失败、口头成功/缺收据/失败结算拒绝、独立宿主观察、问题与文字保存、原目标投递、PR 阶段保护、原身份重查、原派发回执丢失、固定期限、晚到成功及首次业务启动期限。
- `orcaworker` 回归覆盖真实平台样本解析、受管参数冲突、收据重读/身份拒绝、外部链接拒绝，以及目录信任菜单/父 shell 不作为新输入入口。
- [快速入口摘要](quick/summary.json)记录一次成功运行；最终候选的定向结果单独保存，不以 quick 替代环境用例。

## 真实服务闭环的限制

原 SteerlineTest 已有服务持有仓库锁，验证改用单独克隆和独立 STEERLINE_HOME，没有停止原服务或修改生产数据库。测试克隆首次启动 Codex 出现目录信任菜单。一次早期候选把平台残留的 agentIdentity 当作输入就绪，之后只得到 terminal fallback，准备按原期限进入 waiting_decision；没有派发业务。这次失败暴露了输入入口核验缺口，候选已补上类型化入口检查，拒绝向启动信任菜单或无法核验的父 shell 注入。

独立克隆：`C:\Users\lmas1\AppData\Local\Temp\steerline-pr154-repo-b370dd5c94174d778657b3d8a7bec29c`。未通过的原记录保留在 `C:\Users\lmas1\AppData\Local\Temp\steerline-pr154-live-2353153982`；原 terminal 为 `term_29693900-97a7-44e1-bd46-6eff693e4e90`，incarnation 为 `6b0080f8-9043-483a-aef1-d760a9e3c22b`，Dispatch 为 `ctx_a0671c814e9c`。未知 Dispatch 不自动结算、重派或用替代会话恢复。

本次按用户继续验证的指示，仅为独立测试克隆补齐目录信任（未改 sandbox/approval）。此前的实现候选尚未通过最终命令的真实闭环；本次审计使用同一原 Execution 补齐了这项验证，详见下方记录。

## 最小复测入口

```text
go test ./internal/steerline/orcaworker -count=1
go test ./internal/steerline -run "TestWorkerEnvironmentER|TestNativeOrcaFixedLuna|TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement|TestNativeOrcaRejectsLateDispatchMessages" -count=1
go run ./cmd/test-feedback -quick
```

真机入口 `TestWorkerEnvironmentNativeReadinessLive` 默认跳过。显式设置 `STEERLINE_READINESS_LIVE_REPO`（已注册、已信任的独立测试克隆）、`STEERLINE_READINESS_LIVE_COMMIT`（隔离 smoke Spec/Plan 的固定 commit）、`STEERLINE_READINESS_LIVE_CODEX`（原生可执行文件）和可选 `STEERLINE_READINESS_LIVE_REPORT` 后，运行：

```text
go test ./internal/steerline -run "^TestWorkerEnvironmentNativeReadinessLive$" -count=1 -v
```

成功要求真实准备工具证据与原生成功结算，然后在原 terminal/incarnation 上恰好一个不同的业务 Task/Dispatch，并取得业务工具执行及结算；不发布或合并测试 PR。失败保留原资源/状态，不放宽 sandbox 或 approval policy。

## 审计修复与真实闭环

本次审计确认并修复以下问题：

- 重新检查的轮询可产生新 blocker/Decision；结果提交重新核对原目标、Decision 版本及 blocker，不清除更新的故障。
- 原准备轮的工具初始化失败成为不可被同轮后续成功覆盖的事实；晚到成功经明确接纳后不会因原 deadline 再次误阻断。
- 输入入口检查支持不同 Codex 占位词，排除 busy/hook 画面及父 shell；不把启动入口检查视作工具就绪证明。
- 修复 Go 1.24 声明与 Go 1.25 `os.Root` 方法调用不一致，使用有界读取及逐级 rooted mkdir；包回归、vet 和 CLI 构建通过。
- 每次原会话核验对照冻结 Profile、原启动 Operation、实际命令及 linked worktree 写入根；当前格式拒绝缺收据/工具证据的 ready 记录。
- Orca 的 cursor 会固定早期 terminal fallback，即使随后出现精确 transcript 也不必报告 source_changed。适配层用一次有界无旧 cursor 查询发现新增 transcript，单次刷新断线保留可用旧页。
- 真实 Codex 命令工具 wrapper 可将返回值保存为局部变量并 `store` 后 `text`。解析支持已补证的原样返回形式，拒绝结果改写/变量替换；[真实样本](platform-wrapper-audit-2026-09-30.json)纳入回归。

[真实服务记录](native-live-audit-2026-09-30.json)证明最终受管命令下的工具探针、原子收据、原生成功结算及一次业务派发/结算均通过。原 workflow 为 `f9177c604bddbe728d1425d3f0d90a2d`，Execution 为 `7cdad1492cbc3705cb76fbea5e412e04`；准备 Dispatch `ctx_d148cb51743f`，业务 Dispatch `ctx_4038aece5b9c`。两轮保持原 terminal `term_dbaa38e1-9ef8-4b9b-b771-0ba0b840651b` 和 incarnation `65e5fa7d-f88d-4a0f-84c0-3be032be5fd2`。先前超时不重置原 nonce/deadline；显式重新检查接纳原轮完整成功事实，没有重新探针、领取或启动 agent。

真实入口可传 `STEERLINE_READINESS_LIVE_STATE_ROOT` 继续原隔离状态的对账，不创建替代 Execution。成功后的已结算、由本次中心创建的 agent/coordinator terminal 已按原身份关闭。

[审计定向回归](audit-focused.jsonl)、[审计 quick 摘要](audit-quick/summary.json)与 `orcaworker` 包回归通过；不运行全量测试。目录信任依据[OpenAI 官方配置参考](https://learn.chatgpt.com/docs/config-file/config-reference#configtoml)的 `projects.<path>.trust_level`，仅限此测试克隆。

## 合并主分支后的定向验证

已整合 `4b94d82` 的 worker 生命周期重构及 `4a84aa1` 的执行配置隔离规划。readiness 的准备、派发和观察逻辑移入主分支拆分后的入口；中心 Decision/Operation 修改使用私有 domain 候选与显式提交，保留只读 Runtime 边界。测试在命令处理重新加载记录后取得最新权威对象，本地 sandbox 默认值与 Orca 显式配置分别验证。

ER1–ER4、新故障恢复、冻结资源、原 terminal 对账/重启、worker 显式提交与回滚、项目查询快照/事件批读、本地 Windows sandbox 及 Profile 定向回归通过。`orcaworker`、`codexapp`、`codexworker` 包测试、控制台 Profile/项目查询测试、quick、vet、CLI 构建和 diff 检查通过；未运行全量测试。此次冲突整合未重跑真机入口，前述真实记录对应原审计版本。

PR161 新增的 #59 权限编译/快照接线仍待其前置完成；上述结果覆盖 PR154 的实现及主分支代码整合，不作为新增接线的完成证据。
