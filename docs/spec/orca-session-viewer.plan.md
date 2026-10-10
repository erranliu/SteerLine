# Plan：在 Orca 中查看执行会话

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](orca-terminal-transport.spec.md) / [Plan](orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

普通开发直接实施，基于最新主干 `42f36428`，无 roadmap 调度。

沿用候选 Ctrl+N 与原生查看端口，保留 Codex CLI。Orca supervised 的 local/environment 均从原 WorkflowRun 的冻结 profile 及原候选 session/attempt 解析目标，核验原 Dispatch、runtime、worktree、terminal、incarnation 和 agent。所有 Orca 子命令移除继承的远端路由变量；查看不启动、恢复、发送输入或停止 agent。

本机先打开本机 Orca，再精确聚焦原 terminal，要求正式 navigated 回执并再次核验。当前 Orca 1.4.223 CLI 的 terminal switch 使用 host navigation，没有本机 paired desktop 外部导航入口；远端在本机 Orca 创建只读显示终端，由当前 SteerLine 二进制读取原远端屏幕，每帧重新读取候选以跟随同一 PTY 上的续轮 Dispatch，前后核验身份。显示终端不运行 agent；关闭只停止读取。重复打开复用原显示终端，不能将远端 host 的 navigated 当成本机定位成功。屏幕不可用明确标记保留输出，不冒充完整对话/原生可交互远端标签。请求绑定原 SteerLine home，禁止由 Orca 环境改读安装版状态。

## 实施步骤

1. 增加 adapter 身份核验、屏幕读取、本机 focus 回执核验；应用解析冻结配置，接入本机原终端和远端只读显示。显示终端命令由实现生成、独立 argv 与 shell 正确引用，不使用 Codex resume。
2. 更新控制台提示、帮助、状态和重复打开保护；活跃显示窗口复用，已关闭窗口须由完整清单证明缺失后重建，正常停止的显示进程可重开。桌面启动器不加入会随 CLI 退出而杀死桌面的 worker Job；其余读取仍受管。补齐最小确定性验证。
3. 生成独立 debug release；在 SteerlineTest 隔离目录用无模型终端核实 Orca 原生 focus/屏幕及本机显示窗口。实际模型 worker 的完整本机/远端人工窗口单列；不得改动安装环境或现有任务。

## 测试计划

专属[验收](../acceptance/orca-session-viewer.md)。

| 验收 ID | 级别/场景/环境 | 命令及执行时点 | 通过条件/证据位置 |
| --- | --- | --- | --- |
| OSV-A1 | Go `TestOrcaViewer*`，本机/远端确定性，无模型 | 实施后 `go test ./internal/execution/adapters/orcaremote -run '^TestOrcaViewer' -count=1` | 精确原身份、配对/路由、前后身份变化拒绝、未启动/恢复/写入 agent；本机导航回执真实核验；验收明细 |
| OSV-A2 | Go `TestOrcaSession*` / `TestNativeSession*` / `TestProcessHost*` / `TestDesktopOpen*`，隔离 fixture | 实施后 `go test ./internal/app ./internal/app/host ./internal/console ./internal/execution/process -run 'TestOrcaSession\|TestNativeSession\|TestProcessHost\|TestDesktopOpen' -count=1` | 冻结配置、过期候选拒绝、远端显示窗口复用/重开、命令引用、home、桌面子进程存活、控制台与环境过滤；验收明细 |
| OSV-A3 | 本次独立 debug release，SteerlineTest，无模型 CLI primitives | 步骤 3 执行 `scripts/new-debug-release.ps1`，使用其项目目录；原生 `terminal create/show/read --screen/switch`，精确 owned 清理 | 原终端身份保持、屏幕来源如实、只读显示不启动 agent；debug release evidence；不算完整真实模型 worker 验收 |
| OSV-A4 | Go 边界与产品构建/文档 | 实施后 `go test ./internal/testkit/boundary/... -count=1`，debug release 内构建，`python scripts/check_doc_links.py`、`git diff --check` | 生产边界、当前 CLI、文档通过；验收明细 |

不运行 full/L2/L3 模型交付：本次为显示入口，不改变业务派发/交付。原生能力由无模型精确终端测试核对，完整交互与真实模型双目标人工验证单列，不以 fixture 宣称完成。
