# 并行开发指南

> 2026-10-13 当前唯一规划任务为[#91](../spec/tui-http-client.spec.md)。旧二阶段候选/规划领域不作为当前待执行队列，功能地图的“拟议”仅说明未来边界。当前源码仍保留本地 TUI 适配器，HTTP 切换待实施。

本文持续维护 SteerLine 的功能划分、模块所有权、可并行条件和冲突触发点，是设计或调整 roadmap 前的阅读与检查入口。目标是提高 AI 开发的有效吞吐：让更多任务能够独立理解、实现、验证和交付，降低共享修改、等待、返工与集成成本。业务正确性、权限、原请求恢复和唯一交付是必须保持的边界，详见[整体架构](../architecture.md)。

规划时先读[RoadmapGuide](../roadmap/RoadmapGuide.md)，再读本页的判断规则，再按影响展开对应模块、契约和测试。实际源码位置由[代码架构](current-code-architecture.md)维护；本页维护并行决策，拟议任务关系由本轮选定的 drafts/ 草稿维护；用户确认的开发任务及依赖由[正式执行清单](../roadmap/execution-map.md)维护。采用的职责、接口、前置和验证要求须落实到独立 Spec/Plan；本页不分配任务 ID、不改变中心 DAG 或已冻结契约，也不是绕过硬前置的执行许可。

## 并行开发的判断

一个任务应有明确产品成果、主要模块所有者、输入/输出契约、共享修改范围和独立通过条件。目录不同、不同 worktree 或编译互不影响，都不能单独证明任务独立；还要核对业务语义、版本、事务、组装、测试和外部资源。

| 安排 | 成立条件 | 规划与集成方式 |
| --- | --- | --- |
| 可以直接并行 | 消费稳定契约；内部变化、验收和可写资源互不依赖 | 各自交付，按实际共享接线做最小集成验证 |
| 先固定契约，再分开推进 | 多方依赖同一新增字段、接口、权限或来源语义 | 先明确所有者、身份/版本、失败行为和验收，再拆实现与消费；关键前置未稳定者留草稿 |
| 可以分开实现，有序集成 | 各自成果可独立验证，但共用构造、注册、schema 或客户端接线 | 指定共享段的唯一修改所有者和合入顺序，后合入者核验组合，不同时各改一套接线 |
| 等前置交付后实施 | 前置结果会改变任务核心模型、权限、范围或通过条件 | 设置真实硬前置，先完成基础能力；不能用猜测接口固定后继范围 |

上述是规划安排，分别记录能否固定契约、能否开工及能否合入。Codex 开发由runner在硬前置整体交付、结果同步且成果进入主干后启动后继；SteerLine产品仍按中心资格领取任务；共同签名、夹具或接口提前完成不自动释放资格。并行设计、实验或原型不表示对应产品链已经可执行。

## 功能与模块地图

状态“已有”只表示当前代码已有该职责，不证明任意功能组合可并行。表中允许范围均以契约稳定、所有权和资源隔离成立为条件；“拟议”不表示包或能力已经交付。

| 功能 / 状态 | 主要归属 | 可相对独立推进的范围 | 需要协调的触发点 | 当前资料与定位 |
| --- | --- | --- | --- | --- |
| 任务契约、DAG 与资格 / 已有 | task、task/model | 固定契约下的任务校验、查询、选择纯规则及任务应用用例 | TaskContract/身份改变、领取与结果端口、前置/丢弃/后继释放语义 | [数据模型](../product-design/data-model.md)、[任务输入与文档端口](../../internal/task/commands.go)、[纯模型](../../internal/task/model) |
| 候选、派发、反馈、恢复与交付 / 已有 | execution | 沿现有小端口修改明确用例，保持 actor 和提交屏障 | 多项用例共改 WorkflowRun/Execution、Decision 路由、Operation 意图/回执、控制版本或恢复授权；同模块内也需划分共享所有者 | [反馈](../product-design/decisions.md)、[执行共同约束](../spec/execution-architecture.spec.md)、[提交屏障](../../internal/execution/service_commit.go) |
| Codex / 本机 Orca 接入 / 已有 | execution/adapters/codex、orcatransport | 实现自身协议、观察和能力核验；不改中心业务状态机 | 共享 Codex codec、backend 契约、权限/资源封套、绑定格式、会话/轮次或错误分类改变 | [执行配置](../product-design/execution-profiles.md)、[后端契约](../../internal/execution/backend)、[共享协议 codec](../../internal/execution/adapters/protocol/codexapp) |
| 工作区、PR/provider 与进程资源 / 已有 | execution/workspace、publication、process | 既有调用契约下的 Git/provider/OS 操作和事实核验 | base/remote、PR 归属、进程树所有权、清理条件或资源意图改变；共同执行结算需协调 | [工作区设计](../product-design/worktree-preparation.md)、[交付规则](../product-design/data-model.md#pr-与交付)、[资源源码](current-code-architecture.md#module-owners) |
| 会话与活动采集/读取 / 已有 | execution/conversation、采集 adapter、product/query、storage | 来源 mapper、正文读取和活动展示可沿固定模型分别改进 | 来源/轮次/正文修订、规范来源选择、去重游标或写入与 checkpoint 原子性改变 | [会话](../product-design/conversation-events.md)、[Activity](../product-design/activity.md)、[会话存储端口](../../internal/execution/conversation_ports.go) |
| 产品查询与客户端传输 / 已有 | product/api、query、client、httpapi | 稳定 DTO 下的只读投影、查询组合、CLI/TUI 和本机 HTTP 消费适配 | 公开字段/错误/能力/游标语义、请求目标、同快照要求或命令受理规则改变 | [产品 API](../../internal/product/api)、[本机协议](../product-design/local-client-protocol.md)、[查询组装](../../internal/product/query/ports.go) |
| 当前执行 TUI / 已有 | console；final_reader 拥有 final 状态和只读受理，Model 拥有导航/步骤目标/滚动/恢复/原会话入口 | 稳定产品契约下的 final 分页/正文/失效规则可在窄查询端口独立验证；布局、输入和候选选择仍由宿主修改 | 新后台能力、公共 DTO、草稿/原请求绑定、共享焦点/导航或宿主派生的查询/受理目标改变 | [控制台设计](../product-design/execution-console.md)、[final 状态所有者](../../internal/console/final_reader.go)、[宿主接线](../../internal/console/final_replies.go) |
| 持久化与原子提交 / 已有 | storage/ports、sqlite | 已明确语义下的读取或提交实现；消费者仍经窄端口 | schema、唯一约束、CAS/版本、同事务快照、checkpoint 与领域记录提交边界改变 | [数据模型与格式来源](../product-design/data-model.md)、[提交契约](../../internal/storage/ports/transactions.go)、[快照端口](../../internal/storage/ports/application_snapshot.go) |
| 配置、应用组装与服务宿主 / 已有 | app 的 local/terminal 组装单元、app/host | 既有后端的配置/target/preflight/factory 通常只改所属文件；Orca 本机与远端共用单终端实现。稳定宿主/产品端口下的进程控制和入口能力 | 新后端仍改中央显式组合；共同身份/policy、公共构造、配置摘要、exe/home 传递、实例/锁、启动与停服规则改变 | [local 组装](../../internal/app/directory_local.go)、[Orca 单终端组装](../../internal/app/directory_orca_terminal.go)、[组装/宿主源码](current-code-architecture.md#build-install-and-launch-sources) |
| 测试支持与环境 / 已有 | testkit、tests/integration、tests/e2e | 独立 owner 用例和场景，复用正式 driver/codec | 共享 fixture、driver/native/evidence、runner 登记/期限/清理、进程环境变量、账号/终端/仓库资源竞争 | [测试指南](../testing.md)、[E2E 环境](../../tests/e2e/README.md)、[测试边界源码](../../internal/testkit/boundary) |
| 规划草稿、会话、审核与准入 / 拟议 | 未来规划领域；尚非完整实现模块 | 2.2先推导草稿/artifact/人工事项，2.3实际会话接入验证必要权限；成熟后按稳定成果拆分 | 草稿/正式文档权威、审核快照与授权、Git/SQLite 部分完成、规划写入 guard、任务创建端口及会话来源身份 | [规划架构](../product-design/planning-architecture.md)、[审核准入目标](../archive/roadmap/phase2-draft-roadmap-admission.md)；新契约在实施前正式对齐 |
| TUI 经 HTTP 接入 / 拟议 #91 | 现有 httpapi 服务端、待拆纯契约/客户端、console 消费 | #45 基线已有；#91 补接口/纯客户端并换 TUI，共享接线由单一任务拥有，不等待 WebUI/SSE | 同一客户端协议、共享导航/输入组件、消息/权限/回执语义及未具备的服务能力 | [本机客户端协议](../product-design/local-client-protocol.md)、[拟议界面](../product-design/planning-ui.md) |

远端能力需独立核对目标权限、身份、观察/恢复/停止与路径所有权，不能从本机成功推定支持。

## 没有独立契约文档时

已有产品设计、源码接口、活跃 Spec 与测试均可作为当前定位。没有独立文档的边界，先在本页相关条目或本次 Spec 写清以下内容，并链接真实源码：

| 需要说明的内容 | 当前解释 |
| --- | --- |
| 谁生产、谁消费、谁修改 | 领域拥有业务规则；消费者声明必要的小端口；app 负责具体组装，adapter 不取得中心业务写权 |
| 输入/输出与身份 | 公共产品值可序列化；Task/WorkflowRun/Execution/Attempt、原请求、服务实例和版本分别绑定，不能用“最新运行”替代原目标 |
| 状态与失败 | 查询只读；未知外部结果沿原请求核实，不能盲重发；未提供的能力明确不可用；平台接受与业务完成分别判断 |
| 原子与资源边界 | 原生调用不进入数据库事务；业务记录/回执/游标按现有提交屏障；Git、进程和真实环境资源有明确归属和清理条件 |
| 变更影响 | DTO/codec/schema/配置语义变化列出所有生产者、消费者、持久记录、组装入口和受影响测试；未发布格式按当前规则拒绝旧数据，不默认加兼容路径 |

这些说明定位当前约束，不凭本页创造新 API、格式、空包或发布流程。形成需要反复复用的独立契约文档后，由对应领域维护正文，本页改为链接和必要摘要；未来测试组/环境拥有独立说明时同样改链，不积累完整签名、schema、命令矩阵或验收日志。

## 冲突识别与处置

| 冲突类型 | 典型信号 | 处理 |
| --- | --- | --- |
| 业务语义 | 两项工作改变同一阶段、反馈受理、候选退出或交付判定，即使文件不同 | 由语义所有者固定共同规则及验证，再安排实现和消费；未稳定者留草稿 |
| 公共契约 | 同时新增/改名 DTO、错误、游标、端口、codec 或配置字段 | 固定生产者/消费者范围和失败行为，指定共同契约修改所有者 |
| 持久化/事务 | 共改 schema 版本、CAS、唯一约束、快照或提交 checkpoint | 统一格式与事务安排，明确合入顺序；禁止各分支独立发布互不一致的格式 |
| 组装与代码所有权 | 多任务同时改同一构造、注册、dispatcher、客户端或 UI 组件 | 共享段由一个任务拥有，其他任务消费固定边界；后合入者做最小组合验证 |
| 测试与验收 | 改共同 helper/fixture、runner 或断言，可能影响兄弟场景 | 环境/断言所有者核对所有受影响场景；不能削弱夹具或业务门槛来显示并行成功 |
| 外部资源 | 相同仓库 common-dir、home、终端、账号设置、进程环境或发布目标 | 创建独立现场或安排串行使用；不同 worktree 不自动解除仓库锁/平台资源冲突 |

共享 SQLite 或唯一 actor 不要求所有研发串行；稳定契约下的纯规则、查询、adapter 和 UI 可分别开发。共同状态/事务不变量变化时集中协调。文件重叠也不自动变成业务硬前置：先判断是语义依赖、接口依赖、合入顺序，还是仅资源排期，分别记录。

Codex 开发的正式交付依赖维护于 execution-map；SteerLine产品自身的依赖由中心 Task DAG 表达。拟议共享修改和资源排期写入本轮 drafts/ 草稿；正式条目说明在 execution-map 中与已确认范围对应，实际任务 Spec/Plan 落实范围与条件；它们不自动新增中心边或旁路领取状态。

## 用于 roadmap 的步骤

1. 先读判断规则，按功能地图找主要模块及稳定输入。核对当前源码和证据，标出拟议或尚未验证的能力。
2. 每个候选任务写清产品成果、主要所有者、输入/输出契约、共同语义/代码/资源、最小通过条件；列明范围内是否改变接口或格式。
3. 对实际有交接或共享影响的任务对，选择上述安排，分别记录可固定、可开工和可合入条件。不要求全任务做无意义的两两矩阵。
4. 共同基础按真实独立成果拆分。若没有独立通过条件，或双方核心语义尚未稳定，保持同一交付任务或留草稿，不能制造只写签名的“已完成前置”。
5. 在草稿阶段按[验收流程](../acceptance/README.md)规划自动/人工行为，建立任务专属的一份或多份 acceptance 文档并从草稿链接，单份文档不覆盖多任务，未定内容明确标注。鼓励同时识别独立验收活动并安排，但不强制串成固定执行路径。人工组织/判断为主的阶段活动不作为任务；自动验收可独立成任务或纳入实现。少量人工项尽量后置，标明参与人/窗口、截止阶段与实际阻塞范围，避免成为无关任务的开工前置。
6. 纳入执行前明确独立 Spec/Plan、任务身份及 acceptance 记录，经用户确认写入 execution-map；启动时固定实际文档commit，当前空正式表不从草稿回填。Spec 写职责、契约及硬前置；Plan 写共享段所有权、集成顺序和最小测试计划，不让本页成为隐式执行要求。纳入不要求删除草稿条目；真正执行完成后移除，草稿无需维护实时状态。

建议在本轮关系说明中使用以下简表；每项任务仍遵循一个任务、一份 Spec/Plan、一个实现 PR：

| 任务 | 成果 / 主要模块 | 契约变更与消费者 | 共享冲突与所有者 | 开工条件 / 集成顺序 | 最小测试与验收安排 |
| --- | --- | --- | --- | --- | --- |
| 本轮任务 ID | 可独立交付的功能 | 既有契约或具体新增边界 | 语义、共享接线或原生资源 | 硬前置、未稳定点、后合入核验 | Plan 的组/场景、环境；acceptance 链接，人工参与/窗口及可后置范围 |

该简表不是新增任务状态或自动调度协议。架构改善优先解决反复出现的共享修改和上下文负担；不为提高表面任务数量拆出大量无法独立验收的半成品。

## 常见拆分

**新增执行状态并展示。** 先确定 Execution 状态含义、持久记录、查询 DTO、回执/失败与恢复行为；若存在可独立交付的契约基础，先真实交付再让后端和 UI 消费。后端实现与页面展示在稳定边界下分开推进，app/client 接线指定所有者。UI 使用夹具证明展示，不以此宣告真实执行或整链已通过。

**两个后端的局部改进。** Codex 历史分页与 Orca 原终端诊断，在原契约不变时可以分别改进；若同时需要更改共享 Codex codec、能力报告、来源身份或权限资源格式，先统一边界与所有者，再分开实现。

**安装/调试入口与界面改进。** 窗口排版、输入提示可以消费当前产品接口；exe/home 传递、console 启服、命令重执行和服务实例核验是共同集成面，需明确所有者。两份项目配置仍指向同一 run_dir 或仓库时，不能用独立 worktree 声称资源已经隔离。

**多个原生页面。** 共享导航、输入组件与客户端协议先提供真实基础，再分别接领域页面；各页面保留自己的目标/版本/草稿。尚未存在的规划/人工事项/审核接口等2.2/2.3流程成熟后固定，必要权限验证纳入实际会话接入；不能把演示页面视作前置能力交付。

## 测试与独立性证据

测试采用[总入口](../testing.md)和[E2E 约定](../../tests/e2e/README.md)。以下是定位和选择建议，不是每个任务默认必跑的矩阵；每次由实际改动的 Plan 确定级别、组/场景、环境、命令、时点、通过条件和证据。

| 影响范围 | 当前验证定位 | 说明 |
| --- | --- | --- |
| 单 owner 规则 | [任务纯规则用例](../../internal/task/model/taskgraph_test.go)、[执行恢复隔离用例](../../internal/execution/candidate_resume_isolation_test.go)、[控制台交互用例](../../internal/console/interaction_test.go) | 定向选受影响规则；这些用例不证明所有同模块变更可以独立 |
| 公共边界、端口、组装 | [依赖守卫](../../internal/testkit/boundary)、[真实 app/client 接线](../../tests/integration/candidate_console_test.go)、[L1 选择](../../cmd/test-feedback/scoped.go) | 接线或公共契约变化核对实际消费者，fixture 保持正式接口；源码闭包按已有平台守卫核验 |
| 交付与恢复组合 | [登记场景](../../tests/e2e/suites.json)：delivery.basic、recovery.waiting-merge-restart（controlled）及对应 live 场景 | controlled 和 live 分别选择；delivery.basic 只证明 A 交付及 B 就绪，不证明 B 的后继执行 |
| 双本机候选与原生资源 | 登记的 candidates.local 及[本机验收摘要](../acceptance/archive/completed/candidate-acceptance.md) | 按真实身份、目标和构建判断覆盖；当前 E2E runner 的串行/权限/清理约束保持，账号与终端不能默认当作可并行资源 |

已有[任务/执行/控制台独立 owner 探针的固定历史记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/three-line-10-independence-cleanup/independence.md)证明了当时几个明确规则可在所属边界被修改和检验；它们不是所有功能组合或当前任意接口变化的独立性证明。历史失败、未触发分支、原生未测范围和不同源码基线通过[验收摘要与历史快照](../acceptance/README.md)追溯，不能算作当前通过。后续新增证据链接到对应模块/边界的最小摘要，原始运行产物放临时目录，不要求规划者追读全部归档 Spec。

没有专门契约测试时，先使用受影响 owner 用例和真实组装入口；只有确有未覆盖边界才新增最小场景/环境能力，并在 Plan 说明。新增契约/测试说明形成后更新本页链接，保持正文由对应所有者维护。

## 持续维护

模块职责、公开接口/持久格式、共同构造、测试 owner、资源归属或支持范围发生变化时，随对应实现 PR 更新本页受影响条目和链接；草稿前置及合入建议更新本轮 drafts/ 草稿；正式清单随用户确认范围、依赖及真实交付更新 execution-map，不把完成任务表积累在此。仅内容未改变的核对不新增维护记录或任务。

每次更新确认：当前与拟议状态准确、共享冲突及所有者明确、契约/源码/测试入口存在、证据未越过其覆盖范围。形成独立文档后把本页中的详细说明收敛为摘要与链接，废弃规则删除或按独有追溯价值归档。常见冲突可反向推动小端口和领域边界改进；反复跨多个模块修改时先复核任务拆分与契约归属，再决定是否需要结构调整。

纯指南、链接和规划规则修改只做文档静态检查，不运行产品或模型验收。它不建立新的并行执行引擎、自动评分、通用插件或旧格式兼容机制。

## #84 / #45 历史交接

用户于 2026-10-09 明确允许 #84 与 #45 独立并行实现；共享状态、格式、流程语义由 #84 交付，主干集成先 #84 后 #45，#45 的 HTTP/client discard 接线仍由原执行者负责。#84 提供 WorkflowRun `status/flow_ref/step_id`、输入/结果版本绑定，产品 CandidateSummary 的同快照步骤、源执行事实及报告权限边界；SQLite schema 18、runtime 20、worker-flow-v2、Orca execution-state codec 11。#45 后续消费真实字段和结果，不复制转移逻辑；该交接已交付，#44 后续删除由 #91 规划。完整状态按 [execution-map](../roadmap/execution-map.md)核对。
