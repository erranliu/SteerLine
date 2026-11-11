# 待合并 PR 冲突解决与原 Run 续作 Spec

2026-11-06。状态：审计修订后已实现；两仓验证与独立审核见[验收记录](../testing/acceptance/pr-conflict-resolution.md)。用户已授权两仓实现及 Runner 独立 worktree/PR，配套 [Plan](../plans/pr-conflict-resolution.plan.md)。普通开发，不进入 roadmap，不分配任务编号。

## 1. 成果与现状

用户发现待合并 PR 存在冲突时，在 SteerLine 点击“解决合并冲突”。SteerLine 提供简短指令，Runner 在原 worktree、原 session 和现有工作状态上执行，并回传结果。用户随后继续原有合并流程。

同时补齐可复用的原 Run 消息能力：后续需要补测试、修改审查意见时，可以使用相同入口；本次只交付冲突按钮，不提前实现这些业务或通用编排框架。

调查基线：SteerLine `3546eec95af22284da25cb61d7685bbd55fd2138`，Runner 为本机 `C:/Project/SteerRunner` 的只读源码观察，不代表安装 EXE 已支持。实施时记录两仓实际提交和固定 Runner 构建。

| 现有位置 | 结论与复用方式 |
| --- | --- |
| SteerLine `internal/taskrunner/input.go`、`control.go` | 有 PR 就绪后的 feedback 接线，但它不能满足当前 Runner 的受理规则；冲突按钮不经 feedback 或 resume。原问题回答继续复用。 |
| SteerLine `internal/runnerbridge/bridge.go`、`internal/runnerclient/client.go` | 复用持久控制请求、原请求对账和实例绑定；当前控制白名单需接入 message。 |
| Runner `internal/application/engine.go`、`feedback.go` | 当前拒绝开发流程在 pr_ready 后的 feedback；不能简单放开后返回 implement。 |
| Runner `internal/contracts/content.go`、`internal/runstate/record.go`、`internal/application/content.go` | 已有 message 的 revision/text、can_message 和同会话逐轮执行机制；当前仅 conversation 使用，复用机制并扩展到开发 Run。 |
| Runner `docs/workflows/README.md`、`internal/resources/catalog.go` | execute/1 单次结束并停止，conversation/1 当前为只读；保留二者语义，开发组合的公开操作目录补充 message。 |

## 2. 使用流程

1. 在已有绑定 PR 的待合并候选详情操作区提供“解决合并冲突”。资格取当前候选参与、任务状态和 Runner can_message；已知已合入、已关闭或已退出的候选不接受此操作。
2. 点击直接创建一次持久命令，向原 Run 发送短提示词及原 PR 的少量事实。无需用户填写提示词或再次确认。
3. 沿现有活动、问答和最终回复界面展示执行过程；执行中、等待回答或原请求待核实期间禁止并发新输入。
4. 本轮结束后保留现场，展示实际回复；现有输入门禁允许时可再次发送消息。未发现冲突也可以正常结束，不强制产生提交；失败或未知不自动解除原有阻断。

不主动检测冲突，不以 mergeability 为按钮前置；点击前后不新增平台查询、worktree 检查、工作区清洁检查、分支比对、会话探针、恢复式资格核验或环境检测。目标直接使用原绑定，工作现场由执行 Agent 理解和处理。协议现有的 revision、输入串行、原请求对账、停止及结果来源规则继续有效。

## 3. 两仓职责与最小契约

| 所有者 | 本次改动 |
| --- | --- |
| Runner | 将已有 `message --run-id R --input -` 扩展到已到 pr_ready、仍允许输入的开发 Run；复用同一原生会话的一轮执行、问题回答、命令回执与公开事件。公开操作目录和 can_message 与实际受理一致。 |
| SteerLine | 增加冲突按钮及产品命令，选择原候选并读取固定短提示词；用现有控制 outbox 投递 message，透传公开状态和本轮结果。业务语义只在 SteerLine。 |
| 执行 Agent | 根据原上下文和项目规则处理冲突、决定具体 Git 操作与必要验证、提交推送、必要时提问并说明结果。 |

Runner message 使用既有 `steerrunner-cli/v1` 信封、request_id、`expected_revision` 与 `text`，不增加冲突字段、工作目录/session 选择器、工作流配置或结果 schema。SteerLine 将固定短正文与原绑定的 PR URL、目标分支作为 text 冻结；不通过诊断或其他字段追加规则。

Runner 沿原 Run 的执行会话、模型、权限和资源绑定发送一次输入；不新建 Run、worktree 或替代 session，不重跑开发 Workflow，不调用实施/审计/发布提示词，也不自动安排补报或下一步。原生返回会话不可用时如实报错，不创建新会话冒充原现场。正常 message 不新增调用 `VerifyManualRecovery` 等恢复式预检。

