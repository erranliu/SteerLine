# 项目准备、核验与纳管验收（后续批次）

专属任务：[Spec](../../spec/project-onboarding.spec.md) · [Plan](../../plans/project-onboarding.plan.md)。用户明确后置，2026-10-15 仅规划，尚未实现/执行；不与[前置项目切换](web-project-switch.md)混记，无 roadmap Task ID。

| ID | 最小检查与通过条件 | 当前状态 / 证据 |
| --- | --- | --- |
| PON-A1 | 生成完整外部 Codex 准备文本、明确输入/版本/边界；不执行代理/仓库写入，无 TUI 添加入口 | 待实现/待验 |
| PON-A2 | PS-01–PS-07 正反例与现有文档映射；核验选定分支 commit，未提交/越界/链接/大小错误反馈准确，前后无项目初始化 | 待实现/待验 |
| PON-A3 | 通过后用户确认才登记；stale/重复/并发及新初始化旁路守卫，当前合法库明确复用、旧格式/异主库阻断 | 待实现/待验 |
| PON-A4 | 初始化/发布/回执故障保留原 ID 和部分事实；不删库重建、不错误纳管、不争夺既有数据库写者或改写原执行 | 待实现/待验 |
| PON-A5 | 新独立包真实 Edge 首添与已有项目场景；实际文档/commit、原生 DB/登记前后取证及本包清理完成 | 待实现/待验 |
| PON-M1 | 用户用生成提示词在自己的 Codex 准备隔离 SteerlineTest，再回产品核验，体验可理解性/修复反馈；自动通过后参与，窗口待安排，可后置 | 待人工；仅阻塞体验签收 |

## 执行安排与证据

先交付独立 Web/项目切换，再补本 Plan 中的准备/核验/登记、只读副作用检查、故障注入及 `-ProjectOnboarding` 夹具和 Edge 驱动。执行时记录各 ID 对应命令、源码 commit/dirty、包路径、实际结果/限制和原始报告路径；未知、失败、未执行不能标通过。

浏览器采用真实 Go/Web/Edge、Git commit 及 SQLite；测试驱动生成的文档只是外部 Codex 产出的受控替代，不证明模型语义质量。PON-M1 单列实际用户体验。所有产品调试走唯一生成器、SteerlineTest 与 `--simulate`，不使用安装环境或真实 PR。

收尾保存本包 Web/应用/项目服务退出事实、精确 owned 进程与监听为 0。只读核验额外保存核验前后的项目目录/数据库事实；提交故障额外保存原 request ID、唯一登记、authority 及同一操作补齐证据。

本轮文档静态检查不构成产品验收，无 PON-A1–A5 通过记录。

## 已初始化项目的 Web 添加

2026-11-12，基线 `0b0e1bf551aa05322121d6dd271f6804bd7257df` 加本次工作区改动。此批次验证项目切换器、零项目页面及按接入指南核验并保存配置的流程，沿用正常重启装载。

| 验证 | 结果 |
| --- | --- |
| `go test ./internal/app -run '^TestProjectRegistration' -count=1 -v` | 通过。受控 Git HTTP 远端覆盖工作区文件不算已发布、远端更新使旧确认失效、重复确认、重复仓库及重读启动配置。核验和确认均未创建项目状态。 |
| `go test ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -count=1` | 通过。覆盖现有应用 API 和 Web 回归，以及新入口的会话、CSRF 和转发。 |
| `npm --prefix web test -- src/app/project-switch.test.tsx` | 通过。覆盖两个入口、核验后才可确认及保存后的待重启状态。 |
| `npm --prefix web run build` 与本地 Vite/Edge 浏览器检查 | 通过。构建生产资源，并在模拟 API 下检查 1280×800 和 390×844 的入口与表单。 |
| `go run ./cmd/test-feedback -level=L1`、`-level=L2` | 通过。仓库外报告见本次 `project-registration/{L1,L2}/5d86cf854de740b581111b003f529c09/summary.json`。 |

本批次使用隔离 Git 仓库、应用配置和浏览器夹具。正式安装实例中的添加与重启、真实 PR 发布及拟议 PON-A1–A5 的完整方案仍待验证。
