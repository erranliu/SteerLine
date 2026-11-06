# 原生执行配置边界收口 Plan

日期：2026-11-05，状态更新：2026-11-06。状态：代码实施及本轮规定的自动验收完成，远端正向资格已补验通过；对应 [Spec](../spec/native-execution-configuration-boundary.spec.md)，验收与实时结果归[专属记录](../testing/acceptance/native-execution-configuration-boundary.md)。已授权在[Runner PR #122](https://github.com/erranliu/SteerRunner/pull/122)之后提交、推送和创建SteerLine PR；不分配Task ID、不改execution-map、不启动roadmap、不合并或部署，安装切换另行处理。

调查基线：SteerLine `9b2e5d886b8f56eaf07bc697ca78c578c91eec00`；Runner `a66038c89208392c47531e640c84e97728c62424`。实施前重核两仓实际基线与工作区差异，不能仅凭本文把当前新增接口视为已发布。

## 1. 现有能力与必要新增

已核对本仓环境 service/model、app 环境准备/资格/材料、SQLite 环境 current/预留、生成与 Roadmap 调用、Web schema/store/配置页面；Runner 固定提交的 `internal/cli/cli.go`、`internal/resources/configuration.go`、独立配置 Spec、Orca 执行设计和测试指南。

调查基线已有静态 capability_catalog、自包含配置 schema、validate 和原生目录入口；本轮已新增 describe/derive/compose。validate 判断给定输入，新有限无服务动作构造目标 execution 并提供可冻结的通用身份，复用配置描述和生产校验，不新增后台 owner、状态库或原生探测。

本轮源文件 owner 分工如下；Runner 纯规则位于 internal/contracts/execution_configuration.go，schema/能力和 CLI 分别由 internal/resources 和 internal/cli 组装。实际实现与分项结果见专属验收。

| 仓库 / Owner | 修改位置与成果 | 保留 / 删除 |
| --- | --- | --- |
| Runner：公开协议 | `internal/contracts/`、`internal/cli/cli.go`、`internal/resources/` 的 capability/schema/validation；新增 execution_configuration 纯值及固定命令 | 保留现有 validate/native-catalog；新增动作在服务发现前处理 |
| Runner：原生配置规则 | 复用 codex/orcatransport 的配置解析与生产 gate，将原生转换集中为无 I/O 的有限函数 | 原生字段映射只有一个 owner；不从 CLI 引入完整执行生命周期 |
| Runner：测试与文档 | `docs/spec/native-execution-configuration-boundary.spec.md` 及同名 Plan、专属 acceptance；组件与公开 CLI 测试 | 提案在 Runner 正式落地，同源组合/错误/identity/程序绑定有单测和受控实证 |
| SteerLine：契约及调用 | runnercontract、runnerclient、Web nativequery/executionconfig | 无服务调用与纯 DTO；不借 Client.Call 自动附加 state-dir/instance 参数 |
| SteerLine：环境身份/资格 | environmentadaptation service/model；app environment_*；storage ports/sqlite environment_current 与预留接线 | 删除 useNativeOrcaExecutor、原 Context 原生裁剪及 app 模型读取；保留资格纯规则和原事务 |
| SteerLine：内容生成 | runnerclient/content.go、app/test_management.go、app/roadmaps.go、app/project_materials.go 和 project_baseline.go 的生成资格入口 | 删除 GenerationExecution；自动预检与实际派生同源，保留各自内容验证、记录、额度与对账 |
| SteerLine：Web | executionconfig/schema.go、store.go、discovery；配置 API/前端/测试 | 删除样例试探、nativePrograms 字段解释、默认程序映射及前端 adapter settings 改写；保留 CRUD/条件写/目录并发 |

明确保留两类实际消费者：execution/model/runner_profile.go 及其 app/client、taskrunner/projection、query/runner_history 调用继续只读投影原冻结请求，不新增历史 CLI 依赖；scripts/new-debug-release.ps1、debug-template、internal/testkit/remotetest 的可信部署/测试组装按公开 schema/gate 核对，保留显式程序供给。两者不参与新的编辑/派生/资格规则，源码守卫不得误禁这些边界。

## 2. P0 · 两仓契约对齐（进入实现的必要前置）

1. Runner 独立 worktree 分支 `codex/native-execution-config-docs` 已按其 AGENTS 建立配套 Spec/Plan/验收草案，本仓 Spec §3 的共享 CLI 正文与其逐字同步。实施前仍需逐项签收操作、字段缺省/null、CanonicalJSON、source、错误/退出、1 MiB/30 秒和无状态边界，不能用文档同步代替公开实现/构建交接。
2. 使用同一组脱敏输入列出：合法源组合、execute/1 保持与适用性、generate/1 转换、原 Context 输出、可信程序来源、非法/退役配置。对当前共同接受的输入逐项确认保留和变化；现行支持取固定构建公开目录，DSH/退役 helper 历史映射不算当前支持。
3. 按 Spec §4.3 固定 compose 及 validate/save 的逻辑 edit 载荷：saved_digest、服务器已保存 base、selection/unset 与目标 workflow；逐 profile 精确覆盖新增/修改，未变和删除分别处理。新建/复制/多次草稿编辑保留来源，程序权限不得由浏览器输出或新的 response digest 自行授权。不引入签名票据或持久草稿服务。
4. 确认 identity 是源配置语义，不是执行就绪。确定活动/Prepared 的必需冻结字段与当前格式版本调整；记录旧安装由旧版本收尾、新版使用新 home 的操作边界，不执行清理或迁移。
5. 两仓契约与样本存在分歧先修文档，不在调用方补 native 特判。P0 退出条件为可执行且无关键未决字段的配套文档/样本；本轮已完成共享正文签收及固定公开 EXE 对照，后续变更仍按同一条件复核。

## 3. R1 · Runner describe/derive 与身份

1. 把现行有效 native 配置解析、默认值和组合判定整理为共用纯函数，既有生产路径与新增接口复用；用对照输入证明不改变 submit 的接受/拒绝、Workflow 或停止 scope。
2. 实现 describe 的最小逻辑 view、源 execution 摘要和通用 identity；不填造缺省模型/effort。两个阶段都不新增程序摘要、helper完整性描述或部署检查服务；不读程序文件或用户原生配置。
3. 实现 derive：execute/1 对当前合法 native 源通常原样返回，generate/1 做明确的 read_only 用途转换；两者都用同源生产纯配置规则拒绝退役字段，不能只调用宽松 ValidateExecution。原输入不可变，目标 adapter/target/Agent 不换，权限由请求明确指定。完整 Run 文本、materials、publication 和 workflow 内容不由接口构造。
4. 在 CLI parser/dispatch 增加固定无服务入口，复用严格 JSON、错误与编码预算；capability 的 operations 先仅发布 describe/derive。静态声明/schema 与真实实现由同一配置描述生成。
5. 补组件和公开 CLI 受控测试。构建后记录 commit、version、EXE SHA256、resource_digest、operation/组合目录、schema、正反例及零业务副作用证据，作为 S1 输入。没有固定 EXE 不切本仓生产路径。

## 4. S1 · SteerLine 派生与环境身份接入

1. runnercontract 增加 schema/身份/响应纯值；runnerclient 增加固定 stateless 配置调用与 pin/摘要验证。可选 capability 解析错误单独保存为该功能诊断，不阻断已有 Run 的正常查询和 stop。
2. 后端按 home 固定配置建立 describe 快照，按 source execution digest 去重，项目借用不可变值。纯身份计算与视图读取只消费已取得快照；新准入缺快照时给具体原因，不降级本地解析。
3. Prepared/活动保存原 execution、source identity 与派生结果。替换 service.appendRound 的手工转换；实际原请求持久化后才发送。app 的 materialAt/basis、environment_readiness、environment_materials、environment_basis、路由读取和 view 统一消费相同冻结数据。
   调整现有 prepare 中的 ValidateEnvironmentExecution 顺序：先核对源配置及公开转换资格，再 derive，最后对返回的 execute/1 execution 验证结果封存/单次执行能力和完整请求，避免目标 gate 在转换前错误拒绝合法源方案。生成同理，不能先把开发权限当成 read_only 输入校验。
4. model.Key/Qualify 改为校验 identity 的纯计算；StartEnvironment 及 ReserveRunnerCandidate 的环境资格路径在原事务比较来源 execution 摘要和 identity。准备阶段外部查询和事务内复核分别保留，不在 model/sqlite 调 CLI。
   列出所有实际调用点并一并接线，包括 record.Execution 的历史纯比较、candidate.Environment 预留证据和 configDigest 核验。各份 identity 只核验自身 source execution 摘要；活动与候选跨对象比较 context/key/basis，不要求两边源摘要相等。补模型/effort 变更后仍准入的正例，以及伪造自洽 context/digest 无法准入的反例。HTTP 不接受 identity 入参，Store 不把客户端 key 作为权威。
5. 同时升级必需的持久记录格式，旧记录缺 identity 明确拒绝。测试使用全新隔离 home；不能新增旧字段回读、重算或默认空身份。
6. 测试安排、Roadmap 及 app/project_materials.go 的自动材料生成预检共用 generate/1 派生快照；project_baseline.go 的 generationProfile 选项按同一结果投影可用性，产品仍可限制本机 Codex。快照键包含固定 pin/resource_digest、源 execution 摘要、目标 workflow 和明确权限，在准入前有界取得；配置及固定构建不变时，自动维护轮询、锁内预检及实际新请求读取同一结果，不各自重开 CLI。失败诊断同样有界记录并由明确刷新/配置更新重查，不把未取得结果解释为不支持或放行。删除 GenerationExecution 的全部生产消费者；完整 generate input 仍沿现有 schema 校验。原记录观察、取消、unknown、重启不调用 derive。
7. 删除已无消费者的 native 字段转换与测试夹具；转换断言迁到 Runner，本仓改为验证公开返回值、身份、冻结、资格和事务。保留有实际证明目的的正常/失败测试，不机械复制生产映射。

退出条件：NEC-A01–A05/A08/A09；R1 同版真实 EXE 接线通过，原请求观察和停止不依赖新配置能力可用性。

## 5. R2 → S2 · Web 原生映射归 Runner

### R2：Runner

1. 复用 R1 解析/身份，实现 compose 的有限 selection/unset 与匹配 base 继承。匹配包括完整 target/environment；无 base 仅按公开默认构造。非法权限/审计模型缺项及退役 launch/helper 字段明确拒绝，不清洗或改投；不替调用方选择审计模型，SteerLine 自己的草稿建议不受影响。可信base到完整结果的字节比较负责来源，不新增程序摘要或helper检查协议。
2. 同源发布可编辑逻辑字段与已实现操作；复用 capability_catalog 组合及 run_configuration 校验，禁止另建一张支持表。增加程序注入、跨 Agent/target.kind 继承、默认/显式/清除、审计缺项和退役 Agent 的反例。
3. 扩展现有 native-catalog agents/models 的 request_schema，允许可信 execution：agents 只核对 adapter/target，继续拒绝 agent 入参且不按所选 Agent 过滤目录；models 另核对 agent。execution 与 executable 互斥；Codex 目录程序入口只由 Runner 解释，仍只做原元数据协议。没有 profile 的目标查询保留原语义，profile 查询失败不降级另一程序。补自定义 Codex executable 保持、业务 args 不执行、错误目标/程序冲突及原取消清理测试。
4. 交接新的同版 EXE/manifest；S2 不以 R1 的 describe/derive 声明推断 compose 或 native-catalog execution 字段存在。

### S2：SteerLine

1. Web 读取既有 finite capability_catalog 和新编辑说明，生成产品范围交集。去掉虚构 model/helper 与枚举 effort 后试验 schema 的 project 算法；schema 继续校验真实返回输入。
2. 新增 `/api/web/execution-config/compose`，只接逻辑字段和已保存 base ID/摘要；从服务器可信配置取原 execution，通过 nativequery 调固定 Runner。浏览器只见逻辑 view/完整草稿 execution 与安全状态，不用另一个程序授权摘要或文件检查响应来代替可信来源。
3. read 为配置页面提供逻辑 view；discovery 从 view 核对 Agent/目标，再从服务器已保存 profile 传完整 execution 给 native-catalog，不再反解 settings 或丢弃自定义 Codex executable。browser discovery 入参仍只有 profile/选择器；缺扩展能力给具体诊断，不改投默认 Codex。非法/未支持 profile 原文只读且明确原因，不能静默重写；其它 profile 的读取与目录错误隔离。
4. 同阶段修改 validate/save 的 HTTP handler、前端 API 和 Store 方法以消费 `{saved_digest,config,profile_edits}`，执行 Spec §4.3 的恰好一条 edit/可信 base/明确 unset/完整字节比较。原始 config 不是程序授权，compose 返回值也不是票据；每次重建后才保存。保留 If-Match（同时核对 body 摘要）、写权限、developer、pricing_policy、usage_policy、Runner EXE pin、最终原文件复读与显式重启规则；可信部署资产自身来源核验保留。
5. 前端新建/复制/模型/Agent/目标/workflow 切换使用逻辑草稿，累计保存 base_profile_id、selection 和 unset。保留当前切换审计 workflow 时的主模型初始建议、可编辑和用户保存确认；只禁止 Runner 自行补值。合并现有请求取消、草稿 generation、晚到响应围栏，配置已改时拒绝保存，不覆盖用户新选择。
6. 删除 nativePrograms/programs/programsUnchanged/validHelper/helperStatus 的原生解释与已退役helper gate，并同步移除旧LocalHelper/页面状态；不替换为新helper检查协议。仍被Runner EXE/部署资产使用的普通文件核验按真实消费者保留。删除前端 values/changeModel 的原生路径构造及仅供旧路径使用的类型字段。

退出条件：NEC-A01/A06–A09；零项目页面和正式保存入口已使用同版 Runner，无本地旧映射回退。

## 6. 最小测试计划

遵循 [testing.md](../testing.md)及[路径清单](../paths.md)。下表 `<out>` 为 `C:/SteerlineDebug/wt-20a4-native-config/` 下每次独立的新输出叶目录，不写安装/源码现场。新测试名是计划建立的入口，必须先补驱动、非零匹配和正式登记，再执行；不把计划命令当现有测试已存在。

| 项目 / 验收 ID | 环境、入口与级别 | 时点 / 通过条件 |
| --- | --- | --- |
| Runner NEC-A01/A02/A05 | 组件 `go test ./internal/contracts ./internal/resources ./internal/cli -count=1`；公开受控根为 `TestSRExecutionConfigurationPublicCLI`，进入 Runner Controlled 正式清单。执行 `./scripts/test-foundation.ps1 -Pattern '^TestSRExecutionConfigurationPublicCLI$' -RunnerExe <exe> -OutDir <out>` | R1/R2；按 Runner 实际包名和驱动签收前固定命令。验证三操作/schema/错误/身份/预算、stdin/取消、零业务副作用，不能读业务 state-dir 证明身份。 |
| SteerLine NEC-A01/A02/A08 | owner 诊断：`go run ./cmd/test-feedback -out <out> -- ./internal/runnercontract ./internal/runnerclient -count=1`；扩展正式 L4 `go.runner-contract` 的公开 stateless 案例，显式 STEERRUNNER_EXE/VERSION/SHA256 后运行 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.runner-contract -out <out>` | S1/S2；公开真实 EXE 正反例、双端身份与无服务参数，缺 pin/能力明确失败，无模型。同步专项根清单及计数，不继承原“三根”结果。 |
| NEC-A03/A05 | 正式 L4 `go.environment`、`go.storage`：分别 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment -out <out>` 和 `-scenario go.storage` | S1；覆盖同 context 但源摘要因 model/effort 不同仍准入，原生环境差异拒绝；identity/derive 故障、旧格式/source/key/basis 篡改及并发 CAS。每包/根非零，无外部查询进事务。 |
| NEC-A03/A08 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario testing.project-environment-readiness -out <out>` | S1；普通入口检测、stopped/closed 封存、ready 前后派发、unknown/重启及停止沿原请求；不绕过资格写库。 |
| NEC-A04 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario testing.material-generation -out <out>`；L4 `go.roadmap`：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.roadmap -out <out>`；自动预检 App owner 另按下段补正式选择 | S1；两种生成与自动材料预检同源；重跑维护轮询证明无重复配置 CLI，缺派生结果零提交；原材料/schema/结果与额度完整。 |
| NEC-A06/A07 | L4 `go.http` 与 `web.unit`，分别 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http -out <out>`、`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <out>` | S2；覆盖 profile_edits 缺失/重复/多余/未知base、显式unset、多次草稿、header/body旧摘要、恶意raw配置、程序注入/退役helper拒绝、顶层字段、零项目/晚到及原审计建议。目录覆盖自定义Codex程序与不执行业务args，正式根清单同步。 |
| NEC-A09 | owner 守卫诊断：`go run ./cmd/test-feedback -out <out> -- ./internal/testkit/boundary -count=1`；每个代码阶段提交前固定 L1、L2：`go run ./cmd/test-feedback -level=L1 -out <out>`、`-level=L2` | 拒绝新请求构造/资格/前端编辑原生字段转换及纯模型/SQLite外部调用；保留 RunnerProfileDisplay 冻结历史、部署/测试资产公开组装的明确例外，原历史值不受当前配置改变。Windows/Linux依赖图保持。 |
| NEC-A10 配置页 | 在原配置发现浏览器驱动中新增逻辑编辑与保存案例，沿 `scripts/test-execution-profile-discovery.ps1` 增加可选明确的配置编辑模式及固定源码/EXE 参数；命令在驱动实现后登记于专属验收 | S2 最终；新隔离零项目包覆盖正式 compose→validate→save→读取，摘要冲突、退役helper注入与只读配置拒绝，目录成功不授予执行资格。浏览器截图仅辅助，核心断言用正式 API/保存文件。 |
| NEC-A10 原生联验 | 固定同一最终 EXE；已有 L4 live `testing.project-environment-local`、`testing.project-environment-orca` 按独立测试 profile 精确选择；Runner 自身 native execute/generate 场景在配套 Plan 固定 | 最终联验；本机 Codex/Orca本机/Orca远端逐组合记录原配置→派生请求、完整结果、原 session/terminal scope 停止及封存。测试源选择只能走正式 profile，缺能力先补驱动；不凭一次入口通过推定全部目标通过。 |
| 提示词/接线核对 | 原生成/环境/开发输入的冻结正文对照及 `internal/prompts` 实际调用测试；需要改变接线/对应测试时提交独立审核 | 每阶段；配置边界迁移不应改正文，报告实际 UTF-8 字节数前后值。任何确认的正文变化先明确业务理由，不改 golden 掩盖变化。 |
| 当前文档阶段 | `python scripts/check_doc_links.py`、`git diff --check`；核对验收 ID、阶段、链接和测试 ID | 仅文档检查；不运行模型或正式 L1/L2，也不登记实现通过。 |

新增反例优先放现有 owner 及正式复用集；新增 stateless 专项根同步 test-list.json、L4 说明、执行器选择/计数和 Runner Controlled 清单。自动材料预检放现有 App 项目维护测试根中，补入 `go.environment` 的正式 App 根选择并同步机器清单/L4/选择和计数；它与原 L3 material-generation 的真实入口共同覆盖 NEC-A04，不靠默认 Full 或仅定向诊断掩盖未选中。其它配置/环境页面新字段同样先补清单再执行。不默认扩大 Full、全部 live 或新增重复原生矩阵。

## 7. 独立审核、交接及完成定义

1. Runner 先交付配套文档、生产实现、公开 schema/样本、固定构建与受影响真实原生资格；SteerLine 以相同 EXE 独立验证实际 CLI 及产品入口。两仓通过状态分别登记。
2. 每阶段交付表列：实际 commit、EXE SHA/version/resource_digest、operations/转换组合、源/输出身份样本、程序来源策略、测试结果与未验项。未完成能力不在 capability 声明为支持。
3. 审核重点：原生配置规则没有在 Web 或资格模型复写；程序权限没有放宽；pure identity 未变成 readiness；原事务、停止、unknown 和原请求字节保持。提示词/接线/对应测试按仓库治理独立审核，不能由本轮作者自审代替。
4. 当前及长期文档随实际接线更新：docs/architecture/current-code-architecture、integration/steerrunner、执行方案设计及旧专项的后继指向。历史验收不改写，当前构建与未验边界在专属验收持续登记。
5. NEC-A01–A10 的必需自动项均有实际证据，两个阶段旧转换物理删除，最终构建按链路签收，才可宣布本轮收口完成。环境缺失、自动失败或 close/stop 未知均保留为未完成，不改称人工待验。安装切换另行处理。
