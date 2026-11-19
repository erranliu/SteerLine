# 重构维护自然语言执行与预算收尾验收规划

关联 [Spec](../../spec/maintenance-conversation-budget.spec.md)、[Plan](../../plans/maintenance-conversation-budget.plan.md)和[测试安排](../../plans/maintenance-conversation-budget.tests.json)。2026-11-18 已实施，所选受控、公开协议和本机真实整链验收完成。基线 `134fd29e`。

## 自动验收

| 项目 | 方法及通过条件 | 准备和触发 | 当前状态 |
| --- | --- | --- | --- |
| MC-A1 | 定向及公开 Runner 夹具、受控和本机真实执行，实施/修复自然语言回复完成审计，实际正文无固定回执字段，发布/冲突分支有效 | 接线和模板稳定后，正式 ID 见 Plan | 已验收 |
| MC-A2 | 失败/脏工作区/无提交/伪完成反例不能接受；审计只依赖对话与已有公开事实，充分回复可接受，缺口转最小补证 | 确定性夹具后再验证真实调用的无执行目录审计边界 | 已验收 |
| MC-A3 | 空预算默认一个目标，修复不重新选题；金额/时间/用量待定与用户结束驱动收尾；模型材料无预算数字 | 状态和持久化完成后，HTTP/SQLite/受控流程验证 | 已验收 |
| MC-A4 | 收尾、停止、人工答复、未知提交和重启边界不重发、不误接受，不丢已接受成果 | owner 测试及受控恢复流程 | 已验收 |
| MC-A5 | 受控及本机真实 PR 交付、原 PR 冲突续作、独立评审、无成果无空 PR，资源停止及自有远端资源清理 | 固定 Runner、本机 Codex 既有登录、隔离测试仓库及分支，实际版本执行前登记 | 已验收 |
| MC-A6 | API/CLI/SQLite 移除轮次，Edge 宽屏和 375 像素表单、余额、对话及实际请求展开准确，刷新不重发 | 组件/构建完成后运行维护 L3 | 已验收 |
| MC-A7 | 记录全阶段实际渲染正文差异和 UTF-8 字节前后值、业务理由、缺文件失败及独立审核结论 | 最终提示词和接线固定后，独立审核人/代理实施时安排 | 已验收 |

自动方法、选定 ID 与命令来源按 Plan 第 5 节执行。没有额外真人判断项，浏览器操作由自动场景完成。独立提示词审核属于仓库治理要求，实施时记录审核主体与结论。

## 证据登记

实现时记录实际受测 SHA、脏状态、固定 Runner 构建、方案及隔离资源身份、实际命令、summary 与原始证据位置、失败和清理情况。输出目录遵循[路径清单](../../paths.md)，格式遵循[测试指南](../../testing.md)。历史通过结果按原版本引用，文档静态检查与后续业务验证分别记录。

2026-11-18 文档检查在 `134fd29e` 加本次文档变更上执行。`python scripts/check_doc_links.py` 检查 818 份 Markdown、5005 个本地导航链接，零错误；测试安排的七个 ID 已与正式清单核对，有序且唯一。此段保留初稿文档检查，实际功能验证见下文。

用户于本次实施中确认真实验收在本机进行，Plan 和测试安排改选 refactoring.local-codex。产品后续审计仍只接收上下文，实施工作区访问权限不会因本机测试而授予决策调用。

## 实现及提示词审核

已移除维护轮次和轮次预算，以选题源操作、目标起点及接受操作关联成果。金额和时间由宿主判断，默认预算完成一个目标后收尾。实施/修复保存 Final 原文，审计根据上下文及公开事实决定下一步。API、表单和历史视图已更新，出站请求投影仅含指令、材料清单和输出约束。持久化契约升为 schema 52，直接替换旧维护数据格式。

