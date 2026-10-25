# Runner 等待、重试状态与执行页验收规划

日期：2026-10-25。状态：用户后续授权实施 SteerLine 调用方；自动验证与真实链资格分别登记。关联 [Spec](../../spec/runner-wait-retry.spec.md) / [Plan](../../plans/runner-wait-retry.plan.md)。不修改生产 Task 18、原候选或安装目录。

## 自动验收安排

| 项目 | 最小通过条件 | 当前状态 |
| --- | --- | --- |
| SLW-A01 | 公开能力/schema/字段、原 Run/实例/revision/CAS，坏对象保留最后有效值 | owner 与 controlled 自动验证通过 |
| SLW-A02 | 真实本仓服务/DB 与公开 fixture 的等待→恢复/耗尽，零多余候选/输入/Decision 答案 | controlled 自动验证通过 |
| SLW-A03 | 页面文案、时间、近似重试、陈旧/断线/时区/切换、null 清除 | Web 组件通过；真实 Edge 观察 queue/null 通过，人工体验待签收 |
| SLW-A04 | 自动等待无多余继续，manual 原资格/信封，候选退出与 Runner stop 区分 | 原控件/命令不变；owner/Web/controlled 通过，缺 PR 的原取消限制保留 |
| SLW-A05 | 原身份与预算跨宿主/Web 重启、事件幂等、旧响应/替代实例隔离 | owner/controlled 通过；真实浏览器文档重开保持预算，完整 Web 进程重启未取得真机通过 |
| SLW-A06 | 新独立包与固定真实 Runner/Orca 的 >15 秒竞争→自动恢复，API/Web/资源清理取证 | 第 06 轮真实远端 Orca、Edge、原 Run 停止及宿主/offline 结算通过；历史告警与失败单列 |

`recovery.runner-wait-retry` 已实现为 L3 controlled ID，登记 suites.json、test-list.json、L3 列表及执行器选择检查。静态检查、fixture 和 Runner 原生资格各自留证，不共用通过状态。

## 准备、证据与人工体验

准备独立 home/repo/DB/provider/browser、固定 Runner source/EXE SHA/version/resource/schema、具备归属与清理证明的 Orca 测试目标。真实专项涉及同用户全局 CLI cache，按 Plan 串行或证明实际隔离，不干扰其它生产进程。

输出 scope `C:/SteerlineDebug/wt-507e-runner-wait-retry/`；每次使用新的 test-results/diagnostics 叶目录和生成器返回的包身份。逐次登记精确命令、受测两仓 SHA/dirty、构建/实例、实际测试 ID、API/浏览器/原生输出绝对路径、终态及清理。原失败与补测保留。下面文档阶段的“未实施”结论仅描述当时状态，当前实施结果见末节。

可后置人工体验 SLW-M01：自动项通过后，在已交付独立包中体验等待/重试/未知核验文案与操作清晰度。参与人为用户或产品验收者，窗口待定；此项只签收体验，不替代 SLW-A06 的真实证据或任何已知功能失败。本轮不安排人工窗口。

Runner 自身 OCW-A01–A07 由该仓 `docs/acceptance/orca-cli-wait-retry.md` 独立维护。本仓只消费其公开资格，不把两仓 Spec 写完记录成已实现。

## 2026-10-25 文档检查

`python scripts/check_doc_links.py` 检查 576 份 Markdown、3605 个本地链接，0 错误；两仓共 8 份新增/修改文档的本地链接、尾随空白/换行、围栏、共同字段/枚举和 JSON 示例专项检查通过（226 个本地链接，0 错误），两仓 `git diff --check` 通过。没有代码变更，未运行产品 L1/L2 或真实链；上述结果不改变 SLW-A01–A06 的待验收状态。

以上是首轮原稿检查。随后设计审计删除三类加权调度、统一采样退避、写操作新预算与重复公开状态；首版仅消费 repository_inventory/runtime_status 的 queue/calling/backoff/exhausted。未知副作用继续使用原事实/界面；不为其建设第二状态机。Runner 独立实现/验收，SLW-A06 不再是其完成前置。本仓 Spec/Plan 已同步收敛，当前修订提交前另做静态检查，所有功能项仍待验收。

