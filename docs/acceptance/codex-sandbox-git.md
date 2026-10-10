# Codex 沙箱 Git 修复验收

归属普通开发“Codex 沙箱开发与真实 Git 发布修复”；[Plan](../plans/codex-sandbox-git.plan.md)。

| ID | 自动验收项 | 状态与证据 |
| --- | --- | --- |
| CSG-01 | worker 使用真实 PowerShell，固定沙箱/自动审核保留，不扩大 Git 写根 | 通过；Codex adapter 定向测试和两个普通沙箱原生命令，Windows PowerShell 5.1.26100.9444 |
| CSG-02 | 专属 TEMP/TMP 可写且不污染候选；worker Git 自动维护关闭；已有 Git 环境配置保留；拒绝重定向 temp | 通过；WorkerRuntime 测试、原生探针 temp_in_admin=true、maintenance=false、gc=0、候选仅 result.txt。共享 packed-refs 警告另列限制 |
| CSG-03 | 真 Codex 完成 PowerShell、temp、最小项目检查、提交；候选仅含任务产物，兄弟写入拒绝 | 通过；两个 sandbox_probe 全项 true、每个三条原生阶段结果、兄弟 sentinel 未变 |
| CSG-04 | 真 GitHub 经 gh shell helper 上传候选并创建/核验精确 PR；必要请求由 auto_review 处理，无人工确认，重试不重复 PR | 通过；真实 PR #185/#186，各一条 require_escalated 请求；原生日志记录同一发布命令获批执行，不使用认证头绕过 worker helper |
| CSG-05 | 两个连续 worktree；首项真实合并/交付后依赖可启动，后项完成发布；所有本次 GitHub 资源仅属专属 base/head；安装环境不参与 | 通过；两项真实 Delivery、不同 worktree/session，PR 均已合并；GitHub 查询确认本次三个分支已删除 |

人工项：可后置检查 Ctrl+N 的界面体验；不作为无人值守链路前置。真实拒绝、认证不可用或环境启动失败如实记录，不能当通过。

## 2026-10-10 实际结果

基线 `fbde4163a42639d5570dbe718b32cd42e95dc676` 加本次未提交修复。最终真实包为 `C:\SteerlineDebug\dev-fbde4163-dirty-20261010-112910`；manifest 保存产品/helper/模板摘要。最终命令通过统一生成入口新建 live 包，设置该包 `STEERLINE_DEBUG_SANDBOX_ROOT` 后运行 `go test ./tests/e2e/internal/driver -run '^TestDebugReleaseCodexSandbox$' -count=1 -timeout=30m -v`。结果 PASS，373.89 秒，evidence/sandbox-live.json 的 passed=true、deliveries=2、manual_confirmation=false、sibling_unchanged=true。

| 任务 | 原 workflow / candidate | GitHub 事实 |
| --- | --- | --- |
| 1 | `00f35b73c75d4fbdb58392b49a37cc04`；`949c4db66e89b4e3796c2283595d63b50fc042a1` | [PR #185](https://github.com/erranliu/SteerlineTest/pull/185) MERGED；merge `220235d62dd18b4a9458707531219542ff4a0c9c` |
| 2 | `332e62222e801fa8d64707cf054c5f7e`；`4df65f14e2e42ac63a7518752eed9e74262674dd` | [PR #186](https://github.com/erranliu/SteerlineTest/pull/186) MERGED；merge `b4a66aa2dd69c91a3a74f30332c0d91938f7dd49` |

证据：本包 `evidence/sandbox-test.log`、`sandbox-live.json` 和原 `home/state/SteerlineTest-*/runs/*/turns/*/appserver.jsonl` / `home/codex/sessions`。JSON 内 pr.State=OPEN 是发布时快照；随后真实 GitHub 查询和中心 Delivery 确认合并。驱动未启动 viewer、回答人工问题或改变原生审批设置。两次 helper 普通执行失败后，原生自动审批各批准一次固定脚本重试。

最小回归：`go test ./internal/execution/adapters/codex -count=1` 通过；L1 passed=true，133.5 秒，报告 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261010T025856.411321300Z\summary.json`。独立调试 controlled 门槛通过，最终门槛包 `C:\SteerlineDebug\中文 verification-912299bd949343698f96b069cd184687\dev-fbde4163-dirty-20261010-110909`，runtime 104.15 秒；其中真实 opt-in 用例在门槛中跳过，真实通过证据是上述单独 live 运行。

## 2026-10-10 rebase 核验

PR 分支已 rebase 到 `42f36428`。主干 `e84447cc` 已包含等价的 CRLF 验收标题和 Windows 私有目录 ACL 修复；保留主干实现与测试，移除本分支重复改动。Codex adapter 包测试、Windows ACL 与验收标题定向测试通过，真实回归驱动编译通过（未设置 live 环境，opt-in 用例跳过）；文档链接检查 0 错误，差异检查通过。以上是 rebase 后的最小回归，未重新调用真实模型；前述双任务 GitHub 证据仍归属原 `fbde4163` 加未提交修复的独立包。

## 失败现场和边界

- 初次驱动把未准备的隔离 CODEX_HOME 当认证源；随后负向夹具放错 worktree 根，被产品未知身份校验拒绝。已修正驱动，无放宽产品准入。
- 包内 worker gh 登录需要独立准备：上层原有 GH_TOKEN/Git header 隔离保留，未改为把中心凭据透传给 worker；登录只存本包 gh 配置，不写用户 keychain/global Git。
- PowerShell 5 空数组解析错误修复后，曾创建 [PR #184](https://github.com/erranliu/SteerlineTest/pull/184)；驱动误判正常 await_merge 待交互导致该次失败。该 PR 已关闭，验证 SHA 后删除其专属 head；相关失败 base 已清理。不把 #184 记为完成双任务验收。
- `maintenance.auto=false` / `gc.auto=0` 已在真实 worker 生效，但当前 Git 提交仍可能输出共享 `packed-refs.lock` Permission denied。候选提交、分支和发布均经真实核验成功；此次未给共同 Git 根整体写权限，不宣称消除了所有 Git 辅助锁警告。
- 独立 CLI sandbox 的额外诊断因 named permission profile 与原生 turn 的根设置不同而失败，不计产品验收；只在本次生成包内保留。最终 live 包的产品/template 是核验基线，之后只改善测试的 merged-PR 证据和已合并 PR 清理判断，无再次模型调用。
- 安装版没有更新。source/模型权限适配以本机 Codex 为范围，不声明其他 agent、Orca supervised 或所有用户工具/认证组合通过。
