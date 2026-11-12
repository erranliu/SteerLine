# Roadmap 规划专属验收

2026-11-11 · 真实链、专属受控和选定固定回归已通过，人工走查待安排。[Spec](../../spec/roadmap-planning.spec.md) · [Plan](../../plans/roadmap-planning.plan.md) · [测试选择](../../plans/roadmap-planning.tests.json)

Runner #146（`2de536cb`）已合入。自动项的命令、时点和证据要求见 Plan。真实链使用独立调试包、固定 EXE 和隔离测试远端。受控链在 SteerLine 侧重建原请求观察，在 Runner 侧以原生协议夹具重启实际服务。

| ID | 方式与场景 | 最小通过条件 | 状态 |
| --- | --- | --- | --- |
| RPW-A01 | 自动 · Runner 接入和真实多轮 | 拒绝缺能力、错误 schema、缺规则文件或不合格配置。真实本机 Codex 从固定文件和仓库读取材料，两轮沿用原 Run、session、worktree。第二轮理解前文指代并在原稿继续写，预览与稳定链接读取新版本。登记发布凭据与原生主机权限的实际范围。 | 真实两轮、公开 schema 正反例与材料守卫已验证 |
| RPW-A02 | 自动 · 并行、原请求、停服与恢复 | 两任务隔离，同路径活动任务互斥。活动轮无法取得租约，持有时消息和 stop 被阻断。注入 acquire/release 回执丢失、持有租约时重启、过期回执和本地写者未退出。受租约阻挡的 stop 延后至原构建启动并完成对账、归还之后。未核实写者保持阻断，重复请求无重复 Git 写入。 | Runner 持租约重启已验证，SteerLine 受控恢复见下文 |
| RPW-A03 | 自动 · 单任务和全部 Rebase | 单任务只改变自身。批次使用一个目标 commit，任务分别成功、冲突或跳过。一个冲突时另一任务可完成。验证已提交、暂存、未暂存和未跟踪改动的本地检查点可恢复，目标改写或身份不符显示诊断。 | 真实单任务 Rebase 已验证，批次和检查点见下文 |
| RPW-A04 | 自动 · 一键冲突处理 | 从真实 Git 冲突状态归还租约。真实 Codex 按固定短提示词在原会话和同一 worktree 修复并继续。受控覆盖已暂存但未完成、用户补充取舍、未知轮次和显式中止后检查点恢复。仅核实原分支、目标祖先、Rebase 元数据及索引后才更新基线和预览。 | 真实原会话续作已验证，暂存和中止见下文 |
| RPW-A05 | 自动 · 按文件审阅、发布和更新 | 草稿默认选中，用户加选一个额外文件，PR 只含确认集合。覆盖删改、新增、越界、重定向、Rebase 或内容变动使旧确认失效、未知推送/建 PR 对账。复审后更新同一 PR。模拟更新与合并竞争，按实际合入的已确认 head 核验。 | 真实 PR 文件清单已验证，回执恢复和更新见下文 |
| RPW-A06 | 自动 · 合并、同步与稳定链接 | 合并未知时保留原 PR。合并核验后禁止新的规划写入并保存关闭义务。滞后 checkout、错分支、本地独有提交、脏稿及来源缺失均保留合入稿预览。目标分支已发布且包含合并提交的有效来源通过交接，包括随后合入的干净版本。原链接命中对应来源，刷新和重启后仍可用，并单独结算原会话和测试资源。 | 真实合并、同步及链接交接已验证，本地阻断见下文 |
| RPW-A07 | 自动 · 规则和共享入口回归 | Plan 选定的 L1/L2、Roadmap、存储、HTTP、部署及 Web 集合完整通过。提示词正文、字节、实际请求和独立审核记录齐全。 | 通过 |
| RPW-M01 | 人工 · 页面体验 | 能切换并行规划、追问、复制链接，理解批量 Rebase 结果及冲突状态，并通过预览和逐文件 diff 完成发布确认。参与人和窗口待安排，体验允许后置。 | 待验收 |

