# 验收：L4远端测试与64G共享配置

2026-10-26。归属[Spec](../../spec/l4-remote-tests.spec.md) / [Plan](../../plans/l4-remote-tests.plan.md)。当前目录真机及308“用户准备、逐项只检测”的未就绪分支L4已通过；完整进度及308环境全部满足后的L2+真实PR正向仍待验。旧失败和各轮原证据保留，下方按阶段记录，不能认定三项全部范围通过。

## 实施与审计落实

- 三项复用一份既有格式profile，只冻结一次并通过原E2E Run信封传递；同一execution摘要约束实际目录/API/Run输入，场景workflow允许各自必要覆写。
- 脚本仅向正式L4单向传参；三项有明确输入、选择、构建、期限和具体证据校验，没有新增执行框架、proof注册表或Runner接口。本机进度显式`-Local`保留原入口。
- 308从固定源SHA建立自有源码分支，文档绑定专属validation base后冻结夹具SHA，两ref同源，配置.branch指向测试源码分支；正式适配读取的SHA必须相等。材料须已包含在输入Git提交中，不在执行时生成通过脚本。
- 原项目完整L2和真实PR检查/审核/合并独立核验；Git文档记录PR规则与原summary路径，未扩展产品PR配置。保留原验证器Git blob检查、原L2明细及停止后唯一封存的结果关联。
- 进度使用自身正式适配ready前置，不能借308或注入状态；三轮预算、正的单轮期限、完整L2+完整PR与二态准入不变。外围期限覆盖三轮和准备/观察/收尾。
- 自有候选和环境活动均进入原Runner清理，验证分支/head仅在原资源停止后删除；未知、清理失败保留原身份与现场。建资源前的缺项也写正式summary和managed结果。

## 本次配置与来源

源码基线为`50a811dfe084f9132c5046baada2627aaeaed57d`加本次未提交修改，报告记录dirty及文件清单，未部署或修改安装home。证据scope为`C:/SteerlineDebug/wt-4b14-l4-remote/`，每次均使用新叶目录；修复前失败证据保留。

共享文件为`C:/SteerlineTestConfig/l4-remote-profile.json`，profile为`orca-remote-desktop64g`，environment为`e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`，模型`gpt-6-luna/high`，full_access、development/2。本机Orca为`C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`。

从安装配置只读核对公开字段后建立独立文件；后续运行没有安装回退。原临时`C:/sr-runtime/2ba0/01/native/codex.exe`已在测试配置中改为台式机长期安装路径`C:/Users/Administrator/AppData/Local/Programs/OpenAI/Codex/bin/codex.exe`。独立只读诊断实际运行`--no-daemon --version`，得到`codex-cli 0.162.1`、SHA256 `dd13bdb162a184fa04039488b234a8258ef9b76f84e926c505d5742b19249a6a`；原诊断终端close回执`ptyKilled=true`。证据在`diagnostics/tool-path-01/`，实际runtime为`f8616747-34af-4543-8487-ec6b5d8de4ff`。此诊断没有模型业务，不代表项目ready。

最终execution规范化摘要为`b858584bdd3f7188107c7baeb6ae5fd69eda1492f6efc18ce4721b433ecfa026`，在`test-results/L4/discovery-final-02/remote-input.json`和实际Web配置中一致。示例留在[仓库夹具](../../../tests/e2e/testdata/remote-profile.example.json)，实际配置和凭据不入Git。

固定Runner为`C:/SteerlineDebug/wt-4b14-environment-adaptation/diagnostics/merged-runner-02/steerrunner.exe`：版本`0.2.0-dev.3-r2`，源提交`74d594c5f00e32e97d2ee911db5ac787302022bc`，EXE SHA256 `d5b01edd4ac291a8ef1c750c59be2e2f29cdb6fd3d45907ad598aa9757f83f5a`，resource摘要`d5fd5e0303b1ad9c7333acb9b82f18f246e83b71e1edca83ddfb7d5c54004041`。各真实L4由STEERRUNNER_EXE显式选择，没有重新取安装版。

## 已执行结果

下表目录均相对于上述scope；查看各次summary、原JSONL、构建/场景及清理记录，不用受控通过替代真机结果。

