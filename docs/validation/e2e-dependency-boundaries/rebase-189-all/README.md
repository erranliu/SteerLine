# Rebase #189 后全部测试（2026-10-02）

main `a670e9d`（包含 #189 composition）已进入父分支和回归分支。父分支保留备份 `codex/backup-tiered-before189-20261002`；回归分支保留备份 `codex/backup-regression-before189-20261002`。本轮未合并 PR。

Rebase 适配保留主干的 version 2 全局配置、PR provider、注入实现目录及新 composition 校验。E2E driver 使用 steerlineapp.LoadConfig；依赖边界覆盖 steerlineapp；完全访问 roots 校验使用公共执行策略。修正旧测试的 verifyFixedExecutionProfile 参数。L2 构建候选 `7dfef22`，完整回归及全部 L3 候选 `b01b222`，两者生产代码相同，差异为两处核心测试的编译适配。所有场景均独立现场，真实执行 gpt-6-luna / medium，完全访问；失败未自动重试。

## 结果

| 入口 | 结果 | 执行时间 | 证据 |
|---|---|---|---|
| 三 worker 完整 Go | FAIL，complete=false | 804.0s，另准备 5.1s | full/summary.json、full/results.json |
| 完整 L2 | FAIL，2/3 场景通过，cleanup_complete=true | 264.83s | l2/summary.json |
| L3 controlled 两场景 | PASS，2/2 | 171.85s | controlled/summary.json |
| L3 delivery.local-codex | PASS | 140.75s | delivery.local-codex/ |
| L3 delivery.orca-codex | FAIL，服务预检拒绝 | 43.61s | delivery.orca-codex/ |
| L3 recovery.local-codex | PASS | 146.00s | recovery.local-codex/ |
| L3 recovery.orca-codex | FAIL，服务预检拒绝 | 40.66s | recovery.orca-codex/ |
| 带 e2e tag 基础设施 | 6 包通过；15 个顶层测试通过，1 个 opt-in sandbox 测试 skip | 无 token | infra/results.json |
| console 修正编译后单独回归 | FAIL，62 通过、3 失败 | 无 token | console/results.json |

场景汇总：L2 2/3，L3 4/6。全部场景清理完成；没有遗留本次服务或 Orca 资源。L2 无 token 门槛通过后才进入真实阶段。本地链路创建并合并真实 [SteerlineTest PR #41](https://github.com/erranliu/SteerlineTest/pull/41)，本次 base/head refs 由测试清理。Orca 三场景均未进入模型执行阶段，usage 未验证，不将缺失 usage 视为账单零消耗。

已验证原生累计 tokens：L2 local **79,504**，L3 delivery local **80,561**，L3 recovery local **95,066**，共 **255,131**。每个会话只取最终累计样本，不重复相加。recovery local 的 before/after-restart/final/cleanup 均为 95,066，重启和交付轮询没有新增 usage。各 usage JSON 保留模型、effort 和原生 session/cwd 证据。

## 当前失败

1. 三个 Orca 场景在 `internal/steerlineapp/directory.go:227` 被 `workspace-write` 限制拒绝：`terminal host requires workspace-write and an empty agent profile`。这是服务预检失败，尚未测试到 readiness、业务交付或重启。新 composition 的 terminalReadinessRequired 也仍只检查 WindowsSandbox；后续修复完全访问时应同时核对两回合准备条件。原始失败保存各场景的 preflight-error.txt。
2. 核心三个 worker 都在 `internal/steerline/conversation_sources.go:122` 的历史读取路径发生 nil pointer panic，未完成剩余测试。不能将未执行测试算作通过。核心枚举共 309 项，原始日志有 46 个顶层终态（36 pass、6 fail、4 skip），其余覆盖未完成；core-2 另有子测试失败及未匹配的父 run。完整失败、断言和遗漏见 full/results.json 与 full/summary.json。
3. 其他包原始日志：顶层 207 pass、4 fail、4 skip；console 在完整运行时编译失败，34/35 个其他包有终态。Go 1.26 的 build-output/build-fail 记录仍触发严格报告解析的 missing package/action，导致额外报告不完整；构建错误本身和核心 panic 均是实际失败，不能归因于报告格式后忽略。
4. console 编译失败来自冲突遗留的未使用 `bound` 绑定和 publication import。在父分支 `7b5d38f` 修正这两处声明后，回归分支候选 `6cf31f0` 单独执行整个 console 包：仍有 `TestTaskPreviewKeepsOneIdentityAndShowsFixedExecutionPlan`、`TestNativeTaskAndItsRunShareOneListRow`、`TestFileQuerySourceReadsCombinedProjectView` 三项行为失败。没有修改生产行为或放宽断言；不以这次定向运行覆盖完整运行的失败证据。
5. 其他包四项顶层失败：`TestWaitServiceCommandResolvesIdentityAndKeepsPolling`、`TestTaskCLIProcessUsesNativeAuthorityWithoutInitializingMissingRead`、`TestDerivedRunDirIsStableAcrossWorktrees`、`TestNativeConfigRejectsASecondRunDir`。详细诊断和核心六项顶层失败列在 full/results.json。

## 命令和证据

先通过 E2E 包编译、依赖边界/codec/driver/runner 定向测试；核心包编译适配后启动完整入口。完整 Go 与分级场景并行，原生场景串行。

```powershell
go test -tags=e2e ./cmd/steerline ./cmd/test-feedback ./internal/testboundary ./tests/e2e/... -run '^$' -count=1
go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1
go test ./internal/steerline -run '^$' -count=1
go run ./cmd/test-feedback -workers=3 -out=<fresh>
go run ./cmd/test-feedback -level=L2 -out=<fresh>
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex' -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex' -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex' -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex' -out=<fresh>
go test -json -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1
go test -json ./internal/steerline/console -count=1
```

所有原始临时目录见 runs.json。归档 summary、scenario、usage、筛选的断言/构建诊断，不复制认证、私有数据库、原始 prompt 或 dispatch capability。L1 包已由完整入口选择，但因核心 panic 未完成，不能宣称本轮 L1/full 全绿。
