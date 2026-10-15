# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。第一阶段包含本地 Codex、本机 Orca 与远端路径交付。

[文档入口](docs/README.md) · [产品愿景](docs/roadmap/product-vision.md) · [RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md) · [当前代码架构图](docs/architecture/current-code-architecture.md) · [测试总入口](docs/testing.md)

执行已拆到独立 [SteerRunner](https://github.com/erranliu/SteerRunner)：SteerLine 负责任务协调，通过固定 EXE/CLI 调用；运行数据与日志归项目工作目录。配置、固定构建及当前验收边界见 [Runner 接入](docs/integration/steerrunner.md)。

## 测试

日常最小入口为 `go run ./cmd/test-feedback -level=L1`。Full、定向回归、L2/L3、测试环境与独立原生验收统一见[测试指南](docs/testing.md)；其中完整 L2 和 L3 live 会消耗模型 token，完整 L2 会创建并合并真实测试 PR。

## 构建

在 Windows 上，准备好 Go、Node.js 和 npm 后，双击仓库根目录的 `build.bat` 即可一次构建 `bin/steerline.exe` 和 `bin/steerline-web.exe`；窗口会显示结果并等待按键关闭。命令行可调用 `build.bat --no-pause` 跳过等待，或运行等价的 `./build.ps1 -Web`。只构建业务 exe 时运行 `./build.ps1`，无需 Node.js。构建失败时保留该目标的原文件，目标程序仍在运行时拒绝替换。此命令只生成本地二进制，不安装到 `STEERLINE_HOME`，也不切换运行中的服务。安装与源码调试共存的近期范围见[RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md)；完整 stable/dev 版本管理已移出当前规划。

## 本机 Web

`./build.ps1 -Web` 生成 `steerline.exe` 和独立 `steerline-web.exe`，仅 Web 程序嵌入页面。显式设置 `STEERLINE_HOME` 后，分别启动 `steerline serve-app`、`steerline-web serve`，用 `steerline-web open` 授权打开；项目服务通过原生入口独立启动。固定浏览器地址放在 `<home>/web.json`，同一地址可切换该 home 已配置项目，关闭 Web 不停止业务服务或 worker。调试仍先生成新包，详见 [Web 指南](web/README.md)。

## 执行控制台

临时或 AI 调试统一运行 `./scripts/new-debug-release.ps1`，复制 `debug-template/` 并生成 `C:\SteerlineDebug` 下的完整独立 SteerlineTest 环境。执行配置在复制源维护，默认 live；AI 显式使用 `start.bat --simulate` / `console.bat --simulate`。任务状态通过包内 `reset-SteerlineTest.bat` 无备份重置，不使用安装环境。操作见[安装与调试](docs/runtime-install-debug.md)。

运行 `steerline console` 打开当前 TUI，默认枚举 `<STEERLINE_HOME>/etc/*.json` 连接配置，从 HTTP 服务取得执行方案摘要；`-config <path>` 打开明确项目，`-run-id <run-id|task-id>` 定位已有运行。全部业务查询和操作经本机 HTTP API 到 Go 服务；Windows console.bat 进入同一控制台；中心服务由用户从 service-window.bat 独立启动，退出控制台不会停止服务或 worker。

当前控制台使用任务列表、候选摘要和运行/反馈三窗，支持候选切换、原地输入和完整最终回复的只读详情。目标选择、操作键位、草稿及重开语义统一见[执行控制台设计与操作](docs/product-design/execution-console.md)；配置和权限见[执行方案](docs/product-design/execution-profiles.md)，生命周期见[整体架构](docs/architecture.md)。中心仅采集完整最终回复、问题及必要执行事实；完整过程在原生工具查看。恢复仅继续原会话，原会话不可核实时由人处理，详见[会话与恢复精简](docs/spec/product-simplification-04-conversation-recovery.spec.md)。

原生会话查看以 Runner 当前公布的 capability 为准；任务操作仍通过 SteerLine。
