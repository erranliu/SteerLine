# Plan：按项目 × 运行机 × 链路组织环境准备

2026-10-31。对应 [Spec](../spec/project-environment-workflow.spec.md) · [审计与专属验收](../testing/acceptance/project-environment-workflow.md)。状态：已按用户后续授权实施，自动工程验收完成，人工体验M01待走查。普通开发由当前聊天执行，不启动 roadmap，不部署安装环境。

## 1. 基线、范围与交付

调查源码为 `e1968de2d8031c834bb5f35dff8a37a0a41b6a62`。实施前重新固定实际源码 SHA、Spec/Plan 版本及 Runner 公开契约；不得将本轮文档检查视为实现通过。

本次同时修改目标材料归属、生成请求绑定和界面，不能按只读 UI 工作处理。交付按环境派生的固定材料目录、分离的生成/验收输入、受控材料读取与历史查询、所选环境工作区，以及受影响提示词/夹具和验证。环境键算法、ready 判定、原三轮预算、同目标占用、Task 门禁和 Runner 接口不重建。

本聊天对初稿的审计和修订见专属验收第 1 节。固定模型正文与接线已修改并独立审核，最终差异、字节数、批准及运行结果见专属验收第 7 节，不用文档自审代替正文审核。

## 2. 变更落点

| 现有归属 | 计划变更 |
| --- | --- |
| `internal/environmentadaptation/model` | 复用原 Key/Context，集中按完整键派生材料路径；manifest version2 严格绑定键、脚本与依赖；分别定义 Generate/Start 输入及目标来源字段 |
| `internal/app/environment_adaptation.go`、`environment_readiness.go` | 统一构造目标公开事实、期望上下文及有限准备依据；生成选择与目标环境分离；所选环境读取，不再返回所有环境的整份文档 |
| `internal/environmentadaptation/service.go`、`generation.go` | 在原请求冻结/观察流程中保存不可变生成目标及来源；输出验证后才标 generated；保留原配额、取消、未知和停止义务 |
| `internal/storage/sqlite/environments.go` 及当前引用相关测试 | 校验新增目标来源的不可变性、键与 execution 一致性；持久 payload 按项目规则升版；历史筛选在截断前，不能过滤生命周期全量扫描 |
| `internal/app/environment_routes.go`、应用端口、`internal/product/httpcontract/routes.go`、应用 HTTP/客户端及 `web/server` | 新材料只读路由、历史 environment_key 查询参数、独立严格生成 payload 与验收目标复核，贯通 allowlist 和代理 |
| `internal/prompts/*.md`、`materials/*`、`prompts.go`、对应黄金样本/正式镜像 | 将固定全局路径改为从目标键派生的文件引用；修改 manifest 及输出约束，固定规则仍只由同包组装；禁止业务代码补规则 |
| `web/src/features/environment-checks` 及对应样式、任务准备链接 | 运行机/链路选择区、所选环境详情、生成与验收各自的方案选择、上下文导出、材料预览及本环境历史；保持任务返回和原独立停止 |
| `tests/e2e/internal/testenv`、`internal/driver`、`internal/testkit/remotetest`、调试模板/初始化与 owner 夹具 | 将旧固定路径、manifest、命令负载和真实证据定位改为从原 Go 键入口得到的路径；受控数据不冒充真实 ready |
| 测试清单、现行操作文档及 Spec 导航 | 新真实生成场景登记；同步布局、接口和验收边界，保留旧版本历史证据 |

不新增通用文档平台、环境注册中心、任意文件导出接口或替代执行适配器。完整 execution 留在原内部冻结记录，公开查询和 Git 上下文均使用显式允许字段；未知 settings 只参与原键，不透传。

## 3. 实施顺序

### P1：对象、材料及固定上下文，覆盖 A01、A03–A05

