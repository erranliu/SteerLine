# Orca 部署重验调用方

2026-10-19，普通开发，Runner 原停止取证由其 worktree `codex/orca-stop-proof` 的 `docs/spec/orca-shutdown-verification.plan.md` / `docs/acceptance/orca-shutdown-verification.md` 维护。本仓仅将公开的逐 Run 未核实诊断保留到部署报错，不新增原生停止动作或放宽资源门槛。

| 项 | 入口与通过条件 | 结果 |
| --- | --- | --- |
| OSV-S01 | `TestStoppedVerificationKeepsRunDiagnosticsAndGate`：未结算保持失败并保留原 Run 诊断，完整证明才通过 | 通过 |
| OSV-S02 | `TestDeployment*`、`TestServiceSettlement*`：原身份/摘要/配置/实际锁/本地关闭围栏，历史失败回执保持 | 通过 |
| OSV-S03 | `scripts/test-deployment-stop.ps1`：staged stop 失败才进入重验；重验失败/证明不全阻断替换 | 通过 |

独立调试包由 `scripts/new-debug-release.ps1` 从本仓模板新建。原生取证使用本包 Runner EXE 和真实 Orca PTY，Run 是隔离的当前格式契约夹具，零模型/PR；不能据此声明完整业务流程或原安装现场部署已通过。本次没有启动、替换、重置安装环境。

执行：`go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestStoppedVerification' -count=1 -timeout 120s`：app 28.616s、runnerclient 3.306s 通过；cmd 无匹配，仅编译。`scripts/test-deployment-stop.ps1` 通过。

最终调试包：`C:/SteerlineDebug/dev-20261019-093426-abd001f1-dirty`。公开 Runner EXE 的本机/远端原生探针均通过，最终证据在 `evidence/orca-stop-local-recheck/` 和 `evidence/orca-stop-remote/`；首次本机发现 Orca 关闭回执早于终端清单更新，生产正确拒绝，探针改为有界等待原清单后重验，失败记录保留在 `evidence/orca-stop-local/`。执行资源专项由 Runner 专属验收维护，本仓不增加重复原生场景。正常停服仍可仅结束 Orca 观察；原终端仍活动或身份无法核实时，部署继续拒绝，不自动关闭业务终端。
