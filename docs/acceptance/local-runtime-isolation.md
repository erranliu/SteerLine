# 任务 #83 验收规划：本机安装与调试路径隔离

唯一任务归属：#83。来源：[Spec](../spec/local-runtime-isolation.spec.md) / [Plan](../spec/local-runtime-isolation.plan.md)。本文件是该任务专属验收文档，不覆盖其他任务；验收流程见[入口](README.md)。

状态：独立复审 round 1 发现的 P1/P2 已修复。实现候选 `066a5985677cccb4fdb4650bf669ddf15050de90` 在 Windows/amd64 上通过 LI1-A–LI4-A 自动验收；报告绑定 clean source commit，见下方最终记录。PR #247 保持打开且未合并；按当前执行要求，任务仍为执行中，等待后续独立复审与真实合并核验。无本任务人工验收项。固定 Spec/Plan commit 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。实现 PR：[PR #247](https://github.com/erranliu/SteerLine/pull/247)。本文件保留旧证据、复审发现及局部修复事实。

## 自动验收

以下是源场景的全部自动子项，均由 Plan 映射至 `runtime.installed-debug-isolation` 受控场景；实际结果和证据位置逐项登记于下表及执行记录。

| 验收项 | 方法 | 规划行为与通过条件 | 状态 / 结果 |
| --- | --- | --- | --- |
| LI1-A | 自动 | 两个独立测试 clone、不同 home 的安装版与调试版同时运行；配置、状态、日志、服务发现和写命令不串线 | 已验收：同一 controlled 场景分别等待合并和完成，项目/数据库身份独立，状态与写入未串线；清理完成 |
| LI2-A | 自动 | 调试 build、停启及入口重复生成不改安装 exe 摘要、原服务实例或任务事实；调试服务/控制台/CLI 及子进程使用实际调试 exe/home | 已验收：实际源码 exe 用于 CLI、service-window batch 子进程及 ConPTY console；home/bin 无 exe 副本；安装 exe 摘要和服务实例保持，调试同构建重启后任务事实保持 |
| LI3-A | 自动 | 缺少调试路径、home 重叠、显式状态/工作区越出调试 home 拒绝；同仓库跨 home 第二写者被原锁拒绝；误连实例的写命令仍被拒绝 | 已验收：8 项 P1 junction/hardlink/正常路径检查通过；重叠、越界、缺少 source exe、运行中重建、stale instance/config 写入及跨 home 锁均按预期拒绝 |
| LI4-A | 自动 | Windows 中文/空格路径及无关 cwd 可启动并显示准确身份；维护说明区分中心停服、外部执行结算、同构建重启和人工更新 | 已验收：中文/空格路径与无关 cwd 下 console、service 和 CLI 显示实际 exe/home/config/source identity；维护步骤见[本机安装与源码调试](../runtime-install-debug.md) |

环境：Windows/amd64 controlled；两个独立 Git clone/home、真实 SteerLine 进程、SQLite、Git 和受控 worker/provider；不发模型请求。每次输出目录独立，服务和临时资源由场景登记并清理。

## 2026-10-08 最终候选自动验收

精确验证候选：commit `066a5985677cccb4fdb4650bf669ddf15050de90`，tree `98856831907b85a37ba8b5f3364726b0659de358`，Windows/amd64，runner 记录 `clean (this run's logs and summary excluded)`；base/main 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。本记录和 execution-map 的后续提交只同步文档，不改变该实现/测试候选。报告和原始日志保留在用户主机的临时目录。

