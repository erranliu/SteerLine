# 全局执行配置

所有项目共享 `<STEERLINE_HOME>/execution.json` 中的执行方案。当前全局文件使用 `version: 2`；每个方案通过带实现 key 和 codec 版本的 `implementation_configs` 提交原生配置，配置载荷由对应实现严格解码。项目文件显式配置 execution_policy 和 publication_policy，定义宿主读取、none/workspace/host 写入、Git 元数据、工具网络和 escalation=deny；仓库、任务与 PR 仍归项目。`host`、`agent`、`execution_profiles` 和 `default_execution_profile` 都只能写在全局文件的方案中。每个项目仍有独立的服务和 native Task DAG。

先将 [执行配置示例](../examples/execution.json) 复制到 `<STEERLINE_HOME>/execution.json`，再按机器环境修改。至少配置一套方案；有多套方案且启用自动派发时，须指定 `default_execution_profile`。只有一套方案时，它自动成为默认方案。完整项目文件见 [项目示例](../examples/config.json)。

```json
{
  "version": 2,
  "default_execution_profile": "local-strong",
  "execution_profiles": [
    {
      "id": "local-strong",
      "name": "本地 Codex 高强度",
      "host": "local",
      "agent": "codex",
      "implementation_configs": [
        {
          "implementation_key": "codex.worker-profile",
          "codec_version": 1,
          "payload": {
            "executable": "<本机 Codex 可执行文件>",
            "model": "<明确模型>",
            "reasoning_effort": "xhigh",
            "windows_sandbox": "elevated"
          }
        }
      ]
    }
  ]
}
```

CLI 手动启动同样必须指定方案 ID，并通过相同配置摘要和服务回执校验：

```powershell
steerline control -config <project.json> -action start_task -task-id <id> -execution-profile <profile-id> -wait
```

按 `s` 打开全局方案列表，`↑/↓` 选择，`Enter` 确认，`Esc` 取消。服务在领取前核对方案 ID、摘要、Task 资格与目标能力；同一 Task 领取时冻结方案。编辑或删除全局方案不会重定向已开始的任务。控制台和服务加载的项目或全局文件内容不同时，控制台阻止新启动和自动派发；修改配置后重启服务并重新打开控制台。

Orca 本机 Codex 方案使用 `host=orca`、`agent=codex`，并在 `implementation_configs` 中分别提供 `codex.worker-profile` 与 `orca.worker-profile`。项目要求 workspace 写入、Git 元数据写入与网络 allow；Codex codec 校验 `windows_sandbox`（`elevated` 或 `unelevated`）及 `executable`，Orca codec 校验其 `executable`、PowerShell `shell` 和正的 `wait_timeout`。model 和 reasoning_effort 可分别省略；省略时保留启动器默认解析语义。

有无显式 model 都使用同一受管命令，冻结 `approval_policy=never`、Windows sandbox、严格配置校验及 linked worktree 写入根目录。原始 `codex.args` 当前只允许独立的 `--no-alt-screen`、`--no-daemon`、`--search`；子命令、配置/Profile 注入、权限覆盖与目录扩张均在资源创建前拒绝。领取前的 CLI/Orca 查询不初始化沙箱；领取后在原 terminal 上运行 readiness 工具探针，核对真实工具事件、原子收据及原生成功结算后，才派发一次业务任务。

控制台显示冻结权限、环境核验及阻断来源。宿主等待时，文字保留原目标而不注入菜单；中心选项可打开原 terminal 或重新检查。重新检查先对账原操作，不用新会话替换丢失会话，不延长旧准备期限。运行中的 Execution 不受配置文件编辑影响。其他 agent 缺少完整项目策略落实证据时不可领取。local 同样冻结策略、受理配置版本和精确资源；远程方案见下节。

本次已完成审计修复及隔离真机服务闭环；[验收记录](validation/worker-environment-readiness/README.md)区分真实工具证据、受控故障回归及已通过的真机服务闭环。

## 远程 Orca worker

远程方案与本地 Codex、本地 Orca 共用 Task、Decision、Operation 和 PR 交付规则。Run 与协调 inbox 位于中心，工作区与 agent 位于指定远端。首次创建远端工作区和 worker，后续轮次复用同一终端与 incarnation；不创建本地替代执行。

在全局文件的 `execution_profiles` 中添加以下方案，替换连接、仓库与协调终端的真实标识：

```json
{
  "id": "orca-remote",
  "name": "远程 Orca Codex",
  "host": "orca_supervised",
  "agent": "codex",
  "implementation_configs": [
    {
      "implementation_key": "codex.worker-profile",
      "codec_version": 1,
      "payload": {"executable": "<远端使用的 Codex 命令>", "model": "<明确模型>", "reasoning_effort": "high"}
    },
    {
      "implementation_key": "orca.worker-profile",
      "codec_version": 1,
      "payload": {"executable": "<本机 Orca 原生可执行文件>", "shell": "powershell.exe", "wait_timeout": "2m"}
    },
    {
      "implementation_key": "orca.supervised-profile",
      "codec_version": 1,
      "payload": {
        "connection_id": "<environment list 返回的 pairedDeviceId>",
        "environment_id": "<已保存的 environment ID>",
        "repository_selector": "id:<远端 repo ID>",
        "coordinator_terminal": "<中心已打开的 bootstrap terminal handle>",
        "poll_interval": "2s",
        "capability_report_version": 1,
        "remote_cancel_available": false
      }
    }
  ]
}
```

