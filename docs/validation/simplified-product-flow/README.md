# 收敛执行与反馈流程验证

基线：#201 后 5aafff4，已含 #199 和 #200。行为依据：[Spec](../../archive/spec/completed/simplified-product-flow.spec.md)，测试选择：[Plan](../../archive/spec/completed/simplified-product-flow.plan.md)。当前改动不修改候选方案和三线模块化路线图。

## 结果

| 统计范围 | 净变化 |
| --- | ---: |
| 主包直接目录生产 Go | -292 行 |
| 全仓生产 Go | -742 行 |
| 全仓测试 Go | -32 行 |
| 全仓 Go，含测试 | -774 行 |

物理行数含注释/空行；生产排除 _test.go，主包不含子包。逐文件前后数量见 [source-summary.json](source-summary.json)。统计相对最新基线，已经计入统一调度、独立环境命令、回归测试及 E2E helper 的新增代码；删除旧产品分支带来的测试减量单列。

## 行为与断言映射

| 旧路径/断言 | 当前行为与证据 |
| --- | --- |
| 环境 blocker 创建 host Decision，DeferredDecision/Feedback 恢复与补投 | 独立 blocker + open_terminal/recheck_environment；原业务问题/草稿保留。受理与消费均拒绝阻断时新输入。没有外部意图的已保存回答 supersede 并保留审计，重新确认；已绑定原 Task/Dispatch/question 的未知操作只读对账。原 ER2/ER3/ER4、重启/nonce/deadline 保留，新 unsent/unknown 场景实际命中。 |
| ActivityWriter/file normalize/raw tail 混用 | 采集和诊断文件保留，产品 Activity 只读原 run/execution/attempt 的数据库事件；游标推进和来源归属严格核验，缺口明确显示。原 raw fallback 测试改为 raw 文件不参与产品读取的反例。 |
| session/turn/反馈/宿主/ack 各自启动与结果通道 | 公用单步 effect、immutable request/receipt、actor 当前记录核验/CAS。慢动作不阻塞 actor，保存 intent 后才执行，原 receipt 提交后才进下一步。刷新投影不使用捕获的旧 Run；观察消息在该 run 的回执后处理，避免覆盖 opaque cursor/Delivery。未知结果不重发；原 receipt/intent/失败/晚到结果断言保留。 |
| worker/host_service/execution_profile 混合发布、host final marker、中心 commit/push/create | 只接受显式 worker；删除中心发布实现/请求、参数组合和 marker。执行环境负责 Git/PR 权限与凭据；中心仍核验原 provider/head/base/merge 并唯一交付，不下发中心认证值。旧中心发布正例改为受控/真实 worker 实际发布；冻结 provider、重启/remote drift、查询/关闭不创建 PR 的断言保留。 |
| 已交付执行的 native cleanup 私有状态更新 | 已交付执行证据保持冻结。清理进度来自原资源的 confirmed operation receipt；native planner 据此逐个关闭 agent/coordinator。新定向用例验证 unknown 不推进、replacement 不匹配，以及 settled CAS 允许回执但不能重写执行证据。 |

SQLite schema 12、runtime/checkpoint schema 17；旧开发数据与被删除的配置字段明确拒绝，不做迁移或回退。原平台/协议、权限身份、manual recovery 和唯一真实合并交付约束保持。

## 最小验证

全部通过，摘要和报告 SHA-256 见 [validation.json](validation.json)，完整日志保留在 bin/simplified-product-flow/。

- rebase 后 L1 quick：15 包、215 parent/subtest，0 fail/skip/incomplete；all tags/Windows/Linux 源码与依赖守卫未放宽。
- 定向：环境拒绝与旧输入处理；canonical Activity；非阻塞/旧 Attempt/刷新投影/terminal late receipt；原 provider 重启；方案选择摘要；冻结执行的清理回执；原资源 cleanup progress。
- 原 stop/discard、worker intent/receipt/CAS/提交失败回归通过；本机双轮、终端原身份/重启和丢失 dispatch/reply/send/ack/task 的既有 Service 集成断言通过。
- L3 controlled：delivery.basic、recovery.waiting-merge-restart，各自通过且 cleanup_complete。
- L3 live：delivery.local-codex、delivery.orca-codex。真实 gpt-6-luna / medium，真实 Git 提交/推送，loopback provider 的 PR 创建/合并与唯一 TaskDelivery；没有用 mock 认证原生执行。本机原生 usage 总数 79131，终端 204937，交付轮询不新增 usage。两条最终场景清理完成。本轮没有调用真实 GitHub E2E smoke。
- 40 包编译检查及 Windows/Linux amd64 产品构建通过。最终改动另跑冻结清理与边界定向检查；不将早期失败诊断当作最终通过证据。

未选 Full、完整 L2 或所有 live recovery：本次使用 quick、受影响定向、两个 controlled 闭环和双 live delivery；既有 Service 集成覆盖原身份的重启与未知结果恢复。

早期回归发现并修正了 pending 被误当失败、原答案版本丢失、观察 envelope/ack 回执顺序、测试材料旧发布责任，以及交付后清理覆盖冻结执行的错误。失败原始报告仍留在 bin；最终成功报告用独立目录，不覆盖旧证据。
