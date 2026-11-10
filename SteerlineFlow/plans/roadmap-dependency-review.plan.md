# Roadmap 草稿任务与依赖审阅 Plan

依据 [Spec](../spec/roadmap-dependency-review.spec.md)。2026-11-09，用户授权审计、修订并实施；普通开发，不分配任务号、不修改 execution-map、不派发 worker。实际结果归专属验收。

2026-11-10，按用户确认精简三视图为固定建议图，下述步骤与测试安排同步更新；前轮验证见专属验收历史节。

## 审计收敛

- 原设计的节点/边分别查看会增加第二套选中状态和重复详情。修订为单一选中任务，边点击和变更入口都定位后继任务，详情列出该任务的全部直接边及变化理由。
- 不因两个页面都画图就抽取通用 GraphPage 适配器、布局框架或新类型文件。沿现有 Roadmap 类型做 type-only 引用，只增加本功能纯投影与组件。
- 移除“减少交叉”的不必要优化承诺，只做拟议图的稳定分层；不新增布局包、持久视角缓存或预览业务状态。React Flow 自己保存挂载期间视角，记录版本切换由组件 key 重建。
- 实际浏览器发现全标题节点与固定侧栏共同缩小图。收敛为全宽画布、两行节点标题、图下详情；完整标题在清单和详情保留，移除侧栏断点及随标题长度增加的布局空间。
- 截图复查发现既有整块吸顶进度栏遮住画布，取消其 sticky 定位，保留原状态与操作，不增加第二套确认按钮。
- 保留别名归一化比较、差异并集不参与拓扑、历史数据隔离和原确认版本，这些是防止误审阅的实际边界，不扩大为前端业务验证器。

## 1. 现有实现与整合方式

| 位置 | 本次安排 |
| --- | --- |
| `web/src/features/roadmaps/roadmaps.tsx` | 复用 record、analysis、existing、source、历史及禁用条件；替换四列长表，将预览、变更和详情交给专属审阅组件；确认仍走原 action。 |
| `web/src/features/roadmaps/workflow.ts` | 保留当前流程/来源变化判定；不为预览引入新的流程状态。 |
| `web/src/features/tasks/dag.tsx` | 复用 React Flow 的既有使用方式、方向和交互经验；正式 DAG 的 GraphPage/启动状态不是草稿数据模型，不把草稿强转成 GraphPage，也不修改正式 DAG 业务。 |
| `internal/roadmap/model/model.go` | 只读参照 `ValidateAnalysis/resolveDependencies` 的身份与别名语义，后端验证和正式写入保持原 owner。 |
| `web/src/features/roadmaps/roadmaps.test.tsx` | 扩展现有组件/命令/快照回归，按新可访问名称更新查询，同时保留原状态、对象和参数断言。 |
| `web/tests/configuration-navigation.spec.ts`、`web/playwright.config.ts` | 复用受控浏览器的生产构建服务和 API 拦截方式，补专属浏览器场景，不使用安装服务或真实模型。 |

只新增 Roadmap 本地的纯投影模块和审阅组件；不引入通用图框架、新布局依赖、预览 API 或缓存权威。固定提示词、材料、黄金样本及其接线无修改。

## 2. 实施步骤

### P1. 固定边集合与身份投影

在 Roadmap feature 内新增 `dependency-review-model.ts` 及纯函数测试。输入仅为选中记录中的分析和已有任务快照；导出现有任务/依赖类型并使用 type-only 引用，不新建类型层。

1. 生成本地/正式命名空间、规划条目到图节点的映射、正式关联别名、完整任务清单与节点显示内容。
2. 分别解析原始、拟议边：关联条目作为正式节点，指向关联条目的本地引用按正式身份比较，边按前置/后继身份去重；保留原始引用供详情呈现。
3. 保留原始/拟议比较集合、依赖变更分组与汇总，单独提供拟议边；删除视图模式类型和切换筛选函数。全部规划条目及孤立节点保留。
4. 以拟议图确定稳定位置，同层稳定排序；仅原始引用的正式节点留给清单、变更及详情，不进入建议图或占布局位置。移除边及原始逆向关系不进入主图或拓扑排序。拟议纯函数输出不包含 Task 启动/就绪状态。

测试使用明确完整的受控数据，不把截图未展示的关系当成产品事实。基础样本含六个本地任务：tools→seed、seed→left/right、left/right→join、tools→independent，加上正式 #101→tools；未引用的 #105 不入图。另覆盖无依赖、关联别名、内部代号 `8` 与正式 `#8`、顺序/重复变化，以及原始 A→B 改为拟议 B→A 的差异并集。

### P2. 实现预览图和审阅详情

新增 `dependency-review.tsx`，使用已安装的 `@xyflow/react`，禁用节点拖拽/连接。组件接收快照与正式任务链接构造器，不发请求、不调用 submit。

主图固定使用拟议边，移除三视图按钮、对应本地状态、差异边样式和图例。保留建议关系的方向说明、节点类别、选择高亮、缩放和适配画布。新记录默认 fitView，同记录轮询不重新自动 fit。

配套保留变更清单、完整任务清单和单一任务详情。仅保存选中节点一项本地状态，连线/变更点击定位后继任务；键盘入口来自原生按钮/链接，正式节点选择与打开正式 DAG 分开。详情始终基于所选记录，完整保留原文、原始/拟议依赖及理由。零变更显示“与草稿依赖一致”。

### P3. 接回原页面与快照生命周期

替换 `roadmap-dependencies` 区域的四列长表；移除仅供旧表使用的重复呈现，保留其他消费者仍需要的依赖显示。折叠完整 overlap 和来源依据，待澄清问题直接可见。增加汇总并调整确认对象文案。