审计修订复验：全库链接检查仍为 576 份 Markdown、3605 个本地链接、0 错误；两仓 8 份本次文档的 225 个本地链接、围栏/编码/空白、最小公共字段/枚举及 JSON 示例检查通过。Runner 查询窗口改为复用原父调用 90 秒上限，并取实际更早 deadline；本仓仅消费实际值。本轮只授权提交/push Runner 文档，本仓修订保留工作区，未运行功能验收。

## 2026-10-25 调用方实施与复验

用户后续明确授权“steerline这边可以实施了”。受测 SteerLine 为 `9607fb0099dfc00d9f3de35429d4c43fcdc80407` 加本工作区 dirty 修改；Runner fixture 固定已提交文档 `d95bbc1ba207033e8fed7fb03d351d6e5190ed0d`。本次没有使用真实 Runner EXE、Orca 目标或生产 Task 18，未部署，未修改 Runner 实现。

实现沿原 Run/候选投影与事务接入对象/null 和事件，公开 DTO 单独定义展示值并深拷贝，不把 backend 类型带入 HTTP 消费者依赖。新 Orca profile 同时检查等待 schema 和同快照 observe；其它 adapter 及原候选读取/停止不因本项缺能力而整体失效。Web 列表/摘要复用当前查询，单调文字计时无网络请求；到期等待新快照，断线/错误保留最后已知；未知命令、人工问题、停止及原操作资格优先。

共同证据根：`C:/SteerlineDebug/wt-507e-runner-wait-retry/`。下列路径均为其子路径；每次新目录，失败没有覆盖。报告包含源码 dirty 清单、实际选择和执行结果，E2E 包另存冻结二进制清单。

| 验证 | 实际命令与结果 | 证据 |
| --- | --- | --- |
| L1 固定集 | `go run ./cmd/test-feedback -level=L1 -out <目录>`；passed/complete，准备 0.57s、执行 10.38s | `test-results/L1/20261024T235410649Z/summary.json` |
| L2 固定交付冒烟 | `go run ./cmd/test-feedback -level=L2 -out <目录>`；passed，准备 16.97s、执行/清理 140.23s | `test-results/L2/20261024T235344612Z/summary.json` |
| owner/登记/架构 | `go run ./cmd/test-feedback -out <目录> -- ./internal/runnercontract ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner ./internal/product/... ./internal/app ./cmd/test-feedback -run 'Test(ExecutionWait\|PreparationWait\|RunnerDelivery\|CM71ProductDependencyBoundary\|TM05ProductDTO\|TMRegisteredCatalogueMatchesExecutableManifest\|ScenarioContentListMatchesRegisteredIDsAndLevels\|L3AndL4SelectDisjointRegisteredContent)' -count=1`；45 项通过、0 失败、0 测试跳过，22.38s；两个纯契约包无匹配测试，不冒充 owner 全集 | `diagnostics/go-verified-20261024T235528584Z/summary.json` 与 `go-test.jsonl` |
| Web | `./scripts/test-web.ps1 -EvidenceDir <目录>`；build、191 项组件测试、server/httpapi/httpclient 通过；未配置真实浏览器 smoke | `diagnostics/web-verified-20261024T235513724Z/{build.log,client-tests.log,server-tests.log}`；server 报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T235542.232817000Z/summary.json` |
| L3 等待流程 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.runner-wait-retry' -out <目录>`；passed，准备 17.06s、执行/清理 258.44s；唯一所选场景通过，cleanup_complete=true、两项目 Runner cleanup confirmed=true | `test-results/L3/20261024T235344641Z/summary.json`、`scenarios/recovery.runner-wait-retry/scenario.json` 与 `*-runner-cleanup.json` |

L3 使用真实本仓服务/Git/SQLite/HTTP、脚本化公开 Runner peer。依次观察 queue/calling/backoff/null，宿主重启保持原候选/Run/Attempt/deadline；读取不增加输入。原活动路由保存四个不重复等待事件及清除 null；释放 fixture 后唯一交付并核实原停止。另一独立项目验证 exhausted 与原恢复 Decision，不自动 resume；准备阶段缺 PR 绑定的 discard 按原 `merge_state_unknown` 拒绝，候选/Task 和停止资格不变。末尾公共测试清理单独停止所属 fixture Run、核实目录与进程，不把清理当成用户取消成功。

