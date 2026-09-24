# PR 3 Plan：服务窗口生命周期与连接

依据：[Spec](pr-03-service-lifecycle.spec.md)。依赖：PR 2。

实现状态：PR #14 已合并。服务窗口入口、Windows `window-required` 结果、启动状态/日志和窗口消失监测已交付；真实窗口 Ctrl+C、X/重启及两个窗口从零并发均已在当前宿主通过。合并主线后的全仓回归通过，后续质量审计修复已通过定向回归、静态检查和交叉编译。

## 实施步骤

1. **复用连接与身份能力。** 核对现有 canonical common-dir、run_dir、配置摘要、进程身份、新鲜度和协议分类；保留只读 ConnectService、实例绑定的 StopService 与旧快照观察。Windows 离线态改为提供固定服务窗口入口，存活未知时仍禁止替代启动。
2. **增加 Windows 服务窗口入口。** 新增安装目录的 `service-window.bat`/快捷方式说明，以稳定 `bin/steerline.exe` 和绝对配置路径运行 `run -dispatch manual -console summary -service-window`；既有 `run.bat` 及 CLI `run` 的 auto 默认行为保持兼容。用户主动独立打开服务窗口；viewer/TUI 不从自身进程树启动服务。Windows `StartService` / `start-service` 对离线实例返回“需要打开服务窗口”及入口路径，停止旧 detached 子进程尝试。已有兼容实例仍可连接。Linux detached 路径保留平台隔离。
3. **让窗口启动具备诊断与状态。** 前台 `run` 为本次启动生成 startup_id；复用 `startup.json`、进程身份、ready 握手和启动日志。窗口显示初始化、ready、派发/暂停、活动 worker、最近错误和退出结果；详细日志落盘。preflight/锁失败在窗口和日志中给出一致原因。旧后台专用的 launch 锁仅在仍使用该路径的平台保留；Windows 以服务两把锁仲裁。
4. **完成停止与维护路径。** 控制指令正常停服时保留 applied 回执、原进程退出及锁释放证明。Ctrl+C 触发 `shutdown_grace` 路径；固定启动器显示退出原因和再次运行方式。`-service-window` 监测可见 console，窗口 X 使窗口消失后请求退出；强制结束进程也不得伪造 applied。下一次启动通过进程身份、锁和 runtime 安全恢复，核验 worker 不遗留；再次打开读取最新配置且不自动新领任务。
5. **调整客户端与文档。** 为 PR 4 提供“需要服务窗口”的结构化离线结果和固定入口说明；同步 README、状态模型、安装与操作说明。连接到旧 auto 服务时展示实际模式；旧 `run`/`stop` 入口保持可用。

## 验证计划

- 在当前 Windows 主机从安装目录的固定入口打开真实可见服务窗口，使用临时 Git 仓库、假 Beads/worker 和独立 run_dir，不操作生产服务。确认窗口的 summary、startup_id、日志、manual ready 与零新增 claim。
- 在服务和 worker 运行中关闭或强制结束独立 viewer/TUI，再重开连接；比较前后 service/worker 的完整进程身份及数量。窗口保持打开并可最小化。这是 L-A2/L-A3 的合并硬门槛，不以字段断言或仅查看任务状态代替。
- 同时打开两个服务窗口，覆盖相同配置及同 common-dir 不同 run_dir；核验唯一 ready、失败窗口的仓库锁原因与胜出实例未受影响。
- 注入 preflight 失败、锁失败、服务早退、初始化延期；核对窗口、startup.json、日志和客户端状态一致，不自动重启或假报在线。
- 分别用控制请求、Ctrl+C、窗口 X/强制结束来退出服务；检查回执真实性、原身份进程退出、两把锁释放、worker 收尾或异常恢复。重启窗口后确认重新加载配置、无重复 worker/旧请求。
- 复核 PID 复用、权限不足、stale heartbeat、配置/协议冲突和旧快照的只读路径。Linux 至少交叉编译；若无原生会话测试，明确标注验证边界。
- 旧的 Windows `STEERLINE_REQUIRE_DETACHED_TESTS=1` 和“成功脱离所有祖先 Job”的验收命令不再是合并门槛；旧用例改为覆盖 Windows 返回窗口入口及不误报托管。不得靠跳过新的服务窗口用例宣布通过。

## 验证证据与边界

