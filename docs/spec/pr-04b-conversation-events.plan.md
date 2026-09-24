# PR 4B Plan：统一会话事件与完整回复

任务 ID：1；任务名称：统一会话事件与完整回复。

依据：[Spec](pr-04b-conversation-events.spec.md)。依赖：PR 4A 审核合并。

## 实施步骤

1. **先验证来源。** 在隔离仓库采集真实 local Codex、本机 Orca 各至少两轮的进度、工具、最终回复、失败/取消、等待与重放样本。分别记录 backend/protocol/agent/版本/OS/目标及实际字段，脱敏后存为 fixtures；沿用 Spec 01 探测格式。不得将 API 文档或 CLI help 当成原生证据。若公开出口不支持精确阶段，列出协议补齐依赖与未验收能力，再实现诚实的基础降级。
2. **定义事件和读接口。** 建立 envelope、身份质量、事件类型、phase、final availability、正文引用、能力报告及 Query cursor。ReadConversation/ReadMessageBody/ReadRaw 不暴露宿主响应类型或任意路径；与 Spec 02 的 Event/QueryService 共用类型。
3. **实现可靠存储。** 新记录版本、正文写入、去重、批次提交、checkpoint、崩溃重放与缺口先通过故障测试，再接上游。SQLite 存在则复用事务；否则使用唯一持锁写者的版本化日志。记录正文页与修订的一致性、坏尾部处置和磁盘满后的恢复算法。
4. **接入输出适配。** local 解码器保留真实 phase/turn 与 unknown；Orca 优先公开结构化出口、精确绑定 ref/source，terminal 降级独立 segment。相同消息的 delta/snapshot 使用来源修订去重；业务 result 的 summary 独立记录为执行摘要。将旧只按 item ID 合并的显示逻辑移至公共身份模型。
5. **解耦采集与 UI。** Service 运行时在没有客户端的情况下仍采集，客户端只读本地 Query；Windows 服务窗口关闭后采集随服务停止，再从固定入口启动时按持久 checkpoint 对账。每源单次在途读取，有界批次、取消和公平调度。客户端断连与服务重启均保留最后 checkpoint；采集不能阻塞控制或 PR 核验。
6. **完成证据。** 固定当前查询与原始字节读取语义，不建设旧 watch/activity 读取器。更新 activity/data-contract/hosting 说明；提供 docs/validation/ 下的能力矩阵和按 E-A ID 的验证报告，交接 4C 可直接消费的样本与 DTO。

## 验证

- E-A1～E-A9 使用同一公共回放套件运行两来源，断言精确身份、正文哈希、事件数量和 checkpoint；相同文本的真实不同消息不得被误删。
- 注入内容写入前后、事件提交前后、cursor 保存和重启故障；用来源已淘汰、游标非法、source_changed、乱序/重复 delta、快照晚到和终态冲突验证边界。
- 长 final 测试超过 4096 字符、跨多页中文/emoji，完整拼接与源字节/摘要核对；正文在分页中更新返回明确修订变化，损坏/丢失/超额正文有可见错误。
- 有界性测试大文件、单条超限、持续输出与慢读；断言页预算、取消响应和停止命令仍可处理。Windows 真实服务窗口保持打开时关闭/重开 viewer，确认持续采集且不重读全部历史；关窗后重新打开固定入口，确认 checkpoint 对账，并在来源确有缺失时显示缺口。
- 实际 Codex/Orca 样本分别确认精确 turn/phase 或明确缺口；terminal v2、supervised、本机、远端不能互相代验。上游缺必需能力时将该项保存为未验收，不用替身把 E-A9 改为通过。
- 执行 `go test ./... -count=1 -timeout 10m`、`go vet ./...`、watch/viewer/normalizer 回归、支持平台构建及 `git diff --check`；并发采集在支持环境执行 race 检查。

## 完成条件与回退

每项验收有证据，精确双链路能力达到约定门槛才宣告目标完成；若只能完成基础输出，则 PR/报告明确列出阻塞与剩余依赖，协调任务不得擅自当作整阶段完成并启动下一阶段。上游补齐后复用同一接口与回放测试。

当前格式的原始日志保留；不支持的旧客户端或状态格式明确拒绝。更换状态目录前须核实外部执行已停止，不能删除 checkpoint 后盲目重新执行。