审阅组件以项目/实例、source.id、record.id/revision 绑定数据与本地状态；记录或修订改变时重新建立视图，普通同版本轮询保持选择。不能在选中历史详情尚未返回时回用当前分析。当前、历史、已入图标识与原 stale/locked/guidance 一致。

确认继续调用原 `action("roadmap_confirm", record.id, record.revision)`；查看任务或变更仍确认同一完整拟议规划。原按钮禁用门槛不放宽，拒绝/未知沿原处理。布局样式收在 `roadmaps.css`，使用现有主题变量；全宽图配图下详情，图画布独立平移，清单及长文本不溢出。

### P4. 补齐验收并收口

扩展现有 Roadmaps 组件测试并新增投影测试。新增 `web/tests/roadmap-dependency-review.spec.ts`，沿既有受控浏览器方式服务真实生产构建、拦截 API；测试服务只绑定回环随机端口，测试结束关闭服务。浏览器记录命令请求，覆盖预览零写入及显式确认的原目标/版本。

浏览器覆盖 1440px、768px、320px，长标题、选中高亮、固定建议图、变更与原始详情、键盘清单；断言三按钮不存在、被移除的边及仅原始正式引用不入图，轮询保持坐标/视角/选择，零变更有说明。保存截图、trace、命令事实和实际构建来源。该入口为专属补充验收，不新增分级测试集或将其冒称为既有 `web.unit` 的浏览器覆盖。

实施后按[专属验收](../testing/acceptance/roadmap-dependency-review.md)逐项登记受测提交、命令、结果、证据及限制；允许后置的体验项独立保留。删除旧重复表后核对全部正文仍可达，不以更新快照代替行为断言。

## 3. 测试计划

正式 ID、完整命令/环境及集合断言以 [test-list.json](../testing/test-list.json) 为准；有序选择见 [roadmap-dependency-review.tests.json](roadmap-dependency-review.tests.json)。本轮仅改前端展示，运行固定 L1/L2、Web 全集、构建及专属受控浏览器，并按测试指南保留 Roadmap owner 回归。前轮文档检查不替代本轮产品测试。

| 时点与性质 | 验收映射 / ID | 执行安排、通过条件与证据 |
| --- | --- | --- |
| 文档交付，静态检查 | 本轮文档一致性，非产品验收 | `python scripts/check_doc_links.py`、`git diff --check`；检查同名测试安排 ID 均已登记且有序唯一、Spec/Plan/验收映射完整。结果单独登记。 |
| P1–P3 后，定向自动 | RDR-A01–A04、A06；将由 `web.unit` 全集覆盖 | `npm --prefix web test -- src/features/roadmaps/dependency-review-model.test.ts src/features/roadmaps/roadmaps.test.tsx`；执行前须补齐 P1/P3 用例。身份/边集/别名/计数/零变更/详情/快照/确认参数及禁用反例实际通过、非零、无 skip。原始日志存新 diagnostics 目录。 |
| P4 后，功能自动及前端回归 | RDR-A01–A04、A06；`web.unit` | 采用正式清单的 L4 Web 入口并显式传全新 `-out`；全部现有及新增 Vitest 测试通过，原生 JSON/stdout/stderr 可定位。 |
| P4 后，Roadmap owner 回归 | RDR-A04/A06；`go.roadmap` | 按测试指南和正式清单运行 L4 Roadmap 入口并显式传新 `-out`，确认/别名/发布/入图与原请求边界通过；三组串行及排除 live 规则不变，不替代前端产品验收。 |
| 代码完成及提交前，固定回归 | RDR-A06；`l1.minimum`、`smoke.delivery` | 按正式 L1/L2 入口分别使用全新 `-out`，真实选择、非零终态、零 skip、清理和退出码满足各自定义。 |
| 生产构建后，专属补充浏览器自动 | RDR-A01–A06，尤其 A05 | `npm --prefix web run build` 后执行 `npm --prefix web run test:e2e -- roadmap-dependency-review.spec.ts`。Windows、Node 24+、npm ci、Edge；真实 React Flow 渲染和交互、宽窄屏、键盘、零额外写入及原确认命令通过，无 pageerror；服务结束释放端口。受控 API 不代表模型/真实入图链已验。 |
| 自动验收通过后的体验走查 | RDR-M01 | 按专属记录后置安排；参与人/窗口待安排，不阻塞自动实现交付，不以人工待验掩盖自动失败。 |

本轮证据范围使用 `<系统临时目录>/SteerlineDebug/wt-3804-roadmap-simplification`，每次调用按[路径清单](../paths.md)建立独立叶目录，绝对位置登记专属验收。正式分级日志以 `summary.json` 和 managed 原报告为准；定向 Vitest/浏览器/构建分别保留日志，不冒充登记集合整体通过。Playwright 用 `STEERLINE_WEB_EVIDENCE` 指定本次新目录，设置前保存旧值并在 finally 恢复；不将截图、trace 或原始日志提交 Git。构建、浏览器、固定及 Roadmap 重型回归串行执行。

无需真实模型、GitHub PR 或安装环境；不选择全部 L3/L4 或 Full。若实施超出前端展示范围，先修订本设计及所需 owner/系统验证，不能从本计划推定新的业务改动授权。

## 4. 文档与实现交付

交付包括修订后的 Spec、Plan、同名测试安排、前端实现、行为测试、专属验收及索引。按 Spec 核对直接边和详情完整性，实际结果归专属验收；设计审计和行为验证分开登记，不据此声称独立审核通过。本 Plan 不包含提交、推送、提 PR、合并或部署步骤。
