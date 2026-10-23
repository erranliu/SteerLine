# 验收：Orca 单终端原生状态切换

唯一归属：无编号普通开发“Orca 单终端原生状态切换”。[Spec](../../spec/orca-native-state-transitions.spec.md) · [Plan](../../plans/orca-native-state-transitions.plan.md)。日期：2026-10-12。

已实施原生订阅、主代理补查询、本轮正文核验与共用阶段接线。OST-A01–A03 通过；追加真实 PR 验收登记为 OST-A04/A05。2026-10-13 用户取消第二轮并要求整理提 PR：远程首轮已合并交付，本机首轮未完成，不声明两地全流程均通过。

## 自动验收项

| ID | 要求 / 场景 | 通过条件 | 状态 / 证据登记 |
| --- | --- | --- | --- |
| OST-A01 | 现有接口实际可用 / OST-NATIVE | 新包中的唯一普通终端两次只读回合；订阅/主 agent 补查询能核实原身份、开始/正常结束及新旧顺序，输入各一次，资源清理正确；没有模型标记或替代路径 | 通过；本机生产 lifecycle 两轮长输入/正文核验通过，终端已清理；远程真实样本见 A05 |
| OST-A02 | 只接受可信状态及一次补查 / OST-STATE | 本轮开始后可信新done才结束；waiting、旧/合成/重复/子状态不结束；30秒单在途补查，新记录可补认，旧记录/失败/超时不补造，订阅/查询重复只接受一次 | 通过；当前生产规则 18 项、0 skip，含 Markdown、正文不变与原实例校验；旧实验探针已删除，见最新审计记录 |
| OST-A03 | publish旧回复不提前推进 / OST-FLOW | publish working时旧report不能结算或触发下一输入；结束但完整本轮回复缺失时仍不推进，结果完整后按现行流程推进；local/environment判定一致，提示词和每轮输入次数保持 | 通过；本机/environment 两个受控 publish 门槛及共用 Codex round-trip，输入和结果各 3、无提前 publish.report，见生产实施记录 |
| OST-A04 | 本机真实流程 / OST-LIVE-LOCAL | 首轮 SteerlineTest 原生普通终端到专属 PR 核验合并；第二轮已取消 | 未完成；启动回执/就绪失败的候选未计通过，后一个候选完成实现后按用户要求停止，未到 PR |
| OST-A05 | 远程真实流程 / OST-LIVE-REMOTE | 首轮远端原生普通终端完成三个输入/结果，专属 PR 经 SHA 核验合并，交付并释放后继；第二轮已取消 | 首轮通过：SteerlineTest PR #227 已合并，workflow=completed/delivered，后继 dependency_ready=true；第二轮不计通过 |

## 执行与记录

准确命令、环境和时点见 Plan 测试计划。原生接入、生产状态接受及阶段测试均已实施。每项登记源码/构建、Orca版本、准确命令、通过/失败原因和原始证据路径；失败、skip、零匹配、缺信号证明或清理失败不计通过。

只验证上述必要范围，不安排单独人工验收。必要原生信号不可获取时不能用屏幕猜测或其他执行方式替代。

### 2026-10-12 接入门槛

- 源码基线 `8df615db23eb30e7adaa0fdaea40cc736a202a5d`，工作树包含本次未提交变更；本机 Orca 1.4.224。
- 独立包：`C:/SteerlineDebug/dev-20261012-150752-8df615db-dirty`。仅注册本包仓库；没有创建终端、服务或 PR，模型输入数为 0。保留包与注册信息供诊断，不操作安装环境。
- 连接/回执测试：`go run ./cmd/test-feedback -- ./internal/execution/adapters/protocol/orca -run '^TestOrcaNativeStateRPC$' -count=1 -v` 通过，覆盖请求/runtime 身份拒绝、原生订阅拒绝和取消。报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T072523.621339300Z/summary.json`。它不覆盖 OST-A02 的状态规则。
- 原生门槛命令：`powershell -NoProfile -File scripts/test-orca-state-transitions.ps1 -PackageRoot C:/SteerlineDebug/dev-20261012-150752-8df615db-dirty -OrcaExecutable C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`。
- `session.tabs.list` 真实成功，`session.tabs.subscribe` 真实返回 `method_not_supported: Method session.tabs.subscribe requires a streaming transport`。失败报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T072557.193446400Z/summary.json`。本包 `evidence/native-state-access.json` 已由下方最新两轮探针更新。
- 普通本机管道采用 CLI 原生 owner 认证；流式 WebSocket 需要原生配对设备认证，不能沿用该凭据。另查本机 `pairing.getEndpoints` 返回 `pairing_context_unavailable`。当前没有可核实的本机配对连接，按能力门槛停止；未改为轮询、受监督或结构化方案，生产完成判定保持原实现。

