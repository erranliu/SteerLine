# Spec：Worker 分步执行与结构化状态推进

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../architecture/current-code-architecture.md)。

> 2026-10-12：本文保留为原实现的历史契约。当前有效业务规则和实施/测试安排已由 [统一 PR 流程 Spec](worker-pr-flow.spec.md) 与 [Plan](worker-pr-flow.plan.md) 替代；下文的候选 SHA/阶段快照比较不得继续用于当前执行，历史验收结果不改写。

任务 ID：84；任务名称：Worker 分步执行与结构化状态推进；目标：`WORKER-STAGED-FLOW`。配套 [Plan](worker-staged-flow.plan.md)，设计范围见 [roadmap 草稿](../roadmap/drafts/roadmap.md)。已由用户确认纳入 [execution-map](../roadmap/execution-map.md)，尚未启动实现；启动时固定本 Spec/Plan 的实际 Git commit/路径，一个独立实现 PR 交付。

本文由已确认的流程草稿转为本任务唯一的流程/提示词正文，采用现有 worker、简单步骤序列和 adapter；仅 implement 分执行/报告轮，publish 单轮。具体验收安排及结果归[专属验收规划](../acceptance/worker-staged-flow.md)。无未完成硬前置；#83 已通过 PR #247 进入主干，隔离能力作为基线复用，不重新派发。复用已有持续会话、工作区、投递/恢复及唯一交付能力。本文及编号不改变当前 worker-flow-v1 或正在运行的任务。

## 1. 职责与规模

Spec/Plan 定义任务做什么、实现范围、测试和验收要求。SteerLine 管理阶段及推进，worker 按当前阶段短提示词执行具体动作；继续由 worker commit/push/创建 PR。第一版只有两个业务模型阶段：implement 分为 execute（执行）和 report（结果整理），publish 用一轮完成发布和结构化收尾，复用原会话、工作区、反馈和恢复机制：

```text
准备 → implement：执行轮（实现、测试、自查、候选提交）→ 报告轮 → 系统核验
     → publish：单轮（推送、创建或复用 PR、结构化结果）→ 系统核验
     → await_merge（查询、人工合并）
     → delivered（核验交付）
```

测试、自查和修复按 Spec/Plan 在 implement 的执行轮完成，不固定增加 verify/review 阶段。implement 报告轮是阶段内收尾，不增加业务阶段或 DAG Task，不增加 worker 或数据库。正常路径为三次模型轮次，不按任务长度另设分支，也不为 publish 追加报告调用。实现失败也需要整理；任一轮次停止不等于阶段或 Task 完成。

第一版只实现这条开发流程。步骤序列与业务判断集中在流程定义中，worker 的状态和执行端口保持后端无关。“仅 implement 有报告轮”、候选 SHA、PR 和 TaskDelivery 属于开发流程，不能成为 adapter 或通用状态规则的要求。

## 2. 提示词与非提示词各实现什么

| 功能 | 提示词 / worker 实现 | 非提示词实现 |
| --- | --- | --- |
| 任务业务 | 理解固定 Spec/Plan，选择实现方法、修改代码、执行要求的测试和自查 | 绑定原任务契约及引用；不从自由文字重新定义任务范围 |
| 当前阶段动作 | implement 形成候选提交；publish 执行 Git 推送和 PR 创建/复用 | 选择当前阶段、提供已冻结输入；核对候选及 PR 的实际身份 |
| 结果整理 | implement 报告轮只整理既有成果/失败；publish 在执行发布的同一轮返回 done/needs_input/blocked | implement 绑定源执行和报告；publish 直接绑定原发布输入/轮次；核验真实产物后才推进 |
| 阶段推进 | implement 执行轮自由说明、不要求 JSON；其报告轮和 publish 单轮提供结构化结果 | implement 区分两轮终态，publish 核对单轮终态；模型声明与报告成功均不能直接修改阶段状态 |
| 反馈与恢复 | 理解用户反馈，检查当前成果，完成本阶段未完成部分 | 绑定原问题/输入、保存答案与投递意图、核对恢复资格；重开不自动继续 |
| 权限与故障 | 使用已有原生工具与授权机制；报告具体问题和未确认结果 | adapter 配置真实沙箱/审核边界，中心保存阻断/未知事实并控制是否允许继续；阶段状态本身不授予权限 |
| 提交、推送、创建 PR | worker 调用真实工具，报告结果；这些操作不是中心专用程序发布器执行 | provider/工作区核验实际 SHA、分支、PR 和合并；原生记录保留工具事实 |
| 交付与终态 | 不负责合并、关闭 Task 或释放依赖 | 中心经真实合并核验，在唯一交付事务中完成 TaskDelivery、关闭 Task、释放后继 |
| 展示与历史 | implement 执行 final 是正文，报告 summary 是整理说明；publish summary 是该轮的发布结果 | UI 分开显示实现执行/报告事实，例如“执行失败，报告已生成”；publish 不显示不存在的报告轮；结构化说明不直接成为成功/错误权威 |

