# Steerline M0

Steerline 是面向持续软件工程的本机调度器：读取既有 Beads 可执行任务，原子领取后启动一个 `codex exec`，让仓库技能完成开发和提 PR；人工合并后，服务验证 PR 事实再关闭 Beads 任务，释放依赖。

本实现优先支持 Windows 原生运行。没有 Web 服务、新任务数据库、独立 worker、自动规划、多机调度、自动修复/回退或自动合并。worker 就是 Codex 进程。调度器获取目标主线并准备隔离 worktree；代码、测试、审计、提交和创建 PR 由目标仓库的技能负责。并发执行上限固定为 1，准备工作也占执行槽。

## 构建和启动

要求 Go 1.24+、Git、Beads、Codex CLI，以及已配置 Beads 和符合接入契约的目标仓库。本机验证版本：Go 1.26.4、bd 1.1.2、codex-cli 0.145.0。

```powershell
Set-Location C:\Project\SteerLine
go build -o .\bin\steerline.exe .\cmd\steerline
Copy-Item .\examples\config.json .\steerline.local.json
# 编辑 steerline.local.json，填写实际仓库、技能、主线、运行目录、工具和 PR 查询配置。
.\bin\steerline.exe check -config .\steerline.local.json
.\bin\steerline.exe run -config .\steerline.local.json
# 手动派发：服务启动并刷新候选，但不会自动领取 ready 任务
.\bin\steerline.exe run -config .\steerline.local.json -dispatch manual
# 安装当前构建与双击入口；Windows 用户独立打开可见服务窗口（manual）
.\bin\steerline.exe install
.\bin\steerline.exe launchers
& "C:\SteerlineRuntime\service-window.bat" (Resolve-Path .\steerline.local.json).Path
```

### 机器级目录（跨 worktree 稳定）

