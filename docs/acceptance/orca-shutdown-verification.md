# Orca 部署重验调用方

2026-10-19，普通开发，Runner 原停止取证由其 worktree `codex/orca-stop-proof` 的 `docs/spec/orca-shutdown-verification.plan.md` / `docs/acceptance/orca-shutdown-verification.md` 维护。本仓仅将公开的逐 Run 未核实诊断保留到部署报错，不新增原生停止动作或放宽资源门槛。

**P1 审计修复（历史，正向链路以文末补齐记录为准）：** 首版 Runner 的 PTY 清单成功不能证明 Codex native writer-tree/authentication 退出，该成功结论已撤回。修复后缺少原 owner/tree/authentication 绑定证明的 Codex 继续阻断；不新增远端协议或使用调用方 PID 替代执行主机事实。通用 helper 复用既有 owner/exit/authentication 证明。以下旧包结果仅保留历史，不代表当前 Codex 资源结算资格。

| 项 | 入口与通过条件 | 结果 |
| --- | --- | --- |
| OSV-S01 | `TestStoppedVerificationKeepsRunDiagnosticsAndGate`：未结算保持失败并保留原 Run 诊断，完整证明才通过 | 通过 |
| OSV-S02 | `TestDeployment*`、`TestServiceSettlement*`：原身份/摘要/配置/实际锁/本地关闭围栏，历史失败回执保持 | 通过 |
| OSV-S03 | `scripts/test-deployment-stop.ps1`：staged stop 失败才进入重验；重验失败/证明不全阻断替换 | 通过 |

独立调试包由 `scripts/new-debug-release.ps1` 从本仓模板新建。原生取证使用本包 Runner EXE 和真实 Orca PTY，Run 是隔离的当前格式契约夹具，零模型/PR；不能据此声明完整业务流程或原安装现场部署已通过。本次没有启动、替换、重置安装环境。

执行：`go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestStoppedVerification' -count=1 -timeout 120s`：app 28.616s、runnerclient 3.306s 通过；cmd 无匹配，仅编译。`scripts/test-deployment-stop.ps1` 通过。

最终调试包：`C:/SteerlineDebug/dev-20261019-093426-abd001f1-dirty`。公开 Runner EXE 的本机/远端原生探针均通过，最终证据在 `evidence/orca-stop-local-recheck/` 和 `evidence/orca-stop-remote/`；首次本机发现 Orca 关闭回执早于终端清单更新，生产正确拒绝，探针改为有界等待原清单后重验，失败记录保留在 `evidence/orca-stop-local/`。执行资源专项由 Runner 专属验收维护，本仓不增加重复原生场景。正常停服仍可仅结束 Orca 观察；原终端仍活动或身份无法核实时，部署继续拒绝，不自动关闭业务终端。

本轮新建包为 `C:/SteerlineDebug/dev-20261019-094824-2b16b5df`；本机与台式机64G的 P1 探针证据在 `evidence/orca-stop-p1-local/`、`evidence/orca-stop-p1-remote/`。验证目标为 PTY 关闭后缺原生退出证明仍拒绝，并保持 DB/原失败回执不变。代码与逐项结果仍由 Runner 专属验收维护，本仓本轮仅同步设计和验收边界，未修改 caller 实现。两个 PR 继续不合并、不部署，安装环境保持未操作。


## Codex 完整退出证明补齐

本轮补上可以通过的正向链路：原执行主机 helper 实际查询原 app-server/tools 与 TUI Job 为空、关闭认证后，签发与启动记录公钥绑定的退出凭据；原 readonly history helper 在返回结果前核实原 Job 清空，查询失败不抹去已结算的资源事实。Runner 离线入口只读原 Orca 主机公共文件、验签及核对全部 reader，保留 DB 和失败回执。

本仓没有新增停止协议或降低部署门槛，代码变更仍由 Runner PR #27 维护；本仓 PR #352 同步统一生命周期与调用方验收。使用各自原 worktree，未合并或部署。

消费者回归：`go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestStoppedVerification' -count=1 -timeout 120s` 通过（app 67.094s、runnerclient 3.948s，cmd 仅编译）；`scripts/test-deployment-stop.ps1` 通过。

真实 Codex 现场、最终构建身份与正向重验结果在 Runner 同名 acceptance 的本轮记录登记。Run 是绑定真实 helper/native/reader/PTY 的隔离契约夹具，只运行一次无工具 READY bootstrap，零业务输入/PR，不代表完整业务流程或原安装现场已解阻。缺失的历史退出证据不能被新版本追补；这与新运行已能产生并核实完整证明是不同结论。


最终构建的真实停止重验：本机 `C:/SteerlineDebug/dev-20261019-102911-35068b10/evidence/codex-stop-final-local-recheck/`（50.10s）；台式机64G `C:/SteerlineDebug/dev-20261019-103050-35068b10-dirty/evidence/codex-stop-final-remote/`（46.50s）。两边 `stopped-verified.json` 的逐 Run / 聚合 `resources_settled=true`；原签名正文 `writers_absent=true`、`authentication_closed=true`，原生 reader 已结算，result 的 passed/real_codex/resources_cleaned 均 true。两个包固定同一 Runner SHA256 `b5ba7890fca94b158e18d4a50d8aad0b60cc12673fde5ad88475a75221b15150`、helper SHA256 `d01d2206014bda80d171f2165926851590c6546e763e99bc6e0b4af89add613d`。失败迭代及本轮复审明细保留在 Runner acceptance，不据新包结果声称四条原安装历史 Run 已经核实。
