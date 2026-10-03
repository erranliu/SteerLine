# #67 主包隔离守卫与测试归位：受限执行交接

记录日期：2026-10-02。依据 [Spec](../../spec/core-decoupling-06-boundary.spec.md)、[Plan](../../spec/core-decoupling-06-boundary.plan.md)、[共同约束](../../spec/core-decoupling-constraints.md)。

**状态：#67 未完成，不能合并；仍需完成 root 测试归位、quick 接入、完整映射与最终组合验收。第三中性实现的原输入/重启人工确认/唯一交付及资源结算闭环已在协调的完全访问环境定向通过，但不代替 #67 整体交付。**

## 协调接管的阶段记录

2026-10-02 22:57 UTC：执行 turn 因上下文窗口耗尽失败，平台已确认停止；协调在同一 f775 工作区接管，保留全部修改，没有替换执行聊天或增加并发写者。执行者未完成的迁移保存为本地阶段 commit `604cbc0`。本节之后的首次工作区身份、源码摘要和权限限制均为历史快照，不代表当前候选。

- 原生注册 agent 的冻结测试迁到应用包，使用真实目录；通用“两种显式 agent 均保持原固定选择”断言由主包单一中性实现承接。真实目录不注册的 agent 仍拒绝，不宣称扩大原生支持。两个定向命名用例通过。
- 第三实现先绑定最终隔离 provider，再通过公开入口建立任务权威，修复原 fixture 更改仓库身份后的 `authority_mismatch`。
- 创建会话回执补齐版本化原资源绑定封套，保存原 session、generation、process 和 target；重启观察验证封套与实际固定身份，保留 SQLite 一致性校验。缺失封套曾导致真实交付事务报 `contract_mismatch`，已修复 fixture。
- 合并后经公开任务详情查询核对关闭和权威交付，保留原 selected workflow、PR 和 merge commit、仅一次输入/答案、唯一 TaskDelivery 与仅一次资源结算断言。
- 定向报告：`C:/Users/lmas1/.codex/task-state/roadmap-runner/task67-third-original-binding/summary.json`，2 个 parent/subtest 全通过、0 fail/skip、complete，29.650 秒。测试基于 dirty `604cbc0` 加上述修复；不是完整 #67 的已审核 head。此前失败报告同目录 `task67-third-positive-parent604cbc0`、`task67-third-provider-bound`、`task67-third-task-detail`、`task67-third-delivery-diagnostic`、`task67-third-delivery-error` 保留，不豁免失败。

当前已获用户授权在 #67 真实交付后继续 #68–#72；下述首次限制“不启动 #68”表示前置未交付前不启动，不表示新的总范围止于 #67。

### 原生 backend 与 host-wait 归位阶段

- Terminal exit/固定 incarnation/连接、停止未知结果、不同资源停止不受慢观察阻塞、同资源观察不重叠共 6 个原断言移至 `orcaworker/terminal_backend_cd67_test.go`，通过实际实现 backend。协议 fixture 只拥有隔离终端文件，不含中心 SQLite 写者；无消费者的旧“破坏 stop 回执”数据库写 hook 已删除。
- 真实 host-wait 探测保留 `STEERLINE_ORCA_HOST_WAIT_PROBE` 原 opt-in，归实现包并调用实际 conversation mapper；中心持久化/去重/不产生 business Decision 的断言归 `host_wait_business_test.go`，使用中性 source port。Live 开关未启用，不宣称原生认证。
- 原生 preflight、两条默认 backend 的静态能力声明、真实进程 PID 创建身份拒绝归应用包，通过实际内置目录与 process host；中心原 agent 能力证据作用域及绑定/操作记录断言使用单一中性 backend。
- 阶段 JSON 报告均 complete：`task67-terminal-backend-moved` 6/6、`task67-app-preflight-backends-fixed` 3/3、`task67-neutral-capability-scope` 4/4，位于协调的 `C:/Users/lmas1/.codex/task-state/roadmap-runner/`。这些是定向归位检查，不是 #67 最终 quick 验收。
- 修复应用 fixture 的默认路径隔离：`nativeTaskFixture` 设置临时 `STEERLINE_HOME`，应用 TestMain 的默认配置/状态也归生成的隔离 home；当前格式的 Git/时间预算/任务类型由 fixture 显式提供。首次未隔离调用曾改写 `C:/SteerlineRuntime/execution.json`，已通知用户并在 run 状态记录；现有历史备份格式过旧且未证实与原文件相同，原配置恢复等待准确来源，不能以阶段代码通过掩盖此问题。