实施后逐项记录受测 SteerLine/Runner 来源和 EXE SHA-256、实际环境、命令、结果、外部证据路径和未验范围。

## 提示词交付和独立审核

本功能新增两条固定短指令和一份固定规则，供原生会话建立和冲突续作使用。以下文件为从空正文新增的完整差异。

| 实际正文 | UTF-8 字节前 → 后 | 业务用途 |
| --- | --- | --- |
| [创建模板](../../../internal/prompts/roadmap-planning.md) · [实际渲染样本](../../../internal/prompts/testdata/roadmap-planning.golden.md) | 模板 0 → 124，样本 0 → 146 | 给原会话绑定草稿路径、固定规则文件和原始用户目标。样本保留用户 CRLF。 |
| [冲突模板](../../../internal/prompts/roadmap-planning-conflict.md) · [默认正文](../../../internal/prompts/testdata/roadmap-planning-conflict-default.golden.md) · [含用户决定正文](../../../internal/prompts/testdata/roadmap-planning-conflict.golden.md) | 模板 0 → 184，默认 0 → 131，含决定 0 → 168 | 在原 Rebase 现场解决冲突，用户有取舍时按原文交接。 |
| [固定规则文件](../../../internal/prompts/materials/roadmap-planning.md) | 0 → 446 | 明确草稿内容和产品职责，文件由 Agent 从指定路径读取。 |

独立审阅核对了完整正文、字节、组装和 `conversation/2` 实际发送位置。发现先校验材料会挡住已受理 Run 的恢复，以及固定材料路径需要拒绝重定向。已改为先对账原提交，首次发送前再核验路径和摘要。创建时先核验 schema，再独占创建普通材料文件。首次记录未保存时，重试只复用安全路径下字节一致的材料。复审通过。黄金样本、固定规则摘要和源码守卫通过，发送路径测试覆盖缺失、漂移和已受理原请求的恢复。

## Runner 契约取证

从 `2de536cb` 的 Git 归档构建 Windows Runner EXE，SHA-256 为 `d58f0b26abb03cca68a1c9fe3996a0fed10ea55d8602c3a1953b4132edec1f9f`。公开 capabilities 发布 `conversation/2` 的 `workspace_write`。同一 EXE 的 `TestSRManagedConversationWorkspaceWrite` 通过，实际使用临时 Git 和原生协议夹具，覆盖持有租约时重启、原租约归还及同一原生会话在 Git 冲突现场续作。证据根为 `C:\SteerlineDebug\roadmap-planning-6999\diagnostics\runner-146-contract`。

对上述 EXE 的 `capabilities --offline` 输出调用 SteerLine 的 `CompileCatalog` 和 `ValidateManagedConversation`，合法输入通过。缺租约能力、缺失或损坏的 schema、错误工作流版本及只读配置均被拒绝。7 项结果保存在本次调试构建目录的 `check-managed-schema.json`。`go test ./internal/runnerclient ./internal/prompts -count=1` 通过。

## 已执行的真实浏览器验收

使用 Windows、Edge、Codex CLI 0.166.0、上述固定 Runner 和 GitHub 隔离测试仓库。SteerLine 受测源码起点为 `6c71004ab39434fb17770aa918c3f877fc686d78`，包含本次工作区实现。执行 `scripts/test-roadmap-planning.ps1 -Mode Live -RunnerExe <固定EXE> -CodexExe <本机CLI> -OutRoot <本次目录>`。后端和 Web EXE 的 SHA-256 分别为 `295619e830083c76ee232c280c99764a7085df0cabeb8f857446e631b25c2e00` 和 `08ade41b177281bf0f06792dd6be1128e22ba7c72a6b60a9e9cba25cb4757706`，原路径见 `evidence/build.json`。