提示词是行为指导，不能凭自身保证 Git 命令可靠或阻止所有错误工具调用。程序能拒绝错误的结果/状态推进，真正的操作权限仍由原生沙箱及工具边界保障。中心 Operation 保护阶段输入/反馈投递，不冒充 worker 每条 Git/API 操作的事务。

### Worker、流程定义与 adapter

| 边界 | 实现职责 | 不承担的职责 |
| --- | --- | --- |
| Worker | 保存位置和状态，校验来源/版本，渲染输入和解码结果，调用流程判断，原子保存结果/转移/投递意图，复用现有观察、对账和恢复 | 不识别 Codex thread/Orca Dispatch 的内部结构，不按后端复制流程，不让模型直接设置状态 |
| 流程定义 | 普通代码描述步骤顺序、短提示词、结果契约、业务核验和继续/等待/完成条件 | 不实现原生通信，不自行写状态或投递，不成为第二状态权威 |
| 后端 adapter | 原生准备、输入/问题投递、观察、对账、停止/恢复/资源事实；规范化终态、输出来源与能力 | 不选择业务下一步，不渲染开发提示词，不自行判断业务 done 或写中心状态 |

只保留“流程 + 步骤”两层。开发流程的模型步骤为 `implement.execute`、`implement.report`、`publish`，后面是 `await_merge` 外部等待及交付终态；report 是流程内步骤，不是新增 DAG Task。步骤以 ID 关联短模板、结果契约及必要的业务核验函数，转移由普通 Go 函数显式判断。流程标识/版本随执行冻结，缺失规则或必要能力在发送前拒绝，不由模型补全。

这些职责先落在现有 worker 内部的少量函数和接口，不要求新建通用流程应用、渲染/解码服务或多个空包。不建设事件总线、转移/动作注册框架、DSL、模板引擎、提示词插件或独立阶段存储；出现实际复用需求再拆组件。

后端抽象复用现有生命周期端口，具体平台身份作为不透明绑定透传，由 adapter 校验。新增执行路径只增加 adapter、配置 codec 和应用注册，不修改 worker 状态规则或流程模板；必要的来源/能力字段在现有端口补齐，不另建一套 adapter 框架。

执行方案回答“由哪个后端/agent 执行”，流程引用回答“按哪个过程执行”，分别冻结。业务步骤 ID 与原生轮次/传输分类分开，不能把 implement/report 名称直接塞到某平台原有 business/readiness 字段；映射及采集/usage 归 adapter 接线，业务语义由 worker 原输入绑定确定。能力不足据实报告，不隐式切换后端。

以后增加已有能力范围内的流程，主要增加步骤定义和短提示词；涉及新外部副作用或完成条件时还需要业务核验代码，不能承诺任意新流程都只加提示词。首版不为证明扩展性额外交付演示流程或第三 adapter。

## 3. 最小提示词与输入

2026-10-10 用户直接授权恢复此设计。当前实现使用 development/v2；首次/反馈/恢复共用三个模板，报告轮不嵌入整个旧 prompt。源执行已核实结束时，缺少完整正文以未知事实送入 report，不阻止整理；报告/发布结果仍须核对原输入和实际产物。当前接线及最小验证见[恢复 Spec](worker-prompt-convergence.spec.md)、[验收](../acceptance/worker-prompt-convergence.md)。