`STEERLINE_HOME`（默认 `C:\SteerlineRuntime`，可用同名环境变量覆盖）是本机共享的稳定目录：`bin\` 存二进制、`etc\` 存配置、`state\<repo>\` 存该仓库的 `runtime.json` 与 `runs\`。状态目录按仓库 Git common-dir 派生，**同一仓库的所有 worktree/clone 落到同一处**。

```powershell
.\bin\steerline.exe home                       # 打印 home / bin / etc / state / run_dir
.\bin\steerline.exe install                    # 把当前二进制装到 %STEERLINE_HOME%\bin\steerline.exe
```

之后服务、查看器和 `viewer.bat` 都跑 `%STEERLINE_HOME%\bin\steerline.exe`；源码换哪个 worktree 都不影响本机状态。详见 [状态模型](docs/state-model.md)。

多项目：每个目标仓库一份 `etc\<repo>.json`，各自派生出独立的 `state\<repoKey>\`；每个项目起一个 `run` 进程即可并行（锁按仓库）。用菜单选择：

```powershell
.\bin\steerline.exe launchers                 # 在 %STEERLINE_HOME% 生成双击入口 projects/viewer/run/stop/home
.\bin\steerline.exe projects                  # 交互式：选项目 + 选动作
.\bin\steerline.exe projects -action list     # 列出 etc\ 下的项目
```

`check` 只执行工具帮助、Git/Beads 读取、技能契约和 Gitea 仓库查询，可创建指定运行目录，不领取任务或调用模型。`run` 会真正领取任务并调用模型。目标仓库缺少 Beads 时直接报错，不执行 init 或迁移。

`merge-h-308-code` 已新增两个专用技能和合并后结项适配脚本，[接入配置](examples/merge-h-308-code.json) 已更新，既有手动技能保留原用途。真实启动前需将新文件合入 AIRefactor 并完成认证、执行权限及工具配置，见 [接入说明](docs/merge-h-308-integration.md)。本地 check 不会替代远端主线文件的发布。

Windows 的 npm `codex.cmd`/`codex.ps1` 不是原生可执行文件。配置应指向安装包里的 `codex.exe`，或使用 `node.exe` 加真实脚本路径的参数数组；服务拒绝隐式 shell shim。真实机器的原生 exe 路径已写入接入草案，升级 Codex 后应重新核对。

## 配置

完整例子见 [config.json](examples/config.json)。JSON 不支持注释，未知字段被拒绝；相对 repository/run_dir/worktree_root 路径相对配置文件目录，技能路径相对仓库根，执行时映射到新 worktree。可执行文件使用绝对路径或 PATH 中的原生命令。

| 字段 | 用途 / 默认值 |
| --- | --- |
| repository | 已存在的目标 Git 仓库；运行前解析真实路径 |
| remote / branch | 目标远端（默认 origin）和必须显式填写的主线 |
| run_dir / runtime_file | 仓库外的运行目录 / `runtime.json`；拒绝放入主 checkout 或任何 linked worktree |
| worktree_root | 默认 `<run_dir>/worktrees`；必须在已有 checkout 和 Git 元数据之外，不能与运行记录重叠 |
| claimant | 固定领取者；空值使用 Git common-dir 的稳定摘要。恢复时不得改变 |
| bd / git / codex | `{executable, args}`，不通过 shell 拼接任务文本 |
| start_skill / finish_skill | `{path, invocation}`，真实仓库入口，见 [技能契约](docs/skill-contract.md) |
| workflow | 传给仓库技能的顺序/衔接说明；服务不解释流程内部步骤 |
| executable_types | 默认 task、bug、feature、chore；仍排除已分配及 split aggregate |
| task_poll / pr_poll / heartbeat | `10s` / `30s` / `5s`，必须为正时长 |
| command_timeout / shutdown_grace | 短命令和 API 的超时 `30s` / 正常退出等待 worker `10s` |
| model / profile | 可选 Codex 模型和已有 profile，空值沿用本机设置 |
| sandbox | 默认 workspace-write；允许 read-only，以及用户明确授权后显式配置的 danger-full-access |
| pr | Gitea 原生只读 API，或可配置的规范化查询命令，见下文 |
| completion | 可选 `{command:{executable,args},timeout}`；默认 null 使用原生 close，自定义仓库通过它完成合并后的结项，详见 [命令协议](docs/completion-command.md) |

调度器使用本机 Git 准备 worktree，Codex 以该目录为工作根执行；技能不再创建或切换 worktree。技能仍可能需要访问共享 Spec、Beads 和仓库外报告目录，请在已审阅的 Codex profile 中配置必要权限和工具登录；服务不会为了让任务通过而自动提升权限。权限不足作为执行失败/blocked 保留。

## PR 查询

实际接入实现为 Gitea REST API：`GET /api/v1/repos/{owner}/{repo}` 和 `GET /api/v1/repos/{owner}/{repo}/pulls/{number}`。配置 `pr.kind = "gitea"`、实例 `base_url`、`repository = "owner/repo"`，认证只从 `token_env` 指定的环境变量读取。公开仓库可以省略 token_env；指定后要求变量非空。

私有接入例子的变量名为 `STEERLINE_GITEA_TOKEN`。在启动环境中通过现有凭据管理方式设置，勿将值放入配置、提交或命令参数。服务的 PR 查询只需读取权限；Codex 创建 PR 的认证沿用目标仓库技能的工具登录。服务不创建、审批、合并或关闭 PR。

校验 Git remote 与配置的实例和仓库，再验证结果中的 PR URL、查询返回的规范 URL、仓库、编号和 base branch。`merged` 必须有明确合并标志、合并时间和完整 merge commit；单纯 closed 不算完成。网络、权限、信息不足为 unknown 并重试；身份不符、目标主线错误或 closed_unmerged 不结项。

其他平台可设置 `pr.kind = "command"` 和 `pr.query = {"executable": "...", "args": [...]}`。参数项支持 `{pr_url}`、`{number}`、`{repository}` 占位符；替换后仍作为独立参数传递，不进入 shell。命令必须只读、退出 0，并在 stdout 返回一个 JSON 对象：

```json
{
  "url": "https://host/owner/repo/pulls/42",
  "repository": "owner/repo",
  "base_branch": "main",
  "state": "merged",
  "merged": true,
  "merged_at": "2026-09-17T08:30:00Z",
  "merge_commit": "0123456789abcdef0123456789abcdef01234567"
}
```

state 仅接受 open / merged / closed_unmerged / unknown。未知的合并时间和提交为 null。命令模式同时支持 URL 路径 `/pull/42` 与 `/pulls/42`；适配命令必须从平台直接查询，不能直接复述输入 URL 当作核验结果。M0 未提供或实测其他平台的专用适配器，Gitea 是已实现的直接接入路径。

## 运行和恢复

每个 Git common-dir 有一个 OS 文件锁，linked worktree、不同配置和不同 run_dir 都不能绕过它。锁文件位于共享 Git 元数据目录 `steerline.lock`，不进入业务提交；退出/崩溃后由 OS 释放，无需删除锁文件。run_dir 也加锁，防止不同仓库误写同一运行目录。默认 claimant 同样基于 common-dir，修改运行目录仍可核对遗失记录的领取。

有候选任务时先 fetch 配置的远端主线，用本次运行独立的 Git 引用固定 base_commit；fetch 失败不领取任务。领取前保存运行与 workspace_intent，领取成功后持久化，再创建 `<worktree_root>/<run_id>` 和 `codex/<task_id>-<run_id短码>` 分支。核验新目录、分支、HEAD、技能文件和共享 Beads 后保存实际准备结果，才启动 Codex。详细行为见 [worktree 准备与恢复](docs/worktree-preparation.md)。

bd 1.1.2 在领取冲突时可能退出 0 却不返回领取 JSON，因此会同时检查 update 响应和 show 的 ID、assignee、in_progress。worker 使用显式 BEADS_DIR 和 BEADS_ACTOR；从新 worktree 验证数据库身份和领取后才执行。只信任 Beads ready 的依赖就绪结果，不读取数据库内部文件。

启用 completion 时，将命令和 Beads 身份随派发记录固定下来。确认 PR 合并后，服务在该 worktree 中通过参数数组和 stdin JSON 调用命令，核对返回的任务/运行/PR 回执并重新读取 Beads closed。自定义回调失败不会退回普通 close，即使任务已经关闭也继续重试其尾部流程。merge-h-308-code 的适配器复用原有生命周期脚本，处理拆分与后继，并刷新核验 BV。

执行期间事件写入每次运行的 `events.jsonl`，stderr 单独写入 `stderr.log`；同目录的 `activity.jsonl` 是归一化后的稳定事件流（见下文“查看运行”）。运行快照只保存最近事件类型摘要（最多 512 字符），不累计对话。退出 0 后仍必须验证独立结构化结果；PR 绑定在 runtime 中持久保存，并拒绝同一 PR 被多个本地运行复用。

正常停止：前台按 Ctrl+C，或另一终端执行：

```powershell
.\bin\steerline.exe stop -config .\steerline.local.json
```

Windows 的 `service-window.bat` 从稳定安装目录运行 `bin\steerline.exe run -dispatch manual -console summary -service-window`。服务窗口可最小化，需在服务运行期间保持打开；窗口展示启动、ready、worker 与错误，状态摘要也写入 `run_dir\service\startups\<startup_id>\service.log`。服务进程生成 startup_id；只有匹配的 OS 身份和 `ready=true` 快照才算已连接。启动失败的原因写入同目录的 `startup.json` 与日志。旧 `run.bat` 和 CLI `run` 仍默认 auto。

Windows 的 `start-service -config ...` 在离线时返回 `window-required`、固定 launcher 和配置绝对路径；它不会从当前 UI/CLI 的 Job 中启动子服务。已有兼容服务时连接原实例，保留其派发与暂停状态。Linux 仍使用独立进程启动路径。

```powershell
# 明确停止服务，等待 stop 回执、原服务进程退出和仓库/run_dir 锁释放
.\bin\steerline.exe stop-service -config .\steerline.local.json
```

关闭 `watch` 或独立控制台不会发送停止请求；只要服务窗口仍开着，服务和 worker 继续运行。Ctrl+C 或 `stop-service` 请求正常退出；关闭服务窗口的 X 会终止本次服务，重新打开固定入口才会重启。连接会分别报告心跳过期、ready=false、PID 已复用、进程访问受限、配置摘要变化和协议不兼容，不会将未知进程当作已退出。

`run` 每次启动默认 `-dispatch auto`，保持既有自动调度；`-dispatch manual` 会刷新候选并继续处理当前运行、决策和 PR，但不自动领取新任务。控制客户端读取 runtime v4 后发布带实例 ID 和控制版本的不可变请求，服务完成后写持久回执：

```powershell
# 读取本服务最新 ready 候选
.\bin\steerline.exe tasks -config .\steerline.local.json