固定 Runner 为独立冻结的 `84ef7e2d29f522dec04426eddf5c2ea398d43db2`，版本 `0.2.0-dev.4`，SHA256 `6490f64f1866e77efecba6d417fb93f5f41c9c433672bb116735fb673f967142`，副本及公开协议夹具位于本次隔离 scope 的 artifacts。

其 Codex execute/1 要求单行正文。首次公开契约测试发现多行新模板被拒绝后，改用单行固定模板和 JSON 字符串无损承载任务原文。模型输出保持自然语言，原任务与实际发送正文分别保存。新增含 CRLF 和结尾空格的实际 Runner 回归通过。

实际正文和黄金样本差异见[证据](evidence/maintenance-conversation-budget-prompts.diff)。UTF-8 字节以渲染结果计量。

| 样本 | 原字节 | 新字节 | 业务原因 |
| --- | --- | --- | --- |
| first | 616 | 1227 | 自然语言任务生成、推进许可、实际材料路径及实施/发布边界 |
| select | 604 | 1227 | 上下文选题、推进许可、实际材料路径及实施/发布边界 |
| audit 可继续 | 610 | 1592 | 远端不可访问、按对话判断及结果决定规则、实际材料路径及实施/发布边界 |
| audit 收尾 | 0 | 1625 | 新增收尾许可样本，实际材料路径及实施/发布边界 |
| report | 391 | 459 | 共用指令收敛，明确 context 和 diff 的实际材料路径 |
| report JSON 重试 | 476 | 544 | 沿共用表述、实际材料路径及现有格式修复 |
| implement/repair | 734 | 308 | 移除身份回填及输出 schema，任务原文无损单行传输 |
| publish/conflict 包装 | 488 | 488 | 沿现有交付回执契约 |

独立只读审核由本聊天子代理 maintenance_review 完成，批准当前模板、接线、固定来源、样本和对应测试定义。审核发现第二次答复引用第一道题、审计排队期间许可过期及真实证据 map 全 false 可通过三项问题，均已修复。对应问题关联与预算竞态的回归已通过，证据检查按目标 ID 要求 true。审核另实际运行 prompts 包，全部通过。

审核批准的 execute.md SHA256 为 `FF7D979B050CC27A9E0DCE6DCC61BFA3B1770EBC50388B9A56CBEF82C8B97D5D`，prompts.go 为 `849F331E6325873CDEC852B032F6F9EF67313C8F393CABB30A34A86C83C96C78`。字节记录只用于定位本次审核范围。

## 当前验证证据

受测版本均为 `134fd29e` 加对应执行时的工作树差异，实际源码状态以原 summary 为准。证据根为 `C:/SteerlineDebug/wt-99aa-maintenance-conversation/`。

| 验证 | 结果及证据相对根 |
| --- | --- |
| 维护/model/query/prompts/决策定向集合 | 通过，diagnostics/owner-01/summary.json |
| 单行原文、接受/无改动门禁、许可竞态、问题关联和查询白名单 | 通过，diagnostics/owner-02/summary.json |
| 多目标继续改进及接受起点 | `TestBudgetAllowsNextTargetWithPreviousAcceptedBaseline` 单项通过 |
| 前端组件 | 31 项通过，包含默认预算、自然语言回复、指令和出站请求展开 |
| Web 生产构建 | TypeScript 和生产构建通过 |
| L1 | 通过，test-results/L1/01/summary.json；后续单行适配差异由 owner-02、独立 prompts 与真实协议回归补测 |
| L2 | 通过，test-results/L2/01/summary.json |
| 维护 L3 | 通过，test-results/L3/02/summary.json；包含 Edge 宽屏/375 像素、自然语言失败修复、原 PR 交付和恢复、默认预算、并发及零重投 |
| Runner 结果专项 | 通过，test-results/L4/runner-result-02/summary.json；最终隔离构建另通过 runner-result-03/summary.json |
| 实际 Runner 的 CRLF/尾空格补测 | 通过，diagnostics/runner-crlf/summary.json |
| go.storage | 原失败已修复，完整重跑通过，test-results/L4/storage-02/summary.json，原失败保留 |
| go.http | 通过，test-results/L4/http-01/summary.json |
| 本机 Codex 真实交付 | 完整通过，test-results/L4/local-codex-04/summary.json；历史三次失败保留，详情见下文 |

