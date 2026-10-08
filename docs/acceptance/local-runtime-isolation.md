# 任务 #83 验收规划：本机安装与调试路径隔离

唯一任务归属：#83。来源：[Spec](../spec/local-runtime-isolation.spec.md) / [Plan](../spec/local-runtime-isolation.plan.md)。本文件是该任务专属验收文档，不覆盖其他任务；验收流程见[入口](README.md)。

状态：round 2 发现的缺省 state/UI junction 写越界已修复并由真实产品入口负例复验；debug `Operation service`、生成的 `service-window.bat` 和 `console.bat` 也已实际运行。最终实现候选 `ad22d476f87e019a9ad5d0118758b253dd091094` 在 Windows/amd64 clean tree 上通过 LI1-A–LI4-A 自动验收，见最终记录。PR #247 保持打开且未合并，等待新一轮独立复审；任务仍为执行中，不释放后继。无本任务人工验收项。固定 Spec/Plan commit 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。实现 PR：[PR #247](https://github.com/erranliu/SteerLine/pull/247)。本文件保留旧证据、复审发现及局部修复事实。

## 自动验收

以下是源场景的全部自动子项，均由 Plan 映射至 `runtime.installed-debug-isolation` 受控场景；实际结果和证据位置逐项登记于下表及执行记录。

| 验收项 | 方法 | 规划行为与通过条件 | 状态 / 结果 |
| --- | --- | --- | --- |
| LI1-A | 自动 | 两个独立测试 clone、不同 home 的安装版与调试版同时运行；配置、状态、日志、服务发现和写命令不串线 | 已验收：final L3 场景两侧分别等待合并和完成，项目/数据库身份独立，Task/Delivery 写入与快照未串线；清理完成 |
| LI2-A | 自动 | 调试 build、停启及入口重复生成不改安装 exe 摘要、原服务实例或任务事实；调试服务/控制台/CLI 及子进程使用实际调试 exe/home | 已验收：CLI、`Operation service` 调用的 `service-window.bat`、直接生成 batch 和 `console.bat` ConPTY 路径均核对实际 exe/home/config/commit；两次停服回执 `applied`、`process_exited=true`、`locks_released=true`。安装 exe SHA-256 `40e53741c02136e8bcf1269634915bf0cfec186b6629908846c1058f3b3b50d7` 及安装 instance `926f5f25ca799e0f403288ec461f8454` 保持；调试 exe SHA-256 `36b15b59d172be3858ea8c79a0214ca409d8e26538f071bb92b1163e825bcb8f` rebuild 前后相同；调试重启任务事实保持 |
| LI3-A | 自动 | 缺少调试路径、home 重叠、显式状态/工作区越出调试 home 拒绝；同仓库跨 home 第二写者被原锁拒绝；误连实例的写命令仍被拒绝 | 已验收：state/UI junction 的脚本入口与真实 native task/console 入口均在安装侧目录、数据库或 sentinel 改变前拒绝；P1 alias/hardlink/正常路径 10 项、显式越界、stale instance/config 和跨 home 锁断言均通过 |
| LI4-A | 自动 | Windows 中文/空格路径及无关 cwd 可启动并显示准确身份；维护说明区分中心停服、外部执行结算、同构建重启和人工更新 | 已验收：final L3 使用中文/空格路径和无关 cwd；实际 console/service/CLI identity 与维护步骤见[本机安装与源码调试](../runtime-install-debug.md) |

环境：Windows/amd64 controlled；两个独立 Git clone/home、真实 SteerLine 进程、SQLite、Git 和受控 worker/provider；不发模型请求。每次输出目录独立，服务和临时资源由场景登记并清理。

## 2026-10-08 round 1 候选验证（范围有限）

精确验证候选：commit `066a5985677cccb4fdb4650bf669ddf15050de90`，tree `98856831907b85a37ba8b5f3364726b0659de358`，Windows/amd64，runner 记录 `clean (this run's logs and summary excluded)`；base/main 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。round 2 新复现证明该候选的 LI3-A 不通过且 LI2-A batch 部分未验；以下结果只保留其真实执行范围，不作全部验收通过结论。

