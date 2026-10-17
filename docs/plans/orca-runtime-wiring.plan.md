# Orca 执行主机 runtime 接线修复

用户直接授权，普通开发，不纳入 roadmap。验收见[专属记录](../acceptance/orca-runtime-wiring.md)。

## 定位与实施

Runner 主干 `d4969d0` 已合入远端认证实现。Orca 的 GitHub 执行在缺少 `agent_launch.runtime_executable` 时明确拒绝；该 helper 在执行主机准备认证及同权限 app-server，并回传绑定 helper SHA、配置、原会话的准备证据。SteerLine 不实现另一套认证或远端命令通道。

1. 调试生成器增加显式执行主机 helper 绝对路径和 SHA256，缺失/非法值在生成包前拒绝。
2. Orca 始终生成 agent_launch（即使未选模型），保留 AgentExecutable，固定 runtime 路径/SHA 及 inline TUI 启动参数；manifest 登记非秘密构建身份。认证继续由执行主机已有登录提供。
3. 使用固定 Runner EXE 创建新的独立 Orca 调试包，核对远端已安装 helper/native 身份，实际启动任务、核验三阶段/产物/真实专属 PR，停止并核对资源。
4. 同步集成说明、生命周期 W03/W05–W07 当前缺口；节点/E/V 规则不变。
5. 调试模板使用单次 commit 的 `git -c` 身份，避免要求沙箱改写共享 Git config；发布沿 Runner 普通 Git/gh 流程及已准备的认证，不要求执行策略受限的 publish.ps1 或覆写 credential helper。失败包保留，新模板必须生成新包验证。
6. 真实发布发现 Runner broker 缺少 SteerLine 提供的只读 `mergeCommit { oid }` 查询。由 Runner owner 最小补齐并维护跨仓/扩展字段拒绝测试，规格及验收归 Runner `docs/spec/pr-merge-metadata.plan.md`、`docs/acceptance/pr-merge-metadata.md`；本仓不复制认证实现。新 helper 安装到独立执行主机命名空间，新包固定 SHA 后重验。

## 测试计划

| 验收项 | 级别/范围 | 环境、命令与时点 | 通过条件与证据 |
| --- | --- | --- | --- |
| ORW-A01 | L1：定向脚本参数拒绝和生成结果 | 实现后调用 `scripts/new-debug-release.ps1 -Orca` 及非法路径/SHA；再带完整参数生成 | 缺参数在副作用前拒绝；生成配置/manifest 绑定正确 helper，未指定模型仍有 agent_launch；证据在新包 evidence |
| ORW-A02 | L1：Runner adapter/broker 定向测试 | 初始固定源 `go test ./internal/execution/adapters/orcatransport ./internal/execution/agentlaunch -count=1`；broker 修复后 `go test ./internal/execution/adapters/codex -run 'Test.*(AuthenticationBroker|GraphQL|Gh|GH)' -count=1 -timeout=2m` | 既有缺能力/身份反例与 mergeCommit 正反例通过；新包 evidence/runner-targeted.log、broker-targeted.log；Runner PMM-A01 另登记 |
| ORW-A03 | 产品 live 调试验证，W03/W05–W07/V01/V02/V05/V07 | `scripts/new-debug-release.ps1 -Orca -SteerRunnerExe <fixed> -OrcaRuntimeExecutable <remote> -OrcaRuntimeSHA256 <sha> ...`；新包 start，再用公开命令启动本包任务 | 不再报缺 runtime；原会话完成实现/报告/发布，result.txt 字节正确，PR 仅针对本包 SteerlineTest base/head，停止结算成功；新包 evidence 及本专属验收登记 |

不跑全量 L2/L3：本次只改调用方调试配置生成，Runner 原生认证和故障矩阵已有独立验收；本次验证具体产品接线及真实链路，不重复其完整矩阵。不要求人工验收作为交付前置，不操作原安装候选。
