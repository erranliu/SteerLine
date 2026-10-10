# Orca supervised 结果协议审计与原生探针

归属：用户直接要求审计 Orca supervised 分步结果修复方案，并先验证公开接口。普通开发的前置验证，不占 Task ID，不修改 roadmap，不代表修复实现或正式交付验收完成。

## 审计结论与收敛方案

原“完整结果消息 + 原 Dispatch 结算 + 中心冻结证据”契约合理，与 [既有通用结果契约](../spec/orca-native-worker.spec.md)中的 worker_declared 语义一致。初案用 status 传结果、worker_done 传完成，需要新增双消息配对、乱序和缺失恢复；本次真实探针证明可以进一步收敛成单条 worker_done：

- body 继续遵循 Orca 的三句执行摘要。
- payload 保留原生 taskId、dispatchId、outcome，并增加 steerline_result，仅含 version、command_id、input_version、body。中心通过冻结的 Dispatch 映射取得 Execution、Attempt 和 Step，不重复传一套业务身份。
- CLI 使用一份完整原始 --payload；不能同时传 --task-id、--dispatch-id、--outcome 等便捷 payload 参数。按当前原生 preamble 保留执行文件、发送者和其他授权参数。
- 接收者严格核对原 Run/Task/Dispatch、消息来源、terminal/incarnation、中心 Command/输入版本，以及原 Dispatch 的实际结算。先保存结果和消息 ID，再 ack；同 ID 重放去重，冲突结果阻塞。
- 结果冻结与来源字段接入现有 TurnEvidence、StageResult 和会话正文读取。来源标记 worker_declared；完整消息加结算证明应用定义的结果，不声称 provider 原生 final phase，不代替 Git/PR、工具成功或 TaskDelivery 核验。
- 缺字段、超限、错身份或结算未知不推进下一步。沿用保守消息预算，检查完整序列化大小，不截断、不分块，不新增远端文件传输、私有 agent 日志解析或具体 agent 分支。

