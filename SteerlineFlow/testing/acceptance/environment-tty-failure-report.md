# TTY前置失败报告收尾验收

2026-10-30，用户授权修改并实测。[Spec](../../spec/environment-tty-failure-report.spec.md) · [Plan](../../plans/environment-tty-failure-report.plan.md)。普通开发，不部署安装环境，不修改Runner、caller状态机或报告版本。历史远端未完成报告保留于[原验收](environment-publication-scope.md)，不改写为通过。

## 两条链路签收（2026-10-30）

用户确认本机Codex、远端Orca检测流程验收完成，并决定本轮不继续追查Runner通用输入提交问题。签收按各自固定受测版本及实际正/负向范围登记，不把环境检测有效完成等同于环境ready。

| 链路 | 流程验收 | 环境结论及证据 |
| --- | --- | --- |
| 本机Codex | 完成，正向完整链 | SteerLine `edb20d721342409aa77acbe3ef24f0a109ff5d4b`、SteerlineTest `20610e10285d9ab9bb4ea3be0c07410158692fc0` 的正式L4 passed=true/cleanup_complete=true，TTY、完整L2、原PR发布/外部精确合并、ready后准入及停止均通过；详见[原固定版本验收](environment-publication-scope.md)。本次新增tty-failure原生分支在远端验证，不宣称已在本机Codex重跑 |
| 远端Orca | 完成，真实三轮负向闭环；TFR-A04通过 | SteerLine `861698809bbcd6164587709973731596fa350f24`、308 `1c25de58c87b99f44f9fc43f61de09444ba4391d`；三轮TTY均明确失败，原报告completed=true、原Run停止/结果closed、正常failed/not_ready/settled、无PR、准入拒绝及清理通过；完整L2/发布未执行，正向L4保持passed=false |
| 本机Orca | 未完成 | 2026-10-31补齐既有runtime配置及独立注册后已实际调用TTY入口，但找不到pwsh且报告未完成；见[最新续验](local-orca-environment-20261031.md)。此前前置阻断保留为历史，不借用其他链路结果 |

Runner后续修复通常不要求SteerLine代码改动，但可通过正式真实L4验证集成效果；本次没有接入或验证尚未交付的Runner修复。以下实现、受控验证及早期原生失败均保留，最新三轮证据见末节。

## 已实施及审核

同一项目脚本新增tty-failure入口，只记录已明确结束的TTY前置失败；在Git/Check/L2之前分流，复用原双文件原子writer。普通工具仅可调用此记录入口，不重跑探测、测试或发布。报告固定terminal.tty=failed、缺项incomplete，保留原身份/previous/同轮事实。初始空报告或同轮已完成且自有清理确认才可使用；发布事实/未知、PR进度、部分报告及非法字段/状态/ID/进度在写任一文件前拒绝。全部工具不可用或执行受理未知继续保留未完成，不伪造结算。正常三轮预算/停止封存/派发门禁保持。

独立审阅者 `/root/publication_scope_review` 先审核Spec/Plan，指出未知生产者不能借入口声称清理、passed只能保留、非法输入须先全量校验；已收紧。随后批准实际实现、完整execution/generation正文、三项目镜像及对应测试，逐项独立复算20份正文账本后才更新两项摘要期望。新增仅为原工具启动失败时的报告收尾，不增加配置平台、执行模式、Runner能力或跨文件事务。

实际正文差异：execution将“工具启动失败后只返回blocked”明确为“停止检测，仅用可用工具调用tty-failure记录负报告”，未知生产者禁止使用；generation新增同入口、原身份/事实保留及输入拒绝要求。项目文档给出对应命令；正常TTY、完整L2、PR发布标准不放宽。账本和完整Git diff：`C:/SteerlineDebug/wt-5069-tty-failure-report/diagnostics/review-8e37d94d673a458a809d5358b3e49587/`，原字节基线在同scope的`diagnostics/baseline-68e4f017b9384560b83116df27eeb2aa/`。

