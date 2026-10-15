# Plan：本机 Web 前端与 DAG / 执行闭环

范围归[Spec](web-frontend.spec.md)，结果归[专属验收](../acceptance/web-frontend.md)。2026-10-14 用户直接授权本聊天实施，独立项目路径 `web/`，不启动 roadmap 调度。已读取 `docs/spec/SpecGuide.md`，规范版本由本次工作区 Git 差异记录；实施基线 `a81341a5`，现存技术方案文档修改保留。

## 代码调查与整合

- `internal/product/httpapi/server.go` 强制 native loopback/Host/Bearer/无 Origin。`httpclient.Client.Get/Post/RefreshIdentity` 已映射全部必要协议，可由浏览器服务复用，无需新增业务查询/命令权威。
- `api/pages.go`、`graph.go`、`httpcontract/contracts.go` 是实际 JSON/有界分页/信封与回执来源；部分嵌套值无 JSON tag，TS 必须按实际大小写建模。
- `app/app.go`、`execution_service_wiring.go` 统一拥有 native endpoint 生命周期。只增加 Web 配置和最小接线，Web 实现进入 `web/server`，不改 worker/actor/adapter 所有者。
- 真实浏览器回答暴露 `clientDecision.Submit` 已打开命令 store 后又调用另开 store 的 Bound 入口；本批改为将原 store 传给既有 bound 提交函数，保留 attempt/recovery/版本核验及原事务，避免重复 writer 初始化。相应 app/决策回归及新浏览器反馈场景一起验证。
- 原生回答受理含多次宿主/状态/数据库核验，Web 转接使用 `httpclient.OpenWithTimeout` 的独立两分钟有界预算，native/Web 两端写响应预算均覆盖它；普通 native 客户端仍使用原默认值。超时仍保存原信封并按原 ID 对账，不能被视为成功。浏览器用例总预算覆盖冷启动及各阶段显式上限。浏览器离线直接绑定 Query 的 onlineManager；暂停查询仍保留缓存，不能据缓存启用写按钮。
- `httpfs` 提供原子替换和当前用户 ACL，可复用 Web discovery；文件放应用 home 的 `ui/web/<project_id>/`，保持 native state/local-api 严格白名单；CLI 经该受限文件申请一次性打开票据，并核对原 native discovery，浏览器不读取 native credential。
- 调试发布由 `scripts/new-debug-release.ps1` 和 `debug-template/` 维护。新增显式 WebAddress 选择与构建；simulate 仍通过生成包 `start.bat --simulate` 冻结，不能使用安装目录。
- #92/#93 与 Worker 三层重构可能并行。保持 httpcontract/wire 与 existing native client methods，现有 HTTP 路由/业务包不改；最终按实际工作区再次核对共享 app 接线。

## 实施步骤

1. 创建 `web/package.json`、lockfile、Vite/TS/Vitest/Playwright 配置及 feature/shared 分层。浏览器服务置于 `web/server`；带 `webui` build tag 嵌入 `web/server/dist`，无 tag 明确返回产物不可用。生产不需要 Node 常驻。
2. 实现 Web 配置、固定 loopback listener、私有 discovery、CLI 请求短票据/打开浏览器、会话与 CSRF、静态边界/安全头。建立复用 native httpclient 的认证 API 转接；按现有 HTTP 错误保持 code/status，不按连接失败伪造 command_not_found。
3. 实现 TS wire 类型、fetch 与 IndexedDB 请求/草稿。补原信封不可变、Web Locks、原 ID 查询、实例和回执匹配/终态保护；查询按身份隔离并取消失效请求。
4. 迁移原型布局至公共组件，接 DAG/任务列表详情及分页、执行候选/Activity/final/问题、方案选择、启动/派发/停止/查看原对话动作。按钮按已观察能力和资格约束，失败/unknown/离线可见。大 final 通过现有正文分段能力读取。
5. 提供 `web/README.md` 与 `scripts/test-web.ps1` 定向入口。调试模板和生成器接受 WebAddress；构建产物后生成新的 simulate 包，再测试真实浏览器接线。稳定 npm 测试及浏览器场景归 `web/tests`，定向入口登记至测试指南；新增 Web owner Go 测试进入固定 L1，避免扩大现有 full。Web simulate fixture 显式允许一次原 thread 的 publish feedback，其余 fixture 保持原三步骤严格序列；浏览器驱动必须实际提交并核对同 session/execution 新轮次，不以草稿保存代替回答接线。
6. 修复定向验证发现，核对独立路径与 shared app 接线，记录 acceptance 与更新技术/架构索引。用户追加要求审计、直接修复并提 PR；审计完整 Web 差异、整合最新主干、定向回归后提交并创建 PR，不自动合并。

审计补充：提交入口先复制调用方信封，保证锁/持久化等待期间不被外部改写；对账按完成后调度下一轮，切换实例/离线时取消读取及尚未开始的补交；回答资格查询在关闭、候选/版本变化或离线时取消，晚到结果不能提交。分段 final 拒绝重复游标、其他执行/Attempt 的正文引用和越界片段；已结束候选明确显示历史问题。

