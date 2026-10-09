# 本机 Orca 原生通用 worker 验收

只归属本聊天的 Orca 接入直接调整，历史预留 ID 86 已撤回 roadmap，保留编号不复用。依据 [Spec](../spec/orca-native-worker.spec.md) / [Plan](../spec/orca-native-worker.plan.md)；实施与验收在此记录，不成为 #84 的前置或执行队列成员。原始日志放独立临时证据目录。

| ID | 方法 / 步骤 | 通过条件 | 时点与阻塞范围 | 状态 / 证据 |
| --- | --- | --- | --- | --- |
| ONW-A1 | 自动：生产依赖边界、通用配置两 agent、旧配置拒绝、本地策略定向 | Orca 路径无特定 agent 实现；配置值决定原生选择；旧格式在副作用前拒绝，本地 Codex 保持 | 实施后；阻塞本任务交付 | 已通过生产边界、配置、本地策略定向；最终复审见下方记录 |
| ONW-A2 | 自动：受控两轮原生 worker-start、输入/反馈/输出/结算、身份变化/未知回执/恢复/资源；不支持的 structured 类型与部分效果；worker-stop/release 资格及拒绝/未知 | 同一原 worker，明确终态；未知不重发，旧结果不串线；不支持类型阻断后续，首轮输入/效果如实保留；无专属探针/relay，清理有原生证明 | 实施后；阻塞本任务交付 | 受控自动验证通过；真实模型闭环单列 A3 |
| ONW-A3 | 自动：隔离 debug runtime、真实 Orca原生 launcher、工作区读写操作、两轮原会话反馈与原生完成、输出、精确清理；环境先准备 Orca Terminal + YOLO | 权限由 Orca 原生设置持有；模型真实执行；各身份与结果可核对；资源停止清理完成 | 驱动/场景落实后；Terminal模式与权限准备是前置，缺失为真实阻塞，不能用M1后置豁免 | 失败：Orca1.4.209 原生 agent_readiness/terminal_handle_stale；opt-in driver 已落实，不算已跑通 |
| ONW-M1 | 人工：新安装构建启动，确认权限来源提示、待处理反馈和 Orca 原生界面 | 可辨识权限由 Orca 管理，无 Codex专属权限配置误导 | 交付后集中；用户参与，窗口待安排；不阻塞无关研发 | 待验收 |

2026-10-09 规划：用户确认先用宽松权限跑通，agent只为配置字段。已读取当前 Orca 原生启动代码，确认其统一 Agent Permissions / YOLO及默认 launcher 参数映射存在；这是能力定位，尚非本任务产品自动通过证据。C:/SteerlineRuntime 的历史102只作设计输入，不修改其任务、配置或冻结执行。

本轮仅调整规划、未启动实现。独立只读复审 `/root/review_orca_plan` 确认安装版 Orca 的 structured worker 无已核实的同会话续轮入口；已将本轮范围明确为 Terminal模式，并补齐首次参数/单次启动、worker资源清理、测试包命令与清单顺序。此复审不构成 ONW-A1–A3 已验收。

## 本轮文档验证

2026-10-09：安装 Orca CLI 版本 `1.4.209`；其 `out/shared/tui-agent-permissions.js` SHA-256 为 `af431d7d001695dee57537308ded5d0bb3c2630aee2779515450e9d7572411c4`。原生 launcher 权限设置、worker-start 参数及 structured 续轮限制按该安装版本只读定位，不宣称其他版本支持。

独立复审最终通过，无新的实质文档问题；组合启动后的首轮输入/效果按事实保留，不再承诺不支持类型一定零执行。`python scripts/roadmap_task_id.py check` 通过（86身份文件、11草稿任务行、5正式任务行）；`python scripts/check_doc_links.py` 通过（274文件、1764本地链接、零错误）；`git diff --check` 通过。本轮未运行产品自动验收或模型；ONW-A1–A3 与 ONW-M1 均保持待验收。


2026-10-09 用户改为直接实现：已从 execution-map / roadmap 草稿移出历史 #86，恢复 #84 在本分支的原 Spec/Plan，不创建新聊天、不操作其在途工作区。此前“本轮仅规划”记录保留为历史，当前范围以本段和直接调整 Spec/Plan 为准。

## 2026-10-09 当前聊天直接实现与验证

实现：本机 Orca 移除特定 agent 启动/版本/工具/TUI/history 依赖、沙箱探针及 IPC relay，只接受 Orca codec。agent/model/effort 为配置；权限由 Orca 原生 Agent Permissions 持有。工作区先固定，再由 native worker-start 单次启动；续轮仅原 terminal。应用通过通用显式输出材料与原 Dispatch 真实结算冻结完整 final，释放后仍可读取；没有材料时报明确诊断。注册目录的 HostManagedPermissions 使正式服务恢复不强制客户端权限根，其他路径保留原策略。停止/释放回执 unknown 核原持久资源事实；未决启动不能以无 handle 证明 absence。

