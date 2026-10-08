# 任务 #83 验收规划：本机安装与调试路径隔离

唯一任务归属：#83。来源：[Spec](../spec/local-runtime-isolation.spec.md) / [Plan](../spec/local-runtime-isolation.plan.md)。本文件是该任务专属验收文档，不覆盖其他任务；验收流程见[入口](README.md)。

状态：#83 实现完成；[PR #247](https://github.com/erranliu/SteerLine/pull/247) 经第五轮独立复审和必要验证后真实合并，merge commit `f7c6f1d05c78172fe8133f7c1763718df0165fe1`。LI1-A–LI4-A 自动验收通过，无本任务人工项；精确受验源码/结果映射及原失败证据保留如下。一期组合/24小时/同构建恢复是独立待签收活动，不由此任务结算宣布通过。固定 Spec/Plan commit 为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。

## 2026-10-09 交付结算与独立复审

第五轮独立 reviewer `/root/review83_round5` 检查完整候选 diff、固定 Spec/Plan、实际生产消费者和 LI1-A–LI4-A 原始证据，未发现新的产品问题或需求遗漏。协调核对报告及源码映射，在 [PR 复审记录](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6065781425)登记各轮 reviewer/fixer、问题、修复提交和实际验证。最终审核 head `2ce81db45d7bff32a1ffb6134210995529f01b84`，base `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`；平台确认合并时间为北京时间 2026-10-09 01:50:18，merge commit 如上。合并前核实 main 无 branch protection rule 或包含父级的 rulesets、无必需 checks，PR CLEAN/MERGEABLE，并使用预期 head 合并。

自动验收仍绑定 clean `92eaf9b8` runtime 和 clean `9aeed603` 定向/L1/Linux，后续生产 Go 未变化、runtime 候选至审核 head 仅结果文档变化；不是在 merge commit 上重跑。六条共享 SQLite 真实负例、正常单链接、实际 source exe/生成 batch/进程身份/停服/锁释放、安装摘要/实例及正式请求 0→0/0→2 均由独立复审核实。本任务无人工待验或验收豁免；可选文件 symbolic-link 原生负例未运行，不计为通过。旧 delivery 仅保留原受验范围。

复审另做的 8.3 short-path 与嵌套 DebugHome 临时探针，在生成输出前正确拒绝重叠。其目录 `C:/Users/lmas1/AppData/Local/Temp/sl83-r5-alias-20261009-01` 的精确递归及逐文件非递归清理被自动审批拒绝，原因仅 `blocked by policy`，目前保留三个合成文件，无进程、链接或真实认证；未将此探针标记清理完成。产品验收 runtime 的进程、链接和 scratch 清理已完成，该复审探针残留另行保留为操作限制。报告在仓库外 run 的 `review83-round5.md`，详细执行结果按下方既有证据定位。

## 自动验收

以下是源场景的全部自动子项，均由 Plan 映射至 `runtime.installed-debug-isolation` 受控场景；实际结果和证据位置逐项登记于下表及执行记录。

