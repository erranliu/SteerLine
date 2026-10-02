# L2/L3 真实执行接入 Plan

绑定 Spec：[live-tiered-e2e.spec.md](live-tiered-e2e.spec.md)

## 实现

1. 扩展 manifest 与 runner：L2 gated 默认入口、L3 显式环境及场景筛选；先枚举已登记测试，严格报告各阶段和被门槛阻断的真实选择。
2. 在 E2E 内增加真实环境能力：认证隔离、固定 profile、真实 Orca 环境适配与资源回收、worker 发布脚本；L2 使用授权的 SteerlineTest 仓库、独立最小 fixture base 分支及真实 gh PR，L3 使用 loopback provider。复用现有项目/任务/服务驱动，不创建另一套业务状态机。所有真实项目只复制认证，使用完全访问；不依赖 Windows sandbox 初始化。
3. 增加原生 usage 提取及身份核对，保留证据到场景输出；共享业务断言但各测试独立现场。登记 L2 双链路，L3 delivery/recovery 每组双链路与受控场景。
4. 定向验证后实际运行门槛和真实场景；按实际结果修复，更新 README/验证记录。
5. 增加 `scripts/setup-e2e-sandbox.ps1` 和 E2E 专用初始化 helper：检查通过直接复用；失效只调用一次原生 sandbox setup，等待用户授权，之后验证账号。仅检查模式绝不触发授权；取消/超时失败，不调用模型或自动重试。
6. Windows sandbox 预检/初始化工具保留为显式可选开发能力，不属于 L2/L3 前置条件。独立 worker 使用显式项目权限，不继承父级 App 权限模式。
7. 按用户要求把 L2 smoke 双链路改为显式 host 写入 + network allow 的完全访问策略，扩展当前版本本地/Orca 编译与冻结映射；无 Windows sandbox 设置、账号预检和 sandbox canary。workspace/deny 的限制不变。L2 核对 full-access 资源契约，再按受控门槛、真实 token、真实 PR 创建/合并与清理完成验收。
8. 本轮将 L3 controlled/live 全部统一为完全访问；runner 将级别显式传入环境，不依赖场景 ID/组名来推断权限。共享断言读取实际冻结策略和资源，recovery 在重启后再核对。两组共六场景全部执行，四个真实场景分别用精确场景 ID 独立运行，保留各自结果和恢复前后 usage。
9. L3 Orca 原生发布保持 dispatch 的正式 origin 身份，只对脚本的单次 `git push` 用 `-c url.insteadOf=...` 将传输路由到 loopback；不改仓库配置，避免 remote 查询和任务校验看到重写后的地址。脚本明确工作分支为 PR head、main 为 PR base，并禁止向 main 推送。若原生模型因 fixture 契约矛盾停止，修正环境合同后独立重跑该场景及同路径 delivery，不忽略原生反馈或重发旧会话。
10. fixture 明确 readiness 保留完整原生命令结果 JSON，禁止只投影 stdout 或手写 exit 文本；核验器仍要求真实 exit_code、marker 与绑定身份。准备转业务阶段的原输入目标未就绪，可通过已有公开 recheck_environment 一次重查原环境，不重建终端或重发业务输入；新阻断或再次未就绪仍失败。
11. Orca 接受长提示后，等待原终端的 bracketed-paste 草稿稳定至少一秒再完成一次 Enter，记录提交后的原终端状态；CLI 写入字节的 ack 不等于模型已启动，不重新发送提示。
12. Rebase 到 2026-10-02 最新 main 的 #63 implementation envelope 契约：配置使用 version 2 implementation_configs，原生 profile/session/Orca state/policy resource 经实现 codec 和原执行身份验证后读取；不补旧字段兼容。合并 full-access 资源契约及 Orca host 策略支持，统一选择/冻结摘要以避免未变配置被 atomic claim 判为 stale。
13. 继续 rebase 到包含 PR #185 的 `84097f3`，保留主干 lifecycle ports、原始输入恢复与新原子领取校验；删除冲突中的旧中心原生预检接线。复用新 codec，不重新引入已移除字段；以定向生命周期回归、e2e 编译和 L2 controlled smoke 验证，不重复真实模型请求。记录见 `docs/validation/tiered-e2e-testing/rebase-185/`。

## 测试计划

本次 #185 后 smoke 修复：本地 lifecycle 保留空线程的原连接到中心提交会话身份并启动首个回合，拒绝/取消时释放连接，进程仍由中心管理；start 失败保留 SessionID，使未知结果进入阻断状态。Orca 仅等待原终端的空 agentIdentity 完成识别，替换终端、不同 agent、退出及工作区变化立即拒绝。受控 Codex mock 同步增加“空线程不能 resume”的原生约束。

首轮修复后本地真实链路通过、PR #34 合并；Orca 已通过 agent 绑定并创建 coordinator，但 E2E control helper 的 25s 回执等待先于原生资源准备完成而结束。将回执等待绑定到产品请求已有的 ExpiresAt（2m）；外层 helper 进程预算覆盖 25s 身份解析、2m 回执和 5s 退出，普通 CLI 仍为 30s，不修改产品截止、不重发请求。