当前仍需处理其余 21 个 root 平台文本文件、最终工厂/dispatcher 删除、完整映射、quick 与完整复审；不提前合并或启动 #68。

### 托管 worktree 续做阶段（2026-10-03）

执行目录为 `C:/Users/lmas1/.codex/worktrees/2829/SteerLine`，分支绑定为 `codex/67-core-boundary`，续做起点 `7e2cb9a`。以下仅为阶段结果，#67 仍未完成：

- 中心发布凭据过滤直接验证 `workerProcessEnvironment`，固定配置摘要与版本拒绝使用中性目录。
- 首次反馈的三种形式继续保留原全文、唯一会话、部分提交 HEAD、固定身份、未知发送不重发断言；由已有单一中性生命周期端口实际接收输入，持久完成回执绑定原 process/target/execution/attempt/generation，并只读对账。
- 删除无消费者的复制目录替换、原生配置/恢复/身份 helper；原生产断言没有删除。
- 根包 TestMain 为正常测试运行设置独立临时 `STEERLINE_HOME`；协议子进程仍继承其测试父进程环境。
- 定向报告位于本 worktree 忽略目录 `bin/cd67/`：`neutral-service` 中发布两条通过、首次反馈失败；`neutral-service-repaired` 为编译诊断失败；`neutral-service-repaired2` 保留缺失接收输入产物的失败；修复后 `neutral-service-single` 2/2 pass、`neutral-service-other-cases` 5/5 pass（余下两种反馈、事务 owner、旧服务答案）；补齐完成回执身份后 `neutral-service-fixed-identity` 2/2 pass。通过报告均 complete、0 fail/skip；是阶段 dirty 候选，不能作为最终 #67 head 证明。

- 第二个续做阶段删除 root remote 复制工厂、codec 输入/helper 和无消费者 dispatcher；原远端断言仍归 `orca_remote_worker` 与外层协议 fixture。当前实时 root 平台文件为 18 个，仍需归位。
- `quick` 默认接入外层两个 #67 入口；显式指定同一集成包时复用该次执行，核对父用例、七个负向守卫样本、原输入/两条支持路径/remote Protocol/真实 Claim 拒绝子用例各一次 run/terminal，缺失或重复均拒绝。没有运行最终 quick。
- 守卫除 `.git` 元数据外不按目录名排除源码；顶层 `bin` helper 的额外标签传递依赖拒绝样本通过。报告 `bin/cd67/guard-no-directory-exclusions/summary.json` complete/pass，包含 parent 与全部七个负向样本。
- 真实公开 Claim 的 remote 强制准入拒绝已纳入 `TestCD67AdditionalImplementation`；定向 `bin/cd67/remote-claim-required/summary.json` 2/2 pass/complete，不调用真实执行工具，也不认证权限。根包仅编译通过；`cmd/test-feedback` 缺失/重复/截断/必需子用例覆盖定向测试通过。

- 原权限工具 native/live 两个入口归应用包，通过实际目录、资源 codec/命令及生命周期构造。原宿主读取、workspace 读写删除、Git index/ref/reflog、nonce、原终端身份、工具 marker/native shell proof、只读收集和清理断言保留；接受报告 DTO 固定 Project/Execution/Attempt/generation/config，未获得中心写者。
- 可选 local capability 保留原两个 opt-in 和 `local_process` 原证据范围，应用真实 process host/backend 仅核对固定物理进程/停止，raw JSONL 保留与 1 MiB 读取上限不变。旧 root spawner、未消费 marker 分支和原生 Backend 构造 helper 删除；不宣称这些旧 native exec 证据认证当前 app-server 权限。
- 两条超长原生 exec 流断言归 `codexworker/exec_stream_cd67_test.go`，test-only 字节流 fixture 调用实际 `DecodeExecLine`/normalizer；`bin/cd67/native-exec-stream-moved/summary.json` 2/2 pass/complete。根包、应用包均仅编译通过。没有启用三条可选 live 入口；当前仍有 13 个 root 平台文件。

