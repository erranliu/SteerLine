# SteerLine 固定提示词管理与变更门禁 Plan

关联 [Spec](../spec/prompt-governance.spec.md)。2026-10-28，按审计收窄为固定文件、小型组装函数、已有测试和原生审核保护；用户后续已授权本次实现与规定验证。原生 CI 文件随仓库交付，远端保护设置和专属远端验收待对应范围授权；不启动 roadmap。实施基线 `a5c12eafcb2bf6e442de07ded2c6fd4affeb9541`，实际结果见专属验收。

## 实施步骤

### 1. 固定现有正文和接线

核对 Spec 调查表中的生产者与指令写入点，以现有消费者测试捕获同一合成输入的待提交请求。固定文字、动态事实、外部正文和正式资产分别确认来源；客户端展示文字和 schema 不顺带迁移。首轮以等价迁移为默认，必要编码/分隔变化逐项说明，不夹带新业务规则。

同步调查实际托管平台、已有 CI 和审核保护；外部设置未知如实记录，不为调查而创建 PR 或修改设置。

### 2. 增加一个小型 prompts 包

在 `internal/prompts/` 直接嵌入按用途的 Markdown，用少量 Go 函数绑定文件并复用私有渲染辅助函数。参数复用原事实投影和必要结构，缺失必需值、模板解析失败在提交前返回错误；数据不二次解释，外部正文不改写。

不建 catalog、用途/参数 schema、动态注册表、片段图、组件身份和通用来源类型；只有已经重复且需要同步的文字才提取一个私有片段。组装包不持业务状态、不调用 Git/数据库/Runner；不让叶模型依赖 prompts/app，不为格式化复制一套领域模型。writer 源码继续使用现有正式资产。

### 3. 原位接入，保持原业务与存储边界

| 所有者 | 最小改动 |
| --- | --- |
| testmanagement/model、testmanagement | 将固定指令迁出 `internal/testmanagement/model/documents.go` 和 `generation.go`，模型保留原安排事实投影/引用规则；生成流程使用 prompts，完整材料和 schema 不变。 |
| app、runnerclient | `internal/app/test_documents.go` 保留固定材料、writer 同源核验；`internal/runnerclient/prompt_context.go` 保留 Runner 步骤与原正文选择，调用集中组合函数，适配点不再拼固定规则。 |
| sqlite | `internal/storage/sqlite/runner_candidates.go` 的 `ReserveRunnerCandidate` 仅适配迁出的同一渲染调用，保留原安排/正文和 writer 模式核验、幂等冲突。普通读取及 `SaveCandidate` 的冻结比较保持原机制；不增加正文解析或新的恢复验证层。 |
| environmentadaptation、app | 迁移 `internal/environmentadaptation/prompts/materials.md` 及 `internal/app/environment_adaptation.go`、`internal/environmentadaptation/service.go` 的固定文字。prepare/publish 继续覆盖指定步骤；原事实投影、三轮/期限、初始结果、封存和准入不变。组装错误通过准备入口返回。 |
| roadmap | 迁出 `internal/roadmap/generation.go`、`documents.go` 的 Parse/Document 固定说明，保留原材料、固定规范来源、快照资格及 schema。 |
| 原回答 | `internal/taskrunner/input.go` 不改写；用既有用例确认原用户、多题及选项完整。 |

删除迁出的常量/旧文件和多余组装函数，不保留第二条正文生成路径或兼容包装。每个用途从原始正文组装一次，用实际请求测试防重复；不实现运行时重复文本识别。

### 4. 复用源码守卫与 owner 测试

在 `internal/testkit/boundary/` 扩展已有扫描：约束模板嵌入所有者、指令字段/workflow 覆盖的有限写入点，对适配点直接内联、直接追加/替换补简单语法反例；覆盖新增及非当前 build tag 源文件。不要维护事实生产者白名单或跨函数来源分析；静态检查无法识别的间接行为由接线代码审核与实际请求测试承担。

在现有 owner 测试中维护代表性最终正文快照，模板包补必要参数/渲染检查。测试输出同一输入变化前后的调用方正文和字节数，PR 展示差异和原因；不新增 Git 双版本运行工具或专用报告格式。小型模板/结构不变量接入 L1 最小选择，大材料和 owner 回归留定向验证。测试选择变化时同步分级说明、清单和自校验，不增加尚未实现的测试 ID。

### 5. 同步规则并接入已有审核保护

在项目规则/开发说明登记唯一模板来源、禁止额外固定规则和变更说明要求；在路径清单登记实际入口。固定文字增长解释新增依据及替代项，缩短/等长修改同样审核；更新快照不能取代业务断言或批准。

按实际托管能力复用必需 CI 状态与独立审批/代码所有者规则，同时保护组装、接线和 CI 配置。已有 CI 就复用；没有则先交付普通原生测试 job 的接线方案，不创建 `cmd/prompt-check`、可信执行服务、审批状态机或无网络沙箱。平台能力/权限不足时登记限制，不扩成新平台建设。

