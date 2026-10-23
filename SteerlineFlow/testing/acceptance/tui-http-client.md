# 任务 #91 验收：TUI 经 HTTP 接入 Go 后端

> 2026-10-15 TUI 退役：本文保留原交付/验收事实，TUI 专属操作和未验项不再适用，不记为通过。共享后台/API 要求继续归原记录；当前范围见 [TUI 退役](../../spec/tui-retirement.spec.md)。

唯一归属 #91；[Spec](../../spec/tui-http-client.spec.md)、[Plan](../../plans/tui-http-client.plan.md)。2026-10-13 独立复审修复及最终自动复验通过，[PR #299](https://github.com/erranliu/SteerLine/pull/299)已真实合并，#91 实现完成。用户委托的 Windows ConPTY 真终端和 SteerlineTest live 操作已验证；TH-M1 的原生查看窗口渲染及人工体验曾待验，后续 TUI 退役后不再是当前门槛，也不追认为通过。汇总归[execution-map](../../Roadmap/execution-map.md)，不覆盖其他任务历史。

| ID | 方法/入口 | 最小行为和通过条件 | 当前状态 |
| --- | --- | --- | --- |
| TH-A1 | TestTUIHTTPBoundary、L1 边界和源码访问检查 | 实际 app 委托 consoleapp，HTTP consumer 闭包无 SQL/backend/handler；生产 console 只收端口，客户端无业务文件回退，仅允许连接、自己的窗口/原请求及精确生命周期观察 | 自动通过 |
| TH-A2 | Contract/Interaction/Routes、tui-http.local | 查询保持正式投影；TUI final reader 经 HTTP 读跨 Attempt 历史、中文多页正文并核对 UTF-8/摘要/revision，拒绝错身份/迟到/失效游标。模拟正式服务明确返回采集 gap | 自动通过；新增 live 原生 final 采集/正文验证，限本次 Codex 样本 |
| TH-A3 | Contract、原 TUI 开始/反馈版本回归、tui-http.local | 原控制/反馈/环境 target/version/revision/Attempt/recovery 保留；先存原信封再 POST；丢响应、重复、重开、异内容/异目标与换实例被核验。真实 TUI 的原 request 重交仍只有原 run/Execution | 自动通过；复审新增资格取消/迟到、未知状态、任务版本加载回归通过 |
| TH-A4 | TestTUIHTTPRoutes、tui-http.local | 真实原 run 恢复可查，错 Execution/Attempt 提前拒绝；受控打开器绑定 session/generation，重复/unknown 不再调用，不能占用正式 command ID，无新业务轮次 | 自动通过；真实窗口归 TH-M1 |
| TH-A5 | Contract/Interaction、tui-http.local | 原 HTTP applied、原进程退出与两个锁释放及 Job 清理均核实；丢回执保持未知；TUI 关闭后 worker 继续，重开只观察原执行，断线保留观察并禁写 | 自动通过 |
| TH-A6 | 24 文件删除清单、build-only、静态检查 | 三个 WinUI 工程及 global.json 已删除，专属 ui bin/obj 忽略项已清除，当前无 .NET 构建入口；Go 构建/导航/身份通过，历史失败/未验保持 | 自动通过 |
| TH-M1 | Windows 真终端及人工体验判断 | 原键位、中文多行、候选/反馈/final/原会话入口可用，无误提交或串线；记录原身份/步骤/证据 | ConPTY 操作已验；原生查看窗口渲染、桌面输入法/字体体验仍待人工判断 |

环境为 Windows 源码定向与生成的 C:\SteerlineDebug 独立 SteerlineTest 包。生成默认 live；受控驱动显式 `--simulate` 冻结方案，live 另用新包执行真实模型和专属远端 PR，各次范围分开登记。命令、时点、证据逐项映射见 Plan，新增入口须先落实、非零运行；原安装目录不作现场。

TH-M1 可在自动通过后集中到任务签收前，不阻碍无关合入；已知错误、缺接口/能力和自动失败必须修复。原生能力限制不因 TUI 迁移被豁免。

## 结果登记

执行时逐项记日期、源码 commit/tree/dirty、exe摘要、包/服务实例、项目/Task/run/Execution/Attempt/request、实际命令、非零枚举、结果及证据。原始产物放本次新 evidence，仓库只存最小摘要，不提交凭证/DB/完整提示。必要回归另栏登记，不以 L1 代替 TH 自动项。