- 服务 host-exit/两任务隔离/人工后继代次的四条 live 入口归 `steerlineintegration/worker_recovery_live_cd67_test.go`。保留原 opt-in、gpt-6-luna/low、45 秒命令和各入口时间预算，以及原问题/thread/turn/workspace/process、仅人工建立新 Attempt 的断言。A/B、READY、ACK 原指令成为已提交的正式任务契约，fixture task 8/9 显式映射为外层现有 901/902；输入通过实际 app/Service 受理与公开答案端口。
- 旧 detached survival opt-in 的原失败断言仍保留；当前 managed Job 不增加分离运行或恢复 fallback。这个历史 opt-in 没有运行，不能声称旧 detached 能力可用。四个入口仅编译，root 原私有启动/答案/事务写 fixture 已删除；当前剩余 11 个 root 平台文件。

- 丢失 terminal 创建回执、改名 terminal 后重启的两个原断言经真实 app/公开 Claim/recheck/query 通过：`bin/cd67/native-launch-public-ports-compiled/summary.json` 中两条 pass；同报告的来源测试因等待已结束轮次主动重读而失败，报告 complete/failed，没有豁免。
- 来源替换按职责拆为真实原生只读输出与中心单写者持久化：`TestNativeSourceReplacementRetainsOutputEvidence` 保留原 Dispatch、transcript 内容、完整性、source_changed 和绑定游标；`TestCD64SourceReplacementRetainsEvidence` 用单一中性 HostOutput port 经实际 SQLite 保存同一 Attempt/新游标，且不创建 Decision/Operation。原生第一次读取允许原来的 bounded/incomplete 事实，完整性仍在替换后的原断言核对。
- 报告 `bin/cd67/source-replacement-owners/summary.json` 保留中心通过、原生诊断失败；修复仅新增的过强“第一次必须 complete”要求后，`bin/cd67/native-source-readonly-original/summary.json` 1/1 pass/complete。`native-launch-public-ports` 为最初编译诊断失败。未重跑两条已通过的 launch 用例。
- root 原 native launch/source 消费者及无消费者 helper 删除；生命周期消息工具归平台无关 fixture，CD65 现有入口调用中性持久化断言。根包编译通过；剩余 10 个 root 平台文件。

- 真实 readiness smoke 的 `TestWorkerEnvironmentNativeReadinessLive` 归外层，保留原 REPO/COMMIT/CODEX/STATE_ROOT/REPORT opt-in、任务 9001154、已提交 Spec/Plan blob、4 分钟准备预算及受理后 7 分钟结算预算。当前格式由实际 app loader 受理，公开任务/dispatch/Claim/recheck/query 代替旧 private authority/Service 写入；原 readiness/business、结算、不同 Dispatch、原 terminal/incarnation 断言保留，原隔离资源由 checked native close 清理。
- 根包、集成包仅编译通过；没有执行这条 live 或访问真实 runtime。当前剩余 9 个 root 平台文件。

- 中心 ER1 五种原 readiness 情形归 `readiness_business_test.go`，以已观察的通用 probe/settlement 事实验证真实中心门槛。成功准备之后只提交一个独立业务输入，started 事实到达才增加业务 TurnNumber；tool_failure/oral/missing_receipt/failed_settlement 不派发业务，确认失败仍产生中心 Decision。原生工具块/receipt 解析仍归既有 `orcaworker/readiness_test.go`；默认模型和 approval/sandbox 原命令断言经实际应用 command builder 归 `readiness_command_cd67_test.go`，只去重重复五次的命令检查。
- 同一中性资源封套承接 writable roots 被扩张时的原中心拒绝断言。`neutral-readiness-seeded-workspace` 2/2、`neutral-readiness-other-facts` 5/5、`neutral-readiness-bound-roots` 1/1、`native-readiness-default-command` 1/1 均 complete/pass，位于 `bin/cd67/`；`neutral-readiness-facts` 的初始编译失败和 `neutral-readiness-facts-compiled` 的未正确绑定 workspace 失败保留。工作区之外未写入 runner 状态或真实 runtime。
- 仍需完成剩余原生 readiness/host/restart 消费者、根包服务默认工厂和实际 CLI 消费者归位，删除最后的具体 helper，并完成完整 diff/fixture 映射与规定 quick。当前仍为 9 个 root 平台文件，不能把这些阶段通过视为 #67 验收，尚未推送或提出 PR。

## 工作区和源码身份