| UTF-8 LF文件/正文 | 修改前 | 修改后 |
| --- | --- | --- |
| execution（内部及三项目镜像） | 4579 | 5468 |
| generation（内部及三项目镜像） | 5834 | 7004 |
| environment-tty.ps1（三项目同源） | 1105 | 6000 |
| SteerLine verify / environment.md / TTY测试 | 10169 / 4282 / 5599 | 11092 / 5562 / 14115 |
| SteerlineTest verify / environment.md / TTY测试 | 10821 / 4962 / 5660 | 11748 / 6242 / 14115 |
| 308 verify / environment.md / TTY测试 | 14482 / 6453 / 5599 | 15526 / 7733 / 14115 |
| prompts_test.go（仅两项已批准SHA） | 5492 | 5492 |
| environmentadaptation/service_test.go | 26252 | 28004 |

execution SHA256 `38d86dbf7fc486ff4c487aaf70fa1f6dea6173fdff72945a474ae1aa502ecb8b`；generation `91654e118d8650abd00d4ffecc0ab72a416e3ee3fa28c13ffbb4f060c4b63c93`。TTY测试原report1夹具已改为当前report2，保留原非TTY拒绝断言；新增真实pwsh子进程、伪工具零调用、负报告及同轮passed保留、非法输入两文件不变。没有靠更新黄金样本替代批准。

## 已验范围与固定版本

本节路径均相对 `C:/SteerlineDebug/wt-5069-tty-failure-report/`，每次新目录。

| 范围 | 原始报告/日志 | 结果 |
| --- | --- | --- |
| SteerLine真实pwsh项目脚本 | `diagnostics/script-owner-034bb0116f2542a18c1b1c05d11636a3/test.log` | 5个方法（含多组非法输入子例）通过；零外部工具调用仅指新增tty-failure场景。原非TTY场景使用隔离Git/伪工具，仍拒绝完整测试和发布 |
| 完整环境owner | `diagnostics/negative-budget-8221337cd7c44e0ea9597d42d5491794/` | 71项通过、零跳过；新增三轮TTY负报告各原Run停止/结果closed，最终failed/not_ready/settled，无人工取消或第四轮 |
| 固定提示词/远端输入/正式报告验证器 | `diagnostics/contracts-b46a600260d94d51907b333a8dc0e6ff/` | 156项通过、零跳过 |
| 308项目真实pwsh报告入口 | `diagnostics/308-script-owner-1e8bba2079cd45068ace12da564b581c/test.log` | 5方法全通过；13个原要求ID均保留，未启动Unity |

SteerlineTest独立clone `C:/SteerlineDebug/wt-5069-environment-publication-scope/sources/SteerlineTest`：脚本5方法全通过，日志 `.steerrunner/artifacts/tty-report-owner-295c53d1af3c44ba995743160ff83b0b/test.log`。固定材料提交 `b012d0ae4ade12a41dc47cf8571b45a42fc6a968` 的原完整L1五项/L2一项均通过、零跳过；原报告 `.steerrunner/artifacts/tty-failure-b53f8f487d8c40e48da3d63d0afb8efb-l1`、`-l2`。

308独立clone同sources下308，固定材料提交 `1c25de58c87b99f44f9fc43f61de09444ba4391d`；仅报告入口/AST/diff验证，未在本机运行Unity/L1/L2。两个项目均未自动合并，未改用户环境配置。

## 固定回归与项目PR

固定干净源码 `d92784f5e52c8c90eb34eda718a5bef69007d023` 的L1 `test-results/L1/fixed-a5d56372100e43f38973479eb5ebac01/` 通过；完整L2 `test-results/L2/fixed-406b5fcfc8504f50a7167d2a45e6c3d3/` 和readiness L3 `test-results/L3/readiness-b6a02517dd7e4f86b87aa1d36db88836/` 均通过且清理完整。独立审阅者已核对相同干净提交、managed摘要及SHA，未发现新增代码问题。对应Go测试的字节/SHA补充账本为 `diagnostics/owner-test-bytes.json`；不是新增正文批准的替代品。

