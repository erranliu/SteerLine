# PR 3 Plan：后台服务生命周期

依据：[Spec](pr-03-service-lifecycle.spec.md)。依赖：PR 2。

实现状态：本分支已完成代码与文档，等待协调审核。下表记录本环境实际执行的验证；未取得成功证据的验收仍保持未验证，不以 mock 或字段断言替代。

## 实施步骤

1. **提取项目身份与探测。** 增加轻量、只读的配置身份解析，获得 canonical common-dir、run_dir 和配置目标。新增结构化 OS 进程探测结果，Windows 复用 FILETIME/可执行路径身份，Linux 复用 boot ID/starttime；区分不存在与不可访问。
2. **实现连接状态组合。** 在独立的 service client 中结合 PR 1 新鲜度与 v4 实例/能力，形成连接结果。历史快照、控制能力与真实进程身份分开展示，不调用 `NewService`。
3. **实现后台启动原语。** 增加平台专用 detached start，重定向 stdin/out/err，处理 Windows Job/控制台继承和 Linux 会话。worker 的现有 startProcess/Job Object 逻辑继续专用于 worker。
4. **实现 startup 尝试与握手。** 客户端生成 startup_id、创建日志目录、取得 launch 锁、再次核对现有服务；子进程接受内部 startup-id 参数，发布成功快照或失败诊断。等待超时返回 pending 句柄，而不是删除尝试或重启。
5. **接入停止与 CLI。** 加 start-service 命令；StopService 使用 control 客户端并等待最终身份退出。旧 stop 兼容读取仍有效。服务客户端提供给 PR 4，不在本 PR 增加 TUI 页面。
6. **更新操作文档。** README 和 `docs/state-model.md` 说明稳定 exe、启动日志、连接与手动初始模式；记录平台实测范围。

## 验证

- 单测覆盖 heartbeat/ready/进程身份/协议的组合，模拟不存在、访问拒绝和 PID 复用。
- 创建独立父测试进程，由它启动假服务；终止父进程后核验子服务继续，再让其启动受它管理的假 worker，核验 UI 退出与服务退出对 worker 的不同影响。
- 在原生 Windows 上验收 Job 继承和真实 TTY 关闭，不能只以 CreationFlags 值或 mock 断言代替。测试只操作本次临时目录和已记录身份的测试进程，结束时回收它们。
- 并发运行两个 start-service，覆盖相同配置以及同 common-dir 不同 run_dir；检查唯一 ready 实例、失败诊断和连接获胜者行为。
- 故障注入 preflight 失败、锁失败、子进程早退、握手延期；断言 UI/CLI 不误报成功，不自动重试。
- Linux 平台至少做编译检查；若未实机验证，文档明确标注。Windows 是合并验收平台。

### 本分支实际结果

- 新增连接组合、进程匹配/PID 复用/确定不存在分类的原生测试；这些定向测试通过。
- `go test ./... -timeout 10m` 通过（Windows，327.702 秒）；Linux amd64 的 `internal/steerline` 测试二进制和 CLI 均交叉编译通过。
- 原生 Windows 上，`CreateProcess` 使用 `CREATE_BREAKAWAY_FROM_JOB | DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP` 成功；父测试进程的直接 Job flags 为 `0x2800`，但隔离子进程仍通过 `IsProcessInJob` 报告属于一个 flags 为 `0x2000` 的 Job。启动器据此终止该启动尝试、返回明确错误并写入失败 startup 记录。隔离子进程复测确认不是 `IsProcessInJob` 的误判。
- Explorer PID 40708 也处于 Job Object。经 WMI 创建独立 console 的验收探针被本地执行策略拒绝，命令没有执行；本次没有可用的独立真实 TTY 窗口，故未运行 TTY 关闭验收。
- 依赖成功脱离的真实后台 ready/worker 存活、关闭父 TTY 后身份不变、后台 stop 的进程退出、同配置竞争启动及不同 run_dir 锁竞争集成用例按 Job 能力检查跳过，尚无成功脱离结论。需要在允许该子进程链离开所有祖先 Job 的 Windows 会话补跑，尤其 L-A1～L-A5 与 L-A7。
- 原生测试覆盖了启动锁互斥、pending startup_id 重观测、preflight 失败记录与日志、PID 复用/不存在及连接分类；完整套件内前台 CLI 的实例锁和停服回归通过。
- L-A6 的新鲜度、ready、协议不兼容、配置摘要变化、仓库身份不匹配、PID 已复用和确定不存在分类已由原生定向测试覆盖；访问拒绝仍未实测。
- Linux 未在主机实测会话分离。

## 协调审核修复与外部复测

协调审核补充了启动握手并发、多个/损坏的启动记录、失败记录身份、取消、已退出进程、停服实例固定、锁释放以及已有服务不依赖安装文件的回归。修复 Windows 隔离核验前执行服务代码的问题，改为挂起创建并在核验后恢复；父进程身份记录拆为 `process.json`，避免覆盖子进程握手。Windows 已退出但仍有句柄的进程明确判定为已退出；Linux 由异步 Wait 回收子进程。停服超时保留 request_id，缺少回执时仍报告进程退出证据。

协调工作区验证：`go test ./... -count=1 -timeout 10m` 通过（服务包 332.457 秒）；13 个新增审核回归通过（最后补充的无回执进程退出场景另做定向验证）；Linux amd64 测试二进制与 CLI 交叉编译通过。整仓测试中依赖成功脱离宿主的用例仍跳过，不能据此判定 L-A1～L-A5、L-A7 或真实 TTY 门槛通过。

后台存活测试现在强制终止启动父进程并比较前后的 service/worker 身份；提前退出替身不会递归运行 Go 测试；允许成功 breakaway 的 Windows 环境不会因诊断测试而误报失败。以下命令必须在允许脱离所有祖先 Job 的 Windows 会话、PR 最终 head 对应的检出目录运行：

```powershell
$env:STEERLINE_REQUIRE_DETACHED_TESTS = '1'
go test ./internal/steerline -run 'TestDetachedService|TestStartServiceSerializesSameConfigAndRepositoryLockRejectsOtherRunDir|TestStartServiceReportsPreflightFailureAndPendingHandshake' -count=1 -v -timeout 5m
Remove-Item Env:STEERLINE_REQUIRE_DETACHED_TESTS
```

该开关使不能脱离宿主的情况成为失败，而非 skip。本协调宿主运行严格模式仍失败，报告 `parent_job_limit_flags=0x2800` 且子进程仍在 Job 内；这不是成功后台存活的证据。上述命令也不等于真实 TTY 窗口关闭验收：还需要用同类临时 Git/fake 工具夹具，在关闭启动窗口后记录服务与 worker 身份不变，再显式停服并确认回执、两个进程退出和两把锁释放。保留最终 commit、命令输出、startup/runtime 身份及时间；不操作生产服务。

未获得这些 Windows 成功路径与真实 TTY 证据前，PR 保持 draft，不合并，也不启动 PR 4。

## 完成条件

- L-A1～L-A8 完成，尤其具备真实 Windows 父进程退出后的存活证据。
- start-service 返回结构化结果供 UI 调用；启动/停止没有 shell 拼接。
- 启动日志、runtime 与 startup.json 的身份可相互关联，工具/锁失败能直接定位。

## 风险与回退

后台服务脱离与 worker 受控退出有相反的进程生命周期需求，必须使用两条明确的启动路径。该 PR 没有新增 runtime schema；停用后台启动功能后仍可使用前台 run 和 stop。
