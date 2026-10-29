# SteerLine 调用方提示词收敛 Plan

关联[Spec](../spec/caller-prompt-convergence.spec.md)。本修订为用户确认的“短指令＋固定文档读取”，替代PR405允许缺helper和必要原文继续inline的方案。旧实现/回归只证明旧范围；本次已获用户授权，在 PR407 固定模板/组装/门禁基础上整合实现并执行规定回归；不开展原业务复验。

## 固定供给路径与实施边界

调查基线为main a5c12eafcb2bf6e442de07ded2c6fd4affeb9541。按当前Runner公开capabilities/schema逐项核实，不假设另一对话会提供新能力。

| 流程 | 必须文件化的材料 | 已知能力 / 待核实缺口 |
| --- | --- | --- |
| 实施/审计 | 完整安排、Plan/规范/清单、协议、writer | 既有固定 checkout；真实 `.tests.json` 展开后等于完整冻结定义，规则与 writer 同源才引用，缺失阻断 |
| 测试安排生成 | Plan、规范、清单 | 原 `generate.repository` 固定提交的私有普通文件快照；完整 commit、读取能力和必需文件缺一即阻断 |
| Roadmap 解析 | 原 Markdown、任务图、来源映射、规则 | 使用原固定快照；原文、图/映射 JSON 须由正常 Git 准备，逐字对照原权威，schema 不再内嵌原文枚举 |
| Roadmap 文档生成 | 固定规范、原文、确认规划、规则 | 同一快照文件与原始字节/确认数据相等，内部仍保存原材料用于原 manifest 校验；不新增发布或复制渠道 |
| 环境生成/适配 | 规则、报告字段、脚本、完整前轮报告 | generation/execution 规则和 context JSON 须正常 Git 准备；报告字段只维护于执行规则。复用已有结果文件初始JSON携带完整前轮报告和诊断，保持三轮上限，超64KiB明确阻断 |
| 回答/反馈 | 原回答，无文件转交 | 原回复通道；完整实际文本最多 2000 UTF-8 字节，多题合计、超限拒绝；冻结输入不重渲染 |

路径与原 JSON 格式已在 Spec 固定；实际准备结果及缺口登记独立验收。只实现当前正式能力确实支持的读取；仅为用户授权的308上限修复扩展公开generate.repository.paths，不新增私有RPC、旁路服务或临时材料仓库。需要正常Git准备时，不自动改业务仓库、创建PR或热换旧工作区。

## 整合步骤

复用 PR407 的 `internal/prompts` 和源码守卫；短模板与完整规则材料分开，移除原 writer 内联和前轮压缩投影消费者。补充各原服务的固定 Git 读取注入，使用 Spec 指定真实路径及原 JSON 格式，不增数据库字段或临时材料目录；公开Runner文件选择契约另以独立依赖PR交付。长规则采用测试内少量字节摘要快照，短模板使用完整正文黄金样本，两者均无自动更新入口，不新增生产 manifest/预算。每项缺材料反例必须明确断言零 submit；正向夹具通过正常 Git 提交准备文件后，验证实际快照请求及原结果来源。

1. **固定新契约。** 保留PR405原报告和历史现场，新增CPC-D01至CPC-D07验收，不复用旧CPC-A通过结论。短指令仅含动作、必读引用、少量参数、结果位置和完成要求；不建设统一预算流程，用户回答2000字节上限单独处理。
2. **准备校验先于构造/投递。** 复用internal/app/test_documents.go、既有固定Git读取、Runner配置/快照核验和各流程准备入口。核实实际执行侧可读及版本一致。非Git原始数据没有正式文件供给时明确准备不足，不创建模型输入；失败沿原请求/CAS/slot处理，不加状态表。
3. **删除执行提示中的材料展开。** 调整internal/testmanagement/model/documents.go、internal/runnerclient/prompt_context.go、候选准备及internal/storage/sqlite/runner_candidates.go，改引用固定安排/协议/writer。完整安排与来源校验保持；缺writer内联分支删除，错误/缺失进入准备阻断。不改变现有workflow公开/用户正文组合语义，不提前实施Runner新契约。
4. **生成流程按实际能力切换。** 调整internal/testmanagement/generation.go、internal/roadmap/generation.go、internal/roadmap/documents.go及internal/app/roadmap_documents.go，取消原文、清单及规划的inline展开，引用已核实文件。原始数据与代码快照不同则保留各自来源；仅有inline而无文件渠道的路径明确阻断，无内联兜底，不编造文件。
5. **环境规则与事实归材料。** internal/prompts/materials/environment-generation.md保留完整规范源；internal/app/environment_adaptation.go和internal/environmentadaptation/service.go仅生成阶段动作及真实引用。完整规则、原报告、脚本和初始JSON均须可读。前轮投影不再拼回指令，不删除原报告；只有全部旧消费者退役才删相关代码，不迁移历史冻结请求。报告schema/封存、完整L2、PR授权及清理不变。
6. **保持交互与原操作。** 在原回答准备/校验入口按UTF-8计算完整回答，多问题答案合计最多2000字节，投递前超限拒绝并返回实际值。保留原选项、正文和来源身份；不摘要、截断、拆条或转文件。旧输入不重渲染、不补文件、不重发。
7. **同步说明。** 后续实现时更新运行时进度Spec/Plan、docs/integration/runtime-progress.md及生成/环境说明中被取代的内联要求。本次文档不声称当前代码已切换；不改业务测试定义或通过标准。

