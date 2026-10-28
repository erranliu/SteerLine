# 固定提示词管理与变更门禁验收及文档审计

2026-10-28，用户已后续授权实施与规定验证。关联 [Spec](../../spec/prompt-governance.spec.md) / [Plan](../../plans/prompt-governance.plan.md)。仓库内实现及原生 CI 配置已完成，验证结果如下；未更改远端保护或部署，不进入 roadmap 执行队列。

实施基线 `a5c12eafcb2bf6e442de07ded2c6fd4affeb9541`，受测为该 HEAD 加本次未提交工作树，不冒充干净提交或导出 Git 测试历史。Windows/amd64、Go 1.26.4；原始证据根 `C:/SteerlineDebug/wt-2352-prompt-governance/`，按[路径清单](../../paths.md)逐次使用独立叶目录。PR #405 历史通过不算本机制已验。

## 后续文档驱动整合

本文件下列正文与测量属于首次等价迁移历史。PR407 已按 PR406 的文档驱动修订调整为 **9 个短模板、5 份完整规则资产**；新请求不再内联 writer、原规划或前轮摘要，规则资产纳入 L1 的显式字节快照检查。当前行为、准备缺口、字节变化和本轮测试证据统一见 [调用方收敛验收](caller-prompt-convergence.md)，不得沿用下表旧 writer 两分支或原材料 inline 的通过结论。远端保护配置及独立审核仍未验收。

## 2026-10-28 文档审计

本次为当前聊天直接审计，未启动子代理。结论：原稿有一处错误调查前提和四类过度设计，已修订 Spec、Plan 和本记录，尚不构成实现通过。

| 级别 / 发现 | 原设计的问题 | 处置 |
| --- | --- | --- |
| P1：误把预留核验当恢复重算 | `ReserveRunnerCandidate` 的正文检查被解释为普通恢复依赖，进而计划正文事实解析和存储校验重构。实际 `SaveCandidate` 已比较原请求/安排不可变性，读取不渲染。 | 只适配预留阶段迁出的渲染函数，保留原冻结和幂等检查；删除新增解析协议/恢复层，取消无恢复改动时额外恢复 L3 必跑要求。 |
| P2：重复元数据与模板框架 | catalog 的参数/阶段与 Go 类型、调用选择重复，片段图及组件 ID 又增加同步约束。 | Go 小函数直接绑定嵌入文件；取消 catalog、参数 schema、动态注册、片段图和来源凭证类型。 |
| P2：静态检查目标超出可维护边界 | 要求识别任意辅助函数/事实字符串夹带规则，实际会推动全程序来源追踪或庞大放行名单。 | 只自动检查嵌入所有者、有限写入点和直接追加语法；间接语义交实际请求测试与接线审核，不宣传全防护。 |
| P2：运行时重复检测不能可靠区分原文 | 要求拒绝把已组装正文再次传入，但外部用户原文可能合法包含相同文字，需要额外来源类型或文本猜测。 | 单一调用路径加实际正文次数断言，不扫描/删改用户文字，不建跨请求状态。 |
| P2：为日常漂移治理新建 CI 安全平台 | 独立命令、跨 Git 版本执行、可信规则编排、无网络沙箱及审批模拟器扩大到未知托管基础设施。 | 复用现有 Go 测试、PR 差异及托管平台原生保护；不新增执行/审批系统，保留真实远端保护的独立未验边界。 |

结构边界：一个模板/组装包、现有 owner 与边界测试、原生托管规则；不新增产品协议、数据库字段或执行状态机。外部门禁需要实际平台能力及配置，未核实前不承诺硬性阻断。

## 验收安排

精确命令和时点见 Plan 最小验证表；本表维护状态及阻塞范围。零匹配、跳过、超时、缺证据不是通过。

| 项目 | 方法 / 最小通过条件 | 状态与阻塞范围 |
| --- | --- | --- |
| PG-A01 | isolated；嵌入文件与实际用途直接绑定，无旧副本/回退路径，模板错误提交前失败 | 已验收；12 份模板、12 组正文样本及错误反例通过 |
| PG-A02 | isolated；必要参数检查，Unicode/模板语法/指令样数据不被二次执行或改写，无通用规则追加口 | 已验收；参数/CRLF/原文及原回答回归通过 |
| PG-A03 | owner + 原 runtime-progress controlled；原正文和阶段语义、writer 两分支、单次追加及可达材料正确 | 已验收；实际请求、阶段隔离、L3 和清理通过 |
| PG-A04 | isolated；新写入点、错误嵌入所有者、直接内联/追加及非当前 build tag 反例被拒绝 | 已验收（本次有限约束）；L1 和 owner 新守卫通过，不表示全部历史结构约束通过 |
| PG-A05 | owner 合成输入；实际正文快照、必要业务断言和字节前后值完整，变更说明可审阅 | 已验收；10 组逐字一致，2 组仅固定模板换行归一，详见下表 |
| PG-A06/A08 | 本地测试 + 正常独立代码审核；模板/接线/资产及测试、CI 改动都在审核范围，不能以更新快照自批 | 本地检测/配置验证通过；合入前的独立审核待安排，不能用本聊天自查替代批准 |
| PG-A07 | 原预留/幂等/冻结更新及原回答测试；绑定错误拒绝，原请求与安排不可变，恢复不重渲染 | 已验收；原预留/CAS、持久化恢复及原回答回归通过，未增加解析/恢复层 |
| PG-R01 | Plan 指定 owner、runtime-progress L3、固定 L1/L2 和实施后登记自校验完整通过，资源清理完成 | 验收失败（完整边界子项存在基线失败）；其余规定回归通过，不能将整体记为全绿 |