# 只启动指定任务；手动模式下重新核验 ready 后领取
.\bin\steerline.exe control -config .\steerline.local.json -action start_task -task-id <task-id> -wait

# 模式切换、暂停/恢复派发
.\bin\steerline.exe control -config .\steerline.local.json -action set_dispatch_mode -mode manual -wait
.\bin\steerline.exe control -config .\steerline.local.json -action pause_dispatch -wait
.\bin\steerline.exe control -config .\steerline.local.json -action resume_dispatch -wait

# 提交停服请求并等待控制回执（不等同于进程退出确认）
.\bin\steerline.exe control -config .\steerline.local.json -action stop_service -wait

# 一步提交并核验停服、原进程退出和两把服务锁释放
.\bin\steerline.exe stop-service -config .\steerline.local.json

# 用回执中的 request_id 再次查询原操作结果
.\bin\steerline.exe control -config .\steerline.local.json -request-id <request-id>
```

`start_task` 只领取所选任务；模式不符、暂停、占槽、任务失去 ready 或 claim 冲突会返回明确拒绝原因。暂停不冻结已启动 worker。`applied` 表示该指令定义的完成条件已持久化，任务是否交付仍查看 run 状态。控制请求和回执保存在 `control/`，不写入 activity 日志。

停止新派发，等待当前 worker 最多 shutdown_grace，超时则终止它的整棵进程树，保留领取并记录 failed/blocked。Windows 使用“挂起启动 → Job Object → 恢复线程”，避免进程在纳入 Job 前派生子进程；服务崩溃时 Job 句柄关闭也会清理后代。短命令同样有超时及进程树清理。Linux 提供进程组实现，但本次未实机验证 Linux，Windows 才是本次验收平台。

恢复规则：

- 等待合并：重启核对已准备工作目录，正常则恢复 PR 查询，绝不重新执行 Codex；目录/分支/起点异常时阻塞并保留现场。
- worktree 准备失败：保留领取、创建意图和已有目录/分支，failed/blocked，不自动重复创建；准备中崩溃则 unknown/blocked。
- PR 已合并但原生 close 或自定义 completion 失败：保持 waiting_pr_merge，持续重查 PR 并幂等重试。自定义命令必须完整成功，且 show 确认 closed 后才 completed；已经 closed 也不能跳过失败的尾部步骤。
- 遗留 pending/running：核对 PID、进程创建时间和可执行文件；变为 unknown/blocked，保留错误说明、日志和领取。无法确认旧执行已经结束时整个执行槽保守暂停。
- 有固定 claimant 的领取但没有本地运行记录：创建 unknown/blocked 恢复记录，不启动 worker。
- Beads 不可用：暂停新派发，runtime 报告错误并按轮询重试。关键快照写入失败：停止服务和新派发，退出报错，保留上一份完整快照。
- 执行失败、结果无效或未合并关闭：保留现场，不无限自动重跑。其他独立任务可以继续，unknown 则需要先由人确认旧进程已结束。

人工处理 unknown：先停止服务，核对进程树、日志和 Beads 领取；不要只按 PID 杀进程。确认旧执行完全停止后可解除执行槽保留：

```powershell
.\bin\steerline.exe mark-stopped -config .\steerline.local.json `
  -run-id <runtime中的run_id> -confirm-process-stopped
```

