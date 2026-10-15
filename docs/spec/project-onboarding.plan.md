# Plan：项目准备、结构核验与正式纳管（后续批次）

范围归 [Spec](project-onboarding.spec.md)，结果归[专属验收](../acceptance/project-onboarding.md)。用户明确后置，本轮仅规划，不实施，不纳入 roadmap。当前调查基线为 `b657baf39d82ff33e47c7175799421c340748a8d`；实施必须先采用已交付的[前置批次](web-project-switch.plan.md)实际 commit，再核对接口差异。已读取 `docs/spec/SpecGuide.md` 全文，最后修改 commit `d633d1110b4a54c555aa9f67dedb779a7cf9d807`，SHA256 `8b340e17b0f402e924b3ddcb20e91e902bba0421b425921bd7cc62d946d45dd3`。

## 前置与整合

2026-10-15 新增前置整合约束：本任务采用[统一宿主 Plan](unified-startup.plan.md)已实现/验收的固定版本，消费应用 v2、进程内项目端口及成员准入/装载 owner；原调查版本不作为恢复独立项目进程或 TUI 的依据。登记不自动运行/派发，热加载不由统一启动批次提前实现。正式实施前同步本 Plan 的实际代码调查、接口及测试命令，不把旧应用协议当兼容路径。本次衔接维护重读 SpecGuide，版本与 hash 归统一宿主 Plan 的输入记录。

必须已有 Go 应用 host、应用认证/协议、现有项目目录和独立 Web 入口。前置只提供“已配置项目”访问，没有结构核验或建库准入；本任务将其扩展为显式登记，不追溯改写前置验收。

结构规则归[project-structure.md](../product-design/project-structure.md)，模板归[project-onboarding-prompt.md](../product-design/project-onboarding-prompt.md)。项目逻辑进入 Go 应用层，不进入 Web/TUI。复用 `task_documents.go` 的精确 commit 普通 blob 读取、Task 模型的规范路径/摘要能力、`service_preflight.go`/`native_state_preflight.go` 的具体参数/状态检查、`native_project_open.go`/SQLite 的原生权威初始化；不构造假 TaskContract，不复制 Task/DAG 或 worker 流程。

现有 `OpenNativeTaskProject` 和宿主启动可以直接初始化，本任务必须统一关住未纳管新初始化旁路；只加检查页面不足以交付。当前格式库复用只读校验，实际项目写者继续由原服务拥有；新建库和应用登记分别提交、分别留事实。

## 实施步骤

1. 在前置应用协议中固定准备、核验、登记及原请求 DTO/错误码和消费者；确定正式登记的唯一原子发布记录、请求持久事实及其应用级路径。明确“configured/待核验/managed”和连接在线状态独立，不向项目运行目录添加未支持的文件类型。
2. 将可复用 Git/路径/状态核验下沉为实际共用窄能力，增加只读清单解析器及 PS-01–PS-07。明确文件/聚合限额、超时、完整 commit、工作区准备差异及语义检查限制。核验调用栈不得触达 SQLite 初始化、执行目录探测、仓库脚本或模型；新增测试检测真实文件副作用。
3. 根据模板生成自足提示词，填入已确认输入并标记缺口；不把自由文本作为 shell 片段，不自动运行提示词。建立服务端核验结果及依据快照，用户选择/分支改变后显式失效。
4. 实现登记单写及规范 common-dir 并发控制。最终提交重新核验；缺库时在既有仓库锁下初始化，存在当前合法库时只读复用；发布正式目录成员。应用请求记录识别自己的部分完成状态，并提供原 ID 查询与续补，不增加后台无限重试或新 ID 重放。
5. 整合目录和所有新初始化/新任务入口的准入：未登记候选不可初始化或制造新执行，存量 configured 行需显式核验。保留既有执行事实及原反馈/资源操作，不改 worker 流程/恢复。核对无活动服务时与复用活动服务库时两种所有权场景，避免“等待仓库锁而又禁止反馈”的死锁。
6. Web 增加添加面板、准备文本、逐项结果、确认摘要及原请求状态；未成功登记不进入正式成员，不声称结构通过即执行就绪。取消保留用户输入，不删除仓库。TUI 不增加上述流程，只继续消费允许的已有执行能力。
7. 新增隔离核验/登记测试及最小故障注入：初始化后发布失败、发布后丢回执、同 ID 重复与同仓并发，证明只读/原身份/无重复。故障点放测试依赖/存储适配边界，不增加产品测试 HTTP 后门。
8. 扩展唯一调试生成器为“待接入”夹具（拟增 `-ProjectOnboarding`，与 Web 包模式结合），产生隔离空 etc/候选 SteerlineTest clone、有效全局参数和独立状态位置；提示词准备结果由测试驱动按规范生成真实文档/commit，不调用模型。沿用生成器 `--simulate` 冻结，不手工拼安装现场。
9. 新增 Edge `web/tests/project-onboarding.spec.ts`，可执行零项目首添与已有项目切换/失败不污染两条必要路径；新增小型准入 owner 场景进入 L1，跑下表并登记本任务结果。实际用户 Codex 体验单列，不等待它阻碍无关后续。

