# #189 后基线修复

恢复 Orca 完全访问在 composition 中的配置与 readiness 判定；历史读取缺少注入宿主时返回明确错误，测试注入真实受控宿主。修正重构后 fixture 的公开字段、原始回执、PR remote 身份和显示合同，保留原身份与拒绝变更断言。Go JSON 报告支持当前工具链的构建事件，真实构建失败仍失败。先定向验证，必要时按完整入口新暴露的问题继续修复。

## 测试计划

| 级别 | 组/场景 | 环境 | 命令 | 时点/通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | composition/full-access、历史、原会话/回执、console、parser | 本机、无 token | `go test ./internal/steerlineapp ./internal/steerline/console ./internal/testfeedback ./cmd/steerline ./internal/steerline/integration -count=1`；核心按失败测试精确选择 | 修复后；保留拒绝和不重复派发断言 | baseline-repair-189/定向记录 |
| L1 | 固定最小入口 | 本机、无 token | `go run ./cmd/test-feedback -level=L1 -out=<fresh>` | 定向后，全部通过、报告完整 | baseline-repair-189/l1/ |
| Full | 完整确定性 Go | 本机、三 worker | `go run ./cmd/test-feedback -workers=3 -out=<fresh>` | 全部枚举测试/包完整、零失败；新失败先定位修复再新运行 | baseline-repair-189/full/ |
| L2 | smoke.delivery、smoke.local-codex、smoke.orca-codex | gated；完全访问，luna/medium，真实 SteerlineTest PR | `go run ./cmd/test-feedback -level=L2 -out=<fresh>` | 校验恢复后；门槛通过再真实双链路，PR 合并、usage/清理通过 | baseline-repair-189/l2/ |
| L3 | delivery.orca-codex、recovery.orca-codex | live、完全访问，luna/medium | 分别 `go run ./cmd/test-feedback -level=L3 -suite=<组> -env=live '-scenario=<ID>' -out=<fresh>` | Orca 定向后；两回合准备、原执行交付/重启、usage/清理通过 | baseline-repair-189/各场景/ |

本次选择 L1/Full 为用户明确要求，L2 和 Orca L3 覆盖修改的原生入口；不重复已通过且本次不变的本地 L3。证据根 `docs/validation/e2e-dependency-boundaries/`。每次真实运行独立现场、失败不自动重试。
