# 环境检测 PowerShell 5.1 基线验收

2026-10-31，按用户确认的 [Spec](../../spec/environment-powershell-baseline.spec.md) 与 [Plan](../../plans/environment-powershell-baseline.plan.md) 实施。不修改 Runner 源码，不部署安装环境。本记录续接 [本机 Orca 原验收](local-orca-environment-20261031.md)，不覆盖历史版本及其结论。

当前结论：用户授权的正常重启已完成，真实 Runner 链已通过 TTY、PowerShell 5.1、六项环境检查及完整 L1/L2；发布阶段暴露项目脚本的 PS5 空 JSON 数组兼容缺陷。该缺陷已最小修复并通过定向回归，待新固定版本的完整原生验收，当前不能登记 ready。不把独立诊断会话的失败等同原 Runner 链失败。

## 实现及审查

Windows `.ps1` 材料和 manifest 明确使用 `powershell.exe`；移除 PS7 版本门槛及其专属 JSON、摘要、文件 API。共享进度 writer 保留严格 UTF-8、布尔对象、Ordinal 唯一键、锁、合并及原子替换，以有界正则校验协议输入，再解码单个字符串 token。不存在第二套 writer 或自动解释器回退。308 范围限于环境检测与完整 L1/L2 必经脚本，不改其他发布工具的基线。

曾尝试要求 Agent 设置 `exec_command.shell`，同版 Windows Codex 0.159.2 的确定性诊断证明该参数未改变实际外层 shell，随后三仓均已撤回这组指令，未保留无效配置要求。原生测试 fixture 的 Plan 仅补充明确的零自动测试安排，保持原探针职责。

独立审阅者 `/root/local_orca_registration_review` 已审查代码、实际正文、严格输入与并发回归；最终复核三仓撤回后正文与原版本逐字相同、PS5 改造保留。完整差异及逐文件字节/SHA 账本位于下述证据根的 `diagnostics/ps51-review-62fc66b8245d4c0c9072e808fa50002b/`。固定材料的实际 UTF-8 LF 字节变化：

| 材料 | 修改前 | 修改后 | 原因 |
| --- | --- | --- | --- |
| environment-generation | 7004 | 7279 | 固定 Windows 自带解释器及 PS5 可用 API |
| environment-execution | 5468 | 5478 | 执行入口使用 powershell.exe |
| testing | 3196 | 3229 | 共享进度 writer 的 PS5 基线 |

上述三份材料在项目镜像中同步。共享 writer 为 3196→3717 字节。实际原生零测试 Plan 为 222→306 字节，已单独独立审核；不是用黄金样本更新代替正文批准。

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

早期较宽 Go 取证包调用含 50 pass、1 live skip、1 failure：`TestTwoProjectsIsolateTasksGitAndCleanup` 在 StopService 后读取 Tasks 时缺 discovery.json。原日志为 `diagnostics/ps51-evidence-f75d2343b21e4eb4a03441c8dd38552d/`；未证明其为基线问题，不声明整包通过。上表 42 项为精确受影响范围。

## 真实本机 Orca：原失败与配置生效边界

