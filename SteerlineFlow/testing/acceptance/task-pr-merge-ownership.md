# SteerLine 任务 PR 合入核验验收

对应[Spec](../../spec/task-pr-merge-ownership.spec.md)和[Plan](../../plans/task-pr-merge-ownership.plan.md)。此文记录配置接线探针以及后续产品验收。

## 冻结配置探针 PMO-P01

2026-11-04，Windows，SteerLine 工作树 `34db893474c70d22d173bcdc91e6f914d17366da`。自动定向诊断通过，耗时 7.713 秒。以临时 Go overlay 运行，探针与输出存于 `C:\SteerlineDebug\pr-merge-ownership\diagnostics\20261104-093522-1bbff813`。执行命令为：

```powershell
$root = 'C:/SteerlineDebug/pr-merge-ownership/diagnostics/20261104-093522-1bbff813'
go run ./cmd/test-feedback -out "$root/report" -- ./internal/app -overlay "$root/overlay.json" -run '^TestFrozenTaskPRQueryProbe$' -count=1 -v
```

证据入口为原运行目录下的 `report/summary.json` 和 `report/go-test.jsonl`。重跑使用独立空输出目录。

- command 从现有 `Config.runnerPublication` 序列化出的冻结字段读取 executable、三个 argv 占位符和认证引用。替换当前配置后，SteerLine 的受管真实子进程仍收到原 PR URL、编号和仓库，经原环境引用读取合成凭证并返回原 PR 的合入时间和 commit。清除原引用后同一子进程返回失败。
- Gitea 从同一冻结结构取原实例、仓库和认证引用。SteerLine 现有 HTTP 传输向本机夹具发送了原目标的 GET 和正确 Authorization，返回原 PR 的合入事实。清除原凭证后，调用在发出 HTTP 前失败。
- 探针覆盖配置、凭证引用与传输可达性。生产查询归一化、事务接线、实际安装环境的 command 路径与认证、真实 Gitea 实例资格待验证。探针使用合成凭证与回环服务。

## 实施验证

2026-11-04，SteerLine 工作树以 `34db8934` 为基线。核对远端 main `3996bf20`，本项涉及的交付、废弃、provider 和受控夹具基线一致。Runner 对照构建为 `ee1619b4`。

受控 Runner 夹具显式拒绝 `--refresh-pr`。SteerLine 通过固定的命令查询取证，等待合入、任务关闭和停止仍走正式入口。SQLite 回归还覆盖旧 Run 投影显示 open 但本次平台证据已合入、陈旧与错误 head 证据、废弃重放、事务回滚及停止竞争。

| ID | 方式和环境 | 实际结果及报告 |
| --- | --- | --- |
| PMO-A01 | 自动，Windows isolated/controlled | `TestTaskPRQueryUsesFrozenProvider` 通过。真实命令子进程按 PATH、冻结 argv 和认证引用启动，错误输出中的合成凭证未进入诊断。回环 Gitea 验证原目标、Authorization、缺凭证和拒绝重定向。定向报告 `C:/SteerlineDebug/pr-merge-ownership/diagnostics/1793786247603-owner/summary.json`。 |
| PMO-A02 | 自动，Windows controlled | `l1.minimum`、`smoke.delivery` 通过。待合入重启首轮和最终分层收口后的复验均通过。L1/L2 报告位于 `C:/SteerlineDebug/pr-merge-ownership/test-results/L1/1793786247603/summary.json` 和 `C:/SteerlineDebug/pr-merge-ownership/test-results/L2/1793786247603/summary.json`。最终 L3 报告位于 `C:/SteerlineDebug/pr-merge-ownership/test-results/L3/1793786247603/summary.json`。首轮重启报告位于 `C:/SteerlineDebug/pr-merge-ownership/test-results/L3/1793786046062/summary.json`。 |
| PMO-A03 | 自动，Windows isolated | 定向交付/废弃、SQLite 全 owner `go.storage` 通过。L4 报告 `C:/SteerlineDebug/pr-merge-ownership/test-results/L4/1793786247603/summary.json`。 |
| PMO-A04 | 自动，Windows controlled | 未修改的 Runner `TestSRPublicationIdentity` 和 `TestSRPublicationBeforeWorkflowReady` 通过，包含独立刷新和早期原 PR 绑定。结果 `C:/Users/lmas1/.codex/visualizations/2026/11/04/01a1a4b1-15cf-7811-8de5-08acc817ac8f/SteerRunner-runtime-file-probe/.evidence/task-pr-merge-ownership-1793786046062/result.json`。 |

## 失败对照与未验范围

- 首轮构建缺少 `time` 导入，补齐后重跑。首轮 L1 指出纯绑定校验和查询类型放错层，改用现有端口层后 L1 通过。原报告分别在 `C:/SteerlineDebug/pr-merge-ownership/test-results/L2/1793785942965/summary.json` 和 `C:/SteerlineDebug/pr-merge-ownership/test-results/L1/1793786046062/summary.json`。
- 广泛 Runner 定向中的 `TestRunnerPresetFreeze` 有两个重启子例失败，均因 fixture 未公布所选目标的结果文件能力。用未修改的 HEAD Go 源码 overlay 重跑，同两个子例以同一错误失败。变更前后证据分别在 `C:/SteerlineDebug/pr-merge-ownership/diagnostics/1793786247603-baseline/report/summary.json` 和 `C:/SteerlineDebug/pr-merge-ownership/diagnostics/1793786046062-owner/summary.json`。
- 正式安装环境、私有 Gitea 真实认证和项目特有查询命令仍待实测。本次通过范围为隔离 owner、受控产品流程和未修改 Runner 的受控发布。

## 提交前主干复验

以 main `3996bf20` 为 PR 基线，固定 `l1.minimum` 和 `smoke.delivery` 均通过。报告位于 `C:/SteerlineDebug/pr-merge-ownership/test-results/L1/1793787121251/summary.json` 和 `C:/SteerlineDebug/pr-merge-ownership/test-results/L2/1793787121251/summary.json`。
