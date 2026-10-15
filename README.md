# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。第一阶段包含本地 Codex、本机 Orca 与远端路径交付。

[文档入口](docs/README.md) · [产品愿景](docs/roadmap/product-vision.md) · [RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md) · [当前代码架构图](docs/architecture/current-code-architecture.md) · [测试总入口](docs/testing.md)

## 测试

日常最小入口为 `go run ./cmd/test-feedback -level=L1`。Full、定向回归、L2/L3、测试环境与独立原生验收统一见[测试指南](docs/testing.md)；其中完整 L2 和 L3 live 会消耗模型 token，完整 L2 会创建并合并真实测试 PR。

## 构建

在 Windows 上，准备好 Go、Node.js 和 npm 后，双击仓库根目录的 `build.bat` 即可一次构建 `bin/steerline.exe` 和 `bin/steerline-web.exe`；窗口会显示结果并等待按键关闭。命令行可调用 `build.bat --no-pause` 跳过等待，或运行等价的 `./build.ps1 -Web`。只构建业务 exe 时运行 `./build.ps1`，无需 Node.js。构建失败时保留该目标的原文件，目标程序仍在运行时拒绝替换。此命令只生成本地二进制，不安装到 `STEERLINE_HOME`，也不切换运行中的服务。安装与源码调试共存的近期范围见[RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md)；完整 stable/dev 版本管理已移出当前规划。

## 本机 Web

`./build.ps1 -Web` 生成 `steerline.exe` 和独立 `steerline-web.exe`，仅 Web 程序嵌入页面。显式设置 `STEERLINE_HOME` 后，分别启动 `steerline serve-app`、`steerline-web serve`，用 `steerline-web open` 授权打开；项目服务通过原生入口独立启动。固定浏览器地址放在 `<home>/web.json`，同一地址可切换该 home 已配置项目，关闭 Web 不停止业务服务或 worker。调试仍先生成新包，详见 [Web 指南](web/README.md)。

## 独立调试

临时或 AI 调试统一运行 `./scripts/new-debug-release.ps1`，生成 `C:\SteerlineDebug` 下包含业务、Web 和 helper 的独立 SteerlineTest 包。默认 Web 地址为 `127.0.0.1:18742`，可用 `-WebAddress` 指定另一固定端口。默认 live；AI 使用 `start.bat --simulate`，启动后用 `web.bat` 打开页面。任务状态由包内 `reset-SteerlineTest.bat` 无备份重置，活动项目/应用/Web/worker 阻止重置。操作见[安装与调试](docs/runtime-install-debug.md)。

Web 是唯一交互前端，原 TUI、`steerline console` 和 `console.bat` 已移除。后台 CLI、服务窗口与 `run -console` 日志、原生 Codex/Orca 会话查看继续保留；范围和验证见[TUI 退役](docs/spec/tui-retirement.spec.md)。
