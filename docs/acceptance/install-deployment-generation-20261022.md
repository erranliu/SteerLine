# 2026-10-22 安装部署的生成记录接管修复

本次为用户直接授权的安装维护及普通缺陷修复。基线为 `c7f90e04e1d1414386c5c421b3536a7a5fce60c5`，源码包含本次未提交修改。安装现场只用于正式维护；自动回归使用隔离临时 home、Git、SQLite 与公开 Runner CLI fixture。

## 原因与修复

`C:\SteerlineInstall\deployments\20261022-093546-98065de0` 的候选 Runner 接管预检为 compatible=true，但部署报 `SteerlineTest.json: original generation takeover proof is unavailable`。原安装 Runner SHA 为 `72c41ed70d3ce76d9a6c10c5a6338d667bde0c29881502f95d0d2733d49ded2c`，候选为 `b97b6c54d3aa885e2495fba275791e9609da8279c80d5bd45818df08ad1eea57`。历史生成记录又属于更早的构建。

离线重启核验已通过所选候选验证公开报告，却在核对历史生成记录时，使用原安装 pin 再次核对报告中的候选 SHA，混淆了停机/checkpoint 的原身份与兼容报告的验证者身份。

修复后复用已经验证的报告，按生成记录原命名空间、Run/input 或原请求摘要匹配；原安装 pin 仍用于原停机与 checkpoint。未发送和明确拒绝且没有 Run 的生成记录不要求接管。当前宿主失败回执的恢复围栏、实际锁、资源结算及运行中的服务验证保持原要求。

## 自动回归

新增回归位于 `TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite/cross_build_generation_uses_selected_candidate_proof`，包含历史生成、上次常驻 Runner、本次候选三种构建；拒绝原输入、命名空间、候选 SHA、缺失 Run 与只读生成记录五类错误证明，并比较原停机、配置和 SQLite 字节。原 `resources_settled=false` 和诊断保持。

首次补验还发现 `TestDeploymentInterruptedRecovery` 的仓库夹具仍只创建旧 `plan.md`，而共用 seed 已要求 `task.plan.md` 和测试清单；在进入恢复验证前即失败。现复用统一仓库准备器，并为其生成记录补齐公开 Run/input 接管证据，不放宽产品校验。

执行命令：

```powershell
go run ./cmd/test-feedback -- ./internal/app -run '^(TestDeployment|TestTMGenerationTakeoverAndDurableStartupStop|TestGQQueuedGenerationNeedsNoRunnerTakeoverOrStop|TestTakeoverKeepsFrozenCandidatesAndRequiresOriginalRequestDigest)' -count=1 -timeout=240s
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1
```

原始失败复现报告：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261022T095438.171746300Z\summary.json`，准确复现原 generation takeover 报错。夹具失败报告为相邻 `20261022T095548.425366000Z`；其中新增三代构建正例及五项反例均已通过。

最终报告为 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261022T095827.734217100Z\summary.json`：定向 49 项通过、零失败/跳过，complete=true，174.093s wall。脚本路由测试在 Windows PowerShell 5.1 下 exit 0；Markdown 537 份文件、3513 个本地导航链接、零错误；`git diff --check` 通过。该结果仅为所列定向范围，不代表固定完整 L1 或 Full。

## 实际维护

证据目录为 `C:\SteerlineInstall\maintenance\deployment-generation-20261022`。修复后 CLI 使用上述原暂存候选执行只读 `verify-restart`，exit 0，restartable=true、compatible=true、三个项目通过，resources_settled=false。原失败回执保存在 `original-shutdown.json`；完整证明在 `verify-restart.json`。

修复的三份 Go 源码/测试同步至同 HEAD 且原先干净的 `C:\Project\SteerLine`，后续构建入口可继续使用。沿正式命令部署：

```powershell
& C:\Project\SteerLine\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -NoStart -NoPause
```

部署 exit 0，包为 `C:\SteerlineInstall\deployments\20261022-100215-8cb6cf59`；Runner 源码 `b2a05f0684fa670e7ce84e8e9e97dd1c1a3bcb86`，版本 `0.2.0-dev.3-r2`。包内 `shutdown-verification.json` 为 prepared / restart_verification；host_exit/project_closure/writer_exit/compatibility 均 verified，native_resources 保留 unverified，未调用中断恢复或改写原失败回执。

维护证据 `snapshot.py` 通过 SQLite 只读连接，`state-before.json` / `state-installed.json` 证明三个项目数据库在替换前后字节完全相同。安装文件摘要与 deployment.json 一致，执行 profiles 和默认方案保持。随后在独立同用户隐藏进程中执行正式 `start --no-open`，exit 0；等候全部三个项目 ready 且 online，backend/Web ready，degraded=false。`status-after.json` 与 `http-result.json` 保存结果，Web `http://127.0.0.1:18742` 为 HTTP 200。

`state-started.json` 另核对 20 个 Task、20 份契约、原依赖和 dispatch 完全保持；6 个 Candidate、21 份生成记录集合不变，原 Run/input、提交信封、构建 pin、来源材料及 PR 绑定保持，SQLite integrity/foreign-key 通过。原资源未知诊断保存在原停机回执与部署证明中。

未运行 Full、L2/L3、真实模型或真实 PR 验收。正常安装恢复沿既有持久业务授权运行，不把本次维护当作原生执行专项通过。

## 后续重新部署被 Defender 隔离

用户随后重新运行部署，包为 `20261022-101711-676b6e04`。该次部署停止核验仍为 prepared / restart_verification，三个文件已替换，后端 PID 35428 与 Web PID 19816 启动；等待项目 ready 时，Windows Defender 阻止再次调用 `steerline.exe status`。