下列实施及现场记录形成时尚未独立复审/合并；最终状态见文末复审与合并登记。#44 的用户撤销旧人工要求及工程删除不等于原项通过，见[退役记录](archive/superseded/phase2-winui-foundation.md)；#45 历史失败/风险接受保持。

## 固定版本与实际现场

启动 Spec/Plan 固定于 `83a6cc4a7321994a55e5a9ae0f308927eb9fb1b3` 的 `docs/spec/tui-http-client.spec.md`、`.plan.md`；当前 Plan 只澄清入口分工，不改变启动范围。执行起点 `efd492cac9940a50966b3ccdb65d56012931d1d6`，分支 `codex/tui-http-client`。

生产实现 commit `76c5c8e7ce8802f5450081a52462ea46540606d8`；最终验证 commit `2c58de3207ab76aeec5e69ee8421787137051183`，tree `58a45cc876a1d4e5cf5e12dd68ecb6a5cb1b64f2`，clean。后一个 commit 只改边界/测试工具/驱动，生产 TUI/HTTP/Go 链路相同。

本次 evidence：`C:\Users\lmas1\AppData\Local\Temp\steerline-task91-20261013-125616540`。以下相对证据均在此目录，不提交凭证、DB、完整提示或原始产物。

最终包 `C:\SteerlineDebug\dev-20261013-130631-2c58de32`：正式生成器后显式 simulate 冻结；真实 Go/Git/SQLite 配受控 worker/provider，只用本次 SteerlineTest 专属 base/head，无模型或真实远端 PR。Job 无剩余后代，包与日志保留取证；未操作安装环境或其他既有进程。

| 事实 | 精确值 |
| --- | --- |
| 产品 exe SHA-256 | `674bc20933409d0cbb601d7499a94ce4a1ed49750fc2216cc23edd878a5fe304` |
| 项目身份 | `c:\steerlinedebug\dev-20261013-130631-2c58de32\projects\steerlinetest\.git` |
| run / 原 start request | `0d9bef72ba38620e77d2b90188c7a139` |
| Execution / Attempt | `17f9ff37e2f0b23b3bdc546f51566e46` / `353fa111c1e42371ef440b91119986a5` |
| 服务实例 / 原 stop request | `3c04286c0467bfe8cf8cee11d7d56322` / `c5cbefc0ea3cd380d8ae34113286cbcb` |
| 停服 | 原 receipt applied；process_exited/locks_released 均 true；原进程身份与锁路径在本包受限 discovery 保留 |
| 轮次与 final | 原会话三轮；final_bodies=0、final_gaps=1、native_final_verified=false |

simulate 的正式采集诊断为 `Codex app-server identity, version or home is unverified`。本任务验证缺口经 HTTP 如实返回，不伪造原生版本/正文，不将原生采集记为通过。完整正文、中文分页/摘要/历史 Attempt 的传输与 TUI 受理由专门 HTTP 交互和正式 API 查询测试验证；真机体验仍归 TH-M1 和既有平台验收。

## 命令、非零运行与证据

| 命令/范围 | 版本及结果 | 相对证据 |
| --- | --- | --- |
| `go test ./internal/app -run '^TestTUIHTTPBoundary$' -count=1 -v` | 76c5c8e7 clean，1 顶层通过 | boundary.log |
| `go test ./internal/product/httpclient -run '^TestTUIHTTPContract$' -count=1 -v` | 76c5c8e7 clean，1 顶层/5 子场景通过 | contract.log |
| `go test ./internal/console -run '^TestTUIHTTPInteraction$' -count=1 -v` | 76c5c8e7 clean，1 顶层通过，实际 HTTP snapshot/final/正文/离线/停服消费 | interaction.log |
| `go test ./internal/product/httpapi -run '^TestTUIHTTPRoutes$' -count=1 -v` | 76c5c8e7 clean，1 顶层通过，恢复/原生/历史错绑定 | routes.log |
| `powershell -NoProfile -File scripts/test-tui-http.ps1 -DebugRoot C:\SteerlineDebug -OutDir <evidence>\runtime-final` | 2c58de32 clean，1 场景通过、144.47 s，passed/complete/cleaned 均 true | runtime-final/summary.json、runtime.log；本包 evidence/tui-http-service.log |
| `go run ./cmd/test-feedback -build-only -out <evidence>\build` | 76c5c8e7 clean，通过、32.439 s；编译不作为业务通过证据 | build/summary.json |
| 删除及文档：`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff <base> --check` | 最终文档 356 Markdown/2160 本地链接/0 错；92 身份，1 草稿/1 正式任务；格式通过 | winui-deletions.log、doc-links-final.log、task-ids-final.log、diff-check-final.log |
| 回归：`go test ./internal/product/httpapi ./internal/console -count=1` | 76c5c8e7 clean，通过；不替代 TH 自动项 | regression.log |
| 回归：`go run ./cmd/test-feedback -level=L1 -out <evidence>\l1-final` | 2c58de32 clean，12 组/26 包，passed/complete 均 true；执行 128.793 s。未选 Full/L2/L3/live，原生 opt-in 不推定通过 | l1-final/summary.json 与各组日志 |
| 测试工具/边界：`go test ./cmd/test-feedback -count=1`；`go test ./internal/testkit/boundary -run '^TestTM04ExecutionBoundary$' -count=1 -v` | 2c58de32 clean，通过；UI 注入与 HTTP 无 SQL/backend 的正/负例 | test-feedback.log、http-boundary.log |

