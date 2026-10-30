# Plan：项目 × 环境准备状态与统一派发门禁

2026-10-29。按文档审计后收敛方案实施。[Spec](../spec/project-environment-readiness.spec.md) · [验收/审计记录](../testing/acceptance/project-environment-readiness.md)。用户已授权当前聊天直接实施、测试及独立复审；不启动 roadmap，不部署安装环境。下文为确认后的实施安排，实际完成及缺口以验收记录为准。

## 1. 输入与结构约束

源码调查基线 `7617cddc7efac5716c8c38bb63a0157a100acac0`；开工前固定实际源码、Spec/Plan SHA并复核后续差异。遵循[编写规范](../../SteerlineProjectGuideDoc/spec-plan-guidelines.md)、[测试指南](../testing.md)、[路径约定](../paths.md)、[生命周期](../../docs/product-design/worker-lifecycle.md)及[原环境适配规范](../spec/project-environment-adaptation.spec.md)。

整合目标固定如下，实施中不能用新增并行路径代替原入口改造：

| 部分 | 处理 |
| --- | --- |
| 原profile/execution配置及保存/重启链 | 保持格式，不增加环境注册表、profile.environment_id或generation；无需Node/Web配置保存器改造 |
| `environmentadaptation/model` | 集中增加纯上下文/准备依据及资格值规则；完整执行digest和请求冻结保持原样 |
| `project_environments`、原service | 保留全部活动/报告/轮次/预算和原请求；增加一次开始时选择当前引用的具体事务方法 |
| 项目 × 环境当前引用 | 新增最小持久选择/失效记录，原活动状态仍唯一；结果观察只更新原活动 |
| 原app预留接线/SQLite事务 | 删除本机绕过，在原入口传入最小证据并在原事务复核；不再叠加通用verifier/事务接口 |
| 原环境/任务页面 | 读取同一结构化投影，整合步骤、分组、blocker和历史；不再增加人工确认状态机 |

一个普通开发任务按下面顺序完成，不为模块数量拆任务。配置格式保持不变；需要的新SQLite格式不增加旧数据兼容或迁移。

## 2. 实施顺序

### P1：纯规则与最小持久引用

1. 在 `internal/environmentadaptation/model` 集中定义规范上下文/环境键：原execution的规范JSON仅移除Spec规定的四个结构化模型字段，其余设置原样保留。资格键与完整Run/profile digest使用不同用途函数，不能改写原execution或复用删字段后的对象组装执行。
2. 调整 `internal/app/environment_adaptation.go` 构造的准备依据：用同一规范上下文替代原basis中的完整execution，其余有限材料/测试/发布/仓库依据保持。当前查询及候选固定提交使用同一材料构造入口，禁止另造一套hash或全仓语义比较。
3. 在 `internal/storage/sqlite/environments.go` 和原typed端口增加当前引用读取、带expected revision的开始/失效事务。唯一键project/environment_key，字段限定revision/activity_id/失效标记与原因/更新时间；原活动继续承载状态、报告和依据，不新增ready/六阶段/最后通过冗余字段。原SaveEnvironment的SQL/不可变校验提为同包私有、接收原tx的helper，由原保存包装器和新开始事务共用；不能复制校验、嵌套BeginTx或向上层暴露通用事务回调。
4. 开始事务创建活动、原请求及当前引用；观察/完成沿原 `SaveEnvironment` 更新活动，禁止再写当前状态。失效沿原命令journal保存当前引用和回执；重复请求/原回执丢失只对账。原活动冻结约束、请求唯一性和收尾记录必须保留。
5. 按实际开工版本升SQLite格式及同步schema校验和新库夹具，不预先指定未来版本号、不修改配置版本。完成PER-A01/A02/A06的纯值、真实SQLite原子性/失效/迟到反例，再接派发。

### P2：原准备流程扩展及当前查询

1. 同时修改app材料入口和 `environmentadaptation.Service.Start` 中的远端限定，明确支持本机公开execution；原能力校验、固定base、完整L2/PR、双通道、三轮/期限/停止保持。生成活动不选择当前准备引用，不受业务ready门禁。
2. 在app利用已加载开发profile派生去重列表，读取明确引用和原活动，统一输出最终资格、页面说明及下一步；六阶段、最近通过时间从原事实投影。不能依“最近一条成功”回退，不能在GET补行或修改失效标记。
3. 准备/候选开始前沿原项目维护刷新并固定材料观察；查询读取已固定提交的不可变材料/原缓存，不重新fetch或发远端命令。保持原 `appConfigVerifier` 和维护观察的锁后核对；删除文档原先要求的环境generation/配置epoch及自动环境故障归因接线。
4. 显式失效仅作用当前项目 × 环境，机器重建需要各受影响项目明确重新验收；原任务普通失败沿原诊断和恢复，不增加新的自动分类/批量失效协议。
5. 若固定指令/材料需去除“仅远端”，只改 `internal/prompts/*.md` / `materials` 与原组装调用，按[提示词治理](../../docs/prompt-governance.md)交付实际正文差异、UTF-8字节数前后、业务原因和验证，经独立审核；更新黄金样本不代替批准，业务代码不得内联兜底。本轮审计未修改这些资产。

