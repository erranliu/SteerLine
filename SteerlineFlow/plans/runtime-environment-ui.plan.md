# Plan：执行方案与项目运行环境界面梳理

2026-10-31。对应 [Spec](../spec/runtime-environment-ui.spec.md)；[专属验收](../testing/acceptance/runtime-environment-ui.md)。状态：已按审计收窄范围实施；Web/owner/浏览器和固定 L1/L2 通过；历史 L2 两次失败及后续修复/有界复现已分别留证，Windows 原始 Access denied 根因未定；不启动 roadmap、不操作安装现场。

## 1. 基线与最小改动

调查基线为 `cb0c888e7ed6ccea3f9d5c9e2e6427cc3884d0a6`；实施时重新固定实际源码 SHA 和本 Spec/Plan 版本；本轮已按该基线实施，具体结果见专属验收。

| 现有位置 | 拟调整内容 |
| --- | --- |
| `web/src/features/execution-configs/execution-configs.tsx` 及 CSS/测试 | 已保存方案按运行机分组，目录文案与保存/加载事实分离；保留编辑、默认、删除、未保存提示和返回 |
| `web/src/features/environment-checks/use-readiness.tsx` | 共用 stage 文案及当前观察状态，顶部就绪计数/空态/失败缓存；继续复用原 Query key 和轮询 |
| `web/src/features/environment-checks/project-environment.tsx` 及测试 | 单一材料区、运行机分组、卡片操作区、结果摘要/详情与历史定位；删除原全局验收对象选择器 |
| `web/src/features/settings/settings.tsx`、`web/src/app/app-shell.tsx` 及 app 测试 | 运行环境主导航、快捷入口及面包屑统一；保留全局/项目配置分工 |
| `web/src/features/tasks/task-detail.tsx`、`features/tasks/dispatch.tsx`、`features/runs/runs.tsx` 及测试 | 对应环境状态、带上下文定向准备链接，返回任务及 `start_profile` 恢复 |
| `internal/app/environment_readiness.go` 及原 owner 测试 | 只补充原 target/adapter/agent、activity_settled 与从文档 manifest 读取的 script_path 展示字段；不改环境键、资格或持久化 |
| `web/tests/configuration-navigation.spec.ts` | 扩展原受控 UI 场景，验证环境导航、卡片流程和任务返回，沿原 fixture/临时服务器，不新建通用测试平台 |

执行环境分组继续来自原后端键；运行机展示只按已有 target 标识，不调用主机发现或物理别名解析。若补只读字段，先明确字段与已加载 execution/原活动的一一来源，不直接暴露 settings。按[生命周期](../../docs/product-design/worker-lifecycle.md)保留原请求、未知、停止与结算规则。

## 2. 两步实施与依赖

### 第一步：名称、状态和定向路径（P1–P3）

1. **P1，事实与导航梳理，覆盖 A01–A03。** 固定五类事实和六阶段文案；统一项目“运行环境”入口及目录刷新文案。先用现有 options/活动字段列出界面需求，仅补确实缺失的只读字段。分组渲染不改变配置数组顺序或编辑保存语义。
2. **P2，观察有效性，覆盖 A03–A04。** 在原 readiness hook/展示调用方共用有限的 loading/error/cached/offline 表达，不建全局健康状态。将最后成功时间与失败缓存一并展示；在依赖 options 的新操作上处理待核实。已绑定活动的停止继续依赖其自己的原查询与授权。回归顶部、派发栏、任务面板，不能只改环境页。
3. **P3，任务闭环，覆盖 A07。** 从任务或默认方案生成仅含 profile 的链接；任务来源另带 task。按当前项目校验并定位，刷新后恢复。返回同项目 runs 的原任务，通过 `start_profile` 恢复方案选择；不新建 Task 字段或全局持久化草稿。配置变化、方案删除和无效 ID 必须给出具体提示，零自动执行。

P2/P3 依赖 P1 的事实约定。完成后先跑相关 Web 定向用例，确认旧 ready、无效参数和上下文变化不能造成误操作，再进入第二步。

### 第二步：环境页与卡片流程（P4–P5）

4. **P4，材料及卡片操作，覆盖 A02、A05–A06。** 将原材料阅读/生成/导出集中到一个区域；按机器分组环境卡片，关联模型方案仍保留。开始/重新验收、超时、方案选择和失效原因全部绑定卡片。共用环境多个方案按带入方案→组内默认→要求选择的顺序处理。复用原 send、request pending 和刷新，不复制命令仲裁逻辑。将 current revision 和 activity revision 分开传递；显式失效且活动未结算时，显示停止而禁止新验收。
5. **P5，结果层级及交互收口，覆盖 A06、A08。** 先显示检查/L2/原 PR 发布/收尾摘要，原始文件与技术诊断折叠。当前和历史报告按原活动 ID 定位、展开并聚焦，覆盖最近列表之外的记录；旧未结算活动保持可停止。补齐窄屏、长名称/路径、键盘操作与非颜色状态表达。

P4 依赖 P1–P3，P5 依赖 P4；均由当前聊天的后续实现工作完成，不创建并行调度任务。按用户本轮授权执行。