2026-10-22 18:18 的 Defender 1116/1117 事件明确报告 `Trojan:Win32/Bearfoos.A!ml`（ThreatID 2147731250），隔离 `C:\SteerlineInstall\bin\steerline.exe` 并终止后端。该包记录的 EXE SHA256 为 `3f07ee3883032738629a3ec417b414939102801148a4a260dfd257125dc83bd3`。源码构建 EXE 的后续读取也被拦截，不能据构建来源直接判定误报。

执行 `Update-MpSignature` 后，检测库仍为 `1.461.145.0`（更新时间 07:33:44）；`Start-MpScan -ScanType CustomScan -ScanPath C:\Project\SteerLine\bin\steerline.exe` 完成后仍报告同一威胁。检测、保护版本和威胁信息保存于维护证据子目录 `defender-20261022`。本次未修改防护配置、恢复被隔离文件或通过变换构建绕过检测；未将样本上传外部服务。

当前该次重新部署未恢复后端；Web HTTP 仍为 200，安装 Runner 进程仍在。不能用静态 Web 可访问代表三个项目在线。本次仅完成诊断和官方更新/扫描，等待样本复核处理，不将此前成功维护的 ready 结果用于当前状态。

## 2026-10-23 构建变化与检测时间对比

用户追问原先构建未拦截的原因。核对两份部署清单和此前实际摘要验证结果：`20261022-100215-8cb6cf59`（北京时间 18:02:37）与 `20261022-101711-676b6e04`（18:17:38）的三份 EXE SHA256 全部一致。两次 CLI 均为 `3f07ee3883032738629a3ec417b414939102801148a4a260dfd257125dc83bd3`；SteerLine 源码均为 c7f90e04 + 本次修改，Runner 源码均为 b2a05f06。

18:03:42 的原始 `status-after.json` 证明第一包当时 backend/Web/三个项目 ready；18:03:55 的 HTTP 证据为 200。Defender 首次记录该安装 EXE 的检测是 18:18:02，随后于 18:18:39 隔离。18:22 对源码 EXE 的检测类型显示“快速路径”；次日 09:35:31 又拦截 Go 临时 `a.out.exe`。因此可排除“18:17 重建产生了不同 EXE”这一解释，但不能排除较早的新版本内容提供了模型触发特征。

相对前一个已部署的 8e5babcf，c7f90e04 的生产 Go 修改是查询/命令端口精简；本次额外修复仅调整历史生成记录的离线接管匹配。`go.mod`、`go.sum`、`build.ps1`、部署脚本均未在这两个提交间变化，本次也未修改它们。定向回归元数据记录 Go 1.26.4，与次日失败的工具链一致。

Defender 本地签名为 1.461.145.0，最后更新时间在 10 月 22 日 07:33:44；17:50–18:25 的相关事件中没有签名/引擎更新记录。云保护当前启用。延迟分析、云端判定或检测缓存/扫描上下文变化可以解释相同文件先运行、后被拦截，但现有日志没有提供具体模型特征、字节或源码行，不能认定上述任一机制为已证实原因，也不能据此确认误报。

比较摘要保存在 `defender-20261022/build-comparison.json`。本轮只读核对源码、清单、历史状态和安全事件；未重新构建、执行或恢复被检出样本，未修改排除项。当前非管理员查询无法读取实际排除列表，故未用当前扫描结果伪造旧/新版本的同条件比较。

## 2026-10-23 同步 main 与提交前验证

按用户要求保留本次修复，先将六份原文件完整备份至 `C:\Users\lmas1\AppData\Local\Temp\steerline-preserve-fixes-20261023-112112`，再获取远端并将 main 从 `c7f90e04` 快进至 `75f8981b`。恢复修改后仅 `docs/testing.md` 存在内容冲突：保留 main 新的测试分级、固定 L1/L2 命令和文档结构，将本次部署定向回归链接放在“环境与证据”中。三份 Go 源码/测试及原维护记录完整保留。

本轮使用隔离测试现场验证同步后的源码，不访问安装目录，不重新执行历史检测样本。遵循同步后的指南，运行固定 L1、固定受控 L2，另补原部署/生成接管定向回归与部署脚本专项；不运行 L3、L4 全集或真实模型/真实 PR。

| 入口 | 结果与证据 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 32 项通过，零失败/跳过，各组 complete=true；准备及执行合计 9.927s。报告：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261023T032154.680988700Z\summary.json`。 |
| `go run ./cmd/test-feedback -level=L2` | `smoke.delivery` 通过，coverage=full-L2，用例 complete=true、零失败/跳过，整体 91.505s（含准备、执行和清理）。报告：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261023T032216.073488500Z\summary.json`。 |
| 上文“自动回归”的原部署/生成接管定向命令 | 49 项通过，零失败/跳过，complete=true，164.721s wall；包含三代构建正例及五类错误证明反例。报告：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261023T032257.067345000Z\summary.json`。 |
| `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` | exit 0，单次 Go 准备调用、调用号/包绑定、固定 Runner 复用和接管预检门禁通过。 |
| `python scripts/check_doc_links.py`、`git diff --check` 及 Go 格式检查 | Markdown 543 份文件、3507 个本地导航链接、零错误；无空白错误或未解决冲突，三份 Go 文件按 LF 规范化后符合 gofmt。 |

本轮证据为 `75f8981b` 加本次修改的工作树结果；不继承原安装维护的在线结论，也不代表 `go.full` 或真实原生执行验收通过。
