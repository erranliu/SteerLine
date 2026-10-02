# Rebase #185 后完整 smoke 重跑

2026-10-02，代码 HEAD `a18d6e1`，包含 main/PR #185 的 `84097f3`。用户要求重跑 smoke，执行默认完整 L2：

```text
go run ./cmd/test-feedback -level=L2 -out=<新目录>
```

原输出：`C:/Users/lmas1/AppData/Local/Temp/sl-smoke-rebase185-20261002-132533`。

## 结果

完整 L2 **failed**。本目录保留 summary 和三个场景元数据；不能用此前 rebase 前的成功记录替代此次失败。

| 场景 | 结果 | 耗时 | 观察 |
|---|---|---:|---|
| smoke.delivery | pass | 73.73 s | 无 token 门槛及清理通过 |
| smoke.local-codex | fail | 607.08 s | 等待 waiting_pr_merge 超时；已建立原生 app-server 进程和绑定，start_worker_turn 为 in_flight，未取得 native turn ID，未创建 PR |
| smoke.orca-codex | fail | 67.24 s | 模型启动前 identity_mismatch：original Orca agent terminal or agent has changed；未创建 PR |

三个场景 cleanup_complete 均为 true；结束后未发现本次测试/helper 或其本地 app-server 进程。测试仓库没有新增 PR，最新仍为此前的 #33。

本轮没有取得可验证的原生 usage。Orca 项目未产生 sessions 目录，本地回合未建立 native turn；缺失报告不视为零计费，不推断账单。

## 当前定位边界

本地已确认的 prepare_worker_session、workspace、worker_input_value 操作并不证明 start_worker_turn 完成；当前停在新 lifecycle 的原生启动阶段。Orca 的阻断来自原终端/agent 身份核验。两处需要继续对照 lifecycle adapter 的启动/inspect 与原资源绑定，尚未确认最终根因，本轮未修改实现或绕过身份/usage 校验。

此次重跑没有执行 full 或 L3，没有自动重发模型输入。PR 上的真实 L2/L3 历史成功只对应 rebase 前代码；当前最新完整 L2 为失败。