| 自动验收 | 实际命令 / 结果 | 证据 |
| --- | --- | --- |
| LI1-A–LI4-A | 构建 runner：`go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\test-feedback.exe ./cmd/test-feedback`；执行：`& 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\test-feedback.exe' '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\runtime'`。结果：`passed=true`、`cleanup_complete=true`、零 skip，223.9 秒。安装与调试 source identity 均为上述 commit、`dirty=false`；console transcript 记录源码 exe、调试 home/config，service-window batch 启动实际源码 exe，CLI 与子进程身份一致。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\runtime\summary.json`、`e2e-1.jsonl`、`scenarios\runtime.installed-debug-isolation\scenario.json`、`runtime-identities.json`、`resources.json`、`debug-runtime-console.log` 及两侧命令/快照日志 |
| LI2-A | 安装 exe SHA-256 为 `a9a14f1c758fd3dde57f65ee47be03c2bc49b31d1673642f97732e4d174debf9`，build 前后及调试 rebuild 后不变；安装服务实例 ID 保持 `38de188e6c1bb02d93a13498535feaff`。调试 exe SHA-256 为 `5cb6d93c1448da8f7beeb0a9b8d5e2fb2f384f30aab3ec36e2c52f962ea1d0a7`，重建前后相同；同构建重启服务实例从 `fa61775482403d69fe0007fb79b0a12d` 更新为 `535e4948ceedd6beddb564bc38d8afe0`，任务与 Delivery 保持。 | 同上；build SHA-256 也登记在 L3 `summary.json` |
| LI3-A / P1 回归 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-debug-runtime-isolation.ps1 -Out C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\p1-alias`；8 项通过：2 junction、5 hardlink、重复 launchers 的普通路径，安装侧哨兵快照保持且 debug home 未复制 exe。L3 同时通过 home/config/path 越界、stale instance/config、source exe 缺失和跨 home 锁拒绝。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\p1-alias\summary.json` 及各 case 日志；L3 的 `runtime-identities.json` 与拒绝日志 |
| ConPTY helper | `go test -tags=e2e ./tests/e2e/internal/testenv -run '^TestPseudoConsoleCapturesInteractiveChild$' -count=1 -v`；通过，实际 console 子进程 marker 从 ConPTY transcript 捕获。 | 终端结果；实际 TUI transcript 保存在 L3 证据目录 |
| 共享回归 | 构建 runner：`go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\test-feedback.exe ./cmd/test-feedback`。执行该 runner 的 `-level=L1 -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\l1`；L1 固定 quick、3 workers，`passed=true`、`complete=true`，72.3 秒。再执行 `-level=L3 -suite=delivery -env=controlled -scenario=delivery.basic -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\delivery-basic`；`passed=true`、清理完成，72.4 秒。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\l1\summary.json`、`delivery-basic\summary.json` |
| 代表性 Linux 编译 | PowerShell 临时设置 `GOOS=linux`、`GOARCH=amd64` 后运行 `go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\steerline-linux ./cmd/steerline`，结束后恢复变量；通过，产物 22,991,121 字节；仅证明编译通过，不是 Linux 安装验收。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\steerline-linux` |

`go run` 的初次调用在 runner 参数解析阶段返回 `Go test arguments must follow --`，没有启动测试；随后由 PowerShell 直接运行构建后的 `test-feedback.exe` 完成相同选择。报告中的 source commit、tree cleanliness 与 E2E 构建 SHA-256 用于核对实际候选。没有运行全量测试、Full、L2 或 live 场景。可选 `-IncludeSymbolicLinks` 文件符号链接负例未在本机执行；默认 P1 入口中的 junction/hardlink 覆盖已通过，不把可选项记作已验。

## 原执行场景记录（保留历史证据）

环境：Windows/amd64 controlled；两个独立 target Git clone/home；另创建同一 clone 的第二 home 验证仓库锁；受控 worker/provider，无模型请求。场景 `runtime.installed-debug-isolation` 通过，`passed=true`、`complete=true`、`cleanup_complete=true`、零 skip，耗时 224.28 秒。源码构建身份为 commit `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`、`dirty=true`。

自动验收/结果登记命令：

```powershell
go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-runtime-20261008-11'
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=controlled '-scenario=delivery.basic' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-delivery-20261008-01'
go run ./cmd/test-feedback -level=L1 '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-l1-20261008-06'
go test ./internal/app -run 'TestConsoleProjectRefExposesCurrentRuntimeIdentity|TestNativeConfigRejects(ASecondRunDir|WorktreeOutsideHome)$' -count=1
go test ./internal/console -run '^TestConsoleDiagnosticsShowRuntimeIdentity$' -count=1
go test ./internal/app/host -run '^TestWindowsStartServiceRequiresVisibleWindow$' -count=1
go test ./cmd/steerline -count=1
go test -tags=e2e ./tests/e2e/suites/runtime -run '^$' -count=1
git diff --check
```

Linux 代表性源码编译：PowerShell 临时设置 `GOOS=linux`、`GOARCH=amd64`，运行 `go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-linux-build-20261008-01\steerline ./cmd/steerline`，随后恢复原环境变量；命令成功，产物 22,991,569 字节。本结果只表示 Linux 编译通过，不是 Linux 安装验收。

最终隔离场景的 L3 原始报告位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-runtime-20261008-11`：`summary.json`、`e2e-1.jsonl`、`scenarios/runtime.installed-debug-isolation/scenario.json`、`runtime-identities.json`、`resources.json`，以及安装/调试两侧的 waiting-merge 与 completed 快照、命令日志、路径拒绝日志。共享 testenv 路径回归 `delivery.basic` 也通过，`passed=true`、清理完成、零 skip；报告位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-delivery-20261008-01`。最终 L1 summary 位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-l1-20261008-06\summary.json`，`passed=true`、`complete=true`，3 workers/12 groups。Linux 编译产物保留在 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-linux-build-20261008-01\steerline`。

LI1-A 由两侧同时运行及分别等待合并/完成的快照验证。LI2-A 记录安装 exe 构建前后 SHA-256 与安装服务实例 ID；debug exe 副本在重启前后 SHA-256 均为 `dd43089d0a7d84384c872be3a3cd77f53a64c208f2f6d790091cac0fd2e37a47`，debug 服务实例从 `ea5b2f26c4440529e5243c134fbd5245` 变为 `9214812cb3e3a9dae160ab603af2724c`，任务与 Delivery 保持。LI3-A 证据包含整个 home junction alias 重叠拒绝、运行中的 debug exe 阻止 build、home 外状态/工作区拒绝、过期 instance/config 写入拒绝且 request 数量不变、同 clone 跨 home 锁拒绝。LI4-A 使用包含中文和空格的目录及无关 cwd，home CLI 与 service.log 显示本次实际路径和构建身份；生成入口的 `CODEX_HOME` 赋值仅检查了文本。真实 `-Operation console/service` 与生成 batch 的子进程未执行，不能由该记录宣称验证。

开发期的初始试跑曾发现构建 provenance 缺失、Windows PowerShell 5.1 的 UTF-8/哈希/junction 清理差异，以及随 home containment 新错误顺序变化的旧断言；由当时的场景/L1 复验。失败试跑证据保留在 `%TEMP%\steerline-task83-runtime-20261008-01` 至 `-04`、`-10`；当时通过记录为 `-11`。后续独立复审的自动失败及原需求遗漏见下文，仍是最终交付的必验项，不转成人工后验收。

## 2026-10-08 独立复审与 P1 局部修复

独立复审候选为 `ace941de7d4be04c80afcdc2ebe01c47644aa168`，base/main 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。原 L3 报告显示 base commit 加 `dirty=true`，没有完整 dirty 源码内容摘要，不能作为候选 HEAD 的精确验证。

真实 PowerShell 复现目录为 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-review-6705823135e84d87b6ee9f6a345c3e8f`，仅临时安装文件和假认证。`debug/bin` junction 指向 `installed/bin` 时 `-Operation install` 错误成功，将安装哨兵 exe SHA-256 从 `30B476D3FA6E41DBCE1324523F1005213BBC99922C651D087CBE74F3389D56B9` 改为 `DD43089D0A7D84384C872BE3A3CD77F53A64C208F2F6D790091CAC0FD2E37A47`；`debug/codex` junction 指向 `installed/codex` 时 `-Operation launchers -CodexAuthHome <临时假认证源>` 错误成功写入安装 auth。原整个 home alias 断言未覆盖这两个目标。

