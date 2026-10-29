# 调用方提示词收敛验收记录

## 2026-10-29 全部 L4 执行结果

本轮按机器清单调用全部 13 项登记 L4 正式入口；按各 ID 最近一次受管报告，10 项通过、0 项失败、3 项不完整。整体不能登记为全部通过。两个远端场景未完成前置，Go 全集还发生挂起，跳过项按既有规则保留。最终 Go 全集源为 `67c03655e86fa3a05f6fc826a640b0c44fd437fc`；最后的受控补测源为 `a86e6ef96917d25f726f478ecffc3718057d75c7`；不同阶段的完整受测来源见表，不将早期版本当作同一提交全集证明。

证据根为 `C:/SteerlineDebug/wt-2352-all-l4/`，下表路径均相对此根。固定独立 Runner 为 `diagnostics/inputs-20261028T1402Z/steerrunner.exe`：源 `62a35bce94de248e83fc2c09ce4f7ad34af5228d`、版本 `0.2.0-dev.3-r3`、SHA256 `3b9ee831c11695bb046dca55bae02da6cd11a3d1ede6cd3f22511080fa655917`。没有修改该依赖源码。初始输入及最终 13 项定义分别保存在该 inputs 目录原 `inputs.json` 和 `final-selected-definitions-67c03655.json`，不覆盖原快照。

| ID | 受管结果 | 源码 | 原始 summary |
| --- | --- | --- | --- |
| `go.full` | 不完整 | `67c03655` | `test-results/L4/20261028T181317986Z-go-full-boundaries/summary.json` |
| `runtime.dual-home-isolation` | 通过 | `a86e6ef9` | `test-results/L4/20261028T173612639Z-controlled-five-complete/summary.json` |
| `runtime.path-isolation` | 通过 | `a86e6ef9` | `test-results/L4/20261028T173612639Z-controlled-five-complete/summary.json` |
| `runtime.project-maintenance` | 通过 | `a86e6ef9` | `test-results/L4/20261028T172825364Z-maintenance-own-baseline/summary.json` |
| `runtime.service-write-isolation` | 通过 | `a86e6ef9` | `test-results/L4/20261028T173612639Z-controlled-five-complete/summary.json` |
| `runtime.source-build-isolation` | 通过 | `a86e6ef9` | `test-results/L4/20261028T173612639Z-controlled-five-complete/summary.json` |
| `smoke.local-codex` | 通过 | `abe1f0ad` | `test-results/L4/20261028T161220507Z-live-snapshot-identity/summary.json` |
| `testing.management-local-codex` | 通过 | `2d91e9eb` | `test-results/L4/20261028T164156384Z-management-fixed-materials/summary.json` |
| `testing.records` | 通过 | `a86e6ef9` | `test-results/L4/20261028T173612639Z-controlled-five-complete/summary.json` |
| `workflow.local-codex-audited` | 通过 | `2d91e9eb` | `test-results/L4/20261028T163423665Z-audited-worktree/summary.json` |
| `configuration.execution-profiles-orca` | 通过 | `b8e48689` | `test-results/L4/20261028T151507877Z-remote-discovery/summary.json` |
| `testing.project-progress-orca-browser` | 不完整 | `2d91e9eb` | `test-results/L4/20261028T164637621Z-remote-progress/summary.json` |
| `testing.project-environment-orca` | 不完整 | `2d91e9eb` | `test-results/L4/20261028T164931097Z-remote-environment/summary.json` |

通过项均有完整清理证据。最近报告及覆盖核对的可重算索引为 `diagnostics/final-l4-run-index.json`；它仅索引上述原报告，不生成新的测试通过结论。正式报告通过原 `test-feedback record` 按受测 SHA 导出到 [Git 测试历史](../records/)，包含原失败与补测；最终代码记录为 [67c03655](../records/67c03655e86fa3a05f6fc826a640b0c44fd437fc.json)。

### 三项未通过的边界

- `testing.project-progress-orca-browser`：固定 SteerlineTest 源 `8855502c8d65ddf92363549ba7e93f05471c3667` 缺 `SteerlineFlow/environment-execution.md`，`environment_start` 在创建活动或 Run 前明确拒绝；原程序与连续六状态浏览器验收未执行。符合当前 Spec 的缺材料阻断，但不算此 L4 通过。
- `testing.project-environment-orca`：308 原源 `76919e3a0c31da73b48de282cf108e2d614b9244`，本次夹具源 `5d2713f3734dbfdca041fd72e87c11f74f82d9ad`。生成前置的原请求 `42c3a959d5f083ee8f1e10a906acbf58` 经公开 CLI 查询为 `request_not_found`，Caller 保留未知状态。只读核对固定 Git 树为 16,287 文件、390,779,400 字节，确定超出所选 Runner 公布的 4,096 文件 / 64 MiB 上限；该前置无法完成，已通过原应用 stop 结束本次场景。不是环境检测/L2/PR 通过，也不是三轮负向验收。原停止原因及能力对照保存在 `diagnostics/20261028T1656Z-308-capability-stop/` 与 `diagnostics/20261028T1656Z-308-generate-capability.json`。两个项目材料仍须走正常 Git 准备；当前 Spec 也仍阻断缺少完整前轮报告正式供给的新环境轮次。
- `go.full`：实际 Go 结果为 1308 pass / 0 fail / 7 skip，准备 9.425 秒、执行 1973.137 秒。原进程在 `TestLocalOrcaCatalog/seed` 挂起，超过十分钟无后续事件；经核对本次 PID/路径/父命令后对独立 Go 进程组发诊断中断结束，但未取得可用栈。因此还缺两个测试终态及最后两包的本轮终态，不能把本轮理解为仅因 skip 未通过。独立 seed 100 次、本机完整序列 20 次、整个 discovery 包 10 次均通过（`diagnostics/20261028T184011479Z-orca-seed-timeout/`、`diagnostics/20261028T184055968Z-orca-sequence-timeout/`、`diagnostics/20261028T184359980Z-discovery-package-timeout/`）；未复现即未确认根因，不宣称挂起已修。被中断未执行的 executionconfig/nativequery 两包补测通过：`diagnostics/20261028T184955562Z-remaining-web-packages/summary.json`。这些定向结果不替代完整 go.full。诊断信号身份记录为 `diagnostics/20261028T1846Z-live-discovery-stack-signal.json`。跳过明细见下方。

