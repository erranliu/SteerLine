# #39 多候选本机与远端真机验收（实施中，未完整验收）

起点/开工最新 main：`a8ff0f01d38299d77ecc9cd4463b78a1286e1bec`（PR #208 真实合并）；独立托管 worktree：`C:\Users\lmas1\.codex\worktrees\d831\SteerLine`；分支：`codex/candidate-native-acceptance-39`。#38 CC5 仍待用户人工补验；限定合入例外不扩大到 CN1–CN8。

唯一运行入口：`go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out <新的空证据目录>`。远端/混合使用各自场景 ID。设置 `STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 指向绝对 manifest 路径；结构如下，source_commit 替换为本次实际固定 HEAD：

```json
{
  "version": 1,
  "source_commit": "<40位实际实现commit>",
  "repository": "erranliu/SteerlineTest",
  "profiles": [
    {"id":"candidate-orca","chain":"orca-codex"},
    {"id":"candidate-local","chain":"local-codex"}
  ],
  "scenarios": {"candidates.local":["candidate-orca","candidate-local"]}
}
```

当前远端产品 `orca_remote_worker.CheckPolicy` 总是拒绝缺少目标 worker 权限回执的准入；连接可用或认证声明不能代替该权限证明。远端 profile 的 chain 为 orca-remote，remote 使用当前 ProfileConfig：connection_id/environment_id/repository_selector/coordinator_terminal/expected_codex_version/poll_interval/capability_report_version/remote_cancel_available（固定 false）。同一场景恰好选择两个不同 profile；remote 要两份远端，mixed 要本机与远端各一份。没有真实目标身份的模板不能成为准入证据。拒绝在创建项目、PR、worker 或模型请求之前发生。

本机只复制既有 auth.json 到隔离 home，不复制用户配置或任务库。新随机 base 分支位于 erranliu/SteerlineTest；worker 独立 commit/push/create，操作者按期望 head 合并。产品查询规范数据库中的 Task/WorkflowRun/Execution/Decision/Operation/Delivery；driver 读取规范 Conversation/完整 Body，native logs 仅为诊断/usage 证据。清理只关闭本次 base 的 open PR、删除本次分支、停止 owned 进程及本次 Orca 资源并删除 scratch，保留 PR 历史与证据。

实际结果与剩余项将在现场完成后追加。当前注册/定向编译不代表 CN 通过。CN5/故障/晚到受控边界复用 [#37 证据](../candidate-workers/README.md)（生产候选接线未修改）；远端/混合原生故障、合并前放弃和替代等仍需真实准入及现场验证。保持 draft PR 和活动契约入口。
