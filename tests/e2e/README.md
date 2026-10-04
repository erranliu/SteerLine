# 分级测试与 E2E 环境

全项目测试范围、L1/Full/定向入口、独立原生验收和产物导航见[测试总入口](../../docs/testing.md)。本文维护 E2E 环境、场景和扩展的具体约定。

L1 是固定 quick 加按改动选择的定向回归。L2 是无 token 门槛加双真实交付 smoke。L3 按业务组、环境和场景选择，每个测试可以独立运行。Plan 按最小必要范围选择，不自动串跑所有级别。

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
# 只诊断无 token 门槛，不构成完整 L2
go run ./cmd/test-feedback -level=L2 -env=controlled
# L3 两种环境分开，不自动串跑
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex'
```

保留 `-quick` 和 `-workers=3`；后者是完整确定性 Go 回归，不包含带 e2e tag 的场景。L2/L3 当前支持 Windows、串行。L2 默认 gated：先验证 controlled（包括证据和清理），通过才进入两条真实链路；失败不会创建真实会话或发模型请求。L2 禁止直接选择 live 绕过门槛；显式 controlled 的报告 coverage 为 gate-only。L3 必须显式指定 controlled/live 和 suite；组、场景可以逗号多选，未知、重复、零匹配或跨环境选择失败。PowerShell 的带点场景 ID 参数需加引号。

不能混用原始 Go 参数、quick/full/build-only。`-out` 指定空目录；默认写入系统临时目录 `steerline-test-feedback/<运行时间>`，拒绝覆盖已有产物。

## 场景

| 级别/环境 | 场景 ID | 验证 |
| --- | --- | --- |
| L2 controlled | `smoke.delivery` | 正式创建/领取 A、发布 PR、合并前 B 阻塞，合并后原 A 交付且 B 就绪但未启动 |
| L2 live | `smoke.local-codex` / `smoke.orca-codex` | 真实模型、真实 SteerlineTest GitHub PR、合并交付、依赖解锁和正数 usage |
| L3 controlled | `delivery.basic` | 独立受控交付闭环 |
| L3 controlled | `recovery.waiting-merge-restart` | 等待合并后重启，原任务/数据库/工作流/执行/工作区保持，不重派，再正常交付 |
| L3 live | `delivery.local-codex` / `delivery.orca-codex` | 真实模型配 loopback provider，分别独立交付 |
| L3 live | `recovery.local-codex` / `recovery.orca-codex` | 真实执行后重启保持原身份，usage 不增加，再合并交付 |

## 包与所有权

场景位于 `smoke`、`live`、`suites/<组>`，共享 `internal/testenv`、`driver`、`native`、`controlled`。生产代码不导入这些包。L3 不调用 L2 测试函数，共享环境与业务断言，各测试独立创建现场；没有第二套任务状态机。

依赖边界保持同仓、同 go.mod：driver 调用产品现有控制/查询接口；native 集中原生可执行文件、执行配置和 Orca CLI 解码；evidence 通过实现 codec 验证原 profile/resource/session/回合，输出测试自有模型数据与统一校验。testenv 和业务场景只组织现场、消费适配层及 contracts/task 数据合同，不直接导入生产 facade 或 Codex/Orca 实现包。实现重构时优先修改适配层，不复制 codec 或增加测试专用生产接口。

L1/quick 固定执行 `internal/testboundary`：检查生产依赖闭包不能到达 tests、runner、fixture、测试进程包；检查启用 e2e tag 的 Go 导入（包括场景测试和 helper），实现依赖只能出现在 driver/native/evidence。fixture 包作为库存入口时不视为生产入口，但生产经任何中间包引用 fixture 都会失败。可独立运行 `go test ./internal/testboundary -count=1`，无模型请求。

Run 持有本次产品/helper 构建和选择；Scenario 持有 scratch/证据；Project 持有 home、Git、SQLite、服务/provider；Task 来自正式创建回执，Spec/Plan 真实提交并绑定。不同项目不共享可写资源，重启保留原现场，重跑创建新现场。服务/helper 子进程使用 Windows Job 所有权管理，清理本次进程树。

任务创建/查询使用当前 CLI。领取使用公开 SubmitControl/WaitControlReceipt，先 ResolveIdentity 再计算 Profile digest，与控制台/服务一致。执行身份通过公开 reader/query 的同一事务读取，不直接写 SQL 或读取私有业务状态。

controlled 使用真实 SteerLine、Git、SQLite，helper 实现最小 app-server 和 GitHub provider。worker 真实 commit/push/create；中心查询与核验真实合并后提交交付。loopback `git http-backend` 执行真实传输，Git 适配器保留正式 remote 身份及原始输出；外部 PR 状态只存于 provider 文件。

受控场景上限 5 分钟，真实场景 10 分钟；普通驱动命令 30 秒，原生资源操作 45 秒；Orca 准备总期限 4 分钟，服务清理 30 秒，外部资源清理各 2 分钟。按真实条件等待，失败不自动重跑。

## 真实环境

L2 smoke 的两条真实链路和 L3 controlled/live 所有场景均采用显式完全访问（host 写入、network allow → danger-full-access、approval_policy=never）。真实执行固定 `gpt-6-luna` / `medium`。不复制/预检 sandbox 账号、不运行 sandbox canary，不需要管理员初始化。L2 的受控门槛保持原配置；L3 级别由 runner 明确传入，新增组也采用同一权限。共享断言核对实际冻结权限/资源契约，recovery 重启后再次核对。

真实执行需要本机 `codex-cli 0.159.2`（当前产品权限映射）、Codex 登录认证和本地 Orca。每个真实项目创建独立 Codex home，只复制来源 auth.json，写入最小模型和本次仓库信任配置，不复制 sandbox 账号、历史或全局项目配置。来源为当前 CODEX_HOME，未设置时使用用户 .codex。独立 worker 按 SteerLine execution_policy 启动，不继承启动它的 Codex App 会话的“完全访问”模式。

以下 sandbox 工具供其他需要 elevated 的开发场景显式使用，不属于 L2/L3 前置条件。

elevated Windows sandbox 是机器级预置能力：先在测试外完成一次原生管理员初始化，再复用其有效凭据。该模式的项目创建前检查当前版本的标记和两个账号，使用禁止 UI 的 DPAPI 解密及 Windows 账号登录验证；不创建或修复账号。缺失、版本不符或账号失效立即失败，不进入模型阶段。正常复用时，原生执行器刷新本次目录 ACL 不需要管理员弹窗；若运行期间账号或系统策略变化，仍可能触发原生修复，不能承诺所有系统状态下绝无 UAC。

可独立运行无 token、无提权的凭据预检（PowerShell）：

```powershell
$env:STEERLINE_E2E_SANDBOX_PREFLIGHT_HOME = Join-Path $env:USERPROFILE '.codex'
go test ./tests/e2e/internal/native -run '^TestSandboxReadinessExistingAccounts$' -v -count=1
```

预检失败时，从项目运行以下独立脚本。它在检查已通过时直接复用；失效时通过 Windows `RunAs` 仅提升原生 `codex sandbox setup --elevated --user <原用户> --codex-home ...` 命令一次，请用户完成管理员授权，之后回到普通权限验证两个账号。显式绑定原用户，避免 UAC 使用另一管理员账号时初始化到错误身份。Go 构建、检查和测试都不提权。脚本不调用模型，不修改认证、不删除初始化文件、不自动重试。原生命令会初始化机器账号并保存该 home 的 `windows.sandbox=elevated` 配置。默认最长等待 300 秒，包含授权时间；取消、超时或后验失败返回错误。超时后不要立即再次初始化：提升的原生命令可能仍在收尾。

```powershell
.\scripts\setup-e2e-sandbox.ps1
# 只检查，绝不发起管理员授权：
.\scripts\setup-e2e-sandbox.ps1 -CheckOnly
# 显式选择与真实测试相同的来源：
.\scripts\setup-e2e-sandbox.ps1 -CodexHome "$env:USERPROFILE\.codex"
```

脚本依赖 Go 和 `codex-cli 0.159.2`，可从任意当前目录通过绝对路径运行。默认使用 CODEX_HOME，未设置时使用用户 .codex。`approval_policy=never` 管理模型命令审批，不会批准 Windows UAC。测试不通过关闭 sandbox 或自动点击授权来处理初始化问题。

L2 使用用户授权的 [SteerlineTest](https://github.com/erranliu/SteerlineTest)，需要 gh 写权限。每次克隆独立仓库，以 `codex/e2e/<随机ID>/<场景>/base` 保存最小 fixture 树及测试合同，不改变 main。正式 worker 分支针对该 base 创建真实 PR。local-codex 与 orca-codex 均由模型执行 fixture publish.ps1，真实 commit/push/gh create。测试按匹配的 head SHA 转 ready 并合并，由真实 provider 确认交付。清理只关闭本次仍 open 的 PR、删除本次分支，保留真实 GitHub PR 历史。认证只存在项目 scratch，以环境引用传递，不写入产物。

L3 live 使用 loopback Git/PR provider，两条链路均执行真实模型；与无 token 环境分别选择。Orca 使用真实 CLI 创建、绑定、结算、清理资源。测试 CLI 适配器为新建终端注入隔离环境；当前版本长文本注入若明确停在 Pasted Content 输入框，则完成该原终端的一次 Enter，不重发文本。readiness 探针、业务命令和结算都需要真实模型及原生证据，不合成权限或成功回执。

长提示提交前先等待原终端的粘贴草稿稳定至少一秒，并记录提交后状态；accepted/bytesWritten 不代表 agent 已启动。readiness 的 JavaScript exec wrapper 必须保留完整原生工具结果 JSON，不能只打印 stdout 或手写 exit 文本。原输入目标短暂未就绪时，场景可通过正式 recheck_environment 一次检查原环境，保持原 Task/terminal/Dispatch，不能重发旧业务输入。

L3 Orca 发布脚本使用 origin 的正式仓库身份，仅对单次 push 以 Git `url.insteadOf` 路由到本项目 loopback 服务；不改仓库级配置，以免影响正式 remote 校验。工作分支为 PR head，main 为 PR base，脚本禁止直接向 main 推送。

## 产物与失败

summary.json 保存源码/工作树、构建 SHA-256、精确选择、命令、耗时、coverage、清理和 blocked_scenarios；各组保留原始 Go JSON 日志及 stderr。scenarios/<id> 保存公开状态、命令回执、服务日志、资源清单及 scenario.json。L2 live 还须报告真实已合并 PR 的 URL/仓库/状态，否则不能通过。

`*-usage.json` 及筛选原生事件 JSONL 记录 input/cached/output/reasoning/total 和 session/turn/cwd。local 使用绑定 thread/turn 的 app-server 通知；Orca 使用本项目 home 的 session JSONL，核对 cwd 和每个 turn_context 的模型/effort。累计值取最后样本，不重复相加。缺失、零值、身份或模型不符均失败；重启和交付轮询不得增加 usage。数值是 CLI 使用报告，不换算价格或推断账单。

成功/失败均保存诊断，停止本次资源，关闭句柄并删除 scratch；证据目录独立保留。native-logs 仅保存本项目原生日志，不复制凭据/私有 SQLite，认证值脱敏。失败、skip、零匹配、缺失/重复/非法/截断日志、超时、证据缺失、清理或报告写入失败均不能通过。断电后的跨运行回收不属当前最小范围。

## 扩展与 Plan

1. 在对应包增加 e2e tag 顶层测试，用固定场景 ID 创建 Scenario；独立创建现场，业务断言保留在场景/共享断言中。
2. 按需扩展小型任务材料和环境能力。所有外部资源记录精确 ID，清理只能操作本次资源。
3. suites.json 登记 id/level/suite/package/test/env，live 增加 chain（local-codex 或 orca-codex）。同一个包不能混合环境，保证受控门槛不能提前执行 live。
4. Plan 的测试计划列出级别、组/场景 ID、环境、命令、时点、通过条件及证据；未选相关组简述原因，不默认要求 full 或所有层级。

基础设施定向入口：`go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1`。`go test -tags=e2e ./tests/e2e/internal/... -count=1` 按需验证双项目隔离/部分准备清理。直接运行场景包缺少运行配置会失败，统一入口负责构建、枚举登记测试和精确选择。
