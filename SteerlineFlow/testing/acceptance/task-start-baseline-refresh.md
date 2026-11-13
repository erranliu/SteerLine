# 任务启动复用主分支观察

2026-11-13，受测源码为 `d01858d6` 加本次工作区改动。使用隔离 Git 远端、SQLite 与 Runner 协议夹具。

- 从正式任务启动到候选预留共执行一次 Git fetch。主分支提交未变时，环境依据读取次数为零，候选绑定当前检测活动。
- 主分支出现无关提交时核验一次环境依据，继续使用原通过结果。声明依赖改变时撤销资格，阻止任务启动。
- 核验已知新基线期间，旧基线不能完成候选预留。读取故障保留原检测结果，恢复后可重新核验。

证据根为 `C:/Users/lmas1/.codex/visualizations/2026/11/13/01a1d436-fb00-7de0-b4a6-44ef464d1cb3`。

| 验证 | 结果 | 报告 |
| --- | --- | --- |
| 固定 L1 | 通过 | `observe-l1/summary.json` |
| 固定 L2 | 通过，清理完成 | `observe-l2/summary.json` |
| 任务启动定向回归 | 两项通过，零跳过 | `observe-start-final/summary.json` |
| 完整 `go.environment` | 两个用例遇到隔离 Git clone/push 故障，其余用例通过 | `observe-environment-final/summary.json` |
| 环境专项 Git 故障项复验 | 两个根用例及其子项通过 | `observe-environment-git-recheck/summary.json` |

首轮环境专项的新增启动用例缺少完整测试安排，报告保留在 `observe-environment/summary.json`。补齐夹具后，该用例通过定向回归和完整环境专项中的启动检查。