### 原生配对连接补充验证

用户随后选择保留订阅方案、暂不推进，并提供配对链接供只读核验。链接经原生加密认证连接到同一本机 runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`，Orca 1.4.224、scope=runtime；对本包原目标的 `session.tabs.subscribe` 收到合法 snapshot，随后关闭订阅。脱敏证据：本包 `evidence/paired-subscription-access.json`。配对凭据未保存到文件或仓库，模型输入数为 0。

当时只补齐了流式认证可用性，按用户选择暂停，未创建 agent。用户随后明确继续验证并实施；以下记录取代此前暂停状态。

### 2026-10-12 两轮普通终端验证

- 源码基线仍为 `8df615db23eb30e7adaa0fdaea40cc736a202a5d` 加本次工作树；同一独立 live 包、Orca 1.4.224、Codex CLI 0.159.2。生产协议代码完成原生 NaCl 配对认证及订阅/查询，没有调用受监督、worker_done 或结构化对话接口。
- 命令：在进程环境提供 `ORCA_PAIRING_CODE` 后，运行 `powershell -NoProfile -File scripts/test-orca-state-transitions.ps1 -PackageRoot C:/SteerlineDebug/dev-20261012-150752-8df615db-dirty -OrcaExecutable C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`。凭据未写入仓库、配置、日志或证据。
- 原目标 runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`；创建唯一普通终端，分别只读 README 和任务 Spec，每轮输入一次。两轮均有 `origin=hook/kind=transition`、同一 authority/incarnation 下递增 revision、原 provider session 及主 agent working。两轮 done 均无 observation/mainAgent；每轮连续 30 秒没有可信新信号后补一次 `session.tabs.list`，查询仍是缺少这些字段的 done，因此 0/2 结束可核实。
- 证据：本包 `evidence/native-state-access.json`（脱敏字段样本、输入数 2、verified_rounds=0、cleaned_up=true）；失败报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T082455.422283900Z/summary.json`。清理后 CLI `terminal list --worktree <原目标> --json` 返回 terminals=[]。前一版探针因 CLI 的短时 status 查询失败，在模型输入前退出并清理；未重发不确定输入。
- 交叉核对：`session.tabs` 的标题投影会移除 observation/mainAgent；`worktree.ps` 公共主 agent 行也不输出 observation，不能证明来源/顺序。保留部分接入实现，按 Plan 步骤 1 停止生产接线，OST-A01 未通过，OST-A02/A03 待验；101 提前结算缺陷尚未修复。
- 定向命令：`go run ./cmd/test-feedback -- ./internal/execution/adapters/protocol/orca -run '^TestOrcaNativeState(RPC|Stream|ProbeEvidence)$' -count=1 -v -timeout=1m` 通过（12 项，0 skip）。覆盖原生 JSONL 回执、加密握手/订阅/查询、请求/runtime 拒绝、取消，以及缺来源/顺序、旧/合成/其他实例记录的探针拒绝规则。最新报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T083207.416918200Z/summary.json`；不计生产状态规则或阶段验收完成。
- 用户随后明确要求将本机和远程两条配对链接保存以供后续验证；已保存在仓库外个人私有目录 `C:/Users/lmas1/.codex/private/steerline-orca-state-pairings.json`，键为 local/remote，文件权限限定当前用户及 SYSTEM。凭据未进入项目或证据；远程链接尚未做 live 验证。

### 2026-10-12 主代理补查询验证

用户授权“按建议先做验证”。本次只验证状态数据与实验性接受规则，不接入生产 worker 或阶段结算。

