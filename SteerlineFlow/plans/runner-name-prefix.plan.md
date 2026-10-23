# Runner 工作区命名前缀

用户直接授权的两仓普通开发。Runner 的公开 Run 输入新增可选 `name_prefix`，与不透明关联元数据分开；SteerLine 对 C1 生成 `sl-<task_id>`，从 C2 起生成 `sl-<task_id>-c<候选 Number>`，Runner 仅追加自身 Run ID。未提供时 Runner 使用 `sr`。前缀限定 1–48 个小写 ASCII 字母、数字和单个分隔连字符，不接受空值、首尾连字符或连续连字符。

候选 Number 由现有 SQLite 预留事务分配，同事务生成前缀、序列化最终 submit 并保存候选；重复预留使用原 Number 比较最终输入，正常重放直接返回原候选。命名不是身份权威，原 Run/资源/请求绑定保持。[生命周期](../../docs/product-design/worker-lifecycle.md) W01/W02、V01/V06–V08 仅补充命名输入的冻结与校验，不新增节点、重试、恢复或旧现场迁移。范围限 Orca worktree 名称，分支规则保持。

## 实施

1. Runner 发布字段、校验和 schema，在执行快照固定有效前缀，创建与回执核验共用名称函数。
2. SteerLine 同步公开输入类型，在预留事务分配 Number 后完成最终提交；保留原请求及不可变性检查。
3. 补充两仓定向测试、公开 CLI 跨仓契约检查及专属验收记录。

## 测试计划

验收项与结果登记在[专属验收](../testing/acceptance/runner-name-prefix.md)。均为 Windows/amd64、L1 定向验证，修改完成后执行，使用隔离临时 SQLite/Git/受控 Orca；不启动真实 agent 或操作安装环境。

| 验收项 | 场景与命令 | 通过条件及证据 |
| --- | --- | --- |
| NP-A1、NP-A2 | `go test -json ./internal/storage/sqlite ./internal/taskrunner ./internal/runnerclient -run '^TestRunner' -count=1`；其中 `TestRunnerCandidateReservationAndProjection` 与 `TestRunnerPresetFreeze` 覆盖编号/输入、重放及不可变性 | 有效测试非零、零 fail/skip；事务内固定 C1 无候选后缀、C2 带 -c2，后续分配不改变原提交；日志 `steerline-tests.jsonl` |
| NP-A3 | 先从本次 Runner 源码 `go build -o <evidence>/steerrunner.exe ./cmd/steerrunner`，设置 `STEERRUNNER_EXE` 后执行同一 SteerLine 命令 | `TestRunnerPresetFreeze/published-runner-name-prefix` 将真实保存的输入交给公开 `validate --kind run --input - --json`，`ok=true`、`valid=true`；日志同上 |
| NP-A4 | Runner 仓库执行其 [Plan](https://github.com/erranliu/SteerRunner/blob/main/docs/spec/worktree-name-prefix.plan.md) 的契约、freeze、Orca 创建/恢复定向命令 | schema 与生产准入一致，通用前缀、默认前缀、唯一后缀、原创建与只读对账正确；日志 `runner-tests.jsonl` |

证据目录在验收文档记录实际绝对路径。L2/L3/L4 和 full 不选：本次仅改变命名输入与确定性拼接，事务、公开 CLI 和受控 adapter 已覆盖改动；不重复验证模型、交付与原生权限矩阵。