这只把执行状态改为 failed/blocked；任务仍被领取，不会自动重跑。按仓库原有技能人工修复/继续**同一个任务**，保持原 task_id/run_id 和领取者，取得正确 PR 后准备符合结果 Schema 的文件，再显式接回：

```powershell
.\bin\steerline.exe accept-result -config .\steerline.local.json `
  -run-id <runtime中的run_id> -result C:\Recovery\result.json -confirm-process-stopped
.\bin\steerline.exe run -config .\steerline.local.json
```

accept-result 在仓库锁下重新核验调度器准备记录、工作目录/分支/起点、结果和真实 PR，只接受指向目标主线的 open/merged PR，恢复等待合并状态；不会启动 Codex。它不能为准备未完成的任务、旧格式或孤儿记录补造工作环境事实。对未能匹配身份的进程，这个确认代表人工核验责任。两个人工命令均拒绝已知仍在运行的原进程。不要删除领取或运行记录来触发“自动重试”，不要编辑结果把不同任务的 PR 伪装成本任务。

如果是模型版本、权限等启动阶段失败，且工作目录完整、干净、HEAD 仍等于 base_commit、没有绑定 PR，可以在确认旧进程已停止后仅重试这一项：

```powershell
.\bin\steerline.exe retry -config .\steerline.local.json `
  -run-id <已有运行ID> -confirm-process-stopped
```

