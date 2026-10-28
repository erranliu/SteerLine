# SteerLine 调用方提示词收敛 Plan

后续固定说明已按[提示词管理方案](prompt-governance.plan.md)迁入 `internal/prompts/`；本文旧实现路径保留收敛阶段的调查/实施定位，当前模板维护见[维护说明](../../docs/prompt-governance.md)。

关联 [Spec](../spec/caller-prompt-convergence.spec.md)。设计随 PR404 合入后，用户已授权本次实现及规定回归；实际结果见独立验收，原 Task18 业务复验仍不在本次范围。现有Runner契约已足以完成本范围，不等待其workflow声明/输入预算修改。

## 实现整合

1. **固定源码及可比较样本。** 本文调查来自未合入PR399的48ad0146，实施前先核对目标分支与该分支的相关改动，整合既有进度/提示修复；不存在的消费者或用例不当成已经交付。再从现有公开输入及合成夹具准备有/无同源helper、长note多ID安排、环境混合成功/失败报告、Roadmap固定快照及不同版本原文。保留完整原数据及整改前组件字节数；测量脚本/日志只放本次独立diagnostics，不提交真实凭据或整份原生会话。不运行模型取样。
2. **收敛执行上下文。** 在 `internal/testmanagement/model/documents.go` 将完整安排展示改成确定性的选中ID/来源/digest视图；完整 `FrozenTestPlan`、原definitions和进度字段不变。复用 `internal/app/test_documents.go` 及现有固定Git内容读取入口核对Plan、测试规范、清单和同源writer，不读任意活动工作树或借用远端现场。writer只允许CRLF/LF等价；已核实时引用实际项目路径，否则沿原渠道完整内联。调整 `internal/runnerclient/prompt_context.go` 的调用方追加说明，保留当前公开/用户正文及步骤权限，不提前实施Runner新契约的职责改造。
3. **维持原候选核验。** 更新 `internal/storage/sqlite/runner_candidates.go` 当前依赖 `TestingContext(c.TestPlan)` 的校验及相关消费者，核对紧凑视图和源固定安排的一致性；不删除校验、不用模型正文反推新安排、不新增存储schema。必要材料读取失败须在原外部提交前拒绝，并保留原请求/操作身份和既有slot处理规则。
4. **精简内容生成。** 在 `internal/testmanagement/generation.go` 合并重复选择说明，材料和输出schema不变。Roadmap文档生成复用已经要求的固定仓库能力：`internal/app/roadmap_documents.go`、`internal/roadmap/documents.go` 只为快照内规范提供路径/commit/摘要和阅读要求，不复制相同正文；在原来源验证中核对规范manifest。原Markdown、确认规划、解析任务图及非快照材料保留完整inline内容。原固定规范验证不能因不再inline而消失，不向测试安排/环境生成另加仓库能力前置。
5. **精简环境指令与派生事实。** 在 `internal/environmentadaptation/prompts/materials.md` 合并重复原则、删除与外层生成schema完全重复的机械示例；该schema没有定义环境Report内部结构，不能删除后者必需字段说明。`internal/app/environment_adaptation.go` 对前轮事实做Spec规定的确定性投影；`internal/environmentadaptation/service.go` 按prepare/publish保留必要信息，复用现有固定文档与初始结果JSON。原报告和诊断保存机制不改，全部失败/incomplete/unknown及清理边界必须可追溯。无需通用摘要模型或新增报告状态。
6. **同步说明与交付。** 更新 `docs/integration/runtime-progress.md`、运行时进度Spec/Plan中被本次取代的全量内联要求，以及内容生成消费者说明。通用路径/测试清单只有实际语义受影响时才更新，不修改原业务清单。原用户回答路径只做完整性回归，没有已确认冗余则不重构。验收记录保留实际减量、原失败和仍必要的大输入；不把本次组件结果算作Task18连续两轮。

材料分发仍使用当前Runner已有入口：inline、固定代码快照、原任务checkout中的真实文件。禁止新建假Document引用、把本机临时路径当远端路径，或向旧运行手工注入材料。没有可用读取渠道的必要内容继续完整内联；只挪字段不计为减量。