## 3. 验证安排

遵循[测试指南](../testing.md)和[路径清单](../paths.md)。自动验收与必要回归分别记录；不以快照更新或文案字符串匹配代替状态、对象和命令断言。纯文案不新增镜像测试，测试集中验证误操作、状态来源、参数/作用域及导航闭环。

以下均为本轮实施验证入口。输出 scope 建议 `C:\SteerlineDebug\wt-5e65-runtime-env-ui`，每次使用新绝对叶目录；命令中的占位路径执行前替换，记录实际命令、SHA、非零测试数、退出码和清理。共享源码的 Web 构建、浏览器运行及包生成串行。

| 时点与性质 | 验收映射 / 测试 ID | 环境与命令 | 通过条件与证据 |
| --- | --- | --- | --- |
| 文档收口 | 文档一致性，不代表 A 项通过 | 源码 checkout：`python scripts/check_doc_links.py`、`git diff --check`；检查新增文档 UTF-8、表格、相对链接、Spec/Plan/验收 ID 完整映射 | 链接及空白检查通过；文档检查不作为产品通过；实际结果登记专属验收 |
| P1–P5 定向自动 | REU-A01–A07；现有 Web owner 测试，无新增受管 ID | `npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx src/features/execution-configs/execution-configs.test.tsx src/features/tasks/task-detail.test.tsx src/features/runs/runs.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx` | 在原文件扩展状态和命令反例；首次失败/失败缓存/离线恢复、失效且未结算、同机多环境、方案消失及跨项目/实例准确；日志存 diagnostics 新目录 |
| 仅补 app DTO 时，自动 | REU-A02/A03/A06 的字段来源和敏感数据边界；扩展原 `TestProjectEnvironmentReadinessLocalAndModelGrouping` | `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/app -run TestProjectEnvironmentReadinessLocalAndModelGrouping -count=1` | 原 owner 非零命中；字段由已加载 execution/原活动派生、不含凭据、不改原资格；未改 DTO 则明确记不适用 |
| 代码修改后及提交前，必要回归 | `l1.minimum`、`smoke.delivery` | `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`；`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 固定集合通过、无 skip/零命中或未知清理；summary/managed 及原日志定位登记 |
| P5 后，自动及 Web/HTTP 回归 | REU-A01–A07；既有 Web 专项 | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-web.ps1 -EvidenceDir <新Web目录>` | 全部 Web 测试、HTTP owner 和生产构建通过；保留 build/client/server 日志，server 的原报告按其实际返回路径登记 |
| 构建完成后，受控浏览器自动 | REU-A01/A02/A04–A08；扩展既有 `configuration-navigation.spec.ts` | Windows、Node/npm、Edge；`npm --prefix web run test:e2e -- configuration-navigation.spec.ts` | 实际渲染的卡片对象、六状态、任务跳转/刷新/返回、双项目隔离、键盘与宽窄屏断言通过；受控 API，不启动真实 Agent，不代表原生 ready |
| 仅业务门禁规则发生变化时追加 | 原 `testing.project-environment-readiness` | `go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness -out <新L3目录>` | 本轮只调整只读字段和 Web 时不重复执行；原门禁由既有测试维护 |
| 可用构建交付后，后置人工 | REU-M01 | 按专属验收页面走查；参与人和窗口待安排 | 能分清机器/环境/方案、理解阻断并返回原任务；自动已知失败不能转成人工待验 |

受控浏览器沿 `web/playwright.config.ts`，通过 `STEERLINE_WEB_EVIDENCE=<本次仓库外新目录>` 设置 trace/截图输出（或使用原 CLI 的 `--output`）；当前 reporter 为 list，将控制台日志另存本次 diagnostics。设置环境变量时保留并在 finally 恢复原值，不将原始截图或日志提交 Git。执行前核实本机 Edge 及构建输入，不凭命令文本假定真实服务联验。

本任务不改执行、材料、提示词或 Runner，默认不重复本机 Codex/本机 Orca/远端 Orca 的正向 live L4。若实施范围超出只读 DTO 和 Web，先补相应 owner/系统验证并修订范围，不能继承历史原生通过；不为 UI 整理运行全部 L3/L4 或 Full。

## 4. 完成与交付

- 文档交付：Spec、Plan、专属验收及索引同步实际范围；复用原测试入口，不新增受管场景或虚构通过。
- 实现交付：按 A01–A08 逐项登记真实结果与受测 SHA，保留失败和补测的不同原始路径；M01 单独后置，不标全部已验收。
- 保持原配置文件格式、环境键和表结构；无迁移、双写、旧 Check 兼容或人工置 ready。GET/导航/刷新不得增加写操作。
- 如实际触及模型可见正文、接线、资产或对应测试/CI，按[治理规则](../../docs/prompt-governance.md)另附实际正文差异、UTF-8 字节前后、业务原因、验证及独立审核；本轮没有此类修改。
- 本 Plan 不包含提交、发布、部署或安装现场维护步骤；实际实施状态与验收结果分开登记。
