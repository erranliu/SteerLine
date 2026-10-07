# 测试总入口

第一阶段的真实执行/验收仅包含本地 Codex 与本机 Orca；`orca-codex` 场景指本机 Orca。存量远端 opt-in 探针只作单独的历史/实验工具，不在一期必需门槛中；静态构建、协议拒绝和依赖守卫可以继续覆盖现有远端代码，不声明远端可运行。

从仓库根目录运行以下命令。日常按改动选择最小必要范围；需要完整基线时选择 Full，涉及原生交付或恢复时选择对应 L2/L3。各入口分别执行，不会因为跑了 L1 或 Full 就自动跑真实模型。

## 文档静态检查

纯文档、目录导航或源码注释变更不运行产品/模型测试。使用以下独立入口检查本地导航、任务身份和差异格式：

```powershell
python scripts/check_doc_links.py
python scripts/roadmap_task_id.py check
git diff --check
```

链接检查覆盖仓库 README 与 docs 下的 Markdown 文件/目录目标，跳过代码示例、外部 URL 和页内锚点；不声明外部页面、历史章节锚点或原实验可重放性已经验证。任务身份检查使用当前规划，不复用历史验收中写死的任务集合。移动原始证据或原型时另行核对字节摘要；固定 commit 中的路径与契约摘要不因当前导航改写。

## 选择范围

| 入口 | 用途 | 模型 token / 外部效果 |
| --- | --- | --- |
| L1 | 固定最小闭环、规则和依赖边界；可追加定向用例 | 默认无真实模型 |
| 定向 Go 回归 | 指定包或测试，定位受影响行为 | 普通用例无真实模型；显式原生验收另行配置 |
| Full | 全部未启用 e2e tag 的确定性 Go 回归，包含受控进程、Git、SQLite 和接线测试 | 默认无真实模型；登记的原生 opt-in 用例会跳过 |
| L2 controlled | 只检查 smoke 的无 token 门槛 | 本地隔离项目与 loopback Git/PR provider；不构成完整 L2 |
| 完整 L2 | 门槛通过后，本地 Codex 和 Orca + Codex 两条真实交付链路 | 消耗 token；在 SteerlineTest 创建、转 ready、合并真实 PR，并清理本次分支 |
| L3 controlled | 独立选择交付、等待合并后重启等受控场景 | 无 token；本地隔离项目与 loopback provider |
| L3 live | 独立选择本地/Orca 的真实交付或恢复 | 消耗 token；使用 loopback Git/PR provider |
| E2E 基础设施 | 测试环境、driver、codec、证据和清理 | 无真实模型 |
| 编译检查 | 检查包与带 tag 场景能否编译 | 不执行场景，不构成测试通过证据 |

Full 将 `internal/execution` 和 `tests/integration` 两个业务包分别按顶层用例划为三组。每个 worker 顺序运行自己的 execution 分区和 integration 分区；另一个独立串行 Go 命令完整运行其余包（`-p=1 -parallel=1`，包括 `internal/app/host` 服务宿主）。最多四组命令并发，各包使用独立测试进程和产物目录，保留 50 分钟总期限及各组原期限。报告的 `core_package`、`integration_package` 和两份用例枚举记录实际所有者，并核对各顶层用例和其他包均不遗漏、不重复。它与 L3 的“多组端到端场景”是两个范围；完整项目验收应明确选择哪些入口，而不是把它们都称为 Full。

## L1、定向与 Full

```powershell
# 日常最小入口（-quick 是相同入口的旧命令名）
go run ./cmd/test-feedback -level=L1

# L1 加本次改动的定向测试；Go 参数放在 -- 后
go run ./cmd/test-feedback -level=L1 -- ./cmd/test-feedback -run '^TestLevelSelectionsRejectAmbiguity$' -count=1

# 只跑指定包，并保存报告
go run ./cmd/test-feedback -- ./internal/console -count=1

# 完整确定性回归：三 worker 入口
go run ./cmd/test-feedback -workers=3

# 完整串行入口：用于定位进程分组相关问题
go run ./cmd/test-feedback -workers=1
```

