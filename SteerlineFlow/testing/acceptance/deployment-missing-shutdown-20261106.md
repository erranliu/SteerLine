# 放弃旧运行部署时缺少停机回执（2026-11-06）

## 问题与修复

用户提供的正式部署在 `deploy.stop-and-verify` 失败：`C:\SteerlineInstall\application-api\shutdown.json` 不存在。只读检查确认原后端、Web 均 exited，原固定 Runner 的公开 `service status` 为 not_running；原安装程序仍保留，未补写停机回执。

`discard-runs` 原来无条件要求当前宿主的停机回执。现仅对回执不存在的中断现场增加离线证明：原宿主/Web 精确退出及租约、配置中的全部项目与 state 目录完整集合、原宿主检查点与仓库/固定 Runner 绑定、实际仓库和运行目录锁。检查点可以尚未写入 stopped；保存给下次启动的执行方案仍可与原冻结摘要不同。随后沿原固定 Runner 的公开冷查询、持久 stop 信封、原请求核实及严格资源结算流程执行。缺失回执用 `discard.interrupted_host` 和 `stopped` 新核验事实记录，不伪造 `original-shutdown.json`。

已有损坏、过期或本地关闭不明的回执继续拒绝；普通 update/replace/restart 的判定不变。不兼容旧状态、不新增迁移或原宿主自动重启。

源码基点 `8349d9e04ddfd656b68022f7379ec2272c548fdf` 加本工作区修改。证据根 `C:\SteerlineDebug\wt-5a39-missing-shutdown\`，各次输出独立，原始报告记录实际 dirty 文件。

## 已执行验证

| 范围 | 命令与结果 | 证据（相对证据根） |
| --- | --- | --- |
| 定向 Go | `go run ./cmd/test-feedback -out <证据根>\diagnostics\discard-01 -- ./internal/app -run '^TestDeploymentDiscardRuns$' -count=1`，通过，165.172 秒 | `diagnostics/discard-01/summary.json`、`go-test.jsonl` |
| L1 | `go run ./cmd/test-feedback -level=L1 -out <证据根>\test-results\L1\run-01`，通过 | `test-results/L1/run-01/summary.json` |
| L2 | `go run ./cmd/test-feedback -level=L2 -out <证据根>\test-results\L2\run-01`，通过 | `test-results/L2/run-01/summary.json` |
| L4 部署专项 | `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <证据根>\test-results\L4\deployment-01`，complete/passed=true，396.146 秒 | `test-results/L4/deployment-01/summary.json` 及四组原始日志 |
| 清单 | `go test ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest\|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1`，通过 | 本次命令输出 |

定向测试覆盖：已有回执、已保存执行方案、准备期间配置变化、固定 Runner 变化、缺失回执且已有 stopped 检查点、缺失回执且检查点仍 ready、错误宿主/仓库/Runner 资源摘要、运行目录锁占用、损坏回执和过期回执。成功核对原库已归档、新库 manual/paused、新空间通过正式升级核验；中断场景额外核对原宿主与项目证明及无伪造回执。原停止协议的未知控制、恢复授权、资源未结算等拒绝测试保留。

首次未提权调用在创建项目规定的外部报告目录时遇到 Access is denied，未执行 Go 测试；后续获准使用相同尚不存在的目录实际执行。无失败测试自动重试。

## 正式安装维护

隔离部署专项通过后，从本工作区执行：

```powershell
.\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -DiscardRuns -NoPause -TimingPath C:\SteerlineDebug\wt-5a39-missing-shutdown\diagnostics\installation-01\deployment-timings.json
```

正式部署 succeeded=true，总耗时 117.85 秒；原失败阶段 stop-and-verify 通过，75.72 秒。安装包为 `C:\SteerlineInstall\deployments\20261106-031502-f56c5da8\`，含本次 `shutdown-verification.json`、候选升级证明、三个 EXE 摘要及启动回执。Runner 源码 `efb024e103c362645ffa306d8b5459ff9b688ec7`，使用其正式构建入口。

原状态归档到 `C:\SteerlineInstall\maintenance\discard-runs-57790c2b43784a6baecfac705047ea93\`：phase=complete，全部移动 done=true，原 6 个 Run 公开资源证明均 resources_settled=true。原 Run 已停止，本次无须发出新的 Run stop（stops=[]）。原缺失回执诊断和精确 interrupted_host 保留于 discard.json，未补写原 shutdown.json。

安装后从正式 status 和经当前身份核对的 application v2 只读 snapshot/graph 核实：后端、Web 和 3 个项目全部 ready；SteerLine 0、SteerlineTest 5、merge-h-308-code 15 个任务，全部 open、manual/paused。项目配置文件 SHA256、默认执行方案及完整 profiles 的规范化摘要与部署前一致。

证据位于 `diagnostics/installation-01/`：`deployment-transcript.log`、`deployment-timings.json`、`before-status.json`、`before-runner-status.json`、部署前配置摘要、`after-status.json`、各项目完整 graph 和 `final-verification.json`。构建/安装/ready 均由正式脚本完成，未手改安装库、停机回执或原 Run。
