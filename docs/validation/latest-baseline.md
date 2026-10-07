# 2026-10-07 验证基线

来源：[#228 后全量修复原记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/all-tests-after-228-2026-10-07/README.md)。最终实现源码 `120b54f3192d97917eeb3b6b40b7a1aba270e815`，rebase 主线 `82ce43d9bec8f4950b1b784c948a9304b458ff17`。这份历史基线不是当前 HEAD 的重跑结果。

| 范围 | 结果 |
| --- | --- |
| 三 worker Full | 1233 条父子记录，0 失败，18 默认 skip，约 42 分钟 |
| L1 | 459 条记录，0 失败、0 skip |
| 完整 L2 | controlled 与两条真实 smoke，3/3 |
| L3 controlled | delivery / recovery，2/2 |
| L3 live | 两个本机后端 delivery / recovery，4/4 |
| L3 candidates | 1/1，约 16 分钟 |
| 带 tag runner / E2E 基础设施 / 边界 | 161 条记录，0 失败，1 默认 sandbox preflight skip |
| E2E 全包编译 | 通过；0 业务测试，仅编译 |
| roadmap-runner 离线 Python | 18/18；没有运行调度工作流 |

注册表 10 个场景全部覆盖，通过场景均 `cleanup_complete=true`，真实场景均 `usage_verified=true`。环境为 Windows、Go 1.26.4、Codex CLI 0.159.2、Orca 1.4.209、gpt-6-luna/medium 与显式完全访问。skip 不能算原生 opt-in 或管理员 sandbox 验收通过。

Full、基础设施和最终候选运行于 clean `120b54f3`；L2、controlled L3、live delivery/recovery 运行于 clean `64d64a1f`，后续 Go 改动仅涉及 Full 调度/回归及 testenv 项目身份绑定/快照轮询。L1 位于 `9483afa1`。没有声称全部场景在同一最终 SHA 重跑；逐次命令、源码、结果、失败和报告哈希完整保留在[结果索引](baseline-results.json)，该 JSON 未修改原回执。

原始产物位于该记录指定的系统临时目录 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-after-226-20261006-a0ecf55b`，不属于仓库保留承诺。早期 candidates 超时、Orca 未运行及中断均保持失败，不由最终通过覆盖。原记录中尚未删掉的 scratch `C:\Users\lmas1\AppData\Local\Temp\steerline-e2e-2240857957` 不在本次文档清理范围。
