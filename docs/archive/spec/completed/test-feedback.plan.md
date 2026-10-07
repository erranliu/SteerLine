# Plan：测试耗时基线与夹具精简

任务 ID：11；任务名称：测试耗时基线与夹具精简；目标：`REFACTOR`。状态：实现与验证完成，待 PR 评审。唯一执行依据：[Spec](test-feedback.spec.md)。无未完成任务硬前置，一个实现 PR 交付。

## 实施顺序

1. 冻结实现基线，清点默认用例和已有原生验收开关。记录 Go/平台、源码、缓存和并发设置，完整执行一次 Spec T2 命令，保留失败或未完成记录。
2. 实现轻量运行入口与 JSON 汇总。用小型日志样例验证父子事件、并行包、失败、跳过与截断；退出码以实际命令和解析完整性共同决定。完整入口不采用维护测试名称白名单的方式选测。
3. 从最慢用例中定位准备成本，优先检查 `service_test.go` 的 `newFixture`、CLI 构建、Git 仓库初始化及轮询。选择有限夹具组，记录至少三次基线；不根据源文件大小推定耗时。
4. 移除与断言无关的准备，共享本次运行构建的不可变产物，隔离每个可写工作区；用事件同步替代不必要睡眠。对合并/改写用例保存覆盖映射。每次只优化一个可归因的来源，再做同配置配对测量。
5. 编写日常与合入验证入口说明，记录完整回归、集成与原生验收的覆盖边界。报告放在 `docs/validation/test-feedback/`，向 #12/#13 交接耗时和覆盖数据。

## 验证与完成条件

按 TF1–TF5 对照交付。候选修改稳定后执行一次完整无缓存回归、静态检查和双平台构建；最后的文档修订只做文档检查。出现失败先定位，不通过删断言、跳过默认测试、提高生产超时或放松 SQLite 持久化换取通过。

统计工具验证及测量结果必须显示实际改善；不在本任务拆生产包、删除 Beads 路径或建立通用测试调度系统。

## 实施记录

- 原生任务记录：`docs/roadmap/task-ids/11.json`（ID 11）。
- 唯一执行依据：[`docs/spec/test-feedback.spec.md`](test-feedback.spec.md)，SHA-256 `935CDED3827A52669D6A6832E6DFF3CCDDA939F658A4B1D4DE3139FEC1C1ED3A`。
- 实现基线：`f363c427638de5ddee2711fe2a155cc5017484fb`，分支：`codex/11-test-feedback`，PR 基线：`main`。

### 步骤与验收映射

| Spec 项 | 实施结果 | 验证证据 |
| --- | --- | --- |
| T1 范围和边界 | 只改测试工具与测试 fixture；生产协议、状态机、超时和 SQLite 配置未改 | 最终源码差异与 [`coverage-map.md`](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-feedback/coverage-map.md) |
| T2 运行和汇总 | 新增 `cmd/test-feedback`、JSONL 汇总器、现有日志重汇总入口；保留原始 stdout/stderr，记录提交、工作树、Go/平台/并发/缓存/验收开关 | `internal/testfeedback` 的父子、并行包、失败、跳过、截断和缺失终态用例；[`status-filter-review`](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-feedback/status-filter-review/summary.json) 验证只排除本次运行输出文件 |
| T3 fixture 精简 | 运行期只读 Git 种子被复制到每例独立 worktree 和 bare remote；同一测试进程内复用当前源码构建的 CLI | 20 个 fixture 的 Git 准备从 160 次进程调用降到 48 次（含一次性种子）；三次配对中位数从 105.02 秒降至 98.15 秒（6.54%） |
| T4 覆盖与交接 | 原用例无删除、合并、跳过或降层；新增 fixture 可写隔离检查；日常和合入入口、#12/#13 交接写入验证文档 | [`coverage-map.md`](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-feedback/coverage-map.md) 和 [`README.md`](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-feedback/README.md) |
| T5 最终验证 | 最新完整套件、vet、Windows/Linux 构建及差异检查均通过；记录完整套件时延波动 | [`final-full-after-status-filter`](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-feedback/final-full-after-status-filter/summary.json) 及下表 |

### 实际命令和结果

- 修改前完整基线：`go test ./... -json -count=1 -timeout 20m`，退出码 0，墙钟 990.60 秒，538 项终态事件，521 通过、17 跳过、0 失败。
- 最终完整套件：同一命令，退出码 0，墙钟 1085.72 秒，549 项终态事件，532 通过、17 跳过、0 失败。最新运行比基线慢 95.12 秒；此前同代码完整运行是 908.38 秒。变化集中在未修改的 `nativeTaskFixture` 子进程/Git/SQLite 集成测试；该时延波动作为审查风险保留，不宣称完整套件加速。指定慢 fixture 组的三次配对中位数仍下降 6.54%。
- `go vet ./...`：通过。
- Windows `go build ./...`：通过。
- Linux/amd64 `go build ./...`：通过。
- `git diff --check`：通过（Git 对三个既有测试文件提示 LF/CRLF 规范化信息）。
- 最终完整运行保留 17 项原有跳过，均为未启用的平台/原生验收；没有把这些项目计作真实 GitHub、Codex、Orca 或可见窗口验收通过。

### 实现文件

- `cmd/test-feedback/main.go`、`cmd/test-feedback/main_test.go`
- `internal/testfeedback/summary.go`、`internal/testfeedback/summary_test.go`
- `internal/steerline/service_test.go`、`service_client_test.go`、`cli_test.go`
- `docs/validation/test-feedback/`

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
