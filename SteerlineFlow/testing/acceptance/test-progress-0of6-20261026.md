# Task18 第二轮测试进度 0/6 修复

用户要求修复后连续两次真实复验通过再告知。范围是原登记测试、进度写入/采集/页面、真实发布和资源清理；完整 MagicPotion 专项、人工业务验收及 clean 发布 head 测试资格不由此推导。

截至 2026-10-27 03:00：`qualified-01` 完整通过并发布 PR427；`qualified-02` 的完整 EditMode 失败，未发布 PR，已停止并结算资源，连续通过数归零。进度正确保留 6/7，尚未完成连续两次复验。

## 原始故障与边界

原候选 `1af016d628b355e048bb101b47012557`、Runner Run `596b316d8e877daed36279206054f05a` 的原六项执行报告已独立通过，但页面为 0/6，公开接口诊断“进度字段与本次固定测试安排不一致”。公开 Runner `progress-file` 取回的 closed 原始字节为六个 false 测试字段及一个 publication 对象，版本 2、563 字节、SHA256 `d55158423d8617396f6a753ee034e0dead0e237049d611a11779973b4db9d9d6`。原项目固定 base 没有共用进度写入器，原提示却仅要求使用该工具，缺失时保留未完成；后续非测试步骤也未收到该文件的业务边界。严格解析拒绝混合对象符合契约，不能靠忽略字段或模型 final 填回通过。

本次只通过公开服务短暂查询已停止的原 Run，没有发送 task submit/reply/resume，没有改原工作区文件或报告。第一次诊断脚本的 PowerShell JSON 解析及缺少 stop envelope 失败保留；独立后续驱动完成原字节核验，并按实际 instance 单次 public service stop，确认 not_running / offline resources_settled=true。原业务 Run 始终 stopped。证据根：`C:/SteerlineDebug/wt-507e-progress-0of6/original-progress-public-02/`；原截图及报告仍位于 `C:/SteerlineDebug/wt-507e-task18-retest/round-07/`。

## 修复与验证安排

- 调用方实施/审计提示包含本仓正式 PowerShell 写入器源码，缺少工具的项目先接入原登记入口；只由原非零选择、报告、输入和清理验证决定完成。
- 发布及报告提示保留原发布/回复要求，同时明确测试文件只读。冻结字段、初始化、严格解析、Run 身份和版本守卫不变。
- 定向回归执行内联源码，覆盖项目无工具、重跑 false、其它字段保留和 publication 拒绝，并验证实施/发布/报告权限隔离及原用户提示保留。
- 两次真实远端进度复验和清理尚待完成；当前连续通过 0，不把原 Runner 链路两次通过或已有报告计入本次。

首轮完整 affected-owner 回归在 `validation-01/owners.log` 发现两个主干已有断言失败：HTTP consumer 边界尚未允许纯值配置/公开协议包，以及 launcher 列表仍遗漏已实现的 restart.bat。用主干基线 `0ab5e3a59077776fd2969a7941590b1467cf94ba` 的独立导出源码定向复现相同两项失败，证据 `baseline-two-failures.log/json`。维护为精确允许两个不持有执行资源的共享值包、继续检查它们的完整依赖闭包；launcher 检查增加 restart 文件及其真实命令/CRLF。没有放宽任何测试进度、Unity、Run 身份或真实复验门槛；原失败保留，后续验证使用新目录。

## 源码回归及真实复验代际

主干修复构建源 `ab8611c1b8b4b7dd58f0c376c34aa32409a044a7` 的受影响 owner、应用定向失败项及进度回归通过；固定 L1 complete/passed、L2 passed/cleanup_complete 均为 true，证据 `validation-02/`。受控 `testing.runtime-progress` L3 passed/cleanup_complete 通过，证据 `validation-04-progress-controlled/reports/`。前一次 `validation-03-progress-controlled` 被 PowerShell 拆分未加引号的 scenario 参数，在测试选择前失败；修正精确参数数组后另目录执行，没有修改测试或补造报告。

原 Task18/f3 代际与当前 main 的新项目环境适配准入不同。主干包新 `round-01` 在业务接纳前停于 `Runner result-file capability unavailable`，唯一 start request `8419176246953318637e8d4a72eaefcd` 最终 expired/rejected，公开页面未建立 Task18 Run；该 home 已按正式入口停止，offline resources_settled=true，保留 `round-01/pre-admission-cleanup/`。未改新 main 的环境门禁、原项目合同、配置或旧 home/pin，不把这次前置失败记作通过。