- 新包生成：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/new-debug-release.ps1 -Orca -OrcaExecutable C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`。包为 `C:/SteerlineDebug/dev-20261012-171914-8df615db-dirty`，live 方案；源码仍为 `8df615db23eb30e7adaa0fdaea40cc736a202a5d` 加本次工作树，Orca 1.4.224、Codex CLI 0.159.2。
- 命令：从个人私有配对文件将 local 注入 `ORCA_PAIRING_CODE`，运行 `powershell -NoProfile -File scripts/test-orca-state-transitions.ps1 -PackageRoot C:/SteerlineDebug/dev-20261012-171914-8df615db-dirty -OrcaExecutable C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe -MainAgentQuery`。两轮只读正常任务，加一轮长只读命令后的受控中断；模型输入 3 次、interrupt 1 次，每个输入只发送一次。没有模型结束标记、受监督或结构化对话。
- 普通结束 2/2 通过：原终端 runtime/incarnation、worktree/pane 保持；每轮先收到可信主代理 working，查询同 pane 的 mainAgent.done 时主代理状态时刻晚于本轮 working，updatedAt 不早于该结束证据；旧轮 done 被拒绝。正常结束原生 outcome 缺失，仍是未知，不计业务成功。
- 中断实际返回 mainAgent.done/outcome=cancellation，实验性普通完成规则拒绝它。安装包纯规则核验确认明确 Interrupt 与推断中断共用 cancellation，查询本身不能区分两者，不能把这个 done 当作已核实的普通结束。
- 实际证据：本包 `evidence/native-main-agent-query.json`；通过报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T093912.995091500Z/summary.json`。cleaned_up=true；事后原 worktree 的 terminal list 为空，SteerlineTest Git 工作树无变更。中断回合为采集字段做过短时只读查询，不代表生产调度策略已实现。
- 本机安装包纯规则命令：`node C:/SteerlineDebug/dev-20261012-171914-8df615db-dirty/evidence/verify-main-agent-producer.cjs`。仅在独立 Node 进程调用原安装包的纯 normalizer，不注入运行中的 Orca hook，不读 agent 私有日志。6 项通过：主代理 Stop、waiting、子代理 Stop、新旧状态时刻、推断中断 cancellation，以及 prompt 200 字符预览上限。记录本包 `evidence/native-producer-rules.json` 含被调用模块的 SHA256。waiting/子代理项不是 live 人工交互验收。
- 定向命令：`go run ./cmd/test-feedback -- ./internal/execution/adapters/protocol/orca -run '^TestOrcaNativeState(Stream|PSRule)$' -count=1 -v -timeout=1m` 通过（18 项、0 skip）。覆盖加密订阅及两种查询、身份/取消，以及无事件 revision 的新根 done、旧/working/waiting/子/其他目标/中断快照。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T094033.272506200Z/summary.json`。
- 现场准备限制：本机 CLI `terminal create --command codex` 的 rendererBacked 分支创建超时，两次探针在输入前退出；另一次手工创建也超时，原目标 terminal list 均为空，未重发输入。普通配对连接的公开 `terminal.create` 默认原生终端分支成功；该分支保持 ordinary Codex terminal，不建立 structured session。原目标通过既有 `worktree.activate` RPC激活；安装 CLI 未提供同名 activate 命令。
- 结论修正：此前“不带 observation/revision 就不能用”的结论过严，针对查询快照无需强求事件序号。正常结束的主代理查询来源已经验证；OST-A01 本机普通结束通过，OST-A02/A03 仍待生产接线验收。

实施范围中仍须落实两点：prompt 只是 200 字符预览，生产轮次必须绑定原请求回执及状态基线，不能依靠全文或预览唯一性；无信号补查仍 working 后，后续界面 done 应有再次核实的接线，不能耗尽一次查询后永久等待。本次未验证该调度窗口，也未验证完整普通回复归属或 publish 阶段接线，不将这些项声明通过。远程未做 live 验证。

### 2026-10-12 生产实施与验收

- 状态基线、原请求、可信 working 与原 pane/provider 绑定保存到现有封套；只接受新的主代理结束事实。30 秒补查同窗口一次，原 pane 的新 done 提示可另触发确认，重复提示不重复查询。共享 Service 以现有 StopTurn 保存已结束事实；没有完整本轮回复时仍对账，不发下一输入。
- 原生 ordinary Stop 正文小于已知截断边界且与完整可见回复一致后，保存原生普通原文，保持中文、空格及 URI；本文本不会作为模型结束标记。NativeOutcome 独立保存，缺失保留 unknown，采集成功不冒充原生成功。状态封套 codec=3，执行目录 builtin-execution-v8，没有旧格式兼容或迁移。
- OST-A02：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -run '^TestOrcaNativeState(RPC|Stream|PSRule|ProbeEvidence|Gate|Lifecycle|Binding)$' -count=1 -timeout=2m` 通过，35 项、0 skip。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T120157.487776600Z/summary.json`。
- 原 renderer 与启动回归：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport -run '^(TestAgentLaunch.*|TestTerminal.*|TestScreenReply.*|TestRenderedReply.*|TestComposer.*)$' -count=1 -timeout=2m` 通过，28 项、0 skip。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T120204.044180400Z/summary.json`。清理未使用的屏幕基线字段后，再跑 Gate/Lifecycle/Binding 与重复正文回合，11 项通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T122201.356963900Z/summary.json`。
- OST-A03：`go run ./cmd/test-feedback -- ./tests/integration -run '^(TestWorkerStageOrcaNativeStateGate|TestWorkerStageLocalRoundTrip)$' -count=1 -timeout=5m` 通过，4 项、0 skip。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T115913.190834300Z/summary.json`。本机/environment 均复现“publish working + 旧有效 report + 变化后的旧完成行”，查询 working 后不能推进或重复查询；新 done 再确认后，正文缺失状态为 reconciling/unknown、结果仍只有 2 条；当前正文到达后按既有 Git/PR 真值规则到 await_merge，模型输入/结果各 3 次，无 publish.report、无重发、提示词未改。
- OST-A01 生产 lifecycle：先运行 new-debug-release，生成 `C:/SteerlineDebug/dev-20261012-194500-8df615db-dirty`。配对只注入进程环境，运行探针脚本追加 `-Lifecycle`，执行 `TestOrcaNativeStateLiveLifecycle`。同一原普通 Codex terminal 的两轮超过 200 字符的只读输入，经 production StateFeed/lifecycle/observer 完成原生开始、结束补查及完整普通正文核对，输入/结果 2/2、NativeOutcome=unknown。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T115714.929393000Z/summary.json`；本包 `evidence/native-production-lifecycle.json` 保存原身份、两轮结束证据及 cleaned_up=true。事后原目标 terminals=[]，SteerlineTest Git 工作树无修改。
- 真实现场修正：原生后台 PTY 在 UI 未收养时可返回 orphaned=true；保留 runtime/incarnation/worktree/branch/agent 校验，要求 connected 且未 exited，不把 UI 收养标记当作实例身份。前置失败的探针均关闭了自己创建的终端；固定测试 operation ID 的重跑被原生幂等拒绝后，改用该探针原终端身份生成新的独立回合 ID，未重发不确定输入。
- 目录独立性回归 `TestBuiltinCompositionIndependentCatalogsAndHosts` 通过；修正测试中已经滞后的固定 v6 预期，仍验证独立目录/进程主机及构造不探测。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T121719.739695600Z/summary.json`。

本机启动服务需要可用的原生配对认证（现有 ORCA_PAIRING_CODE / ORCA_REMOTE_PAIRING）；environment 使用 Orca 原生保存的认证。预检核对原 runtime 与主代理查询访问，输入前实际订阅原终端。未改安装环境；认证未写入项目/封套/证据。原生正文处于 8000 UTF-16 单元的截断边界时保留结果未核实；不通过 publish.report 修补。没有运行全量、远程 live 或真实 PR 验收。

## 文档检查

2026-10-12 最终检查：`python scripts/check_doc_links.py` 检查 348 个 Markdown、2090 个本地导航目标，0 错误；`git diff --check` 和 `python scripts/roadmap_task_id.py check` 通过。流式 reader 退出时关闭订阅的资源收尾后，`TestOrcaNativeStateStream` 定向复验通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261012T123609.042815000Z/summary.json`。源码未含本机或远程的真实配对凭据字面量。本次仍为无编号普通开发，未改 roadmap、未跑全量。