| 验收项 | 方法 | 规划行为与通过条件 | 状态 / 结果 |
| --- | --- | --- | --- |
| LI1-A | 自动 | 两个独立测试 clone、不同 home 的安装版与调试版同时运行；配置、状态、日志、服务发现和写命令不串线 | **已验收**：最终 L3 两侧项目和数据库身份独立；Task/Delivery 写入、快照及服务实例未串线，场景清理完成 |
| LI2-A | 自动 | 调试 build、停启及入口重复生成不改安装 exe 摘要、原服务实例或任务事实；调试服务/控制台/CLI 及子进程使用实际调试 exe/home | **已验收**：最终 L3 直接 `build.ps1` 对源码 `bin` junction 与输出硬链接均在写入前拒绝；普通首次构建及再次重建成功；安装 exe、SQLite、bin 内容、任务事实与服务实例均保持；真实 service-window、console/ConPTY 和 CLI identity 通过 |
| LI3-A | 自动 | 缺少调试路径、home 重叠、显式状态/工作区越出调试 home 拒绝；同仓库跨 home 第二写者被原锁拒绝；误连实例的写命令仍被拒绝 | **已验收**：round 4 最终 runtime 在 SQLite DB/WAL/SHM 共享硬链接时，产品 CLI 与真实 debug wrapper 六次均在 SQLite 副作用前明确拒绝，哨兵 size/hash 及全部 SQLite 文件快照保持；普通单链接仍到达业务校验；原有最终 L3 的 stale instance 与 changed config 两条拒绝均正式 request 计数 0→0；正常产品领取发布 `resume_dispatch` 与 `start_task` 两条有效请求，计数 0→2；state/UI alias、越界和跨 home 写锁均拒绝 |
| LI4-A | 自动 | Windows 中文/空格路径及无关 cwd 可启动并显示准确身份；维护说明区分中心停服、外部执行结算、同构建重启和人工更新 | **已验收**：最终 L3 使用中文/空格路径及无关 cwd，CLI、ConPTY console、两个 service-window 入口报告精确 exe/home/config/commit/dirty；维护步骤见[本机安装与源码调试](../runtime-install-debug.md) |

环境：Windows/amd64 controlled；两个独立 Git clone/home、真实 SteerLine 进程、SQLite、Git 和受控 worker/provider；不发模型请求。每次输出目录独立，服务和临时资源由场景登记并清理。

## 2026-10-09 round 4 局部修复与精确候选复验

生产实现 commit `9aeed603d46bf7d566b2daf183855dc3c8316fc4`，tree `25ca5decd8e1c918b099303f96595f22c1c2cd7e`。复用 `internal/pathguard`，Windows 用 `GetFileInformationByHandle.NumberOfLinks`、Unix 用原生 `Nlink`，检查 `steerline.db`、`-wal`、`-shm`、`-journal`；已有文件 link count > 1 明确拒绝，缺失及普通单链接按原流程使用。app 配置/home 预检、host 调用 `ValidateNativeStore` 前预检及 app 既有 reader/writer 打开前均接入，保留目录 canonical containment；未改 schema、业务状态机、固定 Spec/Plan 或共享 testenv/driver。

最终受验候选 `92eaf9b8163f71be0554265ddba85ee0708855ee`，tree `28f8395f90b71d3f8a7d0dc139c43d17dbbd2345`，Windows/amd64 clean source，dirty=false。该提交只把新增负例移到两侧真实停服、进程退出及锁释放之后，生产 Go 文件与 `9aeed603` 完全相同。先提交实现/场景，再从各自精确 commit 构建和运行；后续验收文档提交不改变受验代码。

