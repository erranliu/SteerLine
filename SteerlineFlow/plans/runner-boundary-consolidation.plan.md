# SteerLine 与 SteerRunner 职责边界收口 Plan

对应 [Spec](../spec/runner-boundary-consolidation.spec.md)，基线 `57e3c52e`，S1–S4 已实施。审计及全部运行结果归[专属验收](../testing/acceptance/runner-boundary-consolidation.md)。本次普通开发不分配 Task 编号。

## 1. 实施与跨仓顺序

| 阶段 | 实施位置及工作 | 退出条件 |
| --- | --- | --- |
| S1 | runnercontract 增加 native_catalog 纯值；web/server/executionconfig 读取固定 pin 的离线声明；nativequery 扩展有限 stdin；discovery 改接 Runner Orca 查询；删除原 orca/orca_local 连接实现 | SLB-A01–A04/A09/A10，固定 Runner R1 交接证据齐全 |
| S2 | discovery 的 codexModels 改接同一入口，删除原 Codex RPC/模型分页；按实际消费者清理元数据测试夹具 | SLB-A02–A04/A09/A10，固定 Runner R2 的取消/进程树证据齐全 |
| S3 | runnerclient.Catalog 解码已有策略 slots，config.runtimeCatalog 向预检提供冻结纯值，projectPolicySlots 删除硬编码；materialInput 和 PrepareTestPlan 共用 | SLB-A05/A09，正文/字节对照及独立审核完成 |
| S4 | runnerbinding 抽出中性内容请求事实与纯核验；testmanagement/roadmap 的在线观察、app 启动停止准备和 application_verify_stop 接入；删除 roadmapBinding 和对应跨业务 import | SLB-A06–A09，原状态/异常/CAS 和清理回归成立 |

先以 Runner R1 → SteerLine S1 证明公开协议接线，再进行 R2 → S2。S3、S4 可先实施但应分别提交，减少审阅中的行为交叉。各阶段进入正式接入时固定实际 EXE，而非仅用模拟 capabilities 推定支持。

## 2. S1/S2 接入细节

1. 按配套 Runner Spec 的 NC-01–NC-04 确认 stdin/响应/schema、source 与错误/退出码。Web 的 launcher 只暴露固定 native-catalog 动作，复用元数据进程所有权。执行 Client 的 state-dir/instance/full-binding 参数不进入目录调用。
2. profile 选择与当前目标不匹配时在 Web 拒绝。配置保存、Runner pin 或页面选择变化时丢弃晚到结果。保持两并发、共享请求最后订阅者取消，并移除迁移组合的跨请求新鲜缓存。
3. 更新正式页面请求/投影与测试，覆盖零项目及零材料场景。原生已注册但执行未支持的 Agent 保留说明，目录刷新不覆盖已保存配置。错误映射及过期展示沿既有 UI 行为核对。
4. 每阶段完成后删除该阶段旧原生协议、私有凭据读取及无消费者夹具。更新架构/并行开发/配置 Spec 与实际生效范围，守卫拒绝这些原生依赖回流。HTTP API 与配置格式仅按必要变化调整，所有实际变化写入该阶段验收。

## 3. S3 实施细节

用当前固定 Catalog 的 `model_input_contract.workflow.project_policies.slots` 解码，不要求 Runner 新增字段。先独立保存该部分原始公开数据，再记录解析值或策略目录专属错误，由新开发材料预检读取；避免把可选字段类型错误升级为 Catalog 通用加载失败。slot/path 的校验在纯契约与目录适配边界复用，保留严格空数组和缺失区分。运行时只保存冻结目录快照，材料预检和实际候选创建从同一快照读取。

从现有基线构造 git/command/testing 均存在、部分存在、全缺、存在但不可读、非法路径、重复目录及目录缺失的对照。确认当前正常输入的完整请求与字节数一致，异常仅限制新开发请求。按 [提示词治理](../../docs/prompt-governance.md)提交接线、实际正文及 UTF-8 字节数差异和独立审核，不自动更新黄金样本。

## 4. S4 实施细节

1. 列出 generation.observe、roadmap.observe、两个 prepareStops 及 application_verify_stop 的所有现行比较和不同分支，形成逐调用点行为对照。
2. 先用 runnerbinding 的中性纯值替换停服证明中 roadmap→Generation 的转换，保持传输和 Store 操作在 app/原服务。再逐个替换在线 Run/Command/correlation/workflow/revision 比较。
3. 对相同的停止信封条件，仅提取纯决策及构造。每个业务 owner 原有锁、保存 CAS、ID 分配、诊断及发送顺序分别保留原入口。若一处规则不同，先记录原因，不能以删除重复为由改成宽松公共分支。
4. 测试安排、Roadmap parse/文档生成的正常结果处理分别执行回归，证明内容/schema/manifest/确认及额度释放仍由各自规则决定。

实施中补齐零项目离线清理：原 host/Web 已退出、成员集合不变、原回执明确 Clean/零项目/Runner 从未尝试且资源已结算，并确认原 pin 与派生 Runner 状态路径都不存在时，才接受空 Runner scope。已有或尝试过的 Runner 继续原核验。`TestZeroProjectOfflineVerificationRequiresAbsentRunner` 同时覆盖 stop/restart 及状态残留反例，纳入正式 `go.deployment` 集合。

## 5. 最小测试计划

所有实现验证遵循 [testing.md](../testing.md)和[路径清单](../paths.md)。`<out>` 是 `C:/SteerlineDebug/wt-4ba6-boundary/` 下每次调用的独立新叶目录。定向测试中的新测试名由实施阶段建立并检查非零匹配。