远程协议与身份链路保留，但目前 worker-start 未返回绑定实际目标/版本的文件、网络及越权策略证据，所以所请求 execution_policy 在领取前明确拒绝。原先的连接/版本族/remote/federation 检查仍属于远程适配；不能据此推定权限落实，不降级到本机。配置中的协调终端用于预检，每次 WorkflowRun 另建独立的中心协调终端。远端必须已具备 Codex、项目工具和 PR 发布权限。

原生 question 进入共享 Decision；活动 Dispatch 的反馈通过消息投递，已结算轮次的反馈通过原终端开启下一轮。输入回执丢失时只核对原 Task/Dispatch 或 question history，不重复启动或回答。中心重启保留原执行并重新核验，旧服务保存的未投递输入需要重新确认；断连或身份变化保留资源并显示未核实。远端路径按执行主机返回值保存，不按中心操作系统转换。

`worker_done` 只证明轮次结算。提供方证明对应 PR 已合并后才写 TaskDelivery、关闭 Task，并 release 已结算 worker 和关闭排空的中心协调终端；远端 worktree 保留。远程取消仍不可用，停止中心监督不会被显示为远端进程已经退出。

当前实现已通过隔离协议与 Service 测试。2026-09-30 保存的“台式机64g”连接不可达，本次未完成双机真机验收。

## Windows 本地 Codex 沙箱

Windows 本地受限方案将 `codex.windows_sandbox` 固定为 `elevated`（省略时采用该默认值），并随执行方案摘要及 WorkflowRun 一起冻结。原生 app-server 用 `--config windows.sandbox=elevated` 启动，不继承用户全局配置中的 `unelevated`。`workspace-write` 仍只开放原工作区和经过核验的 Git 元数据目录，不扩大为整个仓库目录或全权限。

显式 `unelevated` 只可用于本地 `read-only`；本地 `workspace-write` 使用 linked worktree 的分散可写根，unelevated 无法落实此策略，控制台标记方案不可用、领取入口提前拒绝。`codex.args` 不能重复覆盖受管的 Windows sandbox 值。非 Windows 启动不注入 Windows 参数；Orca 的受管启动和就绪流程见上文。