两项远端原报告均保留 `cleanup_complete=false` / 受管 incomplete。随后只读确认 Caller 零环境活动、零候选，按公开原请求/Runner 停止证明及 provider 分页读取核对零专属 PR，再以冻结 SHA lease 删除各自两个专属 refs；保留原现场与原失败报告。补充清理证据分别为 `diagnostics/20261028T165118058Z-remote-progress-supplemental-cleanup.json`、`diagnostics/20261028T165855120302Z-remote-environment-supplemental-cleanup.json`；补充证据不改写原测试结果。

### Go 全集跳过明细

- `github.com/erranliu/SteerLine/internal/app/host/TestBackgroundConsoleProbe`
- `github.com/erranliu/SteerLine/internal/app/host/TestDesktopOpenHelper`
- `github.com/erranliu/SteerLine/internal/execution/process/TestProcessHostCommandHelper`
- `github.com/erranliu/SteerLine/tests/e2e/internal/driver/TestDebugReleaseRuntime`
- `github.com/erranliu/SteerLine/tests/e2e/internal/driver/TestDebugReleaseSharedRunnerService`
- `github.com/erranliu/SteerLine/tests/e2e/internal/driver/TestDebugReleaseWebProjects`
- `github.com/erranliu/SteerLine/tests/e2e/internal/driver/TestDebugTestManagementOwnedBranchCleanup`

### 已修复的失效点与验证