| 验收项 | 级别/组、环境及命令 | 时点与通过条件 |
| --- | --- | --- |
| SLB-A01/A02 | owner 定向诊断，公开 CLI/RPC 夹具。`go run ./cmd/test-feedback -out <out> -- ./web/server/discovery ./web/server/executionconfig ./web/server/nativequery ./internal/runnercontract -count=1` | S1/S2 开发时，pin/源/限额/退出码、取消/清理、目标身份、目录完整性及固定执行路径有非零通过结果 |
| SLB-A03/A04 | L4 go.http 与 web.unit，隔离环境。分别 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http -out <out>`、`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <out>` | S1/S2 完成时，现有配置编辑和选择不丢失，原目标晚到拒绝、刷新/过期/在途合并、零项目可用。更新受影响 package/root 清单，缺失/重复/skip 失败 |
| SLB-A05 | owner 定向诊断。`go run ./cmd/test-feedback -out <out> -- ./internal/runnerclient ./internal/app -run 'Test.*ProjectPolic|TestRunnerPresetFreeze|Test.*Material|Test.*Prompt' -count=1`，补新增目录测试；另以独立输出执行 `go run ./cmd/test-feedback -out <out> -- ./internal/prompts -count=1` | S3 完成时，每包实际匹配非零，目录同源/路径/空数组/固定 commit/原字节及异常阻断证明齐全。保存 summary/jsonl 和正文对照 |
| SLB-A06–A08 | owner 定向诊断。`go run ./cmd/test-feedback -out <out> -- ./internal/runnerbinding ./internal/testmanagement ./internal/roadmap ./internal/app -run 'Test.*ContentBinding|TestTMGeneration|TestGQ|TestRoadmap|Test.*Startup|Test.*VerifyStop|Test.*SharedRunner' -count=1` | S4 开发及完成时，两个领域正常结果、在线/冷读缺失、原请求/Run/workflow/关联/revision 错误、先保存后停止、未知不重发及结算通过 |
| SLB-A07/A08 | 正式 L3 `testing.material-generation`，`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario testing.material-generation -out <out>`；Roadmap 用正式 L4 `go.roadmap`，`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.roadmap -out <out>` | S4 集成时从正式入口覆盖生成材料；Roadmap owner/App/host 集合覆盖解析、原请求、发布与确认，取消/重启/停止反例由 A06–A08 的定向测试补齐 |
| SLB-A08/A09 | 正式 L4 `go.deployment`：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <out>` | 零项目离线停服核验补齐后，完整原部署集合及新增空 scope 正反例通过；按 Win/非 Win 正式根数同步清单，不放宽原断言 |
| SLB-A09 | 源码/Windows/Linux 依赖守卫，`go run ./cmd/test-feedback -out <out> -- ./internal/testkit/boundary -count=1`；每个代码阶段提交前分别跑固定 `go run ./cmd/test-feedback -level=L1 -out <out>` 和 `-level=L2` | 原生协议只在 Runner、Web 无后端依赖、共同绑定无业务 model 依赖、旧转换和硬编码删除；完整基线非零、无 skip/失败且清理成立 |
| SLB-A10 | 隔离 Web 配置页与固定真实 Runner EXE，无模型。远端沿 `./scripts/test-execution-profile-discovery.ps1 -SteerRunnerExe <exe> -RemoteProfile <profile> -OutRoot <out>`；本机 Orca 沿同脚本 `-SteerRunnerExe <exe> -LocalOrca`，证据读取新包返回路径；Codex 在该原浏览器驱动中补对应查询 | S1/S2 最终构建联验，本机/远端 Orca 和 Codex 目录各记录 source、原始 ID/effort、只读调用、页面结果和精确清理。脚本本机分支新增可选 DebugRoot/OutRoot 透传以遵循本批仓库外路径，原默认入口仍可用 |
| 文档 | `python scripts/check_doc_links.py`、`git diff --check`，核对与 Runner 配套文档的命令/字段/错误/限额/阶段表 | 当前文档阶段及合入前，无坏链接及空白错误，跨仓约定一致 |

上述正式 ID 已按当前 `test-list.json` 和 `tests/e2e/suites.json` 核对。需要新增反例时在原 owner 测试中补充并同步正式定义，新增查询用例在执行前核对注册和非零匹配。SLB-A10 只做配置页查询与保存，调试包启动中原有准备行为分别记录，不能把整包启动写成零 Run；对 native-catalog 本身的零业务副作用由 RNC-A04 与 SLB-A01 单独取证。

最终联验补充两个实际入口：设置 `STEERRUNNER_EXE=<最终 EXE>` 后，以 `go run ./cmd/test-feedback -out <out> -- ./web/server/discovery -tags=runnercontract -run TestNativeCatalogPublishedRunner -count=1` 验证真实 EXE→Web 接线；页面驱动使用 `-LocalOrca -ZeroProjects -CatalogEnvironment <明确远端 ID> -DebugRoot <新包根> -OutRoot <out>`，在同一隔离零项目页面覆盖本机 Codex、Orca 和远端查询。零项目仅移除本次新调试包生成的项目登记，并记录原文件名与摘要，不操作安装目录。

纯文档阶段只执行最后一项。涉及 HTTP/前端的代码阶段必须实际执行 go.http/web.unit；S4 若涉及存储端口值，补 go.storage，涉及 app Roadmap 接线补 go.roadmap。需要修改提示词/资产/测试选择时按仓库治理要求取得独立审核。

## 6. 交接

Runner R1/R2 各提供固定 commit、EXE SHA256、resource_digest、已实现组合、公开 CLI 及真实无模型目录证据。SteerLine 使用相同构建独立验证公开响应与正式配置页。S3 直接使用已公开目录，S4 无 Runner 新接口依赖。任何阶段的产品通过仅登记实际范围，文档静态检查和历史执行资格分别保留。