L3 首次执行因浏览器仍期待旧的轮次耗尽文案失败，更新为默认目标完成及原文展示的断言后通过，原失败保留在 test-results/L3/01。Runner 结果专项首次多行输入失败保留在 runner-result-01，修复后通过。沙箱 npm 安装失败后在正常执行环境完成锁文件依赖安装。

## 基线失败与范围

`go.storage` 的唯一失败为 `TestActivityReadingEnvironmentSummaryExcludesLargeOriginalPayloads`，环境检测摘要返回 running，测试期待 waiting。使用未修改 HEAD 的 git archive 在 scope/baseline 独立目录复现同一失败，原日志为 diagnostics/baseline-storage.jsonl。本次维护相关存储测试和其余专项测试不因此登记整组通过。

额外执行 cmd/test-feedback 全包时，`TestAppDomainSelectionPreservesPlatformOwnersAndExplicitRoots` 仍期待 11 项 Roadmap app 根，当前清单已有 16 项。未修改基线同样失败，证据为 diagnostics/baseline-catalogue.jsonl。正式清单一致性按所选目录检查单独验证，历史失败保留原归属。

真实本机场景的启动命令为 `go run ./cmd/test-feedback -level L4 -suite refactoring -env live -scenario refactoring.local-codex -out C:/SteerlineDebug/wt-99aa-maintenance-conversation/test-results/L4/local-codex-01`。自动审批因使用登录凭据及 erranliu/SteerlineTest 专属分支推送、草稿 PR 和冲突续作的外部副作用缺少明确授权而拒绝该动作。已向用户说明目的地、操作和结束时关闭 PR/清理测试分支的范围，用户随后明确授权。该命令在明确授权后启动，真实结果见下文。

正式测试清单与执行器一致性检查通过，证据为 diagnostics/catalogue-01/summary.json。Markdown 本地链接及差异空白检查通过。首次执行时的基线失败和真实失败保留原报告，后续修复及补验另列。

### 两项基线失败的修复

按用户要求继续排查后，确认两项均为旧测试断言。环境活动投影根据业务检测阶段返回状态，夹具的阶段仍为检测中，因此断言修正为 `running / 检测中`，保留原诊断、绑定和大载荷排除检查。Roadmap 选择器断言修正为现有 16 项显式根，保留各根及源漂移检查。未修改生产状态映射或缩减测试范围，独立审核已批准。

两项定向回归通过，证据为 diagnostics/failure-fixes-01/summary.json。完整 go.storage 在 test-results/L4/storage-02/summary.json 通过；完整 cmd/test-feedback 在 diagnostics/test-feedback-02/summary.json 通过。上述原基线失败日志继续保留，不追改原报告。

## 本机真实执行及清理结果

用户明确授权后实际运行 local-codex-01。主行动 `98b5b36105e0ba5de7082d70cca46e3d` 完成首次调查、自然语言实施、一次审计要求的修复、再次审计接受、草稿 PR #848 发布及固定版本独立评审，进入 delivered。主行动及各操作原话、接受关联和 Git 事实的白名单证据在 diagnostics/live-partial-proof.json。模型审计在无执行目录的 generate 调用中消费上下文。

并发行动 `d95e677d9397f51aabc467a79f18669b` 的首个调查 Run `6a7391064505eb71ca350469204fdf1e` 在原生 prepare_session 阶段进入 outcome_unknown。固定 app-server 端点 `127.0.0.1:49734` 拒绝连接，尚未取得原生 turn。通过公开 inspect 保存诊断后，按 can_resume=true 使用新 request_id/当前 revision 作一次有界只读 resume，结果仍失败。随后以公开 stop 结算原 Run，确认 closed/stopped。