retry 复用同一个任务、领取和 worktree，使用当前配置的模型与权限。它不重新 claim、不根据 ready 派发其他任务，也不处理其他运行的 PR；启动检查仍可只读探测 Beads。旧日志原地保留，新日志写入 `runs/<run_id>/attempts/<attempt_id>`，其中 previous-run.json 保存上一次状态。失败/阻塞后停止；成功提 PR 后仅轮询该 PR，完成结项后停止。已有代码改动、提交、未知执行或其他仍存活的 worker 会拒绝重试，应走人工修复流程。

“并发 1”只表示最多一个 worker，正常 run 会在它退出后继续处理队列；需要限制本次只验证一个已有任务时使用 retry。retry 不是自动重跑机制。默认权限仍是 workspace-write，只有显式配置 sandbox=danger-full-access 才使用全访问。

## 查看运行

规格与实施计划统一见 [Spec + Plan 索引](docs/spec/README.md)；本轮设计见 [控制台升级总览](docs/spec/console-upgrade.md)。以下描述当前已实现行为。

每个运行有两份日志：`events.jsonl` 是 codex/pi 的原始事件流（取证，永不改动）；`activity.jsonl` 是归一化后的稳定记录，与 codex/pi 的 schema 解耦。两者都在 `runs/<run_id>/`（retry 在 `attempts/<attempt_id>/`）。契约见 [activity](docs/activity.md)。

```powershell
# 单终端：run 直接实时渲染（TTY 下默认 pretty）
.\bin\steerline.exe run -config .\steerline.local.json
.\bin\steerline.exe run -config .\steerline.local.json -console summary|json|off

# 第二终端或事后：TTY 下进入全屏查看器（run 列表 + activity + 决策面板）
.\bin\steerline.exe watch -config .\steerline.local.json
.\bin\steerline.exe watch -config .\steerline.local.json --no-tui   # 强制流式（管道/CI 自动如此）
.\bin\steerline.exe watch -config .\steerline.local.json -run-id <run-id|task-id> --follow=false
.\bin\steerline.exe watch -config .\steerline.local.json -run-id <run-id|task-id> --from-start=false
.\bin\steerline.exe watch -config .\steerline.local.json --json   # 稳定 JSONL
.\bin\steerline.exe watch -config .\steerline.local.json --raw    # 原始事件流

# 回答 agent 提出的人工决策（只写答案文件，不干扰运行中的服务）
.\bin\steerline.exe answer -config .\steerline.local.json -run-id <run-id|task-id> -option <option-id> [-note "..."] [-by "<name>"]
```

