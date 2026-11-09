# 主 Agent：给定材料的后续对话提示词建议

状态：按 2026-11-09 用户确认方案实施。取代此前严格零工具、生产永久阻断的草稿要求；复用 Runner generate/1 的本地 Codex 只读执行。配套：[Plan](../plans/main-agent-advisory.plan.md)、[验收](../testing/acceptance/main-agent-advisory.md)。

## 1. 目标与模块边界

核心目标是独立模块及稳定协议，功能保持最小介入。主 Agent 接受外部明确提供的相关材料，唯一业务产物是一段供后续对话使用的提示词。使用者采用、修改、转交或执行属于外部行为，不记采用状态，不投递答案、修改材料、创建 PR 或协调任务。

`internal/mainagent/contract` 仅依赖标准库，拥有 advisory-v1 request/context/material/suggestion 和严格校验。`internal/mainagent` 只校验输入、通过 prompts 同包组装固定正文及校验唯一输出，不依赖 Runner、app、存储、Git、网络、进程或业务命令。`internal/mainagentbridge` 拥有宿主异步作业与原 Runner 请求；app 提供限定业务事实并装配。SQLite 只提供原子 payload/CAS，产品查询返回显式 DTO，不暴露内部执行记录。

协议固定 `steerline-main-agent/advisory-v1` / advisory。输入包括 request_id、goal、可选 user_input、明确材料（id/label/body/source/revision）。64 项材料、序列化上下文 1 MiB、输出 prompt 16 KiB，均按 UTF-8 字节计；非法 UTF-8、重复材料 ID、未知字段、未知协议/能力和超限拒绝，不截断。输出只有 protocol/request_id/prompt，拒绝旧分析/动作对象。

## 2. 生成执行约束

调用链固定为 SteerLine → Runner 公开 generate/1 → Runner 内部本地 Codex。复用现有请求、Run、模型配置、只读权限、完整 final、停止及资源结算；不新增 workflow、限制模式、原生直连或模型 API 旁路。

用户明确接受 generate 既有 `read_only` 边界：原生只读工具可以使用，不承诺严格零工具、无隐式指令或单目录读取隔离。“只依据提供材料、不要探索或调用工具”属于固定提示词的行为要求，不冒充执行层硬保证。

宿主将冻结的规则与请求作为完整文本提交；不提供 generate.repository、业务 directory 或供模型读取的 file 材料，使用 Runner 默认私有空目录。固定规则仍来自 internal/prompts 文件并同包组装，缺资产失败，无 inline 兜底；此用途正式完整文本通道在提示词治理中明确例外，其他用途不变。

设置固定本地 Codex，默认 GPT 6.1 Sol / high；复用已有本地执行器和 Runner compose 配置，配置在新请求准备时冻结，旧请求不因设置变更重建。模型/执行不可用正式失败，不换链路。

## 3. 唯一真实触发来源

用户已选择直接采用**开发候选的明确待答问题**，包括普通待答；不再限制为不存在的 requirements_unclear/requirements_conflict 原因码。原材料不足和冲突是该来源可以处理的内容，不是靠文案识别的分类。

来源为原 owner 持久保存的 Runner 候选投影：候选仍参与、未请求停止，Run 的当前 Question/Attempt 身份完整且处于明确待答状态。通用 blocked/needs_attention、没有 question 的错误、审批不自行推断成来源；不调用模型分类，不修改 Runner。

稳定身份为 project/candidate/原 Attempt+Question occurrence/advisory-v1。后台独立于 Web 从持久事实登记 queued，再准备上下文。心跳、普通 revision、重复消费、重启不重开同一事项；取消或失败不自动重试。新 Question/Attempt 才是新事项。

上下文固定原问题完整题集/选项、候选绑定 commit/digest、Spec/Plan、公共及验收引用、已核实 final 和必要测试安排。不遍历仓库、其他任务或引用图；不混用当前工作树。准备失败保留正式记录、已取得的材料及诊断，不伪造完整上下文。来源解除后取消未提交工作；已提交则停止原生成并结算，历史成功产物仍可查看/复制，不改原问题。

## 4. 宿主可靠性与资源归属

生成记录先于模型提交持久化，包含原 advisory 请求、组装正文/digest、冻结的 Runner 请求、build/namespace、Run 关联与停止信封。核心不接收 resolver/读取闭包。宿主通过现有 home 级 ContentQueue 共用生成配额，不能另开不计预算的模型工作。

首次 Submit 前保存 started/unknown，结果未知只 inspect 原 request_id/Run，不换 ID 重发；queued 尚未尝试才可恢复首次提交。结果需验证原请求/Run、project/object correlation、workflow/input digest、单调 revision、完整 ContentResult 来源及 native success；成功产物只在资源 stopped 后完成。返回旧动作对象、空白或超长正文明确失败，不自动生成修复。

显式取消、源解除、关闭或升级保持原资源身份和停止义务。取消后迟到结果不覆盖取消；停止结果未知保持 waiting，原 stop 信封不因轮询重建，仅明确 stale_revision 拒绝后允许新停止信封。重启恢复、构建 namespace 核验、启动前待停止集合及离线停止证明必须包含本用途；原生资源未结算不能释放配额或报告完成。

## 5. Web 与活动中心

候选详情只读查看状态、本次明确上下文及唯一 prompt，提供复制和显式取消。无生成、编辑、补充生成、采用、执行或材料 PR 入口。查看/切页不产生或取消工作；跨项目/实例/候选/记录隔离响应，历史结果不受原问题可答资格约束。

活动来源为 main_agent_advisory/prompt_suggestion，沿原有只读池、预算、分页、计数、request links 和失败隔离。queued/preparing/running 表示实际阶段；unknown/cancelling/结果未结算为 waiting；经核实唯一结果并结算才 succeeded；已结算失败/取消才终态。成功/失败且源问题仍在时 needs_attention，源解除后不再提醒。导航定位确切候选和 request。

列表、详情、活动 GET 只读持久事实，零模型、Git、Runner 对账和业务写入。公开 DTO 不返回 Runner Request、Projection 或 BuildPin。原问题回答和其他业务入口保持各自 owner。

## 6. 扩展与验收

未来自主业务执行需要新的契约、授权与验收；advisory-v1 不预留 action、executor 或权限扩展开关。接受 Runner 只读工具不授予模块业务执行能力。

验收映射：A01 纯核心/协议及依赖；A02 完整输入及失败材料；A03 单一产物；A04 generate 公开接线和只读配置；A05 无 repository/directory/file、明确文本及如实能力声明；A06 无旧执行出口；A07 去重/未知/取消/配额/停止与恢复；A08 设置和最小 UI；A09 固定提示词实际差异及独立审核；A10 未知协议拒绝；A11 持久真实待答来源自动登记；A12 活动纯观察/导航；A13 无页面推进及跨范围恢复。自动受控与真实模型、人工体验分别登记，不继承旧严格模式或 PR475 的验收。
