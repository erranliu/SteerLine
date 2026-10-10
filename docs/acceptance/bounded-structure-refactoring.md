# 后端组装与最终回复状态收敛：专属验收

唯一归属：本次无编号合并重构，不是两个任务共用验收。来源：[Spec](../spec/bounded-structure-refactoring.spec.md) · [Plan](../spec/bounded-structure-refactoring.plan.md)。2026-10-10 用户授权后已在本聊天实现并自审修复，BS-A1–BS-A5 自动验证及结构自查结果见下文；提交、PR 及独立复审/合并状态归末尾交付记录，不能以自查代替独立复审。

首次实现/验证基线 `21d9735057f990f9fb83a678c5293eca8c8c520c`（2026-10-10）；保留当时该 HEAD 加未提交实现的 `source_dirty=true` 验证。提 PR 前集成基线更新为 `8e292172d84c902e93cedb9870a658e90a9db0c4`，重新验证结果见质量审计记录；不是干净主干验证，rebase、历史测试和文档检查不能代替产品验证。

| ID / 方法 | 自动行为与通过条件 | 环境 / 时点与入口 | 状态 / 证据 |
| --- | --- | --- | --- |
| BS-A1 / 自动 | 原 keys/codec 保持，构造 probe/start 为零，两个目录工厂/host/config 独立；归属与薄组合另由差异/正常 PR 复审确认 | Windows app fixture；已补 TestBuiltinComposition*，执行 Plan owner 命令 | 自动通过；app 报告及结构自查见后文，独立 PR 复审待做 |
| BS-A2 / 自动 | 新基线 profile/摘要、supervised local/environment 同一实现及冻结目标保持，明确断言两项 HostManagedPermissions=false、LocalWorkspace/ServiceStopBehavior/policy resource 接线；旧 host/codec/ref 继续拒绝，不恢复 terminal 语义 | app 新用例与原双目标/冻结/能力回归，后端整理后执行 | 自动通过；app 报告包含 local/environment/默认 environment 三个新子用例及旧入口拒绝 |
| BS-A3 / 自动 | 独立组件用窄查询端口完成绑定/读取/受理；重置后旧索引/正文/预览失效。旧平铺字段/重复规则/无用层另由差异和复审确认 | Windows console stub；已补 TestFinalReader*，按 Plan 映射 L1 实际 console 全包 | 自动通过；独立 reader 用例（含空历史/pin 切换回归）通过，结构自查见后文，独立 PR 复审待做 |
| BS-A4 / 自动 | 跨项目/候选/适用 Attempt/Turn/generation/bodyRef 隔离；版本/gap/历史/正文/预览、pin/焦点/滚动/未发送反馈保持；报告阶段原 implement Turn/无匹配时 CurrentTurn 查询、完整历史不过滤及步骤展示保持。final 组件不触发写命令或原会话打开；宿主 Ctrl+N 行为仍保持 | 隔离 UI home；console 全包、新 TestFinalReader* 和 Model 接线 TestFinalStageQuery*，沿用步骤展示/原会话测试；按 Plan 映射 L1 实际 console 全包 | 自动通过；原 console 全包证据保留，审计修复后实际枚举/结果见质量审计记录 |
| BS-A5 / 自动 | 真实 app/SQLite/controlled local app-server implement/report/publish 三轮原意图、同会话/Attempt/来源及重启不重投保持，最终 L1 规则/领取/存储/依赖守卫通过 | Windows 隔离 home/clone/DB；执行 Plan 的 TestWorkerStageLocalRoundTrip 和 L1 | 自动通过；原 integration/L1 证据保留，最新集成基线重新验证结果见质量审计记录 |

无额外人工/原生项；准备 Go/Git、既有 controlled fixture、独立 evidence 和新增入口的非零枚举。skip、零匹配、只编译、旧基线结果不能通过；缺口在本次实施补齐，不留后续补验。

2026-10-09 规划维护：工作区 rebase 至当时的 origin/main `a49b7922`，未提交规划无冲突恢复；两候选收紧合并为普通开发规格，执行器提取不在范围。不分配新编号、不派发、不改正式执行状态。静态检查按实际命令登记，当时产品项仍全部待验收。

2026-10-09 静态结果：`check_doc_links.py` 检查 293 份 Markdown、1834 条本地导航，0 错误；`roadmap_task_id.py check` 检查 91 份历史身份、10 条草稿及 5 条正式记录，通过，未 reserve 新编号；`git diff --check` 通过。源码静态核对已有测试声明及 BS-A1–BS-A5 的 Plan/acceptance 映射，无未知已有测试名或缺项；当时两个拟新增前缀明确留在实施步骤补齐。这些结果仅验证规划，不代表产品自动项通过。

