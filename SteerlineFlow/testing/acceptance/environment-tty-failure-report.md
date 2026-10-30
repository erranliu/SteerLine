# TTY前置失败报告收尾验收

2026-10-30，用户授权修改并实测。[Spec](../../spec/environment-tty-failure-report.spec.md) · [Plan](../../plans/environment-tty-failure-report.plan.md)。普通开发，不部署安装环境，不修改Runner、caller状态机或报告版本。历史远端未完成报告保留于[原验收](environment-publication-scope.md)，不改写为通过。

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
| SteerLine真实pwsh项目脚本 | `diagnostics/script-owner-034bb0116f2542a18c1b1c05d11636a3/test.log` | 5个方法（含多组非法输入子例）全通过，未执行Git/环境探测/测试/认证或PR |
| 完整环境owner | `diagnostics/negative-budget-8221337cd7c44e0ea9597d42d5491794/` | 71项通过、零跳过；新增三轮TTY负报告各原Run停止/结果closed，最终failed/not_ready/settled，无人工取消或第四轮 |
| 固定提示词/远端输入/正式报告验证器 | `diagnostics/contracts-b46a600260d94d51907b333a8dc0e6ff/` | 156项通过、零跳过 |
| 308项目真实pwsh报告入口 | `diagnostics/308-script-owner-1e8bba2079cd45068ace12da564b581c/test.log` | 5方法全通过；13个原要求ID均保留，未启动Unity |

SteerlineTest独立clone `C:/SteerlineDebug/wt-5069-environment-publication-scope/sources/SteerlineTest`：脚本5方法全通过，日志 `.steerrunner/artifacts/tty-report-owner-295c53d1af3c44ba995743160ff83b0b/test.log`。固定材料提交 `b012d0ae4ade12a41dc47cf8571b45a42fc6a968` 的原完整L1五项/L2一项均通过、零跳过；原报告 `.steerrunner/artifacts/tty-failure-b53f8f487d8c40e48da3d63d0afb8efb-l1`、`-l2`。

308独立clone同sources下308，固定材料提交 `1c25de58c87b99f44f9fc43f61de09444ba4391d`；仅报告入口/AST/diff验证，未在本机运行Unity/L1/L2。两个项目均未自动合并，未改用户环境配置。

## 待验范围

固定SteerLine提交的L1、完整L2和受影响readiness L3继续执行。TFR-A04真实远端尚未开始：材料发布前发现台式机64G的Orca再次remote_runtime_unavailable，Gitea192.168.0.66:3000 API超时/EOF、Git空响应，已告知用户恢复连接；不替换远端目标或绕过原注册repo/terminal链。连接恢复后固定新308提交/原Runner/profile，以新正式L4原报告核实三轮负报告正常结束与原Run/自有ref清理；环境not_ready不冒充正向L4通过。
