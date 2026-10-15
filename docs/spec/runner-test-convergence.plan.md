# Plan：SteerRunner 拆分后的测试精简

日期：2026-10-15。状态：用户已授权实施。依据 [Spec](runner-test-convergence.spec.md)，验收归[本任务专属记录](../acceptance/runner-test-convergence.md)。普通开发，无 Task ID，不改 roadmap/execution-map；本次用户已授权 rebase 后更新文档并实施，包含必要的完整 L2 真实验收。

## 1. 实施前置与整合方式

已 rebase 到 `20f347cd`，保留四个 runtime 子场景与既有二并行策略。此前目标四项中的旧 runtime 聚合项改为四项，总目标七项；不恢复主干删除的 nativeworkspace 和旧隔离脚本。Runner 固定已提交 `af54556` 的独立源码副本（系统临时目录 `steerline-rtc-runner-af54556`），其他 Runner 工作区均有未提交改动，本次不在其中写入。

调查基线及已读规范版本见 Spec 第 2 节。实施开始时登记实际两仓 commit、工作树状态和所选 Runner EXE 的 version/SHA；重新核对测试增删，不能把本次规划基线当作未来构建来源。

继续复用 SteerLine 的 `runnerclient/runnerbridge/taskrunner`、SQLite 和 HTTP/Web owners、`cmd/test-feedback`、testenv/driver、Runner CLI fixture、隔离 Git/PR provider 及新包生成器。只把测试注入点从 Codex/Orca 协议边界上移到 Runner 公开 CLI，真实 L2 仍走现有产品链。不引入共享跨仓 Go 包、私有 IPC、Runner DB/日志读取或另一个执行入口。

规则参考 [worker-lifecycle](../product-design/worker-lifecycle.md)。实施时仅校准其验证映射/当前实现缺口中的测试所有者：W02–W16/W18 执行专项指向 Runner，W01/W12 调用方授权/W17 及停止义务指向 SteerLine，L2 保留正常路径跨仓覆盖。不得改节点、异常规则或复制矩阵。

## 2. 实施步骤

### 步骤 1：逐断言盘点并落实跨仓去向

盘点 `tests/e2e`、`internal/testkit`、普通 `*_test.go`、scripts、`cmd/test-feedback`、调试模板及当前文档中的原生依赖。将清单写入专属验收的迁移明细，覆盖全部 28 项注册场景和非注册探针，分为保留、并入、Runner 已覆盖后删除、Runner 补齐后删除、失效入口删除。

核实 Runner 的现有测试及执行记录。独有执行断言优先复用其 owner 测试；缺口在 Runner 对应包补最小场景并实际定向运行，再登记精确 commit/测试名/证据。此交接是完成条件，不用“交给 Runner 后续测”代替安排；不重新申请/执行已取消的 Orca 真机活动。未拿到 Runner 源码或证据时可继续 SteerLine 独立工作，但 RTC-A5 保持待验，不能声明全部完成。

确认 `delivery.basic` 的独有交付后静默断言归入 `smoke.delivery` 或现有 Task owner；`recovery.answer-replay` 等混合用例中的调用方断言归入步骤 2，不随原生测试删除。

### 步骤 2：先补最小公开 CLI 测试能力

复用 `internal/testkit/protocol/runner_service.go` 的子进程夹具，保留已有 idle-only 消费者的限制，增加显式选择的脚本化业务响应能力；不让空闲宿主用例意外接受 submit。受控 E2E 用同一夹具机制覆盖版本/能力、服务、提交/原回执查询、Run/事件页、control、PR 刷新和停止公开事实。保留真实 Git/SQLite/SteerLine 产品命令；PR fixture 可继续使用 loopback provider。

现有 runnerclient/bridge 测试优先复用；不同消费者需要的业务脚本保持最小，不强行统一所有测试为大型通用模拟器。夹具只记录调用并返回预设公开 DTO，不生成三轮原生工作流或解释 agent prompt。