| 自动验收 | 实际命令 / 结果 | 证据 |
| --- | --- | --- |
| `runtime.installed-debug-isolation` 既有范围 | 构建 runner：`go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\test-feedback.exe ./cmd/test-feedback`；执行：`& 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\test-feedback.exe' '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\runtime'`。runner 结果 `passed=true`、`cleanup_complete=true`、零 skip，223.9 秒。覆盖两侧独立事实、CLI/source identity 和 `Operation console` 的 ConPTY TUI；**不覆盖**缺省 state/UI alias 或实际生成 batch/service 入口。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-05\runtime\summary.json`、`e2e-1.jsonl`、`scenarios\runtime.installed-debug-isolation\scenario.json`、`runtime-identities.json`、`resources.json`、`debug-runtime-console.log` 及两侧命令/快照日志 |
| LI2-A | 安装 exe SHA-256 为 `a9a14f1c758fd3dde57f65ee47be03c2bc49b31d1673642f97732e4d174debf9`，build 前后及调试 rebuild 后不变；安装服务实例 ID 保持 `38de188e6c1bb02d93a13498535feaff`。调试 exe SHA-256 为 `5cb6d93c1448da8f7beeb0a9b8d5e2fb2f384f30aab3ec36e2c52f962ea1d0a7`，重建前后相同；同构建重启服务实例从 `fa61775482403d69fe0007fb79b0a12d` 更新为 `535e4948ceedd6beddb564bc38d8afe0`，任务与 Delivery 保持。 | 同上；build SHA-256 也登记在 L3 `summary.json` |
| 既有 P1 路径子集 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-debug-runtime-isolation.ps1 -Out C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\p1-alias`；8 项通过：2 junction、5 hardlink、重复 launchers 普通路径，安装侧哨兵快照保持且 debug home 未复制 exe。该脚本不覆盖产品运行时缺省 state/UI 写路径。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\p1-alias\summary.json` 及各 case 日志；L3 的 `runtime-identities.json` 与拒绝日志 |
| ConPTY helper | `go test -tags=e2e ./tests/e2e/internal/testenv -run '^TestPseudoConsoleCapturesInteractiveChild$' -count=1 -v`；通过，实际 console 子进程 marker 从 ConPTY transcript 捕获。 | 终端结果；实际 TUI transcript 保存在 L3 证据目录 |
| 共享回归 | 构建 runner：`go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\test-feedback.exe ./cmd/test-feedback`。执行该 runner 的 `-level=L1 -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\l1`；L1 固定 quick、3 workers，`passed=true`、`complete=true`，72.3 秒。再执行 `-level=L3 -suite=delivery -env=controlled -scenario=delivery.basic -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\delivery-basic`；`passed=true`、清理完成，72.4 秒。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\l1\summary.json`、`delivery-basic\summary.json` |
| 代表性 Linux 编译 | PowerShell 临时设置 `GOOS=linux`、`GOARCH=amd64` 后运行 `go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\steerline-linux ./cmd/steerline`，结束后恢复变量；通过，产物 22,991,121 字节；仅证明编译通过，不是 Linux 安装验收。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-final-20261008-06\steerline-linux` |

`go run` 的初次调用在 runner 参数解析阶段返回 `Go test arguments must follow --`，没有启动测试；随后由 PowerShell 直接运行构建后的 `test-feedback.exe` 完成既有场景。报告中的 source commit、tree cleanliness 与 E2E 构建 SHA-256 用于核对候选。没有运行全量测试、Full、L2 或 live 场景。可选 `-IncludeSymbolicLinks` 文件符号链接负例未在本机执行；既有 junction/hardlink 子集通过不覆盖本轮缺省路径失败。

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

round 2 复审已证明本节报告不能关闭全部自动验收：实际 debug/state 和 debug/ui junction 仍写入安装侧，LI3-A 失败；生成的 `service-window.bat`、`console.bat` 及 `Operation service` 未被实际执行，LI2-A 待验。此前 L1、delivery 和正常路径结果保留各自范围，但不得覆盖该失败或未测事实。完整复现、数据库摘要和复审来源见下节。PR #247 尚未合并，任务保持执行中。

## 2026-10-08 独立复审 round 2：缺省写路径失败及 batch 未验

复审报告路径为 `C:\Users\lmas1\.codex\task-state\roadmap-runner\roadmap-steerline-20261008-01a11a74\review83-round2.md`；PR 评论：[#247 review](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6060563615)。复审候选是当前 PR head `0eb6e2b3d0e966662a79f85f17f239bb6c89cfe7`，base/main `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`；受验代码候选仍为 `066a5985677cccb4fdb4650bf669ddf15050de90`。此前源码 exe 传递、bin/codex/hardlink 防护及其报告身份映射继续有效。

仅临时复现目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-task83-review2-ui-c8b39b97ef7540218925bd7f5a9352b3`，使用隔离 clone/home、假认证和本次受控进程；未操作用户日常安装或其他服务。