L1 的固定选择以 [quickTests / quickPackages](../cmd/test-feedback/scoped.go) 与 [任务包选择](../cmd/test-feedback/task_scope.go) 为准；定向用例来自各包的 `*_test.go`。Full 会枚举并核对包/核心顶层用例，不能用部分通过替代完整覆盖。三 worker 入口拒绝已启用的原生验收开关；原生验收按下面的独立入口选择。

## L2 与 L3

L2/L3 当前在 Windows 上串行运行。controlled 使用真实 SteerLine、Git、SQLite 和受控 worker/provider；live 使用真实 Codex/Orca。真实场景固定 `gpt-6-luna` / `medium`，完全访问映射为 `danger-full-access` / `approval_policy=never`。

live 需要本机已登录的 `codex-cli 0.159.2`；Orca 链路另需本地 Orca。完整 L2 还需要 `gh` 对 [SteerlineTest](https://github.com/erranliu/SteerlineTest) 的写权限。场景使用独立 Codex home 和测试项目；完全访问测试不需要 elevated sandbox 初始化。具体环境准备、权限断言、usage 和资源所有权见 [E2E 环境指南](../tests/e2e/README.md)。

```powershell
# 固定 L2：先 controlled，再两条真实链路；门槛失败不进入真实阶段
go run ./cmd/test-feedback -level=L2

# 只诊断门槛，无 token，报告为 gate-only
go run ./cmd/test-feedback -level=L2 -env=controlled

# L3 的 controlled 与 live 分开选择
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled

# 每个真实场景可以独立运行
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'
```

| 级别 | 环境 | 组 | 场景 ID |
| --- | --- | --- | --- |
| L2 | controlled | smoke | `smoke.delivery` |
| L2 | live | smoke | `smoke.local-codex`、`smoke.orca-codex` |
| L3 | controlled | delivery | `delivery.basic` |
| L3 | controlled | recovery | `recovery.waiting-merge-restart` |
| L3 | live | delivery | `delivery.local-codex`、`delivery.orca-codex` |
| L3 | live | recovery | `recovery.local-codex`、`recovery.orca-codex` |

注册表 [suites.json](../tests/e2e/suites.json) 是场景 ID、包、测试名和环境的来源。L3 的 `-suite` / `-scenario` 支持逗号多选；省略场景 ID 时运行所选组和环境下的全部场景。PowerShell 中含点的场景参数应整体加引号。L2 是固定选择，不接受按场景挑选或直接选择 live；L2/L3 不与 `-workers`、`-quick`、`-build-only` 或原始 Go 参数混用。

## 基础设施与编译

```powershell
# runner / 测试适配层的定向回归
go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1

# 含 e2e tag 的项目隔离、部分准备失败和清理回归
go test -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1

# 依赖边界：生产不能依赖测试工具，场景通过测试适配层访问实现
go test ./internal/testkit/boundary/... -count=1

# 普通 Go 包编译检查
go run ./cmd/test-feedback -build-only

# 只编译全部 E2E 场景，不运行场景
go test -tags=e2e ./tests/e2e/... -run '^$' -count=1
```

带 tag 的场景包应通过分级 runner 执行；直接运行缺少 runner 环境的场景会失败。编译成功不证明业务断言、原生证据或清理通过。

## 独立原生验收

这些检查绑定专门的原生现场，不属于固定 L2/L3 注册表。先阅读各测试的前置字段、原身份和实际效果，再只运行选定测试；不要批量开启所有 `STEERLINE_*` 开关。未配置而跳过，表示未验收。

| 检查 | 前置与范围 | 测试来源 |
| --- | --- | --- |
| 本地 Orca host wait | `STEERLINE_ORCA_HOST_WAIT_PROBE` 指向包含原 workspace/terminal/runtime/incarnation 的隔离现场 JSON；检查原终端等待事实 | [TestOrcaNativeHostWait](../internal/execution/adapters/orca/host_wait_live_cd67_test.go) |
| 历史/实验：Orca 远端 P10 reply gate（非一期） | 显式 `STEERLINE_REAL_P10_REPLY_PROBE=1` 及源码列出的原 Run/Task/Dispatch/question 等字段；会对真实原问题发送回复 | [TestSupervisedOrcaRealP10ReplyGate](../internal/execution/adapters/orcaremote/p10_live_cd67_test.go) |
| 原运行恢复预览 | `STEERLINE_REAL_RECOVERY_CONFIG` / `STEERLINE_REAL_RECOVERY_RUN_ID`；读取隔离原现场 | [TestRealIsolatedRecoveryPreview](../internal/app/host/recovery_real_probe_test.go) |
| Windows 服务窗口 / Ctrl+C helper | 只由绑定原进程、启动回执和报告路径的专门探针调用 | [Windows 平台测试](../internal/app/host/platform_windows_test.go) |
| elevated sandbox 凭据检查与设置 | 为显式需要 elevated 的开发场景使用；无 token 的检查和可能触发 UAC 的初始化是两步 | [E2E 指南的 sandbox 工具](../tests/e2e/README.md#真实环境)、[设置脚本](../scripts/setup-e2e-sandbox.ps1) |

示例：已按源码准备好 host-wait probe 后，运行 `go test ./internal/execution/adapters/orca -run '^TestOrcaNativeHostWait$' -count=1 -v`。报告该原生测试的结果，不把其他 opt-in 测试的 skip 算作通过。

## 产物与判断

L1、三 worker Full、L2/L3 默认将证据写到系统临时目录 `steerline-test-feedback/<运行时间>`。串行、定向和编译入口默认写到 `docs/validation/test-feedback/runs/<运行时间>`。可为每次运行显式指定新的 `-out`，已有目录中的产物不能覆盖。

```powershell
$runRoot = Join-Path $env:TEMP ('steerline-tests-' + (Get-Date -Format yyyyMMdd-HHmmssfff))
go run ./cmd/test-feedback -level=L1 ('-out=' + (Join-Path $runRoot 'l1'))
```

查看退出码和 `summary.json`：确定性入口需要 `passed=true`、`complete=true`，并核对 scope、源码/工作树、命令、枚举与实际终态。E2E 需要选择范围、coverage、每个 scenario 的通过/清理和原生证据都符合要求；`gate-only` 不能当完整 L2。真实运行还需正数、同身份及同模型的 usage；recovery 不应增加模型回合。父子测试计数及时间会重叠，不能当成独立场景数量或累加为总耗时。

失败、超时、证据/清理缺失、零匹配或报告不完整都不能算通过；runner 不自动重试。重新运行使用新目录和新现场，保留前次失败。归档优先 summary、scenario、必要的 usage 和诊断，避免提交认证、数据库、原始 prompt 或 dispatch capability。

历史结果只证明报告中记录的候选版本。[#189 后基线修复验收](validation/e2e-dependency-boundaries/baseline-repair-189/README.md)保存近期 L1/Full、L2 和受影响 Orca L3 的结果，不能替代当前改动的验证。

## 扩展与 Plan

新增 E2E 场景、共享环境和测试依赖边界的约定统一见 [E2E 扩展指南](../tests/e2e/README.md#扩展与-plan)。普通包测试放在相应 `*_test.go`；L1 固定范围和 Full 调度见 [scoped.go](../cmd/test-feedback/scoped.go)，选择参数见 [main.go](../cmd/test-feedback/main.go)。

新写或修改的实现 Plan 应列出最小必要的测试级别、组/场景 ID、环境、命令、时点、通过条件及证据位置；未选择相关组时说明原因。纯文档改动检查链接、命令和场景注册一致性即可，不默认要求 L1、Full 或付费场景。

[分级体系 Spec](archive/spec/completed/tiered-e2e-testing.spec.md) · [真实模型接入 Spec](archive/spec/completed/live-tiered-e2e.spec.md) · [依赖边界 Spec](archive/spec/completed/e2e-dependency-boundaries.spec.md)

#75 任务应用迁包的定向入口、旧/新测试父子名称和真实 app/CLI/SQLite 回滚证据见 [任务用例验证](validation/three-line-03-task-usecases/README.md)。固定 L1 已包含 task/model 与 task 应用测试；不默认启用 Full/L2/L3/live。