| 调用方边界 | 复用/落位 | 必须保留的断言 |
| --- | --- | --- |
| 固定 EXE、公开 schema/响应、调用超时 | `internal/runnerclient` | 错版本/SHA 在调用前拒绝；异常 envelope/ID 不误接受；超时按公开语义处理 |
| 保存意图与未知请求 | `internal/runnerbridge`、SQLite runner dispatch/control | 保存失败零调用，提交丢回执后只查原 request_id/body，重启不生成新请求 |
| 公开操作与授权 | `internal/taskrunner`、现有 HTTP owner | 原 Run/题目/当前 revision 绑定；重复原请求幂等，过期新请求拒绝；不自动重放答案/反馈/resume |
| 投影和游标 | bridge、SQLite runner snapshot/reads | 重复页/迟到事件不重复副作用，失败不推进游标，错 Run/已绑定 PR 丢失或替换不变更原绑定 |
| TaskDelivery 与停止 | SQLite runner result/discard、taskrunner settlement | 唯一交付/后继释放/候选停止义务原子；迟到接受继续停止，not_running 或缺投影不误判原生已结算 |

已有测试缺某个必要断言时在其原 owner 补齐，保持普通用例不依赖真实 Runner/Codex/Orca。新增最小 E2E 夹具依赖守卫，允许 live 的公开配置装配，拒绝原生协议模拟器和 Runner 私有依赖返回测试闭包。

### 步骤 3：重接受控场景与合并重复项

改造 `internal/testenv/env.go`、`recovery.go`、`query.go`、driver 和必要 helper，通过公开 CLI 夹具驱动 Task。

- `smoke.delivery` 吸收 `delivery.basic` 的独有断言，删除独立 delivery 包/登记；核对相同 Delivery、后继只 ready、不再提交新业务。
- `recovery.waiting-merge-restart` 只重启 SteerLine，保持 fixture Runner 的原 Run 和公开 PR；核对候选/请求/游标、授权和 Task，删除原生 session/dispatch 次数等执行层断言。
- 四项 runtime 隔离场景保留主干拆分、二并行、独立证据及现有有效隔离门槛，将其受控执行改接公开 CLI fixture；测试中的安装环境仍是隔离构建产物。
- 删除其他 recovery/nativeworkspace/sandbox 场景的 SteerLine 执行入口。调用方故障边界由步骤 2 的较小测试承担。

最终 `suites.json` 精确登记 Spec 第 4 节七项。既有调试包 `--simulate`、冻结/停止/reset、Web smoke 和原生查看的产品能力不得误删；测试支撑若被共享，先替换其必需调用方，再删除无消费者的原生 helper。

### 步骤 4：保留并收口 L2 本机真实闭环

保留 `TestSmokeLocalCodex` 与 `smoke.local-codex` / `local-codex` 名称，复用现有真实项目和发布 fixture，删除 L3 live 交付/恢复及第二个双任务沙箱流程。L2 仍采用受控门槛→真实模型的固定顺序，不增加只跑模拟也能通过 full-L2 的选择。

保留 live 的最小可执行路径解析和公开配置装配；版本/原生前置判断交给 Runner。保持模型/effort 固定、原生登录从宿主引用，修正与“不得复制认证”冲突的现行指南。

将 `LiveDelivery` 的职责收口为 Spec 第 5 节：实际三阶段执行、公开 canonical final/阶段来源、业务产物的 Git commit 内容、精确真实 PR、合并后 Task/后继与清理。Git 内容检查通过提交对象完成，不依赖 Runner 私有工作区布局。删除 `NativeUsage`/rollout/进程注入/沙箱解析；公开来源字段作为 opaque identity 比对，不解析原生格式或重验 Runner 机制。

同时修改 `cmd/test-feedback/e2e.go` 的证据校验：真实 L2 必须有 Runner 固定来源、阶段/final、产物、精确已合并 PR、TaskDelivery 及清理证明。加入有意义的拒绝用例：缺阶段/final 或 Run 不匹配、错误产物、假/错仓库/错 base-head/head SHA/未合并 PR、缺唯一交付/后继事实、缺清理均拒绝。完整日志非零匹配、实际测试终态和 gate 失败阻断仍保留。删掉 usage 门槛时不能放松这些条件。

记录现有业务期限、父 Go 期限与独立清理预算并保持其边界；失败新建现场，不自动重跑。L2 真实 PR只针对本次 SteerlineTest 分支，driver 仅准备 base、查询、转 ready、合并及清理；任务产物及候选发布由真实 Runner/Codex 完成。

