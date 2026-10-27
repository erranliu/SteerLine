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

## 2026-10-27 最新职责划分与暂停现场

用户明确将 Orca → Codex 的输入投递、提交及启动确认交由 SteerRunner 对话，等待 [PR66](https://github.com/erranliu/SteerRunner/pull/66) 完成独立验收并合入。此前本地修复、失败探针、构建和清理记录全部保留；最小探针通过不构成 SteerLine 原生联验通过，也不计入原约定连续两轮。本会话不再重复这部分实现，不在 SteerLine 增加粘贴识别、终端发送或启动确认逻辑。

调整分工时，最新 `segmentfixed-01` 的原 Caller start 请求已接纳，故仅收尾这个新现场，没有重启或重复停止历史现场。身份及固定产物如下：

| 项目 | 原始值 |
| --- | --- |
| Candidate / Caller request | `3e2d6eeb87f074853cdaa1a1b2b744f5` |
| Runner Run / Attempt | `2a550e2c3ee3ab3501506d3f3757d558` / `00004d24fadf25e8725b248e06fcdf89` |
| Runner instance / state-dir | `f53b9d51aad5967c60ecc030ffc03aad` / `C:/slr/egsih62doo6g36sus5gefloed4` |
| Caller source / package | `8205bf0e725c8bdf7c2a1cafae3685de567da152` / `segmentfix-caller-packages/dev-20261027-153401-8205bf0e` |
| 原固定 Runner 构建源 / SHA256 | `dacd91518473bacc1f8e41fbd2627c6305d44740` / `96800b5fca6a8580aab4b560a15d8a9d294183e6d23016355547309c76d0290c` |
| 实际业务 base | `485d88bad9081d3fd8d892116569f763038f7b7e`；原合同 digest 仍为 `51da15701f56ee129ebf21624ac258764ded1d69437bc6cd32b9e3008fdc5724` |
| SDK profile / config SHA256 | `38f094bff5f739c9be7705bb785fb17accd978e203e5c54e05db5a771565f09b` / `8ad8380235d0717510e5ae1b8651f33d6ba4a98e2cc79c0246b2654a1ff2c6b9` |

原安排实际为10项：L1、L2、完整 EditMode、Bingo 奖励/购买/重启/重复确认、Group2099、checkpoint 重启恢复及四结构。全部保留，未过滤完整 EditMode 或其它 extra。暂停前没有已核实的原生启动、实施完成或发布结果；不能据此宣称从未执行任何原生内容，也不把行政暂停诊断成业务测试失败。

按原 Run 公开 stop/CAS 停止，核实 stopped / new_inputs_allowed=false 后才正常停止宿主。closed version1 为388字节，SHA256 `14f4a07d820263e82fd5e68c7094fd39db544c982e21cb7600c13ff88321e9ad`，恰好10个原声明布尔字段且均为 false，没有 publication 元数据。API 和独立目检的停止后截图一致为0/10，没有真实 PR。本轮不计通过，连续数保持0。

原宿主 clean，公开 offline resources_settled=true，精确 owned native census0 / truncated=false / omittedHosts为空，本地 home/bin 进程0。只读 CIM 复核本会话根目录可执行进程为空，已登记验证、探针、打包、clone 和 startup owner 均已结束；startup owner 自然结束，没有再次停止。原终端停止不代表任意脱离终端的后台进程或执行机凭据已经撤销。

证据根为 `C:/SteerlineDebug/wt-507e-progress-0of6/`：`segmentfixed-01/profile-acceptance.json`、`material-admission.json`、`pause-startup-owner-20261027.json`、`pause-cleanup-20261027T1545/result.json`、closed 截图 `observations/2026-10-27T07-43-15-823Z/task18-progress.png` 及根目录 `paused-owned-resource-census-20261027T1545.json`。原构建、正式包、旧接受 pin、失败和停止记录均不覆盖；当前无活动业务复验，定时任务已取消。

后续仅在用户告知 PR66 合入后核对实际合入提交、Runner 自身验收结果及对应构建，再用正式入口生成全新独立调试包，不热替换旧运行。恢复时仍须完整验证原始报告、全部实际安排检查、进度/API/UI、真实发布和资源结算，连续两轮目标不变。PR399 的独立代码审计及受控验证已经完成，当前仅完成文档/资源收尾后进入等待；不创建子代理，不向其它 session 发消息，不合并或部署。

## PR66 合入后：完整首轮与第二轮材料准入失败

用户告知 PR66 已合入后，实际 merge、受测产品及构建、PR399 rebase 与正式新包见[合入审计](pr399-merge-review-20261027.md)。本轮 Runner 产品源 `ffb9c510280d470e064e48770b8eea30174d82c4` / EXE SHA256 `a745ab738152e974850cbaca7851fb13c0d33cd08a915cda555f1ae465556347`，Caller8205，实际业务 base485d；旧现场均未恢复或热换。

`merged66-01` 实际只安排 L1、L2 与四项结构检查，六项 definition 包括 note 与原 registry 完全相同。原 Run `cc0c0701fb3b978ab568f982c787ae89` / Attempt `47f10062ba94ffbe132d9724ba9131e4`，唯一 submit `bc120ef4be8128c9393b4e83cd0fe20a` / command `9bd405cb13dea2d2a59638ede9d86cd5` 已由公开事件及同 command inspect 核实。原生完整公开会话为初始化、原实施、原发布三条输入，原实施及发布 final 均核实，没有 parent 补发、补 Enter、手填进度或 SDK 安装。

最终六份原 managed/summary 字节和 SHA、四结构 exit0、L1/L2 两个原父计划及三个 XML 批次均通过独立核验。XML 实际为11、1、1个 Passed，无 skipped；两个 PlayFlow receipt 的业务覆盖、输入核验和清理均 Passed、externalRequests0。只读调用原 canonical validator，未由 parent 重跑 Unity。两份原计划 codeIdentity 均为 `485d88bad9081d3fd8d892116569f763038f7b7e:056d1daf8ce255d52cb7e0982b7d62fdfbdfbc1690c8815a3b0d0a55ac2cd431`，发布后的按 execution base 重建身份一致；仍保留原报告 before/after dirty，不升级为 clean 发布 head 重跑或完整业务/人工验收。

真实 [PR433](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/433) open/unmerged，base AIRefactor，head `f552c0ddfc23cc4f94dad356b2568e6091988095`；独立 Gitea API 五个变更文件及实际 remote HEAD 一致。RuntimeProgress.ps1 已在 base 中，发布 head 上仍存在且与正式源码等价，未因它不在五个变更中而判缺失。首次采集 PowerShell 数组容器计数错误保留，原 JSON 完整五项，后用原 JSON 与实际 head 内容纠正。

原进度 version19、194字节、SHA256 `71585b019da15301909af3407ce5d3bee8651b9c8612fec1beb19d4adfd4e7a7`，恰好六个 true，无 publication 元数据；发布后及原 Run stop 后的 closed API/UI 均独立目检6/6。先核实 stopped / new_inputs_allowed=false，再正常停止宿主；offline resources_settled=true、完整 owned native census0 / truncated=false / omittedHosts为空、本地 home/bin 进程0。证据根 `C:/SteerlineDebug/wt-507e-progress-0of6/merged66-01/`：`checks-verified.json`、`chain-result.json`、report/input-evidence-index、`diagnostics/native-public-session-final-20261027T091028672Z/`、截图 `observations/2026-10-27T09-02-36-321Z/` 与 `09-12-44-466Z/`、`cleanup-merged66-01/result.json`。首轮真实通过不提前宣称2/2。

`merged66-02` 使用同一原代际和 Runner、新 clone/home，自动原安排保留 L1、L2 及三项活动 extra，却遗漏 Plan 明确映射的四项结构检查。门禁在 `start_task` 前拒绝，公开 valid/connected 快照 runs=null、slot_occupied=false；没有 Task18 Run、submit 或 Unity 业务执行。没有删去 extra、手工补结构项、重生成或第三轮原样抽样。保留 `materials-4.json` 与 startup-result，正常停止生成宿主、offline settled、本地 home/bin 进程0，见 `pre-admission-cleanup-02/result.json` 和 `chain-result.json`。首次收尾脚本未处理公开空 runs 的 null 表示，在 stop 前失败；原失败保留，后续脚本先验证有效快照再完成唯一 stop，没有重放旧停止。

生成提示原本先强调规范必跑，再按 Plan/影响选定向项，对 Plan 自身的必跑表约束不足。新增提示明确规范与 Plan 必跑项先取并集、不得以业务回归替代结构/资源契约，最后逐项核对；不硬编码项目 ID、不新增解析器或手工改写安排。当前 PR 干净产品源 `e87f04b8fc843c10c510a3f7db5396d25fa28607` 与新原代际 `3d42de02758f6de2c86c6d4a698a0eea3c22f317` 只含相同提示增量。

两套受影响 owner 和固定 L1 通过；owner 原生 opt-in 分别跳过3项和2项，不算原生资格。当前 PR 固定 L2 通过且 cleanup_complete=true。原代际首次固定 L2 在请求回执原子 rename 出现 AccessDenied，失败且清理通过；同一干净源码的独立完整 L2 随后通过且清理通过，未确定首次文件访问失败根因。原报告为 `validation-plan-required-main-01/`、`validation-plan-required-original-01/`、`validation-plan-required-original-L2-02/`，组合核验 `validation-plan-required-original-qualified.json`，均位于上述任务根。新提示仅完成组件资格，模型是否保留全部必跑项及真实两轮仍待新现场验证，连续计数从0开始；历史成功与失败不覆盖，Runner 不再修改。

## Plan 必跑修复后：八项报告通过、进度6/8的真实负例

`planrequired-01` 使用干净原 Caller `3d42de02758f6de2c86c6d4a698a0eea3c22f317` 的正式新包、同一 PR66 Runner 和真实 base485d。原安排保留 L1/L2、两个 Bingo extra 和四结构，共八项；全部 definition（含 note）与冻结清单一致。唯一 candidate `e922387aa7a461ac065ca935e0598f48`，Run `ec9a82b1523b4ef62b3ce289849285f3` / Attempt `cc837d85c7d8e8611e07d64f95ff9f8f`，sole submit `054ba288ac8c722991b13a4a4f9191e1` / command `bb22dd26b9ecd88682adca230d326c2c`，由公开事件及原 command 查询核实。没有 parent 额外输入、补 Enter、重发或手写进度。

最终八份原 managed/summary 及四结构 exit0 全部核实；四个原父计划、五批 XML（11/1/1/1/1）均 Passed，无 skipped。四个 PlayFlow 的原 input/cleanup 均 Passed、externalRequests0，coverage为7/6/3/2。所有原计划 codeIdentity 为 `485d88bad9081d3fd8d892116569f763038f7b7e:8518a2957d9898b7d0df27d5dca12bc7318e611456ec4d96c56bd62d6376e621`，仍是原 dirty 工作来源，不冒充 clean 发布 head 重跑或完整 MagicPotion/人工验收。早先编译 Incomplete 保留，最终通过来自原 worker 修复后的真实重跑；parent 仅用原 canonical validator 只读核验。

真实 [PR434](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/434) 保持 open/unmerged，head `24c8499f55f2ab3c6962dabde26e52916cac778c`；API 分页20文件与实际 remote HEAD 一致。公开进度 version11、296字节、SHA256 `a383ea22dfae154f239b256373bd466045069b6c74899c986f58f1e31de08991`，精确八个布尔字段，六 true、两个 Bingo false，无 publication。发布后和原 Run stop 后 closed API/UI 均独立目检6/8；因此整链失败、连续数仍为0，不能把报告通过改写成进度通过。

原入口只为 L1/L2 聚合项设置运行时名称，具名 Case 报告虽通过却没有进度更新。原 worker 的公开完整会话为初始化、实施、发布三条输入；它只核对 helper 存在及 dot-source，没有逐 ID 覆盖核对。新增通用提示要求逐 ID 检查真实入口，不以部分兼容代替全部兼容。主 PR 增量 `4a132d0d122dc03945fdba830b484922825f8b86`，对应原代际 `2bca6d6fa53a81ba41afcb3f3fe4dc2fdec911e8`；不硬编码业务 ID、不从旧报告回填。

项目正式入口的确定性缺口另提交 [PR435](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/435)，head `762348072aed3659a8c65f944715e7bec3cba436`，base485d，八个变更文件，无清单/断言/通过标准变化。共享 managed 发布入口在选择时清零，按本次最终已持久报告的状态及完整 scope 更新已声明字段，删除包装器的粗粒度成功覆盖。具名 Case、Unit、生命周期与既有 L1/L2/EditMode/结构入口使用同一结果边界。旧入口在新增回归中失败；修复后现有报告协议42项加新增进度协议15项断言通过，均为独占协议夹具、未执行 Unity。该项目 PR 的审阅合入是选择新真实业务基线的前置，不自动合并，也不在未合入来源上声称两轮完成。

证据根 `C:/SteerlineDebug/wt-507e-progress-0of6/planrequired-01/`：`blocked-checks-verified.json`、`chain-result.json`、两个 evidence-index、`diagnostics/native-public-session-final-20261027T101149235738Z/`、`cleanup-progress-gap/result.json`。原 Run stopped/new_inputs_allowed=false 后才停止宿主；offline resources_settled=true、完整 owned native0/truncated=false/omittedHosts为空、本地 home/bin0。closed截图为 `observations/2026-10-27T10-18-49-517Z/task18-progress.png`，已目检。没有再次停止或启动旧现场。采集自身的参数预算/引用失败、早期报告传输失败及其成功的原始只读相对路径传输均单独保留，不当作业务失败，也不重跑 Unity。

## Windows 回执快照更新修复与组件收尾

逐 ID 提示验证再次出现 `request_receipt_persist_failed`，根因类别可确定性复现：旧 `os.ReadFile` 不共享删除；即使读者共享删除，旧 `os.Rename` 的 Windows MoveFileEx 仍不能在原读者持有期间替换目标。生产读取改用现有私有快照入口；写入使用按句柄的 snapshot-preserving rename，保留私有 ACL、原大小/schema/envelope核对、原有有界 sharing/access 错误预算。持续排他句柄仍失败且保留旧完整记录，不复制/删除回退、不重试业务动作或扩大权限。没有追溯证明每次历史 AccessDenied 的具体持锁进程，不能宣称所有文件访问失败均已根治。

干净受测产品为主 PR `a1583ee61bd2e9fa77cfd82a22dd6147cc054e24`，原代际 `07fcb905eb2310fae40946a19269b186159c3eae`；对应同一四文件回执快照补丁和相同逐 ID 提示。证据：

- `C:/SteerlineDebug/wt-507e-pr399-receipt-snapshot/` 保留原读者/原写者两种失败反例、真实 held reader 更新、原/新完整快照、Unicode目标、ACL及持久排他失败检查。首次仅改读者和缺少 NUL 的写者试验失败均保留；最终四 owner 通过。
- `validation-receipt-main-01/`：七 owner 完成、239 passed/3原生 opt-in skip、无失败；固定 L1/L2 通过，L2 cleanup=true；L3宿主生命周期通过，进度场景 maintenance GET 超时且清理通过。
- `validation-receipt-followup-02/main-progress/`：同一干净主 PR 的隔离进度场景通过，cleanup=true。初始超时原因未确定，不以此覆盖原失败。
- followup原代际 owner 的包装脚本把 `-out` 放在 `--` 后，Go 拒绝参数，未完成测试；失败日志保留。改为正式参数顺序后执行独立新输出，不重复该调用。
- `validation-receipt-original-03/`：原代际七 owner 完成、204 passed/2原生 opt-in skip、无失败；固定 L1/L2、分别隔离的进度和宿主生命周期 L3 全部通过，所有 E2E cleanup=true，最终 source/head仍固定干净。跳过项不是原生资格，受控场景没有模型或 Unity请求。

这些组件通过不计入原业务两轮。当前无活动 Task18 Run、定时任务或子代理；PR399 保持草稿。待 PR435 审阅合入后核实实际目标提交，以新的正式独立包/home/接受 pin 恢复原全部实际安排检查及两轮完整验收；旧报告、构建和运行身份不热换、不重放。主线新环境代际仍单列未验。

本轮最终使用正式 `new-debug-release.ps1 -SteerRunnerExe <PR66参考构建>` 生成两套新的 ready/clean 包，证据根为 `C:/SteerlineDebug/wt-507e-pr399-receipt-packages/`：当前 PR 包 `packages-main/dev-20261027-191003-72dccdbb`（源码72dccdbb，运行时代码与受测a1583ee6相同），原 Caller 包 `packages-original/dev-20261027-191107-07fcb905`。两份 manifest 都固定 Runner产品ffb9c510/版本0.2.0-dev.3-r3/EXE SHA256 a745ab738152e974850cbaca7851fb13c0d33cd08a915cda555f1ae465556347。这里只构建，没有启动包、模型或 Unity，没有热换旧包。后续文档提交不是重测或重构建来源。受控构建进程 census 为0；业务负例的停止/资源结算仍以原 cleanup 记录为准。

## 2026-10-28 PR435 合入后：九项进度通过，发布输入未确认

按用户明确要求正常合并 PR435，实际 merge 为 `3b59842c3343ce92ef900cc66aeffc7ebb26e2e7`，父提交为485d和原 head76234807，merged_at为 `2026-10-28T06:10:08+08:00`。原清单、断言和通过条件不变。两项服务器 CI 在获取 actions/setup-dotnet 时鉴权失败、未进入项目步骤，原日志保留于 `pr435-merge-20261028/`，不声称服务器 CI 通过。用户随后恢复复验，以实际合入提交作为新执行 base；原合同 digest 和 registry 身份保持冻结。

新 `entryfixed-01` 使用原 Caller `07fcb905eb2310fae40946a19269b186159c3eae` 的正式 ready/clean 包 `dev-20261027-191107-07fcb905` 和上述固定 Runner ffb9c510 / a745ab73 构建。首次 clone 因本地已存在 AIRefactor 而退出，失败保留；只在同一普通 clone 正常切换并快进目标分支，没有重复 clone、业务 start 或 submit。实际九项是 L1、L2、四结构、Bingo UI、Bingo 奖励/购买/重启/重复确认、Group2099共存，definition（含 note）全部严格等于原 registry；没有删除 extra 或缩小测试。

唯一 candidate `cbad26c0f90646efb54c515e19da4251`，Run `600d14e87929e5555c74675b56cde86a` / Attempt `219608c2f90445b83698824345efa5fe`，sole submit `fbf69cf0a00a9cc729b65635cc9ac09b` / command `479f1c30a561cdb08bb256bfb1c5c63e`，由公开事件和原 command inspect 核实。原 worker 实施并修复 Group2099 去重后，九份最终原 managed/summary 全部通过；早先失败、Incomplete 和未安排报告不覆盖。五个原 parent、六批 XML 成员数为11/1/1/1/1/1，全部 Passed、无 skipped；五个 PlayFlow receipt 覆盖7/6/3/2/2、输入和 cleanup 均 Passed、externalRequests0。父进程只读调用原 canonical validator，未重跑 Unity。

正式入口及 writer 与执行 base 的原源码一致，171条公开原生工具调用中涉及进度的两条仅为只读检查，没有 parent 手填字段或 worker 手动运行进度标志命令。原计划和最终工作区 identity 一致为 `3b59842c3343ce92ef900cc66aeffc7ebb26e2e7:31630975b47fe0512d0d4ebcf6b6e30d42c5df83ae3cfd13f87c635b7fec4ee2`，仍为未发布 dirty 工作来源。公开原进度 version30、339字节、SHA256 `2ff4c8be73c5368d0cee44b91af2492a6a241d146c9fe630236bd344981f2707`，恰好九个 true 布尔字段，无 publication 元数据；API和独立目检一致9/9。本轮已证明原 extra 漏回写问题在这一固定来源和实际选择上得到修复，不外推全部活动或主线新环境资格。

整链仍未通过：Runner 在 `publish.execute` 的唯一 start_turn `bbe27895a8d0e57683d4201371a55d48` 为 outcome_unknown，诊断是没有已核实 acceptance receipt、未重发。原生界面显示 `[Pasted Content 3427 chars]` 仍在草稿；完整公开 native session 四页、相同 provider/main，仅有初始化和原实施两条 user，没有接纳的发布输入。UI记录 Orca 命令 context deadline exceeded / unexpected end of JSON input；尚未证明具体投递或提交根因，不能据此声称 Runner 某一内部路径已定位。Gitea state=all、limit50完整六页254条查询没有本轮分支对应 PR。没有补 Enter、重发、手工发布或放宽确认，不能以9/9代替真实发布。

随后只停止该原 Run 一次，核实 stopped / new_inputs_allowed=false，等到 collection closed，再截取并独立目检9/9、无PR的停止后页面；版本/字节/SHA不变。正常停止宿主后，offline resources_settled=true，完整精确 owned native census0 / truncated=false / omittedHosts为空，本地 home/bin进程0。两次原报告/输入采集的临时 PTY 均确认 ptyKilled。原 Run 和宿主已完整结算，不恢复旧现场；第二轮未 clone、准备或启动，连续数仍为0/2。终端清理仍不宣称任意脱离进程或执行机凭据已撤销。

证据根 `C:/SteerlineDebug/wt-507e-progress-0of6/entryfixed-01/`：`blocked-checks-verified.json`、`chain-result.json`、report/input-evidence-index、`verification-20261027T232418439Z-blocked-reports/`、`verification-20261027T232747678Z-blocked-inputs-02/`、`diagnostics/native-public-session-unconfirmed-publish-20261027T232229307663Z/`、`publication-pr-query-complete.json`、`cleanup-blocked-publication/result.json`、`closed-ui-reviewed.json`。原与 closed 截图分别为 `observations/2026-10-27T23-18-28-157Z/`、`2026-10-27T23-36-51-528Z/`。首次采集器命令类型筛选错误保留为自身失败，修正后只读原产物，不当作业务失败或通过。

按既定职责，后续发布输入的提交/接受确认需 Runner 侧修复并独立验收；本会话不增加原生输入逻辑，也不发跨会话消息。取得实际合格新构建后再做正式新包与新现场准入，原目标仍为两次连续完整整链；当前保持等待、PR399草稿，无定时任务、子代理、合并或部署。clean发布head测试、完整MagicPotion/人工验收及当前main新环境代际原生资格仍未获得。