将“空线程不能 resume”同时补入 L2 controlled helper，并以协议单测验证完成回合后才可恢复且不增加 dispatch/session 数；真实 L2 运行后对这个纯受控 fixture 修改独立重跑 controlled 门槛，不重复模型请求。

| 级别 | 组/场景 | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | local.first-turn / feedback / partial-receipt | 本机、受控 app-server，无 token | `go test ./internal/steerline -run '^(TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace\|TestMockAppServerThreadAndTurnContract\|TestAppServerFailedPreparationKeepsPartialSessionBinding)$' -count=1` | 修复后；空线程 resume 被 mock 拒绝，首次回合与同会话反馈仍通过，部分身份保留 | 本轮命令输出 |
| L1 定向 | orca.original-agent-detection 与相关包 | 本机，无 token | `go test ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/execution ./cmd/test-feedback ./tests/e2e/internal/... -count=1` | L2 前；仅同终端启动状态等待，其他身份立即拒绝，相关回归通过 | 本轮命令输出 |
| L2 | smoke.delivery + smoke.local-codex + smoke.orca-codex | controlled 门槛 + live，full access，luna/medium，SteerlineTest | `go run ./cmd/test-feedback -level=L2 -out=<fresh>` | 定向通过后；受控失败时不进真实阶段，两条链路真实 PR 交付、正数 usage 和清理全部通过 | `docs/validation/tiered-e2e-testing/smoke-lifecycle-fix/` |

本次暂不选择 L3/full：用户要求修复并重跑 smoke；L2 覆盖本次改变的两条原生准备/首回合路径，L1 覆盖后续反馈与身份拒绝。

| 级别 | 组/场景 | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | 选择、门槛、usage、环境辅助 | 本机，无模型请求 | `go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1` | 实现后；非法选择拒绝、门槛失败调用数为零、usage 匹配/去重正确 | 命令输出 |
| L1 定向 | rebase.profile/resource codecs、claim/freeze | 本机，无模型请求 | `go test ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1`；`go test ./internal/steerline -run 'Test.*(Policy\|ExecutionProfile\|ManagedOrca\|WindowsSandbox)' -count=1` | rebase 后；新 envelope 绑定、显式完全访问和原子领取/恢复回归通过 | rebase 命令输出 |
| L2 | smoke.delivery | controlled，无模型请求 | `go run ./cmd/test-feedback -level=L2 -env=controlled` | rebase 后；新配置创建、领取、发布、合并、身份读取与清理通过 | rebase 输出目录 summary |
| L3 | delivery.basic | controlled，完全访问，无模型请求 | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=controlled` | rebase 后；实际新资源 envelope 的完全访问策略断言与交付/清理通过 | rebase 输出目录 summary |
| L2 | smoke.delivery + smoke.local-codex + smoke.orca-codex | gated；真实阶段双链路 full access | `go run ./cmd/test-feedback -level=L2` | 本次候选；先受控通过，再确认完全访问冻结契约、两条真实 GitHub PR 创建/合并交付及正数 usage、清理均通过 | `docs/validation/tiered-e2e-testing/live-l2-full-access/` 与运行 summary/scenarios |
| L1 定向 | full-access-policy / worker launch / policy freezing | 本机，无模型请求 | `go test ./internal/steerline/codexworker ./internal/steerline/execution ./cmd/test-feedback ./tests/e2e/internal/... -count=1`；主包权限/Orca 定向检查 | L2 前；双链路 launch/turn 一致、无 Windows sandbox 设置，deny 不放宽、覆盖参数仍拒绝；受控入口可编译运行 | 定向命令输出 |
| L3 | delivery.basic + recovery.waiting-merge-restart | controlled；完全访问配置、无模型请求 | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled` | 本轮环境修改后；各自交付/恢复闭环、实际完全访问记录、清理通过 | `docs/validation/tiered-e2e-testing/live-l3-full-access/` 各场景元数据 |
| L3 | delivery.local-codex | live；完全访问、loopback provider | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex'` | 本轮候选；单场景选择、原生 usage 正数、交付/清理通过 | 同目录对应 summary、policy、usage |
| L3 | delivery.orca-codex | live；完全访问、loopback provider | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex'` | 同上；原生 readiness/业务两回合 succeeded | 同上 |
| L3 | recovery.local-codex | live；完全访问、loopback provider | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'` | 本轮候选；原生 usage 正数，重启原身份/权限不变、无新模型回合，原交付和清理通过 | 同目录对应 summary、policy、前后身份/usage |
| L3 | recovery.orca-codex | live；完全访问、loopback provider | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'` | 同上；原生 readiness/业务结算证据完整 | 同上 |

full 未选择：既有完整基线已通过；本次改变显式 host 权限映射和 E2E 环境，以相关包/主包定向回归和 L2 真实交付闭环验证。真实场景失败保留原始证据，修复后由开发者重新发起新运行；runner 不自动重试。

本轮按用户要求执行全部六个 L3 场景，不重复 L2 真实模型测试：产品权限映射与 L2 完全访问路径未改，已通过的完整 L2 证据继续保留。sandbox 初始化工具未修改且不是前置条件，不再次触发授权；不跑 full Go 回归。
