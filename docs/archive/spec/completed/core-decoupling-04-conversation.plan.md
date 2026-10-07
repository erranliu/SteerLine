# Plan 会话采集与宿主交互实现隔离

任务 ID：65；任务名称：会话采集与宿主交互实现隔离；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-04-conversation.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#64 整体交付；一个实现 PR。

## 实施顺序

1. 划分发现/读取/映射/提交/cursor 推进各环节，冻结来源与采集端口，按当前 history client/mapper 唯一实现迁出。
2. 中心采集器切到通用批次及 commit 成功后的消费确认；mapper 候选状态随失败提交丢弃，并从原 checkpoint 重建同一批次，保留节流、当前/历史优先级和 Attempt 边界；工具字符串化 wrapper、原 agent 识别等待和 deadline 语义沿用当前实现。
3. 将宿主观察/探针/工具解释与动作归入实现端口；纯探针生成与文件写入分开，probe/ignore 写入、清理和打开原 terminal 经 #64 已持久单步；中心准备状态只处理共同事实和人工动作资格。
4. 同步 Service 字段、实时/重放/历史消费者与查询组装；迁移对应 fixture 并删除旧采集/宿主原生接线。

## 所有者与删除

`codex_conversation.go`、`codex_history_port.go`、conversation_service/replay/position、Service.conversationMappers 与具体通道、`worker_environment.go` 的平台调用及 workerflow 就绪消费；保留 PR176 的 query/sqlite 会话只读分层。

删除 root 的 codex_conversation/history_port、具体 conversation mapper 字段、采集原生 callback 及 worker_environment 平台 helper；保留通用采集协调/准备业务状态。平台 fixture 随采集/宿主实现迁包，生产无重复 mapper 或并行查询读取实现。

## 验证与命中核对

会话只读 pure 包及中心来源 commit/cursor 代表用例；复用错误 workspace 拒绝、来源替换/晚到 final、raw/body 损坏和 Unicode 页面。准备侧选 ER1 正向/工具失败、ER3 重查竞争、ER4 固定期限和重启确认，按本轮变化挑必要子用例而不重复全矩阵。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerline ./internal/steerlineintegration -count=1 -run '^(TestCD65SourceCommitAndHostGate|TestCD65HistoryAndPreparation)$'
go list -deps ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/orca_remote_worker
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

这些命令在本次规划中未执行。Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查，实际命中数量与旧到新映射随实现记录。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD65-1 | TestCD65SourceCommitAndHostGate | 中心中性来源批次提交失败不消费 cursor 或 mapper 候选，重试产出同一批次，准备/宿主资格仍由唯一业务状态决定。 |
| CD65-2 | TestCD65HistoryAndPreparation | 外层复用来源绑定/晚到 final/正文完整度及必要准备、重查、重启门槛断言，平台 raw fixture 不再留根包。 |
| CD65-3 | 只读包与结构检查 | 保留 query/sqlite 的会话读取断言；核对 Service 不持有具体 client/mapper，来源实现没有 SQL写端口。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

#66 获得具体类型已隐藏的 Service/采集/宿主端口，可在外层构造全部依赖；#67 接收实际迁移后的来源 fixture 和测试入口清单。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。

## 实现接线记录

所有者、删除与原断言迁移见 [验收记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-04-conversation/README.md)。会话事件 2、SQLite 11 / runtime 16，native history position 带明确来源封套与 64 KiB 上限；旧格式拒绝，不添加迁移。原生 fixture 随消费者归实现或外层，中心保留预算、内容/位置事务、准备/业务资格和人工门槛。协调端在执行聊天停写后完成权限环境交接、独立复审修复和组合验收；诊断通过不代替最终 head 的 JSON 命中。

最终组合验收：clean `942622ead61615fc636e52c075fa7d6cec330ed8`，core 6 / rules 90 / targeted 29 全通过，326.5s；命名入口与 root/outer 所有所选子用例实际命中。源码与最终文档更新的等价性、审核/合并 head/base 由 PR 审核记录说明，报告路径见验收记录。三实现依赖、产品构建、diff 和 task identity 检查通过；未跑全量/真实认证。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/archive/completed/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