## 测试计划

以下新增接口、场景及脚本参数尚不存在，须按步骤补齐后再执行；零匹配不计通过。验收按 ID 链接[专属记录](../acceptance/project-onboarding.md)，原始证据写新临时目录或新独立包 evidence。实施时不得复用损坏旧库当成功新场景，也不改安装目录。

| 验收项 | 级别 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| [PON-A1](../acceptance/project-onboarding.md) | 定向 `project-onboarding.prompt` | `go run ./cmd/test-feedback -- ./internal/app -run '^TestProjectOnboardingPrompt' -count=1`；`npm --prefix web test` | 模板及面板完成后；输入与版本完整、缺口不猜测、无命令执行/TUI 添加入口，自由文本不变可执行参数 | PON-A1；runner/Vitest 日志 |
| [PON-A2](../acceptance/project-onboarding.md) | 隔离 Git `project-onboarding.validation` | `go run ./cmd/test-feedback -- ./internal/app -run '^TestProjectOnboardingValidation' -count=1` | 核验器完成后；PS 正反例/固定 commit/链接与映射通过；文件/DB 调用与目录前后快照证明无初始化 | PON-A2；规则命中表及文件快照 |
| [PON-A3、PON-A4](../acceptance/project-onboarding.md) | 定向 `project-onboarding.registration` | `go run ./cmd/test-feedback -- ./internal/app ./internal/product/applicationhttp -run '^TestProjectOnboardingRegistration' -count=1` | 所有者/提交完成后；并发/stale/重复/异主旧库拒绝、当前库复用、CLI/host 准入及故障后同 ID 续补通过，既有写者不被争夺 | PON-A3、PON-A4；原请求、登记/authority/文件事实、runner summary |
| [PON-A1、PON-A4](../acceptance/project-onboarding.md) | Vitest `project-onboarding.ui` | 隔离存储；`npm --prefix web test` | UI 完成后；未通过不可添加、提交丢回执保留原 ID、关闭/切项目不重发、结构/纳管/就绪分开 | PON-A1、PON-A4；组件反例日志 |
| [PON-A2、PON-A3、PON-A4、PON-A5](../acceptance/project-onboarding.md) | 真实浏览器 `web.project-onboarding` | `./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:<空闲固定端口> -ProjectOnboarding`，新包 `start.bat --simulate`；绑定包后 `npm --prefix web run test:e2e -- project-onboarding.spec.ts` | 全接线完成后；零项目首次准备/失败/确认、真实文档 commit 通过、建库后原权威/登记、已有项目隔离、原 ID 与清理均成立 | PON-A2、PON-A3、PON-A4、PON-A5；截图/trace、提交及数据库/登记前后事实、退出/cleanup |
| [PON-A1、PON-A2、PON-A3、PON-A4、PON-A5](../acceptance/project-onboarding.md) | 必要回归 | `go run ./cmd/test-feedback -level=L1`；`go run ./cmd/test-feedback -- ./internal/app ./internal/task ./internal/product/applicationhttp ./web/server -count=1`；`npm --prefix web run build`；另新包复跑 `project-switch.spec.ts` | 最终候选；原任务文档/初始化边界、认证、项目切换和构建保留，新小型 owner 测试已登记 | 回归摘要及前置场景证据 |
| 文档 | 静态 | `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | 收尾；0 错误 | 静态摘要 |

不选择 Full、全部 L2/L3、真实模型/真实 PR 或远端矩阵；本批为本机文档准入/登记，隔离 Git/SQLite、故障注入和真实 Edge 足以覆盖自动项。PON-M1 由用户在自动通过后使用实际生成提示词准备隔离 SteerlineTest 仓库，再回产品核验，窗口待安排；仅阻塞模板体验签收，机器夹具不冒充该人工结果。本 Plan 不记录实时通过状态。