1. 在原纯模型包集中实现严格完整键校验及 document/script/context 路径，调用方共用。manifest version2 接收预期键并校验 script 精确路径、解释器及依赖；禁止引用其他环境专属目录、绝对路径、路径穿越和未知字段。维持原只读 Git regular blob 读取及原字节摘要，不接受 symlink/LFS 指针当文件正文。
2. 对生成与验收分别定义输入类型，生成仅 target_environment_key 与 generator_profile_id；验收为 environment_key、profile_id、round_timeout_seconds、expected_current_revision。HTTP 外层项目/version 沿原语义；JSON 缺失/null/未知字段分别明确拒绝。
3. 在原 app owner 定义唯一公开事实构造函数和确定性上下文编码：version=1、project_identity、environment_key、execution、publication、previous；execution 仅用公开 target/adapter/Agent/入口/权限事实，publication 仅用公开目标和非秘密引用。原始 args、env 值、transport/settings 和认证正文不导出。项目固定提交和文件摘要留在回包/活动中，不写入文件自身；无前轮为 null。
4. 前轮按同项目同键最近已结算有轮次验收活动选择，复用完整 PreviousFacts；不选运行中活动。期望上下文预览不得依赖待生成输出，缺同源规则或测试输入则列具体前置。严格核对导出后已提交文件；source commit 由原 repository 入口冻结，不做自引用校验。
5. 原 materialAt、查询和候选依据读取共同使用选中环境路径，只把该环境正式文件、共享规则/测试/明确依赖纳入原依据。生成上下文、模型和其他环境文件不纳入 ready 依据。

完成条件：路径/manifest/公开事实/首次导出及 A/B 依据隔离的 owner 正反例通过。此时不以临时兼容读取旧文件来过渡。

### P2：原生成活动、请求及查询，覆盖 A02–A06、A08–A09

1. 重构现有 PrepareGeneration 的具体输入，独立解析目标开发环境与生成方案。目标仅由加载配置及原键核验；生成 execution 沿 GenerationExecution 的本机 Codex read_only 投影和原 Runner catalog 验证，不借目标权限或扩展远端 generate。
2. 保持先查原信封/活动再验证新请求的顺序。保存目标键、目标内部上下文及公开来源摘要、源提交、生成完整 execution 和原 Runner 请求；SQLite 对新增绑定实施与原 execution/Prepared 相同的不可变检查。generation 不写当前资格引用，不占用验收轮次，不改旧 ready。
3. 固定材料集合使用同源共享规则、所选 context、正式测试规范/清单及有限源码/依赖；environment.md 和 verify 脚本只在存在时读取为参考，不作为首次生成前置。材料总字节上限沿原 1 MiB；缺必读项具体阻断，不补 inline 正文。
4. 在原生成观察收尾中严格校验结果结构及所选环境 manifest/路径。错误输出保留原 Run/final 与诊断，活动失败而非 generated；合法 gaps 保留并可见。停止、结算及配额释放仍按原执行事实，解析失败不能冒充资源已停。
5. 新增 `GET /environment/materials?environment_key=…`，提供所选正式材料、生成前置诊断、期望上下文字节/路径/摘要及观察来源；仅原项目提交观察，零 fetch/执行/写入。options 仅含列表及公开生成方案字段。
6. `/environment?environment_key=…` 在原项目记录中先筛选再取20；无筛选保持项目摘要入口。环境过滤不可用于 Observe、原请求查找、启动 census、pending stops 或 reset，旧目标移除后依然可按原 ID 观察/停止。材料查询和历史都贯通原端口、共享 allowlist、应用 HTTP、客户端、Web 代理，未知/重复/非法查询参数按固定契约拒绝。
7. 只变更活动 payload 和必要格式版本，不建第二份 ready 或新注册表；同步当前 SchemaVersion 及格式单一来源说明，旧格式明确拒绝，不迁移现有安装库。

完成条件：原请求重放/冲突、错目标零提交、不可变更新、原配额/未知/取消及20条截断反例通过，HTTP 全路径可达且公开 DTO 无原 settings。

### P3：固定正文、正式材料及所有读取者，覆盖 A03–A06、A11–A13

1. 在 `internal/prompts` 更新 environment-generate/prepare/publish 固定短模板，仅接收原提交、目标键及集中派生路径等少量定位事实；完整行为仍在 materials 的生成/执行规则。更新同源镜像、生成输出约束、manifest2 声明和实际请求断言。
2. 记录每份固定正文及代表请求的修改前后原字节、UTF-8 字节数和完整 diff，说明替换旧路径与目标不明规则的业务依据。独立审阅者检查实际正文、路径、允许字段和接线后批准；有修改则对最终版本重新审核。不得用黄金样本或摘要快照更新代替批准。此审核是必要交付项，实际独立批准及版本见专属验收第7节，文档自审不能替代它。
3. 修改 app/model/storage/HTTP owner 夹具、controlled_environment、native_environment、remote_environment 和外部 PR/L2 取证读取者。准备文档、脚本、PR acceptance 段都从同一键目录读取，项目完整 L2 及原验证 base/PR/SHA/清理标准保持。
4. 调试包按最终冻结 execution 先调用原 Go 键/路径函数，再在独立包项目初始化中准备版本化夹具；可在原 debug-helper 初始化落点补具体逻辑，不在 PowerShell 重写哈希算法。模板保留可复用的项目入口/脚本资产，不能硬编码安装机目录摘要。夹具提交只属于独立测试项目，不成为生产运行时自动写仓库能力。
5. 根项目示例、调试说明和客户端协议同步新文件布局、命令和查询参数。清理当前消费者对旧全局 DocumentPath/GenerationContextPath 的执行依赖；历史 Spec/验收及冻结旧输入保持原文，不批量替换历史记录。新 Run 缺新格式即阻断，无旧路径 fallback。