为复验用户指定的原现场，另用普通独立源码 clone，从原 Caller `57623186b63c16be2a5748665e63d52ae615adbf` 只追加同一修复及断言维护，干净受测源 `42c16465`。主干修复 `24daeeb8` 与原 Caller 修复 `440110fb` 的 stable patch ID 均为 `425e3082cff59b8e6dd39389e2d6576c4fa3d659`，原固定 Runner f3、模型、原合同及已批准 Unity 绑定保持。`original-caller-fix-equivalence.json` 保存来源证明；`validation-05-original-caller/` 及后续 `live-01/`、`live-02/` 单独登记。该原代际复验不证明 main 新环境适配门禁已通过，也不提供产品运行时回退路径。

关联：[Spec](../../spec/runtime-test-progress.spec.md)、[Plan](../../plans/runtime-test-progress.plan.md)、[接入说明](../../../docs/integration/runtime-progress.md)。

## 原代际 live-01 与后续候选 Runner

`live-01` 的原 Run `3808918f4de62554116d03ce354545d4` / Attempt `3e8a3e01bdba5f013a348fe3cabd09c7` 已独立核实原 L1 的 11 个 XML 成员、原 L2 两个各一成员的 XML batch、四个结构检查 exit0，以及原 plan、输入、case receipt、清理和实际 Unity 版本；没有重跑 Unity 冒充原报告。原登记入口已接入正式 writer：开始或重跑置 false，完整 managed 报告和原结果/输入/清理通过才置 true。远端 writer 仅多出空行，其余源码逐行等同正式 writer；父调试进程未写进度。

公开原始进度 version11 为恰好六个声明的 true 布尔字段，无 publication 对象；194 字节，SHA256 `71585b019da15301909af3407ce5d3bee8651b9c8612fec1beb19d4adfd4e7a7`。API 和独立目检截图均显示 6/6。前序 Incomplete / 结构失败报告保留。L1 与 L2 的 dirty codeIdentity 分别为 `afe:c2ad5a...` 和 `afe:c2e04f...`，不能宣称同一最终源码或 clean 发布 head 已通过。

原 f3 Runner 随后停在缺少原输入归属证明：原生 main 已 done、空 prompt、完整同根 hook 元数据，但漏掉 working 时现有独立原输入查询仅接受 thin 元数据。没有真实 PR，故本轮完整复验不通过，连续数仍为 0。原 Run/宿主均按公开控制停止，offline resources_settled=true，完整精确 owned native census0、truncated=false、omittedHosts 为空，本地 home/bin 进程0；原工作区/报告保留。证据为 scope 下 `live-01/checks-verified.json`、`verification-20261026T0915-inputs/`、`diagnostics/report-transfer-recovery-2026-10-26T09-10-56-017Z/` 和 `cleanup-blocked-20261026T0920/`。

