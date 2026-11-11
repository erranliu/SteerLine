# 测试指南

本文按 [SteerLine 测试架构规范](../SteerlineProjectGuideDoc/testing-architecture.md)组织，是本项目日常测试入口。各级说明见 [L1](testing/l1.md)、[L2](testing/l2.md)、[L3](testing/l3.md)、[L4](testing/l4.md)；系统读取 [test-list.json](testing/test-list.json)。

## 日常执行

开发中按需运行定向测试；一批代码改动完成、交付候选稳定后，运行一次下面两个固定测试集，并按“按行为选择测试”补充必要的 L3/L4。后续修复只补受影响验证，不因每次小修重新运行全部集合；仅提交、推送、创建 PR 或更新验收记录不要求在本地重跑。纯文档变更检查链接、相关清单一致性和 `git diff --check`；CI 与任务明确要求的检查仍按各自约定执行。L1 目标整体 1 分钟内，L2 目标整体 5 分钟内，实际总耗时包含准备、执行和清理；冷编译或机器争用超出目标时如实记录。

已有通过结果仅在其覆盖的行为、依赖、测试定义和环境未受后续变化影响时复用；在任务验证汇总中写明范围、原受测版本及报告位置，并补测新增差异。原报告保持原版本，失败或缺证据不作通过复用。已固定的任务要求和 `.tests.json` 仍按其版本执行，不能临时删减已选项或替入口补写进度；确需调整时先修订并固定安排。已完成任务的 Plan 与验收记录保留历史，不作为新任务的通用必跑要求。

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
```

| 级别 | 本项目内容 | 环境与选择 |
| --- | --- | --- |
| [L1](testing/l1.md) | 一个最小 Go 与前端单元/集成集合 | 隔离 Go/Vitest 测试，无模型；需 Go/Git、Node 24+ 及 npm ci；保留固定提示词正文/参数/源码守卫，复用一次真实仓库完整依赖矩阵检查，不累积各功能完整 owner 回归。 |
| [L2](testing/l2.md) | 一个受控核心交付冒烟 | 真实 SteerLine/Git/SQLite，公开 Runner/PR fixture；不需要真实 Runner、模型或 GitHub。 |
| [L3](testing/l3.md) | 可并行功能流程 | controlled，资源隔离，总并发最多二；按受影响 ID 选择。 |
| [L4](testing/l4.md) | 边界、不可并行功能、真实链和 Go 测试集 | controlled/live/isolated；live 必须单独精确选择，按需运行。 |

```powershell
# L3 定向功能；省略 scenario 时运行所选 suite 的该级全部场景
go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled -scenario recovery.waiting-merge-restart,runtime.service-lifecycle
# L4 系统边界；包含项目维护的 runtime 全集会串行运行
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario runtime.path-isolation
# L4 可复用 Go 全集（既有 Full），不默认执行
go run ./cmd/test-feedback -level=L4 -suite=go
# owner 定向诊断不代表登记测试集整体通过
go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/runnerbridge -count=1
```

Full 仅指全部普通 Go 测试，已登记为 L4 `go.full`，不包含 e2e/runneracceptance 标签或 Web；没有测试文件的包单独枚举为编译项。真实测试 skip 在原 summary/退出码/managed 中均不通过。正式复用集可用 `-level=L4 -suite=go -scenario go.http/go.storage/go.environment/go.deployment/go.roadmap`（每次选一个 ID），前端全集用 `-level=L4 -suite=web -scenario web.unit`。前端与 HTTP 检查用 `scripts/test-web.ps1`，其他浏览器及独立脚本入口见 [L4 列表](testing/l4.md)；未接入受管报告的入口不能凭 Agent 自述登记通过。

选中 go.http 时全量执行七个 HTTP/快照/配置/探测/元数据进程 owner，每包必须实际非零；选中 go.environment 时串行执行环境/model/remotetest/中性executionconfig四包全量，以及明确的Windows32根/其他平台31根的 App 环境集成组。源清单/正式定义及 package/root 终态须一致，缺失、额外或重复、跳过、一组失败均使整体不通过；其他 App 回归仍在 go.full。测试范围或定义变化时须验证新范围，不继承旧专项通过。go.deployment 包含host/Windows localservice全量、App11/10根和升级排空明确一根，go.roadmap 包含owner全量、App16根和host一根；选择依据见下节，选中后各组串行、原断言/25m/p1/parallel1不变，live排除与源漂移检查见L4列表。集合范围及原证据见[早期反馈验收](testing/acceptance/test-early-feedback.md)。

go.http 的 Windows 跨进程快照压力保持原30秒窗口、1000次发布和8读者；本机安排与E2E、Full、其他压力串行，独立通过不代表并行组合已验证。此约束也保留在机器清单，不改变L3自身并行实现。

## 按行为选择测试

下表用于编写或修订测试安排，在固定 L1/L2 基线上选择受影响项；不按页面或目录名称自动扩大范围。定向命令只说明其实际子集通过，不冒称完整登记测试集通过。

| 实际改动 | 默认补充验证 |
| --- | --- |
| 局部文案、样式、布局与展示映射 | 相关组件或纯函数测试；布局变化补代表视口、关键状态的浏览器检查，不自动要求 web.unit 或后端 owner 全集。 |
| 前端状态、查询、操作参数或共享组件 | 相关组件/交互及消费者测试；共享影响跨页面、覆盖难以可靠收窄时选择 web.unit。仅调用既有 API 不自动要求后端 owner 全集。 |
| HTTP、存储、环境、部署或 Roadmap 业务规则、契约 | 分别选择 go.http、go.storage、go.environment、go.deployment、go.roadmap，并按受影响消费者补功能场景；纯前端展示不因归属这些模块而触发对应后端专项。 |
| 跨组件核心交付、恢复或生命周期流程 | 在 L2 基础上选择具有独有覆盖的 L3/L4 场景；不重复建设依赖 owner 的内部故障测试。 |

Plan 写清实际变化、测试 ID 与选择原因，定向命令和专属浏览器步骤写入测试计划；`.tests.json` 只保存正式测试 ID，不将未登记的定向命令或专属浏览器步骤冒充正式 ID。所需行为尚无测试入口时安排补齐，不能以收窄范围为由省略必要覆盖。生产构建用于修改涉及构建、类型或打包的验证，浏览器检查用于组件测试无法验证的实际渲染与交互；均不因提交或更新记录单独触发重跑。

## 环境与证据

输出根和 L1–L4 的稳定目录结构统一见[流程路径清单](paths.md)。持续开发与并行调试使用仓库外独立 scope，并通过 `-out` 显式给每级本次执行指定新目录；无参数时的系统临时默认值及各包装器差异也在该清单记录。

从仓库根目录运行，Go 测试需要 Go/Git；L1 与 web.unit 另需 Node 24+，先 npm --prefix web ci。全部 E2E 当前仅支持 Windows。浏览器场景需要 Windows、Node/npm 和 Edge；真实链还需固定独立 Runner EXE、本机 Codex 登录及场景声明的外部权限，详见 [E2E 执行说明](../tests/e2e/README.md)。临时/AI/真实 PR 调试先遵循 [独立调试流程](../docs/runtime-install-debug.md)，不得使用或修改安装目录及保留现场。

正式分级执行要求 GOFLAGS 为空，拒绝继承 -run/-skip/-tags 静默缩窄范围；定向诊断参数显式写在 -- 后。PowerShell 中带点号的 scenario 使用 `-scenario ID`，避免 `-scenario=ID` 被拆参。每次使用新的 `-out <绝对目录>`，默认写系统临时目录。检查退出码、实际选择、源码版本、测试终态及清理证据；跳过、零匹配、超时或缺证据不能算通过。失败保留原日志，补测使用新目录，不自动重试。候选运行时通过 `STEERLINE_PROGRESS_FILE` 汇报测试进度：开始/重跑置 false，完整通过才置 true；失败、跳过、零匹配和清理未核实不完成。入口加锁合并并原子替换，原始证据仍保存在本次独立输出目录。

结果和未验边界写入任务专属 [验收记录](testing/acceptance/README.md)，测试列表只说明测什么。报告格式、Git 历史导出和读取见 [报告约定](testing/reports.md)。历史验收沿用其冻结版本与当时级别，不代表当前代码通过。

默认摘要记录受测版本（含脏状态）、实际命令、结果、原报告位置和未验范围；环境、实际选择及清理事实引用正式报告，不再手工复制一套。截图按代表场景保存；只有任务明确要求或产物传递、现场混用等具体风险需要时，才额外制作逐文件/产物摘要或再次独立审计全部报告与截图。正式入口原有的摘要、报告和清理校验照常执行，纯文档检查无需制作产物清单。

必需验证通过、确认的问题闭环、最终差异已覆盖后结束核验。追加检查须说明尚未覆盖的具体风险或新变化，不因“再确认一下”重复同范围检查；任务明确要求的独立审核、验收场景和真实失败仍按原约定处理。结果复用仅用于开发验证汇总，不改变候选测试进度、Git 历史导出或 PR 审核门禁。

当前 Runner v8 的验证与用户授权安装清理见[专属验收](testing/acceptance/runner-v8-install-reset.md)，此前 v7 升级、启动前取消及同构建重启回归保留在[原验收](testing/acceptance/runner-v7-adaptation.md)：升级须先排空原业务、原生资源及未完成操作；旧目录不迁移，活动任务不跨构建接管。历史三代 Runner 接管修复仅按其[原验收版本](testing/acceptance/install-deployment-generation-20261022.md)追溯；自动测试使用隔离现场，实际安装维护单独记录。

## 维护

修改测试内容时，同步实现、分级列表、JSON、[E2E 登记](../tests/e2e/suites.json)及执行器选择/证据校验。JSON 使用 `id/name/level/environment/command/check`，可选 `note`，不新增未支持的字段。ID 保持验证目的；改级后旧级别选择明确失败。

材料生成阶段由模型读取固定版本的 Plan、本文与正式清单，按“日常执行”和“按行为选择测试”选择测试并生成同名 `.tests.json`，随文档审阅入 Git。运行时只校验该 JSON 的有序 ID，并从同提交正式清单展开完整定义，不调用模型或重选。必跑规则保留在本文，各 ID 的完整定义在正式清单中自足。代码变更选择 L1、L2，再按实际行为影响补充 L3/L4；无需自动测试的 Plan 必须明确原因。实现 Plan 映射验收项和测试 ID，说明原因、时点、特殊准备和证据位置；已有完整命令/环境/断言引用清单，不重复抄写。

```powershell
python scripts/check_doc_links.py
go test ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1
git diff --check
```

实际执行参数按每组execution_parameters与原始command核对，区分Go测试、冻结二进制、test2json转换及Web；顶层并发只聚合同一适用维度，不同为mixed、未执行/未识别为unknown、非Go为not_applicable。控制器GOMAXPROCS、Go包并发、测试并发及外层worker分别记录，不能相互替代，详见[报告约定](testing/reports.md)。

远端用量新增精确L4 `usage.remote-codex`，命令、冻结输入、所需环境变量及单次真实模型/清理范围见正式JSON与L4列表；不纳入默认L1/L2，不得省略 `-env=live` 或改列isolated。