局部修复复用脚本的 canonical containment：在环境赋值、build/install/launchers/认证复制之前核验源码 bin/exe、debug bin/exe、codex/auth 及 console/service/README 输出；已有 reparse 文件及缺失目标尾段也解析，不能规范到单一路径的 hard link 可写文件拒绝。没有修改固定 Spec/Plan、实际 exe 选择或产品启服接线。

定向命令（Windows PowerShell 5.1）：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-debug-runtime-isolation.ps1 -Out C:\Users\lmas1\AppData\Local\Temp\steerline-task83-p1-fix-20261008-04
git diff --check
```

该入口执行真实 `debug-runtime.ps1` 和现有真实 build exe；2 个 junction 负向场景（bin、codex 下缺失 auth 尾段）、5 个 hard link 文件场景（exe/auth/console/service/README）均在副作用前拒绝，安装及 debug 目录/文件快照不变；普通路径 install、连续两次 launchers 成功，auth/入口仅写入 debug，临时安装快照不变。`summary.json` 登记脚本 SHA-256、build exe SHA-256 与执行时 HEAD，日志及现场留在上述新目录。测试中的实例/任务事实是明确标记的合成文件哨兵；没有运行服务或验证 SQLite 任务，不替代最终候选的真实服务实例/任务事实复验。

初次试跑 `%TEMP%\steerline-task83-p1-fix-20261008-01` 的两个 junction 场景已拒绝，随后创建文件 symbolic link 因当前用户缺少 Windows 权限而停止；证据保留。默认定向入口覆盖无需提权的 junction/hard link，`-02`/`-03` 为早期通过记录。文件 symbolic link（含 dangling auth）有显式 `-IncludeSymbolicLinks` 入口，本轮未执行成功，待具备链接创建权限的环境验证；未将其记为通过。

本节的最终 controlled 结果替代此前“仍待实际源码 exe/console/service/batch 验证”的待验状态；此前 base+dirty 报告及 P1 局部脚本不单独代表最终候选通过。PR #247 尚未合并，因此此处记录自动验收事实，不把任务状态改为实现完成，也不释放依赖后继。

临时证据目录在用户主机保留供协调聊天复审；仓库只登记上述最小身份、摘要与结果。

## 人工验收

本任务无人工验收项。一期 24 小时与组合签收安排见独立[一期活动](phase1.md)。

## 记录与交接

执行时按 LI1-A–LI4-A 逐项记录实际命令、环境/构建、日期、结果与临时证据位置；execution-map 同步实现状态、验收摘要并链接本文件。独立阶段活动引用本任务结果，不覆盖本文件明细。
