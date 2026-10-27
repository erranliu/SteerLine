# Spec：L4远端测试统一入口与64G配置

2026-10-26。状态：按五项审计修订，已授权实施；实际结果只见专属验收。普通开发，不进入roadmap。依据现有[测试规范](../testing.md)和[项目环境适配](project-environment-adaptation.spec.md)。[Plan](../plans/l4-remote-tests.plan.md) · [验收安排](../testing/acceptance/l4-remote-tests.md)。

## 1. 目标及边界

将两个已有远端入口及308环境适配纳入既有L4/live，统一使用台式机64G的一份远端profile；通过原test-feedback输出正式受管报告。只补配置读取、三个场景接线及结果校验，不新增测试执行器、配置框架、机器池、远端调度或Runner接口。

原有两个独立入口为`scripts/test-project-progress-live.ps1 -RemoteProfile`与`scripts/test-execution-profile-discovery.ps1 -OrcaEnvironment`。本次将其远端模式登记为L4，同时接通实际选择、构建、期限及证据校验。原本机入口和L1/L2/L3保持自身范围，不要求它们配置远端。

## 2. 唯一共享远端配置

外部文件建议固定为`C:/SteerlineTestConfig/l4-remote-profile.json`，内容直接复用现有`executionconfig.Profile`（id/name/execution/workflow），不增加外层schema。仓库维护一个脱敏示例及用法，实际文件不入Git，不读取或写入安装home作为隐式配置源。

| 内容 | 要求 |
| --- | --- |
| profile | 采用现有`orca-remote-desktop64g`；不是项目ready记录 |
| target | `orca-terminal` / `orca_environment`，以稳定environment ID定位；当前64G ID为`e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`，名称仅供显示 |
| settings | 沿Runner公开格式记录本机Orca路径、远端Codex路径、模型/effort及权限；初始沿既有gpt-6-luna/high配置，工具须为长期可用路径，不能依赖已清理的测试包 |
| workflow | 既有development/2，不配置临时Run/runtime/terminal/worktree身份 |
| 凭据 | 保留现有执行机和本机工具各自登录，不复制token/认证正文进入profile或共享报告 |

正式L4读取`STEERLINE_L4_REMOTE_PROFILE`指定的绝对路径；脚本显式`-RemoteProfile`优先，否则使用该环境变量，二者进入同一读取/校验逻辑。发现入口的远端模式改为从profile取target/transport，不再独立拼接`-OrcaEnvironment`。不增加历史参数兼容或名称搜索回退。本机模式仍显式选择，不能因缺远端配置自动转本机。

每次执行只读取一次，校验后冻结到本次独立证据目录并将同一内容用于所有下游调用；开始后修改共享文件不影响在途测试。记录脱敏配置和摘要、所选源码/项目提交、实际远端身份及固定Runner version/SHA/resource。Runner沿现有`STEERRUNNER_EXE`显式选择并固定本次构建，不混入机器profile；测试运行目录和项目材料也不属于机器profile。

统一的是profile.execution机器配置，执行摘要以该字段规范化JSON计算。场景可显式设置自己的workflow/prompt_overrides（如进度禁止发布、适配prepare/publish），不得改写target/transport/Agent/模型/权限。目录API通过同一profile_id选中冻结transport；实际Run的冻结execution须与共享快照一致，不能只比较配置文件路径。

选中远端场景时，缺配置、无效/错本机target、不可达或实际目标错配必须明确不通过，不能skip或fallback。纯配置问题在创建测试资源和业务输入前拒绝；真实连接失败记录原错误并结算已创建的自有资源。基本连通检查不替代Agent环境准备或项目验收。

## 3. 三项L4范围

以下为登记ID；单项通过`go run ./cmd/test-feedback -level=L4 -suite=<suite> -env=live -scenario=<ID> -out <全新绝对目录>`执行。live沿现有规则一次精确选择一个场景，三项可分别执行，共用冻结自同一文件的远端配置，不互相依赖执行顺序。

正式入口输入明确如下；脚本只单向转换参数后委托test-feedback，不由执行器反调脚本或重复解析共享profile。输入冻结到原E2E Run信封，子进程不依赖被过滤的环境变量。

