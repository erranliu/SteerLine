# 本机 Orca 环境检测续验

2026-10-31，用户要求先不改 Runner，准备本机 Orca 配置并实际验收；不部署安装环境。本轮尚未通过环境检测验收，不能登记 ready，也不能借用本机 Codex 或远端 Orca 的签收。

## 固定输入与配置

证据根为 `C:/SteerlineDebug/wt-5069-local-orca-environment/`，以下现场路径相对此根。正式复测固定 SteerLine 干净提交 `0e6e433115ae875528d5b7f6ca993a6e4128108c`、SteerlineTest 材料 `b012d0ae4ade12a41dc47cf8571b45a42fc6a968`。本机 profile 为 local / orca-terminal / full_access / gpt-6-luna medium / development 2，每轮 1200 秒，明确配置既有 `runtime_executable` 与 SHA。输入清单为 `diagnostics/inputs-matched-86839340fb9348458dc1d4f73398f956/manifest.json`。

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

## 待应用的配置与未验边界

主机存在真实 PowerShell 7.6.9，原 Agent 工具调用观察到 pwsh 不可解析。PATH 继承与 WindowsApps 别名过滤是原因推断；本轮未采集实际 Agent PATH，也未以 cf29 源码冒充复用 helper 的原实现证明。当前准备了一份独立 Orca 启动配置，位于 `diagnostics/orca-launch-configuration-6cd53f7dd5894a6e95285f50fec1b020/`。配置固定真实二进制及 SHA，脚本只调整 Orca 进程及后代 PATH，启动后恢复调用方 PATH；拒绝在已有 Orca 进程时运行，不自行结束其他会话，不永久修改用户 PATH。

`validation.json` 记录真实二进制启动及脚本解析通过，未保存版本输出；尚未应用启动配置或重启 Orca。用户随后明确有效 not_ready 也可完成流程验收，因此不为达到 ready 而重启 Orca，启动配置只作可选准备。当前正修复项目端 pwsh 缺失时的负报告收尾，再以新独立现场核验完整负向闭环；不把 completed=false 算通过。当前证据只证明既有 local runtime 配置可进入真实 Agent 工具执行，不证明环境 ready，也不证明必须修改 Runner。