| 失败 / 待验项 | 实际复现 | round 2 当时状态 |
| --- | --- | --- |
| LI3-A：缺省 `state` 可写路径 | `debug/state` junction 指向 `installed/state`；配置省略 `run_dir`；真实 `debug-runtime.ps1 -Operation cli -Arguments task validate -file <含 {} 的批次>` 最终因 `invalid_batch` 退出 1，但此前已在安装侧创建真实 SQLite：`state/repository-fa682b098eac/steerline.db` 311,296 字节，SHA-256 `7a1c1badaab56cf83bd44138feaf7bd5573b802277da37551312cbbde1f9fe70`，并出现 32,768 字节 SHM。 | **当时失败**。最终候选改为在首次初始化前拒绝，并以目录不存在及 sentinel 未变复验，见最终记录。 |
| LI3-A：缺省 UI 可写路径 | `debug/ui` junction 指向 `installed/ui`；真实 `-Operation console` 在失败前于安装侧创建 `ui/views`。调用顺序中 `preferences.go` 的 `MkdirAll` 早于现有 canonical 检查。 | **当时失败**。最终候选的实际 ConPTY console 在建 views 前拒绝，安装侧目录/sentinel 未变，见最终记录。 |
| LI2-A：真实生成入口 | 旧场景 `debug.StartService()` 由 `testenv/env.go` 直接运行 source exe `run -console off`；`start-service` 只断言 `window-required` JSON。`Operation console` 的 ConPTY transcript 是真实 exe，但没有实际运行 `console.bat`、`service-window.bat` 或 `Operation service`。 | **当时待验**。最终候选执行了 `Operation service`、直接生成的 `service-window.bat` 和 `console.bat`，见最终记录。 |

上述 round 2 失败和待验范围已由候选 `ad22d476f87e019a9ad5d0118758b253dd091094` 修复并复验，逐项命令与证据见下方最终记录。round 1/round 2 原始报告保留各自实际范围；可选文件符号链接负例仍未执行，不计为已验。

## 2026-10-08 最终候选复验（round 2 修复后）

受验实现/测试候选：commit `ad22d476f87e019a9ad5d0118758b253dd091094`，tree `e2470c18a0d29979006aa5e12a440f2035bf5fa0`，Windows/amd64，分级 runner 记录 clean source；base/main `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。round 1 和 round 2 的失败发现未被擦除，以下是该修复候选的新证据。

| 范围 | 实际命令 / 结果 | 证据 |
| --- | --- | --- |
| LI1-A–LI4-A、round 2 state/UI 修复及真实入口 | 构建 runner：`go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\test-feedback.exe ./cmd/test-feedback`；执行 `& 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\test-feedback.exe' '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\runtime'`。`passed=true`、`cleanup_complete=true`、零 skip，231.8 秒。源码 commit 为本节候选、`source_dirty=false`。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\runtime\summary.json`、`e2e-1.jsonl`、`scenarios\runtime.installed-debug-isolation\scenario.json`、`runtime-identities.json`、`resources.json`、`debug-runtime-console.log`、两个 service-window 日志、两侧快照和命令日志 |
| LI3-A 缺省 state alias | `debug/state` junction → 安装侧 `state`，省略 `run_dir`/`worktree_root`。真实脚本 CLI 和源码 exe `task validate -config <config> -file <{} 批次>` 均在初始化前拒绝；目标 `state\repo-17fb9e3ec52e` 未创建，安装 state sentinel SHA-256 `e1dea4ec803dcef52c6778c7bdd85200c8e2fbbf3ebb335dc61999c3d35d41cf` 保持。 | runtime `scenarios\runtime.installed-debug-isolation\runtime-identities.json` 的 `default_state_alias_rejected`；`default-state-script-alias.log`、`default-state-native-alias.log` |
| LI3-A 缺省 UI alias | `debug/ui` junction → 安装侧 `ui`。脚本 console 操作和源码 exe 的真实 ConPTY `console` 入口均在 `ui/views` 创建前拒绝；安装侧 views 目录未创建，UI sentinel SHA-256 `2b332af136fa78809e9ca0d395681822db20c276d2839f5e4a66921b2944b307` 保持。 | 同上 `default_ui_alias_rejected`；`default-ui-native-console-alias.log` |
| LI2-A 真实 `Operation service` 与 launcher | `debug-runtime.ps1 -Operation service` 实际调用生成的 `service-window.bat`：launcher PID `39844`，service PID `39284`，source exe 为 worktree `bin\steerline.exe`，配置/home 为本次 debug 项目，service instance `1ba870992b72269eb007062f959e9bbc`。随后直接执行 `cmd.exe /c call <debug-home>\service-window.bat <config>`：launcher PID `36640`，service PID `39856`，同一实际 exe/home/config，新的 instance `53005c4469696095beb8ace36d380858`。两次停服回执均为 `applied` 且 `process_exited=true`、`locks_released=true`；各自 owned visible-console Job 已关闭。两个周期任务与 Delivery 保持。 | runtime `runtime-identities.json` 的 `service_window_entry`、`generated_service_window_batch`、两个 stop 字段；`debug-operation-service-window-1.log`、`debug-generated-service-window-batch-2.log`、service startup logs、completed/final 快照和 `resources.json` |
| LI2-A 真实 `console.bat` | 通过 Windows ConPTY 执行 `cmd.exe /c call <debug-home>\console.bat --steerline-console-child <config>`；transcript 含实际 exe、home、config、source commit `ad22d476f87e019a9ad5d0118758b253dd091094`、`source_dirty=false`。 | runtime `debug-runtime-console.log` 与 `runtime-identities.json` 的 `console_runtime_identity` |
| P1 alias 子集与 ConPTY helper | `.\scripts\test-debug-runtime-isolation.ps1 -Out C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\p1-alias`；10 项通过（新增 state/ui junction）。`go test -tags=e2e ./tests/e2e/internal/testenv -run '^TestPseudoConsoleCapturesInteractiveChild$' -count=1 -v` 通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\p1-alias\summary.json` 及 case logs；terminal result |
| L1 与共享 delivery | runner `-level=L1 -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\l1`：`passed=true`、`complete=true`、3 workers/12 groups，60.9 秒。runner `-level=L3 -suite=delivery -env=controlled -scenario=delivery.basic -out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\delivery-basic`：`passed=true`、清理完成，65.8 秒。 | `...\l1\summary.json`、`...\delivery-basic\summary.json` |
| 受影响 owner / architecture | `internal/pathguard` junction resolver、app、host、完整 console、`cmd/steerline`、Windows ConPTY smoke、E2E helper/suite compile、`internal/testkit/boundary` 与 `tests/integration` 的 TM04/CD67 boundary tests 均通过；L1 也通过。 | 本轮终端输出、L1 组日志 |
| Linux 编译 | PowerShell 临时设置 `GOOS=linux`、`GOARCH=amd64`，运行 `go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\steerline-linux ./cmd/steerline`，后恢复原变量；通过，23,012,753 字节。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\steerline-linux` |

