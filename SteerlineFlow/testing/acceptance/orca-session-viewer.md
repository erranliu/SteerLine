# Orca 执行会话查看验收

唯一归属[Plan](../../plans/orca-session-viewer.plan.md)，普通开发，无 roadmap Task ID。

| ID | 项目 | 结果 |
| --- | --- | --- |
| OSV-A1 | adapter 本机/远端身份、配对、屏幕和 focus；无 agent 启动/恢复/输入 | 通过；定向及完整 orcaremote 包回归 |
| OSV-A2 | 应用冻结配置、候选及窗口身份、复用/关闭后重开、home/命令引用；桌面启动器与路由过滤 | 通过；本次全部新增定向场景，保留原 Codex 查看回归 |
| OSV-A3 | 独立 debug release 内真实 Orca 无模型原终端 focus/screen/显示窗口 | 通过 primitives；本机 focus navigated=true；本机 Orca 显示远端原终端 tick，source=screen；关闭显示后原远端 PTY/incarnation 仍存活；本次 owned 资源清理归零。没有真实模型请求，不宣称完整真实 agent 窗口已验 |
| OSV-A4 | 边界、构建、文档 | 通过；最终产品构建、TM04/TM08 边界、文档 308 文件/1895 本地链接/0 错误，diff 空白检查通过 |
| OSV-M1 | 本机真实模型候选 Ctrl+N 打开 Orca 并显示原 agent，重复打开/关闭保持执行 | 待人工验证；本次没有用户人工窗口，不阻塞普通代码交付 |
| OSV-M2 | 远端真实模型候选 Ctrl+N 在本机 Orca 显示原远端 PTY；续轮、重复打开、断线、关闭保持原 agent | 待人工验证；只读显示不是 Orca 原生可交互远端标签，不阻塞普通代码交付 |

## 证据及限制

2026-10-10，在 `42f36428` 最新主干上直接实施，dirty=true。定向命令为 Plan OSV-A1/A2；新增所有场景通过。较宽包级回归中 console、orcaremote、process、product/client 均通过；app 唯一失败为 `TestOrcaNativeProfileKeepsFrozenAuthority`，它仍断言新版 supervised 必须缺少 structured final evidence。原 `origin/main` 未修改源码已在独立副本复现同一失败（3.548s），本次不修改该既有断言。

原生证据目录：`C:/SteerlineDebug/dev-20261010-114926-42f36428-dirty/evidence/`。生成入口为 `scripts/new-debug-release.ps1`。只使用其中 SteerlineTest 与本次专属远端工作区 `osv-session-view-20261010-1156`，base `9380908528dcdd1046f59aa2672090ba5456c1e7`，没有 PR、agent 启动、业务输入或安装环境改动。

本机原终端 `term_c4c5596f-9ba8-45f3-857e-cf944a8d3575`；远端原终端 `term_1517e4f7-3874-41f5-a0aa-e6f43baba9d0`；本机远端显示终端 `term_cfaba8d1-4810-47b4-8633-6d5172c6de4b`。focus、screen、前后 identity、关闭及清理回执保存在上述目录。`remote-display-screen.json` 的 source=screen、包含原远端 tick；`remote-after-display-close.json` 证明原远端 incarnation 未变且 connected=true；`viewer-cleanup.json` 的本机/远端 owned remaining 均为 0。远端工作区已正式删除。

首次 `terminal create --focus` 超时，未观察到对应显示句柄，没有重试同一路径。改为先 create 取得正式句柄，再精确 switch 后成功，产品使用该分步路径；创建结果不明时不声称打开成功。无模型原生验证核对真实显示原语，产品候选、冻结配置、worker-show 和读取前后身份边界由确定性测试覆盖；不以原语探针代替 OSV-M1/M2。

最终产品构建位于 `C:/SteerlineDebug/dev-20261010-114926-42f36428-dirty/bin/steerline-session-viewer-final.exe`。原调试包默认二进制和 manifest 未被覆盖；本次最终构建单独保留。
