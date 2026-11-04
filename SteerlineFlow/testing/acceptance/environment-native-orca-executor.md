# 环境检测使用 Orca 原生执行器（2026-11-04）

SteerlineTest 原检测 `2464a93c221a4ee2b91b87f4a9ce2589` 将远端开发方案的显式 Codex 可执行文件及参数带入 `execute/1`，Runner 在创建 Run 前拒绝请求。

新检测在冻结提交信封时移除 Orca Codex 的可执行文件、固定运行时及启动参数，保留模型、推理强度、传输和目标。持久化的开发方案、环境键与准备依据仍取原配置。回归覆盖默认远端 Codex、显式本机 Codex、本机 Pi 和 Codex App Server，并核对原配置投影。

## 隔离验证

测试使用 SteerLine `3996bf20b8c3ea9202deab7a2d493449a3c266c9` 加本次两个 Go 文件的差异。原始报告根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback`。

| 入口 | 结果 | 原始报告 |
| --- | --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 通过 | `20261104T102850.636864000Z/summary.json` |
| `go run ./cmd/test-feedback -level=L2` | 通过 | `20261104T102816.090869200Z/summary.json` |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment` | owner 与 App 两组通过 | `20261104T102915.701156500Z/summary.json` |

默认沙箱内首次 L2 与环境专项的结果均失败，且受管报告的原子重命名被拒绝。补验使用独立输出目录。

## 安装与真实检测

正式部署成功，后端、Web 和三个项目均就绪。安装记录位于 `C:/SteerlineInstall/deployments/20261104-102810-00be52cb/deployment.json`。复用 Runner `0.2.0-dev.4`，SHA-256 为 `6e9ce33b03516a499e118da84325182d07fccf7823d6f14566dc9879af913ea5`。部署前后 `execution_profiles` 的规范化摘要一致。

在 SteerlineTest 使用原方案 `orca-remote-desktop64g` 发起活动 `1e68de340ffd4afc9acf7a5d7baac4b7`。Runner `86548f00e6994be0afedd4aca3af555a` 使用 `orca_agent_session_v1`，确认原生回合已启动且最终成功。该 Run 为 `completed / stopped`，最终报告以 `closed / 1` 封存，确认 PowerShell、Git 和 Python 可用。当前环境引用指向新活动并显示 `ready`。原生停止证据覆盖 Orca 会话。