### P3：原派发事务和占用接线

1. 在 `internal/app/runner_service_wiring.go` 删除空target直接reserve；用同一纯规则从候选原execution得到环境键。仅原ReserveExecution位置过晚，无法阻止PrepareExecution/PrepareTestPlan先生成材料作业：在taskrunner.Service补一个选定profile后的专用准入回调，由app装配同一资格规则，Start重放先返回原事实，再在准备回调前执行预检；原Admit继续只负责Runner实例准入，不混用。taskrunner仅消费返回错误，不解释环境状态，不新增准入框架。app在最终事务外取得准备依据/当前引用观察，将环境键、引用revision、activity_id和basis作为最小准入值附候选。
2. 在 `internal/storage/sqlite/runner_candidates.go` 原重放分支之后、Task领取/候选编号及submit持久化之前，读取当前引用和原活动并校验同项目/环境键、未失效、ready/settled、basis及观察版本；失败整项回滚。相同原请求先返回原候选，当前失效不改原历史或重发请求。
3. 复用原home `environmentAdmission` 临界区处理准备与同公开目标的活动冲突；本机目标键不再为空。准备资格键含权限/Agent入口，占用键仍按公开adapter/target，避免同目标不同profile并行开始准备。仅落库/原记录复核持锁，网络/Git/Runner不持锁；保持已有目标准入→材料保留→SQLite顺序，不增加租约/通用锁服务。
4. 手动启动、默认方案和新增候选通过原Start/Reserve链统一拒绝。自动派发先用廉价只读资格判断正常blocker，未通过时不分配新Start请求或持久失败控制命令；原最终事务复核保留，不新增队列或ready缓存权威。
5. 失效只阻断新候选；原查询、停止、人工回答/恢复和交付保持冻结授权，dispatch_mode/paused继续表达用户意图。准备与失效/预留的提交顺序用真实SQLite屏障验证；重点断言Task、候选编号、Command和Runner调用次数。

### P4：页面、生命周期与旧入口替换

1. 沿原环境HTTP Query/命令/journal扩展去重列表、当前引用和显式失效。开始/失效payload的expected_current_revision必填，缺行初始0；失效携环境键和原因，外层沿原项目目标/版本规则，停止继续用原活动revision。严格校验字段存在性和目标/实例/版本，不能混淆两个revision。只读响应携环境键/引用/原活动及结构化资格，Web不解析文字推断业务。
2. 整理 `web/src/features/environment-checks/project-environment.tsx` 为材料→用户准备→验收→结果，保留原历史/checks/L2/PR/进度/停止入口；不增加清单确认命令或人工ready。
3. 项目顶部显示服务在线与环境计数；任务profile选择及 `features/tasks/dispatch.tsx` 用同一资格来源显示blocker和准备入口。不同模型方案合并展示；不可用时禁新开始，停止/查询保留，切项目/实例和隐藏轮询沿原Query规则。
4. 修改原 `readyFor` 为按当前引用精确读取，不再扫描全历史选择当前；历史查询仍独立保留。删除远端专用资格投影和重复摘要比较，避免旧入口留作第二套准入。
5. 审计启动census/pending stops/接管/关闭/离线核验/reset：继续枚举全部原活动，当前引用不成为资源清单。只在原具体接线补必要字段，不能新增一套引用层恢复流程。实现时同步原环境Spec/产品说明的被替代段落，原历史结果保持原范围。

### P5：最小系统验证

1. 扩展现有受控准备helper，走正式公开开始和Runner报告接纳，让L2和受影响E2E夹具获得资格；不直接插入ready或新增测试绕过开关。owner反例可构造行，但不充作系统通过。受控报告沿公开fixture，不递归运行SteerLine自身L2准备另一套L2。
2. 在 `tests/e2e/suites/testing` 增加一个资源独立的L3 `testing.project-environment-readiness`（计划ID）：两个项目、两个环境、无准备拒绝→准备通过→只放行对应组合→失效再拒绝及正常清理。竞态/迟到/未知/失效组合放owner测试，不各建E2E。
3. 真实远端扩展现有 `testing.project-environment-orca`；本机新增同一个参数化L4 `testing.project-environment-local`（计划ID），复用现有准备驱动/双通道/PR检查，通过local profile选择Codex或Orca，分别固定现场执行。本机/远端 Orca 共用原 repo/terminal 取证及绑定/关闭流程，只改变显式目标选择器；仅本机 Codex 允许直接读原 Run 工作区，两个本机适配器属于不同准备环境。只补原驱动的目标接线，不为三条路径各建流程框架。
4. 本机驱动输入沿既有E2E信封固定：`STEERLINE_L4_LOCAL_PROFILE`（独立profile文件）、`STEERLINE_L4_PROJECT_CONFIG`、`STEERLINE_L4_PROJECT_COMMIT`、固定`STEERRUNNER_EXE`和正数`STEERLINE_L4_ROUND_TIMEOUT_SECONDS`，拒绝非local目标。三路径分别取证，不遍历无关Agent/模型组合，不使用安装home或现有业务任务。
5. 实现驱动后同步 `test-list.json`、l3/l4、`tests/e2e/suites.json`及选择/proof/清理校验；规划阶段不登记假入口。保留固定L1/L2和必要恢复回归，新增命令非零命中且证据完整后才可登记通过。