- 原生会话完成两轮改稿，第二轮从前文取回已确认取舍。真实目标分支更新后，SteerLine 执行 Rebase，原会话处理文件冲突并继续原 Rebase。
- 浏览器勾选草稿和一个额外文件，GitHub 文件清单确认只包含两项。测试端合并 [SteerlineTest PR #797](https://github.com/erranliu/SteerlineTest/pull/797)，项目 checkout 滞后时保留待同步。快进后同一规划链接打开已合入的草稿。
- Playwright 1 项通过，约 4.3 分钟。原会话、服务和远端测试引用核验清理，进程和监听均为零。原始结果、构建信息、浏览器日志、原输入与清理证据位于 `C:\SteerlineDebug\roadmap-planning-6999\test-results\L4\live-20261111-170634`。

验收使用本机 `full_access` Codex 和具有 `SteerlineTest` 推送权限的宿主 GitHub 登录。产品只在浏览器确认后提交所选差异。主机权限范围仍由该执行环境管理。

前置试验暴露了浏览器文案断言、代理读路由、配置派生支持、WindowsApps 原生路径及规划状态目录的接线问题。已修正接线及试验入口。前次规划状态目录使用修正后的停止校验复核，独立结果保存在原试验的 `evidence/stop-reverified.json`。

## 专属受控验收

`scripts/test-roadmap-planning.ps1 -Mode Controlled -OutRoot C:\SteerlineDebug\roadmap-planning-6999\diagnostics\controlled-20261111-174818` 退出码为 0。环境为 Windows、Go 1.26.4、Git 2.46.0.windows.1、Node 24.13.0 和 Vitest 5.0.4。五个规划 App 测试根、Web 代理路由、两个组件用例及生产 Web 构建均通过。

- 实际 Git 检查已暂存冲突仍在 Rebase，原分支继续后才更新基线。显式中止恢复原 HEAD、索引及工作树内容。批次以一个 commit 冻结可执行成员和跳过理由，验证路径占用互斥。
- Git/SQLite 和 Runner 公开协议夹具核对租约原回执、过期版本、当前写者、归还前后的冷启动停止资格和原停止请求重放。
- 审阅标记不可发布的文件，服务端拒绝选中该文件。选中文件形成独立提交，保护推送，回收未知建 PR 回执并更新同一 PR。以实际合入的旧确认 head 核验，阻断滞后 checkout 及脏稿交接。
- 真实浏览器覆盖一个任务的两轮改稿、Rebase、PR 与同步交接。多任务批次和故障注入采用受控证据，人工体验走查待安排。

## 固定回归

以下报告均位于 `C:\SteerlineDebug\roadmap-planning-6999\test-results`，目录中的 `summary.json` 保存完整性与退出事实。

| 测试 | 结果 | 报告目录 |
| --- | --- | --- |
| `l1.minimum` | 通过 | `L1/l1-20261111-170723` |
| `smoke.delivery` | 通过 | `L2/smoke.delivery-20261111-171340` |
| `go.roadmap` | 通过 | `L4/go.roadmap-short-temp-20261111-181341` |
| `go.storage` | 通过 | `L4/go.storage-20261111-172822` |
| `go.http` | 通过 | `L4/go.http-20261111-173132` |
| `go.deployment` | 通过；规划专用冷启动另以原请求夹具补验 | `L4/go.deployment-20261111-173251` |
| `web.unit` | 通过 | `L4/web.unit-limited-20261111-183300` |

定向 App 补验 `diagnostics/final-targeted.log` 通过。持有租约以及仅观察到 owner 已变化时均未生成冷启动 stop；原 release 回执核实后持久生成 stop，重复准备保留其原请求。该组同时验证版本过期、其他租约持有、原本地写者仍在、推送未发出时按原 head 继续、建 PR 回执丢失及同一 PR 更新。存储与冷启动检查使用固定公开接口夹具。真实 Codex 浏览器场景未注入 SteerLine 整体进程崩溃。

早期 `go.roadmap` 试验保留原报告。规划用例的换行符夹具已固定为 LF。既有转正用例在较深临时路径下触发 Git for Windows 的 `$GIT_DIR` 长度限制，使用实际独立短临时目录 `C:\SteerlineDebug\rp` 复跑完整集合通过。

早期 `web.unit` 试验出现既有页面等待超时和活动轮询计数波动，原报告保留。最终正式复测将测试进程限制为四个逻辑处理器，沿用原测试代码和选集，完整通过。

## 合入 main 后的补验

干净工作区提交 `34fbc18b711b9ab42eaac178ec0195e491386692` 合入 `main` `40d710fb89d21923c2a107399b582cd717c3709a`。两侧保留各自的 SQLite 表，合并候选使用 schema 48。独立复核确认规划和维护的提示词资产、黄金正文及原发送接线均保留。

- 在 Windows、Edge、Codex CLI 0.166.0 和相同的 Runner #146 `2de536cb` 上重跑真实链。一个任务在原会话完成两轮改稿、解决实际 Git Rebase 冲突并继续。GitHub [SteerlineTest PR #798](https://github.com/erranliu/SteerlineTest/pull/798) 的文件清单只包含勾选的草稿和一个额外文件。合并后，滞后的项目 checkout 保持待同步，快进后原链接打开已合入草稿。测试远端分支、会话和服务已核验清理，进程与监听均为零。
- 真实补验报告位于 `C:\SteerlineDebug\roadmap-planning-6999\test-results\L4\live-merge-34fbc18b`。后端与 Web EXE 的 SHA-256 分别为 `93a9eddc7d978702ebab8b8276fe5aaedf9c50e6a7819897cba674eca5afa9c0` 和 `8c40f6a9db5eb7d61bff4e821130c80f3ddf488e8c279d718e24310441ae184c`。
- 同一提交的专属受控验收、L1、存储、Runner 客户端、HTTP API、规划和维护代理路由，以及原停止请求定向测试均通过。HTTP API 首轮出现 Windows 文件重命名拒绝访问，独立串行复跑全包通过。证据位于 `C:\SteerlineDebug\roadmap-planning-6999\diagnostics\main-merge-34fbc18b`。其他固定回归保留上节的原受测版本和报告。
- RPW-M01 人工走查待安排。多任务批次和租约故障恢复以受控测试取证，真实浏览器场景未注入 SteerLine 整体进程崩溃。

## PR #511 审查补验

2026-11-12 合入 `main` `8b906653e62d8c56493358c5970c87d3826080c6`。修复提交 `2fd1c3b3b3c61604887b372609b6d47659fc90c8` 将规划租约纳入离线停机核验，同构建重启仍可对账持有的原租约。残缺的 Rebase 元数据和未核实的 Git 写入会保留租约与现场。PR 合入需核验完整选中快照并成功保存，交接前刷新远端目标。已核实的来源交接与原会话关闭独立结算。

- 定向复现确认残缺的 Rebase 元数据会被误判为完成、部分文件核验会改变草稿预览。修复后专属受控验收及定向停机、显式放弃用例通过，包括原租约恢复、错误停机证明、远端分支丢失合并提交以及 Runner 暂不可用时的本地来源交接。报告根为 `C:\SteerlineDebug\roadmap-planning-6999\diagnostics\pr511-review`。
- 首次真实复跑在等待文件审阅时触发 15 秒点击超时，资源已清理。验收脚本改为在原审阅租约归还后勾选，提交 `3aefaab6fe8e787cfb21fc8e13d1409a4d2fb3bd` 在 Windows、Edge、Codex CLI 0.166.0、Runner #146 `2de536cb` 下重跑通过。真实两轮改稿、原会话 Git 冲突续作、只发布勾选的两份文件、合并待同步及稳定链接跳转均通过。测试 PR 为 [SteerlineTest #801](https://github.com/erranliu/SteerlineTest/pull/801)，测试资源清理完成，进程和监听均为零。报告根为 `C:\SteerlineDebug\roadmap-planning-6999\test-results\L4\live-review-3aefaab6`。
- RPW-M01 人工体验仍待安排。真实复跑覆盖一个任务，多任务批次和租约故障使用受控测试取证。