- 实际工作区：`C:/Users/lmas1/.codex/worktrees/f775/SteerLine`。
- HEAD / `origin/main`：`a670e9d1510dfdfd04c1aeb032addd23a0d45941`，#66 合并提交；开始时工作区干净。
- detached HEAD；首次创建 `codex/67-core-boundary` 分支被 `C:/Project/SteerLine/.git/refs/heads/codex/67-core-boundary.lock: Permission denied` 阻断。没有再次操作 refs，没有提交、推送或 PR。
- 当前 Go 源码集合 SHA256：`f08ee646fc1b128c9dc214bfa73a4daeb9f74e77657c57e3f3c3126bd908912a`，465 个文件（全部现存项目 Go 文件及 go.mod/go.sum）。算法及逐文件摘要见 [source-manifest.json](source-manifest.json)。这是未提交源码摘要，不是 commit。
- 原始报告、迁移脚本和工作区 Go cache 保存在忽略目录 `bin/cd67/`、`bin/go-cache/`。报告对应上述 Go 源码；此后只追加验收文档，不改变 Go 内容。

## 已实施内容与所有者

`internal/coreboundary/check.go` 位于中心之外；无视构建标签扫描全部直属 Go 文件名/文本（包括 tests），解析全部项目生产源码图并加入 root 测试 import 边，检查中心/execution/contracts 经任意 helper 的传递禁用依赖以及具体实现反向中心依赖。另运行 Windows/Linux 的实际生产 `go list -deps -json`。直属目录无例外名单；版本未知、go list 失败和空扫描不能视作通过。

`TestCD67CoreBoundary` 在外层集成包。5 个负向 fixture 已实际证明大小写关键词、其他 OS 文件名/文本、额外构建标签下直接 app import、helper 传递 integration import、具体实现经 helper 反向 core import 均被拒绝。

6 个旧入口按实际职责迁移，见 [test-mapping.json](test-mapping.json)：

| 旧位置与入口 | 新所有者/位置 | 验证状态 |
| --- | --- | --- |
| activity / TestCodexNormalizer | codexworker/normalizer_cd67_test.go | 原断言 run/pass |
| activity / TestDetectNormalizer | steerlineintegration/normalizer_composition_test.go | 公共 normalizer 组合，原断言 run/pass |
| worker_local / TestWorkerTurnFindsPRBeforeFinalMessage | codexworker/outcome_cd67_test.go | 原 journal/PR 断言 run/pass |
| orca_worker_observation / TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity | orcaworker/identity_cd67_test.go | 原身份断言 run/pass |
| orca_worker_observation / TestFixedOrcaAgentTerminalRejectsReplacedIncarnation | orcaworker/identity_cd67_test.go | 原 incarnation 拒绝断言 run/pass；只去掉无业务用途的中心 Receipt DTO 包装 |
| worker_codex_live / TestCodexAppServerNativeQuestionLive | steerlineintegration/worker_codex_live_test.go | 保留完整断言及 STEERLINE_CODEX_APP_SERVER_LIVE opt-in；未运行 live |

中心 activity 文件中与存储 round-trip 有关的来源数据改为中性 fixture；原具体 normalizer/detection 断言已迁走。删除的直属文件仅 `worker_codex_live_test.go`，由外层完整承接。

app 新增 `NewServiceWithPorts`，注入现有目录、生命周期和 ConversationSourcePort；原默认入口仍构造原来源。没有向测试导出 Service 私有写者、Store/Runtime 替代接口，也没有修改任何 root 生产文件。

第三实现位于外层 `_test.go`：`additional_implementation_fixture_test.go` 和 `core_decoupling_67_additional_test.go`。已编译的候选通过 `LoadConfigWithDirectory`、正式 app 构造、公共 Claim/查询/答案入口接线；隔离 ledger 保存原输入/回执，重新构造目录/来源/服务后核对原身份和人工门槛。它只声明离线只读 fake 能力，工具查询返回 unknown、取消/替换会话明确拒绝，不产生原生权限认证。**闭环在 Git fixture 初始化阶段被阻断，所有后续业务断言未实际执行；这些代码还需要独立审查、实际调试和补全，不能视为 CD67-2 达标。尤其 TaskDelivery 的正向唯一交付与结算证明尚未交付。**

`TestCD67AdditionalImplementation` 已组合第三实现、既有 app-server 两阶段提交、terminal 原身份恢复以及 remote Protocol 代表入口。它尚未加入 quick 的默认覆盖，真实 remote Claim 拒绝仍需纳入同一最小入口，不能把协议 fake 当作权限认证。

## 实际最小验证

使用工作区内 `GOCACHE=.../bin/go-cache`。默认 Go cache 首次写入被拒绝后，改用已有可写工作区 cache；没有调整 sandbox 或权限。

