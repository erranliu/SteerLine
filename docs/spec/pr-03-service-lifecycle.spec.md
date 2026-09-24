# PR 3 Spec：服务窗口生命周期与连接

状态：2026-09-24 调整 Windows 产品方案；本分支的服务窗口路径已在当前 Windows 宿主通过本 Spec 验收，待 PR 审核。Linux 已交叉编译，原生会话生命周期尚未实测。依赖：PR 2。对应 [Plan](pr-03-service-lifecycle.plan.md)。

## 目标与生命周期边界

Windows 使用用户主动打开的可见“SteerLine 服务窗口”运行服务。窗口可最小化，显示简要状态；服务运行期间窗口保持打开，服务退出后启动器可留在窗口中显示结果。控制台 TUI、viewer 和 watch 是独立客户端，关闭它们不停止仍开着的服务窗口。关闭服务窗口会结束服务，不能继续显示为在线。本 PR 不要求 Windows 服务进程脱离所有 Job Object。

服务窗口必须由用户从稳定安装目录的启动器或桌面快捷方式独立打开，不能由可能随 UI 一起结束的进程直接创建后声称已托管。当前 Windows 宿主禁止子进程从祖先 Job 脱离；可见窗口本身不解除该限制。本方案通过明确保留服务窗口来界定存活期，不引入计划任务或 Windows 系统服务。

## L1：服务连接与实例探测

连接结果同时包含最后快照、新鲜度、进程探测和控制协议能力，不压缩成一个 online 布尔值。

| 观察结果 | 展示与动作 |
| --- | --- |
| 新鲜快照、身份匹配、ready=true、协议兼容 | 已连接，允许提交相应能力支持的控制指令 |
| 身份匹配但 heartbeat 过期或 ready=false | 服务异常/初始化中，保留原实例；禁止自动另起服务窗口 |
| 进程确定不存在，或 PID 已属于另一个进程 | 离线，旧运行状态按历史展示；Windows 提示从固定入口打开服务窗口 |
| 权限不足或身份无法核验 | 存活未知，展示原因；不把它当成进程已死 |
| 无 runtime | 未启动/无记录；Windows 展示服务窗口启动入口与配置路径 |
| 状态文件损坏、版本不支持或身份不匹配 | 显示诊断；不能覆盖状态或假装启动成功 |

进程探测 API 返回确定不存在、身份匹配、身份不匹配和不可核验四类。连接核对 repository identity、run_dir、remote/branch、claimant、配置摘要和控制协议。配置变化时保留观察及明确实例的 stop，禁用其他变更动作；旧 v1–v3 快照仅只读观察，缺少证明时不给控制能力。

## L2：Windows 服务窗口启动

- 稳定入口为新增的 `<STEERLINE_HOME>/service-window.bat <配置路径>` 或指向它的快捷方式。它使用安装目录中的 `bin/steerline.exe`，以绝对配置路径运行 `run -config <路径> -dispatch manual -console summary -service-window`；用户可选择更详细的 `pretty` 输出。缺少安装文件或配置时，窗口显示明确错误并留给用户查看。既有 `run.bat` 和 CLI `run` 的 auto 默认行为保持兼容。
- 服务进程在该窗口前台运行。窗口显示初始化、ready、派发/暂停模式、活动 worker、停止结果和最近错误；详细诊断写入运行目录的启动日志。窗口不争用 viewer 的 TUI 屏幕。
- 通过新入口启动的实例默认 manual，不自动领取新 ready 任务；连接已存在服务时保留其实际 manual/auto 和暂停状态。服务从固定入口启动，不能静默挑选工作目录里的旧 exe。
- Windows 的 `StartService` / `start-service` 对离线实例返回明确的“需要打开服务窗口”结果及固定入口，不从调用它的 UI/CLI 中创建 detached 子进程；若已有兼容实例则连接它。`ConnectService` / `StopService` 继续作为共享客户端能力。Linux 已有 detached 启动路径可保留，按平台分别展示能力。
- 服务取得 repository/run_dir 两把锁后才可 ready。同配置或同仓库不同 run_dir 的第二个窗口必须显示冲突并退出自己的服务进程；不得覆盖已有实例或把冲突仓库误判为自己的服务。
- worker 仍由服务管理，其 Windows Job Object 用于服务退出时清理 worker；服务窗口启动无需 `CREATE_BREAKAWAY_FROM_JOB`。必须在当前宿主实测 worker 的 Job 创建与退出。