implement 执行轮只注入固定事实和短提示词，不要求 JSON 或内部身份回填；其报告轮另带紧凑事实包和结果契约。publish 单轮带固定发布输入、短提示词和同一结果契约。任务规则与测试要求不复制到 common 长模板，权限配置不转写成常驻说明。

**Implement**

> 当前执行实现阶段。按绑定的 Spec/Plan 完成实现、测试、自查和本地候选提交；本阶段不执行发布。结束时正常说明已完成内容、检查结果和未解决的问题，无需结构化输出。

**Publish**

> 当前执行发布阶段。将指定候选 SHA 发布到固定目标，创建或复用匹配的 PR；不修改候选代码。结束时按给定结果契约返回实际 PR URL；未完成则报告已完成部分、阻断或待决定事项。

**Implement 报告轮**

> 只整理输入中的原执行情况，不修代码、不重试命令、不补做检查。按给定结果契约返回结构化报告，包含实际产物、已完成部分、失败或待决定事项；没有证据的内容标明未知，不能把报告生成成功当作原执行成功。

实现报告输入由程序生成：原阶段目标、源执行轮的原生终态/错误、已有执行 final、已采集检查结果及当前候选/已有 PR 查询事实。只带必要内容，不重放整个历史；截断或证据缺口明确标记。报告轮不重新执行 Spec/Plan，信息不足就报告 needs_input/blocked，不补做工作。publish 没有这份报告输入或后续报告轮。

报告轮优先无工具或仅开放核对所需的只读工具，禁止 commit/push/PR 写操作和恢复执行。实际工具限制须由 adapter 支持并核验；只有只读提示词时不能宣称强隔离。首版允许不支持逐轮只读限制的 adapter 继续使用原冻结权限，但报告行为限于只读整理；程序在报告前后核对候选 HEAD、索引/工作区和已绑定发布产物的事实。确认业务写入则阻断，事实未知则对账，均拒绝自动推进；此补充核验不宣称能拦截或发现所有外部写入。第一版不新增独立报告 agent 或模型服务。

固定事实由正常序列化器提供：Spec/Plan 的固定 path/commit/sha256，实际 worktree、branch、base、remote/provider，以及当前步骤必需的候选 SHA、已有 PR 和任务身份。只提供任务确实需要的字段；凭据不放入输入。首次未产生的原生身份不伪造。

反馈、恢复、阻断不再各有一套提示词，而是结构化附加输入：

| 输入类型 | 数据 | 程序发送条件 / worker 用法 |
| --- | --- | --- |
| feedback | 原用户问题/选项/完整文字、已有成果 | 明确接受的原目标反馈；在中心指定阶段处理，不能据“继续”自行换任务或目标 |
| resume | 已核实的当前成果、恢复方式、未完成步骤、原操作结果、用户确认 | 现行恢复门槛通过后发送；先检查已有成果，再继续未完成部分 |
| blocker | 具体失败动作、已知结果、未知事项、实际审核回执、允许的下一步 | 发送资格通过后的明确继续；保留候选，先对账，再处理未完成动作 |

新输入只带相关数据，不携带整个流程规则。确有数据不能清楚表达的必要指导，可针对本次附一句，例如“先核对已有结果，只继续未完成部分”或“为原发布命令申请原生审核”；不能重新长成 common/feedback/resume/blocked 策略模板。

等待合并、已经交付、环境仍阻断或原身份未知时不发送执行 prompt。活动轮次的原生问题仍按原 call/message 回复，不提前开始报告轮。报告轮运行期间不并发启动新的执行轮；新反馈按原资格受理，不改写已冻结的报告输入。

## 4. 结果与系统绑定

implement 执行轮 final 不作结构化解析。程序保存其事实后，在可安全接续的原会话中投递 report，解析该报告轮的规范 final；publish 则直接解析发布单轮的规范 final。两者使用同一严格结果契约，不增加完成工具，也不从自然语言“成功”猜测完成。契约与解析器随流程冻结，只有 implement 执行提示词不承载结构化要求。

implement 报告轮返回 outcome、产物及说明，例如：

