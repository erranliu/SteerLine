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

## 2026-10-12 双任务夹具修复与重新验证

用户授权先 rebase 再修复此前失败的独立沙箱测试。基线为 `d54b51f7`（PR #292），实际验证来自干净提交 `31cded931538decbd4ec146335244a41dc8042e3`。此前 [#288 后重跑](archive/completed/all-tests-after-288-2026-10-10.md) 的第二任务失败仍按原版本保留，不改写为通过。

最新统一提示不再提供 task/run ID，旧夹具却要求模型填写这些字段。夹具改为读取当前 Git 分支，替换继承的 result.txt，仅提交精确 marker/branch 两行；任务/run/session 的权威身份仍由驱动读取正式 workflow/execution 并核对。AGENTS、Spec、Plan 明确第一输入只授权本地实现，报告只格式化已有事实，必须等待另一个显式发布输入。Spec 写明本包专属 SteerlineTest base；真实 Git helper 和原生权限不变。

驱动新增候选精确内容和原回合命令检查：实现轮不能调用 publish.ps1、Git push 或 gh pr create，报告轮不能调用命令，发布轮必须有原生发布命令。十个无模型回归覆盖普通探针、读取脚本、提前发布、报告调用命令和缺发布证据；定向测试、WorkerPrompt 测试、E2E 依赖边界和文档链接检查通过。

`scripts/test-codex-sandbox.ps1` 完整通过：controlled 门槛 105.72 秒；独立 live 包 `C:\SteerlineDebug\dev-20261012-100406-31cded93` 的 `TestDebugReleaseCodexSandbox` 335.68 秒。两项分别完成普通沙箱探针及三个原生阶段，候选仅 result.txt、内容属于各自实际分支；实现/报告轮无发布命令，报告轮无命令调用。第一项合并并由中心交付后，第二项在不同 worktree/session 完整交付；最终 `passed=true`、`deliveries=2`、`manual_confirmation=false`、`sibling_unchanged=true`，覆盖 CSG-01 至 CSG-05。