2026-10-10 基线复核：保留未提交文档后 rebase 到 `21d97350` 并无冲突恢复。主干已合入 PR #262/#264/#266；将旧三实现规划收紧为 local Codex/supervised 两个实现单元，补充 supervised 双目标与旧入口拒绝、报告阶段查询/步骤展示、Ctrl+N 宿主边界。BS-A5 由旧 TM04 双轮改用现有三轮 controlled 测试；新增前缀仍为待实施，自动项全部待验收。本轮只修订规划，不运行产品测试、不修改其他任务交付汇总。

2026-10-10 静态结果：`check_doc_links.py` 检查 297 份 Markdown、1848 条本地导航，0 错误；`roadmap_task_id.py check` 检查 91 份历史身份、10 条草稿、5 条正式记录，通过且未分配编号；`git diff --check` 通过。对 app/console/integration 300 个不同测试声明静态核对，Plan 引用的 15 个既有名称全部存在；BS-A1–BS-A5 在 Spec/Plan/验收中均有映射，四份规划文件基线一致、无行尾空白。TestBuiltinComposition*、TestFinalReader*、TestFinalStageQuery* 仍明确为拟新增，不计为通过。

## 2026-10-10 首次实施与验证

环境：Windows/amd64，Go 1.26.4；本次 app/console/integration 编译并运行当前工作树测试二进制，未启用 native/live 模型开关。HEAD 如上，未提交源码改动；报告保存实际命令、环境、工作树状态与 Go JSON 日志。下列路径均在本机临时 evidence，不提交原始产物。

| 覆盖 | 实际命令与结果 | 证据目录（均有 summary.json） |
| --- | --- | --- |
| BS-A1/A2 | Plan 的完整 app 定向命令，31 项（含子用例）通过、0 skip；两个 TestBuiltinComposition* 非零执行，36.786 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T004531.686764500Z` |
| BS-A3/A4/A5 | `go run ./cmd/test-feedback -level=L1`，passed/complete=true；12 组、484 项、0 skip，execution 117.5 秒；quick-rules 实际执行 console 全包，包括全部新前缀、TestWorkerStagePresentation、TestNativeSessionShortcutOpensSelectedCandidateAndReportsFailure，以及既有迟到结果/历史/pin/预览/草稿/焦点用例。因此按 Plan 替代另一次 console 全包命令 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T004450.120352300Z`；console JSONL 在 `quick-rules/test.jsonl` |
| BS-A5 | `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1`，1 项通过、0 skip，103.889 秒；核对原 implement/report/publish 三个输入/来源、同会话/Attempt、发布冻结 head 与重启不重投 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T004338.164909800Z` |

首次实施增量映射：integration 运行启动后，生产源码只调整了新文件的 import 分组/空行；app 新测试将 Resolve 后的 agent 断言由注册通配符 `*` 修正为实际选定 agent，没有改生产逻辑或削弱产品规则。后续 app 完整选择与 L1 均通过，该轮不重复执行三轮流程。当时 19 份新增/改动 Go 文件按 LF 归一化后的逐文件 SHA-256 清单位于 L1 evidence 的 `bounded-source-manifest.txt`，清单 SHA-256 为 `f839510cf18558c898fdae7bd5b77c6c6025cacf6b806840e60a9c2edce47dc2`；另外删除 `internal/app/directory_remote.go`，其函数已迁入 supervised 所属文件。首次实施验后至该轮结束仅更新文档，下面质量审计的源码与新结果另行登记。

首次 app 定向运行失败记录保留在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T004337.818488500Z`：新增 supervised 用例错误地要求 Resolve 返回通配符 agent；既有回归通过，修正该测试假设后重新执行完整选择并通过。首次编译发现的提取接线/未使用局部变量已在产品验证前修正，不将编译或枚举结果计为验收。

结构自查：中央构造只显式组合两个 registration/codec 与一次局部目录绑定，没有新 builder/resolver 接口或 registry。localExecutionBackend、local policy/preflight/workspace 辅助迁入 local 单元；双目标、supervised policy/preflight/factory 迁入 supervised 单元，旧定义删除。projectState 的 20 个 final/body/preview 平铺字段替换为一个命名 `finalReplyReader`；重置、失效、版本/分页/正文/异步受理只在组件实现，宿主保留项目定位、查询目标、滚动/recovery 和步骤/原会话入口。组件只收窄查询端口，异步捕获固定值；公开 DTO/backend/schema、adapter、runner 与依赖白名单没有修改。架构索引及并行开发指南已更新。

