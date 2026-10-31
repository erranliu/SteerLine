# 环境检测 PowerShell 5.1 基线验收

2026-10-31，按用户确认的 [Spec](../../spec/environment-powershell-baseline.spec.md) 与 [Plan](../../plans/environment-powershell-baseline.plan.md) 实施。不修改 Runner 源码，不部署安装环境。本记录续接 [本机 Orca 原验收](local-orca-environment-20261031.md)，不覆盖历史版本及其结论。

当前结论：本机 Orca 正向完整 L4 已通过独立复审，单轮 ready、真实 PowerShell 5.1/TTY、完整 L1/L2、真实 PR #720、准备前派发拒绝与 ready 后实际准入、原 Run 正常停止及本轮资源清理全部核实。用户批准的别名关闭、原 PATH 恢复及正常重启已完成；没有修改 Runner 源码或部署安装环境。本结论针对下文固定候选版本与独立验收现场，不改写安装环境的项目状态，不借用本机 Codex/远端 Orca 历史结果。

## 实现及审查

Windows `.ps1` 材料和 manifest 明确使用 `powershell.exe`；移除 PS7 版本门槛及其专属 JSON、摘要、文件 API。共享进度 writer 保留严格 UTF-8、布尔对象、Ordinal 唯一键、锁、合并及原子替换，以有界正则校验协议输入，再解码单个字符串 token。不存在第二套 writer 或自动解释器回退。308 范围限于环境检测与完整 L1/L2 必经脚本，不改其他发布工具的基线。

曾尝试要求 Agent 设置 `exec_command.shell`，同版 Windows Codex 0.159.2 的确定性诊断证明该参数未改变实际外层 shell，随后三仓均已撤回这组指令，未保留无效配置要求。原生测试 fixture 的 Plan 仅补充明确的零自动测试安排，保持原探针职责。

独立审阅者 `/root/local_orca_registration_review` 已审查代码、实际正文、严格输入与并发回归；最终复核三仓撤回后正文与原版本逐字相同、PS5 改造保留。完整差异及逐文件字节/SHA 账本位于下述证据根的 `diagnostics/ps51-review-62fc66b8245d4c0c9072e808fa50002b/`。固定材料的实际 UTF-8 LF 字节变化：

| 材料 | 修改前 | 修改后 | 原因 |
| --- | --- | --- | --- |
| environment-generation | 7004 | 7476 | 固定 Windows 自带解释器、PS5 可用 API 及提交前原暂存核验 |
| environment-execution | 5468 | 5478 | 执行入口使用 powershell.exe |
| testing | 3196 | 3229 | 共享进度 writer 的 PS5 基线 |

上述三份材料在项目镜像中同步。共享 writer 为 3196→3717 字节。实际原生零测试 Plan 为 222→306 字节，已单独独立审核；不是用黄金样本更新代替正文批准。

generation 的后置守卫正文为 7279→7476 字节，SHA256 `59230d4b02723693737acb97f82867c39b3388ed201f4893043900048559dc85`。三仓镜像逐字节一致；实际正文获批后才同步唯一黄金摘要。Root/ST 另正规化 PS5 空 PR 列表，三仓提交前校验唯一原探针、非空暂存 diff、未 Trim 的原正文及 `diff --cached --check`，输出原 diff 后才 commit；不新增模型豁免或 Runner 专用能力。

## 固定源码与测试

证据根为 `C:/SteerlineDebug/wt-5069-local-orca-environment/`，下表现场路径相对此根。首轮基线实现提交如下，工作树在当轮测试前干净；后续数组修复的原生输入另行冻结：

- SteerLine：`9b129527f618cac0f7c8e325f90c451379d0b0c7`。
- SteerlineTest：`867f6c65704d135bdf2505b7a4b87615a234c7d2`。
- 308：`354785c9c1c7bb5a8db57dd2150447fb90a6cafe`。