## 2026-10-13 审计与真实流程

- 整合主干 PR #295（`0bcecbe5`），本次构建源为 `4a3329a4` 加工作树。修复 inline Markdown 两种链接显示的正文核对、原生正文被同显示摘要覆盖的问题，以及订阅缺省 incarnation 元数据时仍通过前后 CLI 核验原 PTY。Orca 调试 Spec/Plan 清掉阶段指令，结果带当前分支且只本地提交，使后继有独立变更。
- 去掉已被替代的本机管道代码、实验性探针和独立实验解析器。当前探针脚本默认执行生产 `TestOrcaNativeStateLiveLifecycle`；上方旧命令仅为当时的证据历史。受控阶段测试等待本轮 started 落库后再检查 working，修正了读到上一轮 stopped 快照的测试竞态。
- 最新 OST-A02 命令见 Plan，18 项、0 skip：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T020950.449176800Z/summary.json`。renderer/启动回归 28 项、0 skip：`20261013T015947.412478200Z/summary.json`，同一 runner 根目录。
- 最终开始基线审计补充：递增 observation revision 不能把旧根 working 时刻当作本轮开始；要求 root_working_at 晚于发送前 BaseRootAt，新增拒绝用例。18 项定向复验通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T023251.639715300Z/summary.json`。远程保存的三个实际回合均满足此条件，不重跑已取消的真实验收。
- 最新 OST-A03：`go run ./cmd/test-feedback -- ./tests/integration -run '^(TestWorkerStageOrcaNativeStateGate|TestWorkerStageLocalRoundTrip)$' -count=1 -timeout=5m`，4 项、0 skip，通过报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T020550.843138800Z/summary.json`。修正测试竞态前的失败报告 `20261013T015757.916835400Z` 保留，不计通过。
- 真实环境均先由 new-debug-release 生成。最终本机包 `C:/SteerlineDebug/dev-20261013-100521-4a3329a4-dirty`，Orca 1.4.224；远程控制包 `C:/SteerlineDebug/remote/dev-20261013-100530-4a3329a4-dirty`，environment=台式机64g、Orca 1.4.223，远端专属 checkout `C:/SteerlineDebug/ost-20261012-remote-9f8d/projects/SteerlineTest`。固定 CLI `C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`；Codex CLI 0.159.2。配对凭据不进入包或证据。
- 远程首轮：原 runtime `95d30a41-2f86-43ae-ac0d-9c304b5f1f90`，workflow `3a6581a2d0645bbd3dd0511883c7c8c9`；implement.execute、implement.report、publish 各一次，三轮 accepted/started/settled，working→新的原主 agent ended 均成立，NativeOutcome 均为 unknown。没有 publish.report、受监督结算、模型结束标记或结构化对话。
- [SteerlineTest PR #227](https://github.com/erranliu/SteerlineTest/pull/227)：base=`codex/debug/dev-20261013-100530-4a3329a4-dirty/base`，head=`erranliu/sl-1-3a6581a2d0645bbd3dd0511883c7c8c9`，SHA=`8fbc4024f649c833d2ab2fb9259654c91dea0916`。核验仅改 result.txt、UTF-8/LF 内容及实际分支，再 `gh pr ready`、`gh pr merge --merge --match-head-commit <SHA>`。merge=`b4c1ba7fa65948ae72c4d398b12053a27525d16b`；随后 SteerLine completed/delivered、task 1 closed、task 2 dependency_ready=true。
- 远程证据位于该包 `evidence/pr-task-1-before.json`、`pr-task-1-merged.json`、`task-1-delivered-snapshot.json`、`native-real-flow-summary.json` 和 `final-snapshot.json`。第二任务开始准备时远端尚未 fetch 首轮 merge commit，worktree 创建明确失败、模型输入为 0；用户随后取消第二轮，未继续同步或重试。此准备缺口保留在最终快照，不声明后继真实执行通过。
- 本机首轮：一个输入回执不明的候选未推进或重发；另一个候选就绪超时且无任务输入。显式关闭原资源后新建候选 `3f3ad4da41cb9cda4f91002a97140357`，实现结果已保存、尚在 implement.report 排队时用户要求停止并提 PR。最终快照保留三个候选；本机专属 base 的 PR 查询为空，不计真实 PR 合并通过。
- 两个最终包服务已停止，原终端逐个按 runtime/incarnation 核验后关闭，terminal list totalCount=0。各包 `evidence/service-stop.json`、`cleanup.json` 保存收尾证据。诊断包也已停止自己的服务/终端；保留 Git/PR 事实，不重置或操作安装环境。第二轮按用户最新要求取消，无需继续作为提交前门槛。