## 本次实现与证据

固定文字从 Go 常量和环境旧模板迁入 `internal/prompts/`，12 个文件覆盖实际用途及必要共享片段；没有 catalog、动态注册、组件图或新数据库/协议。模型只提供 `TestingFacts`，runnerclient 仍选择步骤；SQLite 只改预留阶段的同一渲染调用。环境 `appendRound` 将渲染错误传回保存/提交之前，原报告、期限及结果封存规则保持。现有消费者测试与 E2E 断言同步接入，未删业务检查来换取快照通过。

原始证据均在上述独立根目录；下表路径相对该根。各 summary 的 `command`/分组原参数和 JSONL 保存实际命令、版本与选择。本次测试源为未提交工作树，未伪造受测干净 SHA 或 Git 历史记录。

| 范围 / 实际调用 | 原始报告入口 | 结果 |
| --- | --- | --- |
| `go run ./cmd/test-feedback -out <该目录> -- ./internal/prompts ./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1` | `diagnostics/20261028T092926263Z-prompt-owners/summary.json` | complete/passed；235 测试通过、3 个既有真实 Runner opt-in 用例按环境条件跳过，5 个包完成。被跳过用例不计产品原生通过。 |
| `go run ./cmd/test-feedback -out <该目录> -- ./internal/roadmap/... ./internal/environmentadaptation ./internal/app -run 'Test(Roadmap\|ProjectEnvironment\|CallerPrompt\|PromptGovernance)' -count=1` | `diagnostics/20261028T092923044Z-generation/summary.json` | complete/passed；72 测试通过、0 测试跳过。未改的 roadmap/model 包无该前缀匹配，不计全 model 包验收。 |
| `go run ./cmd/test-feedback -out <该目录> -- ./internal/taskrunner -run '^TestRunnerNativeAnswersPreserveQuestionsAndCallerInput$' -count=1` | `diagnostics/20261028T092926777Z-answers/summary.json` | 原回答用例非零通过，complete/passed。 |
| `go run ./cmd/test-feedback -out <该目录> -- ./cmd/test-feedback -count=1` | `diagnostics/20261028T093319958Z-feedback/summary.json` | 123 测试通过、0 跳过；覆盖实际分组选择、自校验及报告完整性。 |
| `go run ./cmd/test-feedback -level=L1 -out <该目录>` | `test-results/L1/20261028T093045033Z/summary.json`、`managed-l1.minimum.json` | complete/passed；84 项通过、0 跳过，6 个分组；准备约 0.5s，执行约 39.7s。 |
| `go run ./cmd/test-feedback -level=L2 -out <该目录>` | `test-results/L2/20261028T093220298Z/summary.json`、`managed-smoke.delivery.json` | 仅选择 smoke.delivery，非零完整通过；准备约 27.1s、执行约 117.5s；cleanup_complete=true、无错误。 |
| `go run ./cmd/test-feedback '-level=L3' '-suite=testing' '-env=controlled' '-scenario=testing.runtime-progress' -out <该目录>` | `test-results/L3/20261028T093614147Z/summary.json`、`managed-testing.runtime-progress.json` | 仅选择原 runtime-progress，非零完整通过；准备约 20.4s、执行约 107.8s；cleanup_complete=true、无错误。 |

3 个未执行的真实 Runner opt-in 用例为 `TestEnvironmentCheckPublishedContract`、`TestProjectEnvironmentExecutionPublishedResultRoundTrip`、`TestRunnerFullBindingPublicCLI`，不在本次无真实 Runner/模型范围内。未启动 Web 全套、Full、L4 live 或原业务两轮。

### 正文等价与减量

实施前使用旧调用方代码冻结相同输入的完整正文，保存黄金样本；固定 writer 源码在期望值中只引用正式资产，比较前扩展为完整字节，不再复制一份资产。变化只有旧环境固定模板的 23 个 CRLF 统一为 LF，动态事实、用户 CRLF 和 writer 字节没有归一化。