验证来源：base HEAD `059f94b7` 加本次工作区修改；各测试在未提交候选上执行，实际通过范围如下，不把 skip 或受控 fixture 当模型执行：

- `go test ./internal/execution/adapters/orca -count=1` 通过，包含通用两 agent、原启动/续轮、unsupported 部分副作用、unknown/释放归属、停止旧 Dispatch、输出完整/原身份/冻结/防篡改、无回执启动不能证 absent、缺输出诊断；live opt-in 默认 skip 不计 A3。
- `go test ./internal/app -run 'Test(.*Orca|.*Profile|.*Configuration|.*Global|PS2)' -count=1` 通过（49.536 秒）；本地策略 `TestPS2NativeLaunchAndInputUseAutomaticReview` 通过。
- `go run ./cmd/test-feedback -level=L1` 通过（85.7 秒，3 workers）；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T052509.966966900Z/summary.json`。此后停止恢复接线另以定向验证记录，不将早期 L1 说成最终全量。
- `go test ./tests/integration -run '^TestOriginalTerminalReconnectAndFeedback$' -count=1 -v` 通过（71.14 秒）；正式服务首轮、停服重连、原问题反馈和 frozen 配置保持。
- `go test ./tests/integration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1 -v` 通过（88.43 秒）；实际运行多轮、派发/反馈/Task 回执丢失恢复与 incarnation 替换拒绝，没有重新派发未知原输入。fixture 使用新 native protocol 与显式材料，已移除模型准备轮，保留有效身份和恢复断言。
- `go test -tags ps2native ./internal/app -run '^$'` 编译通过，旧 tagged driver 收敛为 local 权限验收，Orca真实闭环使用通用 `TestOrcaNativeWorkerSmoke`。

早期受控集成失败原始原因保留：旧 fixture 强制 HostEnvironmentPort 后一直 pending；移除该假前置后发现核心对所有 bound session 强制 PolicyResources，已按目录宿主管理声明修复。旧 named L1 入口指向退役专属测试导致首轮失败，已改为实际通用断言；不把这些失败记录为通过。

### ONW-A3 实际原生启动失败

隔离无远端 Git repository / evidence 根：`C:/Users/lmas1/AppData/Local/Temp/steerline-orca-native-20261009-124146632`。以 Orca 原生 CLI 1.4.209、Terminal模式运行 opt-in `TestOrcaNativeWorkerSmoke`，evidence2–5 保留每次 native receipts。

第一次原启动在升级菜单超时；后续出现 shared background daemon feature incompatibility 菜单。未安装升级或重启共享后台（避免影响 #84）；仅隔离测试界面选择“本次不使用 daemon”并跳过升级。最后界面已显示 YOLO 和正常输入，但原 worker-start 仍返回 agent_readiness timeout / terminal_handle_stale，业务输入未核实送达，两轮真实闭环未通过。此失败属于原生 launcher readiness，不能由单元或受控集成豁免，也不添加特定 agent workaround。

四个原 Dispatch `ctx_88746782b381`、`ctx_eb4bbf59f62a`、`ctx_ce93f0498015`、`ctx_3bf05236dd34` 均通过 native worker-show 核对 state=failed、terminalResource.releaseState=released、owner 为原 Dispatch；工作区枚举仅剩自有普通 fallback shell。已通过 native worktree rm 精确移除四个隔离 worktree，main terminal 枚举为空，并移除自有项目 host setup 注册。保留临时根和原始回执供排障；不改 `C:/SteerlineRuntime`、102 冻结执行、#84 在途工作区或共享后台配置。使用 [orchestration 技能](C:/Users/lmas1/.agents/skills/orchestration/SKILL.md) 执行原生观察与生命周期验证。

A3 保持失败/环境受阻，M1 保持未验。本变更不加入 roadmap、不阻塞 #84；源码实现与原生 live 通过是不同事实。

### 最终停止恢复与复审

最终新增正式服务定向 `go test ./internal/execution -run 'Test(HostManagedStopRecovery|ExecutionStopIntent|PS3StopReceipt|RecoveryCannotSelectReplacementSession)' -count=1` 通过（50.749 秒）。HostManagedStopRecovery 覆盖同步确认与后台对账两条正式入口：native mutation 一次、回复 unknown 保留、持久停止证明出现后才确认，仍 live 的普通 OS 身份不能代替原 native worker 证明。Orca 单元再次通过（2.595 秒），包含当前 Dispatch 变化时仍读取保存停止操作的原 Dispatch；本地权限策略再次通过（0.813 秒），`go build ./internal/app` 通过。

独立只读 reviewer `/root/review_orca_code` 多轮检查并确认全部必须修复项已处理，最终未发现新必须修复问题；原停止对账文件 SHA-256 `b1be248641b9009605b68e1fdc37bf510dbd14f844c287b5b47fb710e4a7d6f4`。审核确认源码正确性不替代 A3 真实模型闭环。A1 / A2 以受控及正式服务已测范围通过，A3 继续保持环境受阻，M1 未验。

`python scripts/check_doc_links.py` 通过（274文件、1751本地链接），`python scripts/roadmap_task_id.py check` 通过（86身份文件、10草稿行、4正式行、next88）。旧84文档的修改仅撤回本聊天先前添加的86前置，保持原在途固定契约；未改变84工作区。

## 2026-10-09 用户要求质量审计、直接修复并提 PR

整合最新默认主干 `e20c28ef`，合并 commit `e755e899` 保留 PR #255 的远端 Orca opaque agent、独立 codec/原生资源和 L3 场景。以下修复在该合并后的工作区执行，无外部 worker、安装或共享后台变更：

- 部分启动清理在任何停止/释放前严格核原 runtime/Run/Task/Dispatch/owner/worktree；不可达或身份变化保持 unknown，不执行副作用。持久 stopped 可继续原 release，不被旧 stop_unknown 回放困住。
- 正式停止保存原生回执；discard 的有/无句柄路径都按原生命周期证明清除实际 StopPending。重启后首次 discard 可以持久化原 Dispatch 停止意图并执行一次；已有未知意图不重发。已结算的持续 worker 通过原 worker-release 收尾，pending/retained 不成为 absence；已结算且 terminal 已退出时不要求虚构 live handle。
- 正常观察与 Done 已保存的恢复分支统一验证完整输出。冻结材料版本2的摘要与 conversation cursor 包含 outcome；只改成功/失败也被拒绝。JSON预算覆盖六倍转义膨胀，合法正文不会在冻结后变成不可读。
- 坏 binding 或未知 Attempt 只产生可信所属执行的局部 gap，不阻断健康来源；已知历史 Attempt 保留原身份和 Ended，取消立即停止。
- 运行期间的原生 agentWait 经生命周期桥接保存宿主等待事实，不生成业务问题。缺失字段与显式 null 区分；未知保留旧 wait，发生序号在原 turn 持久化，相同等待再次发生不被去重。
- native worker-start 的 CLI 传输预算至少覆盖原生 readiness 等待和30秒回执余量，仍使用同一保存 UUID。产品会话设计文档已纠正专属历史读取描述。

本轮验证（base `e755e899` 加上述未提交修复，证据不冒称 clean-source/live）：

| 范围 | 实际命令 / 结果 |
| --- | --- |
| Orca adapter及审计回归 | `go test ./internal/execution/adapters/orca -count=1` 通过（最终6.775秒）；live opt-in默认skip，不计A3 |
| 正式停止/丢弃 | `go test ./internal/execution -run '^TestHostManagedStopRecoveryUsesOriginalLifecycleProof$' -count=1 -v` sync/background/first-discard 三场景通过（29.59秒），实际StopPending清除且一次原stop、零通用Kill |
| 等待桥接与原停止边界 | `go test ./internal/execution -run 'Test(LifecycleHostWait|HostInputWait|HostManagedStopRecovery|ExecutionStopIntent|PS3StopReceipt)' -count=1` 通过（40.711秒；随后新增first-discard另行通过） |
| 配置 / 权限 / 冻结 | `go test ./internal/app -run 'Test(.*Orca|.*Profile|.*Configuration|.*Global|PS2)' -count=1` 通过（52.337秒） |
| 原会话多轮恢复 | `go test ./tests/integration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1 -v` 通过（141.15秒）；随后原生stop和等待边界由上述专属回归验证 |
| L1 | `go run ./cmd/test-feedback -level=L1` 通过（148.7秒）；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T060246.138759900Z/summary.json`。L1之后结算terminal无需live的最终补丁由adapter最终回归覆盖，不宣称重新跑全量 |
| 主干集成 / local保留 | remote adapter/backend 包通过（2.846/0.638秒）；`go test -tags ps2native ./internal/app -run '^$'` 编译通过；local原生权限回归通过（0.867秒），app构建通过 |

独立只读审计者 `/root/review_orca_code` 与 `/root/review_orca_plan` 提出的全部已确认问题已修复并复核。最终专项复核未发现新必须修复问题；所审停止实现 SHA-256 `62f6910f027fea6f670e7167585a444f2146bff031e60c51a1f3349fe78a405d`。原始首轮回归失败属于新增断言揭示的旧缺口或fixture缺乏正确原生资源回执，均修后复验，不把失败当通过。

ONW-A1/A2 保持上述自动范围通过；ONW-A3 仍为前文 Orca1.4.209 launcher readiness 真实环境阻塞，当前审计不运行模型、不宣称真机闭环通过。ONW-M1 未验。PR用于代码审阅，尚未合并或部署；历史86不进入 roadmap、不阻塞84。
