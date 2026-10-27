# Plan：L4远端测试统一入口与64G配置

2026-10-26。依据[Spec](../spec/l4-remote-tests.spec.md)。按五项审计收敛，已授权实施及必要验证。普通开发，不改roadmap。[验收安排](../testing/acceptance/l4-remote-tests.md)。

## 1. 最小实施顺序

| 步骤 | 现有位置与最小修改 | 完成条件 |
| --- | --- | --- |
| 1. 共用profile输入 | `cmd/test-feedback`和现有E2E输入接线读取`STEERLINE_L4_REMOTE_PROFILE`，复用`executionconfig.Profile`校验；脚本仅将显式`-RemoteProfile`解析成同一输入。不建新配置库。仓库加脱敏示例/说明，实际文件置`C:/SteerlineTestConfig/` | LR-A01：只读一次、冻结后下游不重读；target明确64G，缺失/错target拒绝，Runner pin仍独立 |
| 2. 纳入两个既有入口 | 将原进度测试主体移入同名普通helper文件供远端包复用，保留本机测试薄入口及原浏览器目录用例；远端脚本成为对应L4薄入口，本机分支显式`-Local`选择。原six-state及目录断言不重写成另一套模拟 | LR-A03/A04：目录查询从同一profile取target/transport；进度fixture先完成正式ready前置并单列前置PR，任务主体不发布PR |
| 3. 308真实适配 | 在既有`tests/e2e`增加一个具体场景，使用真实Runner和SteerLine环境API；准备308受测提交、配套Git材料、专属验证base，冻结项目/远端身份和单轮期限 | LR-A05：原Agent/脚本跑完整L2+完整PR；停止后封存才ready，前后真实候选准入分别拒绝/允许，随后原候选及全部自有资源结算 |
| 4. 登记与必要证据 | 同步`suites.json`、`test-list.json`、L4文档、E2E README及现有`e2e.go`的live白名单、构建/Web前置、期限/结果校验；沿旧字段为三项各写具体校验分支，不建proof平台 | LR-A02/A06：三项均可单独精确执行，原本机/controlled选择不变，不把目录/进度场景强行当交付PR验证 |
| 5. 验证和维护 | 下表定向/固定回归后，使用同一稳定profile逐项运行三项真实L4，更新专属验收 | 缺环境/材料时记录准确失败或前置缺口，不skip冒充通过；无部署或安装现场改动 |

配置只新增一个共用读取/冻结点；复用原test-feedback/独立进程/构建/资源清理入口。准备profile时可只读核对当前`orca-remote-desktop64g`公开字段，确认长期工具路径后写独立测试配置，后续由该文件维护，不运行时自动同步安装版。冻结前后测试执行配置一致，不生成临时runtime/terminal白名单。

308输入由该场景显式取得：项目clone路径、完整Git SHA、既有格式的测试项目配置路径（含公开PR设置）、正的单轮秒数；冻结这些输入，将执行目录指向独立clone/home，不从Git提交猜本机配置，不增加项目配置注册中心。远端进度沿现有ProjectRepository/ProjectCommit参数取得SteerlineTest原程序。真实测试不修改原checkout、不把两个项目的ready互相替代。

审计落实：按Spec输入表把变量一次冻结到原E2E信封；共用execution不可被场景覆盖，workflow允许明确的场景覆写，目录API使用冻结profile_id/transport。308由原SHA构建测试源码分支与包含固定base名的夹具提交，测试配置指向该分支，公开API取到同一夹具SHA后才开始。PR规则来自Git材料/服务端只读事实，不新增产品PR配置字段；原L2报告和真实PR检查/审核/合并须与脚本报告关联核验。

两项目配套环境材料先经现有生成入口生成、审阅并由各项目正常Git/PR流程维护到所选受测提交中，不能注入ready或用mock报告准备。材料生成是上游维护，不作为每次L4强制模型步骤；测试驱动不自动修改业务主分支。缺材料在构建、推送和模型启动前写正式失败summary。各场景在新home内独立适配，不依赖上一次场景的数据库或成功记录。

本次补齐两项目薄材料：各在独立Git工作区新增`SteerlineFlow/environment.md`和`scripts/verify-environment.ps1`，文档引用原准备/测试/发布规则，脚本只调用原L1/L2、核验原明细和完成专属base的真实PR。维持普通任务发布授权不变，不把环境合并指向main/AIRefactor。固定提交后先验证SteerlineTest本机prepare实际L1/L2及308错误源码的失败报告，再串行复验两项64G L4。修复实际暴露的URL匹配时序、已配置PR凭据引用过滤、测试身份/原报告读取和收尾证据问题。环境门禁通过既有领域拒绝回执返回`environment_not_ready`及原因，避免普通错误被反复观察后显示过期；不增加状态、映射库、安装同步或认证系统。