| 场景 | 专属输入/前置 |
| --- | --- |
| 目录 | 仅共享profile、Runner及Web/Edge/本机目录工具；不要求项目、Unity、材料生成或模型业务登录 |
| 进度 | `STEERLINE_PROGRESS_PROJECT`、`STEERLINE_PROGRESS_COMMIT`为原SteerlineTest路径/完整SHA，`STEERLINE_L4_ROUND_TIMEOUT_SECONDS`为适配正秒数；GitHub授权、该项目配套材料、Edge及原程序依赖 |
| 308 | `STEERLINE_L4_PROJECT_CONFIG`为既有格式测试项目配置绝对路径，`STEERLINE_L4_PROJECT_COMMIT`为该仓库原AIRefactor完整SHA，`STEERLINE_L4_ROUND_TIMEOUT_SECONDS`为正秒数；项目材料、Unity/资源/Gitea授权。仓库路径取显式项目配置，不隐式读取安装版 |

材料生成/审阅/提交是运行前的维护步骤，材料必须包含在输入的完整Git提交中，不要求每次L4重新调用模型；只有场景实际使用的本机模型前置才需验证。缺任何场景自身必需输入（包括受测提交的文档/脚本）在建资源前失败，原因保留在正式summary，不让无关项目/Unity阻塞目录查询。

已有项目能力以薄适配接入：标准环境文档引用原准备/测试/PR说明，标准脚本调用原入口并核验原报告，不搬迁或重写原测试。308复用`tools/run-tests.ps1 -Level L1/L2`及原XML/输入/清理核验；SteerlineTest复用`tools/test-project.py --level L1/L2`和原summary。两者只新增这对材料，独立Git提交供测试夹具固定，普通业务分支不自动改写。现有Orca配对及注册仓库沿用，以真实remote身份（允许`.git`后缀差异）匹配；本机/远端路径可以不同，测试占位URL不得用于真实匹配。已有项目配置的PR凭据环境变量引用须贯穿测试进程，报告只记录引用名。

| ID / suite | 复用及新增范围 | 硬验收边界 |
| --- | --- | --- |
| `configuration.execution-profiles-orca` / configuration | 复用现有独立调试包和`web/tests/execution-configs-live.spec.ts`，保留原本机目录断言，远端目标和Orca路径取共享profile | 正式Web/API取得真实远端Agent/模型/effort目录及来源；既有上下文未核实标记不升级；不提交任务或启动模型业务，不授予项目ready |
| `testing.project-progress-orca-browser` / testing | 保留既有ID和SteerlineTest原程序/Git blob核验、远端执行及同一Edge页面 | 原0→1→2→1→2→3六状态、同版本Runner字节与DOM/API一致、失败/重跑清零、原报告和源程序未被代写；原候选停止及诊断终端清理。进度任务自身不发布PR |
| `testing.project-environment-orca` / testing | 新增308真实场景：`merge-h-308-code`，以AIRefactor固定完整SHA和项目正式材料为输入；走SteerLine公开环境适配命令/查询 | 远端Agent准备并执行原脚本，完整308 L2+完整真实Gitea PR，原Run停止后closed/version=1报告满足硬标准才ready；随后实际新候选预留/派发门槛可通过，验证后停止并结算自有资源 |

### 项目材料和ready前置

机器profile不缓存或共享项目ready。进度场景的SteerlineTest夹具必须在本次隔离home内经正式环境适配获得ready；准备阶段使用Git维护的配套文档/脚本、完整L2及专属验证PR，单列前置操作/成本/结果。准备失败则整个场景不通过，不能注入数据库、直接改状态或借308结果放行。保留原进度任务不发布PR的断言；前置PR与任务执行分别记账，不新增快照导入或ready复用服务。

308使用`SteerlineFlow/environment.md`和`scripts/verify-environment.ps1`或`.sh`，由既有上游生成提示词生成/维护，经审查并Git提交固定，不在运行时临时拼装通过脚本。正式入口仍为项目清单的完整L2（当前Unity 2022.3.62f2、Android、两条Smoke及聚合要求），不缩为单Case、静态检查或全部L3。缺材料、资源、工具、凭据或规则须真实报缺口。项目要求以受测提交的AGENTS/testing/paths为准。

308场景另显式输入既有格式的测试项目配置（仓库及PR provider/repository），冻结公开设置及摘要。当前项目配置没有必需检查/审核字段，不新增这些字段；规则来自受测提交的环境文档/脚本和项目PR规则，服务端实际检查/审核通过只读provider核对。新验证base不假定继承业务分支保护；缺少规则或无法核实必需项时不通过。

两项目的环境文档使用一个`steerline-pr-acceptance` JSON段记录`required_checks`、`minimum_approvals`和`l2_summary`（完整L2原报告的Git忽略相对路径），由原上游材料提示词一并维护。驱动核对原业务分支保护、实际PR事实及原L2明细；308复用原项目只读XML/输入/清理验证器，SteerlineTest核对原summary的非零完整选择、结果与干净源码。此段仅为项目材料中的验收事实，不扩展产品PR配置或Runner协议。

