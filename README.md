# Steerline M0

Steerline 是面向持续软件工程的本机调度器：读取既有 Beads 可执行任务，原子领取后启动一个 `codex exec`，让仓库技能完成开发和提 PR；人工合并后，服务验证 PR 事实再关闭 Beads 任务，释放依赖。

本实现优先支持 Windows 原生运行。没有 Web 服务、新任务数据库、独立 worker、自动规划、多机调度、自动修复/回退或自动合并。worker 就是 Codex 进程。代码、worktree、测试、审计、提交和创建 PR 都由目标仓库的技能负责。

## 构建和启动

要求 Go 1.24+、Git、Beads、Codex CLI，以及已配置 Beads 和符合接入契约的目标仓库。本机验证版本：Go 1.26.4、bd 1.1.2、codex-cli 0.145.0。

```powershell
Set-Location C:\Project\SteerLine
go build -o .\bin\steerline.exe .\cmd\steerline
Copy-Item .\examples\config.json .\steerline.local.json
# 编辑 steerline.local.json，填写实际仓库、技能、主线、运行目录、工具和 PR 查询配置。
.\bin\steerline.exe check -config .\steerline.local.json
.\bin\steerline.exe run -config .\steerline.local.json
```

`check` 只执行工具帮助、Git/Beads 读取、技能契约和 Gitea 仓库查询，可创建指定运行目录，不领取任务或调用模型。`run` 会真正领取任务并调用模型。目标仓库缺少 Beads 时直接报错，不执行 init 或迁移。

**提供的 `merge-h-308-code` 仓库目前存在技能和自定义结项语义冲突。** 已生成 [实际配置草案](examples/merge-h-308-code.json) 和 [接入诊断及最小调整](docs/merge-h-308-integration.md)，原技能未改动。草案当前被 `check` 拒绝是预期行为，不能直接启动业务调度。

Windows 的 npm `codex.cmd`/`codex.ps1` 不是原生可执行文件。配置应指向安装包里的 `codex.exe`，或使用 `node.exe` 加真实脚本路径的参数数组；服务拒绝隐式 shell shim。真实机器的原生 exe 路径已写入接入草案，升级 Codex 后应重新核对。

## 配置

完整例子见 [config.json](examples/config.json)。JSON 不支持注释，未知字段被拒绝；相对 repository/run_dir 路径相对配置文件目录，技能路径相对仓库根。可执行文件使用绝对路径或 PATH 中的原生命令。

| 字段 | 用途 / 默认值 |
| --- | --- |
| repository | 已存在的目标 Git 仓库；运行前解析真实路径 |
| remote / branch | 目标远端（默认 origin）和必须显式填写的主线 |
| run_dir / runtime_file | 仓库外的运行目录 / `runtime.json`；拒绝放入主 checkout 或任何 linked worktree |
| claimant | 固定领取者；空值使用 Git common-dir 的稳定摘要。恢复时不得改变 |
| bd / git / codex | `{executable, args}`，不通过 shell 拼接任务文本 |
| start_skill / finish_skill | `{path, invocation}`，真实仓库入口，见 [技能契约](docs/skill-contract.md) |
| workflow | 传给仓库技能的顺序/衔接说明；服务不解释流程内部步骤 |
| executable_types | 默认 task、bug、feature、chore；仍排除已分配及 split aggregate |
| task_poll / pr_poll / heartbeat | `10s` / `30s` / `5s`，必须为正时长 |
| command_timeout / shutdown_grace | 短命令和 API 的超时 `30s` / 正常退出等待 worker `10s` |
| model / profile | 可选 Codex 模型和已有 profile，空值沿用本机设置 |
| sandbox | 默认 workspace-write，也允许 read-only；不默认绕过审批与沙箱 |
| pr | Gitea 原生只读 API，或可配置的规范化查询命令，见下文 |

Codex 的技能可能需要在仓库外创建 worktree、运行目录或报告。请在已审阅的 Codex profile 中配置必要权限和工具登录；服务不会为了让任务通过而自动提升权限。权限不足作为执行失败/blocked 保留。

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

领取前先写 durable intent，领取成功后再保存状态，然后启动 Codex。bd 1.1.2 在领取冲突时可能退出 0 却不返回领取 JSON，因此会同时检查 update 响应和 show 的 ID、assignee、in_progress。只信任 Beads ready 的依赖就绪结果，不读取数据库内部文件。

