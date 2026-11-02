# Plan：运行环境与项目运行环境最小解耦

2026-11-02，已按用户授权实施，所选自动验证及独立代码审核通过。[Spec](../spec/runtime-project-environment-decoupling.spec.md) · [专属验收](../testing/acceptance/runtime-project-environment-decoupling.md)。基线 `6f1edde1dbc9bb950d579a4cbe1dca67301c778b`，普通开发，不启动 roadmap；验证仅使用隔离测试服务，不使用真实模型、安装现场、部署或真实 PR。

## 1. 实施取舍与落点

目标是把运行环境的识别、配置与探测从项目资格中抽离。复用全局 execution 配置及 discovery；项目检测闭环继续保留。全局探测只提供当前可观察能力，不另建可执行性认证与持久 ready。

下文 A01–A07 均为 Spec 的 RPE-A01–RPE-A07 简写。

| owner | 最小变更 |
| --- | --- |
| `internal/executionconfig` | 承接现有目标识别/校验纯规则，增加按 adapter/target 归组的纯函数；不承接项目 Key/Qualify 或 I/O |
| `internal/environmentadaptation/model` 及其消费者 | 删除原 Target 实现并直接消费中性规则；保留项目非模型 Context/Key、材料路径、Current、摘要和生命周期 |
| `web/server/executionconfig`、`web/server/discovery`、`web/server/execution_config.go` | 原 Snapshot 增加 environments 投影；探测明确选定 profile/目标与实际配置上下文，复用原原生查询、缓存与配置保存，不增加路由 |
| `web/server/nativequery` 及元数据调用点 | 显式传入应用 home 作为子进程 cwd；元数据程序解析/取证固定绝对路径，移除父工作目录的隐式依赖；不改原生目录 RPC 或执行协议 |
| `internal/app/environment_readiness.go`、`environment_adaptation.go`、`environment_admission.go` 及相关 owner | 目标/方案投影与项目资格计算分开；材料读取失败只落在项目状态，移走中性规则依赖，保持原后端准入和占用 |
| Web execution-configs、environment-checks、settings、app、task 文案 | 全局沿原分组展示环境，项目明确“项目运行环境”；保留路由与操作流程，仅必要类型/查询键和文案调整 |
| 现行产品设计/页面说明与测试 | 同步术语和归属；复用原 owner 和受控场景，删除重复规则/测试，不改历史验收结果 |

项目 SQLite 与 Runner 请求/信封不计划改格式。没有全局环境表、配置第二来源、注册器、探测调度器或兼容入口。不能借抽离目标规则重写 execute/1、项目材料协议或整套应用启动。

## 2. 实施顺序

### P1：抽离目标规则，固定语义（A02、A07）

将现有 `environmentadaptation/model.Target` 的校验和稳定目标表示移到 `executionconfig`，补最小目标值/分组函数。一次更新生产消费者并删除旧函数；项目 Context/Key 继续调用中性目标规则。用既有身份测试核对本机 Codex、本机 Orca 和远端 Orca，尤其是同目标不同 Agent/权限的项目键仍不同，模型变化不改变项目键，目标占用范围保持。

不增加全局 `runtime_key`、键映射或登记步骤；home 是外层作用域，目标沿现有公开身份表达。全局分组涵盖全部 profile，项目检测保留 development 类方案筛选。

远端目标保持配置 selector 原值，不在纯规则中读取 Orca 目录或解析名称为 ID。目录新选择用原返回 ID；保留“不同 selector 不自动合并”的范围说明，不能以显示名替换项目摘要或占用身份。

### P2：全局投影与探测收口（A01、A03、A07）

在原全局配置 Snapshot 中派生 environments（adapter/target/profile_ids），不持久化。读取和校验直接使用现有配置与离线 Runner 元数据，不经过 ApplicationDirectory.Connect 或 ProjectRuntime。零项目时不为获得元数据改变共享 Runner 的启动条件。

分组保留原配置顺序，与 saved_digest 一起投影；元数据失败只附诊断。项目层仍消费启动冻结的配置，不从 Web 最新 Snapshot 替换运行中的方案或绕过待重启门槛。

收紧 discovery 请求绑定：存在 profile_id 时校验存在性、adapter/target/Agent 相符，仅使用原目录查询实际支持的参数；不带 profile 的原生默认目录查询继续明确其范围。Orca 仍查当前本机 runtime/已配对远端的目录 RPC，不执行 profile transport/agent_launch，不新增目录查询通道或对 provider/权限/启动参数的验证。未验证上下文继续标 `context_unverified`。

缓存只绑定实际查询输入与当前可观察身份，不把同目标分组或未使用的执行参数当验证依据。远端缓存命中前没有观察到 runtime 重启时沿原 freshness/显式刷新处理，不增加读前 status RPC、重启监听或执行上下文摘要。保持原超时、并发二和共享取消。

现有统一启动会设置 Web cwd=home，但直接 `steerline-web serve` 可继承项目 cwd。为所有元数据 Start/Run 调用显式传入应用 home，并在子进程 cmd.Dir 设置；不使用全局 os.Chdir，不新建配置项或进程选项框架。用独立启动的元数据可执行 fixture，从两个项目父 cwd 启动 Web/查询，核对收到的 cwd 始终为指定 home、查询结果/context_key 不依赖父目录；不据此声称真实 Codex 已读取或未读取某个具体项目配置。