P3 依赖 P1 的固定格式及 P2 的实际接线，完成后才运行真实生成或验收，防止用未受审提示词开展 live 验证。

### P4：所选环境工作区，覆盖 A01–A02、A06–A10

1. 页面先按运行机列链路，按任务 profile 或有效默认定位，普通无定位时要求选择；保留原 profile/task 返回路径。所选环境由原完整键标识，列表不展示模型级条目。
2. 详情按状态→材料与要求→用户准备→验收→结果/历史组织。调用新材料 Query，key 包含 home/project/instance/environment_key；读取失败和生成输入待维护分别展示。期望上下文可预览/导出，文档/脚本不存在仍可引导首次生成，导出操作不写库或自动提交。
3. 生成区固定显示材料目标，单独选本机生成方案；验收区只选本环境方案并显示模型/强度、超时、三轮预算及真实副作用。禁止目标随生成模型变化，保留原双击/pending/原请求核实，不新造命令仲裁器。
4. 切环境清除本次未提交表单及旧详情选择；进行中的命令继续归原对象，不能通过切换或返回重发。切 home/project/instance 丢弃旧观察与操作草稿；迟到回执只更新原目标查询，不抢当前选择。
5. 本环境最近历史与“项目全部活动”分开，过滤先于截断；列表外原报告仍按 ID 可达。结果层级及原技术文件折叠保持。options 故障不封死独立可核实活动的停止；失效与停止分别使用原 current/activity revision。
6. 宽窄屏、键盘焦点、长名称及缓存观察时间沿现行界面验证。任务和派发面板继续使用原资格，不引入全局生成选择、浏览器持久状态或自动改默认。

P4 依赖 P2 的协议和 P3 的文案/输出约束。先补 owner 功能断言，再串行进行浏览器和生产构建。

### P5：验证、真实闭环与交付记录，覆盖全部 A 项及 M01 安排

复用现有 readiness 受控场景补足两环境生成→正常 Git 固定→验收→只放行对应组合；不得直接写 ready 或把 fixture 生成算真实生成。补真实生成专项后执行固定 L1/L2、受影响 Web、受控 L3 及最小 live 范围。失败保留首轮证据，修复后新目录补测，不扩大轮次或改标准。

实现完成时同步 Spec/Plan/专属验收的状态和准确范围，M01 单独安排。没有实际产品验证不能登记已验收；真实生成/新材料路径失败属于工程未完成，不能转写成人工体验待验。

## 4. 验证入口与证据

遵循[测试指南](../testing.md)、[路径清单](../paths.md)及[验收流程](../testing/acceptance/README.md)。建议独占 scope 为 `C:/SteerlineDebug/wt-68d4-project-environment-workflow/`；每次调用取新的绝对输出目录，命令中的 `<新…目录>` 在执行前替换并登记原命令。诊断日志放 diagnostics，正式输出按 L1–L4 分层，保留失败、补测和资源清理位置。

以下为实施验证安排，实际结果见专属记录。扩展测试沿原 owner 文件；新增真实生成场景已实装登记，不以命令文本或规划代替执行证据。

| Spec 项 | 步骤与主要验证 |
| --- | --- |
| PEW-A01、PEW-A02、PEW-A03、PEW-A04、PEW-A05、PEW-A06 | P1–P4 的 model/app/service/storage、正文及 Web owner；生成目标、首次材料和依据隔离 |
| PEW-A07、PEW-A09、PEW-A10 | P2/P4 的请求重放、协议、Web 和浏览器；串对象、先筛后限、旧目标、窄屏与查询故障 |
| PEW-A08、PEW-A12 | P2/P3/P5 的原生命周期 owner、正式 readiness L3；预算、未知、门禁及全闭环 |
| PEW-A11 | P3 的提示词自动项、字节账本及独立实际正文批准 |
| PEW-A13 | P5 的真实生成新场景及新格式本机正式验收，G/V 分别留证 |
| PEW-M01 | P5 后的人工体验，参与人与窗口待安排 |