`watch` 只读 `runtime.json` 和运行文件，不需要服务在跑，也不改任何状态。单次调用固定开始时选中的 run 和日志路径；retry 会显示为新的 attempt，不会跟着 runtime 中的当前任务切换。读取器只提交换行结束的完整记录，支持 UTF-8 与 CRLF，遇到截断、文件替换或 activity `seq` 回退会清空旧代次后继续读。每批最多 256 行或约 1 MiB，单行上限 8 MiB；坏行诊断写入 stderr，JSON/raw stdout 不混入状态文字。若启动 watch 时 `activity.jsonl` 不存在（旧运行或原始宿主采集），会固定使用并归一化 `events.jsonl`；再次启动时会重新选择来源。TTY 下 `--follow=false` 或 `--from-start=false` 会使用流式输出，让这两个参数按请求生效。

查看器按 `run_id` 保持选中项，列表排序变化不会悄悄切换任务；切换 attempt、日志文件代次或读取源时会清空旧日志。runtime 读取失败会保留最后一次成功快照，并显示采样时间与错误；heartbeat 新鲜度独立于 runtime 中服务上次报告的状态。

Agent 需要人工决策时（`outcome=needs_decision`），运行进入 `finish_task_status=waiting_decision`：服务保留 claim/worktree/终端、暂停新派发并等待。`steerline answer` 原子写入答案文件，服务读回后在同一终端续跑。协议见 [decisions](docs/decisions.md)；Beads 与 SteerLine 三个状态轴的关系见 [状态模型](docs/state-model.md)。

执行在 Orca：`host: "orca"` 时由 Orca 创建 worktree 并启动 agent（`events.jsonl` 不再产生，改用 R-a 结果文件）。执行事实（worktree、分支、agent 存活、终端）以 Orca 为准；调度、编排、完成与恢复仍属 SteerLine。红线与规范见 [hosting](docs/hosting.md)、[hosting 规范](docs/spec/hosting-spec.md)。

后续方向已确认：中心 SteerLine 状态机、远端仅 Orca 执行器、桌面与移动端统一接入，并为同任务多 agent 独立 worktree 并行预留模型。详见[架构决策](docs/architecture-direction.md)；这些是目标约束，当前尚未实现。

```json
"agent": "codex",
"host": "orca",
"orca": {"executable": "orca", "shell": "cmd.exe", "wait_timeout": "15m", "keep_terminal": false, "fallback": "local"}
```

`host` 取 `orca`（默认，由 Orca 创建 worktree 并启动 agent）或 `local`（headless 本地进程，保留作回归基线）；`agent` 是交给 Orca `--agent` 的 id（默认 `codex`，可 `pi`/`claude`/…）。执行事实（worktree、agent 存活）以 Orca 为准；任务、完成、PR 核实仍以 Steerline + 平台为准。详见 [hosting 红线](docs/hosting.md)。

## 数据与保留

见 [数据契约](docs/data-contract.md)、[runtime 示例](examples/runtime.json)、[候选快照](examples/tasks.json)、[控制请求](examples/control-request.json)、[控制回执](examples/control-receipt.json) 和 [结果示例](examples/result.json)。运行快照现在为 schema_version=4，可读取 v1–v4；只有持锁服务保存时迁移旧快照。控制协议为 v1，候选任务快照为 v1。所有时间 UTC，未知值 null。runtime.json 只由服务写，控制客户端仅发布不可变请求并读取回执，Codex 仅写自己这次运行的结果文件。

