# 实现质量审计与修复

日期：2026-09-30。审计对象：PR164，原实现 `593a9cf`；对照基线 `c2c9f95`、任务 #54 Spec/Plan。未使用子代理，没有运行全量或真实 agent。

## 发现并修复

| 问题 | 影响与修复 | 针对性验证 |
| --- | --- | --- |
| Codex 原生选项直接解码为公共 DecisionOption | 公共值含中心的 Receiver/Action。合法字符串会进入原生事实，未知字段的其他 JSON 类型则改变原有忽略语义。恢复平台局部 wire schema，只将 ID/Label/Detail 转为公共选项；中心动作字段不会从原生工具输入取得，错误和 nil/空数组语义保持 | `TestDecodeQuestionsKeepsCenterActionsOutOfNativeContent` 修复前失败，显示 Receiver=center/Action=open_terminal；修复后通过，覆盖字符串和异型 JSON 字段 |
| Orca 停止后调用任意 StopHandle.Close | Close 在某些过程实现中会结束进程；原实现只取消自身 orcaProcess 的观察上下文。恢复只释放本后端拥有的观察器，原 terminal close 和回执对账继续作为停止依据 | `TestOrcaStopOnlyReleasesOwnedObserver` 修复前 unowned_handle 子例失败；修复后原观察器会取消、非本后端句柄的 Close/Kill 均不调用 |

同时加强 `TestExecutionRecordsKeepWireIdentity`：除往返解码外，固定 accepted 操作的完整 JSON 文本，避免字段名或状态值变化被同一组编码/解码函数掩盖。

## 审计与验证范围

- 去除类型限定名和既定类型更名后，对照全部受影响生产文件的函数 token：767 个函数保持一致，逐项检查剩余 14 个实质变化；覆盖输入/回答、原生事实、中心转换、停止及启动命令接线。
- 17 组搬迁的记录/身份/问题字段布局和 15 个协议/能力/操作枚举值与主干基线相同；公共包的非标准库传递依赖仍仅自身。
- 修复后的 `execution`、`codexworker`、`orcaworker` 包测试通过。
- `go vet ./internal/steerline/execution ./internal/steerline/codexworker ./internal/steerline` 通过。
- 项目 `-quick` 加原生身份、绑定/停止事务、损坏记录和 Orca 停止回执回归通过；[审计测试摘要](audit-summary.json)保存实际命令、数量（含子测试）和结果，各组均命中。
- `git diff --check` 通过；未改变序列化 schema、权限策略、恢复门槛和副作用提交顺序。原生 schema 只在平台解码器内定义，没有新增公共类型别名或并行实现。

上述结论限于本轮代码审计和确定性回归；不作为真实平台认证或全量故障矩阵结果。
