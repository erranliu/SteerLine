# 主 Agent：给定材料的后续对话提示词建议

状态：已接入现有 Runner generate/1，本地 Codex 只读生成；未合并、未部署。正式契约见[Spec](../../SteerlineFlow/spec/main-agent-advisory.spec.md)、[Plan](../../SteerlineFlow/plans/main-agent-advisory.plan.md)，实际验证与未验项见[验收](../../SteerlineFlow/testing/acceptance/main-agent-advisory.md)。用户已明确放宽此前严格零工具要求，并选择直接接入开发候选的明确待答问题。

## 模块和执行边界

主 Agent 唯一业务产物是一段继续对话的提示词。使用者如何采用、修改或执行属于外部行为。核心只处理给定请求与文本结果；不持有 Runner、存储、Git、文件/网络/进程或业务命令。没有答案投递、材料 PR 或采用状态。

| 所有者 | 职责与稳定边界 |
| --- | --- |
| `internal/mainagent/contract` | stdlib-only advisory-v1 DTO、UTF-8/字段/数量/字节校验 |
| `internal/mainagent` | 明确输入校验、固定提示词同包组装、唯一 prompt 校验，无外部读取或执行 |
| `internal/mainagentbridge` | 异步作业、冻结请求、共享配额、原请求提交/观察、取消及结算 |
| app | 从原 owner 的持久候选投影读取明确待答与固定版本材料；组装本地 Codex 模型配置及恢复/停止接线 |
| SQLite / 查询 / Web | payload/CAS与隔离、纯读活动投影、只读查看/复制/取消，不展示内部执行记录 |

调用链固定 SteerLine → Runner 公开 generate/1 → Runner 内部本地 Codex。复用既有 Run、原请求去重、完整 final、停止和资源结算；没有新增 workflow、原生直连或模型 API 旁路。默认 GPT 6.1 Sol / high，复用已有本地执行器，配置在准备新请求时冻结，重启后新设置生效。

接受 generate 原生只读工具与上下文加载限制，不承诺严格零工具、完全无隐式指令或单目录读取隔离。只依据提供材料、不探索属于固定提示词行为要求。宿主完整组装规则与材料为正式文本，不提供 repository、业务 directory 或 file 材料，使用默认私有空目录；缺资产失败，无 inline 兜底。

## 首批自动触发

来源为仍参与、未停止候选的原生 Question，须有原 Attempt/Question 身份、Run 未关闭且公开 CanReply 为 true。覆盖普通待答，也能帮助继续讨论材料不足和冲突；不依赖不存在的业务原因码，不根据文案或通用 blocked/attention 分类。Runner 不因此取得跨任务协调职责。

后台独立于页面读取持久候选，按 project/candidate/Attempt+Question/protocol 去重登记，再冻结原问题、Spec/Plan、公共及验收引用、已核实 final、必要测试安排。文档按绑定引用的 commit 与 SHA 核验，不混用当前工作树；准备失败也有作业、已取得材料和诊断。心跳、普通 revision、重启不重开同一事项，取消/失败不自动重试。

来源解除后取消尚未提交的生成，已提交则停止原生成并结算；完成历史仍可查看/复制，不代替原问题解除。新问题或 Attempt 使用新 occurrence。

## 异步工作与活动

宿主先保存冻结输入与唯一 Runner 请求，加入 home 共享 ContentQueue，提交前持久 started；未知只查原 request/Run，不重复 Submit。完整结果核验原 correlation/digest/revision、ContentResult 与公开 Final 的 command/attempt/session/turn及原文。只在资源 stopped 后完成；取消后的迟到结果不能覆盖取消。

启动前冷态检查全部未结算 started 请求：原请求不存在则失败/取消并释放槽，不重发；原失败或取消的停止信封随启动恢复。关闭、构建 namespace 及离线停止证明包含本用途。

候选详情显示上下文、状态与只读 prompt，可复制并显式取消；无生成、编辑、补充、采用或执行入口。未提交的 queued/preparing 和所有未结算记录持续观察。页面与活动 GET 均只读持久事实，不调用模型、Git 或 Runner 对账。

活动中心注册 main_agent_advisory/prompt_suggestion，复用原分页、预算、request links 和故障隔离，导航定位候选与精确 request。未知/取消/结果未结算为 waiting；成功只表示提示词已生成并结算，原问题仍在时保留 needs_attention。

## 验证与后续扩展

真实 SQLite 待答投影到后台、生成及活动的受控集成，以及一次真实 Runner/本地 Codex generate 已验证；最终全集结果和人工未验项以专属记录为准。真实链验证只做建议生成，不创建开发任务或真实 PR，不宣称严格零工具。SQLite schema 44，不迁移旧开发数据。

后续自主业务执行必须经新契约、授权与验收显式扩展。advisory-v1 不预留 action/executor 或扩权开关，本轮不建设主 Agent 工作台。
