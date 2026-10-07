# Plan：资源保留与命令收口

绑定[第三批 Spec](product-simplification-03-resources-cli.spec.md)，基线 `8e46537fe8b4b86f117fddaa215fc633279b22a4`（前两批已合入），分支 `codex/product-simplification-resources-cli`。本任务由用户显式指定，不据草稿自动选任务。

## 实施顺序和所有权

1. 丢弃事务和事实只生成原执行停止待办，字段收口为 StopPending；删除 PR/worktree/branch 清理待办、进度和回执、消费循环。保留完整已知 PR 集合核验、原绑定、停止/未知回执对账、实际活动槽、晚到事实、合并冲突和唯一 Delivery。停止回执事务按原身份核验；不把资源保留当业务阻塞。
2. 删除无产品消费者的 publication.Close 和 close 配置、workspace 破坏性清理；测试自有 scratch/PR/分支收尾留在 testkit/E2E，不新增测试专用生产 API。按调用资源分类保留原生 ack/release、协调排空、连接/句柄、探针/反馈临时文件收尾，原生协议行为不改。
3. 候选详情保留 PR、后端/目标、工作区/原生 ID、分支和真实执行状态，显示“保留，需人工处置”及采样时间；不声称存在/清理，不添加清理按钮或状态机。
4. CLI 删除 tasks/stop/projects/task export；未知入口在配置打开前拒绝。保留 console、三个宿主入口、control/answer及原请求查询、完整任务命令和内部 launcher。graph 仅 config/cursor/limit，删除高级 Filter/projection/export；沿同快照分页、直接关系、依赖资格和边界。核心 DAG 环检测/稳定排序/补前置不改。
5. SQLite 15→16（Task 停止待办与 publication 配置字段）；graph 协议 native-task-graph/v1→v2（删除高级过滤契约），cursor 1→2；运行 checkpoint 19 不变（Task/publication 不存其 payload），fixed profile 7→8（冻结 publication.Provider 删除 close），Codex codecs2/执行全局格式2不变。旧输入严格拒绝，旧数据库/graph cursor 保留现场并拒绝；不迁移/回填/双写。若实现证实其他持久所有者承载改变字段，再按实际契约调整并记录依据。
6. 更新当前文档/help/启动器/fixtures，记录删除与保留消费者、副作用分类，以及三批 internal+cmd 同口径行数（生产与测试分列，含注释空白，排除 docs/日志/integration）；不重复计算既有候选保留或第二批认证删除收益。自查、提交推送独立 PR。

## 测试计划

证据根 `docs/acceptance/product-simplification-03-resources-cli/`；完整原始输出留本地，提交精选摘要与范围，失败不覆盖。入口见[测试指南](../testing.md)、[E2E 约定](../../tests/e2e/README.md)。

| 级别 / 组、场景 ID | 环境与命令 | 时点 | 通过条件与证据位置 |
| --- | --- | --- | --- |
| 定向 PS3-01/02/03 | Windows、临时 SQLite/可控双候选/原停止回执；`go run ./cmd/test-feedback -out=<新目录> -- ./internal/execution ./internal/task ./internal/task/model ./internal/storage/sqlite -run 'Test(PS3|NativeDiscard|NativeCleanup|Candidate|TaskDiscard|Discard|Delivery|WorkerFeedback|WorkerLifecycle|WorkerFlow)' -count=1`，按已有顶层名收窄长用例 | 事务/消费者完成后 | 无产品关闭/删除；停止未知、晚到、冲突/唯一交付与兄弟资格通过；directed-resource/summary.json |
| 定向 PS3-04 | Windows、临时数据库/公开 CLI；`go run ./cmd/test-feedback -out=<新目录> -- ./cmd/steerline ./internal/product/query/taskgraph ./internal/product/query ./internal/product/client ./internal/app/host ./tests/integration -run 'Test(PS3|NativeTaskCLI|NativeTaskGraph|Graph|TaskGraph|NativeGraph|Predecessor|TaskPredecessor)' -count=1` | CLI/图查询完成后 | 退役入口/参数副作用前拒绝；基本分页和直接关系、旧格式拒绝、补前置通过；directed-cli/summary.json |
| L1 当前固定集 | Windows，无模型；`go run ./cmd/test-feedback -level=L1 -out=<新目录>` | 定向通过后 | 12组 passed/complete=true，无退役零匹配/skip；l1/summary.json |
| L3 controlled / PS3-05 recovery.waiting-merge-restart | Windows、真实程序/Git/SQLite、可控 worker/provider；`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart' -out=<新目录>` | 接线和定向通过后 | 原 Task/Execution/工作区跨重启保持、等待后唯一交付，测试自身清理完成；controlled/summary.json/scenario.json |
| 静态及构建 | Windows；`go vet ./cmd/steerline ./internal/execution/... ./internal/product/... ./internal/storage/sqlite`、`go build -o bin/steerline-resources-cli.exe ./cmd/steerline`、`python scripts/check_doc_links.py`、`git diff --check` | PR前 | 构建、边界、文档、差异通过；验收 README |