新项目材料为[SteerlineTest PR703](https://github.com/erranliu/SteerlineTest/pull/703)和[308 PR442](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/442)，固定提交如上，未自动合并。应用附件工具不接受HTTP Gitea URL，普通链接保留；这不影响原PR已成功创建。主仓沿[PR413](https://github.com/erranliu/SteerLine/pull/413)维护，原生验收不足时保持草稿。

## TFR-A04早期真实远端失败记录：受理回执阻断

材料发布前曾出现Orca断连及Gitea超时/EOF，原只读前置 `diagnostics/live-preflight-6eeac7823c5c456ea9eddfd8774ec3e1/` 保存Orca失联/Gitea已恢复的原回执。用户明确恢复连接后，核实同一environment `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`、runtime `64bdec68-d3ed-4216-a894-0cc143ed4653`、原308注册仓库及Gitea1.27.1。固定输入 `diagnostics/live-inputs-1ce210223a114596a6d69badfe6060b7/manifest.json`：SteerLine d92784f5、308 1c25de58c8、原full_access/gpt-6-luna high/development2 profile、独立Runner SHA256 `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`、1200秒轮次预算，没有更改Runner或目标配置。

实际正式命令：`go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-orca -out C:/SteerlineDebug/wt-5069-tty-failure-report/test-results/L4/remote-negative-472afe0c10bf400ea1f3c042ef9e1b96`，通过manifest显式设置原五项环境变量。原summary passed=false，场景cleanup_complete=false、scratch保留，受管记录不改写。准备前门禁拒绝、固定夹具 `9e7e39c86ce28b7b124f13c604140f4b85a4e9e2` 及原注册根工作区未变已核实。

原活动 `98a1a8ba871248c68d35d306c8a20a48`、Run `48cfdf179ecba234f61ba1b71e40aa67` 在输入阶段持续operation_outcome_unknown：`Original input has no verified acceptance receipt; no input was resent`。原公开inspect为turn_started=false、turn_id为空、无final，can_reply/can_resume均false，仅can_stop=true。精确绑定的原终端仍停在初始化READY，输入区域存在业务草稿，未观察到已启动的检测任务。原回执为 `diagnostics/original-run-10d6fe66e87041cba5dde0014dab0ec1.json`、`original-run-be1819689604401589a5b7acd26aea32.json` 及 `original-terminal-b64157800a5d46f991a832e787b70aa1/{list,read}.json`；不从尚无业务输出推断原写入必然零受理，不重发、直接发送或恢复未知输入。

这是TTY检测前的输入受理链阻断，没有本轮TTY失败或新tty-failure入口已执行的证据，不能据此判定报告入口成功或目标工具缺失。控制仅允许停止后，经原environment_stop请求 `e48fd442e1881bbd56822170228372b0` applied，最终cancelled、仅一轮、原Run stopped；result-file/v1 closed，SHA256 `7da4d4a3f4e5ac2845d244ebf703f91f5a761304d1789996891cb5af59e22bfe`，内容仍初始report2 completed=false、checks/l2为空。原停止回执在 `diagnostics/original-input-unknown-stop-b6fa6308065c4d51a84a8a6c66e9e603/`，观察器随后project_unavailable是原应用已被测试器关闭，不是停止命令失败。

原场景 `308-runner-cleanup.json` confirmed=true；`diagnostics/original-ref-cleanup-bf2aaee65c154b6f8ddda920c20db20c/` 独立核实两个精确Gitea base无PR、原生分支未发布，按原夹具SHA租约原子删除source/validation两条自有ref，未改原失败summary或scratch。Runner结算仍仅确认原Orca终端，明确未验证或撤销detached进程/执行主机凭据，不能扩大保证。

该次TFR-A01–A03已通过；TFR-A04尝试失败（检测输入尚未核实受理，新报告入口当时未验）。未通过换路径、代写result或伪造TTY故障补证，未修改/重建Runner或部署安装。原失败保持；随后新独立现场已完成三轮负报告，见下一节。

最终独立审阅者已核对原Run/input_digest/attempt/generation/session、terminal runtime/handle/incarnation、原初始result逐字节及摘要、正式summary与managed绑定、停止回执及补充清理，批准事实和未验边界；原生失败不改写。619份Markdown/3915条本地链接零错误，测试清单/场景一致性两项通过，diff检查通过。收尾仅补验收文档，已验实现不再变化。

## TFR-A04真实三轮负向闭环通过

新证据根 `C:/SteerlineDebug/wt-5069-native-input-repro/`。完整汇总 `diagnostics/reproduction-summary-20261030.json`；正式原报告 `test-results/L4/formal-01-b6504501b54b41dcb6504af0a4faa041/summary.json`，场景 `scenarios/testing.project-environment-orca/`。SteerLine固定干净提交86169880，308材料固定1c25de58c8，夹具 `3978ffb965ff441aa107064413c8d2e3c60a0b3b`。沿用原独立Runner EXE SHA256 `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`、原full_access/gpt-6-luna high/development2 profile、1200秒轮次预算及同一远端runtime `64bdec68-d3ed-4216-a894-0cc143ed4653`，未换传输路径或修改目标配置。

实际命令：`go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-orca -out C:/SteerlineDebug/wt-5069-native-input-repro/test-results/L4/formal-01-b6504501b54b41dcb6504af0a4faa041`。一次正式活动 `84e4aa565d694e0ca2eda630dca3de43` 自动产生三个独立原生Run/终端，不是三次完整L4调用。

| 轮次 / 原Run | 原result SHA256 |
| --- | --- |
| 1 / `2890eb5afdaaf0f76d8cf1918a452324` | `79dfe060c17069956abab95bd21675e5c862a71fad4cb137c2c3bd8d5e16e2e5` |
| 2 / `516f94a9efa2e0941b37361b8d26f04e` | `b9dcc26162d82e5107d7e756036177097fa034d80838d69285d481ab4330be96` |
| 3 / `29f9878e8f7888b0cbf24a26faac758f` | `3eb6189b7dc7eac5cc43bafc888ca3c2a14481060960efff52ea80b77acf7356` |

三轮均核实turn_started=true，真实TTY最小入口在进程启动前返回CreateProcessW / OS error -1073283067。Agent遵守固定规则，不换封装重跑检测，仅调用原项目tty-failure入口记录失败；每轮report2 completed=true、terminal.tty=failed，其余12项检查及3项L2均incomplete，PR为空、outcome_unknown=false、cleanup_completed=true。原Run均stopped、result-file/v1均closed、round settled=true、timed_out=false。汇总逐份复算原base64正文SHA及关联身份，与typed report共同字段一致；第二、三轮raw额外保留previous，不将其误判为正文冲突。

`environment-proof.json` 记录三轮预算正常终结，最终failed/not_ready/settled，没有人工取消或第四轮。`provider-pr-proof.json` 独立核实无PR；`admission-proof.json` 核实准备前后均environment_not_ready拒绝、零准入Run。`308-runner-cleanup.json` confirmed=true、`remote-ref-cleanup.json` confirmed=true，场景cleanup_complete=true且无清理错误。清理保证仍受Runner原终端结算边界约束，不扩展为远端主机全部detached进程或凭据撤销。

正式summary/managed/scenario仍passed=false，原命令exit1，因为目标环境未ready；不改写为正向L4通过。本轮已满足Spec/Plan允许的TFR-A04负报告闭环通过条件，用户据此签收远端检测流程。原输入受理未知持续阻断本轮0/3复现，但不证明历史问题已修复；离线快照比较不是原生重复。按用户决定停止追查该Runner问题，不将其列为两条已验流程的待办。本机Orca、远端正向完整L2/PR及未来Runner修复集成仍不继承本次通过。

本次签收及新原生证据经 `/root/acceptance_closure_review` 独立只读复核通过，核对本机固定正向报告、远端managed/summary、三轮raw正文及SHA、停止、无PR、门禁和清理，无确定事实问题。仅修改四份验收记录，619份Markdown/3921条本地链接零错误，测试清单/场景一致性两项通过，git diff --check通过；未修改实现、固定提示词、项目材料或测试定义。