| 任务 | 原 workflow / candidate | GitHub 事实 |
| --- | --- | --- |
| 1 | `213d62a12451adc517638053f8736701`；`d1e9b2a29f39c5ff1624ca759050f7c796f75488` | [PR #215](https://github.com/erranliu/SteerlineTest/pull/215) MERGED；merge `16be10fb9b9c0a9b9998b9817efac101fdd47ca9` |
| 2 | `1346307ef4e027c1614760ebe727b9cd`；`d70c44ae5896127145d163c21311fd0b721ede06` | [PR #216](https://github.com/erranliu/SteerlineTest/pull/216) MERGED；merge `83ac92312430c39f97cad6e54ae963cb8b7f944b` |

两次普通沙箱发布仍遇到凭据获取失败，各自一次同命令原生自动审核获批后才发布；未放宽权限、代答或重试被拒动作。本次样本验证闭环完成，不保证未来审核总会批准，也不宣称普通沙箱凭据问题已经消失。远端核验本次专属 base/两个 head 均已删除，包内服务/helper 进程为零；原始证据及 hash 见[结果索引](codex-sandbox-git-2026-10-12.json)。本次没有登记新的 L3 场景，不把历史失败、其他 opt-in skip 或未运行项目算作通过。

## 2026-10-12 纳入正式 L3

当前场景注册为 `runtime.sandbox-git.local-codex`，L3/runtime/live，独立场景包 `tests/e2e/suites/sandbox`。独立脚本和场景都调用共享 driver，不互相调用测试函数；L2 固定门槛及 L1/Full 选择不变。L3 自动生成新的 C:\SteerlineDebug 包，核对生成 manifest 的源码/工作树与 runner 选择，读取实际冻结模型/权限和三个原回合的正数 usage；自动审核/子会话不作为原始业务 usage。各轮累计值独立记录，不相加为 token 总量。

命令：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=live -scenario=runtime.sandbox-git.local-codex`。干净验证提交 `90278d32b02b4f3964bf1d6a9a78c0a37ffc0045`，测试 `TestCodexSandboxGit` PASS，395.96 秒。统一 summary 的 selected-L3/live、passed/cleanup_complete=true；scenario 的 passed/cleanup_complete/usage_verified=true，provider=github/erranliu/SteerlineTest，deliveries=2。精确结果、三轮 usage、manifest 和报告 hash 见[本次 L3 索引](codex-sandbox-l3-2026-10-12.json)。

| 任务 | workflow / candidate | GitHub 事实 |
| --- | --- | --- |
| 1 | `30ba16eca7165483d2f2d1ea8bfdedf1`；`506a4dcdda511e699848451132e20e33d618f731` | [PR #218](https://github.com/erranliu/SteerlineTest/pull/218) MERGED；merge `a809f67a9e350f8e5859534abd0c817ab687118f` |
| 2 | `b6a9f880a557dccc6c99ce8a89ee292a`；`ccaa621c9b2c3a8af61a503e9c8c93e7a4038047` | [PR #219](https://github.com/erranliu/SteerlineTest/pull/219) MERGED；merge `70b55f44806ea2e97b19af5ce318335a2d683711` |

本次专属 base/两个 head 均已删除，服务/helper 已停止，成功后 evidence/manifest 已归档，生成包及 nonce 父目录已删除。失败时保留私有包供原始 journal 检查，仍清理服务/远端资源；清理失败、报告缺失或仅完成一项都不通过。两次普通沙箱发布失败后，各一次同命令原生审核获批，权限没有放宽。

首次接入的两次失败均保留：`bb8c7393` 的 Windows PowerShell 5 误继承 PowerShell 7 模块路径，生成包失败，后改为子进程使用自身模块默认值；`0fd50c7a` 的领取请求被新加的普通命令 30 秒限截断，后恢复现有控制客户端等待原回执的 390 秒界限，没有重发领取。两份失败 summary 均 cleanup_complete=true，不计通过。最终 runner 全包回归、场景/证据负例、阶段命令检查、失败包保留测试、带 e2e tag 的 testenv 回归、E2E 边界、编译及文档静态检查通过；本次不重跑 Full 或其他 L3 场景，新增正式注册表共九项不代表九项在本次全部重跑。

## 早期失败现场和边界

- 初次驱动把未准备的隔离 CODEX_HOME 当认证源；随后负向夹具放错 worktree 根，被产品未知身份校验拒绝。已修正驱动，无放宽产品准入。
- 包内 worker gh 登录需要独立准备：上层原有 GH_TOKEN/Git header 隔离保留，未改为把中心凭据透传给 worker；登录只存本包 gh 配置，不写用户 keychain/global Git。
- PowerShell 5 空数组解析错误修复后，曾创建 [PR #184](https://github.com/erranliu/SteerlineTest/pull/184)；驱动误判正常 await_merge 待交互导致该次失败。该 PR 已关闭，验证 SHA 后删除其专属 head；相关失败 base 已清理。不把 #184 记为完成双任务验收。
- `maintenance.auto=false` / `gc.auto=0` 已在真实 worker 生效，但当前 Git 提交仍可能输出共享 `packed-refs.lock` Permission denied。候选提交、分支和发布均经真实核验成功；此次未给共同 Git 根整体写权限，不宣称消除了所有 Git 辅助锁警告。
- 独立 CLI sandbox 的额外诊断因 named permission profile 与原生 turn 的根设置不同而失败，不计产品验收；只在本次生成包内保留。最终 live 包的产品/template 是核验基线，之后只改善测试的 merged-PR 证据和已合并 PR 清理判断，无再次模型调用。
- 安装版没有更新。source/模型权限适配以本机 Codex 为范围，不声明其他 agent、Orca supervised 或所有用户工具/认证组合通过。