实施中另执行原 HTTP 集成 6 项：TestAPIQueryParity、TestAPIIdentityAndAuthentication、TestAPIControlRequestFaultRecovery、TestAPIOriginalCommandReceipt、TestAPIServiceLifecycle、TestAPIReconnectAndUnsupported，全部通过（379.310 s）。该次为 HEAD 83a6cc4a 加当时实现工作树，未记录精确 tree，不作为最终构建重新验收；保留工具返回事实，最终版本以表内证据为准。

## 失败保留与修正

早期独立运行发生工具环境缺失、无终端尺寸未发开始、恢复 nil 资源探针崩溃及停服查询过慢/期限不足；分别补真实工具环境/窗口尺寸、保守 nil 处理、连接复用与停止观察期限。失败日志原样保留在临时 steerline-tui-http-runtime-20261013-* 及各自独立包；首轮链路通过位于 steerline-tui-http-runtime-20261013-123302822，不替代最终版本。

76c5c8e7 的 l1/summary.json 为失败：旧清单仍引用已删除 workerreturn，旧 console 边界不认识纯 HTTP 测试依赖。删除失效组，保留生产 console 只收端口的规则，并补 HTTP 到 SQL/query 的负例；随后 l1-final 全部通过，没有禁用业务边界。

76c5c8e7 的 runtime/runtime.log 为失败：驱动错误要求 simulate 必有原生正文，而正式查询明确返回来源未核实 gap。改为要求正文或带诊断的正式缺口，分别计数并保留 native_final_verified=false；随后新包 runtime-final 通过，不把缺口记为采集通过。

## 用户委托的真终端与 live 验收（2026-10-13）

先运行 `scripts/new-debug-release.ps1`，生成 clean `12dbad80dfbfa6e594c1201723d293a3f8dfaace` 的独立包 `C:\SteerlineDebug\dev-20261013-135207-12dbad80`，冻结 live。通过包内 `start.bat` 与 `console.bat` 启动；实际 TUI 在 Windows ConPTY 中运行，以键盘输入和终端输出验收，不用测试模型代替终端。原始证据在该包 `evidence/tui-http-native/`，其中 `source.json` 固定版本、exe 摘要和 Codex 0.159.2。