开发流程仍保留已完成的 pr_ready 步骤和原 PR 绑定；续作通过现有 command、execution、question、final 和事件表达活动，结束后回到可输入状态。原 Workflow 结果和审计是历史事实，不被续作结果覆盖，也不被解释为新改动已经通过审计。页面同时呈现待合并 PR 与当前执行情况，不把“待合并”当作本轮已经结束。

同一会话一次只执行一条消息。等待中的真实问题沿原 reply/answer 回答；本轮结束后允许下一次显式 message。重复相同请求返回原回执；保存失败零发送；发送未知、断线或同构建重启沿原请求对账，不生成替代输入。Runner 只依据原 Run 的停止、关闭及输入状态控制投递；SteerLine 依据候选参与和 Task 终态撤销产品新输入资格，不把这些业务对象传给 Runner。迟到结果不能恢复资格。停止使用现有 Run/候选操作，不新增单轮取消产品能力。

支持范围取原开发流程已支持且公开声明 message 的执行组合，不扩展模型、Agent 或远端审计支持；conversation/1、generate/1、execute/1 的现有行为保持。既有已停止/关闭的 Run 不因此重开，不迁移旧数据或跨构建接管活动 Run。

## 4. 简短提示词与结果

实施时新增 `internal/prompts/pr_conflict_resolution.md`，通过 prompts 包的小函数读取，只附加原 PR URL 和目标分支事实。拟定固定正文为以下一段，文件使用 UTF-8/LF 并保留末尾换行：

```text
解决当前 PR 与目标分支的合并冲突，保留双方修改意图，完成必要验证，提交并推送到原分支以更新原 PR。遇到无法判断的问题时询问用户。简要报告处理结果和验证情况。
```

不指定完整 Git 命令清单、merge/rebase 算法、测试清单、逐文件决策或结构化回复格式。本次没有额外长规则材料，不增加材料分发协议；已有项目材料继续由 Agent 按原现场读取。后续若确需完整材料，再遵循[提示词维护](../../docs/prompt-governance.md)的固定文件规则，不内联兜底。

结果直接使用绑定本次输入的原生 final 和现有执行诊断，不解析正文推进业务、不新增按次结果文件或业务成功表。后续消息的结果沿现有命令/事件历史保留；旧 final 不能显示为本次回复。受理和执行状态来自原回执，冲突处理结论展示 Agent 原回复，不另外生成“冲突已解决”业务状态。不因 Agent 自述或命令 completed 宣称平台已确认可合并。PR 合入与 TaskDelivery 继续沿现有交付路径，不自动合并或关闭任务。

提示词资产及接线实施时须提供实际正文差异、UTF-8 字节数前后值、业务原因和验证，并按项目规则独立审核。本文为固定正文的业务依据，实际实现差异和审核登记到专属验收。

## 5. 生命周期差异与非目标

共用规则链接[Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)。本项差异限于 W08–W14 允许 pr_ready 后显式 message 执行一轮并等待后续输入；正常续作直接沿用原现场，不增加 W02–W07 准备/恢复前置。W17 合入交付、W18 停止及未知零重发保持。实施时同步该文档对应条目，不复制完整异常矩阵。

本次不做自动冲突扫描、自动合并 PR、通用业务编排、独立续作业务表、强制重审、新测试安排生成、材料快照协议、任意目录接管、会话重建、旧数据兼容或安装部署。

## 6. 通过条件

具体步骤、构建和实时结果见[验收规划](../testing/acceptance/pr-conflict-resolution.md)。

| ID | 通过条件 |
| --- | --- |
| PCR-A01 | 固定 Runner 的公开目录、message 受理和 can_message 一致；开发 Run 在原现场完成两轮显式消息，中间可回答问题；无新 Run/worktree/session、额外现场预检或开发阶段重跑。原有内容流程和 execute 语义保持。 |
| PCR-A02 | SteerLine 按钮准确指向原候选/PR，使用唯一固定短提示词和少量事实；无冲突检测或额外现场查询；持久请求、实例绑定、回执/结果及原问题回答经正式产品链路接通。 |
| PCR-A03 | 重复点击、保存失败、并发输入、旧 revision、未知回执及重启不重复投递；停止/退出/交付后零新输入；旧 final 不冒充本次结果，历史 PR/审计不被改写。 |
| PCR-A04 | 固定两仓构建经正式 SteerLine 操作，在真实原 worktree/session 解决人工构造的 PR 冲突并更新同一分支/PR；再次触发无冲突可结束；结果/问答可见，不自动合并 PR，最终按原方式停止资源。 |
| PCR-M01 | 后置体验走查：用户能找到按钮、看懂执行/待回答/待核实和实际结果；不阻塞自动实现和验证。 |