最终 L3 `runtime.installed-debug-isolation` 通过：1 test、0 skip、262.63 秒，`passed=true`、场景 `passed=true`、`cleanup_complete=true`。当前格式 debug 数据库由真实 task validate/create 初始化并经过原有运行/重启；DB 负例的 installed 哨兵复制该真实数据库（376832 字节，SHA-256 `e8f8ead6ea107fec1f5075b8e35911055be6d0033915f2d076422ebd6948f869`），WAL/SHM 哨兵为空。逐文件建立 debug SQLite 路径到临时 installed home 哨兵的硬链接后，实际产品 `task validate -config ... -file <{}>` 和实际 `debug-runtime.ps1 -Operation cli -Arguments @('task','validate','-file',...)` 六次均退出 1，明确报告 `SQLite state file must not be a hard link (2 links)`。每次调用都核对哨兵 size/hash 和 DB/WAL/SHM/journal 的存在性、size/hash 全部保持，没有 SQLite 文件创建/改写；空 WAL/SHM 保持 0 字节与 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`。两侧均为本场景独立 clone/home/假认证，不触及日常安装。

解除链接并恢复原 SQLite 文件后，普通单链接实际 task validate 正常到达 `invalid_batch: schema_version, project_id and tasks are required`。临时 installed 项目真实 DB 在整组负例前后均为 `a23a7b37fad5f4c7532ef68f59a9d34c61a510c5ec3d7c57ab741c3839f83a3e`。原有源码 exe/home、直接 build/rebuild/输出硬链接与 junction 拒绝、state/UI alias、generated service/console batch、原进程退出/锁释放、任务与 Delivery/同构建重启断言全部保留并通过；stale instance/config 正式 request 均 0→0，正常产品领取 0→2。安装 exe SHA-256 `5c31d749dac2b6ddd00d5e3255925a840882213247aa74cbbb7e1f8a65690af7`，实际源码调试 exe `f51df1f7462d3f7850b0f63f65e949ff6aedb514865743ae9365e6d672a8a047`；完整摘要在最终 runtime 证据。

最小回归精确映射：`9aeed603` 上 pathguard 四种文件后缀、app reader 拒绝及 host validator 前拒绝等 9 个顶层定向测试通过；L1 3 workers/12 groups/426 tests/0 skip，`passed=true`、`complete=true`，70.55 秒；Linux/amd64 编译通过，产物 23017964 字节、SHA-256 `D1F912C489A698AC2F33B5F78D69D557A4AF41E1B041916144E75686493D4919`。`92eaf9b8` 之后没有生产 Go 变化，以上必要回归按该生产代码映射复用，不重复全套。`delivery.basic` 保留 `f838dcf587efb2cb22992e6354640f530e0df8b7` 的原通过范围；本轮未改共享 testenv/driver 或业务交付逻辑，当前 runtime/L1 覆盖新增 preflight 风险，未重跑 delivery、P1 脚本、Full/L2/live，不将其旧证据算作当前源码重跑。

精确命令（仓库根目录；每次 runner 输出均新空目录）：

```powershell
$evidence01 = 'C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round4-final-20261009-01'
$evidence02 = 'C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round4-final-20261009-02'
# 9aeed603：生产回归
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./build.ps1
go build -o "$evidence01/test-feedback.exe" ./cmd/test-feedback
go test -v ./internal/pathguard ./internal/app ./internal/app/host -run 'Test(RejectSharedSQLiteFiles|NativeReaderRejectsSharedSQLiteBeforeOpen|NativePreflightRejectsSharedSQLiteBeforeValidator|NativePreflightAllowsItsEmptyRunLockFile|NativeProjectInitializesOnceAndRejectsMismatchedBinding|SteerlineHomeAndDirs|DerivedRunDirIsStableAcrossWorktrees|NativeConfigRejectsASecondRunDir|NativeConfigRejectsWorktreeOutsideHome)$' -count=1
& "$evidence01/test-feedback.exe" '-level=L1' "-out=$evidence01/l1"
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o "$evidence01/linux/steerline" ./cmd/steerline
Remove-Item Env:GOOS; Remove-Item Env:GOARCH
# 92eaf9b8：最终完整 runtime
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./build.ps1
go build -o "$evidence02/test-feedback.exe" ./cmd/test-feedback
& "$evidence02/test-feedback.exe" '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' "-out=$evidence02/runtime"
```

证据01=`C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round4-final-20261009-01`：`targeted-go.log`、`l1/summary.json`、`linux/{build-linux.log,steerline}`。该目录另保留首轮失败 runtime（源码 `9aeed603`，115.44 秒、清理完成）：六条 SQLite 拒绝及哨兵/SQLite 快照已全部通过，但新增 installed DB 摘要断言在安装服务仍运行时取样，正常持久化使字节比较不稳定；不能作整场景通过。修正时点后新现场复验，不覆盖失败产物。

证据02=`C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round4-final-20261009-02`：`build-windows.log`、`runtime/{summary.json,e2e-1.jsonl}`、`runtime/scenarios/runtime.installed-debug-isolation/{scenario.json,resources.json,runtime-identities.json,sqlite-shared-hardlinks.json,sqlite-shared-*-product_cli.log,sqlite-shared-*-debug_wrapper.log,sqlite-single-link-normal-validation.log}`。两轮 scratch `steerline-e2e-3237167511` / `steerline-e2e-2996633080` 已删除，链接在场景内精确解除、原文件恢复，清理无错误。PR #247 未合并；#83 仍执行中，等待新的独立复审，不释放后继。

## 2026-10-09 round 4 独立复审：SQLite 共享硬链接失败

受审 HEAD `054decfcc42f93dab7e8bc95c0f007e60e268708`，实现候选 `f838dcf587efb2cb22992e6354640f530e0df8b7`。独立临时 debug 当前格式数据库初始化后，`steerline.db-shm` 硬链接到另一临时 installed home 空哨兵；实际产品 `task validate -config ... -file <{}>` 与真实 `debug-runtime.ps1 -Operation cli` 均先将哨兵写成 32768 字节，再以 `invalid_batch` 退出 1。哨兵 SHA-256 从 `E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855` 变为 `FD4C9FDA9CD3F9AE7C962B0DDF37232294D55580E1AA165AA06129B8549389EB`。目录 canonical containment 不识别共享硬链接，LI3-A 整体失败；已有 state/UI alias、锁及 request 证据保留原范围。

原始证据：`C:/Users/lmas1/AppData/Local/Temp/sl83-r4-bbf4c68be4f247a8993a01b99c990883/{result.json,wrapper-result.json,wrapper-hardlink-shm.log}`；仅独立临时 Git/home/假认证，原链接已解除。该复审时局部修复及精确候选复验尚未完成；其后结果见上文，固定 Spec/Plan 不变。

## 2026-10-09 round 3 原执行者修复与最终候选复验

最终受验源码 commit `f838dcf587efb2cb22992e6354640f530e0df8b7`，tree `deb3ae0f1dbc3b2a9402e0ecc0681a2a5beb4682`；Windows/amd64 clean tree。固定 Spec/Plan 仍为 `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。本次只补齐 round 3 已发现的 P1 与 request 观察断言，并修正测试夹具的 Windows clone 路径、Git line-ending 环境和 `home` dirty 字段解析；产品实现未扩大到旧格式兼容或迁移。