## 3. 最小测试安排

输出规则沿[路径清单](../paths.md)，每次执行用独立scope下的新绝对叶目录，记录源码/文档SHA和真实命令。以下命令仅供实施后执行，本轮文档审计只跑第4节静态检查。结果归[专属验收](../testing/acceptance/project-environment-readiness.md)。

| 时点 / 验收项 | 级别与入口 | 通过条件 |
| --- | --- | --- |
| P1–P3；PER-A01–A06/A08 | owner定向，真实Git/SQLite及公开Runner fixture；`go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/environmentadaptation/... ./internal/storage/sqlite ./internal/app ./internal/taskrunner -run TestProjectEnvironmentReadiness -count=1`；新增测试统一此可执行前缀 | 环境键/完整冻结分离、引用事务/投影、所有入口和自动tick、并发提交、失效/迟到/占用全部非零命中并通过 |
| P2/P4；PER-A03/A08必要回归 | owner定向，使用下方原环境链回归命令 | 原轮次/取消/未知/生成/封存/接管与非当前活动收尾正确，零新自动输入 |
| 修改代码后及提交前 | 固定L1：`go run ./cmd/test-feedback -level=L1 -out <新L1目录>`；固定L2：`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 正式最小集、受控核心交付及清理完整；夹具不旁路准备 |
| P4；PER-A07 | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-web.ps1 -EvidenceDir <新Web目录>` | Web/HTTP/server断言和生产构建通过；列表分组/投影优先级、步骤/任务阻断、缓存和可见性实际覆盖；不扩大到配置编辑器改造 |
| P5登记后；PER-A09 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness -out <新L3目录>` | 两项目两环境闭环、原任务/候选/Runner计数与正常清理；受控不等于原生通过 |
| 接线后；PER-A08 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.runner-wait-retry,recovery.waiting-merge-restart -out <另一新L3目录>` | 原Run/等待/恢复/交付事实保持，未核实操作零重发；不新增未修改配置保存场景的强制回归 |
| 受控通过且输入固定；PER-A10-LC/LO | 新驱动登记后，L4 live：`go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out <新L4目录>`；按P5分别固定本机Codex与Orca，执行两次 | 各支持路径真实工具/TTY、完整L2、同一真实PR/收尾和门禁。能力不足明确阻断并记缺口，不记全项通过 |
| 同上；PER-A10-R | L4 live：`go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-orca -out <另一新L4目录>`；输入沿E2E README独立remote profile/项目SHA/Runner/预算 | 真实远端完整正向验收及门禁；仅负向正确不能记ready或正向通过 |
| 隔离新包可用；PER-M01 | 人工页面走查，参与人及窗口待安排 | 从未准备辨认责任/下一步/失败项与任务blocker，找到原报告；体验签收不替代A项 |

原环境链回归（执行时替换输出路径）：

```powershell
go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/environmentadaptation/... ./internal/app -run 'Test(ProjectEnvironmentExecution|ProjectEnvironmentWorkflow|ProjectEnvironmentAdmission|ProjectEnvironmentGeneration|CallerPromptConvergenceEnvironment)' -count=1
```

L3总并发最多二；共享源码Web/EXE构建与包生成串行，live单项精确选择。默认不运行Full、全部L3/L4。缺环境/权限、skip、零命中和资源未知不算通过，也不能改称人工待验。真实三路径分开登记，未验路径不得宣称可用；owner/系统证据不能替代原生资格。

## 4. 文档与交付检查

- 实施前后核对三层最小新增量：一个纯模型上下文规则、一份当前引用、原预留事务复核。原活动/配置/Runner仍各自持有唯一事实，删除被替代入口，不留下双套状态或适配旁路。
- 新SQLite结构遵循未发布规则，不新增迁移/双写/回退。安装部署、清库、归档及任务恢复不属本任务授权，不沿用上次安装维护许可。
- 有提示词/接线/资产或对应测试CI变化时按治理门禁独立审核；实现后的正文变更、独立审核与功能证据见专属验收。
- 收口运行 `python scripts/check_doc_links.py`、`git diff --check`，另检查新增文档UTF-8、表格/代码围栏及验收映射。真实结果和本次确认修订记录在专属验收，未满足真实输入的路径保留前置失败证据，不登记通过。
