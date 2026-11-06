# 待合并 PR 冲突解决与原 Run 续作 Plan

2026-11-06。对应 [Spec](../spec/pr-conflict-resolution.spec.md) 和[验收记录](../testing/acceptance/pr-conflict-resolution.md)。用户已授权审计修订后实施；Runner 在独立 worktree 修改并提 PR。不启动 roadmap。实现阶段未部署；后续用户已明确授权再次审计、修复、合并并按正式入口部署，不改变功能方案。

## 1. Runner：扩展已有 message

在独立 Runner 仓库完成以下改动，复用现有执行机制，不引入冲突专用 Workflow。

1. 在 `internal/contracts`、`internal/resources/catalog.go` 及既有 CLI/host 路由中，把 message 的允许范围扩展到已完成开发的原 Run，保持现有 revision/text 信封；公开组合的 operations 声明 message。`internal/runstate/record.go` 的 can_message、can_reply 和实际受理保持一致，修正已有“显示 pr_ready 可 feedback、实际拒绝”的不一致。
2. 在 `internal/application/engine.go`、`worker.go`、`content.go`、`freeze.go` 与结果收尾处复用原消息冻结、可靠输入、问答和 final 机制。开发 Run 的 message 走调用方正文执行，不走 `feedback.go` 的 implement 返回边或 Workflow 结果解析。不要把整套 conversation 只读/材料/关闭逻辑搬到开发流程，也不创建通用策略框架。
3. 正常发送直接使用原工作区、原会话和现有执行配置，不新增 Git/平台/环境查询，不为该入口调用 `qualifyContinuation` / `VerifyManualRecovery`。原生调用、停止和未知请求继续沿现有可靠执行机制，不新增恢复资源的能力。
4. 保留 pr_ready 的开发步骤、PR 绑定与既有审计记录；消息活动、问题和本轮结果使用现有命令/执行/事件字段。将每轮 final 与对应输入关联并留存于原历史，结束后可显式发下一条消息，原问题可正常回答。保持原停服、stop 和现有结果封存语义，不将 result_file 改造成逐轮结果文件。
5. 覆盖原已支持的开发执行组合；必要的适配器修正在 Runner 侧完成，SteerLine 不接管原生链路。同步 Runner 的 CLI、Workflow 说明和能力测试，提交固定构建及定向证据。

Runner 交接只需：实际 commit、EXE/version/SHA256/resource_digest、公开 message/schema/组合目录和同 Run 两轮执行证据。调用方可先按该公开契约用 fixture 开发；真实启用与联验必须使用已实现的固定 EXE，不能仅凭 conversation 已支持 message 推定开发 Run 可用。

## 2. SteerLine：按钮、短指令和原请求通道

1. 在 `internal/runnerclient/client.go` 的写操作与命令白名单接入 message，复用当前实例/完整绑定；沿 `runnercontract` 已有 can_message 和公开目录消费能力，不维护第二份执行组合规则。能力不足只禁用该按钮并给出原因，不阻断原候选观察/停止，不退回 feedback、resume 或 execute。
2. 在 `internal/taskrunner/control.go`、`control_validation.go`、`internal/product/api`、`internal/product/httpapi` 沿现有产品命令加入 `resolve_pr_conflict`。根据原候选、任务参与和公开 can_message 决定资格；不增加点击前 PR 刷新、冲突判断、现场核验或新环境资格。原 PR URL/目标分支从既有绑定读取。
3. 扩展 `internal/runnerbridge/bridge.go` 的 QueueControl 和原 Actions 存储校验，允许 message。复用产品 request_id、随机 Runner request_id、冻结字节、原回执恢复和观察循环。保存失败零发送，已发送结果未知只查原请求；同一候选不并发发送，停止/退出/终态沿现有优先级仲裁。不新增续作表或独立后台 worker。
4. 在 `internal/prompts/pr_conflict_resolution.md` 与 prompts 包组装函数实现 Spec 的短正文，只传原 PR URL 和目标分支。不新增完整规则材料、项目文件或 inline 兜底。新增实际请求正文断言；按[提示词维护](../../docs/prompt-governance.md)列出新增正文、0→实际 UTF-8 字节数、完整投递正文和业务依据，取得独立审核。保留原回答限制，不把系统指令伪装成用户回答。
5. 在 `web/src/features/runs/runs.tsx` 的候选详情操作区接入同一按钮及已有原请求核实机制。复用现有活动、answer、final 和 delivery 展示；沿 Runner 公开执行状态显示本轮活动，不能因 Workflow 仍为 pr_ready 隐藏正在执行/等待回答。同步 `internal/taskrunner/input.go` 的实际问题投影，避免将 pr_ready 的旧 feedback 表单当作新入口；本次不新增通用消息编辑器。若现有投影把 pr_ready 固定当作空闲，同步调整 `internal/taskrunner/projection.go` 等实际消费者，以当前执行事实为准，不新增业务状态源。
6. 同步 `docs/integration/steerrunner.md`、`docs/product-design/worker-lifecycle.md` 和相关产品说明，仅描述本次 message 增量及现有恢复/停止边界。旧冻结验收不改写，无数据迁移、自动重发、跨构建活动接管或部署。