配置校验和成功创建 app-server 不证明实际工具已可执行。elevated 沙箱须在这台机器的原生 Codex 入口完成必要的系统设置；SteerLine 不自动配置 Windows 账户、ACL 或防火墙，不自动回退到 unelevated 或 danger-full-access。设置方式见[官方 Windows sandbox 文档](https://learn.chatgpt.com/docs/windows/windows-sandbox)。原有运行保持冻结权限，不通过编辑全局文件更换已有会话的沙箱方式。

## 项目策略与配置受理

项目文件必须显式包含以下策略，缺失、null、遗漏 git_metadata_write 或旧顶层 sandbox/windows_sandbox 均拒绝。全局 native Codex profile 覆盖也拒绝；不迁移旧执行。

```json
{"execution_policy":{"filesystem":{"read_scope":"host","write_scope":"workspace","git_metadata_write":true},"network":"allow","escalation":"deny"}}
```

当前映射绑定 Codex 0.159.2 的[原生配置 schema](https://raw.githubusercontent.com/openai/codex/rust-v0.159.2/codex-rs/core/config.schema.json)和本机生成的 app-server schema，领取前只运行固定短命版本查询。支持 Windows local elevated 与 Orca 显式 elevated/unelevated 的上述策略；local 的 none + network deny 也有 readOnly 映射，可显式选择 unelevated。其余未经核实的组合明确不可用，包括 host 写入与 network deny。不把任意 runtime 或宿主临时目录加入写根。

原始参数仅接受独立的 --no-alt-screen、--no-daemon、--search；长短选项、等号和分离值形式的权限、目录、model/effort、配置/Profile、子命令或未知参数均拒绝。model 与 effort 独立：Orca 可分别省略，local 保留 high 默认；原生观察到的默认模型不是固定要求。

领取时冻结 FixedExecutionProfile v5 的策略、来源、受理配置版本、实现配置封套及完整摘要；资源阶段以 PolicyResources v3 保存通用 workspace/Git 根与 Codex codec 资源封套，沿用现有原子附件事务。准备、业务轮、反馈与恢复输入检查原快照/资源，不从运行中的仓库修改或新配置文件补造授权。重新加载由现有服务入口显式受理，对新 Execution 生效；已有执行保持原值。runtime checkpoint 当前格式为 12，旧格式拒绝且保留现场。

控制台由同一 ProjectView 的 checkpoint/Domain 显示冻结要求、版本、来源、资源和核验范围。设置核验与实际工具核验分开；未观察的工具值显示未核验。服务与控制台配置摘要不同或读取部分错误时阻止新启动。新策略原生工具证据见[本轮验收](validation/execution-isolation-06-policy/README.md)。


## 项目发布责任

项目必须配置 `"publication_policy":{"publisher":"worker"}`。worker 统一负责 commit、push 和创建 PR；中心只查询、核验合并、提交唯一 TaskDelivery，以及按原身份关闭未合并 PR/清理资源。local、Orca 与具备真实权限证据的远程执行均使用这个责任边界。全局执行方案不再接受 publication_policy；host_service 和 execution_profile 均明确拒绝。

worker 的执行策略和本身凭据必须允许 Git 推送及 PR 创建；中心的 token_env 只用于查询/关闭，不自动下发给 worker。仅有 query 或成功创建会话不能证明发布能力。worker 的成功、URL 和 worker_done 均不代替 provider 的真实 merged 证据。

冻结快照保留 publisher、原 provider/查询与关闭命令、Git、单一 fetch/push URL、base 与认证引用。后续轮次、恢复、PR 查询/关闭/交付使用原快照；远程目标变化仍拒绝副作用。秘密值不入库或写入产物。当前 SQLite schema 12、runtime/checkpoint schema 17 明确拒绝旧开发数据，不迁移、回填或回退。历史真机报告保留原始证据格式。

环境阻断是执行状态旁路，保留业务问题及本地草稿。阻断时新回答明确拒绝；未产生投递意图的已保存回答作废并保留审计，恢复后重新提交。已有未知投递只核对原回执。控制台 Ctrl+O 打开原终端，Ctrl+E 重新检查原环境；CLI 的 control -action open_terminal/recheck_environment -run-id <RunID> 使用同一精确 blocker/turn/attempt/version 目标。

会话和活动只显示 SQLite 已采集的规范记录。采集缺口显示诊断，原始文件仅供手动排障，不自动补画或补投输入。

## 附加执行需求与注册

项目可配置 `execution_requirements`，省略或空对象表示没有附加需求。权限、发布责任及 worker-flow 原有预检仍强制执行。

```json
{
  "execution_requirements": {
    "capabilities": ["workspace_inspection"],
    "tools": [{"id": "git"}],
    "credentials": [{"id": "pr.token_env", "site": "service"}]
  }
}
```

能力只消费经过内容绑定核验、目标/agent/backend/协议/版本完全相符的当前报告。版本查询或受管 CLI 支持不生成 verified；没有报告、unsupported、not_tested 或未知 ID 都拒绝。此例需要原目标的 workspace_inspection 认证报告；没有该证据时不能领取。

工具注册 `git`、`go`、`python`，只执行固定版本查询。省略 `exact_version` 只要求存在；声明后精确比较归一版本，例如 Git `2.51.0.windows.1`、Go `1.26.4`、Python `3.13.1`。解析失败为 unknown。Git 使用原冻结 executable/argv；local 的 go/python 使用与实际 worker 相同的认证引用剔除环境及 PATH，Go 查询固定 GOTOOLCHAIN=local，防止下载工具链。它检查已安装工具链，不断言模块构建会选择同一版本。本机 Orca 只可证明冻结的宿主工作区 Git；终端 PATH 无固定只读查询，因此 go/python 为 unknown。远程工具查询尚无对应协议，全部为 unknown，不能使用中心查询代替。不能声明自定义检查命令。

首版认证引用只注册 `pr.token_env` 的 service site，使用原冻结 provider 的环境变量名。未声明 token_env 的 command provider 即使宿主已登录也不满足此引用；不要求所有项目配置 token。target site 和未知引用拒绝。结果只保存 present/missing/unknown、来源、检查器版本、目标和时间，不保存秘密值或摘要，也不证明登录、远端授权或 worker 沙箱权限。

内建注册键为 `local/codex/local_codex_app_server_v1`、`orca/codex/orca_terminal_v2`、`orca/codex/orca_supervised`；具体目标单独绑定。未知、重复或歧义组合拒绝，注册是固定代码组装，没有插件加载。准备、启动、观察/停止构造、事件/结算、等待/反馈、恢复与清理都由原记录选择注册操作。

FixedExecutionProfile v5 在同一快照中保存需求、非秘密评估、独立证据摘要及 `builtin-execution-v2` 实现配置版本。完整配置摘要包含需求及原权限/发布/Profile 配置，实际资源继续单独核验。证据摘要也纳入完整 Profile 摘要。原 worktree 根从持久 WorkspaceIntent 恢复。领取事务重新读取已受理的项目和全局文件字节摘要；查验后改变配置必须重新加载并检查。重启使用原快照，当前工具/引用丢失只新增 Execution 的阻断事实，不改历史快照。控制台使用同一 ProjectView 展示领取时与当前检查的时间和来源。checkpoint schema 12 及资源格式 v3 生效；旧 Profile 明确拒绝，不回填。

验证与限制见 [#61 验收记录](validation/execution-isolation-08-resolution/README.md)。远程两轮 Service 成功场景仍被前置权限缺证门槛拒绝，未把协议 mock 当作权限或双机认证。