```json
{"outcome":"done","outputs":{"candidate_sha":"实际完整候选 SHA"},"summary":"实现和计划内检查已完成"}
```

发布 done 的 outputs 为 `pr_url`；候选 SHA 从已冻结的发布输入取得，不要求模型重复抄写。needs_input 报告 question，blocked 报告 blocker：具体类别、动作、错误和已知/未知结果；没有的产物省略，不能填假 SHA/URL。summary 不用于解析状态或证明测试通过。

失败和部分完成使用同一契约。publish 单轮中推送成功但 PR 未完成时，outcome 为 blocked，outputs 保留已知提交/分支，blocker 说明失败/未知；不另起报告轮，也不能因部分产物改报 done。implement 的已确认源失败、中断或未知不能被报告轮正常结束覆盖；独立平台事实证明交付时由系统结算，保留原失败记录。

**身份不由模型填写。** implement 的报告输入绑定源执行 Command/轮次和快照，输出再绑定报告 Command/轮次；两个输入的原生来源分别保留，正常共用原 Attempt。publish 结果仅绑定原发布 Command/轮次及冻结候选，不伪造一组报告身份。系统补齐步骤实例与输入版本，模型不能选择源执行或代次；工具 stdout、准备轮和 implement 执行正文不能冒充有效结构化结果。

流程版本及对应步骤顺序、三个短模板和结果契约随 Execution 固定；复用现有配置/输入冻结机制，不另建摘要目录或快照框架。必要的流程引用、步骤位置、输入版本和结果引用保存在既有记录中。implement 两轮及后续 publish 正常复用原 Attempt/session；本开发流程仅 implement 使用 report，publish 不调度 report。活动问答沿原输入绑定，新执行反馈更新版本；implement 报告重试绑定原执行/快照。重复结果 first-wins，冲突或迟到来源不推进。

## 5. 状态切换怎么判断

### 通用状态与流程位置

保留以下有限状态集合，不为精简把不同继续资格都藏进 waiting。`flow_ref/step_id` 标识流程及当前位置，`status` 标识运行情况；不再分设业务 stage_id/round_id。原生轮次、输入和资源事实沿既有记录绑定，同一步骤重试或反馈产生新输入，不复制整套状态。

实施时将现有 WorkflowRun.Phase 及其转移规则收敛到这一个通用状态权威，不并存两套可写的旧 Phase/新 status。Execution 中的原生运行 Status 是不同事实，继续保留；业务位置、通用状态及原生事实由同快照投影给客户端。

| status | 意义 | 可触发的动作 |
| --- | --- | --- |
| queued | 原目标下已有授权，等待指定轮次投递 | 保存原输入意图后投递一次；原生开始事实到达才成为 running |
| running | 指定轮次已开始 | 观察原输出/终态；原生问题按原路由处理，不并发开启另一轮 |
| waiting_input | 有明确问题需要原目标的新答案 | 展示 Decision；按规定的原请求或明确选定轮次继续，不根据自由文字猜下一阶段 |
| blocked | 已确认的失败、能力缺失或前置不满足 | 保留原因及部分成果，等待前置解决/明确继续；不得自动循环 |
| reconciling | 输入、报告或外部副作用结果未知/冲突 | 只读核对原操作及产物，不换 ID 重投，不启动替代 worker |
| waiting_external | 本流程等待已声明外部条件 | 查询该条件或受理允许的反馈，例如开发流程的人工合并 |
| completed / abandoned | 经核验的业务终态 | 不再启动后续轮次；资源停止/清理仍按事实处理，不由业务终态冒充 |

### 结构化结果怎样改变状态

报告不是只存 summary。worker 接收路径核验来源、步骤/轮次绑定、版本、原生回执与结果格式，再调用流程的业务核验函数，按下表判断转移。表中的事件名称只描述已核验结果，不要求建设事件路由或注册系统。接受结果、新状态及下一投递意图同提交成功后才允许外部投递，不能分两次写入后再猜测是否已经推进。