- 初次 Go 全集发现 SQLite 持久化反向依赖环境服务，六个架构断言失败。抽出既有纯值到 `environmentadaptation/model`，存储仅依赖值模型；没有兼容别名、SQL 迁移或第二套环境状态。全集最终结果以上述原报告为准。
- a86e6ef9 的完整 Go 全集另发现两个 E2E 结构门禁失败：远端共享测试支持未登记具名 owner、远端夹具直接导入 app 实现包。精确登记 cmd/test-feedback 与 testenv 两个 owner，补正反例拒绝其他包；配置解码放回既有 driver，provider 查询仅接收 PR 纯值。主边界及 E2E 边界完整回归通过，保留原失败，不禁用检查或开放全局依赖。
- Windows 夹具 stdout 的同步管道写入会阻塞；改用临时文件捕获。调试生成器修复 PowerShell 5 JSON/UTF-8 读取，隔离源码 clone 启用局部 longpaths。生成夹具在自己的正常 Git 提交前准备正式协议、writer 与安排；移除 writer BOM 差异。生产缺材料错误在预留前返回明确拒绝，不再让 start 控制一直 pending。
- 真实交付夹具使用已发布 `development/2`，按实际可选报告采集可信 final；真实 Git 产物、原 Run/候选/来源、精确 merged PR、唯一 TaskDelivery 和后继 ready 未启动断言保留。审计使用 `development-audited/2` 的一次代码审计与必要修复，产品 API/UI 完整保留实施结果来源、修复数量/明细、遗留项和审计前后 Git 提交。未知数量不补零、不自行生成 pass。正式产品结果与原 Runner 公开观察逐项对照，材料按 worktree 引用和原固定 Git 字节核验。真实测试 PR [686](https://github.com/erranliu/SteerlineTest/pull/686)、[688](https://github.com/erranliu/SteerlineTest/pull/688) 在失败清理中关闭，[687](https://github.com/erranliu/SteerlineTest/pull/687)、[689](https://github.com/erranliu/SteerlineTest/pull/689) 成功合并交付；均为专属测试资源。
- 维护复验发现第二项目未完成初始维护即被采为隔离基线，且逐文件 Git 读取会占用共享 fixture 锁。改用 `cat-file --batch` 保留原 Git 字节，添加 UTF-8/CRLF、二进制、空文件、工作区被改及缺提交回归；先经另一项目自己的 `maintenance_check` 建立完整基线，再保留原 commit/generation IDs 隔离比较。中间只等待基线的失败也保留；最终原浏览器断言、reset、停止与结算通过。原 2d91e9eb 的失败包停止成功但生成结算未核实，仍保留原证据，不改为通过。
- 公开 result-file 专项原先依赖协议夹具识别业务提示词而超时；改用该夹具既有 held Turn 内容模式，专门验证原始二进制结果、独立进度、停止后封存与迟到写入不刷新。三个公开 Runner 契约专项全部通过：`diagnostics/20261028T172030158Z-public-runner-held-turn/summary.json`；它们使用同源协议夹具，不作真实 Agent 权限证明。审计页面 17 项单测、TypeScript 检查及各正式浏览器场景的 Web 构建通过。

最终固定回归为 L1 `test-results/L1/20261028T181315851Z-boundaries-final/summary.json`、L2 `test-results/L2/20261028T181327062Z-boundaries-final/summary.json`，均通过且零跳过；受影响 L3 `testing.runtime-progress` 为 `test-results/L3/20261028T172518742Z-final-required/summary.json`，通过、零跳过、完整清理（源码 f4b05bc2，后续只调整维护夹具初始化、配置解码的 test driver 接口与测试边界）。本节不把上述登记测试的通过扩大为全部 CPC 逐文件原生阅读资格；真实读取行为与远端三轮链仍按各自验收范围取证。独立审核及远端保护接入仍待对应验收。

### 实际正文差异与字节数

真实交付在源码 `d87769c3dacddbeee5e442b8c949f903a7a11bb5` 上遇到模型输入等待：模型把调用方原快照摘要与运行时进度文件原始字节摘要比较，因不符而停止。原报告 `test-results/L4/20261028T155629518Z-live-v2-evidence/summary.json` 保留失败及完整清理，[原生记录](../records/d87769c3dacddbeee5e442b8c949f903a7a11bb5.json)未改写为通过。未向旧 Run 补充解释或手填进度。

为消除这项已观测歧义，唯一短模板将“原快照摘要”改为“调用方快照标识”，完整规则新增说明：该标识关联内部冻结安排，由调用方核验；执行方核对固定 Git 文件中的安排、定义及 note，按布尔进度协议处理，不与进度文件摘要比较。原规则没有此边界，新增文字用于修复本轮实际阻塞，不增加第二份规则或 inline 材料路径。实际可读差异保存在原资产与黄金样本的 Git diff：

| UTF-8/LF 资产或实际正文样本 | 修改前字节 | 修改后字节 |
| --- | ---: | ---: |
| `testing-execute.md` 短模板资产 | 417 | 423 |
| `testing-reference.golden.md` 实际正文 | 458 | 464 |
| `testing-step.golden.md` 含用户原 CRLF 的正文 | 486 | 492 |
| `materials/testing.md` 完整规则 | 2963 | 3196 |

其余短模板与完整规则未改。完整规则本次 SHA256 为 `587708f00a71eb6a992072c5b12c5aa7f92ed327e2c614e11e33fa4e2e604542`；字节差异证据为 `diagnostics/inputs-20261028T1402Z/snapshot-identity-rule-change.json`。更新正文样本与规则摘要只触发显式审阅，不代表独立批准；本轮仍需独立审核，不合并或修改远端保护。

## 2026-10-28 main rebase 与完整 L2/L3 复验（前轮）

本次按用户要求 rebase 到 `origin/main` 的 `82161c42e925d81f705c4e0496b47e293897db5b`，随后重跑目录内全部 L2/L3，发现失败后直接修复。下方“前次文档驱动实施记录”中的旧基线、局部验证与失败状态保留历史，以本节为该轮结果。

rebase 冲突保留 main 的 Plan 历史，并把 PR399 的新增执行约束迁入唯一完整规则文件：允许规范必需的环境/输入/输出参数，但不得缩小测试选择；只报告当前步骤事实，不因索要未来 PR URL 阻断报告。`internal/prompts/materials/testing.md` 的 LF UTF-8 字节从 2720 增至 2963（+243），9 个短模板未变；同步规则摘要与既有快照测试，不恢复已移除的内联长指令。

受测源码为干净提交 `82b3158dfef9d07a9a8dc423c51785999934c3e7`，Windows/amd64、Go 1.26.4。以下证据相对独立根 `C:/SteerlineDebug/wt-2352-all-l2-l3/`；L2/L3 选择集合逐项等于目录内对应级别的完整集合，均为 controlled，零跳过，`cleanup_complete=true`。L3 最多两个场景并发。

| 最终复验 | 结果 | 准备 / 执行秒数 | 原始 summary |
| --- | --- | --- | --- |
| 全部 L2 | `smoke.delivery`，1/1 通过 | 15.591 / 85.452 | `test-results/L2/20261028T134428036Z-final/summary.json` |
| 全部 L3 | 7/7 通过 | 28.746 / 534.644 | `test-results/L3/20261028T133535726Z-all-after-fixes/summary.json` |
| L1 | 85 pass / 0 skip | 0.415 / 11.931 | `test-results/L1/20261028T134648591Z-final/summary.json` |
| PR 元数据关联回归 | 11 pass / 0 skip | — / 18.984 | `diagnostics/20261028T134648384Z-baseline-regression/summary.json` |

L3 包括 `configuration.execution-profiles`、`recovery.runner-wait-retry`、`recovery.waiting-merge-restart`、`runtime.dual-home-operation`、`runtime.service-lifecycle`、`runtime.shared-runner-projects`、`testing.runtime-progress`。正式 L1/L2/L3 报告已通过原 `test-feedback record` 导出到[受测提交的 Git 测试历史](../records/82b3158dfef9d07a9a8dc423c51785999934c3e7.json)。后续提交仅补本节与该记录，不改变受测代码。

两次完整 L3 的失败原报告和原生导出均保留，不覆盖为通过：

- `test-results/L3/20261028T125150367Z-5e298df6/summary.json`，源码 `ddb3f4e064c92bcb52800b1928f9513cf73013f8`：远端重试夹具缺结果文件能力及环境就绪前提；原始场景为 6 pass / 1 fail，管理汇总为 5 pass / 1 fail / 1 incomplete（同包合并恢复场景）。[原生失败记录](../records/ddb3f4e064c92bcb52800b1928f9513cf73013f8.json)保留该状态。修复受控能力声明和真实 Git 固定材料，通过公开 `environment_start` 完成原 Run 停止、v1 结果封存、报告通过与环境准入，再启动重试任务；不写数据库绕过生产门禁。保留重启、重试耗尽、零额外业务输入与取消断言；夹具报告只证明 controlled 流程，不计原生环境资格。
- `test-results/L3/20261028T130910070Z-full-final/summary.json`，源码 `0e39cf9a303768df26db7dc509c85b51ba7eb433`：6 pass / 1 fail，进度场景在自动材料准备查询阶段超时，[原生失败记录](../records/0e39cf9a303768df26db7dc509c85b51ba7eb433.json)保留。确定性回归进一步复现 GitHub PR 关联查询会绕过所选自定义 provider、从 PATH 调用 `gh` 的缺陷：修复前两个边界子例失败，修复后通过。现在仅配置为 `gh`/`gh.exe` 的 provider 执行该能力，并使用配置的完整 executable；不支持的自定义 provider 保留原诊断，Gitea 路径不变。定向回归覆盖真实 Git 基线比较/阻断、Gitea 关联及命令选择边界。自动材料等待同时保留最近成功轮询，便于定位后续准备问题。

两项修复各自定向复验后，在同一干净源码上重新执行上述完整 L2/L3，全部通过。Web 测试前置依赖安装及构建通过；未执行 L4、真实模型/Orca、部署或合并，不将 controlled 通过升级为原生资格或完整 Full 验收。

## 前次文档驱动实施记录（历史）

关联 [Spec](../../spec/caller-prompt-convergence.spec.md) / [Plan](../../plans/caller-prompt-convergence.plan.md)。2026-10-28 按用户授权，在 PR407 固定模板机制上整合 PR406 的文档驱动修订。PR406 参考版本为 `e6b97c354f5467d7e847dadeb3d497a2ceca8f0b`；已对 `origin/main` 执行 rebase，基线仍为 `a5c12eafcb2bf6e442de07ded2c6fd4affeb9541`，无新增提交需要重放。未合并 PR406/407、修改 Runner 或部署。

本轮受测为 `345887d1d98224195a44b7744655c10420e3f221` 加本 PR 工作树，原 summary 保留源码状态；不冒充干净提交或真实 Agent 资格。Windows/amd64、Go 1.26.4，独立证据根 `C:/SteerlineDebug/wt-2352-document-driven/`。初次治理及 PR405 的结果保留历史，不继承其通过结论。

## 当前实现及边界

9 个短模板仍从 `internal/prompts` 唯一组装；5 份完整规则保存在同包 `materials/`。新生成请求使用原 `generate.repository` 固定提交，`generate.materials` 为空；原 schema 字段结构及结果验证保留。Roadmap 的 evidence 原文行枚举已从 schema 移除，结果仍逐字对照原文，不从 schema 旁路传送原文。

项目文件必须按正常 Git 流程准备，程序只核验，不导出或自动提交材料：测试 `.tests.json` 展开后逐项等于完整冻结定义及顺序；协议/writer 只允许 LF/CRLF 差异；Roadmap 原文、图、映射、确认规划逐字核对原权威；环境公开配置和完整前轮报告逐字对照 context JSON。详细固定路径和 JSON 格式见 Spec，不新增第二份安排权威、数据库字段、材料发布服务或临时仓库。

环境报告字段只维护在 `environment-execution.md`，生成与执行均须核验/读取。全部成功、失败、incomplete、未知和清理事实完整保留。当前渠道不能把刚封存的完整报告送入下一 Run 的新工作区，因此原轮完成失败后保存准备不足并停止自动新轮次；首次完整成功仍可正常就绪。此缺口不是“三轮自动链通过”。

用户回答按原通道发送，完整实际文本最多 2000 UTF-8 字节，多问题序列化后的文本合计；覆盖 1999/2000/2001、中文、emoji、选项及多题。超限返回实际字节和上限，未生成投递信封，不截断/摘要/转文件。已冻结请求仍按原身份观察、恢复和原 CAS，不重组装或重复提交。

| 验收项 | 已验证范围 | 状态与未验边界 |
| --- | --- | --- |
| CPC-D01 | 实际请求无长规则/源码/原规划 inline；9 个短正文黄金样本及完整请求/schema 反例 | 受控已验；原 Runner/用户正文继续由原来源决定，不把 caller 统计当总模型输入 |
| CPC-D02 | 真实固定 Git 文件、完整提交、规则/原文/确认数据一致、结果来源 manifest | 文件可用性/来源及受控读取已验；真实模型/远端 Agent 读取资格未验 |
| CPC-D03 | 缺 commit/reader/文件、无权限、错误原文/图/映射/规则、过期 context、缺/变 writer 阻断 | 已验；准备不足反例不计为业务完整流程成功 |
| CPC-D04 | 完整冻结安排/note/顺序、真实选择核验、步骤权限、原预留/CAS及进度恢复 | 已验受控链；未触碰旧现场 |
| CPC-D05 | 真实 Git 受控 Roadmap 生成/发布/入图；完整前轮报告文件；环境首次成功与失败阻断 | 受控已验；项目材料须正常 Git 准备，自动下一轮完整报告通道仍缺失 |
| CPC-D06 | 2000 字节边界、合法原回答、原冻结请求观察与恢复 | 已验 owner/受控；真实远端投递未验 |
| CPC-D07 | 已支持条件、准备缺口、原报告和未验资格分别登记 | 已登记；独立审核与远端保护仍待对应验收 |

## 正文变更与门禁

同一合成输入比较 `345887d1` 的黄金样本与本轮短模板，单位 UTF-8 字节，仅统计 caller 新增正文：

| 用途 | 原值 | 当前 |
| --- | ---: | ---: |
| 测试执行，同源 writer | 2183 | 458 |
| 含用户原文的步骤组合 | 2211 | 486 |
| 测试只读步骤 | 303 | 163 |
| 测试安排生成 | 415 | 258 |
| Roadmap 解析 | 765 | 245 |
| Roadmap 文档生成 | 1478 | 343 |
| 环境生成 | 5762 | 294 |
| 环境 prepare | 1022 | 272 |
| 环境 publish | 788 | 259 |

缺 writer 的旧内联样本已删除，新操作直接准备不足。长规则以真实文件完整提供，当前 LF 资产分别为 testing 2720、roadmap-parse 766、roadmap-documents 1363、environment-generation 4731、environment-execution 2877 字节。前轮报告保持完整，不计为删除的信息。长规则使用测试内少量字节摘要快照触发显式审阅，不再复制正文，也不是运行时签名或预算；短模板仍逐字黄金比较。更新期望值不替代独立审核。

## 自动验证与失败留痕

以下路径均相对独立证据根；未导出 Git 测试历史：

- `test-results/L1/20261028T195638654Z/summary.json`：固定 `l1.minimum` 通过，85 pass / 0 skip，准备 0.359 秒、执行 9.670 秒；包含规则字节快照、正文样本与有限源码守卫。
- `test-results/L2/20261028T194844659Z/summary.json`：固定 `smoke.delivery` 通过，准备 15.849 秒、执行 93.904 秒，`cleanup_complete=true`。
- `test-results/L3/20261028T194924885Z/summary.json`：`testing.runtime-progress` 通过，准备 15.353 秒、执行 122.348 秒，`cleanup_complete=true`；实际准备固定协议/writer/安排，保留原进度、重跑、错误、恢复/重启和输入不重复断言。
- `diagnostics/20261028T195810260Z-owners-pass/summary.json`：prompts/taskrunner/testmanagement/roadmap/environmentadaptation/runnerclient/sqlite 完整 owner 回归通过，307 pass / 3 预期 opt-in skip，28.671 秒。三个真实公开环境/Runner 专项未执行，不算通过。
- `diagnostics/20261028T195115621Z-app-verify/summary.json`：应用定向集 42 pass / 1 fail，376.743 秒；真实 Git Roadmap 全链及材料核验通过。唯一失败为旧环境用例仍期待自动第二轮就绪；已修正为首次成功准入、缺完整报告渠道拒绝新 Run 两条路径，`diagnostics/20261028T195605729Z-workflow/summary.json` 3 pass / 0 skip，58.454 秒。不能把前一失败报告记为全绿。
- `diagnostics/20261028T195351573Z-materials/summary.json`：完整前轮报告固定文件、过期 context/被改规则拒绝及生成队列专项，3 pass / 0 skip，71.957 秒。
- `diagnostics/20261028T200030406Z-frozen-recovery/summary.json`：为原已停止 Run 夹具正常 Git 准备协议及正式 writer 后，部署中断、Runner 替换/接管、启动取消 outbox 和原 Run 绑定回归通过，37 pass / 0 skip，280.188 秒；保留旧请求、身份和零额外业务投递检查。
- `diagnostics/20261028T195419564Z-final-owners/summary.json`：扩到既有结构检查的定向运行整体失败，225 pass / 1 skip / 2 fail；两项为 `TestTM04ExecutionBoundary` 及其 `repository_all_tags_windows_linux` 子项，仍是未改动的 `sqlite/environments.go -> environmentadaptation` 禁止依赖。已在首次治理的 main 基线快照与修订版本双向复现，原证据见 [治理验收](prompt-governance.md)。本次有限提示词守卫/L1 通过，不声称完整架构门禁全绿。

本轮早期编译/夹具不完整、缺文件和旧预期失败的原报告均保留在 diagnostics，未覆盖或自动重试。后续按具体失败修正并使用新目录复验；冻结请求恢复的早期缺协议失败已通过正常 Git 夹具准备修复，结果见上项。启动器的 `TestInstallLaunchersWritesCRLF` 在未改的 main 快照上也失败（期望写出文件数与现有四个文件不符），基线复现为 `diagnostics/20261028T200101353Z-baseline-launcher/summary.json`，13.394 秒；该问题不在本次提示词范围，不宣称全 app / Full 通过。

现有调试包装的受控 Runner seed 已组合既有公开 repository capability/schema，避免包内夹具继续声明旧 inline-only 输入；仅做公开夹具适配，PowerShell 语法解析通过，未执行新的调试发布或部署。

未开展真实模型、Orca/Unity 探针、业务 PR、原 Task18 两轮、旧运行注入、跨会话消息或子代理。未启用 GitHub 必需检查/独立审批保护；合入前仍需独立审核，不以本聊天自查替代。

以下各节均为 PR405 已交付范围的历史记录，原失败、证据位置及未验边界不覆盖。

## PR405 第一阶段实现与范围（历史）

实现基于 main `ab66befe2ae4bf13de3e580d62ca2e39d51c84df`；PR399 调查版本为 `48ad01467976aff768c11f5deaaf521efbe717ee`。仅整合本次需要的完整 writer、逐 ID 接线约束、发布/报告只读边界及规范/Plan 必跑并集，不合入 PR399 其他业务改动。未修改 Runner、存储 schema、模型执行入口、预算、旧运行或安装现场。

完整安排仍原样保存，紧凑提示包括有序 ID、来源、定义摘要和原快照摘要；准备阶段从固定 Git blob 核对 Plan、规范、逐项完整定义及 note。writer 只允许 CRLF/LF 等价，缺失或不同源仍完整内联。Roadmap 规范正文保留在冻结记录用于 manifest 校验，模型通过既有固定仓库读取；原 Markdown、确认规划及其他生成的必要 inline 材料不变。环境派生事实仅省去成功项冗长 message，原报告及全部失败、未完成、PR 未知/清理事实保留；阶段从原结果初始 JSON 取关联字段。

## PR405 自动验收（历史）

| 验收项 | 已验证范围 | 当前结果 |
| --- | --- | --- |
| CPC-A01 | 完整快照不改、有序 ID、定义/note/digest/Plan/commit 不一致及必需文件缺失拒绝、mutable checkout 不影响固定来源 | 已验收 |
| CPC-A02 | 无 helper 完整源码、同源/LF/CRLF 路径引用、不同源完整内联、逐入口约束；SQLite 拒绝错误 writer 分发；公开/用户正文保持、发布/报告只读 | 已验收 |
| CPC-A03 | 测试材料和输出 schema 保持；Roadmap 非快照原文和确认输入完整，规范 manifest 缺失/错摘要拒绝，真实 Git 受控文档生成/发布/入图链保持；环境 Report 结构说明完整 | 已验收 |
| CPC-A04 | 长成功明细投影、全部检查/L2 的 ID/状态、完整失败/incomplete 原因、PR URL/未知/清理、无报告诊断、原报告不变及确定性；prepare 保留诊断，publish 不重复前轮问题 | 已验收 |
| CPC-A05 | Unicode、多问题/选项/自由文字原样传递；已冻结环境请求不重准备/重发；原候选 CAS/恢复、L3 重启不增加业务输入 | 已验收 |
| CPC-A06 | 同材料 UTF-8 组成测量、有/无 writer、多 ID 长 note、长前轮报告及固定仓库规范；必要大输入保留 | 已验收 |
| 固定回归 | L1 `l1.minimum`、L2 `smoke.delivery` | 已验收 |

独立证据根：`C:/SteerlineDebug/cpc-9588/`。每次命令的完整参数及受测源码状态见该次 summary，原始日志不进仓库：

- `diagnostics/core-02/summary.json`：`./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1`，210 pass / 3 预期 opt-in skip，30.214 秒；三个 skip 为真实公开环境检查/result 往返/Runner CLI 绑定专项，本次不执行 live，不能算通过。
- `diagnostics/generation-01/summary.json`：Roadmap、app 的选定 44 项通过，包含真实 Git 受控流程及固定源反例；该次环境包编译失败，整次状态为失败，不能登记为整体通过。
- `diagnostics/environment-02/summary.json`：修正测试调用名称后，环境 owner 全部 30 项通过，15.735 秒。
- `diagnostics/answers-01/summary.json`：原生答案完整性用例通过，2.532 秒。
- `diagnostics/semantics-01/summary.json`、`diagnostics/measure-02/summary.json`：原安排、完整 writer、前轮事实、阶段身份/冻结恢复及最终安排测量通过。
- `test-results/L3/progress-03/summary.json`：`-level=L3 -suite=testing -env=controlled -scenario=testing.runtime-progress`，1 场景通过，`cleanup_complete=true`；准备 16.292 秒、执行 126.479 秒。新增紧凑输入到候选检查，保留原进度/重跑/格式错误/断连恢复/重启/原输入和清理检查。该次在提交前工作树执行。
- 同一干净提交的最终回归：`test-results/L1/final-01/summary.json`（44 pass / 0 skip，准备 0.546 秒、执行 10.123 秒）、`test-results/L2/final-01/summary.json`（`smoke.delivery` 通过、cleanup 完整，准备 14.542 秒、执行 99.831 秒）、`test-results/L3/final-01/summary.json`（`testing.runtime-progress` 通过、cleanup 完整，准备 17.702 秒、执行 132.549 秒）。正式报告已通过原 `test-feedback record` 导出到[受测提交的 Git 历史](../records/210790fa4f721db67cbad3dcbec7794f1e4f2bfe.json)；后续仅补验收说明及测试记录，不改受测实现。
- `python scripts/check_doc_links.py`：597 文件、本地链接、0 错误；`git diff --check` 通过。

## PR405 实际减量（历史）

以下为组件的 UTF-8 字节，不是最终模型总输入或投递稳定性结论。测试安排/选择/环境提示以前述 PR399 固定 Git blob 为比较基线；Roadmap 用 main 固定规范。未重复计算请求 JSON 中的持久化副本。

| 相同材料组成 | 整改前 | 整改后 | 保留内容 |
| --- | ---: | ---: | --- |
| 两 ID 长 note 执行上下文；同源 helper | 10893 | 2260 | 全部 ID/顺序/来源/digest，完整 5496 字节快照未改，规范/清单固定文件可读 |
| 同一安排；缺 helper | 10893 | 6004 | 仍完整提供正式 3253 字节 writer 及接线要求，不为减量省源码 |
| 测试选择指令 | 988 | 415 | 规范与 Plan 必跑并集、影响追加、空选择例外、原 schema 和全部 inline 材料 |
| 环境生成固定说明（Git blob LF） | 5896 | 5570 | 完整环境 Report 字段/语义、L2、真实 PR、用户准备及清理约束 |
| 环境 prepare 指令；同一活动/轮次/源码/摘要/失败原因 | 1228 | 1102 | 身份由原结果初始 JSON 提供，完整失败原因仍在 prepare |
| 环境 publish 指令；同一材料 | 994 | 788 | 保留本阶段约束，关联字段由原初始 JSON 提供 |
| 长前轮原报告 / 派生事实 | 15675 | 740 | 全部 ID/状态、完整失败/未完成原因、来源与 PR 事实；原报告未改写 |
| Roadmap 规范相关正文 + inline | 1319 + 10344 | 1917 + 0 | 规范完整固定仓库内容/来源校验；原 Markdown、确认规划仍完整 inline |

规范 5919 的历史工作区数字与本次 Git blob LF 的 5896 区别来自行尾，未据此虚报语义减量。小安排或缺 helper 项目仍可能有较大输入，不设比例门禁。测量脚本与固定来源/组成保存在 `diagnostics/measurements/measure.py`、`components.json`，安排/报告合成夹具及测量日志见对应 Go 用例。

## PR405 原失败与边界（历史）

- `generation-01` 的新增测试误调用不存在的 `Tick`，改为正式 `Observe` 后完整环境 owner 补测通过；保留原编译失败日志。
- L3 首次参数未经 PowerShell 字面引用，在参数校验阶段退出，未执行场景。正式调用对含点号参数加引号。
- `test-results/L3/progress-02` 的新增断言错误要求只发布 implementation 的受控 peer 也提供 publish 提示，等待超时；原产品候选实际已正常接受。改为核对已发布步骤，完整发布/报告权限隔离由真实 workflow 形状的 owner fixture 验证。该次失败且 cleanup 完整；新现场 `progress-03` 已通过。

本次没有新增人工必验项，未执行真实模型、Orca、Unity、真实业务 PR 或原 Task18 两轮。受控/静态结果不声称模型实际阅读、真实发布或输入投递稳定性通过。原现场及历史验收保持原状。

## 2026-10-29 剩余三项修复（复验中）

当前用户授权补齐材料、修复全集挂起及308上限后重跑全部L4。新证据scope为 `C:/SteerlineDebug/wt-2352-l4-repair`，旧现场及失败均保留。

- Go全集根因：`diagnostics/20261029T010756642Z-local-catalog-stress` 在disconnect模式捕获 `win32PipeListener.Close` 等doneCh而listenerRoutine仍等新连接，确认go-winio丢失关闭信号。固定上游 `7e8af9b09c4b3e8da1a8ba080484c8a84b71354a`（PR388），依赖要求Go1.26/x/sys0.48；100次目录场景压力复验 `20261029T011539238Z-catalog-fixed-stress` 58.438秒通过。
- 308：独立Runner [PR71](https://github.com/erranliu/SteerRunner/pull/71)，源码 `a03e30efdee964b1702332e1ec241bde9481db08`。公开paths选择原始文件，容量限制不提高。contracts/resources/workspace/application/integration定向通过；日志 `diagnostics/runner-selected-final.log`、`runner-recovery-selected.log`。调用方测试安排/环境/解析使用完整材料选择，源码调查保留整仓；显式manifest必须精确匹配。
- 同源材料通过正常Git准备：SteerlineTest克隆 `projects/SteerlineTest` 新提交 `268db75d9fcf872f21fa553ed9452ac0ddeba601`（原8855502）；308克隆 `projects/308` 新提交 `97106b9b4d8fc872d0b0acc170ddb8b3449a5d62`（原76919e3）。只加正式规则和writer，原脚本/测试标准未改。进度L4夹具提交前写原三ID固定安排。
- 前轮报告复用原结果文件initial JSON的previous字段；不新增RPC/材料发布器/业务仓库自动提交。完整64KiB边界、三轮/重启、取消和未知结果保持；`20261029T012717337Z-round-budgets` 与 `20261029T012829629Z-workflow-round-retry` 已通过。超限不创建下一Run，保留完整原报告。
- 固定短模板9项正文均无变化。`environment-execution.md` UTF-8/LF **2877→3249（+372）**，两个阶段新增相同一句：若初始JSON含previous，完整读取报告/诊断，不省略失败/清理，最终报告仍只用原字段。SHA256 `d8389c654c64a0058feb5be288ccdf77628f61183396e3e7d42479d5e7014b8a`。业务原因是使用既有文件渠道恢复三轮完整重验，规则副本由新项目提交提供；黄金摘要只触发审核，不代替独立PR审批。
- 诊断失败 `20261029T011802274Z-snapshot-and-rounds` 保留旧schema夹具和旧一轮断言失败；`20261029T012342138Z-contracts-repaired` 剩余公开流程/超时断言已由上述专项修复复验。`20261029T012937261Z-selection-and-boundaries` 与 `20261029T013047438Z-selected-manifest` 通过。未将诊断中的缺原生pin skip计为正式通过。

最终L1/L2/L3和13项L4尚在执行，不能复用上一轮10/13声明本轮已通过；实际结果、清理和原生历史将在执行后登记。依赖PR未合并，不更新安装包或旧Run。

### 全集挂起修复首次正式复验与结果文件接线修正

干净提交 `5e5c307e9fceb6934212bd10429297b5ff3816cc`：L1 `20261029T013208909Z-repairs`、L2 `20261029T013223588Z-repairs`、L3 `20261029T013414952Z-runtime-progress`、L4 go.full `20261029T013549377Z-go-full` 均通过。Full准备10.4秒、执行1153.3秒，**1342测试通过、0失败、0测试skip**；已越过原挂起场景并完成所有组。四份报告已通过原生record命令导出Git历史，未将无测试文件的包当测试skip。GitHub prompt-governance run40801141541通过。

预查原项目脚本发现其会原样保存初始JSON的previous。独立 `diagnostics/previous-metadata-repro/test.log` overlay复现 `unknown field previous`，冻结全集源码没有修改。`fixed-test.log` 验证修复及环境owner通过：以RoundFile承载原Report+只读输入元数据，保留时逐值对照本轮冻结初始文件，修改/未知字段仍拒绝；previous不进入Report领域对象。没有改原项目脚本、标准或宽松解码。

本轮最终规则资产 `environment-execution.md` 为 **2877→3530 UTF-8/LF字节（+653）**，其中已提交阶段2877→3249；最终新增只读元数据保留及冻结核对说明，替换两处“最终报告只使用正式字段”为“报告字段不变，previous仅作输入元数据”。SHA256 `06378bc4ec3ee2122fbb2bdf6813021821c5bf5c0a65f7020597cf8404554c2a`。九项短模板仍无变化。该次正式修正后会重新固定源码与项目材料，重跑L1/L2和全部L4；上面的通过只属于5e5c307e。

### f55ac94e 最终L4批次与308夹具修正

`diagnostics/remaining-l4-index.json` 对应六项受控、真实冒烟、测试管理、审计、远端目录和远端进度共11项L4通过，受测提交均为f55ac94e，清理完整；L1/L2同提交通过。正式记录已原生导出。真实测试PR690/691及远端环境验证PR692均正常合并，准确隔离base/head经provider核验；远端进度自身不发布业务PR。新材料提交为SteerlineTest `d1a64bd6d6f4c85e08fe529b0ed812b0ca6f4bb6`、308 `6e2948eb5e08ac2ddbfbe4842a83ecfc7219dee1`，Runner固定a03e30e/d56116...，完整身份见外部inputs/frozen-inputs.json。

308 `test-results/L4/20261029T024335786Z-remote-308`：原大仓库的真实模型自动材料生成已成功，调用方只读证明生成原结果为 `{tests:[]}`，但准入探针未提交对应固定空安排，先被test_materials_not_ready拒绝，尚未执行环境门禁，不能算308通过。修正仅在该隔离探针提交前提供task.tests.json空选择，完全匹配原Plan“无自动测试”和原生成结果；308环境验收原完整L1/L2/脚本/标准保持。失败summary及原cleanup=false保留，补充清理核验另记diagnostics/308-first-failure-*.json，不改写原失败。

调用方SQLite只读证据：0候选、0环境活动，原生成Run已有runner-cleanup confirmed。旧测试ref先保留并核验provider/原SHA，后续用全新隔离现场复验308及最终全集；最终通过不能由上述材料生成成功推定。

### 308正式完成、全集结构失败与测试归属修正

`diagnostics/308-repair-l4-index.json` 的308在d4d348c0通过（681.86秒，cleanup_complete=true）。真实自动生成按原390,779,400字节/16,287文件提交内的必要文件读取，不再越过快照限额；后续三轮均真实停止封存。活动终态failed/not_ready是环境要求未满足的预期负向结果，不是L4失败；完整L2保持三ID `l2.smoke`、`smoke.startup.first_map`、`stable.core_merge_three`，未执行时如实incomplete、无PR且前后准入拒绝。完整前轮事实通过初始结果文件到第二/第三轮，未改原脚本或标准。旧609b测试ref已在零PR/零候选/零环境活动及Runner收尾证明后，以原SHA lease清理；完整对象另存独立本地备份ref，补充 `308-first-failure-cleanup.json` 为true，原失败summary仍为false。

全集 `test-results/L4/20261029T031034768Z-go-full-final` 完成原始执行：1337通过、6失败、0测试skip，执行1441.6秒，原complete=false如实保留并已原生导出。单一根因是新model_test外部包对本模型包的自导入被纯模型依赖边界拒绝，触发5个owner断言及其父项计数；修正为同包测试，元数据保留/篡改/未知字段断言不变，生产实现未改。`diagnostics/20261029T033705744Z-pure-model-owner` 的全部相关边界及模型反例通过，继续L1/L2和全量Go复验。

测试期间PR407已在f55ac94e合并（377eb7a3），Runner PR71已在a03e30e合并（af29ee47）。main后续新增TTY实现7617cddc，不属于本轮冻结f55/d4d的真实链输入；本批结果不作为新TTY流程的live验收。本跟进差异只补空安排、纯模型测试归属及原生测试记录，保留各报告实际源码身份，不覆盖主线TTY规则。