固定源码流程为：由原AIRefactor SHA建立本次测试源码分支，预先选定验证base名，在夹具提交中固定含该base的配套材料，推送该源码分支并从最终夹具提交建立验证base。隔离项目配置.branch指向测试源码分支，启动/适配前核对其远端tip恰为夹具SHA，发生变化立即拒绝。分别记录原SHA和夹具SHA，不能仅本机checkout旧SHA却让适配API读取AIRefactor最新tip。运行中不改材料，不修改业务主分支。

夹具准备在适配开始前经正常Git流程创建/核实本次专属验证base；同一活动三轮共用，各轮使用Runner独立head。308真实PR仅合入该验证base，不合入AIRefactor或业务主分支；PR实际创建、必需检查/审核、合并及脚本资源清理均由项目脚本完成，驱动只通过公开接口观察，不能代合并来制造脚本通过。

完整L2+完整PR、最多三轮、单轮不可变期限、ready/not_ready二态沿用原Spec。开始前与未通过时，公开API发起候选请求必须被阻断且零新派发；通过后允许一个明确的测试候选，核验原Run绑定后即停止，不额外执行一次业务交付。脚本prepare/publish、result最终一次封存及未知操作只查询原身份的规则不变，不为L4提供旁路。

## 4. 执行、结果与验收

沿用[worker生命周期](../../docs/product-design/worker-lifecycle.md)、[换代恢复](service-generation-recovery.spec.md)及原项目适配收尾规则；测试驱动不重发未知业务、PR或停止操作。三项各建独立home、Git checkout/数据库、工作区、端口及输出目录；同一远端live串行，构建/Web目录仍按[路径清单](../paths.md)串行。缺少所需本机生成模型、Edge或执行机条件明确不通过。

进度及308适配单轮超时显式给定正秒数并冻结；两者外围测试总期限均须容纳最多三轮及既有准备/收尾时间，进度还须容纳原六状态观察。不能沿用较短固定总超时截断仍有效的业务预算。总期限不是另一套业务重试预算；超时按原stop和未知对账收尾，不能直接杀共享服务或其他任务。

接入`tests/e2e/suites.json`、`SteerlineFlow/testing/test-list.json`、L4说明及执行器。继续使用既有字段，远端chain可用明确的`orca-remote`；在原执行器显式接入这三项及各自必要证据，不建立通用proof注册表。目录、进度和环境适配分别校验实际目的，不能对所有live套用真实交付PR证明，也不能只凭脚本exit=0或result.json自然语言通过。

| 验收ID | 必须满足 |
| --- | --- |
| LR-A01 | 三项使用同一外部profile；参数优先级、冻结、缺失/错target/错目标拒绝有效；无安装回退或配置即ready |
| LR-A02 | 三项正式L4/live可精确选择且实际非零命中，清单/文档/执行器一致；缺配置/skip/空选择/缺证据不通过，原本机及受控测试不强制远端 |
| LR-A03 | 真实64G目录查询及来源/既有本机断言通过，零业务Run，清理完整 |
| LR-A04 | 正式ready前置和原远端进度六状态/同版字节/连续浏览器/原报告/停止清理全部通过，不降低原断言 |
| LR-A05 | 308同轮完整L2+完整真实PR+停止后封存报告→ready→真实候选准入；失败时零派发，三轮/期限不被改写，不伪造结果 |
| LR-A06 | 各场景的原提交/Run/PR/测试选择及完整终态、清理事实可核实；未知或清理失败不通过。报告、配置摘要及版本归属准确，不以受控证据冒充真机 |

本期不要求在64G上人为制造全套失败矩阵或消耗满三轮；三轮耗尽、重启和未知不重发沿用已有受控owner回归。真实首次未通过须保留本次完整记录，诊断后另建新输出复验。具体步骤及实时状态仅见[专属验收](../testing/acceptance/l4-remote-tests.md)。

用户收敛环境职责后，308 L4同时验收真实未就绪分支：用户预备变量/PATH，Agent只运行固定脚本、不发现或修复。要求逐项checks保留通过、不通过及未验证；前三轮原Run均停止且结果closed/v1、L2及PR未执行、独立provider核实专属base无PR，准入前后都拒绝且零候选，清理完整时，负向L4可通过，但环境仍not_ready。就绪分支及进度的ready前置仍须完整L2+真实PR，不降低原门槛。未知、缺报告或只写总失败不能当作该负向通过。