| 已核验事件 | 状态影响 | 触发内容 |
| --- | --- | --- |
| result_done | 按流程代码的完成规则更换步骤，并进入 queued、waiting_external 或经完成事务进入 completed | 下一步短提示词、外部等待核验或业务完成处理；adapter 不选择 publish/PR |
| result_needs_input | waiting_input，保留位置与已有成果 | 对应问题/允许的继续动作；答案先持久化，按明确目标投递 |
| result_blocked | blocked，记录确定失败与前置 | 阻断展示/明确处理入口；不自动向后执行 |
| result_unknown | reconciling，保留原位置及原操作身份 | 原请求/外部事实只读对账；例如 blocked 中的 unknown_external 归此类，不当作普通网络重试 |
| result_invalid / validation_failed | blocked，原因 result_invalid 或 artifact_mismatch，保留执行事实 | 开发流程仅允许安全、有界的 implement 报告重试；其他动作必须由对应流程规则声明 |
| external_condition_verified | 按该外部等待规则转移 | 开发流程核验合并后执行唯一交付事务；其他流程消费自己的外部条件 |

模型只给出 outcome/产物/问题，不能指定 status 或下一步骤。`done` 未通过事实核验时不是 result_done；原生或外部结果仍未知时优先 reconciling，不能被一个格式正确的报告清掉。流程函数明确返回下一位置/状态及所需动作，未定义结果不触发新执行。

### 本开发流程的具体规则

implement 区分源执行事实、报告轮事实、报告内容和产物核验，报告轮成功不能覆盖源执行失败。publish 只核对原发布轮次、其结构化结果和真实平台产物，不等待第二轮。任一原生成功终态或模型声明都不独立证明阶段或 Task 完成。

implement 自动报告仅限同一服务实例、源执行轮停止且原 worker/session 可安全接续、无未决输入/活动问题。实现失败且资格满足时仍整理失败结果；崩溃、身份/停止未知或环境阻断则只保存事实并显示结果待核验，不自动重建。publish 失败直接接受其结构化阻断；缺结果时只记录诊断和只读对账，不用报告轮兜底。恢复发送仍遵守人工门槛。

| 当前状态 / 事件 | 程序判断依据 | 转移 / 动作 |
| --- | --- | --- |
| 准备完成 | 现有领取、工作区、原生启动资格和回执满足 | queued(implement.execute)，保存输入意图后发送一次，开始回执到达再 running |
| implement 执行轮结束（成功或失败） | 保存终态/正文/快照；原轮次停止，原 worker/session 与资格已核实 | queued(implement.report)，发送报告 prompt；不直接进入 publish。已确认源失败仍保留，报告最终只能整理阻断/部分成果 |
| implement 执行轮停止未知或 worker 崩溃 | 停止/副作用未知为 reconciling，确认崩溃且安全停止为 blocked | 不发送报告或新建 worker；保留 implement.execute 及原事实，恢复发送需明确授权 |
| implement 的报告为 done | 两轮绑定及成功终态有效，候选 SHA 匹配原快照和当前固定 HEAD/branch，无未确认操作 | 接受 result_done → queued(publish)，冻结候选 SHA 并保存发布意图后投递 |
| publish 单轮结果为 done | 原输入/成功终态有效；provider 核实固定 PR 仓库、head/base、head SHA 等于冻结候选 SHA，无未知操作 | 接受 result_done → waiting_external(await_merge)，绑定 PR；不发 report/Continue |
| 有效 needs_input / 确定 blocked | 问题/阻断与来源匹配，部分成果保留 | waiting_input / blocked；不换阶段，不自动执行。继续目标由原生路由或明确继续动作指定 |
| implement 报告失败、缺结果/JSON 无效 | 报告已知停止为 blocked(result_invalid)，发送/终态未知为 reconciling | 已知停止且资格/预算允许：queued(implement.report) 重试一次，源执行不重做；否则保留待核验 |
| publish 缺 JSON、失败、未知或产物不匹配 | 确认失败/格式/产物问题为 blocked；副作用/终态未知或身份冲突为 reconciling | 留在 publish；只读对账或明确继续原发布步骤，不生成 report、不为补格式重做副作用 |
| 报告写入事实变化或旧来源/输入 | 变更冲突进入 blocked/reconciling；旧结果不具备当前转移资格 | 不触发下一轮，不重定向当前阶段；保留诊断和原归属 |
| await_merge 检测真实合并 | PR 与原候选匹配，合并核验及唯一交付事务成功 | completed(delivered)；通过现有交付端口关闭 Task、释放后继，不新建完成 hook 框架 |
| publish/await_merge 的明确代码反馈 | 可继续、未合并、范围与原契约一致 | queued(implement.execute)，新步骤输入版本；旧 PR/历史保留，之后发布新候选 |
| 明确发布修复或 PR 元数据反馈 | 原目标/候选/资格有效，未知操作已对账 | queued(publish)，保留原提交/PR，不重新实现 |
| 服务重开 | 先恢复原事实及未知操作，新输入必须取得本次明确授权 | 可观察则保持 running；未知为 reconciling，需人工决定为 waiting_input/blocked；不补发原 queued 的报告/下一阶段 |
| Task 终态、候选退出、原 PR 合并交付 | 现有唯一交付或退出事实 | completed / abandoned；拒绝新轮次，迟到结果不能重开，资源状态独立核实 |