仓库实现与外部启用分开交付。外部修改需在对应操作范围获授权后执行；PG-E01 取证前不宣称远端硬门禁生效。本轮不更改远端或安装现场。

## 最小验证计划

原文档阶段只执行链接/空白检查；本次已进入获授权的实施，按下表执行并在专属验收登记实际结果。不得以拟新增包/用例或旧版本通过当作本次证据。

输出按[路径清单](../paths.md)使用 `C:/SteerlineDebug/<scope-id>/` 外部独立目录；每条命令一个新叶目录。owner 定向归 diagnostics，正式 L1–L3 使用对应级别路径，失败与补测分开。

| 验收 / 性质 | ID、环境与命令 | 时点与通过条件 |
| --- | --- | --- |
| PG-A01/A02/A03/A05；自动 | owner 定向 isolated；新增包后 `go run ./cmd/test-feedback -out <独立diagnostics> -- ./internal/prompts -count=1` | 模板完成；缺模板/必需值失败，动态数据与外部原文完整，代表性用途/分支和确定性非零通过。 |
| PG-A03/A07；自动 | owner 定向 isolated；`go run ./cmd/test-feedback -out <另一diagnostics> -- ./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1` | 测试链整合后；实际正文、完整安排、writer 两分支与只读阶段正确；缺/错绑定仍拒绝，原预留/幂等/CAS 不弱化，持久化原请求不可改。 |
| PG-A02/A03/A05/A07；自动 | owner 定向 isolated；`go run ./cmd/test-feedback -out <另一diagnostics> -- ./internal/roadmap/... ./internal/environmentadaptation ./internal/app -run 'Test(Roadmap\|ProjectEnvironment\|CallerPrompt\|PromptGovernance)' -count=1` | 生成/环境整合后；实际 envelope、固定来源/schema 与前轮事实保留，错误在 submit 前失败，原恢复只使用冻结请求。新接线用例用所列前缀。 |
| PG-A04/A06/A08；自动结构检查 | owner 定向 isolated；`go run ./cmd/test-feedback -out <另一diagnostics> -- ./internal/testkit/boundary/... -count=1` | 守卫完成；嵌入所有者、新写入点、直接内联/追加及 inactive 源码反例被拒绝；不据此声称语义或远端审批已验证。 |
| PG-A02/A07；完整性回归 | owner 定向 isolated；`go run ./cmd/test-feedback -out <另一diagnostics> -- ./internal/taskrunner -run '^TestRunnerNativeAnswersPreserveQuestionsAndCallerInput$' -count=1` | 整合后；原多题/选项/Unicode/自由文字及身份保持，实际非零匹配。 |
| PG-A03/A07；受控接线验收 | `testing.runtime-progress`，L3 controlled；`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled '-scenario=testing.runtime-progress' -out <独立L3目录>` | 整合后；复用原场景验证准备到请求、原输入、重跑/错误/重启及清理；不新建恢复链或重复 E2E。PowerShell 中完整引用带点的参数，避免 shell 拆成两个参数。 |
| PG-R01；固定必要回归 | `l1.minimum`，L1 isolated；`go run ./cmd/test-feedback -level=L1 -out <独立L1目录>`；`smoke.delivery`，L2 controlled；`go run ./cmd/test-feedback -level=L2 -out <独立L2目录>` | 最终实现；正式 summary 完整通过、非零、无意外跳过、清理完整。 |
| PG-A06/A08；正常代码审核 | 当前 PR 的独立审核；检查模板、接线、资产、测试与 CI 变化，原因、正文差异和字节前后值 | 合入前；检查变化也被审核，快照更新不自行批准，不实现审批模拟器。 |
| PG-E01；必需外部项 | 真实托管平台原生配置/拒绝对照；命令、平台、参与人、权限/窗口执行前固定，步骤见专属记录 | 远端启用后；缺检查/缺批准/过期批准不能合并，合法最新提交可合并；未满足不影响本地实现验证，但阻断硬门禁生效声明。 |
| 文档/清单一致性 | `python scripts/check_doc_links.py`、`git diff --check`；实施改变测试选择后另跑 `go test ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest\|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1` | 本轮仅前两项；实施后验证实际选择/登记一致。 |

不默认 Full、Web、L4 live、真实模型/业务 PR 或部署。没有恢复实现变化，不强制额外 `recovery.waiting-merge-restart` L3；若实施确实改变恢复路径，再按影响追加并说明原因，不能为满足原错误假设而重构存储。

## 交付

将实际提交、原命令、正文变化、检查/审核及远端状态分别记入[专属验收](../testing/acceptance/prompt-governance.md)。不继承历史通过，不把本地检查说成模型读取或远端保护已验证；不新增 roadmap、兼容链路或原运行输入更新。