失败/修正记录：最初 L3 错把 Orca 设为测试生成默认方案，`20261024T233802459Z-bd2b29a2f1e248808a783091106432b0` 与 `20261024T234024254Z-72af4878cb6548099d99951f6dd03c9c` 报 `configuration_required`；修为本机生成、显式 Orca 交付。`20261024T234404399Z-a414aa94e6474bd0b3b4a40d701b1950` 已覆盖主流程，第二项目在 discard 的既有 PR 资格处失败；测试改为核验原拒绝，不放宽产品保护。`20261024T234852924Z` 与 `20261024T235051541Z` 的活动断言失败源于测试漏解 HTTP `page` 信封，已修正。各次清理结论保留原报告。本次 peer 补同快照 observe，避免用分开的旧查询证明同快照资格。

早期 owner 回归 `diagnostics/20261024T233020179Z-6da188921c2e49539c209bc33aebb3bc/` 不完整且有失败：本次引入的 API 后端类型依赖已改为独立 DTO，并在上述边界测试复验通过；既有 `TestHTTPConsumerBoundary`（Web 依赖 runnercontract）及 `TestInstallLaunchersWritesCRLF`（断言遗漏 restart.bat）未在本项扩张修复，前次[流程目录验收](steerline-flow-layout.md)已有相同基线复现。app 广泛运行还在默认 10 分钟期限留下未结束的启动测试，不能声明 app/Go 全集通过。`diagnostics/owner-fixed-20261024T234034851Z-196929693d2241ada1a81893c03c6dc2/` 保留 HTTP 边界失败。Web 首轮 `diagnostics/web-20261024T233635015Z-f34bc2489d2546f6af7a616387739728/` 的过期 `/tests/records/{id}/evidence` 成功断言已改为验证现有 404，与 HTTP 原规则一致，没有恢复退役路由。

静态复验：`python scripts/check_doc_links.py` 当前 576 份 Markdown、3612 个本地链接、0 错误；`git diff --check` 通过。Spec/Plan/接入说明、客户端协议、worker 生命周期与测试登记已同步。

尚未验：SLW-A06 的真实 Runner/Orca 竞争、自动恢复、取消及浏览器取证，须上游实际实现构建；SLW-M01 人工体验及真实浏览器重启也未执行。上述 owner/controlled 结果不提供原生重试资格，不把本仓完成条件倒置成 Runner 的完成前置。

## 2026-10-25 09:20 合入后续跑

