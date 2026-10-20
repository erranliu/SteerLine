# Runner generate/1 对接与最简测试管理 Plan

日期：2026-10-19；2026-10-20 审查后更新。依据 [Spec](test-management.spec.md)，配套[专属验收](../acceptance/test-management.md)。普通开发，不进入 roadmap；不启动其他聊天或部署。R1 已接入，Runner R2 已合入并在当前 `bf3ec58` 发布；本批补本仓结果消费、换代接线和完整真实闭环，结果只登记 acceptance。

## 1. 调查与实现顺序

已调查当前 `runnercontract/{input,views,json}.go`、`runnerclient/{client,catalog}.go`、`task/{documents.go,model/model.go,model/contract.go}`、`taskrunner/service.go`、`environmentcheck/service.go`、`storage/ports`、`product/{api,query,httpcontract}`、`cmd/{steerline,test-feedback}`、Web runs/Query 和分级/调试脚本。当前没有完整 Plan 生成业务，`RunInput` 仍为开发专用；common/acceptance refs 的 Markdown 门槛不能直接接 JSON，但既有 PlanRef 已提供推导旁置 JSON 所需的路径和 commit，不必新增 Task 字段。环境检查的“非 Task 持久请求/公开 Runner 快照”模式可参考，不复制其状态机。

R1 历史验证固定 Runner `9586ac4`。本次核对 `bf3ec58` 的 `run_evidence`/schema：`.steerrunner/artifacts/`、discover/collect/list/read 和 workspace-git 已交付；初始审查时本仓未消费，按步骤 4 补齐。本批不接 conversation 操作。2026-10-20 的 rebase、构建和定向结果只登记 acceptance。

### 步骤 1：固定跨仓契约，扩展共用客户端

1. 与 Runner 交接 R1、R2 的正式公开 schema/类型、样例及来源语义，记录固定版本。R2 只要求通用的原 Run 报告/日志收集与公开读取，以及当前 commit/clean 观察，不把测试业务下沉到 Runner；封套内执行编号/顺序、定义和选择由测试入口记录。缺出口由 Runner owner 补齐，本仓不读私有状态。此交接属于实施前置，不另拆本仓 Spec/Plan。
2. 在 `runnercontract` 只保留 generate 实际消费的公开值，`runnerclient` 按 Runner 发布的 workflow/schema 验证输入，复用 submit/inspect/原请求、结果和 stop。capabilities 中出现 conversation 不能导致 catalog 拒绝，但本批不建设它的消息/历史/中断/关闭客户端接口。
3. 生成能力不满足时只禁用生成入口；原开发配置 schema/准入仍独立成立。沿 app 的共享服务/build pin，从指定本机 Codex profile 复用 executable/model/effort，按正式 schema 固定 read_only 和 generate workflow，排除开发权限、Git 发布认证、audit 和 prompt overrides；保存派生后的真实请求，不原样复制 profile execution 或修改其默认值。
4. 收集 R3 的真实构建和 schema 到专属验收后才启用真实联验。fixtures 只用于测试，不增加生产样例加载或协议猜测。

### 步骤 2：文档模型、Task 固定与本仓清单

1. 增加窄 `internal/testmanagement` 业务模块，拥有六字段清单、可选 note、ID 安排、结果正文解析和纯汇总规则；它不启动程序、访问 Runner 原生协议或直接依赖 SQLite。沿用严格 JSON/路径规则，确需共用解析时复用合适的纯实现，不让 Task 纯模型依赖业务服务。
2. 在 `taskrunner/service.go` 原候选材料组装处，从 PlanRef 推导安排路径，在同一 commit 读取它和清单，选择/定义/摘要或诊断与候选预留一并持久化，只把实际存在的文档附加 Runner references。必要的普通 Git blob 读取复用 app 文档端口，JSON 由业务模块校验；不改 TaskContract、批次格式、common/acceptance refs 或 Task CLI 参数。
3. 建立 `docs/testing/test-list.json`，首批登记 cmd/test-feedback 的 L1 和现存 L2/L3/L4 入口，核对 E2E ID/命令选择；L1 为可整体报告入口，不逐个登记函数。Web/脚本明确未接入，不为完整适配扩展本批。更新 `docs/testing.md`、E2E 清单链接和同步维护规则，不另建执行选择来源。
4. 未关联、空安排、无效安排和真实结果未完成分别投影；无效 JSON/未知 ID 保存诊断，不新增 Task 领取/启动/交付门禁。不回填历史候选、不热改活动材料；新增测试先在 Plan 和清单中声明目标定义，实施落实入口后才有通过资格。

### 步骤 3：生成用例、CLI 与持久保存