| 时点 / 性质 | 验收与最小入口 | 通过条件及登记 |
| --- | --- | --- |
| 本轮及最终文档检查 | `python scripts/check_doc_links.py`、`git diff --check`；新增文件另查 UTF-8 和空白；脚本核对 PEW-A01–A13/M01 在三文档映射 | 零坏链接，ID 完整；只记文档检查 |
| P1–P3 后，model/service 自动 | `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/environmentadaptation/... -count=1` | 键/路径/manifest2/合法输出、坏目标及路径、首次生成、原预算/TTY/PR/封存回归非零命中 |
| P1–P3 后，app/storage 自动 | 分别用 test-feedback 新 out 执行 `./internal/app -run 'Test.*Environment' -count=1`、`./internal/storage/sqlite -run 'Test.*Environment' -count=1` | 在原文件扩展导出白名单、自引用排除、同键来源、原信封重放、来源不可变、依据隔离、20条截断及只读零调用断言；该表达式也覆盖 TestCallerPromptConvergenceEnvironmentMaterialPreparation；日志与结果登记专属验收 |
| P2 后，协议自动 | test-feedback 新 out：`./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -count=1` | 材料路由及查询贯通、认证/allowlist 保持、非法路径/参数拒绝，未运行产品的 UI fixture 不冒充真实 API |
| P3 后，正文与独立审核 | `go test ./internal/prompts -count=1 -v`；实际 owner 请求断言；原文 diff/字节账本及最终独立批准 | PEW-A11 分自动与独立审核两部分留证；生成/执行规则同源，缺文件不 inline，最终实际请求目标与路径正确 |
| P3 后，外部读取及登记自动 | test-feedback 新 out：`./tests/e2e/internal/testenv ./tests/e2e/internal/driver -tags=e2e -count=1`；另取新 out 跑 `./internal/testkit/remotetest ./cmd/test-feedback -count=1` | 原 PR/L2/工作区证据仍绑定同 Run/键，Codex/Orca 外部读取边界不变；新场景清单/等级匹配 |
| P4 后，Web 定向自动 | `npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx src/features/tasks/task-detail.test.tsx src/features/runs/runs.test.tsx src/app/project-switch.test.tsx` | 选目标、独立模型选择、导出/错误输出、切换/迟到/双击及任务返回行为正确；新增测试实装后非零命中 |
| P4 后，Web/HTTP及生产构建 | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-web.ps1 -EvidenceDir <新Web目录>` | 构建、全部 Web 及原 HTTP owner 通过；按脚本实际返回定位 server 受管报告 |
| P4 后，受控浏览器自动 | `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`；设 `STEERLINE_WEB_EVIDENCE=<新browser目录>`，finally 恢复原值 | 扩展原场景覆盖 PEW-A01/A02/A07/A09/A10，真实渲染宽窄屏/键盘/环境切换；list 日志、trace 和截图存独立目录 |
| P5，受控全链自动 | `go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness -out <新L3目录>` | 扩展既有场景覆盖 PEW-A04/A05/A08/A09/A12；两个项目/环境、先拒绝后准备再派发、失效及完整停止；正式调用不直接写库 |
| 代码修改后及交付前，固定回归 | `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`；`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 固定 `l1.minimum` / `smoke.delivery` 完整通过；skip/零命中/未知清理不通过；正文/夹具变化后按最终版本运行 |
| 独立正文审核后，真实生成自动 | 已实装 `testing.project-environment-generation`（L4/live/testing），入口 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-generation -out <新L4目录>` | PEW-A13-G：原生本机 Codex 给本机和远端两个不同目标生成有效材料；原请求、Git上下文、输出目标/路径、gaps 和停止结算核实，目标远端零验收/零派发，无须远端连接或 ready |
| 真实生成后，真实验收自动 | 既有 `testing.project-environment-local`：`go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <新L4目录>` | PEW-A13-V：使用前一场景的有效本机生成输出，经正常 Git 固定后完整 TTY/L2/真实原 PR发布，外部原规则/精确合并取证、准入探针和全部清理；生成缺陷须修生成来源，不手写通过脚本 |
| 可用构建后，后置人工 | PEW-M01 按专属验收走查，参与人与窗口待安排 | 能分清准备对象/生成位置/验收模型、找到要求和失败原因；单独登记，不覆盖 A 项失败 |

每个 owner 命令单独调用并检查退出码，不能用后一个命令成功掩盖前一个失败。

原始 Web 构建、npm 运行、调试包生成及同一源码的相关 L4 串行执行。L3 遵守总并发上限二；live 精确选择且单项串行。预先检查工具和输入，不能把超时、skip、缺证据或 fixture 通过算成 live 通过。

## 5. 真实链前置与新增场景

新增真实生成场景已在 `tests/e2e/remote` 和现有 testenv owner 落地，复用公开环境命令、generate 查询和原 Run 停止，不新增脚本直接调用模型的旁路。同步 `tests/e2e/suites.json`、`SteerlineFlow/testing/test-list.json`、`testing/l4.md`、执行器及对应清单测试；各次真实结果见专属验收，不以入口存在代替通过。

最小输入使用既有变量：固定独立 `STEERRUNNER_EXE`、本机 Codex `STEERLINE_L4_LOCAL_PROFILE`、远端目标描述 `STEERLINE_L4_REMOTE_PROFILE`、专属项目 `STEERLINE_L4_PROJECT_CONFIG` 及完整 `STEERLINE_L4_PROJECT_COMMIT`。新增生成场景将本机 profile 同时用于生成与本机目标，远端 profile 只用于加载并核对第二目标的公开事实；不调用其实际远端。两条目标按部署支持的原配置格式读取，不复制认证到项目或 Git。

生成前只在独立 clone 中维护共享同源规则和两个目标的固定上下文，项目已有正式测试及依赖；然后由产品实际生成。生成后审阅合法输出和 gaps，通过测试项目正常 Git 流程提交，再记录用于本机验收的最终完整 SHA；不得通过改验收标准弥补生成失败。验收还须预备包含最终源码的专属验证 base、原 PR provider 权限及 L2 所需工具。环境路径按当前 profile 的原键计算，不能复用另一包路径摘要。

本机真实验收另设正的 `STEERLINE_L4_ROUND_TIMEOUT_SECONDS`，沿原预算而非人为延长；真实生成保留原一小时活动期限和 home 生成配额，不新增超时策略。测试只使用独立 home、clone、数据库、固定 Runner 及专属测试 refs，不改安装目录、共享业务 checkout 或业务主分支。

Orca 本机/远端使用共同材料路径/格式读取和公开协议，由 owner 与受控反例覆盖；本次最小 live 结论只包含实际执行的 Codex，不自动宣称 Orca 新材料已原生验收。若实施改变 Orca 专属外部取证行为或执行契约，再先补其明确 live 验收，不以扩大全部模型矩阵替代定向验证。

## 6. 完成界限

全部 PEW-A01–A13 按专属文档登记最终实际结果；A11 独立批准、A13 真实生成和验收分别记录。缺工具、原生能力或外部权限时准确登记待验及阻塞范围，不降级为人工体验或历史继承。M01 可在自动项完成后安排，不用其后置掩盖已知工程失败。

本Plan原实施范围不包含外部通知、功能代码发布或安装部署。2026-11-01用户后续明确要求提交PR，现授权本次实现提交、推送独立分支和创建功能PR；不包含部署或自动合并。独立验收中明确列出的材料提交、专属验证 refs/PR 与精确合并仅用于 A13-V，不作用于业务主分支。涉及额外外部现场维护时按实际用户授权处理。

## 7. 实装中的验证收口

真实 G 首轮完成，但原始项目输入存在缺口，具体报告保留在专属验收。按其事实补齐了固定 Python 测试辅助源码、项目协议及公共要求的有限清单；没有增加目录扫描、命令猜测或旧格式读取。公共要求使用 `SteerlineFlow/environment-common.md`，生成执行仍使用新键目录，根 `environment.md` 只作人工文档导航。

固定规则明确原生快照按契约无 .git、来源由公开固定提交绑定，区分普通 publication base 与独立 validation base；旧环境入口接口由新生成脚本适配，不能改测试标准或伪造通过。实际正文、字节和源码守卫精确登记调整均已独立审核。G 的最终源码另以本次输出目录下的独立 clone 留存，供 V 固定相同生成字节，不能因正常临时现场清理而丢失输入。

PowerShell 执行带点的 scenario ID 时使用 `-scenario 'testing.project-environment-readiness'` 等显式引用形式，防止本机 PowerShell 将未引用的 `-scenario=testing.xxx` 拆成两个参数。Web 原检查脚本在 PowerShell 7 宿主执行，以避免 PS5.1 将成功回执的 stderr 转成终止异常；项目运行脚本仍遵守 Windows PowerShell 5.1 基线。

实现及自动工程验收已完成：最终10055字节规则经独立批准，G09两目标生成与V05本机完整正向流程通过，原失败与资源收尾见专属验收第8节。M01保持后置人工体验，不部署。
