# L2/L3 测试依赖边界实施

基于未合并的 #180 建立独立分支与 PR，只包含本次解耦改动。保持同仓、同 go.mod；不修改生产行为和生产合同。

1. 新增测试自有 evidence 层，集中 profile/resource/session/Orca codec 验证，输出小型证据 DTO 和统一校验。
2. 原生可执行文件、配置 codec key、Orca 返回与库存解码集中到 native；查询 reader 调用集中到 driver。testenv/场景仅组织现场和断言。
3. 新增 L1 依赖边界测试并加入 quickPackages。生产检查基于真实 Go 依赖图排除作为检查入口的 fixture 包；E2E 检查启用 e2e tag，限制实现包只能被 driver/native/evidence 导入。合成图验证传递违规，防止“目录同仓”被误判为生产依赖。
4. 更新 E2E README，执行以下最小验证，记录证据，再 commit/push 并创建基于 #180 分支的独立 PR。

## 测试计划

| 级别 | 组/场景 ID | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | dependency.production / dependency.e2e-layers / evidence.identity | 本机，无模型 | `go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1` | 抽取后；实际依赖图通过，传递违规拒绝，模型/权限/原会话与回合证据保持，错误身份拒绝 | validation 目录命令记录 |
| L1 | 固定最小入口 | 本机，无模型 | `go run ./cmd/test-feedback -level=L1 -out=<fresh>` | 接入后；新边界包在 quick-rules 中实际运行，完整最小入口通过 | summary.json |
| L2 | smoke.delivery | controlled，无模型 | `go run ./cmd/test-feedback -level=L2 -env=controlled -out=<fresh>` | 定向通过后；交付、身份读取、清理通过 | summary/scenario |
| L3 | recovery.waiting-merge-restart | controlled，无模型 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -out=<fresh>` | 同上；恢复原会话/权限、交付、清理通过 | summary/scenario |
| 编译 | 全部 E2E 场景 | 本机，不运行场景 | `go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1` | 抽取后；真实/受控场景编译通过 | 命令输出 |

证据目录：`docs/validation/e2e-dependency-boundaries/`。不选 L2 live、L3 live 和 full：生产执行与原生操作语义不变，抽取使用原 codec，证据单测覆盖两条原生链路；本次验证无需额外 token、测试 PR 或全量基线。

## 用户追加：全部 L3 验收

按用户要求在当前候选 `5ebf704` 上验证全部六个 L3 场景。两种环境分别运行；每个真实场景独立运行，Windows 原生资源串行，full access，gpt-6-luna / medium。保留失败证据，定位后才用新现场重跑；runner 不自动重试。L3 使用 loopback provider，不创建外部 GitHub PR。

| 级别 | 组/场景 ID | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L3 | delivery.basic + recovery.waiting-merge-restart | controlled，无模型 | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out=<fresh>` | 先验证两项受控交付/恢复和清理 | `docs/validation/e2e-dependency-boundaries/l3-all/controlled/` |
| L3 | delivery.local-codex | live，full access | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex' -out=<fresh>` | 独立原生交付、固定模型/effort、正数原生 usage、清理通过 | 同目录 delivery.local-codex/ |
| L3 | delivery.orca-codex | live，full access | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex' -out=<fresh>` | 同上，readiness/business 均成功 | 同目录 delivery.orca-codex/ |
| L3 | recovery.local-codex | live，full access | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex' -out=<fresh>` | 原身份/权限恢复，重启与交付轮询不增加 usage，无新模型回合，交付/清理通过 | 同目录 recovery.local-codex/ |
| L3 | recovery.orca-codex | live，full access | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex' -out=<fresh>` | 同上，原 readiness/business 与原终端保留 | 同目录 recovery.orca-codex/ |

不重复 L1/L2/full：本次是用户明确指定的全 L3 验收；此前 L1 和 smoke 的证据保留，当前运行报告独立记录。若需改代码，再选择对应定向回归。

Orca recovery 首轮真实执行成功，但服务重启的旧启动检查要求 writable roots 非空，与 full access 的空 roots 契约冲突。改为比较环境 roots 与已通过 codec/身份验证的原 PolicyResources roots；runtime 和启动命令仍必需，不补旧数据或放宽资源变更。新增 `TestOrcaRestartPreflightRetainsOriginalPolicyRoots`，以真实受控 claim/resource 持久化验证 workspace/full access 启动以及 roots 变更拒绝。