未运行 Full/L2/L3/live/native 或 `ps1terminal` tag 的两项 Windows 原生终端用例，未重新声称 Orca 双目标原生交付通过；本次只验证组装和 UI 内部行为保持，未修改终端/原生窗口驱动。L1 的 119 个 console 顶层用例集合与本候选默认 build tags 下 `go test ./internal/console -list '^Test'` 的实际枚举一致。普通开发没有新增 Task ID 或修改 execution-map。首次实施结束时独立 PR 复审及提交/PR/合并尚未进行；后续交付见末尾记录，结构自查不能替代独立复审。

交付文档静态检查：297 份 Markdown、1857 条本地导航，0 错误；91 份历史身份、10 条草稿、5 条正式记录检查通过，next ID 保持 91；`git diff --check` 通过。逐文件 Go 指纹未变，窄端口/字段所有权/允许改动范围静态核对通过。静态结果不替代上述自动行为结果。

## 2026-10-10 质量自审与提 PR 验证

用户授权审计、修复并提 PR。在保留工作区改动后，无冲突 rebase 至 `8e292172d84c902e93cedb9870a658e90a9db0c4`，集成 PR #267 的实际分步反馈/测试改动；本轮验证均为该 HEAD 加自审修复，`source_dirty=true`，环境与首次验证一致。

确认并修复一项组件边界遗漏：`cycle` 依赖 Model 的空列表检查，独立调用空 reader 会除零 panic。空历史处理迁回组件，通过返回切换事实保留宿主的原提示/滚动行为；新增 TestFinalReaderCycleOwnsEmptyAndPinnedSelection 验证空状态无副作用，以及历史 item/pin/正文切换。删除只被测试调用的 Model.readFinalPreview 包装，原测试直接验证组件操作；删除 app 中无任何调用者的 agentFor/targetForConfig/targetFor。其余 33 个保留的 app 辅助/backend 方法与当前主干逐函数原文对照一致。未发现其他需要修改的行为退化或 Spec 越界；这是当前聊天自审，不冒充独立 PR 复审。

| 覆盖 | 实际命令与结果 | evidence 目录（均有 summary.json） |
| --- | --- | --- |
| BS-A3/A4 定向修复 | console 的 TestFinalReader*、TestFinalStageQuery*、忽略预览重读与完整历史隔离，10 项通过、0 skip | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T010029.605459700Z` |
| BS-A1/A2 | Plan 的完整 app 定向命令，31 项通过、0 skip，44.027 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T010049.183748400Z` |
| BS-A3/A4/A5 | `go run ./cmd/test-feedback -level=L1`，passed/complete=true，12 组/485 项、0 skip，execution 133.4 秒；console 120 顶层/164 项与当前默认 tags 的 Go 枚举一致，含空历史新回归、步骤查询及 Ctrl+N；按 Plan 替代单独 console 全包 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T010049.609204800Z` |
| BS-A5 | `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1`，1 项通过、0 skip，123.297 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T010049.214104100Z` |

最终源码与上述运行后的逐文件指纹一致：app evidence 中 `bounded-source-manifest.txt` 的 SHA-256 为 `434a44cf364a49cab100aba069592355cef012dcd0ed1c76d05a38b1b149914a`，覆盖全部 19 份新增/改动 Go 文件；原 directory_remote.go 的删除另由 Git diff 记录。验后只改文档，提交时按这些 Go blob 映射候选，不将旧基线结果写成最新通过。静态检查：298 份 Markdown/1864 条本地导航，0 错误；91 份历史身份/10 条草稿/5 条正式记录检查通过；`git diff --check` 通过。Full/L2/L3/live/native/ps1terminal 未选，范围与 Plan 一致。

实现提交：`a5c5fbf9a5f5b77553a547b731568cca6ce32e8c`，分支 `codex/bounded-structure-refactoring`。该提交的全部受影响 Go blob 与上述审计后验证清单一致；验证仍如实记录为提交前 dirty tree 运行，不伪称重新运行了干净提交二进制。提交后只同步本段 PR 元数据，源码保持。

[PR #269](https://github.com/erranliu/SteerLine/pull/269) 已创建并附到当前聊天；GitHub 核实 OPEN、非 draft、base main、head 为上述实现提交且 MERGEABLE。当前没有平台 check run；本聊天自审和本机自动通过不冒充独立 PR 复审或已合并。独立 PR 复审/合并待进行，不修改 execution-map 或分配任务编号。