1. 实现 Spec 的 `tests generate/generation/stop-generation` 项目原生 CLI，经现有应用项目客户端调用用例；原生路由复用请求认证/实例规则，不顺带向浏览器开放生成写接口。输入路径限定项目内普通文件，指定固定本机 execution profile。
2. Store 端口/SQLite 保存原项目请求、Runner 映射、固定输入和目标原内容条件、生成结果与保存结论。持久化先于提交；重启只读取原对象，不创建替代 Run。复用当前请求身份方法，不另建通用作业平台。
3. 从现有 Plan、固定 `docs/testing.md` 和清单构造 inline 材料及 JSON Schema，指令限定按 Plan 选择最小必要入口。校验非空 ID 后有条件保存同目录 JSON；模型空列表只返回待人工核对，不覆盖文件，不追加模型审核。写前核对输入/目标，串行保存并原子替换；写入后回执丢失按原操作和摘要查证。重启后显式以原 generate 请求完成已生成结果的保存，查询仍只读，不重发模型输入。CLI 返回固定路径，由作者维护 Plan 链接和正常提交，不自动改写 Markdown 或 Task。
4. 观察/完成接入项目现有生命周期，不靠浏览器轮询推进文件写入；查询无新输入。生成不创建 Candidate/Task。停止、迟到绑定、应用关闭和整包 reset 均覆盖生成对象；最终结算只认原 Runner 事实。
5. 对齐 rebase 后主干的兼容构建接管：独立生成记录复用现有公开 takeover 证明核验原 Run/input_digest 或原 submit request/digest，保持原材料和 Runner 历史 pin，不直接放宽为任意新构建。将已持久取消/停止纳入启动前原 stop outbox，避免只处理 Candidate/Check；补生成对象的兼容换代、错证明拒绝及取消优先定向测试，归 TM-A04，沿其同一测试命令和证据位置，不另建恢复状态机。

### 步骤 4：结果输出、回传与查询

1. 扩展 `cmd/test-feedback` 与 `internal/testkit/feedback` 的实际报告，输出最小公共封套及四字段结果。保留原 summary/JSONL/日志，复用实际选择/退出/complete 与 Git 版本记录，补执行 ID/顺序、定义摘要及前后 commit/clean。按实际分级/场景/环境映射登记 ID，定向子集和 gate-only 不冒充整体通过；`-out` 等路径参数仍使用原约定。报告写到原隔离证据目录，失败也写报告；不复制测试实现、不改 exit code，不适配全体 Web/脚本。
2. 接入已发布 R2 的 capabilities/schema、公开值和 CLI allowlist；在已有候选观察路径 discover/list，并先持久 collect 原请求再外部调用。未知回执只查证原请求；正常重启不重复收集同一来源，stale_revision 明确未受理后才按新观察规划新请求。read 核验原 Run、artifact ID、长度、base64 和 SHA256 后解析业务报告，保存不可变结果及日志公开引用。测试入口显式使用原工作区 `.steerrunner/artifacts/<执行ID>/` 下的新目录并有项目忽略规则；按 R2 的单文件 1 MiB、每 Run 64 项/16 MiB 约束选择必要报告/日志，不上传全部无关输出。SteerLine 核对计划、定义、环境、选择、实际日志及前后干净同 commit；当前性采用 workspace-git 的 verified/commit/clean，未知不能按 false/null 的默认值判通过。无新增测试触发、指纹或源码监视。
3. `storage/ports`/SQLite 持久保存不可变报告，源键去重/CAS拒绝冲突；当前行按原运行顺序和源码/定义/计划取最新，迟到记录只能加入原历史。存储只返回事实，业务汇总归测试用例/查询层。
4. `product/api/query/client`、HTTP handler/allowlist、HTTP consumer 和 Web typed API 增加候选测试只读查询。证据引用使用正式公开读取/摘要，不能生成任意本机路径访问接口。

### 步骤 5：执行页与文档收尾

1. 在 `web/src/features/runs` 增加窄 `plan-tests.tsx`，由 runs 当前候选引用。Query key 与在线/可见刷新沿现有约定；服务计算汇总，前端不读取清单或判定通过。
2. 展示互斥计数、行详情、未关联/空计划、未完成/待复验、历史和证据；离线显示最后观察，切换不串结果。无运行按钮、新测试页或门禁。
3. 更新 Runner 集成与当前架构的实际实现说明、测试规范/入口、debug-template 中本次需要的最小材料。按 worker-lifecycle 的增量矩阵维护缺口；不能提前把方案写成已验事实。
4. 执行下列最小验证，在 acceptance 逐项登记版本、真实结果、未验项、失败与清理；不把结果写回 Plan 或测试安排。

## 2. 测试计划