最终 L3 `runtime.installed-debug-isolation` 通过：1 test、0 skip、266.06 秒、`complete=true`、`cleanup_complete=true`。LI2-A 的直接 `build.ps1` 保护在安装侧现场实测：源码 `bin` junction 指向隔离 installed/bin 时拒绝；`bin\steerline.exe` 是 installed exe 硬链接时拒绝；普通源码 clone 的首次构建和再次构建均成功。两条拒绝发生在写入前。source clone 在 clone 后、拒绝样例后、构建及重建后 `git status --porcelain` 均为空；运行 `home` 显示源码 commit 与 `source dirty=false`。installed exe SHA-256 `1ca523c0b0540c2dcc6eaaf772c9a77aa1b07bbf09dd74afddda81ef037cc4e9`、SQLite SHA-256 `56632d64828005f0e1553f698e97b447145c7e5ddb7fb577e242db88a46bb737`、service instance `f93f99159fabfe252065c9c33ee2edea`、installed/bin 内容及任务事实前后相同。新正常构建源码 exe SHA-256 `52fe1f7afca1bdab64af44b476aa7f267606b2728590f606abb68c45d7f78f7f`。

LI3-A 正式 request 观察：stale instance 写请求 `0→0`，changed config 写请求 `0→0`；正常 `debug.Claim("a")` 发布计数 `0→2`。两份真实文件分别为 `control/requests/1d09c17c0b0dfd439a5a700bb235b1d6/request.json`（`resume_dispatch`，SHA-256 `377b61f8197ec8db004beb946402c658f6aee1fe848571f47c561127969fdd76`）与 `control/requests/301b4bc4ed49a3281f29ee8c95bdab3c/request.json`（`start_task`，task `1`，SHA-256 `ac206b0c8b1ff97cbc5e4159b6d2d0776a62e1a8400aaec1a839e8662c4c2ff`）；二者均绑定 instance `6ae5ba8feae58dc854ef73274d15078a` 及本次 debug clone `.git` identity。

