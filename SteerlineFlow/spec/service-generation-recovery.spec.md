# Runner 兼容构建接管的调用方契约

2026-10-19，用户直接授权的跨仓恢复实施，基线 `9554eb61eefa417f5cd900d335650dacff3d3311`。与 Runner 工作树 `docs/spec/service-generation-recovery.spec.md` 配套；本仓只消费公开 CLI，不读取 Runner DB/IPC，不解释原生恢复状态。规则归属 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md) 的 W02–W18/E04–E07/V06–V08。[Plan](../plans/service-generation-recovery.plan.md) 与[验收](../testing/acceptance/service-generation-recovery.md)分别维护验证安排与实际结果。

1. 所选 EXE 仍逐次校验绝对路径/SHA/version，活动服务仍逐次验证原 instance/state-dir/当前构建。历史 candidate.RunnerBuild、Check.Selection/resource digest、原 submit/control 信封、Run/input digest、PR/Delivery 事实不可改写。兼容接管只更新 application-api/runner-build.json 和当前 service checkpoint 的 pin。
2. 换构建前调用 `service verify-takeover --candidate <所选 EXE>`；必须同目录、候选 SHA、contract=steerrunner-takeover/v1、全部逐项 compatible/read_only。原 Run 用 ID/input digest 关联；丢失 submit 回执用原 request_id 加 commands 的 canonical request_digest 关联，不能仅凭同名/同标题或重发 submit。读取受限原资源不因此获得新授权。
3. 服务启动前按既有项目锁和 caller Store 检查全部已有候选及停止义务。已有未知 stop 保留原 ID/正文/revision；尚未创建 stop 信封时才用预检中的原 Run revision 保存首次 stop 到既有 outbox，再通过裸 pending_stops 提交。Runner 在 worker/ready 前受理。重复启动重用同一信封；明确拒绝才按既有流程处理，未知不能换 ID。原提交尚无回执时只关联已接受请求，不能创建替代 Run。无法读出原项目停止义务则阻止共享服务启动。
4. 原 Runner 已 ready 时，把原 stop 交给当前精确实例；不将启动输入作为在活动服务中批处理的替代接口。停服与资源退出分别取证；resources_settled=false 不阻止经证明的兼容重启，也不得被重写为 true。
5. 部署先对 staged Runner 做在线只读预检，失败不得停旧服务或替换文件。旧应用/项目/Web/锁/checkpoint/冻结配置的关闭门槛不变。verify-restart 保留原 RunnerBuild、原 service instance/request/SHA 的退出事实，跨构建另输出 candidate_build/takeover；实际接管在新服务取得原 writer lease 后发生。普通部署继续默认复用 Runner，显式 UpdateRunner 才构建新 Runner。
6. SteerLine 不自动生成 submit/reply/resume 或 workflow prompt。Runner 按其原持久授权对账并继续已证明未发送的后继；原 paused/waiting/blocked/stop 保持。Task/DAG 与唯一 Delivery 仍由本地事务权威决定，不增加另一套执行状态机。

Orca 创建成功但原响应丢失仍受原生创建关联/查询接口限制；消费者不能凭列表或新资源绕过。本机 Codex 的兼容 Runner 更新、原结果补收及继续交付资格已在 2026-10-20 独立 live 包补验，实际范围和证据见[验收](../testing/acceptance/service-generation-recovery.md)。其他原生路径及 Orca 窗口仍分别待验。未触碰安装或保留运行目录。
