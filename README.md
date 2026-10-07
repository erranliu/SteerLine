# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。第一阶段只支持和验收本地 Codex 与本机 Orca；远程执行及本机/远端混合流程后续另行规划，不作为当前交付门槛。

[文档入口](docs/README.md) · [产品愿景](docs/roadmap/product-vision.md) · [RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md) · [当前代码架构图](docs/architecture/current-code-architecture.md) · [测试总入口](docs/testing.md)

## 测试

日常最小入口为 `go run ./cmd/test-feedback -level=L1`。Full、定向回归、L2/L3、测试环境与独立原生验收统一见[测试指南](docs/testing.md)；其中完整 L2 和 L3 live 会消耗模型 token，完整 L2 会创建并合并真实测试 PR。

## 构建

在 Windows 上，双击仓库根目录的 `build.bat` 即可构建；窗口会显示结果并等待按键关闭。命令行可运行 `./build.ps1`，或调用 `build.bat --no-pause`。脚本从仓库根目录构建 `cmd/steerline`，固定输出到 `bin/steerline.exe`；构建失败时保留原文件，目标程序仍在运行时拒绝替换。此命令只生成本地二进制，不安装到 `STEERLINE_HOME`，也不切换运行中的服务。安装与源码调试共存的近期范围见[RoadmapGuide](docs/roadmap/RoadmapGuide.md) · [正式执行清单](docs/roadmap/execution-map.md)；完整 stable/dev 版本管理已移出当前规划。

## 执行控制台

运行 `steerline console` 打开当前 TUI，默认枚举 `<STEERLINE_HOME>/etc/*.json` 项目并加载全局 execution.json；`-config <path>` 打开明确项目，`-run-id <run-id|task-id>` 定位已有运行。Windows console.bat 进入同一控制台；中心服务由用户从 service-window.bat 独立启动，退出控制台不会停止服务或 worker。

当前控制台使用任务列表、候选摘要和运行/反馈三窗，支持候选切换、原地输入和完整最终回复的只读详情。目标选择、操作键位、草稿及重开语义统一见[执行控制台设计与操作](docs/product-design/execution-console.md)；配置和权限见[执行方案](docs/product-design/execution-profiles.md)，生命周期见[整体架构](docs/architecture.md)。完整过程时间线、工具正文与轮次折叠不属于当前 TUI。
