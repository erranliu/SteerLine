# 2026-10-26 启动性能优化

用户授权在不减少产品能力的前提下消除重复检查与 Git 调用、优化串行恢复，完成隔离测试后部署到 `C:\SteerlineInstall`，核实三个项目并比较耗时。本次为普通开发及实际安装维护，不按 roadmap 调度，不以安装现场执行自动回归。

源码基线为 `57623186b63c16be2a5748665e63d52ae615adbf`，测试与部署时为本工作区未提交增量；本记录随实现提交，原报告和安装构建记录保留当时的 dirty 身份。证据根为 `C:\SteerlineDebug\wt-704c-startup`；当前安装与之前的只读诊断另有 `C:\SteerlineDebug\install-startup-20261026`。不修改数据库格式、执行配置、Runner 固定构建或启动期限。

## 实现与边界

- [启动查询作用域](../../../internal/app/startup_queries.go)仅复用一个项目、一个准备阶段的仓库身份、remote 与 worktree 读取；键包含实际仓库、Git 命令和参数。成功取得项目锁后重新建立作用域。阶段结束时同时停用所有 Config 副本持有的作用域，后续运行命令继续实时查询；取消信号贯穿原 Background 调用。
- [项目准备](../../../internal/app/project_runtime.go)复用恢复阶段从冻结字节解码的 Config，保留锁前及锁后身份复核、路径/链接、数据库 schema/格式/authority 等准入检查。初始化后不让查询缓存进入正常 actor 的生命周期。
- [恢复准备](../../../internal/app/runner_takeover.go)对已通过冲突普查的独立项目最多并发两个本地配置/数据库准备；统一接管证明、generation/candidate 停止请求与共享 Runner 启动仍按原序处理。worker 仍在持久停止请求受理后恢复。任何失败均关闭已经打开的 store 并释放项目锁。
- 增加恢复准备、Runner 服务/能力、项目准备各阶段与项目 ready 的耗时日志；不通过延长期限掩盖开销。未移除自动恢复、多项目、历史检查、资源围栏或执行能力。

## 隔离性能对比

`internal/app/startup_timing_test.go` 使用 `startup_timing` 构建标签；创建三份独立 clone/common-dir/数据库，无安装数据、真实模型或远端提交。使用同一测试和等价夹具，先后运行原实现与优化实现；不包含编译、夹具建立及 Runner 进程本身的启动时间。

原实现通过 `baseline-overlay.json` 将修改的生产文件映射到基线 Git 内容，并将新增实现/专项测试映射为空包；没有回滚工作区。两次运行分别保留 JSONL，不覆盖失败记录。缓存按机器现状使用，机器另有并行负载；以下为单次配对测量，不宣称统计分位数或完整产品启动耗时。

| 阶段 | 原实现 | 优化后 | 减少 |
| --- | --- | --- | --- |
| 三项目恢复准备 | 17.6435s | 10.4006s | 41.1% |
| 随后的三项目准备 | 39.4621s | 10.8514s | 72.5% |
| 两阶段合计 | 57.1062s | 21.2520s | 62.8% |

原始文件为证据根下 `baseline-recovery-timing-02.jsonl` 与 `optimized-recovery-timing.jsonl`。另一次只测项目准备的独立对比为 42.1462s → 8.9407s，见 `baseline-timing.jsonl` 与 `optimized-timing.jsonl`。

复现命令：

```powershell
go test -overlay C:\SteerlineDebug\wt-704c-startup\baseline-overlay.json -tags=startup_timing ./internal/app -run '^TestStartupRecoveryTiming$' -count=1 -timeout=8m -json
go test -tags=startup_timing ./internal/app -run '^TestStartupRecoveryTiming$' -count=1 -timeout=8m -json
```

## 测试结果

以下路径均相对证据根，原命令和源码/环境见对应 `summary.json`。

| 范围 | 结果 | 原报告 |
| --- | --- | --- |
| L1 最小集合 | 通过；准备 0.81s、执行 18.49s | `test-results/L1/run-01/summary.json` |
| L2 受控交付冒烟 | 通过；准备 28.36s、执行 148.72s，包含清理 | `test-results/L2/run-01/summary.json` |
| 启动、共享 Runner、持久取消、generation 接管与缓存准入 | 24 项通过、零跳过；205.55s wall | `diagnostics/owner-02/summary.json` |
| home 派生、worktree、配置、数据库初始化/绑定及硬链接拒绝 | 6 项通过、零跳过；71.69s wall | `diagnostics/paths-01/summary.json` |
| L3 正常服务生命周期、双项目共享 Runner、待合并重启 | 三场景通过，含原请求/Run 绑定与清理；准备 23.53s、执行 426.74s | `test-results/functional-01/L3/summary.json` |

