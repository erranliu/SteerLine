# Plan：项目远端环境适配

2026-10-26。用户已授权按五项审计收敛实施，并按Runner result最终快照四点适配修订；依据[Spec](../spec/project-environment-adaptation.spec.md)，Runner独立双通道作为接入前提，实际通过与未验项另记。普通开发，不修改roadmap、不部署安装环境。[验收](../testing/acceptance/project-environment-adaptation.md) · [测试安排](project-environment-adaptation.tests.json)。

## 1. 最小整合与前置

| 位置 | 实施时的最小整合 |
| --- | --- |
| `internal/runnercontract`、`internal/runnerclient/progress.go` / catalog / client | 按已实现的公开 result-file 契约补 caller 字段、校验及查询封装；与 progress 独立保存，不增加远端命令或环境专用 Runner 接口 |
| 环境模块自身的生成用例、Runner客户端及home生成配额 | 独立生成输入/输出，不扩展Plan专用GenerationService，不新增通用文档生成框架 |
| 薄业务模块（建议 `internal/environmentadaptation`）及项目 SQLite ports | 保存适用依据、活动/轮次、期限、原Run/请求、两通道快照和业务报告；复用项目actor/观察与存储，不建执行引擎 |
| 原应用宿主、`internal/taskrunner/service.go` 及候选预留事务 | 仅按公开adapter/target键做窄准入占用，从原项目记录重建；不发现主机，不复用服务启停锁；新候选最终复核ready |
| `internal/app/environment_routes.go`、`internal/product/httpcontract`、`web/src/features/environment-checks` 及项目设置入口 | 薄HTTP投影，环境页分开显示进度/验收报告与二态；复用认证、journal、Query/轮询，保留旧checks历史/收尾 |

Runner 双通道是本次指定的已实现前提，本 Plan 不再分为“现在开发/等待 Runner 研发”两套。上游提示词、项目报告schema、状态/预算/有限失效规则和页面可直接实施；接线使用所选实际构建的公开契约/fixture，真实联验时固定EXE与远端身份。核对构建资格不等于新增Runner需求；不满足时明确拒绝，不旁路本机代跑或读取私有文件。取消旧版远端无模型命令及可信进程退出证明前置；不扩展artifacts、通用workflow、context、任意文件或全Agent矩阵。

## 2. 实施顺序

1. **双通道消费与业务格式。** 保留result_file输入和result-file查询，按公开final-only schema/capability核对。复用字节校验，result仅接受disabled/pending/closed及0/1版本，closed/0保留失败diagnostic，封存后不刷新。删除连续版本/观察状态逻辑，不增加采集或恢复框架。项目报告schema仍由SteerLine固定：本轮/源码/脚本关联、completed、完整L2结果、同一PR/检查审核/合并/脚本清理。进度不决定ready，最终报告缺失/超限不降级。完成门槛PEA-A03/A05的正反fixture；不把文件完整性当进程证明。
2. **环境模块生成维护。** 独立构造generate输入和两文本/缺口输出，复用客户端与配额，不向Plan.tests用例加kind分支。机器段登记脚本/解释器/有限依赖及预先存在的固定验证base；脚本prepare验证L2，publish唯一执行PR及最终报告。预览/导出，经正常Git流程，不建apply/自动提交服务。完成门槛PEA-A01。
3. **状态、预算、期限和准入。** 项目库保存三轮活动、每轮独立Run/初始文件/原请求及不可变截止；开始请求只增加一个正的单轮总超时。原应用宿主串行核对/登记目标占用，重启从项目活动恢复后才开放新作业，不建跨库事务。新候选最终预留复核二态/有限输入，旧读取/停止/交付保留。schema按未发布规则拒绝旧格式，不回填旧checks。完成门槛PEA-A02/A04/A06。
4. **既有Run及具体生命周期接线。** development/2的implement只执行脚本prepare，publish唯一执行脚本publish；固定base在submit前验证存在和源码祖先关系，不自动建删base。提示词要求脚本返回前原子写完报告；依据pr_ready或完成/失败、取消/期限发起原stop，再核实停止及唯一封存报告，禁止以运行中result作为停止前置。采集失败不重采同一Run；原Run/PR资源已核实才可消耗剩余预算新建轮次。既有逐类型runner_build_binding、runner_takeover、project_runtime、service-start前stop、application_verify_stop及state_preflight/reset接线保留，不造registry。未知只对账。完成门槛PEA-A03–A05。
5. **页面及有限失效。** 复用项目页面显示二态、轮次、期限、两个通道来源、验收报告/Agent分析、Run收尾及历史。材料维护只比较已登记文件/配置摘要，不扩展源码语义分析；普通提交不机械触发重新适配。完成门槛PEA-A01/A02/A06。
6. **最小集成和真实验证。** 原独立home、Git/SQLite/HTTP与双通道/PR fixture覆盖生成、失败重试、收尾、互斥和准入。必要owner回归及固定L1/L2、Web后，以固定真实Runner/远端完成Agent执行→双文件回传→项目L2+完整PR→收尾→新任务准入；核验自动publish未额外创建PR。完成门槛PEA-A07，受控与真实分别登记。