前序回归按原报告的固定提交记录，不冒充全部在以上提交重新执行。上述三份基线提交的源码树分别与撤回前的 a6da65e4、4570ae5、2b6cd08a 实现树一致；不将此等同随后数组修复的源码树。

| 实际验证 | 结果与原始位置 |
| --- | --- |
| 9b 基线 SteerLine 正式 L1/L2 | 通过；执行 PATH 不含 pwsh。`test-results/L1/ps51-edcbb38971b1496d8b7ca9b96ae90a65/`、`test-results/L2/ps51-fb9dfcef1ef94631bf59e549071796da/`；包装器 `diagnostics/ps51-formal-root-basic-8e3eda57b5e44e82aa886e11170acc93/summary.json` |
| 定向 owner | 固定 2fcd23c4，134/134 通过、零跳过；真实 PS5 严格 JSON/Unicode/大空白输入/Go 并发互操作。`diagnostics/ps51-owner-fixed-9ac6043f170042eba71607ae84cd8dcd/` |
| 原生取证及资源所有权回归 | 固定 2fcd23c4，42/42 通过、零跳过。`diagnostics/ps51-evidence-fixed-26b5537143b64992832f5c693112b369/` |
| controlled L3 testing.project-environment-readiness | 通过，完整清理；`test-results/L3/ps51-171c5ece437c4779a0accfa7b43a3f56/`。这是受控门禁流程，不代替真实 Agent |
| 三仓 TTY 与失败报告、SteerlineTest L2 绑定 | 在过滤 pwsh 的 PATH 下通过。`diagnostics/ps51-controlled-da56f1d98daa4926af3c26ece230c8f3/`；其中 308 首次受管协议的 StrictMode Count 失败另保留，修复后补测 |
| SteerlineTest 基线正式 L1/L2 | 固定 4570ae5 通过，`diagnostics/ps51-formal-st-66c51e22ca2248d7ab99155203735ab6/`。867f6c6 基线源码树与该版本一致 |
| 308 受管报告与进度协议 | 修复后 42+15 个断言通过；`diagnostics/ps51-308-managed-c9c6c4f63cd64a7a9bc72d2c77b6c730/`，正式包装器复核在 `diagnostics/ps51-formal-308-d7b94c9c956b4472a3784e7860373b65/` |
| 308 正式 Unity L1/L2 | Incomplete。真实 Unity 2022.3.62f2 已启动，但缺有效 ULF license/entitlement；不计业务测试通过，不更改许可证。原始目录为 `test-results/L1/ps51-308-42979ee4212e4a8bbfc45b1aabf9f641/` 与 `test-results/L2/ps51-308-286210f771cf4418b2cf30327b00c626/` |
| 原发布块 PS5 数组与身份回归 | `diagnostics/ps51-publication-regression-4baee71b329c4fdb95204603da3a3756/`。旧 Root fa31 的空列表与多列表反例稳定失败；修复后 Root/ST、未改生产逻辑的 308 各七类边界通过。测试运行原 provider 函数与原发布块，使用 scoped provider doubles，不是远端发布验收。前一次 Gitea .cmd 夹具被查询 ampersand 截断的失败保留于 `ps51-publication-regression-eb1814dc34ef48aba49dec70ebe97e5b/`，修正夹具转发后重跑，不据此修改 Gitea 生产代码 |

最终回归与正向 L4 的固定源码：SteerLine `b603f7f7d61d9c3304eb698dfe85dddef7228e73`、SteerlineTest `9e0c33066b23e7c7eb8778fb87fd32ef9a9f8577`、308 `fc18b2d238880d140d0a460b23a7acf65e780503`。以下结果不借前述基线代替：