- 旧实现的 `go test ./... -count=1 -timeout 10m` 已在 Windows 通过（服务包 373.271 秒）；Linux amd64 测试二进制和 CLI 曾交叉编译通过。这些结果只证明旧代码回归，不证明新的服务窗口目标。
- 当前主机中的直接启动、管理员进程和临时系统服务都处于 Job；带 `CREATE_BREAKAWAY_FROM_JOB` 的子进程仍在 Job 或创建被拒。原 Windows detached 路径因此按设计拒绝，并未取得成功后台存活证据。
- 旧的临时 `/IT` Task Scheduler TTY 测试已证明真实 stdin/console 窗口可用，以及旧 detached 启动的失败记录；它没有运行成功的服务/worker。另一个短暂的本机计划任务原型证明任务进程可在发起命令结束后继续运行，但本 Spec 选择服务窗口，不依赖计划任务。
- 新增 `service-window.bat` 保留旧 `run.bat` auto 行为。当前宿主的定向测试已通过：前台 manual/ready、显式 worker、独立客户端退出/重连、preflight 失败记录、控制 stop；opt-in 真实 TTY 测试验证了可见窗口、Ctrl+C 和窗口 X 后的服务/worker 退出与锁释放，以及重新打开。两个可见窗口从零并发的同配置和同仓库不同 run_dir 子场景均通过：一个 ready，另一个因仓库锁退出。测试只操作临时仓库与假工具。
- 首轮全仓回归在旧控制测试 `TestControlStartsOnlySelectedReadyTask` 上发现时序竞争：服务已 claim，假 worker 的启动计数尚未写回。该测试改为有界等待后，第二次 `go test ./... -count=1 -timeout 10m` 通过（服务包 435.547 秒）。最后的小改动只收紧窗口句柄检查并精简一条重复测试；针对最终代码的真实窗口 X/重启定向验收通过。
- 连接分类、启动锁、pending startup_id、preflight 失败记录和 StopService 回执已有原生测试。保留可复用部分；对 Windows 启动结果和窗口生命周期补充真实验收。
- 合入 `origin/main` 的 `69407b0` 后，`go test ./... -count=1 -timeout 10m` 通过（服务包 498.703 秒），`go vet ./...` 通过；Linux amd64 服务测试二进制与 CLI 交叉编译通过。Linux 原生会话生命周期未实测。
- `TestWindowsConcurrentVisibleServiceWindows` 的同配置及同仓库不同 run_dir 两个子场景均通过，分别只有一个窗口 ready，另一个报告锁冲突。合并后真实可见窗口 Ctrl+C 与 X/重启定向测试通过；临时计划任务已清理。权限不足分类通过注入 `ProcessUnknown(access denied)` 的客户端定向测试，未依赖当前账户访问受保护生产进程。

| 验收 | 对应证据 |
| --- | --- |
| L-A1～L-A3 | `TestWindowsVisibleServiceWindowAcceptance`：manual/ready、显式 worker、独立客户端退出与重连、身份不变 |
| L-A4 | `TestWindowsConcurrentVisibleServiceWindows`：两个可见窗口从零并发、同仓库两种配置形态 |
| L-A5 | `TestRunCommandRecordsPreflightFailureAndLog`、`TestWaitForStartupMarksConfirmedEarlyExit`、`TestStartServiceReobservesExistingPendingAttempt` |
| L-A6 | `TestConnectionClassifiesProcessAndSnapshotIndependently`、`TestProcessProbeSeparatesMatchReuseAndAbsent` |
| L-A7 | `TestWindowsForegroundServiceWindowLifecycle` 的 stop 回执/进程/锁；可见窗口 Ctrl+C 与 X 路径 |
| L-A8 | 连接分类测试及服务生命周期审核回归中的旧快照、配置与协议冲突 |
| L-A9 | `TestWindowsVisibleServiceWindowCloseAndRestart`：新实例、旧 worker 不重复、可再次正常停服 |

### 质量审计修复验证

- 修复 JSON 控制台混入普通启动提示及活动字段不完整、复用已归属 startup_id 时污染原实例记录/日志、启动器错误提示将路径中的 `&` 解释为命令、Windows pending 启动丢失配置/仓库冲突分类四项问题。各问题先由定向测试复现，再验证修复。
- 复用原生 CLI 测试覆盖 JSONL；启动记录保护、路径字面量、pending 分类、已有生命周期审核及 preflight 测试通过。再次运行真实窗口 Ctrl+C、X/重启验收；本轮定向回归共 104.364 秒，临时计划任务已清理。
- 修正 Linux detached 测试夹具：PR API 的 HTTP stub 不提供 Git fetch，改用既有 git-proxy 从本地裸仓库获取。最终 `go vet ./...`、Linux amd64 服务测试二进制与 CLI 交叉编译、`git diff --check` 通过。Linux 原生生命周期仍未实测；本轮未重复约八分钟的全仓测试，上述全仓结果属于审计修复前的合并版本。

## 完成条件

- L-A1～L-A9 在当前 Windows 主机取得真实服务窗口、worker、viewer 退出/重连、正常停服与异常关窗证据，尤其比较 service/worker 身份并核验锁与回执。
- Windows 离线客户端明确指向固定服务窗口入口；不再尝试旧 detached 启动或把 Job 内子进程误报为托管成功。
- Spec、Plan、README、状态模型及 PR 描述与最终行为一致；旧后台测试已更新到新目标。验收通过后标记 PR 待审；合并由协调审核流程负责，合并前不启动后续阶段。

## 风险与回退

服务窗口是 Windows 生命周期的可见边界。按 X 或强制结束可能来不及完成 `shutdown_grace`，因此必须依靠 worker Job 管理及下次启动的身份/锁/状态恢复；正常维护应使用控制 stop 或 Ctrl+C。若当前宿主禁止服务窗口内的 worker Job 创建，本 PR 必须报告具体失败并修复受控退出路径，不能以无法脱离 Job 为由跳过验收。旧前台 `run`/`stop` 仍是故障回退入口。