执行期间事件写入每次运行的 `events.jsonl`，stderr 单独写入 `stderr.log`。运行快照只保存最近事件类型摘要（最多 512 字符），不累计对话。退出 0 后仍必须验证独立结构化结果；PR 绑定在 runtime 中持久保存，并拒绝同一 PR 被多个本地运行复用。

正常停止：前台按 Ctrl+C，或另一终端执行：

```powershell
.\bin\steerline.exe stop -config .\steerline.local.json
```

停止新派发，等待当前 worker 最多 shutdown_grace，超时则终止它的整棵进程树，保留领取并记录 failed/blocked。Windows 使用“挂起启动 → Job Object → 恢复线程”，避免进程在纳入 Job 前派生子进程；服务崩溃时 Job 句柄关闭也会清理后代。短命令同样有超时及进程树清理。Linux 提供进程组实现，但本次未实机验证 Linux，Windows 才是本次验收平台。

恢复规则：

- 等待合并：重启只恢复 PR 查询，绝不重新执行 Codex。
- PR 已合并但 bd close 失败：保持 waiting_pr_merge，持续重查 PR 并幂等重试 close，show 确认 closed 后才 completed。
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

accept-result 在仓库锁下重新验证结果和真实 PR，只接受指向目标主线的 open/merged PR，恢复等待合并状态；不会启动 Codex。对未能匹配身份的进程，这个确认代表人工核验责任。两个人工命令均拒绝已知仍在运行的原进程。不要删除领取或运行记录来触发“自动重试”，不要编辑结果把不同任务的 PR 伪装成本任务。

## 数据与保留

见 [数据契约](docs/data-contract.md)、[完整 runtime 示例](examples/runtime.json)、[结果示例](examples/result.json)。所有时间 UTC，未知值 null。runtime.json 只由服务写，Codex 仅写自己这次运行的结果文件。

同目录临时文件写完并 Sync 后，以 Windows MoveFileEx 原子替换发布；短暂 sharing violation 有限重试。替换失败保留旧快照并报错，不先删目标。读取器可能收到短暂共享冲突，应重试打开；一旦读到内容必须是完整 JSON。根据 heartbeat_at 判断是否过期，建议阈值至少为心跳间隔三倍并包含存储抖动余量。不要仅凭 service.status 推断进程还活着。

M0 不自动清理历史：runtime 保存全部运行概要，runs/<run_id> 保存 prompt、Schema、结果和日志，容量随任务数和日志量增长。停机后可备份并移走**已完成**运行的详细日志，保留 runtime 中的历史概要和 PR 绑定；未完成运行的文件不要清理。runtime 是恢复记录和外部读取出口，不是任务数据库。配置和日志应留在本机受控目录；程序不序列化认证环境变量。

## 验证

```powershell
go test ./... -count=1 -timeout 180s
go vet ./...
```

测试使用原生替身命令和本地 HTTP Gitea 替身，不调用真实模型，不领取业务任务，不创建或合并业务 PR。覆盖空闲心跳、跨进程并发 JSON 读取、同仓库锁、领取冲突、单次执行、等待合并释放执行槽、合并结项、依赖释放、查询失败、错误关联、缺失/无效结果、重启防重、关闭重试、Beads 不可用、持久化失败及进程树退出。另有构建并启动真正 CLI 可执行文件、第二实例拒绝和 stop 正常退出的集成测试；临时仓库路径包含空格和中文。

真实环境只读验证了本机 CLI 帮助、目标 Beads JSON、远端/技能配置及 Gitea 版本。真实带认证的 PR 查询、实际 Codex 开发流程、生产 PR 合并结项未执行；技能适配尚未完成，详情见接入诊断。

接口参考：[Beads](https://github.com/gastownhall/beads)、[本机版本对应的 claim 实现](https://github.com/gastownhall/beads/blob/20e493e569c9/cmd/bd/update.go)、[Codex 非交互与结构化输出](https://learn.chatgpt.com/docs/non-interactive-mode)、[Gitea API](https://docs.gitea.com/api/1.24/)。具体命令能力还会在启动时通过本机 help 验证。