修复后先运行 `go test ./internal/steerline -run '^(TestOrcaRestartPreflightRetainsOriginalPolicyRoots|TestManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput)$' -count=1` 和 `go test ./internal/testboundary -count=1`，无 token；通过后只独立重跑 `recovery.orca-codex`。已成功的其他场景不因这个 Orca 重启检查重复调用模型；首轮失败及 usage 单独保留。

## 用户追加：rebase 最新 main 后完整 L2

先将 #180 的分支 rebase 到 `5bbbfcd`（包含 #187 host/source 隔离），再将 #188 的两笔增量接到新父分支，保留两个 PR 的边界。冲突仅涉及原终端检测测试，使用主干的新 `advanceWorkerPreparation` 入口并保留 codec 的 AgentBound 断言，不恢复旧入口。rebase 完成候选为 `e27735a`。

| 级别 | 组/场景 ID | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | 依赖边界、codec、driver/runner | 本机，无模型 | `go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1` | rebase 后；新接口/配置与抽取层回归通过 | `docs/validation/e2e-dependency-boundaries/rebase-187-l2/` |
| L1 定向 | 原终端准备、空线程首回合/反馈、原权限恢复 | 本机、受控实现，无模型 | `go test ./internal/steerline -run '^(TestWorkerEnvironmentWaitsForOriginalAgentDetection|TestOrcaRestartPreflightRetainsOriginalPolicyRoots|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace)$' -count=1` | 冲突后；新准备入口、原身份、完全访问 roots、同会话反馈通过 | 同目录验证记录 |
| 编译 | 全部 E2E 场景 | 本机，不运行场景 | `go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1` | 真实阶段前；场景与最新主干接口可编译 | 命令输出 |
| L2 | smoke.delivery + smoke.local-codex + smoke.orca-codex | gated，真实阶段 full access，luna/medium，SteerlineTest | `go run ./cmd/test-feedback -level=L2 -out=<fresh>` | 定向通过后；无 token 门槛先通过，两条真实 PR 创建/合并、正数 usage、原生身份和清理通过 | 同目录 summary/scenario/usage |

不重复 L3/full：用户指定 rebase 与完整 L2；已有 L3 历史证据保留，不能当作新主干上的 L3 通过记录。

## 用户追加：rebase #189 后重跑全部测试

将父分支及回归记录 rebase 到 main `a670e9d`（#189 composition）。保留当前配置入口、实现目录和校验；测试 driver 改用 steerlineapp.LoadConfig，边界检查覆盖 steerlineapp，完全访问 roots 校验使用公共执行策略。先编译全部 E2E 包，再跑以下完整矩阵；真实场景串行、gpt-6-luna / medium、完全访问，不自动重试。

| 级别 | 组/场景 | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| 编译/定向 | 全 E2E、适配器/边界 | 本机、无 token | `go test -tags=e2e ./cmd/steerline ./cmd/test-feedback ./internal/testboundary ./tests/e2e/... -run '^$' -count=1`；`go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1` | rebase 后；编译和适配器校验通过 | rebase-189-all/README.md |
| full | 全部确定性 Go 测试 | 本机、三 worker | `go run ./cmd/test-feedback -workers=3 -out=<fresh>` | 与分级测试并行；无失败/遗漏，报告完整 | rebase-189-all/full/ |
| L2 | smoke.delivery、smoke.local-codex、smoke.orca-codex | gated；真实 GitHub SteerlineTest | `go run ./cmd/test-feedback -level=L2 -out=<fresh>` | 门槛先通过；两条真实 PR 合并、正数 usage、清理通过 | rebase-189-all/l2/ |
| L3 | delivery.basic、recovery.waiting-merge-restart | controlled | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out=<fresh>` | 独立交付/重启、原身份和清理通过 | rebase-189-all/controlled/ |
| L3 | delivery.local-codex、delivery.orca-codex | live、loopback provider | 分别 `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=<ID>' -out=<fresh>` | 各自独立；模型/权限、正数 usage、交付/清理通过 | rebase-189-all/各场景/ |
| L3 | recovery.local-codex、recovery.orca-codex | live、loopback provider | 分别 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=<ID>' -out=<fresh>` | 各自独立；原身份恢复、重启无新增 usage、交付/清理通过 | rebase-189-all/各场景/ |

本轮按用户明确要求选择全部级别；L1 固定包包含在完整确定性回归中，不重复 quick。失败保留原始证据，不以历史通过记录替代本轮结果。证据根目录为 `docs/validation/e2e-dependency-boundaries/`。
