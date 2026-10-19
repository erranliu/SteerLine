# Windows 启动闪窗修复验收

2026-10-19，普通开发；源码基线 `ab569f3a` 加本次未提交修改。不修改 roadmap、任务状态机、Runner 协议或共享服务所有权。

## 原因与范围

#326 统一宿主启动、#353 应用级共享 Runner 均已合入。共享服务 Spec 明确将闪窗治理后置；常驻服务减少不代表所有后台子命令已无窗口。

`ApplicationDirectory.run` 的项目 census 和 `ProjectRuntime.prepare` 在取锁前、后分别调用 `projectconnection.ResolveSnapshot`，每项目共三次 Git 身份核验。原实现直接启动 Git，没有窗口设置；后台宿主使用 `DETACHED_PROCESS`，这些短命令会创建控制台。托管命令、Runner CLI 和桌面启动器原来仅设 `HideWindow`，仍有非零控制台句柄。

本次共用 `processwindow.Hide`，Windows 设置 `CREATE_NO_WINDOW` 并保留已有创建标志、标准流、Job 管理及取消策略；其他平台无操作。覆盖上述四条路径及 start 调用 Web `open` 的短命令。后台后端/Web 原有 detach 行为、启动 bat 的结果/失败暂停、显式原生会话窗口保持原规则。Microsoft 对该标志的定义见[创建标志文档](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags)。

## 自动验收

环境为 Windows 本工作树的隔离 Go 子进程夹具；未启动调试服务或真实模型，未使用安装环境作为测试现场。

| ID | 验收项与入口 | 结果 |
| --- | --- | --- |
| SCW-A01 | `TestBackgroundCommandsNoConsole` 从真实 detached 调用方执行仓库身份、托管命令、Runner CLI、桌面启动器，子进程调用 `GetConsoleWindow`，四项均须为 0 | 通过；仅隐藏窗口仍不能通过 |
| SCW-A02 | `go test ./internal/projectconnection ./internal/runnerclient ./internal/execution/process ./internal/app/host -count=1 -timeout=120s`：原身份/输出预算、CLI 信封、命令取消、桌面存活与宿主测试 | 全部通过，各包 3.438s / 7.150s / 2.454s / 5.709s |
| SCW-A03 | `go test ./internal/app -run '^(TestUnifiedStartup\|TestSharedRunner)' -count=1 -timeout=180s`（执行时正则使用正常 alternation） | 通过，99.415s；统一启动、已安装 Web opener、共享 Runner 原绑定及停机边界保持 |
| SCW-A04 | `go build ./cmd/steerline ./cmd/steerline-web`；`go test ./internal/testkit/boundary -run '^TestTM08AppHostBoundary$' -count=1 -timeout=90s` | 构建与 Windows/Linux 生产依赖边界检查通过（边界 10.957s） |
| SCW-A05 | 新用例进入 Windows 固定 L1 `quick-core`；`go test ./cmd/test-feedback -run '^TestQuick' -count=1 -timeout=90s`；定向 test-feedback 入口实际执行新用例 | 通过，选择器 1.708s；不运行全量 L1/Full |

SCW-A01 反例：暂时使用 HEAD 的四条原实现运行同一新增测试，四项分别观测到非零 HWND，整体退出 1；随后恢复本次修改，四项均为 0。普通无控制台测试上下文会掩盖问题，因此用例先创建 `DETACHED_PROCESS` 调用方，且调用方与每次子命令均有明确期限。证据在当前聊天工具输出。

定向结构化证据：`go run ./cmd/test-feedback -- ./internal/app/host -run '^TestBackgroundCommandsNoConsole$' -count=1 -timeout=45s`，通过；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261019T092113.587044400Z/summary.json`。其他定向回归与构建证据见当前聊天工具输出。

不选择 L2/L3/L4：本次验证 Windows 子命令控制台创建，已有隔离进程用例能直接观察回归；共享 Runner、业务流程与原生执行规则没有变化，不新增真实模型/PR 场景。

## 人工体验

SCW-M01：用户更新构建后，双击 start 确认多项目启动不再连续闪出控制台，浏览器打开及失败诊断仍正常。当前源码已修复、未部署；人工视觉体验未验，可后置。
