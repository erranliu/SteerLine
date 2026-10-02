# Rebase #187 后完整 L2

2026-10-02 18:19:59–18:27:20（Asia/Shanghai），完整 L2 **PASS**，coverage=full-L2、cleanup_complete=true。候选 `7c2449a` 包含最新 main `5bbbfcd`（#187 host/source 隔离）。#180 分支先 rebase，#188 两笔增量再接到新父分支，保留分层 PR 范围。

唯一冲突在原终端检测测试：保留主干的 `advanceWorkerPreparation` 新入口，使用 codec 的 AgentBound 验证原终端，不恢复旧接口。未新增兼容/回退路径。

| 场景 | 结果 | 用时 | 原生累计 tokens | 测试 PR |
|---|---|---|---|---|
| smoke.delivery | pass / cleanup complete | 64.83s | 无模型请求 | controlled |
| smoke.local-codex | pass / cleanup complete | 115.88s | 96,599 | [#39](https://github.com/erranliu/SteerlineTest/pull/39)，MERGED |
| smoke.orca-codex | pass / cleanup complete | 247.52s | 249,134 | [#40](https://github.com/erranliu/SteerlineTest/pull/40)，MERGED |

命令：`go run ./cmd/test-feedback -level=L2 -out=<fresh>`。先通过无 token 门槛，再进入双真实链路，full access，gpt-6-luna / medium。合计 **345,733 tokens**，含 cached input，取每条链路最终累计样本、不重复相加。本轮没有失败重试。Orca readiness/business 均 succeeded，准备 verified/ready；交付与清理 usage 不增加。额外 `git ls-remote --heads` 检查两个 base、两个 head 均零匹配。

rebase 后无 token 定向验证：

- `go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1`：通过。
- `go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1`：通过。
- `go test ./internal/steerline -run '^(TestWorkerEnvironmentWaitsForOriginalAgentDetection|TestOrcaRestartPreflightRetainsOriginalPolicyRoots|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace)$' -count=1`：通过（91.991s），覆盖冲突中的新准备入口、原终端/roots、空线程首回合和同会话反馈。

原始报告：`C:/Users/lmas1/AppData/Local/Temp/sl-l2-rebase187-20261002-181956`。本目录保存 summary、scenario 和 usage 数值；认证、数据库、完整模型输入保留在隔离现场并由清理回收，不复制进仓库。

本次未重跑 L3/full；此前 L3 记录只对应原候选，不作为本次最新 main 的 L3 验收。