能力布尔值必须在接收、绑定、持久化及恢复接线实现后启用。探针执行时的产品实现仍明确拒绝该开发流程，本记录只证明外部传输可行性；后续接线与当前边界见[实施验收](orca-supervised-results.md)及[能力记录](orca-remote-l3.md#当前分步流程能力缺口)。

## 验证范围与证据

源码 HEAD：40446a671badc901bcf868872a383b421d646803；开始时工作树干净。日期按 Asia/Shanghai：2026-10-10。

原始产物、临时探针脚本、发送/接收记录、断言和清理记录：

`C:/Users/lmas1/AppData/Local/Temp/steerline-orca-result-probe-20261010-092514/`

核心产物：verification.json、verify.py、local/remote-start.json、local/remote-show.json、collect/replay/ack-*.json、local/remote-release.json、cleanup-verification.json。probe.py 是本次现场专用脚本，不是产品驱动或当前 L3 登记入口。

| 项目 | 本机 | 远端 |
| --- | --- | --- |
| Orca CLI / runtime | 1.4.223 / 1.4.223 | 执行端 runtime 1.4.223，经本机 CLI 路由 |
| runtime | e11d4024-70e8-46c6-b86a-e17338831ba9 | 95d30a41-2f86-43ae-ac0d-9c304b5f1f90 |
| agent/model | 用户默认 Codex / gpt-6.1-sol high | Codex / 实际运行配置 gpt-6-luna high，requested/effective 一致 |
| Orca Task | task_c1713074b862 | task_e72b87528870 |
| 成功 Dispatch | ctx_6a1c8f931e1c | ctx_35e070b30c68 |
| 来源 | term_8084c72e-2e48-45e2-9738-bf3d7169bcc1 | dispatch:ctx_35e070b30c68 |
| 原生完成消息 | msg_097d7ccd2848 | relay_2561287b08d2 |
| 接收结果 payload UTF-8 大小 | 341 字节 | 342 字节 |

共同独立 Run：run_e25f555574e2。完整结果批次：delivery_0c6817e3b7ac。

| ID | 方法 / 实际入口 | 结果与边界 |
| --- | --- | --- |
| OSRP-A1 | 真实 worker-start → worker 内部 subprocess argv send → 中心 check；verify.py | 通过：本机、远端 worker_done 自定义嵌套结果完整保留，结果与期望逐字段一致；内层正文 UTF-8 与期望一致。中文、换行、单双引号、反斜杠、emoji、$() 和反引号样本通过 |
| OSRP-A2 | 同一 probe 的 status 小样本与较大样本；verify.py | 通过：112 / 6626 字节 status 正文按 UTF-8 字节精确一致；这是传输样本，不冒充同样大小的 final payload 边界验收。初轮 10000 字符 padding 的 status 也被原生接受，8 KiB 是 SteerLine 保守预算，不是测得的 Orca 原生硬上限 |
| OSRP-A3 | worker-show + 已保存 start/Run + verify.py | 通过：原 Task/Dispatch/Run、创建的 terminal、工作区、精确 worker、执行 host 和 runtime 一致；原 Dispatch completed / worker succeeded |
| OSRP-A4 | check、无 ack 重读、比较完整 messages、处理后 ack | 通过：Delivery ID 和六条消息完整内容不变；处理后 ack 返回空批次。不是中心服务重启或 Operation 响应丢失恢复验收 |
| OSRP-A5 | 下列定向 Go 回归 | 通过：supervised local/environment 生命周期、路由/身份回归，以及既有迟到消息拒绝和原结果绑定测试。不声明新增 payload decoder 已实现 |
| OSRP-A6 | 原生 worker-release、worker-list --include-remote、精确 terminal close/worktree rm/show | 通过：两个成功 worker released 且输出已归档；失败远端 attempt 已 stopped，execution_host 证明 exited；coordinator 关闭，两个探针工作区移除并返回 selector_not_found，本机路径消失 |

定向 Go 命令（无全量、无产品 E2E）：

```powershell
go test ./internal/execution/adapters/orcaremote -run 'Test(Local|Supervised|.*Lifecycle|.*Identity|.*Message)' -count=1
go test ./internal/execution/adapters/orca -run '^(TestCD62NativeFactsAndQuestionRoute|TestWorkerStageNativeFinalUsesOriginalSettledMaterial)$' -count=1 -v
```

现场断言复核：在原始产物目录运行 `python verify.py`；它只读取已保存证据，校验消息、冻结期望和已记录结算，不调用模型或修改 Orca 状态。

## 失败与干预记录

1. 本机首次 worker-start 返回 outcome_unknown / turn_start_unobserved。公开 worker-read 明确显示原提示仍在粘贴草稿；仅向原 terminal 发送一次裸 Enter，未重发任务。后续活动与最终结算得到核实。本次本机传输通过不能证明首次自动提交稳定。
2. 初轮测试模板错误地组合 --payload 与便捷字段，worker_done 被 CLI 明确拒绝，原 Dispatch 未结算。修正为原始 payload 单一路径后，在同一 Task/Dispatch 上收到一条成功完成消息。拒绝不计成功。
3. 本机初轮字面 emoji 在提示注入/脚本生成阶段丢失；该样本未计完整性通过。修正样本用 Unicode 转义构造，再比较实际发送与接收的 UTF-8；最终结果保留 emoji。
4. 远端初轮默认 gpt-6.1-sol 被其 Codex 账号拒绝，Dispatch ctx_876d047e5617 未发送完成。原生 worker-stop 确认关闭该终端；release 返回 retained / identity_unproven，如实保留。采用 105 的现有 Luna high 配置、原 Task、原工作区和 --retry-of 重试；新 Dispatch 成功。公共 task-update 不支持改 spec，修正说明通过该新 Dispatch 的原生消息投递，worker 读取后落实。失败不被新成功覆盖。
5. worker-read 的 transcript 在长工具输入处发生明确 clipping；它仅作诊断，未用于推断规范 final。正式结果来自未截断的原生完成消息。

清理前确认三个探针 Dispatch 均 exited；未用 terminal close 代替 worker-release。删除对象核对为本次创建的精确工作区，探针文件先保存；没有 reset Orca、创建业务 PR、启动 105、改变 execution.json 或运行数据库。

## 尚需实施与验收

外部接口满足收敛方案的核心前置。探针本身未覆盖：新增结果 decoder 的非法字段/错 Command/超限/冲突负例、中心保存/ack 前后重启、每轮完整序列化预算、原会话 execute/report/publish 接线、会话正文读取、真实业务反馈/PR 交付。后续已覆盖与仍待验明细登记在[实施验收](orca-supervised-results.md)。

这些项应在后续实施 Plan 的测试计划中映射到最小定向回归和任务专属验收，不把本次原生探针当成已实现或完整 L2/L3。启动草稿提交问题与默认模型拒绝是独立现场事实，不能归因于结构化结果传输。