精确分级 runner 调用（工作目录为仓库根）：

```powershell
$runner = 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\test-feedback.exe'
& $runner '-level=L1' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\l1'
& $runner '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\runtime'
& $runner '-level=L3' '-suite=delivery' '-env=controlled' '-scenario=delivery.basic' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round2-final-20261008-02\delivery-basic'
```

受影响 owner/边界命令均在候选 `ad22d476f87e019a9ad5d0118758b253dd091094` 上通过：

```powershell
go test ./internal/pathguard -count=1
go test ./internal/app -run 'Test(SteerlineHomeAndDirs|DerivedRunDirIsStableAcrossWorktrees|NativeConfigRejects(ASecondRunDir|WorktreeOutsideHome))$' -count=1
go test ./internal/console -count=1
go test ./internal/app/host -run 'Test(WindowsStartServiceRequiresVisibleWindow|StartupIDAndOutputPathValidation)$' -count=1
go test ./internal/testkit/boundary ./tests/integration -run '^(TestTM04ExecutionBoundary|TestCD67CoreBoundary)$' -count=1
go test ./cmd/steerline -count=1
go test -tags=e2e ./tests/e2e/internal/testenv -run '^TestPseudoConsoleCapturesInteractiveChild$' -count=1 -v
go test -tags=e2e ./tests/e2e/internal/testenv ./tests/e2e/suites/runtime -run '^$' -count=1
```

最终候选将 Windows 真实路径解析集中在 app/host 使用的 `internal/pathguard`，console 在自身 owner 内使用等价的 junction 解析，保持既有 console/product dependency boundary。home/state/SQLite journal/control/service/worktree 路径在 app 与 host 副作用前检查；UI root/profile/lock/journal 在 console 建目录或打开文件前检查。可选 `-IncludeSymbolicLinks` 文件符号链接负例仍未运行；其它上述最终自动验收通过。

临时证据目录在用户主机保留供协调聊天复审；仓库只登记上述最小身份、摘要与结果。

## 人工验收

本任务无人工验收项。一期 24 小时与组合签收安排见独立[一期活动](phase1.md)。

## 记录与交接

执行时按 LI1-A–LI4-A 逐项记录实际命令、环境/构建、日期、结果与临时证据位置；execution-map 同步实现状态、验收摘要并链接本文件。独立阶段活动引用本任务结果，不覆盖本文件明细。