## 最小验证计划

本次按下表实施与验证，实际执行状态、结果和证据在独立验收登记，不以计划表示已通过。原始结果使用 [路径清单](../paths.md) 的独立 `C:/SteerlineDebug/<scope-id>/`；各次正式调用使用新的输出叶目录，报告身份读取实际stdout，不覆盖旧结果。

| 验收 | 范围 / 环境 | 命令或最小补齐 | 时点与通过条件 |
| --- | --- | --- | --- |
| CPC-A01/A02/A06 | testmanagement/model、testmanagement、runnerclient、sqlite；isolated | 在现有 `TestInlineProgressWriterForProjectWithoutHelper`、`TestPMSWorktreePromptContext` 及原候选绑定用例补齐紧凑安排与有/无helper反例；`go run ./cmd/test-feedback -out <独立diagnostics目录> -- ./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1` | 上下文实现后；ID/顺序/definition/note完整保留，错误来源拒绝，无helper仍获完整源码，发布不获写入器，原CAS和冻结输入核验通过 |
| CPC-A03/A04/A06 | roadmap、environmentadaptation、app；isolated | 扩展已有固定仓库文档和环境材料用例；`go run ./cmd/test-feedback -out <另一独立diagnostics目录> -- ./internal/roadmap/... ./internal/environmentadaptation ./internal/app -run 'Test(Roadmap|ProjectEnvironment|CallerPromptConvergence)' -count=1` | 生成实现后；规范来源可读取且被核验，缺失/错版本阻断，原文/确认结果及输出schema不丢，失败投影完整且有真实减量；新增用例使用该命名范围，原测试不得仅因提示变短而删断言 |
| CPC-A05 | taskrunner及上述持久化用例；isolated | `go run ./cmd/test-feedback -out <另一独立diagnostics目录> -- ./internal/taskrunner -run 'TestRunnerNativeAnswersPreserveQuestionsAndCallerInput' -count=1`；在原生成/候选恢复用例覆盖模板改变后仍查询原请求 | 相关整合后；Unicode、多问题/选项/自由文字及来源原样保留，已冻结请求不重渲染、不重复submit |
| CPC-A01/A02/A05 | `testing.runtime-progress`，L3 controlled | `go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.runtime-progress -out <独立L3目录>` | 组件完成后；完整安排、逐字段进度、重跑、错误/恢复/重启、原请求和清理保持。若现有场景未覆盖紧凑输入到候选的接线，在原场景最小扩展，不新增live链 |
| 必要回归 | `l1.minimum` / `smoke.delivery`，L1/L2 isolated/controlled | `go run ./cmd/test-feedback -level=L1 -out <独立L1目录>`；随后 `go run ./cmd/test-feedback -level=L2 -out <独立L2目录>` | 最终实现及提交前；正式summary完整通过、非零、无意外跳过，L2 cleanup完整。按测试指南不默认扩大Full或无关owner |
| 文档 | 本次Spec/Plan及消费者说明 | `python scripts/check_doc_links.py`；`git diff --check` | 文档交付和最终实现前；所有本地链接有效，无空白错误；未执行的项保持待验收 |

不新增长期L4/live项，不启动真实模型、Orca原生探针、MagicPotion/Unity、业务PR或原进度两轮；这些不属于本整改的最小验证。当前frontend无计划改动，不跑Web全套；实现若实际改变公开UI契约，再补对应最小检查并记录原因。

## 自查与收尾

逐项检查：是否只是移动inline字段、是否误删必要材料、是否仍重复内联同一快照规范、是否把派生摘要当原报告、是否跳过完整安排核验、是否错误省略缺失writer，以及是否引入Runner新能力或预算前置。对必要大输入如实保留，不为数字目标削弱语义。

操作、重试和恢复沿[既有生命周期](../../docs/product-design/worker-lifecycle.md)，本次不新增变化。完成实现后将实际commit、调用方组成字节数、报告与清理结果写入[独立验收](../testing/acceptance/caller-prompt-convergence.md)；本次实现与回归不改变PR399既有复验结论或Runner阻塞状态。