## 2. 验证安排

以下ID/入口已登记；实际测试名、非零选择和证据须核对。先定向及固定L1/L2/Web，再执行三项真实L4；缺前置保留准确失败，不冒充通过。配置/选择/证据回归的实际入口为`./internal/testkit/remotetest`、`./cmd/test-feedback`及`./tests/e2e/internal/driver`，包含`TestRemoteL4RequiresLinkedSealedL2AndPR`的来源、第四轮、未封存、缺L2、L2失败、未合并及错机器反例。

| 验收 | 层级/命令及执行时点 | 通过条件 |
| --- | --- | --- |
| LR-A01/A02/A06 | 实施步骤1/4后，`go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/testkit/remotetest ./cmd/test-feedback -count=1`；含`TestRemoteL4*`及现有`TestTMRegisteredCatalogueMatchesExecutableManifest`、`TestScenarioContentListMatchesRegisteredIDsAndLevels`，driver另补owner回归 | 有效profile复用；缺失/本机target/相反配置拒绝；显式参数优先、冻结后不重读；每项独立选择且错级/缺证据/零匹配/skip失败；目录无需PR proof，适配必须有实际硬证据 |
| LR-A05 预算/准入 | 步骤3后，`go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/environmentadaptation ./internal/app -run '^TestProjectEnvironment' -count=1` | 原三轮/期限/重启、未知零重发、完整L2/PR、封存0/1、二态及共享目标准入回归通过；不在远端另建故障矩阵 |
| 全部固定回归 | 代码完成后，`go run ./cmd/test-feedback -level=L1 -out <新L1目录>`及`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 固定集合完整通过，L2清理证据完整；不是308项目L2通过 |
| Web接线回归 | 步骤2/4后，`scripts/test-web.ps1 -EvidenceDir <新diagnostics目录>` | 相关Web构建/前端/服务器通过；真实浏览器断言仍须在下面真机场景完成 |
| LR-A03 真实 | `go run ./cmd/test-feedback -level=L4 -suite=configuration -env=live -scenario=configuration.execution-profiles-orca -out <新L4目录>` | 当前64G、真实目录、原Web断言、来源及清理完整；不启动任务 |
| LR-A04 真实 | `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-progress-orca-browser -out <新L4目录>` | 正式适配前置通过；原SteerlineTest程序、六状态/同版本/连续Edge及停止/清理完整 |
| LR-A05 真实 | `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-orca -out <新L4目录>` | 308固定提交、Unity版本/Android和完整L2、同一真实Gitea PR/审核/检查/合并、停止后封存、二态准入及候选收尾全部核实 |

三项真机命令共同设置`STEERLINE_L4_REMOTE_PROFILE=C:/SteerlineTestConfig/l4-remote-profile.json`及显式`STEERRUNNER_EXE`。本机生成仍需已有本机模型登录；远端使用64G自身凭据。选择/建夹具前校验配置和场景专属输入；准备完成才可开始实际适配，缺前置不能执行半套后标成功。

进度和308的外围期限均由三轮预算加准备/停止预算计算，进度另加原观察窗口，贯通执行器、测试进程、场景context和包装器；不沿用旧20/24/30/35分钟硬上限。目录独立保留原有限查询预算，不受Unity或材料生成前置约束。记录实际耗时，不删断言或延长已提交轮次截止。

## 3. 输出与交付

遵循[路径清单](../paths.md)，使用新外部scope，例如`C:/SteerlineDebug/wt-4b14-l4-remote/`；登记集合放`test-results/L4/<唯一ID>/`，诊断/固定L1/L2分别落对应目录，每次新叶目录。复用summary、managed-ID、scenario.json、构建身份及原日志，补各场景必须的公开证据；不把并行固定构建位置当独立资源。

具体真机步骤、资源/PR清理与实际结果只写[专属验收](../testing/acceptance/l4-remote-tests.md)。原Run/PR未知则按原身份核实，清理未完成保留现场及准确对象，不覆盖失败或开启替代执行。文档/实现/受控测试/真实执行分别记账，不把登记或前置检查算业务通过。

本次验收补充308真实未就绪分支，沿原正式L4选择，不加新ID/框架。报告增加checks，三轮独立Run逐一核验封存及混合结果，独立查provider无PR；公开API再次请求任务仍须environment_not_ready且零候选。就绪分支及六状态进度原要求不变。用户准备环境，Agent只验收，固定脚本无路径发现或安装修复。