新增专项核实重复读取只启动必要 Git 进程、锁边界重新查询、Config 副本退出启动作用域、取消、仓库/remote 变化、数据库硬链接拒绝及并发准备失败的全体锁释放。

L3 通过 `scripts/test-runtime-isolation.ps1 -Level L3 -Suite runtime,recovery -Scenario runtime.service-lifecycle,runtime.shared-runner-projects,recovery.waiting-merge-restart -OutDir C:\SteerlineDebug\wt-704c-startup\test-results\functional-01` 的既有独立进程入口执行，两个 suite 使用独立资源；各场景原始结果及清理记录保留于 summary 指向的目录。

首次 owner 回归中，宿主回调复制的 Config 仍引用已取消的准备作用域，导致持久取消测试失败；修正作用域结束对全部副本生效后整组复验通过。首次恢复计时的构建因诊断文件未使用 import 失败；修正后以新的 `-02` 文件保存实际执行。原失败分别保留在 `diagnostics/owner-01/` 与 `baseline-recovery-timing.jsonl`，不作为通过证据。

## 实际安装维护

部署前证据为 `before-status.json`、`before-instance.json`、`before-backend.log`、`before-deployment.json` 及 `business-before.json`。原后端/Web ready，但三个项目全部因准备超时 failed，`degraded=true`。

第一次正式部署未替换文件：原三个项目在准备阶段失败，停机回执虽然确认本地关闭和锁释放，但没有项目实例 ID，`verify-restart` 正确拒绝。未放宽 verifier、删除回执或离线绕过替换；使用原安装程序正常启动至三个项目 ready，再通过正式部署取得完整停机/同构建重启证明。首次失败包为 `C:\SteerlineInstall\deployments\20261026-034711-b930244c`，失败证据为 `blocked-shutdown.json`、`blocked-verify-restart.stderr.txt` 和 `deployment-transcript-failed-01.log`。

成功部署包为 `C:\SteerlineInstall\deployments\20261026-035257-af6f0549`，沿正式 `scripts/build-deploy.ps1 -InstallHome C:\SteerlineInstall -NoStart -NoPause` 执行，随后使用安装版 `start --no-open`。通过既有独立进程上下文运行，维护脚本退出后再次读取正式 status 核实常驻。三份安装 EXE 摘要与部署记录匹配，Runner SHA256 仍为 `4ef97155f1140fb7ce383f4c9515c603132f2674876e1dbeb7ed73331eaf32b2`。

### 同一安装现场的完整启动对比

两次均从服务已停止后调用安装版 `start --no-open`，计入命令自身及后台恢复时间，以公开 status 的项目 ready/online 判定。系统文件缓存不清空，轮询间隔 500ms 加 status 命令开销，机器负载非严格恒定，属于单次实测；不将它与上面的隔离阶段计时混用。

| 项目 / 范围 | 原安装版 | 优化版 |
| --- | --- | --- |
| SteerLine | 111.29s | 46.46s |
| SteerlineTest | 113.25s | 50.94s |
| merge-h-308-code | 119.09s | 63.54s |
| 全部项目 ready | 119.10s | 63.58s |

完整启动减少 55.52s，约 46.6%。原版于北京时间 11:49:02 开始，优化版于 11:53:57 开始。原始计时为 `before-installed-timing.json` 和 `installed-start-timing.json`，逐次状态为 `before-installed-transitions.json` 和 `installed-start-transitions.json`，后端日志为 `before-installed-backend.log` 和 `after-backend.log`。优化版日志中恢复准备 10.58s，Runner 服务启动 7.41s，含版本/恢复/能力的 Runner 准备共 26.75s，项目 native storage 准备约 9.72–12.07s；后续执行绑定恢复也计入完整 ready 时间。

最终 `final-status.json` 确认 backend/Web ready、三项目 ready 且 online、`degraded=false`；Web HTTP 200。`business-verification.json` 全部通过：三个项目的任务完整行、契约、依赖、authority/dispatch 设置，以及原候选 ID/Run/输入摘要/submit 信封/Runner pin 均保持；数据库 integrity 和 foreign-key 检查通过，完整 execution.json 配置值也保持。

原 Runner 的历史原生资源结算仍为 unverified，保留在成功部署包的 `shutdown-verification.json`，不将其改记为已结算；原宿主退出、项目关闭与锁释放、Runner writer 退出均为 verified。本次确认启动、恢复绑定及上述状态保持，不宣称解决原生远端资源结算问题。