原场景的并发重叠等待未识别原生阻断，继续等待无法满足的条件。主代理中止该测试，不生成或补造通过报告。已给测试加阻断时及时失败的守卫，进入既有 cleanup，独立审核已批准，最终测试包编译通过，产物为 artifacts/refactoring-live-check.test.exe。整个 refactoring.local-codex 未通过，并发原生重叠及后续真实冲突续作尚未验证。受控 L3 中的原 PR 冲突续作验证已通过，两者分别保留。

人工收尾使用现有 runner-cleanup helper，公开 service verify-stopped 确认十个 Run 的资源均已结算，service status 为 not_running 且 last_shutdown.resources_settled=true。核对本轮登记的测试进程全部退出。测试 PR [#848](https://github.com/erranliu/SteerlineTest/pull/848) 已关闭，其 head `codex/runner-ddd7ffb6e36cf08aa7db0cde4dac03cb` 及唯一自有 base `codex/e2e/714defc3da5d48d4b049f45f1881fb7e/refactoring.local-codex/base` 已删除并重查确认。隔离 home 内的一份 auth.json 登录副本已清除，失败现场与诊断保留。

原始运行日志在 test-results/L4/local-codex-01。原 Run inspect、resume、stop 及核验分别保存为 diagnostics/blocked-run-*.json、diagnostics/live-verify-stopped.json、diagnostics/live-service-after-cleanup.json。远端 PR/分支及进程清理的最终证据为 diagnostics/live-cleanup.json。该场景中止后缺少正式 summary，不登记已通过或受管清理通过，手动清理事实按上述独立证据记录。

首次阶段的结论为：功能实现与代码/提示词独立审核完成，MC-A1/A2/A3 的受控与公开协议部分通过，MC-A4/A6/A7 已验收，MC-A5 真实部分及 go.storage 仍有失败。后续修复和最终补验见下文，原失败报告不追改。

## 原生启动修复与第二次真实补验

继续排查原 local-codex-01 的原生日志，确认并发 app-server 共用 CODEX_HOME 首次初始化 SQLite 时，一台退出。真实无登录启动探针复现共享目录仅一台就绪、独立 sqlite_home 两台就绪。历史 stdio reader 也会遇到同样竞态；原服务器关闭后的公开 thread/read 探针进一步证明，另开索引会遗漏原 Turn，原索引才能保留 Session/Turn。

修复在独立 Runner worktree `C:/Users/lmas1/.codex/visualizations/2026/11/18/01a1edd5-b35a-7502-8b43-12ee091099d6/runner-fix`，分支 `codex/maintenance-native-sqlite-isolation`。执行、历史读取、本机恢复、会话分页及远端审计统一使用原资源的 ServerDir/sqlite，认证和配置仍共享原 CODEX_HOME，原进程/端点/generation 及会话核验不变。完整 Codex 适配包通过（90 根 pass，1 个子进程专用 helper 根按设计 skip，不声明完整分级零 skip 资格），独立审核批准全部接线。证据为 diagnostics/runner-codex-02/test.jsonl 和 diagnostics/native-{sqlite,history}-probe-*/result.json。

最终固定 EXE 为 artifacts/steerrunner-sqlite-isolated-final.exe，版本 0.2.0-dev.4，源码基线 `84ef7e2d29f522dec04426eddf5c2ea398d43db2` 加上述未提交修复，source_digest=`411304498b6fd3d085eb1147578512bd6582022ec8ad3db18024b7683ae3e60d`，SHA256=`baf3d36aa2154182cb94a97aaf1b638c1ea02e96e65448f3162ea7152e7d8af7`。该构建的正式 go.runner-result 通过，证据为 test-results/L4/runner-result-03/summary.json。

local-codex-02 已保存两个独立原生会话同时运行的三次公开 inspect 样本，旧 SQLite 启动故障消失。真实模型暴露新模板淡化了材料定位：按相对 maintenance/context.json 查找，未读取清单 file 字段给出的绝对路径，三个调查 Run 因 human_input_required 失败。主行动自动重试后实施成功，随后一次审计在原 Runner account/read 检查返回 content_authentication_unverified，整链仍失败。原错误未保留底层 RPC 原因，未凭此放宽认证判定。原报告在 test-results/L4/local-codex-02；summary 和 scenario 均记录失败且 cleanup_complete=true，Runner 结算确认、测试分支删除、零 PR 及临时认证副本移除均由正式清理完成。

已修复决策模板，明确按 file 绝对路径读取，path 仅是逻辑名称，PR diff 沿同一规则。六份样本逐字差异及字节已更新，不更改目录访问或材料来源。新增行动 blocked/waiting_input/stopped 的及时失败守卫，避免原生 Run 已结束后继续等待不可能的重叠。prompts 和 maintenance owner 的 46 项测试通过，证据为 diagnostics/material-path-01/summary.json；真实测试包编译通过。独立审核批准当时决策模板 SHA256=`0B51446A67614AA89C0EAD891ED95450CD66E6CBF11404DE0637A86498C52DC0` 及守卫，随后在新目录 local-codex-03 补验。

## 实施验收与发布职责补正

local-codex-03 两个调查均成功读取材料，两个实施和审计完成，认证检查错误未复现。两个审计都将方向中的最终 draft PR 要求当作实施验收条件，要求 repair 提前创建 PR；Runner 沿原实施禁止发布规则处理，造成循环补证。主代理通过产品正式 refactoring_stop 停止两个本次行动，使原场景正常失败并清理，不把这一轮算通过。原报告为 test-results/L4/local-codex-03。

该轮 Runner 清理 confirmed=true，GitHub PR 清单为空、自有分支完成清理。主代理额外的只读诊断监视器没有及时关闭 SQLite 连接，导致 Windows 临时 DB 文件删除失败，正式 scenario 如实保留 cleanup_complete=false。已关闭监视器、核实本轮进程退出，并只删除核实过的自有场景残留目录，补清理证据为 diagnostics/local-codex-03-manual-cleanup.json。后续不运行该监视器，原失败报告不追改。

固定决策模板已增加实施/发布阶段边界：当前 execute 只安排实现、修复或必要补证，发布 PR 由 SteerLine 接受成果后安排，不属于当前实施验收条件。此条仅进入决策模板，未给实施 Runner 增加 schema、固定身份字段或机器回执要求。first/select/audit/closing 每份增加 145 字节，最终数值见上表。Spec 拟定规则和 Plan 同步澄清既有职责。

真实场景新增 GitHub createdAt 与首个宿主 publish 操作 CreatedAt 的秒精度比较，对两个行动都验证未提前创建 PR。原结构化 Result.PR 检查仍保留，但不依赖自然语言回执的空 Result 字段证明发布时序。正式 proof 保存两组时间及 pr_created_after_acceptance，入口校验该标志为 true，L4 清单同步。完整 prompts/maintenance/refactoring owner 的 138 项测试通过 diagnostics/publication-stage-01/summary.json；完整调度器包通过 diagnostics/test-feedback-03/summary.json，真实测试包编译通过。

独立审核批准最终职责条款、样本、创建时点判定及本机/远端证据校验，最终决策模板 SHA256=`82463A5E5B5771B9BD6A7C8B327F0911D4212515C4C3C259C12D9AA3E533B4EE`。清单一致性补测通过 diagnostics/catalogue-final/summary.json。上述修正后的真实整链使用 local-codex-04 新现场执行。

## 最终本机真实验收

local-codex-04 正式完整通过，测试正文含清理 604 秒，summary.passed=true、cleanup_complete=true，scenario 同样 passed=true、cleanup_complete=true。固定构建、源码脏状态和实际执行命令均在原 summary 和 build/frozen-binaries.json 中保留。

主行动 `734908342a0aa9b0f2c6ae608f2dced0` 在同一目标上完成自然语言实施、一次最小差异补证、接受、发布与版本化独立评审，草稿 PR 为 [#850](https://github.com/erranliu/SteerlineTest/pull/850)。并发行动 `13866cd30c91d161051ebcdd598b6742` 独立交付 gamma.cjs，PR 为 [#849](https://github.com/erranliu/SteerlineTest/pull/849)，分别持有自己的工作区、分支及成本。两行动 PR 的 GitHub 创建时间均晚于宿主首次 publish 操作时间，实际时间保存在 refactoring-live-proof.json。

原三次 inspect 样本发生在会话已建立、TurnID 尚未回填时，只能单凭它证明原生 Run 重叠。因此清理前补取原两个调查 Run 的完整公开 observe 页面，确认各自独立的 Session/Turn、原生 succeeded、结果 source verified、start_turn accepted 操作及受理/完成事件。公开原生回合生命周期区间为 11:08:59.0168256–11:09:20.361688 UTC 和 11:08:59.0336845–11:09:21.7240726 UTC，重叠约 21.328 秒。原页面和计算结论位于 diagnostics/native-overlap-04，不替换三次采样或正式报告，也不把中心通知时间宣称为模型内部时钟。

主行动初次 head=`7e99f9fdcd923a0ed4b954c5b00c0de2bcd5ad4c`，base=`8f3b33c9da190e152dbb0df4e25a2451449d3e40`。推进专属 base 后，Git merge-tree 确认 alpha.cjs、beta.cjs 的真实内容冲突。原 PR 续作后 head=`77acaa8d16da6acc11c6782517605f3e8c203fa8`，base=`6a499f74ea5a5a08659b77c945111c0753fce725`。两文件的简化、行为检查和并发注释保留，初次与冲突后两棵树的 node check.cjs 均通过。四次 execute 的最新持有者链和独立原生会话均通过；当前目标的接受操作与可信干净提交事实匹配。原 PR 正文保持不变，第二份独立评审对应新 head/base。

独立评审给出 insufficient_evidence，理由是固定评审材料没有 CI 检查结果；这是实际建议，不登记为建议合入或 CI 通过。验收程序独立执行的两个 node 检查通过与评审材料的证据限制分别保留。publish/conflict 的原结构化回执完成，未把实施/修复恢复成 JSON 回执。

最终证据集中在 test-results/L4/local-codex-04/scenarios/refactoring.local-codex：refactoring-live-proof.json、maintenance-view.json、maintenance-peer-view.json、initial-report.json、conflict-report.json、conflict-proof.txt、两份 tree-check.log、maintenance-finals、scenario.json 和 maintenance-live-runner-cleanup.json。原始日志与测试选择见同级上层 summary。

正式清理确认所有本轮 Run 与服务结算，关闭两份 PR、删除唯一自有 base 和两份 head，移除临时认证副本和场景目录。只读复查两份 PR 均 CLOSED，三个精确分支均不存在，登记的自有测试进程退出，证据为 diagnostics/live-final-cleanup.json。MC-A1 至 MC-A7 本次均已验收；远端 Orca 实际运行不在用户选择的本机范围内。

独立审核最后核对正式通过、两个代码树、PR 时序、版本化评审、清理及补充并发原页面，确认本机需求证据充分，无需扩测；同时明确并发证明范围为两个首个只读模型调用，评审未建议合入。最终文档链接检查为 818 份 Markdown、5009 个本地导航链接，零错误；两仓库 git diff --check 通过，Runner origin/main 仍与被测源码基线一致。已通过的早期 L1/L2、HTTP、浏览器和 UI 构建按原版本复用未受影响的覆盖，后续提示词、断言与 Runner 差异分别由上述 owner、全包、公开协议和真实链补验。本地验收结束时，代码保留在本次两个工作区，尚未提交、合并或部署。