定向诊断：

```powershell
go run ./cmd/test-feedback -out bin/cd67/initial-focused -- -p 1 -parallel 1 ./internal/steerlineintegration ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1 -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation|TestCodexNormalizer|TestWorkerTurnFindsPRBeforeFinalMessage|TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestDetectNormalizer)$/(negative_source_fixtures|neutral_fixed_input_restart_and_fresh_manual_answer)$'
```

实际报告 `bin/cd67/initial-focused/summary.json`：**complete=true、passed=false，3 包，12 pass / 2 fail / 0 skip（含 parent/subtest）**，5.390 秒。五个迁入的非 live 入口及负向守卫 parent/各样本均有实际 run/pass；第三实现 parent/子用例 fail。失败在隔离临时 Git push：

```text
sh.exe: *** fatal error - couldn't create signal pipe, Win32 error 5
fatal: Could not read from remote repository.
```

没有重试该 fixture，也没有改为忽略、修改断言、变更准入或伪造授权。两条已支持恢复和 remote Protocol 子用例在本定向选择中未运行，原先 #66 证据不认证本源码。

最终源码守卫诊断：

```powershell
go run ./cmd/test-feedback -out bin/cd67/boundary-current -- ./internal/steerlineintegration -count=1 -run '^TestCD67CoreBoundary$/^repository_all_tags_and_production_graphs$'
```

实际报告 `bin/cd67/boundary-current/summary.json`：**complete=true、passed=false，0 pass / 2 fail / 0 skip**，10.455 秒。检查覆盖 root 180 个 Go 文件、26 个项目源码包、Windows 232/Linux 221 个生产依赖。生产禁用依赖为零；当前 **51 个直属测试/fixture 仍有平台文本、4 条测试直接实现 import 越界**，守卫如实拒绝。见 [boundary-checks.json](boundary-checks.json)。开始时实际 baseline 盘点为 53 个平台测试/fixture 文件（早期进度消息中 56 的人工计数已由完整清点纠正）。

Windows 受影响 root/app/integration/两个实现包及 guard 已通过仅编译；Windows/Linux `go build ./cmd/steerline ./cmd/test-feedback` 通过；`git -c core.autocrlf=false diff --check` 通过。Linux 测试编译首次错误地使用 `-exec go`，包装器返回 unknown command，保留为失败诊断；正确纯编译使用 `GOOS=linux go test -c -o bin/cd67/linux-compile/`。未执行 Linux 测试体。

各包、parent/subtest 实际事件见 [test-hits.json](test-hits.json)，报告摘要及原始日志哈希见 [validation-results.json](validation-results.json)。这些定向检查不替代规定组合。由于 fixture 及 root 隔离仍失败，没有运行 quick/full 或重复平台矩阵，也没有调用真实产品 CLI、业务服务或任务数据库。

## 必须继续完成的 #67 范围

[root-test-inventory.json](root-test-inventory.json) 清点全部原平台直属文件、入口/辅助和 live/evidence 开关，并标记当前剩余；这是完整输入盘点，不是完成映射。剩余 51 个文件需要按业务/原生/跨层拆分，保留已编译原断言和 opt-in 工具/版本/证据要求。

优先拆除 `core_decoupling_66_test_helpers_test.go` 中复制的三个具体平台工厂；中心用一份中性 fake 覆盖 Decision/intent/receipt/原输入/人工门槛/唯一交付，原生命令与 codec 归实现，跨层与 live 经正式 app/公共端口归外层。`linked_workspace_fixture_test.go`、`process_fixture_test.go`、`requirements_fixture_test.go` 仍需完成所有者归位；不能靠统一替换名称或复制另一套工厂达标。

完全访问续做者须先核对本工作区和源摘要，审查并调试第三实现，完成剩余迁移、去重/旧到新映射、guard/第三实现/remote Claim 拒绝的 quick 接入；然后只运行一次稳定的规定最小组合，核对所有包与 parent/subtest run/pass/complete，构建产品并更新最终所有者文档。当前代码保留失败和未通过项，不能用 exit=0、正则零命中或生产依赖清零替代全部验收。

全部实现完成后，才建立分支、提交、推送和可合并 PR，交协调独立复审/亲自修复；执行者不自行合并。remote 原生权限证据缺口保持原支持限制。本轮没有修改持久版本、旧格式策略、准备记录拒绝或首个资源操作前的 Git 授权检查。
