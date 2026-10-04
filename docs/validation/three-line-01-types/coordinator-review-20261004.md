# #73 协调复审修复

协调候选：`8734a164905c702fa696319aa254c07778f9d3f9`；目标 main：`a8ff0f01d38299d77ecc9cd4463b78a1286e1bec`。执行聊天已停写，协调聊天独立检查 Spec/Plan、完整实际 diff、生产消费者、纯包/守卫、快照转换、Windows fixture 和主干冲突接线。

独立 AST 核对实际 base/head 的全部 Go 声明，并人工检查不等价声明、删除的 alias/纯转发及新纯快照/端口；核对方法仅归一化项目 import 资格名、经查证的符号搬迁名称、格式与注释，保留字面值/字段/断言，不以工具摘要替代复审。

发现并修复：

- `taskview.graphNode` 对每个图节点调用 `Snapshot.TaskGraph`，重复复制整个 Deliveries map；Project 与 Paginate 现在每次快照只转换一次，再供各节点复用。资格、PR 详情、同快照与游标语义不变。
- Plan/验收表格中的正则 `|` 未转义导致列拆分；已修正 Markdown 表格，并为复审修复补齐测试计划。

独立验证：

- `go run ./cmd/test-feedback -level=L1 -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review73-l1-8734a16`：通过。此报告固定原候选；随后只补影响图投影的定向检查。
- `go run ./cmd/test-feedback -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review73-graph-final -- ./internal/steerline/taskview ./internal/steerline -run '^(TestGraphViewUsesOneSnapshot|TestTaskGraphLimitsAndExportPublication|TestTaskDependencyStateRequiresEveryPredecessorDelivery|TestMC5|TestMC6)$' -count=1`：2 包、12 parent/subtest 通过，0 skip；固定快照/图分页/依赖资格/唯一交付及多合并/未知/迟到冲突均保持。
- Markdown 表格列数与 `git diff --check` 通过；无新测试场景、验收豁免、Full/L2/L3/live 运行。

原始报告保留在仓库外协调 run；此次源码修复由新的复审 commit 固定，PR 评论记录最终 SHA。之前 source-index 与报告保持其历史候选语义，不覆盖旧证据。
