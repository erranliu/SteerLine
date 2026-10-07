# Plan：任务图规则拆包与测试分层

任务 ID：13；任务名称：任务图规则拆包与测试分层；目标：`REFACTOR`。状态：实现与验证完成，待 PR 评审。唯一执行依据：[Spec](taskgraph-modules.spec.md)。硬前置：[#12](beads-path-removal.spec.md) 已合入；一个实现 PR 交付。

## 实施顺序

1. 在 #12 最终版本复核 #11 耗时与用例清单。列出 `dependencyCycle`、`StableTaskOrder`、数字排序、图遍历/边界计算的调用方，标注 SQLite 快照与 ready 资格产生的位置。
2. 建立 `internal/steerline/taskgraph` 及最小算法类型，先搬环检测和排序，再搬邻域/截断。父包用薄转换调用，保持既有协议与错误语义；不移动持久化模型或改变事务边界。
3. 迁移纯规则测试矩阵，保留父包的事务、版本、交付及代表性接线测试。对迁入的每组断言建立映射；覆盖算法的重复大矩阵合并到规则包，实际 SQLite/Git/文件行为继续在相应层验证。
4. 建立 integration 包，将旧来源拒绝和至少一个原生命令成功测试改为实际 CLI 黑盒测试。统一本次 CLI 构建，保留独立目录、数据库和清理；从原包删除对应重复用例，不暴露内部字段。
5. 对纯规则和确有独立现场的用例评估并行，以小规模并发配对测量；任何进程全局状态或真实宿主资源不并行。对改变的并发测试做针对性的重复和 race 检查，失败先解决隔离问题。
6. 更新测试入口、包依赖说明及 `docs/validation/taskgraph-modules/` 报告。使用 `go list` 和测试 JSON 验证独立反馈路径及完整入口覆盖。

## 验证与完成条件

按 TM1–TM6 验收，先运行规则包、图查询/调度、CLI 集成与相关消费者测试；候选稳定后运行一次 #11 完整无缓存入口、静态检查和双平台构建。报告 cold build 成本与稳定缓存条件下测试成本，避免用缓存命中冒充算法变快。

交付规则包单独测试命令和集成包命令，并解释哪类改动需要跨层回归。若发现必须扩大到存储或整个 Service 才能实现本 Spec，先收敛为这里固定的算法边界；进一步模块化另行规划，不将本任务变为无界重写。

## 实施记录

- 实现基线：#12 合入后的 `8e5df5e6d1cceae9b7b82aa6b1653372e214d9e7`。
- 验收明细、测试迁移映射、完整 JSONL 与逐包时延：[验证报告](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/taskgraph-modules/README.md)。
- `internal/steerline/taskgraph` 只实现内存纯规则；父包保留 Task/DTO 转换、同快照 ready 资格、事务/交付权威与导出 I/O。
- `internal/steerline/integration` 将旧来源拒绝和原生任务读取改为真实 CLI 进程测试。`TestMain` 对该包构建一次 CLI，场景使用独立临时状态。

| Spec 项 | 结果 | 证据 |
| --- | --- | --- |
| TM1 | 环检测、稳定拓扑排序和确定性图投影位于规则包；依赖仅为 `sort`、`time`，无父包、SQLite、UI、网络或平台依赖 | `go list` 依赖检查；`go test ./internal/steerline/taskgraph` |
| TM2 | QueryService 从同一快照传节点、边和已归一过滤值；协议、游标、图/数据库版本、边界常量、ready 资格和导出写入边界保持 | 核心包图查询、ready、游标、导出与同快照回归；完整套件 |
| TM3 | 两个真实 CLI 黑盒场景共用 integration 包的一次 CLI 构建；拒绝场景确认无状态写入，成功场景确认原生 DB/revision 保留 | `go test ./internal/steerline/integration -count=1 -timeout 2m` |
| TM4 | 算法矩阵集中在纯规则包；核心事务/读取/持久化断言保留；迁移映射列于验证报告；`go test ./...` 默认发现两个新包 | 验证报告迁移表和完整套件摘要（353 终态：343 通过、10 跳过、0 失败） |
| TM5 | 使用 #11 `cmd/test-feedback` 保存 post-#12 基线和候选全量/构建探针；纯规则并发三组成对中位数差 20ms，且两组并行较慢，故保持串行。最终全量墙钟 641.348s，对基线 +13.214s（2.10%），处于已记录的基线运行波动内 | `baseline-retry/summary.json`、`baseline-build/summary.json`、`repeat-serial*/summary.json`、`repeat-parallel*/summary.json`、`final-full-serial/summary.json`、`final-build-serial/summary.json` |
| TM6 | 全量无缓存套件、vet、Windows/Linux 构建与 diff 检查通过；规则与集成测试保持串行，无并发测试变更。race 工具在本机不可用，环境限制如实记录 | 验证报告“Validation”小节，未宣称 race 通过 |

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
