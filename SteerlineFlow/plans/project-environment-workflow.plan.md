# Plan：按项目 × 运行机 × 链路组织环境准备

2026-11-01 收简稿。[Spec](../spec/project-environment-workflow.spec.md) · [适配指导草稿](../../SteerlineProjectGuideDoc/environment-adaptation.md) · [此前验收](../testing/acceptance/project-environment-workflow.md)。用户已授权本轮实施环境准备流程解耦，工程验证进行中；项目初始化适配不做，不维护安装现场。普通开发，不启动 roadmap。

## 1. 差距、范围与最小交付

基线 `752b1ed1df4e745c663fb2453ec029798d435d62`，既有功能 PR [#416](https://github.com/erranliu/SteerLine/pull/416)。旧 G09/V05 和 10055 字节规则的结果只属于原版本，不覆盖本稿。

本次收简后的用户流程是“选项目/环境 → 首次适配一次生成配套草稿 → 整体审阅并一次 Git 固定 → 用户准备 → 调用验收”。产品仍按项目×运行机×链路管理资格，保留模型操作参数、三轮预算、同目标占用、未知只对账和 Task 准入事务。

| 材料/事实 | 唯一职责 |
| --- | --- |
| 通用指导及生成/执行/报告协议 | SteerLine 提供调查方法和调用契约，无项目工具默认政策 |
| 项目入口 environment-project.json | 定位公共政策及唯一正式测试清单，不列环境检查或生成参考 |
| 本键 environment.md + 验收脚本 | 文档唯一声明要求、checks 和阻断 gaps，脚本实现；运行补充文件只用 dependencies，验收不依赖生成记录 |
| 原活动/请求 | 冻结源提交、读取文件及少量公开目标事实；不要求项目维护 input/context 或先提交输入 |

首次初始化适配是未来能力，本轮只定义整体输入/输出及无预提交的边界。现阶段可由用户/授权 Agent 一次准备草稿，不要求先接受中间材料。日常更新也整体展示标准与脚本差异，正式采用后再验收；不禁止生成标准变更草稿，但禁止脚本执行时改标准。

## 2. 实施 owner

| owner | 变更 |
| --- | --- |
| SteerlineProjectGuideDoc 及导航 | 收简指导，保留实际调查→项目接口二级关系；正式接线前统一现有规范/添加项目方案的旧入口表述 |
| 项目材料公共解析、internal/environmentadaptation/model | 项目入口纯解析共用；manifest3 增 required_checks/gaps，删除独立 input；文档直接决定是否有阻断缺口 |
| internal/app 环境材料/适配/资格 owner | 删除硬编码项目路径、context 文件匹配与预提交守卫；从正式文档取标准，目标事实只在受理时冻结 |
| internal/environmentadaptation 与 SQLite | 保存唯一标准来源/目标事实和原请求不可变绑定，严格检查报告；必要格式换代，不兼容或迁移旧安装数据 |
| internal/testmanagement、app 的 test_documents/project_materials/test_records/维护 | 共用项目入口及同提交正式清单，覆盖快照、引用 allowlist、任务准入和记录，不保留第二清单 |
| internal/prompts、协议镜像/schema/样本 | 固定规则只保留协议；同包短指令仅接受定位事实，删除 context 必读和项目工具实现细节 |
| HTTP/客户端/Web、testenv/driver/夹具 | 去掉 context 导出/提交操作，显示整体草稿；测试项目专属原始报告解析与生产 owner 隔离 |

不新建材料服务、注册表、生成参考配置、要求继承引擎或 Runner 能力契约。manifest3 与报告 version2 分别处理，统一项目入口不改变测试清单格式或选择规则。

## 3. 步骤与完成条件

### R0：指导与首次适配的整体边界（D01、A11/A16 的指导审阅）

交付收简指导：真实项目调查后一次形成入口、环境要求文档/检查声明及脚本，缺失的必要公共说明/协议副本也作为同一草稿包处理。无需环境 ready、input/context 或任何中间提交；指导不自动注入日常验收。

不在本次实现初始化按钮、调查器、落盘/发布或纳管 API。未来初始化接线须核验已有只读渠道能共同提供完整指导/协议与固定项目快照；不能假定 Runner 有跨来源文件/临时快照覆盖能力，不能用内联规则或要求项目预提交材料兜底。此依赖及具体接线/验证归未来初始化 Plan。本轮独立审阅指导草稿；正式实际模型接线再审核最终正文、样本和渠道。

### R1：唯一要求文档及共享测试来源（A03–A06、A14、A15、A17）

1. 项目入口 version1 仅保留 version/requirements/testing_catalogue；manifest3 增 required_checks 和显式 gaps 数组，检查/阻断标准只在环境文档。删除 input、跨文件检查集合对账和 context Git 字节匹配。生成结果顶层 gaps 只能逐项等于文档投影，不是另一来源。
2. 从同一正式 environment.md 冻结全部业务 checks，追加 terminal.tty；拒绝漏/重复/未知/failed/incomplete。运行依赖唯一补充清单为 dependencies，公共政策或辅助工具被实际引用就按运行依据计入，纯调查来源不计入。
3. 环境验收、测试安排生成/固定校验、任务材料/准入、记录/页面共用项目入口及同受测提交；替换原固定项目路径和短指令，保留独立的固定测试执行/进度协议。缺实际正式清单/L2 时如实阻断，不以另一清单或空清单兜底。
4. 复用原 regular blob、路径/重复 JSON 键/字段与限额检查。正文/脚本/机器段必须配套，检查声明只存在文档，manifest2 不兼容；保留原 1 MiB/Runner 快照/64 KiB 报告限制，不加任意新限额。

5. 材料查询、开始验收和候选准入直接从固定文档检查 gaps，非空/缺字段/格式错即阻断，不查询原生成记录补资格；环境文档改变为有缺口时也不沿用旧 ready。缺口解决后正常 Git 更新并重新验收；目标工具尚未准备按现场 checks 检测，不误判为无法开始检测的标准缺口。

完成条件：不同项目/链路可采用不同文档和检查集合；旧固定路径及 input/context 均不存在时可读取正式接口。坏格式、错误路径、缺依赖及真实缺口有明确诊断，不让模型补通过。

### R2：事实冻结、请求与资格（A02–A09、A15、A17）

1. 把目标公开事实作为原活动/信封数据冻结，用同包短指令的少量参数引用项目/键、adapter/target、明确平台/入口、结构化权限和公开发布目标；不提供 Git context 路径，不透传 settings/凭据，不把长报告或规则塞进指令/schema。
2. 材料查询只返回正式文件/来源、缺项和只读目标预览；删除 expected_context/context_digest 及导出/提交守卫。缺正式文档仍可定位目标/查看适配入口；未来无项目入口的首次整体适配只按 R0 设计，不冒充本次已实现。
3. 原生成入口内明确两种读取方式：本环境无文档时，以完整指导/协议和固定源码调查实际程序，支持 Runner 已有整树快照或核验充足的显式文件集，不只给入口/清单；已有有效文档且引用足够时按有限引用更新。不增加模式配置、流程服务或用户前置。首次整个项目缺入口的初始化仍归 R0 未来范围，不冒充已实现；原渠道无法提供完整必读文件时明确能力缺口，不内联或预提交兜底。

4. 旧脚本/依赖删除或改名、旧文档机器段错误时，保存可读原文、缺失路径和源摘要，自动用同一固定源码调查修复。越界/其他环境引用不解引用，不自动降原标准；新请求实际必读的完整协议/指导、核验源码及项目政策/正式定义缺失仍阻断，未知事实写文档 gaps。生成不要求旧接口先通过；验收仍严格要求正式依赖齐全且 gaps 为空。
5. 资格依据按 Spec §5.2：本环境文档（含 checks/gaps）/脚本、执行协议、实际公共/运行依赖、全部 L2、规范项目入口及原执行/发布上下文；无 input 或生成状态资格，生成参考/指导/模型不计入。A/B、公共依赖变化和原候选事务保持。

运行依赖仍在原 owner 全量读取/哈希且缺失阻断，固定 Git 工作区不裁剪；Runner 阅读引用只列环境文档/脚本、执行协议及共享项目来源，manifest 定位运行辅助文件。用重复路径不挤占正文预算的反例和真实原请求验证，不扩大预算或新建引用中间层。
6. 原请求重放先读冻结信封，再验证新配置；事实、来源和检查绑定不可变。历史完整报告保留回看，不再强制送入生成短指令；验收后继轮初始文件的完整 previous、原停止和封存义务不变。必要时统一持久格式升版，无双版本读路径。

完成条件：一次受理即可冻结事实，不要求用户 Git 提交事实；原配额/预算/未知/取消/停止保持，生成不授予或撤销 ready。

### R3：协议/输出及独立审核（A06、A11、A16）

同步环境与受影响测试安排短指令、完整协议、输出 schema、正式镜像和请求样本。要求/脚本配套输出供整体审阅，标准变化有明确 diff，运行时只执行已采用文档，不另维护脚本标准。删除旧 context 引用、完整历史塞入生成输入和项目版本/工具实现政策；少量动态事实不包含固定行为规则，generate.materials 不内联完整材料。

SteerLine 只限定既有 ps1/powershell.exe、sh/bash 调用协议；Runner 核验其实际执行/workflow/通道能力，解释器和工具版本由项目目标现场检查。记录每份正文/实际请求完整 diff、前后 UTF-8 字节及所有必读协议总量和去向，独立审核最终正文/接线/测试资产；原批准和黄金样本更新不替代批准。先获批准再开展新版真实生成/验收。

### R4：界面与外部消费者（A01、A07、A09、A10、A14、A15、A18）

界面保留选环境和各自模型操作，移除 context 预提交，显示一个配套草稿包、来源和正常 Git 采用路径。正式材料的缺口从 environment.md 投影，无历史生成记录也可显示/阻断；旧接口待修与机器未满足分别说明。调查/按引用更新由原操作处理，不新加按钮；切换/迟到/双击/缓存和停止守卫不变。

测试夹具在独立 clone 一次形成入口、文档/检查和脚本，不人为设置 input/context 前置提交；正式 G/V 绑定同一实际生成字节。按场景显式隔离已有 Python/PowerShell 原报告校验器，维护自定义目录下真实工具/格式与 hash，错格式拒绝；不新增产品报告注册表，也不运行模型生成的自证脚本。

### R5：验证与交付（全部受影响 A 项，M01 后置）

受控/owner 验证无 input/context 前置的配套生成/正式采用，项目与环境隔离、唯一清单、检查精确集合及原生命周期。未来首次整个项目适配尚未实装时，仅 D01 审阅其整体草稿契约，不把 fixture 或手工准备算作自动初始化通过。

必须补三组反例：仅有项目入口/清单而无环境文档时，实际生成输入还包含指导和可调查的源码；旧依赖删除/改名或坏机器段时可调查输出修复草稿，但原材料仍不能验收；带非空 gaps 的同一 Git 文档在没有原生成记录的新 home/人工材料场景也零 Runner 验收提交，清空后仍须完整 checks/L2/PR。另核验真正必读依据缺失仍阻断、非法旧路径不读取、缺目标工具仍可检测失败，防止放宽安全或用新阻断替代现场检测。

独立批准后，复用真实生成场景为本机/远端两个目标输出文档/脚本，远端只生成；将有效输出整体一次 Git 固定，再以相同本机字节完成 TTY/完整 L2/唯一原 PR、外部核验、准入探针和全部清理。失败保留原证据，修生成来源后另目录补测，不手改输出/扩大预算。登记 A14–A18 及受影响 A01–A13 新版本结果，原 G09/V05 保持历史结论。

R1 以 R0 确定的材料职责为前提；R2 依赖 R1，R3 审核 R1/R2 的实际接线，R4 依赖最终材料/协议，R5 在 R1–R4 和独立批准后执行。本轮已获用户实施授权，工程验证进行中；R0 的整个项目初始化接线仍留未来。

## 4. 验证入口与证据

遵循[测试指南](../testing.md)、[路径清单](../paths.md)与[验收流程](../testing/acceptance/README.md)。实施时固定新的源码 SHA、协议版本和实际命令。独立 scope 继续为 `C:/SteerlineDebug/wt-68d4-project-environment-workflow/`，本次另取 `requirements-decoupling` 子范围，每个命令使用新绝对 out；日志、失败及清理证据不覆盖此前输出。

| 时点 / 验收映射 | 最小入口与环境 | 通过条件 |
| --- | --- | --- |
| 当前设计修订；D01 | `python scripts/check_doc_links.py`、`git diff --check`；新增指导另查 UTF-8、正文及字节账本，独立只读审阅 | 导航/链接/空白零错误，A14–A18/D01 有映射；一次配套生成/采用、单一要求来源、二级职责与未来能力边界一致；只记文档与指导审阅，不算业务验收 |
| R1/R2；A03–A06、A08、A14、A15、A17 | `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/environmentadaptation/... -count=1`；本地 owner | manifest3/checks/gaps、冷启动无生成记录也阻断、空 gaps 不授 ready、路径边界、现场失败、原 L2/TTY/PR/封存及依据隔离正反例 |
| R1/R2；A02–A06、A09、A14、A15、A17 | 分别用 test-feedback 新 out 跑 `./internal/app -run 'Test.*Environment' -count=1`、`./internal/storage/sqlite -run 'Test.*Environment' -count=1` | 首环境调查的实际输入含源码/指导，旧依赖删除/改名可修而验收仍拒绝，核心依据缺失/危险引用阻断；另测同提交冻结、零副作用、不可变与重放，新用例须被选择器命中 |
| R1/R2；A14、A17 | test-feedback 新 out：`./internal/testmanagement/... -count=1`，再单独新 out 跑 `./internal/app -run 'Test.*(TestManagement|TestDocument|TestRecord|ProjectMaintenance|Environment)' -count=1` | 测试安排/材料/记录及环境消费同提交唯一清单，旧固定项目路径缺失可用；实施时核对每类新增用例非零命中，缺失则显式补原 owner 选择器 |
| R2/R4；A07、A09、A15 | test-feedback 新 out：`./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -count=1` | 材料来源贯通现有路由；认证、allowlist、严格参数和查询边界保持 |
| R3；A11、A16 | `go test ./internal/prompts -count=1 -v`、实际 app 请求断言，以及最终独立正文审核 | 实际 diff/字节及总量/内容去向完整，只有协议同源；无项目路径清单、版本政策或 inline 兜底 |
| R3/R4；A13、A14、A18 | test-feedback 新 out：`./tests/e2e/internal/testenv ./tests/e2e/internal/driver -tags=e2e -count=1`；另取新 out 跑 `./internal/testkit/remotetest ./cmd/test-feedback -count=1` | 项目材料按声明读取；场景专属校验器对自定义目录原始报告独立核验，错格式/改工具拒绝，原 PR/L2/Run 身份及清单匹配保持 |
| R4；A01、A06、A07、A09、A10、A14、A15 | 直接在 PowerShell 7 运行 `./scripts/test-web.ps1 -EvidenceDir <新Web目录>`；扩展既有 environment owner 用例 | Web/HTTP/构建通过；无生成历史也显示正式文档缺口，区分旧接口失效/标准未知/现场工具不满足，无串对象或新增操作步骤 |
| R4；A01、A07、A10 | `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`，`STEERLINE_WEB_EVIDENCE` 指向新 browser 目录 | 宽窄屏、键盘、环境切换与要求来源显示；finally 恢复环境变量，保留截图/trace |
| R5；A04、A05、A08、A12、A14、A17、A18 | `go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario 'testing.project-environment-readiness' -out <新L3目录>` | 两项目/环境和自定义路径闭环、前后准入及失效隔离；不直接写 ready，不将 fixture 当真实生成 |
| 代码修改后及交付前；全部受影响自动项 | 分别运行 `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`、`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 固定 l1.minimum、smoke.delivery 完整通过，无 skip/零命中/未知清理；新增场景若需登记则同步清单及执行器，不默认跑 Full |
| 独立批准后 R5；A13-G、A18 | `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario 'testing.project-environment-generation' -out <新L4目录>` | 实际两个目标生成，项目材料无旧路径依赖，原始输入/输出、Git、gaps 和停止证据齐备 |
| G 完成并 Git 固定后；A13-V、A18 | `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario 'testing.project-environment-local' -out <新L4目录>` | 用相同生成字节完成 TTY/完整 L2/原 PR/外部核验/准入/清理，未扩大到远端验收 |
| 可用构建后；M01 | 按原专属验收安排用户走查，参与窗口待安排 | 能指出准备对象、项目要求来源、生成/验收模型及下一步，人工结果单独登记 |

逐个检查实际命令退出码与命中用例，不用下一命令成功掩盖失败。Web 构建、原始 npm、调试包及同源码 live 串行；L3 总并发不超过二。当前实施已进入运行时正文、接线及消费者修改，按上表执行工程检查、最终独立审核及新版真实 G/V；不以早先设计审阅代替。

## 5. 真实验证与授权边界

复用独立 STEERRUNNER_EXE、本机/远端目标 profile、专属项目配置及完整源码 SHA、原正轮次超时/一小时生成期限和三轮预算。正式输入是已知项目源码与实际政策，目标事实由原请求冻结；不预先创建或提交 input/context。真实验收前正常维护验证 base 包含采用的材料，不能将它提前成为生成步骤。

只用独立 home/clone/数据库，保留输出与失败现场。专属测试 refs/PR/精确合并按此前用户明确授权的测试项目和范围，不操作业务主分支、不部署或自动合并功能 PR。本机 Codex 的结果不扩大为所有链路、报告格式或未来初始化已通过。

## 6. 完成条件

本轮交付环境准备解耦实现，须有全部受影响自动项、最终正文/接线独立批准及新版本真实 G/V；未来初始化功能另行实现验收。D01、历史通过或人工体验均不替代缺失工程证据。

## 7. 本轮指导正文与独立审阅

此前收简稿为 6667 字节，SHA256 `b5468e0f1a63bf0cc7e6e89be5c5bdda32692fab71f3a00cbb45303bc2893887`，其独立通过仅覆盖旧稿。本轮修复三处流程耦合：在同一文档明确阻断 gaps 及顶层投影，首份环境材料调查完整源码，旧失效引用作为修复事实而非生成前置。新增内容没有移到另一份配置/文件，未增加模式配置、服务或框架。

实际正文变化集中于指南的调查步骤、环境文档职责、缺口独立阻断和自动调用段：增加旧依赖删除/改名及危险引用处理，区分真正必读依据与待修旧接口；明确新 home/人工材料无需生成记录，目标工具未准备仍可现场检测。新增 1225 UTF-8 字节的业务原因是使生成、材料采用与验收各自有完整依据和守卫，未复写项目工具政策或报告 schema。

| 文件 | 相对基线 HEAD 的 UTF-8 字节 | 本轮实际正文 SHA256 |
| --- | --- | --- |
| `SteerlineProjectGuideDoc/environment-adaptation.md` | 0 → 7892（上一稿 6667 → 7892） | `9f6fd073fccbb570df19cbe552a069875f26861d492bf701f7d9a5f1e9f3009d` |
| `SteerlineProjectGuideDoc/README.md` | 3102 → 3319，本轮未再改 | `9104f26e918426adcf2088ed82771eba65159687fd689bcdf746db463e910a52` |

新文件全文即相对基线的实际新增差异，README 仍用普通 Git diff 审阅。独立只读审阅者 `/root/prompt_review` 已核验上述最终正文/hash，并审核 Spec、Plan 及验收映射：通过，无阻断问题。确认缺口由 Git 文档独立判定、首环境调查材料完整、旧接口失效可修而验收不降级，未增加文件层/配置/框架，原事实冻结及生命周期保持。旧稿批准不覆盖本版；本次批准只限设计/指导及导航，正式接线仍须审核最终实际请求。

本轮文档检查为 633 文件、3993 本地链接、0 错误；空白/UTF-8、含 gaps 的 manifest3 示例、未增加 input/context 的最小布局及 A01–A18/D01 映射通过。未改变运行时提示词/样本/请求或产品代码，未运行业务模型/产品测试，不把文档检查算业务通过。


## 8. 本轮实施状态

用户已授权实施 R1–R5 的环境准备解耦，R0 的整个项目初始化接线不做。上一节“仅文档”的结论属于设计审阅时点；本版已修改实际消费者、协议和界面，最终指导7983字节、SHA256 `d56cdd2c1fdc26b52da1e3a85f950cbf128a799bcdba90ad5ccae204d868fa2a`。当前正文/接线经 `/root/prompt_review` 独立批准，实际差异、字节账本、问题修复和新测试结果统一见[专属验收 §10](../testing/acceptance/project-environment-workflow.md#10-环境要求与调用协议解耦实施2026-11-01验证进行中)。工程验证及新版真实 G/V 进行中，旧 G09/V05 不作为本轮通过。