### 步骤 5：删除剩余测试、脚本与过期规则

根据步骤 1 的去向清单，清理 `internal/native`、`internal/controlled`、`internal/testkit/protocol` 及 driver/testenv 中仅服务执行专项的实现/测试。保留通用 process、Git/provider、Task/HTTP 夹具与步骤 4 所需 live 最小装配，不按目录名批量删除。

审计 `scripts/test-orca-state-transitions.ps1`、`probe-orca-*.py`、`test-codex-sandbox.ps1`、`setup-e2e-sandbox.ps1` 及 debug driver 的全部消费者；专项入口迁出/删除，混合入口保留 SteerLine 调试/隔离功能，真实完整运行统一复用 L2 driver，避免再维护平行验收链。

更新 `cmd/test-feedback` 的参数/注册验证/测试、L1 与 Full 包及用例归属、场景期限选择和证据字段，移除远端 manifest、Orca chain/native env 和原生专项开关。核对普通 Go 自动发现和带 tag 的依赖闭包，不能只删 manifest。

### 步骤 6：文档、验证及交付

更新 `docs/testing.md`、`tests/e2e/README.md`、recovery README、集成说明和受影响的调试指南/导航，写明七场景、完整 L2 命令/外部效果、gate-only、Runner 所有者及新证据要求。与旧任务 Plan/验收冲突时，增加明确的当前测试入口指向，不篡改原冻结契约或历史通过/失败/取消记录。

依下表执行最小验证，结果逐项登记专属验收，附迁移清单和净行数（含移入普通 `.go` 的测试基础设施）。两仓实际改动分别记录 Git commit/PR（如实施交付要求创建 PR），不自动合并、发布或把此任务加入 roadmap。

## 3. 测试计划

