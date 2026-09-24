# PR 3 Plan：服务窗口生命周期与连接

依据：[Spec](pr-03-service-lifecycle.spec.md)。依赖：PR 2。

实现状态：2026-09-24 调整 Windows 产品方案。现有分支已实现连接探测、控制和旧的 detached 启动，但 Windows 路径仍按旧目标尝试脱离 Job；新服务窗口模式尚未实现或验收。PR 保持 draft，不把旧测试通过或预期失败视为新 Spec 完成。

## 实施步骤

1. **复用连接与身份能力。** 核对现有 canonical common-dir、run_dir、配置摘要、进程身份、新鲜度和协议分类；保留只读 ConnectService、实例绑定的 StopService 与旧快照观察。Windows 离线态改为提供固定服务窗口入口，存活未知时仍禁止替代启动。
2. **增加 Windows 服务窗口入口。** 新增安装目录的 `service-window.bat`/快捷方式说明，以稳定 `bin/steerline.exe` 和绝对配置路径运行 `run -dispatch manual -console summary`；既有 `run.bat` 及 CLI `run` 的 auto 默认行为保持兼容。用户主动独立打开服务窗口；viewer/TUI 不从自身进程树启动服务。Windows `StartService` / `start-service` 对离线实例返回“需要打开服务窗口”及入口路径，停止旧 detached 子进程尝试。已有兼容实例仍可连接。Linux detached 路径保留平台隔离。
3. **让窗口启动具备诊断与状态。** 前台 `run` 为本次启动生成 startup_id；复用 `startup.json`、进程身份、ready 握手和启动日志。窗口显示初始化、ready、派发/暂停、活动 worker、最近错误和退出结果；详细日志落盘。preflight/锁失败在窗口和日志中给出一致原因。旧后台专用的 launch 锁仅在仍使用该路径的平台保留；Windows 以服务两把锁仲裁。
4. **完成停止与维护路径。** 控制指令正常停服时保留 applied 回执、原进程退出及锁释放证明。Ctrl+C 触发 `shutdown_grace` 路径；固定启动器显示退出原因和再次运行方式。窗口 X/强制结束按异常退出处理，不伪造 applied；下一次启动通过进程身份、锁和 runtime 安全恢复，核验 worker 不遗留。再次打开读取最新配置且不自动新领任务。
5. **调整客户端与文档。** 为 PR 4 提供“需要服务窗口”的结构化离线结果和固定入口说明；同步 README、状态模型、安装与操作说明。连接到旧 auto 服务时展示实际模式；旧 `run`/`stop` 入口保持可用。

## 验证计划

- 在当前 Windows 主机从安装目录的固定入口打开真实可见服务窗口，使用临时 Git 仓库、假 Beads/worker 和独立 run_dir，不操作生产服务。确认窗口的 summary、startup_id、日志、manual ready 与零新增 claim。
- 在服务和 worker 运行中关闭或强制结束独立 viewer/TUI，再重开连接；比较前后 service/worker 的完整进程身份及数量。窗口保持打开并可最小化。这是 L-A2/L-A3 的合并硬门槛，不以字段断言或仅查看任务状态代替。
- 同时打开两个服务窗口，覆盖相同配置及同 common-dir 不同 run_dir；核验唯一 ready、失败窗口的仓库锁原因与胜出实例未受影响。
- 注入 preflight 失败、锁失败、服务早退、初始化延期；核对窗口、startup.json、日志和客户端状态一致，不自动重启或假报在线。
- 分别用控制请求、Ctrl+C、窗口 X/强制结束来退出服务；检查回执真实性、原身份进程退出、两把锁释放、worker 收尾或异常恢复。重启窗口后确认重新加载配置、无重复 worker/旧请求。
- 复核 PID 复用、权限不足、stale heartbeat、配置/协议冲突和旧快照的只读路径。Linux 至少交叉编译；若无原生会话测试，明确标注验证边界。
- 旧的 Windows `STEERLINE_REQUIRE_DETACHED_TESTS=1` 和“成功脱离所有祖先 Job”的验收命令不再是合并门槛；旧用例改为覆盖 Windows 返回窗口入口及不误报托管。不得靠跳过新的服务窗口用例宣布通过。

## 现有证据与未完成项

- 旧实现的 `go test ./... -count=1 -timeout 10m` 已在 Windows 通过（服务包 373.271 秒）；Linux amd64 测试二进制和 CLI 曾交叉编译通过。这些结果只证明旧代码回归，不证明新的服务窗口目标。
- 当前主机中的直接启动、管理员进程和临时系统服务都处于 Job；带 `CREATE_BREAKAWAY_FROM_JOB` 的子进程仍在 Job 或创建被拒。原 Windows detached 路径因此按设计拒绝，并未取得成功后台存活证据。
- 旧的临时 `/IT` Task Scheduler TTY 测试已证明真实 stdin/console 窗口可用，以及旧 detached 启动的失败记录；它没有运行成功的服务/worker。另一个短暂的本机计划任务原型证明任务进程可在发起命令结束后继续运行，但本 Spec 选择服务窗口，不依赖计划任务。
- 现有 `run.bat` 已可双击打开前台服务窗口，但默认 `-console pretty` 且 `run` 默认 auto；需新增本方案的 `service-window.bat`，提供 manual/summary、启动诊断与维护文案。尚未在当前主机取得 L-A1～L-A9 的新方案完整证据。
- 连接分类、启动锁、pending startup_id、preflight 失败记录和 StopService 回执已有原生测试。保留可复用部分；对 Windows 启动结果和窗口生命周期补充真实验收。

## 完成条件

- L-A1～L-A9 在当前 Windows 主机取得真实服务窗口、worker、viewer 退出/重连、正常停服与异常关窗证据，尤其比较 service/worker 身份并核验锁与回执。
- Windows 离线客户端明确指向固定服务窗口入口；不再尝试旧 detached 启动或把 Job 内子进程误报为托管成功。
- Spec、Plan、README、状态模型及 PR 描述与最终行为一致；旧后台测试已更新到新目标。PR 审核通过前保持 draft，不合并，也不启动 PR 4。

## 风险与回退

服务窗口是 Windows 生命周期的可见边界。按 X 或强制结束可能来不及完成 `shutdown_grace`，因此必须依靠 worker Job 管理及下次启动的身份/锁/状态恢复；正常维护应使用控制 stop 或 Ctrl+C。若当前宿主禁止服务窗口内的 worker Job 创建，本 PR 必须报告具体失败并修复受控退出路径，不能以无法脱离 Job 为由跳过验收。旧前台 `run`/`stop` 仍是故障回退入口。