| 身份 | 值 |
| --- | --- |
| exe SHA-256 | `5e9aabcc539b68e83bd52868c528dc83e0238a2ddb3710db09183331a64e6ef4` |
| 服务 / 原 PID | `6f5bd3bb9eff62e188ee649f6dbbb6a2` / `56032`，精确创建代际见 identity-start.json |
| 原 run/start request | `25ceb9e820503ca9004c7a0991fb2b3d` |
| Execution / Attempt | `880f81915070a23592da7a23fad263d5` / `b6ec5a2d2faee8d4fca9bbde9139e34f` |
| 原 session / generation | `01a133da-e18e-79a3-8afe-53dd97de2b57` / `7f1e76527d84375f2f9d44902c0c2edf` |
| 专属 PR | [SteerlineTest #230](https://github.com/erranliu/SteerlineTest/pull/230)，draft/open；base `codex/debug/dev-20261013-135207-12dbad80/base`，head `codex/1-25ceb9e82050` |
| head commit | `56ca46608ae1e820a2bab80cdb5d8c31c024134b`；仅 result.txt 两行新增 |

| 操作 / 对应项 | 实际结果 | 证据（上述目录内） |
| --- | --- | --- |
| s → Live → F8 → Enter；TH-A3 / TH-M1 | 原方案背景可读，确认后创建原 run/Execution；三阶段到 await_merge，真实 PR 限专属 base/head | terminal-ansi.log、snapshot-after-start.json、snapshot-before-feedback.json |
| f、[；TH-A2 / TH-M1 | 原生 canonical final 可阅读及切历史；初始三个正文逐个核对 UTF-8 字节、SHA-256、revision 和完整性，全部通过，无采集 gap | final-index.json、final-body-checks.json、terminal-ansi.log |
| Ctrl+N；TH-A4 / TH-M1 | HTTP applied；实际 Codex resume 进程绑定原 session、原 worktree 和本包 app-server。终端输出延迟时进行了两次打开，记录两个独立打开 ID；未产生新业务轮次。未检查该原生窗口的渲染 | native-viewer-process.json；本包 ui/http-requests 的两条原信封 |
| a → 中文两行 → F8 → F2 → F8 → F2；TH-A3 / TH-M1 | 正文完整保留；只读背景中 F2 不提交，返回反馈区显式 F2 只生成一条回答 `fa4e8a12bc0b541690c9ce6567c46a91`，源 Attempt、决策版本及 run/recovery revision 不漂移 | hidden-f2-draft.json、feedback-original-result.json、terminal-ansi.log |
| q、重开、u/F8；TH-A4–A5 / TH-M1 | 关闭客户端后服务/worker 继续；反馈使同一 run/Execution/Attempt/session 到第六轮，复用 #230/原 head；重开可经 HTTP 只读恢复核验，无替代执行 | snapshot-after-client-close.json、snapshot-before-stop.json、recovery.json、final-index-after-feedback.json |

中文反馈为两行：“中文第一行：请只复核当前结果，不修改其他文件。”及“第二行：确认后在原会话继续，并复用当前 PR。”最终索引六个 available canonical final，未出现 gap；正文逐项摘要验证覆盖前三个，不把后续索引观察说成六个正文都已逐项核验。本次未合并 #230，保留 draft/open 取证；未验 TaskDelivery/后继任务解锁，这不属于 #91 的 HTTP 客户端交付要求。

### 验收发现、修复与固定新包复验

1. 离线提示仍声称可看本地历史：改为只保留本窗口观察与草稿。`105329712b481d79da5612b185c81443d131cc9f` 新包冷启动离线实际显示新提示，任务列表为空，无业务库回退。
2. 展示元数据读取与后台 HTTP 共用互斥锁：为缓存 Project 分离读写锁，新增 Contract 第六场景，用阻塞中的 snapshot 请求证明展示读取在 250 ms 内返回。上述 commit 的 client/console 全包测试及消费/边界定向通过。
3. 真终端停服返回 identity_conflict：TUI 保存的控制 revision 被 HTTP 映射及正式请求登记丢弃，服务退出但原回执无法匹配。修复 commit `708dab2cdad45ee55ee030aa6248a5270e822c5f` 保留原 revision；停服仍按原实例执行，不新增 revision 阻止停服的语义。新增 `TestAPIStopPreservesDisplayedRevision` 使用真实 host 登记入口，核对提交/回读/重复只有一次正式登记；原 payload 对账负例继续通过。旧现场未知状态和缺回执保留，`cleanup.json` 只证明原 PID/锁已清理，不冒充 HTTP applied。

最终 clean commit `708dab2cdad45ee55ee030aa6248a5270e822c5f`，tree `cd0b205bd21423f3929c18dd891d30e6ecfc1224`。另生成新包 `C:\SteerlineDebug\dev-20261013-144411-708dab2c`，冻结 simulate，只复验键位/停服，不把同包切 live；exe SHA-256 `f1b347940085313e96268204e90062d003247c4a8e6d7b21a00d121b24deed8b`。

实际终端 `x → Enter` 的原 stop `b2aab133984dacf31e183a1c20101157`、实例 `93e0caada4be3bb2287db912af0d0e1d`、PID `51656`：原 HTTP receipt applied，原信封和正式请求 revision 同为 1；原进程退出且两把锁均能独占打开，TUI 保存 action=applied。证据在新包 `evidence/tui-http-stop-final/` 的 identity-start/source/stop-original-result/stop-proof/view-after-stop JSON 与 terminal-ansi.log。之后 q 正常退出；本次手工启动的两个服务及所有 TUI、原 viewer 均已结束，调试包保留，未操作安装环境或其他现场进程。

| 最终复验 | 结果 / 证据 |
| --- | --- |
| `go test ./internal/product/httpapi -run '^(TestAPIStopPreservesDisplayedRevision\|TestAPIMappedControlPayloadBinding\|TestAPIAuthoritativeControlBinding\|TestTUIHTTPRoutes)$' -count=1 -v` | 非零通过；`dev-20261013-142454-10532971/evidence/tui-http-postfix/stop-revision-regression.log`（修复工作树，与 708dab2c 相同内容） |
| `go run ./cmd/test-feedback -level=L1 -out <postfix-evidence>/l1-stop-final` | 708dab2c clean，12 组/26 包，passed/complete 均 true，223.458 s；同目录 summary.json/log |
| `powershell -NoProfile -File scripts/test-tui-http.ps1 -OutDir <stop-final-evidence>/runtime` | 708dab2c clean，真实 TUI 模型/HTTP/Go，221.72 s；passed/complete/cleaned 均 true。生成包 dev-20261013-144629-708dab2c，三轮、原 stop `608d0ddc8149bf3be929d4b8dceb17a3`，原 applied/进程退出/锁释放通过；runtime/summary.json、runtime.log |

simulate 的明确采集 gap 与 live 的原生 final 分开计数。已覆盖真终端键位、中文 Unicode 多行、候选/反馈、final、恢复和停服；中文由 ConPTY 注入，未使用桌面输入法。TH-M1 剩余原生查看窗口内容渲染及人工字体/输入法体验判断保持待验，不标为全量人工签收。该次现场验收结束时独立复审与 #299 合并尚未完成，后续结果如下。

## 独立复审、冲突解决与最终复验（2026-10-13）

用户直接授权复审、修复、验证及合并。独立审查使用无本聊天实现历史的只读 `codex review` 进程；前三轮审查完整 PR 差异，第四轮复核最后修复 commit。只读审查未执行 Go 测试，实际验证由实施聊天执行，不将静态审查说成动态测试通过。原始证据：`C:\Users\lmas1\AppData\Local\Temp\steerline-pr299-review-20261013-174011`。

| 轮次 / 候选 | 发现与结算 | 证据 |
| --- | --- | --- |
| 1：641b95c5，base 231fa360 | 三项 P2：活动始终首个 200 条、资格 HTTP 阻塞 Update、重开停服缺原进程代际。9d4f4695 修复，近期活动投影、异步资格及实际子进程停服恢复回归通过 | independent-review-1.log、review-regression.log |
| 2：9d4f4695，base 231fa360 | 三项 P2：pending_verification 日志不能重开、Close 等待未取消的 HTTP、CM72/CD66 仍用旧本地消费夹具。82dc448c 修复，真实窗口日志重开、阻塞 HTTP 关闭和在线/离线 HTTP 夹具通过 | independent-review-2.log、pending-stop-reopen.log、cli-http-fixture.log、offline-http-fixture.log、review2-client-final.log |
| 3：82dc448c，base 0b7652ac | 两项 P2：outcome_unknown 直接进入不支持该值的日志、详情未加载时提交零任务版本。24cdd7a1 将 synthetic 控制/命令/反馈 unknown 统一映射为 indeterminate；开始操作等待原任务版本再冻结提交；对应回归通过 | independent-review-3.log、review3-regression-final.log |
| 4：`codex review --commit 24cdd7a1` | 复核最后两项及 Windows 文件竞争修复，未发现新增可修缺陷；前三轮中未修改的部分仍由原完整差异审查覆盖。八项审查发现均已修复，无未解决项 | independent-review-4.log |

冲突仅在 `worker_lifecycle.go`：保留主干 #302 的共享 frozen 组装与独立只读 inspection，移除旧重复组装，保持 nil 资源不证明外部退出。merge commit `641b95c5` 接入主干 `231fa360`；之后 `ece49c74` 接入主干 #303 的 `0b7652acafe9d0a860c174b7d2f6f40d73261781`，无新增冲突。相关恢复定向 `TestRecoveryInspectionReadsBoundRunWithoutServiceResources`、`TestRecoveryInspectionReadsNativeClaimFromCenter`、`TestWorkerRecovery.*` 通过（324.445 s），证据 lifecycle-integration.log。

首次合并版本 L1 不完整：主干清单仍引用已重命名的 `TestTerminalClippedOutputCannotComplete`。对齐现有 `TestTerminalClippedScreenDoesNotBlockNativeFinal` 后非零运行通过，未跳过该行为（l1-integration/summary.json 保留失败，后续成功另存）。应用组装全包通过（189.056 s，app-regression.log），CM72 根场景通过（128.55 s），CD66 指定离线场景通过（25.09 s），均为隔离夹具，不触及安装环境。

82dc448c 的 runtime-reviewed 为失败：本包 `dev-20261013-181820-82dc448c` 在发布 ready discovery 时遭 Windows `Access is denied`，服务退出；旧普通 Go 读句柄排除删除共享可稳定复现（discovery-lock-repro.log）。共享删除读句柄单独不足以保证 Windows 替换，进一步对已知 sharing/access 错误作最多 40 次、10 ms 间隔的原子替换重试；持续错误仍返回原错误，未降低 ACL 或采用 copy/delete 回退。专属 Windows 测试核对短时持有、旧/新完整快照和持续占用失败保持旧快照（discovery-sharing-regression.log）。

最终测试固定 clean commit `24cdd7a116912ef43d4aa845b8a5830c552a21a3`，tree `92b7dbdeb6fa71678797b358d8dc8594d80c344f`，base `0b7652ac`。后续纯结果文档提交不改变被测生产代码。

| 最终命令 / 范围 | 非零结果与证据 |
| --- | --- |
| `go test ./internal/app -run '^TestTUIHTTPBoundary$' -count=1 -v` | 通过，boundary-delivery.log |
| `go test ./internal/console ./internal/product/httpclient ./internal/product/httpapi ./internal/product/httpfs -count=1` | 四包通过，含全部复审回归/Routes/Contract，consumer-delivery.log |
| `go run ./cmd/test-feedback -level=L1 -out <evidence>/l1-delivery` | 12 组/26 包，passed/complete=true，172.975 s；l1-delivery/summary.json |
| `powershell -NoProfile -File scripts/test-tui-http.ps1 -OutDir <evidence>/runtime-delivery` | 真实 TUI 模型→HTTP→Go、原请求/重开/退出客户端后继续/三轮/停服通过；173.81 s；passed/complete/cleaned=true；runtime-delivery/summary.json、runtime.log |

最终新包 `C:\SteerlineDebug\dev-20261013-184448-24cdd7a1` 冻结 simulate；exe SHA-256 `39217a61e5b3469c66bbcb5aeccf277da5fdf8fd929cb9534f4f1d1e91e922a5`。原 run/start `412ff1127729f5b3e3029c7c2db97e38`；Execution `2b7f2a407db5c2da17d9cadfb5682502`，Attempt `6b5279e03ef4902f700ff30412efd56e`；实例 `7888cdacb4c0b6bc5dc264f25eae2e64`，stop `baf431aefcf28fc05fccef569882eebe`。原 applied、原进程退出/两锁释放齐备，Job 无剩余后代。本轮四个调试包均无剩余本次进程（cleanup-final.json），包和失败证据保留。simulate 仍为 final 来源 gap，不替代前述 live 样本或人工体验。

GitHub 核验 #299 为 MERGED，`mergedAt=2026-10-13T10:55:58Z`；head `5fa28575b5bb23bb3c2fce47e56af0607e1b8ae6`（最终代码加结果文档），实际 merge commit `56552550641dc787c794bc6e486c94d41b90b0ad`。该提交两个父级分别为已核对的 main `0b7652acafe9d0a860c174b7d2f6f40d73261781` 和本 PR head；已 fetch 并核验进入 origin/main。合并使用 `--match-head-commit`，无强制推送或 admin 绕过；最终 head 的平台 CI 诊断返回零 checks/statuses，不冒充平台 CI 通过。本地验证以表内记录为准。

独立复审、必要自动验证和真实合并核验均已完成，roadmap 记实现完成并移除草稿任务行。TH-M1 剩余项继续后置，不宣称全量验收或远程部署通过。合并后只同步结果文档，静态导航/Task ID/差异格式通过，不以文档同步替代产品重测。