所有自动验收映射至 [RTC-A1～RTC-A6](../acceptance/runner-test-convergence.md#验收项)。原始日志使用 `cmd/test-feedback` 默认新临时目录或本次新建空目录，登记在专属验收对应项。表中新增守卫/夹具/证据拒绝测试由步骤 2/4/5 补齐后执行，不把尚无能力的入口算作已覆盖。

| 验收项 | 级别、组/场景 | 环境、命令与执行时点 | 通过条件与证据登记 |
| --- | --- | --- | --- |
| RTC-A1、A3、A6 | 定向 test-feedback 选择/证据拒绝 | Windows 隔离 fixture；步骤 4/5 后：`go run ./cmd/test-feedback -- ./cmd/test-feedback -count=1` | 七项精确选择、L2 完整两阶段/失败阻断、退役参数拒绝、缺证据必败；非零命中。A1/A3 登记 summary 与实际测试清单 |
| RTC-A2 | unit/SQLite caller owners | 临时库/公开 CLI 子进程；步骤 2 后：`go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner -count=1`；`go run ./cmd/test-feedback -- ./internal/storage/sqlite -run '^TestRunner' -count=1` | 原请求/授权/事件/事务/绑定/交付/停止边界实际通过。A2 登记包、用例、summary 与原日志 |
| RTC-A2、A5、A6 | 定向边界、driver/testenv/fixture | 无原生工具的隔离环境；步骤 5 后：`go run ./cmd/test-feedback -- ./internal/testkit/boundary/... ./tests/integration ./tests/e2e/internal/... -count=1` | 实际 retained owner/新夹具守卫通过，无私有依赖/原生测试链；删除原生 helper 后仍可运行。A5/A6 登记依赖及命中清单 |
| RTC-A1、A2 | L2 controlled，`smoke.delivery` | CLI/PR fixture、真实 SteerLine/Git/SQLite；步骤 3 后：`go run ./cmd/test-feedback -level=L2 -env=controlled` | 唯一交付/静默/依赖通过，coverage=gate-only，零真实模型。A1/A2 登记 scenario/summary/cleanup |
| RTC-A3 | 完整 L2，`smoke.delivery`＋`smoke.local-codex` | 最终保留代码，真实固定 Runner、本机 Codex 已登录、有 SteerlineTest 权限；步骤 4/5 后：先设置 `STEERRUNNER_EXE` 为选定绝对路径，再运行 `go run ./cmd/test-feedback -level=L2` | gate 通过后 live 非零执行；真实实现/报告/发布、产物/PR/merge/唯一交付/后继/清理全部满足 Spec 第 5 节，coverage=full-L2。A3 登记两阶段报告、EXE/配置来源与 PR URL/分支/commit 证据 |
| RTC-A4 | L3 controlled，waiting-merge-restart/runtime isolation（四项） | 步骤 3/5 完成后的实际提交、隔离夹具：`go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled '-scenario=recovery.waiting-merge-restart,runtime.source-build-isolation,runtime.path-isolation,runtime.service-write-isolation,runtime.dual-home-isolation'` | 五项均实际命中通过/清理；runtime 含从 HEAD 构建时必须绑定已提交代码。A4 登记源码与五项 scenario |
| RTC-A5 | Runner 专项归属核验 | 步骤 1 的 Runner 实际源码/对应最小 fixture；步骤 5 删除前，逐项在迁移明细登记真实包、测试名、精确命令并执行 | 已覆盖或本次补齐的独有断言有可追溯结果，不要求取消的 Orca 真机。命令尚待源码盘点固定，但补齐与实际执行由步骤 1 明确承担；缺资料时 A5 待验，不虚报迁移完成 |
| RTC-A6 | L1 固定集及编译 | 最终测试选择修改后：`go run ./cmd/test-feedback -level=L1`；`go run ./cmd/test-feedback -build-only`；`go test -tags=e2e ./tests/e2e/... -run '^$'` | 固定集通过且完整，普通/e2e 编译成功；编译不算场景通过。A6 登记三个结果及选择范围 |
| RTC-A5、A6 | 所有者/脚本/文档静态审计 | 最后执行：`rg -n -i 'orca|rollout|usage_verified|sandbox|app-server|steerrunner' tests internal/testkit cmd/test-feedback scripts`；`go list -test -deps ./...`；`go list -tags=e2e -test -deps ./tests/e2e/...`；`python scripts/check_doc_links.py`；`git diff --check`；解析保留 `.ps1` 检查语法 | 逐命中按归属说明，L2 配置中的 codex-app-server 等允许，专项无执行消费者/失效包；导航/差异/脚本零错误。A5/A6 保存审计清单与检查结果，不把 rg 非零匹配机械当失败 |

夹具消费者补充：`web/tests/smoke.spec.ts`、`project-switch.spec.ts` 和 debug runtime 测试原来读取原生 turns/process.json，随步骤 3/5 改成公开 Run/原请求记录；保留关闭页面、重启 Web、多项目、反馈及请求幂等断言，删除原生进程取证。保留 debug-template 发布脚本自己的 PR 复用测试。RTC-A2/A4/A6 最小追加 `scripts/test-debug-release.ps1 -SteerRunnerExe <本次固定EXE>`、新 simulate 包的 `scripts/test-web-smoke.ps1` 与 `scripts/test-unified-startup.ps1`（仅其现有两项目浏览器入口，不跑其他原生矩阵），按各脚本保存包内证据。必要共享夹具消费者发生重接，因此这里实际执行这两项已有浏览器场景，不另建组合矩阵。

必要的 HTTP 授权/投影断言复用现有 owner；如步骤 1 发现必须迁入或修改 HTTP 测试，在 RTC-A2 明细登记精确测试名及定向命令并执行，不扩张到整组 Web 浏览器验收。

不默认执行 Full：涉及包、固定 L1、七场景与闭包编译已覆盖本次测试重接；Full 调度选择逻辑通过其 owner 测试核对。确需全量才用现有三 worker 入口。其余 L3 live、Orca、原生恢复/沙箱矩阵由 Runner 专项承担，不在 SteerLine 重跑。完整 L2 是本任务必要的真实验收，不能以已取消其他原生目标、成本或仅编译为由省略。

## 4. 规划交付的静态检查

本轮仅新增 Spec、Plan、专属验收与导航；执行 `python scripts/check_doc_links.py` 和 `git diff --check`，核对验收项/测试计划映射及七场景与 L2 契约一致。结果只写专属验收的文档检查段，RTC-A1～A6 在实施与实际验收前均保持待验。