同目录临时文件写完并 Sync 后，以 Windows MoveFileEx 原子替换发布；短暂 sharing violation 有限重试。替换失败保留旧快照并报错，不先删目标。读取器可能收到短暂共享冲突，应重试打开；一旦读到内容必须是完整 JSON。根据 heartbeat_at 判断是否过期，建议阈值至少为心跳间隔三倍并包含存储抖动余量。不要仅凭 service.status 推断进程还活着。

M0 不自动清理历史：runtime 保存全部运行概要，runs/<run_id> 保存 prompt、Schema、结果和日志，容量随任务数和日志量增长。停机后可备份并移走**已完成**运行的详细日志，保留 runtime 中的历史概要和 PR 绑定；未完成运行的文件不要清理。runtime 是恢复记录和外部读取出口，不是任务数据库。配置和日志应留在本机受控目录；程序不序列化认证环境变量。

worktrees/<run_id>、任务分支和 refs/steerline/bases/<run_id> 同样保留，包括准备失败或 fetch 后尚未领取留下的现场。不把 worktree 当日志目录自动删除；等待合并时保留目录但释放执行槽，后续任务从新获取的主线起步。

## 验证

```powershell
go test ./... -count=1 -timeout 10m
go vet ./...
```

测试使用原生替身命令和本地 HTTP Gitea 替身，不调用真实模型，不领取业务任务，不创建或合并业务 PR。覆盖空闲心跳、跨进程并发 JSON 读取、同仓库锁、领取冲突、单次执行、等待合并释放执行槽、合并结项、依赖释放、查询失败、错误关联、缺失/无效结果、重启防重、关闭重试、Beads 不可用、持久化失败及进程树退出。另有构建并启动真正 CLI 可执行文件、第二实例拒绝和 stop 正常退出的集成测试；临时仓库路径包含空格和中文。

PR 3 的 Windows 服务窗口路径已在当前宿主使用临时仓库、假 Beads/worker 和独立可见 console 定向验证：manual/ready、显式 worker、独立客户端退出后重连、控制 stop、Ctrl+C、窗口 X 关闭后的进程/worker 退出与重新打开，以及锁竞争和 preflight 失败。窗口模式不要求服务脱离祖先 Job；服务进程监测其 console 窗口，窗口消失时请求停服。全仓 `go test ./... -count=1 -timeout 10m` 通过；两个窗口从零并发仍按 PR 3 Plan 验收。

worktree 测试使用真实的临时 Git 仓库与本地 bare 远端，验证远端主线更新后新任务使用新提交、原 checkout 的分支/HEAD/文件保持不变、目录与分支冲突不覆盖、共享 Beads 身份错误或技能缺失时不启动、准备前后崩溃不重建不重跑，以及结构化结果不能改写工作环境事实。

completion 测试覆盖关闭后部分失败、重启重试、错误回执以及错误 PR/领取者不调用。目标仓库适配器测试命令为 `python -B -m unittest discover -s C:/Projects/merge-h-308-code/tools/Beads/tests -p test_steerline_completion.py -v`，只操作临时替身，不写真实 Beads。

真实环境只读验证了本机 CLI 帮助、目标 Beads JSON、BV JSON 形状、远端/技能配置及 Gitea 版本。专用技能和适配器已在本地新增并验证；真实带认证的 PR 查询、模型驱动的技能执行与 PR 出版、生产 PR 合并结项未执行，详情见接入说明。

接口参考：[Beads](https://github.com/gastownhall/beads)、[本机版本对应的 claim 实现](https://github.com/gastownhall/beads/blob/20e493e569c9/cmd/bd/update.go)、[Codex 非交互与结构化输出](https://learn.chatgpt.com/docs/non-interactive-mode)、[Gitea API](https://docs.gitea.com/api/1.24/)。具体命令能力还会在启动时通过本机 help 验证。