## 3. 测试安排

下列测试前缀已在所属模块落地，实际执行数及结果见专属验收，零匹配不能充作通过。机器安排只引用当前已登记的固定 `l1.minimum`、`smoke.delivery`。适配标准不要求全 L3；本功能必要的故障/owner 测试不属于对受管项目追加环境门槛。

| 验收项 | 计划入口 | 环境、时点及通过条件 |
| --- | --- | --- |
| PEA-A01 | 所属generation/app单测 `TestProjectEnvironmentGeneration*` | 步骤2后；真实Git材料/有限路径/提示词、双通道写入要求、缺口、无L3强制/自动通过/Agent伪报告要求 |
| PEA-A02/A04/A06 | SQLite/taskrunner/app `TestProjectEnvironmentAdmission*`、`TestProjectEnvironmentRetry*` | 步骤3–5后；跨项目同目标已有任务/适配的双向竞态、事务回滚、占用恢复，三轮/截止不重置、有限失效与普通提交区分 |
| PEA-A03/A05 | runnerclient/业务owner `TestProjectEnvironmentExecution*` | 步骤1/4后；result仅0/1及三种状态，空文件/封存失败诊断/摘要/缺失/超限/互串/旧轮/半文件；运行中结果不可见而pr_ready仍触发stop，waiting_input不提前停止，先stop后唯一封存报告；封存不刷新，查询故障/重启、未知/取消/超时零重发，失败仅在资源安全且预算内新建Run |
| PEA-A07 受控 | app/HTTP组合 `TestProjectEnvironmentWorkflow*` | 步骤6；实际Git/SQLite/项目API+同版双通道/PR fixture，合法Run材料覆写、失败→独立新轮→完整L2/PR→pr_ready触发原stop→核实停止/唯一封存结果→ready→新候选；耗尽阻断，无重复发布 |
| PEA-A01/A02/A06/A07 Web | `web/src/features/environment-checks/project-environment.test.tsx`、现有原请求及项目切换测试 | 步骤5–6；二态、轮次/期限、进度与报告区分、Agent排障回复、只读历史、停止和原请求核实；真实浏览器另验 |
| 全部回归 | `go run ./cmd/test-feedback -level=L1 -out <out>`；`go run ./cmd/test-feedback -level=L2 -out <out>`；相关 owner 与 Web tests/build | 实现完成后；受控固定集和必要定向项，不默认 Full/全L3/全L4 |
| PEA-A07 真实 | 环境页开始→既有远端Run/Agent执行脚本→两个文件回传→完整项目L2+验证PR合并→原Run停止/清理/最终快照→任务准入 | 受控通过后；固定具备双通道的EXE、正式schema、实际CLI/API/浏览器、源码/脚本/远端身份及同一PR URL，核验无第二次发布；当前待安排 |

Go 定向项使用 `go run ./cmd/test-feedback -out <out> -- <实际所属包> -run '<上述实际前缀>' -count=1`。包与实际测试数量在实施时核实，不因设计建议目录尚未创建就运行空测试。Web 沿现有 npm 接线，生成构建位置按[路径清单](../paths.md)串行。

报告写到仓库外独立根 `C:/SteerlineDebug/wt-4b14-environment-adaptation/`，按 testing/paths 约定的级别或 diagnostics 每次创建唯一输出叶目录；原失败和补测分别保留。真实验证生成独立包，不能使用安装 home、真实业务工作区或业务主分支承载故障注入。受控通过不代替真实远端及真实 PR，Runner 原生专项不在本仓复制全矩阵。

## 4. 文档阶段与交付

本轮先修订文档再实施SteerLine，逐项运行所需owner、固定L1/L2及Web验证；静态文档检查不代替产品通过。真实远端/PR在独立场景具备前置后联验，缺前置记录准确未验原因，不修改Runner或安装现场。

交付包含SteerLine代码、提示词模板、项目报告/脚本约定、双通道消费、具体生命周期接线和验证记录。不修改Runner、不新增通用框架，不将项目L2 fixture通过冒充真实PR完成；具体实现和联验事实只登记专属验收。

实现使用 `internal/environmentadaptation`、app的环境端口/准入及具体生命周期接线、项目SQLite记录和既有命令journal。固定验证base先核对，单脚本分prepare/publish，home目标准入锁独立于服务锁/生成配额，生成用例不进入Plan专用GenerationService。这五项审计收敛不再引出通用registry、文件平台或工作流引擎。