独立 Runner 修复见 [SteerRunner PR62](https://github.com/erranliu/SteerRunner/pull/62)：复用原 terminal 围栏和唯一完整原输入证明，在稳定完整元数据下补证，不新增状态/协议/重放，不虚构 working。主线受测源码 `8eda1434` 正式 L1 702 顶层+29扩展通过；原代候选 `8e1ce9f9` 正式 L1 675顶层+29扩展通过，两个代际 owner 均通过，同形测试在旧 f3 失败。stable patch-id 均为 `6c2ff4585680000f87e68df9fef2686c980331b4`。

用户已明确允许未来新轮使用候选 Runner，原代候选 EXE SHA256 为 `30b3bc832a123427b9494675b242351df975e99994e5688121ef185b4055355c`。另存授权 `runner-pin-authorization-20261026T0930.json`，用原干净 Caller42c16465 及该 Runner 重新通过正式生成入口建立独立包 `candidate-caller-packages/dev-20261026-173725-42c16465`；未来 `candidate-01/02` 采用新普通 clone/home 和独立固定，原 home/pin 不变。原模型、环境 profile、合同、base、L1/L2/四结构及 extra 检查要求保持。仍需两次真实发布前后/closed 采集、原报告、UI 与资源清理完整合格；原 6/6 或 Runner-only 历史成功不计入。


## candidate-01：八项进度与原生输入换行

用户批准的 `8e1ce9f` / `30b3bc...` Runner 新轮原 Run `0f5ba57d9803ed6eefd6fc4112bf716f`、Attempt `23dbf530f1b807550c948f410033b430` 实际绑定八项：L1、L2、Bingo、Group2099 及四项结构检查。全部逐字段匹配原 registry，未删除额外项。原 L1 XML 11 成员、L2 两个各一成员 batch、两项 L4 各一成员及四结构 exit0 均独立核验；canonical Test-ArchitectureResults 只读复核原四份 parent plan、五个 batch 的 XML、输入与清理通过，没有重跑 Unity。前序失败报告保留。

原登记入口接入正式 writer，公开 version33 为恰好八个声明的 true 布尔字段，无 publication 元数据；302 字节，SHA256 `0a1b323fd2072babff36eb71e998194fa8162cd564be764ed9c034c00287a5fe`。API 与独立目检截图均为 8/8。这是发布前进度成功，不能代替真实发布或完整复验。

实施原生完成后没有真实 PR。只读诊断发现 Windows CRLF paste 将每个 CR 变成额外 LF：9498 字节原逻辑提示含 57 个 CRLF，9497 字节 native transcript 与该 paste 转换逐字节相等；现有严格匹配期望 CRLF→单 LF 的 9440 字节。因此该轮完整链路失败、连续数仍为 0。Runner 追加修复在唯一发送前编译原 CRLF，不放宽历史正文匹配；沿同一 PR62 提交，完整新构建资格另行登记。

原 worker 还曾误判未安装 SDK，下载官方安装脚本并在自身 TEMP 安装 SDK8.0.427，使用子进程 PATH；独立只读查证机器原有 SDK8.0.426 自 10 月 23 日已存在。父调试进程没有安装 SDK，没有追认环境不变或用该轮计通过。后续仅提出现有 SDK8.0.426 的子进程 PATH 绑定，不改全局环境、原 profile/home/pin 或失败报告；新 Runner 字节及该独立环境例外须完成资格后取得未来新轮明确批准，尚未启用。

原 Run/宿主已停止，public offline resources_settled=true，完整精确 owned native census0 / truncated=false / omittedHosts 为空，本地 home/bin 进程0。没有重发 submit/reply/resume，没有改写进度或私有 Runner 数据库。证据在 scope 下 `candidate-01/checks-verified.json`、`verification-20261026T1150-reports/`、`verification-20261026T1155-inputs/`、`diagnostics/native-attribution-20261026T1125/`、`diagnostics/sdk-install-variance-2026-10-26T10-57-53-639Z/` 及 `cleanup-blocked-20261026T1200/result.json`。原 dirty 代码身份差异、clean 发布 head / 完整 MagicPotion / 人工业务未验边界保留。

## 新 Runner / 现有 SDK 授权与测试安排遗漏

用户后续明确允许新 Runner 和现有 SDK8.0.426 子进程 PATH 绑定，授权独立保存为 `wire-sdk-authorization-20261026T1400.json`；未修改旧授权、原 home/pin 或失败报告。Runner PR62 已由用户合并，独立 API 确认 merge commit `9acd5649187a1a4035f1d901479b84c1fb0b49e4`。真实复验继续固定已批准的原代干净构建源 `7c0df6cafb050f294e7066dfdeb896424fe78230`，EXE SHA256 `00051e332f19805db1f814069c98da48be7411b8b2799955283bc3ee81edca0d`，不把 merge commit 冒充原构建来源。

新增普通 clone/home 的 `wire-01` 通过公开 presentation 固定 SDK 方案 digest `38f094bff5f739c9be7705bb785fb17accd978e203e5c54e05db5a771565f09b`，原 profiles/default/Unity 方案全部保留。但是系统只生成四结构，外部严格必跑门禁在第一次 Task18 start 前拒绝。通过正式维护入口取消原生成并核实 stopped 后，只重生成一次；该原始结果仍缺 L1/L2，没有继续反复抽样、手工补 ID 或启动不完整轮次。这个 home 已正常停止，offline resources_settled=true，没有 Task18 业务候选、业务 submit 或原生 worker。原拒绝、生成、回执与停机证据均保留于 `wire-01/`，不计完整通过。

生成提示原来强调“最小必要入口”，没有明确项目规范中的固定必跑优先于 Plan 仅列出的定向项。本次在同一 Caller 修复中明确先保留规范必跑，再追加 Plan 定向项；不在通用产品硬编码目标项目 ID，不修改原测试清单、Plan、业务合同或严格复验门禁。只有 Plan 和规范均允许例外时才返回空安排。提示修正不等于模型已正确选择，后续新现场仍须核实公开原安排包含 L1/L2、四结构及全部 extra。

主线干净受测源 `384e12cb84658eb7e00d169d7c8a14d10719472a` 与独立原代干净源 `8205bf0e725c8bdf7c2a1cafae3685de567da152` 的本次生成提示 product patch-id 均为 `84c1c71088a4dd8257876132803fb0c847695103`。两个代际 testmanagement/runnerclient owner、固定 L1 和 L2 已通过，L2 cleanup_complete=true；原代正式调试包生成另以 `validation-required-original-01/result.json` 为准。原 Caller42 源码和旧包保留。完整真实进度复验仍为0/2，不将材料生成、组件通过或历史8/8算作发布通过。后续使用新的普通 clone/home，任何已初始化失败现场均不换包或改 pin。

## qualified-01：首轮完整通过

2026-10-27 首轮使用原代 Caller `8205bf0e`、已批准 Runner `7c0df6ca` / SHA256 `00051e...` 和既有 SDK8.0.426 子进程 PATH 方案，独立准入记录为 `qualified-caller-wire-sdk-admission-20261026T1430.json`。原 profiles 及 Unity 方案保留。公开 SDK 方案 digest 为 `38f094bff5f739c9be7705bb785fb17accd978e203e5c54e05db5a771565f09b`；每个新 home 在唯一业务提交前分别冻结实际配置和首次接纳的 Run/Attempt。

首轮实际选择九项：L1、L2、Bingo UI 打开、Bingo 奖励/购买/重启/重复确认、Group2099 共存和四结构。原 Run `2a5ca45b65aa402d5ded6ab5c95f2f35` / Attempt `6ff6c14964fc2f349a388bc78249f526` 的原 worker 在发布前重跑最终所选 Unity 入口；五份 parent plan、六个 XML batch 均独立核验，L1 11 成员、L2 两个各一成员 batch、三个活动入口各一成员，无失败或跳过，原输入及 cleanup 通过。四结构原报告 exit0。父调试进程仅对原产物运行 canonical 只读核验，没有重跑 Unity。

真实 [PR427](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/427) 的仓库、base `AIRefactor`、head `1be62312c58b4f88bb0cf0ab74d01247b330d813`、远端 HEAD 及完整 51 个变更文件均独立核验，保持 open/unmerged。正式 writer 是第 51 个文件，SHA256 `d39d89f2f3353116c7f507da998f518a27564aa418082faba9a20b68755bfcb4`；原入口按原结果驱动字段。公开 native session 完整读取只有 READY、原实施和原发布输入，没有额外业务输入、手填运行时标志或 SDK 安装事件。

发布后及原 Run 停止后的 API/独立目检页面均为 9/9。closed version26、339 字节、SHA256 `2ff4c8be73c5368d0cee44b91af2492a6a241d146c9fe630236bd344981f2707`，恰好九个 true 布尔字段，没有 publication 元数据。随后原宿主正常停止，offline resources_settled=true，完整精确 owned native census0 / truncated=false / omittedHosts 为空，本地 home/bin 进程0。记录 `qualified-01/chain-result.json`，至此连续通过 1/2，没有提前通知完成。

原最终测试记录的 dirty codeIdentity 均为 `afe:ff8ed63b91f0862b6fc5bbdb0673440b5c914e4478863bb4556d29f1676844cd`，另一次工作区身份重建为 `afe:2a93b43bb95c1bfbdcad8347bdc8a2a667e3fdb13ef83c2e7d2b1731ded3ec57`，差异原因未独立确定。它们不证明 clean 发布 head、完整 MagicPotion 或人工业务验收。原终端及宿主资源结算通过，也不等于任意脱离终端的后台进程或执行机凭据已撤销。

证据位于 `qualified-01/verification-20261027T0025-reports/`、`verification-20261027T0045-inputs-03/`、`diagnostics/native-public-session-20261027T0040/`、两次截图 `observations/2026-10-26T16-21-55-336Z/` 和 `16-40-09-092Z/`、`cleanup-20261027T0050/result.json`。原始传输失败和 PR 文件分页不足均保留；后续完整传输有 SHA 核验，未以截断内容补造业务通过。

## qualified-02：完整 EditMode 失败，连续数归零

第二轮仍使用同一已批准 Caller/Runner/SDK 代际，新 home 实际冻结七项：L1、L2、完整 `editmode.run` 和四结构。没有删除额外 EditMode、缩小 Filter/Category 或降低通过要求。原 Run `0752a85a0e8a67197c4466ab077d4296` / Attempt `6964e6330b1118e2d900995af5290ec5` 只有唯一 accepted submit `e9b9edb41ffe082745ad644163ac7a33` / command `157b38afb532c0c677a713b264c90910`。

原七份最终 managed 报告及其关联 summary 的字节、SHA 和 frozen definition 已独立核实：L1、L2、四结构报告为 passed，EditMode 为 failed。完整 EditMode 原入口退出码 2，Filter 和 Category 均为空；原 XML `Test-Log/editmode/1cf2b749ceb149bba36da8b94c2885e4/results.xml` 有 3578 个实际成员：3534 通过、11 失败、33 跳过，SHA256 `2021fb0bc7db19ea364d68d5a9ebfd6b1d05b7e580d5aebe0bed3b35b2e5af64`。失败包括 MagicPotion BP 消费断言、schedule inventory 清单差异、资源夹具目录、GamePreferences 和 StartupConfig；跳过项包括需 Source 资源或单独新 Editor 执行的 Explicit/Ignore 用例。未将这些失败统一归因为已证实的基线故障。

公开 version29、220 字节、SHA256 `f05d6be153a65da9090ce39a8f7bdda514b341b78eb618e7aab34fe16cbb46c7` 恰好七个布尔字段：六项 true、EditMode false，无 publication 元数据，API 和独立目检页面一致为 6/7。这次未完成与真实报告一致，不能手填第七项或当成原 0/6 复发。原实施 final 已取得验证；进入发布后原 start_turn 因 `Remote Orca runtime closed the connection` 被拒绝，未创建 PR，也未 resume/reply/重放。

该轮完整复验失败，连续通过数从 1 归零；尚未达到用户要求的连续两次通过。保留原 worktree、所有失败和跳过报告，不启动第三轮原样补测。负向原始核验为 `qualified-02/blocked-checks-verified.json`，原报告及 XML 属性/失败成员只读采集在 `verification-20261027T0305-blocked-reports-02/remote-report-evidence.json`；原完整 XML 留在原工作区。第一次全 XML 传输被远端连接中断，保留 `verification-20261027T0255-blocked-reports/` 和 ptyKilled 证明；第二次只读取得原 SHA、完整统计及所有失败/跳过成员，没有重跑测试。

唯一原 Run stop 已接纳并核实 stopped / new_inputs_allowed=false。首个停止驱动在进度仍为 observing 的短暂窗口要求立即 closed 而中止；原失败保留，后续仅延续只读确认，不重发 stop。进度随后 closed 且版本/字节/SHA 不变，API 和独立目检仍为 6/7；宿主正常停止，offline resources_settled=true，完整精确 owned native census0 / truncated=false / omittedHosts 为空，本地 home/bin 进程0。证据为 `cleanup-blocked-20261027T0305/`、`cleanup-completion-20261027T0310/result.json`、closed 截图 `observations/2026-10-26T19-00-19-730Z/task18-progress.png` 和 `chain-result.json`。没有活动 Task18 Run，没有热换包、恢复原轮或新增业务轮。

后续阻塞是原业务项目完整 EditMode 的断言、夹具和显式独立执行准备，另有发布阶段的远端连接拒绝。现有固定测试/不删项/不缩小范围约束下不能将这些改成通过，也不能用重复抽样选择另一套测试绕过。本次先保留明确负向证据及完整清理，是否扩展到这些业务测试问题另待用户明确方向；本 Caller 修复保持 draft，未部署或合并。

## 2026-10-27 后续交付与 PR399 合入审计

用户已批准业务修复、确认两项废弃 Stage33 检查无效，并合并 PR428；上述“待用户方向”为原 qualified-02 停止时的历史状态。按用户随后恢复原进度复验的要求，已建立全新 post428-01 并接纳10项原安排，连续数仍未达2/2。原 home、输入、失败及清理事实不变。

PR399 已解决与当前 main 的唯一边界测试冲突，保留主线更严格限制。新的组件、固定 L1/L2 和受控进度验证及未验边界见[合入审计](pr399-merge-review-20261027.md)；原代际 Caller 的真实链不作为当前 main 新环境代际的证明，PR 保持草稿。
