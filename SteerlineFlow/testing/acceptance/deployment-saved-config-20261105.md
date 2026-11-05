# 保存执行配置后的部署校验与安装恢复（2026-11-05）

用户要求修复保存配置后部署失败，并恢复此次 `C:\SteerlineInstall` 的失败部署。源码为 worktree `C:\Users\lmas1\.codex\worktrees\1d4d\SteerLine`，基于 `c94cc5aa047e3449703c1868e8d8a6628c350616` 加本次未提交修改。普通开发直接实施，未启动 Roadmap；无固定提示词、材料、黄金样本、数据库格式或 Runner 协议变更。

## 原因和修复边界

原实例北京时间 15:06:41 启动，`execution.json` 在 16:10:01 修改，17:00:17 停机检查点仍保留原加载摘要。三个项目的实例及进程身份、stopped/ready=false 均匹配，只有当前配置与历史摘要不同。原失败计时为 `C:\Project\SteerLine\bin\deploy-timings\8d50f125c6c24b0db58af8b829f9a582.json`，暂存包为 `C:\SteerlineInstall\deployments\20261105-090100-0d90d01d`，未进入归档或替换。

具有当前实例匹配停机回执和 stopped 检查点时，核验原宿主/项目实例、有效历史配置摘要、仓库路径/身份/remote/分支和固定 Runner 绑定，不再要求已保存的配置字节与旧进程加载时相同。原摘要保持原值，新宿主读取当前保存的配置；普通离线停止、同构建重启及显式 discard-runs 共用该规则。缺少当前停机回执的中断恢复仍要求摘要完全一致。项目清单、实际锁、原 Run/input 和资源结算仍沿原路径核验；离线停止核验也在完成前重读配置，拒绝准备期间的修改。

回归覆盖执行方案语义修改后核验、同构建重启保留未知资源诊断、完整离线 discard 保留保存后的方案、准备中再次修改配置拒绝，以及不同实例/宿主/仓库/remote/分支/Runner、无摘要、运行中检查点拒绝。原中断恢复的 changed-config、配置在恢复中变化等反例保持。

## 隔离验证

Windows/amd64，Go 1.26.4、Node 24.13.0。独立证据根为 `C:\SteerlineDebug\wt-1d4d-deploy-config`；下列路径相对该根。全部自动测试使用独立夹具，不以安装现场作为测试环境。

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 定向部署回归四根及全部子项 | passed，155.187 秒 | `diagnostics/20261105T091105916Z-469a02fe6ba249de992816dd6f31f444/summary.json` |
| 固定 L1 | passed，准备 0.597 秒、执行 29.194 秒 | `test-results/L1/20261105T091243292Z-d1e0955efd3146278be3c584136f0b9d/summary.json` |
| 固定 L2 | passed，准备 9.246 秒、执行 137.241 秒，cleanup_complete=true | `test-results/L2/20261105T091338074Z-d655a77d2ba64cc69ca3f074282ac9ac/summary.json` |
| L4 go.deployment | passed，执行 443.368 秒；host、Windows localservice 全量，App 10 根及全部子项、原升级排空根 | `test-results/L4/20261105T091449762Z-534edcc336ff410f93702d3f33af9b9c/summary.json`、`managed-go.deployment.json` |
| 生产构建 SteerLine/Web/Runner | passed，32.780 秒 | `diagnostics/build-20261105T091822982Z-7df41412c45c432f90013c75f230d3c8.json` |

`git diff --check` 与全仓文档链接检查通过。定向测试之后仅抽取相同清单比较函数到 application_catalog.go；L1/L2/L4 均使用抽取后的版本。

## 实际安装恢复

部署前经原固定 Runner 的公开离线接口确认 9 个 Run：7 个 stopped，2 个尚可正式 stop，无未解决的 resume 授权。三个项目原任务/依赖数分别为 merge-h-308-code 15/4、SteerLine 0/0、SteerlineTest 5/4；已只读计算任务定义、完整契约和依赖摘要用于恢复后核对。配置含 7 个执行方案、默认 project-default。

全部隔离测试通过后，实际沿修复后的正式入口执行：

```powershell
& C:\Users\lmas1\.codex\worktrees\1d4d\SteerLine\scripts\build-deploy.ps1 -UpdateRunner -DiscardRuns -RunnerRepo C:\Project\SteerRunner -InstallHome C:\SteerlineInstall -NoPause -TimingPath C:\SteerlineDebug\wt-1d4d-deploy-config\diagnostics\deployment-20261105T092306714Z-cfa13b1147704bf198703f4eb77d3d3b.json
```

正式部署 succeeded=true，166.24 秒；stop-and-verify 136.25 秒，正式停止 2 个原 Run，并通过原固定 Runner 的公开证明核实全部 9 个历史 Run resources_settled=true。该模式恢复全部任务为 open、manual/paused，原执行及控制历史只归档，不继续旧运行。部署未重试，也未人工改写停机回执、检查点或原 Run。

最终包为 `C:\SteerlineInstall\deployments\20261105-092318-1bf2828e`；其中 `shutdown-verification.json` 记录 outcome=prepared、source=discarded_runs，`timings.json`、`deployment.json` 及 `startup-07b542949e824ffca2f7bd269debbbf9.log/.json` 保留正式输出。原状态由正式入口归档至 `C:\SteerlineInstall\maintenance\discard-runs-ddd99c1ab7e04e468242ffad2e299b5f`，`discard.json` 为 complete，五项目录切换全部完成。

SteerLine 来源为上述基线加本次未提交修改（source_dirty=true）；Runner 来源 `a66038c89208392c47531e640c84e97728c62424`，version `0.2.0-dev.4`，SHA256 `f065b78b9b305efa9e13611fbb102864cf8f8482256a32362b1828c5fc57fb4b`。

2026-11-05 17:27（Asia/Shanghai）最终只读核验：后端/Web ready，全部三个项目 ready/online、degraded=false，Web HTTP 200。逐项比较归档与新状态：20 个任务的编号/标题/类型/优先级/目标、完整固定契约（含原 ID、摘要及所有引用）和全部 8 条依赖完全相等；所有任务 open、无 claimant，三个项目 manual/paused。三个库 integrity_check=ok、foreign_key_check 无问题，runner_candidates、project_environments、test_generations、roadmaps 全部为零。7 个执行方案、默认 project-default 及 Runner 以外的全部全局配置摘要不变，三个项目配置字节摘要不变；安装的三个 EXE 与正式包/部署记录摘要逐项匹配。

最终核验摘要为 `C:\SteerlineDebug\wt-1d4d-deploy-config\diagnostics\installation-final-20261105T0926.json`，仅含数量、相等性、状态及摘要结论，不含任务正文或数据库。安装原生资源证明保持 Runner 原 scope，不扩大为所有脱离会话后台资源不存在。

额外将原配置和 SQLite 复制到外部诊断目录的操作被自动审批以目的地未获明确授权、数据可能敏感为由拒绝，未执行。后续采用只读摘要比较及产品正式部署入口在安装目录内的原生归档，不另复制安装数据库。

未执行 go.full、真实模型或新 PR 交付；隔离测试不替代实际安装就绪核验。