| 最终范围 | 实际结果与证据 |
| --- | --- |
| Root b603 正式 L1/L2 | 全部通过；`diagnostics/ps51-formal-root-basic-540d45a05fc9471c83b6b5b0bb6daa2a/`，实际目录 `test-results/L1/ps51-6c364c60b4b74d95be89c2fc41c232dc/`、`test-results/L2/ps51-84bc161a4cd84cc38c3eb0e486a29b9a/`，源码干净、无 pwsh PATH |
| SteerlineTest 9e0 正式 L1/L2 | 全部通过，`diagnostics/ps51-formal-st-27ab41629c154cb183c98ebeacb67c2b/` 指向项目原始 summary/managed |
| 三仓原提交/发布块 | `diagnostics/ps51-publication-regression-a4188bf2bf0d4492a15f74d9dac4d613/`：各两个方法、13 个子场景通过；真实 PS5、隔离 Git、无 pwsh PATH。六类暂存状态与七类原 provider 路径，旧源码真实失败。初次 Git 夹具 cwd 不符合原调用导致的失败保留于 `ps51-publication-regression-d43a7dc2ca774781a123b356c5d09179/`；修正夹具并增加准确拒绝原因断言后补测 |
| 原派发启动时序 | `diagnostics/ps51-admission-start-owner-f885469e281a4e7da192825dbb0748c1/`，9/9、零跳过；五阶段只在原 turn 身份齐全后触发一次停止，七种外来或未知状态拒绝；诊断在提交前执行，实际正文由账本及随后 b603 冻结 |
| 308 fc18 正式入口 | `diagnostics/ps51-formal-308-14db7cbb4337408cbb1c814b1909bee7/`：受管协议通过，Unity L1/L2 仍 Incomplete（缺有效许可证），实际目录 `test-results/L1/ps51-308-914f7e19afe8455492aca21b5f93ff7f/`、`test-results/L2/ps51-308-370ef46d3a7340618f8afd972d51bba8/`。不计 Unity 业务通过 |

完整最终代码/材料差异及双 SHA/字节账本 `diagnostics/ps51-review-b36cdccdc87d4d2f963ef2ec1c4eb188/` 已独立复算 54 行、零差异；其后只有本验收导航和记录更新。原发布守卫 13 子场景及材料收口分别经独立审查，原始审核账本 `ps51-review-bfbc3cdb4c004e85a8206008bdeb772a/`、`ps51-review-619961dcfbd64d469d148132a5e67d73/`、黄金同步后的 `ps51-review-774aa4cb37c041c6ba3329792ef7a197/` 保留。

早期较宽 Go 取证包调用含 50 pass、1 live skip、1 failure：`TestTwoProjectsIsolateTasksGitAndCleanup` 在 StopService 后读取 Tasks 时缺 discovery.json。原日志为 `diagnostics/ps51-evidence-f75d2343b21e4eb4a03441c8dd38552d/`；未证明其为基线问题，不声明整包通过。上表 42 项为精确受影响范围。

## 真实本机 Orca：原失败与配置生效边界

