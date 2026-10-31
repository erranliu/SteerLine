# 本机 Orca 环境检测续验

本文保留原固定版本的负向续验。后续移除 PS7 要求、恢复配置并完成本机 Orca 正向完整 L4，见 [PowerShell 5.1 基线最终验收](environment-powershell-baseline.md)；不将本文历史 not_ready 读成新版本结论。

2026-10-31，用户要求先不改 Runner，准备本机 Orca 配置并实际验收；不部署安装环境。真实三轮负向检测流程验收完成；环境仍为 not_ready，不登记 ready，也不借用本机 Codex 或远端 Orca 的结果。用户明确正确 not_ready 可通过流程验收；原正向 L4 报告保持失败。

## 固定输入与配置

证据根为 `C:/SteerlineDebug/wt-5069-local-orca-environment/`，以下现场路径相对此根。首次注册修复后的复测固定 SteerLine `0e6e433115ae875528d5b7f6ca993a6e4128108c`、SteerlineTest `b012d0ae4ade12a41dc47cf8571b45a42fc6a968`；最终负向闭环固定干净 SteerLine `5da9357dc3d8c480994ce36bb4060c5b7d1d5c5f`、SteerlineTest `868b2e7fc6808a404eff8ddcb823a4395c2e7c9a`。本机 profile 为 local / orca-terminal / full_access / gpt-6-luna medium / development 2，每轮 1200 秒，明确配置既有 `runtime_executable` 与 SHA。输入清单为 `diagnostics/inputs-matched-86839340fb9348458dc1d4f73398f956/manifest.json`。

Runner 使用已提交源码 `cf29e1484f38b8e98e87e1c6505cdc96da8cbdb4` 的独立干净构建，不新增 Runner 源码修改；版本 0.2.0-dev.4、resource_version 3、state/v5。EXE SHA256 为 `f8d3c6a38d6381a5ac788070566e1189a3584f0bd767b0ca15bc8d66174a0a5c`；既有 helper SHA256 为 `1320bf201492994da04991c0118549f0648b99d6ca1df99d966d6b72903cf7e9`。包目录 `packages/committed-cf29-4b2d0273e81543bf8ef39b571168c203/` 保存实际 build metadata。没有使用另一工作区尚未提交的本机/远端统一实现。

## 已修复的测试准备缺口

正式本机 L4 使用独立 clone；此前只按 remote URL 找到用户已有 Orca 仓库，Runner 却按调用方 clone 的 path 定位，导致 `selector_not_found`，尚未进入项目检测。修复仅在测试驱动：本机按原 remote 加精确 clone path 绑定，缺注册时通过公开 repo add 注册该独立副本；远端继续沿原 remote 路由。

仅撤销本次创建的 local setup。正常原 Runner/ref 结算后，按原环境/候选 Run 的 workspace 与 branch 验证自有工作区及零终端，每个原 id 只删除一次；随后有界只读等待 Orca 清单、原路径和 Git worktree 注册全部结算，最后删除自有 setup。已有用户项目不取得删除所有权。未知资源、不完整清单或未核实清理均保留失败。

独立审阅者 `/root/local_orca_registration_review` 拒绝最初的“只剩源工作区”假设、空清单误判及异步删除未等待；修复后批准实际六文件 diff 和前后 SHA/字节账本。账本及原 diff 在 `diagnostics/registration-review-ec9d2783909a4e55906f26c150e0538b/`；无提示词正文修改。

| 测试驱动文件（UTF-8 LF） | 修改前字节 | 修改后字节 |
| --- | --- | --- |
| orca_evidence_target.go | 688 | 2428 |
| orca_evidence_target_test.go | 932 | 3221 |
| project_progress_remote.go | 12491 | 13329 |
| remote_evidence.go | 16462 | 17183 |
| orca_local_registration.go | 0 | 6618 |
| orca_local_registration_test.go | 0 | 5892 |

定向 `go test -tags=e2e ./tests/e2e/internal/testenv -run '^TestOrcaEnvironment' -count=1` 通过；含精确仓库选择、原工作区所有权、19组清理副作用/拒绝/延迟反例，以及真实临时 Git 工作区“文件消失但注册仍在不得完成”的核验。L1 `test-results/L1/registration-final-5abfa5f876da42859ab902c71546bf64/`、L2 `test-results/L2/registration-final-ed1811d457624d66b9e4a513da2839f1/` 通过，L2 清理完整；两份报告如实记录提交前 dirty 状态，与账本和随后固定提交对应，不冒充干净提交历史。

## 真实执行与失败分类