同一候选的其他最小回归：P1 `test-debug-runtime-isolation.ps1` 10 项通过；L1 `passed=true`、`complete=true`、3 workers/12 groups、426 tests、0 skip、61.9 秒；L3 `delivery.basic` 1 test、0 skip、66.6 秒且清理完成。Linux/amd64 `go build ./cmd/steerline` 通过，产物 23,012,753 字节，SHA-256 `955654B42D0F69689D8E4356D66539C58B8D4CE19A6D0F9085540760179DCEAB`。此前在 Windows component resolver 候选上通过的 owner/boundary 定向组无后续生产 Go 代码变化；最终 L1 亦重跑该固定回归。

精确命令（仓库根目录，所有 runner 输出使用本次新目录）：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build.ps1
.\scripts\test-debug-runtime-isolation.ps1 -Out 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\p1'
go build -o 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\test-feedback.exe' ./cmd/test-feedback
$runner = 'C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\test-feedback.exe'
& $runner '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\runtime'
& $runner '-level=L1' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\l1'
& $runner '-level=L3' '-suite=delivery' '-env=controlled' '-scenario=delivery.basic' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08\delivery-basic'
```

最终证据根为 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-final-20261009-08`：`direct-build.json`、`p1\summary.json`、`runtime\summary.json`、`runtime\e2e-1.jsonl`、`runtime\scenarios\runtime.installed-debug-isolation\{scenario.json,resources.json,runtime-identities.json}`、`l1\summary.json`、`delivery-basic\summary.json`、`linux\steerline`。临时原始产物留在该机器目录，不提交仓库。

前序失败复验均保留并限定为测试夹具问题：`steerline-task83-round3-final-20261008-04\runtime` 的源 clone 路径触发 Windows 长路径；`...-05\runtime` 与 `...-06\runtime` 的 clone checkout 使用系统 `core.autocrlf=true`，而构建进程禁用系统/全局 Git 配置，导致 dirty probe 报告大量 CRLF 文件；`...-07\runtime` 已显示 clean/dirty=false，但旧断言误找字面量 `dirty=false`，未按 `home` 实际格式读取 `source dirty   false`。这些场景均 `cleanup_complete=true`；当前代码通过将 clone 与构建置于相同 Git 环境、使用短 scratch 路径并解析字段值完成修复。

PR #247 当前保持 OPEN、未合并；任务 #83 继续执行中，等待新的独立复审。此记录不构成合并、实现完成或后继释放。

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

