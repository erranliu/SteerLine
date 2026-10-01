# #62 通用执行事件与问题边界验收

范围：独立 Spec/Plan 的通用事件、问题与回答路由，保留单写者、原资源身份、回执和人工恢复门槛。root WorkerEvent 不携带原生 envelope/messages，codecs 归实现，中心保存通用问题与版本/用途/身份/摘要绑定的 64 KiB 路由封套。旧问题请求/待答字段明确拒绝，不迁移或双写。

## 独立复审修复

- 整合主干 `f5ef8c0` 的纯投影拆包，决策请求/答案校验唯一归 contracts，projection 使用严格格式/路由身份核验；root 不恢复第二套 Runtime 投影。旧格式拒绝测试经实际 projection.Run 执行，现行 projection fixture 补新格式与 QuestionID。
- 通用 start 消费仍调用 Codex UUID 规则：改为中心 recordWorkerTurnStarted，只按固定输入/Turn 与保存的 Operation 接受；原生 ID 规则继续由 Codex decoder 拒绝。非 UUID start、重复不增加轮次、不同 pending Turn 拒绝、gap 不结算的回归在修复前失败、修复后通过。
- Orca raw 问题/选项的首尾空白会与保存时规范化的内容冲突，导致丢失 ack 后无法对账：解码器输出统一规范化内容，同一消息重放与保存值一致，回归先失败后通过。
- call ID 去重缺少 Turn/代次范围：按完整 Project/Workflow/Execution/Attempt/Generation/Turn/Question 身份核对，前一轮同名 call 不阻止新问题；原身份内容变更仍拒绝，回归先失败后通过。

## 实际验证

最终 `go run ./cmd/test-feedback -quick -- ./internal/steerline ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1 -run '^(TestCD62NativeFactsAndQuestionRoute|TestCD62QuestionReceiptBarrier)$'` 通过（73.5s）。quick-core 1 包/6 测试事件、quick-rules 9 包/68 事件、quick-targeted 3 包/17 事件；每个 Plan 入口和新增修复回归均实际命中。原始日志在系统临时目录，不纳入仓库。

受主干整合和共用 start 影响，追加 projection 与 app-server 两轮、宿主阻断/原问题代表用例通过（56.1s）；projection 纯包此前全包测试通过。构建产品入口、受影响包 go vet、execution/contracts 依赖图、roadmap_task_id.py check 与 diff 检查通过。未跑全量、真实 agent 或权限/双机认证，remote Claim 缺证拒绝保持。

## 旧到新测试与剩余所有者

- Codex 事实/问题及答案证据、未解决问题 journal replay → codexworker/event_test.go 的 CD62 用例及子用例。
- Orca 原问题、迟到 Dispatch/替换终端 → orcaworker/event_test.go 的 CD62 用例及子用例。
- 中心完整回答、下一问题推进前保存接受状态、失败提交不 ack → worker_events_cd62_test.go 的 CD62QuestionReceiptBarrier，原事务断言保留。
- 配置/完整原生绑定与目录外移交 #63/#66；原生启动、恢复、结算协调交 #64，具体历史采集/宿主解释交 #65。这些有生产消费者，不宣称 root 已零平台文件或零关键字。

活动表移出 #62，保留身份及下游接口引用；TaskDelivery、发布责任与 remote 权限支持范围未扩展。