| 代表性正文 | 实施前 / 后 UTF-8 字节 | 说明 |
| --- | --- | --- |
| 测试选择、执行含 writer、引用 writer、只读、原文组合 | 415→415、5927→5927、2183→2183、303→303、2211→2211 | 逐字一致，用户模板样文字和 CRLF 原样保留。 |
| 环境生成 / 带前轮事实 | 5785→5762、5904→5881 | 各减少 23 字节，仅固定模板 CRLF→LF，不删规则或事实。 |
| 环境 prepare / 重试 / publish | 1022→1022、1071→1071、788→788 | 逐字一致，诊断只入 prepare、共享结果约束各一次。 |
| Roadmap 解析 / 文档生成 | 765→765、1478→1478 | 逐字一致，固定来源与完整材料渠道保留。 |

这些数字只表示同一合成样本的调用方正文，不是全部模型输入或长度预算。

### 失败与基线隔离

- 首轮 `diagnostics/20261028T091650255Z-template/summary.json` 在编译时发现迁移处 `Record` 应为 `model.Record`，已修正；不算通过证据。
- `diagnostics/20261028T091943683Z-templates/summary.json` 揭示环境固定模板换行差异；核实后仅按设计统一固定模板 LF，并显式更新两份期望值。正式资产和外部文字不做同类改写。
- `diagnostics/20261028T092334366Z-owners/summary.json` 的完整边界检查失败后，使用 `git archive HEAD` 导出原始基线到 `baseline/a5c12eaf-20261028T092659552Z/`，运行相同 `go run ./cmd/test-feedback -out <新目录> -- ./internal/testkit/boundary/... -count=1`；原基线结果在 `diagnostics/20261028T092701873Z-baseline-boundary/summary.json`。
- 最终完整边界结果为 `diagnostics/20261028T093235232Z-boundary-final/summary.json`：与原基线相同的 8 个失败测试名（含父测试/子项），本次 `TestPromptGovernanceSourceBoundary` 及依赖负例通过。剩余基线问题是 sqlite/environments 对 environmentadaptation 的依赖、remotetest 的工具归属和 testenv 对 app 的依赖；没有放宽这些规则。原模型对 writer 资产的依赖已随本次拆分移走。
- 一次 L3 启动被 PowerShell 将未引用的 `-scenario=testing.runtime-progress` 拆成两个参数，在准入解析前失败，未执行测试也未产生通过报告；已用完整引用参数的新目录正式执行，结果见上表。

完整 owner 边界仍为失败，PG-R01 不记整体已验收；这是已复现的先存问题，不宣称本次消除了全部结构债，也不为完成提示词管理而扩大到无关存储/远端测试重构。

## 必需外部项 PG-E01

使用真实托管平台原生必需状态与审批/代码所有者能力，不自建可信执行或审批服务。平台、专属测试仓库/分支、现有 CI、审核资格、参与人/窗口、操作权限均待固定；对应外部操作范围授权后再执行。本项不是可选体验验收，未满足阻断硬门禁生效声明，但不阻塞本地代码实施及独立验证。

1. 核验原生必需测试状态和独立审核规则覆盖模板、组装/接线与检查配置，普通角色不能直接推送或绕过；平台无按路径审批则采用统一独立审批，不另建路径审批程序。
2. 专属测试 PR 缺测试状态或独立批准时实际合并被拒绝；仅改快照、标签或检查配置也不能免除审核。
3. 追加提交后，核实旧批准及旧 head 检查不能放行；目标分支变化按平台支持重新检查或经合并队列验证。
4. 最新提交完成合法测试及独立审核后，实际核实可合并，并清理专属资源。记录检查、批准、拒绝及最终提交回执，不保存凭据。

当前：待验收，未配置/未执行。已通过只读 GitHub CLI 查询核实 Actions 工作流为 0、main 保护接口为 `404 Branch not protected`；连接器读取保护曾因 integration 权限返回 403，故没有把它当成“未保护”的证明。本次提供 `.github/workflows/prompt-governance.yml`，YAML 语法及本地所用完整 L1 已验证，尚未提交到远端执行；独立审核者资格和平台保护效果仍待固定。能力不足不新增替代系统，管理员主动关闭规则不在保证范围。

## 文档检查

初稿：`python scripts/check_doc_links.py` 扫描 600 份 Markdown、3807 个本地链接，0 错误；`git diff --check` 通过。此为修订前检查事实，不代表审计后版本已检查，也不作为任何实现/远端项的通过证据。

此前文档审计阶段（实施前）：`python scripts/check_doc_links.py` 扫描 600 份 Markdown、3808 个本地链接，0 错误；`git diff --check` 通过，新增三份文档另检查行尾空白。当时仅文档，未运行模型、业务测试、PR 或部署，未将实现或远端项记为通过。

实施完成后：链接检查扫描 601 份 Markdown、3818 个本地链接，0 错误；`git diff --check` 及全部改动/新文件的行尾空白检查通过。12 份固定模板无 CRLF；黄金样本保留预期用户 CRLF。Actions YAML 经解析，包含一个 job 及 pull_request/push/merge_group/workflow_dispatch 触发器；远端实际执行及分支保护仍未验，不能以 YAML 解析或本地 L1 替代。