实际正式入口均为 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <新目录>`，通过输入清单设置独立 Runner、local profile、项目配置、完整 SHA 和轮次预算。原失败 summary、managed、scenario 及 scratch 不改写。

| 正式现场（test-results/L4/） | 实际结果 | 补充停止与清理 |
| --- | --- | --- |
| configured-dbafb2c032d647029ac1a22921df4a27 | 首次旧包 SHA `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`；selector_not_found，业务输入为零，未执行检测 | 原正式 environment_stop applied；原 Runner 清理 confirmed；`diagnostics/selector-ref-cleanup-b1591ed8d973465e9854bc5ec127955a/cleanup.json` 证明两条自有 ref 无 PR、按原 SHA 租约删除。注册探针在 `diagnostics/registration-probe-459bf7b3d41441a6a6863be9f4c58bc2/` 单独创建并完整撤销 |
| registered-eae3c7c220ce4237aa6691362b27dcba | 精确注册/取证成功；门禁预检 start_task 的命令回执原子替换返回 Access is denied；没有创建环境活动，不属于环境检测结果 | 原 Runner 清理 confirmed；`diagnostics/precheck-cleanup-94ce87bf5e454654a0077d5a8e381dc8/cleanup.json` 核实无 PR、两原 ref 及本次 setup 已删除，保留源文件 |
| registered-recheck-ec93b8f1aa694993befcfae8d051daa1 | 新现场门禁前置通过；原 Run 真正启动并读项目材料，实际 exec_command(tty:true) 调用固定 tty 入口；找不到 pwsh，未运行 prepare/L2/发布，结果 completed=false | 原 environment_stop applied；原 Run stopped、result closed；原 Runner 清理 confirmed；`diagnostics/missing-shell-cleanup-dd8cdd87f22b4720855cd86b403f721d/cleanup.json` 证明无 PR、两原 ref、精确原工作区及本次 setup 已删除，源文件保留 |

最后一次原活动 `156899c91cac46e783d1d8fe407c0d5f`、原 Run `3a27c30e674156aa6d643332727cc384`，固定夹具 `44c7adde064ce7465681baef291e37ced3f77213`。公共取证 `diagnostics/native-observation-9374925b7f564a2f994c28746b9a562d/` 核实 turn_started=true、原 session/runtime/terminal 绑定及 Agent 工具错误；没有向原终端手动发送、Enter 或重放业务输入。

停止回执在 `diagnostics/missing-shell-stop-55e109aba1304715962cd523a28b1ac1/permission-failure-stop-receipt.json`：原 Run 已 stopped，result-file/v1 closed，初始 report2 原字节 SHA256 `4c1373fad90c7dfc2ae91b9df6b58ab4a1fb9fae0aef69d2ebabae0abe98af4c`，completed=false、checks/l2 为空。停止后测试器关闭应用，观察器随后 project_unavailable，不重发停止请求。

两类问题分别登记：Agent 的实际工具环境不能解析 pwsh，是已检测到的环境配置缺项；Agent 没有调用 tty-failure 收尾，原报告仍未完成，是本次检测流程未完整验收。此轮只有一轮，人工停止，不声称三轮负向闭环通过。正式 L4 保持 passed=false/cleanup_complete=false，补充清理不覆盖原报告。

## 未应用的可选启动配置

主机存在真实 PowerShell 7.6.9，原 Agent 工具调用观察到 pwsh 不可解析。PATH 继承与 WindowsApps 别名过滤是原因推断；本轮未采集实际 Agent PATH，也未以 cf29 源码冒充复用 helper 的原实现证明。当前准备了一份独立 Orca 启动配置，位于 `diagnostics/orca-launch-configuration-6cd53f7dd5894a6e95285f50fec1b020/`。配置固定真实二进制及 SHA，脚本只调整 Orca 进程及后代 PATH，启动后恢复调用方 PATH；拒绝在已有 Orca 进程时运行，不自行结束其他会话，不永久修改用户 PATH。

`validation.json` 记录真实二进制启动及脚本解析通过，未保存版本输出；尚未应用启动配置或重启 Orca。用户随后明确有效 not_ready 也可完成流程验收，因此不为达到 ready 而重启 Orca，启动配置只作可选准备。随后仅修复项目端负报告收尾并完成下节真实闭环；没有应用 PATH 配置或重启 Orca，不把前述 completed=false 算通过。既有 local runtime 配置可进入真实 Agent 工具执行，不证明环境 ready，也不证明必须修改 Runner。


## 项目端失败记录修复与真实负向闭环

SteerlineTest 的同一 verify 脚本及项目文档现在明确：原 pwsh 命令不可解析且纯最小探针已结束时，允许通过已有 `powershell.exe` 仅调用 `-Phase tty-failure`。Windows PowerShell 5.1 不执行替代探测、prepare、测试、认证或 PR；正常就绪仍要求 pwsh 7+、真实 TTY 和原完整 L2。脚本显式读取 UTF8，进度对象转字典使用两个引擎共有接口，同一原子 writer 使用共有三参数 Stream.Write 和 File.Replace；没有第二套 writer、旧数据迁移或重试降级。

完整差异/字节与 SHA 账本为 `diagnostics/project-negative-recorder-review/`。独立审核批准三文件精确内容：environment.md 6242→7035 字节，verify-environment.ps1 11748→11953，test-environment-tty.py 14115→16814。固定 execution/generation 正文、Runner、权限、报告版本与检测标准不变，仅此受测项目增加明确记录入口。真实 PS7 原边界和 PS5 无 pwsh PATH/非 Git/伪工具零调用/UTF8 原身份及 previous/未知发布拒绝测试共6方法通过；原日志在项目 clone `.steerrunner/artifacts/ps51-negative-fixed-cb79d5adfbbe4eb582f956d9834b3b15/test.log`。首次 Replace 的 `$null` 参数绑定失败保留于 `ps51-negative-22f7eab08ee347848184f07f029ae158`，修正为 NullString 后补测，不改写失败。

固定项目提交868b2e7的原 L1 五项/L2 一项完整通过、零跳过、干净来源，报告 `.steerrunner/artifacts/ps51-negative-6b4542b2c3de4aadaa988b4aa04e7b0f-l1`、`-l2`。这些是项目修复回归，不冒充目标 Agent 内执行过 L2。

最终原输入清单 `diagnostics/inputs-negative-ec0d61cb34924237bc0fff7fe28fc4e8/manifest.json`；正式报告 `test-results/L4/negative-complete-7808a4b72da44217a19a48a9589b0ee7/`，场景仍为 testing.project-environment-local、原入口/参数形式不变。单次原活动 `6fd437d9f62541b494fcb60e77b78407` 自动走完三轮；固定夹具 `eda830b05b2398f0a7fb8a5a2fac373814975d0c`，不是三次手工重跑或人工取消。

| 轮次 / 原 Run | 原 result 字节数 | 原 result SHA256 |
| --- | --- | --- |
| 1 / `93d3e342970be4f08e0268be4c72600a` | 1224 | `61be8aafb355469cbc9a0d09fcb39c8721f25e4eaf5ad65330db73d18d0ee9d4` |
| 2 / `40009db504415096e1d74789ce460eb3` | 2840 | `1ca82d22a4468b9a8a38d0cbf28a59e8534f1c33f8445b46a7cc0a54c84633fb` |
| 3 / `35042f98f635a8fb8cfafd8a31cdc40e` | 2844 | `f065d69f4e4a56f12fdd723d02a248ee5118bb5fd520f659bce62e9d15a1ecb2` |

三轮均实际调用最小 TTY 入口并观察到 pwsh 不可解析，随后仅调用同一项目失败记录入口，未修工具或 PATH、未重跑探测、未执行 prepare/L1/L2/发布脚本。第二、三轮通用结果整理曾返回 done，通用工作流因此发出 publish 提示；Agent 读取原负报告后拒绝实际发布脚本，不将收到提示描述为执行过发布。原报告均 completed=true、terminal.tty=failed，其余5项 check 及 demo.l2.smoke 均 incomplete；PR 字段为空、outcome_unknown=false、cleanup_completed=true，不新增 passed。

逐轮原 result 均 closed、Run stopped、round settled=true、timed_out=false。raw 正文与 typed report 一致，原初始身份及冻结 previous 逐值保留；本次派生逐字节核验记录为 `diagnostics/negative-flow-verification-20261031.json`。最终 environment-proof/environment-latest 为 failed/not_ready/settled。provider-pr-proof 核实无 PR；admission-proof 核实准备前后均 environment_not_ready 拒绝、没有准入 Run。308-runner-cleanup、remote-ref-cleanup、orca-local-registration-cleanup 全部 confirmed=true；原三工作区一次删除后异步结算、仅本次 setup 撤销。撤销登记阶段不删源文件，随后场景正常清理自有 scratch；两个用户既有登记保留。scenario cleanup_complete=true、cleanup_errors=null。

原 summary/managed/scenario 仍 passed=false、命令 exit1，代表正向环境资格未通过；按用户确认的口径，本次完整真实负报告/停止/门禁/清理满足负向检测流程验收。结果封存保证不扩大到全部进度快照：最终 proof 中第二、三轮缓存 progress 状态仍为 observing，字段值均 false；报告 result 的 closed 与原 Run 停止单独核实。原生正向完整 L2/发布/ready、其他项目环境和未来 Runner 版本不继承本次结果。全程未部署安装环境。

最终独立审阅者 `/root/local_orca_registration_review` 已核对三轮原正文/冻结输入/SHA、实际终端、原停止/无PR/派发拒绝、异步工作区清理及五份验收表述，批准本机Orca有效not_ready闭环。补充只读 `diagnostics/final-ref-absence.json` 核实全部五条精确自有远端ref已不存在；不覆盖正式报告。