同时收口父进程内的程序解析：当前 discovery 的 LookPath/Stat 先于子进程启动，仅设置 cmd.Dir 不足。目录查询拒绝含目录的相对 executable；绝对程序直接使用，裸程序名只从绝对 PATH 目录项按原生扩展名规则查找，排除 cwd/相对/空项，不修改进程 PATH。结果必须为绝对路径并用于同一次取证与启动；仅在现有元数据 owner 中做小型辅助函数，不建通用 resolver。fixture 补显式绝对程序在双父 cwd 下结果相同、相对配置在两处均拒绝，以及两项目各有同名不同程序但相同绝对 PATH 时不会命中项目程序的反例；不改 profile 保存或实际项目执行的路径契约。

补产品 HTTP 入口级反例：合法全局配置但 etc 无项目；项目配置错误；项目库不存在；项目缺全部环境文件。对项目连接/Git/库访问设置失败探针，证明全局路径未触达它们；只查目录不创建执行。

开发能力投影补齐公共 schema 的必填 `access_mode`；Web 新建及无匹配模板的链路切换也显式生成开发所需权限。增加能力组合正例、真实缺权限配置仍拒绝的反例，以及本机仅有 Pi 模板时新建 Orca＋Codex 的页面提交回归。以完整 Web 服务子包、`go.http`、`web.unit`、L1/L2及构建验证，独立证据根 `C:/SteerlineDebug/wt-68cd-combination-access-mode/`；不修改安装配置，不放宽真实保存或执行校验。

权限界面精简移除下拉和专用设置函数，保留固定说明；链路构造公共路径和能力试样公共初始化各写入一次权限，不建通用权限抽象。显式选择开发工作流时设置 full_access，内容方案改名仍保留 read_only。增加页面及只读→开发转换回归，执行 web.unit、完整 Web 服务及 runnerclient owner、go.http、固定 L1/L2、构建与原配置导航浏览器场景；证据根 `C:/SteerlineDebug/wt-68cd-simplify-access-mode/`，原报告独立保留。实际结果归专属验收，完成后独立复审并提 PR，不操作安装配置。

### P3：项目层独立组装与最小页面适配（A04–A06）

项目 options 先得到中性目标和方案元数据，再对每个原项目检测上下文计算材料/Current/资格。来源或材料失败仅附对应项目诊断，不能让已知目标消失。复用原环境服务与 Qualify，不把 Web discovery 注入后端任务准入，不新增全局资格门禁。

全局页复用原探测组件，已保存列表改用 Snapshot 分组并删除原前端归组算法；项目页保留按检测上下文分组的卡片与路由，将标题/导航/摘要/任务阻断明确为项目运行环境，缺材料提示项目未接入并链接原执行配置页。前端分开全局探测和项目资格类型，按各自 scope 使用现有 Query；原任务参数、返回路径和活动停止继续工作。

修正当前编辑页 probeProfile 选择未比较远端 environment 的问题；完整目标和 Agent 匹配才传已保存 profile_id。无匹配时沿默认目录查询并说明范围，防止后端收紧身份校验后正常切换远端反而被旧方案阻断。

同步现行 `docs/product-design/worker-lifecycle.md` 的环境边界及全局配置/项目环境的产品设计术语；原 Spec 已由新稿明确替代的部分不复制重写，历史记录不覆盖。项目调用契约没有变化，本轮不改项目适配指导或其他模型可见材料、入口 argv、提示词正文/组装、Runner 请求或信封。

### P4：验证、清除重复与交付（A01–A07）

审查依赖方向和 diff：全局配置/探测不依赖项目检测模型；项目服务消费中性目标规则；旧目标实现和替代性包装已删除；无新表/库/配置、作业和全局资格。结果按专属验收登记，失败保留原输出，补测新目录。

依赖顺序 P1 → P2 → P3 → P4。本计划不预留多仓 Runner 改造；确有公开能力缺口应先列明具体缺口并修订范围，不能引入探针框架或假设未公布接口。

## 3. 验证计划

遵循[测试指南](../testing.md)和[路径清单](../paths.md)。实施证据使用仓库外 `C:/SteerlineDebug/wt-68cd-runtime-project-environment/`，各调用使用唯一 `<run-id>`。下表保留实施验证安排；实际命令、结果、失败/补验与审核记录归专属验收，不以此表代替原报告。