接收路径可以概括为：

```text
implement：保存执行终态/正文/快照 → 资格满足后投递报告轮
publish：保存原发布终态/回执 → 直接接收该轮结构化结果
  → 按各自来源绑定严格解析、去重
  → needs_input/blocked：保存原因，留在当前步骤
  → done：implement 核对两轮，publish 核对单轮；再核验本阶段产物
  → 按固定流程的普通代码判断新状态/步骤/动作
  → 同一业务提交接受结果、新状态与动作意图
  → 若允许继续：按该原动作投递并确认回执；未知只对账
```

这是阶段边界核验，不是中心重新测试所有代码。实现 done 仍含模型对业务完成的声明；是否满足 Spec/Plan 由该任务已有验证及人工审阅处理，不把候选存在或模型测试报告冒充独立验收。

## 6. 重试与权限边界

相同服务实例内，用户启动候选授权按冻结顺序完成 implement 执行/报告和 publish 单轮。服务重开遵守[停服恢复 Spec](worker-service-recovery.spec.md)的人工门槛，不补发报告或下一阶段。implement 已执行但报告缺失时，明确继续只能整理原结果；publish 的明确继续仍是原发布步骤，先查询已有分支/PR，再完成未完成动作，不另拆报告轮。

恢复仅继续已核验的原会话；原会话不可恢复时保留事实并交由人工处理，不开放新 thread、新会话或新 Attempt 替代。步骤状态不扩大当前后端的恢复能力。

只有 implement 的报告格式错误或已知失败，才可在原服务/资格满足时对同一执行/快照 report 重试一次；仍失败则待核验。未知报告发送先对账，不换 ID 盲发；重试不能触发实现或发布。publish 不套用此规则，结果缺失/无效时只读对账并保留待核验，不能自动再跑一轮发布来补 JSON。所有执行、报告和发布调用及 usage 如实记录。

结果未知时先对账原请求；人工点击继续不能证明旧副作用未发生，也不能用新身份重发。已知部分成果、失败和未知事项在报告缺失时也由程序保留，不能只剩一个格式错误覆盖原问题。

发布权限保持既有原生设置。需要批准的原发布命令走原生审核；拒绝/不可用或获准重试仍失败时保持 blocked，不改工具、账号或权限绕过。临时网络失败先查询实际结果，再做有界重试。首次 Git 失败不临时改成模型手工构造 blob/tree/commit 的 API 路径；如需另一种发布方式，应先明确并验证其契约。

这些边界由状态机、原生权限适配和结果核验负责；worker 仍负责调用具体工具，程序不能仅靠阶段字段硬拦截其全部脚本。故障指导只在相应阻断输入需要时提供，不能宣称精简提示词已经修复 Git/沙箱兼容性。

## 7. 实施与扩展边界

本任务实施时调整[整体架构](../architecture.md)、[执行共同约束](execution-architecture.spec.md)和[架构规划约束](../roadmap/architecture-constraints.md)中的“中心不管理业务步骤”限制；保留原会话、冻结权限/目标、恢复授权、单写者、唯一交付和双本机支持范围。不恢复固定五步编排、固定审查循环或额外认证语言。

