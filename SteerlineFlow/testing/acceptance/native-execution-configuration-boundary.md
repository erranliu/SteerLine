# 原生执行配置边界收口：验收规划

日期：2026-11-05，状态更新：2026-11-06。状态：代码实施及本轮规定的自动验收完成，正式回归、本机/浏览器及同版远端正向联验已签收。用户随后授权先Runner后SteerLine分别提交、推送和创建PR；上游为[Runner PR #122](https://github.com/erranliu/SteerRunner/pull/122)。不合并或部署、不切换SteerLine/Runner安装，不启动roadmap。下方历史记录保留其原时点。

调查版本：SteerLine `9b2e5d886b8f56eaf07bc697ca78c578c91eec00`；Runner `a66038c89208392c47531e640c84e97728c62424`（GitHub 固定提交只读内容）。实际受测两仓提交、EXE version/SHA256/resource_digest、profile/目标及隔离目录均在执行前填写，不能沿用调查版本冒充受测构建。

## 1. 自动验收矩阵

所有项方法为自动。命令与正式测试映射见 Plan §6；未实现的公开 CLI 根、浏览器编辑模式与正式计数必须先补齐再执行。原始报告用仓库外每次独占目录，按 [paths.md](../../paths.md)记录 summary、managed、JSONL、浏览器及 Run 证据，不进 Git。

| ID | 操作与最小通过条件 | 准备 / 触发 / 阻塞范围 | 状态 |
| --- | --- | --- | --- |
| NEC-A01 | 固定 EXE 测 describe/derive/compose、schema/source/input_digest、严格 JSON/限额/退出码；只读入口零 state-dir、服务、Agent、Run/PR 副作用；不以读 Runner 私有库核验 | Runner 新驱动/固定构建；各阶段交接前，阻塞对应新配置接入 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A02 | 对照执行转换的源与输出：目标/Agent/偏好保持，execute 合法 native 选择、generate read_only、无升权；未支持/非法/退役配置明确拒绝 | Runner 共同接受的对照样本与原生生产 gate；R1，阻塞派生接入 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A03 | 正式项目检测入口保存原配置/源身份/实际请求；Run 结果归属、原 scope 停止及 closed 报告才决定资格；失败与未知保持 not_ready | 新 home/项目/普通 result fixture；S1，阻塞环境资格切换 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A04 | 正式测试生成、Roadmap 与自动材料生成预检消费同一派生快照；轮询不重复CLI，预检与实际提交一致；材料/原文/schema/额度保持，缺能力零提交 | controlled fixture/同版EXE、App正式根及material-generation入口；S1，阻塞两类新生成及自动准备 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A05 | 每份源摘要自校验；活动/候选源摘要因model/effort不同但context/basis相同仍准入；目标/Agent/权限/程序差异不合并；篡改/并发/旧格式拒绝，纯事务无外部调用 | identity 样本、实际 SQLite 事务；S1，阻塞新身份格式与准入 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A06 | 正式 Web 构造/保存新建/复制/模型/目标/workflow；保留SL审计模型草稿建议，Runner不擅自补值；程序注入/跨组合继承/退役helper源拒绝，不清洗或改投 | R2、零项目隔离配置、可信程序夹具；S2，阻塞配置编辑切换 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A07 | profile_edits缺失/重复/多余/未知base与摘要冲突拒绝；显式unset及多轮草稿可重建并与提议字节一致；原顶层字段/显式重启保持；目录沿用自定义Codex程序且不执行业务args | go.http/web.unit 与正式浏览器驱动；S2，阻塞保存/UI及目录迁移完成 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A08 | 关闭新配置能力后已有请求观察/停止继续；重启保持原输入/namespace/instance，unknown 只查原请求；新操作清晰拒绝 | 公开服务/请求故障与重启夹具；每阶段，阻塞恢复安全签收 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A09 | 旧转换/样例试探删除；源码/依赖 owner、受影响正式集、提示词实际字节/接线独立审核及文档检查成立 | 各阶段代码与固定基线；提交/交接前，阻塞该阶段签收 | 受控/固定 EXE 验证通过；原生项见 A10 |
| NEC-A10 | 同一最终 EXE 通过下列实际链路证据；真实配置编辑、generate、环境检测及原 Run 收尾分别签收 | 独立测试机器/登录/目标与浏览器驱动；最终，阻塞“整体完成”声明 | 通过；规定分项均有同版实际证据，远端补验见第8节 |

## 2. 最终联验分项

NEC-A10 不共用通过状态，各行记录实际配置与 EXE；不存在或未声明的组合按配套 Runner 合同明确范围，不能默认为通过。

| 子项 | 实际入口 / 必需证据 | 当前状态 / 前置 |
| --- | --- | --- |
| A10-C | 本机 Codex describe→generate read_only 与 execute 检测；正式 Run/内容结果及停止，原 profile 不变 | 通过：native-generate-02、live-local-codex-02，原结果与清理均已核实 |
| A10-L | Orca 本机已声明 Codex execute 派生；模型偏好、原 session/terminal scope、最终结果封存及资格 | 通过：live-local-orca-04，含真实检测/准入/双端重启/原Run停止、报告归档和清理 |
| A10-R | Orca 远端同一明确目标的 execute 派生→实际项目脚本→closed 结果→资格；无改投/重复输入 | 通过：live-remote-orca-04；实际ready、原Run停止/封存、同home双端重启、Task准入及清理均通过；02/03失败保留 |
| A10-W | 零项目正式配置页新建/复制/切换、逻辑edits→compose/validate/save/重读；原审计建议和自定义Codex目录程序保持，冲突/退役helper注入拒绝 | 通过；browser-edit-04，正式编辑/磁盘重读/反例与清理均通过 |
| A10-RS | 同版 Run 已冻结后重启 SteerLine/Runner，原观察与停止不调用 derive；精确资源归属/停止回执及测试资源收尾 | 本机及远端通过：live-local-codex-02、live-local-orca-04、live-remote-orca-04 的同home双端重启与原输入/身份/结果断言；Runner另有native-generate-02 |

Pi 等未受本次执行转换影响的组合保留 Runner 配置/compose 与原 gate 受控回归，不由 Codex 真实通过推定它们已获得新原生资格。本轮不要求人工体验项；人工截图只作辅助，核心动作和通过条件由自动断言负责。自动能力/环境缺失如实登记，不能改记人工后置。

## 3. 执行登记格式与本次状态

每次实际执行追加：日期、两仓受测 commit、EXE/资源摘要、精确命令/实际选择、环境/profile/目标、报告根、结果、失败/skip/未验、Run/request/stop 身份与清理结果。Runner 自身验收、本仓受控接口、本仓真机/产品入口分别登记；同一源码但更换 EXE 后重新核对，不拼接不同构建的结果冒充同版联验。

实施结果按下方新增记录与矩阵更新；未执行或失败项保持未完成。历史 `environment-native-orca-executor`、原生目录及项目环境通过只用于问题调查，不计本轮新增契约资格。

2026-11-05 初稿文档阶段：`python scripts/check_doc_links.py` 通过（694 个 Markdown、4281 个本地链接、0 错误）；`git diff --check` 通过，另对三个新增文件及修改索引检查 UTF-8、行尾空白和末尾换行，均通过。当轮未修改生产代码、提示词、schema 或测试/CI，未执行 L1/L2、Runner 构建、模型/浏览器或 NEC-A01–A10 的实现验收，尚未做独立审核。

## 4. 2026-11-05 独立文档审核与修订

用户明确要求独立子代理审核并修复确认问题，暂不实施代码。审核者为本聊天独立只读子代理 `audit_native_config_design`；主代理核对证据后修订，未改生产代码、提示词、测试或 CI。原审核版本的 SHA256：Spec `064255cba45ece5205a3cdf841ef83f9e8bb25ac36f22d7fb6b48d79a854881d`，Plan `c12865252f1c5647ec04a54c30918081ba646ade9ac87128b77f76469aa0e222`。

| 项 | 确认问题及现有代码证据 | 文档修订 |
| --- | --- | --- |
| D01 / P1 | validate/save 只有完整Config，无法重建base/显式清除；web共享API与executionconfig.Store.validate | Spec §4.3规定无持久票据的profile_edits、saved_digest、逐profile重建/比对，Plan同步handler/Store/API/草稿和反例 |
| D02 / P1 | discovery.go 从原profile取Codex executable，删除本地解码后缺替代传递 | R2扩展既有native-catalog接受服务器可信execution，保留选择器/互斥校验、原程序元数据上下文及禁止业务args；无新目录CLI |
| D03 / P2 | app/project_materials.go自动生成前也调用GenerationExecution，原计划遗漏 | 自动材料预检和project_baseline生成选项接同一有界派生快照，轮询/锁内不重复CLI；Plan增加正式选择覆盖 |
| D04 / P2 | execution/model/runner_profile.go在app、taskrunner及query中从冻结原请求生成展示 | 明确保留纯历史/配置展示例外，不能影响编辑/准入/执行，不让历史读取新增CLI依赖；可信部署/测试资产供给同时明确保留边界 |
| D05 / P2 | identity比较未区分每份源摘要与跨活动/候选等价，可能误使模型变更失效 | 写出源摘要分别校验、跨对象比较context/key/basis的规则及model/effort变化正例；不建额外identity表/服务 |
| D06 / P2 | 原页面切换审计workflow会把主模型作为草稿建议，初稿禁止复制夹带UI政策变化 | 保留SteerLine草稿建议、编辑及保存确认，只禁止Runner擅自填审计模型 |

收窄项：R1只实现有消费者的view/identity/derive。进一步在Runner独立worktree核对同一固定基线后，独立子代理确认直接删除program_binding_digest、integrity_checks和对应部署检查，不再延后建设：contracts/run.go:380–391、resources/configuration.go:39–44已拒绝显式Orca launch/helper，既有native-catalog验收也将其列为退役反例。可信base/default→compose→保存重建→完整字节比较已满足授权。execute对合法native源保持，退役源拒绝，不能只用宽松ValidateExecution清洗；generate只读派生仍保留。Runner EXE pin和可信部署资产自身来源核验保留。

审核认为三个有限操作各有现有消费者、最小冻结context为纯资格/事务所需，可保留，不扩展通用配置框架。

独立子代理已对修订作定向复核：六项确认问题均已解决，未发现新的阻塞性设计矛盾；其补充指出的测试目录路径和 agents/models 适用选择器文字也已更正，本轮文档定向复核通过。文档审核不等于Runner侧契约签收或NEC-A01–A10实现验收通过。

用户随后明确要求Runner侧开独立worktree，仅改配套文档。已从固定main基线建立分支`codex/native-execution-config-docs`，本次工作区定位为`C:/Users/lmas1/.codex/visualizations/2026/11/05/01a1ab15-e7d3-7c23-b44a-8c039cafaf7b/SteerRunner`，新增其`docs/spec/`下同名Spec/Plan及`docs/acceptance/`专属规划；仅在旧native-catalog Spec加后继草案导航。

最终独立跨仓复核通过：共享CLI正文及native-catalog execution条款逐字一致，两个阶段与职责清楚；移除退役helper拟议协议后无剩余阻塞性设计问题。文档检查：SteerLine全库694个Markdown、4284个本地链接、0错误；Runner四个改动文档20个本地链接、0错误；两仓共享协议/目录段比对、UTF-8/新增文件空白及`git diff --check`通过。两仓工作区均只有文档变更，未提交/推送、未实施或运行任何代码验收。

## 5. 2026-11-05 实施与构建交接

Runner 始终使用用户要求的独立 worktree：`C:/Users/lmas1/.codex/visualizations/2026/11/05/01a1ab15-e7d3-7c23-b44a-8c039cafaf7b/SteerRunner`，分支 `codex/native-execution-config-docs`。受测源码均为调查基线加本轮未提交修改（dirty），不能把基线 commit 当作完整源码身份。未切换现有安装、未处理其原活动。

最终固定 Runner 为该目录同级 `runner-evidence/config-handoff-build/steerrunner.exe`：version `0.2.0-dev.4`，SHA256 `fee54d50352caa5df832ddbbd250250810a4cb7baa3c83150e697041f1f988aa`，resource_digest `b0a833884534996049477c43969acf9ba40bbe3ca72edfceab9b42d96740f344`，build source_digest `8ea283162b74db21d20ddbd6c7e1fb0cb929ceea492f56c7882432ebf9f2d2ab`。后续联验全部显式设置 STEERRUNNER_EXE 为此文件；早期不同摘要构建只保留其原证据，不拼接为本次同版原生资格。

实现职责：Runner 提供 stateless 三操作、通用 identity、同源逻辑编辑说明和可信 execution 目录上下文；SteerLine 消费冻结结果，负责资格/key/basis/事务、材料和编辑保存授权。删除 GenerationExecution、useNativeOrcaExecutor、原身份 native 裁剪、Web 原生程序/helper解释及前端 settings 映射。Prepared/record 必需冻结源 identity 和实际派生 execution，SQLite schema_version=40、record.format_version=1；无旧版读取/迁移。新增 owner 守卫覆盖新请求/编辑边界；历史 RunnerProfileDisplay 和可信部署/测试资产组装仍是明确例外。

所有下表相对报告根以 `C:/Users/lmas1/.codex/visualizations/2026/11/05/01a1ab15-e7d3-7c23-b44a-8c039cafaf7b/` 为前缀。命令从本仓执行，每次 -out 指向表列独占目录；原始 JSON/JSONL、实际命令、选择和清理不进 Git。

| 正式选择/入口 | 报告根 | 实际结果 |
| --- | --- | --- |
| 固定 `-level=L1` / `-level=L2` | sl-evidence/L1-final、sl-evidence/L2-final | 通过；L1 含 owner/固定正文守卫，L2 为完整受控交付冒烟。随后仅验收驱动接线修改另行复验，不改记旧报告源码 |
| L4 go.http | sl-evidence/http-03 | 七个 owner 全量通过；保留原压力窗口/次数，未与 E2E 并行 |
| L4 go.storage | sl-evidence/storage-02 | 全量通过；后续非法 context 规范化负例另行直接测试通过 |
| L4 go.environment | sl-evidence/environment-03 | 四个 owner 及 Windows31 个 App 根全通过；包含生成快照/自动材料预检接线 |
| L4 go.roadmap | sl-evidence/roadmap-01 | 正式全集通过 |
| L4 go.runner-contract，STEERRUNNER_EXE 固定最终 EXE | sl-evidence/runner-contract-handoff | 五个公开 EXE 根全通过，包括 Web 实际 compose/validate/save 和缺失/重复/多余 edit、未知 base、程序注入、摘要冲突反例 |
| L4 web.unit | sl-evidence/web-unit-03 | 前端全集通过，包括 20 个 execution 配置用例 |
| L3 testing.project-environment-readiness,testing.material-generation | sl-evidence/L3-environment-generation-01 | 两个正式流程通过，487.7 秒，清理通过 |
| discovery 取消定向回归 | sl-evidence/discovery-cancel-01 | 原共享/取消根及新确定性同 key replacement 根通过，完整 HTTP/L1 又通过 |
| 真实零项目编辑驱动 | C:/SteerlineDebug/wt-20a4-native-config/browser-edit-04 | passed=true，stop_exit=0，cleanup_exit=0；固定最终 EXE，新独立可写 home，实际 compose→validate→save→磁盘重读、审计初始建议及不同模型、400/412反例通过 |

真实浏览器命令为 `./scripts/test-execution-profile-discovery.ps1 -SteerRunnerExe <上述最终 EXE> -DebugRoot C:/SteerlineDebug/wt-20a4-native-config/packages -OutRoot C:/SteerlineDebug/wt-20a4-native-config/browser-edit-04 -ConfigurationEdit -ZeroProjects -IndependentProcess`；具体实际参数以驱动报告为准。原包冻结，编辑 home 在独立报告根内创建。该结果不等于原生 execute/generate 或远端目标资格。

保留失败：早期 owner/正式测试中的缺新 format/identity 夹具、旧 validate/save mock、controlled 源缺显式权限、纯模型测试错误导入 runtime testkit 已修复，原断言及依赖边界未削弱。browser-edit-01/02 为驱动遗漏展开高级设置/未实际改审计模型；03 确认并修复目录最后订阅取消后错误复用旧 flight 的竞态。旧 worker 仅按确证本次 PID/命令停止，四次独立驱动最终均记录自身 stop/verify 清理，失败报告不覆盖。

## 6. 提示词、接线与独立实现审核

业务原因只涉及原生 execution 所有权、冻结配置与编辑授权，提示词/材料协议继续由原组装函数提供。实际正文 diff 为零，未更新 golden。固定 rendered UTF-8 字节前后值：testing-generate 328→328，testing-reference 95→95，testing-readonly 163→163，testing-step 123→123，environment-invoke 396→396，roadmap-parse 356→356，roadmap-documents 503→503。L2 正文 testing 3256→3256、roadmap-parse 766→766、roadmap-documents 1676→1676、roadmap-spec-guide 4250→4250；L1 源文件 environment 412→412、roadmap-documents 511→511、roadmap-parse 364→364、testing-execute 47→47、testing-generate 266→266、testing-readonly 164→164、testing-step 24→24。Go 组装源码仅工作区行尾导致 raw 文件字节差，不是正文变化。完整 20 文件清单及前后字节在 sl-evidence/prompt-body-comparison.json。

独立只读子代理 audit_native_config_design 对跨仓设计、三操作、identity空偏好容器例外、compose保留非偏好字段、同源 reasoning_efforts、S1/S2接线/权限/事务、提示词与对应测试逐轮审核并确认修复。最后定向复审通过 discovery flight 的 pointer equality/原清理占用slot、真实浏览器完整断言与清理门槛，两仓共享 CLI 及目录 execution 正文逐字一致。此次真实本机 Codex 又暴露验收证据没有保存 identity：生产活动 ready，原 Run stopped/closed，场景内部退出0，但外层 InitialReport 环境键为空导致正式失败。驱动已增加实际冻结 identity，校验器先验证源绑定，缺失/错误源/错误context三反例通过；独立复核确认保留全部原生/结果/停止门禁。旧正式失败保持失败，新驱动必须另起报告重跑。

原生项目源为 sl-evidence/live-inputs/source 的自有独立 Git fixture，commit `9540eb6cd1ac3bd1276a6e77b403bfc8dcb50f6a`，只含基线原文环境材料和 go.mod。每次 L4 驱动另冻结自己的探针 commit/随机 codex/e2e 测试分支，目标仓库 erranliu/SteerlineTest；不操作其默认分支。显式 profile 文件 local-codex/local-orca/remote-orca 与项目 JSON 位于 live-inputs；Orca transport 绝对程序由可信测试配置供给，无退役 helper/launch 参数。Orca Task 材料生成使用额外显式 STEERLINE_L4_GENERATION_PROFILE，不从受测 Orca 配置改投。

2026-11-05 补充收尾：Runner正式完整 L1/Controlled 最终通过（873测试、29根，受控零skip）。本仓 go.deployment 正式全集 sl-evidence/deployment-01 通过；补输入与报告接线后 L1-handoff、L2-handoff 均通过。Runner focused native-generate-02 通过，Run `7d24913b665aa47f07dad6d20caad5c2`，实际派生只读生成、完整结果、同版服务重启及停止/资源结算；它不替代 SteerLine 原生检测项。

Orca首次 live-local-orca-01 在创建检测前因自动材料生成默认采用受测Orca而configuration_required；02追加本机方案但遗漏maintenance_profile选择，独立审核确认后已补正式命令与applied回执，03使用修正驱动继续验证。01/02未创建检测Run，原驱动因缺环境结果按原保守规则保留隔离scratch/自有Orca注册和随机测试ref，原报告仍失败，不伪造closed或清理通过。后续核实与清理单独存证，不改写原失败。

为验证A10-RS，新live驱动在原检测Run已stopped/closed后，对同一home执行SteerLine/Runner停止及重启，核对完整Prepared（含身份/源/派生）、原请求字节、RunID/input_digest、封存Result与单轮ready状态，再走实际Task准入原Run及停止。该断言在新增后的原生报告中签收；较早编译的Orca03不继承新断言通过。原环境/生成的组件重启/unknown/停止与不可用新能力反例依旧保留。

## 7. 最终联验收尾记录

本机 Codex：sl-evidence/live-local-codex-02 正式 L4 passed=true，实际 execute 检测 ready、原结果 closed/Run stopped、同 home SteerLine+Runner 重启保持完整 Prepared/原请求字节/RunID/input_digest/封存结果，随后 Task 准入探针的原 Run 与显式停止、全部自有 ref/资源清理通过。原先 live-local-codex-01 的外层报告接线失败保持失败，不覆盖。

远端：sl-evidence/live-remote-orca-01 在绑定前发现同 URL 两个已登记 checkout，原驱动只有 URL 匹配；进一步核实最终 Runner workspace.go 的真实合同为按源目录 basename 匹配。新驱动保留显式源目录名并同时核对 URL，不修改远端注册。新自有源为 live-inputs/SteerlineTest（同一 fixture commit），显式项目配置 project-canonical-name.json。live-remote-orca-02 成功绑定目标 e68f4edf-8a83-42c2-a9da-6d404d3aa3d8、runtime 21d98aa2-d284-41f3-a6cc-42fb22d728a2、repo 903fb315-7d5e-4246-8913-816e1d3bdf03（C:/Projects/SteerlineTest），实际执行一次项目入口。原 Run c553d5f7361ac741cf70f9382c994d38 的最终结果 completed=true、ready=false，报告 node.exe/npm.cmd 不在目标 PATH；Git/Go/Python 检查通过。原 Run stopped、结果 closed，前后准入均拒绝且零候选 Run，自有 ref/Runner 清理通过。正式 summary 仍 passed=false，不能以传输/负向证据冒充远端 ready，亦未修改目标安装。

本机 Orca03 的实际 ready、Task 准入/原 Run 停止成立，但 Orca 非 force worktree rm 拒绝两个 Run 自有的未跟踪 progress/result 二进制文件，原正式报告失败。清理驱动已增加严格自有文件归档：原 Runner/ref 结算及完整 setup/worktree/branch/空终端检查后，git status -z 只能含对应原 Run 两个协议文件；独占归档并复读原字节，再核对源未变后精确移除，不 force 删除其他 dirty 内容。原 rm、异步文件/Git 及 setup 清理门槛保留，外来/已跟踪/另一 Run 文件反例与二进制保真测试通过；经独立只读审核批准。04 是新的完整验收，结果另记。

旧失败隔离 fixture 已另作受限收尾：sl-evidence/retained-cleanup-driver 保存仓外 Go overlay 源和映射，只为诊断加载既有严格清理函数，未向源仓添加虚拟文件。retained-cleanup-01 的 local-orca-01/02/03 三个案例均重新通过公开 service verify-stopped，再核对精确测试 ref 原 tip、完整注册/工作区/空终端，完成自有 ref/注册/工作区及原 Run 报告归档；remote-orca-01 在仓库绑定前无业务 Run，仅核对并清理原 ref。四个子例均通过，原源 clone 与旧失败报告保留，不能称 remote01 也执行了该 overlay 的 verify-stopped。

受控配置流程保留前三轮失败：L3-configuration-01 因 fixture development/2 缺实现正文；02 首次接线错用 implement 键，被原 SQLite implement.execute 冻结提示词检查拒绝；随后依据实际 Runner StepImplement 常量复用既有完整 fixture steps/states。合成正文 Fixture implementation instruction. 既有35B→复用35B，文字diff=0，development/2 从缺正文到接入该35B是测试接线变化，生产 prompts/golden 零变化。独立复核通过。03 编辑 phase 已通过，原浏览器驱动第二轮保存只等上轮已存在提示后立即 reload，取消尚未完成的第二次 PUT；新驱动每轮先监听精确 PUT /api/web/execution-config、断言本次200并等待本次新方案名称，保留两方案/重启加载/实际消费全部门槛，无 sleep。04 新轮签收结果另记。

本机 Orca 新轮 sl-evidence/live-local-orca-04 正式 L4 passed=true（254.24 秒），同一最终 Runner，新的独立 home/显式 profile/同源码 fixture；实际 execute 检测 ready、同 home 双端重启的完整冻结/原结果断言、Task 准入原 Run 和停止、自有 ref、报告原字节归档、Orca 工作区/注册与 Runner 清理均通过。由该新报告签收 A10-L，不借旧03内部业务步骤冒充整体通过。

最终正式结果：sl-evidence/L3-configuration-04 完整通过（两种方案保存/磁盘加载/显式重启/原候选字节保持/新方案实际消费，清理通过）；sl-evidence/L1-complete passed=true（准备1.1秒、执行26.7秒），sl-evidence/L2-delivery-final passed=true。最后静态检查只对十个已修改的 App 测试文件作 gofmt，无语义或正文变更；Runner也仅格式化已审核的route_test.go夹具。生产代码与固定Runner产物不变；无新安装/数据兼容路径。

独立只读最终复核确认：生产边界、纯事务/原请求恢复、公开三操作/目录、提示词与测试接线、严格自有清理、浏览器等待及文档scope均无剩余代码阻塞。当前剩余项只有远端目标 PATH 中 Node/npm 前置未满足，A10-R 正向未通过，故不宣布整体原生资格完成。未提交/推送或切换安装，未来操作旧现场仍由旧版本排空，当前新必需格式不兼容旧记录。

## 8. 2026-11-06 远端工具调查与环境修复

用户授权查清 Node/npm 未安装还是 PATH 未继承，修复后重跑正向验收；暂不提交或部署。本轮未改两仓生产代码、提示词、固定材料或项目检测入口，继续使用上述最终 Runner EXE，SHA256 复核一致；目标 UUID/runtime/repo 均与 remote02 一致，不改投其他机器或仓库。

仓外证据根为 `sl-evidence/remote-node-investigation`。before、inventory、package-source 的原输出核对 Process/User/Machine PATH、Get-Command、标准安装及版本管理器目录、卸载注册表、winget 清单和既有用户工具目录：目标用户缺失可用 Node/npm 安装，未发现已有安装但 Orca 丢失其 PATH 的证据。官方 Node24.22.2 MSI 校验后安装返回1603，原日志确认 Error1925（远端 Orca 未提权，不能全用户安装），回滚且无 nodejs 安装目录；未触发 UAC 或重启 Orca。

改用 nodejs.org 同版本官方 Windows ZIP，与官方 SHASUMS256.txt 核对 SHA256 `ca1bff5d8ead13c9f2ea404a176db46bd430704d5b5113734049fa365e7799e2`，完整安装到已有且被 Orca 继承的 `C:/Users/Administrator/.local/bin`。每个顶层目标不存在才复制，原37个工具文件哈希不变；保留下载包/checksum/安装回执。Node v24.22.2、npm11.19.1 在新建终端直接按 PATH 查询成功。补写用户持久 PATH 时初次追加在末尾，导致 Orca 新终端去重后 WindowsApps Python Store 别名优先；remote03 的正式失败保持失败，实际 Node/npm 已通过，Python 版本查询失败，原 Run stopped/结果closed及自有ref/Runner清理通过。该顺序问题由本次修复引入，未归因于原始缺失或项目脚本。

path-order-repair 证据确认实际 Python 解析顺序，仅将本次唯一追加的 `.local/bin` 移至用户 PATH 首位，其他项顺序不变；Machine PATH 不变，无临时进程 PATH 注入。after-path-order-fresh 在新的远端终端实际查询 Git2.55.0、Go1.26.4、Node24.22.2、npm11.19.1、Python3.13.1 全部通过，Python 解析到原 `.local/bin/python.exe`。所有诊断自有终端的精确 ptyKilled 回执保留；长嵌套命令停滞、屏幕折行的仓外驱动读取失败均保留原输出，未重发未知安装、未操作业务终端。后续完整正向报告另记，不以版本查询或03负向证据签收 A10-R。

最终新轮 `sl-evidence/live-remote-orca-04/summary.json` 正式 passed=true、cleanup_complete=true、exit0，实际唯一根 TestRemoteProjectEnvironment 通过、0skip；场景173.49秒（正式execution_seconds=174.08）。使用相同固定最终 EXE、remote-orca profile、目标UUID/runtime/repo；原fixture源提交仍为 `9540eb6cd1ac3bd1276a6e77b403bfc8dcb50f6a`，本轮自有准入探针fixture提交 `ff9bb231ce1dc38aae130a2507e74fe79dac8467`。两仓仍为前述基线加未提交修改，未将基线冒充完整受测源码。

精确正式入口为 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario testing.project-environment-orca -out <上述完整base>/sl-evidence/live-remote-orca-04`；显式 STEERRUNNER_EXE 使用第5节最终EXE，STEERLINE_L4_PROJECT_CONFIG 使用 `sl-evidence/live-inputs/project-canonical-name.json`，STEERLINE_L4_PROJECT_COMMIT 使用上述原fixture源SHA，STEERLINE_L4_ROUND_TIMEOUT_SECONDS=600，STEERLINE_L4_REMOTE_PROFILE 使用 `live-inputs/remote-orca.json`，STEERLINE_L4_GENERATION_PROFILE 使用 `live-inputs/local-codex.json`。后两个相对目录也位于 sl-evidence 下，全部输入原文件及正式 remote-input.json、managed报告、原始JSONL保留。

本轮检测 activity `a86d5f872e034653801a046a3a03e085`、原 Run `ca30e5b64ceeb184d2c94b619181fb78`：一次实际项目入口、五项工具通过，completed/ready=true，原Run stopped、result.collection_status=closed/version1。environment-restart-proof 保留同home双端重启前后完整Prepared/identity/源与派生execution、原请求字节、RunID/input_digest及封存Result一致，仍单轮ready。Task准入前拒绝且零Run，通过后实际唯一探针Run `1851918e61d19eccafa6d74ae165d067`，随后显式停止原Run；admission-proof、原started/stoppedRun和candidate均保留。自有三个随机测试ref清理confirmed=true，308-runner-cleanup confirmed=true且未保留现场，正式managed结果与summary一致通过；已有远端注册及原仓库不变，无业务发布PR。

据此签收 A10-R、远端A10-RS，并闭合本轮NEC-A01–A10规定的自动验收。历史失败不改写，不扩展为未声明Agent组合或Runner通用完整原生级别通过。本轮只有远端用户工具安装/PATH修复及验收文档更新，无项目生产代码/正文/接线修改，沿用此前已通过L1/L2和独立审核，无需以重复无关回归代替远端正式证据。未提交/推送两仓改动、未部署或切换安装。

## 9. 2026-11-06 两仓实现最终独立审核与修复

用户要求独立子代理最终核对职责边界、调用链和验收证据，修复确认问题，暂不提交或部署。新独立只读代理 `final_cross_repo_audit` 读取两仓实际已跟踪diff及新增文件、Spec/Plan/验收、原始报告；未沿用前次审核结论代替本轮审查，未运行live或干扰服务。审核确认Runner三操作/身份/生产gate与调用方职责分离；环境检测、测试生成、Roadmap、自动材料预检、冻结恢复及SQLite准入主链成立，saved-base重新构造保存授权未被绕过。

发现并修复两个P2，均在[配置页](../../../web/src/features/execution-configs/execution-configs.tsx)：

- 目录查询按组合取第一份保存profile，两个同组合Codex方案有不同程序时，编辑或复制第二方案可能向第一程序发现目录。现按当前 `baseProfileID` 及完整adapter/target/agent绑定；无base的逻辑默认不借另一方案。base ID或保存摘要变化重新发起查询并取消旧请求，显式重读同ID但程序变更不复用旧目录。
- 自动建议默认模型时无条件清空unset，默认模型无effort时可能重新继承保存base的旧effort。现仅移除已被显式赋值字段的unset，没有default_effort时保留reasoning_effort unset，最终compose与保存不再带旧值。

[页面回归](../../../web/src/features/execution-configs/execution-configs.test.tsx)保留原20例断言，新增三例：两份自定义程序选择第二及复制来源保持；412后显式重读同ID新程序，释放已取消旧响应仍不覆盖新目录；保存Orca base原有high但新模型无effort时，实际compose请求保留unset且最终保存无effort。原无base用例补充不传profile_id的断言。独立复核又指出新effort夹具复用了含退役executable的旧模板；仅将新增三个案例使用的源调整为当前合法字段，旧用例意图保留。同固定最终Runner的公开stateless describe实际校验两个Codex程序源和Orca effort base，三例均exit0，无服务/Run副作用。

本次生产修改仅前端目录来源/取消依赖及unset意图；Runner生产/固定EXE、后端协议、提示词和资产均未改。页面原fixture workflow的 `preserve` 正文在localPair中复用，实际文字diff=0、UTF-8 8B→8B，未添加规则、未改golden；业务原因是目录来源及逻辑清除必须与真实compose保持一致。独立子代理已对生产fix、合法夹具、原断言保留及正文复用逐项复核通过。

新证据根统一为第5节base下 `sl-evidence/final-audit-20261106`。保留 web-regression-before 原失败：目录来源与effort清除两个缺陷实际被断言捕获；新重读用例初次错误假设无冲突时有重读按钮，按实际412流程修正。web-owner-after 为22/23通过、仅该驱动错误；web-unit-01全通过但尚含上述待修夹具，不用于最终源码签收。最终 web-unit-02 正式passed，原生Vitest 288/288、0skip，其中配置页23例全通过；web-build.json/log记录 `npm --prefix web run build` exit0。固定L1-01 passed（execution18.0秒）；L2-01 passed、cleanup_complete=true、exit0、0skip（execution81.64秒）。各正式命令仍为原test-feedback入口，仅out采用上述新独立叶目录。

审核者重新读取固定EXE实际SHA、Runner formal-handoff完整L1/29根Controlled、public-handoff两根及native-generate-02原报告；本仓原14份正式summary、真实本机Codex/Orca与远端04 managed摘要、原ready/stopped/closed proof、同home重启和实际Task准入/停止证据核对成立。它们证明各自冻结源码/EXE及原链路范围，不把原浏览器结果当作本次新UI缺陷回归；最终修复以后端未变为范围依据，另运行配置浏览器流程复验，结果另记。独立生产/回归复核无剩余确认问题，未提交、推送或部署。

最终 `final-audit-20261106/L3-configuration-01` 正式passed=true、cleanup_complete=true、exit0、0skip，execution216.53秒。两个真实浏览器阶段edit/loaded分别通过；保存两种Agent方案、配置改变时阻断新候选、显式停启后磁盘/页面加载、关闭Web后两方案候选输入的实际消费/冻结、原候选输入摘要保持、Runner清理全部通过。独立复核核对summary/managed摘要、浏览器原日志、原候选与新增Codex/Pi的Submit.Input；本轮快照中Codex已有Run，Pi取证止于候选输入冻结，不宣称两种Agent都已启动Run。原受控断言与完整范围未缩减。本次前端问题由新增有效owner回归直接覆盖，L3补充完整流程证据，不将受控结果说成新的真实模型链路。

最终文档收尾：694个Markdown/4290个本地链接、0错误，两仓diff检查和共享CLI正文逐字比对通过。两仓HEAD保持原调查基线，固定Runner EXE SHA不变；本轮独立审核/修复/正式测试与源码文件摘要另存 `final-audit-20261106/verification.json`。确认的两项P2均已修复并经独立复核，无剩余确认问题；未提交或部署。

## 10. 2026-11-06 PR交付与上游依赖

用户随后授权Runner→SteerLine分别提交、推送和创建PR，暂不合并或部署。上游[Runner PR #122](https://github.com/erranliu/SteerRunner/pull/122)已先创建；Runner继续使用原独立worktree/分支。本仓分支为 `codex/native-execution-configuration-boundary`，PR正文与Runner双向回链，评审/未来合入顺序Runner在先。Runner不依赖本仓代码；本仓依赖其公开三操作/identity/native-catalog execution能力，缺能力阻断对应新操作，原冻结请求观察/停止保持。

前述原生资格严格绑定原固定EXE SHA及原调查基线加dirty实现。Runner已同步其最新main并另建集成EXE，owner与公开接口补验记录在上游PR，不把旧EXE原生资格复制给新产物。SteerLine提交只封存实际修改，不将原dirty报告改写成在干净提交上重跑；当前main集成冲突/实际补验及其受测提交、产物另记。SQLite schema40/record format1不提供旧记录兼容，旧安装仍由原构建排空，后续安装或构建选择另行处理。

两份PR已按顺序创建并互链：上游[Runner #122](https://github.com/erranliu/SteerRunner/pull/122)，下游[SteerLine #462](https://github.com/erranliu/SteerLine/pull/462)。本仓基于 `a54280510802c1208d9a1a3ff0ec473cad4ebd24` 重放为生产提交 `3cb5c0ddcdeb017fb4b10fd8f8c953668a8889c5`；保留main计费归属/显式材料准备/部署恢复行为。四处真实冲突分别保留两份文档双方说明、RunUsage同快照字段及新增Configuration能力、VerifyUsagePolicy授权核验及新增Configure/cache。其后目标main前进到 `3546eec95af22284da25cb61d7685bbd55fd2138`，刷新后merge-tree无内容冲突，未为无冲突的后续主干改写已验提交。

独立只读代理 `final_cross_repo_audit` 定向复核冲突与自动合并的生成/资格/保存/用量授权调用链，发现两个上游新增测试尚未接新保存协议：Go encode缺Store参数导致编译失败，前端usage PUT mock仍读取raw config。提交 `5b760679601f6bcecb97b097a721800e48676d2e` 仅修复这两个测试文件，提取信封config并补saved_digest/空profile_edits/If-Match断言；原精确日志叶授权、待重启和取消旧读断言保留。原HTTP01编译失败保留，HTTP02另起复验。独立复核确认生产职责及维护worker冻结profile/derive接线完整，未改提示词或放宽测试。

当前main集成补验统一存第5节base下 `publication-20261106`，受测源码为干净提交 `5b760679`，不是旧dirty基线；后续提交只补记录/文档：

| 选择 | 独立报告目录 | 实际结果 |
| --- | --- | --- |
| L4 go.http | sl-http-02 | 七个owner全量通过，51.74秒；原压力窗口及计数不变，执行期间未与E2E/其他压力并行 |
| L4 go.environment | sl-environment-01 | 四个owner及Windows31个App根全量通过，423.18秒 |
| L4 web.unit | sl-web-unit-01 | 301/301、0skip，含新增usage共同保存协议和配置页23例 |
| Web生产构建 | sl-web-build.json/log | npm --prefix web run build，exit0 |
| 固定L1 | sl-L1-01 | passed，execution15.26秒 |
| 固定L2 | sl-L2-01 | passed、cleanup_complete=true、exit0、0skip，95.29秒 |
| L4 go.runner-contract | sl-runner-contract-01 | 五个公开根通过，24.01秒；固定Runner PR集成EXE SHA c279ca725c9486bd50a2d183ca8dcd71c026f5b455a4b45ab9f5a24f1442c5b5，含三操作、Web重验保存及原服务结算 |
| 维护worker四根diagnostics | sl-maintenance-worker-01 | 四根通过，20.25秒；取消/慢读、持久超时、禁止候选隐式生成、冻结profile，原断言保留；不冒充完整Full |

六项正式报告已通过原record入口导出到 [5b760679测试记录](../records/5b760679601f6bcecb97b097a721800e48676d2e.json)，保留各自定义/原始报告及真实受测SHA，不导出dirty旧报告。新集成EXE仅获得上述公开接口和受控/owner范围；此前本机/远端真实模型仍绑定原fee54d构建，不推定新构建原生资格。本轮未重跑完整go.full、真实模型或上游已知未通过的维护长浏览器专项；原历史边界保持。PR仅供评审，未合并、未部署。