| 时点 / 映射 | ID、环境与命令 | 通过条件 / 证据 |
| --- | --- | --- |
| 当前文档；全部 | 文档检查：`python scripts/check_doc_links.py`、`git diff --check` | 本地链接无误、正文边界一致；结果登记专属验收，不跑业务测试 |
| P1；A02/A07 | owner / isolated：`go run ./cmd/test-feedback -out <diagnostics/run-id> -- ./internal/executionconfig ./internal/environmentadaptation/... ./internal/storage/sqlite -count=1` | 项目键/依据/资格未放宽，中性分组准确；原 Target 删除；summary 与原 Go 日志 |
| P2/P3；A01/A03–A05 | owner / isolated：`go run ./cmd/test-feedback -out <diagnostics/run-id> -- ./internal/app -run Environment -count=1`；另一次 `go run ./cmd/test-feedback -out <diagnostics/run-id> -- ./web/server/... -count=1` | 零项目/零材料、目标/Agent 错绑、已观察输入/缓存、默认目录未验证范围、两项目及准入/停止反例通过；另含直接 Web 启动、双父 cwd、相对程序拒绝及同名程序干扰的元数据进程 fixture，核对绝对程序取证/启动一致、固定 home cwd 和目录/context_key；分别保存 summary/日志，非零命中 |
| P3；A06 | Web / controlled：`./scripts/test-web.ps1 -EvidenceDir <diagnostics/run-id>`；另执行 `npm --prefix web run test:e2e -- configuration-navigation.spec.ts` | 原全局/项目路由、无项目访问、项目文案、任务返回、缓存与草稿断言通过；浏览器证据使用独立输出目录，具体输出设置见专属验收 |
| P4；A04/A05 | `testing.project-environment-readiness` / L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness -out <test-results/L3/run-id>` | 原 Current/迟到/占用/候选事务与两项目隔离通过；缺材料不成为全局探测前置，停止/清理核实；summary/managed/scenario |
| P4；A02/A05 | `runtime.shared-runner-projects` / L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario=runtime.shared-runner-projects -out <test-results/L3/run-id>` | 中性目标抽离后共享服务/跨项目占用与结算无回归；summary/managed/scenario |
| 代码修改后、交付前；全部 | `l1.minimum` / L1 isolated：`go run ./cmd/test-feedback -level=L1 -out <test-results/L1/run-id>`；`smoke.delivery` / L2 controlled：`go run ./cmd/test-feedback -level=L2 -out <test-results/L2/run-id>` | 两固定集完整通过，零匹配/意外跳过/清理未知不计通过；每级独立报告 |

受控场景增加必要断言时同步原 test-list.json、可读分级列表、suite manifest 和执行器的实际断言说明，不新增等价场景 ID。测试清单/执行器一致性按测试指南校验。Web、构建与调试包生成沿路径规则串行，避免共享源码产物冲突。

本轮不选择 Full、真实模型/PR 的 L4 或新增人工窗口：运行/停止协议、原生目录 RPC、transport 调用与项目检测接口保持；元数据调用仅显式固定 cwd 和不依赖父目录的程序解析，并以独立进程 fixture 核验该上下文。owner、HTTP、Web、受控 L3 及固定 L1/L2 验证本次结构调整。若实现实际改到原生 RPC、transport 调用或执行接线，须先补相应原生验证和提示词治理安排，不能沿用此处豁免。

## 4. 交付边界

用户已在方案独立审核修订后授权实施、验证及独立代码审核。当前按本 Plan 执行，实际版本、失败/复验、清理和审核结论归专属验收；不沿用编写阶段的文档检查冒充代码通过。固定提示词及其接线/资产/对应测试或 CI 如有变化，按 AGENTS 和提示词维护规则提供实际正文差异、前后字节、业务原因、验证及独立审核；本轮文档不充当该批准。

## 5. 安装现场缺陷修复（2026-11-02）

先 rebase `origin/main`，再只读核对安装清单、进程构建和公开接口，区分部署落后与请求阻塞。修复原 options 组装：提取纯加载配置投影，通过同一路由的严格查询模式返回；前端沿原 Query scope 先显示配置，完整资格仍决定 verified，显式刷新覆盖两个查询。抽取原文档/manifest/脚本读取供预览和无当前检测状态共用，完整 basis 留在已有 ready 和检测入口；复用已有请求内 blob cache，并加入仓库身份，避免跨上下文重复读取。保持页面布局、项目键、准入、停止、提示词及 Runner 接线。

新增 owner 与前端回归对应 Spec 第 7 节；定向 owner 先诊断，再执行 `web.unit`、`go.http`、`go.environment`、固定 L1/L2 和 Web 构建；输出根 `C:/SteerlineDebug/wt-68cd-environment-options-latency/`，每次调用使用独立叶目录，保留失败。文档链接与 diff 检查完成后独立审核修复，实际结果追加原验收记录。安装现场仅作只读诊断；源码修复通过不冒充安装版已更新或真实项目检测通过。

## 6. 可读远程机名称的最小适配

提取执行方案页已有的全局远程机目录 Query 和名称解析，项目环境页的分组及当前对象标题复用该实现；目录仍按应用 home 缓存，不扩项目接口或持久配置。仅有远程目标时查询目录，显式刷新时重查；名称请求不控制列表或 verified。新增 UUID→可读名称与目录挂起不影响资格的前端回归，并在原配置导航浏览器场景验证名称显示；执行 Web 构建、`web.unit`、固定 L1/L2及文档检查，证据根 `C:/SteerlineDebug/wt-68cd-readable-machine-names/`。不修改后端、HTTP/存储协议、环境依据或执行接线，不重复无影响的 Go owner 全集；结果归原专属验收。