同一候选成果调整现有 worker 的状态/步骤处理，交付一个代码内建开发流程、三个短模板和两个已有后端接线。协调结果来源、持久提交、查询/UI 和 app 组装；共享契约先固定，不按目录强拆任务。新增外部业务需要自己的核验代码，普通流程主要增加步骤与短提示词；事件框架、额外演示流程、第三 adapter 和通用模板体系留到实际需求出现时再规划。

本机安装/调试隔离与未来原生页面的接口影响在 Plan 中核对。开发执行按当前 RoadmapGuide：用户确认 #84 纳入 execution-map 后，启动时固定本 Spec/Plan 的实际 Git commit/路径，核对两个后端能力及测试准备；不要求外部中心准入。产品测试中的 Task/DAG 创建和交付仍走真实接口，不由开发文件模拟。输入/结果及必要格式版本在实现中一并落地；未支持旧格式在副作用前拒绝并保留现场，不迁移、双写或自动清空已有运行。

## 8. 当前代码与需要调整的部分

以下是 2026-10-09 本工作树的源码核对，不表示新流程已实现。

| 当前代码 | 已有能力 / 限制 | 本次调整 |
| --- | --- | --- |
| [model/state.go](../../internal/execution/model/state.go) | 平台无关的 BeginTurn/StopTurn/问答规则，但 State 包含 PR，StopTurn 无 PR 时进入 waiting_decision，绑定开发交付 | 复用纯规则基础，收敛为单一 status/步骤位置；PR/Task 完成判断集中在开发流程函数，不复制一套并行状态权威 |
| [prompt.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/prompt.go)、[publication_snapshot.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/publication_snapshot.go) | 单函数拼接实现、提交、推送和创建 PR 全部要求，追加固定发布目标 | 按步骤选择三个短模板与必要事实；复用输入冻结，不建立模板引擎或快照框架 |
| [领取](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/native_claim_command.go)、[领域记录](../../internal/execution/model/records.go) | CurrentStep 领取时固定为 worker；持久化已有身份、轮次、反馈和恢复字段，没有新步骤/报告绑定 | 增加必要流程引用、step_id/status、源执行/报告输入与结果引用；复用已有记录、版本和原子提交 |
| [worker_lifecycle.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_lifecycle.go)、[完成处理](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_completion.go)、[停止解析](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_state.go) | 原输入意图/原生回执/会话恢复已共用；业务 TurnInput.Purpose 当前固定 business；停止后按 PR/问答走固定分支 | 现有接收路径解码结果、调用流程核验/转移函数并调度动作；步骤标签与原生 purpose 明确映射，不新增流程运行框架 |
| [backend/session.go](../../internal/execution/backend/session.go)、[LifecyclePort](../../internal/execution/backend/lifecycle.go)、[执行目录](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/execution_directory.go) | 已有 Prompt/CommandID、生命周期执行/观察/对账/恢复的窄端口与后端注册；尚无新的结果角色/报告能力契约 | 复用并收窄消费者能力，补必要不透明轮次绑定和真实能力描述；report_completion 原本是原生结算，不等于新模型报告轮 |
| [Codex adapter](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/adapters/codex)、[Orca adapter（历史）](https://github.com/erranliu/SteerLine/tree/872dba4462a4f659d7685eb2a397211a1653c632/internal/execution/adapters/orca)、[app 注册](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/app/directory.go) | 两套原生实现经目录组装，平台身份/配置由实现拥有；Orca 已有 business/readiness 采集分类 | 补齐规范终态/final 来源、步骤标签映射及实际工具约束能力；原生差异留在 adapter，新增路径只增加实现/codec/注册 |

先固定最小结果契约、步骤字段和来源绑定，再调整现有接收/提交、adapter 映射及 UI 消费；PR 实际核验、恢复和唯一 TaskDelivery 复用原端口。必要的 schema/codec/公开 DTO 版本随本实现一起更新；集成顺序和最小测试见 Plan。当前仅完成规划，运行代码及安装程序尚未调整。
