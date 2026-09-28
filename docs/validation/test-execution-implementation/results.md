# 测试执行提速：实现验证

2026-09-28，在 `9877cb0` 基础上的实现工作区验证。Windows/amd64、Go 1.26.4、20 个逻辑 CPU、沿用 Go 构建缓存；`-count=1`，未启用真实 Codex、Orca、PR 或可见窗口验收。产品业务代码、SQLite 配置和构建策略未改。本次按 [Spec](../../spec/test-execution-efficiency.spec.md) 和 [Plan](../../spec/test-execution-efficiency.plan.md) 独立实施，未创建 SteerLine 中心任务或 DAG 边。

## 快速与完整套件

| 入口 | 执行墙钟 | 结果 |
| --- | ---: | --- |
| `go run ./cmd/test-feedback -quick` | 29.769 秒 | `scope=quick`，固定闭环、错误 PR 门槛及两个规则包通过；0 skip |
| 同一 quick 入口加一个显式 taskgraph 定向用例 | 41.999 秒 | `scope=quick`，三个组均通过；0 skip |
| `go run ./cmd/test-feedback -workers=3` | 263.318 秒 | `scope=full`，准备另计 5.129 秒；3 个核心组及其他包通过 |
| `go run ./cmd/test-feedback -workers=1` | 463.780 秒 | `scope=full`，串行全套通过 |

三进程执行时间为串行的 **56.8%**，低于 Spec 的 75% 门槛，也低于参考机 300 秒目标。运行时自动列出 276 个核心顶层用例并分别分配 90/92/94 项；三个核心组实际耗时 207.394/224.510/206.708 秒，其他包 38.769 秒。总计 11 个包，487 个测试及子测试终态；串行与三进程均为 470 pass、17 skip、0 fail，逐项 `(package, name, status)` 完全一致。17 个默认原生/平台 skip 没有被计作真实平台验收。构建准备与执行分别计时，没有宣称编译提速。

[机器、分组、终态摘要和原始 JSON 流压缩归档](suite-comparison.json)由 [归档脚本](archive-suite.py)生成；每个 `.jsonl.gz` 解压后是原始 Go JSON 输出，并记录原始字节 SHA-256。运行目录还保留各组 stderr 与原始 JSON。先期定位的完整基线、实验输入及选择理由见[诊断记录](../test-execution-diagnosis/README.md)。

首轮完整运行发现 Windows 下分组临时路径过深，Orca 测试的 Git worktree 报 `'$GIT_DIR' too big`。执行器改为在系统临时目录创建短且独立的组路径后，该用例通过；上表及归档均取修复后的完整通过运行。失败首轮未计为验收。

## 定向配对与边界

[配对结果和每次输出](paired-summary.json)由 [Go overlay 配对脚本](run-paired.py)生成。在相同产品源码下，使用原 `9877cb0` 测试文件与新测试文件交替执行三组前后对照，编译准备不计入执行墙钟：

| 固定用例 | 原版中位数 | 实现后中位数 | 实现后 / 原版 |
| --- | ---: | ---: | ---: |
| 连续准备 10 个原生 Git fixture | 4.724 秒 | 1.967 秒 | 41.6% |
| 六种 DAG 拒绝中的指定测试 | 14.767 秒 | 4.144 秒 | 28.1% |

新增 A/B/种子隔离测试实际修改、提交并 push A，核对 B 和种子的 HEAD、文件及 remote refs 不变，并核对 A 清理后 B 仍可用。DAG 的内存 AND 矩阵、环拒绝无副作用、配置选择的两个真实 worker/重启、原生错误 PR 与最小闭环均在完整套件中通过。独有恢复与真实进程用例没有删除或默认跳过。

执行器的单元测试覆盖权重分组、缺失/重复/截断/非法 JSON、意外 skip、进程树取消，以及环境开关识别；设置 `STEERLINE_REAL_SAMPLE=1` 时三进程入口在启动测试前拒绝。`scripts/test_steerline_verify.py` 用成功与 go_test 失败两种回执核对固定检查顺序、revision 和临时报告路径。`go vet ./...`、Windows `go build ./...`、Linux/amd64 `go build ./...` 均通过。真实中心任务的 verify 步骤没有在本次运行，因为本实现按用户要求不创建任务；已验证其命令接线与回执映射，完整测试入口自身由上表独立执行。

日常开发使用 quick 加改动路径的定向测试；稳定候选交付前执行一次 full。数据库格式、恢复、外部执行身份、PR 交付或共享接口变更可提前执行 full。既有中心任务契约中的完整回归与真机门槛不因本改动放松。
