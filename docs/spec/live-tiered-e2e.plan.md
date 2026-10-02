# L2/L3 真实执行接入 Plan

绑定 Spec：[live-tiered-e2e.spec.md](live-tiered-e2e.spec.md)

## 实现

1. 扩展 manifest 与 runner：L2 gated 默认入口、L3 显式环境及场景筛选；先枚举已登记测试，严格报告各阶段和被门槛阻断的真实选择。
2. 在 E2E 内增加真实环境能力：认证隔离、固定 profile、真实 Orca 环境适配与资源回收、worker 发布脚本；L2 使用授权的 SteerlineTest 仓库、独立最小 fixture base 分支及真实 gh PR，L3 使用 loopback provider。复用现有项目/任务/服务驱动，不创建另一套业务状态机。Windows 管理员初始化由机器环境一次性准备；复用前以无 UI 的 DPAPI 和既有账号登录检查有效状态，失效在模型调用前拒绝，不在每个场景初始化账号。
3. 增加原生 usage 提取及身份核对，保留证据到场景输出；共享业务断言但各测试独立现场。登记 L2 双链路，L3 delivery/recovery 每组双链路与受控场景。
4. 定向验证后实际运行门槛和真实场景；按实际结果修复，更新 README/验证记录。
5. 增加 `scripts/setup-e2e-sandbox.ps1` 和 E2E 专用初始化 helper：检查通过直接复用；失效只调用一次原生 sandbox setup，等待用户授权，之后验证账号。仅检查模式绝不触发授权；取消/超时失败，不调用模型或自动重试。
6. 将专用账号预检/复制限于 local-codex elevated 模式；Orca unelevated 只复用认证，不依赖管理员初始化。文档明确独立 worker 不继承父级 App 权限模式。
7. 按用户要求把 L2 smoke 双链路改为显式 host 写入 + network allow 的完全访问策略，扩展当前版本本地/Orca 编译与冻结映射；无 Windows sandbox 设置、账号预检和 sandbox canary。workspace/deny 的限制不变。L2 核对 full-access 资源契约，再按受控门槛、真实 token、真实 PR 创建/合并与清理完成验收。

## 测试计划

| 级别 | 组/场景 | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | 选择、门槛、usage、环境辅助 | 本机，无模型请求 | `go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1` | 实现后；非法选择拒绝、门槛失败调用数为零、usage 匹配/去重正确 | 命令输出 |
| 环境预检 | native.sandbox-existing-accounts | 本机 Windows，无模型请求、无提权 | 设置 `STEERLINE_E2E_SANDBOX_PREFLIGHT_HOME` 后运行 `go test ./tests/e2e/internal/native -run '^TestSandboxReadinessExistingAccounts$' -v -count=1` | 初始化后、真实场景前；当前版本标记及两个既有账号均可解密/登录；失效直接报错 | 预检输出；不保存凭据 |
| L1 定向 | sandbox-setup.gate | 本机，无模型请求、无提权 | `go test ./tests/e2e/cmd/sandbox-setup -count=1` | 脚本实现后；有效状态和 check-only 不提权，失效仅一次初始化，取消和后验失败拒绝成功 | 命令输出 |
| L1 定向 | native.sandbox-mode-seeding | 本机，无模型请求、无提权 | `go test ./tests/e2e/internal/native -count=1` | 模式隔离修正后；elevated 拒绝失效状态，unelevated 在无专用账号文件时仍可建立认证 home，且不复制账号 | 命令输出 |
| 环境准备 | sandbox-setup.native | 本机 Windows，无模型请求；失效时用户授权 | `.\scripts\setup-e2e-sandbox.ps1`，之后再跑一次 `-CheckOnly` 和默认入口 | 脚本实现后；一次原生初始化和账号后验成功，后续检查/复用不再次授权；无需重复 L2/L3 | 原生命令及脚本输出 |
| L2 | smoke.delivery + smoke.local-codex + smoke.orca-codex | gated；真实阶段双链路 full access | `go run ./cmd/test-feedback -level=L2` | 本次候选；先受控通过，再确认完全访问冻结契约、两条真实 GitHub PR 创建/合并交付及正数 usage、清理均通过 | `docs/validation/tiered-e2e-testing/live-l2-full-access/` 与运行 summary/scenarios |
| L1 定向 | full-access-policy / worker launch / policy freezing | 本机，无模型请求 | `go test ./internal/steerline/codexworker ./internal/steerline/execution ./cmd/test-feedback ./tests/e2e/internal/... -count=1`；主包权限/Orca 定向检查 | L2 前；双链路 launch/turn 一致、无 Windows sandbox 设置，deny 不放宽、覆盖参数仍拒绝；受控入口可编译运行 | 定向命令输出 |
| L3 | recovery.waiting-merge-restart | controlled | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled` | 环境修改后；身份保持、无重派、原交付完成 | 前后快照 |
| L3 | recovery.local-codex / recovery.orca-codex | live | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'`（Orca 替换场景 ID） | 最终候选，各自运行；原生 usage 正数，重启无额外模型回合 | usage、前后快照、资源清单 |
| L3 | delivery.local-codex / delivery.orca-codex | live | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex'`（Orca 替换场景 ID） | 独立选择通过定向检查；交付行为由 L2 同一共享断言验证，不额外重复消费 | 选择检查、L2 证据 |

full 未选择：既有完整基线已通过；本次改变显式 host 权限映射和 E2E 环境，以相关包/主包定向回归和 L2 真实交付闭环验证。真实场景失败保留原始证据，修复后由开发者重新发起新运行；runner 不自动重试。

本次 smoke 完全访问改造不重复 L3 真实模型测试：L3 模式未改变，优先完成用户指定 L2，减少重复 token 消耗；sandbox 隔离证明不由 L2 交付 smoke 承担。当前新增显式 host 策略属于产品权限映射变化，因此补 codexworker/主包定向回归，但不跑 full。
