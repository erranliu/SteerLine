# worker 环境就绪实现候选验收

日期：2026-09-30。依据：[Spec](../../spec/worker-environment-readiness.spec.md)、[Plan](../../spec/worker-environment-readiness.plan.md)。状态：代码候选；真实服务准备→业务闭环未通过，不标记交付完成。

## 已取得的证据

- [平台门槛](platform-gate-2026-09-30.json)：Windows 本机 Orca 1.4.209、Codex 0.159.2，在同一真实 agent terminal/incarnation、workspace-write、unelevated、approval=never 下，由命令工具执行读 Git、读输入、写/读/删除临时文件及原子收据。保存了确切 transcript 的 tool-call 和 tool-result、exit 0、nonce/脚本摘要；复用 worker 声明的 relay 后，原 Dispatch 完成原生成功结算。中心没有代执行探针或代写成功收据。
- 最终定向运行通过 39 个顶层用例（含 ER1–ER4、相关原生身份/重启、反馈与写者回归）；CLI 构建及最终 quick 入口通过。未运行全量测试。

ER1–ER4 使用同一 Orca fake 补充故障、重复操作和重启断言；只把它们作为受控回归。包括准备失败、口头成功/缺收据/失败结算拒绝、独立宿主观察、问题与文字保存、原目标投递、PR 阶段保护、原身份重查、原派发回执丢失、固定期限、晚到成功及首次业务启动期限。
- `orcaworker` 回归覆盖真实平台样本解析、受管参数冲突、收据重读/身份拒绝、外部链接拒绝，以及目录信任菜单/父 shell 不作为新输入入口。
- [快速入口摘要](quick/summary.json)记录一次成功运行；最终候选的定向结果单独保存，不以 quick 替代环境用例。

## 真实服务闭环的限制

原 SteerlineTest 已有服务持有仓库锁，验证改用单独克隆和独立 STEERLINE_HOME，没有停止原服务或修改生产数据库。测试克隆首次启动 Codex 出现目录信任菜单。一次早期候选把平台残留的 agentIdentity 当作输入就绪，之后只得到 terminal fallback，准备按原期限进入 waiting_decision；没有派发业务。这次失败暴露了输入入口核验缺口，候选已补上类型化入口检查，拒绝向启动信任菜单或无法核验的父 shell 注入。

独立克隆：`C:\Users\lmas1\AppData\Local\Temp\steerline-pr154-repo-b370dd5c94174d778657b3d8a7bec29c`。未通过的原记录保留在 `C:\Users\lmas1\AppData\Local\Temp\steerline-pr154-live-2353153982`；原 terminal 为 `term_29693900-97a7-44e1-bd46-6eff693e4e90`，incarnation 为 `6b0080f8-9043-483a-aef1-d760a9e3c22b`，Dispatch 为 `ctx_a0671c814e9c`。未知 Dispatch 不自动结算、重派或用替代会话恢复。

目录信任仍待用户确认/原生处理。最终受管参数增加了严格配置、明确写入根及临时目录排除，尚未取得这份最终命令的真实准备→业务成功记录。平台门槛样本证明真实工具事件和原生结算可取得，不能替代最终服务闭环。

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
