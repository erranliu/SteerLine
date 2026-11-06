# 测试生成受阻状态展示修复

2026-11-06。实现基线 `3ee95909df9dd5cb9668e7f00c750d3677804e1e` 加本工作区变更；基于已合入的异步活动中心 #465。普通开发，不进入 roadmap。

## 范围与通过条件

现场只读检查发现 SteerlineTest 的 #102 生成请求为 accepted，原 Runner 为 blocked/manual_intervention_required，prepare_session 的 app-server 连接被拒绝，尚未开始模型回合；维护页仍统计为生成中。本次只修调用方的状态展示与现有重试资格表达，不修复 Runner 原生启动、不停止或重试现场、不部署。

- TGA-A1：Generation.NeedsAttention 只读取原结构化快照；正常、无快照、诊断文案相似、已取消及已结算记录不误报，不改变 accepted/Settled。
- TGA-A2：维护页 4/5 就绪、1 项受阻时停止转圈并显示原因；仍有其他生成执行时保留运行提示。activity 进入 attention/ongoing，未结算不进入 recent，历史批次或替换后的生成不重新触发当前提醒。GET 不改变原记录。
- TGA-A3：原受阻 Run 继续被既有 ObserveNext/维护 worker 观察，无新增 submit/stop；迟到的原成功可完成批次。attention 为 running 子集，不改变批次终态判断。
- TGA-A4：前端使用后端已有 retryable 资格；取消意图已保存但未结算时显示等待确认、禁用重试。原停止确认后才允许一次显式新尝试。

## 实现

单一纯判断放在测试生成 model，维护查询和存储只读活动适配共用；原业务 state、队列、worker、停止 outbox 和请求身份规则不变。查询增加少量派生字段，无新表、schema 升级、后台任务、控制接口或提示词修改。维护页原重试入口消费后端 can_retry，生成详情消费 Settled 的只读结果。

## 验证与证据

所有输出位于 `C:/SteerlineDebug/wt-ef60-generation-attention/`；受控 fixture 不调用真实模型或 GitHub，不把受控通过视为 app-server 原生启动已修复。各次 summary 保存实际命令、源码状态、执行参数与原日志定位。

| 范围 / 命令（均以 `go run ./cmd/test-feedback` 开始，显式传本行目录为 `-out`） | 状态 | 本 scope 下的独立证据目录 |
| --- | --- | --- |
| `-- ./internal/testmanagement ./internal/testmanagement/model ./internal/app ./internal/product/query ./internal/storage/sqlite -run 'Test(GenerationNeedsAttention\|GQ\|TMGeneration\|AsyncActivity)' -count=1` | 初次失败：前四包通过，SQLite 新夹具恢复步骤未复读已保存对象，触发不可变关联校验；已修正夹具，随后由 storage 全集复验通过 | `diagnostics/20261106T071825075Z-eca76c12efdc4075988fade608ebf344/summary.json` |
| `-level=L1` | 已验收，22.4 秒 | `test-results/L1/20261106T072005283Z-ad7d739c7a824fef9ead0cfd7584526a/summary.json` |
| `-level=L2` | 已验收，110.2 秒；受控交付与清理通过 | `test-results/L2/20261106T072100961Z-5f5bc2f8fc94430c8c2b48a16bd93469/summary.json` |
| `-level=L4 -suite=web -scenario web.unit` | 已验收，336/336 | `test-results/L4/20261106T071901487Z-439e9332328f4f62aa790abdb47ec6ab/summary.json` |
| `-level=L4 -suite=go -scenario go.storage` | 已验收，94.0 秒；含修正后的 4/5 原生成只读投影和 attention 计数回归 | `test-results/L4/20261106T072046706Z-45d0b98c22f9448e9eab5f2b69e6ccc0/summary.json` |
| `-level=L4 -suite=go -scenario go.http` | 已验收；与 L2/L3 串行，保留 HTTP 全集及原快照压力范围 | `test-results/L4/20261106T072519880Z-33407fc40fc94cdb95c2a93b04baccda/summary.json` |
| `-level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities` | 已验收，121.9 秒；HTTP/浏览器、原活动重启、零额外输入及资源清理通过 | `test-results/L3/20261106T072245380Z-2ef2636c89de4cb39854fb9fbf1ef1fa/summary.json` |

首次 L3 停在准备阶段：新 worktree 没有场景要求的预构建 `web/server/dist`，未执行场景；失败证据 `test-results/L3/20261106T072131032Z-905de5c1e7d342b194779fde51258774/summary.json`。显式执行 `npm.cmd --prefix web run build`，TypeScript 与生产构建通过，日志 `diagnostics/web-build-20261106T072218164Z/build.log`；在新目录重跑后的结果独立登记于上表，不覆盖原失败。

静态检查：`git diff --check`、`python scripts/check_doc_links.py` 通过。TGA-A1/A3/A4 的 Go 行为证据在定向报告中，TGA-A2 的真实 SQLite 与 Web 行为由上述两个 L4 覆盖；L3 是现有活动观察流程回归，不宣称真实 app-server 启动已修复。

自动回归通过不等于人工体验通过；人工体验待安排，安装现场与 #102 原 Run 保持未操作。本次没有提示词正文、组装、资产或对应提示词测试/CI 变更。
