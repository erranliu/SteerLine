# L3 核心断连恢复

日期：2026-10-12。审计后首轮范围，新增场景已实现/注册并分别通过；实时证据与失败修正记录见验收。原六条 controlled/三条 live 全面草案由以下三组 controlled/两条 live 替代，不新增 suite 或 roadmap Task。

[Spec](../../../../docs/spec/worker-disconnect-recovery.spec.md) · [Plan / 测试计划](../../../../docs/spec/worker-disconnect-recovery.plan.md#测试计划) · [验收项](../../../../docs/acceptance/worker-disconnect-recovery.md#自动验收项) · [E2E 约定](../../README.md)

## 当前入口与规划边界

原有 recovery.waiting-merge-restart 与 recovery.local-codex 主要验证等待合并后的重启，不能代表本轮活动断连/原进程退出恢复。新增 ID 已有顶层测试、驱动并注册 suites.json；运行时仍须通过枚举验证，不使用 skip 占位。

正式隔离服务/Git/SQLite/answer 贯穿受控场景，fake 只模拟原生公开边界。不直接改业务 SQL；通过条件等待/调用计数，不靠猜测 sleep。细密的协议/事务变体放定向 Go 测试，L3 只保留代表链。

## Controlled 场景

| 代号 / 场景 ID / 顶层测试 | 最小步骤与必要分支 | 通过条件 / 验收 |
| --- | --- | --- |
| C1 / recovery.observation-reconnect / TestObservationReconnect | Codex helper 原活动轮次断开观察后重接；原成功结果离线未读、原 report 明确 failed无final、成功缺final各一分支。Orca local/environment fake 在中心退出后按原输入完成，重开原 terminal，核对已保存 working→done 与普通回复。Windows ConPTY 在有明确run目标的控制台发送u、一次读取错误后退出 | 原输入发送计数不增、原结果一次结算；失败有正式Decision，成功缺final仍未核实；轮次结束与任务成功区分；u前后无Command/Attempt/控制写且无panic。CR-A01/A02/A05/A06 |
| C2 / recovery.codex-resume / TestCodexResume | 102型代表链：旧implement失败，精确owned app-server退出；同thread模拟人工新提交；公开核实原thread/cwd，正式continue，同thread新Attempt进入implement核验并生成新report source，再正常发布/交付。独立report-resume分支使用source 成功但已发送 report 失败 | 不把人工final写成旧结果；新实施核验成果，旧失败保留，新结果来源完整；已发送失败报告重新 implement；仅未发送首次报告可沿用 source。重复答案与已存结果重开由C3/R1覆盖。CR-A03/A04 |
| C3 / recovery.resume-gates / TestResumeGates | Codex会话丢失、旧写者未停止、冻结身份不符各一拒绝；原会话人工新turn活动/空闲未知、正式continue重复提交与过期版本拒绝。已授权新输入被helper接受但丢回执后退出/重开，只对账原请求。Orca local/environment错误target/terminal代次、terminal丢失与output source/gap各一拒绝 | 拒绝分支新增输入为零、不替代原session/terminal、不补Enter；重复答案及未知发送重启不增加业务发送，原请求核实后最多一次结果。CR-A03/A05 |

C1 每种后端/目标保留一个核心重接链，不做所有窗口的笛卡尔积。C2/C3 分支共用 helper/隔离夹具构建能力。必要分支以 branches/<name>/result.json 记录 adapter/target/fault、identity、发送次数、expected/actual与cleanup；单个顶层PASS不能掩盖缺分支。未知回执不能公开查询时，应明确阻断且零重发，不要求 fake 虚构可用的查询API。

CR-A07 不新增顶层场景：C1 验证两路径接回事实经共同 worker 分类、同事实不反复创建 Decision；C2 验证 Codex 重建沿既有 Attempt/Command/结果事务；C3 验证 Orca 丢失原终端与不支持资格被共同门槛拒绝。C1 中已停止的原 Orca 终端还需正式 continue，沿用原 terminal/Attempt 发一个新受管 Command，不能因 ManagedProcess=false 误走进程重建。非平台原资源复用/拒绝、迟到资格与只读装配由 Plan B1 定向 fixture 和已有架构检查覆盖；L3 正常终态本身不证明代码解耦。

## Live 场景

| 代号 / 场景 ID / 顶层测试 / chain | 最小真实链与通过条件 |
| --- | --- |
| V1 / recovery.codex-resume-live / TestCodexResumeLive / local-codex | 合成任务自身的只读等待窗口中停止本次owned app-server，重开中心核实原thread，正式continue后新Attempt仍同thread/worktree。受管implement处理当前成果、report来源正确，真实final/正数usage、正常PR/合并/唯一Delivery与清理。只做一次代表进程故障；人工未绑定成果由C2覆盖。 |

V2 的 chain 需新增当前单终端产品装配/前置/证据/清理，不能复活旧 orca-codex/supervised 配置或仅注册测试名。沿 testenv/native/driver 只补本机链所需路径，远端manifest与真实断连链暂不扩展。

Live 不伪造final/SL_RESULT，不改中心状态。合成任务等待是自身业务步骤，不能由服务补发提示触发故障。故障只影响本次owned资源，不操作全机网络/用户终端。Orca usage未公开记unavailable；零重发由输入计数和绑定证明，不能靠离线期token不增。跨中心实例的新业务阶段遵守正式继续授权，补读证据本身不发送业务输入。

## 规划命令

以下命令按组选择已注册入口；每个独立顶层测试仍为 5 分钟业务期限。每次使用runner默认新证据目录或空-out；Windows L3串行，不混Go原始参数/L2/Full。

```powershell
# C1
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.observation-reconnect,recovery.failure-results,recovery.missing-successful-result,recovery.terminal-local-reconnect,recovery.terminal-environment-reconnect'
# C2
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.codex-resume,recovery.codex-report-resume,recovery.report-source-changed'
# C3
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.resume-gates,recovery.answer-replay,recovery.terminal-local-unknown-send,recovery.terminal-environment-unknown-send,recovery.terminal-resource-gates,recovery.terminal-target-gate,recovery.terminal-incarnation-gate,recovery.terminal-gap-gate'
# V1
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.codex-resume-live'
# V2
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-local-reconnect-live'
# R1：现有必要回归，已注册
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart'
```

## 驱动、时限与证据

复用现有隔离夹具、正式answer、owned资源与证据框架。需补CrashService（现有RestartService为Stop+Start）、精确app-server退出、当前单终端fake、helper线程/轮次及发送计数、一次原生断读/丢回执、ConPTY的u与q输入。不建设通用存储commit barrier或全部外部写操作故障平台；不得为故障注入增加产品环境变量后门。

controlled每场景默认5分钟、live每场景10分钟业务期限，另有有界owned清理。若必要分支实测不能稳定在限期内完成，拆场景并同步映射，不默默扩大上限。失败/清理失败/必需前置缺失如实报告，不skip为通过。

V1预检公开原thread读取/resume、配置/权限；V2预检当前单终端read/send/target设置与工作区信任。所有PR只操作erranliu/SteerlineTest本次专属base/head。证据保存源码/dirty/exe/CLI版本、原/新身份、Decision/Command/Operation与版本、原生业务发送计数、final/source、PR/SHA/Delivery及清理；验收文档逐项登记实际路径。

本轮不验证真实远端主机重启/断电、Orca替代terminal恢复、首次准备/原生问答/发布的全窗口崩溃。既有未知操作守卫仍必须成立，C3验证代表拒绝路径；远端产品规则由environment fake覆盖，不能写成远端live已验。

## 实测后拆分的独立入口

观察重接单现场实测 215.39 秒通过；多现场共享原 5 分钟期限会超时，已依 Plan 拆分而未提高时限。C2 的答案重放检查拆到 C3 的独立入口，让 C2 保留正常交付。四种 terminal 拒绝分别使用独立现场，避免连续四次启动共用一个期限或复用上一故障状态。C2 的失败实施恢复、未变 report source、变化 report source 分别验证；变化分支复用活着的原资源，无额外故障。三组仍只覆盖原定代表故障，不做全排列。当前受控 16 个入口、live 2 个入口；可从组命令中选择任意单个 ID。

| 组 | 场景 ID 后缀 / 顶层测试 | 现场 |
| --- | --- | --- |
| C1 | observation-reconnect / TestObservationReconnect | Codex 断观察后补读原成功结果 |
| C1 | failure-results / TestFailureResults | report 明确失败无 final，u 正常/读取错误 |
| C1 | missing-successful-result / TestMissingSuccessfulResult | 原成功缺 final 仍未核实 |
| C1 | terminal-local-reconnect / TestTerminalLocalReconnect | 原本机终端离线完成后重接及正式继续 |
| C1 | terminal-environment-reconnect / TestTerminalEnvironmentReconnect | 同规则下的 environment 目标 |
| C2 | codex-resume / TestCodexResume | 人工新提交、原进程死亡、同 thread 新实施核验 |
| C2 | codex-report-resume / TestCodexReportResume | 原 source 成功，失败 report 同 thread 重新实施 |
| C2 | report-source-changed / TestChangedReportSourceReverification | 原资源仍可用，工作区变化后新实施与 report source |
| C3 | resume-gates / TestResumeGates | 原 thread 丢失拒绝替代 |
| C3 | answer-replay / TestRecoveryAnswerReplay | 新输入接受后重复原答案/过期不同答案，零重复输入 |
| C3 | terminal-local-unknown-send / TestTerminalLocalUnknownSend | 已接受丢回执，原接受/开始不明则阻断且零重发 |
| C3 | terminal-environment-unknown-send / TestTerminalEnvironmentUnknownSend | environment 同一未知请求阻断且零重发 |
| C3 | terminal-resource-gates / TestTerminalResourceGates | 原 terminal 丢失，零替代/重发 |
| C3 | terminal-target-gate / TestTerminalTargetGate | 错误原 runtime，零替代/重发 |
| C3 | terminal-incarnation-gate / TestTerminalIncarnationGate | 原 terminal 代次变化，零替代/重发 |
| C3 | terminal-gap-gate / TestTerminalGapGate | 原生状态订阅缺口，零替代/重发 |

Codex 旧写者、活动人工 turn/空闲未知、配置身份与迟到版本守卫另在 B1 的实际 worker/adapter 定向测试留证；C3 answer-replay 通过正式 API 检查重复与过期答案。验收必须分别登记，不能因表中某一代表链通过就推断其他条件已验。

## PR #295/#296 整合说明

2026-10-13：development/v3 不恢复成果快照/候选 SHA；已发送失败报告重新实施核验。Orca controlled 提供公开加密状态订阅和 main-agent 查询，普通屏幕只核对回复与 composer；缺少原接收/开始证明的 unknown-send 保持阻断，不从正文补造回执。新轮次观察重接之前须保存原 working 证据，结果保存后的原终端丢失不会抹去中心结果。2026-10-12 的全组及 live 证据属于整合前版本，当前最小重验与限制见专属验收。

2026-10-14：本机 Orca 部署已停用，V2 本机 live 入口与登记删除。底层隔离 local/environment 夹具保留；远端真实恢复当前未登记，不计为已验。