所有正式尝试均使用原入口 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <新目录>`。独立 Runner 源码固定 cf29e1484f38b8e98e87e1c6505cdc96da8cbdb4，EXE SHA256 为 `f8d3c6a38d6381a5ac788070566e1189a3584f0bd767b0ca15bc8d66174a0a5c`，helper 为 `1320bf201492994da04991c0118549f0648b99d6ca1df99d966d6b72903cf7e9`；本机 orca-terminal/full_access、gpt-6-luna medium、development 2、每轮 1200 秒。项目为 SteerlineTest。

| test-results/L4/ 原报告 | 实际结果 |
| --- | --- |
| ps51-ceaa6f88c73b40d88d5db4e4084323f9 | 材料生成返回未知测试 ID，未创建环境活动或 Run；不属于环境 not_ready。补充精确资源清理证据 `diagnostics/ps51-pre-environment-cleanup-7b1e968c2fa1419dbaf4822aab6cb20c/`，不覆盖原 cleanup=false |
| ps51-3dc6ed5ff6704ec78d701f2c4bbdad92 | 修正 Plan 后真实三轮负向，环境 failed/not_ready/settled，原 Run stopped/result closed，无 PR、派发拒绝、清理完成。Agent 实际外层 shell 仍选择 WindowsApps PS7，TTY 启动失败；未执行完整 prepare/L1/L2/发布。正向 L4 保持 passed=false |
| ps51-c5ff93a7acc4475bbe741d2128346a59 | 显式 shell 指令未改变实际外层宿主；三轮负向、cleanup_complete=true，正向 L4 passed=false。该无效指令随后撤回。本轮完整原正文独立复核尚未完成，不据此追加签收 |
| ps51-a091d34690064e288abe54cb7e606362 | 干净 Root fa31ec5d、项目 867f6c6，新 runtime d64c。真实 prepare 完整通过，publish 推送探针分支后因空 PR URL 失败；只查询未重发发布。原 Run 正常停止且 result closed，但发布 unknown 使活动保持 unverified/not settled。随后中断唯一自有等待驱动，未生成最终 summary；不算 L4 通过。原 Runner 清理 confirmed；后续用户明确批准三个 refs、原工作区及自有注册的补充清理，见下文 |
| ps51-36ebe5e108e14d8fb9dd6799b0d147c8 | Root 982608cf、项目 56c2cd6；两轮真实 prepare 通过，但 Agent 因原脚本缺提交前暂存 diff 核对而未调用 publish。原活动按正式 stop 取消，两原 Run/result stopped/closed；原 L4 passed=false/cleanup=false、Runner 清理 confirmed。后续用户单独批准该组两 refs、两工作区及自有注册删除；补充清理完成，不覆盖原失败 |
| ps51-a0fa1bc977bd4cd18d58469cfddf8920 | Root 52d7bdc6、项目 9e0c330；环境原 Run 单轮 ready、PR #719 已正常合入专属验证 base、result/progress closed，独立证据通过。后置派发探针 cab021 在 turn_started=false、TurnID 空时即被测试驱动停止，返回 native stop unknown；整个 L4 passed=false/cleanup=false，不将环境检测成功混成全场景通过。该原 Run 及所属资源隔离保留 |
| ps51-f60dd0a8c0d74ee0937c8413da3a5074 | Root b603f7f7、项目 9e0c330；共用驱动等待原派发 turn 真实启动再停止。完整 L4 exit0、summary/managed/scenario passed=true、cleanup_complete=true、无 skip/errors。真实 ready、PR #720、准备前拒绝→ready 后准入及本轮资源结算全部独立复核，见最终原生证据 |

用户随后批准关闭 Windows pwsh 应用执行别名，并精确撤销此前唯一新增的 User PATH 项。已执行：设置界面对应开关为关，顶层 pwsh.exe alias 不存在；User PATH 与原备份精确一致，Machine PATH 和其他 User PATH 项不变。证据 `diagnostics/ps51-approved-configuration-962a2d181b3048dab8cf082349fdb27a/configuration.json`，独立审阅者再次读取持久 PATH 验证一致。

配置后的只读新终端预检 `diagnostics/ps51-after-configuration-preflight-c18a44e281f74330b246f3b3c7b0870c/` 仍解析到旧进程继承的两条 pwsh 路径；本次自有诊断终端已单次关闭，原 CLI 回执逐字副本 `close-transcript-copy.json` 的 ptyKilled=true。不关闭或向原 worker 补发输入。另起子进程使用持久 Machine+User PATH，可解析 Git 和 PowerShell 5.1，pwsh 列表为空；这只是配置诊断，不算 Orca 原生验收。

首次正常关闭 Orca 的 UI 操作曾被自动审批拒绝，因为旧 d117c7bdfe87aca680dbacaab4618773 停止状态未知工作区仍有两个终端。用户随后明确接受会话可能断开的风险并批准正常重启。旧桌面进程正常退出，新桌面 PID 25024、runtime d64c6673-348a-4c73-9f1c-c60a51ec8f0c；没有强杀或操作旧终端。原 daemon 保留，不据此断言全部进程环境已重置。证据 `diagnostics/ps51-approved-restart-f9bc63e96fc84e2db3c7890471159806/` 的 status.json 是启动中间 not_running，后续 status-ready.json 及公开终端回执证明新 runtime。

重启后的自有诊断终端仍有持久 PATH 以外的应用包内 pwsh 别名；本次探针已关闭且原回执 ptyKilled=true。独立 fake-provider Codex 会话选中该别名并返回 AccessDenied，但它使用独立 CODEX_HOME 与工具配置，不能代表原 Runner 会话。随后原公开终端历史 `diagnostics/ps51-restarted-native-observation-64ffe477b6ff4789ad412bf0122e5de9/terminal-history.json` 证明实际 tty/prepare 通过，报告记录 PS5.1.26100.9711、原完整 L1/L2。保留两种事实，不推断原链仍被别名阻断，也不为原链改写配置。

原生发布失败已用真实 PS5 独立复现：`@('[]'|ConvertFrom-Json).Count` 为 1，解析结果加括号后为 0。Root/ST 对原列表解析增加一层括号，保留发布顺序、身份、unknown 与零重复写入规则。308 原 Gitea 数组路径通过同样回归，未作该数组修复；后续三仓共同补齐了提交前原暂存守卫。

原失败活动 817f004767774d71be909406aba5771d / Run bf1902a1a79dc316c37650b577335a37 的停止首次明确 rejected/stale_revision；按公开契约刷新后仅再提交一次，回执 applied，不重复已受理停止。`diagnostics/ps51-restarted-native-stopped-9b163f84aea245c79894bf443491b179/` 核实 stopped/result closed，原字节 SHA256 `85749a3f6c6701aa603e758a9bfe1e8cb189ee4d28594d732ac34f19169a5932`；round settled=true，但活动因原报告 publication unknown 仍 unverified。停止观察 helper 在应用退出后如实返回 identity rejected。`diagnostics/ps51-publication-array-cleanup-8349c1b4d8aa46ff868a0ccb147a4a6a/runner-cleanup.json` 是后续现有 helper 的原清理回执 confirmed=true，不冒充场景自身的完整清理报告。

首轮三个远端测试 refs、原工作区和自有注册的补充删除曾被自动审批拒绝；用户随后明确批准具体范围。`diagnostics/ps51-publication-original-resource-cleanup-95e5fdd896784289a0cd1b2327de775a/` 核实零终端、无 PR、三个精确 SHA 租约后删除这三条原 refs 与 bf190 原工作区、自有 setup；异步删除/原 Git 注册均结算，源 clone 和原报告保留，用户项目及旧 d117 未触碰。

第二轮两个原 Run aef23c3bfb80e323354d277cf69147f9、b45b148ad7e6709741777c5fe27dfa85 均 stopped/result closed，原字节 SHA 分别为 `eb14c5548ced756db7ff2c32ea36f8d94843de1bb38c9603c3fe172e28291d44`、`fa2dcdb23e29b1b50c3f7a4f7bc932effe7c496eed688bfe6b207b422ccd9931`。清理计划 `diagnostics/ps51-second-cleanup-preflight-37bbfc00b956425a9c546aa0b47a4a24/cleanup-plan.json` 先只读核实两 refs 原 SHA、无 PR 和三个位置零终端；用户随后单独批准。等本轮 L4 结束后重新核验，仅清理这两个原 refs、两个原工作区及 8272 自有注册，`diagnostics/ps51-second-authorized-cleanup-4d5625608df44a478b97519ab6c4c155/cleanup.json` confirmed=true。源 clone、报告和其他全部既有 setup 保留，不混用首轮授权或本轮 L4 清理回执。

第三轮的环境 Run 8f57ddd406b548cf60e04840468c2e4f 已 stopped/result closed（1343 字节、SHA `c65ec48defd910b3d9d8375750b2c90eaa5de76213e84c7c1b105689380b17e3`），其 ready/PR #719 独立有效。未知的是另外的后置派发 Run cab02162b58a7fff214b320ac5b28a69；不重发已受理停止、不关闭原 PTY、不删除它所属 refs/工作区或伪造退出证明。新等待逻辑只修 L4 驱动的时序前置，未修复或验收 Runner 提前停止能力。

## 最终原生证据与验收结论

最终输入 `diagnostics/inputs-ps51-831407b071384b6c81e5ce58d77ccd82/manifest.json` 固定干净 Root b603、项目 9e0、原独立 Runner/helper/每轮预算，并明确列出旧 d117 与 cab021 保留。最终正式报告根 `test-results/L4/ps51-f60dd0a8c0d74ee0937c8413da3a5074/`；活动 `9288dfd222cf4e659f1ad17183cd555b`，原环境 Run `9cb6990c190cc23667b75d7a5a72b7c0`，fixture `3a2b48ec5b4d95aeedc0bfd7fa5d12997d19d70e`。

- `environment-proof.json`：单轮 ready，六项 checks 全部 passed，实际 PowerShell 5.1.26100.9711/真实 TTY；原完整 L2 唯一 demo.l2.smoke 实际执行 1 项，零失败/错误/跳过，前后 clean 且固定 fixture 不变。脚本 Git blob SHA `e85d312bde215d28f344ba031105a2a3730200bd5d2e90633a1148d74a2c8218` 与原输入一致。
- 原 Run stopped、stop_status=stopped；result 与 progress 双 closed，原 result 1343 字节、SHA `31d02f72e2ad29bd5156a3fd0d2a3b6547d6829b3b98903af370e4156c2e6649`，raw 解码/typed/初始五项身份一致、无 previous，两个进度布尔值为 true。报告只有在执行及生产者停止后封存一次。
- PR [#720](https://github.com/erranliu/SteerlineTest/pull/720) 的 head `e244f53a418227f7ad32d105c1c4125b1980b49c`、原 URL/base/head 全部核实，只有一个文件/一个提交；外部按声明检查及审核要求正常合入专属验证 base，merge `91197b4c56157d81d8efbae029c41f3af1d87ec1`。未合入 main，未部署安装环境。
- `admission-before.json` 明确 environment_not_ready 拒绝；ready 后原派发 Run `a09c3a590abbffa91e2c9f36ade1ea31` 的 `admission-started-run.json` 记录 turn_started=true、完整原 session/turn 身份，随后单次受理停止，最终同 Run/digest/session/turn stopped。`admission-proof.json` 与原 stopped candidate 保持原参与绑定，没有用另一个 Run 替代。
- `308-runner-cleanup.json`、`remote-ref-cleanup.json`、`orca-local-registration-cleanup.json` 均 confirmed=true；只处理本轮四条 refs、两个原 worker 与自有 setup 027cabe2，零终端、异步删除及 Git 注册结算核实。该 L4 的结束清单保留两个用户项目、旧 d117、当时的第二轮 8272 与第三轮 974b；随后单独获准清理 8272 见上，不混称所有历史现场已清理。最终本轮 scratch 与 worker 路径已不存在。

正式外部取证沿同一 Orca 公开 Shell 入口；原 `original-l2-summary.json` 与公开分片解码完全一致。补充公开终端 `diagnostics/ps51-admission-fixed-native-observation-212568217b7949b5b62c3acc9c67bcf7/terminal-read.json` 只佐证实际 publish 命令、原暂存 diff 及 PR #720 查询；关闭后读取的 terminal-history.json 为零行，不称完整 TTY 历史，也不借用 #719 的历史替代本轮证据。

独立审阅者 `/root/local_orca_registration_review` 已复核本轮 raw/typed、固定来源、原 L2、PR 原回执、启动后停止、全报告及清理，批准本机 Orca 的完整正向 L4。本轮 live 覆盖“准备前拒绝→ready 后准入及正常停止”；失效后再次拒绝仍仅有前述 controlled L3，不声称本次 live 覆盖。历史 not_ready/流程失败、308 Unity 许可证未满足、其他两条路径未在新 PS5 版本重验及两原 unknown 均保留各自边界。