全部自动验收项链接[本任务验收](../acceptance/test-management.md)。下表 TM 是本批验证分组；定向 Go 用例采用 `TestTM` 前缀。L3 live 场景和浏览器驱动在步骤 5 注册并落实，运行前必须查到非零匹配。证据根使用新的系统临时目录或本次独立 debug 包 `evidence/test-management/`，原日志不整批入库。

| 验收项 | 级别 / 组 / 场景 ID | 环境与命令 | 时点与通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| [TM-A01](../acceptance/test-management.md#自动验收) | L1 定向 / `TM.runner-contract` | 无模型；`go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/taskrunner ./internal/app -run '^TestTM' -count=1` | 步骤 1 后；实际公开 schema 正反样例、开发 profile 映射 read_only 正例、开发字段不泄漏，catalog 接受存在的 conversation 描述，开发准入保持、缺能力零回退 | TM-A01 / `contract/summary.json` 及原请求断言日志 |
| [TM-A02](../acceptance/test-management.md#自动验收) | L1 定向 / `TM.documents` | 隔离 Git/JSON；`go run ./cmd/test-feedback -- ./internal/testmanagement ./internal/taskrunner ./internal/app -run '^TestTM' -count=1` | 步骤 2 后；严格字段、重复/未知 ID、同 commit 推导和候选预留原子快照；缺失/无效/空分别展示、缺失引用不送 Runner，Task 无新增门禁 | TM-A02 / `documents/` |
| [TM-A03](../acceptance/test-management.md#自动验收)、[TM-A04](../acceptance/test-management.md#自动验收) | L1 定向集成 / `TM.generation` | Runner 公开 CLI fixture、临时目录/SQLite；`go run ./cmd/test-feedback -- ./internal/testmanagement ./internal/app ./internal/storage/sqlite ./internal/product/httpapi ./internal/product/applicationhttp ./cmd/steerline -run '^TestTM' -count=1` | 步骤 3 后；固定规范路径、空生成/坏ID/保存冲突不覆盖，写后崩溃及显式继续保存零模型重发；取消/迟到/未知/停机及持久化零提交，查询无副作用 | TM-A03/A04 / `generation/`，含提交/停止计数 |
| [TM-A05](../acceptance/test-management.md#自动验收)、[TM-A06](../acceptance/test-management.md#自动验收) | L1 定向集成 / `TM.results` | 实际报告正反样本和公开 Runner fixture；`go run ./cmd/test-feedback -- ./cmd/test-feedback ./internal/testkit/feedback ./internal/testmanagement ./internal/runnerbridge ./internal/storage/sqlite ./internal/product/query -run '^TestTM' -count=1` | 步骤 4 后；分级/场景到ID映射、子集/gate-only不冒充整体；原退出码保持，前后干净同commit才计当前通过，选择/环境/定义/清理/日志不足拒绝通过，错配/冲突/乱序/补测/版本失效及正式查询 | TM-A05/A06 / `results/`，实际执行输出与消费前后记录 |
| [TM-A07](../acceptance/test-management.md#自动验收) | Web 定向 / `TM.execution-view` | Vitest、隔离 QueryClient；`npm --prefix web test -- src/features/runs/plan-tests.test.tsx src/features/runs/runs.test.tsx src/shared/api/queries.test.ts`；API 断言并入 `TestTM` 定向用例 | 步骤 5 后；计数/展开/证据/空与未关联/待复验/最新失败、离线/跨项目候选切换/迟到响应；无调度/门禁 | TM-A07 / `web/`，请求计数/渲染断言 |
| [TM-A08](../acceptance/test-management.md#自动验收) | L3 / `testing` / `testing.management-local-codex` | 隔离 debug 包 + 固定真实 Runner/Codex；`go run ./cmd/test-feedback -level=L3 -suite=testing -env=live '-scenario=testing.management-local-codex'` | R3、上述受控项通过并构建后；正式 CLI 真实生成和保存、同一隔离候选上真实通过/失败测试→Runner出口→产品API→浏览器可见；核对原来源/包归属及结算 | TM-A08 / `live/`，Runner/build pin、生成源/result、两个报告、API/浏览器断言和清理 |

### 必须补齐的验证能力

TM-A08 的远端收尾补充：正式停止/verify-stop 后，按本次原 Run 的公开 `execution.branch` 和固定测试 commit 核对 head/base，先查询 head 的全部 PR 历史；仅无 PR、原 commit 精确匹配时用 lease 删除并补查为空，变更/未知保留并失败。后验入口为 `STEERLINE_TM_CLEANUP_ROOT=<已结算本包>` 配合 `go test ./tests/e2e/internal/driver -run '^TestDebugTestManagementOwnedBranchCleanup$' -count=1 -timeout=90s`，只补原包收尾，不重新调用模型；必须取得新 verify-stop 原 Run/input/settled 证明再清理。场景步骤末尾及发现遗留后执行，证据登记 `owned-head-cleanup/summary.json` 和包内 `test-management-owned-cleanup/`，不回写历史 proof。

TM-A04/TM-A08 的控制回执未知补充：驱动在 resume/start 提交前写入原 `ControlInput`，只提交一次；公开错误 `request_receipt_persist_failed` 后仅按原 request_id 查询权威回执，明确拒绝不按未知处理。新增正反例 `TestTMControlReceiptFailureQueriesOriginalWithoutResubmit`，并入上述定向命令的 `-run 'TestTMGeneration|TestTMControlReceipt|TestDebugPublicationReusesPR'`；Windows 隔离 fixture，在真实补验前验证，必须一次 Submit、原 ID Query、无替代请求；证据登记专属验收 `control-reconcile`。

TM-A04/TM-A08 的真实生成对象换构建补验：先固定同源码、不同版本/SHA 的 Runner A/B，运行 `scripts/test-test-management.ps1 -SteerRunnerExe <A绝对路径> -RunnerCandidate <B绝对路径> -Out <新目录>`。入口统一创建新的 live 调试包；正式生成保存后，用生产公开预检和 verify-restart 证明原服务退出/候选兼容，再更新本包 Runner 传输 pin、启动 B。确认原 Generation/Run/input/final/result 不变，随后沿原真实场景完成程序通过/失败→报告/API/浏览器及正式停止/资源结算。源码定向验证为 `go test ./tests/e2e/internal/driver ./internal/testmanagement ./internal/app -run 'TestTMGeneration|TestDebugPublicationReusesPR' -count=1 -timeout=120s`，在驱动完成后、真实场景前执行。结果登记到专属验收，包内 `evidence/test-management/generation-takeover*.json/log` 保存换代证据，失败使用新包补验，不 reset 已开始的候选。

- 步骤 1 补 Runner 正式契约 fixture；步骤 2–4 增加以 `TestTM` 为前缀的实际用例并保证各命令非零命中。已有包不是新建测试框架；可合并同场景多验收项的断言。
- 步骤 5 新增 `web/tests/test-management.spec.ts`，扩展现有隔离浏览器驱动，让 L3 场景使用本次包的稳定 Web 地址和项目身份调用该文件；不默认执行全部 Playwright 场景。补充受影响 API/HTTP/client 边界守卫与严格 JSON 反例。
- 在 `tests/e2e/suites.json`、E2E 内容清单和 live 场景支持/证据校验中注册 `testing.management-local-codex`，不只写场景名。Driver 必须先执行 `scripts/new-debug-release.ps1`，显式固定 R3 的 Runner；AI 模拟另用新包 `start.bat --simulate`，不能用模拟结果顶替本场景。
- 真实场景复用原开发候选公开结果链，驱动在 SteerlineTest 本次专属分支运行确定性通过/失败测试；失败反例为隔离测试输入，不提交故意破坏的产品代码、不新增测试执行开关到生产流程。只在固定现场证明闭环，不要求因此创建/合并 PR；若已有驱动必须触发 PR，仅允许 `erranliu/SteerlineTest` 的本次专属 base/head，并核对清理。
- 跨仓出口缺失时先固定 R2 并由 Runner 补齐，R3 不足则保留真实项待验，不能降格为 helper/Agent 自述通过。本仓契约/用例开发继续进行；功能交付仍以 R3 和 TM-A08 为必要前置。

不默认选完整 L2（会额外运行完整开发/PR交付）、无关 L3、系统组合 L4 或 Full。原生取消、恢复、权限和会话矩阵由 Runner 验证，本仓只测调用方原请求、来源、事务和消费边界。按受影响范围定向回归原开发客户端/Task/宿主，前端接线后执行 `npm --prefix web run build`；构建仅为编译检查，不替代自动验收。新增纯业务模块纳入固定 L1 最小集；真实闭环仍须满足 R2/R3，不用定向 fixture 或编译替代。

## 3. 人工安排与交付

[TM-M01](../acceptance/test-management.md#人工验收)：自动闭环后在同一隔离包提供页面，由用户体验中文文案、信息密度、折叠和日志可读性；窗口待定，可后置，不阻塞无关任务。自动来源/计数/切换失败不能归到人工项豁免。

交付说明包含最终公开字段与调用示例、清单/安排/结果格式、实际 Runner pin、定向与真实证据、未验和残留。本次一套 Spec/Plan 统一管理 Runner 接入和测试业务，不再拆成本仓两个互相等待的任务；Runner 自身实现及验收仍归其仓库。实现、执行结果、提交/PR/合并和部署事实分别登记，不因方案完整宣称功能完成。