## 最小验证计划

本次按下表实施验证；准备缺口未解决时分别登记已验/未支持范围，不以阻断用例通过冒充完整流程通过。报告按[路径清单](../paths.md)使用独立scope及新的输出叶目录。

| 验收 | 环境 / 命令与补齐 | 时点与通过条件 |
| --- | --- | --- |
| CPC-D01/D02/D03/D04 | isolated；go run ./cmd/test-feedback -out <独立diagnostics目录> -- ./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1；原缺helper用例改为投递前拒绝，补真实材料读取夹具 | 实现后；无正文展开，引用真实；缺文件/错版本/错误主机/无权限均不submit；原ID/定义/note/字段/CAS不变 |
| CPC-D02/D03/D05/D07 | isolated；go run ./cmd/test-feedback -out <另一独立diagnostics目录> -- ./internal/roadmap/... ./internal/environmentadaptation ./internal/app -run 'Test(Roadmap|ProjectEnvironment|CallerPromptConvergence)' -count=1 | 生成切换后；原文/确认结果/报告/规范完整且正确可读，无inline兜底；结果和授权/清理核验不变 |
| CPC-D06 | isolated；go run ./cmd/test-feedback -out <另一独立diagnostics目录> -- ./internal/taskrunner -run 'TestRunnerNativeAnswers' -count=1；补1999/2000/2001字节、中文/Unicode、多问题合计和原恢复反例 | 整合后；合法答案及身份不变，超限不投递、不转文件；冻结输入不重构或重复submit |
| CPC-D04/D06/D07 | L3 controlled；go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled '-scenario=testing.runtime-progress' -out <独立L3目录>；原场景补固定文件/短指令/准备失败 | 具备材料的最小受控链完成后；安排、进度/重跑/错误/恢复/重启及清理保持，不冒充真实远端Agent已读 |
| 必要回归 | go run ./cmd/test-feedback -level=L1 -out <独立L1目录>；go run ./cmd/test-feedback -level=L2 -out <独立L2目录> | 最终实现提交前；正式报告通过，非零、无意外skip，E2E清理完整，不扩大Full或无关live |
| 文档 | python scripts/check_doc_links.py；git diff --check | 本轮及实现交付前；引用有效，计划/实现/历史验收状态分开 |

驱动只扩展现有owner/受控场景，不新增生产运输接口来让夹具通过。不提交凭据/完整原生会话。模型实际阅读、真实发布及远端投递资格不由上述测试推定；本范围不安排真实模型、Orca探针、Unity、业务PR或原Task18两轮。

## 交付与等待

供给缺口、已支持流程的读取核验、未支持流程阻断和仍待真实验证范围分别交付。发现需要新材料发布方式或Runner能力时单列设计问题，不默认扩展授权。

实际源码、报告/清理及状态登记到[验收记录](../testing/acceptance/caller-prompt-convergence.md)。操作恢复沿[既有生命周期](../../docs/product-design/worker-lifecycle.md)，本修订仅改变新输入的准备和内容，不恢复旧运行。本次修改实现并执行规定测试，业务复验仍不在范围。

## 2026-10-29 剩余L4修复与复验

1. Go全集：固定go-winio上游Close竞态修复，保留栈/关闭看门狗；先100次目录场景压力复验。子进程fixture在TestMain/init分发，父测试实际断言保持；需独立调试包的四项显式E2E测试归e2e标签并同步脚本，不把条件skip当普通Full通过。
2. 308：Runner公开schema/capabilities、Git对象快照及边界测试增加显式路径选择，保留原提交来源和所有容量限制；调用方只选择完整必需材料并核验精确manifest，源码调查保留整仓。构建固定独立Runner，真实公开schema导出为契约夹具。
3. 远端材料：两个独立真实项目克隆从原固定提交准备同源规则/writer。进度L4在夹具提交前提供原三ID安排。复用初始结果文件提供完整前轮事实，补三轮/重启/完整性/超限/未知结果反例。
4. 修复后运行L1、L2、受影响L3及全部13项L4，live逐项精确选择，构建及维护场景串行。每次新输出目录，保留每个失败，不修改项目原测试标准。实际读取、原生封存、远端资源和清理分别核实，真实通过结果导出原生历史后更新验收与PR。