| 验收 | 实际执行及证据 | 结果 |
| --- | --- | --- |
| LR-A01/A02/A06 | `go run ./cmd/test-feedback -out <diagnostics/remote-owner-final-03> -- ./internal/testkit/remotetest ./cmd/test-feedback -count=1` | 116项通过，零跳过；含配置/项目输入冻结、缺材料和溢出拒绝、精确登记选择、缺证据、错误来源、第四轮、未封存、缺/失败L2、未合并及错机器反例；前置失败有summary且零执行组 |
| 原预算/二态/恢复 | `go run ./cmd/test-feedback -out <diagnostics/environment-regression-01> -- ./internal/environmentadaptation ./internal/app -run '^TestProjectEnvironment' -count=1` | 35项通过，零跳过；未改变产品三轮、期限、未知零重发、封存与二态规则 |
| driver补充owner | `diagnostics/remote-owner-final-02/`，三个owner包 | 130项通过、4项既有显式debug-release/native入口未启用而跳过；不算这些原生专项通过，也不算三项远端L4通过 |
| 固定L1 | `go run ./cmd/test-feedback -level=L1 -out <test-results/L1/final-03>` | passed=true |
| 固定L2 | `scripts/test-l2.ps1 -OutDir <test-results/L2/final-03>` | passed=true，cleanup_complete=true；这是SteerLine受控L2，不是308 L2 |
| Web | `scripts/test-web.ps1 -EvidenceDir <diagnostics/web-final-01>` | 225条前端测试通过，HTTP/server owner通过；浏览器真机另见下一项 |
| LR-A03 真实64G目录 | `scripts/test-execution-profile-discovery.ps1 -RemoteProfile <共享文件> -SteerRunnerExe <固定EXE> -OutRoot <test-results/L4/discovery-final-02>`，委托configuration.execution-profiles-orca正式L4 | passed=true，cleanup_complete=true。真实Edge本机models、64G environments/agents/models均ready，来源分别probe/registered；冻结execution一致、context_unverified保留，零业务候选/Run |
| LR-A04 进度前置 | `scripts/test-project-progress-live.ps1 -ProjectRepository C:/Projects/SteerlineTest -ProjectCommit a161f31f2ea7537d95af0b0fde977329bf91dc12 -RemoteProfile <共享文件> -SteerRunnerExe <固定EXE> -RoundTimeoutSeconds 1200 -OutRoot <test-results/L4/progress-prerequisite-final>` | passed=false。Git该提交缺SteerlineFlow/environment.md，原git show退出128；summary保留准确原因，cleanup_complete=true，零执行组，未创建远端Run/PR或授予ready |
| LR-A05 308前置 | `scripts/test-project-environment-live.ps1 -ProjectConfig C:/SteerlineTestConfig/l4-308-project.json -ProjectCommit afe67b539e787fe05ce58876d612a6edb45e25f7 -RemoteProfile <共享文件> -SteerRunnerExe <固定EXE> -RoundTimeoutSeconds 1200 -OutRoot <test-results/L4/environment-prerequisite-final>` | passed=false。原AIRefactor提交缺SteerlineFlow/environment.md，原git show退出128；summary保留准确原因，cleanup_complete=true，零执行组，未进入Unity/L2/PR或准入探针 |

`discovery-01`暴露PowerShell 5的原生参数拆分；`discovery-02`暴露调试包冻结标志未接线；`discovery-03`暴露目录场景误用了要求任务身份的快照；`discovery-04`业务报告已通过但包装器把正常stderr当异常。均已修正并用新目录复验。最终目录场景包含真实Agent目录断言和长期工具配置；没有删原断言降低标准。

## 首次执行后的未验范围（历史）

两个项目需要按各自正常维护流程生成、审阅并Git提交环境文档/验收脚本及真实PR规则，再选择包含材料的完整SHA，以同一共享profile和新输出目录复验。现有上游提示词已补充PR规则和原L2报告定位要求，但本次没有替项目生成无条件通过脚本、自动修改业务分支或注入ready。

仍未验证：SteerlineTest正式环境适配前置及迁移后完整六状态连续浏览器；308远端Unity/Android资源、完整项目L2、真实Gitea PR检查/审核/合并、停止后封存、通过前后候选准入及自有PR/head清理的成功闭环。相关helper已编译和接线，不能把这点当真机通过。三轮耗尽/重启/未知的受控owner回归也不替代这些事实。

## 薄适配实施与后续真机验证