定时检查确认 [Runner PR #57](https://github.com/erranliu/SteerRunner/pull/57) 于 `2026-10-25T01:03:37Z` 合入 `main`，远端默认分支及独立 clone 的 HEAD 均为 `5df57b69e98382abcd63676b04a73079fd939fa6`。本任务建立 `C:/SteerlineDebug/wt-507e-runner-wait-retry/runner-source-5df57b6` 干净源码副本，不修改 Runner 实施聊天工作区；SteerLine 当前保留于 `codex/runner-wait-retry-ui`，尚未 commit/push/提 PR。

已核对正式公开契约：12 个字段、对象/null、query/stage/reason、次数与 UTC、事件顶层值均与调用方一致；`run_observation` 公布同快照及 `max_events=500`，满足原调用方每页 100 的要求。删除原手写简化 schema，以合入源码的公开 `execution-wait.schema.json` 逐字节固定为 `internal/testkit/protocol/testdata/execution-wait.schema.json`，由协议 fixture 嵌入。SHA256 为 `5697e0ad803b3f5ccd3e48c2b91fac2f7cf215ce99ff40247dfeac87b90836a1`，与上游 Git blob、源码 checkout 一致；两仓原类型和产品 DTO 字段集合一致，生产解码无需改动。静态证据位于 `diagnostics/upstream-20261025T0120/contract-audit.json`，不等同于实际 CLI 输出或原生资格。

### 安全阻塞与需要用户处理的边界

合入 PR 正文及其 `docs/acceptance/orca-cli-wait-retry.md` 明确保留未核查的安全告警：2026-10-25 08:45（北京时间）Defender 检测 `Trojan:Win32/Bearfoos.A!ml`，隔离 `C:/SteerlineDebug/runner-cli-wait-retry/final-05/controlled/steerrunner.exe`；完整 Controlled 失败，L2 和 local/remote 原生 case 未派发。用户要求的 SteerLine 真机验证不能继承该缺失资格，也不能把合入视为告警解除。

本聊天在 `2026-10-25T01:23:05Z` 只读查询 `Get-MpThreat` / `Get-MpThreatDetection`：ThreatID `2147731250`、`ActionSuccess=true`、`ThreatStatusID=3`、原受检路径仍有记录，`IsActive=false`。后者不构成人工判定或误报结论。该次限定本任务的原始记录保存于 `diagnostics/upstream-20261025T0120/status.json`。已告知用户并请求核查 Windows 安全中心的检测及提供处理结论；本聊天没有构建/执行新 Runner、恢复/更名受检 EXE，或修改防护设置。

当前仅执行 SteerLine 自身与公开协议夹具的回归，不运行独立 Runner 实现。SLW-A06、真实 EXE 身份/原生/浏览器取证和最终提 PR 保持待验。定时续跑已保留合入/源码/分支与阻塞状态，未变化时保持安静；收到明确处理结论后再继续真机流程，通过后按原授权提交、push、创建并附加 SteerLine PR，不自动合并。

### 正式 schema 夹具下的回归

所有路径仍相对共同证据根，入口未执行真实 Runner。L1/L2 使用原固定命令，定向命令为 `go run ./cmd/test-feedback -out <目录> -- ./internal/runnercontract ./internal/runnerclient ./internal/runnerbridge ./internal/app -run 'Test(ExecutionWait\|PreparationWait\|RunnerDeliveryReadProjection)' -count=1`；L3 使用原精确 ID。不是完整 Go 全集或真机通过。

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 正式 schema 与 caller/bridge/DTO | 27 项通过、0 失败/跳过，11.78s | `diagnostics/upstream-owner-20261025T012308347Z/summary.json` |
| L1 | passed/complete，准备 0.72s、执行 10.33s | `test-results/L1/20261025T012347766Z/summary.json` |
| L2 | passed，准备 19.28s、执行/清理 92.62s | `test-results/L2/20261025T012347913Z/summary.json` |
| L3 等待流程 | passed，准备 19.31s、执行/清理 198.02s；cleanup_complete=true，两项目 Runner cleanup confirmed=true | `test-results/L3/20261025T012347913Z/summary.json`、`scenarios/recovery.runner-wait-retry/scenario.json` 与 `*-runner-cleanup.json` |

`python scripts/check_doc_links.py` 与 `git diff --check` 本轮通过；所有历史失败/基线限制保留上节，真实测试阻塞未据此解除。

## 2026-10-25 12:29 控制变量排查 Defender

用户进一步授权排查隔离原因，并允许控制变量多次 build。本轮保持 Defender 设置不变，不恢复隔离或执行历史受检摘要，不更名受检程序。证据根为 `diagnostics/defender-build-20261025T042930496Z/`，完整可读说明为该目录 `REPORT.md`，逐组结果为 `matrix.json`；以下结果补充前节仅依据历史记录保留的阻塞判断。

5 次构建、6 次显式自定义扫描全部由当前 Defender 报告“未发现威胁”，退出码 0 且产物仍在。受检定义版本由历史事件中的 `1.461.183.0` 变为本轮 `1.461.190.0`，引擎均为 `1.1.26090.9`；没有回退定义来复现历史分类。具体证据：

- 干净合入版本 `5df57b69e98382abcd63676b04a73079fd939fa6` 按原 `scripts/build.ps1` 连续构建两次，SHA-256 均为 `4a7e7ceab07a52ce880d6d09af587f18af5fdfb735b24cb5b7742b3d819d874d`，两次扫描未检出。
- 依据原冻结清单还原 755 个输入，8 个文件恢复原 CRLF 后全部摘要相同。重建 Go VCS 元数据按原状态省略后，产物 SHA-256 精确等于原 `final-build.json` 的 `f83d475fb77c9848e9d8aec9694bafbd54b30da1b407bec005e40050a7a442a5`，扫描未检出。这是追溯旧样本的诊断产物，未执行，不作为后续运行构建。
- 原 `controlled-normalized-final/steerrunner.exe` 的现存副本本身即为上述历史摘要；在其原目录原地扫描同样未检出，没有复制或恢复隔离文件。被隔离的原 `final-05/controlled/steerrunner.exe` 仍缺失。
- 改动前 `af5e244ddae05d4c19069852d56c5a74a68ab1ce` 的正常构建也未检出；`go.mod`、`go.sum`、Windows `.syso` 资源未被此次功能修改，`go mod verify` 返回 `all modules verified`。

正式合入构建另通过 `version --json` 和 `capabilities --offline --json` 的只读实际执行，退出码均为 0；该阶段没有启动业务服务或原生 agent。实际 CLI 的公开等待 schema 与本仓 fixture 完全相同，`run_observation` 为同快照、max_events=500；resource digest 为 `635c09b807d2a10e484710042c53541e1355dbe8221abde4b600c597509811cd`。原始输出及比较在 `08-merged-readonly-runtime/`。

原失败日志最早在 `TestSRResultAcquisition/audit-zero` 的 service start 报系统病毒拒绝，此后多数错误由同一 EXE 被隔离/移除连锁产生。本轮使用相同 SHA 的正常正式合入构建，按原公开入口精确复跑 `TestSRResultAcquisition` 与 `TestSROrcaCLIWaitRetry`，新目录为 `09-controlled-trigger/`；两个顶层、17 个变体全部通过，退出码 0、scenario_count=2，分别耗时 745.02s / 88.52s，于 `2026-10-25T04:53:41Z` 完成。这是 Runner 服务/CLI/SQLite 与原隔离协议 fixture，不是真实 Orca 或模型。结束核查中相关新检测为 0、诊断目录运行进程为 0、正式及受控 EXE 均仍存在，测试自身清理断言通过；原记录为 `result.json`、`integration.log`、`source-identity.json` 和证据根的 `post-runtime-status.json`。

当前可确认的是历史检测尚未在本轮扫描/只读执行中复现。病毒库、云端判定、扫描上下文或环境策略变化均是可能解释；没有唯一归因或微软确认误报。实时保护、行为监控和 IOAV 保持开启，但普通用户的 `-CheckExclusion` 返回 `0x80070005`，有效排除项查询被隐藏，因此这项条件未核实，不能将未检出写成无条件的安全证明。原事件 XML、扫描完成事件和该权限限制保存在本轮证据。

本轮没有修改 Runner 生产逻辑、依赖或包装方式来规避检测。后续运行候选只选择正常正式合入构建并固定 SHA，在防护保持开启的新隔离包验证；历史告警、当前未复现的结论及实际未验范围分别保留。若发生新的防护拒绝/隔离，保存失败并停止对应程序，不通过修改或变形产物强行执行。SLW-A06 真实链仍须独立完成，之后才按用户授权提交 SteerLine PR。

续跑以本节新证据为准：当前正常正式构建已通过扫描、只读 CLI 和原触发路径的受控执行，可继续隔离真机联验；历史告警继续单列保存，不声称已被微软解除。此轮没有修改 SteerLine 产品代码，原 L1/L2/L3/Web 结果维持各自冻结范围；真机联验后仍须做最终回归和提 PR 前检查。

## 2026-10-25 13:00 恢复连续实施与真机联验

用户明确要求继续原任务并取消定时；`runner` heartbeat 已设为 PAUSED，本轮连续执行。工作分支快进到已合入候选停止可靠性修复的主干 `1883879dd70b188811427b4b4f72528019f91b0c`，原工作区改动通过专属 stash 保留并恢复；三个文档入口冲突保留两项功能说明，产品代码自动合并后重新验证。Runner 固定正常构建 `4a7e7cea…19d874d`、源码 `5df57b6`，没有改写上游工作区或部署。

真实目标沿当前指南：远端 Orca environment `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`，runtime `f8616747-34af-4543-8487-ec6b5d8de4ff`，已登记 `erranliu/SteerlineTest`，`gpt-6-luna` / `medium`。所有包由 `new-debug-release.ps1 -DebugRoot C:/SteerlineDebug/wt-507e-runner-wait-retry/packages -SteerRunnerExe <固定正式构建>` 生成；各包的实际 manifest、方案、CLI/浏览器日志单独保留。调用方材料生成使用本机 Codex，交付显式选独立 Orca profile；同一 live provider，在首次宿主启动前固定完整配置，不热改原候选。真实认证仍属于原工具登录，没有复制安装环境。

诊断驱动及原始证据在 `diagnostics/native-20261025T0508`、`-02` 至 `-06`。私有 LOCALAPPDATA 只用于本包 Runner 的故障通道，注入持有者不占用真实用户锁；转发器逐次恢复原 cache 并取得原共享互斥，调用真实安装版 Orca，记录请求/原回执，不伪造原生结果。进程启动采用同用户独立上下文；所有业务请求来自正式调用方入口。以下失败保留原结果，不合并为通过：

| 轮次 / 包 | 原失败与后续处理 |
| --- | --- |
| 01 / `dev-20261025-130242-9607fb00-dirty` | 原 Orca-only 模板缺本机材料生成方案，准备阶段 configuration_required，无候选启动。 |
| 02 / `dev-20261025-130651-9607fb00-dirty` | 已观察真实 queue 超过旧 15 秒；从工具 Job 启动 Web 常驻进程被拒绝。原资源停止遇未知；独立清理后 `post-cleanup-original-environment.json` 明确 resources_settled=true，原失败不覆盖。 |
| 03 / `dev-20261025-131045-9607fb00-dirty` | Web stop 返回 Access is denied；收尾恰逢原创建 owner 启动，保留 outcome_unknown。转发日志没有发出 worktree create，原 stop/不完整绑定与 offline resources_settled=false 均保留，不删除状态或伪造已结算。 |
| 04 / `dev-20261025-131614-1883879d-dirty` | >15 秒、浏览器重开、同候选/Run/Attempt 自动恢复及真实模型产物已取得。一次显式 stop 被 stale_revision 明确零受理拒绝；驱动未正确处理该拒绝。清理顺序先关 Runner 再关宿主又导致原停机回执身份不同；后续公共 offline 核实 resources_settled=true。普通本机 marker 程序不适用远端 agent 的执行环境，其失败另保留。 |
| 05 / `dev-20261025-132557-1883879d-dirty` | Orca fixture Plan 没有可选清单入口，也未声明无需清单自动项，空安排按原规则拒绝，未启动候选。已在模板明确无需项目清单自动项，仍要求原生执行核对文件字节/提交；调用方联验独立负责等待与资源验收。 |
| 06 / `dev-20261025-132926-1883879d-dirty` | 真机补验通过，result.passed=true；保留原版本门槛，两次 stale_revision 均证明零受理后刷新同一 Run，第三次 stop 受理。原 Run stopped 后由原宿主结算服务，launcher 退出 0、shutdown.clean=true、offline resources_settled=true。 |

前期从默认环境调用包内转发器的 offline 核实因重复共享锁而超时；这些错误结果也保留于 `post-cleanup-verify.json`。补核实必须使用该包原 private-cache 环境，不能把超时解释成资源已停。没有改防护或出现新的相关检测。

合并后固定回归已通过：`test-results/L1/20261025T0520-main`、`test-results/L2/20261025T0520-main`、`test-results/L3/20261025T0520-main`；owner 定向与候选停止相关检查为 `diagnostics/owner-main-20261025T0520`，Web 为 `diagnostics/web-main-20261025T0520`。补充模板 Plan 后另用新目录重跑 L1/L2；真实第 06 轮及最终清理结果续记，不预先记为 A06 通过。

### 13:40 最终结果与边界

第 06 轮于 `2026-10-25T05:35:34.291Z` 完成，`result.json.passed=true`；证据根 `C:/SteerlineDebug/wt-507e-runner-wait-retry/diagnostics/native-20261025T0508-06/`，包为 `packages/dev-20261025-132926-1883879d-dirty`。受测 SteerLine 为主干 `1883879d` 加本项修改，固定 Runner version `0.2.0-dev.3-r2`、source `5df57b6`、EXE SHA-256 `4a7e7ceab07a52ce880d6d09af587f18af5fdfb735b24cb5b7742b3d819d874d`、resource digest `635c09b807d2a10e484710042c53541e1355dbe8221abde4b600c597509811cd`；实际 schema 仍等于仓内 fixture。

- 原候选 `6d835642565aec09cc66e0acb79e69f6`、Run `c7bcc572c65f274e5b50facc8e00fab7`、Attempt `2e40d53b7a97c27bf25f2217675d7d7d`。query started `05:32:11.4373179Z`、deadline `05:33:41.4361557Z`；占用 **17.514683 秒**后在原窗口内释放。排队 calls_started=0，浏览器文档重开前后同一对象/预算，无 resume_required；释放后原身份自动继续、execution_wait=null，页面移除等待块。
- `queue.json`、`after-old-limit.json`、`browser-reopen.json`、`recovered.json`、`native-active.json` 及三张 PNG/`browser-trace.zip` 保存 API 与真实 Edge 证据；`activity.json` 保存原等待/清除事件。候选命令只有一条 submit，没有调用方 reply/resume 或第二候选。Runner 自己的三个正式工作流输入各不重复，不把它们误称为调用方重试。
- 真实 Codex 实施并核对文件，独立诊断终端再次逐字节核验 marker/实际分支/UTF-8 LF、仅 result.txt 提交及干净工作区，提交 `3a99fd208fd7e2961979d72c4b42bbb9a5b5bb09`。原始结果在 `cleanup-result.json.artifact`，诊断终端关闭 `ptyKilled=true`；不是根据模型自然语言认定产物成功。
- 两次 stop 明确拒绝 stale_revision 后刷新同一原 Run，第三次受理；`runner-stopped.json` 为 stopped、new_inputs_allowed=false、execution_wait=null。随后原宿主关闭，launcher 退出 0、shutdown.clean=true；`offline-stop.json` 对两条原 Run（本机材料生成及 Orca 交付）逐项和整体 resources_settled=true。
- `native-audit.json` 最终 52 对真实意图/回执，只有一次工作区创建，三个工作流输入 SHA 均唯一，errors=[]。后置只读核实/清理也有独立回执，不复用业务输入。
- `github-ref-cleanup.json` 按原固定 commit 的 lease 删除六个本次 base 并复查不存在，无真实测试 PR。三个实际创建工作区按原回执 ID 清理；`final-cleanup.json` 核对四个自有目标（含第 03 轮未知创建的预期位置）均不存在、精确 Git 引用已清理、远端根工作区状态未改变，诊断终端关闭确认。第 03 轮保留原不完整绑定/未结算的失败库，不伪造其公共账本已成功；独立位置核对未发现实际原生工作区。
- `postflight.json` 为本任务所属常驻进程 0、新相关 Defender 检测 0，实时/行为/IOAV 保护保持开启。失败与补验现场均保留，不使用安装版或生产 Task 18。

最终回归：L1 `test-results/L1/20261025T0531-fixture-final` passed/complete，准备 0.70s、执行 7.32s；L2 `test-results/L2/20261025T0531-fixture-final` passed/cleanup_complete，准备 14.49s、执行/清理 82.92s。L3 `test-results/L3/20261025T0520-main` 仅选 recovery.runner-wait-retry，passed/cleanup_complete，准备 23.94s、执行/清理 210.97s；owner `diagnostics/owner-main-20261025T0520` 的 52 项及 Web `diagnostics/web-main-20261025T0520` 的 build/200 项组件/server 检查通过。模板补充仅为测试说明，产品程序代码与该轮 L3/owner/Web 相同；真实第 06 轮使用补充后的模板。

SLW-A01–A06 的上述自动/controlled/真实远端范围分别通过，未把受控 backoff/耗尽/坏对象/宿主重启当作真实故障矩阵。真实 Web 服务进程重启未取得通过（有原驱动失败记录），实际 browser 文档重开已验；本机 Orca、上游完整 Controlled/L2/local+remote 原生矩阵、Go Full 及人工 SLW-M01 均不宣称通过。前述 HTTP 边界/launcher 基线和历史 Defender 有效排除项不可读限制继续保留。定时任务保持 PAUSED，不自动合并最终 SteerLine PR。