## L3：启动记录与确认

- 服务窗口本身为每次启动生成唯一 startup_id，并在自己的 `service/startups/<startup_id>/startup.json` 与 `service.log` 记录初始化、成功或失败及实例身份。记录只表示这次尝试，不替代 runtime/任务状态权威。
- ready 只在取得两把锁、读取/恢复快照、完成 preflight、控制 checkpoint 和指令消费初始化后发布。manual 启动不新领任务；既有 PR 核验与已提交答案仍按原语义继续。
- viewer 从 runtime 读取 startup_id，并通过匹配的启动记录、进程身份、ready=true 与协议能力确认连接。窗口尚在初始化时显示“启动中”；超时或观察中断显示“结果待确认”，保留实例与诊断，不自动另起窗口。
- 同时打开两个服务窗口时至多一个 ready。失败窗口保留可见原因和日志位置；用户关闭它不影响胜出的实例。

## L4：停止、关闭与重启

- `StopService` 提交 PR 2 的 stop_service。只有 applied 回执、原进程退出及两把锁释放都核验后，才显示可重新启动；短暂收尾显示“停止收尾中”。
- 退出 viewer、watch、控制台 TUI、换项目或读取超时都不发送 stop；服务窗口与 worker 保持原身份运行。
- 在服务窗口按 Ctrl+C 应走可观察的正常退出路径，等待当前 worker 按 `shutdown_grace` 收尾并保存状态。固定启动器在服务退出后显示退出原因和再次运行方式，便于维护。
- `-service-window` 要求可见 console 并监测其窗口句柄；点击窗口的关闭按钮使窗口消失后，服务请求退出并清理 worker。强制结束进程也属于异常退出：两者都不得伪造 stop applied。下次打开通过真实进程身份、锁和 runtime 进行恢复/诊断；worker 不得成为无人管理的遗留进程。
- 服务窗口退出后由用户重新打开固定入口才重启；新实例重新加载配置，不自动重复执行旧实例的控制请求或新领任务。对停服造成的 worker 终止继续保留原有 failed/blocked、claim 与 worktree 语义。

## L5：兼容与范围

- 前台 `run`、`stop` 和批处理入口继续可用；Windows 服务窗口模式是本 PR 的受支持运行方式。服务窗口需要保持打开，可最小化；不保证关闭该窗口、注销或重启后继续运行，也不提供开机自启或跨用户管理。
- 本 PR 不引入计划任务、Windows 系统服务或要求安装虚拟机。若系统禁止当前 worker 的 Job 管理，启动必须显示具体失败原因，不能显示假在线。
- 启动沿用现有 preflight；工具、技能或 Orca 不可用时提供窗口提示与启动日志。旧二进制缺少控制能力时显示不兼容，不能直接编辑 runtime 升级活跃实例。
- Linux detached 路径不改变 Windows 服务窗口的验收语义；Linux 若无原生实测需在文档中标明。

## 验收标准

| ID | 场景与结果 |
| --- | --- |
| L-A1 | 从固定入口打开真实 Windows 服务窗口，取得匹配 startup_id、进程身份和 ready 快照；默认 manual，未新领 ready 任务，窗口可见状态与日志一致 |
| L-A2 | 服务窗口保持打开时，关闭或强制结束独立 viewer/TUI，服务及运行中的 worker 保持同一身份存活 |
| L-A3 | 重开 viewer/TUI 连接原实例，服务和 worker 数量未增加，模式及暂停状态未被重置 |
| L-A4 | 同配置及同仓库不同 run_dir 同时打开服务窗口，至多一个服务 ready；失败窗口显示真实锁冲突 |
| L-A5 | preflight 失败、服务早退、初始化超时均显示真实窗口状态、startup 记录和日志，不显示假在线或自动重启 |
| L-A6 | PID 复用、权限不足、stale heartbeat 分别正确分类；未知不触发替代启动 |
| L-A7 | 控制 stop 有 applied 回执；Ctrl+C 可核验服务/worker 退出及锁释放；关闭 viewer 不停服；强制关闭服务窗口不伪造 applied 回执，重开后能安全恢复 |
| L-A8 | 旧快照只读可看；协议/配置冲突不会向错误实例提交动作 |
| L-A9 | 服务窗口退出后重新打开会重读配置并建立新实例，不重复旧请求、不遗留 worker，窗口显示重启结果 |