用户确认复用已有配对与原测试/PR链路。新增scope为`C:/SteerlineDebug/wt-4b14-environment-thin-adapters/`，独立源worktree在其`sources/`，两项目分支均为`codex/environment-thin-adapter-20261026`。原`C:/Projects/SteerlineTest`的main及`C:/Projects/merge-h-308-code`的AIRefactor未改动。各项目仅增加环境文档和具体薄脚本，未建通用框架或Runner接口。SteerlineTest固定材料提交`b3e3be7d7025b16838064bdb595bb31d59561d34`；308最新材料提交`63cf1c0838067463f31615d046f0729102ca6ae6`（06真机受测为`3736e306bb0e5fc945625d0539254b3ad7313f15`；其后补Unity路径参数转交，并明确Agent自行查找、核实及准备工具）。L4另生成含本次专属base与探针的夹具提交，分别记录来源，不能冒充业务主分支已合入。

沿用上述共享profile、执行摘要和固定Runner。308独立项目配置只读取原公开配置字段建立，必须明确native任务权威；远端/本机仓库仍按remote URL对应现有注册身份，本机C:与远端D:路径不同无需重新配对。传递已配置的PR token环境变量引用，未把token写入配置或报告。

| 范围 | 本scope证据 | 实际结果 |
| --- | --- | --- |
| 薄脚本本机校验 | `diagnostics/demo-prepare-01/`、`demo-fixed-01/`、`308-invalid-source-01/` | 演示项目原L1/L2通过；308错误固定源码被脚本拒绝并写未完成项原因，未执行Unity或PR |
| 真实演示适配 | `test-results/L4/progress-03/`、`progress-04/` | 两次均第一轮ready，原始完整L2通过，原Run停止，result closed/v1；真实PR [679](https://github.com/erranliu/SteerlineTest/pull/679)、[680](https://github.com/erranliu/SteerlineTest/pull/680)仅合入自有验证base。整项L4分别在私有store身份查询、PS5中文报告读取处失败，未到六状态完整验收 |
| 演示收尾 | `diagnostics/progress-02-cleanup/`、`progress-03-cleanup/`、`progress-04-cleanup-03/` | 原Runner停止事实核实后删除本次准确source/base/head；原失败保留。04的早期清理尝试亦保留，最终采用原confirmed清理回执，未重建已删除Runner命名空间 |
| 308夹具修复 | `test-results/L4/308-01/`至`308-04/` | 原任务配置、验收引用、控制回执错误已修；前四次尚未启动远端适配Run。04发现产品环境门禁普通错误不能形成最终控制回执，最终过期；改为既有领域拒绝`environment_not_ready`，未新增状态 |
| 308第一轮远端 | `test-results/L4/308-05/`、`diagnostics/308-05-native-read*.json` | 准入前正确拒绝，Run `61b937317d619e4dc54f9a6dbd02eb04`实际接受。Agent误把CRLF磁盘摘要当Git blob摘要，并发现138项缺失/长路径文件，未执行原L1/L2或PR。本轮经公开environment_stop停止、封存，原失败不改写；文档补充longpaths及从固定提交恢复，提示词明确Git blob字节 |
| 308失败收尾 | `diagnostics/308-01-cleanup-02/`、`308-02-cleanup/`至`308-05-cleanup/`，`308-05-stop/`及诊断终端close回执 | 原Runner确认停止且自有base无PR后，删除准确source/base；短暂断连导致的诊断终端关闭失败已按原handle核实为仍运行后关闭，ptyKilled=true |
| 所属回归 | `diagnostics/owners-final-01/`、`admission-receipt-01/`及既有`credential-wiring-01/`、`ps5-utf8-01/` | 所属包及门禁回归通过；中文原报告在PS5按UTF8读取验证通过。owner中既有显式debug/native跳过不能当真机通过 |
| 308原始失败报告 | `test-results/L4/308-06/`、`diagnostics/308-06-original-activity.json`、`308-06-stop/`、`308-06-cleanup/` | 准入前及时拒绝，Run `eaa3d985599ee1f2cdf0363cd459c442`实际恢复源码并运行prepare。原L1检查要求Unity 2022.3.62f2而默认安装路径不存在，全部3项L2保持incomplete且带同一真实原因，PR未创建；通过原environment_stop显式取消，原Run stopped、result closed/v1。仅执行第一轮，未宣称预算耗尽；活动取消后自有ref清理完成，原失败summary保留 |
| Unity路径及新版失败分支 | `diagnostics/308-06-unity2-evidence.json`及对应close、`308-missing-unity-01/` | 只读核对常用Unity目录/Hub清单未找到同版路径，不能据此断言全机没有Unity；诊断终端ptyKilled=true。新脚本将UNITY_EDITOR_PATH传给原-UnityPath；本机用明确缺失路径实际执行原入口，确认completed=true、3项incomplete及PR未运行。此项不冒充308 L2通过 |
| 最终固定回归 | `test-results/L1/final-02/`、`test-results/L2/final-02/`、`diagnostics/environment-final-01/` | 固定L1/L2通过，L2 cleanup_complete=true；原环境适配预算/二态/恢复等所属回归通过。589份Markdown的3681条本地链接零错误，git diff --check通过 |
| 进度05完整前置 | `test-results/L4/progress-05/`、`diagnostics/progress-05-cleanup/` | 第一轮ready、原L2及真实PR [681](https://github.com/erranliu/SteerlineTest/pull/681)合并、stopped及closed/v1均独立核实；整项在浏览器旧路径约束处失败，六段进度未执行。修复为显式场景root校验自己的home；同时修正收尾误把git命令送给SteerLine的问题。原Runner确认停止后，准确三个ref已不存在 |
| 进度06原生输入阻断 | `test-results/L4/progress-06/`、`diagnostics/progress-06-native-read.json`、`progress-06-stop/`、`progress-06-cleanup/` | 原完整L1/L2通过，publish原生输入缺少verified start receipt；Runner按原身份观察，未重发。公开environment_stop后原Run停止/封存、无PR，source/base/head核实删除；原失败保留，不用旧ready记录绕过独立适配前置 |
| 最新提示词及回归 | `diagnostics/environment-final-02/`、`test-results/L1/final-04/`、`test-results/L2/final-04/` | 35项环境所属测试通过、零跳过；固定L1/L2通过。提示词明确准备命令可执行，由Agent自行发现工具路径/准备依赖；不让默认路径缺失直接变成人工提供路径前置 |

测试驱动的必要修复只涉及真实URL匹配次序、合法command PR provider、HTTP/store身份区别、原报告UTF8读取、配置的凭据引用过滤、现有候选停止入口与原Runner清理。未知或清理失败现在同时使场景失败并保留scratch，避免仅内层保留、父进程误删。生产门禁用现有领域拒绝回执向调用者返回缺口。L2+完整PR、三轮及二态不变。

308最新复验为`test-results/L4/308-07/`，未取得完整L2及真实Gitea PR通过证据。Agent曾查到Unity 2022.3.62f2安装登记，进行工具发现；此前默认路径/Hub诊断不能作为全机没有Unity的结论。用户随后将职责收敛为“项目声明要求、用户准备环境变量、执行端只检测验收”，原自动准备测试已通过公开environment_stop取消，原Runner cleanup confirmed=true；`diagnostics/308-07-stop/`及`308-07-cleanup/`保存原回执及两个准确自有ref的清理事实。未启动后继测试。旧自动准备提示词/材料须按新职责修订，不能宣称新方案已实施。完整进度仍需新目录复验，不把已经补材料等同于308环境ready。


## 用户准备环境、远端只检测的本轮验收

2026-10-26。按用户最新要求收敛：项目Git文档声明变量/PATH、版本及模块要求，用户预先准备；远端Agent只运行固定脚本，不查找安装目录、不安装或修复、不设置环境变量/PATH/Git配置。Report新增具体checks列表，独立于L2及PR；逐项passed/failed/incomplete及原因，全部要求、完整L2和完整PR同轮通过才ready。三轮预算及二态不变，已核实的适配阻断回复触发原stop，避免等待用户在Run内修复。

证据scope为`C:/SteerlineDebug/wt-4b14-environment-requirements/`。共享profile/执行摘要与上述固定Runner相同。项目材料仍在独立分支`codex/environment-thin-adapter-20261026`：308受测材料`e755702be3e719beaedc92c884cdb98e46e1cda3`，SteerlineTest材料`8855502c8d65ddf92363549ba7e93f05471c3667`，均未合入原AIRefactor/main。308 L4固定夹具SHA为`065d9e5de30df87a2a7abb7882357cc2529acb8c`，活动`18fd3eb282e94a96866697221e71b21c`。单轮300秒，未在途中延长，三轮没有重置。

结论：真实远端两轮脚本检测和分项报告已验；完整L4 **验收失败**，第三轮Orca原生状态连接无法就绪、未发送实施输入，不能冒充三轮完整脚本验收或后置准入已验。总体保持not_ready，未运行项目L1/L2或创建验证PR。各轮独立报告，未跨轮拼通过项；原summary保留passed=false及当时cleanup_complete=false，后续人工清理事实单列，不改写原失败。

| 原轮次 | 原Run | 原事实 |
| --- | --- | --- |
| 1 | `45853d5ab73084be43753d07950e6c04` | 脚本实际完成11项检查：5通过、4不通过、2未验证；verified阻断回复后停止，result closed/version=1、completed=true，未超时 |
| 2 | `a236e3829baba9797cba522ca75706d3` | 脚本结果与第一轮一致；原生状态对账达到300秒期限后停止，result closed/version=1、completed=true；未重发原输入 |
| 3 | `57c2e99b2978589464c8d5c4ed0a6909` | Runner报告native readiness unavailable / state streaming timeout，no input was sent；到期停止，result closed/version=1仅包含初始未完成报告，completed=false、无checks。此轮不算完成检测 |

前两轮逐项真实结果：

| 状态 | 项目与实际值 |
| --- | --- |
| 通过（5项） | PowerShell 7.6.7；Git LFS 3.7.1；core.longpaths=true；固定HEAD/工作区干净；299个版本化Android Bundle文件字节符合原清单 |
| 不通过（4项） | UNITY_EDITOR_PATH未设置；当前PATH无法执行dotnet（不据此断言全机没安装SDK）；PATH python为3.13，要求3.12；该解释器pytest为9.1.1，要求8.4.1 |
| 未验证（2项） | Android模块和Unity许可：Editor变量前置不满足。未搜索安装目录或把许可文件存在当作许可有效 |

原始证据：`test-results/L4/308-01/summary.json`、对应scenario目录及`environment-proof.json`；`diagnostics/original-activity-01.json`为原caller记录的只读导出，含三轮原请求、Run、封存字节与报告。`remote-native-01.json`/`remote-native-02.json`仅是原终端只读补充，不替代result文件硬证据。

原Runner收尾回执`confirmed=true`，三轮原Run均stopped。`diagnostics/remote-cleanup-01/`保留原收尾回执；`diagnostics/remote-ref-cleanup-01/completed.json`记录原Runner确认停止后独立Gitea查询专属base无PR，并删除本次两个准确source/base ref。失败scratch保留。未启动第四轮、未恢复原输入，也未开启新的环境活动绕过本次预算。

本轮补齐未完成初始报告的诊断：不再让空message覆盖原Run连接错误，展示“验收脚本未完成”及原执行原因。该分支由新增owner回归验证；未声称它已经通过新的完整三轮真机复验。

| 回归 | 本scope证据 | 结果 |
| --- | --- | --- |
| 环境模块/应用所属 | `diagnostics/final-owner-01/` | 新增混合checks、三轮/重启保持not_ready及初始未完成报告保留原错误；37项通过，零跳过 |
| 执行器/配置/driver | `diagnostics/owners-01/` | 140项通过，4项既有显式debug/native opt-in未启用跳过；不计这些专项通过 |
| 封存报告验收 | `diagnostics/report-validation-01/` | 17项通过，含完整正向L2/PR与混合负向，缺轮、未封存、全绿冒充混合、实际PR、错机器、重复项拒绝 |
| 固定回归 | `test-results/L1/final-02/`、`test-results/L2/final-02/` | passed=true，L2 cleanup_complete=true |
| Web | `diagnostics/web-01/` | 构建、225条前端测试及HTTP/server owner通过；含同页分项通过/不通过但资格仍未就绪，不计真实浏览器完整进度通过 |
| 两项目脚本 | `diagnostics/local-checks-01/`、`diagnostics/demo-prepare-01/` | 308本机实际混合失败报告完整；演示项目独立clone中5项要求及原完整L1/L2通过，PR仍pending，未误置ready |

尚未完成：第三轮原生连接稳定后的完整308负向L4、真实后置拒绝准入及自动清理闭环；308全部要求满足后的完整L2+Gitea PR正向；旧进度六状态真机闭环。此次不把它们标为通过。


## 最低版本与 Orca 重试验收

用户要求Python/pytest接受更高版本并再试一次。项目原工具入口只记录实际版本，CI文档3.12/8.4.1不能直接变成精确相等的门禁。308已改为Python >=3.12、pytest >=8.4.1，并用原tools/testing/pytest_adapter.py收集现有tool-runners.json登记项：非零、分类完整、无收集错误才通过pytest检查。更高版本不是无限兼容承诺；完整执行仍由原L1/L2验收。Unity按项目明确约束仍固定2022.3.62f2。不安装、降级或修改远端工具/变量/PATH。

308材料提交`76919e3a0c31da73b48de282cf108e2d614b9244`，仍在原独立维护分支；本轮夹具SHA为`f3111e5fcd3057d08aa6f7f7af15348a6cf113bb`。沿用原共享profile、execution摘要`b858584bdd3f7188107c7baeb6ae5fd69eda1492f6efc18ce4721b433ecfa026`和原固定Runner，没有重新配对或改成另一台机器；native wait_timeout仍40s，单轮总期限仍300s，三轮预算未改。

实际命令：`scripts/test-project-environment-live.ps1 -ProjectConfig C:/SteerlineTestConfig/l4-308-project.json -ProjectCommit 76919e3a0c31da73b48de282cf108e2d614b9244 -SteerRunnerExe C:/SteerlineDebug/wt-4b14-environment-adaptation/diagnostics/merged-runner-02/steerrunner.exe -RemoteProfile C:/SteerlineTestConfig/l4-remote-profile.json -RoundTimeoutSeconds 300 -OutRoot C:/SteerlineDebug/wt-4b14-environment-requirements/test-results/L4/308-02`。

**本轮308负向L4已验收**：summary passed=true、cleanup_complete=true，正式TestRemoteProjectEnvironment耗时529.03s。活动`220492e1e355450fa70abe8300d4c581`三轮均实际完成检查、原Run停止及result closed/version=1，均未超时；报告逐项一致为7通过、2不通过、2未验证。活动耗尽后state=failed，执行资格仍not_ready；这不表示308已能执行任务或完整L2/PR已通过。

| 轮次 | 原Run | 结果 |
| --- | --- | --- |
| 1 | `570d9dcaa896422bb09a2eea809c1927` | 11项完整报告，completed=true；stopped、closed/v1，无超时 |
| 2 | `30a7f4146da24df09e5f13b628c1f4cb` | 11项完整报告，completed=true；stopped、closed/v1，无超时 |
| 3 | `95104681a675f05def34508ed04fe37e` | 11项完整报告，completed=true；stopped、closed/v1，无超时 |

当前通过项为PowerShell 7.6.7、Git LFS 3.7.1、Git长路径、固定干净源码、Python 3.13.1、pytest 9.1.1原适配器成功收集2个已分类测试、299个Android Bundle文件实际字节。未通过为UNITY_EDITOR_PATH未设置及当前PATH找不到dotnet；Android模块与Unity许可依赖Editor入口，保持未验证。原项目L1/L2和PR都未执行，不把pytest收集当测试执行或完整L2通过。

`admission-proof.json`证明before_rejected=true、after_rejected=true、after_run_id为空；前后公开任务请求均为environment_not_ready且零候选。`provider-pr-proof.json`独立查询专属base无PR。`remote-ref-cleanup.json`确认两个测试source/base及三个准确Runner head的清理处理完成；未推送head按不存在处理。原Runner cleanup confirmed=true，自动清理闭环通过；原失败报告未覆盖，不再需要手工代清理。

同scope下`diagnostics/versions-local-01/`实际验证Python 3.13.1与pytest 8.4.1的最低版本/原适配器收集分支通过。SteerLine固定`test-results/L1/version-01/`、`test-results/L2/version-01/`均passed=true，L2 cleanup_complete=true；新生成提示词也明确最低兼容边界，不再把CI示例版本精确锁死。没有重跑已通过且未改动的Web内容。

Orca前次故障定位：第二轮脚本已写完整结果，但原生turn/final未能核实，最终按300s期限停止；第三轮在Runner查询Orca原生main-agent就绪/状态流时超时，明确no input was sent，脚本未执行。它不是Unity/Python项目检查超时，也不能据此断定配对丢失。固定Runner源码的ready/state-subscribe/query路径、旧原记录及本轮只读environment/terminal查询已核对；更底层是网络、Orca服务还是状态流实现问题，现有证据不足以确定。本次同配置三轮均正常，故本次未复现，不声称已修复Orca根因或靠延长超时通过。

仍未验：308全部用户要求满足后的完整L2+Gitea真实PR正向、旧进度六状态连续真机闭环。当前负向流程验收不替代这些范围。