主干整合补充：已读取[Worker 生命周期规则](../product-design/worker-lifecycle.md)。三层重构后的 PR 绑定只更新事实，不再替反馈设置 waiting_input；因此既有可选问题生成器必须同时保留 pending 与 answer_saved，避免 PR 轮询替换尚未投递的原答案。本批只修正 W12/W17 的决策保留条件，复用原 actor/反馈入口，不新增恢复或重试流程。

## 测试计划

所有项链接[本任务验收](../acceptance/web-frontend.md)。命令从仓库根目录执行；原始产物写系统临时目录或新独立调试包 evidence，验收文档只记最小摘要。

| 验收项 | 级别 / 组 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| WEB-A1 | 定向构建 `web.build` | Windows/Node/npm；`npm --prefix web ci`、`npm --prefix web run build`；`go build -tags webui -o <临时exe> ./cmd/steerline` | 工程及接线完成后，TS/资源与 Go 编译通过 | WEB-A1 / 构建日志 |
| WEB-A2、A4 | L1 Web owner 与定向 HTTP `web.server` | 隔离 Go fixture；`go run ./cmd/test-feedback -- ./web/server ./internal/product/httpapi ./internal/product/httpclient -count=1`；受影响 app 定向测试 | 认证/映射接线后，非法请求和原协议断言实际命中，预算与取消保留 native 认证，无业务替代 | WEB-A2/A4 / runner summary |
| WEB-A4 / 必要 app 受理回归 | 定向 `web.decision-wiring` | `go run ./cmd/test-feedback -- ./internal/app -run 'Test(Decision|Answer|NativeAsync|AsyncDecision)' -count=1`；真实浏览器同会话 publish feedback | store 复用改动后，原问题/答案、attempt/recovery 资格及 native 实例核验通过，浏览器确实产生原 session 的第四轮 | WEB-A4 / app runner、browser evidence |
| WEB-A4、A6 / PR 轮询与原反馈 | 定向 `web.feedback-preservation` | `go run ./cmd/test-feedback -- ./internal/execution -run 'Test(CandidateOpenPRPoll|CandidateClosedPRPoll|WorkerFeedbackPhase|WaitingPRFeedback)' -count=1`；隔离 Go fixture；另 web.smoke | 原决策保留修复后，先证明新增场景失败再修复通过；重复 PR 轮询不替换 answer_saved，不产生额外轮次，浏览器提交后确实完成原会话第四轮 | WEB-A4/A6 / 定向 summary、浏览器证据 |
| WEB-A3、A5 | 定向前端 `web.client`、`web.input` | Vitest/隔离 IndexedDB；`npm --prefix web test` | 客户端/组件实现后，未知/重连/回执/输入/候选反例通过 | WEB-A3/A5 / Vitest 日志 |
| WEB-A3、A5 / 审计修复 | 定向 `web.audit-races` | 同上命令；client、Answer、App 的真实延迟 Promise/持久化场景 | PR 候选前，调用方修改不改变已提交信封、重复存储观察不堆积对账、取消缺失查询不重发、四类资格取消不发旧回答、错误 final 分页不混身份 | WEB-A3/A5 / 审计日志 |
| WEB-A4、A6 的驱动能力 | E2E 基础设施 `web.history-fixture` | `go run ./cmd/test-feedback -- ./tests/e2e/internal/controlled -count=1` | 夹具扩展后，原线程一次 publish feedback、严格普通序列与 `thread/turns/list` 单轮分页/只读事实通过 | WEB-A4/A6 / fixture runner summary |
| WEB-A4、A5、A6 | 定向浏览器 `web.smoke` | 新包；`scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:<本次固定空闲端口>`；包 `start.bat --simulate`；`npm --prefix web run test:e2e`，以 STEERLINE_WEB_CONFIG 指定该包配置、STEERLINE_HOME 指向该包 home | 真实静态包和服务启动后，Playwright 原生浏览器完成本机授权/任务候选/回答/final/刷新/离线；另实际截图检查宽/窄布局，清理本包进程 | WEB-A4–A6 / 本次临时 Playwright 产物、包 evidence |
| 回归 | 固定 L1 + Web owner | `go run ./cmd/test-feedback -level=L1` | 最终候选，现有边界与新 owner 通过 | 回归 / summary |
| 文档 | 静态 | `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | 文档与实现完成后，0 错误 | 静态摘要 |

实施中必须补齐 browser driver、simulate 指向及 Web owner 登记，不能只留下未来补验。前端场景为持续保留的定向入口，不假冒现有 L3 注册场景。除原反馈决策保留修正外无执行规则或远端平台变更，不选择全量 Full/L2/live/全平台矩阵；本机 simulate 浏览器场景覆盖本批真实接线，既有 L1/HTTP 定向覆盖回归。

WEB-M1 由用户在自动验证后参与，桌面输入法与原生查看窗口体验待安排；记录实际构建/环境，仅阻塞该人工体验签收，不阻塞无关后续开发。Plan 不维护实时结果。
