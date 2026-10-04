# #39 多候选本机验收（真机验收中）

起点/开工最新 main：`a8ff0f01d38299d77ecc9cd4463b78a1286e1bec`（PR #208 真实合并）；独立托管 worktree：`C:\Users\lmas1\.codex\worktrees\d831\SteerLine`；分支：`codex/candidate-native-acceptance-39`。#38 CC5 仍待用户人工补验；限定合入例外不扩大到 CN1–CN8。

唯一运行入口：`go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out <新的空证据目录>`。设置 `STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 指向绝对 manifest 路径；结构如下，source_commit 替换为本次实际固定 HEAD。两份 profile 按本机 Orca、本地 Codex 顺序固定，以在 Orca 完成准备后立即启动本地兄弟并记录真实业务重叠：

```json
{
  "version": 1,
  "source_commit": "<40位实际实现commit>",
  "repository": "erranliu/SteerlineTest",
  "profiles": [
    {"id":"candidate-orca","chain":"orca-codex"},
    {"id":"candidate-local","chain":"local-codex"}
  ]
}
```

用户已将任务名称/范围收缩为「多候选本机验收」，应用协调文档提交 `b53ef52abdd36bfc7d1e8d65bd5b86957794f27c`（本分支对应 `da34963`）。当前仅保留本机字段和场景，无旧远端 manifest 兼容；生产远端规则未改。本任务不提供 #42 的远端证明，也不把远端列为阻塞或待补验。

本机只复制既有 auth.json 到隔离 home，不复制用户配置或任务库。新随机 base 分支位于 erranliu/SteerlineTest；worker 独立 commit/push/create，操作者按期望 head 合并。产品查询规范数据库中的 Task/WorkflowRun/Execution/Decision/Operation/Delivery；driver 读取规范 Conversation/完整 Body，native logs 仅为诊断/usage 证据。清理只关闭本次 base 的 open PR、删除本次分支、停止 owned 进程及本次 Orca 资源并删除 scratch，保留 PR 历史与证据。

已有结果：`native-wiring-01` 的27测试事件通过（含 MC5–MC6）、`adapters-final` 通过；`local-04` 在固定 `1b361a8` 构建上真实通过，两个候选与后继对应测试 PR #63/#64/#65，胜出与后继真正合并，落选晚到更新保持归属，清理完成。`local-05` 使用 `2eddcd6` 的新本机范围构建验证新增连接故障/放弃/兄弟继续/人工新增；最终逐项结果随后登记，不能用先前通过覆盖尚未执行的本机增量。

失败记录全部保留：local-01 漏 current Task/graph 版本被服务拒绝；local-02 暴露 Orca 生命周期缺 history binding，未用 raw 填产品正文；local-03 已通过正文/原会话/双PR但原生 resume 前的旧 usage 快照被测试适配器错误拒绝，等待诊断被10分钟总期限掩盖，已修为精确新 turn/started 前缀处理和即时错误报告；local-04 修复重验通过。remote-mixed-01/capabilities.json 是范围收缩前的只读/前置诊断历史（无远端资源或模型请求），不计本任务当前验收、阻塞或待办。

仅尚未执行的人工操作可登记 roadmap 后推进；实际本机功能或自动验证失败必须修复。CN5 和完整故障矩阵复用 [#37 证据](../candidate-workers/README.md)并已定向复核本次受影响接线；不默认 Full/L2 或模型矩阵。#38 CC5 仍由原记录跟踪，没有宣称通过。