所有正式尝试均使用原入口 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <新目录>`。独立 Runner 源码固定 cf29e1484f38b8e98e87e1c6505cdc96da8cbdb4，EXE SHA256 为 `f8d3c6a38d6381a5ac788070566e1189a3584f0bd767b0ca15bc8d66174a0a5c`，helper 为 `1320bf201492994da04991c0118549f0648b99d6ca1df99d966d6b72903cf7e9`；本机 orca-terminal/full_access、gpt-6-luna medium、development 2、每轮 1200 秒。项目为 SteerlineTest。

| test-results/L4/ 原报告 | 实际结果 |
| --- | --- |
| ps51-ceaa6f88c73b40d88d5db4e4084323f9 | 材料生成返回未知测试 ID，未创建环境活动或 Run；不属于环境 not_ready。补充精确资源清理证据 `diagnostics/ps51-pre-environment-cleanup-7b1e968c2fa1419dbaf4822aab6cb20c/`，不覆盖原 cleanup=false |
| ps51-3dc6ed5ff6704ec78d701f2c4bbdad92 | 修正 Plan 后真实三轮负向，环境 failed/not_ready/settled，原 Run stopped/result closed，无 PR、派发拒绝、清理完成。Agent 实际外层 shell 仍选择 WindowsApps PS7，TTY 启动失败；未执行完整 prepare/L1/L2/发布。正向 L4 保持 passed=false |
| ps51-c5ff93a7acc4475bbe741d2128346a59 | 显式 shell 指令未改变实际外层宿主；三轮负向、cleanup_complete=true，正向 L4 passed=false。该无效指令随后撤回。本轮完整原正文独立复核尚未完成，不据此追加签收 |
| ps51-a091d34690064e288abe54cb7e606362 | 干净 Root fa31ec5d、项目 867f6c6，新 runtime d64c。真实 prepare 完整通过，publish 推送探针分支后因空 PR URL 失败；只查询未重发发布。原 Run 正常停止且 result closed，但发布 unknown 使活动保持 unverified/not settled。随后中断唯一自有等待驱动，未生成最终 summary；不算 L4 通过。原 Runner 清理 confirmed，原测试分支/工作区/自有注册的补充清理待具体用户授权 |

用户随后批准关闭 Windows pwsh 应用执行别名，并精确撤销此前唯一新增的 User PATH 项。已执行：设置界面对应开关为关，顶层 pwsh.exe alias 不存在；User PATH 与原备份精确一致，Machine PATH 和其他 User PATH 项不变。证据 `diagnostics/ps51-approved-configuration-962a2d181b3048dab8cf082349fdb27a/configuration.json`，独立审阅者再次读取持久 PATH 验证一致。

配置后的只读新终端预检 `diagnostics/ps51-after-configuration-preflight-c18a44e281f74330b246f3b3c7b0870c/` 仍解析到旧进程继承的两条 pwsh 路径；本次自有诊断终端已单次关闭，原 CLI 回执逐字副本 `close-transcript-copy.json` 的 ptyKilled=true。不关闭或向原 worker 补发输入。另起子进程使用持久 Machine+User PATH，可解析 Git 和 PowerShell 5.1，pwsh 列表为空；这只是配置诊断，不算 Orca 原生验收。

首次正常关闭 Orca 的 UI 操作曾被自动审批拒绝，因为旧 d117c7bdfe87aca680dbacaab4618773 停止状态未知工作区仍有两个终端。用户随后明确接受会话可能断开的风险并批准正常重启。旧桌面进程正常退出，新桌面 PID 25024、runtime d64c6673-348a-4c73-9f1c-c60a51ec8f0c；没有强杀或操作旧终端。原 daemon 保留，不据此断言全部进程环境已重置。证据 `diagnostics/ps51-approved-restart-f9bc63e96fc84e2db3c7890471159806/` 的 status.json 是启动中间 not_running，后续 status-ready.json 及公开终端回执证明新 runtime。

重启后的自有诊断终端仍有持久 PATH 以外的应用包内 pwsh 别名；本次探针已关闭且原回执 ptyKilled=true。独立 fake-provider Codex 会话选中该别名并返回 AccessDenied，但它使用独立 CODEX_HOME 与工具配置，不能代表原 Runner 会话。随后原公开终端历史 `diagnostics/ps51-restarted-native-observation-64ffe477b6ff4789ad412bf0122e5de9/terminal-history.json` 证明实际 tty/prepare 通过，报告记录 PS5.1.26100.9711、原完整 L1/L2。保留两种事实，不推断原链仍被别名阻断，也不为原链改写配置。

原生发布失败已用真实 PS5 独立复现：`@('[]'|ConvertFrom-Json).Count` 为 1，解析结果加括号后为 0。Root/ST 对原列表解析增加一层括号，保留发布顺序、身份、unknown 与零重复写入规则。308 原函数及过滤路径通过同样回归，未改生产脚本。

原失败活动 817f004767774d71be909406aba5771d / Run bf1902a1a79dc316c37650b577335a37 的停止首次明确 rejected/stale_revision；按公开契约刷新后仅再提交一次，回执 applied，不重复已受理停止。`diagnostics/ps51-restarted-native-stopped-9b163f84aea245c79894bf443491b179/` 核实 stopped/result closed，原字节 SHA256 `85749a3f6c6701aa603e758a9bfe1e8cb189ee4d28594d732ac34f19169a5932`；round settled=true，但活动因原报告 publication unknown 仍 unverified。停止观察 helper 在应用退出后如实返回 identity rejected。`diagnostics/ps51-publication-array-cleanup-8349c1b4d8aa46ff868a0ccb147a4a6a/runner-cleanup.json` 是后续现有 helper 的原清理回执 confirmed=true，不冒充场景自身的完整清理报告。

专属三个远端测试 refs、原工作区和自有注册的补充删除被自动审批拒绝，理由为具体删除范围缺明确授权；已向用户询问，确认前保留。旧 d117 及用户既有项目不属于这次删除范围。待完成新固定版本完整 L4、PR/派发门禁/原 Run 结算及独立复审；正确 not_ready 仍可验收检测流程，不能以原发布脚本失败或缺失报告登记完成。