## 3. 最小测试与登记

具体行为和实时结果归[专属验收](../testing/acceptance/pr-conflict-resolution.md)。遵循[测试指南](../testing.md)和[路径清单](../paths.md)，优先扩展已有 owner、公开 Runner fixture 与真实链驱动，不为此功能建立新的测试框架。

| 验收项 | 时点 / 环境 | 入口与通过条件 |
| --- | --- | --- |
| PCR-A01、PCR-A03 | Runner 接线后 / controlled | 运行 contracts/resources/application 与受影响 adapter 的定向测试，覆盖原开发 Run 两轮 message、问答、原现场不变、无额外预检/阶段重跑、并发/unknown/停止/重启；沿 Runner 正式 L1/L2 入口回归。实际命令在验收中固定。 |
| PCR-A02、PCR-A03 | SteerLine 接线后 / isolated、controlled | 扩展现有 taskrunner/runnerbridge/runnerclient/HTTP/存储/Web owner 和公开 fixture。定向入口：`go run ./cmd/test-feedback -out <新目录> -- ./internal/prompts ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner ./internal/app -count=1`。运行既有 L4 `go.http`、`go.storage`、`web.unit`，验证产品命令、实际短正文、原 message/问答/final、重复点击/保存失败/丢回执、旧 final 和权限终态。 |
| PCR-A01–A03 必要回归 | 提交前 / controlled、isolated | 正式 L1/L2：`go run ./cmd/test-feedback -level=L1 -out <新目录>`、`go run ./cmd/test-feedback -level=L2 -out <新目录>`；运行现有 L3 `recovery.waiting-merge-restart`，验证原候选和请求沿同构建重启不重复投递。 |
| PCR-A04 | 固定两仓构建后 / live | 复用既有 live 隔离/配置驱动，准备原 PR 冲突，经正式产品命令触发，读取实际回复；再次触发无冲突，最后按原方式停止。选择本次实际使用的开发组合，执行前在验收中固定命令、profile、模型和构建；未验证组合如实列出，不据单条链宣称全组合通过。 |

若现有入口无法表达必要行为，只补最小驱动/场景；成为长期 L1–L4 项时同步 suites.json、test-list.json、可读列表与执行器，不预先强制新增场景 ID。驱动可查询 Git/PR 作为验收证据，这不成为产品发送前置；不得代解冲突或代推候选分支。不默认扩大为 Full 或全部 live。缺真实环境或权限时记录具体阻塞并保留未验，不能改称人工体验或用受控结果替代真实链通过。

PCR-M01 在自动闭环后安排，参与人/窗口待定；只阻塞体验签收。提示词正文差异、字节数前后值、业务依据、验证及独立审核必须登记。

## 4. 交付顺序

Runner 公开契约与原 Run 执行 → SteerLine 消息接线和按钮 → 受控回归 → 固定构建真实联验 → 提示词独立审核及文档收尾。实现与证据按两仓分别提交，SteerLine 不导入 Runner 内部包。后续按用户新授权完成合并前审计，修复复验通过后先合并 Runner，再合并 SteerLine，通过既有部署入口更新；停止原运行与升级排空遵循原规则，不因本功能绕过。