受验实现/测试候选：commit `ad22d476f87e019a9ad5d0118758b253dd091094`，tree `e2470c18a0d29979006aa5e12a440f2035bf5fa0`，Windows/amd64，分级 runner 记录 clean source；base/main `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。round 1 和 round 2 的失败发现未被擦除，以下是该修复候选当时的新证据。round 3 后，此节的整体验收通过结论已撤销：direct build.ps1 失败未覆盖，stale instance/config 的请求计数断言无效；其余证据仅保留原实际范围。

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

## 2026-10-08 独立复审 round 3：请求观察局部修复

复审候选 `f5c729f985f56f1a59c7506a6bc07dfc6e9ab402`，base/main `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`；报告 `C:\Users\lmas1\.codex\task-state\roadmap-runner\roadmap-steerline-20261008-01a11a74\review83-round3.md`。此前 round 1/round 2 场景的请求计数 glob 为 `control/requests/*.json`，生产正式协议实际使用 `control/requests/<request ID>/request.json`，因此历史 stale instance/config 拒绝前后计数恒零，“未发布请求”未有效验证；原错误返回、身份和任务事实证据仍保留其范围。

本轮仅修改 runtime 场景：观察正式 request 文件；保留 stale instance/config 拒绝及计数不增长断言；利用已有正常 `debug.Claim("a")` 经产品客户端发布 `resume_dispatch` 和 `start_task` 两条请求，要求计数增加 2，核验原 request ID 与目录名、repository/instance、目标 task，并在 `runtime-identities.json` 保存正式路径、文件摘要与身份。没有手工伪造正向请求、生产入口或生产代码改动。

本轮局部复验通过：Windows/amd64 controlled，仅执行一次 `runtime.installed-debug-isolation`，退出 0、`passed=true`、`complete=true`、`cleanup_complete=true`、1 个测试、零 skip；runner execution 238.367 秒。没有 L1、Full、L2、live 或其他产品场景重跑。本轮没有初次失败或自动重试，历史失败目录保持。

精确命令（工作树 `C:\Users\lmas1\.codex\worktrees\52f5\SteerLine`）：

```powershell
go build -o C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round3-p2-20261008-01/test-feedback.exe ./cmd/test-feedback
& 'C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round3-p2-20261008-01/test-feedback.exe' '-level=L3' '-suite=runtime' '-env=controlled' '-scenario=runtime.installed-debug-isolation' '-out=C:/Users/lmas1/AppData/Local/Temp/steerline-task83-round3-p2-20261008-01/runtime'
```

受验源码身份为 HEAD `f5c729f985f56f1a59c7506a6bc07dfc6e9ab402` 加本轮 runtime 测试与两份结果文档修改，`source_dirty=true`，不能称为 clean HEAD 验证；生产源码未修改。受验测试 Git blob `072258ca49b848521debb5336c1b97c5b204649a`，SHA-256 `4b49dbdbf6ae58e216a61c22f8a87054f3f2d3e23785c406cdcefb9a3b4a526e`（测试之后未再改动）；runner 构建的产品 exe SHA-256 `d520de8d717eb577927f48e4ae97f911e75d40b8c9fb42250420f0f28a96195b`，helper `1075e9b17f9f5f4655745aa4820845d0abf5c3f9ef99823f8fa9f4d27689ee29`。

证据根 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-round3-p2-20261008-01\runtime`，包括 `summary.json`、`e2e-1.jsonl`、`scenarios\runtime.installed-debug-isolation\scenario.json`、`resources.json`、`runtime-identities.json` 及原产品命令回执。`debug.stale_instance_request_counts` 与 `debug.changed_config_request_counts` 均为 before=0/after=0；`debug.normal_control_publication` 为 before=0/after=2，正式 `resume_dispatch` 请求 ID `04eafec8b5b29349f05b7a0895d13226`、`start_task` ID `76458df0fdfe561476f74a240777fe9c`，两者 instance=`b31e36254605c40292ceff67980ab64e`、repository identity 为本次 debug clone 的 `.git`；正式文件路径和 SHA-256 保留在同一证据中。场景自身停止 batch/service/helper 资源；scratch `C:\Users\lmas1\AppData\Local\Temp\steerline-e2e-3769137333` 已删除。

此次范围不关闭 LI2-A 的 direct build.ps1 失败，不替代原执行者完成 P1 后的最终验证或新独立复审；PR 未合并，不宣称最终实现全部通过。

## 人工验收

本任务无人工验收项。一期 24 小时与组合签收安排见独立[一期活动](phase1.md)。

## 记录与交接

执行时按 LI1-A–LI4-A 逐项记录实际命令、环境/构建、日期、结果与临时证据位置；execution-map 同步实现状态、验收摘要并链接本文件。独立阶段活动引用本任务结果，不覆盖本文件明细。
