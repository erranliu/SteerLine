# Spec 发布与 PR 实现迁移

任务 ID：56；任务名称：发布与 PR 实现迁移；目标：`EXECUTION-ISOLATION`。状态：已实施，待合入。配套 [Plan](execution-isolation-03-publication.plan.md)。硬前置：[#54](execution-isolation-01-contracts.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

建立 `internal/steerline/publication`，把 Git 发布和 PR provider 的外部操作从中心接线分离。遵守[共同约束](execution-isolation-constraints.md)，保持当前本机受控发布和 Orca worker 发布行为；项目发布责任配置由 #60 交付。

## 文件与契约

| 文件 | 职责 |
| --- | --- |
| `request.go` | 固定执行/轮次、工作区/分支、base、待发布 HEAD/index-tree 和请求摘要 |
| `publish.go` | 宿主提交、推送、建 PR 的外部操作及已有发布的查询对账 |
| `pr.go` | PR 身份、head/base、状态、merged-at/merge-commit 的外部事实与身份校验 |
| `provider.go` | 生产消费者需要的查询、创建及关闭能力；不支持时明确拒绝 |
| `gitea.go`、`command.go` | 现有 provider 实现和配置验证，不扩大支持范围 |

迁移 `pr.go`、`worker_publish_local.go` 中的外部操作及必要 marker 验证，中心保留工作轮终态关联和发布请求受理。现有宿主发布的 GitHub command provider、精确仓库/执行分支、base、已暂存改动及 HEAD 核验仍成立，不把搬包解释为支持任意 provider 发布。

保留已有轮次用途与发布资格：readiness 的工具结果、worker_done 和 PR 文本不受理为业务发布请求。宿主阻断不能覆盖已核验的 PR 合并事实，发布包不自行清除 blocker 或恢复业务派发。

请求接收固定小输入，不接受 Service、Run、数据库或可写业务指针。工具和认证来源通过非秘密引用或运行时依赖提供；宿主发布路径不向 worker 下发宿主凭据，worker 发布仍使用其目标环境自己的认证。PR 正文/错误/日志不能泄漏 token。结果包含平台身份、Git 进度事实及未知边界，不能用一个 bool 代表提交、推送和建 PR 全部完成。

共享命令、PR 状态/交付记录和查询 DTO 保持 contracts 中的唯一数据定义。publication 的 provider 事实与外部操作不接 query/sqlite 业务读写，中心通过既有读端口核对并在原事务关联/交付；不因搬包重建 QueryService 或删除指向共享值的 facade 别名。

## 持久化与失败边界

中心先保存发布意图及本次候选证据，再执行外部动作，之后分别持久化已核实事实和 PR 关联。意图提交失败不能 commit/push/create；外部操作超时先按原 HEAD、分支、请求和平台 PR 查询对账，不能重复提交或盲建 PR。

迁移 `reconcileLocalWorkerPublication` 时保留当前只查询、不重试发布的边界：核对保存的 index-tree、起始提交 ancestry、精确远端分支和唯一匹配 PR。未找到 PR 或不能证明远端 HEAD 一致仍是未知，不能由新包自动 commit/push/create 以修复现场。发布回执与业务变更使用现有显式事务。

保持已有 Git clean/staged 发布条件，只将检查迁移到对应发布阶段，不新增中心项目测试门槛。PR 查询失败保留原链接和事实；closed-unmerged 不计交付。TaskDelivery、关闭 Task 和后继解锁继续由原唯一事务实现，provider 不能自行结项，清理失败不能撤销已交付事实。

## 并行与交接

与 [#55](execution-isolation-02-workspace.spec.md) 无硬依赖，可以并行。使用 #54 的工作区和身份契约，不复制 #55 的资源解析。共享 Git command 输入及中心发布接线由集成人串行处理，后合入方检查双方接口；#57 在两项均合入后接收稳定边界。

#60 使用本任务的请求和 provider 能力，改变发布责任归属；本任务不能提前增加策略默认值或按能力自动切换发布责任。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| EP1 | publication 不依赖中心或业务事务；PR/provider 与 Git 发布生产调用使用唯一实现 |
| EP2 | 现有本机隔离 Git 发布有效链及错误 marker/执行分支拒绝通过，意图先于副作用 |
| EP3 | provider 精确身份和真实合并门槛保持；未知发布只查询原候选，不重复 commit/push/create 或 TaskDelivery |

复用临时 Git、mock provider 及交付测试，不要求真实推送、人工合并或 provider 全矩阵。