仅新增能捕获破坏性调用、保留/停止分离、公开退役入口和采样边界的最小断言，复用已有双候选、停止身份、迟到结果、合并冲突测试。PS3-05选等待合并重启以覆盖冻结 publication 字段变化与资源保留；正常交付由该场景最后阶段覆盖，无需第二个 controlled 场景。不选 Full、完整 L2 或 live：本批删除产品破坏性接口及高级读取，不改变原生 ack/release 操作；如实际必须改变原生协议，先补对应最小证据再交付。

## 实际选择与结果

实现版本为SQLite16/fixed profile8/graph及cursor v2；checkpoint19/Codex codecs2保持。删除与保留消费者、副作用分类、实际源文件位置和行数见[验收记录](../acceptance/archive/completed/product-simplification-03-resources-cli/README.md)。

资源实际定向采用TestPS3、MC6（弃候选/多合并/晚合并）、MC8（晚启动/晚反馈/未知停止）、三个TaskDiscard、ExecutionStopIntent、两条WorkerFlowDelivery、冻结provider重启/漂移及SQLite旧格式拒绝；28项27通过，1旧停止断言修正后只重跑该项及Orca停止/remote ack共6项，全部通过。精确原命令与结果在对应summary.json，原失败不改写。CLI/规则最终42项、console完整112项通过。UI发现并修复丢弃Task旧问题/合并提示错误；补验console全包、L1，不扩展live。

当前固定L1 12组passed/complete=true（110.085秒）；L3只选recovery.waiting-merge-restart，保持30秒普通命令/5分钟业务及原清理期限，通过164.934秒，cleanup_complete=true。初次旧AssertFullAccess编译失败修正为既有FixedNativeLaunch；随后并发运行task create超时，原失败留存。独立运行通过，未放宽期限，负载影响只作推测。E2E基础设施tag编译、当前构建/go vet/文档/diff通过。不跑Full/完整L2/live：产品原生release/ack及adapter协议无变化；已有保留所有者的可控定向与一个L3覆盖当前改动。

行数按8e46537f与最终internal+cmd（含空白/注释、分列测试，排除docs/日志/integration）比较：审计后生产净减1291、测试净增172；三批各自差值生产合计3919、测试合计净减960，不计其他PR或重复收益。完整原始证据留本地，仅精选摘要入库。最后局部变量/注释改名不变行为，历史PR显示由最终console全包覆盖，不重复原生或L3。

独立实现已提交[PR #238](https://github.com/erranliu/SteerLine/pull/238)，质量审计与必要复验完成，交付状态以PR为准。

## PR #238 合并前质量审计

审计范围为丢弃后的原执行停止、终态 Attempt 与资源状态分离、精确回执绑定、客户端最后提交门和当前 CLI/图分页。发现直接修复，不扩大产品清理能力。

| 级别 / 场景 ID | 环境 / 命令 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| 定向 PS3-AUD-01 | Windows，无服务外发；`go run ./cmd/test-feedback -out=<新目录> -- ./internal/console -run '^TestPS3Audit' -count=1` | 修复前应复现；修复后已丢弃Task即使保留旧问题/版本也不能外发、草稿保留 | audit-ui-before/after摘要 |
| 定向 PS3-AUD-02/03 | Windows，临时SQLite、可控执行/原停止handle；`go run ./cmd/test-feedback -out=<新目录> -- ./internal/execution -run '^TestPS3Audit' -count=1` | 复现支持后端被host过滤、只有owned handle和终态Attempt不能结算；修复后原操作只发一次，停止/历史分开、全绑定校验、无资源删除 | audit-stop-before/after摘要 |
| 回归 / 生命周期与停止接口 | 无模型；PS3原子回执、原停止未知重放、资源保留、Orca停止/远端ack定向和console全包 | 修复接线后；当前已有原绑定/未知/迟到/原子约束保持 | audit-regression摘要 |
| L1 | Windows，`go run ./cmd/test-feedback -level=L1 -out=<新目录>` | 合并前；固定12组passed/complete，不更改期限/选择 | audit-l1摘要 |
| 静态 | 产品构建、受影响go vet、文档链接、diff检查 | 合并前；全部通过，无新生产Close/Delete入口 | audit摘要 |

不改变原生adapter stop/release/ack实现或增加远端取消，只按已有后端能力调用原精确停止/查询；控测不冒充native/live证明。既有L3等待合并重启已覆盖普通交付，若审计修复改到它的生产路径则重跑同一controlled场景；否则保留原采样说明，不再开live/Full。

审计结果：停止/原子专项10项、生命周期回归14项、console/CLI/规则130项、L1 12组（123.448秒）、MC6获胜/落选/多合并/迟到场景及同一L3 controlled重启交付通过。修复已注册非local支持被过滤、owned-only丢失、终态Attempt结算和非OS限制；完整绑定/原操作格式加强，物理停止与冻结业务历史分离。保留观察流排空及迟到输出，明确停止后不计活动槽；客户端最终提交拒绝已退出Task。原生adapter协议及生产资源保留不变，不跑Full/live。最后winner冻结状态限定由MC6定向复验，构建/vet再过，未追认L1/L3为最后提交全部重跑。

同步main 2d0e811b的acceptance工作流，所有本任务验收方法、失败/通过记录和精选证据以[专属记录](../acceptance/archive/completed/product-simplification-03-resources-cli/README.md)为准；原始报告沿原实际输出路径留本地。当前行数1291生产净减/172测试净增，三批合计3919生产/960测试净减；其他main改动仅文档，不计收益。
